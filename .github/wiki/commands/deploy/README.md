# nself deploy

**Category**: Deployment Commands

Manage deployments to staging and production environments.

## Overview

All deployment operations use `nself deploy <subcommand>` for deploying to remote servers, managing releases, and infrastructure provisioning.

**Features**:
- ✅ Automated deployment workflows
- ✅ Zero-downtime deployments
- ✅ Rollback support
- ✅ Environment protection
- ✅ Server provisioning
- ✅ Release management

## Subcommands

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| [staging](#nself-deploy-staging) | Deploy to staging | Test before production |
| [production](#nself-deploy-production) | Deploy to production | Live deployment |
| [upgrade](#nself-deploy-upgrade) | Upgrade deployment | Update nself version |
| [server](#nself-deploy-server) | Server management | Provision/manage servers |
| [provision](#nself-deploy-provision) | Provision infrastructure | Setup new servers |
| [sync](#nself-deploy-sync) | Sync files to server | Push code/config |
| [release](#nself-deploy-release) | Create release package | Package for deployment |
| [protect](#nself-deploy-protect) | Environment protection | Prevent accidents |
| [rollback](#nself-deploy-rollback) | Rollback deployment | Undo deployment |
| [status](#nself-deploy-status) | Deployment status | Check deploy state |
| [history](#nself-deploy-history) | Deployment history | View past deploys |
| [logs](#nself-deploy-logs) | Deployment logs | View deploy logs |
| [verify](#nself-deploy-verify) | Verify deployment | Post-deploy checks |
| [health](#nself-deploy-health) | Health check | Service health |
| [config](#nself-deploy-config) | Deploy configuration | Manage deploy settings |
| [secrets](#nself-deploy-secrets) | Sync secrets | Push secrets securely |
| [backup](#nself-deploy-backup) | Pre-deploy backup | Safety backup |
| [restore](#nself-deploy-restore) | Restore deployment | Recovery |
| [remote](#nself-deploy-remote) | Remote server actions | Execute on server |
| [ssh](#nself-deploy-ssh) | SSH to server | Connect to server |
| [tunnel](#nself-deploy-tunnel) | Create SSH tunnel | Secure connection |
| [init](#nself-deploy-init) | Initialize deployment | First-time setup |
| [validate](#nself-deploy-validate) | Validate before deploy | Pre-deploy check |

## Quick Start

### First-Time Deployment Setup

```bash
# 1. Initialize deployment configuration
nself deploy init --env staging

# 2. Configure server
nself deploy config set staging --host 167.235.233.65 --user root

# 3. Provision server
nself deploy provision staging

# 4. Deploy
nself deploy staging
```

## nself deploy staging

Deploy to staging environment.

**Usage**:
```bash
nself deploy staging [OPTIONS]
```

**Options**:
- `--skip-backup` - Skip pre-deploy backup
- `--skip-tests` - Skip pre-deploy tests
- `--force` - Force deploy (skip confirmations)
- `--dry-run` - Show what would be deployed

**Workflow**:
1. Run pre-deploy checks
2. Create backup
3. Sync code and configuration
4. Run database migrations
5. Restart services
6. Run health checks
7. Verify deployment

**Example**:
```bash
nself deploy staging
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║         Deploying to Staging Environment                  ║
╚═══════════════════════════════════════════════════════════╝

Pre-Deployment Checks
──────────────────────────────────────────────────────────
✓ Configuration valid
✓ Server accessible (167.235.233.65)
✓ Git repository clean
✓ All tests passing

Creating Backup
──────────────────────────────────────────────────────────
✓ Database backed up: staging-pre-deploy-20260213.sql
✓ Configuration backed up

Deploying
──────────────────────────────────────────────────────────
→ Syncing code...              ✓ (15 files)
→ Syncing configuration...     ✓
→ Running migrations...         ✓ (3 applied)
→ Restarting services...       ✓
→ Running health checks...     ✓

Post-Deployment
──────────────────────────────────────────────────────────
✓ All services healthy
✓ GraphQL API responding
✓ Auth service responding

✓ Deployment successful!

URL: https://staging.example.com
Time: 2m 15s
```

## nself deploy production

Deploy to production environment.

**Usage**:
```bash
nself deploy production [OPTIONS]
```

**Options**:
- `--confirm` - Explicitly confirm production deploy
- `--skip-backup` - Skip pre-deploy backup (NOT recommended)
- `--strategy STRATEGY` - Deployment strategy (blue-green/rolling)

**Safety Features**:
- Requires explicit confirmation
- Mandatory backup
- Pre-deploy validation
- Automatic rollback on failure
- Post-deploy verification

**Example**:
```bash
nself deploy production
```

**Output**:
```
╔═══════════════════════════════════════════════════════════╗
║    🚨 PRODUCTION DEPLOYMENT 🚨                            ║
╚═══════════════════════════════════════════════════════════╝

⚠️  WARNING: You are about to deploy to PRODUCTION
    Server: 5.75.235.42
    Domain: example.com
    Current version: v0.9.8
    New version: v0.9.9

This will affect LIVE users and services.

Type 'DEPLOY TO PRODUCTION' to confirm:
```

## nself deploy upgrade

Upgrade nself version on server.

**Usage**:
```bash
nself deploy upgrade [VERSION] [OPTIONS]
```

**Options**:
- `--version VERSION` - Specific version to upgrade to
- `--latest` - Upgrade to latest version
- `--check` - Check available versions

**Examples**:
```bash
# Check available versions
nself deploy upgrade --check

# Upgrade to latest
nself deploy upgrade --latest

# Upgrade to specific version
nself deploy upgrade v1.0.0
```

## nself deploy server

Manage deployment servers.

**Usage**:
```bash
nself deploy server <action> [SERVER] [OPTIONS]
```

**Actions**:
- `list` - List configured servers
- `add` - Add new server
- `remove` - Remove server
- `test` - Test server connection
- `info` - Show server details

**Examples**:
```bash
# List servers
nself deploy server list

# Add server
nself deploy server add staging --host 167.235.233.65 --user root

# Test connection
nself deploy server test staging
```

## nself deploy provision

Provision new server infrastructure.

**Usage**:
```bash
nself deploy provision <environment> [OPTIONS]
```

**Options**:
- `--provider PROVIDER` - Cloud provider (hetzner/aws/digitalocean)
- `--size SIZE` - Server size
- `--region REGION` - Server region

**Provisions**:
- Docker and Docker Compose
- SSL certificates
- Firewall rules
- SSH keys
- Monitoring agents

**Examples**:
```bash
# Provision staging server
nself deploy provision staging

# Provision with specific provider
nself deploy provision production --provider hetzner --size cx21 --region fsn1
```

## nself deploy sync

Sync code and configuration to server.

**Usage**:
```bash
nself deploy sync <environment> [OPTIONS]
```

**Options**:
- `--code-only` - Sync code only
- `--config-only` - Sync configuration only
- `--exclude PATTERN` - Exclude files matching pattern

**Syncs**:
- Application code
- Docker Compose files
- Environment configuration
- Nginx configuration
- SSL certificates

**Examples**:
```bash
# Sync everything
nself deploy sync staging

# Code only
nself deploy sync staging --code-only

# Config only
nself deploy sync staging --config-only
```

## nself deploy release

Create deployment release package.

**Usage**:
```bash
nself deploy release [VERSION] [OPTIONS]
```

**Options**:
- `--tag TAG` - Git tag for release
- `--changelog` - Generate changelog
- `--artifacts` - Include build artifacts

**Creates**:
- Release tarball
- Version manifest
- Changelog
- Deployment instructions

**Examples**:
```bash
# Create release
nself deploy release v0.9.9

# With changelog
nself deploy release v0.9.9 --changelog
```

## nself deploy protect

Protect environment from accidental deployments.

**Usage**:
```bash
nself deploy protect <environment> [OPTIONS]
```

**Options**:
- `--enable` - Enable protection
- `--disable` - Disable protection
- `--list` - List protected environments

**Protection Features**:
- Require explicit confirmation
- Additional authentication
- Deployment windows
- Approval requirements

**Examples**:
```bash
# Protect production
nself deploy protect production --enable

# Disable protection
nself deploy protect staging --disable
```

## nself deploy rollback

Rollback to previous deployment.

**Usage**:
```bash
nself deploy rollback [OPTIONS]
```

**Options**:
- `--version VERSION` - Rollback to specific version
- `--confirm` - Skip confirmation

**Rollback Process**:
1. Stop current services
2. Restore previous code
3. Restore database backup
4. Restart services
5. Verify rollback

**Examples**:
```bash
# Rollback to previous version
nself deploy rollback

# Rollback to specific version
nself deploy rollback --version v0.9.8
```

## nself deploy status

Check deployment status.

**Usage**:
```bash
nself deploy status [ENVIRONMENT]
```

**Shows**:
- Current deployed version
- Deployment time
- Service health
- Recent changes

**Examples**:
```bash
# Check staging status
nself deploy status staging

# Check production status
nself deploy status production
```

## nself deploy history

View deployment history.

**Usage**:
```bash
nself deploy history [ENVIRONMENT] [OPTIONS]
```

**Options**:
- `--limit N` - Show last N deployments
- `--since DATE` - Show deployments since date

**Examples**:
```bash
# Last 10 deployments
nself deploy history production --limit 10

# Deployments this month
nself deploy history production --since "2026-02-01"
```

## nself deploy verify

Verify deployment success.

**Usage**:
```bash
nself deploy verify [ENVIRONMENT]
```

**Verifies**:
- All services running
- Database migrations applied
- Health checks passing
- URLs accessible
- SSL certificates valid

**Examples**:
```bash
# Verify staging deployment
nself deploy verify staging
```

## nself deploy remote

Execute commands on remote server.

**Usage**:
```bash
nself deploy remote <environment> <command>
```

**Examples**:
```bash
# Run health check
nself deploy remote staging "nself health"

# View logs
nself deploy remote production "nself logs --tail 50"

# Check status
nself deploy remote staging "nself status"
```

## nself deploy ssh

SSH into deployment server.

**Usage**:
```bash
nself deploy ssh <environment>
```

**Examples**:
```bash
# Connect to staging
nself deploy ssh staging

# Connect to production
nself deploy ssh production
```

## Deployment Strategies

### Blue-Green Deployment

```bash
# Deploy to green environment
nself deploy production --strategy blue-green

# Automatically switches traffic after verification
```

**Process**:
1. Deploy to inactive environment (green)
2. Run tests on green
3. Switch traffic to green
4. Keep blue as fallback

### Rolling Deployment

```bash
# Deploy with rolling updates
nself deploy production --strategy rolling
```

**Process**:
1. Update servers one at a time
2. Verify health after each
3. Continue or rollback on failure

### Canary Deployment

```bash
# Deploy to subset of servers
nself deploy production --strategy canary --percentage 10
```

**Process**:
1. Deploy to 10% of servers
2. Monitor metrics
3. Gradually increase percentage
4. Full rollout or rollback

## Deployment Hooks

### Pre-Deploy Hooks

```bash
# .nself-deploy/hooks/pre-deploy.sh
#!/bin/bash
echo "Running pre-deploy tasks..."
npm run build
npm test
```

### Post-Deploy Hooks

```bash
# .nself-deploy/hooks/post-deploy.sh
#!/bin/bash
echo "Running post-deploy tasks..."
nself db migrate up
nself health
# Send deployment notification
```

## Environment Configuration

### Server Configuration

```bash
# .nself-deploy/staging.conf
SERVER_HOST=167.235.233.65
SERVER_USER=root
SERVER_PORT=22
SSH_KEY=~/.ssh/id_rsa
DOMAIN=staging.example.com
```

### Deployment Settings

```bash
# .nself-deploy/deploy.conf
BACKUP_BEFORE_DEPLOY=true
RUN_MIGRATIONS=true
RESTART_SERVICES=true
HEALTH_CHECK_TIMEOUT=120
ROLLBACK_ON_FAILURE=true
```

## Best Practices

### 1. Always Deploy to Staging First

```bash
# Test on staging
nself deploy staging

# Verify
nself deploy verify staging

# Then production
nself deploy production
```

### 2. Use Version Tags

```bash
# Create release
git tag v0.9.9
git push --tags

# Deploy specific version
nself deploy production --version v0.9.9
```

### 3. Monitor After Deployment

```bash
# Deploy
nself deploy production

# Monitor
nself deploy remote production "nself monitor"

# Watch logs
nself deploy logs production -f
```

### 4. Have Rollback Plan

```bash
# Before deploy, verify rollback works
nself deploy verify-rollback

# After failed deploy
nself deploy rollback
```

## Troubleshooting

### Deployment Fails

```bash
# Check logs
nself deploy logs staging

# Verify server connection
nself deploy server test staging

# Run deployment in verbose mode
nself deploy staging --verbose
```

### Rollback Not Working

```bash
# Check backup exists
nself deploy backup list

# Manual rollback
nself deploy ssh staging
cd /app
git checkout previous-version
nself restart
```

## Related Commands

- `nself build` - Build before deployment
- `nself config sync` - Sync configuration
- `nself db backup` - Backup database
- `nself health` - Health checks

## See Also

- [Deployment Guide](../../guides/DEPLOYMENT.md)
- [Server Provisioning](../../guides/PROVISIONING.md)
- [Zero-Downtime Deployment](../../guides/ZERO-DOWNTIME.md)
- [Rollback Procedures](../../guides/ROLLBACK.md)
