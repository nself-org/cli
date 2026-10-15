package commands

import (
	"path/filepath"
	"testing"
)

func TestPluginDirSharedEnvExpansion(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NSELF_PLUGIN_DIR", "~/custom-plugins")
	if got, want := resolvePluginDir(), filepath.Join(home, "custom-plugins"); got != want {
		t.Fatalf("plugin command dir = %q, want %q", got, want)
	}
}
