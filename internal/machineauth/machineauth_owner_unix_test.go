//go:build unix

package machineauth

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLookupParentDirAndOwner covers the token file location checks.
func TestLookupParentDirAndOwner(t *testing.T) {
	cases := []struct {
		name    string
		dirMode os.FileMode
		wantErr bool
	}{
		{"world-writable non-sticky dir", 0o777, true},
		{"group-writable non-sticky dir", 0o770, true},
		{"sticky world-writable dir", 0o777 | os.ModeSticky, false},
		{"private dir", 0o700, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := homeDir(t)
			p := writeTokenFile(t, home, "tok-value\n", 0o600)
			if err := os.Chmod(filepath.Dir(p), c.dirMode); err != nil {
				t.Fatal(err)
			}
			tok, src, err := Lookup()
			if c.wantErr {
				wantE426(t, err, "writable by other users")
				return
			}
			if err != nil || tok != "tok-value" || src != SourceFile {
				t.Errorf("Lookup = %q, %q, %v", tok, src, err)
			}
		})
	}
	t.Run("owner mismatch", func(t *testing.T) {
		home := homeDir(t)
		writeTokenFile(t, home, "tok-value\n", 0o600)
		oldOwner, oldUID := fileOwner, currentUID
		t.Cleanup(func() { fileOwner, currentUID = oldOwner, oldUID })
		fileOwner = func(os.FileInfo) (int, bool) { return 4242, true }
		currentUID = func() int { return 1 }
		_, _, err := Lookup()
		wantE426(t, err, "owned by uid 4242")
		currentUID = func() int { return 4242 }
		if _, _, err := Lookup(); err != nil {
			t.Errorf("matching owner: %v", err)
		}
	})
}
