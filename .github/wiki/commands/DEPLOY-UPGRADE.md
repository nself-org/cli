# nself upgrade (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself deploy upgrade` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself upgrade <subcommand>
```

### New Command
```bash
nself deploy upgrade <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself upgrade check` | `nself deploy upgrade check` |
| `nself upgrade perform` | `nself deploy upgrade perform` |
| `nself upgrade rolling` | `nself deploy upgrade rolling` |
| `nself upgrade status` | `nself deploy upgrade status` |
| `nself upgrade switch <color>` | `nself deploy upgrade switch <color>` |
| `nself upgrade rollback` | `nself deploy upgrade rollback` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All deployment-related commands moved under `nself deploy`.

## Automatic Migration

The old command still works temporarily and automatically redirects:

```bash
$ nself upgrade check
⚠  The 'nself upgrade' command is deprecated.
   Please use: nself deploy upgrade

# Automatically redirects to: nself deploy upgrade check
```

## Timeline

- **v0.9.9**: Deprecation warning added
- **v1.0.0**: Command will be removed

## Quick Migration Examples

### Before (Deprecated)
```bash
# Check for updates
nself upgrade check

# Perform zero-downtime upgrade
nself upgrade perform

# Rollback if needed
nself upgrade rollback
```

### After (Current)
```bash
# Check for updates
nself deploy upgrade check

# Perform zero-downtime upgrade
nself deploy upgrade perform

# Rollback if needed
nself deploy upgrade rollback
```

## See Also

- [deploy](../commands/DEPLOY.md) - Consolidated deployment management
- [COMMAND-TREE-V1](../commands/COMMAND-TREE-V1.md) - Full command structure

---

**Removal Date**: v1.0.0  
**Category**: Deprecated Commands
