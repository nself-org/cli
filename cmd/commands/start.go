package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/hasura"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/license/entitlement"
	"github.com/nself-org/cli/internal/lifecycle"
	"github.com/nself-org/cli/internal/migration"
	"github.com/nself-org/cli/internal/search"
	"github.com/nself-org/cli/internal/telemetry"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// startCmd, its flags, and the RootCmd registration were extracted to
// start_cmd.go (T-P6-E2-W1-S1-T3) for 300-line compliance.

func runStart(cmd *cobra.Command, _ []string) error {
	opts, err := resolveStartOpts(cmd)
	if err != nil {
		return err
	}

	// Telemetry: capture start time and first-run state before execution.
	startTime := time.Now()
	firstRun := isFirstStart()

	// Telemetry: emit start_attempt immediately (opt-in only).
	if telemetry.IsOptedIn() {
		telemetry.Send("start_attempt", map[string]any{
			"first_run": firstRun,
		})
	}

	// Telemetry: deferred start_result emits on all exit paths (success or failure).
	var startErr error
	defer func() {
		if !telemetry.IsOptedIn() {
			return
		}
		props := map[string]any{
			"first_run":   firstRun,
			"duration_ms": time.Since(startTime).Milliseconds(),
			"success":     startErr == nil,
		}
		if startErr != nil {
			props["failure_category"] = classifyStartError(startErr)
		}
		telemetry.Send("start_result", props)
	}()

	// ── Plugin lifecycle: dormant banners (read-only — auto-removal is build-only) ──
	// Show warnings for dormant plugins so operators know to renew or rebuild.
	showDormantBannersOnStart(opts.quiet)

	cwd, startErr := os.Getwd()
	if startErr != nil {
		return fmt.Errorf("getting working directory: %w", startErr)
	}

	allowLegacy, _ := cmd.Flags().GetBool("allow-legacy")

	projectDir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		// Before reporting "no project found", check if this is a v0.9 directory.
		// FindNSelfRoot won't find a .env marker in a v0.9 project, so we must
		// probe cwd directly first.
		if count, names := migration.CheckLegacyProject(cwd); count >= migration.DetectionThreshold {
			if allowLegacy {
				ui.Warn(fmt.Sprintf("WARNING: v0.9 project detected (%d artifact(s): %s). Proceeding due to --allow-legacy (not recommended).", count, strings.Join(names, ", ")))
				projectDir = cwd
			} else {
				ui.Error(fmt.Sprintf("v0.9 project detected. Found %d legacy artifact(s): %s", count, strings.Join(names, ", ")))
				fmt.Fprintln(os.Stderr, "Run `nself migrate` first. See https://nself.org/docs/migrate/from-v0.9")
				startErr = fmt.Errorf("v0.9 project detected — run `nself migrate` first")
				return startErr
			}
		} else {
			return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
		}
	}

	// ── v0.9 artifact detection (S60-T02) ────────────────────────────────
	// Extracted to start_checks.go (T-P6-E2-W1-S1-T3).
	if err := checkLegacyProjectGate(projectDir, allowLegacy); err != nil {
		startErr = err
		return startErr
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	lifecycle.TrapSignals(ctx, cancel, func() {}, 10*time.Second, os.Exit)

	totalSteps := 7
	currentStep := 0

	ui.CommandHeader("nSelf Start", "Booting your stack")

	// ── Docker preflight (first check, before anything else) ─────────
	if err := checkDockerAvailable(ctx); err != nil {
		return err
	}

	// ── AI auto-install (T-05-05) + Auto-build detection ──────────────
	// Both extracted to start_checks.go (T-P6-E2-W1-S1-T3). Auto-build runs
	// BEFORE the docker-compose.yml check because build creates it.
	autoInstallAIIfNeeded(ctx)

	if !opts.skipBuild {
		if err := autoRebuildIfNeeded(projectDir, opts); !entitlement.StartMayContinue(err, os.Stderr) {
			return err
		}
	}

	// ── Step 1: Validate docker-compose.yml ──────────────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Checking docker-compose.yml")

	if err := checkComposeFileExists(projectDir); err != nil {
		return err
	}
	ui.Success("docker-compose.yml found")

	// ── Step 2: Load configuration ───────────────────────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Loading configuration")

	// Config load/validate + env-overrides extracted to start_config.go
	// (T-P6-E2-W1-S1-T3). opts is mutated in place (skipHealthChecks/timeout).
	cfg, err := loadAndValidateStartConfig(cmd, &opts, projectDir)
	if err != nil {
		return err
	}

	// DNS/BASE_DOMAIN/auto-trust/license/cert checks extracted to
	// start_config.go (T-P6-E2-W1-S1-T3).
	runPostConfigChecks(ctx, cmd, cfg, projectDir, opts.verbose)

	// Compose manifest read + --skip-plugins filter extracted to
	// start_config.go (T-P6-E2-W1-S1-T3).
	composeFiles := loadStartComposeFiles(projectDir, opts)
	if !opts.skipPlugins {
		entitlement.RefreshForStart(ctx, projectDir, os.Stderr)
	}

	// ── Step 3: Port availability check ──────────────────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Checking port availability")

	// Extracted to start_ports.go (T-P6-E2-W1-S1-T3).
	if err := checkStartPorts(ctx, opts, projectDir, composeFiles); err != nil {
		return err
	}

	// ── Step 4: Start PostgreSQL (Phase 1) ──────────────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Starting PostgreSQL")

	// Steps 4-6 extracted to start_containers.go (T-P6-E2-W1-S1-T3). The
	// embedded-PG cleanup func (non-nil only on that path) must be deferred
	// here, from runStart's own scope, to match the original's
	// `defer epgCleanup()` lifetime (held until runStart returns).
	compose, pgCleanup, err := startPostgresPhase(ctx, opts, cfg, projectDir, composeFiles)
	if err != nil {
		return err
	}
	if pgCleanup != nil {
		defer pgCleanup()
	}

	// ── Step 5: Database initialization (Phase 2) ───────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Initializing database")

	if err := initializeDatabasePhase(ctx, cfg, opts); err != nil {
		return err
	}

	// ── Step 6: Start remaining services (Phase 3) ──────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Starting remaining services")

	if err := startRemainingServicesPhase(ctx, compose, projectDir, cfg); err != nil {
		return err
	}

	// ── Health auto-restart (--watch) ────────────────────────────────
	// Start the Restarter in a background goroutine. It polls running containers
	// and automatically restarts any that become unhealthy, up to MaxAttempts.
	if opts.watch {
		restartPolicy := health.DefaultRestartPolicy()
		dockerClient := health.NewShellDockerClient(cfg.ProjectName)
		restarter := health.NewRestarter(dockerClient, restartPolicy)
		if err := restarter.Start(ctx); err != nil {
			ui.Warn(fmt.Sprintf("Health auto-restart could not start: %v", err))
		} else {
			ui.Info(fmt.Sprintf("Health auto-restart enabled (poll: %s, max attempts: %d)",
				restartPolicy.PollInterval, restartPolicy.MaxAttempts))
		}
	}

	// ── Step 7: Health checks ────────────────────────────────────────
	currentStep++
	ui.Step(currentStep, totalSteps, "Running health checks")

	if opts.skipDBInit {
		// CI/E2E mode: wait only for the backend triad (postgres, hasura,
		// hasura-auth) to be healthy, then exit 0. Migrations and seed are
		// intentionally absent — the test suite manages schema state itself.
		if waitErr := waitCIReady(ctx, cfg, projectDir, opts.timeout, opts.verbose); waitErr != nil {
			startErr = fmt.Errorf("CI readiness gate failed: %w", waitErr)
			return startErr
		}
	} else if opts.skipHealthChecks {
		ui.Warn("Health checks skipped (--skip-health-checks)")
	} else {
		requiredPct := 80
		if opts.quick {
			requiredPct = 60
		}
		if cfg.HealthCheckRequired > 0 && !opts.quick {
			requiredPct = cfg.HealthCheckRequired
		}

		report := runHealthCheckLoop(ctx, cfg, projectDir, opts.timeout, requiredPct, opts.verbose)
		_ = report // Final report used inline by the loop for display.
	}

	// ── Hasura metadata apply (FIX-CLI-3) ─────────────────────────────
	// Skipped for --skip-db-init (CI/E2E mode): the test suite manages
	// schema/metadata state itself, matching the migrations/seed skip above.
	if !opts.skipDBInit {
		if err := hasura.ApplyIfPresent(ctx, cfg, projectDir); err != nil {
			startErr = err
			return startErr
		}
	}

	// ── MeiliSearch index warm-up ────────────────────────────────────
	// Runs asynchronously so it never delays the startup sequence.
	// Activated only when MEILISEARCH_WARMUP_QUERIES is set and the
	// Search engine is meilisearch.
	if cfg.Search.Enabled && strings.EqualFold(cfg.Search.Engine, "meilisearch") {
		if warmupQueries := search.QueriesFromEnv(); len(warmupQueries) > 0 {
			go func() {
				warmupCfg := search.WarmupConfig{
					Host:        "localhost",
					Port:        cfg.Search.Port,
					MasterKey:   cfg.Search.MeiliSearch.MasterKey,
					IndexPrefix: cfg.Search.IndexPrefix,
					Queries:     warmupQueries,
				}
				result, _ := search.Warmup(context.Background(), warmupCfg)
				if opts.verbose {
					ui.Dimmed(fmt.Sprintf("  MeiliSearch warm-up: %d/%d queries succeeded",
						result.QueriesSucceeded, result.QueriesAttempted))
				}
			}()
		}
	}

	// ── Display URLs ─────────────────────────────────────────────────
	ui.Separator()

	// URL list: localhost:<port> endpoints for default local domains
	// (local.nself.org needs DNS/hosts setup and 502s on a fresh machine),
	// nginx-routed domain URLs for custom domains. See start_urls.go.
	ui.SummaryBox("nSelf Stack Running", stackURLs(cfg))

	if opts.debug {
		ui.Section("Debug Info")
		ui.Dimmed(fmt.Sprintf("  Project dir:  %s", projectDir))
		ui.Dimmed(fmt.Sprintf("  Project name: %s", cfg.ProjectName))
		ui.Dimmed(fmt.Sprintf("  Environment:  %s", cfg.Env))
		ui.Dimmed(fmt.Sprintf("  Timeout:      %ds", opts.timeout))
		ui.Dimmed(fmt.Sprintf("  Quick mode:   %v", opts.quick))
		ui.Dimmed(fmt.Sprintf("  Fresh:        %v", opts.fresh))
		ui.Dimmed(fmt.Sprintf("  Clean start:  %v", opts.cleanStart))
	}

	// Telemetry: emit start_success (opt-in only).
	if telemetry.IsOptedIn() {
		telemetry.Send("start_success", map[string]any{
			"first_run":   firstRun,
			"duration_ms": time.Since(startTime).Milliseconds(),
		})
	}

	return nil
}
