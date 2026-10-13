// Package pintest gives tests a HOME whose directory chain passes the
// pinned-host-keys ancestor rule (sdk/go/remote, sshd secure_path style): no
// directory above the pin store may be group or world writable unless it is
// a sticky directory such as /tmp.
//
// Why: CI hosts keep TMPDIR on an external drive whose mount root is 0775.
// t.TempDir() then sits under a writable ancestor and every v1.5 test that
// pins a host key is refused. The production rule is correct and stays as it
// is; tests that are not about that rule ask for Home instead of t.TempDir().
//
// Inputs: a testing.TB. Output: a fresh, empty, test-owned directory.
// Constraints: stdlib only; never relaxes any permission check.
package pintest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Home returns a fresh directory suitable as $HOME for a pin-store test.
// It uses t.TempDir() when that location's ancestors are sealed (the normal
// case, byte-for-byte the old behaviour) and otherwise a directory under
// /tmp, removed when the test ends. When no location is sealed the test is
// skipped with the reason instead of failing on the host's layout.
func Home(t testing.TB) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return t.TempDir() // the production check is skipped on windows
	}
	if Sealed(os.TempDir()) {
		return t.TempDir()
	}
	for _, base := range []string{"/tmp", "/private/tmp"} {
		if !Sealed(base) {
			continue
		}
		dir, err := os.MkdirTemp(base, "nself-pin-")
		if err != nil {
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}
	t.Skipf("no temp location with sealed ancestors (TMPDIR=%q): the pinned-host-keys rule refuses group/world-writable parents", os.Getenv("TMPDIR"))
	return ""
}

// Sealed reports whether dir and every directory above it (symlinks
// resolved) is not group/world writable, or is a sticky directory.
func Sealed(dir string) bool {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	for d := resolved; ; d = filepath.Dir(d) {
		fi, err := os.Stat(d)
		if err != nil {
			return false
		}
		if fi.Mode().Perm()&0o022 != 0 && fi.Mode()&os.ModeSticky == 0 {
			return false
		}
		if d == filepath.Dir(d) {
			return true
		}
	}
}
