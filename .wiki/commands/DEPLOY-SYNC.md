# nself sync (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Split into `nself deploy sync` and `nself config sync`

## Migration Guide

This command is deprecated. Functionality is split between two new commands:

### Configuration Synchronization
Use `nself config sync` for environment file synchronization:

```bash
# Old
nself sync pull staging

# New
nself config sync pull staging
```

### Deployment Synchronization
Use `nself deploy sync` for deployment-related sync operations:

```bash
# Old
nself sync deploy

# New
nself deploy sync
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself sync pull <env>` | `nself config sync pull <env>` |
| `nself sync push <env>` | `nself config sync push <env>` |
| `nself sync status` | `nself config sync status` |
| `nself sync history` | `nself config sync history` |
| `nself sync profiles` | `nself config sync profiles` |
| `nself sync deploy <env>` | `nself deploy sync <env>` |

## Why Deprecated and Split

The original `sync` command had two distinct responsibilities:
1. **Configuration management** (syncing .env files, secrets)
2. **Deployment operations** (syncing code, containers)

These are now properly separated:
- **`nself config sync`** - Environment configuration management
- **`nself deploy sync`** - Deployment and code synchronization

## Automatic Migration

The old command still works temporarily and automatically redirects:

```bash
$ nself sync pull prod
⚠  The 'nself sync' command is deprecated.
   Please use: nself deploy sync

# Automatically redirects based on subcommand
```

## Timeline

- **v0.9.9**: Deprecation warning added
- **v1.0.0**: Command will be removed

## Quick Migration Examples

### Configuration Sync (Before)
```bash
# Pull production config
nself sync pull prod

# Push to staging
nself sync push staging

# Check sync status
nself sync status
```

### Configuration Sync (After)
```bash
# Pull production config
nself config sync pull prod

# Push to staging
nself config sync push staging

# Check sync status
nself config sync status
```

### Deployment Sync (Before)
```bash
# Sync deployment
nself sync deploy prod
```

### Deployment Sync (After)
```bash
# Sync deployment
nself deploy sync prod
```

## See Also

- [config](../commands/CONFIG.md) - Configuration management
- [deploy](../commands/DEPLOY.md) - Deployment management
- [CONFIG-SYNC](../commands/CONFIG-SYNC.md) - Config sync documentation

---

**Removal Date**: v1.0.0  
**Category**: Deprecated Commands
