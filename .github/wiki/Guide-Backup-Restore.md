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

## Destinations for stream, schedule and the heartbeat

`nself backup stream`, `nself backup schedule` and the heartbeat (`--heartbeat-to`) accept the same destinations as `nself backup create`:

- an rclone remote: `name:path`, or `s3://` `r2://` `minio://` `b2://` `gcs://` `az://`
- a local or mounted directory: `path://<absolute dir>`
- a server from `.nself/control-plane.yaml`: `host://<server>/<absolute dir>` (over SSH, with a pinned host key)

Values that rclone could read as an option (`-x`, `--config=...`), connection strings (`:s3,key=...:bucket`) and other URL schemes (`file://`, `http://`) are refused before anything runs.

Credentials never go in the destination, and that includes userinfo in a scheme URI: `s3://user@bucket` and `s3://KEY:SECRET@bucket/path` are refused on purpose, because no bucket name can hold `@` or `:`. `backup schedule` writes `--to` and `--heartbeat-to` into a systemd unit, so it refuses a value that carries inline options or credentials (`:s3,access_key_id=...:bucket`, `s3://KEY:SECRET@bucket/path`). Use a named remote from `rclone.conf` (`rclone config`), or define the remote in the environment with `RCLONE_CONFIG_<NAME>_TYPE` and its keys, put those lines in a file and pass it with `backup schedule --env-file`, then give the destination as `<name>:<path>`. The service unit is written with mode 0600.

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

## Retention and Prune Holds

`nself backup prune` applies a tiered retention policy: it keeps the newest backup of each of the last N days, N weeks (Sundays) and N months (the 1st). The defaults are 7 daily, 4 weekly and 12 monthly. Set them with `BACKUP_RETENTION_DAILY`, `BACKUP_RETENTION_WEEKLY` and `BACKUP_RETENTION_MONTHLY`, or per run with `--keep-daily`, `--keep-weekly` and `--keep-monthly`. `BACKUP_RETENTION_DAYS` is a legacy setting that prune does not read. Preview a run with `nself backup prune --dry-run`.

Prune never deletes a backup whose tag is `keep` or starts with `keep-`. The `update stack` command tags its pre-update backup `keep-stack-<run-id>-<nonce>` so an interrupted or failed update can always roll back; those backups survive prune until you release them.

The tag is the part of the file name after the timestamp, for example `myproj_full_20261028_120000_keep-stack-<run-id>-<nonce>.dump`. There is no command to change a tag. To release a held backup, rename the file in your backup directory so the tag no longer starts with `keep` (keep the extension, including `.age` on encrypted backups), then prune:

```bash
mv myproj_full_20261028_120000_keep-stack-abc123.dump myproj_full_20261028_120000_released-stack-abc123.dump
nself backup prune --dry-run   # the renamed backup is now subject to the policy
```

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
