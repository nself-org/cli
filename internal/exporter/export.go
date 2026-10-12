// Package exporter writes a read-only, snapshot-consistent portable bundle.
package exporter

import (
	"context"
	"errors"
	"io"
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
	// Warn receives non-secret preflight warnings; nil means os.Stderr.
	Warn io.Writer
}

// stagePrefix names the private scratch directory next to --out.
const stagePrefix = ".nself-export-"

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
	stage, err := os.MkdirTemp(filepath.Dir(p.out), stagePrefix)
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

// Purpose: finish all bundle members; the snapshot lives only as long as the
// reads that need it. Inputs: prepared read plan and writer. Outputs: finalized
// manifest. Constraints: errors withhold source details; publication belongs to Export.
func writeBundle(ctx context.Context, cfg *config.Config, p exportPlan, w *portable.Writer, keys []string) (portable.Manifest, error) {
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: version.GetVersion(), Source: "nself"}, Postgres: p.db.info, DB: portable.DB{Schemas: p.db.schemas}, Exemptions: []portable.Exemption{}, SourceCounts: []portable.SourceCount{}, Compat: []string{}}
	err := runBundleSteps(bundleSteps{
		database: func() error { return p.db.write(ctx, cfg, w, &m) },
		auth:     func() error { return writeAuth(ctx, p.db.tx, w, &m) },
		commit:   func() error { return p.db.tx.Commit(ctx) },
		storage:  func() error { return p.store.write(ctx, w, &m) },
		metadata: func() error { return writeMetadata(ctx, cfg, w, &m, keys) },
	})
	if err != nil {
		return m, err
	}
	m, err = w.Finish(m)
	if err != nil {
		return m, safeFailure("finish manifest")
	}
	return m, nil
}

// bundleSteps are the ordered reads of one export. database and auth need the
// snapshot; storage and metadata do not.
type bundleSteps struct {
	database, auth, commit, storage, metadata func() error
}

// Purpose: run the reads in the one safe order. Inputs: the steps.
// Outputs: the first failure as a withheld-details error.
// Constraints: the read-only snapshot transaction ends right after the last
// step that uses it (table hashes, pg_dump with --snapshot, auth). Left open
// it would hold ACCESS SHARE on every exported table through the object
// download and the Hasura call, queueing any concurrent ALTER TABLE and with
// it every application query on that table, and would pin vacuum's xmin.
func runBundleSteps(s bundleSteps) error {
	if err := s.database(); err != nil {
		return err
	}
	if err := s.auth(); err != nil {
		return safeFailure("read auth")
	}
	if err := s.commit(); err != nil {
		return safeFailure("finish snapshot")
	}
	if err := s.storage(); err != nil {
		return safeFailure("read storage")
	}
	if err := s.metadata(); err != nil {
		return safeFailure("read metadata")
	}
	return nil
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
