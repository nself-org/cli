# nself staging

> **⚠️ DEPRECATED in v0.9.6**: This command has been consolidated.
> Please use `nself deploy staging` instead.
> See [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md) and [v0.9.6 Release Notes](../releases/v0.9.6.md) for details.

Staging environment management command for deploying, testing, and managing staging environments.

## Synopsis

```bash
nself staging <subcommand> [options]
```

## Description

The `nself staging` command provides staging-specific functionality that mirrors production capabilities while adding features useful for testing, such as data seeding, environment resets, and syncing from production.

## Subcommands

### init

Initialize a new staging environment configuration.

```bash
nself staging init <domain> [--email <email>] [--server <host>]
```

**Arguments:**
- `domain` - Staging domain (e.g., `staging.example.com`)

**Options:**
- `--email` - SSL certificate email
- `--server` - SSH hostname for staging server

**What it creates:**
- `.environments/staging/.env` - Environment configuration
- `.environments/staging/.env.secrets` - Secrets template
- `.environments/staging/server.json` - Server configuration

**Example:**
```bash
nself staging init staging.example.com --email admin@example.com
```

### status

Show staging environment status.

```bash
nself staging status
```

Displays:
- Local configuration status
- Server connection status
- Remote deployment health (if connected)

### deploy

Deploy to the staging server.

```bash
nself staging deploy [--dry-run] [--force]
```

**Options:**
- `--dry-run` - Show what would be done without doing it
- `--force, -f` - Skip confirmations

**Deployment steps:**
1. Test SSH connection
2. Build locally
3. Sync files via rsync
4. Copy secrets
5. Start services with docker-compose
6. Verify health

**Example:**
```bash
nself staging deploy --dry-run    # Preview deployment
nself staging deploy              # Execute deployment
```

### reset

Reset the staging environment.

```bash
nself staging reset [--data] [--force]
```

**Options:**
- `--data` - Also remove all data volumes (database, storage)
- `--force, -f` - Skip confirmation

**Warning:** Using `--data` will destroy all staging data.

**Examples:**
```bash
nself staging reset               # Restart services
nself staging reset --data        # Reset everything including data
```

### seed

Seed staging with test data.

```bash
nself staging seed [file] [--file <path>]
```

**Arguments:**
- `file` - Path to SQL seed file

**Seed file locations checked by default:**
- `seeds/staging.sql`
- `seed/staging.sql`
- `data/seed-staging.sql`

**Example:**
```bash
nself staging seed
nself staging seed --file seeds/test-data.sql
```

### sync

Sync data from production to staging.

```bash
nself staging sync [db|files] [--force]
```

**Arguments:**
- `db` or `database` - Sync database only
- `files` or `storage` - Sync storage files only

**Options:**
- `--force, -f` - Skip confirmation

**Note:** This subcommand outlines the recommended workflow for production-to-staging data sync. For the current implementation, use `nself deploy staging` (this command is deprecated).

**Typical sync workflow:**
1. Create production database backup
2. Transfer to staging
3. Restore (optionally anonymizing sensitive data)
4. Sync storage files if needed

### logs

View staging logs.

```bash
nself staging logs [service] [-f] [-n <lines>]
```

**Arguments:**
- `service` - Specific service to view (optional)

**Options:**
- `-f, --follow` - Follow log output
- `-n, --lines` - Number of lines to show (default: 100)

**Examples:**
```bash
nself staging logs                # All service logs
nself staging logs nginx -f       # Follow nginx logs
nself staging logs hasura -n 500  # Last 500 Hasura logs
```

### shell / ssh

Connect to staging server.

```bash
nself staging shell [service]
nself staging ssh [service]
```

**Arguments:**
- `service` - Connect to specific container (optional)

**Examples:**
```bash
nself staging shell               # SSH to server
nself staging shell postgres      # Exec into postgres container
```

### secrets

Manage staging secrets.

```bash
nself staging secrets <action>
```

**Actions:**
- `generate [--force]` - Generate staging secrets
- `show` - Show secrets (masked)

**Examples:**
```bash
nself staging secrets generate
nself staging secrets show
```

## Server Configuration

The staging server is configured in `.environments/staging/server.json`:

```json
{
  "name": "staging",
  "type": "staging",
  "host": "staging.example.com",
  "port": 22,
  "user": "deploy",
  "key": "~/.ssh/staging_key",
  "deploy_path": "/opt/nself"
}
```

### Required Server Setup

Before deploying, ensure your staging server has:

1. **Docker and Docker Compose** installed
2. **SSH access** configured
3. **Deploy path** created with correct permissions

```bash
# On staging server
mkdir -p /opt/nself
chown deploy:deploy /opt/nself
```

## Workflow

### Initial Setup

```bash
# 1. Initialize staging environment
nself staging init staging.example.com --email admin@example.com

# 2. Configure server
# Edit .environments/staging/server.json with SSH details

# 3. Generate secrets
nself staging secrets generate

# 4. Deploy
nself staging deploy
```

### Regular Deployment

```bash
# Make changes locally, then:
nself staging deploy
```

### Testing with Fresh Data

```bash
# Reset environment and seed with test data
nself staging reset --data
nself staging deploy
nself staging seed
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `STAGING_DEPLOY_PATH` | Remote deployment path | `/opt/nself` |

## Differences from Production

| Feature | Staging | Production |
|---------|---------|------------|
| Hasura Console | Enabled | Disabled |
| Debug Mode | Off | Off |
| Log Level | info | warning |
| Mailpit | Enabled | Disabled |
| Monitoring | Optional | Required |

## Related Commands

- [nself env](ENV.md) - Environment management
- [nself deploy](DEPLOY.md) - General deployment
- [nself prod](PROD.md) - Production management

## See Also

- [Deployment Pipeline Guide](../guides/DEPLOYMENT.md)
- [Environment Configuration](../guides/ENVIRONMENTS.md)
