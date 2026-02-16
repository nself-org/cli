# nself server (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself deploy server` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself server <subcommand>
```

### New Command
```bash
nself deploy server <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself server init <host>` | `nself deploy server init <host>` |
| `nself server check <host>` | `nself deploy server check <host>` |
| `nself server status [env]` | `nself deploy server status [env]` |
| `nself server diagnose <env>` | `nself deploy server diagnose <env>` |
| `nself server setup <component>` | `nself deploy server setup <component>` |
| `nself server ssl` | `nself deploy server ssl` |
| `nself server dns` | `nself deploy server dns` |
| `nself server secure` | `nself deploy server secure` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All deployment-related commands moved under `nself deploy`.

## Automatic Migration

The old command still works temporarily and automatically redirects:

```bash
$ nself server init root@example.com
⚠  The 'nself server' command is deprecated.
   Please use: nself deploy server

# Automatically redirects to: nself deploy server init root@example.com
```

## Timeline

- **v0.9.9**: Deprecation warning added
- **v1.0.0**: Command will be removed

## Quick Migration Examples

### Before (Deprecated)
```bash
# Initialize server
nself server init root@5.75.235.42 --domain example.com

# Check server
nself server check root@5.75.235.42

# Diagnose issues
nself server diagnose prod
```

### After (Current)
```bash
# Initialize server
nself deploy server init root@5.75.235.42 --domain example.com

# Check server
nself deploy server check root@5.75.235.42

# Diagnose issues
nself deploy server diagnose prod
```

## See Also

- [deploy](../commands/DEPLOY.md) - Consolidated deployment management
- [COMMAND-TREE-V1](../commands/COMMAND-TREE-V1.md) - Full command structure

---

**Removal Date**: v1.0.0  
**Category**: Deprecated Commands
