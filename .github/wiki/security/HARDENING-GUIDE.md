# nself Security Hardening Guide

**Version:** v0.10.0
**Last Updated:** February 11, 2026
**Status:** Production Ready

This comprehensive guide covers the security hardening features introduced in nself v0.10.0, which addressed 39 critical production security issues through a 3-phase implementation.

---

## Table of Contents

1. [Introduction](#introduction)
2. [Quick Start](#quick-start)
3. [Environment-Specific Security](#environment-specific-security)
4. [Security Features Explained](#security-features-explained)
5. [Using the Security Audit](#using-the-security-audit)
6. [Using the Hardening Command](#using-the-hardening-command)
7. [Manual Security Configuration](#manual-security-configuration)
8. [Production Deployment Checklist](#production-deployment-checklist)
9. [Compliance Considerations](#compliance-considerations)
10. [Troubleshooting](#troubleshooting)
11. [Advanced Topics](#advanced-topics)

---

## Introduction

### Security Philosophy: Secure by Default

nself v0.10.0 introduces a **secure-by-default** approach to deployment security. Rather than requiring developers to manually configure security settings, nself now:

- Generates strong random secrets automatically during initialization
- Configures environment-appropriate CORS policies
- Runs all services as non-root users
- Exposes ports conditionally based on environment
- Provides automated security auditing and hardening tools

### Who This Guide Is For

This guide is for:

- **Developers** deploying nself applications to production
- **DevOps engineers** responsible for security compliance
- **Security teams** auditing nself deployments
- **Architects** designing secure multi-tenant systems

### What Changed in v0.10.0

The v0.10.0 security hardening addressed 39 production security issues across three phases:

**Phase 1: Core Infrastructure (14 issues)**
- Default weak credentials removed
- Strong secret generation implemented
- Non-root container users for all services
- Environment-aware CORS configuration

**Phase 2: Service Security (15 issues)**
- Mailpit authentication in production
- Redis password requirements
- MinIO credential hardening
- Search service API key generation
- Custom service CORS templates

**Phase 3: Operations (10 issues)**
- Security audit command (`nself audit security`)
- Automated hardening wizard (`nself harden`)
- Port exposure management
- Configuration validation

---

## Quick Start

For busy developers who need to verify security quickly:

### 4-Step Security Verification

```bash
# 1. Run security audit
nself audit security

# 2. Review findings
# Check for: weak secrets, exposed ports, CORS wildcards, root containers

# 3. Apply automated fixes (if issues found)
nself harden

# 4. Rebuild and restart
nself build && nself restart
```

### Common Commands

```bash
# Full security audit
nself audit security

# Check specific components
nself audit security secrets      # Check secret strength
nself audit security cors         # Check CORS configuration
nself audit security ports        # Check exposed ports
nself audit security containers   # Check container users

# Automated hardening
nself harden                      # Interactive wizard
nself harden all                  # Apply all fixes automatically
nself harden secrets              # Rotate weak secrets only
nself harden cors                 # Fix CORS only
```

### When to Harden

Run security hardening:

1. **Before first deployment** - Ensure secure configuration from day one
2. **After upgrading nself** - Apply new security best practices
3. **Before production launch** - Final security verification
4. **During security audits** - Compliance verification
5. **After configuration changes** - Ensure changes maintain security

---

## Environment-Specific Security

nself adapts security settings based on the `ENV` variable in your `.env` file.

### Development Environment (`ENV=dev`)

**Philosophy:** Convenient but still secure

**Security Settings:**
- CORS: Permissive (allows localhost, local.nself.org)
- Ports: Database exposed on 127.0.0.1 for tools like pgAdmin
- Secrets: Strong but displayed in logs for debugging
- SSL: Self-signed certificate (automatic)
- Console Access: Hasura console enabled
- Authentication: Relaxed for rapid iteration

**Example Configuration:**
```bash
ENV=dev
BASE_DOMAIN=local.nself.org

# CORS - allows localhost and local domains
HASURA_GRAPHQL_CORS_DOMAIN=http://localhost:*,http://*.local.nself.org,https://*.local.nself.org

# Port exposure - database accessible from host
POSTGRES_EXPOSE_PORT=auto  # Exposes on 127.0.0.1:5432

# Console access - enabled for debugging
HASURA_GRAPHQL_ENABLE_CONSOLE=true

# SSL - self-signed (automatic)
SSL_MODE=local
```

**When to Use:**
- Local development
- Running on localhost
- Testing new features
- Debugging issues

### Staging Environment (`ENV=staging`)

**Philosophy:** Production-like security with some flexibility

**Security Settings:**
- CORS: Restricted to staging domain + localhost for testing
- Ports: Database internal-only (Docker network)
- Secrets: Production-strength (32-96 chars)
- SSL: Let's Encrypt or custom certificate
- Console Access: Enabled but authenticated
- Authentication: Full authentication required

**Example Configuration:**
```bash
ENV=staging
BASE_DOMAIN=staging.myapp.com

# CORS - staging domain + localhost for testing
HASURA_GRAPHQL_CORS_DOMAIN=https://*.staging.myapp.com,http://localhost:3000

# Port exposure - internal only
POSTGRES_EXPOSE_PORT=false

# Console access - enabled but requires admin secret
HASURA_GRAPHQL_ENABLE_CONSOLE=true

# SSL - Let's Encrypt
SSL_MODE=letsencrypt
```

**When to Use:**
- Pre-production testing
- Client demos
- QA environment
- Integration testing

### Production Environment (`ENV=prod` or `ENV=production`)

**Philosophy:** Maximum security, zero compromise

**Security Settings:**
- CORS: Strictly limited to production domain only
- Ports: All services internal-only (no host exposure)
- Secrets: Maximum strength (48-96 chars)
- SSL: Required (Let's Encrypt or custom)
- Console Access: Disabled
- Authentication: Full authentication + audit logging
- Mailpit: Disabled (use real email provider)

**Example Configuration:**
```bash
ENV=production
BASE_DOMAIN=myapp.com

# CORS - production domain only
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com

# Port exposure - all internal
POSTGRES_EXPOSE_PORT=false

# Console access - disabled
HASURA_GRAPHQL_ENABLE_CONSOLE=false

# SSL - required
SSL_MODE=letsencrypt

# Email - real provider (not Mailpit)
EMAIL_PROVIDER=sendgrid
SENDGRID_API_KEY=SG.xxxxx

# Monitoring - enabled
MONITORING_ENABLED=true
```

**When to Use:**
- Production deployments
- Customer-facing applications
- Production data
- Compliance-required environments

### Environment Comparison Table

| Feature | Development | Staging | Production |
|---------|-------------|---------|------------|
| CORS Policy | Permissive (localhost + local domains) | Restricted (staging domain + localhost) | Strict (production domain only) |
| Port Exposure | Database on 127.0.0.1 | Internal only | Internal only |
| Secret Strength | 32-64 chars | 32-96 chars | 48-96 chars |
| SSL Required | No (self-signed) | Recommended | Required |
| Hasura Console | Enabled | Enabled (authenticated) | Disabled |
| Mailpit | Enabled | Enabled | Disabled |
| Debug Logging | Verbose | Standard | Minimal |
| Audit Logging | Optional | Recommended | Required |
| Backup Schedule | Weekly | Daily | Daily + PITR |
| Monitoring | Optional | Recommended | Required |

---

## Security Features Explained

### 1. CORS Configuration (Environment-Aware)

**What It Is:**
Cross-Origin Resource Sharing (CORS) controls which domains can access your API.

**How It Works:**
nself automatically configures CORS based on your environment:

```bash
# Development - allows multiple origins for testing
dev:     http://localhost:*,http://*.local.nself.org,https://*.local.nself.org

# Staging - staging domain + localhost for testing
staging: https://*.staging.myapp.com,http://localhost:3000

# Production - strict domain restriction
prod:    https://*.myapp.com
```

**Security Impact:**
- Development: Convenient, allows local testing
- Production: Prevents unauthorized API access from other domains

**Configuration:**
```bash
# Manual override (not recommended)
HASURA_GRAPHQL_CORS_DOMAIN=https://app.myapp.com,https://admin.myapp.com
```

**Best Practices:**
- Never use `*` (wildcard) in production
- Only allow your actual domains
- Use HTTPS in production
- Test CORS policies before deploying

### 2. Secret Management (Strong Random Generation)

**What Changed:**
- **Before v0.10.0:** Default weak secrets in templates
- **After v0.10.0:** Strong random secrets generated automatically

**Secret Types and Lengths:**

| Secret Type | Development | Production | Format |
|-------------|-------------|------------|--------|
| Passwords (PostgreSQL, MinIO) | 32 chars | 48 chars | Alphanumeric |
| Admin Secrets (Hasura) | 64 chars | 96 chars | Hex |
| JWT Keys | 64 chars | 96 chars | Hex |
| API Keys (Storage, Search) | 48 chars | 64 chars | Hex |

**Generation Methods:**
nself uses multiple secure random sources:

1. **openssl** (primary): `openssl rand -hex 48`
2. **/dev/urandom** (fallback): `head -c 48 /dev/urandom | base64`
3. **date-based** (last resort): `date +%s%N | sha256sum`

**Example Strong Secrets:**
```bash
# PostgreSQL (32 chars alphanumeric)
POSTGRES_PASSWORD=a8F3xK9mP2qR7nL4vY6wZ1eC5tB8jH0i

# Hasura Admin Secret (64 chars hex)
HASURA_GRAPHQL_ADMIN_SECRET=8f3c2a1b6e5d4f7a9c0b3e1d5f8a2c4b7e9d1f3a5c7b9e1d3f5a7c9b1e3d5f7a

# JWT Key (64 chars hex)
HASURA_JWT_KEY=3f5a7c9b1e3d5f7a8c0b2e4d6f8a1c3b5e7d9f1a3c5b7e9d1f3a5c7b9e1d3f5a

# MinIO (32 chars alphanumeric)
MINIO_ROOT_PASSWORD=mN7pQ2vL9xK4fY6wZ8eC1tB5jH3rG0iA
```

**Security Benefits:**
- Resistant to brute force attacks
- No dictionary words
- High entropy
- Unique per deployment

### 3. Port Exposure (Conditional Based on Environment)

**What It Is:**
Controls whether service ports are exposed to the host machine.

**Configuration Options:**
```bash
# Auto (default) - exposes in dev only
POSTGRES_EXPOSE_PORT=auto

# Always expose (not recommended for production)
POSTGRES_EXPOSE_PORT=true

# Never expose (production default)
POSTGRES_EXPOSE_PORT=false
```

**Behavior by Environment:**

| Environment | POSTGRES_EXPOSE_PORT=auto | Actual Behavior |
|-------------|---------------------------|-----------------|
| Development | true | Exposed on 127.0.0.1:5432 |
| Staging | false | Internal Docker network only |
| Production | false | Internal Docker network only |

**Security Impact:**
- **Exposed (dev):** Convenient for tools like pgAdmin, TablePlus
- **Internal (prod):** Eliminates external attack surface

**Docker Compose Example:**
```yaml
# Development - exposed
postgres:
  ports:
    - "127.0.0.1:5432:5432"  # Only localhost can access

# Production - not exposed
postgres:
  # No ports section - internal only
  networks:
    - myapp_network
```

**Other Services:**
- **Always Internal:** auth, hasura (accessed via nginx only)
- **Conditionally Exposed:** postgres (auto), redis (auto)
- **Always Exposed:** nginx (80, 443)

### 4. Non-Root Containers (All Services)

**What Changed:**
All containers now run as non-root users with proper UID/GID mapping.

**User Mappings:**

| Service | User | UID:GID | Purpose |
|---------|------|---------|---------|
| PostgreSQL | postgres | 999:999 | Database files |
| Hasura | hasura | 1000:1000 | API engine |
| Auth | node | 1000:1000 | Auth service |
| Redis | redis | 999:999 | Cache data |
| MinIO | minio | 1001:1001 | Object storage |
| Grafana | grafana | 472:472 | Monitoring UI |
| Prometheus | prometheus | 65534:65534 | Metrics database |
| Nginx | nginx | 101:101 | Reverse proxy |

**Docker Compose Example:**
```yaml
postgres:
  image: postgres:16-alpine
  user: "999:999"  # postgres user
  volumes:
    - postgres_data:/var/lib/postgresql/data

hasura:
  image: hasura/graphql-engine:v2.44.0
  user: "1000:1000"  # hasura user
```

**Security Benefits:**
- Limits blast radius of container compromise
- Prevents privilege escalation
- Follows principle of least privilege
- Complies with security best practices (CIS benchmarks)

**Volume Permissions:**
nself automatically sets correct permissions on volumes:

```bash
# During init
chown -R 999:999 ./data/postgres  # postgres user
chown -R 1001:1001 ./data/minio   # minio user
chown -R 472:472 ./data/grafana   # grafana user
```

### 5. Mailpit Security (Dev vs Prod)

**Development:**
```bash
# Mailpit enabled for email testing
MAILPIT_ENABLED=true
MAILPIT_UI_PORT=8025
MAILPIT_ROUTE=mail.local.nself.org

# No authentication required (local only)
```

**Production:**
```bash
# Mailpit disabled - use real email provider
MAILPIT_ENABLED=false

# Configure production email
EMAIL_PROVIDER=sendgrid
SENDGRID_API_KEY=SG.xxxxx
```

**Why:**
- Mailpit is for development/testing only
- Not designed for production email delivery
- No authentication in default configuration
- Production needs real email provider (SendGrid, Postmark, SES)

**Migration Path:**
```bash
# Development
nself build  # Mailpit included

# Production
# 1. Set email provider
EMAIL_PROVIDER=sendgrid
SENDGRID_API_KEY=your-key

# 2. Disable Mailpit
MAILPIT_ENABLED=false

# 3. Rebuild
nself build && nself restart
```

### 6. Custom Service CORS (Environment-Aware Patterns)

**Template System:**
Custom services (CS_N) get environment-aware CORS configuration automatically.

**Example - Express.js Service:**
```javascript
// src/services/api/middleware/cors.js
const cors = require('cors');

const getCorsOrigin = () => {
  const env = process.env.ENV || 'dev';
  const baseDomain = process.env.BASE_DOMAIN || 'localhost';

  switch (env) {
    case 'production':
    case 'prod':
      return `https://*.${baseDomain}`;

    case 'staging':
      return [
        `https://*.${baseDomain}`,
        'http://localhost:3000'  // For testing
      ];

    case 'dev':
    case 'development':
    default:
      return [
        'http://localhost:*',
        `http://*.${baseDomain}`,
        `https://*.${baseDomain}`
      ];
  }
};

module.exports = cors({
  origin: getCorsOrigin(),
  credentials: true
});
```

**Template Locations:**
- Express: `src/templates/services/javascript/express-ts/middleware/cors.ts.template`
- Fastify: `src/templates/services/javascript/fastify-ts/plugins/cors.ts.template`
- FastAPI: `src/templates/services/python/fastapi/middleware/cors.py.template`

**Automatic Injection:**
When you create a custom service, nself:
1. Copies the appropriate template
2. Injects environment-aware CORS configuration
3. Replaces {{SERVICE_NAME}} placeholders
4. Creates Dockerfile with proper user settings

---

## Using the Security Audit

### Running the Audit

```bash
# Full security audit (all checks)
nself audit security

# Specific checks
nself audit security secrets      # Check weak secrets
nself audit security cors         # Check CORS configuration
nself audit security ports        # Check exposed ports
nself audit security containers   # Check container users
```

### Understanding the Output

**Example Output:**
```
╔══════════════════════════════════════════════════════════════╗
║                    Security Audit                            ║
╚══════════════════════════════════════════════════════════════╝

Environment: production

✓ Checking secrets...
  ✓ POSTGRES_PASSWORD: 48 characters (strong)
  ✓ HASURA_GRAPHQL_ADMIN_SECRET: 96 characters (strong)
  ✓ HASURA_JWT_KEY: 96 characters (strong)
  ⚠ MINIO_ROOT_PASSWORD: 16 characters (too short, minimum 24)

✓ Checking CORS configuration...
  ✓ CORS restricted to: https://*.myapp.com

✓ Checking port exposure...
  ✓ Database not exposed externally

✓ Checking container users...
  ✓ postgres: running as non-root (999:999)
  ✓ hasura: running as non-root (1000:1000)
  ✓ auth: running as non-root (1000:1000)
  ⚠ redis: running as root (needs user: "999:999")

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SUMMARY
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Found 2 security issue(s)

Fix automatically: nself harden
```

### What Each Check Does

#### 1. Secrets Check (`secrets`)

**Purpose:** Identifies weak or default secrets

**Checks:**
- Secret length (minimum 24 characters)
- Weak patterns (admin, password, test, demo, etc.)
- Default template values
- Common passwords

**Issues Detected:**
```bash
⚠ HIGH - Secret too short: REDIS_PASSWORD
  Current: 12 characters, Minimum: 24
  → Generate: openssl rand -hex 32

⚠ CRITICAL - Default secret detected: POSTGRES_PASSWORD
  Value: postgres-dev-password
  → Rotate: nself harden secrets
```

#### 2. CORS Check (`cors`)

**Purpose:** Validates CORS configuration for each environment

**Checks:**
- No wildcard (*) in production
- HTTPS required in production
- Appropriate origins for environment

**Issues Detected:**
```bash
⚠ CRITICAL - Wildcard CORS in production
  Current: HASURA_GRAPHQL_CORS_DOMAIN=*
  → Fix: nself harden cors
```

#### 3. Ports Check (`ports`)

**Purpose:** Ensures services aren't unnecessarily exposed

**Checks:**
- Database not exposed in production
- Admin interfaces not exposed
- Only nginx on 80/443

**Issues Detected:**
```bash
⚠ HIGH - Database exposed in production
  Current: POSTGRES_EXPOSE_PORT=true
  → Fix: Set POSTGRES_EXPOSE_PORT=false
```

#### 4. Containers Check (`containers`)

**Purpose:** Verifies all services run as non-root

**Checks:**
- Each service has user: directive
- Correct UID/GID for each service
- Volume ownership matches container user

**Issues Detected:**
```bash
⚠ MEDIUM - Service running as root: redis
  → Fix: Add user: "999:999" in docker-compose.yml
```

### Exit Codes

```bash
# Check exit code
nself audit security
echo $?

# Exit codes:
# 0 - All checks passed
# 1 - Issues found
# 2 - Critical issues found
```

### Automated Usage in CI/CD

```bash
# .github/workflows/security-audit.yml
name: Security Audit

on:
  push:
    branches: [main, staging, production]

jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install nself
        run: curl -sSL https://install.nself.org | bash

      - name: Run security audit
        run: nself audit security

      - name: Fail on critical issues
        run: |
          if [ $? -eq 2 ]; then
            echo "Critical security issues found"
            exit 1
          fi
```

---

## Using the Hardening Command

The `nself harden` command automatically fixes common security issues.

### Interactive Wizard

```bash
# Run interactive wizard
nself harden
```

**What It Does:**
1. Runs full security audit
2. Shows all detected issues
3. Asks for confirmation
4. Applies fixes automatically
5. Shows summary of changes

**Example Session:**
```
╔══════════════════════════════════════════════════════════════╗
║              Security Hardening Wizard                       ║
╚══════════════════════════════════════════════════════════════╝

Running security audit...

Found 3 security issues:
  ⚠ Weak secret: MINIO_ROOT_PASSWORD
  ⚠ Wildcard CORS in production
  ⚠ Database exposed in production

Apply automatic hardening fixes? (y/N): y

Applying Security Hardening...

✓ Rotated: MINIO_ROOT_PASSWORD
✓ Fixed CORS: HASURA_GRAPHQL_CORS_DOMAIN
✓ Secured ports: POSTGRES_EXPOSE_PORT=false

Security hardening complete!
Rebuild and restart: nself build && nself restart
```

### Automatic Hardening (All Fixes)

```bash
# Apply all fixes without prompting
nself harden all
```

**What It Fixes:**
- Rotates all weak secrets
- Fixes CORS configuration for environment
- Disables port exposure in production
- Updates configuration files

**Use Cases:**
- CI/CD pipelines (non-interactive)
- Batch security updates
- Emergency hardening

### Targeted Hardening

#### Secrets Only

```bash
# Rotate weak secrets only
nself harden secrets
```

**What It Does:**
- Scans for weak/default secrets
- Generates strong replacements
- Updates .env file
- Creates backup (.env.backup-TIMESTAMP)

**Example Output:**
```
Rotating weak secrets...

✓ Rotated: POSTGRES_PASSWORD
  New value: aK8fM3nP7qR2vL9xY6wZ1eC5tB4jH0iG

✓ Rotated: HASURA_GRAPHQL_ADMIN_SECRET
  New value: 3f5a7c9b1e3d5f7a8c0b2e4d6f8a1c3b5e7d9f1a3c5b7e9d1f3a5c7b9e1d3f5a

✓ Strengthened: MINIO_ROOT_PASSWORD
  New value: mN7pQ2vL9xK4fY6wZ8eC1tB5jH3rG0iA

Rotated 3 weak secret(s)
```

#### CORS Only

```bash
# Fix CORS configuration only
nself harden cors
```

**What It Does:**
- Removes wildcard CORS
- Sets environment-appropriate domains
- Updates configuration

**Example Output:**
```
Hardening CORS configuration...

Removed wildcard CORS (*)
Set CORS: https://*.myapp.com

CORS configuration hardened
```

### When to Use Each Approach

| Approach | Use Case | Interactive | Safe for Production |
|----------|----------|-------------|---------------------|
| `nself harden` | First-time hardening | Yes | Yes (with review) |
| `nself harden all` | CI/CD automation | No | Yes (after testing) |
| `nself harden secrets` | Secret rotation only | No | Yes (requires restart) |
| `nself harden cors` | CORS issues only | No | Yes |

### Backup and Rollback

**Automatic Backups:**
```bash
# Hardening creates backups automatically
ls -la .env.backup-*
# .env.backup-20260211-143000

# Rollback if needed
cp .env.backup-20260211-143000 .env
nself build && nself restart
```

**Manual Backup Before Hardening:**
```bash
# Create backup first
cp .env .env.manual-backup

# Then harden
nself harden all

# Rollback if needed
cp .env.manual-backup .env
```

---

## Manual Security Configuration

For advanced users who want manual control over security settings.

### CORS: How to Configure for Your Domain

**Single Domain:**
```bash
# Allow only your main domain
HASURA_GRAPHQL_CORS_DOMAIN=https://myapp.com
```

**Multiple Subdomains:**
```bash
# Wildcard subdomain
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com
```

**Multiple Specific Domains:**
```bash
# Comma-separated list
HASURA_GRAPHQL_CORS_DOMAIN=https://app.myapp.com,https://admin.myapp.com,https://dashboard.myapp.com
```

**Development + Production:**
```bash
# Development
HASURA_GRAPHQL_CORS_DOMAIN=http://localhost:*,https://*.local.nself.org

# Production
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com
```

**Testing CORS:**
```bash
# Test from browser console
fetch('https://api.myapp.com/v1/graphql', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({ query: '{ __typename }' })
})
.then(r => r.json())
.then(console.log)
.catch(console.error);
```

### Secrets: How to Generate Manually

**Using OpenSSL (Recommended):**
```bash
# Hex (for admin secrets, JWT keys)
openssl rand -hex 48  # 96 characters

# Base64 (for general secrets)
openssl rand -base64 48  # 64 characters

# Alphanumeric (for passwords)
openssl rand -base64 48 | tr -dc 'a-zA-Z0-9' | head -c 32
```

**Using /dev/urandom:**
```bash
# Hex
head -c 48 /dev/urandom | xxd -p -c 48

# Base64
head -c 48 /dev/urandom | base64 | tr -d '\n'
```

**Using Python:**
```python
import secrets

# Hex (96 chars)
print(secrets.token_hex(48))

# URL-safe base64 (64 chars)
print(secrets.token_urlsafe(48))
```

**Secret Strength Requirements:**

| Secret Type | Minimum Length | Recommended | Format |
|-------------|----------------|-------------|--------|
| Database passwords | 24 chars | 32-48 chars | Alphanumeric |
| Admin secrets | 32 chars | 64-96 chars | Hex |
| JWT secrets | 32 chars | 64-96 chars | Hex |
| API keys | 24 chars | 48-64 chars | Hex or Base64 |
| Encryption keys | 32 chars | 64 chars | Hex |

### Ports: When and Why to Expose

**Guidelines:**

1. **Never Expose in Production:**
   ```bash
   ENV=production
   POSTGRES_EXPOSE_PORT=false  # Always false
   ```

2. **Expose in Development for Tools:**
   ```bash
   ENV=dev
   POSTGRES_EXPOSE_PORT=auto  # Exposes on 127.0.0.1 only
   ```

3. **Bind to Localhost Only:**
   ```yaml
   # docker-compose.yml
   postgres:
     ports:
       - "127.0.0.1:5432:5432"  # NOT "5432:5432"
   ```

**Why:**
- `5432:5432` - Exposed on all interfaces (dangerous)
- `127.0.0.1:5432:5432` - Only localhost (safe for dev)
- No ports section - Internal only (best for prod)

**Accessing Internal Services:**
```bash
# From another container (always works)
psql -h postgres -U postgres -d myapp

# From host in development
psql -h 127.0.0.1 -U postgres -d myapp

# From host in production (use docker exec)
docker exec -it myapp_postgres psql -U postgres -d myapp
```

### Container Users: Understanding the Mappings

**Why Non-Root Matters:**
```bash
# Running as root (BAD)
docker run -it --rm alpine sh
whoami  # root - can do anything

# Running as user (GOOD)
docker run -it --rm --user 1000:1000 alpine sh
whoami  # 1000 - limited permissions
```

**Setting Custom User:**
```yaml
# docker-compose.yml
myservice:
  image: myimage:latest
  user: "1000:1000"  # UID:GID
  volumes:
    - mydata:/app/data
```

**Volume Ownership:**
```bash
# Must match container user
sudo chown -R 1000:1000 ./data/myservice

# Or use docker to set ownership
docker run --rm -v $(pwd)/data/myservice:/data alpine \
  sh -c "chown -R 1000:1000 /data"
```

**Common UIDs:**
```bash
# System users (< 1000)
0     - root (never use)
101   - nginx
472   - grafana
999   - postgres, redis

# Regular users (>= 1000)
1000  - hasura, auth, custom services
1001  - minio
```

---

## Production Deployment Checklist

Use this checklist before deploying to production.

### Pre-Deployment Security Audit

```bash
# 1. Set production environment
export ENV=production

# 2. Run comprehensive security audit
nself audit security

# 3. Check for critical issues
if [ $? -ne 0 ]; then
  echo "Fix security issues before deploying"
  exit 1
fi

# 4. Verify all checks pass
# Expected: "All security checks passed!"
```

### Required Secrets

**Verify all secrets are strong and unique:**

```bash
# Check secret lengths
grep -E "(PASSWORD|SECRET|KEY)" .env | while read line; do
  key=$(echo $line | cut -d= -f1)
  value=$(echo $line | cut -d= -f2)
  length=${#value}

  if [ $length -lt 24 ]; then
    echo "⚠ $key is too short ($length chars)"
  else
    echo "✓ $key is strong ($length chars)"
  fi
done
```

**Required Secrets Checklist:**

- [ ] `POSTGRES_PASSWORD` (48+ chars, alphanumeric)
- [ ] `HASURA_GRAPHQL_ADMIN_SECRET` (96+ chars, hex)
- [ ] `HASURA_JWT_KEY` (96+ chars, hex)
- [ ] `MINIO_ROOT_PASSWORD` (32+ chars, alphanumeric)
- [ ] `S3_SECRET_KEY` (64+ chars, hex)
- [ ] `S3_ACCESS_KEY` (24+ chars, alphanumeric)
- [ ] `GRAFANA_ADMIN_PASSWORD` (32+ chars, alphanumeric)
- [ ] `MEILISEARCH_MASTER_KEY` (32+ chars, hex)

### CORS Configuration

**Production CORS Checklist:**

- [ ] No wildcard (*) in CORS
- [ ] Only production domains listed
- [ ] HTTPS required (no HTTP)
- [ ] Tested from browser
- [ ] Credentials allowed if needed

```bash
# Verify CORS setting
grep CORS .env

# Expected (production):
# HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com

# NOT:
# HASURA_GRAPHQL_CORS_DOMAIN=*
# HASURA_GRAPHQL_CORS_DOMAIN=http://localhost:*
```

### Port Security

**Production Port Checklist:**

- [ ] Database not exposed: `POSTGRES_EXPOSE_PORT=false`
- [ ] Redis not exposed (internal only)
- [ ] Only nginx on 80/443
- [ ] No debug ports open
- [ ] Firewall configured

```bash
# Verify no exposed ports
docker-compose config | grep -A 5 "ports:"

# Expected: Only nginx with 80/443
# nginx:
#   ports:
#     - "80:80"
#     - "443:443"
```

### SSL/TLS Setup

**SSL Configuration:**

```bash
# Option 1: Let's Encrypt (recommended)
SSL_MODE=letsencrypt
LETSENCRYPT_EMAIL=admin@myapp.com
BASE_DOMAIN=myapp.com

# Option 2: Custom certificate
SSL_MODE=custom
SSL_CERT_PATH=/path/to/cert.pem
SSL_KEY_PATH=/path/to/key.pem

# Option 3: Cloudflare (if using Cloudflare)
SSL_MODE=cloudflare
# Let Cloudflare handle SSL
```

**SSL Checklist:**

- [ ] Certificate valid for your domain
- [ ] Certificate not expired
- [ ] Private key secured (600 permissions)
- [ ] HTTPS redirect enabled
- [ ] HSTS header configured

**Test SSL:**
```bash
# Check certificate
openssl s_client -connect myapp.com:443 -servername myapp.com

# Check SSL grade
curl -s https://www.ssllabs.com/ssltest/analyze.html?d=myapp.com
```

### Monitoring and Alerting

**Enable Production Monitoring:**

```bash
# Enable monitoring stack
MONITORING_ENABLED=true

# Configure Grafana
GRAFANA_ADMIN_PASSWORD=strong-password-here

# Configure Alertmanager
ALERTMANAGER_SMTP_HOST=smtp.gmail.com
ALERTMANAGER_SMTP_FROM=alerts@myapp.com
ALERTMANAGER_SMTP_TO=team@myapp.com
```

**Monitoring Checklist:**

- [ ] Grafana accessible and configured
- [ ] Prometheus collecting metrics
- [ ] Alerts configured for critical services
- [ ] Log aggregation working (Loki)
- [ ] Dashboard created for key metrics

### Backup Configuration

**Production Backup Setup:**

```bash
# Enable daily backups
BACKUP_ENABLED=true
BACKUP_SCHEDULE="0 2 * * *"  # Daily at 2 AM
BACKUP_RETENTION_DAYS=30

# Cloud backup (recommended)
BACKUP_CLOUD_PROVIDER=s3
S3_BACKUP_BUCKET=myapp-backups
S3_BACKUP_ACCESS_KEY=xxx
S3_BACKUP_SECRET_KEY=xxx
```

**Backup Checklist:**

- [ ] Backups enabled and scheduled
- [ ] Backup location accessible
- [ ] Backup restoration tested
- [ ] Off-site backups configured
- [ ] Backup monitoring enabled

### Final Verification

```bash
# Run complete production checklist
nself audit security

# Start services
nself build && nself start

# Verify all services healthy
docker ps --filter "health=healthy"

# Test application
curl -I https://myapp.com
curl -I https://api.myapp.com

# Check logs for errors
docker-compose logs --tail=100

# Monitor for 10 minutes
watch -n 30 'docker ps'
```

---

## Compliance Considerations

nself v0.10.0 provides features to support various compliance frameworks.

### SOC 2 Compliance

**Type II Controls Supported:**

1. **CC6.1 - Logical and Physical Access Controls**
   - Non-root container users
   - Strong password requirements
   - Role-based access control

2. **CC7.2 - System Monitoring**
   - Prometheus metrics
   - Grafana dashboards
   - Audit logging

3. **CC7.3 - Security Monitoring**
   - Security audit command
   - Automated hardening
   - Regular security scans

**Implementation:**
```bash
# Enable audit logging
NSELF_AUDIT_ENABLED=true

# Enable monitoring
MONITORING_ENABLED=true

# Run security audits regularly
nself audit security >> /var/log/nself/security-audits.log
```

**Documentation Needed:**
- Security policy document
- Access control procedures
- Incident response plan
- Change management procedures

### GDPR Requirements

**Data Protection Features:**

1. **Data Encryption**
   - At-rest: PostgreSQL encryption
   - In-transit: SSL/TLS required
   - Backups: Encrypted storage

2. **Access Controls**
   - Strong authentication
   - Password policies
   - Session management

3. **Audit Trails**
   - Audit logging enabled
   - All data access logged
   - Log retention policies

**Implementation:**
```bash
# Enable encryption
POSTGRES_SSL_MODE=require
MINIO_ENCRYPTION=true

# Enable audit logging
NSELF_AUDIT_ENABLED=true
AUDIT_RETENTION_DAYS=2555  # 7 years for GDPR

# Configure data retention
DATA_RETENTION_DAYS=365
```

**GDPR Checklist:**

- [ ] Privacy policy published
- [ ] Data processing documented
- [ ] Consent mechanisms implemented
- [ ] Data export functionality
- [ ] Data deletion functionality
- [ ] Breach notification procedures
- [ ] DPO appointed (if required)

### HIPAA Considerations

**Technical Safeguards:**

1. **Access Control (§164.312(a)(1))**
   - Unique user identification
   - Emergency access procedures
   - Automatic logoff
   - Encryption and decryption

2. **Audit Controls (§164.312(b))**
   - Hardware, software, procedural mechanisms
   - Record and examine activity

3. **Integrity (§164.312(c)(1))**
   - Mechanisms to authenticate ePHI
   - Protect ePHI from alteration/destruction

4. **Transmission Security (§164.312(e)(1))**
   - Integrity controls
   - Encryption

**Implementation:**
```bash
# Encryption required
SSL_MODE=letsencrypt
POSTGRES_SSL_MODE=require

# Audit logging required
NSELF_AUDIT_ENABLED=true
AUDIT_LOG_PHI_ACCESS=true

# Session timeout
AUTH_JWT_ACCESS_TOKEN_EXPIRES_IN=900  # 15 minutes
AUTH_REQUIRE_REAUTH_FOR_PHI=true
```

**HIPAA Checklist:**

- [ ] Risk assessment completed
- [ ] Business Associate Agreements (BAAs) signed
- [ ] Encryption implemented (at-rest and in-transit)
- [ ] Access controls implemented
- [ ] Audit controls implemented
- [ ] Disaster recovery plan
- [ ] Breach notification procedures

### Data Encryption

**At-Rest Encryption:**

```bash
# PostgreSQL
# Enable pgcrypto extension
POSTGRES_EXTENSIONS=pgcrypto,uuid-ossp

# Encrypt sensitive columns
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE patients (
  id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
  name TEXT,
  ssn TEXT,  -- Will be encrypted
  email TEXT
);

-- Encrypt SSN
INSERT INTO patients (name, ssn, email)
VALUES ('John Doe', pgp_sym_encrypt('123-45-6789', 'encryption-key'), 'john@example.com');

-- Decrypt SSN
SELECT name, pgp_sym_decrypt(ssn::bytea, 'encryption-key') as ssn, email
FROM patients;
```

**In-Transit Encryption:**

```bash
# Force SSL for database connections
POSTGRES_SSL_MODE=require

# Force HTTPS for all web traffic
NGINX_FORCE_HTTPS=true

# TLS 1.2+ only
NGINX_SSL_PROTOCOLS="TLSv1.2 TLSv1.3"
```

**Storage Encryption:**

```bash
# MinIO server-side encryption
MINIO_ENCRYPTION=true
MINIO_ENCRYPTION_KEY=your-encryption-key-here

# Encrypt backups
BACKUP_ENCRYPTION=true
BACKUP_ENCRYPTION_KEY=your-backup-encryption-key
```

---

## Troubleshooting

Common issues and solutions when hardening security.

### Permission Errors with Non-Root Containers

**Symptom:**
```
Error: Permission denied: '/var/lib/postgresql/data'
```

**Cause:**
Volume ownership doesn't match container user.

**Solution:**
```bash
# Option 1: Fix ownership on host
sudo chown -R 999:999 ./data/postgres
sudo chown -R 1001:1001 ./data/minio
sudo chown -R 472:472 ./data/grafana

# Option 2: Use docker to fix ownership
docker run --rm -v $(pwd)/data/postgres:/data alpine \
  sh -c "chown -R 999:999 /data"

# Option 3: Reinitialize (destructive)
nself stop
rm -rf ./data/postgres
nself start  # Will recreate with correct permissions
```

**Prevention:**
```bash
# nself init automatically sets correct permissions
nself init

# Or run setup script
bash scripts/setup-permissions.sh
```

### CORS Blocking Legitimate Requests

**Symptom:**
```
Access to XMLHttpRequest has been blocked by CORS policy
```

**Cause:**
- Domain not in CORS whitelist
- HTTP instead of HTTPS
- Credentials not allowed

**Solution:**
```bash
# 1. Check current CORS setting
grep CORS .env

# 2. Add your domain
# Before:
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com

# After:
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com,https://admin.myapp.com

# 3. Rebuild and restart
nself build && nself restart

# 4. Test from browser console
fetch('https://api.myapp.com/v1/graphql', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  credentials: 'include',
  body: JSON.stringify({ query: '{ __typename }' })
})
```

**Debug CORS:**
```bash
# Check CORS headers
curl -I -X OPTIONS \
  -H "Origin: https://app.myapp.com" \
  -H "Access-Control-Request-Method: POST" \
  https://api.myapp.com/v1/graphql

# Expected response includes:
# Access-Control-Allow-Origin: https://app.myapp.com
# Access-Control-Allow-Credentials: true
```

### Missing Required Secrets

**Symptom:**
```
Error: Missing environment variable: HASURA_GRAPHQL_ADMIN_SECRET
```

**Cause:**
Secret not set in .env file.

**Solution:**
```bash
# 1. Generate secret
SECRET=$(openssl rand -hex 48)

# 2. Add to .env
echo "HASURA_GRAPHQL_ADMIN_SECRET=$SECRET" >> .env

# 3. Rebuild
nself build && nself restart
```

**Automated Fix:**
```bash
# Use hardening command
nself harden secrets

# Or regenerate all secrets
nself init --regenerate-secrets
```

### Port Conflicts

**Symptom:**
```
Error: Bind for 0.0.0.0:5432 failed: port is already allocated
```

**Cause:**
Another service using the same port.

**Solution:**
```bash
# 1. Find what's using the port
sudo lsof -i :5432
# or
netstat -tulpn | grep 5432

# 2. Option A: Stop conflicting service
sudo systemctl stop postgresql

# 3. Option B: Change nself port
# Edit .env:
POSTGRES_PORT=5433

# 4. Rebuild
nself build && nself restart
```

**Production:**
```bash
# Don't expose ports in production
POSTGRES_EXPOSE_PORT=false

# Access via docker exec instead
docker exec -it myapp_postgres psql -U postgres
```

---

## Advanced Topics

### Custom Secret Rotation Schedules

**Manual Rotation:**
```bash
# Rotate specific secret
nself harden secrets

# Or manual
NEW_SECRET=$(openssl rand -hex 48)
sed -i "s/POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$NEW_SECRET/" .env
nself restart
```

**Automated Rotation (Cron):**
```bash
# Create rotation script
cat > /opt/nself/rotate-secrets.sh << 'EOF'
#!/bin/bash
cd /var/www/myapp
source .env

# Rotate secrets
nself harden secrets

# Rebuild and restart
nself build && nself restart

# Notify team
curl -X POST https://slack.com/api/chat.postMessage \
  -H "Authorization: Bearer $SLACK_TOKEN" \
  -d "channel=#devops" \
  -d "text=Secrets rotated for myapp"
EOF

# Schedule monthly rotation
0 2 1 * * /opt/nself/rotate-secrets.sh >> /var/log/nself/rotation.log 2>&1
```

### CI/CD Integration

**GitHub Actions:**
```yaml
# .github/workflows/deploy.yml
name: Deploy to Production

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install nself
        run: curl -sSL https://install.nself.org | bash

      - name: Copy secrets
        run: |
          echo "${{ secrets.ENV_PRODUCTION }}" > .env

      - name: Security audit
        run: |
          nself audit security
          if [ $? -ne 0 ]; then
            echo "Security audit failed"
            exit 1
          fi

      - name: Deploy
        run: |
          nself build
          nself start

      - name: Health check
        run: |
          sleep 30
          curl -f https://api.myapp.com/healthz || exit 1
```

### Zero-Downtime Hardening

**Strategy:**
```bash
# 1. Create new secrets (don't apply yet)
NEW_POSTGRES_PW=$(openssl rand -hex 48)
NEW_HASURA_SECRET=$(openssl rand -hex 96)

# 2. Update auth service to accept BOTH old and new secrets
# (requires code changes to support dual-secret validation)

# 3. Deploy updated auth service
nself deploy auth

# 4. Update .env with new secrets
sed -i "s/POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$NEW_POSTGRES_PW/" .env
sed -i "s/HASURA_GRAPHQL_ADMIN_SECRET=.*/HASURA_GRAPHQL_ADMIN_SECRET=$NEW_HASURA_SECRET/" .env

# 5. Rolling restart (one service at a time)
nself restart postgres --wait
nself restart hasura --wait
nself restart auth --wait

# 6. Remove old secret support from auth service
# 7. Deploy final auth service
```

### Security Monitoring

**Prometheus Alerts:**
```yaml
# monitoring/alerts/security.yml
groups:
  - name: security
    interval: 1m
    rules:
      - alert: WeakSecretDetected
        expr: nself_security_audit_failures > 0
        for: 5m
        annotations:
          summary: "Weak security configuration detected"

      - alert: UnauthorizedAccess
        expr: rate(nginx_http_requests_total{status="401"}[5m]) > 10
        annotations:
          summary: "High rate of unauthorized access attempts"
```

**Grafana Dashboard:**
```bash
# Import security dashboard
curl -X POST https://grafana.myapp.com/api/dashboards/import \
  -H "Authorization: Bearer $GRAFANA_API_KEY" \
  -d @monitoring/dashboards/security.json
```

---

## Summary

nself v0.10.0 provides comprehensive security hardening with:

- **Secure by Default:** Strong secrets generated automatically
- **Environment-Aware:** Different security levels for dev/staging/prod
- **Automated Tools:** `nself audit security` and `nself harden`
- **Compliance Ready:** Supports SOC 2, GDPR, HIPAA
- **Best Practices:** Non-root containers, CORS configuration, port security

**Quick Security Workflow:**
```bash
# 1. Initialize with strong secrets
nself init

# 2. Configure for your environment
export ENV=production

# 3. Run security audit
nself audit security

# 4. Apply hardening if needed
nself harden

# 5. Deploy
nself build && nself start

# 6. Verify
curl -f https://api.myapp.com/healthz
```

For more information:
- [Migration Guide v0.10.0](MIGRATION-V0.10.0.md)
- [Security Best Practices](SECURITY-BEST-PRACTICES.md)
- [Compliance Guide](COMPLIANCE-GUIDE.md)

---

**Last Updated:** February 11, 2026
**nself Version:** v0.10.0+
