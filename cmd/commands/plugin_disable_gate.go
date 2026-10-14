package commands

// Purpose: confirm a plugin disable marker before changing installed state.
// Inputs: command flags and plugin name. Outputs: a read-only plan or refusal.
// Constraints: v1.4 and outside-project disables retain their existing behavior;
// this preflight does not regenerate or predict the post-disable compose tree.

import (
	"context"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func init() {
	pluginDisableCmd.Flags().Bool("yes", false, "Confirm disabling a plugin on a prod-class environment")
	pluginDisableCmd.Flags().Bool("force", false, "Confirm pending hand-edited generated file overwrites")
}

func confirmPluginDisable(cmd *cobra.Command, name string) error {
	if !compat.V15() {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	projectDir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return nil // Plugin state can be managed outside a project.
	}
	state, err := reconcile.LoadGeneratedState(projectDir)
	if err != nil {
		return err
	}
	req := reconcile.Request{
		ProjectDir: projectDir,
		Command:    reconcile.CmdPluginRemove,
		Trigger:    reconcile.Trigger{Kind: reconcile.TriggerPlugin, Subject: name},
		HandEdited: state.HandEditedFn(projectDir),
		Extra: []reconcile.Effect{{Kind: reconcile.EffectPluginFragment,
			Target: name + "/.disabled", Detail: "disable marker; regeneration requires nself build"}},
		Stderr: cmd.ErrOrStderr(),
	}
	plan, err := reconcile.Compute(context.Background(), req)
	if err != nil {
		return fmt.Errorf("planning plugin disable: %w", err)
	}
	if err := reconcile.RenderHuman(cmd.ErrOrStderr(), *plan); err != nil {
		return err
	}
	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	return reconcile.Confirm(*plan, reconcile.ApplyOptions{
		Yes: yes, Force: force, Interactive: interactiveConfirm(cmd),
	}, true, cmd.ErrOrStderr())
}
