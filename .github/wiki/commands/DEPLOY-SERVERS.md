# nself servers (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself deploy server` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself servers <subcommand>
```

### New Command
```bash
nself deploy server <subcommand>
```

## Why Deprecated

The `servers` command was consolidated with `server` command into `nself deploy server` as part of v1.0 command consolidation (79 → 31 commands).

## Command Mapping

All server management functionality is now under `nself deploy server`:

| Old Command | New Command |
|-------------|-------------|
| `nself servers list` | `nself deploy server list` |
| `nself servers add <name>` | `nself deploy server add <name>` |
| `nself servers remove <name>` | `nself deploy server remove <name>` |
| `nself servers ssh <name>` | `nself deploy server ssh <name>` |
| `nself servers info <name>` | `nself deploy server info <name>` |

## Automatic Migration

The old command still works temporarily and automatically redirects:

```bash
$ nself servers list
⚠  The 'nself servers' command is deprecated.
   Please use: nself deploy server

# Automatically redirects to: nself deploy server list
```

## Timeline

- **v0.9.9**: Deprecation warning added
- **v1.0.0**: Command will be removed

## See Also

- [deploy](../commands/DEPLOY.md) - Consolidated deployment management
- [DEPLOY-SERVER](../commands/DEPLOY-SERVER.md) - Server command migration

---

**Removal Date**: v1.0.0  
**Category**: Deprecated Commands
