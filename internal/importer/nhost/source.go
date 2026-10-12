// Package nhost registers the nhost importer.
// Purpose: move an nhost project (Postgres, Hasura metadata, hasura-auth users with bcrypt,
// hasura-storage objects) into a started nSelf project through the shared bundle pipeline.
// Inputs: env-name flags for the DB URL, admin secret and S3 keys; plain flags for URLs.
// Outputs: a sealed portable bundle with producer source nhost. Constraints: no secret on argv,
// in logs, reports or errors; the postgres source owns the snapshot, this one adds to it.
package nhost

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/hasurameta"
	"github.com/nself-org/cli/internal/importer/postgres"
)

// Source is the nhost importer.
type Source struct{}

func init()                 { importer.Register(Source{}) }
func (Source) Name() string { return "nhost" }

// Flags lists the nhost inputs; the storage flags are required only when the source holds files.
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{
		{Name: "db-url-env", Usage: "Environment variable containing the nhost PostgreSQL URL", EnvVarName: true},
		{Name: "hasura-url", Usage: "Base URL of the nhost Hasura (https, or http on loopback)"},
		{Name: "admin-secret-env", Usage: "Environment variable containing the Hasura admin secret", EnvVarName: true},
		{Name: "storage-endpoint", Usage: "S3 endpoint URL of the hasura-storage bucket (https, or http on loopback)"},
		{Name: "s3-access-key-env", Usage: "Environment variable containing the S3 access key", EnvVarName: true},
		{Name: "s3-secret-env", Usage: "Environment variable containing the S3 secret key", EnvVarName: true},
		{Name: "bucket", Usage: "S3 bucket that hasura-storage writes to"},
	}
}

// pgInput maps the nhost flags onto the postgres source's single flag.
func pgInput(in importer.Input) importer.Input {
	return importer.Input{Values: map[string]string{"from-env": in.Values["db-url-env"]}, Env: in.Env, Target: in.Target}
}

func connect(ctx context.Context, in importer.Input) (*pgx.Conn, error) {
	value, err := in.Secret("db-url-env")
	if err != nil {
		return nil, err
	}
	cfg, err := pgx.ParseConfig(value)
	if err != nil {
		return nil, errs.New("E517", "source connection configuration invalid")
	}
	cfg.ConnectTimeout = 10e9
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, errs.New("E517", "source database unreachable")
	}
	return db, nil
}

// Detect runs the postgres checks, then proves the Hasura admin secret works, reads the Hasura
// version, and refuses a source that holds uploaded files without the S3 flags.
func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	d, err := postgres.Source{}.Detect(ctx, pgInput(in))
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
	if _, err = hasurameta.Export(ctx, base, secret); err != nil {
		return d, err
	}
	d.Kind = "nhost"
	d.Notes = append(keepNotes(d.Notes), "hasura "+version)
	db, err := connect(ctx, in)
	if err != nil {
		return d, err
	}
	defer func() { _ = db.Close(ctx) }()
	files, err := uploadedFiles(ctx, db)
	if err != nil {
		return d, err
	}
	if files > 0 {
		if err = needStorageFlags(in); err != nil {
			return d, err
		}
	}
	return d, nil
}

// keepNotes drops the postgres source's "looks like nhost" hint: this is the nhost source.
func keepNotes(notes []string) []string {
	out := []string{}
	for _, n := range notes {
		if !strings.Contains(n, "looks like nhost") {
			out = append(out, n)
		}
	}
	return out
}
