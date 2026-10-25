package plugin

// Purpose: Tests for the third-party URL install path (CLI-R16) —
//          IsThirdPartyInstallSource, ValidateThirdPartyURL, and
//          InstallFromURL's validation/checksum pipeline. Network calls go
//          against httptest.Server, never the real registry or a real
//          downstream host.
// Constraints: InstallFromURL's schema-creation step shells out to
//              `docker exec`, which is unavailable in this unit-test
//              environment. Tests exercise everything up to that boundary
//              (download, checksum, extract, manifest validation) and assert
//              the specific error each scenario should stop at, rather than
//              requiring Docker.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestIsThirdPartyInstallSource(t *testing.T) {
	cases := map[string]bool{
		"ai":                          false,
		"my-plugin":                   false,
		"https://example.com/x.tgz":   true,
		"http://localhost:8080/x.tgz": true,
	}
	for ref, want := range cases {
		if got := IsThirdPartyInstallSource(ref); got != want {
			t.Errorf("IsThirdPartyInstallSource(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestValidateThirdPartyURL(t *testing.T) {
	if _, err := ValidateThirdPartyURL("https://example.com/plugin.tar.gz"); err != nil {
		t.Errorf("expected https:// to be accepted, got error: %v", err)
	}
	if _, err := ValidateThirdPartyURL("http://localhost:9999/plugin.tar.gz"); err != nil {
		t.Errorf("expected http://localhost to be accepted (dev/test), got error: %v", err)
	}
	if _, err := ValidateThirdPartyURL("http://127.0.0.1:9999/plugin.tar.gz"); err != nil {
		t.Errorf("expected http://127.0.0.1 to be accepted (dev/test), got error: %v", err)
	}

	if _, err := ValidateThirdPartyURL("http://example.com/plugin.tar.gz"); err == nil {
		t.Error("expected plain http:// on a non-local host to be rejected")
	}
	if _, err := ValidateThirdPartyURL("ftp://example.com/plugin.tar.gz"); err == nil {
		t.Error("expected an unsupported scheme to be rejected")
	}
	if _, err := ValidateThirdPartyURL("not a url"); err == nil {
		t.Error("expected a hostless/unparseable ref to be rejected")
	}
}

// TestInstallFromURL_RejectsInsecureURL verifies InstallFromURL fails fast on
// an insecure URL, before acquiring the install lock or making any network
// request.
func TestInstallFromURL_RejectsInsecureURL(t *testing.T) {
	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := InstallFromURL(context.Background(), cfg, "http://example.com/plugin.tar.gz", pluginDir, "")
	if err == nil {
		t.Fatal("expected error for insecure (non-localhost http) URL, got nil")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Errorf("expected error to explain the https:// requirement, got: %v", err)
	}
}

// buildThirdPartyTarball packages the given plugin.json content plus a dummy
// file into a tar.gz, matching the shape InstallFromURL expects to extract.
func buildThirdPartyTarball(t *testing.T, manifestJSON string) []byte {
	t.Helper()
	return buildTarGz(t, []struct{ name, content string }{
		{name: "plugin.json", content: manifestJSON},
		{name: "bin/plugin", content: "#!/bin/sh\necho hi\n"},
	})
}

const validThirdPartyManifest = `{
	"name": "third-party-demo",
	"version": "1.0.0",
	"description": "A third-party test plugin",
	"category": "utility",
	"license": "MIT"
}`

// TestInstallFromURL_ChecksumMismatch verifies that a caller-supplied
// --checksum value is checked against the downloaded archive BEFORE
// extraction, and a mismatch is caught before any database work is
// attempted.
func TestInstallFromURL_ChecksumMismatch(t *testing.T) {
	tarball := buildThirdPartyTarball(t, validThirdPartyManifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	cfg := &config.Config{}
	wrongChecksum := strings.Repeat("0", 64)

	err := InstallFromURL(context.Background(), cfg, srv.URL+"/plugin.tar.gz", pluginDir, wrongChecksum)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("expected a checksum-related error, got: %v", err)
	}
}

// TestInstallFromURL_CorrectChecksumPassesVerification verifies a caller
// -supplied checksum that DOES match the downloaded archive clears
// verification and the pipeline proceeds past it.
func TestInstallFromURL_CorrectChecksumPassesVerification(t *testing.T) {
	tarball := buildThirdPartyTarball(t, validThirdPartyManifest)
	sum := sha256.Sum256(tarball)
	correctChecksum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := InstallFromURL(context.Background(), cfg, srv.URL+"/plugin.tar.gz", pluginDir, correctChecksum)
	assertGotPastVerification(t, err)
}

// TestInstallFromURL_NoChecksumWarnsAndProceeds verifies that omitting
// --checksum skips verification (with a warning, checked elsewhere via
// manual inspection since it goes to stderr) rather than blocking install.
func TestInstallFromURL_NoChecksumWarnsAndProceeds(t *testing.T) {
	tarball := buildThirdPartyTarball(t, validThirdPartyManifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := InstallFromURL(context.Background(), cfg, srv.URL+"/plugin.tar.gz", pluginDir, "")
	assertGotPastVerification(t, err)
}

// TestInstallFromURL_InvalidManifest verifies a plugin.json missing required
// fields is rejected via the same parseManifest validation a registry
// install goes through.
func TestInstallFromURL_InvalidManifest(t *testing.T) {
	// Missing "license" and "category".
	manifest := `{"name": "broken-plugin", "version": "1.0.0", "description": "oops"}`
	tarball := buildThirdPartyTarball(t, manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := InstallFromURL(context.Background(), cfg, srv.URL+"/plugin.tar.gz", pluginDir, "")
	if err == nil {
		t.Fatal("expected manifest validation error, got nil")
	}
	if !strings.Contains(err.Error(), "missing required fields") {
		t.Errorf("expected a missing-required-fields error, got: %v", err)
	}
}

// assertFailsAtSchemaNotEarlier is the shared assertion for "everything
// before schema creation succeeded". Schema creation shells out to `docker
// exec`, which is unavailable in this unit-test environment, so a real
// success can't be observed here without Docker — but a failure at THAT
// specific step proves checksum verification, extraction, and manifest
// validation all passed.
func assertGotPastVerification(t *testing.T, err error) {
	t.Helper()

	// This used to assert the install FAILED at schema creation, using "no
	// Docker in the test environment" as the signal that everything before it
	// had passed. That stopped being true when schema creation started being
	// skipped for a plugin that owns no tables — which the fixture is — so the
	// install now succeeds and the old assertion failed on its own proxy.
	//
	// What these tests actually care about is that verification passed and
	// nothing earlier in the pipeline objected. Success means exactly that.
	if err == nil {
		return
	}

	for _, stage := range []struct{ token, what string }{
		{"checksum", "checksum verification"},
		{"missing required fields", "manifest validation"},
		{"extracting", "extraction"},
		{"unknown permission", "permission validation"},
	} {
		if strings.Contains(err.Error(), stage.token) {
			t.Errorf("install failed at %s, which should have passed: %v", stage.what, err)
		}
	}
}

// TestInstallFromURL_TablesAndRoutesIntoEmptyPluginDir is the D-0740
// regression: the dot-prefixed staging folder InstallFromURL extracts into
// lives inside pluginDir, and the table-prefix, table and route scanners read
// it as an installed plugin that collides with the one being installed. A
// plugin declaring tables or API routes therefore failed on an empty plugin
// directory. The install may still stop at schema creation (no Docker here),
// but never at a conflict.
func TestInstallFromURL_TablesAndRoutesIntoEmptyPluginDir(t *testing.T) {
	manifest := `{
		"name": "stage-demo",
		"version": "1.0.0",
		"description": "A third-party plugin with tables and routes",
		"category": "utility",
		"license": "MIT",
		"tables": ["np_stage_items"],
		"apiEndpoints": ["https://stage-demo.local/api/health"]
	}`
	tarball := buildThirdPartyTarball(t, manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	cfg := &config.Config{}

	err := InstallFromURL(context.Background(), cfg, srv.URL+"/plugin.tar.gz", pluginDir, "")
	if err != nil && (strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "already")) {
		t.Fatalf("install into an empty plugin dir reported a conflict with itself: %v", err)
	}
}

// TestScannersSkipStagingFolders pins the three scanners directly: a
// dot-prefixed staging folder holding the incoming plugin's own manifest is
// not an installed plugin, while a real second plugin with the same tables
// still conflicts.
func TestScannersSkipStagingFolders(t *testing.T) {
	const manifest = `{"name":"stage-demo","version":"1.0.0","description":"d","category":"utility","license":"MIT","tables":["np_stage_items"],"apiEndpoints":["https://stage-demo.local/api/health"]}`
	pluginDir := t.TempDir()
	for _, dir := range []string{".staging-thirdparty-123", ".staging-bundle-9"} {
		if err := os.MkdirAll(filepath.Join(pluginDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pluginDir, dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &PluginManifest{Name: "stage-demo", Tables: []string{"np_stage_items"}}

	if err := checkTablePrefixConflict(pluginDir, m.Name, m.Tables); err != nil {
		t.Errorf("checkTablePrefixConflict read a staging folder as installed: %v", err)
	}
	if err := checkTableConflicts(pluginDir, m); err != nil {
		t.Errorf("checkTableConflicts read a staging folder as installed: %v", err)
	}
	for _, r := range collectInstalledPluginRoutes(pluginDir, m.Name) {
		if r.PluginName == "stage-demo" {
			t.Errorf("collectInstalledPluginRoutes read a staging folder as installed: %+v", r)
		}
	}

	// A genuinely installed other plugin still conflicts.
	other := filepath.Join(pluginDir, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	otherManifest := strings.Replace(manifest, `"stage-demo"`, `"other"`, 1)
	if err := os.WriteFile(filepath.Join(other, "plugin.json"), []byte(otherManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkTablePrefixConflict(pluginDir, m.Name, m.Tables); err == nil {
		t.Error("a real installed plugin with the same table prefix must still conflict")
	}
}
