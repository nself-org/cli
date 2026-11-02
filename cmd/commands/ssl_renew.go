package commands

// Purpose: Implements `nself ssl renew` — split out of ssl.go (file-size
// ratchet, internal/repoqa) alongside the other ssl_*.go concern splits
// (ssl_add.go, ssl_install.go, ssl_setup.go, ssl_renewal.go).
// Inputs: an optional domain arg; the project workdir.
// Outputs: a reloaded nginx, and — when certbot actually renewed the
// lineage — the renewed cert installed at ssl/certificates/<domain-safe>/.
// Constraints: certbot renew --cert-name must run BEFORE any nginx reload
// (reloading first serves whatever cert is already on disk, which is a
// no-op for freshness); installIssuedCert must only run on a genuine
// renewal, never unconditionally.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// runSSLRenew implements `nself ssl renew [domain]`.
func runSSLRenew(cmd *cobra.Command, args []string) error {
	if useACMEEngine(cmd) {
		return v15ACMEError(runSSLRenewACME(cmd, args))
	} else if err := rejectACMEFlags(cmd); err != nil {
		return err
	}
	ui.CommandHeader("nself ssl renew", "Reload nginx and optionally renew certificates")

	cwd, err := os.Getwd()
	if err != nil {
		ui.Error("Failed to determine working directory")
		return fmt.Errorf("getting working directory: %w", err)
	}

	workdir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents: run 'nself init' to create a project")
	}

	cfg, cfgErr := config.Load(workdir)
	if cfgErr != nil {
		return fmt.Errorf("loading config: %w", cfgErr)
	}

	if len(args) > 0 {
		if err := refuseCertbotOverACME(workdir, cfg.Nginx.FrontedBy); err != nil {
			return err
		}
	}

	// 1. certbot if available and domain provided. This MUST run before any
	// nginx reload — reloading first (the previous order) reloads whatever
	// cert is already on disk and accomplishes nothing toward serving a
	// freshly renewed one.
	if len(args) > 0 {
		domain := args[0]
		if certbotPath, lookErr := exec.LookPath("certbot"); lookErr == nil {
			// Capture the live cert's mtime before renewing so we can tell
			// whether certbot actually renewed it or the lineage is not yet
			// within its renewal window (a no-op certbot exits 0 for either
			// case). Missing file (zero time) counts as "any post-run file
			// is new".
			liveCertPath := filepath.Join(letsEncryptLiveDir, domain, "fullchain.pem")
			var beforeMtime time.Time
			if info, statErr := os.Stat(liveCertPath); statErr == nil {
				beforeMtime = info.ModTime()
			}

			fmt.Printf("Running certbot renew for %s...\n", domain)
			certbotCmd := exec.Command(certbotPath, "renew", "--cert-name", domain)
			certbotCmd.Stdout = os.Stdout
			certbotCmd.Stderr = os.Stderr
			if err := certbotCmd.Run(); err != nil {
				return fmt.Errorf("certbot renew failed: %w", err)
			}

			renewed, installErr := installRenewedCertIfDue(domain, workdir, cfg.Nginx.FrontedBy, beforeMtime)
			if installErr != nil {
				return fmt.Errorf("installing renewed certificate for %s: %w", domain, installErr)
			}
			if renewed {
				fmt.Println("  renewed certificate installed for nginx.")
			} else {
				fmt.Println("  certificate not yet due for renewal — nothing to install.")
			}
		} else {
			fmt.Println("certbot not found — skipping ACME renewal (nginx reload only).")
		}
	}

	// 2. Reload nginx via docker compose exec — after any renewal+install
	// above, so a freshly installed cert is the one actually served.
	fmt.Println("Reloading nginx...")
	if err := nginxCompose(cmd.Context(), workdir, "-s", "reload"); err != nil {
		// Non-fatal: nginx container may not be running.
		fmt.Fprintf(os.Stderr, "Warning: nginx reload reported an error (may not be running): %v\n", err)
	} else {
		fmt.Println("  nginx reloaded.")
	}

	// 3. Show updated certificate status for the provided domain.
	if len(args) > 0 {
		domain := args[0]
		fmt.Printf("\nCertificate status for %s:\n", domain)
		cert, tlsErr := checkDomainTLS(domain, 10*time.Second)
		if tlsErr != nil {
			fmt.Printf("  Could not connect to %s: %v\n", domain, tlsErr)
		} else {
			now := time.Now()
			daysRemaining := int(cert.NotAfter.Sub(now).Hours()) / 24
			issuer := certIssuer(cert)
			fmt.Printf("  Issuer: %s\n", issuer)
			fmt.Printf("  Expiry: %s (%d days remaining)\n", cert.NotAfter.Format("2006-01-02"), daysRemaining)
		}
	}

	fmt.Println("SSL renew complete.")
	return nil
}

// installRenewedCertIfDue installs domain's certbot-renewed cert into the
// served certificate tree (D-0045), only when certbot genuinely renewed it
// (the live fullchain.pem's mtime moved past beforeMtime): renew --cert-name
// exits 0 even when no lineage was due, and stale files must not be reinstalled.
func installRenewedCertIfDue(domain, workdir, frontedBy string, beforeMtime time.Time) (bool, error) {
	liveCertPath := filepath.Join(letsEncryptLiveDir, domain, "fullchain.pem")
	info, err := os.Stat(liveCertPath)
	if err != nil {
		return false, fmt.Errorf("checking renewed cert for %s (did certbot succeed?): %w", domain, err)
	}
	if !info.ModTime().After(beforeMtime) {
		return false, nil
	}

	certDir, sslErr := nginxtopo.ServedCertDir(workdir, frontedBy, domain)
	if sslErr != nil {
		return false, fmt.Errorf("resolving the served ssl directory: %w", sslErr)
	}
	if err := os.MkdirAll(certDir, 0750); err != nil {
		return false, fmt.Errorf("creating %s: %w", certDir, err)
	}
	if err := installIssuedCert(domain, certDir); err != nil {
		return false, err
	}
	return true, nil
}

// refuseCertbotOverACME stops a certbot renew before it runs when the ACME
// engine manages the served stack's certificates: installRenewedCertIfDue would copy
// certbot's pair through the generation symlink, in place and outside the
// ACME lock (D-0461). An unreadable lineages.json refuses too. Same rule as
// `ssl setup` (D-0411).
func refuseCertbotOverACME(workdir, frontedBy string) error {
	sslDir, err := nginxtopo.ServedSSLDir(workdir, frontedBy)
	if err != nil {
		return fmt.Errorf("resolving the served ssl directory: %w", err)
	}
	if f, err := acme.Load(sslDir); err != nil {
		return fmt.Errorf("reading the ACME lineages: %w (fix ssl/.acme/lineages.json, or use --acme)", err)
	} else if len(f.Lineages) > 0 {
		return fmt.Errorf("the ACME engine manages this project's certificates (%s): use `nself trust ssl renew --acme`; certbot would overwrite them", acme.StateDir(sslDir))
	}
	return nil
}
