package commands

// Purpose: The three `nself security` subcommand handlers — audit (runs
// every check and prints a report), status (a condensed pass/fail
// summary), and setup (prints or, with --apply, performs the planned
// hardening steps). Split out of security.go (CLI-R12) to separate the
// command handlers from the check functions
// (security_checks_certs_keys.go, security_checks_system.go) and the
// cobra wiring that remains in security.go.
// Inputs: the cobra.Command + args/flags (--apply for setup).
// Outputs: printed audit/status reports, or applied hardening changes.
// Constraints: pure move — no behavior changes.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runSecurityAudit(cmd *cobra.Command, args []string) error {
	ui.CommandHeader("nself security audit", "Read-only security audit")
	findings := runChecks()
	pass, fail := 0, 0
	for _, f := range findings {
		mark := "FAIL"
		if f.OK {
			mark = "PASS"
			pass++
		} else {
			fail++
		}
		fmt.Printf("  [%s] %-25s  %s\n", mark, f.Name, f.Detail)
	}
	setData(cmd, securityAuditData{Findings: newSecurityFindings(findings), Passed: pass, Failed: fail})
	fmt.Printf("\n%d passed, %d failed\n", pass, fail)
	if fail > 0 {
		fmt.Println("\nRun 'nself security setup' to apply baseline hardening.")
	}
	return nil
}

func runSecurityStatus(cmd *cobra.Command, args []string) error {
	ui.CommandHeader("nself security status", "Current security posture")
	findings := runChecks()
	score := 0
	for _, f := range findings {
		if f.OK {
			score++
		}
	}
	pct := (score * 100) / len(findings)
	setData(cmd, securityStatusData{Findings: newSecurityFindings(findings), Score: score, Total: len(findings), Percent: pct})
	fmt.Printf("Posture score: %d/%d (%d%%)\n\n", score, len(findings), pct)
	for _, f := range findings {
		state := "needs attention"
		if f.OK {
			state = "ok"
		}
		fmt.Printf("  - %s: %s (%s)\n", f.Name, state, f.Detail)
	}
	return nil
}

// hardeningStep describes a single setup action.
type hardeningStep struct {
	Name string
	Cmd  []string
}

func plannedHardeningSteps() []hardeningStep {
	// S32-T11: install iptables-persistent / netfilter-persistent so firewall
	// rules (ufw + any custom iptables) survive reboots. On Debian/Ubuntu ufw
	// rules persist via the ufw systemd unit, but raw iptables rules do not
	// without netfilter-persistent. Install both packages + enable the service
	// and save current rules to /etc/iptables/rules.v4 and rules.v6.
	return []hardeningStep{
		{Name: "Install ufw + fail2ban + iptables-persistent", Cmd: []string{"apt-get", "install", "-y", "ufw", "fail2ban", "iptables-persistent", "netfilter-persistent"}},
		{Name: "Allow SSH (22)", Cmd: []string{"ufw", "allow", "22/tcp"}},
		{Name: "Allow HTTP (80)", Cmd: []string{"ufw", "allow", "80/tcp"}},
		{Name: "Allow HTTPS (443)", Cmd: []string{"ufw", "allow", "443/tcp"}},
		{Name: "Enable ufw", Cmd: []string{"ufw", "--force", "enable"}},
		{Name: "Enable fail2ban", Cmd: []string{"systemctl", "enable", "--now", "fail2ban"}},
		{Name: "Enable netfilter-persistent", Cmd: []string{"systemctl", "enable", "--now", "netfilter-persistent"}},
		{Name: "Save iptables rules (v4)", Cmd: []string{"sh", "-c", "iptables-save > /etc/iptables/rules.v4"}},
		{Name: "Save iptables rules (v6)", Cmd: []string{"sh", "-c", "ip6tables-save > /etc/iptables/rules.v6"}},
		{Name: "Disable SSH password auth", Cmd: []string{"sed", "-i", "s/^#*PasswordAuthentication.*/PasswordAuthentication no/", "/etc/ssh/sshd_config"}},
		{Name: "Disable SSH root login", Cmd: []string{"sed", "-i", "s/^#*PermitRootLogin.*/PermitRootLogin no/", "/etc/ssh/sshd_config"}},
		{Name: "Reload sshd", Cmd: []string{"systemctl", "reload", "sshd"}},
	}
}

func runSecuritySetup(cmd *cobra.Command, args []string) error {
	apply, _ := cmd.Flags().GetBool("apply")
	ui.CommandHeader("nself security setup", "Baseline server hardening")

	if runtime.GOOS != "linux" {
		setData(cmd, newSecuritySetupData(!apply, false, nil, nil))
		fmt.Println("Server hardening is only supported on Linux hosts.")
		return nil
	}

	steps := plannedHardeningSteps()
	setData(cmd, newSecuritySetupData(!apply, true, nil, steps))

	if !apply {
		fmt.Println("Dry-run — no changes will be made. Pass --apply to execute.")
		fmt.Println()
		for i, s := range steps {
			fmt.Printf("  %2d. %-32s  $ %s\n", i+1, s.Name, strings.Join(s.Cmd, " "))
		}
		fmt.Println("\nRe-run with --apply (as root) to perform these steps.")
		return nil
	}

	if os.Geteuid() != 0 {
		return fmt.Errorf("'nself security setup --apply' must be run as root (try sudo)")
	}

	var failed []string
	for _, s := range steps {
		fmt.Printf("→ %s\n", s.Name)
		c := exec.Command(s.Cmd[0], s.Cmd[1:]...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Printf("  step failed: %v (continuing)\n", err)
			failed = append(failed, s.Name)
		}
	}
	setData(cmd, newSecuritySetupData(false, true, failed, steps))
	fmt.Println("\nHardening complete. Run 'nself security audit' to verify.")
	return nil
}
