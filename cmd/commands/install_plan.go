package commands

// Purpose: preview extension regeneration using a disposable plugin tree.
// Inputs: registry plugin names and add/remove flags. Outputs: change plan.
// Constraints: no installer, data removal or confirmation step touches live state.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func runExtensionPlan(cmd *cobra.Command, names []string, add bool) error {
	if err := validateExtensionPlan(cmd, names); err != nil {
		return err
	}
	defer build.SnapshotEnv()()
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	project, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return fmt.Errorf("--plan requires a project: %w", err)
	}
	cfg, err := config.Load(project)
	if err != nil {
		return err
	}
	installed, err := filepath.Abs(resolvePluginDir())
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "nself-extension-plan-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(root) }()
	staged := filepath.Join(root, "plugins")
	if err := plugin.CopyPlanTree(installed, staged); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := stageExtensionChange(ctx, cmd, cfg, names, add, staged, root); err != nil {
		return err
	}
	return renderExtensionPlan(ctx, cmd, names, add, project, staged, installed)
}

// stageExtensionChange confines membership changes and download caches to root.
func stageExtensionChange(ctx context.Context, cmd *cobra.Command, cfg *config.Config, names []string, add bool, staged, root string) error {
	if add {
		for _, pair := range [][2]string{{"key", "NSELF_PLUGIN_LICENSE_KEY"}, {"tier", "NSELF_PLUGIN_INSTALL_TIER"}} {
			if value, _ := cmd.Flags().GetString(pair[0]); value != "" {
				if err := os.Setenv(pair[1], value); err != nil {
					return err
				}
			}
		}
		if flagOn(cmd, "skip-sbom-check") {
			if err := os.Setenv("NSELF_SKIP_SBOM_CHECK", "1"); err != nil {
				return err
			}
		}
		if os.Getenv("NSELF_LICENSE_SKIP_VERIFY") == "1" && !flagOn(cmd, "force") {
			return fmt.Errorf("NSELF_LICENSE_SKIP_VERIFY requires --force flag")
		}
		ctx = plugin.WithSignatureBypassAcknowledged(ctx, flagOn(cmd, "force"))
		if err := plugin.StageForPlan(ctx, cfg, names, staged, root, flagOn(cmd, "allow-eol")); err != nil {
			return err
		}
	} else {
		for _, name := range names {
			if err := plugin.StageRemoveForPlan(staged, name, flagOn(cmd, "force")); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderExtensionPlan computes against the real project using staged plugins.
func renderExtensionPlan(ctx context.Context, cmd *cobra.Command, names []string, add bool, project, staged, installed string) error {
	command, effect := reconcile.CmdPluginInstall, reconcile.EffectPluginInstall
	if !add {
		command, effect = reconcile.CmdPluginRemove, reconcile.EffectPluginRemove
	}
	req := reconcile.Request{ProjectDir: project, Command: command,
		Trigger:    reconcile.Trigger{Kind: reconcile.TriggerPlugin, Subject: strings.Join(names, ",")},
		Build:      build.BuildOptions{PlanPluginDir: staged, PlanPluginTarget: installed},
		Containers: true, RemoveOrphans: true, Runtime: extensionRuntime, Stderr: cmd.ErrOrStderr()}
	for _, name := range names {
		req.Extra = append(req.Extra, reconcile.Effect{Kind: effect, Target: name, Detail: "requested extension change"})
	}
	p, err := reconcile.Compute(ctx, req)
	if err != nil {
		return err
	}
	if flagOn(cmd, "json") {
		outcomeOf(cmd).plan = p
		data := removeFacts(cmd, names)
		if add {
			data = addFacts(cmd, names)
		}
		return output.EmitData(pilotWriter(), invokedKey(cmd), data)
	}
	return reconcile.RenderHuman(cmd.OutOrStdout(), *p)
}

func validateExtensionPlan(cmd *cobra.Command, names []string) error {
	for _, flag := range []string{"version", "checksum"} {
		if value, _ := cmd.Flags().GetString(flag); value != "" {
			return fmt.Errorf("--plan does not support --%s", flag)
		}
	}
	if value, _ := cmd.Flags().GetString("channel"); value != "" && value != "stable" {
		return fmt.Errorf("--plan does not support non-stable --channel")
	}
	for _, flag := range []string{"preview", "dry-run", "show-graph", "delete-data", "with-optional", "strict"} {
		if flagOn(cmd, flag) {
			return fmt.Errorf("--plan cannot be combined with --%s", flag)
		}
	}
	for _, name := range names {
		if err := plugin.ValidateName(name); err != nil {
			return fmt.Errorf("--plan supports registry plugin names only: %w", err)
		}
		if resolvesToBundle(name) {
			return fmt.Errorf("--plan does not support bundles; plan individual registry plugins")
		}
		for _, family := range builtinFamilies {
			if family.slug == name {
				return fmt.Errorf("--plan does not support builtin toggles")
			}
		}
	}
	return nil
}
