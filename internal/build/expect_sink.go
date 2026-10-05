package build

// expect_sink.go — a write-mode Sink that holds a build to a confirmed render
// (P7-LIVE-03, review M3 / EPIC ruling B).
//
// Purpose: `nself build` shows a plan, asks for confirmation, then writes. The
// write must be that render and nothing else, even if an input changed after
// the plan was made. expectSink wraps the disk sink: every write must carry
// exactly the bytes the confirmed render planned for that path, every removal
// and chmod must be one it planned, and anything else stops the build with an
// error before the offending write. After the build, verify checks that the
// disk holds every planned file and that every planned removal is gone.
// Inputs: the PlannedBuild of the confirmed render (BuildOptions.Expect).
// Outputs: errors naming the path that deviated.
// Constraints: write mode only; reads and directory creation pass through.
// Effects are not asserted here (they run through writeEffects).

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// deviationError is a write, removal or chmod that is not part of the
// confirmed render.
type deviationError struct{ msg string }

func (e *deviationError) Error() string { return e.msg }

// errDeviated builds a deviationError.
func errDeviated(path, why string) error {
	return &deviationError{msg: fmt.Sprintf("the project changed after the plan was made: %s %s; nothing further was written (re-run nself build --plan)", path, why)}
}

// expectSink is the checking disk sink.
type expectSink struct {
	*diskSink
	exp  *PlannedBuild
	keyr *memSink // canonicalises paths exactly as plan mode did
}

func newExpectSink(root string, exp *PlannedBuild) *expectSink {
	d := newDiskSink(root)
	return &expectSink{diskSink: d, exp: exp, keyr: &memSink{diskSink: diskSink{root: root}}}
}

// setFronting tells the sink where the fronting stack's nginx/sites dir is.
func (e *expectSink) setFronting(dir string) { e.fronting, e.keyr.fronting = dir, dir }

func (e *expectSink) WriteFile(p string, data []byte, perm fs.FileMode) error {
	if err := e.check(p, data); err != nil {
		return err
	}
	return e.diskSink.WriteFile(p, data, perm)
}

func (e *expectSink) WriteAtomic(p string, data []byte, perm fs.FileMode) error {
	if err := e.check(p, data); err != nil {
		return err
	}
	return e.diskSink.WriteAtomic(p, data, perm)
}

// check requires the planned bytes for p.
func (e *expectSink) check(p string, data []byte) error {
	k := e.keyr.key(p)
	if e.backupPath(k) {
		return nil
	}
	f, ok := e.exp.Files[k]
	if !ok {
		return errDeviated(k, "was not in the plan")
	}
	if !bytes.Equal(f.Data, data) {
		return errDeviated(k, "would be written with different content than the plan")
	}
	return nil
}

func (e *expectSink) Remove(p string) error {
	k := e.keyr.key(p)
	if e.backupPath(k) {
		return e.diskSink.Remove(p)
	}
	if _, rewritten := e.exp.Files[k]; !rewritten && !inSorted(e.exp.Removed, k) {
		return errDeviated(k, "would be removed but the plan does not remove it")
	}
	return e.diskSink.Remove(p)
}

func (e *expectSink) Chmod(p string, perm fs.FileMode) error {
	k := e.keyr.key(p)
	if f, ok := e.exp.Files[k]; ok && f.Perm == perm {
		return e.diskSink.Chmod(p, perm)
	}
	if m, ok := e.exp.Modes[k]; ok && m == perm {
		return e.diskSink.Chmod(p, perm)
	}
	return errDeviated(k, fmt.Sprintf("would get mode %04o which the plan does not set", perm))
}

// backupPath reports whether k is under the nginx/sites snapshot area
// (.nself/backups) and the confirmed render carries the nginx-sites-backup
// effect: the snapshot's file names carry a wall-clock stamp, so they cannot be
// planned byte for byte, but they are allowed nowhere else and only then.
func (e *expectSink) backupPath(k string) bool {
	if !strings.HasPrefix(k, ".nself/backups/") {
		return false
	}
	if checkBackupRoot(e.root) != nil {
		return false
	}
	for _, fx := range e.exp.Effects {
		if fx.Kind == EffectNginxSitesBackup {
			return true
		}
	}
	return false
}

// checkBackupRoot requires .nself and .nself/backups, where they exist, to be
// real directories inside the project: no symlinked component (each is
// Lstat'ed), so a snapshot write or prune cannot be redirected out of the tree.
func checkBackupRoot(root string) error {
	for _, rel := range []string{".nself", filepath.Join(".nself", "backups")} {
		p := filepath.Join(root, rel)
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			return nil // created later as a real directory
		}
		if err != nil {
			return fmt.Errorf("checking %s: %w", p, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("refusing to use %s for backups: it is a symlink or not a directory", p)
		}
	}
	return nil
}

func inSorted(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}

// verify checks the disk against the confirmed render after the build: every
// planned file exists with its planned bytes and mode, every planned chmod
// holds, every planned removal is gone. Any failure is E452: a build never
// reports as applied a file it did not write.
func (e *expectSink) verify() error {
	bad := func(k, why string) error {
		return errs.New("E452", fmt.Sprintf("planned file %s %s after the build", k, why)).
			WithWhy("the build finished but the disk does not hold what the confirmed plan listed").
			WithFix("re-run nself build --plan to see what differs, then run nself build again; check that the project is writable")
	}
	modes := runtime.GOOS != "windows"
	keys := make([]string, 0, len(e.exp.Files))
	for k := range e.exp.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		want := e.exp.Files[k]
		got, err := os.ReadFile(e.abs(k))
		if err != nil {
			return bad(k, "is missing")
		}
		if !bytes.Equal(got, want.Data) {
			return bad(k, "does not hold the planned content")
		}
		if info, serr := os.Stat(e.abs(k)); modes && serr == nil && info.Mode().Perm() != want.Perm {
			return bad(k, fmt.Sprintf("has mode %04o, not the planned %04o", info.Mode().Perm(), want.Perm))
		}
	}
	for k, m := range e.exp.Modes {
		if info, err := os.Stat(e.abs(k)); modes && err == nil && info.Mode().Perm() != m {
			return bad(k, fmt.Sprintf("has mode %04o, not the planned %04o", info.Mode().Perm(), m))
		}
	}
	for _, k := range e.exp.Removed {
		if _, err := os.Stat(e.abs(k)); err == nil {
			return bad(k, "was planned for removal but still exists")
		}
	}
	return nil
}
