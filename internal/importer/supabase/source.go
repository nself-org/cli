// Package supabase registers the Supabase importer.
// Purpose: snapshot a Supabase project (direct database connection), its auth users with their
// bcrypt hashes and its storage objects into a portable bundle; row level security policies
// become Hasura permissions where they match `auth.uid() = <column>` and are reported otherwise.
// Inputs: --db-url-env, --api-url or --project-ref, --service-key-env (all secrets by variable
// name). Outputs: bundle and detection. Constraints: no URL, password or key in errors or reports.
package supabase

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/postgres"
)

type Source struct{}

func init()                 { importer.Register(Source{}) }
func (Source) Name() string { return "supabase" }
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{
		{Name: "db-url-env", Usage: "Environment variable containing the Supabase direct database URL", EnvVarName: true},
		{Name: "api-url", Usage: "Supabase API URL for storage (default https://<project-ref>.supabase.co)"},
		{Name: "project-ref", Usage: "Supabase project reference, used to build the default API URL"},
		{Name: "service-key-env", Usage: "Environment variable containing the service_role key (storage API)", EnvVarName: true},
	}
}

// ownedSchemas are the schemas Supabase manages; they are never dumped. The Supabase roles
// (anon, authenticated, service_role) are never recreated: the dump uses --no-owner --no-privileges.
var ownedSchemas = []string{"_realtime", "_analytics", "_supavisor", "cron", "pgsodium_masks", "pgmq", "pgtle", "dbdev", "auth", "storage", "realtime", "supabase_functions", "extensions", "graphql", "graphql_public", "pgbouncer", "pgsodium", "vault", "net", "supabase_migrations", "information_schema", "hdb_catalog", "nself_ops"}

func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	db, err := dial(ctx, in)
	if err != nil {
		return importer.Detection{}, err
	}
	defer func() { _ = db.Close(ctx) }()
	return detect(ctx, db, in)
}

func dial(ctx context.Context, in importer.Input) (*pgx.Conn, error) {
	value, err := in.Secret("db-url-env")
	if err != nil {
		return nil, err
	}
	return postgres.Dial(ctx, value)
}

func detect(ctx context.Context, db *pgx.Conn, in importer.Input) (importer.Detection, error) {
	d := importer.Detection{Kind: "supabase", Notes: []string{}}
	var err error
	if d.Version, err = postgres.Version(ctx, db, in.Target); err != nil {
		return d, err
	}
	var gotrue bool
	if db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='auth' AND table_name='users' AND column_name='encrypted_password')`).Scan(&gotrue) != nil {
		return d, errs.New("E517", "source auth lookup failed")
	}
	if !gotrue {
		return d, errs.New("E517", "source is not a Supabase database (auth.users with encrypted_password is missing)")
	}
	rows, err := db.Query(ctx, `SELECT nspname::text FROM pg_namespace WHERE left(nspname,3) <> 'pg_' AND left(nspname,3) <> 'np_' AND NOT nspname = ANY($1) ORDER BY nspname`, ownedSchemas)
	if err != nil {
		return d, errs.New("E517", "source schema lookup failed")
	}
	defer rows.Close()
	for rows.Next() {
		var schema string
		if rows.Scan(&schema) != nil {
			return d, errs.New("E517", "source schema lookup failed")
		}
		d.Schemas = append(d.Schemas, schema)
	}
	if rows.Err() != nil {
		return d, errs.New("E517", "source schema lookup failed")
	}
	rows.Close()
	if db.QueryRow(ctx, `SELECT count(*) FROM auth.users`).Scan(&d.SourceUsers) != nil {
		return d, errs.New("E517", "source auth lookup failed")
	}
	objects, err := storageObjects(ctx, db)
	if err != nil {
		return d, err
	}
	if objects > 0 {
		d.Notes = append(d.Notes, "source has storage objects; the storage API is required")
	}
	return d, nil
}

// storageObjects counts storage.objects (0 when the table is absent).
func storageObjects(ctx context.Context, db *pgx.Conn) (int64, error) {
	var n int64
	if db.QueryRow(ctx, `SELECT CASE WHEN to_regclass('storage.objects') IS NULL THEN 0 ELSE (SELECT count(*) FROM storage.objects) END`).Scan(&n) != nil {
		return 0, errs.New("E517", "source storage lookup failed")
	}
	return n, nil
}
