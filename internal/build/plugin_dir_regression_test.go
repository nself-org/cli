package build

import (
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestPluginDirSharedEnvExpansion(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NSELF_PLUGIN_DIR", "~/custom-plugins")
	cfg := &config.Config{}
	cfg.PluginSystem.Dir = "/configured/plugins"
	want := filepath.Join(home, "custom-plugins")
	if got := DefaultPluginDir(); got != want {
		t.Errorf("compose discovery dir = %q, want %q", got, want)
	}
	if got, err := resolvePluginNginxDir("", cfg); err != nil || got != want {
		t.Errorf("nginx dir = %q, %v; want %q", got, err, want)
	}
	if got := buildTemplateVars("", cfg)["NSELF_PLUGIN_DIR"]; got != want {
		t.Errorf("template dir = %q, want %q", got, want)
	}
	if got, err := resolvePluginNginxDir("~/explicit", cfg); err != nil || got != filepath.Join(home, "explicit") {
		t.Errorf("explicit nginx dir = %q, %v; want caller override", got, err)
	}
}
