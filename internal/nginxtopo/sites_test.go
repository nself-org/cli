package nginxtopo

import (
	"os"
	"path/filepath"
	"testing"
)

// TestServedSitesDirInternalLinks accepts in-root sites and symlinked project
// roots, including hosts where the temporary root itself contains a symlink.
func TestServedSitesDirInternalLinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	actual := filepath.Join(root, "routes")
	if err := os.MkdirAll(actual, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../routes", filepath.Join(root, "nginx", "sites")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{root, alias} {
		got, err := ServedSitesDir(project, "")
		if err != nil || got != filepath.Join(project, "nginx", "sites") {
			t.Fatalf("valid internal link: got %q, %v", got, err)
		}
	}
}
