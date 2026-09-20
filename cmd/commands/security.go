// Package commands — security.go
//
// `nself security` provides server-level security operations: audit, setup,
// and status. These commands inspect the host environment (firewall, fail2ban,
// SSH config) and the running stack for common hardening gaps. They are
// intentionally non-destructive by default — `setup` requires `--apply` to
// make changes, otherwise it prints what would be done.
package commands

import (
	"github.com/spf13/cobra"
)

// securityCmd is the parent command for `nself security ...`.
var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Server security: audit, setup, and status",
	Long: `Inspect and harden the host running your nself stack.

Subcommands:
  nself security audit    Run a read-only security audit of the host + stack
  nself security setup    Apply baseline hardening (firewall, fail2ban, sshd)
  nself security status   Show the current security posture as a summary

All commands are safe by default — 'setup' will not change anything unless
'--apply' is passed. Without '--apply' it prints the steps it would take.`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself security` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var securityAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run a read-only security audit",
	RunE:  runSecurityAudit,
}

var securityStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current security posture",
	RunE:  runSecurityStatus,
}

var securitySetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Apply baseline server hardening",
	Long: `Apply baseline server hardening:
  - Enable UFW firewall and allow 22/80/443
  - Install and enable fail2ban with sshd jail
  - Disable SSH password authentication and root login

Without --apply, prints the planned actions and exits without making changes.`,
	RunE: runSecuritySetup,
}

func init() {
	securitySetupCmd.Flags().Bool("apply", false, "Actually apply changes (default: dry-run)")
	securityCmd.AddCommand(securityAuditCmd)
	securityCmd.AddCommand(securityStatusCmd)
	securityCmd.AddCommand(securitySetupCmd)
	RootCmd.AddCommand(securityCmd)
}

// finding represents one security check result.
type finding struct {
	Name   string
	OK     bool
	Detail string
}

// runChecks performs all read-only security probes and returns findings.
func runChecks() []finding {
	var out []finding
	out = append(out, checkFirewall())
	out = append(out, checkIptablesPersistent())
	out = append(out, checkFail2ban())
	out = append(out, checkSSHConfig())
	out = append(out, checkRootLogin())
	out = append(out, checkUnattendedUpgrades())
	out = append(out, checkDockerExposedPorts())
	out = append(out, checkEnvFilePerms())
	out = append(out, checkCertExpiry())
	out = append(out, checkKeyRotationAge())
	out = append(out, checkAdminPortBinding())
	out = append(out, checkCSRFGuard())
	return out
}
