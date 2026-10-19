# Guide: Migrating from v1 (Bash CLI) to v2 (Go CLI)

ɳSelf v1 was a Bash-based CLI. v2 is a complete Go rewrite with a new configuration format, Docker naming conventions, and improved safety. This guide walks you through the upgrade process.

For a summary of breaking changes, see [[Upgrading-from-v1]].

## Before You Begin

- **Back up your data.** The migration command does this automatically, but run a manual dump first:
  ```bash
  # On your v1 installation
  nself db dump
  ```
- Know your current `.env` values, especially `POSTGRES_PASSWORD`, `HASURA_GRAPHQL_ADMIN_SECRET`, and `HASURA_JWT_KEY`.

## Breaking Changes

| v1 Env Var | v2 Equivalent | Notes |
|-----------|---------------|-------|
| `NHOST_SUBDOMAIN` | `PROJECT_NAME` | Required |
| `NHOST_REGION` | *(removed)* | No cloud regions in v2 |
| `NHOST_BACKEND_URL` | `BASE_DOMAIN` | Domain only, not full URL |
| `STORAGE_ENABLED` | `MINIO_ENABLED` | |
| `S3_BUCKET` | `MINIO_DEFAULT_BUCKET` | |
| `S3_ACCESS_KEY` | `MINIO_ACCESS_KEY` | |
| `S3_SECRET_KEY` | `MINIO_SECRET_ACCESS_KEY` | |
| `AUTH_SMTP_HOST` | `SMTP_HOST` | |
| `GRAPHQL_JWT_SECRET` | `HASURA_GRAPHQL_JWT_SECRET` | |
| `NHOST_ENV` | `ENV` | Normalised: `development` → `dev` |

Docker resource names also changed:

| Resource | v1 Pattern | v2 Pattern |
|----------|-----------|-----------|
| Network | `nself_{project}` | `{project}_default` |
| Container | `nself_{project}_{service}` | `{project}_{service}_1` |
| Volume | `nself_{project}_{service}` | `{project}_{service}` |

## Migration Steps

### 1. Install ɳSelf v1.0.0 (Go rewrite)

```bash
brew upgrade nself-org/nself/nself
nself version    # should show the latest release
```

### 2. Run the migration command

Navigate to your project directory:

```bash
cd /path/to/my-project

# Dry run first — shows what will change without modifying anything
nself migrate run --dry-run

# Run migration with automatic backup
nself migrate run --backup
```

The migration command:
1. Detects the v1 installation
2. Creates a backup at `~/.nself/migration-backups/{timestamp}/`
3. Stops v1 containers
4. Maps v1 env vars to v2 format
5. Runs `nself build` to generate v2 configs
6. Renames the Docker network
7. Starts v2 services
8. Verifies health

### 3. Verify

```bash
nself health     # all services should be healthy
nself urls       # confirm URLs are correct
```

## Rollback

If anything goes wrong, roll back to v1 using the backup created in step 2:

```bash
nself migrate list                           # list available backups
nself migrate rollback --to-backup 20260328120000
```

Rollback restores your `.env` files, `docker-compose.yml`, and project state, then restarts v1 containers.

## See Also

- [[Upgrading-from-v1]], breaking changes summary
- [[cmd-migrate]], migrate command reference
- [[Installation]], fresh v2 install

---
← [[Home]] | [[_Sidebar]]
