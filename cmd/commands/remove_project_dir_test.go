package commands

// Purpose: D-0504 regression for bundle routing with a project plugin directory.
// Inputs: a bundle ledger in the directory named only by the project .env.
// Outputs: the bundle delete-data confirmation guard, with no plugin removed.
// Constraints: local fixtures only; exercise both compatibility modes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func TestRemoveBundleProjectPluginDirCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		t.Setenv("NSELF_PLUGIN_DIR", "")
		envPath := filepath.Join(p.project, ".env")
		body, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, []byte("\nNSELF_PLUGIN_DIR="+p.plugins+"\n")...)
		if err := os.WriteFile(envPath, body, 0o600); err != nil {
			t.Fatal(err)
		}
		setFlags(t, removeCmd, "delete-data")
		err = runRemove(removeCmd, []string{"chat"})
		if detail := errs.Describe(err); detail == nil || detail.Code != "E403" || !strings.Contains(err.Error(), "bundle remove") {
			t.Fatalf("remove: %v, want bundle confirmation E403", err)
		}
		for _, name := range []string{"bots", "livekit"} {
			if _, err := os.Stat(filepath.Join(p.plugins, name)); err != nil {
				t.Errorf("refused removal changed %s: %v", name, err)
			}
		}
	})
}
