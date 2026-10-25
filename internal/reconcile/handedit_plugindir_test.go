package reconcile

// Purpose: HandEditedFn resolves "@plugins/" paths under the plugin directory
// the project's env cascade names, not the shell's (D-0644).
// Inputs: a record, a project whose .env sets NSELF_PLUGIN_DIR, and a shell
// that leaves it unset or points it at a different directory.
// Outputs: pass/fail on whether a hand-edited fragment is seen.
// Constraints: temp dirs only.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandEditedFnPluginDirOnlyInProjectEnv(t *testing.T) {
	for _, tc := range shellPluginDirs {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnv(t)
			project, plugins := t.TempDir(), t.TempDir()
			tc.shell(t)
			if err := os.WriteFile(filepath.Join(project, ".env"), []byte("NSELF_PLUGIN_DIR="+plugins+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(plugins, "p.yml"), []byte("edited\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			s := &GeneratedState{Files: map[string]string{PluginsPrefix + "p.yml": sha256Hex([]byte("rendered\n"))}}
			if !s.HandEditedFn(project)(PluginsPrefix + "p.yml") {
				t.Fatal("a fragment edited under the .env plugin directory must be seen")
			}
			if err := os.WriteFile(filepath.Join(plugins, "p.yml"), []byte("rendered\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if s.HandEditedFn(project)(PluginsPrefix + "p.yml") {
				t.Fatal("an unchanged fragment must not report drift")
			}
		})
	}
}
