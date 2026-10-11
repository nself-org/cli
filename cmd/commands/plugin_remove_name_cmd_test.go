package commands

// Purpose: P7-PLUG-20 fix, command level: `remove` and `plugin remove` refuse a
// path-shaped plugin name before the dry-run plan, the bundle ledger lookup and
// the removal, in v1.4 and v1.5 mode, with and without --force.
// Inputs: chatFixture (plugins bots and livekit, bundle chat recorded) plus a
// victim directory next to the plugin directory.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestRemoveRefusesPathShapedNamesCmd(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := chatFixture(t)
		victim := filepath.Join(filepath.Dir(p.plugins), "victim")
		if err := os.MkdirAll(victim, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(victim, "keep.txt"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		type runner struct {
			label string
			run   func(args []string) error
			cmd   *cobra.Command
			flags []string // flag sets to try, "" = none
		}
		runners := []runner{
			{"plugin remove", func(a []string) error { return runPluginRemove(pluginRemoveCmd, a) }, pluginRemoveCmd, []string{"", "force"}},
			{"remove", func(a []string) error { return runRemove(removeCmd, a) }, removeCmd, []string{"", "force", "dry-run"}},
		}
		for _, r := range runners {
			for _, name := range []string{"./bots", "../victim", "bots/", "../plugins/bots", "../../victim"} {
				for _, flag := range r.flags {
					if flag != "" {
						setFlags(t, r.cmd, flag)
					}
					err := r.run([]string{name})
					if d := errs.Describe(err); d == nil || d.Code != "E401" {
						t.Errorf("%s %q (%s): err = %v, want E401", r.label, name, flag, err)
					}
					if flag != "" {
						resetFlag(r.cmd, flag)
					}
				}
			}
		}
		if _, err := os.Stat(filepath.Join(victim, "keep.txt")); err != nil {
			t.Errorf("the victim directory was damaged: %v", err)
		}
		if _, err := os.Stat(filepath.Join(p.plugins, "bots", "plugin.json")); err != nil {
			t.Errorf("bots was removed: %v", err)
		}
		l, _, err := bundle.LedgerStore(p.plugins).Load()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := l.Plugins["bots"]; !ok {
			t.Error("the ledger lost bots")
		}
	})
}
