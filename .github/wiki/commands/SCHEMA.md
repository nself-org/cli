# Schema Management in nself

Schema management in nself is handled through two complementary tools: **migrations** (via `nself db migrate`) and the **Hasura Console** (via `nself hasura`). There is no standalone `nself db schema` subcommand — schema operations are surfaced through the appropriate tool depending on the operation.

---

## Tools for Schema Management

| Operation | Command |
| --------- | ------- |
| Apply pending migrations | `nself db migrate` |
| Create a new migration | `nself db migrate create <name>` |
| Check migration status | `nself db migrate status` |
| Open the Hasura Console (GUI schema editor) | `nself hasura console` |
| Apply Hasura metadata | `nself hasura metadata apply` |
| Export current Hasura metadata | `nself hasura metadata export` |
| Run Hasura migrations directly | `nself hasura migrate apply` |
| Open a raw database shell | `nself db shell` |

---

## Migrations

Migrations are SQL files that evolve your database schema over time. They live in the `migrations/` directory of your nself project.

### Run All Pending Migrations

```bash
nself db migrate
```

### Check Migration Status

```bash
nself db migrate status
```

Shows which migrations have been applied and which are pending.

### Create a New Migration

```bash
nself db migrate create add_user_preferences
```

Creates a new timestamped SQL file in `migrations/`. Edit the file to add your schema changes, then run `nself db migrate` to apply it.

### Rollback a Migration

```bash
nself db migrate rollback
nself db migrate rollback --steps 3   # Roll back 3 migrations
```

### Full Reference

See [Database Workflow Guide](../guides/DATABASE-WORKFLOW.md) for the complete migration workflow, including environments, conflict resolution, and CI/CD integration.

---

## Hasura Console (Visual Schema Editor)

The Hasura Console provides a GUI for creating tables, columns, relationships, and permissions without writing SQL directly. Changes made in the Console are tracked as migrations automatically.

### Open the Console

```bash
nself hasura console
```

Opens the Hasura Console at `http://localhost:9695` with migration tracking enabled. All GUI changes automatically create migration files in your `migrations/` directory.

### Apply Metadata Changes

```bash
nself hasura metadata apply
```

Applies relationship, permission, and event trigger configuration from `metadata/` to your running Hasura instance.

### Export Current Metadata

```bash
nself hasura metadata export
```

Exports the current Hasura configuration (tables, relationships, permissions) to the `metadata/` directory so it can be version-controlled.

---

## Raw Database Shell

For ad-hoc schema inspection or one-off queries:

```bash
nself db shell
```

Opens a `psql` session connected to your project database. Use this for inspection only — prefer migrations for any schema changes you want to track.

```sql
-- List all tables
\dt

-- Describe a specific table
\d users

-- Show table columns
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = 'users'
ORDER BY ordinal_position;
```

---

## Schema Workflow (Recommended)

### Development

1. Open Hasura Console: `nself hasura console`
2. Create/modify tables and relationships using the GUI
3. Console auto-generates migration files in `migrations/`
4. Commit migration files to version control
5. Apply to staging: `nself deploy staging` (runs migrations automatically)
6. Apply to production: `nself deploy production`

### Code-First (SQL Migrations)

1. Create migration file: `nself db migrate create <description>`
2. Edit the generated `.sql` file with your DDL
3. Apply locally: `nself db migrate`
4. Update Hasura metadata if new tables/relationships were added: `nself hasura metadata apply`
5. Commit both migration + metadata files
6. Deploy to environments

---

## Related Documentation

- [Database Workflow Guide](../guides/DATABASE-WORKFLOW.md) — Full migration workflow
- [Hasura Command Reference](HASURA.md) — All `nself hasura` subcommands
- [Database Command Reference](DB.md) — All `nself db` subcommands
- [Backup & Restore](BACKUP.md) — Schema backups and point-in-time recovery
