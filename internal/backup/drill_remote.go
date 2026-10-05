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
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
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
	Warnings         []string // why source counts were not used, key mismatches
	Duration         time.Duration
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

// DrillRemote runs the drill. It returns a non-nil result whenever the restore
// was attempted, with the error that made it fail. A cancelled ctx (SIGINT,
// SIGTERM) unwinds through the defers that remove the container and the
// decrypted files and writes no heartbeat.
func DrillRemote(ctx context.Context, o DrillRemoteOptions) (*DrillRemoteResult, error) {
	start := time.Now()
	if !heartbeatProjectRe.MatchString(o.Project) {
		return nil, fmt.Errorf("project name %q cannot be used as a heartbeat key", o.Project)
	}
	if err := dockerCmd(ctx, "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		// errs.New, not Wrap: a wrapped *exec.ExitError would set the exit class.
		return nil, errs.New("E220", fmt.Sprintf("Docker is not available for the drill container: %v", err))
	}
	sweepStaleDrills(ctx)
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
	lctx, cancel := fetchContext(ctx, 0)
	objs, err := src.List(lctx, "")
	cancel()
	if err != nil {
		_, cause := classifyFetch(lctx, err)
		return nil, fmt.Errorf("%w: list backups: %s", errs.ErrBackupRemoteFailed, cause)
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
	estimates, warnings := loadEstimates(ctx, hbDest, o.Project, obj)

	hb := Heartbeat{Kind: "drill", Project: o.Project, BackupKey: path.Base(obj.Key), Bytes: obj.Size, Encrypted: encrypted,
		CLIVersion: version.Version, Mismatches: []string{}, Result: "failed", SchemaVersion: HeartbeatSchemaVersion}
	restored, runErr := restoreAndCount(ctx, src, obj.Key, obj.Size, identity, encrypted)
	if runErr == nil {
		hb.RestoredRows = restored
		hb.Mismatches = compareRows(estimates, restored)
		if len(hb.Mismatches) == 0 {
			hb.Result = "ok"
		}
	}
	hb.At = time.Now().UTC().Format(time.RFC3339)
	res := &DrillRemoteResult{Heartbeat: hb, Estimated: len(estimates) > 0, Warnings: warnings}
	for t := range restored {
		res.Tables = append(res.Tables, t)
	}
	sort.Strings(res.Tables)
	if hbDest != nil && ctx.Err() == nil {
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
		if len(estimates) == 0 {
			return res, errs.New("E218", "restore drill cannot verify the backup: "+strings.Join(append(warnings, cannotVerify), "; "))
		}
		return res, errs.New("E218", fmt.Sprintf("restore drill failed: %d table(s) differ from the backup's row counts: %s", len(hb.Mismatches), strings.Join(hb.Mismatches, "; ")))
	}
	return res, nil
}
