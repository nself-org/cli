# nself deploy

Deploy your nself project to remote servers via SSH.

## Usage

```bash
nself deploy <environment> [OPTIONS]
nself deploy <subcommand> [OPTIONS]
```

## Environment Deployment

```bash
# Deploy to staging environment
nself deploy staging

# Deploy to production
nself deploy prod

# Deploy to custom environment
nself deploy my-custom-env

# Preview deployment without executing
nself deploy staging --dry-run

# Skip health checks after deployment
nself deploy prod --skip-health

# Force deployment without confirmation
nself deploy staging --force
```

## Subcommands

| Command | Description |
|---------|-------------|
| `init` | Initialize deployment configuration |
| `ssh` | Legacy SSH deployment |
| `status` | Show deployment status |
| `rollback` | Rollback to previous deployment (delegates to `backup rollback`) |
| `logs` | View deployment logs |
| `webhook` | Setup GitHub webhook for auto-deploy |
| `health` | Check deployment health |
| `check-access` | Verify SSH access to environments |

## Options

| Option | Description |
|--------|-------------|
| `--dry-run` | Preview deployment without executing |
| `--check-access` | Verify SSH connectivity before deploy |
| `--force`, `-f` | Skip confirmation prompts |
| `--rolling` | Use rolling deployment (zero-downtime) |
| `--skip-health` | Skip health checks after deployment |
| `--include-frontends` | Include frontend apps (default for staging) |
| `--exclude-frontends` | Exclude frontend apps (default for production) |
| `--backend-only` | Alias for `--exclude-frontends` |

## Deployment Workflow

When you run `nself deploy <env>`, the following steps are executed:

1. **Local Build** - Runs `nself build` to ensure configs are up-to-date
2. **Create Directories** - Creates remote directory structure
3. **Sync Project Files** - Transfers docker-compose.yml, nginx/, postgres/, services/, monitoring/, ssl/
4. **Sync Environment** - Transfers .env and .env.secrets (merged on server)
5. **Pull Images** - Runs `docker compose pull` on server
6. **Start Services** - Runs `docker compose up -d --force-recreate`
7. **Health Checks** - Verifies services are running

## Files Synced

The deploy command syncs these files/directories:

- `docker-compose.yml` - Service definitions
- `nginx/` - Nginx configs, SSL certs
- `postgres/` - Database init scripts
- `services/` - Custom service code (CS_N)
- `monitoring/` - Prometheus, Grafana configs
- `ssl/certificates/` - SSL certificates
- `.env` - Environment configuration
- `.env.secrets` - Sensitive credentials

## Environment Configuration

Environments are configured in `.environments/<name>/`:

```
.environments/
├── staging/
│   ├── .env           # Environment variables
│   ├── .env.secrets   # Sensitive credentials (chmod 600)
│   └── server.json    # SSH connection details
└── prod/
    ├── .env
    ├── .env.secrets
    └── server.json
```

### server.json Format

```json
{
  "host": "your-server.example.com",
  "port": 22,
  "user": "root",
  "key": "~/.ssh/id_ed25519",
  "deploy_path": "/opt/nself"
}
```

## Service Deployment Scope

### Always Deployed
- **Core Services (4)**: PostgreSQL, Hasura, Auth, Nginx

### Based on *_ENABLED
- **Optional Services (7)**: nself-admin, MinIO, Redis, Functions, MLflow, Mail, Search
- **Monitoring Bundle (10)**: Prometheus, Grafana, Loki, Tempo, etc.
- **Custom Services (CS_N)**: User-defined APIs

### Environment-Based
- **Staging**: Deploys everything including frontend apps
- **Production**: Excludes frontend apps (use `--include-frontends` to override)

Frontend apps in production are typically deployed externally (Vercel, Cloudflare, CDN).

## Examples

### Basic Deployment

```bash
# Create environment
nself env create staging staging

# Configure server connection
# Edit .environments/staging/server.json

# Deploy
nself deploy staging
```

### Preview Deployment

```bash
nself deploy prod --dry-run
```

Output shows:
- Files that will be synced
- Steps that will be executed
- No changes are made

### Check SSH Access

```bash
# Check access to all environments
nself deploy check-access

# Check specific environment before deploying
nself deploy staging --check-access
```

### Zero-Downtime Deployment

```bash
nself deploy prod --rolling
```

Uses rolling updates to restart services one at a time.

### Deploy Backend Only

```bash
# Exclude frontend apps (default for production)
nself deploy prod --backend-only

# Or explicitly
nself deploy staging --exclude-frontends
```

## Health Checks

After deployment, nself checks:
- Docker services running count
- Nginx status
- Container health states

If health checks fail, services may still be starting. Use:

```bash
nself deploy status
nself deploy logs
nself deploy health prod
```

## Rollback

If something goes wrong:

```bash
nself deploy rollback
```

This resets to the previous git commit and restarts services.

## Troubleshooting

### SSH Connection Failed

```bash
# Test connectivity
nself deploy check-access

# Ensure SSH key is added to server
ssh-copy-id -i ~/.ssh/id_ed25519 user@server
```

### Services Not Starting

```bash
# View logs
nself deploy logs

# Check server directly
ssh user@server "cd /opt/nself && docker compose logs"
```

### Nginx Config Errors

The deploy command now:
1. Cleans old nginx configs before regeneration
2. Only generates configs for enabled services
3. Skips frontend configs on Linux servers

If you still have issues, check which services are enabled in your .env.

## Requirements

- SSH access to target server
- Docker and Docker Compose on server
- rsync on local machine (scp used as fallback)

## Rollback (`nself deploy rollback`)

Roll back to a previous deployment state:

```bash
nself deploy rollback latest              # Rollback to latest backup
nself deploy rollback backup <id>         # Rollback to specific backup ID
nself deploy rollback migration 2         # Rollback 2 database migrations
nself deploy rollback deployment          # Rollback to previous deployment
nself deploy rollback --dry-run           # Preview what would be rolled back
```

> **Migration:** If you were using `nself rollback`, update to `nself deploy rollback`.
> The standalone `nself rollback` command is deprecated and will be removed in v1.0.0.

## Related Commands

- [ENV.md](ENV.md) - Environment management
- [PROD.md](PROD.md) - Production configuration
- [STAGING.md](STAGING.md) - Staging environment
- [BACKUP.md](BACKUP.md) - Backup and recovery (deprecated — use db/deploy/infra commands)
