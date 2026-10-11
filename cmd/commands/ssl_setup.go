package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var sslSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up SSL certificates via DNS-01 challenge",
	Long: `Provision SSL certificates with DNS-01 validation.

In v1.5 mode (NSELF_V15=1) the CLI's own ACME client issues the certificate and
--install-cron installs the nself-acme-renew timer; in v1.4 mode certbot does
(--acme selects the ACME client there too).

Supports wildcard certificates for *.domain when --wildcard is specified.
Providers: cloudflare (default), route53, digitalocean.

Example:
  nself ssl setup --provider cloudflare --wildcard
  nself ssl setup --provider route53 --domain api.example.com`,
	RunE: runSSLSetup,
}

var sslAddCmd = &cobra.Command{
	Use:   "add <domain>",
	Short: "Provision an SSL certificate for a single domain",
	Long: `Provision an SSL certificate for a custom domain (certbot in v1.4 mode, the ACME client over HTTP-01 in v1.5 mode).

After provisioning, generates an nginx server block and reloads nginx.

Example:
  nself ssl add custom.example.com`,
	Args: cobra.ExactArgs(1),
	RunE: runSSLAdd,
}

func init() {
	sslSetupCmd.Flags().String("provider", "cloudflare", "DNS provider (cloudflare, route53, digitalocean)")
	sslSetupCmd.Flags().Bool("wildcard", false, "Request wildcard certificate (*.domain)")
	sslSetupCmd.Flags().String("email", "", "Email for Let's Encrypt registration")
	sslSetupCmd.Flags().Bool("staging", false, "Use Let's Encrypt staging environment")
	sslSetupCmd.Flags().Bool("install-cron", false, "Install a systemd timer for automatic certificate renewal (Linux only)")
	sslCmd.AddCommand(sslSetupCmd)
	sslAddCmd.Flags().String("upstream", "", "Backend service to proxy to (host:port), e.g. app:3000")
	sslCmd.AddCommand(sslAddCmd)
}

// certbotProviderFlag maps provider names to certbot --dns-* flag names.
var certbotProviderFlag = map[string]string{
	"cloudflare":   "dns-cloudflare",
	"route53":      "dns-route53",
	"digitalocean": "dns-digitalocean",
}

func runSSLSetup(cmd *cobra.Command, args []string) error {
	if useACMEEngine(cmd) {
		return v15ACMEError(runSSLSetupACME(cmd, args))
	} else if err := rejectACMEFlags(cmd); err != nil {
		return err
	}
	provider, _ := cmd.Flags().GetString("provider")
	wildcard, _ := cmd.Flags().GetBool("wildcard")
	email, _ := cmd.Flags().GetString("email")
	staging, _ := cmd.Flags().GetBool("staging")
	installCron, _ := cmd.Flags().GetBool("install-cron")

	ui.CommandHeader("nself ssl setup", "SSL certificate provisioning via DNS-01")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	workdir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return fmt.Errorf("no nself project found: run 'nself init' first")
	}

	cfg, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if _, err := exec.LookPath("certbot"); err != nil {
		return fmt.Errorf("certbot not found in PATH — install certbot and the DNS plugin first")
	}

	domain := cfg.BaseDomain
	if domain == "" || domain == "localhost" {
		return fmt.Errorf("BASE_DOMAIN must be set to a real domain for SSL setup")
	}

	if email == "" {
		email = cfg.AdminEmail
	}
	if email == "" {
		return fmt.Errorf("--email is required (or set ADMIN_EMAIL in .env)")
	}

	// Build certbot command.
	certArgs := []string{
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--email", email,
	}

	// Provider-specific flags.
	dnsFlag, ok := certbotProviderFlag[provider]
	if !ok {
		return fmt.Errorf("unsupported SSL provider %q (supported: cloudflare, route53, digitalocean)", provider)
	}
	certArgs = append(certArgs, "--"+dnsFlag)

	// Provider credentials file (certbot convention).
	credFile := fmt.Sprintf("/etc/letsencrypt/%s.ini", provider)
	switch provider {
	case "cloudflare":
		certArgs = append(certArgs, "--dns-cloudflare-credentials", credFile)
	case "route53":
		// route53 uses AWS env vars, no credentials flag needed.
		_ = credFile
	case "digitalocean":
		certArgs = append(certArgs, "--dns-digitalocean-credentials", credFile)
	}

	// Domain(s).
	if wildcard {
		certArgs = append(certArgs, "-d", "*."+domain, "-d", domain)
	} else {
		certArgs = append(certArgs, "-d", domain)
		// Add standard subdomains.
		for _, sub := range []string{"api", "auth"} {
			certArgs = append(certArgs, "-d", sub+"."+domain)
		}
	}

	if staging {
		certArgs = append(certArgs, "--staging")
	}

	// No --cert-path/--key-path: certbot ignores them for `certonly` and always
	// writes to /etc/letsencrypt/live/<domain>/. They previously pointed at
	// /etc/nginx/ssl/<dotted-domain>/, which is a CONTAINER path being handed to
	// a certbot process running on the HOST, so it neither placed nor found
	// anything. The certificate is installed explicitly below instead.
	ui.Info(fmt.Sprintf("Running: certbot %s", strings.Join(certArgs, " ")))

	certCmd := exec.Command("certbot", certArgs...)
	certCmd.Stdout = os.Stdout
	certCmd.Stderr = os.Stderr
	if err := certCmd.Run(); err != nil {
		return fmt.Errorf("certbot failed: %w", err)
	}

	// Install into the tree compose mounts at /etc/nginx/ssl, using the same
	// certificates/<domain-safe> layout internal/ssl and `ssl add` use.
	// For a wildcard request the lineage is still named after the bare domain.
	certDir := filepath.Join(workdir, "ssl", "certificates", domainToFilesafe(domain))
	if err := os.MkdirAll(certDir, 0750); err != nil {
		return fmt.Errorf("creating cert directory: %w", err)
	}
	if err := installIssuedCert(domain, certDir); err != nil {
		return fmt.Errorf("installing certificate for %s: %w", domain, err)
	}

	// Reload nginx.
	if err := nginxCompose(cmd.Context(), workdir, "-s", "reload"); err != nil {
		ui.Warn(fmt.Sprintf("Nginx reload failed: %v (container may not be running)", err))
	}

	ui.Success("SSL setup complete.")

	if installCron {
		if err := installSSLRenewalCron(workdir); err != nil {
			ui.Warn(fmt.Sprintf("Could not install renewal timer: %v", err))
			ui.Info("Add manually to crontab: 30 3 * * * certbot renew --quiet")
		} else {
			ui.Success("SSL renewal systemd timer installed (nself-ssl-renew.timer).")
		}
	} else {
		ui.Info("To enable auto-renewal: nself ssl setup --install-cron")
		ui.Info("Or add manually to crontab: 30 3 * * * certbot renew --quiet")
	}
	return nil
}
