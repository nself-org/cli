package backup

// stream_dest_compat.go — ADR 0021 gate for bare local paths given to backup stream (D-0676).
//
// Purpose: v1.4.13 handed --to, BACKUP_REMOTE, the heartbeat remote and the schedule --to straight to `rclone rcat`, which treats a bare path (/mnt/backups, ./backups, backups) as its local backend. The destinations grammar (D-0257) only knows path://, so on v1.4 a bare path is mapped to path://<absolute dir> with a one-line notice and keeps working; under compat.V15 it is refused with a hint.
// Inputs: one destination value as the user wrote it, and the directory a relative path is resolved against ("" = the working directory, which is the project directory for stream and for the schedule's unit).
// Outputs: the value to parse (unchanged unless it is a bare path), or an error under v1.5. The notice goes to bareNoticeOut (stderr) once per value per process.
// Constraints: a bare path is what rclone reads as local: no ":" before the first "/". Every other value already meant the same thing on v1.4.13, and an option-shaped value (leading "-") or a connection string stays refused for security (D-0257, D-0467). The V15 refusal never echoes the value; the notice quotes it (a local path carries no credentials). path:// then applies its own rules (0600 files in 0700 dirs, no "..", no symlink below the root): an accepted deviation from rclone, see the wiki.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/backup/destinations"
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

// isBarePath reports whether rclone would have read uri as a local path: not
// empty, not option-shaped, and no ":" before the first "/" (so no scheme, no
// name:path remote and no :backend connection string). A value starting with
// "/" is always a path (no rclone remote starts with "/"), commas included.
// A value with padding whitespace or a control character, and a relative
// value with an inline-credential shape (a "," before a ":", e.g.
// "myr,pass=ab/cd:p", or a ",key=" option with no ":", see HasInlineOption), are
// left to the parser, which refuses them without echoing them (D-0467).
func isBarePath(uri string) bool {
	if uri == "" || strings.HasPrefix(uri, "-") || uri != strings.TrimSpace(uri) {
		return false
	}
	if strings.IndexFunc(uri, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return false
	}
	if strings.HasPrefix(uri, "/") {
		return true
	}
	if (strings.Contains(uri, ":") && destinations.HasInlineCredentials(uri)) || destinations.HasInlineOption(uri) {
		return false
	}
	head, _, _ := strings.Cut(uri, "/")
	return !strings.Contains(head, ":")
}

// settleBarePath returns uri unchanged unless it is a bare path. On v1.4 a
// bare path becomes path://<dir>: made absolute against base (or the working
// directory), cleaned, and resolved when the directory itself is a symlink,
// as rclone's local backend did. Under v1.5 it is an error.
func settleBarePath(uri, base string) (string, error) {
	if !isBarePath(uri) {
		return uri, nil
	}
	// compat.V15(D-0676): bare stream path streams as path:// with a notice -> refused
	if compat.V15() {
		return "", fmt.Errorf("a bare local path is not a backup destination; write it as path:///<absolute dir> (for example path:///var/backups/nself)")
	}
	dir := uri
	if !filepath.IsAbs(dir) {
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("resolve a relative destination: %w", err)
			}
			base = wd
		}
		dir = filepath.Join(base, dir)
	}
	dir = filepath.Clean(dir)
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
		_, _ = fmt.Fprintf(bareNoticeOut, "notice: backup destination %q is a bare path; streaming to %q. Write %q instead: nself v1.5 refuses bare paths.\n", uri, mapped, mapped)
	}
	return mapped, nil
}
