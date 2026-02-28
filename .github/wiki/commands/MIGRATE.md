# nself perf migrate - Cross-Environment Migration

> **DEPRECATED COMMAND NAME**: This command was formerly `nself migrate` in v0.x. It has been consolidated to `nself perf migrate` in v1.0. The old command name may still work as an alias.

**Version 0.4.6** | Environment migration and synchronization

---

## Overview

The `nself perf migrate` command handles cross-environment migration, allowing you to migrate schema, data, and configuration between local, staging, and production environments.

---

## Usage

```bash
nself perf migrate <source> <target> [options]
nself perf migrate <subcommand> [options]
```

---

## Subcommands

### `<source> <target>`

Migrate from source to target environment.

```bash
nself perf migrate local staging      # Migrate local to staging
nself perf migrate staging prod       # Migrate staging to production
```

### `diff <source> <target>`

Show differences between environments.

```bash
nself perf migrate diff local staging    # Compare local vs staging
nself perf migrate diff staging prod     # Compare staging vs prod
```

### `sync <source> <target>`

Keep environments continuously in sync.

```bash
nself perf migrate sync staging prod          # One-time sync
nself perf migrate sync staging prod --watch  # Continuous sync
```

### `rollback`

Rollback the last migration.

```bash
nself perf migrate rollback           # Undo last migration
```

---

## Options

| Option | Description |
|--------|-------------|
| `--dry-run` | Preview migration without making changes |
| `--schema-only` | Migrate only database schema |
| `--data-only` | Migrate only data (no schema changes) |
| `--config-only` | Migrate only configuration |
| `--force` | Skip confirmation prompts |
| `--watch` | Continuous sync mode (with sync) |
| `--json` | Output in JSON format |
| `-h, --help` | Show help message |

---

## Environments

| Environment | Description |
|-------------|-------------|
| `local` | Local development environment |
| `dev` | Alias for local |
| `staging` | Staging environment |
| `prod` | Production environment |
| `production` | Alias for prod |

---

## Examples

```bash
# Preview migration without changes
nself perf migrate local staging --dry-run

# Migrate only schema
nself perf migrate staging prod --schema-only

# Compare environments
nself perf migrate diff staging prod

# Continuous sync (Ctrl+C to stop)
nself perf migrate sync staging prod --watch

# Rollback last migration
nself perf migrate rollback
```

---

## Migration Process

When running a migration:

1. **Validation** - Source and target environments are validated
2. **Safety Check** - Production migrations require confirmation
3. **Backup** - Target environment is backed up automatically
4. **Checkpoint** - Migration checkpoint is saved for rollback
5. **Migration** - Schema, data, and/or config are migrated
6. **Verification** - Migration is verified

---

## Components

Migrations can include:

| Component | Description |
|-----------|-------------|
| **Schema** | Database tables, indexes, constraints |
| **Data** | Database records |
| **Config** | Environment variables, service settings |

Use `--schema-only`, `--data-only`, or `--config-only` to migrate specific components.

---

## Checkpoints

Each migration creates a checkpoint in `.nself/migrations/`. Checkpoints allow you to rollback if needed.

```bash
# View checkpoints
ls .nself/migrations/

# Rollback using checkpoint
nself perf migrate rollback
```

---

## Production Safety

When migrating to production:

- Confirmation is required (type 'yes')
- Use `--force` to skip confirmation (CI/CD use)
- Automatic backup is created before migration
- Consider `--dry-run` first

```bash
# Safe production migration workflow
nself perf migrate staging prod --dry-run    # Preview
nself perf migrate staging prod              # Execute with confirmation
```

---

## Configuration Files

Migration uses these configuration sources:

| Environment | Config Files |
|-------------|--------------|
| local | `.env`, `.env.local`, `.env.dev` |
| staging | `.env.staging`, `.environments/staging/server.json` |
| production | `.env.prod`, `.environments/prod/server.json` |

---

## Related Commands

- [sync](SYNC.md) - Data synchronization
- [env](ENV.md) - Environment management
- [deploy](DEPLOY.md) - Deployment
- [db migrate](DB.md) - Database migrations

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
