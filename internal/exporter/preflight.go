package exporter

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// The tool containers run as the caller's uid:gid; Windows has none.
	if currentUID() < 0 {
		return p, errors.New("export: db export supports Linux and macOS only (this platform has no numeric user id for the tool containers)")
	}
	p.out, err = filepath.Abs(opts.Out)
	if err != nil {
		return p, safeFailure("resolve output")
	}
	if hit, err := scan(strings.NewReader(p.out), secretPatterns(opts.SecretValues)); hit || err != nil {
		return p, safeFailure("output path contains a configured secret")
	}
	if fi, serr := os.Stat(filepath.Dir(p.out)); serr != nil || !fi.IsDir() {
		return p, errors.New("export: the parent directory of --out must exist before export starts")
	}
	if err = emptyOutput(p.out); err != nil {
		return p, err
	}
	warnStaleStages(opts.Warn, filepath.Dir(p.out))
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
// The stage holds the archive (about 1.2 times the relation bytes, counting
// the tool's output file before it is copied into the bundle) and, during the
// secret scan, a full plain-SQL rendering of that archive. COPY text and hex
// bytea can be larger than the relations, so the scan allowance is 2.5 times
// the relation bytes. Objects are copied once.
func diskCheck(out string, relations, objects uint64, probe func(string) (uint64, error)) error {
	if relations > math.MaxUint64/6 {
		return errs.New("E525", "")
	}
	estimate, ok := uint64(0), true
	for _, part := range []uint64{relations, (relations + 4) / 5, relations/2*5 + relations%2*3, objects} {
		if estimate > math.MaxUint64-part {
			ok = false
			break
		}
		estimate += part
	}
	if !ok {
		return errs.New("E525", "")
	}
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

// currentUID is a test seam for the unsupported-platform refusal.
var currentUID = os.Getuid

// Purpose: tell the operator about stage directories an interrupted export
// left behind. Inputs: warning writer (stderr when nil) and the --out parent.
// Outputs: one stderr line naming the directories. Constraints: names only,
// never contents or secrets; failures to list are silent (nothing to act on).
func warnStaleStages(w io.Writer, parent string) {
	if w == nil {
		w = os.Stderr
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), stagePrefix) {
			names = append(names, e.Name())
		}
	}
	if len(names) > 0 {
		_, _ = fmt.Fprintf(w, "warning: %s holds unscrubbed export stage directories from an interrupted run (%s); if no export is running, remove them with: rm -rf %s/%s*\n", parent, strings.Join(names, ", "), parent, stagePrefix)
	}
}
