# nself db reset

**Category**: Database Commands

Completely reset the database to a clean state.

## Overview

**⚠️ DANGER**: This command permanently deletes all data in your database!

The `reset` command drops and recreates the database, providing a fresh start. It's useful during development but should never be used in production.

**Features**:
- ✅ Complete data deletion
- ✅ Schema recreation
- ✅ Environment protection (prod disabled)
- ✅ Confirmation prompts
- ✅ Automatic backup option

## Usage

```bash
nself db reset [OPTIONS]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--force` | Skip confirmation (dev only) | false |
| `--backup` | Create backup before reset | false |
| `--no-seed` | Skip seed data after reset | false |
| `--no-migrate` | Skip migrations after reset | false |

## Behavior by Environment

### Development (ENV=dev)

```bash
nself db reset
```

**Allowed** ✅
Prompts for confirmation.

### Staging (ENV=staging)

```bash
nself db reset
```

**Blocked** ❌
Requires explicit override:
```bash
ALLOW_STAGING_RESET=true nself db reset
```

### Production (ENV=prod)

```bash
nself db reset
```

**BLOCKED** 🚫
**NEVER ALLOWED** - No override available.

**Error**:
```
✗ FATAL: Database reset is disabled in production

This operation would permanently delete all production data.
Use 'nself db backup' and 'nself db restore' instead.

If you genuinely need to reset production:
1. Create full backup
2. Document justification
3. Use manual PostgreSQL commands
4. This command will not run in production, by design.
```

## Examples

### Basic Reset

```bash
nself db reset
```

**Output**:
```
⚠️  WARNING: This will permanently delete ALL database data!

Database: myapp_db
Environment: dev
Tables: 24
Estimated rows: 15,423

This action CANNOT be undone.
Type 'reset' to confirm:
```

After typing 'reset':
```
→ Creating backup before reset...
  ✓ Backup created: backups/pre-reset-20260213-143022.sql

→ Resetting database...
  ⠋ Dropping database...              ✓
  ⠋ Creating database...              ✓
  ⠋ Applying schema extensions...     ✓

✓ Database reset complete

Next steps:
  nself db migrate up    # Apply migrations
  nself db seed          # Load seed data
```

### Reset with Auto-Backup

```bash
nself db reset --backup
```

**Automatically creates backup before reset.**

### Force Reset (Development Only)

```bash
nself db reset --force
```

**⚠️ Skips confirmation.**

**Use in**:
- Automated test scripts
- Development workflows
- CI/CD pipelines (dev/test environments)

### Reset and Rebuild

```bash
nself db reset && nself db migrate up && nself db seed
```

**Common workflow**:
1. Reset database
2. Apply all migrations
3. Load seed data

**One-liner**:
```bash
alias db-fresh="nself db reset --force && nself db migrate up && nself db seed"
db-fresh
```

## What Gets Deleted

### All Data

- ✅ All tables
- ✅ All rows
- ✅ All indexes
- ✅ All constraints
- ✅ All triggers
- ✅ All functions
- ✅ All views
- ✅ Migration history

### What Persists

- ✅ Database name (recreated)
- ✅ PostgreSQL extensions (reinstalled)
- ✅ Migration files (in hasura/migrations/)
- ✅ Seed files (in db/seeds/)
- ✅ Backup files (in backups/)

## Reset Workflow

### Complete Reset Flow

```
1. Prompt for confirmation
   ↓
2. Create backup (if --backup)
   ↓
3. Disconnect all clients
   ↓
4. DROP DATABASE
   ↓
5. CREATE DATABASE
   ↓
6. Install extensions (uuid-ossp, pgcrypto, etc.)
   ↓
7. Apply migrations (if --no-migrate not set)
   ↓
8. Load seed data (if --no-seed not set)
   ↓
9. Verify database ready
```

## Common Use Cases

### Fresh Start During Development

```bash
# Complete refresh
nself db reset --backup
nself db migrate up
nself db seed
nself restart
```

### Testing Migrations

```bash
# Reset and test migration
nself db reset --force
nself db migrate up
# Verify migration worked correctly
```

### Cleaning Test Data

```bash
# After running tests
npm test
nself db reset --force
nself db seed
```

### Schema Experimentation

```bash
# Try schema changes
nself db reset
nself db migrate create test-schema
# Edit migration
nself db migrate up
# If not working, reset and try again
```

## Safety Features

### Confirmation Prompts

**Development**:
```
Type 'reset' to confirm:
```

**Staging** (with override):
```
🚨 STAGING ENVIRONMENT 🚨
This will delete ALL staging data!
Type 'RESET STAGING' to confirm:
```

### Environment Checks

```bash
# Checks ENV variable
if [[ "$ENV" == "prod" ]]; then
  echo "FATAL: Reset disabled in production"
  exit 1
fi
```

### Automatic Backups

```bash
# In .env
AUTO_BACKUP_BEFORE_RESET=true

# Creates backup automatically
nself db reset
# → Creates backups/pre-reset-TIMESTAMP.sql
```

## Troubleshooting

### Cannot drop database

**Error**:
```
ERROR: database "myapp_db" is being accessed by other users
```

**Solutions**:
```bash
# Stop services first
nself stop

# Reset database
nself db reset

# Restart services
nself start
```

### Permission denied

**Error**:
```
ERROR: must be owner of database myapp_db
```

**Solutions**:
```bash
# Check database owner
nself db shell -c "\l myapp_db"

# Change owner
nself db shell -c "ALTER DATABASE myapp_db OWNER TO $POSTGRES_USER;"

# Then reset
nself db reset
```

### Reset blocked in production

**Error**:
```
✗ FATAL: Reset disabled in production
```

**This is intentional. If you really need to reset production**:
```bash
# Manual procedure (EXTREME CAUTION)
nself db backup full-production-backup-$(date +%Y%m%d).sql

# Manually using psql
psql -U postgres -c "DROP DATABASE myapp_db;"
psql -U postgres -c "CREATE DATABASE myapp_db;"

# Then rebuild
nself db migrate up
# DO NOT seed production data!
```

## Alternatives to Reset

### Rollback Migrations

```bash
# Instead of full reset
nself db migrate down
nself db migrate down
# Fix migration
nself db migrate up
```

### Truncate Tables

```bash
# Delete data, keep schema
nself db shell -c "TRUNCATE users, orders, products CASCADE;"
nself db seed
```

### Restore from Backup

```bash
# Instead of reset
nself db restore backups/known-good-state.sql
```

## Best Practices

### 1. Always Backup Before Reset

```bash
# Even in development
nself db reset --backup
```

### 2. Never Use in Production

```bash
# Production reset is DISABLED
# Use backup/restore instead
nself db backup
nself db restore known-good-backup.sql
```

### 3. Use in Automated Tests

```bash
# test-setup.sh
nself db reset --force
nself db migrate up
nself db seed test-data
```

### 4. Document Reset Necessity

```bash
# In development docs
## When to Reset Database
- After major schema refactoring
- When seed data is corrupted
- To test fresh installation
- Before integration test runs

## When NOT to Reset
- To fix data errors (use SQL instead)
- To test rollback (use migrate down)
- In staging/production (use restore)
```

### 5. Create Reset Workflow Aliases

```bash
# In ~/.bashrc or project scripts
alias db-fresh="nself db reset --force && nself db migrate up && nself db seed"
alias db-clean="nself db reset --force && nself db migrate up"
alias db-test="nself db reset --force && nself db migrate up && nself db seed test-fixtures"
```

## Reset vs Other Commands

| Command | Data | Schema | Use Case |
|---------|------|--------|----------|
| `nself db reset` | ✅ Deletes | ✅ Drops | Fresh start |
| `nself db migrate down` | ❌ Keeps | ⚠️ Rolls back | Undo migration |
| `nself db restore` | ✅ Replaces | ✅ Replaces | Restore backup |
| `TRUNCATE` | ✅ Deletes | ❌ Keeps | Clear data |

## Related Commands

- `nself db backup` - Backup before reset
- `nself db migrate up` - Apply migrations after reset
- `nself db seed` - Load data after reset
- `nself db restore` - Alternative to reset

## See Also

- [Database Management](README.md)
- [Migration Guide](../../guides/MIGRATIONS.md)
- [Backup & Recovery Guide](../../guides/BACKUP-RECOVERY.md)
- [Development Workflows](../../guides/DEV-WORKFLOWS.md)
