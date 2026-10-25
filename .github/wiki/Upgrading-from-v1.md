# Upgrading from v1

ɳSelf v1.0.0 is a complete rewrite in Go (replacing the legacy Bash CLI). The interface is largely compatible but there are breaking changes you must address before upgrading a v0.x project.

## Breaking Changes

### Commands Removed

| v1 Command | v2 Alternative |
|------------|---------------|
| `nself deploy` | Removed. Use `nself build && nself start` on your server directly, or set up your own CI/CD pipeline. |
| `nself k8s` | Removed. v2 is Docker Compose only. |
| `nself helm` | Removed. |
| `nself provider` | Removed. |
| `nself perf` / `nself bench` | Removed. |
| `nself scale` | Removed. |
| `nself sync` | Removed. Use `nself config export` / `nself config import`. |
| `nself prod` / `nself staging` | Removed. Use `--env prod` / `--env staging` flags. |

### Flag Changes

| Command | v1 Flag | v2 Flag | Notes |
|---------|---------|---------|-------|
| `nself reset` | `--yes` | `--confirm` | Confirms destructive `--hard` reset |
| `nself clean` | `--all` | `--all` | Unchanged but now project-scoped by default |

### `.env` Format Differences

v1 used nHost-style env vars. v2 uses normalized names:

| v1 Variable | v2 Variable |
|-------------|-------------|
| `NHOST_SUBDOMAIN` | `PROJECT_NAME` |
| `NHOST_REGION` | _(dropped , no cloud regions in v2)_ |
| `NHOST_BACKEND_URL` | `BASE_DOMAIN` |
| `STORAGE_ENABLED` | `MINIO_ENABLED` |
| `S3_BUCKET` | `MINIO_DEFAULT_BUCKET` |
| `S3_ACCESS_KEY` | `MINIO_ACCESS_KEY` |
| `S3_SECRET_KEY` | `MINIO_SECRET_ACCESS_KEY` |
| `AUTH_SMTP_HOST` | `SMTP_HOST` |
| `AUTH_SMTP_PORT` | `SMTP_PORT` |
| `AUTH_SMTP_USER` | `SMTP_USER` |
| `AUTH_SMTP_PASS` | `SMTP_PASSWORD` |
| `GRAPHQL_JWT_SECRET` | `HASURA_GRAPHQL_JWT_SECRET` |
| `NHOST_ENV` | `ENV` |

### Docker Resource Naming

Docker networks and containers are named differently in v2:

| Resource | v1 | v2 |
|----------|----|----|
| Network | `nself_{project}` | `{project}_default` |
| Container | `nself_{project}_{service}` | `{project}_{service}_1` |
| Volume | `nself_{project}_{service}` | `{project}_{service}` |

## Migration Steps

### 1. Back Up Your Data

```bash
nself db backup
```

Store the backup file somewhere safe before proceeding.

### 2. Install v2

```bash
brew upgrade nself-org/nself/nself
# or
curl -sSL https://install.nself.org | bash
```

Verify: `nself version` should show `v1.0.9` or later.

### 3. Run the Automated Migration

If you have an existing v1 project directory:

```bash
cd /path/to/your/project
nself migrate run --backup
```

This will:
1. Detect v1 artifacts automatically
2. Create a timestamped backup at `~/.nself/migration-backups/{timestamp}/`
3. Stop v1 containers
4. Translate your `.env` vars to v2 format
5. Regenerate `docker-compose.yml` and Nginx config
6. Start v2 and verify health

### 4. Alternative: Re-initialize

For a clean slate (recommended if the project is not in production):

```bash
nself init --name myapp --domain myapp.dev
nself build
nself start
```

Then restore your data: `nself db restore /path/to/backup.sql`

### Rollback

If migration fails, roll back to v1 state:

```bash
nself migrate rollback --yes
```

Or restore a specific backup:

```bash
nself migrate rollback --backup 20260328-143022 --yes
```

`--yes` confirms the restore without a prompt. From v1.5 a rollback without `--yes` asks on a terminal and is refused with E403 anywhere else (scripts, CI); in v1.4 it still runs but prints a deprecation warning (under `--json`, in `meta.deprecations`).

---

See [[Guide-Migration-from-v1]] for a detailed walkthrough with troubleshooting steps.
