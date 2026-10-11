//go:build unix && !aix && !solaris && !illumos

package remote

// Purpose: tests for the sshd secure_path check over the pin directory's
// ancestors (D-0484). No network, no root needed.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeInfo is a FileInfo with a chosen mode and owner.
type fakeInfo struct {
	mode os.FileMode
	uid  uint32
}

func (f fakeInfo) Name() string       { return "d" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() os.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return true }
func (f fakeInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

func TestAncestor_Rule(t *testing.T) {
	me := uint32(os.Geteuid())
	other := me + 1
	dir := os.ModeDir
	sticky := os.ModeDir | os.ModeSticky
	for _, tc := range []struct {
		mode os.FileMode
		uid  uint32
		ok   bool
	}{
		{dir | 0o755, me, true},
		{dir | 0o700, 0, true},
		{sticky | 0o777, 0, true},     // /tmp
		{sticky | 0o777, me, me == 0}, // sticky only helps when root owns it
		{dir | 0o777, 0, false},
		{dir | 0o775, 0, false},
		{dir | 0o757, me, false},
		{dir | 0o755, other, other == 0},
	} {
		err := ancestorOK(fakeInfo{tc.mode, tc.uid}, "/pin", "/a")
		if (err == nil) != tc.ok {
			t.Errorf("mode %v uid %d: err = %v, want ok=%v", tc.mode, tc.uid, err, tc.ok)
		}
	}
}

// nestedPin returns a pin file at home/shared/ci/known_hosts with every dir
// 0700 except shared, plus the shared dir.
func nestedPin(t *testing.T) (PinnedHostKeys, string) {
	t.Helper()
	_, home := pinFile(t)
	shared := filepath.Join(home, "shared")
	p := PinnedHostKeys{Path: filepath.Join(shared, "ci", "known_hosts")}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	return p, shared
}

func TestAncestor_WritableAncestorRefused(t *testing.T) {
	skipWindows(t)
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o755, true}, {0o700, true}, {0o777, false}, {0o775, false}, {0o757, false}, {os.ModeSticky | 0o777, false}} {
		t.Run(fmt.Sprintf("%v", tc.mode), func(t *testing.T) {
			p, shared := nestedPin(t)
			if err := p.Add("n1", keyA); err != nil {
				t.Fatal(err)
			}
			before := readPin(t, p)
			if err := os.Chmod(shared, tc.mode); err != nil {
				t.Fatal(err)
			}
			_, lerr := p.Lookup("n1")
			aerr := p.Add("n2", keyB)
			if tc.ok {
				if lerr != nil || aerr != nil {
					t.Fatalf("ancestor %v refused: %v, %v", tc.mode, lerr, aerr)
				}
				return
			}
			rerr := p.Remove("n1")
			for op, err := range map[string]error{"Lookup": lerr, "Add": aerr, "Remove": rerr} {
				if err == nil || !strings.Contains(err.Error(), "ancestor "+shared+" ") {
					t.Errorf("%s with ancestor %v: %v", op, tc.mode, err)
				}
			}
			if readPin(t, p) != before {
				t.Error("pin file changed")
			}
			if fi, _ := os.Stat(shared); fi.Mode()&(os.ModePerm|os.ModeSticky) != tc.mode {
				t.Errorf("ancestor was chmod-ed to %v", fi.Mode())
			}
		})
	}
}

// A symlinked pin dir is checked along its resolved path too: a safe link
// into a world-writable tree is refused.
func TestAncestor_ResolvedPathChecked(t *testing.T) {
	skipWindows(t)
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o755, true}, {0o777, false}} {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			p, home := pinFile(t)
			open := filepath.Join(home, "open")
			real := filepath.Join(open, "real")
			if err := os.MkdirAll(real, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(open, tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, filepath.Dir(p.Path)); err != nil {
				t.Fatal(err)
			}
			aerr := p.Add("n1", keyA)
			if tc.ok {
				if aerr != nil {
					t.Fatalf("link into a %04o tree refused: %v", tc.mode, aerr)
				}
				return
			}
			if aerr == nil || !strings.Contains(aerr.Error(), "ancestor "+open+" ") {
				t.Errorf("link into a %04o tree: %v", tc.mode, aerr)
			}
			if _, err := os.Stat(filepath.Join(real, "known_hosts")); err == nil {
				t.Error("pin file written under a world-writable ancestor")
			}
		})
	}
}

// A new pin dir under a writable ancestor is refused before anything is
// created there.
func TestAncestor_NothingCreatedUnderWritableAncestor(t *testing.T) {
	skipWindows(t)
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o755, true}, {0o777, false}} {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			_, home := pinFile(t)
			open := filepath.Join(home, "open")
			if err := os.Mkdir(open, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(open, tc.mode); err != nil {
				t.Fatal(err)
			}
			p := PinnedHostKeys{Path: filepath.Join(open, "new", "ci", "known_hosts")}
			err := p.Add("n1", keyA)
			_, serr := os.Stat(filepath.Join(open, "new"))
			if tc.ok {
				if err != nil || serr != nil {
					t.Fatalf("0755 ancestor: Add %v, stat %v", err, serr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "ancestor "+open+" ") {
				t.Errorf("Add under a %04o ancestor: %v", tc.mode, err)
			}
			if !os.IsNotExist(serr) {
				t.Errorf("Add created %s under a writable ancestor (%v)", filepath.Join(open, "new"), serr)
			}
		})
	}
}

// The walk goes all the way up: a writable directory two levels above the
// pin dir is refused too, not only the immediate ancestor.
func TestAncestor_DeepAncestorRefused(t *testing.T) {
	skipWindows(t)
	for _, tc := range []struct {
		mode os.FileMode
		ok   bool
	}{{0o755, true}, {0o777, false}} {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			_, home := pinFile(t)
			open := filepath.Join(home, "open")
			p := PinnedHostKeys{Path: filepath.Join(open, "a", "b", "known_hosts")}
			if err := os.MkdirAll(filepath.Dir(p.Path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(open, tc.mode); err != nil {
				t.Fatal(err)
			}
			err := p.Add("n1", keyA)
			if tc.ok {
				if err != nil {
					t.Fatalf("0755 deep ancestor refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "ancestor "+open+" ") {
				t.Errorf("Add under a 0777 deep ancestor: %v", err)
			}
		})
	}
}

func TestTryLockFD_BadDescriptorErrors(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	for _, ex := range []bool{true, false} {
		ok, err := tryLockFD(f, ex)
		if ok || err == nil {
			t.Errorf("tryLockFD(closed, %v) = %v, %v; want false and an error", ex, ok, err)
		}
	}
}

func TestPinLockWaitDefaultIsThirtySeconds(t *testing.T) {
	if pinLockWait != 30*time.Second {
		t.Errorf("pinLockWait = %v, want 30s (docs and changelog say 30 seconds)", pinLockWait)
	}
}
