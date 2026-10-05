// Package migrate is the boot-time plugin migration helper (contract
// plugin.boot-migrations v1, ADR 0010, PLUG epic D5).
//
// A plugin calls Apply from main() before it serves traffic. Apply runs the
// plugin's own migrations/*.sql files against its np_<name> schema:
//
//   - files are the *.sql entries of one directory (no recursion, no
//     *.down.sql), applied in plain lexical (byte) order of the file name;
//   - one transaction per file: the file's SQL and its ledger row commit
//     together or not at all, so a crash never leaves a file half-applied or
//     applied-but-unrecorded;
//   - every file transaction sets lock_timeout, statement_timeout and a
//     search_path pinned to the plugin schema (SET LOCAL semantics);
//   - the ledger is np_<name>.schema_migrations(filename, checksum,
//     applied_at); an applied file whose sha256 changed is fatal;
//   - a session-level advisory lock keyed per plugin serialises replicas that
//     boot at the same time, and is held for the whole run (including the
//     Between callback);
//   - Tolerant mode defers files that fail with a missing-dependency SQLSTATE
//     (42P01, 42703, 42704) until a fixed point, runs Options.Between (the
//     plugin's Go-native migrations), then applies the rest strictly.
//
// Migration files must not contain transaction control (BEGIN, COMMIT) or
// statements that cannot run in a transaction block (CREATE INDEX
// CONCURRENTLY): the per-file transaction is what makes the ledger honest.
//
// The package never builds SQL from file names: names and checksums travel as
// bind parameters. Only the schema name is interpolated, and only after it
// matches ^np_[a-z][a-z0-9_]*$ and is quoted as an identifier.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Default per-file timeouts (PLUG epic D5).
const (
	DefaultLockTimeout      = 5 * time.Second
	DefaultStatementTimeout = 60 * time.Second
)

var (
	schemaRE     = regexp.MustCompile(`^np_[a-z][a-z0-9_]*$`)
	identifierRE = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
)

// Options configures Apply and Status.
type Options struct {
	// Schema is the plugin's own schema, np_<name>. Required.
	Schema string
	// FS is the file system holding the migration files, typically an
	// embed.FS. Dir is the directory inside FS (empty or "." for its root).
	// With FS nil, Dir is a directory on disk. One of FS or Dir is required.
	FS  fs.FS
	Dir string
	// SearchPath lists extra schemas appended after Schema in the pinned
	// search_path (for example "public" for an extension). Empty by default.
	SearchPath []string
	// Tolerant defers dependency errors (42P01, 42703, 42704) until a fixed
	// point, runs Between, then applies what remains strictly.
	Tolerant bool
	// LockTimeout and StatementTimeout apply to every file transaction.
	// Zero means DefaultLockTimeout / DefaultStatementTimeout.
	LockTimeout      time.Duration
	StatementTimeout time.Duration
	// Between runs once, with the advisory lock held, after the tolerant pass
	// and before the strict pass. Only valid with Tolerant. It receives its
	// own pool connections, so the pool must not be limited to one.
	Between func(ctx context.Context) error
}

// Result reports what one Apply call did.
type Result struct {
	// Applied lists the files this call applied, in order.
	Applied []string
}

// FileError names the migration file that failed.
type FileError struct {
	File string
	Err  error
}

func (e *FileError) Error() string { return fmt.Sprintf("migrate: file %s: %v", e.File, e.Err) }
func (e *FileError) Unwrap() error { return e.Err }

// ChecksumError reports an applied file whose content changed on disk.
type ChecksumError struct {
	File          string
	Recorded, Now string
}

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("migrate: file %s was applied with checksum %s but now has %s: "+
		"an applied migration must never be edited, add a new file instead", e.File, e.Recorded, e.Now)
}

// plan is the validated, resolved form of Options.
type plan struct {
	opts    Options
	fsys    fs.FS
	schema  string // quoted identifier
	path    string // search_path value
	lockKey string
}

func newPlan(o Options) (*plan, error) {
	if !schemaRE.MatchString(o.Schema) || len(o.Schema) > 63 {
		return nil, fmt.Errorf("migrate: schema %q must match np_<name> (^np_[a-z][a-z0-9_]*$, at most 63 bytes)", o.Schema)
	}
	if o.Between != nil && !o.Tolerant {
		return nil, errors.New("migrate: Options.Between requires Tolerant")
	}
	if o.LockTimeout < 0 || o.StatementTimeout < 0 {
		return nil, errors.New("migrate: timeouts must not be negative")
	}
	if o.LockTimeout == 0 {
		o.LockTimeout = DefaultLockTimeout
	}
	if o.StatementTimeout == 0 {
		o.StatementTimeout = DefaultStatementTimeout
	}
	path := pgx.Identifier{o.Schema}.Sanitize()
	for _, s := range o.SearchPath {
		if !identifierRE.MatchString(s) {
			return nil, fmt.Errorf("migrate: search path entry %q is not a plain lower-case identifier", s)
		}
		path += ", " + pgx.Identifier{s}.Sanitize()
	}
	fsys := o.FS
	switch {
	case fsys == nil && o.Dir == "":
		return nil, errors.New("migrate: one of Options.FS or Options.Dir is required")
	case fsys == nil:
		fsys = os.DirFS(o.Dir)
	case o.Dir != "" && o.Dir != ".":
		sub, err := fs.Sub(fsys, o.Dir)
		if err != nil {
			return nil, fmt.Errorf("migrate: dir %q: %w", o.Dir, err)
		}
		fsys = sub
	}
	return &plan{opts: o, fsys: fsys, schema: pgx.Identifier{o.Schema}.Sanitize(), path: path,
		lockKey: o.Schema + ".schema_migrations"}, nil
}

// Apply applies every pending migration file. It is safe to call on every
// boot and from several replicas at once. A failure leaves no ledger row for
// the failing file, applies nothing after it, and returns an error naming it.
func Apply(ctx context.Context, pool *pgxpool.Pool, opts Options) (Result, error) {
	var res Result
	p, err := newPlan(opts)
	if err != nil {
		return res, err
	}
	files, err := p.readFiles()
	if err != nil {
		return res, err
	}
	conn, err := lockConn(ctx, pool, p.lockKey)
	if err != nil {
		return res, err
	}
	defer closeConn(conn)

	if err := p.ensureLedger(ctx, conn); err != nil {
		return res, err
	}
	recorded, err := p.loadLedger(ctx, conn)
	if err != nil {
		return res, err
	}
	var pending []file
	for _, f := range files {
		sum, done := recorded[f.name]
		switch {
		case !done:
			pending = append(pending, f)
		case sum != f.sum:
			return res, &ChecksumError{File: f.name, Recorded: sum, Now: f.sum}
		}
	}

	pending, err = p.runRounds(ctx, conn, pending, p.opts.Tolerant, &res)
	if err != nil {
		return res, err
	}
	if !p.opts.Tolerant {
		return res, nil
	}
	if p.opts.Between != nil {
		if err := p.opts.Between(ctx); err != nil {
			return res, fmt.Errorf("migrate: Between callback: %w", err)
		}
	}
	_, err = p.runRounds(ctx, conn, pending, false, &res)
	return res, err
}
