# nself functions (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service functions` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself functions <subcommand>
```

### New Command
```bash
nself service functions <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself functions status` | `nself service functions status` |
| `nself functions init [--ts]` | `nself service functions init [--ts]` |
| `nself functions enable` | `nself service functions enable` |
| `nself functions disable` | `nself service functions disable` |
| `nself functions list` | `nself service functions list` |
| `nself functions create <name>` | `nself service functions create <name>` |
| `nself functions delete <name>` | `nself service functions delete <name>` |
| `nself functions test <name>` | `nself service functions test <name>` |
| `nself functions logs [-f]` | `nself service functions logs [-f]` |
| `nself functions deploy [target]` | `nself service functions deploy [target]` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All service commands moved under `nself service`.

## Timeline

- **v0.9.9**: Deprecation warning
- **v1.0.0**: Command removed

## See Also

- [service](../commands/SERVICE.md) - Full service management documentation

---

**Removal Date**: v1.0.0
