package commands

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var sslCmd = &cobra.Command{
	Use:   "ssl",
	Short: "Manage SSL certificates",
	Long: `Manage SSL certificates for your nSelf project.

Subcommands:
  status   Show certificate expiry, covered domains, and CA trust status
  renew    Reload nginx and optionally renew certificates`,
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
	printLineageStatus(os.Stdout, workdir, cfg.Nginx.FrontedBy)

	if anyExpired {
		return fmt.Errorf("one or more certificates are expired")
	}

	return nil
}

// printLineageStatus prints the lineages the CLI manages (<served ssl>/.acme/
// lineages.json, contract:cli.tls-lineages) with the expiry of the certificate in
// their first target, then warns when host certbot also manages one of their
// names. It prints nothing when no lineages are recorded, so `status` output
// without --acme use is unchanged.
func printLineageStatus(w io.Writer, projectDir, frontedBy string) {
	sslDir, err := nginxtopo.ServedSSLDir(projectDir, frontedBy)
	if err != nil {
		return
	}
	f, err := acme.Load(sslDir)
	if err != nil {
		_, _ = fmt.Fprintf(w, "Lineages: cannot read %s: %v\n\n", acme.StateDir(sslDir), err)
		return
	}
	if len(f.Lineages) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "Lineages managed by nself (%s):\n%-26s %-9s %-13s %-12s %-6s %-8s %s\n", filepath.Join(acme.StateDir(sslDir), "lineages.json"),
		"LINEAGE", "CHALLENGE", "PROVIDER", "EXPIRY", "DAYS", "STATUS", "TARGETS")
	for _, l := range f.Lineages {
		expiry, days, status := "-", "-", "UNREADABLE"
		if len(l.Targets) > 0 {
			if c, cerr := ssl.ReadDiskCert(filepath.Join(sslDir, filepath.FromSlash(l.Targets[0]), "fullchain.pem")); cerr == nil {
				d := int(math.Floor(time.Until(c.NotAfter).Hours() / 24))
				expiry, days, status = c.NotAfter.UTC().Format("2006-01-02"), fmt.Sprint(d), strings.ToUpper(string(ssl.Classify(d)))
			}
		}
		_, _ = fmt.Fprintf(w, "%-26s %-9s %-13s %-12s %-6s %-8s %s\n", l.Name, l.Challenge, l.DNSProvider, expiry, days, status, strings.Join(l.Targets, ","))
		for _, name := range certbotOverlap(filepath.Dir(letsEncryptLiveDir), l) {
			_, _ = fmt.Fprintf(w, "WARNING: %s is also managed by certbot (%s); two renewers can overwrite each other. Stop the certbot renewal for it once adopted (nself never edits certbot state).\n", l.Name, name)
		}
	}
	_, _ = fmt.Fprintln(w)
}

// certbotOverlap returns the certbot renewal confs under dir whose lineage name
// or certificate names include one of l's domains.
func certbotOverlap(dir string, l acme.Lineage) (confs []string) {
	cbs, err := acme.ReadCertbot(dir)
	if err != nil {
		return nil
	}
	for _, cb := range cbs {
		for _, d := range l.Domains {
			if cb.Name == d || slices.Contains(cb.Domains, d) {
				confs = append(confs, cb.Conf)
				break
			}
		}
	}
	return confs
}

// pollServed waits for nginx to serve the certificate with fingerprint want for
// sni: `nginx -s reload` returns before the old workers stop answering.
func pollServed(ctx context.Context, addr, sni, want string) (err error) {
	for i := 0; i < 20; i++ {
		got, perr := acmeD.probe(ctx, addr, sni, 10*time.Second)
		if err = perr; err == nil {
			if got.SHA256 == want {
				return nil
			}
			err = fmt.Errorf("nginx at %s serves another certificate for %s than the one just installed", addr, sni)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(acmeVerifyWait):
		}
	}
	return err
}

// Helpers shared by the --acme commands: the E151 error and the terms-of-service prompt.

// e151 turns an error into the CLI's E151 carrying its remediation.
func e151(err error) error {
	what, fix := "ACME: "+err.Error(), "re-run with --dry-run to see the plan"
	var ae *acme.Error
	if errors.As(err, &ae) {
		what, fix = ae.What, ae.Fix
	}
	e := errs.New("E151", what)
	e.Why, e.Fix, e.Wrapped = "", fix, err
	return e
}

// askTTY prompts on a terminal and reports a yes; false when stdin is not a terminal.
func askTTY(prompt string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
}
