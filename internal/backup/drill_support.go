package backup

// drill_support.go — the helpers of `backup drill --from` that touch disk,
// age and the remotes: free-space check, identity lookup, decryption, the
// drill heartbeat write and the download-decrypt-restore-count pipeline.
// It also holds the bounded remote reads (fetchContext) and the classification
// of their errors by exit code (classifyFetch): rclone exit 3 (directory not
// found) and 4 (file not found) and fs.ErrNotExist (path:// and host://) mean
// "missing"; everything else is a failing remote and surfaces as E219.
// Constraints: plaintext lives only in a 0700 nself-drill-* temp directory
// that is removed on every return path (and swept by sweepStaleDrills after a
// SIGKILL); remote reads and writes are bounded by fetchContext.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/errs"
)

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
	pctx, cancel := fetchContext(ctx, 0)
	defer cancel()
	return dest.Put(pctx, local, hb.Project+"/drill.json")
}

// restoreAndCount downloads, decrypts, restores into a throwaway container
// and counts rows. Every temporary file and the container are removed on return.
func restoreAndCount(ctx context.Context, src destinations.Destination, key string, size int64, identity string, encrypted bool) (map[string]int64, error) {
	tmp, err := os.MkdirTemp("", "nself-drill-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	file := filepath.Join(tmp, "backup.dl")
	gctx, cancel := fetchContext(ctx, size)
	err = src.Get(gctx, key, file)
	cancel()
	if err != nil {
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
	if err := ctx.Err(); err != nil {
		return nil, err
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

// remoteFetchTimeout bounds one remote List/Get/Put. Unexported and a var so
// tests can shorten it; it is not a flag.
var remoteFetchTimeout = 60 * time.Second

// fetchContext bounds one remote read. A large object gets one extra second
// per MiB (a 1 MiB/s floor) on top of the base, so a real backup download is
// bounded but not cut off.
func fetchContext(ctx context.Context, size int64) (context.Context, context.CancelFunc) {
	d := remoteFetchTimeout
	if size > 0 {
		d += time.Duration(size>>20) * time.Second
	}
	return context.WithTimeout(ctx, d)
}

// classifyFetch reports whether err means "no such object" and, if not, a
// short cause. fctx is the bounded context the read ran under.
func classifyFetch(fctx context.Context, err error) (missing bool, cause string) {
	var ee *exec.ExitError
	switch {
	case errors.Is(fctx.Err(), context.DeadlineExceeded):
		return false, "the remote did not answer within the time limit"
	case fctx.Err() != nil:
		return false, "the read was interrupted"
	case errors.Is(err, fs.ErrNotExist):
		return true, ""
	case errors.As(err, &ee) && (ee.ExitCode() == 3 || ee.ExitCode() == 4):
		return true, ""
	case errors.As(err, &ee):
		return false, fmt.Sprintf("rclone exited with status %d", ee.ExitCode())
	case errors.Is(err, exec.ErrNotFound):
		return false, "rclone is not installed"
	}
	return false, "the remote could not be read"
}

// problemText is the one-line form of a status problem for the JSON
// `problems` array: "[E219] what", without the Why/Fix/Docs lines.
func problemText(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return fmt.Sprintf("[%s] %s", ce.Code, ce.What)
	}
	return err.Error()
}
