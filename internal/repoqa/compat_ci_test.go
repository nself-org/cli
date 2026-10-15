package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestV15SuiteInCI prevents the default compatibility mode from hiding v1.5
// regressions. A required job must run the entire suite with NSELF_V15=1.
func TestV15SuiteInCI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Env             map[string]string `yaml:"env"`
			ContinueOnError bool              `yaml:"continue-on-error"`
			Steps           []struct {
				Env             map[string]string `yaml:"env"`
				Run             string            `yaml:"run"`
				If              string            `yaml:"if"`
				ContinueOnError bool              `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, job := range workflow.Jobs {
		if job.ContinueOnError {
			continue
		}
		for _, step := range job.Steps {
			mode := job.Env["NSELF_V15"]
			if value, ok := step.Env["NSELF_V15"]; ok {
				mode = value
			}
			args := strings.Fields(step.Run)
			if mode != "1" || len(args) < 3 || args[0] != "go" || args[1] != "test" || step.ContinueOnError {
				continue
			}
			for _, arg := range args[2:] {
				if arg == "./..." {
					return
				}
			}
		}
	}
	t.Fatal("CI must run go test ./... in a required job with NSELF_V15=1")
}
