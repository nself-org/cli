package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// catalogCacheFixture isolates HOME and the catalog cache, and (when body is
// not empty) stores it as the cached catalog.
func catalogCacheFixture(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cache := t.TempDir()
	t.Setenv("NSELF_PLUGIN_CACHE", cache)
	if body != "" {
		if err := os.WriteFile(filepath.Join(cache, "catalog.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const routerCatalog = `{"schema_version":1,"plugins":[
 {"slug":"traefik","tier":"free","installable":true,"commands":{"command":"traefik","subcommands":[]},"bundles":[]},
 {"slug":"claw-cli","tier":"licensed","installable":true,"commands":{"command":"claw","subcommands":[]},"bundles":[]}]}`

func TestRouterHintE117(t *testing.T) {
	compattest.Set(t, true)
	catalogCacheFixture(t, routerCatalog)

	err := ProxyCommand("traefik", nil)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E117" {
		t.Fatalf("got %v, want E117", err)
	}
	if !strings.Contains(err.Error(), "provided by plugin traefik: run nself add traefik") {
		t.Fatalf("message lacks the add hint: %q", err)
	}
	if code := errs.ExitCodeFor(err); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
}

func TestRouterKeepsExistingHint(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		catalogCacheFixture(t, routerCatalog)

		// A command no catalog plugin provides keeps the generic hint.
		err := ProxyCommand("frobnicate", nil)
		if err == nil || !strings.Contains(err.Error(), "nself install frobnicate") || strings.Contains(err.Error(), "E117") {
			t.Fatalf("unknown command lost its generic hint: %v", err)
		}
		// The suppressed hint (registry already spoke) and a registry hint stay
		// as given even though the catalog knows the command.
		err = ProxyCommandWithHint("claw", nil, "")
		if err == nil || strings.Contains(err.Error(), "E117") || strings.Contains(err.Error(), "install it with") {
			t.Fatalf("suppressed hint was overridden: %v", err)
		}
		err = ProxyCommandWithHint("claw", nil, "nself install claw-cli")
		if err == nil || strings.Contains(err.Error(), "E117") || !strings.Contains(err.Error(), "nself install claw-cli") {
			t.Fatalf("registry hint was overridden: %v", err)
		}
	})

	// v1.4 keeps the generic hint even for a command the catalog provides, and
	// with no cache v1.5 does too (the unknown-command path never goes online).
	compattest.Set(t, false)
	catalogCacheFixture(t, routerCatalog)
	if err := ProxyCommand("traefik", nil); err == nil || strings.Contains(err.Error(), "E117") || !strings.Contains(err.Error(), "nself install traefik") {
		t.Fatalf("v1.4 changed the hint: %v", err)
	}
	compattest.Set(t, true)
	catalogCacheFixture(t, "")
	if err := ProxyCommand("traefik", nil); err == nil || strings.Contains(err.Error(), "E117") || !strings.Contains(err.Error(), "nself install traefik") {
		t.Fatalf("no cache should keep the generic hint: %v", err)
	}
}

// TestPluginBinDirRefusesWithoutHOME proves D-1396: when HOME is unset the
// router must not fall back to a shared directory like /tmp/.nself/plugins/bin,
// because any local user could plant a binary there that cron or systemd would
// execute with the caller's secrets.
func TestPluginBinDirRefusesWithoutHOME(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	_, err := pluginBinDir()
	if err == nil {
		t.Fatal("pluginBinDir succeeded without HOME; expected refusal")
	}
	if !strings.Contains(err.Error(), "HOME is not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestProxyBinaryAtRefusesUnsafePath wires the ownership check through
// ProxyBinaryAt so that a binary in an unsafe directory is rejected even
// when the binary itself is well-formed. Removing the call to
// pluginperm.Verify in ProxyBinaryAt would make this test pass.
func TestProxyBinaryAtRefusesUnsafePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ownership check is a no-op on Windows")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, "nself-testplug")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Make the bin directory world-writable -> ProxyBinaryAt must refuse.
	if err := os.Chmod(binDir, 0o757); err != nil {
		t.Fatal(err)
	}
	err := ProxyBinaryAt(bin, root, "testplug", nil)
	if err == nil || !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("ProxyBinaryAt did not refuse world-writable dir: %v", err)
	}
}
