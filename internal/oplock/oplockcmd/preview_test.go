//go:build !windows

package oplockcmd

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/spf13/cobra"
)

func TestPreviewUnderHeldLock(t *testing.T) {
	for _, row := range []struct{ path, flag string }{{"build", "plan"}, {"config set", "preview"}, {"update", "plan"}} {
		t.Run(row.path, func(t *testing.T) {
			compattest.Set(t, true)
			dir := projectDir(t)
			t.Setenv(oplock.EnvToken, "")
			held, err := oplock.Acquire(context.Background(), dir, oplock.Opts{Command: "holder"})
			if err != nil {
				t.Fatal(err)
			}
			defer held.Release()
			ran := false
			_, cmd := newTree(write(), func(*cobra.Command) error { ran = true; return nil })
			cmd.Flags().Bool(row.flag, false, "preview")
			if err := cmd.Flags().Set(row.flag, "true"); err != nil {
				t.Fatal(err)
			}
			// The registry's path identifies the real command even in this fixture.
			entry := write()
			entry.Path = "nself " + row.path
			cmd.RunE = Wrap(func(*cobra.Command) (*cmdregistry.Command, error) { return entry, nil }, func(*cobra.Command, []string) error { ran = true; return nil })
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatal(err)
			}
			if !ran {
				t.Fatal("preview body did not run")
			}
		})
	}
}
