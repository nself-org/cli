# nself backup

<!-- BEGIN PROSE:summary -->
> Backup operations: create, list, restore, verify, prune, config, status, init-key.
<!-- END PROSE:summary -->

## Synopsis

```
nself backup <subcommand> [flags]
```

## Description

<!-- BEGIN PROSE:description -->
Backup, restore, verify, and schedule ɳSelf project data.

---
## backup stream

Stream a live backup to S3, R2, Backblaze B2, GCS, or Azure Blob. No temp files written.

### Pipeline

```
pg_dump (streaming) | age (encrypt) | rclone rcat (multipart upload)
```

### Usage

```bash
nself backup stream --to <url> [--recipient <key>] [--heartbeat-to <remote>] [--heartbeat-required] [--dry-run]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--to` | `NSELF_BACKUP_DESTINATION` | Destination URL (rclone remote path) |
| `--recipient` | `NSELF_BACKUP_RECIPIENT` | Encryption recipient: age key, SSH key, or `github:<user>` (repeatable) |
| `--heartbeat-to` | `NSELF_BACKUP_HEARTBEAT_REMOTE` | rclone remote that receives `<project>/backup.json` after a successful upload |
| `--heartbeat-required` | false | Exit non-zero when the heartbeat cannot be written. The backup itself is kept either way |
| `--dry-run` | false | Preview without running |

### Heartbeat (contract:cli.backup-heartbeat v1)

With a heartbeat remote set, a successful upload is followed by one small object, `<project>/backup.json`, on that remote. It lets freshness be checked off the box. It is written only after the backup upload succeeded; a failed backup writes nothing. A heartbeat failure is a warning unless `--heartbeat-required` is set. Use a separate remote or bucket for heartbeats.

The object is JSON with 2-space indent, sorted keys and a trailing newline:

| Field | Type | Value |
|---|---|---|
| `schema_version` | string | `"1"` |
| `kind` | string | `"backup"` |
| `project` | string | project name |
| `at` | string | RFC 3339 UTC time the upload finished |
| `result` | string | `"ok"` (a failed backup writes nothing) |
| `backup_key` | string | object key of the backup, without the remote |
| `bytes` | int | size of the uploaded (encrypted) object |
| `encrypted` | bool | age recipients were used |
| `cli_version` | string | version of the writing binary |
| `approx_rows` | object or null | `{"<schema>.<table>": n_live_tup}` from `pg_stat_user_tables`, read just before the dump. `null` when the estimate could not be read (needs `psql` on PATH) |
| `restored_rows` | null | drill only |
| `mismatches` | string[] | drill only; `[]` for a backup |

The object never holds a secret, hostname or bucket name. Resumed backups (`backup resume`) do not write a heartbeat.

### Examples

```bash
# Stream encrypted backup to S3
nself backup stream --to s3:mybucket/backups --recipient age1abc123

# Use GitHub SSH keys for encryption
nself backup stream --to r2:mybucket/backups --recipient github:myusername

# Use env-configured destination (no flags needed)
nself backup stream

# Dry run to confirm destination
nself backup stream --to b2:mybucket --dry-run

# Write the freshness heartbeat to a separate remote after the upload
nself backup stream --to r2:mybucket/backups --recipient age1abc123 --heartbeat-to r2hb:heartbeats
```

### Requirements

- `pg_dump` on PATH
- `rclone` on PATH (configured with target remote)
- `age` on PATH (only required when encryption recipients are specified)
- `psql` on PATH (only used to read `approx_rows` when a heartbeat remote is set; without it `approx_rows` is `null`)

### Environment variables

| Variable | Description |
|---|---|
| `NSELF_BACKUP_DESTINATION` | Default destination URL |
| `NSELF_BACKUP_RECIPIENT` | Default age/SSH public key (space-separated for multiple) |
| `NSELF_BACKUP_HEARTBEAT_REMOTE` | Default heartbeat remote (the `--heartbeat-to` flag wins). Environment only: `nself.yaml` has no config keys. |
| `NSELF_BACKUP_CHUNK_MB` | Multipart chunk size in MB (default: 64, handled by rclone) |
| `AWS_ACCESS_KEY_ID` | S3/R2/B2 access key |
| `AWS_SECRET_ACCESS_KEY` | S3/R2/B2 secret key |

---

## backup restore-remote

Restore a backup directly from a remote URL. No local disk space required.

### Pipeline

```
rclone cat <from> | age --decrypt | pg_restore
```

### Usage

```bash
nself backup restore-remote --from <url> [--key <identity-file>] [--yes]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--from` | — | Source URL (rclone remote path) |
| `--key` | `~/.config/nself/age-key.txt` | Path to age identity file |
| `--yes` | false | Skip confirmation on production |

### Examples

```bash
# Restore encrypted backup from S3
nself backup restore-remote --from s3:mybucket/backups/myproject_stream_20260423.sql.age \
  --key ~/.config/nself/age-key.txt

# Restore unencrypted backup
nself backup restore-remote --from r2:mybucket/backups/myproject_stream_20260423.sql
```

---

## backup resume

Resume a previously interrupted streaming backup.

Since rclone `rcat` uploads are not resumable at the protocol level, resume re-streams the full backup and overwrites the partial remote object at the same key.

### Usage

```bash
nself backup resume <backup-id>
```

Resume state is stored in `~/.nself/backup-state/<id>.json`.

---

## backup schedule

Install a systemd timer to run `nself backup stream` on a cron schedule.

### Usage

```bash
nself backup schedule --cron "0 2 * * *" --to <url> [--recipient <key>]... [--heartbeat-to <remote>] [--env-file <path>] [--dry-run]
```

Run it from the project directory (it must hold a `.env` or `nself.yaml`; anywhere else the command refuses). The unit runs in that directory (`WorkingDirectory`) and calls the absolute path of the `nself` binary that is running now (symlinks resolved), so a different `nself` on PATH can never be picked up. The unit has no `EnvironmentFile` line unless `--env-file` is given; the project `.env` is read from the working directory as usual.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--cron` | — | Cron expression (e.g. `0 2 * * *`) |
| `--to` | `NSELF_BACKUP_DESTINATION` | Destination URL |
| `--recipient` | — | Encryption recipient: age key, SSH key, or `github:<user>`. Repeatable; each one is written into the unit |
| `--heartbeat-to` | — | Heartbeat remote written into the unit as `--heartbeat-to` (see [backup stream](#backup-stream)) |
| `--env-file` | — | Absolute path of an environment file for the unit. No default: without it the unit has no `EnvironmentFile` line |
| `--unit-dir` | `/etc/systemd/system` | Systemd unit directory |
| `--dry-run` | false | Print unit files without writing |

### Examples

```bash
# Schedule nightly encrypted backup at 02:00 UTC
nself backup schedule --cron "0 2 * * *" --to s3:mybucket/backups --recipient age1abc123

# Preview the systemd units
nself backup schedule --cron "0 2 * * *" --to r2:mybucket --dry-run

# Two recipients and a heartbeat remote, from the project directory
nself backup schedule --cron "30 2 * * *" --to r2:mybucket/backups --recipient age1abc123 --recipient age1def456 --heartbeat-to r2hb:heartbeats
```

### Status

After scheduling, check the timer with:

```bash
nself backup status
systemctl status nself-backup-stream.timer
```

---

## backup create

Create a local backup (written to `BACKUP_DIR`, default `./backups`).

```bash
nself backup create [--type full|wal|metadata|minio|all] [--encrypt] [--tag <label>] [--dry-run]
```

---

## backup restore

Restore from a local backup file.

```bash
nself backup restore <backup-id|latest> [--only pg,minio,metadata] [--decrypt-key <file>] [--yes]
```

---

## backup verify

Verify backup integrity, optionally running a restore test in a temporary container.

```bash
nself backup verify <backup-id|latest> [--restore-test] [--cleanup] [--keep]
```

---

## backup list

List backups in the local backup directory.

```bash
nself backup list [--remote <name>] [--since 24h] [--format table|json]
```

---

## backup prune

Remove backups beyond the retention policy.

```bash
nself backup prune [--keep-daily 7] [--keep-weekly 4] [--keep-monthly 12] [--dry-run]
```

---

## backup status

Show backup subsystem status: last run, next scheduled run, retention policy.

```bash
nself backup status [--format json]
```

---

## backup drill

Restore the most recent backup into a scratch database, measure how long the
restore took against an RTO target, and smoke-check the result.

```bash
nself backup drill [--file <backup>] [--rto-hours 4] [--dry-run] [--json]
```

| Flag | Default | Meaning |
|---|---|---|
| `--file` | most recent backup | Backup file to drill against. |
| `--rto-hours` | `4.0` | RTO target in hours. `0` disables the gate. |
| `--dry-run` | `false` | Validate inputs and exit without restoring. |
| `--json` | `false` | Emit the drill result as JSON on stdout. |

### Critical tables

After the restore, the drill checks that a list of critical tables exists by
name in the scratch database. The list comes from `BACKUP_CRITICAL_TABLES`
(comma-separated) when set, and otherwise falls back to the built-in
`np_`-prefixed default: `np_users`, `np_licenses`, `np_audit_log`,
`np_plugins`, `np_billing`.

Two things about this check are worth knowing before you rely on it:

- It tests **presence only**. A table that exists but holds no rows counts as
  present. The drill does not compare row counts against the source database.
- A missing table does **not** fail the drill. The verdict stays `PASS` and the
  missing names are listed as an advisory. Treat the drill as a restore-path
  smoke test, not as a data-completeness gate.

Deployments whose schema does not use the `np_` prefix will see every default
name reported missing until they set `BACKUP_CRITICAL_TABLES`.

## backup init-key

Generate an age encryption keypair for backup encryption.

```bash
nself backup init-key
```

Outputs the public key to add to `.env` as `BACKUP_AGE_RECIPIENTS`.

---

## Related

- [[cmd-pitr]], point-in-time recovery via WAL archiving
- [[Home]]
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
| `config` | View backup configuration |
| `create` | Create a new backup |
| `drill` | Run a DR drill: restore latest backup into scratch DB and measure RTO |
| `init-key` | Generate age encryption keypair for backups |
| `list` | List available backups |
| `pitr` | Point-in-time recovery: enable, disable, status, base-backup, restore |
| `prune` | Remove old backups by retention policy |
| `restore` | Restore from a backup |
| `restore-remote` | Restore a backup directly from a remote URL |
| `resume` | Resume an interrupted streaming backup |
| `schedule` | Schedule recurring streaming backups via systemd timers |
| `status` | Show backup subsystem status |
| `stream` | Stream an encrypted backup directly to a remote destination |
| `verify` | Verify backup integrity |
<!-- END GENERATED:subcommands -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Create a local backup
nself backup create

# List available backups
nself backup list

# Stream an encrypted backup straight to S3, no temp files
nself backup stream --to s3:mybucket/backups --recipient age1abc123

# Check backup subsystem status: last run, next scheduled, retention
nself backup status
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
