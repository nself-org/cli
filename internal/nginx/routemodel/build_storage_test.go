package routemodel

import (
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: D-0770 regression. v1.5 stores objects in core SeaweedFS (OBJECT_STORAGE_ENABLED),
// so the model must carry a core:storage route to seaweedfs:8333 even with MINIO_ENABLED=false.
// Inputs: NSELF_V15 and OBJECT_STORAGE_ENABLED env, MINIO_ENABLED via cfg.Minio.Enabled.
// Outputs: the core:storage route table assertions below.
func TestBuildStorageRoute(t *testing.T) {
	tests := []struct {
		name         string
		v15, objects bool
		minio        bool
		wantUpstream string // "" means no core:storage route
		wantConsole  bool
	}{
		{"v15 seaweedfs", true, true, false, "seaweedfs:8333", false},
		{"v15 seaweedfs wins over minio intent", true, true, true, "seaweedfs:8333", false},
		{"v15 minio plugin keeps its route", true, false, true, "minio:9000", true},
		{"v15 no storage", true, false, false, "", false},
		{"v14 ignores object storage", false, true, false, "", false},
		{"v14 core minio", false, false, true, "minio:9000", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_V15", map[bool]string{true: "1", false: "0"}[tc.v15])
			t.Setenv("OBJECT_STORAGE_ENABLED", map[bool]string{true: "true", false: "false"}[tc.objects])
			cfg := &config.Config{ProjectName: "b", BaseDomain: "b.test", Env: "dev"}
			cfg.Minio.Enabled = tc.minio
			cfg.Minio.Port = 9000
			m, err := Build(cfg, t.TempDir(), false)
			if err != nil {
				t.Fatal(err)
			}
			var storage, console *Route
			for i := range m.Routes {
				switch m.Routes[i].ID {
				case "core:storage":
					storage = &m.Routes[i]
				case "core:storage-console":
					console = &m.Routes[i]
				}
			}
			if (storage != nil) != (tc.wantUpstream != "") {
				t.Fatalf("core:storage present = %v, want upstream %q", storage != nil, tc.wantUpstream)
			}
			if storage != nil && storage.RawUpstream != tc.wantUpstream {
				t.Errorf("core:storage upstream = %q, want %q", storage.RawUpstream, tc.wantUpstream)
			}
			if (console != nil) != tc.wantConsole {
				t.Errorf("core:storage-console present = %v, want %v", console != nil, tc.wantConsole)
			}
		})
	}
}
