# Guide: Backup and Restore

Protect your data with regular, tested backups.

## Automated Backups (Recommended)

Install the backup plugin for scheduled, automated backups:

```bash
nself plugin install backup
nself build && nself restart
```

Configure schedule and retention in `.env`:

```env
BACKUP_SCHEDULE=0 2 * * *          # daily at 2am
BACKUP_RETENTION_DAYS=30           # keep 30 days
BACKUP_S3_BUCKET=my-backup-bucket  # optional: upload to S3/MinIO
```

## Manual Database Backup

```bash
nself db dump
```

Writes a pg_dump file to `.nself/backups/` with a timestamped filename.

## Backup to S3 / MinIO

Configure in `.env` (or `.env.secrets`):

```env
BACKUP_S3_ENDPOINT=https://s3.amazonaws.com
BACKUP_S3_BUCKET=my-backups
BACKUP_S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
BACKUP_S3_SECRET_KEY=<secret>
```

Compatible with AWS S3, MinIO, Cloudflare R2, Backblaze B2, and any S3-compatible provider.

## Encryption and the backup key

`nself backup stream`, and `nself backup create` with encryption on (`--encrypt`), encrypt with [age](https://age-encryption.org). In v1.5 (`NSELF_V15=1`), a project with no recipient configured gets an identity created once at `~/.config/nself/<project>-age.key`, and restores find it on their own. Set `BACKUP_AGE_RECIPIENTS` to encrypt to a key you keep elsewhere; production should work that way, so the server never holds the key that decrypts its own backups.

**A lost identity makes every backup encrypted to it unrecoverable.** Copy it off the machine right after the first backup, then prove the copy with `nself backup drill --from <remote> --identity <copy>`, which restores into a throwaway container. `nself doctor` reminds you until you mark the copy done with `touch ~/.config/nself/<project>-age.key.backed-up`.

Full rules, lookup order, error codes and the threat model: [[cmd-backup]] (Zero-config encryption key).

## Restore from Backup

```bash
nself db restore .nself/backups/myproject_20260328_020000.sql
```

The restore command:
1. Stops application services (keeps Postgres running)
2. Drops and recreates the target database
3. Restores from the dump file
4. Restarts services

> **Important:** Test your restores regularly. Untested backups are not backups.

## Backup Verification Checklist

- [ ] Backup job runs successfully (check logs: `nself logs backup`)
- [ ] Backup files appear in `.nself/backups/` or S3 bucket
- [ ] Restoration test succeeds on a staging instance
- [ ] Retention policy removes old files as expected

## See Also

- [[plugin-backup]], backup plugin reference
- [[cmd-db]], db command reference
- [[Guide-Production-Deployment]], server setup

---
← [[Home]] | [[_Sidebar]]
