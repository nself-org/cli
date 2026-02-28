# nself db restore

**Category**: Database Commands

Restore your PostgreSQL database from a backup file.

## Overview

Restore a database from a SQL dump file created by `nself db backup` or `pg_dump`. Supports full restores, selective restores, and various safety options.

**Features**:
- ✅ Full database restoration
- ✅ Selective table restoration
- ✅ Safety confirmations (prod/staging)
- ✅ Compressed backup support
- ✅ Automatic decompression
- ✅ Pre-restore validation

## Usage

```bash
nself db restore [OPTIONS] <BACKUP_FILE>
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-t, --table TABLE` | Restore specific table(s) only | all |
| `--clean` | Drop existing objects before restore | false |
| `--create` | Create database if not exists | false |
| `--data-only` | Restore data only (no schema) | false |
| `--schema-only` | Restore schema only (no data) | false |
| `--no-owner` | Skip ownership restoration | false |
| `--verify` | Verify backup before restoring | false |
| `--force` | Skip confirmation prompts | false |
| `-v, --verbose` | Show detailed output | false |

## Arguments

| Argument | Description |
|----------|-------------|
| `BACKUP_FILE` | Required: Path to backup file |

## Examples

### Basic Restore

```bash
nself db restore backups/backup_20260213_143022.sql
```

**Output**:
```
⚠️  WARNING: This will overwrite the current database!

Database: myapp_db
Backup: backups/backup_20260213_143022.sql
Size: 15.3 MB
Created: 2026-02-13 14:30:22

Continue? [y/N]: y

→ Restoring database...
  ⠋ Dropping existing tables...     ✓
  ⠋ Restoring schema...              ✓
  ⠋ Restoring data...                ✓
  ⠋ Creating indexes...              ✓
  ⠋ Applying constraints...          ✓

✓ Database restored successfully (8.3s)

Restored:
  Tables: 24
  Rows: 15,423
  Indexes: 18
```

### Restore Compressed Backup

```bash
nself db restore backups/backup.sql.gz
```

**Automatically detects and decompresses gzip files.**

### Restore with Clean

```bash
nself db restore --clean backups/backup.sql
```

**Behavior**:
- Drops all existing tables first
- Ensures clean slate
- Prevents conflicts from existing data

**Use when**:
- Restoring to a fresh state
- Schema conflicts with existing database
- Complete database reset needed

### Restore Specific Table

```bash
nself db restore --table users backups/full-backup.sql
```

**Output**:
```
→ Restoring table: users
✓ Table restored successfully
  Rows restored: 1,423
```

### Schema-Only Restore

```bash
nself db restore --schema-only backups/schema-backup.sql
```

**Use when**:
- Initializing new environment with structure
- Testing schema changes
- Preparing database for data import

### Data-Only Restore

```bash
nself db restore --data-only backups/data-backup.sql
```

**Use when**:
- Schema already exists
- Refreshing data only
- Copying data between environments

### Force Restore (No Confirmation)

```bash
nself db restore --force backups/backup.sql
```

**⚠️ Dangerous**: Skips all confirmation prompts.

**Use only in**:
- Automated scripts
- CI/CD pipelines
- Development environments

### Verify Before Restore

```bash
nself db restore --verify backups/backup.sql
```

**Checks**:
- File exists and is readable
- File is valid SQL dump
- Backup not corrupted
- Compatible with current PostgreSQL version

**Output**:
```
→ Verifying backup file...
  ✓ File exists: backups/backup.sql
  ✓ File readable: yes
  ✓ Valid SQL dump: yes
  ✓ PostgreSQL version: 15.3 (compatible)
  ✓ Database: myapp_db
  ✓ Tables: 24
  ✓ Estimated size: 15.3 MB

Backup is valid ✓
```

## Restore Strategies

### Development Environment

```bash
# Quick restore without confirmation
nself db restore --force backups/dev-backup.sql
```

### Staging Environment

```bash
# Restore production backup to staging
nself db restore --clean backups/prod-latest.sql

# Verify restoration
nself db shell -c "SELECT COUNT(*) FROM users;"
```

### Production Environment

```bash
# CRITICAL: Always backup current state first
nself db backup emergency-pre-restore.sql

# Restore with maximum safety
nself db restore --verify backups/verified-backup.sql

# Verify restoration
nself health
```

## Point-in-Time Recovery

### Using Timestamped Backups

```bash
# List available backups
ls -lh backups/

# Restore specific point in time
nself db restore backups/backup_20260213_120000.sql
```

### Rollback Workflow

```bash
# Before risky operation
nself db backup pre-operation.sql

# Operation fails
nself db migrate up
# ERROR!

# Rollback to backup
nself db restore --clean backups/pre-operation.sql

# Verify rollback
nself db migrate status
```

## Partial Restore

### Restore Multiple Tables

```bash
nself db restore --table users --table orders backups/backup.sql
```

### Exclude Large Tables

```bash
# Restore everything except logs
# (requires manual pg_restore commands)
pg_restore -d myapp_db -T logs backups/backup.sql
```

### Restore from Remote

```bash
# Download and restore
curl https://backups.example.com/backup.sql.gz | \
  gunzip | \
  nself db restore -

# Or with remote storage
nself backup pull prod-latest.sql.gz
nself db restore prod-latest.sql.gz
```

## Safety Features

### Confirmation Prompts

**Development** (ENV=dev):
```
Continue? [y/N]:
```

**Staging** (ENV=staging):
```
⚠️  STAGING ENVIRONMENT
This will overwrite staging data.
Type 'staging' to confirm:
```

**Production** (ENV=prod):
```
🚨 PRODUCTION ENVIRONMENT 🚨
This will PERMANENTLY OVERWRITE production data!

Type 'RESTORE PRODUCTION' to confirm:
```

### Automatic Pre-Restore Backup

```bash
# In .env
AUTO_BACKUP_BEFORE_RESTORE=true

# Restore creates safety backup first
nself db restore backups/backup.sql
# Creates: backups/pre-restore-20260213-143022.sql
```

## Troubleshooting

### Restore fails with permission error

**Error**:
```
ERROR: permission denied for database myapp_db
```

**Solutions**:
```bash
# Check user permissions
nself db shell -c "\du"

# Grant required permissions
nself db shell -c "ALTER DATABASE myapp_db OWNER TO postgres;"
```

### Table already exists

**Error**:
```
ERROR: relation "users" already exists
```

**Solutions**:
```bash
# Use --clean to drop existing tables
nself db restore --clean backups/backup.sql

# Or manually drop database
nself db shell -c "DROP DATABASE myapp_db;"
nself db shell -c "CREATE DATABASE myapp_db;"
nself db restore backups/backup.sql
```

### Foreign key constraint violations

**Error**:
```
ERROR: insert or update violates foreign key constraint
```

**Solutions**:
```bash
# Restore with constraints temporarily disabled
nself db shell -c "SET session_replication_role = replica;"
nself db restore backups/backup.sql
nself db shell -c "SET session_replication_role = DEFAULT;"
```

### Out of memory during restore

**Error**:
```
ERROR: out of memory
```

**Solutions**:
```bash
# Increase shared_buffers temporarily
# Edit postgresql.conf or use docker-compose environment variable

# Or restore tables one at a time
nself db restore --table users backups/backup.sql
nself db restore --table orders backups/backup.sql
```

### Backup file corrupted

**Error**:
```
ERROR: invalid byte sequence
```

**Solutions**:
```bash
# Verify backup file
file backups/backup.sql

# Try with --verify first
nself db restore --verify backups/backup.sql

# If compressed, decompress manually
gunzip -t backups/backup.sql.gz
```

## Verification After Restore

### Check Row Counts

```bash
nself db shell -c "
SELECT
  schemaname,
  tablename,
  n_live_tup as row_count
FROM pg_stat_user_tables
ORDER BY n_live_tup DESC;
"
```

### Check Indexes

```bash
nself db shell -c "\di"
```

### Check Constraints

```bash
nself db shell -c "
SELECT
  conname,
  contype,
  conrelid::regclass as table_name
FROM pg_constraint
WHERE connamespace = 'public'::regnamespace;
"
```

### Run Application Tests

```bash
# After restore, verify application works
nself start
nself health
# Run integration tests
```

## Best Practices

### 1. Always Backup Before Restore

```bash
nself db backup emergency-backup.sql
nself db restore backups/backup.sql
```

### 2. Verify Backup First

```bash
nself db restore --verify backups/backup.sql
nself db restore backups/backup.sql
```

### 3. Test Restore in Non-Prod First

```bash
# Test on staging
nself env switch staging
nself db restore backups/prod-backup.sql

# Verify works
nself health

# Then restore to production
nself env switch prod
nself db restore backups/prod-backup.sql
```

### 4. Document Restore Procedures

```bash
cat > RESTORE-PROCEDURE.md << 'EOF'
## Emergency Restore Procedure

1. Backup current state
2. Verify backup file
3. Stop services
4. Restore database
5. Verify restoration
6. Start services
7. Run health checks
8. Monitor for issues

## Rollback Plan
If restore fails:
1. Restore from emergency backup
2. Investigate backup file issue
3. Contact team
EOF
```

### 5. Automate Restore Testing

```bash
#!/bin/bash
# test-restore.sh

# Weekly restore test
nself db backup test-backup.sql
nself db restore --verify test-backup.sql

# Restore to temporary database
createdb test_restore_db
pg_restore -d test_restore_db test-backup.sql

# Verify
psql test_restore_db -c "SELECT COUNT(*) FROM users;"

# Cleanup
dropdb test_restore_db
```

## Related Commands

- `nself db backup` - Create backups
- `nself db migrate` - Apply migrations after restore
- `nself db reset` - Complete database reset
- `nself backup list` - List available backups

## See Also

- [Database Management](README.md)
- [nself db backup](backup.md)
- [Backup & Recovery Guide](../../guides/BACKUP-RECOVERY.md)
- [pg_restore Documentation](https://www.postgresql.org/docs/current/app-pgrestore.html)
