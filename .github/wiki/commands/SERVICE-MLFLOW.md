# nself mlflow (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service mlflow` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself mlflow <subcommand>
```

### New Command
```bash
nself service mlflow <subcommand>
```

## Command Mapping

| Old Command | New Command |
|-------------|-------------|
| `nself mlflow status` | `nself service mlflow status` |
| `nself mlflow enable` | `nself service mlflow enable` |
| `nself mlflow disable` | `nself service mlflow disable` |
| `nself mlflow ui` | `nself service mlflow ui` |
| `nself mlflow experiments` | `nself service mlflow experiments` |
| `nself mlflow runs [experiment]` | `nself service mlflow runs [experiment]` |
| `nself mlflow compare <run1> <run2>` | `nself service mlflow compare <run1> <run2>` |
| `nself mlflow artifacts <run>` | `nself service mlflow artifacts <run>` |

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All service commands moved under `nself service`.

## Timeline

- **v0.9.9**: Deprecation warning
- **v1.0.0**: Command removed

## See Also

- [service](../commands/SERVICE.md) - Full service management documentation

---

**Removal Date**: v1.0.0
