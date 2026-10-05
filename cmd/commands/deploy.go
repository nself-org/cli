package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"

	"github.com/spf13/cobra"
)

// remotePathRe allows safe remote path characters: alphanumeric, slash, hyphen, underscore, dot.
// Aliased from internal/deploy.RemotePathRe (T31 fix) so every remote-path
// validation site in the codebase (this file's --remote-path flag,
// env_target_crud.go's `env target add --remote-path`, and
// internal/controlplane's inventory Load/synthesize) shares one definition
// instead of three that could silently drift apart.
var remotePathRe = deploy.RemotePathRe

// sshKeyRe allows safe filesystem path characters for the SSH key path.
// The key path is interpolated into the rsync "-e ssh -i %s ..." string, which
// rsync shell-interprets — so it must never contain shell metacharacters.
var sshKeyRe = deploy.SSHKeyRe

// Deploy strategies.
var deployStrategies = map[string]bool{
	"rolling":    true,
	"blue-green": true,
	"canary":     true,
	"preview":    true,
}

var deployCmd = &cobra.Command{
	Use:   "deploy [target]",
	Short: "Deploy the stack to a target environment",
	Long: `Deploy the nSelf stack to one environment.

Executes: build (if needed) then start, with strategy-aware orchestration.

Targets are the environments in .nself/control-plane.yaml (or every
NSELF_DEPLOY_HOST_<ENV> variable), plus local. Only the named environment is
deployed; an unknown name is refused (E483) and never falls back to another.
  local       Equivalent to 'nself build && nself start' on this machine
  staging     Deploy to the staging environment (NSELF_DEPLOY_HOST_STAGING)
  production  Deploy to production (alias of prod; requires confirmation)
  <name>      Any other environment in the inventory, e.g. qa

The target can be specified as a positional argument or via --env:
  nself deploy staging
  nself deploy --env staging

Examples:
  nself deploy local
  nself deploy --env staging --dry-run
  nself deploy staging --dry-run
  nself deploy production --strategy=blue-green
  nself deploy --env prod --force --follow
  nself deploy production --rolling --skip-health`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeploy,
}

var deployStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show deployment status for a target",
	RunE:  runDeployStatus,
}

var deployRollbackCmd = &cobra.Command{
	Use:   "rollback [target]",
	Short: "Roll back the last deployment for a target",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDeployRollback,
}

var deployLogsCmd = &cobra.Command{
	Use:   "logs [target]",
	Short: "Show deployment logs for a target",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDeployLogs,
}

var deployHealthCmd = &cobra.Command{
	Use:   "health [target]",
	Short: "Run health checks against a target deployment",
	Long: `Run health checks (nself doctor) against a target deployment.

With no [target] (or [target]=local), runs doctor checks against the local
docker daemon — unchanged from prior versions.

With [target]=staging|prod (or --server <name>), resolves that target's
primary server from .nself/control-plane.yaml (or NSELF_DEPLOY_HOST_<TARGET>)
and runs the check over SSH on the remote host itself, instead of silently
running local checks under a remote-sounding target name.

Note: remote checks require the target host's own nself CLI to support
'nself doctor' — an older remote CLI version returns a clear error naming
the likely version-drift cause rather than a raw SSH failure.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployHealth,
}

var deployCheckAccessCmd = &cobra.Command{
	Use:   "check-access",
	Short: "Verify access to configured deploy targets (deprecated: use 'nself env target probe')",
	Long: `Check SSH capability for all configured deploy environments.

Deprecated: this command performs a shallow env-var check for backward compatibility.
For full per-server SSH capability resolution use:

  nself env target probe [env]

The output format and exit code semantics are preserved for backward compatibility.`,
	RunE: runDeployCheckAccess,
}

// deployPromoteCmd promotes a canary deploy to 100% green traffic.
var deployPromoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Promote a canary deploy to 100% green traffic",
	Long: `Flip Nginx upstream weights from the current canary split to 100% green.
Use after manual inspection of a canary deploy.

Example:
  nself deploy --canary 10   # start canary at 10%
  nself deploy promote       # flip to 100% green after review`,
	RunE: runDeployPromote,
}

// deployEnvironmentsCmd lists environments and their servers with resolved
// capability.  This is the contract that the nSelf Admin companion consumes;
// absence of this endpoint caused Admin 500 errors (T04).
var deployEnvironmentsCmd = &cobra.Command{
	Use:   "environments",
	Short: "List configured deploy environments and server capabilities",
	Long: `List every environment defined in .nself/control-plane.yaml (or synthesized
from NSELF_DEPLOY_HOST_<TARGET> env vars) together with the resolved SSH
capability of each server.

Output is always JSON:

  {
    "environments": [
      {
        "name": "staging",
        "kind": "remote",
        "servers": [
          {
            "name": "staging-app",
            "role": "app",
            "capability": "manage",
            "reason": ""
          }
        ]
      }
    ]
  }

No SSH keys or credential material appear in the output.

Examples:
  nself deploy environments
  nself deploy environments | jq '.environments[].name'`,
	RunE: runDeployEnvironments,
}

func init() {
	f := deployCmd.Flags()
	f.String("strategy", "rolling", "Deploy strategy: rolling|blue-green|canary|preview")
	f.Bool("dry-run", false, "Preview the deploy without executing")
	f.Bool("force", false, "Skip confirmation prompts (prod requires --confirm or --force)")
	f.Bool("rolling", false, "Alias for --strategy=rolling")
	f.Bool("skip-health", false, "Skip post-deploy health checks")
	f.Bool("include-frontends", false, "Include frontend apps in the deploy")
	f.Bool("exclude-frontends", false, "Exclude frontend apps from the deploy")
	f.String("env", "", "Target environment: local, or any inventory environment such as staging, prod or qa (overrides positional arg; required env vars: NSELF_DEPLOY_HOST, NSELF_DEPLOY_USER, NSELF_DEPLOY_KEY_PATH)")
	f.Bool("follow", false, "Stream container logs after deploy until Ctrl-C (staging/prod only)")
	f.Bool("yes", false, "Skip prod confirmation prompt (alias for --force)")
	f.Bool("json", false, "Emit JSON output")

	// Blue/green canary flags (Y17 — blue_green_deploy feature flag).
	f.Int("canary", 0, "Start a canary deploy at N%% traffic to green (0 = full flip)")
	f.Bool("skip-canary", false, "Skip canary phase and flip directly to 100% green")
	f.Bool("force-migration", false, "Force deploy even with backward-incompatible migrations (disables canary)")

	deployStatusCmd.Flags().String("env", "", "Environment to check")
	deployStatusCmd.Flags().Bool("json", false, "Emit JSON output")
	deployStatusCmd.Flags().Bool("blue-green", false, "Show blue/green state alongside deployment status")

	// T05: --server selects a subset of servers for the pipeline.
	f.String("server", "", "Deploy to a specific server only (name from control-plane inventory)")

	deployStatusCmd.Flags().String("server", "", "Filter status output to a specific server")

	deployLogsCmd.Flags().String("server", "", "Stream logs from a specific server via SSH")

	deployHealthCmd.Flags().String("server", "", "Run health check on a specific server via SSH")
	deployHealthCmd.Flags().Bool("json", false, "Emit JSON output")

	deployCheckAccessCmd.Flags().Bool("json", false, "Emit JSON output (deprecated alias: use 'nself env target probe')")

	deployCmd.AddCommand(deployStatusCmd)
	deployCmd.AddCommand(deployRollbackCmd)
	deployCmd.AddCommand(deployLogsCmd)
	deployCmd.AddCommand(deployHealthCmd)
	deployCmd.AddCommand(deployCheckAccessCmd)
	deployCmd.AddCommand(deployPromoteCmd)
	deployCmd.AddCommand(deployEnvironmentsCmd)

	RootCmd.AddCommand(deployCmd)
}

// ── helpers ──────────────────────────────────────────────────────────────────

// deployEnvNameRe is the shape an environment name must have before it is used
// in a file name (.env.<env>) or an environment-variable name.
var deployEnvNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// resolveTarget maps the operator's word to an environment name that exists.
//
// Accepted: "local"; any environment in the inventory (.nself/control-plane.yaml
// or NSELF_DEPLOY_HOST_<ENV>); "production" as an alias of "prod" when the
// inventory has no environment literally named production; and the legacy
// "staging"/"prod" names, which have always resolved even with no inventory
// (the legacy single-host path). Everything else, including an empty name, is
// refused with E483 listing the known environments. It never falls back to a
// default or to "all".
func resolveTarget(raw string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "local" {
		return "local", nil
	}
	var inv *controlplane.Inventory
	var loadErr error
	if deployEnvNameRe.MatchString(t) {
		if root, err := projectRoot(); err == nil {
			inv, loadErr = controlplane.Load(root)
		}
		if name, ok := inventoryEnvName(inv, t); ok {
			return name, nil
		}
		switch t {
		case "production":
			return "prod", nil
		case "staging", "prod":
			return t, nil
		}
	}
	what := fmt.Sprintf("invalid target %q (known: %s)", raw, strings.Join(knownDeployEnvs(inv), ", "))
	if loadErr != nil {
		what += fmt.Sprintf("; inventory unreadable: %v", loadErr)
	}
	return "", errs.New("E483", what)
}

// projectRoot returns the nSelf project root, falling back to cwd if not found.
func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	root, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return cwd, nil
	}
	return root, nil
}

// runCLISelf invokes this same binary with args (used for build/start chaining).
// Falls back to exec.LookPath("nself") if os.Executable fails.
func runCLISelf(ctx context.Context, workdir string, args ...string) error {
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin, _ = exec.LookPath("nself")
	}
	if bin == "" {
		return fmt.Errorf("unable to locate nself binary")
	}
	c := exec.CommandContext(ctx, bin, args...)
	c.Dir = workdir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = os.Environ()
	return c.Run()
}

// ── T04: deploy environments ──────────────────────────────────────────────────
