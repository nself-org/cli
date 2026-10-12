package exporter

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

type exportPlan struct {
	out   string
	db    *snapshot
	store storagePlan
	note  string
}

// Purpose: prepare a read plan without writing output. Inputs: export options.
// Outputs: open snapshot and enumerated objects. Constraints: failed preflight
// closes the snapshot and never creates a stage directory.
func prepareExport(ctx context.Context, cfg *config.Config, opts Options) (exportPlan, error) {
	var p exportPlan
	var err error
	if hit, err := scan(strings.NewReader(opts.Out), secretPatterns(opts.SecretValues)); hit || err != nil {
		return p, safeFailure("output path contains a configured secret")
	}
	p.out, err = filepath.Abs(opts.Out)
	if err != nil {
		return p, safeFailure("resolve output")
	}
	if err = emptyOutput(p.out); err != nil {
		return p, err
	}
	if err = started(ctx, cfg, opts.ProjectDir); err != nil {
		return p, err
	}
	p.db, err = openSnapshot(ctx, cfg, opts.Schemas)
	if err != nil {
		return p, err
	}
	ok := false
	defer func() {
		if !ok {
			p.db.close(ctx)
		}
	}()
	p.store, p.note, err = listStorage(ctx, cfg, opts.NoStorage)
	if err != nil {
		return p, safeFailure("list storage")
	}
	if err = diskCheck(p.out, p.db.bytes, p.store.bytes, opts.FreeSpace); err != nil {
		return p, err
	}
	ok = true
	return p, nil
}

// Purpose: refuse occupied outputs before any writes. Inputs: absolute path.
// Outputs: E527 or nil. Constraints: symlinks are never accepted.
func emptyOutput(out string) error {
	fi, err := os.Lstat(out)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return safeFailure("inspect output")
	}
	if !fi.IsDir() {
		return errs.New("E527", "")
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		return safeFailure("inspect output")
	}
	if len(entries) != 0 {
		return errs.New("E527", "")
	}
	return nil
}

// Purpose: verify a started project's postgres via the shared health funnel.
// Inputs: project directory. Outputs: refusal or nil. Constraints: read-only.
func started(ctx context.Context, cfg *config.Config, dir string) error {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return safeFailure("resolve project")
		}
	}
	c := docker.NewCompose(filepath.Join(dir, "docker-compose.yml"))
	c.EnvFiles = []string{filepath.Join(dir, ".nself", "compose.env")}
	states, err := c.ComposeServiceStates(ctx, dir, "postgres")
	if err != nil || len(states) == 0 {
		return errors.New("export: project must be started")
	}
	for _, s := range states {
		if s.State != "running" {
			return errors.New("export: project must be started")
		}
	}
	err = docker.HealthGate(ctx, docker.ServiceSpec{Name: "postgres", HasHealthcheck: true}, func(ctx context.Context) ([]docker.ContainerState, error) {
		return c.ComposeServiceStates(ctx, dir, "postgres")
	}, nil)
	if err != nil {
		return safeFailure("postgres health gate")
	}
	return nil
}

// Purpose: check free bytes before creating stage. Inputs: relation/object sizes.
// Outputs: E525 or nil. Constraints: arithmetic overflow fails closed.
func diskCheck(out string, relations, objects uint64, probe func(string) (uint64, error)) error {
	if relations > (math.MaxUint64-objects)/6*5 {
		return errs.New("E525", "")
	}
	estimate := relations + (relations+4)/5
	if estimate > math.MaxUint64-objects {
		return errs.New("E525", "")
	}
	estimate += objects
	if probe == nil {
		probe = freeSpace
	}
	path := filepath.Dir(out)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			return safeFailure("probe disk")
		}
		path = parent
	}
	free, err := probe(path)
	if err != nil {
		return safeFailure("probe disk")
	}
	if free < estimate {
		return errs.New("E525", "")
	}
	return nil
}
