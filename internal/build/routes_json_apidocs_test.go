package build

// Purpose: routes.json carries the api-docs route exactly once, drops it when
//          API_DOCS_ENABLED=false, and a bad API_DOCS_PATH fails the build (D-0310, D-0455).
// Inputs:  the dev-minimal plan fixture with extra .env lines.
// Outputs: none (t.Error on assertion failure).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/nginx/routemodel"
)

// buildRoutesWithEnv builds the dev-minimal fixture with extra .env lines and
// returns the decoded routes.json (nil when the build failed) and the error.
func buildRoutesWithEnv(t *testing.T, extra string) (*routemodel.Model, error) {
	t.Helper()
	f := newPlanFixture(t, "dev-minimal")
	for _, k := range []string{"API_DOCS_ENABLED", "API_DOCS_PATH"} {
		t.Setenv(k, "") // restored on cleanup; Load writes the .env values into the process env
	}
	envPath := filepath.Join(f.workdir, ".env")
	b, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, append(b, []byte(extra)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(f.workdir, BuildOptions{Force: true}); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(f.workdir, ".nself", "generated", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m routemodel.Model
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return &m, nil
}

// countAPIDocsRoutes counts routes that are, or serve, the api-docs site.
func countAPIDocsRoutes(m *routemodel.Model) int {
	n := 0
	for _, r := range m.Routes {
		if r.ID == "core:api-docs" || strings.HasSuffix(r.File, "api-docs.conf") {
			n++
		}
	}
	return n
}

func TestRoutesJSONAPIDocs(t *testing.T) {
	t.Run("default has one api-docs route", func(t *testing.T) {
		m, err := buildRoutesWithEnv(t, "")
		if err != nil {
			t.Fatal(err)
		}
		if n := countAPIDocsRoutes(m); n != 1 {
			t.Fatalf("api-docs routes = %d, want 1", n)
		}
	})
	t.Run("custom path reaches the second location", func(t *testing.T) {
		m, err := buildRoutesWithEnv(t, "API_DOCS_PATH=/reference\n")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range m.Routes {
			if r.ID == "core:api-docs" && (len(r.Locations) != 2 || r.Locations[1].Path != "/reference") {
				t.Fatalf("locations = %+v", r.Locations)
			}
		}
	})
	t.Run("enabled false drops the route", func(t *testing.T) {
		m, err := buildRoutesWithEnv(t, "API_DOCS_ENABLED=false\n")
		if err != nil {
			t.Fatal(err)
		}
		if n := countAPIDocsRoutes(m); n != 0 {
			t.Fatalf("api-docs routes = %d, want 0", n)
		}
	})
	t.Run("bad path fails the build", func(t *testing.T) {
		if _, err := buildRoutesWithEnv(t, "API_DOCS_PATH=/x;\n"); err == nil || !strings.Contains(err.Error(), "API_DOCS_PATH") {
			t.Fatalf("err = %v, want an API_DOCS_PATH error", err)
		}
	})
}
