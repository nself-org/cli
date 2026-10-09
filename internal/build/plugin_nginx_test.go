package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginNginxRouteMarkerAndPrune(t *testing.T) {
	project, plugins := t.TempDir(), t.TempDir()
	conf := filepath.Join(plugins, "fixture", "nginx", "route.conf")
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("server { listen 80; server_name fixture.test; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if count, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test")); err != nil || count != 1 {
		t.Fatalf("inject: count=%d err=%v", count, err)
	}
	sites := filepath.Join(project, "nginx", "sites")
	data, err := os.ReadFile(filepath.Join(sites, "fixture-route.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), nginxGeneratedMarker+"\n") {
		t.Fatalf("copied conf missing generated marker: %q", data)
	}
	if removed, foreign, err := pruneGeneratedNginxSites(sites); err != nil || removed != 1 || len(foreign) != 0 {
		t.Fatalf("prune: removed=%d foreign=%v err=%v", removed, foreign, err)
	}
}
