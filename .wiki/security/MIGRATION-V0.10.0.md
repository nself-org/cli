# Migration Guide: nself v0.10.0

**Version:** v0.10.0
**Release Date:** February 11, 2026
**Migration Time:** 15-30 minutes
**Downtime Required:** 5-10 minutes

This guide helps you safely upgrade to nself v0.10.0, which includes comprehensive security hardening and breaking changes to default configurations.

---

## Table of Contents

1. [Overview](#overview)
2. [Breaking Changes](#breaking-changes)
3. [Impact Assessment](#impact-assessment)
4. [Migration Path](#migration-path)
5. [Environment-Specific Migration](#environment-specific-migration)
6. [Step-by-Step Instructions](#step-by-step-instructions)
7. [Troubleshooting Migration Issues](#troubleshooting-migration-issues)
8. [Post-Migration Checklist](#post-migration-checklist)
9. [Rollback Instructions](#rollback-instructions)

---

## Overview

### What Changed in v0.10.0

nself v0.10.0 introduces **security-first deployment** with 39 fixes across 3 phases:

**Phase 1: Core Infrastructure (14 issues)**
- Removed all default weak credentials
- Implemented strong secret generation (32-96 chars)
- Added non-root container users for all services
- Implemented environment-aware CORS configuration

**Phase 2: Service Security (15 issues)**
- Mailpit authentication in production (disabled by default)
- Redis password enforcement
- MinIO credential hardening
- Search service API key generation
- Custom service security templates

**Phase 3: Operations (10 issues)**
- Security audit command (`nself audit security`)
- Automated hardening wizard (`nself harden`)
- Port exposure management (conditional by environment)
- Configuration validation

### Why These Changes Matter

**Security Impact:**
- **Before:** Default passwords like `postgres-dev-password` in templates
- **After:** Strong random 48-character passwords auto-generated
- **Result:** Eliminates #1 attack vector (weak credentials)

**Compliance Impact:**
- **Before:** Manual security configuration prone to errors
- **After:** Automated security auditing and hardening
- **Result:** SOC 2, GDPR, HIPAA compliance ready

**Operational Impact:**
- **Before:** No way to verify security posture
- **After:** `nself audit security` shows all issues
- **Result:** Continuous security monitoring

### Who Needs to Migrate

| Your Situation | Migration Required | Priority |
|----------------|-------------------|----------|
| New nself deployment | No | N/A |
| Development only (no prod) | Yes | Low |
| Staging/pre-prod | Yes | Medium |
| Production with default secrets | Yes | **CRITICAL** |
| Production with custom secrets | Yes | Medium |

---

## Breaking Changes

### 1. Removed Default Credentials

**CRITICAL: This is the most impactful change.**

#### What Was Removed

All weak default secrets have been removed from templates and examples:

| Secret | Old Default | New Behavior |
|--------|-------------|--------------|
| `POSTGRES_PASSWORD` | `postgres-dev-password` | Generated (32-48 chars) |
| `HASURA_GRAPHQL_ADMIN_SECRET` | `hasura-admin-secret-dev` | Generated (64-96 chars) |
| `HASURA_JWT_KEY` | `development-secret-key-minimum-32-characters-long` | Generated (64-96 chars) |
| `MINIO_ROOT_PASSWORD` | `minioadmin` | Generated (32-48 chars) |
| `S3_SECRET_KEY` | `storage-secret-key-dev` | Generated (48-64 chars) |
| `S3_ACCESS_KEY` | `storage-access-key-dev` | Generated (24-32 chars) |

#### Why They Were Removed

These default secrets were:
- Publicly documented
- Easy to guess
- Used in production by mistake
- A major security vulnerability

#### Impact

**New Deployments:**
- `nself init` generates strong secrets automatically
- No action required

**Existing Deployments:**
- Old secrets still work (backward compatible)
- **Security audit will flag them as weak**
- You must rotate secrets to pass security checks

### 2. Environment-Aware CORS

#### What Changed

CORS configuration now varies by environment automatically:

**Before v0.10.0:**
```bash
# Same CORS for all environments
HASURA_GRAPHQL_CORS_DOMAIN=http://localhost:*,https://*.local.nself.org
```

**After v0.10.0:**
```bash
# Development
HASURA_GRAPHQL_CORS_DOMAIN=http://localhost:*,http://*.local.nself.org,https://*.local.nself.org

# Staging
HASURA_GRAPHQL_CORS_DOMAIN=https://*.staging.myapp.com,http://localhost:3000

# Production
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com
```

#### Impact

**Development:** No change - still permissive
**Staging:** Slightly more restrictive
**Production:** **Wildcard (*) CORS now fails security audit**

If you're using `CORS=*` in production, you'll need to specify your actual domains.

### 3. Non-Root Container Users

#### What Changed

All services now run as non-root users with explicit UID/GID.

**Before v0.10.0:**
```yaml
# docker-compose.yml
postgres:
  image: postgres:16-alpine
  # Runs as root by default
```

**After v0.10.0:**
```yaml
# docker-compose.yml
postgres:
  image: postgres:16-alpine
  user: "999:999"  # postgres user
```

#### Impact

**New Deployments:** Works automatically

**Existing Deployments:** Volume ownership issues may occur

**Fix:**
```bash
# Update volume ownership
sudo chown -R 999:999 ./data/postgres
sudo chown -R 1001:1001 ./data/minio
sudo chown -R 472:472 ./data/grafana
```

### 4. Port Exposure Changes

#### What Changed

Database port exposure is now conditional based on environment.

**Before v0.10.0:**
```yaml
# Always exposed
postgres:
  ports:
    - "5432:5432"
```

**After v0.10.0:**
```yaml
# Development - exposed on localhost only
postgres:
  ports:
    - "127.0.0.1:5432:5432"

# Production - not exposed at all
postgres:
  # No ports section - internal Docker network only
```

#### Impact

**Development:** Database accessible via `localhost:5432`
**Production:** Database only accessible via Docker network

**Access in Production:**
```bash
# Use docker exec instead
docker exec -it myapp_postgres psql -U postgres -d myapp
```

### 5. Mailpit Production Behavior

#### What Changed

Mailpit is now disabled automatically in production.

**Before v0.10.0:**
```yaml
# Mailpit enabled in all environments
mailpit:
  image: axllent/mailpit
```

**After v0.10.0:**
```bash
# Development/Staging: Enabled
MAILPIT_ENABLED=true

# Production: Disabled (automatically)
# Use real email provider instead
EMAIL_PROVIDER=sendgrid
```

#### Impact

**Development/Staging:** No change
**Production:** Must configure real email provider (SendGrid, Postmark, SES, etc.)

---

## Impact Assessment

### For New Deployments

**Impact:** None (all defaults are secure)

**Action Required:** None

**Recommendation:** Just run `nself init && nself build && nself start`

### For Existing Deployments

**Run Security Audit to Assess Impact:**

```bash
# Clone/upgrade nself to v0.10.0
cd /path/to/nself
git pull origin main

# Go to your project
cd /path/to/your-project

# Run security audit
nself audit security
```

**Interpret Results:**

**0 issues found:**
- Your deployment is already secure
- Safe to upgrade with minimal risk

**1-5 issues found:**
- Minor security improvements needed
- Can upgrade with `nself harden` fixes

**6+ issues found:**
- Significant security gaps
- Plan maintenance window for secret rotation
- Follow step-by-step migration carefully

**Critical issues (wildcard CORS, exposed ports in prod):**
- Immediate action required
- Schedule emergency maintenance

### For Production Deployments

**High Priority Migration If:**
- [ ] Using default secrets (postgres-dev-password, hasura-admin-secret-dev, etc.)
- [ ] Using wildcard CORS (*) in production
- [ ] Database port exposed externally (5432:5432)
- [ ] Services running as root
- [ ] No security auditing in place

**Medium Priority Migration If:**
- [ ] Custom secrets but weak (<24 chars)
- [ ] Some services as non-root, some as root
- [ ] No automated security validation

**Low Priority Migration If:**
- [ ] Strong custom secrets already
- [ ] CORS properly configured
- [ ] Services already non-root
- [ ] Security measures in place

---

## Migration Path

### Safe Upgrade Process (8 Steps)

This is the recommended migration path that minimizes risk and downtime.

#### Step 1: Backup Current Configuration

```bash
# Backup your entire project
cd /path/to/your-project
tar -czf ../project-backup-$(date +%Y%m%d-%H%M%S).tar.gz .

# Backup critical files
cp .env .env.backup-pre-migration
cp docker-compose.yml docker-compose.yml.backup
cp -r data/ data.backup/

# Verify backups
ls -lh ../*.tar.gz
ls -lh .env.backup-*
```

#### Step 2: Upgrade nself

```bash
# Option A: Homebrew
brew upgrade nself

# Option B: Manual
cd /path/to/nself
git pull origin main
sudo make install

# Verify version
nself version
# Expected: v0.10.0 or higher
```

#### Step 3: Audit Current Setup

```bash
# Run security audit
nself audit security

# Save results
nself audit security > security-audit-pre-migration.txt

# Review issues
cat security-audit-pre-migration.txt
```

#### Step 4: Review Proposed Changes

```bash
# See what will change (dry run)
nself harden --dry-run

# Expected output:
# - Secrets to rotate
# - CORS changes
# - Port exposure changes
# - Container user updates
```

#### Step 5: Apply Hardening

```bash
# Interactive (recommended for first time)
nself harden

# Or automatic (if you reviewed and approved)
nself harden all
```

#### Step 6: Rebuild Configuration

```bash
# Regenerate docker-compose.yml with new settings
nself build

# Review changes
git diff docker-compose.yml
```

#### Step 7: Test in Non-Production First

**If you have staging:**
```bash
# Apply to staging first
cd /path/to/staging-project
nself harden all
nself build
nself restart

# Test for 24-48 hours
# Monitor for issues
```

**If no staging, use local test:**
```bash
# Create test copy
cp -r /path/to/project /path/to/project-test
cd /path/to/project-test

# Apply migration
nself harden all
nself build
nself start

# Test thoroughly
```

#### Step 8: Apply to Production (with Downtime)

```bash
# Schedule maintenance window

# 1. Notify users
echo "System maintenance in progress..."

# 2. Stop services
nself stop

# 3. Apply hardening
nself harden all

# 4. Rebuild
nself build

# 5. Start services
nself start

# 6. Verify health
nself status
docker ps
curl -f https://api.myapp.com/healthz

# 7. Monitor for 1 hour
watch -n 30 'docker ps --format "table {{.Names}}\t{{.Status}}"'
```

### Estimated Downtime

| Environment | Downtime | Reason |
|-------------|----------|--------|
| Development | 0 minutes | Can migrate while running |
| Staging | 5 minutes | Services restart |
| Production | 5-10 minutes | Services restart + verification |

### Rollback Plan

If issues occur during migration:

```bash
# 1. Stop services
nself stop

# 2. Restore configuration
cp .env.backup-pre-migration .env
cp docker-compose.yml.backup docker-compose.yml

# 3. Rebuild with old config
nself build

# 4. Start services
nself start

# 5. Verify
curl -f https://api.myapp.com/healthz
```

---

## Environment-Specific Migration

### Development Environment

**Characteristics:**
- Running on localhost
- Using default secrets (acceptable for local)
- Frequent restarts

**Migration Steps:**

```bash
# 1. Upgrade nself
brew upgrade nself

# 2. Pull latest changes
cd /path/to/project
git pull

# 3. Audit (expect some warnings - that's okay for dev)
nself audit security

# 4. Optional: Rotate secrets for consistency
nself harden secrets

# 5. Rebuild and restart
nself build && nself restart

# 6. Continue developing
```

**Downtime:** 2-3 minutes

**Impact:** Minimal - secrets may change but that's fine for local dev

### Staging Environment

**Characteristics:**
- Pre-production testing
- Should mirror production security
- Can tolerate brief downtime

**Migration Steps:**

```bash
# 1. Notify team
echo "Staging maintenance starting..."

# 2. Backup
tar -czf ~/staging-backup-$(date +%Y%m%d).tar.gz .

# 3. Upgrade nself
sudo apt update && sudo apt upgrade nself

# 4. Audit
nself audit security > audit-before.txt

# 5. Apply hardening
nself harden all

# 6. Rebuild
nself build

# 7. Stop and start (controlled restart)
nself stop
nself start

# 8. Verify
nself status
curl -f https://staging.myapp.com/healthz

# 9. Test for 24 hours before production
# - Run integration tests
# - Manual testing
# - Monitor logs
```

**Downtime:** 5-7 minutes

**Impact:** Brief downtime, test thoroughly

### Production Environment

**Characteristics:**
- Zero-tolerance for issues
- Must maintain data integrity
- Requires careful planning

**Migration Steps:**

```bash
# PREPARATION (days before)
# 1. Test in staging first
# 2. Review all changes
# 3. Schedule maintenance window
# 4. Notify users/stakeholders
# 5. Prepare rollback plan

# MAINTENANCE WINDOW
# 1. Enable maintenance mode
echo "Maintenance in progress" > /var/www/html/maintenance.html
# (Configure nginx to show maintenance page)

# 2. Backup everything
tar -czf /backup/prod-pre-migration-$(date +%Y%m%d-%H%M%S).tar.gz /var/www/myapp
pg_dump -h localhost -U postgres myapp > /backup/myapp-$(date +%Y%m%d-%H%M%S).sql

# 3. Verify backups
ls -lh /backup/*.tar.gz
ls -lh /backup/*.sql

# 4. Stop services gracefully
nself stop

# 5. Upgrade nself
cd /opt/nself
git pull origin main
sudo make install

# 6. Audit current config
cd /var/www/myapp
nself audit security > /var/log/nself/audit-before-migration.txt

# 7. Apply hardening
nself harden all

# 8. Review changes
git diff .env docker-compose.yml

# 9. Rebuild configuration
nself build

# 10. Start services
nself start

# 11. Health checks
sleep 30
nself status

# 12. Verify each service
curl -f https://api.myapp.com/healthz || echo "API health check failed"
curl -f https://auth.myapp.com/healthz || echo "Auth health check failed"
curl -f https://myapp.com || echo "App health check failed"

# 13. Run post-migration audit
nself audit security > /var/log/nself/audit-after-migration.txt

# 14. Compare audits
diff /var/log/nself/audit-before-migration.txt /var/log/nself/audit-after-migration.txt

# 15. Disable maintenance mode
rm /var/www/html/maintenance.html

# 16. Monitor for 1 hour
watch -n 30 'docker ps && docker stats --no-stream'

# 17. Monitor logs
docker-compose logs -f --tail=100
```

**Downtime:** 10-15 minutes (including verification)

**Impact:** Brief downtime, significant security improvement

---

## Step-by-Step Instructions

### Detailed Migration Procedure

#### 1. Pre-Migration Checklist

- [ ] Read this entire migration guide
- [ ] Test migration in development first
- [ ] Test migration in staging (if available)
- [ ] Schedule production maintenance window
- [ ] Notify users/stakeholders of downtime
- [ ] Backup all data (database, files, config)
- [ ] Verify backup integrity
- [ ] Have rollback plan ready
- [ ] Team members on standby

#### 2. Backup Current System

```bash
#!/bin/bash
# backup.sh - Comprehensive backup script

PROJECT_DIR="/var/www/myapp"
BACKUP_DIR="/backup"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo "Starting backup..."

# 1. Project files
cd "$PROJECT_DIR"
tar -czf "$BACKUP_DIR/myapp-files-$TIMESTAMP.tar.gz" \
  .env \
  docker-compose.yml \
  nginx/ \
  data/ \
  --exclude='data/postgres/*' \
  --exclude='node_modules'

# 2. PostgreSQL database
docker exec myapp_postgres pg_dump -U postgres myapp > "$BACKUP_DIR/myapp-db-$TIMESTAMP.sql"

# 3. MinIO data (if applicable)
docker exec myapp_minio mc mirror /data "$BACKUP_DIR/minio-$TIMESTAMP/"

# 4. Verify backups
echo "Backup created:"
ls -lh "$BACKUP_DIR"/*$TIMESTAMP*

# 5. Test database backup
echo "Testing database backup..."
head -n 50 "$BACKUP_DIR/myapp-db-$TIMESTAMP.sql"

echo "Backup complete!"
```

#### 3. Upgrade nself CLI

```bash
# Check current version
nself version

# Upgrade via Homebrew (recommended)
brew upgrade nself

# Or manual install
cd /opt/nself
git pull origin main
sudo make install

# Verify upgrade
nself version
# Expected: v0.10.0 or higher

# Test command
nself audit security --help
```

#### 4. Run Pre-Migration Audit

```bash
# Navigate to project
cd /var/www/myapp

# Run comprehensive audit
nself audit security > /tmp/audit-before.txt

# Review results
cat /tmp/audit-before.txt

# Count issues
grep "⚠" /tmp/audit-before.txt | wc -l
grep "✗" /tmp/audit-before.txt | wc -l

# Identify critical issues
grep "CRITICAL" /tmp/audit-before.txt
```

#### 5. Review Proposed Changes

```bash
# See what will change (if dry-run supported)
nself harden --dry-run

# Or manually review with audit
nself audit security secrets      # Shows weak secrets
nself audit security cors         # Shows CORS issues
nself audit security ports        # Shows port exposure issues
nself audit security containers   # Shows root containers
```

#### 6. Apply Security Hardening

```bash
# Option A: Interactive (recommended first time)
nself harden
# Answer prompts:
# - Review each proposed change
# - Confirm before applying

# Option B: Automatic (after reviewing)
nself harden all
# Applies all fixes automatically

# Option C: Selective
nself harden secrets  # Only rotate secrets
nself harden cors     # Only fix CORS
```

#### 7. Rebuild Configuration

```bash
# Regenerate docker-compose.yml
nself build

# Review what changed
git diff docker-compose.yml

# Key changes to look for:
# - user: directives added
# - ports: sections modified/removed
# - environment: variables updated
```

#### 8. Stop Services

```bash
# Graceful stop with timeout
nself stop

# Verify all stopped
docker ps
# Expected: No myapp containers running

# Optional: Remove old containers
docker-compose down
```

#### 9. Fix Volume Permissions (if needed)

```bash
# Check if non-root users need permission fixes
ls -la data/

# Fix ownership for each service
sudo chown -R 999:999 data/postgres
sudo chown -R 1001:1001 data/minio
sudo chown -R 472:472 data/grafana

# Verify
ls -la data/
```

#### 10. Start Services

```bash
# Start all services
nself start

# Watch startup
docker-compose logs -f

# Wait for health checks
sleep 30

# Check status
nself status
docker ps
```

#### 11. Verify Services

```bash
# Health checks
curl -f https://api.myapp.com/healthz || echo "API failed"
curl -f https://auth.myapp.com/healthz || echo "Auth failed"
curl -f https://myapp.com || echo "App failed"

# Database
docker exec myapp_postgres psql -U postgres -c "SELECT version();"

# Redis (if enabled)
docker exec myapp_redis redis-cli ping

# MinIO (if enabled)
docker exec myapp_minio mc admin info local
```

#### 12. Run Post-Migration Audit

```bash
# Run audit again
nself audit security > /tmp/audit-after.txt

# Compare before/after
diff /tmp/audit-before.txt /tmp/audit-after.txt

# Expected: Fewer or zero issues
```

#### 13. Test Application Functionality

```bash
# Functional tests
# - User login
# - API queries
# - File uploads
# - Email sending

# Example GraphQL test
curl -X POST https://api.myapp.com/v1/graphql \
  -H "Content-Type: application/json" \
  -H "x-hasura-admin-secret: $NEW_ADMIN_SECRET" \
  -d '{"query":"{ __typename }"}'

# Example auth test
curl -X POST https://auth.myapp.com/signin/email-password \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'
```

#### 14. Monitor for Issues

```bash
# Monitor containers
watch -n 10 'docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"'

# Monitor logs for errors
docker-compose logs -f | grep -i error

# Monitor resource usage
docker stats --no-stream

# Monitor for 1 hour minimum
```

---

## Troubleshooting Migration Issues

### Services Won't Start (Missing Secrets)

**Symptom:**
```
Error: Missing environment variable: HASURA_GRAPHQL_ADMIN_SECRET
```

**Cause:** Secret not generated or not in .env

**Solution:**
```bash
# Regenerate secrets
nself harden secrets

# Or manually
echo "HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -hex 48)" >> .env

# Rebuild and restart
nself build && nself restart
```

### Permission Denied (Volume Ownership)

**Symptom:**
```
postgres: Permission denied for directory "/var/lib/postgresql/data"
```

**Cause:** Volume owned by root, container running as non-root

**Solution:**
```bash
# Stop services
nself stop

# Fix ownership
sudo chown -R 999:999 data/postgres
sudo chown -R 1001:1001 data/minio
sudo chown -R 472:472 data/grafana

# Start services
nself start
```

### CORS Errors (Too Restrictive)

**Symptom:**
```
Access to XMLHttpRequest has been blocked by CORS policy
```

**Cause:** Domain not in CORS whitelist after hardening

**Solution:**
```bash
# Add your domain to CORS
# Edit .env:
HASURA_GRAPHQL_CORS_DOMAIN=https://*.myapp.com,https://app.myapp.com

# Rebuild and restart
nself build && nself restart
```

### Port Access Issues (Database Not Exposed)

**Symptom:**
```
psql: could not connect to server: Connection refused
```

**Cause:** Database port no longer exposed in production

**Solution:**
```bash
# Option 1: Use docker exec (recommended)
docker exec -it myapp_postgres psql -U postgres -d myapp

# Option 2: Re-expose port (not recommended for prod)
# Edit .env:
POSTGRES_EXPOSE_PORT=true
# Rebuild and restart
nself build && nself restart
```

### Email Not Sending (Mailpit Disabled)

**Symptom:**
```
SMTP connection failed
```

**Cause:** Mailpit disabled in production, no email provider configured

**Solution:**
```bash
# Configure real email provider
# Edit .env:
EMAIL_PROVIDER=sendgrid
SENDGRID_API_KEY=SG.xxxxx

# Or for development, re-enable Mailpit:
ENV=dev
MAILPIT_ENABLED=true

# Rebuild and restart
nself build && nself restart
```

---

## Post-Migration Checklist

### Immediate Verification (within 1 hour)

- [ ] All containers running: `docker ps`
- [ ] No container restarts: `docker ps --format "{{.Names}}: {{.Status}}"`
- [ ] API responding: `curl -f https://api.myapp.com/healthz`
- [ ] Auth working: Test login
- [ ] Database accessible: `docker exec myapp_postgres psql -U postgres -c "SELECT 1"`
- [ ] No errors in logs: `docker-compose logs --tail=100 | grep -i error`

### Functional Testing (within 24 hours)

- [ ] User registration works
- [ ] User login works
- [ ] API queries return data
- [ ] File uploads work (if applicable)
- [ ] Email sending works (if applicable)
- [ ] Search works (if applicable)
- [ ] Background jobs running (if applicable)
- [ ] Admin interface accessible
- [ ] Monitoring dashboards showing data

### Security Verification

```bash
# Run security audit
nself audit security

# Expected: "All security checks passed!"
# or significantly fewer issues than before

# Verify secrets changed
grep "PASSWORD\|SECRET\|KEY" .env | wc -l
# Should see all secrets

# Verify no default secrets
grep -i "dev-password\|admin-secret-dev\|minioadmin" .env
# Should return nothing

# Verify CORS
grep CORS .env
# Should NOT contain "*" in production

# Verify port exposure
grep "EXPOSE_PORT" .env
# Should be false in production
```

### Performance Monitoring (ongoing)

```bash
# Monitor resource usage
docker stats

# Check response times
time curl -s https://api.myapp.com/healthz

# Monitor error rates
docker-compose logs --since 1h | grep -c error

# Check disk usage
df -h
docker system df
```

---

## Rollback Instructions

If you need to rollback the migration:

### Quick Rollback (5 minutes)

```bash
# 1. Stop current services
nself stop

# 2. Restore configuration
cp .env.backup-pre-migration .env
cp docker-compose.yml.backup docker-compose.yml

# 3. Restore data (if needed)
rm -rf data/
cp -r data.backup/ data/

# 4. Rebuild with old config
nself build

# 5. Start services
nself start

# 6. Verify
curl -f https://api.myapp.com/healthz
```

### Full Rollback (15 minutes)

```bash
# 1. Stop services
nself stop
docker-compose down -v  # Remove volumes

# 2. Restore full backup
cd /var/www
rm -rf myapp/
tar -xzf /backup/myapp-files-TIMESTAMP.tar.gz -C myapp/

# 3. Restore database
docker-compose up -d postgres
sleep 10
cat /backup/myapp-db-TIMESTAMP.sql | docker exec -i myapp_postgres psql -U postgres

# 4. Start all services
cd myapp
nself start

# 5. Verify
nself status
```

### After Rollback

```bash
# Document why rollback was needed
echo "Rollback performed at $(date)" >> /var/log/nself/rollback.log
echo "Reason: [describe issue]" >> /var/log/nself/rollback.log

# Report issue
# https://github.com/nself-org/cli/issues

# Plan retry
# - Identify root cause
# - Test fix in staging
# - Schedule new migration attempt
```

---

## Summary

### Migration Success Criteria

Migration is successful when:

- [ ] All services running
- [ ] Security audit passes (0 critical issues)
- [ ] Application functionality works
- [ ] No errors in logs
- [ ] Performance acceptable
- [ ] Users can access system

### Next Steps After Migration

1. **Monitor for 48 hours**
   - Watch for any issues
   - Check logs regularly
   - Monitor user reports

2. **Update documentation**
   - Document new secrets (securely)
   - Update runbooks
   - Update team knowledge base

3. **Schedule next security audit**
   - Monthly: `nself audit security`
   - Quarterly: Full security review
   - Annually: External security audit

4. **Consider additional hardening**
   - Enable monitoring if not already
   - Configure backups
   - Set up alerts
   - Review [Security Hardening Guide](HARDENING-GUIDE.md)

### Getting Help

If you encounter issues:

1. **Check troubleshooting section** (above)
2. **Review logs:** `docker-compose logs`
3. **Run audit:** `nself audit security`
4. **Check GitHub issues:** https://github.com/nself-org/cli/issues
5. **Create new issue** with:
   - Migration step where issue occurred
   - Error messages
   - Output of `nself audit security`
   - Environment (dev/staging/prod)

### Related Documentation

- [Security Hardening Guide](HARDENING-GUIDE.md) - Comprehensive security reference
- [Security Best Practices](SECURITY-BEST-PRACTICES.md) - Ongoing security practices
- [Compliance Guide](COMPLIANCE-GUIDE.md) - SOC 2, GDPR, HIPAA compliance

---

**Migration Guide Version:** 1.0
**Last Updated:** February 11, 2026
**nself Version:** v0.10.0+
