package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// envExplainProject creates a temp project dir with the given .env* files
// and chdirs the test into it (t.Chdir auto-restores on cleanup).
func envExplainProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	t.Chdir(dir)
	return dir
}

// envExplainRoot returns the real envExplainCmd for testing, resetting its
// --reveal flag to false on cleanup so state never leaks into another test or
// into the actual registered command tree.
func envExplainRoot(t *testing.T) *cobra.Command {
	t.Helper()
	t.Cleanup(func() {
		_ = envExplainCmd.Flags().Set("reveal", "false")
	})
	return envExplainCmd
}

// TestEnvExplain_NoArgLists_Cascade verifies the no-argument form lists every
// cascade file with its existence, in load order.
func TestEnvExplain_NoArgLists_Cascade(t *testing.T) {
	envExplainProject(t, map[string]string{
		".env":         "PROJECT_NAME=demo\n",
		".env.secrets": "POSTGRES_PASSWORD=abc\n",
	})
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv("NSELF_LEGACY_ENV_ORDER")

	out, err := captureStdout(t, func() error {
		return runEnvExplain(envExplainRoot(t), nil)
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	for _, want := range []string{".env", ".env.dev", ".env.secrets", ".env.local"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".env.ai") {
		t.Errorf("output should not mention .env.ai under the canonical order:\n%s", out)
	}
}

// TestEnvExplain_NoArgLegacyMode_ShowsWarning verifies that when
// NSELF_LEGACY_ENV_ORDER is set, the overview labels the mode as legacy and
// warns about it.
func TestEnvExplain_NoArgLegacyMode_ShowsWarning(t *testing.T) {
	envExplainProject(t, map[string]string{".env": "X=1\n"})
	_ = os.Setenv("NSELF_LEGACY_ENV_ORDER", "1")
	t.Cleanup(func() { _ = os.Unsetenv("NSELF_LEGACY_ENV_ORDER") })

	out, err := captureStdout(t, func() error {
		return runEnvExplain(envExplainRoot(t), nil)
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	if !strings.Contains(out, "LEGACY") {
		t.Errorf("expected LEGACY mode label:\n%s", out)
	}
	if !strings.Contains(out, ".env.ai") {
		t.Errorf("legacy cascade should list .env.ai:\n%s", out)
	}
}

// TestEnvExplain_NoArg_ReadsEnvFromDotEnvFile verifies `nself env explain`
// agrees with config.Load(): with no ENV set in the process environment, it
// must resolve ENV from the project's own .env file (ENV=prod here) and
// list .env.prod as the active cascade layer — not silently show "dev" the
// way it did before config.ResolveEnv existed. This is the same
// production-server bug shape as config.TestLoad_EnvResolvedFromDotEnvFile_WhenProcessEnvUnset:
// this command exists specifically so an operator can check what a build
// would actually do, so it must never disagree with Load() about the
// answer.
func TestEnvExplain_NoArg_ReadsEnvFromDotEnvFile(t *testing.T) {
	envExplainProject(t, map[string]string{
		".env":      "ENV=prod\n",
		".env.prod": "PROJECT_NAME=demo\n",
	})
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv("NSELF_LEGACY_ENV_ORDER")

	out, err := captureStdout(t, func() error {
		return runEnvExplain(envExplainRoot(t), nil)
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	if !strings.Contains(out, "prod") {
		t.Errorf("output should show the resolved env as prod:\n%s", out)
	}
	if !strings.Contains(out, "from .env") {
		t.Errorf("output should name .env as the source that decided ENV:\n%s", out)
	}
	if !strings.Contains(out, ".env.prod") {
		t.Errorf("output missing .env.prod as the active cascade layer:\n%s", out)
	}
}

// TestEnvExplain_NoArg_ProcessEnvWinsOverDotEnvFile verifies an ENV already
// set in the process environment is shown as the source, not .env's own
// ENV= key, matching config.ResolveEnv's precedence.
func TestEnvExplain_NoArg_ProcessEnvWinsOverDotEnvFile(t *testing.T) {
	envExplainProject(t, map[string]string{".env": "ENV=prod\n"})
	_ = os.Setenv("ENV", "staging")
	t.Cleanup(func() { _ = os.Unsetenv("ENV") })

	out, err := captureStdout(t, func() error {
		return runEnvExplain(envExplainRoot(t), nil)
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	if !strings.Contains(out, "staging") {
		t.Errorf("output should show the resolved env as staging (process env):\n%s", out)
	}
	if !strings.Contains(out, "from process environment") {
		t.Errorf("output should name the process environment as the source:\n%s", out)
	}
}

// TestEnvExplain_VarArg_RedactsByDefault verifies that explaining a specific
// variable shows which file wins but redacts the value unless --reveal.
func TestEnvExplain_VarArg_RedactsByDefault(t *testing.T) {
	envExplainProject(t, map[string]string{
		".env":         "API_KEY=secret-value\n",
		".env.secrets": "API_KEY=different-secret-value\n",
	})
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv("NSELF_LEGACY_ENV_ORDER")

	out, err := captureStdout(t, func() error {
		return runEnvExplain(envExplainRoot(t), []string{"API_KEY"})
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	if strings.Contains(out, "secret-value") {
		t.Errorf("value leaked without --reveal:\n%s", out)
	}
	if !strings.Contains(out, ".env.secrets wins") {
		t.Errorf("expected .env.secrets to win (highest existing precedence):\n%s", out)
	}
}

// TestEnvExplain_VarArg_RevealShowsValue verifies --reveal shows the actual
// winning value.
func TestEnvExplain_VarArg_RevealShowsValue(t *testing.T) {
	envExplainProject(t, map[string]string{
		".env.local": "API_KEY=personal-secret\n",
	})
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv("NSELF_LEGACY_ENV_ORDER")

	root := envExplainRoot(t)
	if err := root.Flags().Set("reveal", "true"); err != nil {
		t.Fatalf("setting --reveal: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runEnvExplain(root, []string{"API_KEY"})
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	if !strings.Contains(out, "personal-secret") {
		t.Errorf("expected revealed value in output:\n%s", out)
	}
}

// TestEnvExplain_VarArg_NotSetAnywhere verifies a var absent from every
// cascade file gets a clear "not set" message instead of an error.
func TestEnvExplain_VarArg_NotSetAnywhere(t *testing.T) {
	envExplainProject(t, map[string]string{".env": "OTHER_VAR=1\n"})
	_ = os.Unsetenv("ENV")
	_ = os.Unsetenv("NSELF_LEGACY_ENV_ORDER")

	out, err := captureStdout(t, func() error {
		return runEnvExplain(envExplainRoot(t), []string{"MISSING_VAR"})
	})
	if err != nil {
		t.Fatalf("runEnvExplain() error: %v", err)
	}
	if !strings.Contains(out, "not set by any file") {
		t.Errorf("expected 'not set' message:\n%s", out)
	}
}
