// Package hasuracloud registers the hasura-cloud importer.
// Purpose: move a Hasura Cloud (or any self-hosted Hasura) project that owns no platform auth:
// its Postgres database through the snapshot path and its metadata through export_metadata.
// Inputs: --db-url-env, --hasura-url, --admin-secret-env. Outputs: a sealed portable bundle with
// producer source hasura-cloud and no users. Constraints: no secret on argv, in logs or errors.
package hasuracloud

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/hasurameta"
	"github.com/nself-org/cli/internal/importer/postgres"
)

// Source is the hasura-cloud importer.
type Source struct{}

const (
	msgAuthSchema   = "source database has an auth schema: use `nself db import nhost` for an nhost project, or `nself db import postgres`"
	msgStorageFiles = "source database holds hasura-storage files, which this source does not move: use `nself db import nhost` with S3 access, or `nself db import postgres` and move the files yourself"
	msgLookup       = "source database lookup failed"
)

func init() {
	importer.AllowMessages(msgAuthSchema, msgStorageFiles, msgLookup)
	importer.Register(Source{})
}

type rower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// ownDatabase refuses a database whose auth or storage data this source would drop without a word:
// the postgres snapshot excludes the auth schema entirely and would dump storage.files rows with no
// objects behind them. Both mean the project is an nhost project (or close enough that the nhost
// source is the right one).
func ownDatabase(ctx context.Context, q rower) error {
	var auth int64
	if q.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'auth'`).Scan(&auth) != nil {
		return errs.New("E517", msgLookup)
	}
	if auth > 0 {
		return errs.New("E517", msgAuthSchema)
	}
	var present bool
	if q.QueryRow(ctx, `SELECT to_regclass('storage.files') IS NOT NULL`).Scan(&present) != nil {
		return errs.New("E517", msgLookup)
	}
	if !present {
		return nil
	}
	var rows bool
	if q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM storage.files)`).Scan(&rows) != nil {
		return errs.New("E517", msgLookup)
	}
	if rows {
		return errs.New("E517", msgStorageFiles)
	}
	return nil
}
func (Source) Name() string { return "hasura-cloud" }

// Flags lists the hasura-cloud inputs.
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{
		{Name: "db-url-env", Usage: "Environment variable containing the PostgreSQL URL of the database Hasura is connected to", EnvVarName: true},
		{Name: "hasura-url", Usage: "Base URL of the Hasura project (https, or http on loopback)"},
		{Name: "admin-secret-env", Usage: "Environment variable containing the Hasura admin secret", EnvVarName: true},
	}
}

func pgInput(in importer.Input) importer.Input {
	return importer.Input{Values: map[string]string{"from-env": in.Values["db-url-env"]}, Env: in.Env, Target: in.Target}
}

// Detect runs the postgres checks, proves the admin secret and reads the Hasura version. A
// database that holds an auth schema or hasura-storage files is refused (see ownDatabase).
func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	d, err := postgres.Source{}.Detect(ctx, pgInput(in))
	if err != nil {
		return d, err
	}
	db, err := hasurameta.Connect(ctx, in)
	if err != nil {
		return d, err
	}
	err = ownDatabase(ctx, db)
	_ = db.Close(ctx)
	if err != nil {
		return d, err
	}
	base, err := hasurameta.Base(in.Values["hasura-url"])
	if err != nil {
		return d, err
	}
	secret, err := in.Secret("admin-secret-env")
	if err != nil {
		return d, err
	}
	version, err := hasurameta.Version(ctx, base)
	if err != nil {
		return d, err
	}
	raw, err := hasurameta.Export(ctx, base, secret)
	if err != nil {
		return d, err
	}
	if err = hasurameta.Check(raw); err != nil {
		return d, err
	}
	d.Kind = "hasura-cloud"
	d.Notes = append(d.Notes, "hasura "+version)
	return d, nil
}

// Extract writes the bundle at stage: the postgres snapshot plus hasura/metadata.json.
func (Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	base, err := hasurameta.Base(in.Values["hasura-url"])
	if err != nil {
		return importer.Extracted{}, err
	}
	secret, err := in.Secret("admin-secret-env")
	if err != nil {
		return importer.Extracted{}, err
	}
	return hasurameta.Extract(ctx, postgres.Source{}, pgInput(in), stage, "hasura-cloud", base, secret, nil)
}
