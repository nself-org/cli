package commands

// Purpose: tests for the opt-in AI auto-install decision in `nself start`
// (D-0221). The install must never run just because NSELF_MASTER_SECRET is
// set; it needs AI_AUTO_INSTALL=true or the ai plugin installed.
// Constraints: no network, no real installer (package-var seams).

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAIAutoInstallWanted(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		pluginSet bool
		want      bool
	}{
		{"default: unset, master secret set, no plugin", map[string]string{"NSELF_MASTER_SECRET": "fake-test-secret"}, false, false},
		{"explicit true", map[string]string{"AI_AUTO_INSTALL": "true"}, false, true},
		{"explicit TRUE (case-insensitive)", map[string]string{"AI_AUTO_INSTALL": "TRUE"}, false, true},
		{"explicit 1", map[string]string{"AI_AUTO_INSTALL": "1"}, false, true},
		{"plugin installed, unset", map[string]string{}, true, true},
		{"plugin installed, false suppresses", map[string]string{"AI_AUTO_INSTALL": "false"}, true, false},
		{"plugin installed, 0 suppresses", map[string]string{"AI_AUTO_INSTALL": "0"}, true, false},
		{"false without plugin", map[string]string{"AI_AUTO_INSTALL": "false"}, false, false},
		{"junk value is not opt-in", map[string]string{"AI_AUTO_INSTALL": "maybe"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aiAutoInstallWanted(func(k string) string { return tc.env[k] }, tc.pluginSet)
			if got != tc.want {
				t.Fatalf("aiAutoInstallWanted = %v, want %v", got, tc.want)
			}
		})
	}
}

// stubAIInstall swaps the installer, plugin probe and Ollama probe, and
// returns a pointer to the installer call count.
func stubAIInstall(t *testing.T, pluginInstalled, ollamaUp bool) *int {
	t.Helper()
	calls := 0
	oi, op, oh := runDoctorAIFn, aiPluginInstalledFn, ollamaHealthyFn
	runDoctorAIFn = func(context.Context, doctorAIFlags) error { calls++; return nil }
	aiPluginInstalledFn = func() bool { return pluginInstalled }
	ollamaHealthyFn = func(context.Context) bool { return ollamaUp }
	t.Cleanup(func() { runDoctorAIFn, aiPluginInstalledFn, ollamaHealthyFn = oi, op, oh })
	return &calls
}

func TestAutoInstallAIIfNeeded(t *testing.T) {
	cases := []struct {
		name        string
		aiEnv       string
		plugin      bool
		ollamaUp    bool
		wantInstall int
	}{
		{"default does not install", "", false, false, 0},
		{"explicit true installs", "true", false, false, 1},
		{"plugin installed installs", "", true, false, 1},
		{"plugin installed but false", "false", true, false, 0},
		{"wanted but ollama already healthy", "true", false, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_MASTER_SECRET", "fake-test-secret")
			t.Setenv("AI_AUTO_INSTALL", tc.aiEnv) // "" behaves as unset
			calls := stubAIInstall(t, tc.plugin, tc.ollamaUp)
			autoInstallAIIfNeeded(context.Background(), t.TempDir())
			if *calls != tc.wantInstall {
				t.Fatalf("installer calls = %d, want %d", *calls, tc.wantInstall)
			}
		})
	}
}

// TestAutoInstallAIIfNeededProjectEnv: AI_AUTO_INSTALL set only in the project
// env cascade is honored by the gate, which runs before config.Load (D-0614).
func TestAutoInstallAIIfNeededProjectEnv(t *testing.T) {
	cases := []struct {
		name        string
		files       map[string]string
		procEnv     string
		plugin      bool
		wantInstall int
	}{
		{"project .env opts out with plugin installed", map[string]string{".env": "AI_AUTO_INSTALL=false\n"}, "", true, 0},
		{"project .env opts in", map[string]string{".env": "AI_AUTO_INSTALL=true\n"}, "", false, 1},
		{"env layer overrides .env", map[string]string{".env": "AI_AUTO_INSTALL=true\n", ".env.dev": "AI_AUTO_INSTALL=false\n"}, "", false, 0},
		{"process env wins over the project file", map[string]string{".env": "AI_AUTO_INSTALL=false\n"}, "true", false, 1},
		{"no setting and plugin installed installs", map[string]string{".env": "PROJECT_NAME=x\n"}, "", true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for f, body := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("ENV", "dev")
			t.Setenv("AI_AUTO_INSTALL", tc.procEnv)
			calls := stubAIInstall(t, tc.plugin, false)
			autoInstallAIIfNeeded(context.Background(), dir)
			if *calls != tc.wantInstall {
				t.Fatalf("installer calls = %d, want %d", *calls, tc.wantInstall)
			}
		})
	}
}
