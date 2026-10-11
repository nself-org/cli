//go:build !windows

package report

// nginx_unix_test.go: the nginx unreadable-file notes need chmod-unreadable
// files, which Windows ignores, so the test runs on Unix only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S-5: an unreadable nginx conf or directory becomes a note.
func TestReadNginxUnreadableIsANote(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ok.conf"), "listen 80;\n")
	writeFile(t, filepath.Join(root, "locked.conf"), "listen 81;\n")
	writeFile(t, filepath.Join(root, "sites", "inner.conf"), "listen 82;\n")
	if err := os.Chmod(filepath.Join(root, "locked.conf"), 0); err != nil {
		t.Skip("chmod unavailable")
	}
	if err := os.Chmod(filepath.Join(root, "sites"), 0); err != nil {
		t.Skip("chmod unavailable")
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "sites"), 0o755) })
	files, notes := readNginx(root)
	if len(files) != 1 || files[0].Name != "nginx/ok.conf" {
		t.Errorf("files = %v", files)
	}
	joined := strings.Join(notes, "|")
	for _, want := range []string{"nginx/locked.conf skipped: unreadable", "nginx/sites skipped: unreadable"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes %q lack %q", joined, want)
		}
	}
}
