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
## Destination kinds

`backup create --remote`, `backup list --remote`, `backup restore-remote --from` and `backup config` accept three kinds of destination. The kind follows from the URI.

| Kind | URI | Needs |
|---|---|---|
| rclone | `s3://`, `r2://`, `minio://`, `b2://`, `gcs://`, `az://`, or a configured rclone remote such as `s3:bucket/prefix` | `rclone` on PATH |
| path | `path:///mnt/backups` (absolute directory, local or mounted disk) | nothing |
| host | `host://<server>/srv/backups` (a server from `.nself/control-plane.yaml`, absolute directory) | `ssh`, `scp`, a pinned host key |

rclone behaviour is unchanged.

**path://** refuses `..`, a destination directory that is itself a symlink (use the real path), and any symlink below the directory, so a link cannot send a write outside it. A file is written to `<key>.tmp`, fsynced, renamed into place and re-read to compare its sha256. Directories are created `0700`, files `0600`.

**host://** goes through the shared SSH funnel (`sdk/go/remote`): one `ssh` or `scp` argv builder, remote paths limited to `[a-zA-Z0-9/_.-]` with no `..`, and every unsafe name rejected before any connection. The server name must be in the inventory with a `host` (`user@host`); `ssh_key_ref` names an environment variable holding the key path. Host keys are pinned: ssh runs with `StrictHostKeyChecking=yes` against `~/.config/nself/backup_known_hosts` (override with `NSELF_BACKUP_KNOWN_HOSTS`), and the file must hold a key for the alias `nself-ci-<server>`. A server with no pinned key is refused. A file is copied to `<key>.tmp`, its sha256 compared, then moved into place.

```bash
# Back up to a mounted disk, no rclone installed
nself backup create --remote path:///mnt/backups
nself backup list --remote path:///mnt/backups
nself backup restore-remote --from path:///mnt/backups/myproject_full_20261005.dump --yes

# Back up to an inventory server
nself backup create --remote host://backup1/srv/nself-backups
```

`nself backup config` lists the three kinds and marks the one the configured remote selects. `backup stream`, `backup schedule` and the heartbeat remote still take rclone remotes only.

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

A stream backup that fails or uploads zero bytes exits non-zero and deletes the partial object from the remote, so nothing that picks the newest object can select it. If that delete fails, the error says the object is still there and the command still exits non-zero.

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
| `--from` | — | Source URL: rclone remote path, `path://<dir>/<object>` or `host://<server>/<dir>/<object>` |
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

Show backup subsystem status: last run, next scheduled run, retention policy, and, when a heartbeat remote is set, how fresh the newest off-box backup and restore drill are.

```bash
nself backup status [--format json] [--heartbeat-to <remote>] [--max-age 26h] [--max-drill-age 35d] [--project <name>]
```

| Flag | Default | Meaning |
|---|---|---|
| `--heartbeat-to` | `NSELF_BACKUP_HEARTBEAT_REMOTE` | Remote that holds `<project>/backup.json` and `<project>/drill.json` (rclone, `path://` or `host://`). |
| `--max-age` | unset | Exit non-zero with `[E217]` when the newest off-box backup is older (`26h`, `1d12h`), missing, or reports a failed backup. |
| `--max-drill-age` | unset | Exit non-zero with `[E218]` when the newest restore drill is older (`35d`), missing, or reports `result: failed`. |
| `--project` | the current project | Project name that selects the heartbeat objects. With `--heartbeat-to` no project directory is needed and only the `offbox` object is printed (the owner-machine jobs run from `$HOME`). |

The check fails closed. A missing, stale, failed, unparseable, wrong-version, wrong-project or future-dated heartbeat is never OK, and its age is always shown. A heartbeat that cannot be fetched or parsed exits with `[E219]` whether or not a threshold is given. A missing object (rclone exit code 3 or 4, or no such file for `path://` and `host://`) is told apart from an unreachable or misconfigured remote by exit code, never by words in the output: a missing object is `[E217]`/`[E218]` "none found", anything else is `[E219]` with its cause. Every remote read is cut off after 60 seconds.

A remote is `s3://`, `r2://`, `minio://`, `b2://`, `gcs://`, `az://`, `path://`, `host://`, or a configured rclone remote written exactly `name:path`. Anything that could be read as an rclone option or connection string (a leading `-`, `:backend,opts:path`, `file://`, `http(s)://`) is refused before rclone starts. `--max-age` or `--max-drill-age` with no heartbeat remote also gives `[E219]`.

Existing fields are unchanged. `--format json` appends the `offbox` object (contract:cli.backup-status v1); the text form gains two lines, `Off-box backup:` and `Off-box drill:`, only when a heartbeat remote is set.

```json
{"offbox": {"source": "heartbeat", "last_backup": {"...": "backup.json"}, "last_drill": {"...": "drill.json"},
  "backup_age_seconds": 7200, "drill_age_seconds": 1301460, "max_age_exceeded": false, "max_drill_age_exceeded": false,
  "problems": []}}
```

| Field | Meaning |
|---|---|
| `source` | `heartbeat`, or `none` when no heartbeat remote is set |
| `last_backup`, `last_drill` | the heartbeat objects (contract:cli.backup-heartbeat), `null` when none was found |
| `backup_age_seconds`, `drill_age_seconds` | seconds since `at`, `null` when none was found |
| `max_age_exceeded` | `--max-age` was given and the backup is stale, missing or failed |
| `max_drill_age_exceeded` | `--max-drill-age` was given and the drill is stale, missing or failed |
| `problems` | additive: one string per missing, stale, failed or unreadable heartbeat (`"[E219] drill heartbeat is unreadable: ..."`), `[]` when there is none. A JSON-only consumer reads the reason here; the exit status still carries the verdict |

```bash
nself backup status --project nself-web --heartbeat-to r2:nself-backup-heartbeat --max-age 26h --max-drill-age 35d --format json
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

### Off-box drill: `--from`

```bash
nself backup drill --from <remote> [--identity <age key>] [--key <object>] [--heartbeat-to <remote>] [--project <name>] [--json]
```

Restores the newest `<project>_stream_*` backup of a remote (rclone, `path://` or `host://`) into a throwaway `postgres:16-alpine` container, so a backup is proven by a restore and never by its existence. Run it on the owner machine only, the one that holds the age identity. Production never holds a key that decrypts its backups.

| Flag | Default | Meaning |
|---|---|---|
| `--from` | none | Remote that holds the backups. Selects the off-box drill. |
| `--identity` | `~/.config/nself/<project>-backup-age.key`, then `age-key.txt` | age identity file that decrypts the object. Only its path is passed to `age`. |
| `--key` | newest by name | Object to drill instead of the newest. |
| `--heartbeat-to` | `NSELF_BACKUP_HEARTBEAT_REMOTE` | Remote that receives `<project>/drill.json`. Without one nothing is written. |
| `--project` | the current project | Project name for the object prefix and heartbeat keys. With `--from` no project directory is needed. |

What it does:

1. Preflight: Docker reachable, `age` installed for `.age` objects, free disk of twice the download size. Any miss exits with `[E220]` and writes no `drill.json`.
2. Downloads the object through the destination interface and decrypts it into a private (`0700`) temp directory, removed on every exit.
3. Starts a throwaway container: random name `nself-drill-<16 hex>` and label `org.nself.drill`, random password passed through the environment (never printed, never in argv), no published port and no network. It never touches the project database or containers; removal refuses any name that is not a throwaway.
4. Restores by content, not extension: a file that starts with `PGDMP` is a custom-format archive and goes through `pg_restore --no-owner --no-acl`; anything else is plain SQL and goes through `psql`. Stream objects are custom-format archives named `.sql`.
5. Counts rows per table (`count(*)`) and compares them with the source counts in `<project>/backup.json` (`approx_rows`): a table the backup counted non-empty must have rows, and a table may fall short of its count by at most 10% (the counts are `n_live_tup` estimates). Without source counts the drill cannot verify: no `--heartbeat-to`, no readable `backup.json`, or a `backup_key` that is not the object restored (the drill warns) all fail with `[E218]` "cannot verify" and `result: failed`. It never passes on "some table has rows". Every `pg_restore` line that contains `error:` also fails the drill, unless it is on the (currently empty) allowlist in `internal/backup/drill_verify.go`.
6. Interrupts: SIGINT and SIGTERM stop the drill, remove the container and the decrypted files, and write no `drill.json`. After a SIGKILL or a crash, the next drill run starts by removing drill containers (label `org.nself.drill`, throwaway name only) and `nself-drill-*` temp directories older than 6 hours, owned by the same user. Other containers are never touched. The remote download is bounded (60 seconds plus one second per MiB of the backup).
7. Writes `<project>/drill.json` (`kind: drill`, `restored_rows`, `mismatches`, `result: ok` or `failed`), then removes the container. A failed drill still writes it, and exits with `[E218]`.

| Code | Meaning |
|---|---|
| E217 | Off-box backup stale, missing or failed (`backup status --max-age`) |
| E218 | Restore drill stale, missing or failed (`backup status --max-drill-age`), or a drill that did not match its backup |
| E219 | Heartbeat unreadable, or no heartbeat remote for a threshold |
| E220 | Drill cannot start: Docker, `age` or free disk (exit class 2 under `NSELF_V15=1`) |

`--file` and `--dry-run` do not apply with `--from`. Without `--from` the drill behaves as before, as described below.

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
