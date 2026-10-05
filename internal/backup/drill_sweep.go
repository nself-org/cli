package backup

// drill_sweep.go — remove what a killed drill left behind.
//
// Purpose: SIGINT and SIGTERM unwind the drill through its defers, but a
// SIGKILL or a crash cannot. Every drill run therefore starts by removing
// stale throwaway containers and stale decrypted temp directories of earlier
// runs, so decrypted backup data never stays at rest on the owner machine.
// Inputs: the context of the drill; the temp directory.
// Outputs: none (best effort, failures are logged).
// Constraints: containers are selected by the label org.nself.drill only and
// removed only when the name is a drill throwaway (throwawayNameRe, a subset of
// removable()), so a live project container or a running `verify
// --restore-test` container is never touched. Both kinds must be older than
// drillStaleAfter, so a drill running right now in another process survives.
// Temp directories must be real directories (no symlink) named nself-drill-<n>,
// mode 0700 and owned by the current user.

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// drillStaleAfter is how old a drill container or temp directory must be to be
// swept. A var so tests can use zero.
var drillStaleAfter = 6 * time.Hour

var drillTempDirRe = regexp.MustCompile(`^nself-drill-[0-9]+$`)

// sweepStaleDrills removes stale drill containers and temp directories.
func sweepStaleDrills(ctx context.Context) {
	sweepDrillContainers(ctx, drillStaleAfter)
	sweepDrillTempDirs(os.TempDir(), drillStaleAfter)
}

func sweepDrillContainers(ctx context.Context, olderThan time.Duration) {
	out, err := dockerCmd(ctx, "ps", "-a", "--filter", "label="+containerLabel, "--format", "{{.Names}}").Output()
	if err != nil {
		slog.Warn("could not list stale drill containers", "error", err)
		return
	}
	for _, name := range strings.Fields(string(out)) {
		if !throwawayNameRe.MatchString(name) || !removable(name) {
			continue
		}
		created, err := dockerCmd(ctx, "inspect", "-f", "{{.Created}}", name).Output()
		if err != nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(created)))
		if err != nil || time.Since(at) < olderThan {
			continue
		}
		slog.Warn("removing a stale drill container", "container", name)
		(&Container{Name: name}).Remove()
	}
}

func sweepDrillTempDirs(dir string, olderThan time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !drillTempDirRe.MatchString(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		fi, err := os.Lstat(p)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 || !ownedByCurrentUser(fi) || time.Since(fi.ModTime()) < olderThan {
			continue
		}
		slog.Warn("removing a stale drill temp directory", "dir", p)
		if err := os.RemoveAll(p); err != nil {
			slog.Warn("could not remove a stale drill temp directory", "dir", p, "error", err)
		}
	}
}

// ownedByCurrentUser reports whether fi is owned by the running user. The uid
// is read by reflection so the file builds on every platform: where Sys() has
// no Uid (Windows) the temp directory is per-user and the answer is true.
func ownedByCurrentUser(fi os.FileInfo) bool {
	v := reflect.Indirect(reflect.ValueOf(fi.Sys()))
	if v.Kind() != reflect.Struct {
		return true
	}
	uid := v.FieldByName("Uid")
	if !uid.IsValid() || !uid.CanUint() {
		return true
	}
	return uid.Uint() == uint64(os.Getuid())
}
