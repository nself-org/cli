package plugin

// Purpose: Tests for the trust gates on the third-party URL install (D-0842):
//          the v1.5 unsigned-install refusal with its --allow-unsigned opt-in,
//          and the refusal to take the name of an official registry plugin.
// Constraints: the registry lookup goes through the thirdPartyRegistry seam so
//              no test reaches the network.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestMain keeps every test in this package off the real registry: the URL
// install now looks the plugin name up there.
func TestMain(m *testing.M) {
	thirdPartyRegistry = func(context.Context) (*Registry, error) { return &Registry{}, nil }
	os.Exit(m.Run())
}

func stubThirdPartyRegistry(t *testing.T, reg *Registry, err error) {
	t.Helper()
	old := thirdPartyRegistry
	thirdPartyRegistry = func(context.Context) (*Registry, error) { return reg, err }
	t.Cleanup(func() { thirdPartyRegistry = old })
}

// servedTarball serves the demo plugin and counts the downloads.
func servedTarball(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	tarball := buildThirdPartyTarball(t, validThirdPartyManifest)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestInstallFromURL_V15RefusesUnsignedWithoutFlag: a URL install has no
// release signature, so in v1.5 it needs --allow-unsigned. Without it the
// install stops with E107 before anything is downloaded.
func TestInstallFromURL_V15RefusesUnsignedWithoutFlag(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv(allowUnsignedEnv, "")
	srv, hits := servedTarball(t)
	pluginDir := t.TempDir()

	err := InstallFromURL(context.Background(), &config.Config{}, srv.URL+"/p.tar.gz", pluginDir, "")
	if codeOfErr(err) != "E107" {
		t.Fatalf("want E107, got %v", err)
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Errorf("refused install still downloaded the archive (%d request(s))", n)
	}
	if _, statErr := os.Stat(filepath.Join(pluginDir, "third-party-demo")); !os.IsNotExist(statErr) {
		t.Errorf("refused install left a plugin directory (stat err %v)", statErr)
	}
}

// TestInstallFromURL_V15AllowUnsignedProceeds: --allow-unsigned (threaded as
// NSELF_PLUGIN_ALLOW_UNSIGNED) lets the v1.5 URL install through.
func TestInstallFromURL_V15AllowUnsignedProceeds(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv(allowUnsignedEnv, "1")
	srv, _ := servedTarball(t)

	err := InstallFromURL(context.Background(), &config.Config{}, srv.URL+"/p.tar.gz", t.TempDir(), "")
	assertGotPastVerification(t, err)
	if codeOfErr(err) == "E107" {
		t.Fatalf("--allow-unsigned still refused: %v", err)
	}
}

// TestInstallFromURL_V14StillInstallsUnsigned: the v1.4 behaviour is kept.
func TestInstallFromURL_V14StillInstallsUnsigned(t *testing.T) {
	t.Setenv("NSELF_V15", "")
	t.Setenv(allowUnsignedEnv, "")
	srv, _ := servedTarball(t)

	err := InstallFromURL(context.Background(), &config.Config{}, srv.URL+"/p.tar.gz", t.TempDir(), "")
	assertGotPastVerification(t, err)
	if codeOfErr(err) == "E107" {
		t.Fatalf("v1.4 URL install was refused: %v", err)
	}
}

// TestInstallFromURL_RefusesRegistryPluginName: a URL archive cannot take the
// name of a plugin the registry serves, and the installed copy is untouched.
func TestInstallFromURL_RefusesRegistryPluginName(t *testing.T) {
	t.Setenv("NSELF_V15", "")
	stubThirdPartyRegistry(t, &Registry{Plugins: []PluginManifest{{Name: "third-party-demo"}}}, nil)
	srv, _ := servedTarball(t)

	pluginDir := t.TempDir()
	installed := filepath.Join(pluginDir, "third-party-demo")
	if err := os.MkdirAll(installed, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(installed, "official.marker")
	if err := os.WriteFile(marker, []byte("official"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := InstallFromURL(context.Background(), &config.Config{}, srv.URL+"/p.tar.gz", pluginDir, "")
	if codeOfErr(err) != "E106" {
		t.Fatalf("want E106, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("refused install replaced the official plugin directory: %v", statErr)
	}
}

// TestInstallFromURL_V15RegistryLookupFailsClosed: when the registry cannot be
// read the name check cannot run, so v1.5 refuses instead of installing blind.
func TestInstallFromURL_V15RegistryLookupFailsClosed(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv(allowUnsignedEnv, "1")
	stubThirdPartyRegistry(t, nil, errors.New("registry unreachable"))
	srv, _ := servedTarball(t)

	pluginDir := t.TempDir()
	err := InstallFromURL(context.Background(), &config.Config{}, srv.URL+"/p.tar.gz", pluginDir, "")
	if err == nil {
		t.Fatal("want a refusal when the registry cannot be read, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(pluginDir, "third-party-demo")); !os.IsNotExist(statErr) {
		t.Errorf("failed-closed install left a plugin directory (stat err %v)", statErr)
	}
}
