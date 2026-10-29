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
// case) and otherwise a directory under /tmp, removed when the test ends. The
// directory is chmod 0700 either way, so a umask of 002 does not make HOME
// itself a group-writable ancestor. When no location is sealed the test is
// skipped with the reason instead of failing on the host's layout.
func Home(t testing.TB) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return t.TempDir() // the production check is skipped on windows
	}
	if Sealed(os.TempDir()) {
		return private(t, t.TempDir())
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
		return private(t, dir)
	}
	t.Skipf("no temp location with sealed ancestors (TMPDIR=%q): the pinned-host-keys rule refuses group/world-writable parents", os.Getenv("TMPDIR"))
	return ""
}

// private makes dir 0700. t.TempDir creates its dir with 0777 &^ umask, so on a
// host with umask 002 it is 0775 and is itself the group-writable ancestor of
// $HOME/.config/nself that the pin store refuses (D-0697).
func private(t testing.TB, dir string) string {
	t.Helper()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Sealed reports whether dir passes the same ancestor rule as production
// (sdk/go/remote ancestorOK): every directory above it, on the path as given
// and with symlinks resolved, is owned by the current user or root, and is not
// group/world writable unless it is a root-owned sticky directory.
func Sealed(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	for _, start := range []string{abs, resolved} {
		for d := start; ; d = filepath.Dir(d) {
			fi, err := os.Stat(d)
			if err != nil || !ancestorOK(fi) {
				return false
			}
			if d == filepath.Dir(d) {
				break
			}
		}
	}
	return true
}

func ancestorOK(fi os.FileInfo) bool {
	uid, ok := owner(fi)
	if !ok {
		return true // no owner information on this platform
	}
	if uid != 0 && uid != os.Geteuid() {
		return false
	}
	if fi.Mode().Perm()&0o022 != 0 && (fi.Mode()&os.ModeSticky == 0 || uid != 0) {
		return false
	}
	return true
}
