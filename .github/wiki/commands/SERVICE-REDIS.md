# nself redis (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service redis` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself redis <subcommand>
```

### New Command
```bash
nself service redis <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself redis init` | `nself service redis init` |
| `nself redis add --name <name>` | `nself service redis add --name <name>` |
| `nself redis list` | `nself service redis list` |
| `nself redis get <name>` | `nself service redis get <name>` |
| `nself redis delete <name>` | `nself service redis delete <name>` |
| `nself redis test <name>` | `nself service redis test <name>` |
| `nself redis health [name]` | `nself service redis health [name]` |
| `nself redis pool configure` | `nself service redis pool configure` |
| `nself redis pool get <name>` | `nself service redis pool get <name>` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All service commands moved under `nself service`.

## Timeline

- **v0.9.9**: Deprecation warning
- **v1.0.0**: Command removed

## See Also

- [service](../commands/SERVICE.md) - Full service management documentation

---

**Removal Date**: v1.0.0
