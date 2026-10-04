package commands

// Purpose: Implements `nself ssl add <domain>` — issues a certbot cert for
// an additional custom domain and wires its nginx config, plus the small
// helper that makes a domain name safe to embed in a filename. Split out
// of ssl_setup.go (CLI-R12) to separate this subcommand from the main
// `nself ssl setup` flow (ssl_setup.go), the cert-install/nginx-config
// helpers (ssl_install.go), and the renewal-hook installers
// (ssl_renewal.go).
// Inputs: the cobra.Command + args (the domain to add) and flags shared
// with `ssl setup` (provider, email).
// Outputs: an issued certificate installed via installIssuedCert and a
// generated nginx server block via writeCustomDomainConf.
// Constraints: certificate and conf go to the served nginx/ssl trees
// (P7-LIVE-02, D-0045); the nginx test/reload run in the served stack's
// compose project. domainToFilesafe is also used by ssl_install.go and
// ssl_renewal.go.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runSSLAdd(cmd *cobra.Command, args []string) error {
	domain := args[0]
	upstream, _ := cmd.Flags().GetString("upstream")

	ui.CommandHeader("nself ssl add", fmt.Sprintf("Provision certificate for %s", domain))

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
		return fmt.Errorf("certbot not found in PATH")
	}

	email := cfg.AdminEmail
	if email == "" {
		return fmt.Errorf("ADMIN_EMAIL must be set in .env for certificate registration")
	}

	// Create the cert output directory so certbot can write into it.
	//
	// This must match the layout the nginx container actually sees. Compose
	// mounts "./ssl:/etc/nginx/ssl:ro" and internal/ssl writes certificates to
	// ssl/certificates/<dir>, where <dir> is the domain with dots replaced by
	// dashes. Writing to ssl/<dotted-domain> instead produced a cert on disk
	// that the generated server block could never reference, so `ssl add`
	// reported success while nginx kept serving the self-signed wildcard.
	//
	// Both targets are the SERVED tree (internal/nginxtopo, D-0045): a project
	// with NGINX_FRONTED_BY set has no nginx of its own, so its certificate and
	// custom-domain conf belong in the fronting stack's ssl/ and nginx/ dirs.
	// Resolved before certbot runs so an unconfirmed layout fails early.
	frontedBy := cfg.Nginx.FrontedBy
	domainSafe := domainToFilesafe(domain)
	certDir, err := servedCertDir(workdir, frontedBy, domain)
	if err != nil {
		return fmt.Errorf("locating the served ssl directory: %w", err)
	}
	servedRoot, err := nginxtopo.ServedRoot(workdir, frontedBy)
	if err != nil {
		return fmt.Errorf("locating the served nginx stack: %w", err)
	}
	if err := os.MkdirAll(certDir, 0750); err != nil {
		return fmt.Errorf("creating cert directory: %w", err)
	}

	// Use HTTP-01 challenge for single domain adds (simpler, no DNS provider needed).
	//
	// --cert-path/--key-path are deliberately NOT passed: certbot ignores them
	// for `certonly` (they apply to `install`), which is why earlier versions
	// appeared to place the certificate where nginx expected it while certbot
	// actually wrote to /etc/letsencrypt/live/<domain>/ and nothing ever bridged
	// the two. We copy explicitly below instead.
	certArgs := []string{
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--email", email,
		"--webroot",
		"--webroot-path", "/var/www/certbot",
		"-d", domain,
	}

	ui.Info(fmt.Sprintf("Provisioning certificate for %s...", domain))
	certCmd := exec.Command("certbot", certArgs...)
	certCmd.Stdout = os.Stdout
	certCmd.Stderr = os.Stderr
	if err := certCmd.Run(); err != nil {
		return fmt.Errorf("certbot failed: %w", err)
	}

	// Install the issued certificate into the tree nginx actually reads.
	// Without this the cert exists only under /etc/letsencrypt and nginx keeps
	// serving the self-signed wildcard, which is a silent no-op from the
	// operator's point of view.
	if err := installIssuedCert(domain, certDir); err != nil {
		return fmt.Errorf("installing certificate for %s: %w", domain, err)
	}

	// Write the nginx server block for this custom domain.
	if err := writeCustomDomainConfServed(workdir, frontedBy, domain, upstream); err != nil {
		return fmt.Errorf("writing nginx conf: %w", err)
	}

	// Validate nginx config before reloading.
	testCmd := exec.Command("docker", "compose", "exec", "nginx", "nginx", "-t")
	testCmd.Dir = servedRoot
	if out, testErr := testCmd.CombinedOutput(); testErr != nil {
		return fmt.Errorf("nginx config test failed: %s", string(out))
	}

	reloadCmd := exec.Command("docker", "compose", "exec", "nginx", "nginx", "-s", "reload")
	reloadCmd.Dir = servedRoot
	reloadCmd.Stdout = os.Stdout
	reloadCmd.Stderr = os.Stderr
	if err := reloadCmd.Run(); err != nil {
		ui.Warn(fmt.Sprintf("Nginx reload failed: %v", err))
	}

	confDir, err := servedConfDir(workdir, frontedBy)
	if err != nil {
		return fmt.Errorf("locating the served nginx directory: %w", err)
	}
	ui.Info(fmt.Sprintf("Custom domain conf written to %s",
		filepath.Join(confDir, fmt.Sprintf("custom-%s.conf", domainSafe))))
	ui.Success(fmt.Sprintf("Certificate provisioned for %s.", domain))
	return nil
}

// domainToFilesafe replaces dots and colons with dashes so a domain name is
// safe to embed in a filename. e.g. "my.custom.com" -> "my-custom-com".
func domainToFilesafe(domain string) string {
	r := strings.NewReplacer(".", "-", ":", "-")
	return r.Replace(domain)
}
