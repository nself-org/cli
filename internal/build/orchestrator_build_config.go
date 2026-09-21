package build

// orchestrator_build_config.go — Build() Steps 1-4: load config via the env
// cascade, persist auto-generated secrets, fix .env file permissions,
// validate config, preflight nginx domain-conflict check, and the --check /
// cache-fresh early exits. Split from orchestrator.go (T-P6-E2-W1-S1-T3).
// Inputs:  st.workdir, st.opts (already populated by Build()).
// Outputs: sets st.cfg; returns a non-nil *BuildResult when Build() must
//          return immediately (the --check or cache-fresh path), or an
//          error. Both nil means the caller should continue to the next
//          phase.
// Constraints: pure move, same checks/output/errors/order, no behavior
//              change.

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx"
	"github.com/nself-org/cli/internal/setup"
)

// loadValidateConfig runs Steps 1-4 of Build(). See file header for the
// (*BuildResult, error) contract.
func (st *buildState) loadValidateConfig() (*BuildResult, error) {
	// ── Step 1: Load config via env cascade ─────────────────────────
	var err error
	st.cfg, err = config.Load(st.workdir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// ── Step 1.5: Persist auto-generated secrets to .env.secrets ────
	// Skipped entirely for --check. "--check" means "validate only" — it
	// must be a pure read. Before this fix it ran unconditionally, so a
	// `nself build --check` against production wrote freshly-generated
	// PLUGIN_INTERNAL_SECRET/NOTIFY_INTERNAL_SECRET values into
	// backend/.env.secrets even though nothing downstream of --check ever
	// reads them — a validate-only invocation must never mutate the
	// project it is inspecting.
	if !st.opts.Check {
		if err := persistGeneratedSecrets(st.workdir, st.cfg); err != nil {
			return nil, fmt.Errorf("persisting generated secrets: %w", err)
		}

		// Fix permissions on .env files — ensure they are owner-only (0600).
		for _, envFile := range []string{".env", ".env.local", ".env.secrets", ".env.computed"} {
			if err := setup.EnsureEnvFilePermissions(filepath.Join(st.workdir, envFile)); err != nil {
				return nil, fmt.Errorf("fixing env file permissions: %w", err)
			}
		}
	}

	// ── Step 2: Validate config ─────────────────────────────────────
	if err := config.Validate(st.cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// ── Step 2.5: Preflight — nginx domain conflict check ────────────
	routes := buildNginxRoutes(st.cfg)
	if conflict, pairs := nginx.HasDomainConflict(routes); conflict {
		msg := "nginx domain conflict detected:\n"
		for _, p := range pairs {
			msg += "  " + p + "\n"
		}
		return nil, fmt.Errorf("%s", msg)
	}

	// ── Step 3: If --check, return after validation ─────────────────
	if st.opts.Check {
		return &BuildResult{
			ProjectName: st.cfg.ProjectName,
			Duration:    time.Since(st.start),
		}, nil
	}

	// ── Step 4: Check cache (skip if not --force and cache fresh) ───
	if !st.opts.Force {
		needsRebuild, err := NeedsRebuild(st.workdir)
		if err != nil {
			return nil, fmt.Errorf("checking build cache: %w", err)
		}
		// A profile switch changes which services are emitted but touches
		// neither .env's mtime nor the CLI version, so the mtime/version check
		// above cannot see it.
		if ProfileChanged(st.workdir, string(st.opts.Profile)) {
			needsRebuild = true
		}
		if !needsRebuild {
			return &BuildResult{
				ProjectName: st.cfg.ProjectName,
				ComposeFile: filepath.Join(st.workdir, "docker-compose.yml"),
				NginxConfig: filepath.Join(st.workdir, "nginx", "nginx.conf"),
				Duration:    time.Since(st.start),
			}, nil
		}
	}

	return nil, nil
}
