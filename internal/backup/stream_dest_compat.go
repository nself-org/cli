package backup

// stream_dest_compat.go — ADR 0021 gate for bare local paths given to backup stream (D-0676).
//
// Purpose: v1.4.13 handed --to, BACKUP_REMOTE, the heartbeat remote and the schedule --to straight to `rclone rcat`, which treats a bare path such as /mnt/backups as its local backend. The destinations grammar (D-0257) only knows path://, so on v1.4 a bare absolute path is mapped to path://<dir> with a one-line notice and keeps working; under compat.V15 it is refused with a hint.
// Inputs: one destination value as the user wrote it.
// Outputs: the value to parse (unchanged unless it is a bare absolute path), or an error under v1.5. The notice goes to bareNoticeOut (stderr) once per value per process.
// Constraints: only a value starting with "/" is a bare path here: every other value already meant the same thing on v1.4.13 or is refused for security (D-0467). Relative paths are not mapped. The error never echoes the value; the notice does, because a local path carries no credentials.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/compat"
)

// bareNoticeOut receives the v1.4 notice. A var so tests can capture it.
var bareNoticeOut io.Writer = os.Stderr

// bareNoticed records the values already noticed in this process, so a run
// that resolves the same destination twice prints one line.
var (
	bareNoticeMu sync.Mutex
	bareNoticed  = map[string]bool{}
)

// isBarePath reports whether uri is a bare absolute local path.
func isBarePath(uri string) bool {
	return strings.HasPrefix(uri, "/")
}

// settleBarePath returns uri unchanged unless it is a bare absolute path. On
// v1.4 a bare path becomes path://<dir> (cleaned, and resolved when the
// directory itself is a symlink, as rclone's local backend did); under v1.5
// it is an error.
func settleBarePath(uri string) (string, error) {
	if !isBarePath(uri) {
		return uri, nil
	}
	// compat.V15(D-0676): bare absolute stream path streams as path:// with a notice -> refused
	if compat.V15() {
		return "", fmt.Errorf("a bare local path is not a backup destination; write it as path:///<dir> (for example path:///var/backups/nself)")
	}
	dir := filepath.Clean(uri)
	if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
	}
	mapped := "path://" + dir
	bareNoticeMu.Lock()
	defer bareNoticeMu.Unlock()
	if !bareNoticed[uri] {
		bareNoticed[uri] = true
		_, _ = fmt.Fprintf(bareNoticeOut, "notice: backup destination %s is a bare path; streaming to %s. Write %s instead: nself v1.5 refuses bare paths.\n", uri, mapped, mapped)
	}
	return mapped, nil
}
