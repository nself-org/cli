package plugin

// Purpose: D-0827 test that a --tier override (NSELF_PLUGIN_INSTALL_TIER)
// applies to the plugin the operator named only, never to its dependencies.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestInstallDependenciesIgnoresTierOverride: `plugin update app --tier
// licensed` used to force "licensed" onto a free-only dependency that was
// missing, and ResolvePluginTier rejected it. The override must not reach the
// dependency, and must be back in place when the dependencies are done.
func TestInstallDependenciesIgnoresTierOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tarball") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"plugins":[{"name":"dep","version":"1.0.0","description":"d","category":"c","license":"MIT","tier":"free"}]}`))
	}))
	defer srv.Close()
	setTestHome(t, t.TempDir())
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "pro")

	m := &PluginManifest{Name: "app", Dependencies: []string{"dep"}}
	err := installDependencies(context.Background(), &config.Config{}, "app", m, t.TempDir())
	if err != nil && strings.Contains(err.Error(), "only has a") {
		t.Fatalf("the tier override reached the dependency: %v", err)
	}
	if v := os.Getenv("NSELF_PLUGIN_INSTALL_TIER"); v != "pro" {
		t.Fatalf("the override must be restored for the rest of the install, got %q", v)
	}
}
