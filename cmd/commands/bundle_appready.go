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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat"
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
	bundleLoadConfig     = config.Load
)

// startStackInProcess runs `nself start` with its default flags. Outside an
// nSelf project it returns appready.ErrNoProject, not start's own error.
func startStackInProcess(ctx context.Context) error {
	if _, err := bundleProjectDir(); err != nil {
		return err
	}
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
	cfg, err := bundleLoadConfig(root)
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	return cfg, root, nil
}

// cachedProjectConfig loads the project config once per hook set: the metadata
// and health steps poll, and a reload prints an "ENV resolved" INFO line each
// time. A failed load is not cached (the next poll retries).
func cachedProjectConfig() func() (*config.Config, string, error) {
	var cfg *config.Config
	var root string
	return func() (*config.Config, string, error) {
		if cfg != nil {
			return cfg, root, nil
		}
		c, r, err := bundleProjectConfig()
		if err != nil {
			return nil, "", err
		}
		cfg, root = c, r
		return cfg, root, nil
	}
}

// newBundleAppReadyHooks builds the hooks for `bundle install <slug>`: reconcile
// in both modes; start, Hasura metadata and service health only in v1.5.
func newBundleAppReadyHooks(cmd *cobra.Command, slug string) *appready.Hooks {
	trigger := reconcile.Trigger{Kind: reconcile.TriggerBundle, Subject: slug}
	h := &appready.Hooks{
		Reconcile: func(context.Context) error {
			if _, err := bundleProjectDir(); err != nil {
				if errors.Is(err, appready.ErrNoProject) {
					// The v1.4 hint reconcileAfterExtension prints outside a project.
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Extension files changed outside a project. Run 'nself build --plan' in a project to review regeneration.")
				}
				return err
			}
			return bundleReconcile(cmd, trigger)
		},
	}
	// compat.V15(P7-PLUG-19): bundle install reconciles only -> also starts the stack and waits for app-ready
	if compat.V15() {
		projectConfig := cachedProjectConfig()
		h.Start = func(ctx context.Context) error { return bundleStartStack(ctx) }
		h.ExportMetadata = func(ctx context.Context) ([]byte, error) {
			cfg, _, err := projectConfig()
			if err != nil {
				return nil, err
			}
			return bundleExportMetadata(ctx, cfg)
		}
		h.ServiceHealth = func(ctx context.Context) error {
			cfg, root, err := projectConfig()
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
		}
	}
	return h
}
