package database

import (
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestPluginMigrationsSharedEnvExpansion(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NSELF_PLUGIN_DIR", "~/custom-plugins")
	cfg := &config.Config{}
	cfg.PluginSystem.Dir = "/configured/plugins"
	want := filepath.Join(home, "custom-plugins", "backup", "migrations")
	if got := migrationsDir(cfg, "backup"); got != want {
		t.Fatalf("migrations dir = %q, want %q", got, want)
	}
}
