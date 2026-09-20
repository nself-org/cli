// Package commands — server.go
//
// `nself server` closes CLI gap G-011: provisioning, resizing, and
// destroying a server had no CLI surface, so an operator building a new CI
// box or tearing one down had to fall back to raw `hcloud server create` /
// `hcloud server delete` — neither expressible through nself, and neither
// carrying the safety checks below. This is distinct from `nself access`
// (SSH keys on an already-deployed server) and `nself security` (auditing
// one), both of which assume the server already exists.
package commands

import (
	"github.com/nself-org/cli/internal/server"

	"github.com/spf13/cobra"
)

// serverCmd is the parent command for `nself server ...`.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Provision, list, resize, and destroy Hetzner Cloud servers",
	Long: `Manage the lifecycle of a Hetzner Cloud server: create one, list what
exists, resize one, or destroy one.

Subcommands:
  nself server provision   Create a new server
  nself server list        List servers in the project
  nself server resize      Change a server's type (CPU/RAM/disk)
  nself server destroy     Delete a server

destroy is safe by default:
  - refuses to run without a verified backup (--snapshot or --force-no-backup)
  - protects the server's primary IP(s) from auto-deletion unless --release-ip
    is passed

resize refuses (with an explanation) to shrink a server's disk — Hetzner
Cloud has no API for that; the only path is snapshot -> new server -> restore.

A Hetzner Cloud API token is required: set HETZNER_NSELF_TOKEN (or a
project-scoped equivalent via --token-env, or HCLOUD_TOKEN) in the
environment, or pass --token explicitly. The token is never logged.`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself server` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	for _, c := range []*cobra.Command{serverProvisionCmd, serverListCmd, serverResizeCmd, serverDestroyCmd} {
		c.Flags().String("token", "", "Hetzner Cloud API token (overrides --token-env)")
		c.Flags().String("token-env", server.DefaultTokenEnvVar, "env var to read the API token from")
	}

	serverCmd.AddCommand(serverProvisionCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverResizeCmd)
	serverCmd.AddCommand(serverDestroyCmd)
	RootCmd.AddCommand(serverCmd)
}
