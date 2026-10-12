// Package exporter writes a read-only, snapshot-consistent portable bundle.
package exporter

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/portable"
	"github.com/nself-org/cli/internal/version"
)

// Options describes an export. FreeSpace is a test seam, never an env override.
// SecretValues and Keys come from the resolved environment, not raw env files.
type Options struct {
	Out          string
	Schemas      []string
	NoStorage    bool
	SecretValues []string
	Keys         []string
	ProjectDir   string
	FreeSpace    func(string) (uint64, error)
}

// Summary is the command's envelope data; it never includes credentials.
type Summary struct {
	Out         string `json:"out"`
	Tables      int    `json:"tables"`
	Users       int    `json:"users"`
	Objects     int    `json:"objects"`
	StorageNote string `json:"storage_note"`
}

// Purpose: preflight, extract, scrub and publish a bundle.
// Inputs: loaded project config and options. Outputs: counts or a safe error.
// Constraints: no source writes; temporary output is removed on every failure.
func Export(ctx context.Context, cfg *config.Config, opts Options) (Summary, error) {
	if cfg == nil || opts.Out == "" {
		return Summary{}, errors.New("export: config and --out are required")
	}
	p, err := prepareExport(ctx, cfg, opts)
	if err != nil {
		return Summary{}, err
	}
	defer p.db.close(ctx)
	stage, err := os.MkdirTemp(filepath.Dir(p.out), ".nself-export-")
	if err != nil {
		return Summary{}, safeFailure("create stage")
	}
	defer func() { _ = os.RemoveAll(stage) }()
	w, err := portable.NewWriter(stage)
	if err != nil {
		return Summary{}, safeFailure("create bundle")
	}
	m, err := writeBundle(ctx, cfg, p, w, opts.Keys)
	if err != nil {
		return Summary{}, err
	}
	if err = scrubBundle(ctx, cfg, stage, m, opts.SecretValues); err != nil {
		return Summary{}, err
	}
	if err = publishBundle(stage, p.out); err != nil {
		return Summary{}, err
	}
	return Summary{Out: p.out, Tables: len(m.DB.Tables), Users: m.Auth.Users, Objects: len(m.Storage.Objects), StorageNote: p.note}, nil
}

// Purpose: finish all bundle members while the database snapshot remains live.
// Inputs: prepared read plan and writer. Outputs: finalized manifest.
// Constraints: errors withhold source details; publication belongs to Export.
func writeBundle(ctx context.Context, cfg *config.Config, p exportPlan, w *portable.Writer, keys []string) (portable.Manifest, error) {
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: version.GetVersion(), Source: "nself"}, Postgres: p.db.info, DB: portable.DB{Schemas: p.db.schemas}, Exemptions: []portable.Exemption{}, SourceCounts: []portable.SourceCount{}, Compat: []string{}}
	if err := p.db.write(ctx, cfg, w, &m); err != nil {
		return m, err
	}
	if err := writeAuth(ctx, p.db.tx, w, &m); err != nil {
		return m, safeFailure("read auth")
	}
	if err := p.store.write(ctx, w, &m); err != nil {
		return m, safeFailure("read storage")
	}
	if err := writeMetadata(ctx, cfg, w, &m, keys); err != nil {
		return m, safeFailure("read metadata")
	}
	if err := p.db.tx.Commit(ctx); err != nil {
		return m, safeFailure("finish snapshot")
	}
	m, err := w.Finish(m)
	if err != nil {
		return m, safeFailure("finish manifest")
	}
	return m, nil
}

// Purpose: publish scrubbed output with a final occupancy check.
// Inputs: owned private stage and destination. Outputs: rename or safe refusal.
// Constraints: never overwrite a file, symlink or non-empty directory.
func publishBundle(stage, out string) error {
	if err := emptyOutput(out); err != nil {
		return err
	}
	// A directory rename cannot replace a file or a non-empty directory. Avoid
	// unlinking a destination that could have changed since the occupancy read.
	if err := renameBundle(stage, out); err != nil {
		return safeFailure("publish bundle")
	}
	return nil
}

func safeFailure(step string) error {
	return errors.New("export: " + step + " failed (source details withheld)")
}
