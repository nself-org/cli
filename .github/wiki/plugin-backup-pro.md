# Backup Pro Plugin

> PostgreSQL backup and restore automation with scheduling, multi-target storage, AES-256 encryption, and tracked restore jobs. **Pro plugin, requires license.**

> **Requires:** Basic license tier or higher. `nself license set nself_pro_...`

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install backup
nself build
```

The license is validated server-side against `ping.nself.org/license/validate`. An insufficient tier returns an error and the purchase URL.

## What It Does

Schedules and runs `pg_dump` backups, then streams them directly to S3-compatible storage (AWS, R2, MinIO, B2). Adds production capabilities over the free backup plugin: cron-based scheduling, multiple storage targets, optional pre-compression, AES-256 encryption at rest, and idempotent restore jobs that can resume a failed restore without re-pulling the full artifact.

## Free vs Pro

| Capability | Free `backup` (port 3050) | Pro `backup` (port 3210) |
|------------|---------------------------|--------------------------|
| Scheduled `pg_dump` | Yes | Yes |
| S3 upload | Single bucket | Multi-target (S3, R2, MinIO, B2) |
| Encryption at rest | No | AES-256 |
| Restore tracking | Basic | Idempotent, resumable, tracked |
| Retention policies | Fixed | Configurable |

The free plugin (`plugins/free/backup`, port 3050) handles standard scheduled dumps. This pro variant (`bundles/paid/backup`, port 3210) adds scheduling controls, multi-target storage, encryption, and tracked restores.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Postgres connection string |
| `BACKUP_PLUGIN_PORT` | No | `3210` | Service port override |
| `BACKUP_STORAGE_PATH` | No | `/tmp/nself-backups` | Local staging path |
| `BACKUP_S3_ENDPOINT` | No | — | S3 endpoint URL |
| `BACKUP_S3_BUCKET` | No | — | Target bucket |
| `BACKUP_S3_ACCESS_KEY` | No | — | S3 access key |
| `BACKUP_S3_SECRET_KEY` | No | — | S3 secret key |
| `BACKUP_S3_REGION` | No | — | S3 region |
| `BACKUP_ENCRYPTION_KEY` | No | — | AES-256 encryption key |
| `BACKUP_DEFAULT_RETENTION_DAYS` | No | `30` | Retention period |
| `BACKUP_MAX_CONCURRENT` | No | — | Max concurrent jobs |

Reference vault credentials. Never hardcode secrets.

## Ports

| Port | Purpose |
|------|---------|
| 3210 | Backup pro REST API |

Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly. The free backup plugin uses port 3050.

## Database Tables

4 tables added to your Postgres database (prefix `np_`):

- `np_backup_schedules`, configured backup schedules
- `np_backup_artifacts`, backup artifact index and status
- `np_backup_restore_jobs`, restore job history and progress
- `np_backup_webhook_events`, webhook delivery log

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

```
GET  /health   — Liveness probe
GET  /         — Plugin index / capability list
```

Refer to the plugin's OpenAPI spec under `bundles/paid/backup/` for the full route list.

---

Related: [[plugin-backup]] · [[Plugin-Overview]] · [[Home]]
