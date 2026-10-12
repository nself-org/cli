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
	out, err := filepath.Abs(opts.Out)
	if err != nil {
		return Summary{}, errors.New("export: resolve output")
	}
	if err = emptyOutput(out); err != nil {
		return Summary{}, err
	}
	if err = started(ctx, cfg, opts.ProjectDir); err != nil {
		return Summary{}, err
	}
	db, err := openSnapshot(ctx, cfg, opts.Schemas)
	if err != nil {
		return Summary{}, err
	}
	defer db.close(ctx)
	store, note, err := listStorage(ctx, cfg, opts.NoStorage)
	if err != nil {
		return Summary{}, safeFailure("list storage")
	}
	if err = diskCheck(out, db.bytes, store.bytes, opts.FreeSpace); err != nil {
		return Summary{}, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(out), ".nself-export-")
	if err != nil {
		return Summary{}, safeFailure("create stage")
	}
	defer func() { _ = os.RemoveAll(stage) }()
	w, err := portable.NewWriter(stage)
	if err != nil {
		return Summary{}, safeFailure("create bundle")
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: version.GetVersion(), Source: "nself"}, Postgres: db.info, DB: portable.DB{Schemas: db.schemas}, Exemptions: []portable.Exemption{}, SourceCounts: []portable.SourceCount{}, Compat: []string{}}
	if err = db.write(ctx, cfg, w, &m); err != nil {
		return Summary{}, err
	}
	if err = writeAuth(ctx, db.tx, w, &m); err != nil {
		return Summary{}, safeFailure("read auth")
	}
	if err = store.write(ctx, w, &m); err != nil {
		return Summary{}, safeFailure("read storage")
	}
	if err = writeMetadata(ctx, cfg, w, &m, opts.Keys); err != nil {
		return Summary{}, safeFailure("read metadata")
	}
	if err = db.tx.Commit(ctx); err != nil {
		return Summary{}, safeFailure("finish snapshot")
	}
	m, err = w.Finish(m)
	if err != nil {
		return Summary{}, safeFailure("finish manifest")
	}
	if err = scrubBundle(ctx, cfg, stage, m, opts.SecretValues); err != nil {
		return Summary{}, err
	}
	if err = emptyOutput(out); err != nil {
		return Summary{}, err
	}
	if _, err = os.Lstat(out); err == nil {
		if err = os.Remove(out); err != nil {
			return Summary{}, safeFailure("publish bundle")
		}
	}
	if err = os.Rename(stage, out); err != nil {
		return Summary{}, safeFailure("publish bundle")
	}
	return Summary{Out: out, Tables: len(m.DB.Tables), Users: m.Auth.Users, Objects: len(m.Storage.Objects), StorageNote: note}, nil
}

func safeFailure(step string) error {
	return errors.New("export: " + step + " failed (source details withheld)")
}
