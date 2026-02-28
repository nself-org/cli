# nself harden

**Version**: v0.9.9  
**Status**: Active

## Overview

Security hardening automation for nself deployments. This command provides automated fixes for common security issues identified by security audits, including weak secrets rotation, CORS configuration hardening, and other security best practices.

## Usage

```bash
nself harden [command] [options]
```

## Commands

| Command | Description |
|---------|-------------|
| `all` | Apply all hardening fixes automatically |
| `secrets` | Rotate weak or compromised secrets |
| `cors` | Fix CORS (Cross-Origin Resource Sharing) configuration |
| `help` | Show help information |

## Interactive Mode

Running `nself harden` without a subcommand launches an interactive wizard:

```bash
nself harden
```

The wizard will:
1. Scan your configuration for security issues
2. Present findings with severity levels
3. Allow you to select which fixes to apply
4. Apply selected hardening measures
5. Verify the fixes were successful

## Subcommands

### all - Apply All Fixes

```bash
nself harden all
```

Applies all available security hardening measures:
- Rotates weak secrets (passwords < 32 characters)
- Fixes CORS configuration
- Hardens HTTP security headers
- Updates insecure defaults

**Example:**
```bash
nself harden all
```

### secrets - Rotate Weak Secrets

```bash
nself harden secrets
```

Identifies and rotates weak secrets in your configuration:
- Database passwords
- API keys and admin secrets
- JWT secrets
- Encryption keys

**What it does:**
1. Scans environment files for secrets
2. Identifies weak secrets (length, entropy, known patterns)
3. Generates cryptographically strong replacements
4. Updates environment files
5. Restarts affected services

**Example:**
```bash
nself harden secrets
```

### cors - Fix CORS Configuration

```bash
nself harden cors
```

Hardens Cross-Origin Resource Sharing (CORS) configuration:
- Removes wildcard (`*`) origins in production
- Configures specific allowed origins
- Sets appropriate CORS headers
- Restricts methods and headers

**Example:**
```bash
nself harden cors
```

## Examples

### Full Security Audit and Fix

```bash
# Run interactive wizard
nself harden

# Or apply all fixes non-interactively
nself harden all
```

### Rotate Specific Secrets

```bash
# Rotate only secrets
nself harden secrets

# Then rebuild and restart
nself build
nself restart
```

### Fix CORS Issues

```bash
# After security scan identifies CORS issues
nself harden cors
```

## What Gets Hardened

### Secrets Rotation

**Scanned secrets:**
- `POSTGRES_PASSWORD`
- `HASURA_GRAPHQL_ADMIN_SECRET`
- `HASURA_GRAPHQL_JWT_SECRET`
- `AUTH_SERVER_URL`
- `AUTH_SECRET_KEY`
- Custom API keys

**Criteria for weak secrets:**
- Length < 32 characters
- Low entropy (predictable patterns)
- Common passwords (dictionary check)
- Hardcoded defaults

### CORS Hardening

**Before:**
```bash
HASURA_GRAPHQL_CORS_DOMAIN=*
```

**After:**
```bash
HASURA_GRAPHQL_CORS_DOMAIN=https://yourdomain.com,https://app.yourdomain.com
```

### Security Headers

Ensures proper HTTP security headers:
- `X-Frame-Options: DENY`
- `X-Content-Type-Options: nosniff`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security` (HSTS)

## Notes

- Always run `nself build && nself restart` after hardening
- Backup `.env` files before applying changes
- Test thoroughly in staging before production
- Some changes may require updating client applications
- Secrets rotation may invalidate existing sessions

## Troubleshooting

### Services Won't Start After Hardening

```bash
# Check logs
nself logs

# Rollback to backup
cp .env.backup .env
nself build && nself restart
```

### CORS Errors in Browser

```bash
# Check current CORS settings
nself config env | grep CORS

# Add your domain
nself harden cors
# Follow prompts to add allowed origins
```

### Secrets Not Updating

```bash
# Verify environment file was updated
cat .env | grep SECRET

# Force rebuild
nself build --force
nself restart
```

## See Also

- [auth](../commands/AUTH.md) - Authentication and security
- [config](../commands/CONFIG.md) - Configuration management
- [doctor](../commands/DOCTOR.md) - System diagnostics
- [audit](../commands/AUDIT.md) - Security auditing

---

**Documentation**: https://docs.nself.org/security/hardening  
**Category**: Security
