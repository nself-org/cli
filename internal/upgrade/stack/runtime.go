package stack

// Purpose: connect stack execution to the existing backup, reconcile and Docker APIs.
// Inputs: a consented project configuration and journal. Outputs: step errors.
// Constraints: construction performs no I/O; no backup or volume is pruned.

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/reconcile"
)

// Runtime owns the adapters for one consented stack update.
type Runtime struct {
	dir    string
	cfg    *config.Config
	stderr io.Writer
}

// Adapter seams preserve the shared implementation while allowing isolated tests.
var runtimeCompute = reconcile.Compute
var runtimeApplyBuild = reconcile.ApplyBuild
var runtimeMigrateUp = database.MigrateUp

// stepTimeout allows slow pulls and migrations without extending every step.
func stepTimeout(name string) time.Duration {
	if name == "pull" || name == "migrate" {
		return 30 * time.Minute
	}
	return 10 * time.Minute
}

// recoveryTimeout keeps restore, restart and row counts bounded by the migration budget.
func recoveryTimeout() time.Duration { return stepTimeout("migrate") }

// NewRuntime prepares adapters without reading or mutating the project.
func NewRuntime(dir string, cfg *config.Config, stderr io.Writer) *Runtime {
	copy := *cfg
	if copy.Backup.Dir == "" {
		copy.Backup.Dir = "backups"
	}
	if !filepath.IsAbs(copy.Backup.Dir) {
		copy.Backup.Dir = filepath.Join(dir, copy.Backup.Dir)
	}
	return &Runtime{dir: dir, cfg: &copy, stderr: stderr}
}

// compose resolves the same manifests and interpolation inputs as start.
func (r *Runtime) compose() (*docker.Compose, error) {
	files, err := build.ReadComposeManifest(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read compose manifest: %w", err)
	}
	for i, p := range files {
		if !filepath.IsAbs(p) {
			files[i] = filepath.Join(r.dir, p)
		}
	}
	c := docker.NewCompose(files...)
	c.EnvFiles = build.ComposeEnvFiles(r.dir)
	return c, nil
}

// Run dispatches one shared-plan stack step; unknown steps fail closed.
func (r *Runtime) Run(ctx context.Context, name string, j *Journal) error {
	ctx, cancel := context.WithTimeout(ctx, stepTimeout(name))
	defer cancel()
	switch name {
	case "build":
		return r.build(ctx)
	case "migrate":
		_, err := runtimeMigrateUp(ctx, r.cfg, "")
		return err
	case "readiness":
		if err := r.readiness(ctx); err != nil {
			return err
		}
		return r.trackTables(ctx)
	case "health":
		return r.health(ctx)
	case "pull", "start":
		c, err := r.compose()
		if err != nil {
			return err
		}
		if name == "pull" {
			return c.ComposePullDiagnosed(ctx, r.dir)
		}
		return c.ComposeUp(ctx, r.dir)
	default:
		return fmt.Errorf("unsupported stack step %q", name)
	}
}

// build previews production-class changes and applies the already authorized build.
func (r *Runtime) build(ctx context.Context) error {
	req := reconcile.Request{ProjectDir: r.dir, Command: reconcile.CmdBuild, Stderr: r.stderr}
	if reconcile.EnvClass(r.cfg.Env) == reconcile.ClassProd {
		p, err := runtimeCompute(ctx, req)
		if err != nil {
			return fmt.Errorf("preview build: %w", err)
		}
		if r.stderr != nil {
			if err := reconcile.RenderHuman(r.stderr, *p); err != nil {
				return fmt.Errorf("render build preview: %w", err)
			}
		}
	}
	_, _, err := runtimeApplyBuild(ctx, req, reconcile.ApplyOptions{Yes: true})
	if err != nil {
		return fmt.Errorf("apply build: %w", err)
	}
	return nil
}

// CountRows records exact database counts with the existing backup count oracle.
func (r *Runtime) CountRows(ctx context.Context) (map[string]int64, error) { return r.countRows(ctx) }
