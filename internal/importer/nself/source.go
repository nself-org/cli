// Package nself registers the portable-bundle importer: the re-import half of `nself db export`.
// Purpose: validate a v1 bundle and stage it unchanged so the shared loader and verifier run on it.
// Inputs: the bundle directory (Values["dir"]). Outputs: a staged byte-identical bundle.
// Constraints: the bundle is never modified; errors carry the portable reader's text (member paths and
// manifest values), which the shared run filters before it reaches the user.
package nself

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/importer/postgres"
	"github.com/nself-org/cli/internal/portable"
	"path/filepath"
	"reflect"
)

// Source imports a directory written by `nself db export` (or kept by --keep-stage).
type Source struct{}

func init()                 { importer.Register(Source{}) }
func (Source) Name() string { return "nself" }

// Flags declares the bundle directory. The command takes it as its one positional argument and
// stores it under this name (cmd/commands/db_import_src_nself.go).
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{{Name: "dir", Usage: "Portable bundle directory written by `nself db export` (positional argument)"}}
}

// open reuses this run's verified source reader, or verifies the whole bundle before staging.
// Format or major failures are E515; integrity and reader-directory mismatches are E516.
func open(in importer.Input) (*portable.Reader, error) {
	dir := in.Values["dir"]
	if dir == "" {
		return nil, errs.New("E517", "bundle directory required")
	}
	var r *portable.Reader
	var err error
	if in.VerifiedBundle != nil {
		root, e := filepath.EvalSymlinks(dir)
		if e != nil || root != in.VerifiedBundle.Dir() {
			return nil, errs.New("E516", "bundle changed while it was staged")
		}
		r = in.VerifiedBundle
	} else {
		r, err = portable.Open(dir)
	}
	if err != nil {
		return nil, err
	}
	// compat.sql would run as the database owner before any other check; nself db export never
	// writes one, so a bundle that carries it did not come from the exporter.
	for _, f := range r.Manifest().Files {
		if f.Path == "db/compat.sql" {
			return nil, errs.New("E516", "bundle carries db/compat.sql, which nself db export never writes; import it with the source that wrote it")
		}
	}
	return r, nil
}

// Detect reads the manifest, checks the Postgres major against the target and reports the bundle's
// schemas and user count so --plan runs the same target preflights as the real run.
func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	r, err := open(in)
	if err != nil {
		return importer.Detection{}, err
	}
	m := r.Manifest()
	d := importer.Detection{Kind: "nself", Version: m.Postgres.ServerVersion, Schemas: m.DB.Schemas, SourceUsers: m.Auth.Users, Objects: m.Storage.Objects, Notes: []string{fmt.Sprintf("portable bundle from %s %s (source %s)", m.Producer.Tool, m.Producer.Version, m.Producer.Source)}}
	d.VerifiedBundle = r
	if in.Target == nil {
		return d, nil
	}
	db, err := load.Connect(ctx, in.Target)
	if err != nil {
		return d, err
	}
	var num int
	err = db.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&num)
	_ = db.Close(ctx)
	if err != nil {
		return d, errs.New("E521", "target version unavailable")
	}
	return d, postgres.CheckVersion(m.Postgres.Major, num/10000)
}

// Extract copies the verified bundle into stage byte for byte. A hard link would be refused by the
// reader (members must be singly linked), so every member is copied through a checking reader.
// The staged copy is opened again and its manifest must equal the one verified from the source, so
// a bundle that changes while it is staged is refused (E516) rather than loaded.
func (Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	r, err := open(in)
	if err != nil {
		return importer.Extracted{}, err
	}
	if err = copyBundle(ctx, r, stage); err != nil {
		return importer.Extracted{}, err
	}
	staged, err := portable.Open(stage)
	if err != nil {
		return importer.Extracted{}, err
	}
	if !reflect.DeepEqual(staged.Manifest(), r.Manifest()) {
		return importer.Extracted{}, errs.New("E516", "bundle changed while it was staged")
	}
	return importer.Extracted{Stage: stage, VerifiedBundle: staged}, nil
}
