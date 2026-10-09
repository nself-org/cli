package commands

// Purpose: the "nself bundle install" and "nself bundle remove" subcommands
// and their RunE. Inputs are the cobra command/args; outputs are installed/
// removed plugins or an error.
// Constraints: split out of bundle.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"errors"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/reconcile"

	"github.com/spf13/cobra"
)

var bundleInstallCmd = &cobra.Command{
	Use:   "install <bundle>",
	Short: "Install every plugin in a bundle (license-gated, atomic rollback on failure)",
	Long: `Install all plugins in the named bundle in a single transaction.

License validation runs for every plugin BEFORE any filesystem change.
On any per-plugin install failure, every plugin installed in this invocation
is rolled back so the system is left in a clean state.

Examples:
  nself bundle install nsentry
  nself bundle install nclaw --channel beta
  nself bundle install nchat --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleInstall,
}

var bundleRemoveCmd = &cobra.Command{
	Use:   "remove <bundle>",
	Short: "Remove every plugin in a bundle",
	Long: `Remove all plugins in the named bundle. Plugin data is dropped by default;
pass --keep-data to preserve schema/tables for later reinstall.

Examples:
  nself bundle remove nsentry
  nself bundle remove nclaw --keep-data
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
	result, err := bundleInstall(context.Background(), args[0], opts)
	if result != nil && result.Changed {
		// compat.V15(P7-LIVE-06): bundle subprocess build -> cmd-layer reconcile in both modes
		return errors.Join(err, reconcileAfterExtension(cmd, reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: args[0]}))
	}
	return err
}

func runBundleRemove(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	keepData, _ := cmd.Flags().GetBool("keep-data")

	opts := bundle.RemoveOpts{
		DryRun:   dryRun,
		KeepData: keepData,
	}
	result, err := bundle.Remove(context.Background(), args[0], opts)
	if result != nil && result.Changed {
		// compat.V15(P7-LIVE-06): bundle subprocess build -> cmd-layer reconcile in both modes
		return errors.Join(err, reconcileAfterExtension(cmd, reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: args[0]}))
	}
	return err
}

// ── Registration ─────────────────────────────────────────────────────
