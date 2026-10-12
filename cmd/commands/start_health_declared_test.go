package commands

// start_health_declared_test.go: D-0502. The start health report drops a
// failed check for a service the generated compose does not declare (nginx
// under the traefik provider) and recounts; declared services and healthy
// results always stay.

import (
	"testing"

	"github.com/nself-org/cli/internal/health"
)

func healthFixture() *health.HealthReport {
	return &health.HealthReport{
		Results: []health.HealthResult{
			{Service: "postgres", Status: "healthy"},
			{Service: "hasura", Status: "starting"},
			{Service: "nginx", Status: "not_found"},
			{Service: "minio", Status: "running"},
		},
		Healthy: 2, Unhealthy: 2, Total: 4,
	}
}

func TestFilterDeclaredHealth(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		declared                  map[string]bool
		want                      []string
		healthy, unhealthy, total int
	}{
		// Positive control: an unreadable compose keeps the old list, nginx included.
		{name: "compose unreadable", declared: nil, want: []string{"postgres", "hasura", "nginx", "minio"}, healthy: 2, unhealthy: 2, total: 4},
		{name: "nginx not generated", declared: map[string]bool{"postgres": true, "hasura": true, "minio": true}, want: []string{"postgres", "hasura", "minio"}, healthy: 2, unhealthy: 1, total: 3},
		{name: "nginx generated but missing", declared: map[string]bool{"postgres": true, "hasura": true, "nginx": true, "minio": true}, want: []string{"postgres", "hasura", "nginx", "minio"}, healthy: 2, unhealthy: 2, total: 4},
		// A healthy result is never hidden, even when its name is not declared.
		{name: "healthy undeclared kept", declared: map[string]bool{"hasura": true}, want: []string{"postgres", "hasura", "minio"}, healthy: 2, unhealthy: 1, total: 3},
	} {
		in := healthFixture()
		got := filterDeclaredHealth(in, tc.declared)
		var names []string
		for _, r := range got.Results {
			names = append(names, r.Service)
		}
		if len(names) != len(tc.want) {
			t.Fatalf("%s: services %v, want %v", tc.name, names, tc.want)
		}
		for i := range names {
			if names[i] != tc.want[i] {
				t.Fatalf("%s: services %v, want %v", tc.name, names, tc.want)
			}
		}
		if got.Healthy != tc.healthy || got.Unhealthy != tc.unhealthy || got.Total != tc.total {
			t.Errorf("%s: %d/%d healthy, %d unhealthy; want %d/%d, %d", tc.name, got.Healthy, got.Total, got.Unhealthy, tc.healthy, tc.total, tc.unhealthy)
		}
		if len(in.Results) != 4 || in.Total != 4 {
			t.Errorf("%s: input report was mutated", tc.name)
		}
	}
	if filterDeclaredHealth(nil, map[string]bool{"x": true}) != nil {
		t.Error("nil report must stay nil")
	}
}

// TestComposeDeclaredServicesUnreadable: when docker compose cannot list the
// services the declared set is nil, so the filter keeps every result.
func TestComposeDeclaredServicesUnreadable(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no docker binary: the listing fails
	if got := composeDeclaredServices(t.Context(), t.TempDir()); got != nil {
		t.Fatalf("declared=%v, want nil when compose cannot be read", got)
	}
}
