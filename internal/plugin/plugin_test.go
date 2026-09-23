package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// --- helpers ---

// writeManifestFile writes a minimal valid plugin.json into pluginDir/{name}/plugin.json.
// Tables and licenseType are optional; pass nil/empty to omit them.
func writeManifestFile(t *testing.T, pluginDir, name, version string, tables []string, requiresLicense bool) {
	t.Helper()
	dir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeManifestFile: mkdir: %v", err)
	}

	type manifestJSON struct {
		Name            string   `json:"name"`
		Version         string   `json:"version"`
		Description     string   `json:"description"`
		Category        string   `json:"category"`
		License         string   `json:"license"`
		Tables          []string `json:"tables,omitempty"`
		RequiresLicense bool     `json:"requires_license,omitempty"`
	}

	m := manifestJSON{
		Name:            name,
		Version:         version,
		Description:     "Test plugin " + name,
		Category:        "utility",
		License:         "MIT",
		Tables:          tables,
		RequiresLicense: requiresLicense,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("writeManifestFile: marshal: %v", err)
	}

	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("writeManifestFile: write: %v", err)
	}
}

// writeManifestFileAs is writeManifestFile but lets the on-disk directory
// name and the manifest's internal "name" field differ — needed to simulate
// Update()'s ".prev" backup directory, which keeps the ORIGINAL plugin.json
// (declaring the real plugin name, e.g. "notify") under a renamed directory
// (e.g. "notify.prev"); namePattern rejects dots, so the JSON "name" field
// itself can never legitimately be "notify.prev".
func writeManifestFileAs(t *testing.T, pluginDir, dirName, manifestName, version string, tables []string, requiresLicense bool) {
	t.Helper()
	dir := filepath.Join(pluginDir, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeManifestFileAs: mkdir: %v", err)
	}

	type manifestJSON struct {
		Name            string   `json:"name"`
		Version         string   `json:"version"`
		Description     string   `json:"description"`
		Category        string   `json:"category"`
		License         string   `json:"license"`
		Tables          []string `json:"tables,omitempty"`
		RequiresLicense bool     `json:"requires_license,omitempty"`
	}

	m := manifestJSON{
		Name:            manifestName,
		Version:         version,
		Description:     "Test plugin " + manifestName,
		Category:        "utility",
		License:         "MIT",
		Tables:          tables,
		RequiresLicense: requiresLicense,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("writeManifestFileAs: marshal: %v", err)
	}

	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("writeManifestFileAs: write: %v", err)
	}
}

// --- ListInstalled tests ---

// TestListInstalled_EmptyDir verifies that ListInstalled returns an empty (nil)
// slice and no error when the plugin directory exists but contains no plugins.
func TestListInstalled_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled on empty dir: unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("ListInstalled on empty dir: expected 0 plugins, got %d", len(plugins))
	}
}

// TestListInstalled_NonExistentDir verifies that ListInstalled returns nil, nil
// when the plugin directory does not exist (same behaviour as an empty dir).
func TestListInstalled_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled on non-existent dir: unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("ListInstalled on non-existent dir: expected 0 plugins, got %d", len(plugins))
	}
}

// TestListInstalled_OnePlugin verifies that ListInstalled discovers and returns
// exactly one plugin when a single valid manifest is present.
func TestListInstalled_OnePlugin(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, "my-plugin", "1.0.0", nil, false)

	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "my-plugin" {
		t.Errorf("expected plugin name %q, got %q", "my-plugin", plugins[0].Name)
	}
	if plugins[0].Version != "1.0.0" {
		t.Errorf("expected version %q, got %q", "1.0.0", plugins[0].Version)
	}
}

// TestListInstalled_MultiplePlugins verifies that ListInstalled returns all
// installed plugins when multiple manifest directories are present.
func TestListInstalled_MultiplePlugins(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, "alpha-plugin", "1.0.0", nil, false)
	writeManifestFile(t, dir, "beta-plugin", "2.0.0", nil, false)
	writeManifestFile(t, dir, "gamma-plugin", "3.0.0", nil, false)

	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}

	// Build a set of returned names for order-independent assertion.
	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Name] = true
	}
	for _, expected := range []string{"alpha-plugin", "beta-plugin", "gamma-plugin"} {
		if !names[expected] {
			t.Errorf("expected plugin %q to be in results, but it was not", expected)
		}
	}
}

// TestListInstalled_IgnoresNonDirEntries verifies that files (not directories)
// in the plugin directory are silently skipped.
func TestListInstalled_IgnoresNonDirEntries(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, "real-plugin", "1.0.0", nil, false)

	// Write a plain file at the top level — should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "not-a-plugin.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("writing stray file: %v", err)
	}

	plugins, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled: unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin (stray file ignored), got %d", len(plugins))
	}
}

// --- License validation tests (S1-T05 regression) ---
// These use ValidateLicenseRemote via httptest to avoid any real network calls.

// TestLicense_HTTP200ValidFalse_IsInvalid is the S1-T05 regression test.
// A server that returns HTTP 200 with {"valid": false} must be treated as an
// invalid license — HTTP 200 alone does not mean the license is accepted.
func TestLicense_HTTP200ValidFalse_IsInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"valid":false}`))
	}))
	defer srv.Close()

	valid, err := ValidateLicenseRemote(
		context.Background(),
		"nself_pro_testkey_aaaaaaaaaaaaaaaa",
		srv.URL,
	)
	if valid {
		t.Fatal("S1-T05 regression: HTTP 200 with valid=false must not be treated as a valid license")
	}
	if err == nil {
		t.Fatal("S1-T05 regression: expected a non-nil error when valid=false")
	}
}

// TestLicense_HTTP200ValidTrue_IsValid verifies that a server returning
// HTTP 200 with {"valid": true} is correctly treated as a valid license.
func TestLicense_HTTP200ValidTrue_IsValid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"valid":true,"tier":"pro","plugins":[]}`))
	}))
	defer srv.Close()

	valid, err := ValidateLicenseRemote(
		context.Background(),
		"nself_pro_testkey_aaaaaaaaaaaaaaaa",
		srv.URL,
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !valid {
		t.Fatal("expected valid=true, got false")
	}
}

// TestLicense_NetworkError_UsesCachedResult verifies that when the network is
// unavailable, CheckLicenseCacheOffline returns the previously cached "valid"
// result rather than failing immediately. This exercises the offline grace path.
func TestLicense_NetworkError_UsesCachedResult(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_testkey_aaaaaaaaaaaaaaaa"

	// Pre-populate the cache with a valid result.
	if err := CacheLicense(key, true, cacheDir); err != nil {
		t.Fatalf("CacheLicense: %v", err)
	}

	// Simulate a network outage by using the offline cache check (which is what
	// checkLicense calls when ValidateLicenseRemoteWithEntitlements returns an
	// error indicating network unavailability).
	offlineValid, found := checkLicenseCacheOffline(key, cacheDir)
	if !found {
		t.Fatal("expected cached result to be found after CacheLicense")
	}
	if !offlineValid {
		t.Fatal("expected offline cache to report valid=true for a previously-valid key")
	}
}

// --- Table conflict tests (S2-T04 regression) ---

// TestTablePrefixConflict_SamePrefix_ReturnsError is the S2-T04 regression
// test. Two plugins sharing the same table prefix (e.g. "np_chat_") must be
// rejected with an error.
func TestTablePrefixConflict_SamePrefix_ReturnsError(t *testing.T) {
	dir := t.TempDir()

	// Install an existing plugin that owns the "np_chat_" prefix.
	writeManifestFile(t, dir, "chat-plugin", "1.0.0", []string{"np_chat_messages"}, false)

	// Attempt to install a new plugin that claims the same prefix.
	newTables := []string{"np_chat_rooms"}
	err := checkTablePrefixConflict(dir, "new-chat-plugin", newTables)
	if err == nil {
		t.Fatal("S2-T04 regression: expected error for overlapping table prefix, got nil")
	}
}

// TestTablePrefixConflict_DifferentPrefixes_NoError verifies that two plugins
// with distinct table prefixes do not conflict.
func TestTablePrefixConflict_DifferentPrefixes_NoError(t *testing.T) {
	dir := t.TempDir()

	// Install an existing plugin with the "np_notify_" prefix.
	writeManifestFile(t, dir, "notify-plugin", "1.0.0", []string{"np_notify_events"}, false)

	// New plugin uses a completely different prefix — should be fine.
	newTables := []string{"np_billing_invoices"}
	err := checkTablePrefixConflict(dir, "billing-plugin", newTables)
	if err != nil {
		t.Fatalf("expected no error for distinct table prefixes, got: %v", err)
	}
}

// TestTablePrefixConflict_SamePlugin_Skipped verifies that a reinstall of the
// same plugin (same name) does not trigger a conflict with its own tables.
func TestTablePrefixConflict_SamePlugin_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, "chat-plugin", "1.0.0", []string{"np_chat_messages"}, false)

	// Reinstall — the conflict check must skip the plugin being installed.
	err := checkTablePrefixConflict(dir, "chat-plugin", []string{"np_chat_messages"})
	if err != nil {
		t.Fatalf("expected no conflict when reinstalling the same plugin, got: %v", err)
	}
}

// TestTablePrefixConflict_PrevBackupDir_Skipped is the prod-evidence
// regression test (2026-09-23, nself 1.4.8, "nself plugin update notify"):
// Update() renames the current install to "{name}.prev" before reinstalling,
// so the conflict scan must skip that backup directory too, not just the
// plugin's own live directory name.
func TestTablePrefixConflict_PrevBackupDir_Skipped(t *testing.T) {
	dir := t.TempDir()
	// The backup dir keeps the ORIGINAL plugin.json — Update() renames the
	// directory only, it does not rewrite the manifest inside it.
	writeManifestFileAs(t, dir, "notify.prev", "notify", "1.0.0", []string{"np_notify_events"}, false)

	err := checkTablePrefixConflict(dir, "notify", []string{"np_notify_events"})
	if err != nil {
		t.Fatalf("expected no conflict against own .prev update-backup dir, got: %v", err)
	}
}

// TestTablePrefixConflict_DifferentPlugin_PrevSuffix_StillConflicts verifies
// that skipping "{name}.prev" is scoped to the plugin being installed: a
// genuinely different plugin whose directory happens to end in ".prev" must
// still be treated as a real conflict.
func TestTablePrefixConflict_DifferentPlugin_PrevSuffix_StillConflicts(t *testing.T) {
	dir := t.TempDir()
	writeManifestFileAs(t, dir, "billing.prev", "billing", "1.0.0", []string{"np_notify_events"}, false)

	err := checkTablePrefixConflict(dir, "notify", []string{"np_notify_events"})
	if err == nil {
		t.Fatal("expected conflict: billing.prev is a different plugin, not notify's own backup")
	}
}

// TestTableConflicts_PrevBackupDir_Skipped mirrors
// TestTablePrefixConflict_PrevBackupDir_Skipped for the exact-table-name
// conflict check.
func TestTableConflicts_PrevBackupDir_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeManifestFileAs(t, dir, "cron.prev", "cron", "1.0.0", []string{"np_cron_jobs"}, false)

	newPlugin := &PluginManifest{Name: "cron", Tables: []string{"np_cron_jobs"}}
	if err := checkTableConflicts(dir, newPlugin); err != nil {
		t.Fatalf("expected no conflict against own .prev update-backup dir, got: %v", err)
	}
}

// --- Version regex anchoring tests (S1-T11 regression) ---

// TestVersionRegex_SuffixRejected is the S1-T11 regression test.
// "1.2.3suffix" must not match the version pattern — the regex must be anchored
// so that trailing characters are not silently accepted.
func TestVersionRegex_SuffixRejected(t *testing.T) {
	if versionPattern.MatchString("1.2.3suffix") {
		t.Error("S1-T11 regression: version regex must reject \"1.2.3suffix\" — ensure pattern is anchored with $ at end")
	}
}

// TestVersionRegex_ValidSemver verifies that a well-formed semver string is
// accepted by the version pattern.
func TestVersionRegex_ValidSemver(t *testing.T) {
	if !versionPattern.MatchString("1.2.3") {
		t.Error("expected \"1.2.3\" to be accepted by versionPattern")
	}
}

// TestVersionRegex_PrereleaseAccepted verifies that pre-release suffixes
// (e.g. "0.1.0-beta") are accepted by the semver pattern used for
// plugin manifests. The pattern supports X.Y.Z with optional pre-release
// (-suffix) and build metadata (+suffix).
func TestVersionRegex_PrereleaseAccepted(t *testing.T) {
	if !versionPattern.MatchString("0.1.0-beta") {
		t.Error("expected \"0.1.0-beta\" to be accepted by versionPattern")
	}
}
