package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestV15SuiteInCI prevents the default compatibility mode from hiding v1.5
// regressions. A required job must run the whole module, cmd/commands included
// (D-0275: named skips only), with NSELF_V15=1.
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
			if runsV15Suite(step.Run) {
				return
			}
		}
	}
	t.Fatal("CI must run the whole v1.5 suite, cmd/commands included, in a required job with NSELF_V15=1")
}

// runsV15Suite reports whether a step's script tests the whole module: go test
// over ./... with nothing filtered out of the package list. Skipping named
// tests (-skip) is allowed; dropping a package is not.
func runsV15Suite(run string) bool {
	return strings.Contains(run, "go test") && strings.Contains(run, " ./...") &&
		!strings.Contains(run, "grep -v") && !strings.Contains(run, "-run '")
}

// TestRunsV15SuiteControls is the positive control: the old narrowed shapes are
// refused and the whole-module form is accepted.
func TestRunsV15SuiteControls(t *testing.T) {
	for _, tc := range []struct {
		name, run string
		want      bool
	}{
		{"whole module with skips", "go test -mod=vendor -timeout 20m -skip '^(TestX)$' ./...", true},
		{"cmd/commands filtered out", "go test -mod=vendor $(go list -mod=vendor ./... | grep -v '/cmd/commands$')", false},
		{"only a -run selection", "go test -mod=vendor -run '^TestX$' ./cmd/commands", false},
		{"no go test", "echo ./...", false},
	} {
		if got := runsV15Suite(tc.run); got != tc.want {
			t.Errorf("%s: runsV15Suite = %v, want %v", tc.name, got, tc.want)
		}
	}
}
