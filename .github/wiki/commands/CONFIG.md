# nself config - Configuration Management

**Version 0.4.6** | Manage and validate configuration

---

## Overview

The `nself config` command provides comprehensive configuration management for your nself project. View, modify, validate, and compare configurations across environments.

---

## Usage

```bash
nself config <subcommand> [options]
```

---

## Subcommands

### `show` (default)

Show current configuration.

```bash
nself config                    # Show config
nself config show               # Same as above
```

### `get <key>`

Get specific configuration value.

```bash
nself config get POSTGRES_HOST
nself config get HASURA_GRAPHQL_ADMIN_SECRET --reveal
```

### `set <key> <value>`

Set configuration value.

```bash
nself config set REDIS_ENABLED true
nself config set POSTGRES_PORT 5433
```

### `list`

List all configuration keys.

```bash
nself config list               # All keys
nself config list --json        # JSON array
```

### `edit`

Open .env in editor.

```bash
nself config edit               # Uses $EDITOR
```

### `validate`

Validate configuration.

```bash
nself config validate           # Check for issues
```

### `diff <env1> <env2>`

Compare configurations between environments.

```bash
nself config diff local staging
nself config diff staging prod
```

### `export`

Export configuration (with redacted secrets).

```bash
nself config export             # JSON export
nself config export --reveal    # Include secrets
```

### `import <file>`

Import configuration from file.

```bash
nself config import config.json
```

### `reset`

Reset to defaults.

```bash
nself config reset              # Interactive
nself config reset --force      # No confirmation
```

---

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--env NAME` | Target environment | current |
| `--reveal` | Show secret values | false |
| `--json` | Output in JSON format | false |
| `--no-backup` | Don't create backup before changes | false |
| `--output FILE` | Export to specific file | - |
| `--force` | Skip confirmation | false |
| `-h, --help` | Show help message | - |

---

## Configuration Categories

Configuration is organized into categories:

| Category | Variables |
|----------|-----------|
| **Core** | PROJECT_NAME, ENV, BASE_DOMAIN, POSTGRES_*, HASURA_*, AUTH_* |
| **Services** | REDIS_*, MINIO_*, MAILPIT_*, MEILISEARCH_*, MLFLOW_*, FUNCTIONS_* |
| **Monitoring** | MONITORING_*, PROMETHEUS_*, GRAFANA_*, LOKI_*, TEMPO_* |
| **Custom** | CS_*, FRONTEND_APP_* |

---

## Examples

```bash
# View configuration
nself config show

# Get specific value
nself config get POSTGRES_PASSWORD --reveal

# Enable Redis
nself config set REDIS_ENABLED true

# Validate configuration
nself config validate

# Compare environments
nself config diff local staging

# Export configuration
nself config export --json > config.json

# Import configuration
nself config import config.json
```

---

## Output Example

### Show Command

```
  ➞ Environment: local
  ➞ File: .env

  Core Configuration
  PROJECT_NAME=myapp
  BASE_DOMAIN=local.nself.org
  POSTGRES_DB=myapp_db
  POSTGRES_PASSWORD=********

  Services
  REDIS_ENABLED=true
  MINIO_ENABLED=true
  MAILPIT_ENABLED=true

  Monitoring
  MONITORING_ENABLED=true

  ℹ Secret values redacted. Use --reveal to show.
```

### Validate Command

```
  ➞ Validating Configuration

  ✓ PROJECT_NAME configured
  ✓ BASE_DOMAIN configured
  ! POSTGRES_PASSWORD: Password too short (< 12 chars)
  ✓ HASURA_GRAPHQL_ADMIN_SECRET configured

  ⚠ Configuration valid with 1 warning(s)
```

---

## Secret Keys

The following key patterns are treated as secrets and redacted by default:

- `*PASSWORD*`
- `*SECRET*`
- `*TOKEN*`
- `*KEY*`
- `*CREDENTIAL*`
- `*AUTH*`
- `*PRIVATE*`

Use `--reveal` to show actual values.

---

## Validation Checks

The `validate` subcommand checks:

1. **Required keys** - Essential configuration present
2. **Password strength** - Minimum 12 characters
3. **Default values** - Not using 'changeme' etc.
4. **Duplicate keys** - No duplicate definitions

---

## Backups

By default, backups are created before changes:

```
.env.bak                    # Latest backup
.env.20260123_103000.bak    # Timestamped backup
```

Use `--no-backup` to skip backup creation.

---

## Environment Files

| Environment | Primary File | Fallback |
|-------------|--------------|----------|
| local/dev | .env | .env.local |
| staging | .env.staging | - |
| prod | .env.prod | .env.production |

---

## Related Commands

- [env](ENV.md) - Environment management
- [doctor](DOCTOR.md) - System diagnostics
- [build](BUILD.md) - Build configuration

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
