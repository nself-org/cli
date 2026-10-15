package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveNginxSitesDirRejectsEscape covers an existing and a dangling sites
// link, plus an escaped nginx parent whose sites directory does not exist yet.
func TestResolveNginxSitesDirRejectsEscape(t *testing.T) {
	for _, component := range []string{"sites", "dangling-sites", "nginx"} {
		t.Run(component, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(root, "nginx")
			if component != "nginx" {
				if err := os.MkdirAll(link, 0o755); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(link, "sites")
			}
			if component == "dangling-sites" {
				outside = filepath.Join(outside, "missing")
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			got, err := resolveNginxSitesDir(root, "")
			if err == nil || got != "" || !strings.Contains(err.Error(), "outside") {
				t.Fatalf("must refuse escaped sites path, got %q, %v", got, err)
			}
		})
	}
}
