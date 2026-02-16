# nself storage (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service storage` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself storage <subcommand>
```

### New Command
```bash
nself service storage <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself storage init` | `nself service storage init` |
| `nself storage upload <file>` | `nself service storage upload <file>` |
| `nself storage list [prefix]` | `nself service storage list [prefix]` |
| `nself storage delete <path>` | `nself service storage delete <path>` |
| `nself storage config` | `nself service storage config` |
| `nself storage status` | `nself service storage status` |
| `nself storage test` | `nself service storage test` |
| `nself storage graphql-setup` | `nself service storage graphql-setup` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All service commands moved under `nself service`.

## Timeline

- **v0.9.9**: Deprecation warning
- **v1.0.0**: Command removed

## See Also

- [service](../commands/SERVICE.md) - Full service management documentation

---

**Removal Date**: v1.0.0
