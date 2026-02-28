# Secrets Management - Quick Reference

**nself CLI version:** 0.9.6+

## Essential Commands

### Generate Secrets
```bash
# Generate all secrets
nself config secrets generate

# Generate specific secret
nself config secrets generate API_KEY 32 hex
```

### View Secrets
```bash
# List all (masked)
nself config secrets list

# Get specific (masked)
nself config secrets get POSTGRES_PASSWORD

# Get actual value (⚠️ use carefully!)
nself config secrets get POSTGRES_PASSWORD --reveal
```

### Rotate Secrets
```bash
# Rotate one secret
nself config secrets rotate POSTGRES_PASSWORD

# Rotate all secrets
nself config secrets rotate --all
```

### Validate Security
```bash
# Run all security checks
nself doctor

# Validate secrets only
nself config secrets validate
```

### Backup & Encryption
```bash
# Encrypt secrets
nself config secrets encrypt

# Decrypt secrets
nself config secrets decrypt
```

---

## External Integrations

### HashiCorp Vault
```bash
# Import from Vault
nself config secrets import vault secret/nself

# Export to Vault
nself config secrets export vault secret/nself
```

### AWS Secrets Manager
```bash
# Import from AWS
nself config secrets import aws nself/production

# Export to AWS
nself config secrets export aws nself/production
```

### Environment Variables
```bash
# Set environment variables
export NSELF_SECRET_DATABASE_PASSWORD="secure"
export NSELF_SECRET_API_KEY="another-secure"

# Import
nself config secrets import env
```

---

## Security Checklist

### Before Production Deployment

- [ ] Run `nself doctor` (passes all checks)
- [ ] Secrets are 16+ characters
- [ ] No weak passwords detected
- [ ] File permissions are 600
- [ ] File is in .gitignore
- [ ] Not tracked by git
- [ ] Backup created and stored securely

### Quick Fix Commands

```bash
# Fix permissions
chmod 600 .env.secrets

# Add to gitignore
echo ".env.secrets" >> .gitignore

# Remove from git
git rm --cached .env.secrets

# Rotate weak secrets
nself config secrets rotate --all
```

---

## Common Workflows

### New Project Setup
```bash
nself init                           # Auto-generates secrets
nself doctor                         # Verify security
nself config secrets list            # Review secrets
```

### Compromised Secret
```bash
nself config secrets rotate API_KEY  # Generate new value
nself restart                        # Apply changes
```

### Team Member Leaves
```bash
nself config secrets rotate --all    # Rotate everything
nself restart                        # Apply changes
```

### Production Deployment
```bash
# Generate production secrets
nself config secrets generate

# Export to AWS (recommended)
nself config secrets export aws myapp/prod

# Or encrypt for manual transfer
nself config secrets encrypt
# Transfer .env.secrets.enc securely
# On server:
nself config secrets decrypt
```

---

## Troubleshooting

| Issue | Command | Fix |
|-------|---------|-----|
| Weak password | `nself doctor` | `nself config secrets rotate <KEY>` |
| Wrong permissions | `nself doctor` | `chmod 600 .env.secrets` |
| In git | `nself doctor` | `git rm --cached .env.secrets` |
| Not in .gitignore | `nself doctor` | `echo ".env.secrets" >> .gitignore` |
| Lost file | N/A | Restore from backup or regenerate |

---

## Secret Types

| Type | Length | Characters | Use Case |
|------|--------|------------|----------|
| hex | 32-64 | 0-9, a-f | API keys, tokens |
| base64 | 24-48 | A-Z, a-z, 0-9, +/= | Binary data |
| alphanumeric | 16-32 | A-Z, a-z, 0-9 | Passwords |

---

## Best Practices

✅ **DO:**
- Use `nself config secrets generate`
- Rotate secrets regularly
- Encrypt backups
- Use external secret managers in production
- Run `nself doctor` before deployment

❌ **DON'T:**
- Use weak defaults (postgres, admin, password)
- Commit secrets to git
- Share secrets in plain text
- Reuse secrets across environments
- Skip validation

---

## Get Help

```bash
# Command help
nself config secrets --help

# Full documentation
cat docs/configuration/SECRETS-MANAGEMENT.md

# Run diagnostics
nself doctor
```

---

**Quick Reference Version:** 1.0
**Last Updated:** January 31, 2026
