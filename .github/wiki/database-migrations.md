# Database Migrations

> Internal reference for how ɳSelf tracks and applies database schema migrations.

## Overview

ɳSelf manages PostgreSQL schema migrations through two tracking tables:

| Table | Purpose |
|---|---|
| `np_common.schema_versions` | Lightweight applied-migration log (name + timestamp) |
| `nself_ops.migrations` | Audit log with migration ID, name, checksum, and applied timestamp |

Both tables are updated atomically on every migration apply and revert. If either update fails, the transaction rolls back and neither table is modified.

## Apply (`nself db migrate up`)

For each pending migration file:

- **Transactional DDL** (default): the migration SQL and both INSERT statements run inside a single `BEGIN … COMMIT` block. `SET LOCAL lock_timeout = '5s'` and `SET LOCAL statement_timeout = '60s'` are applied before the DDL to prevent blocking production deployments.
- **Non-transactional DDL** (`CREATE INDEX CONCURRENTLY`, `ALTER TYPE ADD VALUE`, etc.): the DDL runs outside a transaction (required by PostgreSQL). Immediately after, both INSERT statements are wrapped in their own `BEGIN … COMMIT` block so that the recording step is still atomic.

The 60s `statement_timeout` is the default. A migration that needs longer, such as a data backfill, sets its own with a comment line anywhere in the file: `-- nself:statement-timeout=600s` raises it to 10 minutes and `-- nself:statement-timeout=0` removes the limit. The directive applies to `up` and `down`, with or without `--migration-dir`; `lock_timeout` stays at 5s.

`nself db migrate status` is read-only (SELECT statements only). If the ledger still has legacy rows that `up` or `down` would upgrade, status warns on stderr and reports `ledger_needs_upgrade: true` in its `--json` data and in the `nself_db_migrate_status` MCP result.

In both cases, a failure rolls back to a clean state — no orphan rows are left in either tracking table.

## Revert (`nself db migrate down`)

`MigrateDown` reads the most recently applied migration from `np_common.schema_versions`, locates the corresponding `.down.sql` file, and runs a single transaction:

```sql
BEGIN;
<down migration DDL>
DELETE FROM np_common.schema_versions WHERE name = '<name>';
DELETE FROM nself_ops.migrations WHERE name = '<name>';
COMMIT;
```

Both `DELETE` statements execute inside the same transaction as the down DDL. Either all three succeed together, or the transaction rolls back and both tracking tables remain unchanged.

## Double-apply protection

`ApplyFile` checks `np_common.schema_versions` before executing a migration. If the filename is already present:

1. The stored checksum in `nself_ops.migrations` is compared against the file's current SHA-256.
2. If the checksums match, the migration is skipped with `(skipped=true, nil)`.
3. If the checksums differ, the function returns an error — the file was modified after it was applied and manual intervention is required.

## Layout auto-detection

`migrate up` and `migrate status` scan the first existing directory from:

1. `hasura/migrations/default/` (standard Hasura layout)
2. `hasura/migrations/` (flat Hasura layout)
3. `migrations/` (legacy)
4. `postgres/migrations/` (repo-local layout, e.g. ntask)

Both subcommands accept `--migration-dir <dir>` to override auto-detection. On remote targets (`--env staging|prod`) the flag is forwarded to the remote CLI; the pre-flight version-drift check guarantees the remote binary understands it.

Remote migration commands validate `--plugin` locally before dispatch and forward it once. A directory value equal to `--plugin` remains a directory value and does not suppress plugin validation or forwarding.

## Migration file naming

| Pattern | Meaning |
|---|---|
| `NNN_<description>.sql` | Up migration |
| `NNN_<description>.down.sql` | Corresponding down migration |

Down files are excluded from `scanMigrations` and are only loaded by `MigrateDown`.

## SQL injection safety

All migration names inserted into SQL statements use single-quote escaping (`strings.ReplaceAll(name, "'", "''")`) before interpolation. This applies to both `DELETE` statements in `MigrateDown` and both `INSERT` statements in the recording step.

## Related pages

- [[cmd-db]] — `nself db` command reference
- [[cmd-migrate]] — v0.9→v1 project migration (separate from schema migrations)
- [[Home]]
