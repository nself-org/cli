# nself realtime (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service realtime` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself realtime <subcommand>
```

### New Command
```bash
nself service realtime <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself realtime status` | `nself service realtime status` |
| `nself realtime enable` | `nself service realtime enable` |
| `nself realtime disable` | `nself service realtime disable` |
| `nself realtime subscriptions` | `nself service realtime subscriptions` |
| `nself realtime connections` | `nself service realtime connections` |
| `nself realtime test` | `nself service realtime test` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All service commands moved under `nself service`.

## Timeline

- **v0.9.9**: Deprecation warning
- **v1.0.0**: Command removed

## See Also

- [service](../commands/SERVICE.md) - Full service management documentation

---

**Removal Date**: v1.0.0
