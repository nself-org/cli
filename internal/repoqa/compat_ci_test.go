package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestV15SuiteInCI prevents the default compatibility mode from hiding v1.5
// regressions. A required job must run the suite (cmd/commands narrowed, D-0275) with NSELF_V15=1.
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
			if mode != "1" || step.ContinueOnError {
				continue
			}
			// The step must test the module (go list ./...) and keep the Hasura
			// remote tests of cmd/commands under v1.5.
			if strings.Contains(step.Run, "go test") && strings.Contains(step.Run, "go list -mod=vendor ./...") &&
				strings.Contains(step.Run, "-run '^TestDBHasuraRemotePartialFailure$' ./cmd/commands") {
				return
			}
		}
	}
	t.Fatal("CI must run the v1.5 suite (go list ./... plus cmd/commands Hasura tests) in a required job with NSELF_V15=1")
}
