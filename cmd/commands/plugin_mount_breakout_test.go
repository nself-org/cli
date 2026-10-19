package commands

// Purpose: the break-out owner of a command decides whether an installed
// plugin may mount it (E405). A plugin that owns the whole command must not
// lose it to the plugin that owns one of its subcommands (P7-CI-47: `ci` and
// `ci eval` in the real table).
// Inputs:  the generated canonTable and hand-made mount specs.
// Constraints: no disk, no network.

import (
	"testing"

	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

func TestBreakoutOwnerPrefersTheCommandRow(t *testing.T) {
	old := canonTable
	t.Cleanup(func() { canonTable = old })
	canonTable.Breakouts = []canonRowT{
		{From: []string{"ci", "eval"}, To: []string{"eval"}, Plugin: "nself-eval-gate"},
		{From: []string{"ci"}, To: []string{"ci"}, Plugin: "ci"},
		{From: []string{"plugin", "dev"}, To: []string{"plugin-dev", "dev"}, Plugin: "plugin-dev"},
	}
	cases := map[string]string{
		"ci":     "ci",         // the command row, though the subcommand row is listed first
		"plugin": "plugin-dev", // only subcommand rows: the first-word match still holds
		"eval":   "",           // not a From word
		"nope":   "",
	}
	for name, want := range cases {
		if got := breakoutOwner(name); got != want {
			t.Errorf("breakoutOwner(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPlanMountMountsTheCIPluginBesideEvalGate(t *testing.T) {
	t.Setenv("NSELF_V15", "1") // v1.4 keeps a same-owner break-out in core and does not mount it
	old := canonTable
	t.Cleanup(func() { canonTable = old })
	canonTable.Breakouts = []canonRowT{
		{From: []string{"ci", "eval"}, To: []string{"eval"}, Plugin: "nself-eval-gate"},
		{From: []string{"ci"}, To: []string{"ci"}, Plugin: "ci"},
	}
	root := &cobra.Command{Use: "nself"}
	specs := []mount.Spec{{Slug: "ci", Command: "ci"}, {Slug: "nself-eval-gate", Command: "eval"}}
	got, problems := planMount(root, specs)
	if len(problems) != 0 || len(got) != 2 {
		t.Fatalf("mountable=%+v problems=%+v", got, problems)
	}
}
