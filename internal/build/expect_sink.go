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
	"sort"
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

func inSorted(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}

// verify checks the disk against the confirmed render after the build.
func (e *expectSink) verify() error {
	keys := make([]string, 0, len(e.exp.Files))
	for k := range e.exp.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		got, err := os.ReadFile(e.abs(k))
		if err != nil || !bytes.Equal(got, e.exp.Files[k].Data) {
			return errDeviated(k, "does not hold the planned content after the build")
		}
	}
	for _, k := range e.exp.Removed {
		if _, err := os.Stat(e.abs(k)); err == nil {
			return errDeviated(k, "was planned for removal but still exists")
		}
	}
	return nil
}
