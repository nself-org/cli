package commands

// Purpose: Health-check polling and post-start diagnostics for `nself start`:
// waiting for services to reach a healthy threshold, printing per-service
// detail, verifying Docker availability, soft license revalidation, the
// CI-mode backend-triad readiness gate, and dormant-plugin banners. Split
// out of start.go (CLI-R12) to separate the polling/diagnostic loops from
// the main orchestration body in runStart.
// Inputs: a context.Context and *config.Config plus the workdir, timeout,
// and verbosity flags carried by the caller in runStart.
// Outputs: *health.HealthReport values, printed ui output, and errors used
// to fail `nself start --ci-ready` deterministically.
// Constraints: pure move — no behavior changes. waitCIReady must keep
// checking exactly the ciReadyServices set (postgres, hasura, auth) that CI
// pipelines depend on for --skip-db-init readiness.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
)

// runHealthCheckLoop polls service health until the required percentage is met
// or the timeout expires. It updates a spinner with live progress and prints
// per-service status on completion.
func runHealthCheckLoop(ctx context.Context, cfg *config.Config, workdir string, timeoutSec, requiredPct int, verbose bool) *health.HealthReport {
	timeout := time.Duration(timeoutSec) * time.Second
	healthCtx, healthCancel := context.WithTimeout(ctx, timeout)
	defer healthCancel()

	// Services the generated compose declares; checks for anything else are
	// dropped when they fail (D-0502: no nginx under the traefik provider).
	declared := composeDeclaredServices(healthCtx, workdir)

	const pollInterval = 3 * time.Second
	var lastReport *health.HealthReport

	// Show initial spinner.
	hcSp := ui.NewSpinner(fmt.Sprintf("Waiting for services... (timeout: %ds)", timeoutSec))
	hcSp.Start()

	for {
		report, err := health.RunAllChecks(healthCtx, cfg, workdir)
		if err != nil {
			// Transient error: retry unless we are out of time.
			select {
			case <-healthCtx.Done():
				hcSp.Fail(fmt.Sprintf("Health check timed out after %ds: %v", timeoutSec, err))
				return lastReport
			case <-time.After(pollInterval):
				continue
			}
		}

		report = filterDeclaredHealth(report, declared)
		lastReport = report
		actualPct := 0
		if report.Total > 0 {
			actualPct = (report.Healthy * 100) / report.Total
		}

		// Success: threshold met.
		if actualPct >= requiredPct {
			hcSp.Success(fmt.Sprintf("Health checks passed: %d/%d healthy (%d%%)",
				report.Healthy, report.Total, actualPct))
			printServiceDetails(report, verbose)
			return report
		}

		// All services healthy (edge case where total is 0 or rounding).
		if report.Healthy == report.Total && report.Total > 0 {
			hcSp.Success(fmt.Sprintf("All %d services healthy", report.Total))
			printServiceDetails(report, verbose)
			return report
		}

		// Not yet healthy: stop current spinner, print progress, start fresh spinner.
		hcSp.Stop()
		ui.Info(fmt.Sprintf("Waiting for services... %d/%d healthy (%d%%, need %d%%)",
			report.Healthy, report.Total, actualPct, requiredPct))
		hcSp = ui.NewSpinner(fmt.Sprintf("Retrying health checks... %d/%d healthy", report.Healthy, report.Total))
		hcSp.Start()

		// Wait before next poll, or bail if context is done.
		select {
		case <-healthCtx.Done():
			hcSp.Fail(fmt.Sprintf("Health checks below threshold after %ds: %d/%d healthy (%d%%, required %d%%): %s",
				timeoutSec, report.Healthy, report.Total, actualPct, requiredPct, errs.ErrHealthTimeout))
			// Show which services are still unhealthy.
			for _, r := range report.Results {
				if !r.OK() {
					ui.Warn(fmt.Sprintf("  %s %s: %s (%s)", "\u2717", r.Service, r.Status, r.Details))
				}
			}
			return report
		case <-time.After(pollInterval):
			// Continue polling.
		}
	}
}

// printServiceDetails prints per-service health results. In normal mode it only
// shows unhealthy services. In verbose mode it shows all services with response times.
func printServiceDetails(report *health.HealthReport, verbose bool) {
	if verbose {
		ui.Section("Service Health Details")
		for _, r := range report.Results {
			if r.OK() {
				ui.Success(fmt.Sprintf("  %s %s: healthy (%s)", "\u2713", r.Service, r.Duration))
			} else {
				ui.Warn(fmt.Sprintf("  %s %s: %s (%s)", "\u2717", r.Service, r.Status, r.Details))
			}
		}
	} else {
		// Show only unhealthy services in non-verbose mode.
		for _, r := range report.Results {
			if !r.OK() {
				ui.Warn(fmt.Sprintf("  %s %s: %s (%s)", "\u2717", r.Service, r.Status, r.Details))
			}
		}
	}
}

// checkDockerAvailable verifies Docker is installed and running before starting
// the stack. It distinguishes between Docker not being installed (binary absent
// from PATH) and Docker being installed but the daemon not running.
func checkDockerAvailable(ctx context.Context) error {
	if _, lookErr := exec.LookPath("docker"); lookErr != nil {
		return fmt.Errorf("docker binary not found: %w", errs.ErrDockerNotInstalled)
	}
	dockerCmd := exec.CommandContext(ctx, "docker", "info")
	dockerCmd.Stdout = nil
	dockerCmd.Stderr = nil
	if err := dockerCmd.Run(); err != nil {
		return fmt.Errorf("docker info failed — start Docker and try again: %w", errs.ErrDockerNotRunning)
	}
	return nil
}

// isTerminal returns true when os.Stdin is an interactive terminal.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// checkLicenseHeartbeat performs a soft license revalidation if the cached
// validation is older than 7 days. It warns on revoked or expired licenses
// but never blocks the startup sequence. Network errors are treated as
// informational only.
func checkLicenseHeartbeat(ctx context.Context, cfg *config.Config, verbose bool) {
	key, err := license.GetKey()
	if err != nil || key == "" {
		return
	}

	cacheDir := plugin.LicenseCacheDir()
	if !plugin.NeedsRevalidation(key, cacheDir) {
		if verbose {
			ui.Dimmed("  License cache is current (revalidation not needed)")
		}
		return
	}

	if verbose {
		ui.Info("License cache is stale — revalidating...")
	}

	pingURL := cfg.PluginSystem.PingURL
	if pingURL == "" {
		pingURL = "https://ping.nself.org"
	}

	revalCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	valid, err := plugin.ValidateLicenseRemote(revalCtx, key, pingURL)
	if err != nil {
		// Network error: informational only, don't block startup.
		ui.Info("License revalidation skipped (network unavailable)")
		if verbose {
			ui.Dimmed(fmt.Sprintf("  %v", err))
		}
		return
	}

	// Update the cache with the fresh result.
	_ = plugin.CacheLicense(key, valid, cacheDir)

	if valid {
		if verbose {
			ui.Success("License revalidated successfully")
		}
	} else {
		// License revoked or expired: warn but don't block existing services.
		ui.Warn("License validation failed — your license may be revoked or expired")
		ui.Warn("Existing services will continue running. New plugin installs may be blocked.")
		ui.Warn("Visit https://nself.org/pricing to check your subscription status.")
	}
}

// ciReadyServices is the fixed set of backend services that must be healthy
// before --skip-db-init mode considers the stack ready for CI/E2E use.
// Postgres must be running so tests can connect directly; hasura and hasura-auth
// (the "auth" compose service) are required for GraphQL and JWT flows.
var ciReadyServices = []string{"postgres", "hasura", "auth"}

// waitCIReady polls until all three CI backend services (postgres, hasura,
// hasura-auth) report a healthy status, or until the timeout expires.
// It returns nil when the readiness gate is satisfied, or a non-nil error
// that causes nself start to exit non-zero (so CI jobs fail deterministically).
func waitCIReady(ctx context.Context, cfg *config.Config, workdir string, timeoutSec int, verbose bool) error {
	timeout := time.Duration(timeoutSec) * time.Second
	ciCtx, ciCancel := context.WithTimeout(ctx, timeout)
	defer ciCancel()

	const pollInterval = 3 * time.Second

	sp := ui.NewSpinner(fmt.Sprintf("CI mode: waiting for backend triad (postgres, hasura, auth) — timeout %ds", timeoutSec))
	sp.Start()

	for {
		report, err := health.RunAllChecks(ciCtx, cfg, workdir)
		if err == nil && report != nil {
			// Check only the three CI-critical services.
			healthyCount := 0
			for _, r := range report.Results {
				for _, want := range ciReadyServices {
					if r.Service == want && r.OK() {
						healthyCount++
					}
				}
			}
			if healthyCount >= len(ciReadyServices) {
				sp.Success(fmt.Sprintf("CI backend triad ready (postgres, hasura, auth) — %d/%d healthy", healthyCount, len(ciReadyServices)))
				if verbose {
					for _, r := range report.Results {
						ui.Dimmed(fmt.Sprintf("  %s: %s", r.Service, r.Status))
					}
				}
				return nil
			}
		}

		// Not ready yet: wait or bail on context expiry.
		select {
		case <-ciCtx.Done():
			sp.Fail(fmt.Sprintf("CI readiness gate timed out after %ds: backend triad not healthy", timeoutSec))
			return fmt.Errorf("backend triad not healthy after %ds (postgres+hasura+auth required for CI): %w",
				timeoutSec, errs.ErrHealthTimeout)
		case <-time.After(pollInterval):
			// Continue polling.
		}
	}
}

// showDormantBannersOnStart reads the lifecycle store and prints a warning for
// every dormant plugin. It does NOT auto-remove (build-only) and does NOT save
// the store — this is a read-only diagnostic pass to inform operators.
func showDormantBannersOnStart(quiet bool) {
	if quiet {
		return
	}
	store, err := plugin.LoadLifecycleStore()
	if err != nil {
		return // non-fatal: lifecycle is advisory
	}
	now := time.Now()
	for _, rec := range store.Records {
		if rec.State == plugin.StateDormant {
			ui.Warn(plugin.DormantBanner(rec, now))
		}
	}
}
