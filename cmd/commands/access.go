// Package commands — access.go
//
// `nself access` manages SSH keys on an already-deployed server: granting a
// teammate access, revoking it, and listing who currently has it. This is
// distinct from `nself security`, which hardens firewall/fail2ban/sshd on
// the host but has no concept of individual keys or users, and distinct from
// `hcloud ssh-key create`, which only injects a key at server-creation time
// and does nothing for a server that is already running.
package commands

import (
	"github.com/spf13/cobra"
)

// accessCmd is the parent command for `nself access ...`.
var accessCmd = &cobra.Command{
	Use:   "access",
	Short: "Manage SSH key access on an already-deployed server",
	Long: `Grant, revoke, and list SSH key access on a running nself host.

Subcommands:
  nself access grant    Add (or update) a person's key in authorized_keys
  nself access revoke   Remove a person's key from authorized_keys
  nself access list     Show who currently has access

Every grant and revoke:
  - is idempotent: re-granting the same key for the same person is a no-op
  - backs up authorized_keys with a timestamp before making any change
  - prints the key's fingerprint back for verification
  - is recorded in a local audit log (~/.nself/access-audit.log)

Revoke refuses to remove the last remaining key on a host unless --force is
given, since that would lock out all SSH access. Pass --dry-run on grant or
revoke to see the resulting authorized_keys diff without changing anything.`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself access` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var accessGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant a person SSH key access to a host",
	Long: `Add a person's public key to a host's authorized_keys, or update it if
they already have one.

--sudo and --docker record the intended privilege level as metadata for
audit and inventory purposes; this command does not itself modify OS group
membership. Use your normal provisioning path (e.g. usermod -aG) for that.`,
	Example: `  nself access grant --host root@203.0.113.5 --user alice --key @alice.pub
  nself access grant --host root@203.0.113.5 --user bob --key "ssh-ed25519 AAAA... bob@laptop" --sudo
  nself access grant --host root@203.0.113.5 --user carol --key @carol.pub --expires 2026-12-31
  nself access grant --host root@203.0.113.5 --user dave --key @dave.pub --dry-run`,
	RunE: runAccessGrant,
}

var accessRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke a person's SSH key access to a host",
	Long: `Remove a person's public key from a host's authorized_keys.

Refuses to remove the last remaining key on the host — that would lock out
all SSH access — unless --force is given.`,
	Example: `  nself access revoke --host root@203.0.113.5 --user alice
  nself access revoke --host root@203.0.113.5 --user alice --dry-run
  nself access revoke --host root@203.0.113.5 --user alice --force`,
	RunE: runAccessRevoke,
}

var accessListCmd = &cobra.Command{
	Use:   "list",
	Short: "List who has SSH key access to a host",
	Example: `  nself access list --host root@203.0.113.5
  nself access list --host root@203.0.113.5 --json`,
	RunE: runAccessList,
}

func init() {
	for _, c := range []*cobra.Command{accessGrantCmd, accessRevokeCmd, accessListCmd} {
		c.Flags().String("host", "", "SSH connection target, [user@]host (required)")
		c.Flags().String("identity", "", "local SSH private key used to connect (default: ~/.ssh/id_ed25519)")
	}

	accessGrantCmd.Flags().String("user", "", "label identifying whose key this is (required)")
	accessGrantCmd.Flags().String("key", "", "public key material, or @path/to/file (required)")
	accessGrantCmd.Flags().Bool("sudo", false, "record sudo as the intended privilege level")
	accessGrantCmd.Flags().Bool("docker", false, "record docker-group access as the intended privilege level")
	accessGrantCmd.Flags().String("expires", "", "optional expiry date, YYYY-MM-DD")
	accessGrantCmd.Flags().Bool("dry-run", false, "print the resulting authorized_keys diff, change nothing")

	accessRevokeCmd.Flags().String("user", "", "label identifying whose key to remove (required)")
	accessRevokeCmd.Flags().Bool("force", false, "allow removing the last remaining key on the host")
	accessRevokeCmd.Flags().Bool("dry-run", false, "print the resulting authorized_keys diff, change nothing")

	accessListCmd.Flags().Bool("json", false, "output as JSON")

	accessCmd.AddCommand(accessGrantCmd)
	accessCmd.AddCommand(accessRevokeCmd)
	accessCmd.AddCommand(accessListCmd)
	RootCmd.AddCommand(accessCmd)
}
