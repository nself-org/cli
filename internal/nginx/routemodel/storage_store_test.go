package routemodel

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// routeByID returns the built route with the given id, or nil.
func routeByID(m *Model, id string) *Route {
	for i := range m.Routes {
		if m.Routes[i].ID == id {
			return &m.Routes[i]
		}
	}
	return nil
}

// TestStorageRouteFollowsActiveStore: v1.5 SeaweedFS gets a storage route to seaweedfs:8333
// and no console route; v1.4 and MinIO keep the MinIO upstreams (D-0683).
func TestStorageRouteFollowsActiveStore(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Run("v1.5 seaweedfs", func(t *testing.T) {
		compattest.Set(t, true)
		t.Setenv("OBJECT_STORAGE_ENABLED", "true")
		cfg := &config.Config{ProjectName: "demo", BaseDomain: "example.test"}
		m, err := Build(cfg, t.TempDir(), false)
		if err != nil {
			t.Fatal(err)
		}
		r := routeByID(m, "core:storage")
		if r == nil || r.RawUpstream != "seaweedfs:8333" {
			t.Fatalf("storage route = %+v, want upstream seaweedfs:8333", r)
		}
		if routeByID(m, "core:storage-console") != nil {
			t.Fatal("SeaweedFS has no console: storage-console route must not exist")
		}
	})
	t.Run("v1.4 minio", func(t *testing.T) {
		compattest.Set(t, false)
		t.Setenv("OBJECT_STORAGE_ENABLED", "false")
		cfg := &config.Config{ProjectName: "demo", BaseDomain: "example.test"}
		cfg.Minio.Enabled, cfg.Minio.Port = true, 9000
		m, err := Build(cfg, t.TempDir(), false)
		if err != nil {
			t.Fatal(err)
		}
		if r := routeByID(m, "core:storage"); r == nil || r.RawUpstream != "minio:9000" {
			t.Fatalf("v1.4 storage route = %+v", r)
		}
		if routeByID(m, "core:storage-console") == nil {
			t.Fatal("v1.4 keeps the console route")
		}
	})
}
