package repoqa

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLegacyCILeavesDefaultLinuxChecksToNselfCI prevents duplicate checks while
// preserving the OS matrix and the additional compatibility and contract gates.
func TestLegacyCILeavesDefaultLinuxChecksToNselfCI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ job, step string }{
		{"fmt", "gofmt check"},
		{"ci", "Vet"},
		{"ci", "Build"},
		{"ci", "Test"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			for _, step := range workflow.Jobs[tc.job].Steps {
				if step.Name != tc.step {
					continue
				}
				if tc.job == "fmt" || step.If != "matrix.skip_docker_tests != 'true' && matrix.os != 'ubuntu-latest'" {
					t.Errorf("%s duplicates the required nself-ci default Linux check", tc.step)
				}
			}
		})
	}
}
