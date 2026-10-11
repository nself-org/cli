package commands

// Purpose: tests for the opt-in AI auto-install decision in `nself start`
// (D-0221). The install must never run just because NSELF_MASTER_SECRET is
// set; it needs AI_AUTO_INSTALL=true or the ai plugin installed.
// Constraints: no network, no real installer (package-var seams).

import (
	"context"
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
			autoInstallAIIfNeeded(context.Background())
			if *calls != tc.wantInstall {
				t.Fatalf("installer calls = %d, want %d", *calls, tc.wantInstall)
			}
		})
	}
}
