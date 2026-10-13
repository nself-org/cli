package pintest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSealedRefusesWritableParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	root := t.TempDir()
	loose := filepath.Join(root, "loose")
	if err := os.Mkdir(loose, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o775); err != nil { // defeat umask
		t.Fatal(err)
	}
	child := filepath.Join(loose, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if Sealed(child) {
		t.Fatal("a 0775 ancestor must not count as sealed")
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if !Sealed(child) && Sealed(os.TempDir()) {
		t.Fatal("a 0755 chain under a sealed temp dir must be sealed")
	}
}

func TestSealedStickyUserOwnedDirIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a non-root POSIX user")
	}
	dir := filepath.Join(t.TempDir(), "sticky")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, os.ModeSticky|0o777); err != nil {
		t.Fatal(err)
	}
	// production: sticky only helps when root owns the directory
	if Sealed(dir) {
		t.Fatal("a user-owned sticky world-writable dir must not count as sealed")
	}
}

func TestSealedFollowsAsGivenAndResolvedChains(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	root := t.TempDir()
	loose := filepath.Join(root, "loose")
	if err := os.Mkdir(loose, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o775); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(loose, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if Sealed(link) {
		t.Fatal("a symlink into a 0775 parent must not count as sealed (resolved chain)")
	}
}

func TestHomeIsFreshAndEmpty(t *testing.T) {
	h := Home(t)
	ents, err := os.ReadDir(h)
	if err != nil || len(ents) != 0 {
		t.Fatalf("Home must be an empty dir: %v %v", ents, err)
	}
}
