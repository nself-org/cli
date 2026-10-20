package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImageConcatenationScan exercises the repository scanner on source files,
// including partial image references that are not complete string literals.
func TestImageConcatenationScan(t *testing.T) {
	for _, tc := range []struct {
		name, expr string
		want       bool
	}{
		{"partial postgres", `"postgres:" + version`, true},
		{"parenthesized", `("postgres:") + version`, true},
		{"split repository", `"docker.io/" + "library/postgres:" + version`, true},
		{"partial tag", `"postgres:16-" + variant`, true},
		{"full tag", `"postgres:16-alpine" + suffix`, true},
		{"url", `"postgres://" + host`, false},
		{"metadata", `"fixture:" + name`, false},
		{"plain text", `"postgres" + name`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "image.go"), []byte("package fixture\nvar image = "+tc.expr+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			hits := scanImageLiterals(t, dir)
			if got := len(hits) > 0; got != tc.want {
				t.Errorf("scan(%s) = %v, want image=%v", tc.expr, hits, tc.want)
			}
			if len(hits) > 1 && !strings.Contains(tc.expr, `"postgres:16-alpine"`) {
				t.Errorf("duplicate image detections: %v", hits)
			}
		})
	}
}
