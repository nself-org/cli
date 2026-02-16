# nself Security Guide

**Version 0.9.9** | Security best practices for nself deployments

---

## Overview

This guide covers security best practices for deploying and operating nself in production environments.

---

## Pre-Deployment Checklist

- [ ] All default passwords changed
- [ ] JWT secrets are unique per environment
- [ ] SSL certificates are valid (not dev certs)
- [ ] Database not exposed to public internet
- [ ] Redis not exposed to public internet
- [ ] Firewall rules configured
- [ ] Monitoring enabled
- [ ] Backups configured and tested

---

## Secrets Management

### Generate Secure Secrets

```bash
# Initialize with secure defaults
nself init --secure
```

This generates cryptographically secure secrets for:
- JWT signing key
- Database passwords
- Admin passwords
- API keys

### External Secret Managers

For production, consider:
- HashiCorp Vault
- AWS Secrets Manager
- Kubernetes Secrets
- Environment-specific `.secrets` files

### Secret File Permissions

```bash
# Secure permissions on .secrets
chmod 600 .secrets

# Verify
ls -la .secrets
# -rw-------  1 user user 256 Jan 20 10:00 .secrets
```

---

## Network Security

### Firewall Configuration

Only expose necessary ports:

```bash
# Allow HTTP/HTTPS only
ufw allow 80/tcp
ufw allow 443/tcp
ufw enable
```

### TLS Configuration

nself enforces TLS 1.2+ by default:

```nginx
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
```

### Internal Networks

Services communicate on internal Docker network:
- Not exposed to host
- Isolated from other containers
- Only Nginx exposes ports

---

## Authentication Security

### Password Hashing

Passwords are hashed using PBKDF2-SHA256:
- 100,000 iterations
- Random 32-byte salt
- No plaintext storage

### JWT Security

- Secrets generated with `openssl rand -hex 32`
- Configurable expiration
- Secure algorithm (HS256)

### Rate Limiting

```bash
# Enable rate limiting in .env
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=60
```

---

## Database Security

### Connection Security

```bash
# Require SSL connections
POSTGRES_SSL=require
```

### Access Control

- Use least-privilege accounts
- Separate read/write accounts
- Regular password rotation

### Backups

```bash
# Automated encrypted backups
nself db backup --encrypt

# Verify backup integrity
nself db backup --verify
```

---

## Container Security

### Non-Root Users

Containers run as non-root where possible.

### Read-Only Filesystems

Production containers use read-only root:

```yaml
read_only: true
tmpfs:
  - /tmp
```

### Resource Limits

```yaml
deploy:
  resources:
    limits:
      cpus: '2'
      memory: 2G
```

---

## Monitoring Security

### Log Sensitive Data

Never log:
- Passwords
- API keys
- Personal data
- Session tokens

### Security Alerts

```bash
# Enable security monitoring
SECURITY_ALERTS=true
ALERT_EMAIL=security@example.com
```

---

## Security Audit

Run the security doctor:

```bash
nself doctor --security
```

This checks:
- File permissions
- Exposed ports
- Password strength
- SSL configuration
- Known vulnerabilities

---

## Incident Response

### If Compromised

1. Rotate all secrets immediately
2. Review access logs
3. Patch vulnerability
4. Notify affected users

### Secret Rotation

```bash
# Rotate all secrets
nself init --rotate-secrets

# Redeploy
nself deploy prod
```

---

## See Also

- [Security Audit](../security/SECURITY-AUDIT.md)
- [deploy command](../commands/DEPLOY.md)
- [ssl command](../commands/SSL.md)
