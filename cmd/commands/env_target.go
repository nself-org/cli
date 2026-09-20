package commands

// env_target.go — implements `nself env target` subcommand group.
//
// Commands:
//   nself env target list    — list server inventory (S23.T01)
//   nself env target add     — add a server to an environment (S23.T01)
//   nself env target remove  — remove a server from an environment (S23.T01)
//   nself env target probe   — resolve capability of targets (S23.T02)
//   nself env target migrate — migrate legacy env-var config to control-plane.yaml (S23.T03)
//
// Security invariants (per S23 CR-C):
//   - SSHKeyRef holds an env-var NAME, never an inline key path or secret.
//   - Inline secrets in --host or --key-ref are rejected on input.
//   - All inventory file writes use mode 0600 (enforced by controlplane.Write).
//   - Probe never forwards credentials; SSH uses BatchMode=yes ForwardAgent=no.
//   - No secret values appear in any output (JSON or table).
//
// RunE bodies live in env_target_crud.go (list/add/remove) and
// env_target_probe.go (probe/migrate + shared helpers) — CLI-R12 Batch B
// mechanical file-size split.

import (
	"github.com/spf13/cobra"
)

// ── parent ───────────────────────────────────────────────────────────────────

var envTargetCmd = &cobra.Command{
	Use:   "target",
	Short: "Manage control-plane server targets",
	Long: `Manage the multi-server inventory stored in .nself/control-plane.yaml.

Subcommands:
  list     List servers in the control-plane inventory
  add      Add a server to an environment
  remove   Remove a server from an environment
  probe    Resolve runtime capability for one or more targets
  migrate  Migrate legacy NSELF_DEPLOY_HOST_* env vars to control-plane.yaml`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself env target` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ── list ─────────────────────────────────────────────────────────────────────

var envTargetListCmd = &cobra.Command{
	Use:   "list [env]",
	Short: "List servers in the control-plane inventory",
	Long: `Print all servers from .nself/control-plane.yaml in a table or JSON.

When [env] is supplied, only servers for that environment are shown.
Use --all to include hidden servers (those with no host configured).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnvTargetList,
}

// ── add ──────────────────────────────────────────────────────────────────────

var envTargetAddCmd = &cobra.Command{
	Use:   "add <env> <server-name>",
	Short: "Add a server to an environment",
	Long: `Add a server entry to .nself/control-plane.yaml.

--host must be user@host (SSH target). Never supply an inline key; use
--key-ref to name the environment variable that holds the key file path.

Example:
  nself env target add staging web1 \
    --host ubuntu@staging.example.com \
    --role app \
    --key-ref NSELF_SSH_KEY_STAGING \
    --remote-path /opt/nself \
    --primary`,
	Args: cobra.ExactArgs(2),
	RunE: runEnvTargetAdd,
}

// ── remove ───────────────────────────────────────────────────────────────────

var envTargetRemoveCmd = &cobra.Command{
	Use:   "remove <env> <server-name>",
	Short: "Remove a server from an environment",
	Args:  cobra.ExactArgs(2),
	RunE:  runEnvTargetRemove,
}

// ── probe ────────────────────────────────────────────────────────────────────

var envTargetProbeCmd = &cobra.Command{
	Use:   "probe [env[:server]]",
	Short: "Resolve runtime capability for one or more targets",
	Long: `Run SSH + Docker probes and print a capability matrix.

When called with no argument all servers in the inventory are probed.
Narrow to one environment with "probe staging" or to one server with
"probe staging:web1".

The command always exits 0; it is a read-only diagnostic. Secrets are
never included in the output — SSHKeyRef env-var names are shown but
their values are not.

Use --refresh to bypass the 60-second on-disk probe cache.
Use --json for machine-readable output.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnvTargetProbe,
}

// ── migrate ──────────────────────────────────────────────────────────────────

var envTargetMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate legacy NSELF_DEPLOY_HOST_* env vars to control-plane.yaml",
	Long: `Synthesize .nself/control-plane.yaml from the legacy single-server env-var
conventions and write it to disk (mode 0600).

This command is idempotent: if control-plane.yaml already exists it is read,
migrated to the current schema version, and rewritten in place without
overwriting any manually added server entries.

After running this command, set NSELF_DEPLOY_HOST_* env vars in .env.secrets
(not .env.dev) and verify with: nself env target list`,
	RunE: runEnvTargetMigrate,
}

// ── init ──────────────────────────────────────────────────────────────────────

func init() {
	// list flags
	envTargetListCmd.Flags().Bool("json", false, "Emit JSON output")
	envTargetListCmd.Flags().Bool("all", false, "Include hidden servers (no host configured)")

	// add flags
	envTargetAddCmd.Flags().String("host", "", "SSH target in user@host form (required for remote servers)")
	envTargetAddCmd.Flags().String("role", "app", "Server role: app|lb|observability|db|worker")
	envTargetAddCmd.Flags().String("key-ref", "", "Name of the env var holding the SSH key path (never the key path itself)")
	envTargetAddCmd.Flags().String("remote-path", "/opt/nself", "Absolute path on the remote host where nSelf is installed")
	envTargetAddCmd.Flags().Bool("primary", false, "Mark this server as the primary app server")
	envTargetAddCmd.Flags().StringSlice("upstreams", nil, "Upstream app server names (LB role only)")

	// probe flags
	envTargetProbeCmd.Flags().Bool("json", false, "Emit JSON output")
	envTargetProbeCmd.Flags().Bool("refresh", false, "Bypass the 60-second probe cache")

	// Wire into envTargetCmd
	envTargetCmd.AddCommand(envTargetListCmd)
	envTargetCmd.AddCommand(envTargetAddCmd)
	envTargetCmd.AddCommand(envTargetRemoveCmd)
	envTargetCmd.AddCommand(envTargetProbeCmd)
	envTargetCmd.AddCommand(envTargetMigrateCmd)

	// Wire envTargetCmd into envCmd (already registered in env.go).
	envCmd.AddCommand(envTargetCmd)
}
