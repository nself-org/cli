# nself backup

**Category**: Backup & Recovery Commands

Comprehensive backup and disaster recovery management.

## Overview

All backup and recovery operations use `nself backup <subcommand>` for creating, managing, and restoring backups across all services.

**Features**:
- ✅ Automated backup scheduling
- ✅ Full system backups
- ✅ Point-in-time recovery
- ✅ Off-site backup storage
- ✅ Disaster recovery plans

## Subcommands

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| [create](#nself-backup-create) | Create full backup | Manual backup |
| [restore](#nself-backup-restore) | Restore from backup | Recovery |
| [list](#nself-backup-list) | List all backups | Browse backups |
| [delete](#nself-backup-delete) | Delete backup | Cleanup |
| [schedule](#nself-backup-schedule) | Configure automated backups | Set schedule |
| [verify](#nself-backup-verify) | Verify backup integrity | Testing |
| [export](#nself-backup-export) | Export to external storage | Off-site backup |
| [rollback](#nself-backup-rollback) | Quick rollback | Emergency recovery |

## nself backup create

Create comprehensive backup of entire nself stack.

**Usage**:
```bash
nself backup create [OPTIONS]
```

**Options**:
- `--name NAME` - Backup name/label
- `--compress` - Compress backup
- `--encrypt` - Encrypt backup
- `--remote` - Upload to remote storage
- `--include SERVICES` - Specific services only
- `--exclude SERVICES` - Exclude services

**Backs Up**:
- PostgreSQL database (full dump)
- MinIO storage (all buckets)
- Configuration files (.env, etc.)
- Hasura metadata
- User uploads
- Redis data (if persistence enabled)
- Custom service data

**Examples**:
```bash
# Full backup
nself backup create

# Named backup
nself backup create --name pre-migration-backup

# Compressed and encrypted
nself backup create --compress --encrypt

# Backup to remote storage
nself backup create --remote --compress
```

**Output**:
```
Creating full system backup...

→ Backing up PostgreSQL database...
  ✓ Database dumped (145 MB)

→ Backing up MinIO storage...
  ✓ 3 buckets backed up (2.3 GB)

→ Backing up configuration...
  ✓ Configuration files backed up

→ Backing up Hasura metadata...
  ✓ Metadata exported

→ Compressing backup...
  ✓ Compressed (2.5 GB → 687 MB, 72% reduction)

→ Encrypting backup...
  ✓ Encrypted with AES-256

Backup completed successfully!

Location: backups/backup-20260213-143022.tar.gz.enc
Size: 687 MB (compressed, encrypted)
Duration: 2m 15s

Restore with:
  nself backup restore backups/backup-20260213-143022.tar.gz.enc
```

## nself backup restore

Restore system from backup.

**Usage**:
```bash
nself backup restore <backup_file> [OPTIONS]
```

**Options**:
- `--decrypt` - Decrypt backup (auto-detected)
- `--partial` - Restore specific components
- `--verify` - Verify before restoring
- `--force` - Skip confirmations

**Restore Process**:
1. Verify backup integrity
2. Stop current services
3. Restore database
4. Restore storage
5. Restore configuration
6. Restore metadata
7. Restart services
8. Verify restoration

**Examples**:
```bash
# Full restore
nself backup restore backups/backup-20260213.tar.gz

# Verify first
nself backup restore backups/backup-20260213.tar.gz --verify

# Partial restore (database only)
nself backup restore backups/backup-20260213.tar.gz --partial database

# Force restore (skip confirmations)
nself backup restore backups/backup-20260213.tar.gz --force
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║              System Restore from Backup                   ║
╚═══════════════════════════════════════════════════════════╝

Backup: backup-20260213-143022.tar.gz.enc
Created: 2026-02-13 14:30:22
Size: 687 MB (compressed, encrypted)

⚠️  WARNING: This will overwrite current data!

Components to restore:
  ✓ PostgreSQL database (145 MB)
  ✓ MinIO storage (2.3 GB)
  ✓ Configuration files
  ✓ Hasura metadata

Type 'RESTORE' to confirm:
```

## nself backup list

List all available backups.

**Usage**:
```bash
nself backup list [OPTIONS]
```

**Options**:
- `--remote` - Include remote backups
- `--format FORMAT` - Output format (table/json/csv)
- `--filter DATE` - Filter by date
- `--sort FIELD` - Sort by field

**Examples**:
```bash
# List local backups
nself backup list

# Include remote
nself backup list --remote

# Export to CSV
nself backup list --format csv > backups.csv
```

**Output**:
```
Available Backups

Name                        Created              Size      Type      Location
────────────────────────────────────────────────────────────────────────────────
pre-migration-backup        2026-02-13 14:30    687 MB    Full      Local
weekly-backup-2026-02-10    2026-02-10 00:00    645 MB    Full      Remote (S3)
daily-backup-2026-02-12     2026-02-12 00:00    652 MB    Full      Remote (S3)
manual-backup               2026-02-05 16:45    621 MB    Full      Local

Total: 4 backups (2.6 GB local, 1.3 GB remote)
```

## nself backup delete

Delete old backups.

**Usage**:
```bash
nself backup delete <backup_name> [OPTIONS]
```

**Options**:
- `--remote` - Delete from remote storage
- `--older-than DAYS` - Delete backups older than N days
- `--keep-latest N` - Keep latest N backups
- `--confirm` - Skip confirmation

**Examples**:
```bash
# Delete specific backup
nself backup delete manual-backup

# Delete old backups
nself backup delete --older-than 30

# Keep only latest 10
nself backup delete --keep-latest 10

# Delete from remote
nself backup delete old-backup --remote
```

## nself backup schedule

Configure automated backup schedules.

**Usage**:
```bash
nself backup schedule <action> [OPTIONS]
```

**Actions**:
- `add` - Add backup schedule
- `remove` - Remove schedule
- `list` - List schedules
- `enable` - Enable schedule
- `disable` - Disable schedule

**Examples**:
```bash
# Daily backup at 2 AM
nself backup schedule add daily \
  --time "02:00" \
  --compress \
  --remote

# Weekly backup on Sunday
nself backup schedule add weekly \
  --day sunday \
  --time "00:00" \
  --compress \
  --remote \
  --encrypt

# Hourly backup (last 24 hours)
nself backup schedule add hourly \
  --retain 24 \
  --compress

# List schedules
nself backup schedule list
```

**Schedule Output**:
```
Backup Schedules

ID   Name      Frequency    Time     Retention    Remote    Encrypt    Status
──────────────────────────────────────────────────────────────────────────────
1    daily     Daily        02:00    30 days      Yes       Yes        Active
2    weekly    Weekly       00:00    90 days      Yes       Yes        Active
3    hourly    Hourly       --       24 hours     No        No         Active

Next scheduled backup: daily (in 11 hours 30 minutes)
```

## nself backup verify

Verify backup integrity and restorability.

**Usage**:
```bash
nself backup verify <backup_file> [OPTIONS]
```

**Options**:
- `--full` - Full restoration test
- `--checksum` - Verify checksums
- `--decrypt` - Test decryption

**Checks**:
- File integrity (checksums)
- Compression validity
- Encryption (if applicable)
- Database dump validity
- File structure completeness

**Examples**:
```bash
# Quick verification
nself backup verify backups/backup-20260213.tar.gz

# Full test (restores to temporary environment)
nself backup verify backups/backup-20260213.tar.gz --full

# Checksum only
nself backup verify backups/backup-20260213.tar.gz --checksum
```

**Output**:
```
Verifying backup: backup-20260213-143022.tar.gz.enc

✓ File exists and readable
✓ Checksum valid (SHA256: abc123...)
✓ Encryption valid (AES-256)
✓ Compression valid (gzip)
✓ Database dump structure valid
✓ Storage backup structure valid
✓ Configuration files present
✓ Hasura metadata valid

Backup verification: PASSED

Estimated restore time: 3-5 minutes
Backup is ready for restoration
```

## nself backup export

Export backups to external storage.

**Usage**:
```bash
nself backup export <backup_file> <destination> [OPTIONS]
```

**Supported Destinations**:
- S3 (AWS, MinIO, DigitalOcean Spaces)
- Google Cloud Storage
- Azure Blob Storage
- SFTP/SCP
- rsync

**Examples**:
```bash
# Export to S3
nself backup export backups/backup-20260213.tar.gz \
  s3://my-backups/nself/

# Export to Google Cloud Storage
nself backup export backups/backup-20260213.tar.gz \
  gs://my-backups/nself/

# Export via rsync
nself backup export backups/backup-20260213.tar.gz \
  user@backup-server:/backups/nself/
```

## nself backup rollback

Quick rollback to previous state.

**Usage**:
```bash
nself backup rollback [OPTIONS]
```

**Options**:
- `--to VERSION` - Rollback to specific backup
- `--latest` - Rollback to latest backup (default)
- `--preview` - Preview changes without applying

**Examples**:
```bash
# Rollback to latest backup
nself backup rollback

# Rollback to specific backup
nself backup rollback --to backup-20260210

# Preview rollback
nself backup rollback --preview
```

## Disaster Recovery

### Complete Disaster Recovery

```bash
# 1. Provision new server
nself deploy provision production

# 2. Install nself
curl -sSL https://install.nself.org | bash

# 3. Pull backup from remote
nself backup list --remote
nself backup restore s3://backups/backup-20260213.tar.gz.enc

# 4. Verify services
nself health

# 5. Update DNS
# Point domain to new server
```

### Backup Strategy (3-2-1 Rule)

```
3 copies of data:
  - 1 production (live)
  - 1 local backup
  - 1 remote backup

2 different storage types:
  - Local disk
  - Cloud storage (S3)

1 off-site copy:
  - S3 in different region
```

**Implementation**:
```bash
# Hourly local backups (24 hour retention)
nself backup schedule add hourly --retain 24

# Daily remote backups (30 day retention)
nself backup schedule add daily \
  --remote \
  --compress \
  --encrypt \
  --retain 30

# Weekly off-site backups (90 day retention)
nself backup schedule add weekly \
  --remote s3://backups-us-east/ \
  --compress \
  --encrypt \
  --retain 90
```

## Best Practices

### 1. Regular Automated Backups

```bash
# Daily at 2 AM
nself backup schedule add daily --time "02:00" --remote
```

### 2. Test Backups Monthly

```bash
# Monthly backup verification
nself backup verify --full $(nself backup list --latest)
```

### 3. Keep Multiple Generations

```bash
# 30 daily + 12 weekly + 12 monthly
nself backup schedule add daily --retain 30
nself backup schedule add weekly --retain 12
nself backup schedule add monthly --retain 12
```

### 4. Off-Site Storage

```bash
# Always backup to remote storage
nself backup create --remote --compress --encrypt
```

### 5. Document Recovery Procedures

```bash
# Create recovery runbook
cat > DISASTER-RECOVERY.md << 'EOF'
## Disaster Recovery Procedure

1. Provision new server
2. Install nself
3. Restore latest backup
4. Verify services
5. Update DNS
6. Monitor for 24 hours

Recovery Time Objective (RTO): 2 hours
Recovery Point Objective (RPO): 24 hours
EOF
```

## Related Commands

- `nself db backup` - Database-only backup
- `nself config backup` - Configuration backup
- `nself deploy backup` - Pre-deploy backup

## See Also

- [Backup Strategy Guide](../../guides/BACKUP-STRATEGY.md)
- [Disaster Recovery](../../guides/DISASTER-RECOVERY.md)
- [Off-Site Storage](../../guides/OFFSITE-BACKUP.md)
