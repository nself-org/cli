# SQL Injection Review - Remaining Files

**Status:** TODO
**Priority:** HIGH
**Date:** 2026-01-30

## Overview

This document tracks files that still need to be reviewed and updated for SQL injection protection.

## Files Already Fixed ✅

- ✅ `/src/lib/admin/api.sh`
- ✅ `/src/lib/auth/user-manager.sh`
- ✅ `/src/lib/auth/role-manager.sh`

## Files That Need Review

### High Priority (Known SQL Usage)

#### Auth Module
- [ ] `/src/lib/auth/permission-manager.sh`
- [ ] `/src/lib/auth/audit-log.sh`
- [ ] `/src/lib/auth/apikey-manager.sh`
- [ ] `/src/lib/auth/session-manager.sh`
- [ ] `/src/lib/auth/user-metadata.sh`
- [ ] `/src/lib/auth/user-profile.sh`
- [ ] `/src/lib/auth/device-manager.sh`
- [ ] `/src/lib/auth/jwt-manager.sh`
- [ ] `/src/lib/auth/custom-claims.sh`
- [ ] `/src/lib/auth/user-import-export.sh`
- [ ] `/src/lib/auth/magic-link.sh`
- [ ] `/src/lib/auth/hooks.sh`
- [ ] `/src/lib/auth/auth-manager.sh`

#### Auth MFA Module
- [ ] `/src/lib/auth/mfa/backup-codes.sh`
- [ ] `/src/lib/auth/mfa/totp.sh`
- [ ] `/src/lib/auth/mfa/policies.sh`
- [ ] `/src/lib/auth/mfa/sms.sh`
- [ ] `/src/lib/auth/mfa/email.sh`
- [ ] `/src/lib/auth/mfa/webauthn.sh`

#### Billing Module
- [ ] `/src/lib/billing/core.sh`
- [ ] `/src/lib/billing/stripe.sh`
- [ ] `/src/lib/billing/stripe_new.sh`
- [ ] `/src/lib/billing/usage.sh`
- [ ] `/src/lib/billing/quotas.sh`

#### Tenant Module
- [ ] `/src/lib/tenant/core.sh`
- [ ] `/src/lib/tenant/lifecycle.sh`
- [ ] `/src/lib/tenant/routing.sh`

#### Organization Module
- [ ] `/src/lib/org/core.sh`

#### White Label Module
- [ ] `/src/lib/whitelabel/domains.sh`
- [ ] `/src/lib/whitelabel/branding.sh`
- [ ] `/src/lib/whitelabel/themes.sh`
- [ ] `/src/lib/whitelabel/email-templates.sh`

#### Webhooks Module
- [ ] `/src/lib/webhooks/core.sh`

#### Rate Limit Module
- [ ] `/src/lib/rate-limit/core.sh`
- [ ] `/src/lib/rate-limit/endpoint-limiter.sh`
- [ ] `/src/lib/rate-limit/user-limiter.sh`
- [ ] `/src/lib/rate-limit/ip-limiter.sh`
- [ ] `/src/lib/rate-limit/strategies.sh`

#### Redis Module
- [ ] `/src/lib/redis/core.sh`
- [ ] `/src/lib/redis/cache.sh`
- [ ] `/src/lib/redis/sessions.sh`
- [ ] `/src/lib/redis/rate-limit-distributed.sh`

#### Database Module
- [ ] `/src/lib/database/core.sh`

#### Storage Module
- [ ] `/src/lib/storage/graphql-integration.sh`
- [ ] `/src/lib/storage/upload-pipeline.sh`

#### Build Module
- [ ] `/src/lib/build/database.sh`
- [ ] `/src/lib/build/core-modules/database-init.sh`

#### Backup Module
- [ ] `/src/lib/backup/automated.sh`
- [ ] `/src/lib/backup/recovery.sh`

#### Security Module
- [ ] `/src/lib/security/scanner.sh`
- [ ] `/src/lib/security/incident-response.sh`
- [ ] `/src/lib/security/webauthn.sh`

#### Monitoring Module
- [ ] `/src/lib/monitoring/alerting.sh`
- [ ] `/src/lib/monitoring/lb-health.sh`
- [ ] `/src/lib/monitoring/metrics-dashboard.sh`

#### Observability Module
- [ ] `/src/lib/observability/health.sh`
- [ ] `/src/lib/observability/logging.sh`
- [ ] `/src/lib/observability/metrics.sh`
- [ ] `/src/lib/observability/tracing.sh`

#### Autofix Module
- [ ] `/src/lib/autofix/postgres-connection.sh`
- [ ] `/src/lib/autofix/comprehensive.sh`
- [ ] `/src/lib/autofix/fixes/healthcheck-complete.sh`
- [ ] `/src/lib/autofix/fixes/healthcheck.sh`
- [ ] `/src/lib/autofix/fixes/database.sh`
- [ ] `/src/lib/autofix/fixes/schema.sh`

### Medium Priority (May Use SQL)

#### Services Module
- [ ] `/src/lib/services/hasura-metadata.sh`
- [ ] `/src/lib/services/service-builder.sh`

#### Migration Module
- [ ] `/src/lib/migrate/supabase.sh`
- [ ] `/src/lib/migrate/firebase.sh`

#### Environment Module
- [ ] `/src/lib/env/switch.sh`
- [ ] `/src/lib/env/create.sh`

#### Compliance Module
- [ ] `/src/lib/compliance/framework.sh`
- [ ] `/src/lib/compliance/reports.sh`

#### Secrets Module
- [ ] `/src/lib/secrets/audit.sh`
- [ ] `/src/lib/secrets/encryption.sh`
- [ ] `/src/lib/secrets/environment.sh`
- [ ] `/src/lib/secrets/vault.sh`

#### Deploy Module
- [ ] `/src/lib/deploy/credentials.sh`
- [ ] `/src/lib/deploy/security-preflight.sh`

#### Recovery Module
- [ ] `/src/lib/recovery/disaster-recovery.sh`

#### Dev Module
- [ ] `/src/lib/dev/sdk-generator.sh`
- [ ] `/src/lib/dev/test-helpers.sh`
- [ ] `/src/lib/dev/docs-generator.sh`

#### Plugin Module
- [ ] `/src/lib/plugin/registry.sh`

### Low Priority (Unlikely SQL)

These files were flagged by grep but are unlikely to have SQL:
- Help files (`.help.txt`)
- README files
- Provider files (cloud deployment)
- SSL/certificate files
- Utility files
- Build orchestration files

---

## Review Process

For each file:

1. **Search for SQL queries:**
   ```bash
   grep -n "psql.*-c" filename.sh
   grep -n "SELECT\|INSERT\|UPDATE\|DELETE" filename.sh
   ```

2. **Check for string interpolation:**
   ```bash
   grep -n "WHERE.*=.*'\$" filename.sh
   grep -n "VALUES.*'\$" filename.sh
   ```

3. **If SQL found:**
   - Source `/src/lib/database/safe-query.sh`
   - Replace with parameterized queries
   - Add input validation
   - Update function to use `pg_query_*` functions

4. **Test:**
   - Add to `/src/tests/security/test-sql-injection.sh`
   - Run tests
   - Verify with malicious input

5. **Mark as complete:**
   - Check the box in this document
   - Commit with message: "security: fix SQL injection in [filename]"

---

## Quick Search Commands

### Find all files with SQL
```bash
# Find potential SQL queries
grep -r "SELECT\|INSERT\|UPDATE\|DELETE" src/lib/ \
  --include="*.sh" \
  | grep -v ".vulnerable" \
  | grep -v "safe-query.sh"
```

### Find string interpolation in SQL
```bash
# Find potential SQL injection points
grep -r "psql.*-c.*\\\$" src/lib/ \
  --include="*.sh" \
  | grep -v ".vulnerable" \
  | grep -v "safe-query.sh"
```

### Count remaining files
```bash
# Count files that need review
grep -c "\[ \]" docs/security/SQL-REVIEW-REMAINING.md
```

---

## Progress Tracking

**Total Files:** ~90
**Reviewed:** 3 ✅
**Remaining:** ~87 📋

**High Priority:** ~65 files
**Medium Priority:** ~20 files
**Low Priority:** ~5 files

---

## Notes

- Backup original files to `.vulnerable` before modifying
- Always add tests for new secure implementations
- Document any special cases or complex queries
- Run full test suite before committing

---

**Last Updated:** 2026-01-30
