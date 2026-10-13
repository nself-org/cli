package commands

// Purpose: The `nself trust` dry-run plan printer, the post-run summary
// printer, and the plain `nself trust status` handler. Split out of
// trust.go (CLI-R12) to separate these printers/status check from the
// config building (trust_config.go), the entry point/cobra wiring
// (trust.go), the undo path (trust_undo.go), and the detailed status
// report (trust_status_detailed.go).
// Inputs: a trustOpts + the runtime GOOS (printTrustPlan), a
// *trust.TrustResult (printTrustSummary), or a trust.TrustConfig
// (runTrustStatus).
// Outputs: printed ui output describing what trust would do, what it did,
// or its current state.
// Constraints: pure move — no behavior changes.

import (
	"fmt"

	"github.com/nself-org/cli/internal/trust"
	"github.com/nself-org/cli/internal/ui"
)

// printTrustPlan prints a summary of what the trust command will do.
func printTrustPlan(opts trustOpts, goos string) {
	fmt.Println()
	ui.Section("Setup plan")

	if !opts.SkipDNS {
		switch goos {
		case "darwin":
			fmt.Println("  1. dnsmasq — install (if needed) + configure address=/.local/127.0.0.1")
			fmt.Println("     /etc/resolver/local — write nameserver 127.0.0.1 (requires admin)")
		case "linux":
			fmt.Println("  1. DNS — configure systemd-resolved or dnsmasq for .local wildcard resolution")
		}
	} else {
		fmt.Println("  1. DNS — SKIPPED (--skip-dns)")
	}

	if !opts.SkipSSL {
		fmt.Println("  2. mkcert — install CA (if needed) + generate wildcard certificates")
	} else {
		fmt.Println("  2. SSL — SKIPPED (--skip-ssl)")
	}

	if !opts.SkipPorts {
		switch goos {
		case "darwin":
			fmt.Println("  3. pfctl — configure 80→Nginx HTTP and 443→Nginx SSL port forwarding (requires admin)")
		case "linux":
			fmt.Println("  3. iptables — configure 80→Nginx HTTP and 443→Nginx SSL port forwarding (requires root)")
		}
	} else {
		fmt.Println("  3. Port forwarding — SKIPPED (--skip-ports)")
	}

	fmt.Println()
}

// printTrustSummary prints the result of the trust setup.
func printTrustSummary(result *trust.TrustResult) {
	fmt.Println()
	ui.Section("Setup complete")

	if result.DNSConfigured {
		if result.DNSAlreadyDone {
			fmt.Println("  \u2713 dnsmasq — already configured")
		} else {
			fmt.Println("  \u2713 dnsmasq — configured")
		}
	}

	if result.ResolverConfigured {
		if result.ResolverAlreadyDone {
			fmt.Println("  \u2713 /etc/resolver/local — already present")
		} else {
			fmt.Println("  \u2713 /etc/resolver/local — written")
		}
	}

	if result.CertsGenerated {
		if result.CertsAlreadyDone {
			fmt.Println("  \u2713 SSL certificates — already valid")
		} else {
			fmt.Println("  \u2713 SSL certificates — generated")
		}
	}

	if result.PortsConfigured {
		if result.PortsAlreadyDone {
			fmt.Println("  \u2713 Port forwarding — already active")
		} else {
			fmt.Println("  \u2713 Port forwarding — configured")
		}
	}

	if len(result.Skipped) > 0 {
		fmt.Println()
		for _, step := range result.Skipped {
			fmt.Printf("  - %s — skipped (headless): no system change made\n", step)
		}
		ui.Warn("Headless mode (NSELF_HEADLESS=1 or CI=1): run `nself trust` once on a machine with admin rights, or set NSELF_HEADLESS=0.")
	}

	if len(result.Errors) == 0 && len(result.Skipped) == 0 {
		fmt.Println()
		ui.Success("Local dev trust setup complete — your *.local projects are ready for HTTPS.")
	}
}

// runTrustStatus prints the current trust status as a table.
func runTrustStatus(cfg trust.TrustConfig) error {
	status := trust.CheckStatus(cfg)

	fmt.Println()
	ui.Section("Trust status")

	check := func(ok bool) string {
		if ok {
			return "\u2713"
		}
		return "\u2717"
	}

	fmt.Printf("  %s mkcert installed\n", check(status.MkcertInstalled))
	fmt.Printf("  %s mkcert CA trusted\n", check(status.CAInstalled))
	fmt.Printf("  %s SSL certificates exist\n", check(status.CertsExist))
	fmt.Printf("  %s SSL certificates valid (>30 days)\n", check(status.CertsValid))
	fmt.Printf("  %s dnsmasq installed\n", check(status.DNSInstalled))
	fmt.Printf("  %s dnsmasq .local configured\n", check(status.DNSRunning))
	fmt.Printf("  %s resolver configured\n", check(status.ResolverConfigured))
	fmt.Printf("  %s port forwarding active\n", check(status.PortsForwarding))
	fmt.Println()

	allGood := status.MkcertInstalled &&
		status.CAInstalled &&
		status.CertsValid &&
		status.DNSRunning &&
		status.ResolverConfigured &&
		status.PortsForwarding

	if allGood {
		ui.Success("All trust components are configured and active.")
	} else {
		ui.Warn("Some trust components are not configured. Run 'nself trust' to set them up.")
	}

	return nil
}
