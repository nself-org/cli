package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseProcessCompletionChecks guards the operator checks that prevent a
// green release chain from leaving an unusable SDK or the wrong installer Latest.
func TestReleaseProcessCompletionChecks(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/wiki/Release-Process.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	checks := map[string][]string{
		"admin before tag":         {"S2 `admin-lockstep`", "S4 `tag`"},
		"formula PR and rerun":     {"S6 `homebrew-formula`", "verify-homebrew-lockstep"},
		"SDK module resolution":    {"sdk/go/scripts/sdk-tag.sh", "go list -m", "github.com/nself-org/cli/sdk/go/v2@"},
		"Latest after SDK publish": {"releases/latest", "--latest=false", "install.sh"},
	}
	for name, required := range checks {
		t.Run(name, func(t *testing.T) {
			for _, text := range required {
				if !strings.Contains(doc, text) {
					t.Errorf("release runbook omits %q", text)
				}
			}
		})
	}
}
