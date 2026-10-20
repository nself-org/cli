//go:build unix

package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckStorePathLinks(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "hardlink"}[linked], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ci.age")
			if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if linked {
				if err := os.Link(path, filepath.Join(dir, "release.age")); err != nil {
					t.Fatal(err)
				}
			}
			err := checkStorePath(dir, path, errors.New("missing"))
			if (err != nil) != linked {
				t.Fatalf("linked=%v: error=%v", linked, err)
			}
		})
	}
}
