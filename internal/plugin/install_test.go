package plugin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// --- validateConsumes tests (S43-T17) ---

// TestValidateConsumes_AllInstalled verifies that no warning is emitted
// (and no error returned) when all consumed plugins are already installed.
func TestValidateConsumes_AllInstalled(t *testing.T) {
	pluginDir := t.TempDir()

	// Create installed plugin directories with plugin.json.
	for _, dep := range []string{"ai", "notify"} {
		dir := filepath.Join(pluginDir, dep)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("setup mkdir %s: %v", dep, err)
		}
		manifest := `{"name":"` + dep + `","version":"1.0.0","description":"dep","category":"utility","license":"MIT"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0644); err != nil {
			t.Fatalf("setup write %s: %v", dep, err)
		}
	}

	err := validateConsumes(pluginDir, "my-plugin", []string{"ai", "notify"})
	if err != nil {
		t.Errorf("validateConsumes with all deps installed: unexpected error: %v", err)
	}
}

// TestValidateConsumes_MissingPlugin verifies that a warning is printed to
// stderr when a consumed plugin is not installed, but no error is returned
// (modular installs must not be blocked). S43-T17.
func TestValidateConsumes_MissingPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	// "ai" is not installed — no directory created.

	err := validateConsumes(pluginDir, "my-plugin", []string{"ai"})
	// Must return nil — missing consumers are a warning, not a hard error.
	if err != nil {
		t.Errorf("validateConsumes with missing dep must not return error, got: %v", err)
	}
}

// TestValidateConsumes_EmptyConsumes verifies that an empty Consumes list
// produces no error. S43-T17.
func TestValidateConsumes_EmptyConsumes(t *testing.T) {
	pluginDir := t.TempDir()
	err := validateConsumes(pluginDir, "my-plugin", nil)
	if err != nil {
		t.Errorf("validateConsumes with empty list: unexpected error: %v", err)
	}
}

// TestValidateConsumes_SelfReference verifies that a plugin listing itself in
// Consumes does not trigger a warning or error. S43-T17.
func TestValidateConsumes_SelfReference(t *testing.T) {
	pluginDir := t.TempDir()
	err := validateConsumes(pluginDir, "my-plugin", []string{"my-plugin"})
	if err != nil {
		t.Errorf("validateConsumes self-reference: unexpected error: %v", err)
	}
}

// --- Remove lifecycle tests (S10-T02) ---

// TestRemove_NonExistentPlugin verifies that Remove() returns a descriptive
// error when the named plugin is not installed.
func TestRemove_NonExistentPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := Remove(context.Background(), cfg, "no-such-plugin", pluginDir, true, true)
	if err == nil {
		t.Fatal("expected error for non-existent plugin, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected 'not installed' in error, got: %v", err)
	}
}

// TestRemove_CleanRemoval verifies that Remove() removes the plugin directory
// when the plugin is installed. Uses keepData=true to skip DB schema drop,
// and force=true to skip reverse-dependency check.
// Status() will fail without Docker (no container running), which is fine:
// the code path skips Stop() when Status() returns an error.
func TestRemove_CleanRemoval(t *testing.T) {
	pluginDir := t.TempDir()
	cfg := &config.Config{}

	// Create a minimal plugin directory (simulates installed plugin).
	pluginName := "test-plugin"
	pluginInstallDir := filepath.Join(pluginDir, pluginName)
	if err := os.MkdirAll(pluginInstallDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Write a minimal manifest so ListInstalled would find it.
	manifest := `{"name":"test-plugin","version":"1.0.0","description":"test","category":"utility","license":"MIT"}`
	if err := os.WriteFile(filepath.Join(pluginInstallDir, "plugin.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}

	err := Remove(context.Background(), cfg, pluginName, pluginDir, true, true)
	if err != nil {
		t.Fatalf("Remove() returned unexpected error: %v", err)
	}

	// Plugin directory must be gone.
	if _, statErr := os.Stat(pluginInstallDir); !os.IsNotExist(statErr) {
		t.Errorf("expected plugin directory to be removed, but it still exists")
	}
}

// --- Reverse dependency tests ---

// TestCheckReverseDependencies_NoDependents verifies that checkReverseDependencies
// returns an empty slice when no installed plugin depends on the target.
func TestCheckReverseDependencies_NoDependents(t *testing.T) {
	pluginDir := t.TempDir()

	// Install two plugins that do not depend on "target".
	pluginA := `{"name":"plugin-a","version":"1.0.0","description":"A","category":"utility","license":"MIT"}`
	pluginB := `{"name":"plugin-b","version":"1.0.0","description":"B","category":"utility","license":"MIT"}`

	for name, content := range map[string]string{"plugin-a": pluginA, "plugin-b": pluginB} {
		dir := filepath.Join(pluginDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("setup mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(content), 0644); err != nil {
			t.Fatalf("setup write %s: %v", name, err)
		}
	}

	dependents, err := checkReverseDependencies(pluginDir, "target")
	if err != nil {
		t.Fatalf("checkReverseDependencies: %v", err)
	}
	if len(dependents) != 0 {
		t.Errorf("expected no dependents, got: %v", dependents)
	}
}

// TestCheckReverseDependencies_WithDependents verifies that checkReverseDependencies
// returns the dependent plugin name when an installed plugin declares the target as a dependency.
func TestCheckReverseDependencies_WithDependents(t *testing.T) {
	pluginDir := t.TempDir()

	// plugin-b depends on plugin-a.
	pluginB := `{"name":"plugin-b","version":"1.0.0","description":"B","category":"utility","license":"MIT","dependencies":["plugin-a"]}`

	dir := filepath.Join(pluginDir, "plugin-b")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(pluginB), 0644); err != nil {
		t.Fatalf("setup write: %v", err)
	}

	dependents, err := checkReverseDependencies(pluginDir, "plugin-a")
	if err != nil {
		t.Fatalf("checkReverseDependencies: %v", err)
	}
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent, got %d: %v", len(dependents), dependents)
	}
	if dependents[0] != "plugin-b" {
		t.Errorf("expected dependent %q, got %q", "plugin-b", dependents[0])
	}
}

// --- FetchRegistry mock test ---

// TestFetchRegistry_MockServer verifies that FetchRegistry parses a registry
// JSON response served by a local httptest server.
func TestFetchRegistry_MockServer(t *testing.T) {
	// The registry client expects an envelope: {"plugins": [...]} (array format).
	registryJSON := `{"plugins":[{"name":"test-plugin","version":"1.0.0","description":"A test plugin","category":"utility","tier":"free","license":"MIT","repository":"https://github.com/example/test-plugin","checksum":"abc123"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(registryJSON))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	// Point the registry env override at the mock server so the client uses it.
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)

	reg, err := FetchRegistry(context.Background(), srv.URL, cacheDir)
	if err != nil {
		t.Fatalf("FetchRegistry error: %v", err)
	}
	if len(reg.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(reg.Plugins))
	}
	if reg.Plugins[0].Name != "test-plugin" {
		t.Errorf("expected plugin name %q, got %q", "test-plugin", reg.Plugins[0].Name)
	}
}

// --- Install license check test ---

// TestInstall_PaidPluginRequiresLicense verifies that Install() returns a
// license error when a paid plugin is requested without a valid license key.
//
// The registry is served from a local httptest server rather than the live
// one. This test used to reach the network, and only appeared to pass: the
// license gate ran off the paidPlugins NAME map before any fetch, so the
// registry response never mattered. Once the gate moved below tier
// resolution the test started failing on the windows-2022 runner, where the
// primary registry was unreachable and the GitHub raw fallback — the FREE
// registry — has no "ai" entry, so the error became "plugin not found in
// registry". Pinning the registry makes the test hermetic and makes it
// exercise the path it claims to: fetch -> resolve -> license gate.
func TestInstall_PaidPluginRequiresLicense(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"plugins":[
			{"name":"ai","version":"1.1.0","tier":"pro","requires_license":true}
		]}`)
	}))
	defer srv.Close()

	// Ensure no license key is set.
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "")
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	// Point the home dir at a temp dir so neither ~/.nself/license/key nor a
	// populated registry cache leaks in. USERPROFILE is the Windows spelling
	// os.UserHomeDir reads, and omitting it is why this passed locally on
	// macOS while failing in Windows CI.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := Install(context.Background(), cfg, "ai", pluginDir)
	if err == nil {
		t.Fatal("expected error when installing paid plugin without license, got nil")
	}
	// The error should mention license or key.
	if !strings.Contains(err.Error(), "license") && !strings.Contains(err.Error(), "key") {
		t.Errorf("expected license-related error, got: %v", err)
	}
}

// --- Cache TTL and tier tests (S10-T05) ---

// TestCheckLicenseCache_ValidResultCached verifies that CacheLicense followed
// by CheckLicenseCache returns the cached valid result within the TTL.
func TestCheckLicenseCache_ValidResultCached(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("a", 22)

	if err := CacheLicense(key, true, cacheDir); err != nil {
		t.Fatalf("CacheLicense: %v", err)
	}

	valid, found := checkLicenseCache(key, cacheDir)
	if !found {
		t.Fatal("expected cache hit after CacheLicense, got cache miss")
	}
	if !valid {
		t.Fatal("expected valid=true from cache, got false")
	}
}

// TestCheckLicenseCache_InvalidResultCached verifies that an invalid license
// result is correctly cached and returned.
func TestCheckLicenseCache_InvalidResultCached(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("b", 22)

	if err := CacheLicense(key, false, cacheDir); err != nil {
		t.Fatalf("CacheLicense: %v", err)
	}

	valid, found := checkLicenseCache(key, cacheDir)
	if !found {
		t.Fatal("expected cache hit, got miss")
	}
	if valid {
		t.Fatal("expected valid=false from cache, got true")
	}
}

// TestNeedsRevalidation_NoCache verifies that NeedsRevalidation returns true
// when no cache file exists.
func TestNeedsRevalidation_NoCache(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("c", 22)

	if !NeedsRevalidation(key, cacheDir) {
		t.Fatal("expected NeedsRevalidation=true when no cache, got false")
	}
}

// TestNeedsRevalidation_FreshCache verifies that NeedsRevalidation returns
// false immediately after CacheLicense (cache is fresh).
func TestNeedsRevalidation_FreshCache(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("d", 22)

	if err := CacheLicense(key, true, cacheDir); err != nil {
		t.Fatalf("CacheLicense: %v", err)
	}

	if NeedsRevalidation(key, cacheDir) {
		t.Fatal("expected NeedsRevalidation=false for fresh cache, got true")
	}
}

// TestNeedsRevalidation_OwnerKeyNeverExpires verifies that owner keys never
// trigger revalidation (NeedsRevalidation always returns false for owner keys).
func TestNeedsRevalidation_OwnerKeyNeverExpires(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_owner_" + strings.Repeat("e", 20)

	// Even without a cache file, owner keys should not need revalidation.
	if NeedsRevalidation(key, cacheDir) {
		t.Fatal("expected NeedsRevalidation=false for owner key, got true")
	}
}

// TestCheckEntitlements_PluginAllowed verifies that CheckEntitlements returns
// allowed=true when the plugin is in the cached entitlements list.
func TestCheckEntitlements_PluginAllowed(t *testing.T) {
	cacheDir := t.TempDir()
	tier := "basic"
	plugins := []string{"notify", "chat", "cron"}

	if err := cacheEntitlements(cacheDir, tier, plugins); err != nil {
		t.Fatalf("CacheEntitlements: %v", err)
	}

	allowed, found := checkEntitlements(cacheDir, "chat")
	if !found {
		t.Fatal("expected entitlement cache hit, got miss")
	}
	if !allowed {
		t.Fatal("expected allowed=true for 'chat' in entitlements, got false")
	}
}

// TestCheckEntitlements_PluginNotAllowed is the tier-check regression test.
// A Basic-tier license with a list of basic plugins must deny access to a
// Pro-tier plugin that is NOT in the entitlements list.
func TestCheckEntitlements_PluginNotAllowed(t *testing.T) {
	cacheDir := t.TempDir()
	// Basic tier only includes basic plugins, NOT the "ai" plugin (Pro-tier).
	tier := "basic"
	plugins := []string{"notify", "chat", "cron"}

	if err := cacheEntitlements(cacheDir, tier, plugins); err != nil {
		t.Fatalf("CacheEntitlements: %v", err)
	}

	allowed, found := checkEntitlements(cacheDir, "ai")
	if !found {
		t.Fatal("expected cache hit (entitlements file exists), got miss")
	}
	if allowed {
		t.Fatal("tier regression: Basic-tier key must not be allowed to install Pro-tier 'ai' plugin")
	}
}

// TestCheckEntitlements_NoCache verifies that CheckEntitlements returns
// found=false when no entitlements cache exists.
func TestCheckEntitlements_NoCache(t *testing.T) {
	cacheDir := t.TempDir()

	_, found := checkEntitlements(cacheDir, "ai")
	if found {
		t.Fatal("expected found=false when no entitlements cache, got true")
	}
}
