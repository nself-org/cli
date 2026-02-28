# SQL Injection Vulnerability Remediation Report
## nself Project - Security Audit Results

**Date**: 2026-01-31
**Auditor**: Security Review
**Severity**: CRITICAL
**Status**: IN PROGRESS

---

## Executive Summary

A comprehensive security audit has identified **approximately 150 SQL injection vulnerabilities** across the nself codebase, primarily in billing and multi-tenancy modules. These vulnerabilities pose a **CRITICAL security risk** as they:

1. Allow attackers to access sensitive financial data
2. Enable unauthorized modification of billing records
3. Permit tenant data access across isolation boundaries
4. Could lead to complete database compromise

**Immediate action required** to remediate these vulnerabilities before production deployment.

---

## Vulnerability Distribution

### By Module

| Module | File | Vulnerabilities | Risk Level |
|--------|------|----------------|------------|
| Billing Usage | `src/lib/billing/usage.sh` | ~60 | CRITICAL |
| Billing Reports | `src/lib/billing/reports.sh` | ~30 | CRITICAL |
| Billing Core | `src/lib/billing/core.sh` | ~10 | HIGH (partially fixed) |
| Tenant Lifecycle | `src/lib/tenant/lifecycle.sh` | ~25 | CRITICAL |
| Tenant Core | `src/lib/tenant/core.sh` | ~20 | HIGH (partially fixed) |
| Tenant Routing | `src/lib/tenant/routing.sh` | ~15 | HIGH |
| **TOTAL** | | **~150** | **CRITICAL** |

### By Vulnerability Pattern

1. **Direct String Interpolation** (~100 instances)
   ```bash
   # VULNERABLE
   WHERE customer_id = '${customer_id}'

   # SAFE
   WHERE customer_id = :'customer_id'
   ```

2. **Missing Input Validation** (~50 instances)
   ```bash
   # VULNERABLE
   local service="$1"
   # ... used directly in SQL

   # SAFE
   validate_service_name "$service" || return 1
   ```

3. **Dynamic SQL Construction** (~20 instances)
   ```bash
   # VULNERABLE
   where_clause="customer_id = '${customer_id}'"

   # SAFE - Use parameterized queries only
   ```

---

## Critical Vulnerabilities (Top 10)

### 1. Usage Aggregate Functions (60 instances)
**File**: `src/lib/billing/usage.sh`
**Lines**: 305-312, 350-357, 389-424, 448-481, 492-514, 525-536, 548-559, 581-593

**Vulnerability**:
```bash
billing_db_query "
    SELECT COALESCE(SUM(quantity), 0)
    FROM billing_usage_records
    WHERE customer_id = '${customer_id}'  # INJECTABLE
    AND service_name = '${service}'        # INJECTABLE
    AND recorded_at >= '${start_date}'     # INJECTABLE
    AND recorded_at <= '${end_date}';      # INJECTABLE
" | tr -d ' '
```

**Impact**: Attacker can extract all usage data, billing information, or modify query logic.

**Fix**:
```bash
# Validate all inputs FIRST
validate_customer_id "$customer_id" || return 1
validate_service_name "$service" || return 1
validate_date_format "$start_date" || return 1
validate_date_format "$end_date" || return 1

# Use parameterized query
billing_db_query "
    SELECT COALESCE(SUM(quantity), 0)
    FROM billing_usage_records
    WHERE customer_id = :'customer_id'
    AND service_name = :'service_name'
    AND recorded_at >= :'start_date'
    AND recorded_at <= :'end_date';
" "tuples" "customer_id" "$customer_id" "service_name" "$service" "start_date" "$start_date" "end_date" "$end_date" | tr -d ' '
```

---

### 2. Usage Export Functions (Multiple Formats)
**File**: `src/lib/billing/usage.sh`
**Lines**: 1202-1228, 1239-1283

**Vulnerability**:
```bash
where_clause="customer_id = '${customer_id}'"  # INJECTABLE
if [[ -n "$start_date" ]]; then
  where_clause+=" AND recorded_at >= '${start_date}'"  # INJECTABLE
fi
```

**Impact**: Complete data exfiltration via export functionality.

**Fix**: NEVER build WHERE clauses with string concatenation. Use parameterized queries exclusively.

---

### 3. Reports - Monthly Recurring Revenue
**File**: `src/lib/billing/reports.sh`
**Lines**: 79-103, 123-135, 143-175

**Vulnerability**:
```bash
billing_db_query "
    SELECT month, active_subs, new_subs
    FROM monthly_stats
    WHERE DATE_TRUNC('month', created_at) >= :'start_month'::date  # SAFE
    AND DATE_TRUNC('month', created_at) <= :'end_month'::date      # SAFE
" "tuples" "start_month" "$start_month" "end_month" "$end_month"
```

**Status**: ✅ ALREADY FIXED (Good example of proper parameterization)

---

### 4. Tenant Provisioning
**File**: `src/lib/tenant/lifecycle.sh`
**Lines**: 34-35, 44-46, 57-61, 70-72, 76-79

**Vulnerability**:
```bash
slug_exists=$(docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "SELECT COUNT(*) FROM tenants.tenants WHERE slug = '$tenant_slug'" | tr -d ' \n')
  # INJECTABLE ^^^^^^^^^^^^^^^

owner_id=$(docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "SELECT id FROM auth.users WHERE email = '$owner_email' LIMIT 1" | tr -d ' \n')
  # INJECTABLE ^^^^^^^^^^^

tenant_id=$(docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "INSERT INTO tenants.tenants (name, slug, plan_id, owner_user_id)
   VALUES ('$tenant_name', '$tenant_slug', '$plan', '$owner_id')
   RETURNING id" | tr -d ' \n')
  # ALL INJECTABLE ^^^^^^^^^^^^^ ^^^^^^^^^^^^^ ^^^^^^ ^^^^^^^^^^
```

**Impact**: CRITICAL - Allows account takeover, privilege escalation, and tenant isolation bypass.

**Fix**: Must use safe query wrapper with validation:
```bash
# Validate ALL inputs
validate_identifier "$tenant_slug" 100 || return 1
validate_email "$owner_email" || return 1
validate_identifier "$tenant_name" 255 || return 1
validate_identifier "$plan" 50 || return 1

# Use safe query function (defined in src/lib/database/safe-query.sh)
slug_exists=$(safe_query "
  SELECT COUNT(*) FROM tenants.tenants WHERE slug = :'param1'
" "$tenant_slug" | tr -d ' \n')

owner_id=$(safe_query "
  SELECT id FROM auth.users WHERE email = :'param1' LIMIT 1
" "$owner_email" | tr -d ' \n')

tenant_id=$(safe_query "
  INSERT INTO tenants.tenants (name, slug, plan_id, owner_user_id)
  VALUES (:'param1', :'param2', :'param3', :'param4')
  RETURNING id
" "$tenant_name" "$tenant_slug" "$plan" "$owner_id" | tr -d ' \n')
```

---

### 5. Tenant Lifecycle Operations (Suspend/Delete)
**File**: `src/lib/tenant/lifecycle.sh`
**Lines**: 184-196, 210-214, 271-282, 307-315

**Vulnerability**:
```bash
docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
  "UPDATE tenants.tenants
   SET status = 'suspended', suspended_at = NOW(),
       metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{suspension_reason}', '\"$reason\"'::jsonb)
   WHERE id = '$tenant_id' OR slug = '$tenant_id'" >/dev/null 2>&1
   # INJECTABLE ^^^^^^^^^^^ ^^^^^^^^^^^^^^^^^^^^^^^ ^^^^^^^^^^^^^^
```

**Impact**: Arbitrary tenant manipulation, data corruption.

---

### 6. Tenant Member Management
**File**: `src/lib/tenant/core.sh`
**Lines**: 434-444, 457-469

**Vulnerability**:
```bash
DELETE FROM tenants.tenant_members tm
USING tenants.tenants t
WHERE tm.tenant_id = t.id
AND (t.id = '$tenant_id' OR t.slug = '$tenant_id')  # INJECTABLE
AND tm.user_id = '$user_id';                         # INJECTABLE
```

**Impact**: Unauthorized user removal, privilege manipulation.

**Status**: ⚠️ PARTIALLY FIXED - Some functions use `pg_query_safe()`, others don't.

---

### 7. Usage Trends Reporting
**File**: `src/lib/billing/reports.sh`
**Lines**: 482-534, 538-568, 572-609

**Vulnerability**:
```bash
service_filter=""
if [[ "$service" != "all" ]]; then
  service_filter="AND service_name = '${service}'"  # INJECTABLE
fi

billing_db_query "
  SELECT date, service_name, total_usage
  WHERE recorded_at >= NOW() - INTERVAL '${days} days'  # INJECTABLE
  ${service_filter}  # INJECTABLE CLAUSE
" "tuples"
```

**Impact**: Data exfiltration, query manipulation.

---

### 8. Tenant Domain Verification
**File**: `src/lib/tenant/routing.sh`
**Lines**: 220-222, 284-289

**Vulnerability**:
```bash
verified=$(docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "SELECT is_verified FROM tenants.tenant_domains
   WHERE domain = '$domain' AND tenant_id = '$tenant_id'" | tr -d ' \n')
   # INJECTABLE ^^^^^^^^ INJECTABLE ^^^^^^^^^^^^^^^
```

**Impact**: Domain hijacking, certificate fraud.

---

### 9. Usage Aggregation by Period
**File**: `src/lib/billing/usage.sh`
**Lines**: 839-861, 870-892, 901-923

**Vulnerability**:
```bash
where_clause="customer_id = '${customer_id}'"  # INJECTABLE
if [[ -n "$start_date" ]]; then
  where_clause+=" AND recorded_at >= '${start_date}'"  # INJECTABLE
fi

billing_db_query "
  SELECT service_name, DATE_TRUNC('hour', recorded_at) as hour
  FROM billing_usage_records
  WHERE ${where_clause}  # ENTIRE CLAUSE INJECTABLE
"
```

**Impact**: Complete query control, data manipulation.

---

### 10. Dashboard Metrics
**File**: `src/lib/billing/reports.sh`
**Lines**: 769-773

**Vulnerability**:
```bash
total_customers=$(billing_db_query "SELECT COUNT(*) FROM billing_customers WHERE deleted_at IS NULL;" "tuples" 2>/dev/null | tr -d ' ')
total_mrr=$(billing_db_query "SELECT COALESCE(SUM(99), 0) FROM billing_subscriptions WHERE status = 'active';" "tuples" 2>/dev/null | tr -d ' ')
```

**Status**: ✅ SAFE (No user input in these queries)

---

## Remediation Plan

### Phase 1: Immediate Fixes (Priority: CRITICAL)
**Target**: Week 1
**Scope**: Fix top 10 critical vulnerabilities

**Files to Fix**:
1. `src/lib/billing/usage.sh` - All aggregate functions
2. `src/lib/billing/usage.sh` - All export functions
3. `src/lib/tenant/lifecycle.sh` - Provisioning workflow
4. `src/lib/tenant/lifecycle.sh` - Suspend/delete operations

**Approach**:
- Add input validation for ALL user inputs
- Convert ALL string interpolation to parameterized queries
- Use `billing_db_query()` with proper parameter binding
- Use validation functions from `src/lib/utils/validation.sh`

---

### Phase 2: Comprehensive Remediation (Priority: HIGH)
**Target**: Week 2
**Scope**: Fix remaining ~140 vulnerabilities

**Files to Fix**:
1. `src/lib/billing/reports.sh` - All reporting functions
2. `src/lib/tenant/core.sh` - Remaining unsafe queries
3. `src/lib/tenant/routing.sh` - Domain verification queries

---

### Phase 3: Security Hardening (Priority: MEDIUM)
**Target**: Week 3
**Scope**: Add additional security layers

**Tasks**:
1. Create SQL injection detection tests
2. Add rate limiting for query-heavy endpoints
3. Implement query auditing/logging
4. Add prepared statement enforcement
5. Create security regression tests

---

## Validation Functions Available

The codebase already has validation functions in `src/lib/utils/validation.sh`:

```bash
validate_uuid()           # Validates UUIDs
validate_email()          # Validates email addresses
validate_identifier()     # Validates alphanumeric identifiers (table names, slugs)
validate_integer()        # Validates integers
validate_boolean()        # Validates boolean values
```

**Usage**:
```bash
# Validate before use
if ! customer_id=$(validate_uuid "$customer_id" 2>/dev/null); then
  error "Invalid customer ID format"
  return 1
fi

# Now safe to use in parameterized query
billing_db_query "SELECT * FROM billing_customers WHERE customer_id = :'param1'" "tuples" "param1" "$customer_id"
```

---

## Safe Query Patterns

### Pattern 1: Simple SELECT with Parameters
```bash
# BEFORE (VULNERABLE)
result=$(billing_db_query "SELECT * FROM table WHERE id = '${id}'" | tr -d ' ')

# AFTER (SAFE)
validate_uuid "$id" || return 1
result=$(billing_db_query "
  SELECT * FROM table WHERE id = :'id'
" "tuples" "id" "$id" | tr -d ' ')
```

### Pattern 2: Multiple Parameters
```bash
# BEFORE (VULNERABLE)
billing_db_query "
  SELECT * FROM table
  WHERE customer_id = '${customer_id}'
  AND service = '${service}'
  AND date >= '${start_date}'
"

# AFTER (SAFE)
validate_customer_id "$customer_id" || return 1
validate_service_name "$service" || return 1
validate_date_format "$start_date" || return 1

billing_db_query "
  SELECT * FROM table
  WHERE customer_id = :'customer_id'
  AND service = :'service'
  AND date >= :'start_date'
" "tuples" \
  "customer_id" "$customer_id" \
  "service" "$service" \
  "start_date" "$start_date"
```

### Pattern 3: Dynamic WHERE Clauses (AVOID!)
```bash
# BEFORE (VULNERABLE)
where_clause="customer_id = '${customer_id}'"
if [[ -n "$service" ]]; then
  where_clause+=" AND service = '${service}'"
fi
billing_db_query "SELECT * FROM table WHERE ${where_clause}"

# AFTER (SAFE) - Use COALESCE or conditional logic in SQL
billing_db_query "
  SELECT * FROM table
  WHERE customer_id = :'customer_id'
  AND (:'service' = '' OR service = :'service')
" "tuples" \
  "customer_id" "$customer_id" \
  "service" "${service:-}"
```

---

## Testing Requirements

### Unit Tests Needed
```bash
# Create: src/tests/security/test-sql-injection.sh

test_customer_id_injection() {
  # Attempt injection
  local malicious_id="1' OR '1'='1"

  # Should fail validation
  if validate_customer_id "$malicious_id" 2>/dev/null; then
    fail "Validation should reject SQL injection attempt"
  fi

  pass "SQL injection blocked by validation"
}

test_parameterized_query_safety() {
  # Even with malicious input, parameterized query should treat as literal
  local malicious_service="api'; DELETE FROM billing_usage_records; --"

  # Should return no results (or error), NOT execute DELETE
  local result
  result=$(usage_get_service "$malicious_service" "2026-01-01" "2026-01-31" 2>/dev/null)

  # Verify no data was deleted
  local count
  count=$(billing_db_query "SELECT COUNT(*) FROM billing_usage_records" "tuples")

  if [[ $count -eq 0 ]]; then
    fail "Data was deleted - SQL injection successful!"
  fi

  pass "Parameterized query prevented SQL injection"
}
```

---

## Timeline Summary

| Phase | Duration | Vulnerabilities Fixed | Status |
|-------|----------|----------------------|--------|
| Phase 1 | Week 1 | 60 (Critical billing) | 🔴 NOT STARTED |
| Phase 2 | Week 2 | 90 (All remaining) | 🔴 NOT STARTED |
| Phase 3 | Week 3 | N/A (Hardening) | 🔴 NOT STARTED |
| **Total** | **3 weeks** | **~150** | **0% Complete** |

---

## Recommendations

### Immediate Actions (DO NOW)
1. ❌ **DO NOT deploy to production** until Phase 1 is complete
2. ✅ **Start Phase 1 remediation** immediately
3. ✅ **Add security tests** to CI/CD pipeline
4. ✅ **Enable SQL query logging** for audit trail

### Long-term Improvements
1. Enforce parameterized queries via linting rules
2. Add SQL injection detection to pre-commit hooks
3. Implement query whitelisting for high-risk operations
4. Add honeypot fields to detect automated attacks
5. Enable PostgreSQL audit logging

### Code Review Checklist
Before merging any SQL-related changes, verify:
- [ ] All user inputs are validated with appropriate validation functions
- [ ] All SQL queries use parameterized binding (`:param` syntax)
- [ ] No string interpolation (`${var}`) in SQL queries
- [ ] No dynamic WHERE clause construction via string concatenation
- [ ] Input validation tests exist for all new functions
- [ ] SQL injection tests exist for all query functions

---

## Conclusion

The nself project contains approximately **150 SQL injection vulnerabilities** across billing and multi-tenancy modules. These represent a **CRITICAL security risk** that must be addressed before any production deployment.

The remediation plan outlined above provides a systematic approach to fixing all vulnerabilities within 3 weeks, with the most critical issues addressed in Week 1.

**Estimated Effort**: 60-80 hours of developer time
**Risk if not fixed**: Complete database compromise, financial data breach, tenant isolation bypass
**Priority**: CRITICAL - Block production deployment until complete

---

## References

- OWASP SQL Injection: https://owasp.org/www-community/attacks/SQL_Injection
- PostgreSQL Parameterized Queries: https://www.postgresql.org/docs/current/sql-prepare.html
- Input Validation Best Practices: https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html

---

**Report Generated**: 2026-01-31
**Last Updated**: 2026-01-31
**Next Review**: After Phase 1 completion
