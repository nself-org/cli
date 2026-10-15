package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/nginx/routemodel"
)

// TestFrontedAPIDocsOwnership exercises the served directory through real builds,
// including the legacy fronted layout and the shared-proxy registration path.
func TestFrontedAPIDocsOwnership(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "shared"}[shared], func(t *testing.T) {
			x := newSharedFx(t)
			owner := x.env("owna", "a.test")
			if shared {
				owner = x.ownerEnv()
			}
			x.mustBuild(t, x.owner, owner)
			ownerPath := filepath.Join(x.sites(), "api-docs.conf")
			ownerDocs := readText(t, ownerPath)
			x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
			x.mustBuild(t, x.other(), x.frontedEnv("cproj", "c.test"))
			bPath := filepath.Join(x.sites(), "bproj.api-docs.conf")
			cPath := filepath.Join(x.sites(), "cproj.api-docs.conf")
			_ = readText(t, bPath)
			var routes routemodel.Model
			if err := json.Unmarshal([]byte(readText(t, filepath.Join(x.backend(), ".nself", "generated", "routes.json"))), &routes); err != nil {
				t.Fatal(err)
			}
			for _, route := range routes.Routes {
				if route.ID == "core:api-docs" && route.File != "nginx/sites/bproj.api-docs.conf" {
					t.Errorf("docs route points at %s", route.File)
				}
			}
			otherDocs := readText(t, cPath)
			if got := readText(t, ownerPath); got != ownerDocs {
				t.Fatal("fronted build overwrote owner's docs")
			}
			for _, kv := range os.Environ() {
				if key, _, ok := strings.Cut(kv, "="); ok && isBuildEnvKey(key) {
					_ = os.Unsetenv(key)
				}
			}
			plan, err := Build(x.backend(), BuildOptions{Mode: ModePlan, EnvOverlay: map[string][]byte{
				".env": []byte(x.frontedEnv("bproj", "b.test", "API_DOCS_ENABLED=false")),
			}})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(plan.Planned.Removed, "@fronting/bproj.api-docs.conf") {
				t.Fatalf("disabled docs removal missing from plan: %v", plan.Planned.Removed)
			}
			_ = readText(t, bPath) // planning must leave the live docs in place
			x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test", "API_DOCS_ENABLED=false"))
			if _, err := os.Stat(bPath); !os.IsNotExist(err) {
				t.Fatalf("disabled docs remain served: %v", err)
			}
			if readText(t, cPath) != otherDocs || readText(t, ownerPath) != ownerDocs {
				t.Fatal("disabling docs changed another project's docs")
			}
			// A manual replacement at our old path is not ours to remove.
			if !shared {
				writeFixtureFile(t, bPath, "# operator-managed\nserver {}\n", 0o644)
				x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test", "API_DOCS_ENABLED=false"))
				if got := readText(t, bPath); got != "# operator-managed\nserver {}\n" {
					t.Fatal("removed an operator-managed conf")
				}
			}
		})
	}
}
