package commands

// Purpose: D-0849. Auto-install resolves only from the configured registry: an entry cached from another registry is not used, and a failing registry is an error, not a fallback to the public host.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin"
)

func TestAutoInstallIgnoresCacheOfAnotherRegistry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	dir := plugin.DefaultCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cached := `{"origin":"https://old-registry.example/registry.json","plugins":[{"name":"ci","version":"1.0.0","pluginType":"cli"}]}`
	if err := os.WriteFile(filepath.Join(dir, "registry.json"), []byte(cached), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := autoInstallResolve(t.Context(), aiSlug, "")
	if err == nil || !strings.Contains(err.Error(), "fetching the catalogue") {
		t.Fatalf("resolved from another registry's cache or fell back: %v", err)
	}
}
