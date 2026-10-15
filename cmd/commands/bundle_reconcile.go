package commands

// Purpose: regenerate extension changes through the shared change-plan engine.
// Inputs: the command flags and a plugin or bundle trigger; output: the applied plan.
// Constraints: only cmd/commands may call reconcile. A container swap (an
// orphan the new compose no longer defines) is finished in the same apply:
// orphans go, then the services that have no container yet start (D-0432).

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
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
	// Containers and RemoveOrphans put the orphan-remove effects in the plan
	// the operator confirms; the swap below runs only after the apply passed.
	req := reconcile.Request{ProjectDir: projectDir, Command: command, Trigger: trigger,
		Containers: true, RemoveOrphans: true, Runtime: extensionRuntime, Stderr: cmd.ErrOrStderr()}
	opt := reconcile.ApplyOptions{Yes: yes, Force: force, Interactive: interactiveConfirm(cmd)}
	plan, err := reconcile.Apply(context.Background(), req, opt)
	if err != nil {
		if detail := errs.Describe(err); detail != nil && detail.Code == "E403" {
			return errs.New("E403", "extension files changed but regeneration needs confirmation").
				WithWhy(err.Error()).WithFix("review the pending change with 'nself build --plan', then rerun with --yes (and --force for hand-edited files)")
		}
		return fmt.Errorf("regenerating %s:%s: %w", trigger.Kind, trigger.Subject, err)
	}
	if err := swapContainers(context.Background(), cmd, projectDir, plan); err != nil {
		return err
	}
	if plan.RequiresConfirmation || len(reconcile.HandEditedPaths(*plan)) > 0 {
		return nil // Apply already printed the plan before its confirmation gate.
	}
	return reconcile.RenderHuman(cmd.ErrOrStderr(), *plan)
}

// Seams for the container half of an extension reconcile; tests replace them.
var (
	extensionRuntime       reconcile.ContainerRuntime // nil: the docker CLI
	extensionRemoveOrphans = docker.RemoveOrphans
	extensionStartServices = func(ctx context.Context, projectDir string, services []string) error {
		return docker.NewCompose().ComposeUp(ctx, projectDir, services...)
	}
)

// swapContainers finishes a proxy swap after the files are written: it removes
// the orphans the applied plan listed, then starts the services the plan says
// have no container, so the new proxy can bind the ports the old one held.
// Without an orphan nothing runs: a plain add leaves new services to
// `nself start`, as the plan says.
func swapContainers(ctx context.Context, cmd *cobra.Command, projectDir string, plan *reconcile.Plan) error {
	var orphans []docker.OrphanContainer
	for _, e := range plan.Effects {
		if e.Kind == reconcile.EffectOrphanRemove {
			// Target is the container name; docker rm -f takes a name as well as an id.
			orphans = append(orphans, docker.OrphanContainer{ID: e.Target, Name: e.Target})
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	if errList := extensionRemoveOrphans(ctx, orphans); len(errList) > 0 {
		return fmt.Errorf("generated files were written but orphaned containers were not removed (run 'nself build --remove-orphans --yes', then 'nself start'): %w", errors.Join(errList...))
	}
	var create []string
	for i := range plan.Containers.Items {
		if it := &plan.Containers.Items[i]; it.Action == reconcile.ContainerCreate {
			create = append(create, it.Service)
		}
	}
	if len(create) == 0 {
		return nil
	}
	if err := extensionStartServices(ctx, projectDir, create); err != nil {
		return fmt.Errorf("orphaned containers were removed but the new services did not start (run 'nself start'): %w", err)
	}
	for i := range plan.Containers.Items {
		if plan.Containers.Items[i].Action == reconcile.ContainerCreate {
			plan.Containers.Items[i].AppliedBy = reconcile.AppliedThisCommand
		}
	}
	return nil
}
