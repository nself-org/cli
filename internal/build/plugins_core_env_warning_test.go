package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoreEnvNoMatchWarningCandidates checks that the diagnostic lists the
// same service names lookup accepts, including a differing manifest name.
func TestCoreEnvNoMatchWarningCandidates(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, candidates string
	}{
		{"different", "fxm", "fx, nself-fx, plugin-fx, fxm, nself-fxm, plugin-fxm"},
		{"same", "fx", "fx, nself-fx, plugin-fx"},
		{"empty", "", "fx, nself-fx, plugin-fx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writePluginManifestJSON(t, pluginDir, "fx", 3777)
			manifest := fmt.Sprintf(`{"name":%q,"port":3777}`, tc.manifest)
			if err := os.WriteFile(filepath.Join(pluginDir, "fx", "plugin.json"), []byte(manifest), 0644); err != nil {
				t.Fatal(err)
			}
			in := "services:\n  one:\n    image: a\n  two:\n    image: b\n"
			resetWarnOnce()
			t.Cleanup(resetWarnOnce)
			var out string
			stderr := captureStderr(t, func() {
				out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "fx"))
			})
			if out != in {
				t.Errorf("fragment changed: %q", out)
			}
			want := "none is named " + tc.candidates + ", so the core env vars were not injected"
			if !strings.Contains(stderr, want) {
				t.Errorf("warning = %q, want %q", stderr, want)
			}
		})
	}
}
