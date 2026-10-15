package commands

// start_config.go — configuration load/validate and post-config environment
// checks (DNS, BASE_DOMAIN drift, auto-trust prompt, license heartbeat, TLS
// cert expiry) for `nself start`. Split out of start.go (T-P6-E2-W1-S1-T3)
// for 300-line compliance.
// Inputs:  the relevant runStart locals (cmd, projectDir, opts, cfg, ctx).
// Outputs: loadAndValidateStartConfig returns the loaded *config.Config (or
//          error); it also mutates opts.skipHealthChecks/opts.timeout via
//          pointer, matching the original inline env-level overrides.
//          runPostConfigChecks and loadStartComposeFiles are side-effect /
//          plain-value helpers with no error return, matching the originals.
// Constraints: pure move, same checks/output/errors/order, no behavior change.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/truststate"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// loadAndValidateStartConfig loads the resolved Config, applies env-level
// overrides for --skip-health-checks/--timeout when the flags were not set
// explicitly (mutating opts in place), and validates the result (Step 2).
func loadAndValidateStartConfig(cmd *cobra.Command, opts *startOpts, projectDir string) (*config.Config, error) {
	cfg, err := config.Load(projectDir)
	if err != nil {
		ui.UXError(
			"Failed to load configuration",
			err.Error(),
			[]string{
				"Check your .env file for syntax errors",
				"Run 'nself init' to regenerate configuration",
			},
		)
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Apply env-level overrides for skip-health-checks.
	if cfg.SkipHealthChecks {
		opts.skipHealthChecks = true
	}
	// Apply env-level timeout if flag was not explicitly set.
	if !cmd.Flags().Changed("timeout") && !opts.quick && cfg.HealthCheckTimeout > 0 {
		opts.timeout = cfg.HealthCheckTimeout
		if opts.timeout < 30 {
			opts.timeout = 30
		}
		if opts.timeout > 600 {
			opts.timeout = 600
		}
	}

	// Validate config before proceeding. The auto-build path already runs
	// validation inside build.Build(), but we must also validate here for
	// the --skip-build path or when docker-compose.yml already exists.
	if err := config.Validate(cfg); err != nil {
		ui.UXError(
			"Configuration validation failed",
			err.Error(),
			[]string{
				"Review your .env file and fix the reported issues",
				"Run 'nself build --check' to re-validate",
			},
		)
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	if opts.verbose {
		ui.Info(fmt.Sprintf("Project: %s | Domain: %s | Env: %s", cfg.ProjectName, cfg.BaseDomain, cfg.Env))
	}
	ui.Success(fmt.Sprintf("Configuration loaded (%s)", cfg.Env))

	return cfg, nil
}

// runPostConfigChecks runs the DNS resolution check, BASE_DOMAIN change
// detection, the interactive auto-trust prompt, the license revalidation
// heartbeat, and the TLS cert expiry preflight — all warn-and-continue
// checks that never fail the start sequence.
func runPostConfigChecks(ctx context.Context, cmd *cobra.Command, cfg *config.Config, projectDir string, verbose bool) {
	// ── DNS resolution check ─────────────────────────────────────────────
	dnsResolved := true
	if cfg.BaseDomain != "" && cfg.BaseDomain != "localhost" && cfg.BaseDomain != "127.0.0.1" {
		if _, err := net.LookupHost(cfg.BaseDomain); err != nil {
			dnsResolved = false
			ui.Warn(fmt.Sprintf("DNS not configured for %s — run 'nself dns-setup' or 'nself trust' to configure local DNS", cfg.BaseDomain))
		}
	}

	// ── BASE_DOMAIN change detection ─────────────────────────────────────
	if ts, err := truststate.Load(); err == nil {
		if ts.TrustedDomain != "" && ts.TrustedDomain != cfg.BaseDomain {
			ui.Warn(fmt.Sprintf("BASE_DOMAIN changed from %s to %s — SSL certificates need regeneration. Run 'nself trust' to reconfigure.", ts.TrustedDomain, cfg.BaseDomain))
		}
	}

	// ── Auto-trust prompt ────────────────────────────────────────────────
	if !dnsResolved {
		if ts, _ := truststate.Load(); ts.TrustedDomain != cfg.BaseDomain && isTerminal() {
			ui.Info("Would you like to run 'nself trust' now to configure local DNS? [Y/n]: ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(answer)
			if answer == "" || strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes" {
				if err := runDNSSetup(cmd, nil); err != nil {
					ui.Warn(fmt.Sprintf("DNS setup failed: %v", err))
				} else {
					_ = truststate.Save(truststate.TrustState{TrustedDomain: cfg.BaseDomain})
				}
			}
		}
	}

	// ── License revalidation heartbeat ──────────────────────────────
	// Soft check: warn on revoked/expired licenses but never block
	// existing services from starting. Only new plugin installs are
	// blocked by invalid licenses.
	checkLicenseHeartbeat(ctx, cfg, verbose)

	// ── Cert expiry preflight ────────────────────────────────────────
	if msg := certExpiryWarning(startCertPath(projectDir, cfg.BaseDomain), ssl.NeedsCerts(cfg.SSLMode)); msg != "" {
		ui.Warn(msg)
	}
}

// startCertPath is the fullchain nginx serves for baseDomain:
// <project>/ssl/certificates/<dir>/fullchain.pem, the path internal/ssl's
// Generator writes and nginx's ssl_certificate reads. The old join left out
// ssl/ and warned on every start (D-0503).
func startCertPath(projectDir, baseDomain string) string {
	dir := ssl.DomainToDirName(baseDomain)
	return filepath.Join(projectDir, "ssl", "certificates", dir, "fullchain.pem")
}

// certExpiryWarning returns the start warning for the certificate at
// certPath, or "" when there is nothing to say. A missing file only warns
// when the SSL mode expects nself to have generated one (needsCerts);
// letsencrypt, custom and none projects may have no local pair yet.
func certExpiryWarning(certPath string, needsCerts bool) string {
	if _, err := os.Stat(certPath); errors.Is(err, fs.ErrNotExist) && !needsCerts {
		return ""
	}
	days, err := ssl.CheckCertExpiry(certPath)
	switch {
	case err != nil:
		return fmt.Sprintf("TLS certificate issue: %v", err)
	case days < 30:
		return fmt.Sprintf("TLS certificate expires in %d days — renew soon", days)
	}
	return ""
}

// loadStartComposeFiles reads the compose manifest (needed for
// port-ownership checks) and applies the --skip-plugins filter. Read early —
// before the port check — so CheckAllPortsFiltered can query only nself's
// own containers and avoid flagging them as conflicts.
func loadStartComposeFiles(projectDir string, opts startOpts) []string {
	composeFiles, err := build.ReadComposeManifest(projectDir)
	if err != nil {
		if opts.verbose {
			ui.Warn(fmt.Sprintf("Could not read compose manifest: %v (using defaults)", err))
		}
		composeFiles = nil
	}

	// --skip-plugins: keep only the first entry (base docker-compose.yml).
	if opts.skipPlugins && len(composeFiles) > 1 {
		ui.Info("Skipping plugin compose files (--skip-plugins)")
		composeFiles = composeFiles[:1]
	}

	return composeFiles
}
