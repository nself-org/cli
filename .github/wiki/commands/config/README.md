# nself config

**Category**: Configuration Commands

Manage environment configuration, secrets, and application settings.

## Overview

All configuration operations use `nself config <subcommand>` for managing environment variables, secrets, validation, and configuration synchronization.

**Features**:
- ✅ Environment file management (.env hierarchy)
- ✅ Secrets encryption and storage
- ✅ Configuration validation
- ✅ Environment switching
- ✅ Secrets vault integration

## Subcommands

| Subcommand | Description | Use Case |
|------------|-------------|----------|
| [show](#nself-config-show) | Display current configuration | View all settings |
| [get](#nself-config-get) | Get specific config value | Retrieve single value |
| [set](#nself-config-set) | Set configuration value | Update settings |
| [list](#nself-config-list) | List all variables | Audit configuration |
| [validate](#nself-config-validate) | Validate configuration | Pre-deployment check |
| [env](#nself-config-env) | Environment management | Switch environments |
| [secrets](#nself-config-secrets) | Manage secrets | Handle credentials |
| [vault](#nself-config-vault) | Vault operations | Secure storage |
| [sync](#nself-config-sync) | Synchronize config | Deploy sync |
| [export](#nself-config-export) | Export configuration | Backup settings |
| [import](#nself-config-import) | Import configuration | Restore settings |
| [diff](#nself-config-diff) | Compare configurations | Environment diff |
| [merge](#nself-config-merge) | Merge configurations | Combine settings |
| [template](#nself-config-template) | Generate from template | Initialize config |
| [decrypt](#nself-config-decrypt) | Decrypt secrets | View encrypted values |
| [encrypt](#nself-config-encrypt) | Encrypt values | Secure credentials |
| [rotate](#nself-config-rotate) | Rotate secrets | Security maintenance |
| [audit](#nself-config-audit) | Configuration audit | Security review |
| [backup](#nself-config-backup) | Backup configuration | Safety copy |
| [restore](#nself-config-restore) | Restore from backup | Recovery |

## Environment File Hierarchy

nself uses a cascading environment file system:

```
.env.dev          # Base configuration (committed to git)
    ↓
.env.local        # Developer overrides (gitignored)
    ↓
.env.staging      # Staging overrides (on staging server)
    ↓
.env.prod         # Production overrides (on prod server)
    ↓
.env.secrets      # Ultra-sensitive (generated on server, SSH-synced)
```

Later files override earlier ones.

## Quick Start

### View Current Configuration

```bash
nself config show
```

**Output**:
```
Current Configuration (dev environment)

Project:
  PROJECT_NAME=myapp
  ENV=dev
  BASE_DOMAIN=localhost

Database:
  POSTGRES_DB=myapp_db
  POSTGRES_USER=postgres
  POSTGRES_PORT=5432

Services:
  REDIS_ENABLED=true
  MINIO_ENABLED=true
  MONITORING_ENABLED=false

Loaded from:
  .env.dev (base)
  .env.local (overrides: 3 variables)
```

### Get Specific Value

```bash
nself config get POSTGRES_DB
```

**Output**:
```
myapp_db
```

### Set Configuration Value

```bash
nself config set REDIS_ENABLED true
```

**Output**:
```
✓ Updated .env.local
  REDIS_ENABLED=true

Restart required: redis

Run 'nself restart redis' to apply changes
```

### Validate Configuration

```bash
nself config validate
```

**Output**:
```
Validating configuration...

✓ Required variables present (12/12)
✓ Port availability checked
✓ Service dependencies satisfied
✓ Domain format valid
✗ Weak password: POSTGRES_PASSWORD

Warnings: 1
Errors: 0

Configuration is valid with warnings
```

## nself config show

Display complete configuration or specific sections.

**Usage**:
```bash
nself config show [OPTIONS] [SECTION]
```

**Options**:
- `--section SECTION` - Show specific section (database, services, monitoring)
- `--format FORMAT` - Output format (table/json/yaml/env)
- `--with-secrets` - Include masked secrets (dev only)
- `--computed` - Show computed variables

**Examples**:
```bash
# All configuration
nself config show

# Database section only
nself config show --section database

# JSON format
nself config show --format json

# Include secret values (masked)
nself config show --with-secrets
```

## nself config get

Retrieve value of specific configuration variable.

**Usage**:
```bash
nself config get <KEY>
```

**Examples**:
```bash
# Get single value
nself config get POSTGRES_DB

# Use in scripts
DB=$(nself config get POSTGRES_DB)
echo "Database: $DB"

# Check if variable exists
if nself config get REDIS_ENABLED >/dev/null 2>&1; then
  echo "Redis is configured"
fi
```

## nself config set

Set or update configuration variable.

**Usage**:
```bash
nself config set <KEY> <VALUE> [OPTIONS]
```

**Options**:
- `--env ENV` - Target environment file (.env.local, .env.staging, etc.)
- `--global` - Set in base .env.dev
- `--secret` - Mark as secret (encrypted storage)
- `--no-restart` - Don't prompt for service restart

**Examples**:
```bash
# Set in local config
nself config set REDIS_PORT 6380

# Set in specific env file
nself config set --env .env.staging BASE_DOMAIN staging.example.com

# Set and mark as secret
nself config set --secret DATABASE_PASSWORD secure-random-password
```

## nself config list

List all configuration variables with sources.

**Usage**:
```bash
nself config list [OPTIONS]
```

**Options**:
- `--section SECTION` - Filter by section
- `--source` - Show which file each variable comes from
- `--secrets-only` - Show only secret variables
- `--overrides-only` - Show only overridden variables

**Examples**:
```bash
# List all variables
nself config list

# Show sources
nself config list --source

# Only secrets
nself config list --secrets-only
```

## nself config validate

Validate configuration for errors and warnings.

**Usage**:
```bash
nself config validate [OPTIONS]
```

**Options**:
- `--strict` - Fail on warnings
- `--env ENV` - Validate specific environment
- `--fix` - Auto-fix common issues

**Checks**:
- Required variables present
- Port availability
- Service dependencies
- Secret strength
- Format validation

**Examples**:
```bash
# Basic validation
nself config validate

# Strict mode (fail on warnings)
nself config validate --strict

# Auto-fix issues
nself config validate --fix
```

## nself config env

Environment management and switching.

**Usage**:
```bash
nself config env <action> [ENV]
```

**Actions**:
- `list` - List available environments
- `switch` - Switch to environment
- `current` - Show current environment
- `create` - Create new environment config
- `delete` - Remove environment config

**Examples**:
```bash
# List environments
nself config env list

# Switch to staging
nself config env switch staging

# Create new environment
nself config env create production
```

## nself config secrets

Manage encrypted secrets.

**Usage**:
```bash
nself config secrets <action> [OPTIONS]
```

**Actions**:
- `list` - List all secrets (masked)
- `add` - Add new secret
- `update` - Update existing secret
- `remove` - Remove secret
- `rotate` - Rotate secret value
- `show` - Show decrypted value (requires auth)

**Examples**:
```bash
# List secrets
nself config secrets list

# Add secret
nself config secrets add DATABASE_PASSWORD

# Rotate secret
nself config secrets rotate JWT_SECRET
```

## nself config vault

Vault integration for centralized secrets.

**Usage**:
```bash
nself config vault <action> [OPTIONS]
```

**Actions**:
- `init` - Initialize vault connection
- `pull` - Pull secrets from vault
- `push` - Push secrets to vault
- `sync` - Bi-directional sync
- `status` - Show vault connection status

**Supported Vaults**:
- HashiCorp Vault
- AWS Secrets Manager
- Google Secret Manager
- Azure Key Vault
- 1Password
- BitWarden

**Examples**:
```bash
# Initialize vault
nself config vault init --provider vault --url https://vault.example.com

# Pull secrets
nself config vault pull

# Push local secrets
nself config vault push
```

## nself config sync

Synchronize configuration across environments.

**Usage**:
```bash
nself config sync <action> <environment>
```

**Actions**:
- `pull` - Pull config from remote server
- `push` - Push config to remote server
- `diff` - Show differences
- `merge` - Merge configurations

**Examples**:
```bash
# Pull from staging
nself config sync pull staging

# Push to production (requires confirmation)
nself config sync push prod

# Show diff
nself config sync diff prod
```

## nself config export

Export configuration for backup or transfer.

**Usage**:
```bash
nself config export [OPTIONS] [FILE]
```

**Options**:
- `--format FORMAT` - Export format (env/json/yaml)
- `--include-secrets` - Include encrypted secrets
- `--env ENV` - Export specific environment

**Examples**:
```bash
# Export to file
nself config export config-backup.json

# Export with secrets (encrypted)
nself config export --include-secrets full-backup.enc

# Export specific environment
nself config export --env production prod-config.json
```

## nself config import

Import configuration from file.

**Usage**:
```bash
nself config import <FILE> [OPTIONS]
```

**Options**:
- `--merge` - Merge with existing config
- `--overwrite` - Overwrite existing values
- `--dry-run` - Show what would be imported

**Examples**:
```bash
# Import from file
nself config import config-backup.json

# Dry run first
nself config import --dry-run backup.json

# Merge with existing
nself config import --merge partial-config.json
```

## nself config diff

Compare configurations between environments.

**Usage**:
```bash
nself config diff <env1> [env2]
```

**Examples**:
```bash
# Compare local with staging
nself config diff local staging

# Compare staging with production
nself config diff staging prod
```

**Output**:
```
Configuration Differences: local → staging

Added in staging:
  + ENABLE_MONITORING=true
  + BACKUP_SCHEDULE=daily

Modified:
  ~ BASE_DOMAIN: localhost → staging.example.com
  ~ POSTGRES_PASSWORD: *** → *** (different)

Removed:
  - DEBUG_MODE=true
```

## nself config merge

Merge multiple configuration files.

**Usage**:
```bash
nself config merge <file1> <file2> [OPTIONS]
```

**Options**:
- `--strategy STRATEGY` - Merge strategy (prefer-first/prefer-second/prompt)
- `--output FILE` - Output merged config

**Examples**:
```bash
# Merge two configs
nself config merge .env.base .env.overrides

# With output file
nself config merge --output .env.merged base.env prod.env
```

## nself config template

Generate configuration from template.

**Usage**:
```bash
nself config template <template> [OPTIONS]
```

**Templates**:
- `minimal` - Bare minimum configuration
- `standard` - Standard setup with common services
- `full` - All available options
- `production` - Production-ready template

**Examples**:
```bash
# Generate from template
nself config template standard > .env

# Production template
nself config template production --env prod > .env.prod
```

## nself config encrypt / decrypt

Encrypt or decrypt configuration values.

**Usage**:
```bash
nself config encrypt <value>
nself config decrypt <encrypted_value>
```

**Examples**:
```bash
# Encrypt value
ENCRYPTED=$(nself config encrypt "my-secret-password")

# Decrypt value
nself config decrypt "$ENCRYPTED"
```

## nself config rotate

Rotate secrets and credentials.

**Usage**:
```bash
nself config rotate <secret_name> [OPTIONS]
```

**Options**:
- `--all` - Rotate all secrets
- `--auto` - Auto-generate new value
- `--length N` - Password length for auto-generation

**Examples**:
```bash
# Rotate single secret
nself config rotate HASURA_ADMIN_SECRET --auto

# Rotate all secrets
nself config rotate --all
```

## nself config audit

Audit configuration for security issues.

**Usage**:
```bash
nself config audit [OPTIONS]
```

**Checks**:
- Weak passwords
- Default values in production
- Exposed secrets in git
- File permissions
- Environment-specific issues

**Examples**:
```bash
# Full audit
nself config audit

# Export audit report
nself config audit --export audit-report.txt
```

## nself config backup / restore

Backup and restore configuration.

**Usage**:
```bash
nself config backup [FILE]
nself config restore <FILE>
```

**Examples**:
```bash
# Create backup
nself config backup config-backup-$(date +%Y%m%d).tar.gz

# Restore from backup
nself config restore config-backup-20260213.tar.gz
```

## Best Practices

### 1. Never Commit Secrets

```bash
# .gitignore should include:
.env
.env.local
.env.staging
.env.prod
.env.secrets
```

### 2. Use Environment Hierarchy

```bash
# Base config (committed)
.env.dev

# Per-developer (gitignored)
.env.local

# Server-specific (on servers only)
.env.staging
.env.prod
.env.secrets
```

### 3. Validate Before Deployment

```bash
# Pre-deployment check
nself config validate --strict
nself config audit
```

### 4. Rotate Secrets Regularly

```bash
# Rotate all secrets
nself config rotate --all

# Or on schedule
0 0 1 * * nself config rotate --all
```

### 5. Backup Configuration

```bash
# Before major changes
nself config backup pre-migration-backup.tar.gz
```

## Common Workflows

### Initial Setup

```bash
# 1. Generate from template
nself config template standard > .env.dev

# 2. Create local overrides
nself config set --env .env.local POSTGRES_PASSWORD local-dev-password

# 3. Validate
nself config validate
```

### Environment Switch

```bash
# Switch to staging
nself config env switch staging

# Verify
nself config show

# Build with staging config
nself build
nself start
```

### Secret Management

```bash
# Add new secret
nself config secrets add API_KEY

# Rotate existing
nself config rotate JWT_SECRET --auto

# Sync to vault
nself config vault push
```

### Deployment

```bash
# Pull production config
nself config sync pull prod

# Validate
nself config validate --strict

# Deploy
nself deploy production
```

## Related Commands

- `nself build` - Generate configs after changes
- `nself restart` - Apply config changes
- `nself doctor` - Diagnose config issues
- `nself deploy` - Deploy with config

## See Also

- [Environment Configuration Guide](../../guides/ENVIRONMENT.md)
- [Secrets Management](../../guides/SECRETS.md)
- [Deployment Guide](../../guides/DEPLOYMENT.md)
