package repoqa

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

// branchProtectionPolicy is the slice of scripts/.policy/branch-protection.yaml
// this guard reads: the pending app-pinned nself-ci blocks and the per-repo
// required_status_checks.checks lists.
type branchProtectionPolicy struct {
	Pending map[string][]yaml.Node `yaml:"pending_nself_ci"`
	Repos   map[string]struct {
		Required struct {
			Checks []yaml.Node `yaml:"checks"`
		} `yaml:"required_status_checks"`
	} `yaml:"repos"`
}

// overlappingRepos returns the repos listed in pending_nself_ci that also have
// a non-empty required_status_checks.checks block (already moved to live).
func overlappingRepos(doc []byte) ([]string, error) {
	var p branchProtectionPolicy
	if err := yaml.Unmarshal(doc, &p); err != nil {
		return nil, err
	}
	var out []string
	for repo := range p.Pending {
		if r, ok := p.Repos[repo]; ok && len(r.Required.Checks) > 0 {
			out = append(out, repo)
		}
	}
	sort.Strings(out)
	return out, nil
}

func TestBranchProtectionNoRepoInBothBlocks(t *testing.T) {
	path := filepath.Join(repoRoot(t), "scripts", ".policy", "branch-protection.yaml")
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	got, err := overlappingRepos(doc)
	if err != nil {
		t.Fatalf("parse policy: %v", err)
	}
	if len(got) > 0 {
		t.Fatalf("repos in both pending_nself_ci and required_status_checks.checks: %v", got)
	}
}

func TestOverlappingReposDetectsDuplicate(t *testing.T) {
	doc := []byte(`
pending_nself_ci:
  org/a:
    - {context: "x", app_id: 1}
  org/b:
    - {context: "x", app_id: 1}
repos:
  org/a:
    required_status_checks:
      checks:
        - {context: "x", app_id: 1}
  org/b:
    required_status_checks:
      contexts: ["x"]
`)
	got, err := overlappingRepos(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "org/a" {
		t.Fatalf("got %v, want [org/a]", got)
	}
}

func TestOverlappingReposRejectsBadYAML(t *testing.T) {
	if _, err := overlappingRepos([]byte("pending_nself_ci: [")); err == nil {
		t.Fatal("expected parse error")
	}
}
