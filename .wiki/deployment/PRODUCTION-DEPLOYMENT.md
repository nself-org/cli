# Production Deployment Guide

Complete guide for deploying nself to production environments with automatic SSL, security hardening, and zero-downtime deployments.

## Quick Start

```bash
# 1. Initialize your server (one-time setup)
nself server init root@your-server.com --domain example.com

# 2. Create production environment
nself env create prod prod

# 3. Generate secure secrets
nself config secrets generate --env prod

# 4. Validate before deployment
nself config validate prod

# 5. Deploy to production
nself deploy prod
```

## Server Initialization

The `nself server init` command prepares a fresh VPS for nself deployment:

```bash
nself server init <host> [options]
```

### What It Does

1. **System Update**: Updates packages and installs Docker + Docker Compose
2. **Security Hardening**: Configures firewall (UFW), fail2ban, SSH hardening
3. **nself Environment**: Creates `/var/www/nself` directory structure
4. **DNS Fallback**: Configures reliable DNS resolution (Cloudflare, Google fallback)
5. **SSL Certificates**: Automatic Let's Encrypt setup (if domain points to server)

### Options

| Option | Description |
|--------|-------------|
| `--host, -h` | Server hostname or IP |
| `--user, -u` | SSH user (default: root) |
| `--port, -p` | SSH port (default: 22) |
| `--key, -k` | SSH private key file |
| `--domain, -d` | Domain for SSL setup |
| `--env, -e` | Environment name (default: prod) |
| `--skip-ssl` | Skip SSL certificate setup |
| `--skip-dns` | Skip DNS fallback configuration |
| `--yes, -y` | Skip confirmation prompts |

### Examples

```bash
# Basic server setup
nself server init root@server.example.com

# With domain and custom SSH key
nself server init root@server.example.com --domain example.com --key ~/.ssh/deploy_key

# Non-interactive (for automation)
nself server init root@server.example.com --domain example.com --yes
```

### Supported Providers

- Hetzner Cloud
- DigitalOcean
- Vultr
- Linode
- AWS EC2
- Google Cloud
- Azure
- Any VPS with SSH access

## Secrets Management

Production deployments require secure, randomly-generated secrets.

### Generate Secrets

```bash
nself config secrets generate --env prod
```

This creates `.environments/prod/.env.secrets` with:

- `POSTGRES_PASSWORD` (44 chars)
- `HASURA_GRAPHQL_ADMIN_SECRET` (64 chars)
- `AUTH_JWT_SECRET` (64 chars)
- `REDIS_PASSWORD` (44 chars)
- `MINIO_ROOT_PASSWORD` (44 chars)
- `MEILISEARCH_MASTER_KEY` (44 chars)
- `GRAFANA_ADMIN_PASSWORD` (32 chars)
- `ENCRYPTION_KEY` (32 chars)

### Validate Secrets

```bash
nself config secrets validate --env prod
```

### Rotate Secrets

```bash
# Rotate a single secret
nself config secrets rotate POSTGRES_PASSWORD --env prod

# Rotate all secrets (requires service restart)
nself config secrets rotate --all --env prod
```

### View Secrets

```bash
# Masked view
nself config secrets show --env prod

# Unmask values (use with caution)
nself config secrets show --env prod --unmask
```

## Pre-deployment Validation

Always validate before deploying to production:

```bash
nself config validate prod
```

### What's Validated

1. **Configuration Files**
   - `.env` file exists and is valid
   - `docker-compose.yml` is valid
   - nginx configuration is present
   - SSL certificates exist

2. **Security Settings**
   - All required secrets are set
   - Passwords meet minimum length requirements
   - No insecure default values
   - Services bound to localhost only

3. **Deployment Readiness**
   - Docker is running
   - SSH connectivity to server
   - Git status (uncommitted changes warning)
   - Recent backup exists

### Strict Mode

For production, use strict mode to treat warnings as errors:

```bash
nself config validate prod --strict
```

### Auto-fix

Attempt to automatically fix issues:

```bash
nself config validate prod --fix
```

## Security Pre-flight Checks

When deploying to production (`ENV=prod`), nself automatically runs security checks:

### Blocked if Missing

- `POSTGRES_PASSWORD`
- `HASURA_GRAPHQL_ADMIN_SECRET`
- `AUTH_JWT_SECRET`

### Blocked if Insecure

- Passwords containing: `password`, `changeme`, `secret`, `admin`, `12345`
- Services bound to `0.0.0.0` instead of `127.0.0.1`

### Warned

- Passwords shorter than recommended length
- Let's Encrypt not configured
- Admin services enabled in production

### Force Deploy

To bypass security checks (NOT RECOMMENDED):

```bash
nself deploy production --force
```

## SSL Certificates

nself handles SSL automatically based on environment:

### Development (ENV=dev)

- Uses mkcert for trusted local certificates
- Certificates for `*.localhost` and `*.local.nself.org`
- Run `nself trust` to install root CA

### Staging (ENV=staging)

- Attempts Let's Encrypt staging certificates
- Falls back to mkcert if DNS not configured

### Production (ENV=prod)

- Automatic Let's Encrypt certificates
- Requires DNS pointing to server OR DNS API configured
- Auto-renewal via cron job

### DNS-based SSL (Wildcard Certificates)

For wildcard certificates, configure DNS provider:

```bash
# In .env.prod
DNS_PROVIDER=cloudflare
DNS_API_TOKEN=your-cloudflare-api-token
```

Supported DNS providers:
- `cloudflare` - Cloudflare
- `route53` - AWS Route53
- `digitalocean` - DigitalOcean

### Manual SSL Commands

```bash
# Check SSL status
nself auth ssl status

# Force regenerate certificates
nself auth ssl bootstrap

# Renew Let's Encrypt certificates
nself auth ssl renew
```

## Deployment

### Standard Deployment

```bash
nself deploy production
```

### Dry Run (Preview)

```bash
nself deploy production --dry-run
```

### Rolling Deployment (Zero-downtime)

```bash
nself deploy production --rolling
```

### Skip Health Checks

```bash
nself deploy production --skip-health
```

### Include/Exclude Frontends

```bash
# Include frontends (default for staging)
nself deploy staging --include-frontends

# Exclude frontends (default for production)
nself deploy production --exclude-frontends
```

## Health Checks

After deployment, nself validates:

1. Docker services running
2. nginx responding
3. Database connectivity
4. HTTP endpoints

### Manual Health Check

```bash
nself deploy health --env prod
```

### Detailed Health Report

```bash
nself doctor
```

## Rollback

If deployment fails:

```bash
nself backup rollback --env prod
```

## Backup Before Deploy

Always backup before major deployments:

```bash
# Create full backup
nself backup create

# Create database-only backup
nself backup create database

# Configure cloud backup
nself backup cloud setup
```

## Troubleshooting

### Check Server Readiness

```bash
nself server check root@server.example.com
```

### View Deployment Logs

```bash
nself deploy logs --env prod
```

### Diagnose Issues

```bash
nself doctor
```

### Fix Common Issues

```bash
nself doctor --fix
```

## Environment Configuration

### Directory Structure

```
.environments/
├── staging/
│   ├── .env           # Configuration
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
  "host": "server.example.com",
  "port": 22,
  "user": "root",
  "key": "~/.ssh/deploy_key",
  "deploy_path": "/var/www/nself",
  "project_subdir": ""
}
```

**`deploy_path`** — Where your project lives on the server (default: `/var/www/nself`). Set this if your project is deployed to a custom path (e.g. `/opt/myapp`).

**`project_subdir`** — Optional subdirectory within `deploy_path` where the nself `.env` files and `nself build` should run (used for monorepos). Leave empty for standard deployments.

Example for a monorepo with backend in a subdirectory:

```json
{
  "host": "server.example.com",
  "port": 22,
  "user": "root",
  "key": "~/.ssh/deploy_key",
  "deploy_path": "/opt/myapp",
  "project_subdir": "backend"
}
```

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `ENV` | Environment name (dev/staging/prod) | Yes |
| `PROJECT_NAME` | Docker project prefix | Yes |
| `BASE_DOMAIN` | Base domain for services | Yes |
| `DEPLOY_HOST` | SSH hostname | For deploy |
| `DNS_PROVIDER` | Let's Encrypt DNS provider | For wildcard SSL |
| `DNS_API_TOKEN` | DNS provider API token | For wildcard SSL |

## Best Practices

1. **Always validate before deploying**: `nself config validate prod`
2. **Use separate secrets per environment**: Never share production secrets
3. **Backup before major changes**: `nself backup create`
4. **Use dry-run first**: `nself deploy production --dry-run`
5. **Monitor after deployment**: `nself doctor`
6. **Rotate secrets regularly**: Every 90 days minimum
7. **Keep secrets out of git**: Use `.env.secrets` (gitignored)
