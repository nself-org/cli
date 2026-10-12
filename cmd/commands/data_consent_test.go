package commands

// Purpose: tests for the data-loss consent gate (D-0285). Table-driven, no
// network, no disk. They avoid t.Parallel because they flip NSELF_V15 and
// os.Args, and touch the shared cobra commands.

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// gateCmd builds a throwaway command with the flags the gate reads.
func gateCmd(buf *bytes.Buffer) *cobra.Command {
	c := &cobra.Command{Use: "prune"}
	root := &cobra.Command{Use: "nself"}
	sub := &cobra.Command{Use: "backup"}
	root.AddCommand(sub)
	sub.AddCommand(c)
	c.Flags().Bool("yes", false, "")
	c.Flags().Bool("dry-run", false, "")
	c.SetErr(buf)
	return c
}

// skipOnTerminal skips a test that would block on the y/N prompt: the gate asks
// only when both stdin and stderr are terminals.
func skipOnTerminal(t *testing.T) {
	t.Helper()
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd())) {
		t.Skip("stdin and stderr are terminals: the v1.5 gate would prompt")
	}
}

// withArgs sets os.Args for one test and restores it afterwards.
func withArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = old })
}

// TestDataLossGate covers every gate outcome for a bare, --yes and skip-flag run.
func TestDataLossGate(t *testing.T) {
	cases := []struct {
		name     string
		v15      string
		args     []string
		flags    map[string]string
		argv     []string
		wantErr  bool
		wantWarn bool
		wantRun  bool
		// needNoTTY skips the case on a terminal, where the gate would prompt.
		needNoTTY bool
	}{
		{name: "v1.4 bare warns and runs", v15: "0", wantWarn: true, wantRun: true},
		{name: "v1.4 yes is silent", v15: "0", flags: map[string]string{"yes": "true"}, wantRun: true},
		{name: "v1.4 silenced by --quiet", v15: "0", argv: []string{"nself", "backup", "prune", "--quiet"}, wantRun: true},
		{name: "v1.4 silenced by --no-deprecation-warnings", v15: "0", argv: []string{"nself", "--no-deprecation-warnings"}, wantRun: true},
		{name: "v1.5 bare non-tty refuses", v15: "1", wantErr: true, needNoTTY: true},
		{name: "v1.5 yes runs", v15: "1", flags: map[string]string{"yes": "true"}, wantRun: true},
		{name: "v1.5 skip flag runs", v15: "1", flags: map[string]string{"dry-run": "true"}, wantRun: true},
		{name: "v1.4 skip flag has no warning", v15: "0", flags: map[string]string{"dry-run": "true"}, wantRun: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needNoTTY {
				skipOnTerminal(t)
			}
			t.Setenv("NSELF_V15", tc.v15)
			if tc.argv == nil {
				tc.argv = []string{"nself", "backup", "prune"}
			}
			withArgs(t, tc.argv...)
			var buf bytes.Buffer
			c := gateCmd(&buf)
			for k, v := range tc.flags {
				if err := c.Flags().Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			ran := false
			run := withDataLossConsent(func(*cobra.Command, []string) error { ran = true; return nil }, "deletes old backups", "dry-run")
			err := run(c, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if ran != tc.wantRun {
				t.Fatalf("ran = %v, want %v", ran, tc.wantRun)
			}
			if tc.wantErr {
				var ce *errs.CLIError
				if !errors.As(err, &ce) || ce.Code != "E403" || !strings.Contains(ce.Fix, "--yes") {
					t.Fatalf("want E403 with a --yes fix, got %v", err)
				}
			}
			got := buf.String()
			if tc.wantWarn {
				want := "warning: nself backup prune without --yes is deprecated (deletes old backups); from v1.5 it needs --yes or a terminal confirmation. Pass --yes now.\n"
				if got != want {
					t.Fatalf("warning = %q, want %q", got, want)
				}
			} else if got != "" {
				t.Fatalf("unexpected stderr output %q", got)
			}
		})
	}
}

// TestDataLossGateCommands checks, for each of the five commands, that it has
// the yes flag and that in v1.5 a bare run is refused with E403 before the
// original RunE (the side effect) is reached.
func TestDataLossGateCommands(t *testing.T) {
	cmds := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"backup pitr restore", pitrRestoreCmd},
		{"db pitr restore", dbPITRRestoreCmd},
		{"db migrate down", dbMigrateDownCmd},
		{"update project rollback", migrateRollbackCmd},
		{"backup prune", backupPruneCmd},
	}
	for _, tc := range cmds {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cmd.Flags().Lookup("yes") == nil {
				t.Fatalf("%s has no --yes flag", tc.name)
			}
			// Other tests may have left these flags set on the shared command.
			for _, f := range []string{"yes", "dry-run", "list"} {
				if fl := tc.cmd.Flags().Lookup(f); fl != nil {
					old, changed := fl.Value.String(), fl.Changed
					_ = tc.cmd.Flags().Set(f, "false")
					t.Cleanup(func() {
						_ = fl.Value.Set(old)
						fl.Changed = changed
					})
				}
			}
			skipOnTerminal(t)
			t.Setenv("NSELF_V15", "1")
			var buf bytes.Buffer
			tc.cmd.SetErr(&buf)
			t.Cleanup(func() { tc.cmd.SetErr(nil) })
			err := tc.cmd.RunE(tc.cmd, nil)
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E403" {
				t.Fatalf("v1.5 bare run: want E403, got %v", err)
			}
		})
	}
}

// TestDataLossGateSkipFlagsExist guards the skip flags the gate relies on.
func TestDataLossGateSkipFlagsExist(t *testing.T) {
	if backupPruneCmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("backup prune lost its --dry-run flag")
	}
	if migrateRollbackCmd.Flags().Lookup("list") == nil {
		t.Fatal("rollback lost its --list flag")
	}
}
