//go:build unix

package cmdlog

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteNarrowsOldLogDirAndRotatedFiles: a logs dir (0755) and rotated
// files (0644) from before D-0473 are narrowed; nothing is widened.
func TestWriteNarrowsOldLogDirAndRotatedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "logs")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"nself.log", "nself.log.1", "nself.log.2"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("fakeOldValue123\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	narrow := filepath.Join(dir, "nself.log.3")
	if err := os.WriteFile(narrow, []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}

	New(dir).Begin([]string{"nself", "version"})(0, nil)

	want := map[string]os.FileMode{
		dir:                               0o700,
		filepath.Join(dir, "nself.log"):   0o600,
		filepath.Join(dir, "nself.log.1"): 0o600,
		filepath.Join(dir, "nself.log.2"): 0o600,
		narrow:                            0o400, // never widened
		other:                             0o644, // not a log file
	}
	for p, mode := range want {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != mode {
			t.Errorf("%s: mode %o, want %o", filepath.Base(p), got, mode)
		}
	}
}
