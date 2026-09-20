package commands

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var sslCmd = &cobra.Command{
	Use:   "ssl",
	Short: "Manage SSL certificates",
	Long: `Manage SSL certificates for your nSelf project.

Subcommands:
  status   Show certificate expiry, covered domains, and CA trust status
  renew    Reload nginx and optionally renew certificates`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself trust ssl` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var sslStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show SSL certificate status",
	Long: `Show SSL certificate status by connecting live to configured domains.

Checks TLS certificates for the base domain and standard subdomains (api, auth).
Exits non-zero when any certificate is expired.`,
	RunE: runSSLStatus,
}

var sslRenewCmd = &cobra.Command{
	Use:   "renew [domain]",
	Short: "Reload nginx and optionally renew certificates",
	Long: `Reload nginx with existing certificates and optionally run certbot renewal.

If a domain argument is provided and certbot is installed, runs:
  certbot renew --cert-name <domain>

Exits non-zero on failure.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSSLRenew,
}

func init() {
	sslCmd.AddCommand(sslStatusCmd)
	sslCmd.AddCommand(sslRenewCmd)
	// CLI-R11 core list: `trust` absorbs dns-setup and ssl. The old top-level
	// spelling keeps working through legacy_spellings.go.
	trustCmd.AddCommand(sslCmd)
}

// checkDomainTLS connects to host:443 with TLS and returns the leaf certificate.
func checkDomainTLS(domain string, timeout time.Duration) (*x509.Certificate, error) {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: timeout},
		"tcp",
		domain+":443",
		&tls.Config{InsecureSkipVerify: false},
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates returned")
	}
	return certs[0], nil
}

// isLocalDomain returns true if the domain resolves to localhost or is a local name.
func isLocalDomain(domain string) bool {
	if domain == "localhost" || domain == "127.0.0.1" || domain == "::1" {
		return true
	}
	// Check if it ends with .local
	if strings.HasSuffix(domain, ".local") {
		return true
	}
	return false
}

// certIssuer returns a short issuer string from a certificate.
func certIssuer(cert *x509.Certificate) string {
	if len(cert.Issuer.Organization) > 0 && cert.Issuer.Organization[0] != "" {
		return cert.Issuer.Organization[0]
	}
	return cert.Issuer.CommonName
}

// runSSLStatus implements `nself ssl status` — live TLS check.
func runSSLStatus(cmd *cobra.Command, args []string) error {
	ui.CommandHeader("nself ssl status", "SSL certificate status")

	cwd, err := os.Getwd()
	if err != nil {
		ui.Error("Failed to determine working directory")
		return fmt.Errorf("getting working directory: %w", err)
	}

	workdir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents: run 'nself init' to create a project")
	}

	cfg, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	base := cfg.BaseDomain
	domains := []string{
		base,
		"api." + base,
		"auth." + base,
	}

	const tlsTimeout = 10 * time.Second
	const expiringThreshold = 14

	// Print table header.
	fmt.Println()
	fmt.Printf("%-24s %-20s %-12s %-6s %s\n", "DOMAIN", "ISSUER", "EXPIRY", "DAYS", "STATUS")
	fmt.Println(strings.Repeat("-", 75))

	anyExpired := false

	for _, domain := range domains {
		if isLocalDomain(domain) {
			fmt.Printf("%-24s %-20s %-12s %-6s %s\n", domain, "-", "-", "-", "SKIPPED (local)")
			continue
		}

		cert, tlsErr := checkDomainTLS(domain, tlsTimeout)
		if tlsErr != nil {
			fmt.Printf("%-24s connection failed (domain may not be live)\n", domain)
			continue
		}

		now := time.Now()
		expiry := cert.NotAfter
		daysRemaining := int(expiry.Sub(now).Hours()) / 24
		issuer := certIssuer(cert)
		expiryStr := expiry.Format("2006-01-02")

		var status string
		if now.After(expiry) {
			status = "EXPIRED"
			anyExpired = true
		} else if daysRemaining <= expiringThreshold {
			status = "EXPIRING"
		} else {
			status = "OK"
		}

		// Truncate issuer to fit column.
		if len(issuer) > 18 {
			issuer = issuer[:18]
		}

		fmt.Printf("%-24s %-20s %-12s %-6d %s\n", domain, issuer, expiryStr, daysRemaining, status)
	}

	fmt.Println()

	if anyExpired {
		return fmt.Errorf("one or more certificates are expired")
	}

	return nil
}
