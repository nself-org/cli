# nself search (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service search` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself search <subcommand>
```

### New Command
```bash
nself service search <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself search enable [engine]` | `nself service search enable [engine]` |
| `nself search disable` | `nself service search disable` |
| `nself search status` | `nself service search status` |
| `nself search list` | `nself service search list` |
| `nself search setup` | `nself service search setup` |
| `nself search test ["query"]` | `nself service search test ["query"]` |
| `nself search reindex` | `nself service search reindex` |
| `nself search config` | `nself service search config` |
| `nself search docs [engine]` | `nself service search docs [engine]` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All service commands moved under `nself service`.

## Timeline

- **v0.9.9**: Deprecation warning
- **v1.0.0**: Command removed

## See Also

- [service](../commands/SERVICE.md) - Full service management documentation

---

**Removal Date**: v1.0.0
