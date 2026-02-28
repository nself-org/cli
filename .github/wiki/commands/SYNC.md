# nself deploy sync - Environment Synchronization

> **DEPRECATED COMMAND NAME**: This command was formerly `nself sync` in v0.x. It has been consolidated to `nself deploy sync` in v1.0. The old command name may still work as an alias.

**Version 0.9.9** | Synchronize data and configuration between environments

---

## Overview

The `nself deploy sync` command synchronizes databases, files, and configuration between environments. It enables workflows like pulling production data to staging or syncing configuration across team members.

---

## Basic Usage

```bash
# Sync database from production
nself deploy sync db prod

# Sync configuration files
nself deploy sync config staging

# Full sync
nself deploy sync full staging
```

---

## Sync Types

### Database Sync

```bash
# Pull database from remote
nself deploy sync db prod
nself deploy sync db staging

# Push database to remote
nself deploy sync db push staging
```

### File Sync

```bash
# Sync uploads/assets
nself deploy sync files prod

# Sync specific directory
nself deploy sync files prod --path uploads/
```

### Configuration Sync

```bash
# Sync .env files
nself deploy sync config prod

# Sync all config
nself deploy sync config prod --all
```

### Full Sync

```bash
# Database + files + config
nself deploy sync full staging
```

---

## Environment Access

Sync requires SSH access to the target environment:

```bash
# Check access
nself env access --check staging

# Configure SSH
nself servers add staging user@staging.example.com
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `db` | Sync database |
| `files` | Sync files/uploads |
| `config` | Sync configuration |
| `full` | Sync everything |
| `--path` | Specific path to sync |
| `--dry-run` | Preview changes |
| `--force` | Skip confirmations |

---

## Safety Features

### Production Protection

```
⚠ Pulling production database
  This will overwrite local data!

Proceed? [y/N]
```

### Data Anonymization

```bash
# Anonymize sensitive data
nself deploy sync db prod --anonymize
```

---

## See Also

- [env](ENV.md) - Environment management
- [db](DB.md) - Database operations
- [deploy](DEPLOY.md) - Deployment
