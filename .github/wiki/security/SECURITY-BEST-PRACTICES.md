# nself Security Best Practices

**Last Updated:** January 31, 2026
**Version:** v0.9.6+

This guide provides comprehensive security best practices for nself projects, from development through production deployment.

## Table of Contents

- [Quick Start Security Checklist](#quick-start-security-checklist)
- [Secret Management](#secret-management)
- [Development Security](#development-security)
- [Production Security](#production-security)
- [Compliance Guidelines](#compliance-guidelines)
- [Security Tools](#security-tools)
- [Incident Response](#incident-response)

---

## Quick Start Security Checklist

Before deploying to production, ensure you've completed these critical security tasks:

### ✅ Essential Security Steps

```bash
# 1. Run security scan
nself auth security scan --deep

# 2. Run production audit
nself auth security audit

# 3. Generate and review security report
nself auth security report

# 4. Rotate all default secrets
nself auth security rotate POSTGRES_PASSWORD
nself auth security rotate HASURA_GRAPHQL_ADMIN_SECRET
nself auth security rotate JWT_SECRET

# 5. Install git hooks to prevent secret commits
cp src/templates/git/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### 🔴 Critical Issues to Fix

**MUST FIX before production:**

1. **Default Secrets** - All secrets must be rotated from defaults
2. **Hasura Console** - Must be disabled in production (`HASURA_GRAPHQL_ENABLE_CONSOLE=false`)
3. **SSL/TLS** - Must be enabled with valid certificates
4. **File Permissions** - `.env` and `.env.secrets` must be `600`
5. **Git Exposure** - Sensitive files must be in `.gitignore`

---

## Secret Management

### Strong Secret Generation

nself automatically generates strong secrets during initialization:

```bash
# Initialize with strong random secrets
nself init

# Secrets are automatically generated:
# - POSTGRES_PASSWORD: 32 char alphanumeric
# - HASURA_GRAPHQL_ADMIN_SECRET: 64 char hex
# - JWT_SECRET: 64 char hex
# - MINIO_ROOT_PASSWORD: 32 char alphanumeric
```

### Secret Requirements

| Secret Type | Minimum Length | Character Set | Example |
|-------------|----------------|---------------|---------|
| Passwords | 16 characters | Alphanumeric + symbols | `aB3$dF7&kL9*nP2@qR5%` |
| Admin Secrets | 32 characters | Hex | `a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6` |
| JWT Secrets | 64 characters | Hex | `a1b2c3...` (64 chars) |
| API Keys | 32 characters | Alphanumeric | `aBcD3FgH7JkL9MnP2QrS5TvW8XyZ` |

### Secret Rotation

Rotate secrets regularly (every 90 days recommended):

```bash
# Rotate specific secret
nself auth security rotate POSTGRES_PASSWORD

# This will:
# 1. Generate new strong random value
# 2. Create backup of old .env
# 3. Update secret in place
# 4. Prompt to restart services
```

### Secret Storage

**Development:**
- `.env` - Local development secrets (gitignored)
- `.env.local` - Personal overrides (gitignored)

**Production:**
- `.env.secrets` - Production secrets (gitignored, 600 permissions)
- Vault integration (optional): `nself config vault enable`

**NEVER:**
- ❌ Commit secrets to git
- ❌ Share secrets in Slack/email
- ❌ Log secrets in application logs
- ❌ Store secrets in docker-compose.yml

---

## Development Security

### Secure Development Environment

1. **Use Strong Defaults**

```bash
# nself init generates strong random secrets by default
nself init

# Verify secrets are strong
nself auth security scan
```

2. **Install Git Hooks**

Prevent accidental secret commits:

```bash
# Install pre-commit hook
cp src/templates/git/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit

# Now git will block commits containing:
# - .env files
# - Hardcoded secrets
# - Large files (>10MB)
```

3. **Validate Configuration**

```bash
# Check for security issues
nself auth security scan

# Check for misconfigurations
nself config validate
```

### Code Security

**SQL Injection Prevention:**

```sql
-- ❌ BAD: String concatenation
EXECUTE 'SELECT * FROM users WHERE id = ' || user_id;

-- ✅ GOOD: Parameterized query
EXECUTE 'SELECT * FROM users WHERE id = $1' USING user_id;
```

**XSS Prevention:**

```javascript
// ❌ BAD: Unescaped user input
div.innerHTML = userInput;

// ✅ GOOD: Escaped output
div.textContent = userInput;

// ✅ GOOD: React auto-escapes
<div>{userInput}</div>
```

---

## Production Security

### Pre-Deployment Checklist

Run comprehensive audit before deploying:

```bash
# Full security audit
nself auth security audit

# Should check:
# ✓ SSL/TLS enabled
# ✓ Admin secrets configured
# ✓ Hasura console disabled
# ✓ CORS restricted
# ✓ Monitoring enabled
# ✓ Backups configured
# ✓ No exposed admin ports
```

### Environment Configuration

**Production .env requirements:**

```bash
# Environment
ENV=production

# Security
HASURA_GRAPHQL_ENABLE_CONSOLE=false
HASURA_GRAPHQL_DEV_MODE=false
DEBUG=false
SSL_ENABLED=true

# CORS (restrict to your domain)
HASURA_GRAPHQL_CORS_DOMAIN=https://yourdomain.com

# Monitoring (required)
MONITORING_ENABLED=true

# Strong secrets (rotate from defaults!)
POSTGRES_PASSWORD=<64-char-random>
HASURA_GRAPHQL_ADMIN_SECRET=<96-char-random>
JWT_SECRET=<96-char-random>
```

### SSL/TLS Configuration

```bash
# Generate SSL certificate
nself auth ssl generate yourdomain.com

# For Let's Encrypt (production)
SSL_MODE=letsencrypt
LETSENCRYPT_EMAIL=admin@yourdomain.com
LETSENCRYPT_DOMAIN=yourdomain.com

# Verify SSL configuration
nself auth ssl info
```

### Network Security

**Firewall Rules:**

```bash
# Allow only necessary ports
# HTTP/HTTPS for public access
ufw allow 80/tcp
ufw allow 443/tcp

# SSH for administration (restrict to specific IPs)
ufw allow from 1.2.3.4 to any port 22

# Block everything else
ufw default deny incoming
ufw default allow outgoing
ufw enable
```

**Container Network Isolation:**

```yaml
# docker-compose.yml - Internal network
services:
  postgres:
    networks:
      - internal
    # NO ports exposed!

  hasura:
    networks:
      - internal
      - external
    # Exposed via nginx only

networks:
  internal:
    driver: bridge
    internal: true
  external:
    driver: bridge
```

---

## Compliance Guidelines

### GDPR Compliance

**Data Protection:**

1. **Data Minimization**
   - Only collect necessary data
   - Document data retention policies
   - Implement data deletion procedures

2. **Right to Access**
   ```sql
   -- User data export query
   SELECT * FROM users WHERE id = $user_id;
   SELECT * FROM user_messages WHERE user_id = $user_id;
   ```

3. **Right to be Forgotten**
   ```sql
   -- Anonymize user data
   UPDATE users
   SET email = 'deleted@privacy.local',
       display_name = 'Deleted User',
       avatar_url = NULL
   WHERE id = $user_id;

   -- Or hard delete
   DELETE FROM users WHERE id = $user_id CASCADE;
   ```

4. **Audit Logging**
   ```bash
   # Enable audit logging
   MONITORING_ENABLED=true
   AUDIT_LOGGING=true

   # Logs stored in: logs/audit.log
   ```

### HIPAA Compliance

**For healthcare applications:**

1. **Encryption**
   - Enable SSL/TLS for all connections
   - Encrypt database backups
   - Use encrypted volumes for data at rest

2. **Access Controls**
   - Implement role-based access (RBAC)
   - Require MFA for administrators
   - Log all data access

3. **Audit Trails**
   - Track all PHI access
   - Store logs for 7 years minimum
   - Implement automated alerting

### SOC 2 Compliance

**Security Controls:**

1. **Access Management**
   ```bash
   # Enable MFA
   nself auth mfa enable --method=totp

   # Configure session timeouts
   AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN=900  # 15 minutes
   AUTH_JWT_REFRESH_TOKEN_EXPIRES_IN=86400  # 24 hours
   ```

2. **Change Management**
   - Document all deployments
   - Require code reviews
   - Maintain change logs

3. **Incident Response**
   - Document security incidents
   - Implement automated alerts
   - Maintain incident response plan

---

## Security Tools

### Comprehensive Security Scan

```bash
# Basic scan (secrets, permissions, config)
nself auth security scan

# Deep scan (includes SQL injection, XSS)
nself auth security scan --deep

# Scan specific environment
nself auth security scan --env=.env.prod
```

**Scan checks:**
- ✓ Weak passwords/secrets
- ✓ Default secrets still in use
- ✓ Secrets exposed in git
- ✓ File permissions
- ✓ SQL injection vulnerabilities
- ✓ XSS risks
- ✓ Configuration security
- ✓ Container security

### Production Audit

```bash
# Full production readiness audit
nself auth security audit

# Checks:
# - SSL/TLS configuration
# - Authentication security
# - Monitoring status
# - Backup configuration
# - Network security
# - Database security
# - Compliance readiness
```

### Security Reporting

```bash
# Generate security report
nself auth security report --output=security-report.txt

# Share with security team
cat security-report.txt | mail -s "Security Audit" security@company.com
```

### Secrets Rotation

```bash
# Rotate specific secret
nself auth security rotate POSTGRES_PASSWORD

# Rotate all secrets
for secret in POSTGRES_PASSWORD HASURA_GRAPHQL_ADMIN_SECRET JWT_SECRET; do
  nself auth security rotate $secret
done

# Restart services to apply
nself restart
```

---

## Incident Response

### Security Incident Process

1. **Detection**
   ```bash
   # Monitor for suspicious activity
   nself logs --grep="authentication failed"
   nself logs --grep="permission denied"

   # Check for brute force attempts
   grep "Failed login" logs/auth.log | wc -l
   ```

2. **Containment**
   ```bash
   # Immediately rotate compromised secrets
   nself auth security rotate HASURA_GRAPHQL_ADMIN_SECRET

   # Block suspicious IPs
   ufw deny from 1.2.3.4

   # Disable compromised user accounts
   # (Use Hasura console or SQL)
   ```

3. **Investigation**
   ```bash
   # Review audit logs
   tail -n 1000 logs/audit.log

   # Check for data exfiltration
   nself logs --since=24h --grep="SELECT.*FROM"

   # Generate forensic report
   nself auth security report --output=incident-$(date +%Y%m%d).txt
   ```

4. **Recovery**
   ```bash
   # Restore from clean backup
   nself backup restore backup-YYYYMMDD-HHMMSS.sql

   # Verify system integrity
   nself doctor
   nself auth security scan --deep
   ```

5. **Post-Incident**
   - Document incident timeline
   - Update security procedures
   - Implement additional controls
   - Conduct post-mortem review

### Emergency Contacts

```bash
# Security team contacts (customize for your org)
SECURITY_TEAM_EMAIL=security@company.com
SECURITY_TEAM_SLACK=#security-alerts
SECURITY_ONCALL=+1-555-SECURITY
```

---

## Security Checklist by Environment

### Development

- [ ] Strong random secrets generated
- [ ] `.gitignore` includes `.env*`
- [ ] Git hooks installed
- [ ] Regular security scans
- [ ] No production data in dev

### Staging

- [ ] Separate secrets from production
- [ ] SSL enabled
- [ ] Monitoring enabled
- [ ] Access restricted to team
- [ ] Regular security audits

### Production

- [ ] All default secrets rotated
- [ ] SSL/TLS with valid certificate
- [ ] Hasura console disabled
- [ ] CORS restricted to domain
- [ ] Monitoring and alerting enabled
- [ ] Backup strategy implemented
- [ ] Firewall configured
- [ ] Audit logging enabled
- [ ] MFA for admin accounts
- [ ] Incident response plan documented

---

## Quick Reference

### Common Commands

```bash
# Security scanning
nself auth security scan              # Basic scan
nself auth security scan --deep       # Deep scan with SQL/XSS
nself auth security audit             # Production audit
nself auth security report            # Generate report

# Secret management
nself auth security rotate <SECRET>   # Rotate secret
openssl rand -hex 32                  # Generate 32-char hex
openssl rand -base64 32               # Generate 32-char base64

# SSL management
nself auth ssl generate               # Generate self-signed
nself auth ssl renew                  # Renew Let's Encrypt
nself auth ssl info                   # Certificate info
nself auth ssl trust                  # Trust local certificates

# Monitoring
nself logs --grep="error"             # Search logs
nself status                          # Service health
nself doctor                          # Diagnostics
```

### Security Resources

- [nself Security Documentation](README.md)
- [SQL Injection Prevention](SQL-SAFETY.md)
- [Input Validation Reference](VALIDATION_FUNCTIONS_REFERENCE.md)
- [Dependency Scanning](DEPENDENCY-SCANNING.md)

---

## Support

For security issues or questions:

- **Security Email:** security@nself.org
- **GitHub Issues:** [github.com/nself-org/cli/issues](https://github.com/nself-org/cli/issues)
- **Documentation:** [nself.org/docs/security](https://nself.org/docs/security)

**Report security vulnerabilities privately to:** security@nself.org
