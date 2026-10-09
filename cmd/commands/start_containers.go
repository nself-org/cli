package commands

// Purpose: start PostgreSQL and the remaining services for nself start.
// Inputs: start options, config, project path and Compose files.
// Outputs: a Compose client, optional embedded-PG cleanup, or an error.
// Constraints: check locked images before up; runStart owns embedded-PG cleanup.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/build"
	composepkg "github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/embedded"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"
)

var newStartCompose = docker.NewCompose
var ensureStartImages = docker.EnsureComposeImages

// startPostgresPhase starts PostgreSQL (Step 4): either the embedded
// pglite/wasmtime runtime (--embedded-pg / NSELF_EMBEDDED_PG=true) or the
// standard Docker postgres container. Returns the *docker.Compose to reuse
// for the remaining-services step, and — on the embedded-PG path only — a
// cleanup func the caller must defer from its own scope (runStart), since
// the original code deferred epgCleanup() directly from runStart.
//
// startEmbeddedPGRuntime is conditionally compiled: the real implementation
// lives in start_embedded_cgo.go (//go:build cgo) and a stub that returns
// an error lives in start_embedded_nocgo.go (//go:build !cgo).
func startPostgresPhase(ctx context.Context, opts startOpts, cfg *config.Config, projectDir string, composeFiles []string) (compose *docker.Compose, cleanup func(), err error) {
	compose = newStartCompose(composeFiles...)
	compose.EnvFiles = build.ComposeEnvFiles(projectDir)

	// ── Embedded PG path (--embedded-pg / NSELF_EMBEDDED_PG=true) ───
	// When embedded PG is requested, boot pglite via wasmtime instead of
	// the Docker postgres container. The WASM module listens on a Unix
	// domain socket; a wire-protocol bridge proxies Hasura's TCP traffic.
	if opts.embeddedPG {
		epgSp := ui.NewSpinner("Starting embedded PostgreSQL (pglite/wasmtime)...")
		epgSp.Start()

		runtimeDir := filepath.Join(projectDir, ".nself", "embedded-pg")
		if mkErr := os.MkdirAll(runtimeDir, 0o700); mkErr != nil {
			epgSp.Fail(fmt.Sprintf("Could not create embedded PG runtime dir: %v", mkErr))
			return compose, nil, fmt.Errorf("embedded-pg: mkdir runtime dir: %w", mkErr)
		}

		wasmPath, fetchErr := embedded.FetchOrCached(ctx, embedded.DefaultPGliteVersion)
		if fetchErr != nil {
			epgSp.Fail(fmt.Sprintf("Failed to fetch pglite WASM: %v", fetchErr))
			ui.UXError(
				"Embedded PG: pglite fetch failed",
				fetchErr.Error(),
				[]string{
					"Check network connectivity to ping.nself.org",
					"Try clearing the cache: rm -rf ~/.nself/cache/pglite",
					"Use standard PostgreSQL instead: remove --embedded-pg flag",
				},
			)
			return compose, nil, fmt.Errorf("embedded-pg: fetch wasm: %w", fetchErr)
		}

		epgCleanup, bridgeSockPath, epgErr := startEmbeddedPGRuntime(ctx, runtimeDir, wasmPath) //nolint:staticcheck // SA4023: linux-only always-error by design, see below
		// SA4023 on linux: startEmbeddedPGRuntime always returns a non-nil error
		// there, because the wasmtime shim does not implement the ~113 Emscripten
		// host imports pglite needs — the failure message below says exactly that.
		// The check is correct and intentional; the runtime is EXPERIMENTAL and
		// fails closed by design. Do not "simplify" this into an unconditional
		// error path: the darwin build can succeed, so the branch is real there.
		if epgErr != nil { //nolint:staticcheck // SA4023: always-true on linux by design, see above
			epgSp.Fail(fmt.Sprintf("Embedded PG failed to start: %v", epgErr))
			ui.UXError(
				"Embedded PG start failed",
				epgErr.Error(),
				[]string{
					"Embedded PostgreSQL (pglite/wasmtime) is EXPERIMENTAL and not yet functional — " +
						"the wasmtime shim does not yet implement the ~113 Emscripten host imports required.",
					"Use standard PostgreSQL instead: omit the --embedded-pg flag (nself start).",
					"Status: tracked in P104 residue; see FEATURES.md for details.",
				},
			)
			return compose, nil, epgErr
		}

		epgSp.Success(fmt.Sprintf("Embedded PostgreSQL ready (pglite v%s / wasmtime v25.x)", embedded.DefaultPGliteVersion))
		ui.Info(fmt.Sprintf("  Socket: %s", bridgeSockPath))
		ui.Info("  Hasura will connect via UDS bridge — no Docker postgres container required")
		return compose, epgCleanup, nil
	}

	// Lock mode checks images before every up and refreshes Compose's files.
	if composepkg.PinningMode() == composepkg.PinningLock {
		refs := make([]docker.LockedImage, 0, len(composepkg.LockedImages()))
		for _, ref := range composepkg.LockedImages() {
			refs = append(refs, docker.LockedImage{Name: ref.Name, Repository: ref.Repository, Version: ref.Version, IndexDigest: ref.IndexDigest, Mirror: ref.Mirror})
		}
		if _, pullErr := ensureStartImages(ctx, projectDir, compose, refs); pullErr != nil {
			return compose, nil, errs.Wrap("E465", "image unavailable: "+pullErr.Error(), pullErr)
		}
		if files, readErr := build.ReadComposeManifest(projectDir); readErr == nil {
			if opts.skipPlugins && len(files) > 0 {
				files = files[:1]
				if _, statErr := os.Stat(filepath.Join(projectDir, docker.ImageOverrideFile)); statErr == nil {
					files = append(files, filepath.Join(projectDir, docker.ImageOverrideFile))
				}
			}
			compose.ComposeFiles = files
		} else {
			return compose, nil, readErr
		}
	}
	// Legacy mode keeps the first-run pull, but now surfaces its error.
	firstRunMarker := filepath.Join(projectDir, ".nself", ".first-run-complete")
	if _, err := os.Stat(firstRunMarker); os.IsNotExist(err) && composepkg.PinningMode() != composepkg.PinningLock {
		if !opts.quiet {
			ui.Info("First run detected — pulling Docker images (this takes 1-3 minutes on slow connections)...")
		}
		donePull := ui.FirstRunProgress(opts.quiet)
		pullCtx, pullCancel := context.WithTimeout(ctx, 10*time.Minute)
		pullErr := compose.ComposePullDiagnosed(pullCtx, projectDir)
		pullCancel()
		donePull()
		if pullErr != nil {
			return compose, nil, errs.Wrap("E465", "first-run image pull failed: "+docker.Diagnose(pullErr.Error()), pullErr)
		}
		// Write the first-run marker so subsequent starts skip this step.
		if mkErr := os.MkdirAll(filepath.Dir(firstRunMarker), 0o700); mkErr == nil {
			f, fErr := os.OpenFile(firstRunMarker, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if fErr == nil {
				_ = f.Close()
			}
		}
	}

	// Remove stale hash-prefixed rename-leftover containers (interrupted
	// recreates) so they cannot hold ports or shadow the clean
	// <project>_<service> names (e.g. b6d7..._ntask_hasura, gap #21).
	_ = docker.RunPreStartCleanup(ctx, cfg.ProjectName)

	// Clean start: remove all containers first.
	if opts.cleanStart {
		sp := ui.NewSpinner("Removing existing containers...")
		sp.Start()
		if err := compose.ComposeDown(ctx, projectDir, docker.DownOptions{
			RemoveOrphans: true,
		}); err != nil {
			sp.Fail(fmt.Sprintf("Cleanup failed: %v", err))
			// Non-fatal: continue with startup.
		} else {
			sp.Success("Existing containers removed")
		}
	}

	// Fresh mode: force recreate by running down first.
	if opts.fresh && !opts.cleanStart {
		sp := ui.NewSpinner("Forcing container recreation...")
		sp.Start()
		if err := compose.ComposeDown(ctx, projectDir, docker.DownOptions{}); err != nil {
			sp.Fail(fmt.Sprintf("Recreation cleanup failed: %v", err))
			// Non-fatal: continue with startup.
		} else {
			sp.Success("Containers cleared for fresh start")
		}
	}

	// Phase 1: Start only postgres so schemas can be created before auth starts.
	sp := ui.NewSpinner("Starting PostgreSQL...")
	sp.Start()

	// Skip ComposeUp for an already healthy postgres unless a clean/fresh
	// start cleared it. Compose may otherwise recreate it after config drift.
	if !opts.cleanStart && !opts.fresh {
		containerName := fmt.Sprintf("%s_postgres", cfg.ProjectName)
		health, healthErr := docker.GetHealthStatus(ctx, containerName)
		if healthErr == nil && postgresAlreadyRunning(health) {
			sp.Success("PostgreSQL container already running and healthy")
			return compose, nil, nil
		}
	}

	if err := compose.ComposeUp(ctx, projectDir, "postgres"); err != nil {
		sp.Fail("Failed to start PostgreSQL")
		ui.UXError(
			"Failed to start PostgreSQL",
			err.Error(),
			[]string{
				"Check Docker is running: docker info",
				"Review logs: nself logs postgres",
				"Try a clean start: nself start --clean-start",
				"Rebuild: nself build --force",
			},
		)
		return compose, nil, fmt.Errorf("compose up postgres: %w", err)
	}
	sp.Success("PostgreSQL container started")

	return compose, nil, nil
}

// postgresAlreadyRunning reports whether a docker.GetHealthStatus result for
// the project's postgres container means `nself start` can safely treat
// PostgreSQL as already up — i.e. skip `docker compose up -d postgres` for
// this run rather than asking Compose to reconcile it. Only "healthy" (the
// container exists, has a healthcheck, and it is passing) qualifies:
// "starting" may still fail its first probe, "unhealthy"/"none"/"not_found"
// all mean the container needs a real compose-up to reach a good state.
func postgresAlreadyRunning(healthStatus string) bool {
	return healthStatus == "healthy"
}

// initializeDatabasePhase runs database initialization (Step 5). Must run
// BEFORE other services start: the auth service requires the auth schema to
// exist in PostgreSQL, so schemas and extensions must be created while only
// postgres is running. When opts.skipDBInit is set (CI/E2E mode) this
// entire step is bypassed: migrations and seed are skipped, and the stack
// boots to a bare schema state.
func initializeDatabasePhase(ctx context.Context, cfg *config.Config, opts startOpts) error {
	if opts.skipDBInit {
		ui.Warn("Database initialization skipped (--skip-db-init / CI mode)")
		return nil
	}

	dbSp := ui.NewSpinner("Waiting for PostgreSQL ready and initializing schemas...")
	dbSp.Start()

	dbCtx, dbCancel := context.WithTimeout(ctx, 90*time.Second)
	defer dbCancel()

	if err := database.InitializeDatabase(dbCtx, cfg); err != nil {
		dbSp.Fail("Database initialization failed")
		// An invalid-identifier failure (e.g. POSTGRES_DB derived from a
		// hyphenated project directory name) is a config problem, not a
		// connectivity/credentials one — the generic hints below actively
		// misdirect for this case, so name the real problem instead.
		hints := []string{
			"Check PostgreSQL logs: nself logs postgres",
			"Verify POSTGRES_USER and POSTGRES_PASSWORD in .env",
			"Try restarting: nself restart",
		}
		if errors.Is(err, database.ErrInvalidIdentifier) {
			hints = []string{
				"The name is not a valid SQL identifier: it must start with a letter or underscore and contain only letters, digits, and underscores",
				"Fix the offending value (commonly POSTGRES_DB) in .env directly",
				"Or regenerate .env: nself init --force --name <valid-name>",
			}
		}
		ui.UXError(
			"Database initialization failed",
			err.Error(),
			hints,
		)
		return fmt.Errorf("database init: %w", err)
	}
	dbSp.Success("Database initialized (schemas, extensions, grants)")
	return nil
}

// startRemainingServicesPhase runs `docker compose up` for the rest of the
// stack (Step 6), then kicks off a best-effort background cleanup of exited
// init containers (meilisearch-init) and zombie containers from interrupted
// starts.
func startRemainingServicesPhase(ctx context.Context, compose *docker.Compose, projectDir string, cfg *config.Config) error {
	svcSp := ui.NewSpinner("Running docker compose up...")
	svcSp.Start()
	if err := compose.ComposeUp(ctx, projectDir); err != nil {
		svcSp.Fail("Docker compose up failed")
		ui.UXError(
			"Failed to start services",
			err.Error(),
			[]string{
				"Check Docker is running: docker info",
				"Review logs: nself logs",
				"Try a clean start: nself start --clean-start",
				"Rebuild: nself build --force",
			},
		)
		return fmt.Errorf("compose up: %w", err)
	}
	svcSp.Success("All services started")

	// Post-start cleanup: remove init containers (meilisearch-init)
	// that completed and are sitting in exited state, plus zombie containers
	// from interrupted starts.
	go func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = docker.RunPostStartCleanup(cleanupCtx, cfg.ProjectName)
	}()

	return nil
}
