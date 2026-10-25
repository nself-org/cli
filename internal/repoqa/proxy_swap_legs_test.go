package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestProxySwapRecoveryLegsRunOnPullRequests: the round-trip and kill-mid-remove
// legs of the proxy swap e2e used to run only on manual dispatch, so a pull
// request that broke the way back from a swap, or the recovery after a kill,
// stayed green (D-1165). Every leg step must run on pull_request.
func TestProxySwapRecoveryLegsRunOnPullRequests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/proxy-swap-e2e.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
				If   string `yaml:"if"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			for _, leg := range []string{"--add", "--round-trip", "--kill-mid-remove", "--renew"} {
				if !strings.Contains(step.Run, "proxy-swap.sh "+leg) {
					continue
				}
				seen[leg] = true
				if !strings.Contains(step.If, "github.event_name == 'pull_request'") {
					t.Errorf("step %q (proxy-swap.sh %s) does not run on pull_request: if = %q", step.Name, leg, step.If)
				}
			}
		}
	}
	for _, leg := range []string{"--add", "--round-trip", "--kill-mid-remove", "--renew"} {
		if !seen[leg] {
			t.Errorf("no workflow step runs proxy-swap.sh %s", leg)
		}
	}
}
