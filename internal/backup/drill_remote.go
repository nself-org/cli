package backup

// drill_remote.go — `nself backup drill --from <remote>`: restore the newest
// (or a named) off-box backup into a throwaway container and prove it.
//
// Purpose: a backup that has not been restored is a hope. The drill downloads
// the backup through the Destination interface, decrypts it with an age
// identity that lives on the machine running the drill, restores it into a
// throwaway postgres container, counts rows per table and compares them with
// the estimates in the backup heartbeat, then writes `<project>/drill.json`
// (contract:cli.backup-heartbeat kind drill) to the heartbeat remote.
// Inputs: project name, source destination, optional object key, age identity
// path, heartbeat remote.
// Outputs: a DrillRemoteResult and the drill heartbeat; coded errors E218
// (drill failed) and E220 (cannot start: Docker, age, disk).
// Constraints: never touches the project's own database or containers; the
// only container is the throwaway of restore_container.go, removed on every
// exit path. Plaintext lives in a 0700 temp directory that is removed too.

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
)

// DrillRemoteOptions are the inputs of DrillRemote.
type DrillRemoteOptions struct {
	Project     string
	From        string // destination holding <project>_stream_* objects
	Key         string // object to restore; "" = newest
	Identity    string // age identity file; "" = default locations
	HeartbeatTo string // heartbeat remote; "" = drill.json is not written
}

// DrillRemoteResult is the outcome of a drill. Heartbeat is what drill.json holds.
type DrillRemoteResult struct {
	Heartbeat        Heartbeat
	Tables           []string // restored tables, sorted
	Estimated        bool     // row estimates from the backup heartbeat were used
	HeartbeatWritten bool
	Duration         time.Duration
}

// freeDiskBytes reports the free bytes under dir. A variable for tests.
var freeDiskBytes = func(dir string) (uint64, error) {
	out, err := exec.Command("df", "-Pk", dir).Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	f := strings.Fields(lines[len(lines)-1])
	if len(f) < 4 {
		return 0, fmt.Errorf("unexpected df output")
	}
	kb, err := strconv.ParseUint(f[3], 10, 64)
	return kb * 1024, err
}

// selectBackup picks key, or the newest <project>_stream_ object by name (the
// name carries a UTC timestamp), ties broken by modification time.
func selectBackup(objs []destinations.Object, project, key string) (destinations.Object, error) {
	prefix := project + "_stream_"
	var best destinations.Object
	found := false
	for _, o := range objs {
		base := path.Base(o.Key)
		if !strings.HasPrefix(base, prefix) || (!strings.HasSuffix(base, ".sql") && !strings.HasSuffix(base, ".sql.age")) {
			continue
		}
		if key != "" {
			if o.Key == key || base == key {
				return o, nil
			}
			continue
		}
		if !found || base > path.Base(best.Key) || (base == path.Base(best.Key) && o.ModTime.After(best.ModTime)) {
			best, found = o, true
		}
	}
	if key != "" || !found {
		return best, fmt.Errorf("%w: no %s* backup object found at the source", errs.ErrBackupNotFound, prefix)
	}
	return best, nil
}

// resolveIdentity returns the age identity file: the flag, else the owner
// key locations. The file is never read here, only passed to age.
func resolveIdentity(project, flag string) (string, error) {
	if flag != "" {
		if _, err := os.Stat(flag); err != nil {
			return "", fmt.Errorf("%w: age identity file: %v", errs.ErrBackupDecryptFailed, err)
		}
		return flag, nil
	}
	home, _ := os.UserHomeDir()
	for _, n := range []string{project + "-backup-age.key", "age-key.txt"} {
		p := filepath.Join(home, ".config", "nself", n)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: no age identity found; pass --identity <file>", errs.ErrBackupDecryptFailed)
}

func decryptAge(ctx context.Context, identity, in, out string) error {
	fin, err := os.Open(in)
	if err != nil {
		return err
	}
	defer func() { _ = fin.Close() }()
	fout, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = fout.Close() }()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "age", "--decrypt", "-i", identity)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = fin, fout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", errs.ErrBackupDecryptFailed, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// compareRows lists the tables the backup estimated non-empty that are missing
// or empty after the restore. With no estimates it requires at least one
// non-empty table, so an empty restore never passes.
func compareRows(estimates, restored map[string]int64) []string {
	m := []string{}
	if estimates == nil {
		for _, n := range restored {
			if n > 0 {
				return m
			}
		}
		return append(m, "<no restored table has rows>")
	}
	for t, est := range estimates {
		if est > 0 && restored[t] <= 0 {
			m = append(m, t)
		}
	}
	sort.Strings(m)
	return m
}

// writeDrillHeartbeat puts <project>/drill.json through the destination.
func writeDrillHeartbeat(ctx context.Context, dest destinations.Destination, hb Heartbeat) error {
	data, err := hb.Marshal()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "nself-hb-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	local := filepath.Join(dir, "drill.json")
	if err := os.WriteFile(local, data, 0o600); err != nil {
		return err
	}
	return dest.Put(ctx, local, hb.Project+"/drill.json")
}

// DrillRemote runs the drill. It returns a non-nil result whenever the restore
// was attempted, with the error that made it fail.
func DrillRemote(ctx context.Context, o DrillRemoteOptions) (*DrillRemoteResult, error) {
	start := time.Now()
	if !heartbeatProjectRe.MatchString(o.Project) {
		return nil, fmt.Errorf("project name %q cannot be used as a heartbeat key", o.Project)
	}
	if err := dockerCmd(ctx, "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		return nil, errs.Wrap("E220", "Docker is not available for the drill container", err)
	}
	src, err := OpenDestination(o.From)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}
	var hbDest destinations.Destination
	if o.HeartbeatTo != "" {
		if hbDest, err = OpenDestination(o.HeartbeatTo); err != nil {
			return nil, fmt.Errorf("%w: heartbeat remote: %v", errs.ErrBackupRemoteFailed, err)
		}
	}
	objs, err := src.List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("%w: list backups: %v", errs.ErrBackupRemoteFailed, err)
	}
	obj, err := selectBackup(objs, o.Project, o.Key)
	if err != nil {
		return nil, err
	}
	encrypted := strings.HasSuffix(obj.Key, ".age")
	identity := ""
	if encrypted {
		if _, err := exec.LookPath("age"); err != nil {
			return nil, errs.Wrap("E220", "the age binary is required to decrypt this backup", err)
		}
		if identity, err = resolveIdentity(o.Project, o.Identity); err != nil {
			return nil, err
		}
	}
	free, err := freeDiskBytes(os.TempDir())
	if err != nil || free < 2*uint64(obj.Size) {
		return nil, errs.New("E220", fmt.Sprintf("not enough free disk for the drill: need %d bytes (twice the %d byte backup) in %s", 2*obj.Size, obj.Size, os.TempDir()))
	}
	var estimates map[string]int64
	if hbDest != nil {
		if b, st, _ := fetchHeartbeat(ctx, hbDest, o.Project, "backup", time.Now()); st == hbOK && b.BackupKey == path.Base(obj.Key) {
			estimates = b.ApproxRows
		}
	}

	hb := Heartbeat{Kind: "drill", Project: o.Project, BackupKey: path.Base(obj.Key), Bytes: obj.Size, Encrypted: encrypted,
		CLIVersion: version.Version, Mismatches: []string{}, Result: "failed", SchemaVersion: HeartbeatSchemaVersion}
	restored, runErr := restoreAndCount(ctx, src, obj.Key, identity, encrypted)
	if runErr == nil {
		hb.RestoredRows = restored
		hb.Mismatches = compareRows(estimates, restored)
		if len(hb.Mismatches) == 0 {
			hb.Result = "ok"
		}
	}
	hb.At = time.Now().UTC().Format(time.RFC3339)
	res := &DrillRemoteResult{Heartbeat: hb, Estimated: estimates != nil}
	for t := range restored {
		res.Tables = append(res.Tables, t)
	}
	sort.Strings(res.Tables)
	if hbDest != nil {
		if werr := writeDrillHeartbeat(ctx, hbDest, hb); werr != nil {
			slog.Warn("could not write the drill heartbeat", "error", werr)
			if runErr == nil && hb.Result == "ok" {
				runErr = fmt.Errorf("%w: drill passed but drill.json could not be written: %v", errs.ErrBackupRemoteFailed, werr)
			}
		} else {
			res.HeartbeatWritten = true
		}
	}
	res.Duration = time.Since(start)
	if runErr != nil {
		return res, runErr
	}
	if hb.Result != "ok" {
		return res, errs.New("E218", fmt.Sprintf("restore drill failed: %d table(s) missing or empty after restore: %s", len(hb.Mismatches), strings.Join(hb.Mismatches, ", ")))
	}
	return res, nil
}

// restoreAndCount downloads, decrypts, restores into a throwaway container
// and counts rows. Every temporary file and the container are removed on return.
func restoreAndCount(ctx context.Context, src destinations.Destination, key, identity string, encrypted bool) (map[string]int64, error) {
	tmp, err := os.MkdirTemp("", "nself-drill-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	file := filepath.Join(tmp, "backup.dl")
	if err := src.Get(ctx, key, file); err != nil {
		return nil, fmt.Errorf("%w: download: %v", errs.ErrBackupRemoteFailed, err)
	}
	if fi, err := os.Stat(file); err != nil || fi.Size() == 0 {
		return nil, fmt.Errorf("%w: downloaded backup is empty", errs.ErrBackupRemoteFailed)
	}
	if encrypted {
		plain := filepath.Join(tmp, "backup.plain")
		if err := decryptAge(ctx, identity, file, plain); err != nil {
			return nil, err
		}
		_ = os.Remove(file)
		file = plain
	}
	c, err := StartContainer(ctx, ContainerSpec{})
	if err != nil {
		return nil, err
	}
	defer c.Remove()
	if err := RestoreIntoContainer(ctx, c, file); err != nil {
		return nil, err
	}
	return CountRows(ctx, c)
}
