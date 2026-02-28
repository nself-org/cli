# nself email (DEPRECATED)

**Version**: Deprecated in v0.9.9  
**Status**: ⚠️ DEPRECATED - Use `nself service email` instead

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself email <subcommand>
```

### New Command
```bash
nself service email <subcommand>
```

## Command Mapping

All subcommands remain the same, just use `nself service email` instead:

| Old Command | New Command |
|-------------|-------------|
| `nself email setup` | `nself service email setup` |
| `nself email list` | `nself service email list` |
| `nself email configure <provider>` | `nself service email configure <provider>` |
| `nself email configure --api <p>` | `nself service email configure --api <p>` |
| `nself email validate` | `nself service email validate` |
| `nself email check` | `nself service email check` |
| `nself email check --api` | `nself service email check --api` |
| `nself email test [email]` | `nself service email test [email]` |
| `nself email test --api [email]` | `nself service email test --api [email]` |
| `nself email docs [provider]` | `nself service email docs [provider]` |
| `nself email detect` | `nself service email detect` |

## Why Deprecated

As part of the v1.0 command consolidation:
- **79 → 31 top-level commands** reduced complexity
- All service-specific commands moved under `nself service`
- Improved discoverability and consistency
- Easier to document and maintain

## Automatic Migration

The old command still works temporarily and automatically redirects:

```bash
$ nself email setup
⚠  The 'nself email' command is deprecated.
   Please use: nself service email

# Automatically redirects to: nself service email setup
```

## Timeline

- **v0.9.9**: Deprecation warning added
- **v1.0.0**: Command will be removed
- **Migration period**: ~3 months from v0.9.9 release

## Updated Documentation

For complete email service documentation, see:
- [service email](../commands/SERVICE.md#email-service) - Full email service docs

## Quick Migration Examples

### Before (Deprecated)
```bash
# Setup email
nself email setup

# Test email
nself email test admin@example.com

# Check configuration
nself email validate
```

### After (Current)
```bash
# Setup email
nself service email setup

# Test email
nself service email test admin@example.com

# Check configuration
nself service email validate
```

## See Also

- [service](../commands/SERVICE.md) - Consolidated service management
- [COMMAND-TREE-V1](../commands/COMMAND-TREE-V1.md) - Full command structure

---

**Removal Date**: v1.0.0  
**Category**: Deprecated Commands
