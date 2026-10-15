package build

import (
	"strings"
	"testing"
)

func TestPluginPlanUsesInstalledPaths(t *testing.T) {
	p := &PlannedBuild{Files: map[string]PlannedFile{".nself/compose-files.txt": {Data: []byte("/temp/staged/foo/docker-compose.plugin.yml\n")}}, Effects: []PlannedEffect{{Target: "/temp/staged/foo"}}}
	normalizePluginPlan(p, BuildOptions{PlanPluginDir: "/temp/staged", PlanPluginTarget: "/installed/plugins"})
	if got := string(p.Files[".nself/compose-files.txt"].Data); strings.Contains(got, "/temp/staged") || !strings.Contains(got, "/installed/plugins/foo/") {
		t.Fatal(got)
	}
	if p.Effects[0].Target != "/installed/plugins/foo" {
		t.Fatal(p.Effects)
	}
}

func TestPluginPlanWriteRefused(t *testing.T) {
	if _, err := Build(t.TempDir(), BuildOptions{PlanPluginDir: "/staging"}); err == nil {
		t.Fatal("write mode accepted staged plugins")
	}
}
