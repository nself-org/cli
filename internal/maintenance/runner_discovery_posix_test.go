//go:build darwin || linux

package maintenance

import (
	"path/filepath"
	"testing"
)

// TestDefaultRunnerRootGlobs_CoversGithubRunnerNaming pins the naming
// conventions the fallback must match. nSelf staging installs the web repo's
// runner at /home/runner/github-runner, which matches no actions-runner*
// pattern; it held 5.1G that cleanup could never see, and the box filled to
// 100% on 2026-09-11 with that space unreclaimable. A runner this never
// discovers is a runner it can never clean.
func TestDefaultRunnerRootGlobs_CoversGithubRunnerNaming(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"opt actions-runner", "/opt/actions-runner/_work"},
		{"opt numbered actions-runner", "/opt/actions-runner-3/_work"},
		{"home actions-runner", "/home/runner/actions-runner/_work"},
		{"home github-runner (the one that was missed)", "/home/runner/github-runner/_work"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Dir(tc.path)
			var matched bool
			for _, g := range defaultRunnerRootGlobs {
				ok, err := filepath.Match(g, root)
				if err != nil {
					t.Fatalf("bad glob %q: %v", g, err)
				}
				if ok {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("no glob in defaultRunnerRootGlobs matches %q; that runner's disk can never be reclaimed", root)
			}
		})
	}
}
