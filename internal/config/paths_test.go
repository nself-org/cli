package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePluginDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct{ name, env, configured, want string }{
		{"default", "", "", filepath.Join(home, ".nself", "plugins")},
		{"config", "", "/configured", "/configured"},
		{"config-home", "", "~/configured", filepath.Join(home, "configured")},
		{"env-wins", " ~/env ", "/configured", filepath.Join(home, "env")},
		{"empty-env", "  ", "~/configured", filepath.Join(home, "configured")},
		{"home", "~", "", home},
		{"named-user", "~other/plugins", "", "~other/plugins"},
		{"relative", "relative/plugins", "", "relative/plugins"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_PLUGIN_DIR", tc.env)
			if got := ResolvePluginDir(tc.configured); got != tc.want {
				t.Errorf("ResolvePluginDir(%q) = %q, want %q", tc.configured, got, tc.want)
			}
		})
	}
}

func TestCertificateDirName(t *testing.T) {
	for domain, want := range map[string]string{"": "localhost", "localhost": "localhost", "api.example.org": "api-example-org"} {
		if got := CertificateDirName(domain); got != want {
			t.Errorf("CertificateDirName(%q) = %q, want %q", domain, got, want)
		}
	}
}

func TestResolvePluginDirMissingHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("NSELF_PLUGIN_DIR", "")
	if got, want := ResolvePluginDir(""), filepath.Join("/tmp", ".nself", "plugins"); got != want {
		t.Errorf("missing-home default = %q, want %q", got, want)
	}
	if got := ResolvePluginDir("~/configured"); got != "~/configured" {
		t.Errorf("missing-home shorthand = %q, want literal path", got)
	}
}
