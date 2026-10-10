package commands

// Purpose: the cmd-side hooks of `nself bundle install` app-ready (P7-PLUG-19):
// reconcile, start, Hasura metadata and service health, injected into
// internal/bundle because that package may not import these layers.
// Inputs: the cobra command (flags for reconcile) and the reconcile trigger.
// Outputs: *appready.Hooks whose functions run against the current project.
// Constraints: every hook that needs a project returns appready.ErrNoProject
// (reconcile) or an error outside one; start runs in process through startCmd
// with its default flags; nothing calls compose or build directly.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/reconcile"

	"github.com/spf13/cobra"
)

// Seams so tests never start a stack or reach Hasura.
var (
	bundleReconcile      = reconcileAfterExtension
	bundleStartStack     = startStackInProcess
	bundleExportMetadata = database.HasuraExportMetadata
	bundleRunHealth      = health.RunAllChecks
)

// startStackInProcess runs `nself start` with its default flags.
func startStackInProcess(ctx context.Context) error {
	startCmd.SetContext(ctx)
	return startCmd.RunE(startCmd, nil)
}

// bundleProjectDir is the nSelf project containing the working directory.
func bundleProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return "", appready.ErrNoProject
	}
	return root, nil
}

func bundleProjectConfig() (*config.Config, string, error) {
	root, err := bundleProjectDir()
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	return cfg, root, nil
}

// newBundleAppReadyHooks builds the hooks for `bundle install <slug>`.
func newBundleAppReadyHooks(cmd *cobra.Command, slug string) *appready.Hooks {
	trigger := reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: slug}
	return &appready.Hooks{
		Reconcile: func(context.Context) error {
			if _, err := bundleProjectDir(); err != nil {
				return err
			}
			return bundleReconcile(cmd, trigger)
		},
		Start: bundleStartStack,
		ExportMetadata: func(ctx context.Context) ([]byte, error) {
			cfg, _, err := bundleProjectConfig()
			if err != nil {
				return nil, err
			}
			return bundleExportMetadata(ctx, cfg)
		},
		ServiceHealth: func(ctx context.Context) error {
			cfg, root, err := bundleProjectConfig()
			if err != nil {
				return err
			}
			report, err := bundleRunHealth(ctx, cfg, root)
			if err != nil {
				return err
			}
			var bad []string
			for _, r := range report.Results {
				if !r.OK() {
					bad = append(bad, fmt.Sprintf("%s (%s)", r.Service, r.Status))
				}
			}
			if len(bad) > 0 {
				return fmt.Errorf("unhealthy services: %s", strings.Join(bad, ", "))
			}
			return nil
		},
	}
}
