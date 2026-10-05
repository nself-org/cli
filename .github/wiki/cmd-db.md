# nself db

<!-- BEGIN PROSE:summary -->
> Database operations: migrations, backups, restore, seed, shell.
<!-- END PROSE:summary -->

## Synopsis

```
nself db <subcommand> [flags]
```

## Description

<!-- BEGIN PROSE:description -->
Database operations: migrations, backups, restore, seed, and shell.

---
## db migrate

```bash
nself db migrate <subcommand> [flags]
```

### Flags (persistent)

| Flag | Description |
|---|---|
| `--plugin <name>` | Scope migration operations to a specific plugin |

### db migrate up

Apply all pending migrations in lexicographic order.

```bash
nself db migrate up [--dry-run] [--migration-dir <path>] [--plugin <name>]
```

| Flag | Description |
|---|---|
| `--dry-run` | List pending migrations without applying them |
| `--migration-dir <path>` | Apply all `.sql` files in `<path>` in lexicographic order, skipping already-applied files |

With `--migration-dir` and `--dry-run`, the command lists the pending files of that directory and applies nothing. It reads the ledger (`np_common.schema_versions`, plus the `nself_ops.migrations` checksums) with SELECT statements only: no table is created, no row is written, and nothing is run inside a rolled-back transaction. It fails, as the real run would, when an applied file was edited after apply or when a pending file is rejected by the SQL lint or the ALTER prerequisite check. With `--env`/`--server`, both flags are forwarded to the remote host.

**Examples:**

```bash
# Apply all pending migrations
nself db migrate up

# Preview pending migrations
nself db migrate up --dry-run

# Apply external directory of SQL files (G-008)
nself db migrate up --migration-dir /path/to/plugin/migrations

# Preview what that directory would apply (writes nothing)
nself db migrate up --migration-dir /path/to/plugin/migrations --dry-run
```

### db migrate apply

Apply a single SQL migration file by path (G-008).

```bash
nself db migrate apply --file <path>
```

| Flag | Required | Description |
|---|---|---|
| `--file <path>` | yes | Path to the `.sql` migration file to apply |

**Double-apply protection:** if the file's filename is already recorded in `schema_versions`, the command prints `already applied: <filename> (skipped)` and exits cleanly with code 0. It does not re-execute the SQL.

**Checksum tracking:** the SHA-256 checksum of the file is stored in `nself_ops.migrations` for audit purposes.

**Non-transactional detection:** SQL files containing `CREATE INDEX CONCURRENTLY`, `DROP INDEX CONCURRENTLY`, `REINDEX CONCURRENTLY`, or `ALTER TYPE` are run outside a transaction automatically.

**Examples:**

```bash
# Apply a specific external RLS migration
nself db migrate apply --file /path/to/plugin/rls_policy.sql

# Apply a migration from a plugin's migration directory
nself db migrate apply --file ~/.nself/plugins/claw/migrations/20240115_rls.sql
```

**Use case:** plugin-claw external RLS migrations can be applied via CLI without requiring `nself db shell` as a workaround.

### db migrate down

Revert the most recently applied migration.

```bash
nself db migrate down [--migration-dir <path> [--steps N]]
```

Without `--migration-dir`, looks for a corresponding `.down.sql` file next to the original migration.

| Flag | Description |
|---|---|
| `--migration-dir <path>` | Revert directory migrations: each reverted file `<name>.sql` needs `<name>_down.sql` or `<name>.down.sql` in `<path>` |
| `--steps N` | With `--migration-dir`: number of most recent migrations to revert (default 1) |

With `--migration-dir`, every step runs the down file and removes the migration's rows from both ledgers in one transaction. The newest applied migration must be a file of `<path>`; otherwise nothing is reverted and the error names it. A missing down file is an error naming both expected paths, and every step is checked before the first one runs. `--steps` without `--migration-dir` is an error.

### db migrate status

Show applied and pending migrations.

```bash
nself db migrate status
```

Reports: migration name, status (applied/pending), and timestamp for applied migrations.

### db migrate create

Create a new migration file pair (up + down).

```bash
nself db migrate create <name>
```

Creates `migrations/<timestamp>_<name>.sql` and `migrations/<timestamp>_<name>.down.sql`.

Name must be lowercase alphanumeric with underscores or hyphens only.

---

## db seed

Run seed data for the current environment.

```bash
nself db seed [file]
nself db seed run [--env <env>] [--fixture <name>] [--reset]
nself db seed list
nself db seed verify [--fixture <name>]
nself db seed graph
```

---

## db backup

```bash
nself db backup [file]
nself db backup list [--format table|json]
```

---

## db restore

```bash
nself db restore <file> [--overwrite] [--yes]
```

---

## db shell

Open an interactive psql shell inside the project's Postgres container.

```bash
nself db shell
```

---

## db drop

Drop the project database. This is destructive and irreversible.

```bash
nself db drop [--yes]
```

---

## db reset

Drop and recreate the project database from scratch.

```bash
nself db reset [--force] [--yes]
```

---

## db lint

Audit database schema and RLS policies.

```bash
nself db lint [--format table|json] [--rls] [--metric] [--matrix] [--remediate]
```

---

## db hasura

Hasura metadata operations.

```bash
nself db hasura console
nself db hasura metadata apply
nself db hasura metadata export
nself db hasura metadata reload
nself db hasura diff
nself db hasura validate
```

---
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Subcommands

<!-- BEGIN GENERATED:subcommands -->
| Name | Description |
|------|-------------|
| `backup` | Create pg_dump backup |
| `backup-sync` | Sync local backups to remote storage (cross-region) |
| `backup-sync-status` | Show cross-region backup sync status |
| `drift` | Schema drift detection: scan np_* tables for Theme 25 column compliance |
| `drop` | Drop the project database (DESTRUCTIVE) |
| `fk-index` | Audit and create indexes for foreign key columns |
| `hasura` | Hasura metadata operations |
| `lint` | Check RLS policies on tenant-scoped tables |
| `list` | List databases in the project Postgres instance |
| `migrate` | Manage database migrations |
| `pgbouncer` | PgBouncer connection pooler operations |
| `pitr` | Point-in-time recovery: status, enable, test, restore |
| `reconcile` | Push repo-declared Hasura metadata (permissions, relationships, table tracking) to a live instance |
| `reset` | Drop and recreate database (DESTRUCTIVE) |
| `reset-checksum` | Reset stored checksum for a migration (dangerous) |
| `restore` | Restore from backup |
| `restore-drill` | Run a non-destructive restore drill |
| `restore-drill-list` | List past restore drill results |
| `rls` | Row-Level Security management: audit, apply, rollback |
| `seed` | Database seeding: run, list, verify, graph |
| `shell` | Open psql interactive shell |
| `soft-delete` | Manage soft-delete (deleted_at) patterns across tables |
| `verify` | Verify what a Hasura role can actually reach via role-scoped introspection |
| `verify-checksums` | Verify migration file checksums against stored values |
<!-- END GENERATED:subcommands -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Apply all pending migrations
nself db migrate up

# Open an interactive psql shell inside the Postgres container
nself db shell

# Audit RLS policies and schema for tenant-scoped tables
nself db lint

# Take a local backup, then check migration status
nself db backup
nself db migrate status
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Architecture]] — stack overview including migration engine
- [[cmd-plugin-dev]] — plugin development, including plugin-specific migrations
- [[cmd-backup]] — full backup subcommand reference

[[Home]]
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
