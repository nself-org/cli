package config

import (
	"path/filepath"
	"testing"
)

func setPluginDirHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestDefaultPluginDir(t *testing.T) {
	home := setPluginDirHome(t)
	t.Setenv("NSELF_V15", "1")

	t.Setenv("NSELF_PLUGIN_DIR", "")
	if got, want := DefaultPluginDir(), filepath.Join(home, ".nself", "plugins"); got != want {
		t.Errorf("unset env: %q, want %q", got, want)
	}
	t.Setenv("NSELF_PLUGIN_DIR", "  /abs/plugins  ")
	if got := DefaultPluginDir(); got != "/abs/plugins" {
		t.Errorf("env is trimmed: %q", got)
	}
	t.Setenv("NSELF_PLUGIN_DIR", "~/custom")
	if got, want := DefaultPluginDir(), filepath.Join(home, "custom"); got != want {
		t.Errorf("v1.5 expands a literal ~ in the env value: %q, want %q", got, want)
	}
}

func TestResolvePluginDir(t *testing.T) {
	home := setPluginDirHome(t)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	def := filepath.Join(home, ".nself", "plugins")

	t.Setenv("NSELF_V15", "1")
	for in, want := range map[string]string{
		"~/.nself/plugins": def,
		"~":                home,
		"/abs/plugins":     "/abs/plugins",
		"rel/plugins":      "rel/plugins",
		"":                 def,
	} {
		if got := ResolvePluginDir(in); got != want {
			t.Errorf("v1.5 ResolvePluginDir(%q) = %q, want %q", in, got, want)
		}
	}

	// v1.4 keeps a configured value as written (D-0634); empty still defaults.
	t.Setenv("NSELF_V15", "")
	if got := ResolvePluginDir("~/.nself/plugins"); got != "~/.nself/plugins" {
		t.Errorf("v1.4 must not expand: %q", got)
	}
	if got := ResolvePluginDir(""); got != def {
		t.Errorf("v1.4 empty: %q, want %q", got, def)
	}
}

func TestExpandHomeDir(t *testing.T) {
	home := setPluginDirHome(t)
	for in, want := range map[string]string{
		"~/.nself/plugins": filepath.Join(home, ".nself", "plugins"),
		"~":                home,
		"/abs/plugins":     "/abs/plugins",
		"rel/plugins":      "rel/plugins",
		"~user/plugins":    "~user/plugins",
		"":                 "",
	} {
		if got := ExpandHomeDir(in); got != want {
			t.Errorf("ExpandHomeDir(%q) = %q, want %q", in, got, want)
		}
	}
}
