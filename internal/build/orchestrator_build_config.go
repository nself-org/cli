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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginx"
	"github.com/nself-org/cli/internal/nginx/routemodel"
)

// loadValidateConfig runs Steps 1-4 of Build(). See file header for the
// (*BuildResult, error) contract.
func (st *buildState) loadValidateConfig() (*BuildResult, error) {
	st.ensureSeam()
	// ── Step 1: Load config via env cascade ─────────────────────────
	var err error
	repin := pinCompatEnv() // a project file must not toggle NSELF_V15
	st.cfg, err = config.LoadWithOptions(st.workdir, config.LoadOptions{RemoteDeploy: st.opts.RemoteDeploy})
	repin()
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
		if err := persistGeneratedSecretsFx(st.workdir, st.cfg, st.fx, st.sink); err != nil {
			return nil, fmt.Errorf("persisting generated secrets: %w", err)
		}

		// Fix permissions on .env files — ensure they are owner-only (0600).
		for _, envFile := range []string{".env", ".env.local", ".env.secrets", ".env.computed"} {
			if err := ensureEnvFilePermissions(st.sink, filepath.Join(st.workdir, envFile)); err != nil {
				return nil, fmt.Errorf("fixing env file permissions: %w", err)
			}
		}
	}

	// compat.V15(P7-DEPL-01): legacy duplicate errors -> E055 with both route IDs
	v15 := compat.V15()
	// ── Step 2: Validate config ─────────────────────────────────────
	if err := config.Validate(st.cfg); err != nil {
		if v15 && strings.Contains(err.Error(), "[duplicate-routes]") && !strings.Contains(err.Error(), "\n") {
			m, buildErr := st.modelRoutes(nginx.NewGenerator(st.cfg, st.workdir).HasSSL(), nil)
			if buildErr != nil {
				return nil, buildErr
			}
			_, duplicateErr := routemodel.Validate(m, true)
			if duplicateErr != nil {
				return nil, errs.New("E055", strings.TrimPrefix(duplicateErr.Error(), "E055 "))
			}
		}
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// ── Step 2.5: Preflight — nginx domain conflict check ────────────
	st.routes, err = st.modelRoutes(nginx.NewGenerator(st.cfg, st.workdir).HasSSL(), nil)
	if err != nil {
		return nil, err
	}
	warnings, err := routemodel.Validate(st.routes, v15)
	if err != nil {
		if v15 {
			return nil, errs.New("E055", strings.TrimPrefix(err.Error(), "E055 "))
		}
		return nil, err
	}
	for _, warning := range warnings {
		slog.Warn(warning)
	}

	// ── Step 3: If --check, return after validation ─────────────────
	if st.opts.Check {
		return &BuildResult{
			ProjectName: st.cfg.ProjectName,
			Duration:    time.Since(st.start),
		}, nil
	}

	// ── Step 4: Check cache (skip if not --force and cache fresh) ───
	// Plan mode always renders: the cache answers "is the disk fresh", and a
	// plan must show what apply would write.
	if !st.opts.Force && !st.planning() {
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
		if _, err := os.Stat(filepath.Join(st.workdir, ".nself", "generated", "routes.json")); os.IsNotExist(err) {
			needsRebuild = true
		}
		if !needsRebuild {
			// A held write (confirmed render) must never end here with planned
			// files outstanding: reconcile.Apply forces a rebuild for a
			// non-empty plan; this is the backstop.
			if es, ok := st.sink.(*expectSink); ok {
				if err := es.verify(); err != nil {
					return nil, err
				}
			}
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

// ensureEnvFilePermissions fixes an existing env file to 0600 when it is more
// permissive, through the sink (setup.EnsureEnvFilePermissions semantics: a
// missing file is a no-op). Plan mode records the mode change, not applies it.
func ensureEnvFilePermissions(sink Sink, path string) error {
	info, err := sink.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat env file %s: %w", path, err)
	}
	if info.Mode().Perm() != 0600 {
		if err := sink.Chmod(path, 0600); err != nil {
			return fmt.Errorf("chmod env file %s: %w", path, err)
		}
	}
	return nil
}
