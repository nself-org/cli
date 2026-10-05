# Plugin migrations contract (plugin.boot-migrations v1)

Every plugin that ships SQL applies its own migrations when its service process starts
(ADR 0010). `nself plugin install` creates the schema,
role and grants only. It never runs plugin SQL. The CLI waits for the plugin to report that its
migrations are applied, through the `/health` field below.

The Go helper is `github.com/nself-org/cli/sdk/go/v2/migrate`. A plugin in another language
implements this page itself and must pass the same behaviour.

## Entrypoint convention

Embed the files, open the pool, call `migrate.Apply` before serving, and fail the boot on error.

```go
//go:embed migrations/*.sql
var migrationFiles embed.FS

opts := migrate.Options{
    Schema: "np_myplugin",    // np_<name>, validated
    FS:     migrationFiles,
    Dir:    "migrations",
}
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()
if _, err := migrate.Apply(ctx, pool, opts); err != nil {
    log.Fatalf("migrations: %v", err) // never serve on a half-migrated schema
}
```

Serve readiness in `/health`:

```go
st, err := migrate.Status(r.Context(), pool, opts)
// {"status":"ok", "migrations":{"applied":3,"expected":3}}
fmt.Fprintf(w, `{"status":"ok",%s}`, migrate.HealthField(st))
```

`Status` reads the ledger without locking or writing. A missing ledger counts as zero applied.
`expected` counts the files currently in the migration directory. `applied` counts those recorded
with a matching checksum. Two more numbers appear in the field only when non-zero:
`"drifted":n` (a recorded file whose content changed, not counted in `applied`) and
`"deleted":k` (a recorded file no longer on disk). `Progress.Ready()` is false when `applied !=
expected`, or when either is non-zero. A legacy ledger without a checksum column is read by file
name only, so drift stays invisible until `Baseline` runs.

## Rules

| Rule | Behaviour |
|---|---|
| Files | `*.sql` in one directory, no recursion, `*.down.sql` excluded, everything else ignored |
| Order | plain lexical (byte) order of the file name. Zero-pad numbers: `001_x.sql`, `010_y.sql`. A pending file that sorts before an applied one is refused (`*migrate.OutOfOrderError`) before anything runs; name new files after the last applied one. Tolerant mode skips this check, because deferral reorders on purpose and a retry after a failed boot must work |
| Transaction | one per file: the SQL and its ledger row commit together, or neither does |
| Transaction control | a file with `BEGIN`, `COMMIT`, `ROLLBACK`, `END`, `ABORT`, `START TRANSACTION`, `SAVEPOINT`, `RELEASE` or `PREPARE TRANSACTION` is refused before any statement runs, with an error naming the file and line (`ErrTxControl`). Words inside strings, comments, quoted identifiers and `$$` bodies do not count. `CREATE INDEX CONCURRENTLY` cannot run in the file transaction either |
| Timeouts | `SET LOCAL lock_timeout = 5s` and `statement_timeout = 60s` per file; override with `Options.LockTimeout` and `Options.StatementTimeout` |
| `search_path` | `SET LOCAL` to the plugin schema, so unqualified names land in it. `Options.SearchPath` appends extra schemas (for example `public` for an extension) |
| Lock | session-level `pg_advisory_lock(hashtext('np_<name>.schema_migrations'))` on a dedicated connection, held for the whole run. Replicas booting together serialise; two plugins never block each other. The wait ignores role-level timeouts and ends only when the context ends. A crashed process drops its connection, but the server releases the lock only once it notices: a backend busy inside a long statement keeps it until the statement ends or `statement_timeout` (60 s by default) cancels it, so a restarted plugin may wait that long. Setting `client_connection_check_interval` on the plugin role shortens it |
| Failure | the failing file rolls back alone: no effect, no ledger row. Nothing after it runs. The error names the file (`*migrate.FileError`) |
| Checksum | an applied file whose sha256 changed makes `Apply` fail before anything runs (`*migrate.ChecksumError`). Never edit an applied file: add a new one |
| Deleted file | an applied file removed from disk is ignored by `Apply` and reported by `Status` as `deleted` |
| Idempotent | a second boot applies nothing and leaves the ledger untouched |
| SQL safety | file names and checksums are bind parameters. Only the schema name is interpolated, after it matches `^np_[a-z][a-z0-9_]*$` and is quoted |

## Ledger

Created in the plugin's own schema under the advisory lock. The schema is created only when absent.

```sql
CREATE TABLE IF NOT EXISTS np_<name>.schema_migrations (
    filename   text PRIMARY KEY,
    checksum   text NOT NULL,            -- sha256 hex of the raw file bytes
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

`Apply` never guesses about an existing install. It refuses, with an error that points to
`Baseline`, when:

- the schema already has tables, views, sequences, functions or enum types but no ledger
  (`ErrUnledgered`): replaying every file over a hand-built schema would fail or corrupt data;
- the ledger exists but has no `checksum` column (`ErrLegacyLedger`), such as claw's
  `(filename, applied_at)` ledger. The column is detected through `pg_attribute`.

An empty schema (what `nself plugin install` leaves behind) is fine.

## Adopting an existing install: Baseline

```go
res, err := migrate.Baseline(ctx, pool, opts) // one-off, deliberate
// res.Baselined lists the files recorded without running them
```

`Baseline` takes the same lock as `Apply` and works in one transaction. It records every current
file with its sha256 and runs none of them. With a legacy ledger it first adds the `checksum`
column, backfills it from the current files, and gives rows whose file is gone the empty checksum
(`Status` then reports them as `deleted`). With a current ledger it only adds files that are not
yet recorded and leaves existing rows, drifted ones included, alone. It is idempotent.
Run it once when `Apply` returns `ErrUnledgered` or `ErrLegacyLedger`, then boot normally.

## Tolerant mode (Go-native migrations)

A plugin that interleaves Go-native migrations with SQL files (claw) sets `Tolerant: true` and
`Between`:

1. Tolerant pass: a file that fails with SQLSTATE `42P01`, `42703` or `42704` is deferred,
   left unrecorded, and retried. Rounds repeat until nothing is pending or a round makes no progress.
2. `Between(ctx)` runs once, with the lock held. An error fails the boot.
3. Strict pass: whatever is left runs strictly; the first failure is fatal and names its file.

Any other error in the tolerant pass is fatal at once. Transaction control is refused as in strict mode. `Between` is only valid with `Tolerant`.
It uses the pool, which is separate from the lock connection, so a pool limited to one
connection still works.

## Errors and the CLI

`Apply` returns `*migrate.FileError` (file and cause), `*migrate.ChecksumError`, `*migrate.OutOfOrderError`, or an error wrapping `ErrUnledgered` or `ErrLegacyLedger`. A plugin logs
the error and exits non-zero, so the container shows unhealthy and `nself plugin install` fails
with a coded error naming the plugin once its bounded wait (default 120 s) ends. `nself doctor`
flags a plugin with `migrations/` whose manifest lacks `migrations.apply: boot`.

## Verification

Tests run against a real `postgres:16` container: fresh apply, re-run, concurrent replicas, kill
mid-file (context cancel and backend termination), checksum drift, transaction-control refusal,
out-of-order refusal, baseline and legacy-ledger upgrade, tolerant mode, timeouts and the health field. Run them with `cd sdk/go && go test ./migrate/...` (Docker required; an
unreachable Docker is a failure, not a skip).

---
[[Home]]
