package commands

// Purpose: D-0745 keeps remove's public help aligned with its data policy.
// Inputs: Cobra's rendered help in both compatibility modes.
// Outputs: assertions for the preservation default and confirmation rules.
// Constraints: help rendering must not execute a removal.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestRemoveHelpDataPolicy(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		var out bytes.Buffer
		previous := removeCmd.OutOrStdout()
		removeCmd.SetOut(&out)
		t.Cleanup(func() { removeCmd.SetOut(previous) })
		if err := removeCmd.Help(); err != nil {
			t.Fatal(err)
		}
		help := strings.Join(strings.Fields(out.String()), " ")
		for _, want := range []string{
			"--delete-data",
			"v1.5 keeps data by default",
			"plugin --delete-data asks for confirmation (or pass --yes)",
			"v1.4 plugin remove drops data unless --keep-data and never asks",
			"bundle --delete-data always asks for confirmation (or pass --yes)",
		} {
			if !strings.Contains(help, want) {
				t.Errorf("remove help lacks %q:\n%s", want, out.String())
			}
		}
	})
}
