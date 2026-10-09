package commands

// Purpose: `nself access revoke` handler — delegates to access.Revoke and
// surfaces the ErrLastKey lockout guard as an actionable error rather than a
// raw wrapped message.
// Inputs: --host, --identity, --user, --force, --dry-run.
// Outputs: printed confirmation (or dry-run diff) plus the revoked key's
// fingerprint; a non-nil error when the user is unknown or the lockout guard
// trips.

import (
	"errors"
	"fmt"

	"github.com/nself-org/cli/internal/access"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runAccessRevoke(cmd *cobra.Command, args []string) error {
	user, _ := cmd.Flags().GetString("user")
	if user == "" {
		return fmt.Errorf("--user is required")
	}
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	targets, selected, err := resolveAccessTargets(cmd)
	if err != nil {
		return err
	}
	if err := confirmAccessTargets(cmd, targets, dryRun); err != nil {
		return err
	}
	for _, target := range targets {
		if err := revokeAccessTarget(cmd, target, selected, user, force, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func revokeAccessTarget(cmd *cobra.Command, target accessTarget, selected bool, user string, force, dryRun bool) error {
	t := target.Transport
	if !selected {
		ui.CommandHeader("nself access revoke", t.Describe())
	}

	result, err := access.Revoke(cmd.Context(), t, access.RevokeRequest{
		User: user, Force: force, DryRun: dryRun,
	})
	if err != nil {
		if errors.Is(err, access.ErrLastKey) {
			ui.Error(err.Error())
			return fmt.Errorf("revoke access for %s on %s: last key on host", user, t.Describe())
		}
		return fmt.Errorf("revoke access for %s on %s: %w", user, t.Describe(), err)
	}

	if dryRun {
		if selected {
			fmt.Printf("%s/%s host=%s status=dry-run diff=%q\n", target.Env, target.Server, target.Host, result.Diff)
			return nil
		}
		ui.Info("Dry run: no changes made. Resulting authorized_keys diff:")
		fmt.Print(result.Diff)
		return nil
	}

	if result.BackupPath != "" {
		if selected {
			fmt.Printf("%s/%s host=%s status=revoked fingerprint=%s backup=%s\n", target.Env, target.Server, target.Host, result.Fingerprint, result.BackupPath)
			return nil
		}
		ui.Info("Backed up authorized_keys to " + result.BackupPath)
	}
	if selected {
		fmt.Printf("%s/%s host=%s status=revoked fingerprint=%s\n", target.Env, target.Server, target.Host, result.Fingerprint)
		return nil
	}
	ui.Success(fmt.Sprintf("Revoked %s's access to %s", user, t.Describe()))
	ui.Info("Fingerprint: " + result.Fingerprint)
	return nil
}
