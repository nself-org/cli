package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noHomeDir(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
	t.Setenv("NSELF_PLUGIN_DIR", "")
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("this platform resolves a home directory without HOME")
	}
}

func inTmp(p string) bool {
	for _, base := range []string{"/tmp", "/private/tmp"} {
		if p == base || strings.HasPrefix(p, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// D-1413: resolvePluginDir used to answer /tmp/.nself/plugins with no home.
func TestResolvePluginDirNeverFallsBackToTmp(t *testing.T) {
	noHomeDir(t)
	if got := resolvePluginDir(); inTmp(got) {
		t.Fatalf("resolvePluginDir() = %q with no home directory; want a path outside /tmp", got)
	}
}

func TestResolvePluginDirCheckedNoHome(t *testing.T) {
	noHomeDir(t)
	dir, err := resolvePluginDirChecked()
	if err == nil || dir != "" || !strings.Contains(err.Error(), "HOME") {
		t.Fatalf("resolvePluginDirChecked() = %q, %v; want an error naming HOME", dir, err)
	}
	// An explicit NSELF_PLUGIN_DIR needs no home.
	want := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", want)
	if dir, err := resolvePluginDirChecked(); err != nil || dir != want {
		t.Fatalf("with NSELF_PLUGIN_DIR: %q, %v; want %q, nil", dir, err, want)
	}
}

func TestAutoInstallPluginRefusesWithoutHome(t *testing.T) {
	noHomeDir(t)
	// Never reach a real registry: the guard must refuse before any network use.
	t.Setenv("NSELF_PLUGIN_REGISTRY", "https://127.0.0.1:1")
	bin, err := autoInstallPlugin("ci", "")
	if err == nil || bin != "" || !strings.Contains(err.Error(), "HOME") {
		t.Fatalf("autoInstallPlugin with no home = %q, %v; want an error naming HOME", bin, err)
	}
}
