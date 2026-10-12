// Package postgres registers the base snapshot importer.
// Purpose: detect a Postgres source. Inputs: --from-env variable name.
// Outputs: version and user schemas. Constraints: no DSN or passwords in errors.
package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/load"
)

type Source struct{}

func init()                 { importer.Register(Source{}) }
func (Source) Name() string { return "postgres" }
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{{Name: "from-env", Usage: "Environment variable containing the source PostgreSQL URL", EnvVarName: true}}
}
func connect(ctx context.Context, in importer.Input) (*pgx.Conn, error) {
	value, err := in.Secret("from-env")
	if err != nil {
		return nil, err
	}
	return Dial(ctx, value)
}

// Dial opens the source connection for a URL taken from an environment variable. Errors carry
// fixed texts only, never the URL.
func Dial(ctx context.Context, value string) (*pgx.Conn, error) {
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

// Version reads the source server version and refuses a source the target cannot restore
// (E518). It is shared by every source that snapshots a Postgres database.
func Version(ctx context.Context, db *pgx.Conn, target *config.Config) (string, error) {
	var version string
	var num int
	if db.QueryRow(ctx, "SHOW server_version").Scan(&version) != nil || db.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&num) != nil {
		return version, errs.New("E517", "source version unavailable")
	}
	// server_version_num is exact for betas ("17beta1") and for the pre-10 numbering.
	major := num / 10000
	targetMajor := major
	if target != nil {
		targetDB, e := load.Connect(ctx, target)
		if e != nil {
			return version, e
		}
		var tv int
		e = targetDB.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&tv)
		_ = targetDB.Close(ctx)
		if e != nil {
			return version, errs.New("E521", "target version unavailable")
		}
		targetMajor = tv / 10000
	}
	return version, CheckVersion(major, targetMajor)
}
func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	db, err := connect(ctx, in)
	if err != nil {
		return importer.Detection{}, err
	}
	defer func() { _ = db.Close(ctx) }()
	detection := importer.Detection{Kind: "postgres", Notes: []string{}}
	if detection.Version, err = Version(ctx, db, in.Target); err != nil {
		return detection, err
	}
	rows, err := db.Query(ctx, `SELECT nspname::text FROM pg_namespace WHERE left(nspname,3) <> 'pg_' AND left(nspname,3) <> 'np_' AND nspname NOT IN ('information_schema','hdb_catalog','nself_ops','auth') ORDER BY nspname`)
	if err != nil {
		return detection, errs.New("E517", "source schema lookup failed")
	}
	defer rows.Close()
	for rows.Next() {
		var schema string
		if rows.Scan(&schema) != nil {
			return detection, errs.New("E517", "source schema lookup failed")
		}
		detection.Schemas = append(detection.Schemas, schema)
	}
	if rows.Err() != nil {
		return detection, errs.New("E517", "source schema lookup failed")
	}
	var auth bool
	if db.QueryRow(ctx, `SELECT to_regclass('auth.users') IS NOT NULL`).Scan(&auth) != nil {
		return detection, errs.New("E517", "source auth lookup failed")
	}
	if auth {
		detection.Notes = append(detection.Notes, "looks like nhost")
		if db.QueryRow(ctx, `SELECT count(*) FROM auth.users`).Scan(&detection.SourceUsers) != nil {
			return detection, errs.New("E517", "source auth lookup failed")
		}
	}
	return detection, nil
}
func CheckVersion(source, target int) error {
	if source < 12 {
		return errs.New("E518", fmt.Sprintf("source Postgres major %d is older than 12; upgrade the source to Postgres 12 or newer", source))
	}
	if source > target {
		return errs.New("E518", fmt.Sprintf("source Postgres major %d requires POSTGRES_VERSION=%d (minimum supported source is 12)", source, source))
	}
	return nil
}
