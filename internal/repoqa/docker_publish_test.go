package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDockerPublishLatestReusesIndex guards D-0351: a second build produces new
// provenance attestations and a different index digest, even with cached layers.
func TestDockerPublishLatestReusesIndex(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/docker-publish.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string
				Uses string
				If   string
				Run  string
				Env  map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	builds, latest := 0, 0
	for _, step := range workflow.Jobs["docker"].Steps {
		if strings.HasPrefix(step.Uses, "docker/build-push-action@") {
			builds++
		}
		if step.Name != "Tag as latest" {
			continue
		}
		latest++
		if step.If != "steps.version.outputs.latest == 'true'" {
			t.Error("latest must remain conditional on the release preflight")
		}
		if step.Uses != "" || step.Env["VERSION"] != "${{ steps.version.outputs.version }}" {
			t.Error("latest must retag the resolved version without another build action")
		}
		want := `docker buildx imagetools create -t nself/nself:latest "nself/nself:${VERSION}"`
		if strings.TrimSpace(step.Run) != want {
			t.Errorf("latest must copy the published index: got %q, want %q", step.Run, want)
		}
	}
	if builds != 1 || latest != 1 {
		t.Errorf("want one build and one latest promotion, got %d builds and %d promotions", builds, latest)
	}
}
