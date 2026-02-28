# nself config sync

**Version**: v0.9.9  
**Status**: Active (replaces `nself sync` for config operations)

## Overview

Environment configuration synchronization between local and remote servers. This command handles pulling and pushing of environment files (`.env`, `.env.staging`, `.env.prod`) and secrets between your local machine and deployment servers.

## Usage

```bash
nself config sync <action> <environment> [options]
```

## Subcommands

| Subcommand | Description |
|------------|-------------|
| `pull <env>` | Pull configuration from remote environment |
| `push <env>` | Push configuration to remote environment |
| `status` | Show sync status and configuration |
| `history` | View sync history log |
| `profiles` | Manage sync profiles |

## Commands

### pull - Pull Configuration

Pull environment configuration from remote server to local machine.

```bash
nself config sync pull <environment>
```

**What it pulls:**
- Environment-specific file (`.env.staging`, `.env.prod`)
- Secrets file (if you have access)
- Server configuration

**Example:**
```bash
# Pull staging configuration
nself config sync pull staging

# Pull production configuration (requires higher access)
nself config sync pull prod
```

**Access Control:**
- **Dev**: Cannot pull staging or prod
- **Sr Dev**: Can pull staging
- **Lead Dev**: Can pull staging and prod

### push - Push Configuration

Push local configuration to remote server.

```bash
nself config sync push <environment> [options]
```

**What it pushes:**
- Environment-specific modifications
- Updated service configurations
- Non-secret environment variables

**Example:**
```bash
# Push to staging
nself config sync push staging

# Push to production (requires confirmation)
nself config sync push prod --confirm
```

**Options:**
- `--confirm` - Skip confirmation prompt
- `--dry-run` - Preview changes without applying

### status - Show Sync Status

Display current sync configuration and status.

```bash
nself config sync status
```

Shows:
- Last sync timestamp for each environment
- Configured remote servers
- Your access level
- Pending changes

**Example:**
```bash
nself config sync status
```

### history - View Sync History

Show sync operation history.

```bash
nself config sync history [options]
```

**Options:**
- `--limit <n>` - Show last n operations (default: 10)
- `--env <name>` - Filter by environment

**Example:**
```bash
# Show last 10 sync operations
nself config sync history

# Show last 20 staging syncs
nself config sync history --env staging --limit 20
```

### profiles - Manage Sync Profiles

Configure sync profiles for different servers.

```bash
nself config sync profiles <action>
```

**Actions:**
- `list` - List all profiles
- `add <name>` - Add new profile
- `remove <name>` - Remove profile
- `show <name>` - Show profile details

## Configuration Files

### Sync Profile Configuration

Location: `.nself/sync/profiles.yaml`

```yaml
profiles:
  staging:
    host: staging.example.com
    user: deploy
    port: 22
    path: /var/www/nself
    
  production:
    host: prod.example.com
    user: deploy
    port: 22
    path: /var/www/nself
```

### Sync History

Location: `.nself/sync/history.log`

```
2026-02-16 10:30:15 | pull | staging | success | admin
2026-02-16 09:15:22 | push | staging | success | admin
```

## Access Control

### Developer Access Levels

| Role | Local | Staging | Production | Secrets |
|------|-------|---------|------------|---------|
| **Dev** | ✓ | ✗ | ✗ | ✗ |
| **Sr Dev** | ✓ | ✓ | ✗ | ✗ |
| **Lead Dev** | ✓ | ✓ | ✓ | ✓ |

### SSH Key Requirements

Access is controlled via SSH keys:
```bash
# Check your access level
nself config sync status

# Test access to staging
nself config sync pull staging --dry-run
```

## Examples

### Complete Sync Workflow

```bash
# 1. Pull staging config to test changes
nself config sync pull staging

# 2. Make changes locally
vim .env.staging

# 3. Test changes
nself build --env staging
nself start

# 4. Push back to staging
nself config sync push staging

# 5. Deploy changes
nself deploy staging
```

### Production Sync (Lead Dev Only)

```bash
# Pull production config
nself config sync pull prod

# Pull production secrets
nself config sync pull secrets

# Make changes
vim .env.prod

# Push back (requires confirmation)
nself config sync push prod --confirm
```

### Check Sync History

```bash
# View recent syncs
nself config sync history

# View staging sync history
nself config sync history --env staging --limit 20
```

## Security

### What Gets Synced

**Synced:**
- Environment configuration (`.env.staging`, `.env.prod`)
- Service settings
- Domain configuration

**NOT Synced:**
- Master secrets (`.secrets` - only Lead Dev can pull)
- SSH keys
- Local-only files (`.env.local`)

### Encryption

- All sync operations use SSH encryption
- Secrets are never transmitted without encryption
- SSH keys must be authorized on remote server

## Troubleshooting

### SSH Connection Failed

```bash
# Test SSH access
ssh user@host "echo Connection successful"

# Check SSH key is added
ssh-add -l

# Add key if needed
ssh-add ~/.ssh/deploy_key
```

### Permission Denied

```bash
# Check your access level
nself config sync status

# Request access from team lead
# Lead Dev must add your SSH key to remote server
```

### Sync Conflicts

```bash
# View differences
nself config sync pull staging --dry-run

# Backup before pulling
cp .env.staging .env.staging.backup

# Pull and merge
nself config sync pull staging
```

## See Also

- [config](../commands/CONFIG.md) - Configuration management
- [deploy](../commands/DEPLOY.md) - Deployment operations
- [env](../commands/ENV.md) - Environment management

---

**Replaces**: `nself sync` (for config operations)  
**Category**: Configuration Management
