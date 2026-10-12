package postgres

// Purpose: export one repeatable-read snapshot into the portable contract.
// Inputs: source connection and stage path. Outputs: hashed dump and schema files.
// Constraints: pg_dump uses target image, EnvPass credentials, host uid:gid, same snapshot.
import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/portable"
	"github.com/nself-org/cli/internal/postgres/tablehash"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
)

var runOneShot = docker.RunOneShot

func (s Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	detection, err := s.Detect(ctx, in)
	if err != nil {
		return importer.Extracted{}, err
	}
	if in.Target == nil {
		return importer.Extracted{}, errs.New("E520", "target config required for snapshot extraction")
	}
	db, err := connect(ctx, in)
	if err != nil {
		return importer.Extracted{}, err
	}
	return Snapshot(ctx, db, detection, in.Target, stage, Options{Source: "postgres"})
}

// Options lets another Postgres-backed source reuse the snapshot extractor.
// Source names the manifest producer. InTx runs inside the snapshot transaction once the dump
// members are in the bundle (work holds the pg_dump outputs); it replaces the hasura-auth user
// export. Finish runs after the snapshot commits and before the manifest is written.
type Options struct {
	Source string
	InTx   func(ctx context.Context, tx pgx.Tx, w *portable.Writer, m *portable.Manifest, work string) error
	Finish func(ctx context.Context, w *portable.Writer, m *portable.Manifest) error
}

// Snapshot exports detection.Schemas from db inside one REPEATABLE READ snapshot into a portable
// bundle at stage. It closes db. Errors are fixed credential-free texts.
func Snapshot(ctx context.Context, db *pgx.Conn, detection importer.Detection, target *config.Config, stage string, opt Options) (importer.Extracted, error) {
	if target == nil {
		_ = db.Close(ctx)
		return importer.Extracted{}, errs.New("E520", "target config required for snapshot extraction")
	}
	defer func() { _ = db.Close(ctx) }()
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return importer.Extracted{}, errs.New("E520", "source snapshot failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var snapshot string
	if tx.QueryRow(ctx, "SELECT pg_export_snapshot()").Scan(&snapshot) != nil {
		return importer.Extracted{}, errs.New("E520", "source snapshot export failed")
	}
	tables, err := tablehash.ListTables(ctx, tx, detection.Schemas)
	if err != nil {
		return importer.Extracted{}, errs.New("E520", "source tables unavailable")
	}
	kept := []tablehash.Table{}
	for _, t := range tables {
		if !importer.SystemTables(t.Schema, t.Name, false) {
			if _, e := database.SanitizeIdentifier(t.Schema); e != nil {
				return importer.Extracted{}, errs.New("E520", "unsupported source schema")
			}
			if _, e := database.SanitizeIdentifier(t.Name); e != nil {
				return importer.Extracted{}, errs.New("E520", "unsupported source table")
			}
			kept = append(kept, t)
		}
	}
	if err = refusePartitioned(ctx, tx, detection.Schemas); err != nil {
		return importer.Extracted{}, err
	}
	hashed, bad, err := tablehash.Snapshot(ctx, tx, kept)
	if err != nil || len(bad) > 0 {
		return importer.Extracted{}, errs.New("E520", "source table hashing failed")
	}
	major := majorOf(detection.Version)
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: opt.Source}, Postgres: portable.PostgresInfo{ServerVersion: detection.Version, Major: major}, DB: portable.DB{Schemas: detection.Schemas}}
	for _, h := range hashed {
		t := portable.Table{Schema: h.Table.Schema, Name: h.Table.Name, Rows: h.Rows, Hash: h.Hash}
		t.PK, err = primaryKey(ctx, tx, t.Schema, t.Name)
		if err != nil {
			return importer.Extracted{}, errs.New("E520", "source key lookup failed")
		}
		m.DB.Tables = append(m.DB.Tables, t)
		schema, _ := database.SanitizeIdentifier(t.Schema)
		name, _ := database.SanitizeIdentifier(t.Name)
		var count int64
		if tx.QueryRow(ctx, "SELECT count(*) FROM "+schema+"."+name).Scan(&count) != nil {
			return importer.Extracted{}, errs.New("E520", "independent source count failed")
		}
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "table", ID: t.Schema + "." + t.Name, Count: count, Method: "independent count(*)"})
	}
	image, err := load.Image(ctx, target)
	if err != nil {
		return importer.Extracted{}, err
	}
	if err = os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
		return importer.Extracted{}, errs.New("E520", "snapshot staging unavailable")
	}
	work, err := os.MkdirTemp(filepath.Dir(stage), ".extract-")
	if err != nil {
		return importer.Extracted{}, errs.New("E520", "snapshot staging unavailable")
	}
	defer func() { _ = os.RemoveAll(work) }()
	spec := dumpSpec(db.Config(), image, work)
	base, err := dumpArgs(snapshot, detection.Schemas)
	if err != nil {
		return importer.Extracted{}, err
	}
	for _, job := range []struct {
		path string
		args []string
	}{{"data.dump", []string{"-Fc"}}, {"schema.sql", []string{"--schema-only"}}} {
		spec.Args = append(append([]string(nil), base...), job.args...)
		spec.Args = append(spec.Args, "-f", "/export/"+job.path)
		if _, _, err = runOneShot(ctx, spec); err != nil {
			return importer.Extracted{}, errs.New("E520", "pg_dump snapshot extraction failed")
		}
	}
	writer, err := portable.NewWriter(stage)
	if err != nil {
		return importer.Extracted{}, errs.New("E520", "bundle creation failed")
	}
	for _, name := range []string{"data.dump", "schema.sql"} {
		if err = writeMember(writer, "db/"+name, filepath.Join(work, name)); err != nil {
			return importer.Extracted{}, errs.New("E520", "dump member write failed")
		}
	}
	if opt.InTx != nil {
		err = opt.InTx(ctx, tx, writer, &m, work)
	} else {
		err = exportUsers(ctx, tx, writer, &m)
	}
	if err != nil {
		return importer.Extracted{}, publicSourceError(err, "E520", "auth snapshot extraction failed")
	}
	if tx.Commit(ctx) != nil {
		return importer.Extracted{}, errs.New("E520", "snapshot commit failed")
	}
	if opt.Finish != nil {
		_ = db.Close(ctx) // the source stays idle while Finish may download for a long time
		if err = opt.Finish(ctx, writer, &m); err != nil {
			return importer.Extracted{}, publicSourceError(err, "E520", "source extraction failed")
		}
	}
	if _, err = writer.Finish(m); err != nil {
		return importer.Extracted{}, errs.New("E520", "bundle manifest write failed")
	}
	return importer.Extracted{Stage: stage}, nil
}
func dumpSpec(cfg *pgx.ConnConfig, image, work string) docker.RunSpec {
	host := cfg.Host
	network := ""
	if runtime.GOOS == "linux" {
		network = "host"
	} else if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		host = "host.docker.internal"
	}
	sslMode := "prefer"
	if u, e := url.Parse(cfg.ConnString()); e == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		if mode := u.Query().Get("sslmode"); mode != "" {
			sslMode = mode
		}
	} else if match := regexp.MustCompile(`(?:^|\s)sslmode=['"]?([a-z-]+)`).FindStringSubmatch(cfg.ConnString()); len(match) == 2 {
		sslMode = match[1]
	}
	return docker.RunSpec{Image: image, Network: network, User: strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), Mounts: []docker.Mount{{Source: work, Destination: "/export"}}, EnvPass: map[string]string{"PGHOST": host, "PGPORT": strconv.Itoa(int(cfg.Port)), "PGUSER": cfg.User, "PGDATABASE": cfg.Database, "PGPASSWORD": cfg.Password, "PGSSLMODE": sslMode}}
}
func writeMember(w *portable.Writer, member, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = w.WriteFile(member, io.Reader(f))
	return err
}
func primaryKey(ctx context.Context, tx pgx.Tx, schema, name string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT a.attname::text FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace JOIN unnest(i.indkey) WITH ORDINALITY k(attnum,pos) ON true JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=k.attnum WHERE i.indisprimary AND n.nspname=$1 AND c.relname=$2 ORDER BY k.pos`, schema, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var col string
		if err = rows.Scan(&col); err != nil {
			return nil, err
		}
		out = append(out, col)
	}
	return out, rows.Err()
}

// publicSourceError keeps a hook's own coded error (its text is fixed and credential-free by
// the hook's contract) and turns anything else into the generic one.
func publicSourceError(err error, code, message string) error {
	var cli *errs.CLIError
	if errors.As(err, &cli) {
		return cli
	}
	return errs.New(code, message)
}
