package commands

// Purpose: the "nself bundle install" and "nself bundle remove" subcommands
// and their RunE. Inputs are the cobra command/args; outputs are installed/
// removed plugins or an error.
// Constraints: split out of bundle.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/reconcile"

	"github.com/spf13/cobra"
)

var bundleInstallCmd = &cobra.Command{
	Use:   "install <bundle>",
	Short: "Install every plugin in a bundle (license-gated, atomic rollback on failure)",
	Long: `Install all plugins in the named bundle in a single transaction.

Licence checks run BEFORE any filesystem change: a paid bundle needs the
bundle entitlement and every plugin's licence; a free bundle skips the ping_api
bundle entitlement call (plugin licences are still checked). On any per-plugin
install failure, every plugin installed in this invocation is rolled back so
the system is left in a clean state.

After the plugins are installed the command records the bundle in the install
ledger, then reconciles the project. With NSELF_V15=1 it also starts the stack
and reports the app-ready steps: readiness (plugin migrations applied), tracking
(tables tracked in Hasura) and health, and exits 2 (E135) at the first failing
step. Without it, those steps print as "not verified" and the exit stays 0.

Examples:
  nself bundle install nsentry
  nself bundle install nclaw --channel beta
  nself bundle install nchat --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleInstall,
}

var bundleRemoveCmd = &cobra.Command{
	Use:   "remove <bundle>",
	Short: "Remove a bundle and the plugins only it needs",
	Long: `Remove a bundle and the plugins only it needs.

A plugin is removed only when no other installed bundle references it and you
did not install it on your own; the rest stay, and the plan says why. The
install ledger (<plugin dir>/.nself-state/bundles.json) holds those references.
Plugin data (schema/tables) is kept by default from v1.5; pass --delete-data to
drop it (asks for confirmation on a terminal, or pass --yes; without either it
exits 4 with E403). In v1.4 mode a paid bundle still drops data by default and
prints a warning. --dry-run prints the plan and changes nothing. If a removal
fails the command stops; run it again to continue.

Examples:
  nself bundle remove nsentry
  nself bundle remove nclaw --delete-data --yes
  nself bundle remove nchat --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleRemove,
}

// bundleInstall is a command seam so the regeneration branch can be exercised
// with a local fixture after the bundle installer reports changed membership.
var bundleInstall = bundle.Install

func runBundleInstall(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	strict, _ := cmd.Flags().GetBool("strict")
	channelStr, _ := cmd.Flags().GetString("channel")

	opts := bundle.InstallOpts{
		DryRun:  dryRun,
		Force:   force,
		Strict:  strict,
		Channel: bundle.Channel(channelStr),
	}
	if !dryRun {
		// P7-PLUG-19: Install runs reconcile, start, readiness, tracking and health.
		opts.Hooks = newBundleAppReadyHooks(cmd, args[0])
	}
	result, err := bundleInstall(plugin.WithCallerReconciles(context.Background()), args[0], opts)
	recordBundleInstall(cmd, result) // P7-SURF-13: the envelope reports the ledger-backed result
	// Reconcile exactly once: Install ran it through the hook unless it never got
	// that far (a member failed and was rolled back, or no hooks ran).
	if result != nil && result.Changed && !result.Reconciled {
		// compat.V15(P7-LIVE-06): bundle subprocess build -> cmd-layer reconcile in both modes
		return errors.Join(err, bundleReconcile(cmd, reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: args[0]}))
	}
	return err
}

func runBundleRemove(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	keepData, _ := cmd.Flags().GetBool("keep-data")
	deleteData, _ := cmd.Flags().GetBool("delete-data")
	yes, _ := cmd.Flags().GetBool("yes")

	// compat.V15(P7-PLUG-20): --keep-data keeps plugin data -> deprecated no-op (data is kept by default)
	if compat.V15() && keepData {
		fmt.Fprintln(os.Stderr, "warning: --keep-data is deprecated and has no effect from v1.5 (data is kept by default); it is removed in v1.6.0. Use --delete-data to drop data.")
	}
	opts := bundle.RemoveOpts{
		DryRun:     dryRun,
		KeepData:   keepData,
		DeleteData: deleteData,
		Yes:        yes,
		Confirm:    interactiveConfirm(cmd),
	}
	result, err := bundle.Remove(context.Background(), args[0], opts)
	recordBundleRemove(cmd, result) // P7-SURF-13
	if result != nil && result.Changed {
		// compat.V15(P7-LIVE-06): bundle subprocess build -> cmd-layer reconcile in both modes
		return errors.Join(err, reconcileAfterExtension(cmd, reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: args[0]}))
	}
	return err
}

// ── Registration ─────────────────────────────────────────────────────
