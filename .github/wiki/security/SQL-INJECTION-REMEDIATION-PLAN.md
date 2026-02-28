# SQL Injection Remediation Plan

**Date Created:** 2026-01-31
**Status:** In Progress
**Total Vulnerabilities:** 150+ instances across 40+ files
**Fixed So Far:** 6 command injection vulnerabilities
**Remaining:** 150+ SQL injection vulnerabilities

---

## Executive Summary

This document outlines a phased approach to remediating 150+ SQL injection vulnerabilities identified in the nself codebase. The vulnerabilities stem from direct SQL string interpolation instead of using parameterized queries from `/src/lib/database/safe-query.sh`.

**Critical Risk:** These vulnerabilities allow attackers to:
- Read, modify, or delete sensitive data
- Bypass authentication and authorization
- Access encryption keys and secrets
- Manipulate billing and usage data
- Compromise multi-tenant data isolation

---

## Vulnerability Categories by Severity

### CRITICAL (Immediate Action Required)

| File | Instances | Risk | User Input | Priority |
|------|-----------|------|------------|----------|
| `secrets/vault.sh` | 10+ | Encryption key exposure | ✅ Yes | **P0** |
| `billing/quotas.sh` | 25 | Payment fraud | ✅ Yes | **P0** |
| `billing/usage.sh` | 16 | Billing manipulation | ✅ Yes | **P0** |

**Total CRITICAL:** 51+ instances

### HIGH (Fix Within 1 Week)

| File | Instances | Risk | User Input | Priority |
|------|-----------|------|------------|----------|
| `org/core.sh` | 11 | Org data breach | ✅ Yes | **P1** |
| `tenant/core.sh` | 7+ | Tenant isolation breach | ✅ Yes | **P1** |
| `auth/mfa.sh` | ~10 | Auth bypass | ✅ Yes | **P1** |
| `auth/roles.sh` | ~8 | Privilege escalation | ✅ Yes | **P1** |
| `auth/sessions.sh` | ~7 | Session hijacking | ✅ Yes | **P1** |

**Total HIGH:** 43+ instances

### MEDIUM (Fix Within 2 Weeks)

| File | Instances | Risk | User Input | Priority |
|------|-----------|------|------------|----------|
| `auth/devices.sh` | ~5 | Device tracking manipulation | ⚠️ Partial | **P2** |
| `auth/webhooks.sh` | ~5 | Webhook manipulation | ⚠️ Partial | **P2** |
| `observability/*` | 14 | Metrics/logs manipulation | ⚠️ Partial | **P2** |
| `plugin/core.sh` | 1 | Malicious plugins | ⚠️ Partial | **P2** |

**Total MEDIUM:** 25+ instances

### LOW (Fix Within 1 Month)

| File | Instances | Risk | User Input | Priority |
|------|-----------|------|------------|----------|
| `database/core.sh` | 3 | Migration manipulation | ❌ No (filesystem) | **P3** |
| Other files | ~30 | Various | Mixed | **P3** |

**Total LOW:** 33+ instances

---

## Quick Wins Analysis

Files with many instances that can be batch-fixed with similar patterns:

### Batch Fix Group 1: Billing Files (41 instances)
- `billing/quotas.sh` (25 instances)
- `billing/usage.sh` (16 instances)
- **Pattern:** All use similar SELECT/INSERT/UPDATE patterns
- **Approach:** Create common functions in billing/core.sh
- **Estimated Time:** 4-6 hours

### Batch Fix Group 2: Auth Files (35+ instances)
- `auth/mfa.sh` (~10 instances)
- `auth/roles.sh` (~8 instances)
- `auth/sessions.sh` (~7 instances)
- `auth/devices.sh` (~5 instances)
- `auth/webhooks.sh` (~5 instances)
- **Pattern:** User ID and session-based queries
- **Approach:** Use safe-query.sh helpers, validate UUIDs
- **Estimated Time:** 6-8 hours

### Batch Fix Group 3: Secrets/Vault (10+ instances)
- `secrets/vault.sh` (10+ instances)
- **Pattern:** Key-value storage with encryption metadata
- **Approach:** Parameterize all queries, strict key name validation
- **Estimated Time:** 3-4 hours
- **Risk:** HIGHEST - handles encryption keys

---

## Migration Strategy to safe-query.sh

### Phase 1: Foundation (Complete ✅)
- [x] Create safe-query.sh library
- [x] Fix command injection in safe-query.sh
- [x] Fix command injection in billing/core.sh
- [x] Document safe query patterns

### Phase 2: Critical Files (Week 1) - **CURRENT PHASE**

#### Priority 0: Encryption Keys (Day 1-2)
- [ ] **secrets/vault.sh** (10+ instances)
  - Functions: vault_store_secret, vault_get_secret, vault_delete_secret, vault_rotate_key
  - Risk: Catastrophic if compromised
  - Approach: Parameterize all queries, add key name validation

#### Priority 0: Billing System (Day 3-5)
- [ ] **billing/quotas.sh** (25 instances)
- [ ] **billing/usage.sh** (16 instances)
  - Functions: quota_check, quota_set, usage_record, usage_get
  - Risk: Payment fraud, revenue loss
  - Approach: Create safe helper functions for common patterns

### Phase 3: High Priority Files (Week 2)

#### Organization & Tenant Management
- [ ] **org/core.sh** (11 instances)
- [ ] **tenant/core.sh** (7+ instances)
  - Functions: tenant_create, tenant_delete, org_member_add, etc.
  - Risk: Data breach, tenant isolation failure
  - Approach: UUID validation + parameterized queries

#### Authentication System
- [ ] **auth/mfa.sh** (~10 instances)
- [ ] **auth/roles.sh** (~8 instances)
- [ ] **auth/sessions.sh** (~7 instances)
  - Functions: mfa_enable, role_assign, session_create, etc.
  - Risk: Auth bypass, privilege escalation
  - Approach: Use pg_select_by_id, pg_update_by_id helpers

### Phase 4: Medium Priority Files (Week 3-4)

- [ ] **auth/devices.sh** (~5 instances)
- [ ] **auth/webhooks.sh** (~5 instances)
- [ ] **observability/metrics.sh** (~8 instances)
- [ ] **observability/traces.sh** (~6 instances)
- [ ] **plugin/core.sh** (1 instance - needs review)

### Phase 5: Low Priority & Cleanup (Week 5)

- [ ] Review database/core.sh (migration version handling - low risk)
- [ ] Scan for any missed vulnerabilities
- [ ] Add SQL injection test suite
- [ ] Add pre-commit hooks
- [ ] Update documentation

---

## Implementation Pattern Guide

### Pattern 1: Simple SELECT by ID

**BEFORE (Vulnerable):**
```bash
local result=$(docker exec -i "$container" psql -U "$user" -d "$db" -c \
  "SELECT * FROM users WHERE id = '$user_id'" 2>/dev/null)
```

**AFTER (Safe):**
```bash
source "$SCRIPT_DIR/../database/safe-query.sh"
local result=$(pg_query_json "
  SELECT * FROM users WHERE id = :'param1'
" "$user_id")
```

### Pattern 2: INSERT with RETURNING

**BEFORE (Vulnerable):**
```bash
local id=$(docker exec -i "$container" psql -U "$user" -d "$db" -t -c \
  "INSERT INTO tenants (name, slug) VALUES ('$name', '$slug') RETURNING id" | xargs)
```

**AFTER (Safe):**
```bash
source "$SCRIPT_DIR/../database/safe-query.sh"
local id=$(pg_query_value "
  INSERT INTO tenants (name, slug)
  VALUES (:'param1', :'param2')
  RETURNING id
" "$name" "$slug")
```

### Pattern 3: UPDATE with Multiple Columns

**BEFORE (Vulnerable):**
```bash
docker exec -i "$container" psql -U "$user" -d "$db" -c \
  "UPDATE vault SET encrypted_value = '$value', updated_at = NOW() WHERE id = '$id'"
```

**AFTER (Safe):**
```bash
source "$SCRIPT_DIR/../database/safe-query.sh"
pg_query_safe "
  UPDATE vault
  SET encrypted_value = :'param1',
      updated_at = NOW()
  WHERE id = :'param2'
" "$value" "$id"
```

### Pattern 4: Complex JOIN Query

**BEFORE (Vulnerable):**
```bash
local result=$(docker exec -i "$container" psql -U "$user" -d "$db" -t -A -c \
  "SELECT t.*, u.email FROM tenants t
   JOIN users u ON u.id = t.owner_id
   WHERE t.slug = '$slug'" 2>/dev/null)
```

**AFTER (Safe):**
```bash
source "$SCRIPT_DIR/../database/safe-query.sh"
local result=$(pg_query_json "
  SELECT t.*, u.email
  FROM tenants t
  JOIN users u ON u.id = t.owner_id
  WHERE t.slug = :'param1'
" "$slug")
```

### Pattern 5: Multiple Parameters

**BEFORE (Vulnerable):**
```bash
docker exec -i "$container" psql -U "$user" -d "$db" -c \
  "INSERT INTO tenant_members (tenant_id, user_id, role)
   VALUES ('$tenant_id', '$user_id', '$role')"
```

**AFTER (Safe):**
```bash
source "$SCRIPT_DIR/../database/safe-query.sh"
pg_query_safe "
  INSERT INTO tenant_members (tenant_id, user_id, role)
  VALUES (:'param1', :'param2', :'param3')
" "$tenant_id" "$user_id" "$role"
```

---

## Input Validation Strategy

### Always Validate Before Query

```bash
# UUID validation
user_id=$(validate_uuid "$user_id") || {
  echo "ERROR: Invalid UUID format"
  return 1
}

# Email validation
email=$(validate_email "$email") || {
  echo "ERROR: Invalid email format"
  return 1
}

# Identifier validation (alphanumeric + hyphen/underscore only)
slug=$(validate_identifier "$slug" 100) || {
  echo "ERROR: Invalid slug format"
  return 1
}

# Integer validation with range
limit=$(validate_integer "$limit" 1 1000) || {
  echo "ERROR: Invalid limit (must be 1-1000)"
  return 1
}
```

### Critical Validation Rules

| Input Type | Validation Function | Max Length | Allowed Characters |
|------------|-------------------|------------|-------------------|
| User ID, Tenant ID | `validate_uuid` | 36 | UUID format only |
| Email | `validate_email` | 254 | RFC 5322 compliant |
| Slug | `validate_identifier` | 100 | a-z, 0-9, -, _ |
| Role Name | `validate_identifier` | 50 | a-z, 0-9, -, _ |
| Key Name (vault) | `validate_identifier` | 100 | a-z, 0-9, -, _ |
| Environment | `validate_identifier` | 50 | a-z, 0-9, -, _ |
| JSON | `validate_json` | N/A | Valid JSON only |

---

## Testing Strategy

### Test Suite Structure

```
src/tests/security/
├── sql-injection/
│   ├── test-tenant-injection.sh
│   ├── test-vault-injection.sh
│   ├── test-billing-injection.sh
│   ├── test-auth-injection.sh
│   └── test-common-payloads.sh
└── README.md
```

### Common SQL Injection Payloads to Test

```bash
# Classic termination
payload="'; DROP TABLE users; --"

# Boolean-based blind
payload="' OR 1=1 --"

# Union-based
payload="' UNION SELECT * FROM secrets.vault --"

# Stacked queries
payload="'; DELETE FROM tenants.tenants; SELECT 1 --"

# Comment-based
payload="admin'--"

# Encoded payloads
payload="admin%27%20OR%201%3D1%20--"
```

### Integration Test Example

```bash
test_tenant_create_injection() {
  echo "Testing tenant creation with SQL injection..."

  # Attempt SQL injection in tenant name
  local result
  result=$(nself tenant create "'; DROP TABLE tenants.tenants; --" 2>&1)

  # Should reject or escape properly
  if echo "$result" | grep -q "Invalid"; then
    echo "✓ SQL injection properly rejected"
    return 0
  else
    echo "✗ VULNERABILITY: SQL injection not prevented"
    return 1
  fi
}
```

---

## Pre-Commit Hook

Add to `.git/hooks/pre-commit`:

```bash
#!/usr/bin/env bash

echo "Running SQL injection vulnerability scan..."

# Scan for unsafe SQL patterns
unsafe_patterns=$(grep -rn \
  --include="*.sh" \
  -E '(psql.*-c.*"\$|docker exec.*psql.*"\$)' \
  src/lib/ \
  | grep -v safe-query.sh \
  | grep -v ":'param")

if [[ -n "$unsafe_patterns" ]]; then
  echo "ERROR: Found potential SQL injection vulnerabilities:"
  echo "$unsafe_patterns"
  echo ""
  echo "Use safe-query.sh functions instead:"
  echo "  pg_query_safe, pg_query_value, pg_query_json"
  exit 1
fi

echo "✓ No SQL injection vulnerabilities detected"
```

---

## Progress Tracking

### Week 1 (Current)
- [x] Document vulnerabilities (SECURITY-FIX-REPORT.md)
- [x] Create remediation plan (this document)
- [ ] Fix secrets/vault.sh (10+ instances) - **IN PROGRESS**
- [ ] Fix billing/quotas.sh (25 instances)
- [ ] Fix billing/usage.sh (16 instances)

**Week 1 Target:** 51 vulnerabilities fixed

### Week 2
- [ ] Fix org/core.sh (11 instances)
- [ ] Fix tenant/core.sh (7+ instances)
- [ ] Fix auth/mfa.sh (~10 instances)
- [ ] Fix auth/roles.sh (~8 instances)
- [ ] Fix auth/sessions.sh (~7 instances)

**Week 2 Target:** 43 vulnerabilities fixed (Total: 94)

### Week 3-4
- [ ] Fix remaining auth files (~15 instances)
- [ ] Fix observability files (~14 instances)
- [ ] Fix plugin/core.sh (1 instance)

**Week 3-4 Target:** 30 vulnerabilities fixed (Total: 124)

### Week 5
- [ ] Review and fix remaining files (~30 instances)
- [ ] Create test suite
- [ ] Add pre-commit hooks
- [ ] Final security audit

**Week 5 Target:** All vulnerabilities fixed + prevention measures

---

## Success Metrics

### Code Quality
- [ ] Zero instances of unsafe SQL interpolation
- [ ] All user input validated before queries
- [ ] All database queries use safe-query.sh functions
- [ ] shellcheck passes with zero SQL-related warnings

### Testing
- [ ] SQL injection test suite passes 100%
- [ ] Integration tests verify parameterized queries work
- [ ] Pre-commit hook prevents new vulnerabilities

### Documentation
- [ ] All developers trained on safe-query.sh usage
- [ ] contributing/CONTRIBUTING.md updated with security requirements
- [ ] Security best practices documented
- [ ] Code review checklist includes SQL injection checks

---

## Resources

### Documentation
- `/Users/admin/Sites/nself/SECURITY-FIX-REPORT.md` - Detailed vulnerability report
- `/Users/admin/Sites/nself/src/lib/database/safe-query.sh` - Safe query library
- `/Users/admin/Sites/nself/docs/security/` - Security documentation

### Tools
- `src/scripts/security-audit.sh` - Automated vulnerability scanner
- `grep -rn 'psql.*-c.*"\$' src/lib/` - Manual scan command

### Reference
- [OWASP SQL Injection](https://owasp.org/www-community/attacks/SQL_Injection)
- [PostgreSQL Parameter Binding](https://www.postgresql.org/docs/current/libpq-exec.html)
- [Bash Security Best Practices](https://mywiki.wooledge.org/BashGuide/Practices)

---

## Next Steps (Immediate)

1. ✅ Create this remediation plan
2. 🔄 **Fix secrets/vault.sh (10+ instances)** - IN PROGRESS
3. ⏳ Fix billing/quotas.sh (25 instances)
4. ⏳ Fix billing/usage.sh (16 instances)
5. ⏳ Review and commit changes
6. ⏳ Continue with Week 2 priorities

---

**Document Version:** 1.0
**Last Updated:** 2026-01-31
**Next Review:** After each phase completion
**Owner:** Security Team
