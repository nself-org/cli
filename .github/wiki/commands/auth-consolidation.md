# Auth Command Consolidation

**Status:** Completed
**Date:** January 30, 2026
**Version:** Command Tree v1.0

## Overview

All authentication and security-related commands have been consolidated under `nself auth` with 38 subcommands total. This refactoring follows the Command Tree v1.0 specification to reduce the number of top-level commands from 79 to 31.

## Command Structure

```
nself auth <category> <action> [options]
```

### Categories (10)

1. **Authentication** (3 subcommands)
   - login
   - logout
   - status

2. **MFA** (4 subcommands)
   - mfa enable
   - mfa disable
   - mfa verify
   - mfa backup-codes

3. **Roles** (4 subcommands)
   - roles list
   - roles create
   - roles assign
   - roles remove

4. **Devices** (4 subcommands)
   - devices list
   - devices register
   - devices revoke
   - devices trust

5. **OAuth** (7 subcommands)
   - oauth install
   - oauth enable
   - oauth disable
   - oauth config
   - oauth test
   - oauth list
   - oauth status

6. **Security** (3 subcommands)
   - security scan
   - security audit
   - security report

7. **SSL** (5 subcommands)
   - ssl generate
   - ssl install
   - ssl renew
   - ssl info
   - ssl trust

8. **Rate Limiting** (3 subcommands)
   - rate-limit config
   - rate-limit status
   - rate-limit reset

9. **Webhooks** (5 subcommands)
   - webhooks create
   - webhooks list
   - webhooks delete
   - webhooks test
   - webhooks logs

**Total:** 38 subcommands

## Migration

### Old Commands → New Commands

| Old Command | New Command | Status |
|-------------|-------------|--------|
| `nself mfa` | `nself auth mfa` | ✅ Deprecated wrapper active |
| `nself roles` | `nself auth roles` | ✅ Deprecated wrapper active |
| `nself devices` | `nself auth devices` | ✅ Deprecated wrapper active |
| `nself oauth` | `nself auth oauth` | ✅ Deprecated wrapper active |
| `nself security` | `nself auth security` | ✅ Deprecated wrapper active |
| `nself ssl` | `nself auth ssl` | ✅ Deprecated wrapper active |
| `nself trust` | `nself auth ssl trust` | ✅ Deprecated wrapper active |
| `nself rate-limit` | `nself auth rate-limit` | ✅ Deprecated wrapper active |
| `nself webhooks` | `nself auth webhooks` | ✅ Deprecated wrapper active |

### Backward Compatibility

All old commands still work but show deprecation warnings:

```bash
$ nself mfa enable --user=123 --method=totp
⚠  WARNING: 'nself mfa' is deprecated. Use 'nself auth mfa' instead.
   This compatibility wrapper will be removed in v1.0.0

[command continues normally...]
```

## File Structure

```
src/cli/
├── auth.sh                          # Main consolidated command
├── _deprecated/                     # Backup directory
│   ├── mfa.sh.backup               # Original implementation
│   ├── roles.sh.backup             # Original implementation
│   ├── devices.sh.backup           # Original implementation
│   ├── oauth.sh.backup             # Original implementation
│   ├── security.sh.backup          # Original implementation
│   ├── ssl.sh.backup               # Original implementation
│   ├── trust.sh.backup             # Original implementation
│   ├── rate-limit.sh.backup        # Original implementation
│   └── webhooks.sh.backup          # Original implementation
├── mfa.sh                           # Deprecation wrapper
├── roles.sh                         # Deprecation wrapper
├── devices.sh                       # Deprecation wrapper
├── oauth.sh                         # Deprecation wrapper
├── security.sh                      # Deprecation wrapper
├── ssl.sh                           # Deprecation wrapper
├── trust.sh                         # Deprecation wrapper
├── rate-limit.sh                    # Deprecation wrapper
└── webhooks.sh                      # Deprecation wrapper
```

## Implementation Details

### Main Auth Command (`src/cli/auth.sh`)

The main auth.sh file:
- Uses `cli-output.sh` for consistent output formatting
- Implements authentication commands directly (login, logout, status)
- Delegates to original backup implementations for complex subcommands
- Provides comprehensive help text with all 38 subcommands
- Bash 3.2 compatible

### Deprecation Wrappers

Each wrapper file:
- Shows a yellow warning message using ANSI escape codes
- Informs users of the new command syntax
- States when the wrapper will be removed (v1.0.0)
- Delegates to the new `nself auth <category>` command using `exec`

Example deprecation message:
```bash
⚠  WARNING: 'nself mfa' is deprecated. Use 'nself auth mfa' instead.
   This compatibility wrapper will be removed in v1.0.0
```

### Delegation Pattern

The auth.sh command delegates to original implementations stored in `_deprecated/`:

```bash
cmd_auth_mfa() {
  local action="${1:-}"
  shift

  # Delegate to original implementation
  if [[ -f "$SCRIPT_DIR/_deprecated/mfa.sh.backup" ]]; then
    bash "$SCRIPT_DIR/_deprecated/mfa.sh.backup" "$action" "$@"
  else
    cli_error "MFA module not found"
    exit 1
  fi
}
```

## Examples

### Before (Old Commands)
```bash
# MFA management
nself mfa enable --user=123 --method=totp
nself mfa verify --user=123 --code=123456

# OAuth setup
nself oauth install
nself oauth config google --client-id=xxx --client-secret=yyy

# SSL certificates
nself ssl bootstrap
nself trust

# Rate limiting
nself rate-limit check ip 192.168.1.1
```

### After (New Consolidated Commands)
```bash
# MFA management
nself auth mfa enable --user=123 --method=totp
nself auth mfa verify --user=123 --code=123456

# OAuth setup
nself auth oauth install
nself auth oauth config google --client-id=xxx --client-secret=yyy

# SSL certificates
nself auth ssl generate
nself auth ssl trust

# Rate limiting
nself auth rate-limit check ip 192.168.1.1
```

## Benefits

1. **Logical Grouping:** All auth/security commands under one namespace
2. **Discoverability:** Users can explore all security features with `nself auth --help`
3. **Consistency:** Follows Command Tree v1.0 pattern used across nself
4. **Backward Compatible:** Old commands still work during transition period
5. **Clear Migration Path:** Deprecation warnings guide users to new syntax

## CLI Output

The auth command uses the standardized `cli-output.sh` library:

- `cli_success()` - Success messages with ✓ icon
- `cli_error()` - Error messages with ✗ icon
- `cli_warning()` - Warning messages with ⚠ icon
- `cli_info()` - Info messages with ℹ icon
- Cross-platform compatible (Bash 3.2+)
- Respects `NO_COLOR` environment variable

## Testing

To test the consolidated commands:

```bash
# View all auth commands
nself auth --help

# Test delegation to subcommands
nself auth mfa help
nself auth oauth list
nself auth ssl status

# Test backward compatibility (shows deprecation warning)
nself mfa help
nself oauth help
```

## Removal Timeline

- **v0.6.x - v0.9.x:** Deprecation warnings active, old commands work
- **v1.0.0:** Remove all deprecation wrappers, only `nself auth` works

## Related Documentation

- [Command Tree v1.0](COMMAND-TREE-V1.md)
- CLI Output Library (`src/lib/utils/cli-output.sh`)
- [Auth Command Reference](AUTH.md)

---

**Last Updated:** January 30, 2026
