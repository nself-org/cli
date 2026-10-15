//go:build !windows

package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// D-0755: real command flags must permit plans under a foreign write holder,
// without creating a lock file themselves. Mutation controls still fail E460.
func TestOpLockPlanPreview(t *testing.T) {
	cases := []struct {
		args []string
		read bool
	}{
		{[]string{"build", "--plan"}, true},
		{[]string{"build", "--plan", "--json"}, true},
		{[]string{"config", "set", "BASE_DOMAIN", "z.test", "--preview"}, true},
		{[]string{"update", "--plan"}, true},
		{[]string{"build"}, false},
		{[]string{"build", "--plan=false"}, false},
		{[]string{"config", "set", "BASE_DOMAIN", "z.test"}, false},
		{[]string{"config", "set", "BASE_DOMAIN", "z.test", "--preview=false"}, false},
		{[]string{"update"}, false},
		{[]string{"update", "--plan=false"}, false},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			compattest.Set(t, true)
			dir := project(t)
			cmd := planPreviewCommand(t, tc.args)
			ran := false
			run := locked(func(c *cobra.Command, _ []string) error {
				ran = true
				if tc.read && oplock.FromContext(c.Context()) != nil {
					t.Error("plan/preview body has an operation lock")
				}
				return nil
			})
			if tc.read {
				if err := run(cmd, nil); err != nil {
					t.Fatal(err)
				}
				if lockFileExists(dir) {
					t.Error("plan/preview created an operation lock file")
				}
			}
			startBuildHolder(t)
			ran = false
			cmd.SetContext(context.Background())
			err := run(cmd, nil)
			if tc.read {
				if err != nil || !ran {
					t.Fatalf("plan/preview under holder: err=%v ran=%v", err, ran)
				}
			} else if d := errs.Describe(err); d == nil || d.Code != "E460" || ran {
				t.Fatalf("mutation under holder must fail E460 before body: err=%v ran=%v", err, ran)
			}
		})
	}
}

// planPreviewCommand parses the real flags and restores their state after use;
// only the lock decorator runs, so these tests never invoke Docker or a network.
func planPreviewCommand(t *testing.T, args []string) *cobra.Command {
	t.Helper()
	reattachRealTree()
	cmd, flags, err := RootCmd.Find(args)
	if err != nil || cmd == nil || cmd == RootCmd {
		t.Fatalf("find %v: %v", args, err)
	}
	// Merge inherited flags before saving state.
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		value, changed := f.Value.String(), f.Changed
		t.Cleanup(func() { _ = f.Value.Set(value); f.Changed = changed })
	})
	if err := cmd.ParseFlags(flags); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.SetContext(context.Background()) })
	return cmd
}
