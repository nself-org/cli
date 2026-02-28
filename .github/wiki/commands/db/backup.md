# nself db backup

**Category**: Database Commands

Create a complete backup of your PostgreSQL database.

## Overview

Backup your database to a SQL dump file that can be restored later. Supports full database backups, specific schema backups, and automated backup strategies.

**Features**:
- ✅ Full database dump
- ✅ Schema-only or data-only backups
- ✅ Compressed backups (gzip)
- ✅ Automatic timestamping
- ✅ Environment-aware (dev/staging/prod)
- ✅ Remote backups (to S3, MinIO, etc.)

## Usage

```bash
nself db backup [OPTIONS] [FILENAME]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-o, --output FILE` | Output filename | auto-generated |
| `-c, --compress` | Compress with gzip | false |
| `--schema-only` | Backup schema without data | false |
| `--data-only` | Backup data without schema | false |
| `--table TABLE` | Backup specific table(s) | all |
| `--remote` | Upload to remote storage | false |
| `-v, --verbose` | Show detailed output | false |

## Arguments

| Argument | Description |
|----------|-------------|
| `FILENAME` | Optional: Custom backup filename |

## Examples

### Basic Backup

```bash
nself db backup
```

**Output**:
```
→ Creating database backup...
  Database: myapp_db
  Size: 15.3 MB

✓ Backup created: backups/backup_20260213_143022.sql
```

### Named Backup

```bash
nself db backup before-migration.sql
```

**Output**:
```
→ Creating database backup...
✓ Backup created: backups/before-migration.sql
```

### Compressed Backup

```bash
nself db backup --compress
```

**Output**:
```
→ Creating compressed backup...
  Database: myapp_db
  Original size: 152.7 MB
  Compressed: 18.3 MB (88% reduction)

✓ Backup created: backups/backup_20260213_143022.sql.gz
```

### Schema-Only Backup

```bash
nself db backup --schema-only schema-backup.sql
```

**Use when**:
- Backing up database structure
- Version controlling schema
- Migrating schema to new environment

**Output**:
```
→ Backing up schema only...
✓ Backup created: backups/schema-backup.sql
  (no data included)
```

### Data-Only Backup

```bash
nself db backup --data-only data-backup.sql
```

**Use when**:
- Preserving data before schema changes
- Copying data to different schema version
- Testing with production-like data

### Backup Specific Table

```bash
nself db backup --table users users-backup.sql
```

**Output**:
```
→ Backing up table: users
✓ Backup created: backups/users-backup.sql
  Rows: 1,423
```

### Multiple Tables

```bash
nself db backup --table users --table orders multi-table-backup.sql
```

### Remote Backup

```bash
nself db backup --remote --compress
```

**Output**:
```
→ Creating compressed backup...
✓ Backup created: backups/backup_20260213_143022.sql.gz

→ Uploading to remote storage...
  Destination: s3://backups/myapp/backup_20260213_143022.sql.gz
✓ Upload complete
```

## Backup Location

### Default Directory

```
backups/
├── backup_20260213_143022.sql
├── backup_20260213_120000.sql
├── before-migration.sql
└── pre-deploy.sql.gz
```

### Custom Directory

```bash
BACKUPS_DIR=/path/to/backups nself db backup
```

## Automated Backups

### Daily Backup (Cron)

```bash
# Crontab entry for daily backup at 2 AM
0 2 * * * cd /path/to/project && nself db backup --compress --remote
```

### Pre-Deployment Backup

```bash
#!/bin/bash
# deploy.sh

# Always backup before deployment
nself db backup pre-deploy-$(date +%Y%m%d).sql

# Deploy
git pull
nself db migrate up
nself restart
```

### Backup on Schedule

```bash
# Weekly full backup
0 3 * * 0 nself db backup --compress weekly-backup.sql.gz

# Daily incremental backup
0 3 * * 1-6 nself db backup --data-only daily-backup.sql
```

## Backup Strategies

### Development

```bash
# Before major changes
nself db backup before-refactor.sql

# After successful changes
nself db backup after-refactor.sql
```

### Staging

```bash
# Daily backups with 7-day retention
nself db backup --compress staging-$(date +%A).sql.gz

# Before deploying to production
nself db backup pre-prod-deploy-$(date +%Y%m%d).sql.gz
```

### Production

```bash
# Hourly backups (last 24 hours)
nself db backup --compress --remote prod-hourly-$(date +%H).sql.gz

# Daily backups (last 30 days)
nself db backup --compress --remote prod-daily-$(date +%Y%m%d).sql.gz

# Weekly backups (last 12 weeks)
nself db backup --compress --remote prod-weekly-$(date +%Y-W%W).sql.gz

# Monthly backups (permanent)
nself db backup --compress --remote prod-monthly-$(date +%Y-%m).sql.gz
```

## Backup Verification

### Test Restore

```bash
# Create backup
nself db backup test-backup.sql

# Test restore to temporary database
nself db restore test-backup.sql --target temp_db

# Verify
nself db shell -c "SELECT COUNT(*) FROM temp_db.users;"

# Clean up
nself db shell -c "DROP DATABASE temp_db;"
```

### Checksum Verification

```bash
# Create backup with checksum
nself db backup mybackup.sql
sha256sum backups/mybackup.sql > backups/mybackup.sql.sha256

# Verify later
sha256sum -c backups/mybackup.sql.sha256
```

## Remote Storage

### S3 Configuration

```bash
# In .env
BACKUP_REMOTE_TYPE=s3
BACKUP_S3_BUCKET=my-backups
BACKUP_S3_REGION=us-east-1
BACKUP_S3_PREFIX=myapp/

# Backup
nself db backup --remote
```

### MinIO Configuration

```bash
# In .env
BACKUP_REMOTE_TYPE=minio
BACKUP_MINIO_ENDPOINT=minio.example.com
BACKUP_MINIO_BUCKET=backups
BACKUP_MINIO_ACCESS_KEY=...
BACKUP_MINIO_SECRET_KEY=...

# Backup
nself db backup --remote
```

### Custom Remote Command

```bash
# In .env
BACKUP_REMOTE_COMMAND="rsync -avz $BACKUP_FILE user@backup-server:/backups/"

# Backup
nself db backup --remote
```

## Backup Retention

### Automatic Cleanup

```bash
# In .env
BACKUP_RETENTION_DAYS=30

# Cleanup old backups
nself backup clean
```

### Manual Cleanup

```bash
# Remove backups older than 30 days
find backups/ -name "*.sql" -mtime +30 -delete

# Remove backups older than 7 days (compressed only)
find backups/ -name "*.sql.gz" -mtime +7 -delete
```

## Troubleshooting

### Backup fails with permission error

**Error**:
```
pg_dump: error: permission denied for database
```

**Solutions**:
```bash
# Check user permissions
nself db shell -c "\du"

# Grant backup privileges
nself db shell -c "GRANT CONNECT ON DATABASE myapp_db TO backup_user;"
```

### Out of disk space

**Error**:
```
pg_dump: error: could not write to file: No space left on device
```

**Solutions**:
```bash
# Check disk space
df -h

# Use compression
nself db backup --compress

# Backup to remote storage
nself db backup --remote

# Clean old backups
nself backup clean
```

### Backup takes too long

**Symptoms**: Backup running for hours on large database.

**Solutions**:
```bash
# Use parallel dump (PostgreSQL 9.3+)
pg_dump -j 4 -Fd -f backups/parallel-backup myapp_db

# Backup specific tables only
nself db backup --table large_table

# Exclude large tables
pg_dump --exclude-table-data=logs myapp_db > backups/no-logs.sql
```

### Cannot connect to database

**Error**:
```
pg_dump: error: connection to database failed
```

**Solutions**:
```bash
# Check database is running
nself status postgres

# Check connection details
grep POSTGRES .env

# Test connection
nself db shell -c "SELECT 1"
```

## Best Practices

### 1. Always Backup Before Major Changes

```bash
# Before migrations
nself db backup pre-migration.sql
nself db migrate up

# Before deployments
nself db backup pre-deploy.sql
./deploy.sh
```

### 2. Use Compression for Large Databases

```bash
# Saves disk space and transfer time
nself db backup --compress
```

### 3. Test Your Backups

```bash
# Regularly verify backups can be restored
nself db restore test-backup.sql --verify
```

### 4. Multiple Backup Locations

```bash
# Local + remote
nself db backup mybackup.sql
nself db backup --remote
```

### 5. Document Backup Strategy

```bash
# Create backup documentation
cat > BACKUP-STRATEGY.md << 'EOF'
## Backup Schedule
- Hourly: Last 24 hours
- Daily: Last 30 days
- Weekly: Last 12 weeks
- Monthly: Permanent

## Restoration Testing
- Weekly restoration test to staging
- Monthly full disaster recovery drill
EOF
```

## Related Commands

- `nself db restore` - Restore from backup
- `nself db migrate` - Apply migrations (backup before!)
- `nself backup clean` - Clean old backups
- `nself backup list` - List available backups

## See Also

- [Database Management](README.md)
- [nself db restore](restore.md)
- [Backup Guide](../../guides/BACKUP-RECOVERY.md)
- [pg_dump Documentation](https://www.postgresql.org/docs/current/app-pgdump.html)
