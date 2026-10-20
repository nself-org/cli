package commands

import (
	"context"
	"github.com/nself-org/cli/internal/deploy"
	"os"
	"path/filepath"
	"testing"
)

func TestScratchAllowlistedSiteSurvivesBuildPrune(t *testing.T) {
	for _, regenerated := range []bool{false, true} {
		t.Run(map[bool]string{false: "pruned", true: "regenerated"}[regenerated], func(t *testing.T) {
			proj := nginxProject(t, "NSELF_DEPLOY_NGINX_INCLUDE=sites/stale.conf\n")
			ran := 0
			useScratchBuild(t, scratchBuildStub(t, &ran, func(root string) {
				p := filepath.Join(root, "nginx", "sites", "stale.conf")
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				if regenerated {
					wf(t, p, testBuildMarker+"# regenerated\n")
				}
			}))
			scratch, err := runScratchBuild(context.Background(), proj, "staging", deploy.TargetSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(scratch, "nginx", "sites", "stale.conf"))
			if err != nil {
				t.Fatal(err)
			}
			want := testBuildMarker + "# stale generated output\n"
			if regenerated {
				want = testBuildMarker + "# regenerated\n"
			}
			if string(got) != want {
				t.Fatal("allowlisted content lost or regenerated content overwritten")
			}
		})
	}
}
