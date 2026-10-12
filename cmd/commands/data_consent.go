package commands

// Purpose: consent gate for the data-loss commands that had no consent flag (D-0285).
//
// Five commands destroy data and used to run the moment they were typed:
// backup pitr restore, db pitr restore, db migrate down, update project
// rollback (legacy: migrate rollback) and backup prune. They now take --yes.
// The gate lives here, in one place, and wraps each command's RunE at init
// time, so the command files stay as they were.
//
// Behaviour (ADR 0021 compat switch):
//   - --yes set: runs, silent.
//   - v1.4 (default), no --yes: runs, with one stderr deprecation line.
//   - v1.5 (NSELF_V15=1), no --yes: asks on a terminal, otherwise refuses
//     with E403 before any side effect.
//   - a read-only or preview flag (--dry-run, --list) skips the gate entirely.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// consentFlagUsage is the help text of the --yes flag on the gated commands.
const consentFlagUsage = "Confirm the data loss without a prompt (required from v1.5 unless you answer a terminal prompt)"

// dataLossGate names one gated command, what it destroys and the flags that
// make a run harmless (a preview or a listing).
type dataLossGate struct {
	cmd   *cobra.Command
	what  string
	skips []string
}

// init adds --yes to the five data-loss commands and wraps their RunE with the
// consent gate. It must not run twice for one command (a second --yes flag
// would panic), so each command is listed exactly once.
func init() {
	gates := []dataLossGate{
		{cmd: pitrRestoreCmd, what: "overwrites the live database with a point-in-time copy"},
		{cmd: dbPITRRestoreCmd, what: "overwrites the live database with a point-in-time copy"},
		{cmd: dbMigrateDownCmd, what: "reverts the last migration and drops what it created"},
		{cmd: migrateRollbackCmd, what: "restores an older project layout over the current one", skips: []string{"list"}},
		{cmd: backupPruneCmd, what: "deletes old backups", skips: []string{"dry-run"}},
	}
	for _, g := range gates {
		g.cmd.Flags().Bool("yes", false, consentFlagUsage)
		g.cmd.RunE = withDataLossConsent(g.cmd.RunE, g.what, g.skips...)
	}
}

// withDataLossConsent returns a RunE that runs the consent gate first and the
// original RunE only when the gate passes. what says, in a short phrase, what
// the command destroys. When any of skipFlags is set on the command line the
// gate is bypassed with no warning (those flags never change data).
func withDataLossConsent(next func(*cobra.Command, []string) error, what string, skipFlags ...string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		for _, f := range skipFlags {
			if set, _ := cmd.Flags().GetBool(f); set {
				return next(cmd, args)
			}
		}
		if err := requireDataLossConsent(cmd, what); err != nil {
			return err
		}
		return next(cmd, args)
	}
}

// requireDataLossConsent is the gate. With --yes it returns nil and prints
// nothing. Without it, v1.4 prints one deprecation line (unless warnings are
// silenced) and returns nil; v1.5 asks on a terminal and otherwise returns an
// E403 error that nothing has changed behind.
func requireDataLossConsent(cmd *cobra.Command, what string) error {
	if yes, _ := cmd.Flags().GetBool("yes"); yes {
		return nil
	}
	// compat.V15(D-0285): bare data-loss command runs with a deprecation warning -> refused (E403) unless --yes or a terminal yes
	if compat.V15() {
		prompt := fmt.Sprintf("%s %s. This cannot be undone. Continue?", cmd.CommandPath(), what)
		if c := interactiveConfirm(cmd); c != nil && c(prompt) {
			return nil
		}
		return errs.New("E403", fmt.Sprintf("%s %s and was not confirmed", cmd.CommandPath(), what)).
			WithFix("pass --yes to confirm, or run it on a terminal and answer y")
	}
	if !warningsSilenced(os.Args) {
		path := strings.TrimPrefix(cmd.CommandPath(), "nself ")
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: nself %s without --yes is deprecated (%s); from v1.5 it needs --yes or a terminal confirmation. Pass --yes now.\n", path, what)
	}
	return nil
}
