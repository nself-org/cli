package commands

// ops.go — 'nself ops' command group for ops-profile deployments.
//
// Purpose: deploy the nSelf backend using the ops service profile, which
// enables the observability/CI stack while excluding app-specific services
// (minio, mailpit, admin, functions, search). The ops target is read from
// NSELF_DEPLOY_HOST_OPS (or OPS_DEPLOY_HOST as a legacy fallback).
//
// Commands:
//   nself ops deploy   Deploy with the ops profile to the ops server
//
// Inputs:
//   NSELF_DEPLOY_HOST_OPS - Remote host in user@host:/path format
//   NSELF_DEPLOY_KEY_PATH - SSH private key path
// Outputs: compose file built with --profile ops then rsynced; docker stack updated.
// Constraints: requires 'build' binary (self-invocation) and rsync/ssh in PATH.

import (
	"fmt"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var opsCmd = &cobra.Command{
	Use:   "ops",
	Short: "Ops-profile deployment and management",
	Long: `Manage deployments using the nSelf ops service profile.

The ops profile enables the observability + CI stack (prometheus, grafana,
loki, forgejo, container-registry) while excluding app-specific services
(minio, mailpit, nSelf Admin UI, search).

Required environment variable:
  NSELF_DEPLOY_HOST_OPS   Remote host in user@host:/remote/path format

Examples:
  nself ops deploy
  nself ops deploy --dry-run
  nself ops deploy --follow`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself ops` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var opsDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy with the ops profile to the ops server",
	Long: `Build the compose file with --profile ops then deploy it to the ops server.

Steps:
  1. Build docker-compose.yml with profile=ops (excludes app services)
  2. rsync the generated compose to the remote ops host
  3. Pull updated images and run 'docker compose up -d' on the remote
  4. Optionally stream logs (--follow)

The remote host is resolved from NSELF_DEPLOY_HOST_OPS (or OPS_DEPLOY_HOST
as a legacy fallback). The SSH key is resolved from NSELF_DEPLOY_KEY_PATH.`,
	RunE: runOpsDeploy,
}

func init() {
	opsDeployCmd.Flags().Bool("dry-run", false, "Preview deploy steps without executing")
	opsDeployCmd.Flags().Bool("follow", false, "Stream container logs after deploy until Ctrl-C")

	opsCmd.AddCommand(opsDeployCmd)
	RootCmd.AddCommand(opsCmd)
}

// runOpsDeploy implements 'nself ops deploy'.
//
// Purpose:  Build with ProfileOps then deploy to the ops SSH target.
// Inputs:   --dry-run, --follow flags; SSH config from env vars.
// Outputs:  Remote stack updated with the ops service profile.
// Constraints: rsync and ssh must be in PATH; remote host must be configured.
func runOpsDeploy(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	follow, _ := cmd.Flags().GetBool("follow")

	cfg := deploy.SSHConfigFromEnv("ops")
	cfg.Follow = follow

	if cfg.Host == "" {
		return fmt.Errorf("ops deploy: no remote host configured; set NSELF_DEPLOY_HOST_OPS=user@host:/path")
	}

	workdir, err := projectRoot()
	if err != nil {
		return fmt.Errorf("ops deploy: %w", err)
	}

	ui.Info(fmt.Sprintf("Building compose with profile=%s...", compose.ProfileOps))

	if dryRun {
		ui.Info(fmt.Sprintf("DRY-RUN: would build --profile ops then deploy to %s", cfg.Host))
		return nil
	}

	// Build the compose file with the ops profile.
	result, err := build.Build(workdir, build.BuildOptions{
		Profile: compose.ProfileOps,
	})
	if err != nil {
		return fmt.Errorf("ops deploy: build --profile ops: %w", err)
	}

	composePath := result.ComposeFile
	if composePath == "" {
		// Fall back to the conventional location when BuildResult does not populate it.
		composePath = workdir + "/docker-compose.yml"
	}

	ui.Info(fmt.Sprintf("Deploying ops profile to %s...", cfg.Host))
	if err := deploy.DeployViaSsh(cmd.Context(), cfg, composePath); err != nil {
		return fmt.Errorf("ops deploy: %w", err)
	}

	ui.Success("Ops deploy complete")
	return nil
}
