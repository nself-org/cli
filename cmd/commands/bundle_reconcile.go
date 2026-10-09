package commands

// Purpose: regenerate extension changes through the shared change-plan engine.
// Inputs: the command flags and a plugin or bundle trigger; output: the applied plan.
// Constraints: only cmd/commands may call reconcile; no service is started here.

import (
	"context"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func init() {
	// These commands also run through the canonical add/remove shims, whose
	// corresponding flags already exist. Register only the missing flags here.
	bundleInstallCmd.Flags().Bool("yes", false, "Confirm regeneration on a prod-class environment")
	bundleRemoveCmd.Flags().Bool("yes", false, "Confirm regeneration on a prod-class environment")
	bundleRemoveCmd.Flags().Bool("force", false, "Allow overwriting hand-edited generated files")
	pluginRemoveCmd.Flags().Bool("yes", false, "Confirm regeneration on a prod-class environment")
}

// reconcileAfterExtension applies generated state after extension membership
// has changed. The caller has already installed or removed the plugin files.
func reconcileAfterExtension(cmd *cobra.Command, trigger reconcile.Trigger) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	projectDir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		// Installing a plugin outside a project is supported: there is no
		// generated project state to update until the user enters one.
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Extension files changed outside a project. Run 'nself build --plan' in a project to review regeneration.")
		return nil
	}
	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	command := reconcile.CmdPluginInstall
	if trigger.Kind == reconcile.TriggerBundle {
		command = reconcile.CmdBundleInstall
	}
	if cmd.Name() == "remove" {
		command = reconcile.CmdPluginRemove
		if trigger.Kind == reconcile.TriggerBundle {
			command = reconcile.CmdBundleRemove
		}
	}
	req := reconcile.Request{ProjectDir: projectDir, Command: command, Trigger: trigger,
		Stderr: cmd.ErrOrStderr()}
	opt := reconcile.ApplyOptions{Yes: yes, Force: force, Interactive: interactiveConfirm(cmd)}
	plan, err := reconcile.Apply(context.Background(), req, opt)
	if err != nil {
		if detail := errs.Describe(err); detail != nil && detail.Code == "E403" {
			return errs.New("E403", "extension files changed but regeneration needs confirmation").
				WithWhy(err.Error()).WithFix("review the pending change with 'nself build --plan', then rerun with --yes (and --force for hand-edited files)")
		}
		return fmt.Errorf("regenerating %s:%s: %w", trigger.Kind, trigger.Subject, err)
	}
	if plan.RequiresConfirmation || len(reconcile.HandEditedPaths(*plan)) > 0 {
		return nil // Apply already printed the plan before its confirmation gate.
	}
	return reconcile.RenderHuman(cmd.ErrOrStderr(), *plan)
}
