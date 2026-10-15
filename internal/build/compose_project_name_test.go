package build

// Purpose: guard Bash-era project identity migrations before build can exit.
// Inputs: conflicting, matching, and absent legacy compose names in .env.
// Outputs: actionable warning assertions from the real config-loading path.
// Constraints: no Docker daemon or project writes; restore process environment.

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidateConfig_ComposeProjectNameWarning(t *testing.T) {
	for _, legacy := range []string{"legacy-stack", "new-stack", ""} {
		t.Run("legacy="+legacy, func(t *testing.T) {
			t.Cleanup(SnapshotEnv())
			t.Setenv("ENV", "dev")
			t.Setenv("COMPOSE_PROJECT_NAME", "")
			dir := t.TempDir()
			env := "PROJECT_NAME=new-stack\n"
			if legacy != "" {
				env += "COMPOSE_PROJECT_NAME=" + legacy + "\n"
			}
			testWriteFile(t, filepath.Join(dir, ".env"), env, 0600)
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			st := &buildState{workdir: dir, opts: BuildOptions{Check: true}}
			if result, err := st.loadValidateConfig(); err != nil || result == nil {
				t.Fatalf("loadValidateConfig: result=%v err=%v", result, err)
			}
			got := logs.String()
			if legacy == "legacy-stack" {
				for _, want := range []string{"level=WARN", "COMPOSE_PROJECT_NAME", "PROJECT_NAME", "legacy-stack", "new-stack", "legacy-stack_postgres_data", "com.docker.compose.project=legacy-stack"} {
					if !strings.Contains(got, want) {
						t.Errorf("warning missing %q: %s", want, got)
					}
				}
			} else if strings.Contains(got, "COMPOSE_PROJECT_NAME") {
				t.Errorf("unexpected project-name warning: %s", got)
			}
		})
	}
}
