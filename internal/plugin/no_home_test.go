package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noHome unsets every variable os.UserHomeDir reads, so the home is unknown.
func noHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "") // plan9
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("this platform resolves a home directory without HOME")
	}
}

func underTmp(p string) bool {
	for _, base := range []string{"/tmp", "/private/tmp"} {
		if p == base || strings.HasPrefix(p, base+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// D-1413: with no home directory the bin dir used to fall back to
// /tmp/.nself/plugins/bin, which any local user can pre-create and fill with
// binaries the proxy would then exec.
func TestPluginBinDirNeverFallsBackToTmp(t *testing.T) {
	noHome(t)
	if got := pluginBinDir(); underTmp(got) {
		t.Fatalf("pluginBinDir() = %q with no home directory; want a path outside /tmp", got)
	}
}

func TestProxyRefusesWithoutHome(t *testing.T) {
	noHome(t)
	for name, call := range map[string]func() error{
		"ProxyCommand": func() error { return ProxyCommand("foo", nil) },
		"ProxyBinary":  func() error { return ProxyBinary("nself-foo", "foo", nil) },
	} {
		err := call()
		if !errors.Is(err, ErrNoHome) {
			t.Errorf("%s with no home = %v, want ErrNoHome", name, err)
		}
	}
}

func TestPublishRefusesWithoutHome(t *testing.T) {
	noHome(t)
	m := &PluginManifest{PluginType: "cli", BinaryName: "nself-foo"}
	err := linkCLIBinary(t.TempDir(), "foo", m)
	if !errors.Is(err, ErrNoHome) {
		t.Fatalf("linkCLIBinary with no home = %v, want ErrNoHome", err)
	}
	if !strings.Contains(err.Error(), "HOME") {
		t.Errorf("error %q must name HOME", err)
	}
}
