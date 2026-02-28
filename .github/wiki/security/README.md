# nself Security

Security documentation and best practices for nself.

---

## Overview

nself provides comprehensive security features out of the box, including SSL/TLS encryption, secrets management, authentication, and production-hardening capabilities.

---

## Security Documentation

### Audit & Compliance

| Document | Description |
|----------|-------------|
| **[Security Audit](SECURITY-AUDIT.md)** | Complete security assessment and findings |
| **[Dependency Scanning](DEPENDENCY-SCANNING.md)** | CI/CD security scanning and vulnerability detection |

### Configuration Guides

| Document | Description |
|----------|-------------|
| **[SSL Setup](../configuration/SSL.md)** | SSL/TLS certificate configuration |
| **[Secrets Management](../configuration/SECRETS-MANAGEMENT.md)** | Managing sensitive credentials |

---

## Security Features

### SSL/TLS Encryption

nself provides automatic SSL certificate management:

**Development:**
- Self-signed certificates (auto-generated)
- No configuration required
- Works with local domains (*.local.nself.org)

**Production:**
- Let's Encrypt automatic certificate issuance
- Automatic renewal (90-day cycle)
- Custom SSL certificates supported
- HTTP to HTTPS redirect
- HSTS headers enabled

```bash
# Enable SSL in production
SSL_MODE=letsencrypt
SSL_EMAIL=admin@example.com
```

**[View SSL Setup Guide](../configuration/SSL.md)**

---

### Secrets Management

Environment-specific secret handling:

**Development:**
- Secrets in `.env.dev` (gitignored)
- Safe defaults for testing
- Test API keys encouraged

**Production:**
- `.secrets` file (generated on server)
- Never committed to git
- SSH-only access
- Automatic secret generation

**[View Secrets Management Guide](../configuration/SECRETS-MANAGEMENT.md)**

---

### Authentication Security

**JWT Configuration:**
```bash
# Strong JWT keys (minimum 32 characters)
HASURA_JWT_KEY=$(openssl rand -hex 32)
JWT_ACCESS_TOKEN_EXPIRES_IN=900     # 15 minutes
JWT_REFRESH_TOKEN_EXPIRES_IN=2592000  # 30 days
```

**Password Requirements:**
```bash
AUTH_PASSWORD_MIN_LENGTH=8
AUTH_PASSWORD_HIBP_ENABLED=true  # Check against breached passwords
```

**OAuth Security:**
- State parameter validation
- PKCE for mobile apps
- Secure redirect URIs
- Provider-specific configurations

---

### Database Security

**Connection Security:**
```bash
# SSL connections in production
POSTGRES_SSL_MODE=require

# Strong passwords
POSTGRES_PASSWORD=$(openssl rand -hex 32)
```

**Row-Level Security (RLS):**
```sql
-- Example: Users can only see their own data
CREATE POLICY user_isolation ON users
  FOR SELECT
  USING (id = current_user_id());
```

**Backup Encryption:**
```bash
BACKUP_ENABLED=true
BACKUP_ENCRYPTION=true
BACKUP_ENCRYPTION_KEY=$(openssl rand -hex 32)
```

---

### Network Security

**Docker Network Isolation:**
- Services communicate on private Docker network
- Only Nginx exposes public ports (80, 443)
- Internal services not accessible externally

**Nginx Security Headers:**
```nginx
# Auto-configured in production
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
X-XSS-Protection: 1; mode=block
Strict-Transport-Security: max-age=31536000
Content-Security-Policy: default-src 'self'
```

**Rate Limiting:**
```bash
# Per-service rate limiting
CS_1_RATE_LIMIT=100  # 100 requests per minute
```

---

### API Security

**GraphQL Security:**
```bash
# Disable introspection in production
HASURA_GRAPHQL_ENABLE_INTROSPECTION=false

# Disable console in production
HASURA_GRAPHQL_ENABLE_CONSOLE=false

# Enable admin secret
HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -hex 32)
```

**CORS Configuration:**
```bash
# Restrict CORS to your domains
HASURA_GRAPHQL_CORS_DOMAIN=https://app.example.com
```

---

### Container Security

**Non-Root Users:**
```dockerfile
# All containers run as non-root by default
USER 1001
```

**Read-Only Filesystem:**
```yaml
# Where possible
securityContext:
  readOnlyRootFilesystem: true
```

**Resource Limits:**
```bash
# Prevent resource exhaustion
CS_1_MEMORY=512M
CS_1_CPU=0.5
```

---

## Security Checklist

### Development

- [ ] Use `.env.local` for local secrets (gitignored)
- [ ] Verify `.env.secrets` is in `.gitignore`
- [ ] Set `.env.secrets` permissions to `600`
- [ ] Install git pre-commit hooks: `nself auth security install-hooks`
- [ ] Use test API keys for third-party services
- [ ] Enable Hasura console for debugging
- [ ] Use self-signed SSL certificates
- [ ] Enable verbose logging for debugging

### Staging

- [ ] Use staging-specific secrets
- [ ] Enable SSL with Let's Encrypt
- [ ] Test authentication flows
- [ ] Verify CORS settings
- [ ] Test backup procedures
- [ ] Enable monitoring

### Production

- [ ] Generate strong secrets (`openssl rand -hex 32`)
- [ ] Verify `.env.secrets` NEVER committed to git
- [ ] Confirm `.env.secrets` has `600` permissions
- [ ] Enable SSL with Let's Encrypt or custom certs
- [ ] Disable Hasura console and introspection
- [ ] Restrict CORS to production domains
- [ ] Enable backup encryption
- [ ] Enable audit logging
- [ ] Configure rate limiting
- [ ] Set up monitoring and alerts
- [ ] Test disaster recovery procedures
- [ ] Review security audit findings

---

## Security Scanning & CI/CD

### Automated Security Scanning

nself implements comprehensive automated security scanning in CI/CD:

**Tools:**
- **ShellCheck** - Shell script security analysis
- **Gitleaks** - Secret detection in code and git history
- **TruffleHog** - Advanced secret scanning with verification
- **Trivy** - Container and dependency vulnerability scanning
- **Semgrep** - Static application security testing (SAST)
- **Hadolint** - Dockerfile security linting

**[View Complete Dependency Scanning Guide](DEPENDENCY-SCANNING.md)**

### Local Security Scanning

Run security scans locally before committing:

```bash
# Run all security scans
./src/scripts/security/scan-dependencies.sh

# Install pre-commit hooks
pip install pre-commit
pre-commit install

# Run pre-commit hooks manually
pre-commit run --all-files
```

### CI/CD Integration

Security scans run automatically:
- **On every push** to main/develop
- **On every pull request**
- **Daily at 2 AM UTC** (scheduled)
- **Manually** via workflow dispatch

Results are uploaded to GitHub Security tab (SARIF format).

---

## Security Audit

nself has undergone a comprehensive security audit covering:

| Category | Status | Details |
|----------|--------|---------|
| Hardcoded Credentials | ✅ PASS | No credentials found in codebase |
| API Keys & Tokens | ✅ PASS | All keys environment-based |
| Command Injection | ✅ PASS | Input validation and sanitization |
| SQL Injection | ✅ PASS | Parameterized queries throughout |
| Docker Security | ✅ PASS | Non-root users, minimal images |
| Git History | ✅ PASS | No secrets in commit history |

**[View Complete Security Audit](SECURITY-AUDIT.md)**

---

## Best Practices

### 1. Use Environment-Specific Secrets

```bash
# .env.dev (development)
POSTGRES_PASSWORD=dev-password
HASURA_GRAPHQL_ADMIN_SECRET=dev-secret

# .env.prod (production - on server only)
POSTGRES_PASSWORD=<generated-strong-password>
HASURA_GRAPHQL_ADMIN_SECRET=<generated-strong-secret>

# .env.secrets (production secrets - NEVER commit)
STRIPE_SECRET_KEY=sk_live_...
JWT_PRIVATE_KEY=...
```

**🔒 .env.secrets Security Best Practices:**

1. **Auto-Protection:** Run `nself init` to automatically:
   - Add `.env.secrets` to `.gitignore`
   - Set file permissions to `600` (user read/write only)
   - Create a git pre-commit hook to prevent accidental commits

2. **Manual Setup (if needed):**
   ```bash
   # Add to .gitignore
   echo ".env.secrets" >> .gitignore

   # Set secure permissions
   chmod 600 .env.secrets

   # Install pre-commit hook
   nself auth security install-hooks
   ```

3. **Verify Protection:**
   ```bash
   # Check file permissions
   ls -la .env.secrets
   # Should show: -rw------- (600)

   # Check .gitignore
   grep ".env.secrets" .gitignore
   # Should return: .env.secrets

   # Test pre-commit hook
   git add .env.secrets
   # Should be blocked with warning message
   ```

4. **Never Commit These Files:**
   - `.env.secrets`
   - `.env.local`
   - `.env.prod`
   - `.env.staging`
   - Any file containing `SECRET`, `PASSWORD`, `KEY`, `TOKEN`

### 2. Rotate Secrets Regularly

```bash
# Generate new secrets
nself secrets generate --env prod

# Rotate database password
nself secrets rotate postgres --env prod

# Rotate all secrets
nself secrets rotate --all --env prod
```

### 3. Enable All Production Security Features

```bash
# SSL
SSL_MODE=letsencrypt
SSL_EMAIL=admin@example.com

# Hasura
HASURA_GRAPHQL_ENABLE_CONSOLE=false
HASURA_GRAPHQL_ENABLE_INTROSPECTION=false
HASURA_GRAPHQL_DISABLE_SCHEMA_EXPORT=true

# Database
POSTGRES_SSL_MODE=require
BACKUP_ENCRYPTION=true

# Logging
SECURITY_AUDIT_LOGGING=true
AUTH_EVENT_LOGGING=true
```

### 4. Regular Security Updates

```bash
# Update nself
nself self-update

# Update Docker images
nself build --pull

# Check for security advisories
nself doctor --security
```

### 5. Monitor Security Events

```bash
# Enable monitoring
MONITORING_ENABLED=true

# Review logs regularly
nself logs --security-events
nself logs auth

# Set up alerts
nself monitor alert create \
  --name "Failed login attempts" \
  --condition "rate > 10/min" \
  --action notify
```

---

## Common Security Issues

### Issue: Secrets Committed to Git

**Problem:** Accidentally committed `.env` file with secrets

**Solution:**
```bash
# Remove from git history
git filter-branch --force --index-filter \
  'git rm --cached --ignore-unmatch .env' \
  --prune-empty --tag-name-filter cat -- --all

# Force push (only if safe)
git push origin --force --all

# Rotate all exposed secrets immediately
nself secrets rotate --all --env prod
```

### Issue: SSL Certificate Expired

**Problem:** Let's Encrypt certificates expire

**Solution:**
```bash
# Auto-renewal is enabled by default
# To manually renew:
nself ssl renew --env prod

# Check renewal status:
nself ssl status
```

### Issue: Weak Passwords in Production

**Problem:** Used weak passwords or development passwords in production

**Solution:**
```bash
# Generate and set strong passwords
NEW_PASSWORD=$(openssl rand -hex 32)
nself secrets set POSTGRES_PASSWORD "$NEW_PASSWORD" --env prod

# Restart affected services
nself restart postgres --env prod
```

---

## Compliance

### GDPR Compliance

- **Data Portability:** Built-in database export
- **Right to Erasure:** User deletion capabilities
- **Data Minimization:** Collect only necessary data
- **Encryption:** At-rest and in-transit encryption

### SOC2 Compliance

- **Access Controls:** Role-based access control
- **Audit Logging:** Comprehensive event logging
- **Encryption:** SSL/TLS and backup encryption
- **Change Management:** Version-controlled infrastructure

### HIPAA Compliance

- **Encryption:** All data encrypted
- **Access Controls:** Granular permissions
- **Audit Trails:** Complete activity logging
- **Backup and Recovery:** Automated encrypted backups

---

## Security Contacts

### Reporting Security Issues

**Email:** security@nself.org

**PGP Key:** Available at https://nself.org/security.txt

**Response Time:** Within 48 hours

### Responsible Disclosure

We follow responsible disclosure practices:

1. Report the issue via security@nself.org
2. Wait for acknowledgment (48 hours)
3. Allow time for fix (typically 30 days)
4. Coordinated public disclosure

**Bug Bounty:** Coming soon

---

## Related Documentation

- **[Architecture](../architecture/ARCHITECTURE.md)** - System architecture
- **[Deployment](../deployment/README.md)** - Production deployment
- **[Configuration](../configuration/README.md)** - Secure configuration
- **[Guides](../guides/README.md)** - Security guides

---

**[Back to Documentation Home](../README.md)**
