//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckStorePathRefusesHardlink(t *testing.T) {
	dir := t.TempDir()
	release, ci := filepath.Join(dir, "release.age"), filepath.Join(dir, "ci.age")
	if err := os.WriteFile(release, []byte("ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkStorePath(dir, release, errors.New("missing")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(release, ci); err != nil {
		t.Fatal(err)
	}
	if err := checkStorePath(dir, ci, errors.New("missing")); err == nil {
		t.Fatal("accepted hardlinked release store as ci")
	}
}
