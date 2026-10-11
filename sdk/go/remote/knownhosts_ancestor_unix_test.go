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
