package commands

// Purpose: Implements `nself trust status --verbose` (the detailed report):
// inspects the installed mkcert CA certificate, pf anchor, and launch
// daemon state and prints a full diagnostic breakdown, including parsing
// the CA cert's not-before/not-after and subject fields. Split out of
// trust.go (CLI-R12) to separate this larger detailed-report handler from
// the entry point/cobra wiring (trust.go), config building
// (trust_config.go), plan/summary/plain-status printers (trust_status.go),
// and the undo path (trust_undo.go).
// Inputs: a trust.TrustConfig describing what should be installed.
// Outputs: a printed multi-section diagnostic report.
// Constraints: pure move — no behavior changes.

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/trust"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// runTrustStatusDetailed shows trusted cert CAs with expiry dates and warns
// on certificates expiring within 30 days. Returns exit code 2 via error when
// any component is missing or a cert is near-expiry.
func runTrustStatusDetailed(cmd *cobra.Command, cfg trust.TrustConfig) error {
	status := trust.CheckStatus(cfg)

	fmt.Println()
	ui.Section("Trust status — certificate authorities")

	hasWarning := false

	// mkcert CA info.
	if !status.MkcertInstalled {
		fmt.Println("  ✗ mkcert — not installed")
		hasWarning = true
	} else {
		caPath, caErr := ssl.MkcertCACertPath()
		if caErr != nil {
			fmt.Println("  ✗ mkcert CA — path unavailable")
			hasWarning = true
		} else {
			caInfo := parseCACertInfo(caPath)
			if caInfo == nil {
				if status.CAInstalled {
					fmt.Printf("  ✓ mkcert CA — trusted (expiry unreadable)\n")
				} else {
					fmt.Printf("  ✗ mkcert CA — not trusted  path: %s\n", caPath)
					hasWarning = true
				}
			} else {
				daysLeft := int(time.Until(caInfo.NotAfter).Hours()) / 24
				mark := "✓"
				if !status.CAInstalled {
					mark = "✗"
					hasWarning = true
				}
				if daysLeft < 30 {
					hasWarning = true
					fmt.Printf("  %s mkcert CA %-32s  expires: %s  WARN: %d days remaining\n",
						mark, caInfo.Subject.CommonName,
						caInfo.NotAfter.Format("2006-01-02"), daysLeft)
				} else {
					fmt.Printf("  %s mkcert CA %-32s  expires: %s  (%d days)\n",
						mark, caInfo.Subject.CommonName,
						caInfo.NotAfter.Format("2006-01-02"), daysLeft)
				}
			}
		}
	}

	// Project SSL cert (if WorkDir is set).
	fmt.Println()
	ui.Section("Trust status — project certificates")

	if cfg.WorkDir == "" {
		fmt.Println("  (no project directory — run from a project root or pass -p)")
	} else {
		certPath := filepath.Join(cfg.WorkDir, "ssl", "fullchain.pem")
		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			fmt.Println("  ✗ project cert — not found (run 'nself trust' to generate)")
			hasWarning = true
		} else {
			certInfo := parseCACertInfo(certPath)
			if certInfo == nil {
				fmt.Printf("  ? project cert — found but unreadable: %s\n", certPath)
				hasWarning = true
			} else {
				daysLeft := int(time.Until(certInfo.NotAfter).Hours()) / 24
				if daysLeft < 0 {
					fmt.Printf("  ✗ project cert %-30s  EXPIRED %d days ago\n",
						certInfo.Subject.CommonName, -daysLeft)
					hasWarning = true
				} else if daysLeft < 30 {
					fmt.Printf("  ⚠ project cert %-30s  expires: %s  WARN: %d days remaining\n",
						certInfo.Subject.CommonName,
						certInfo.NotAfter.Format("2006-01-02"), daysLeft)
					hasWarning = true
				} else {
					fmt.Printf("  ✓ project cert %-30s  expires: %s  (%d days)\n",
						certInfo.Subject.CommonName,
						certInfo.NotAfter.Format("2006-01-02"), daysLeft)
				}
			}
		}
	}

	// Other trust components.
	fmt.Println()
	ui.Section("Trust status — components")

	check := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}

	fmt.Printf("  %s DNS (.local wildcard)\n", check(status.DNSRunning))
	fmt.Printf("  %s /etc/resolver configured\n", check(status.ResolverConfigured))
	fmt.Printf("  %s port forwarding (80/443)\n", check(status.PortsForwarding))
	fmt.Println()

	if !status.DNSRunning || !status.ResolverConfigured || !status.PortsForwarding {
		hasWarning = true
	}

	if jsonEnvelopeOn(cmd) {
		data := trustStatusData{MkcertInstalled: status.MkcertInstalled, CAInstalled: status.CAInstalled, CertsExist: status.CertsExist,
			CertsValid: status.CertsValid, DNSInstalled: status.DNSInstalled, DNSRunning: status.DNSRunning,
			ResolverConfigured: status.ResolverConfigured, PortsForwarding: status.PortsForwarding, Warnings: hasWarning}
		if hasWarning {
			// The verdict is the exit status; the document is still written.
			if err := emitEnv(cmd, data, false); err != nil {
				return err
			}
			return &plugin.ExitCodeError{Code: 1}
		}
		setData(cmd, data)
	}

	if hasWarning {
		ui.Warn("Some trust components need attention. Run 'nself trust' to fix.")
		// Return a sentinel that produces exit code 2 (warning, not hard error).
		return fmt.Errorf("trust warnings present (exit code 2)")
	}

	ui.Success("All trust components configured — no expiry warnings.")
	return nil
}

// parseCACertInfo reads and parses the first PEM certificate in path.
// Returns nil if the file cannot be read or parsed (graceful degradation).
func parseCACertInfo(path string) *x509.Certificate {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return cert
}
