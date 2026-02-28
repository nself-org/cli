# SQL Injection Fix Guide
## Step-by-Step Remediation Instructions for nself

**Target Audience**: Developers fixing SQL injection vulnerabilities
**Prerequisite Reading**: SQL_INJECTION_REMEDIATION_REPORT.md

---

## Table of Contents

1. [Understanding the Problem](#understanding-the-problem)
2. [Required Tools](#required-tools)
3. [Fix Pattern Templates](#fix-pattern-templates)
4. [File-by-File Instructions](#file-by-file-instructions)
5. [Testing Your Fixes](#testing-your-fixes)
6. [Common Pitfalls](#common-pitfalls)

---

## Understanding the Problem

### Why String Interpolation is Dangerous

```bash
# VULNERABLE CODE
local customer_id="$1"
billing_db_query "SELECT * FROM billing_customers WHERE customer_id = '${customer_id}'"

# If attacker provides: customer_id="1' OR '1'='1"
# Query becomes: SELECT * FROM billing_customers WHERE customer_id = '1' OR '1'='1'
# Result: Returns ALL customers (not just customer 1)
```

### How Parameterized Queries Prevent Injection

```bash
# SAFE CODE
local customer_id="$1"
validate_uuid "$customer_id" || return 1  # Validation layer 1

billing_db_query "
  SELECT * FROM billing_customers WHERE customer_id = :'customer_id'
" "tuples" "customer_id" "$customer_id"  # Parameterization layer 2

# If attacker provides: customer_id="1' OR '1'='1"
# Validation rejects it (not a valid UUID)
# Even if validation bypassed, PostgreSQL treats entire string as literal value
# Query searches for customer with ID literally = "1' OR '1'='1" (not found, safe)
```

---

## Required Tools

### 1. Validation Functions
**Location**: `src/lib/utils/validation.sh`

Already available:
- `validate_uuid()` - Validates UUIDs (customer IDs, tenant IDs, user IDs)
- `validate_email()` - Validates email addresses
- `validate_identifier()` - Validates alphanumeric identifiers (slugs, service names)
- `validate_integer()` - Validates integers (limits, counts)

### 2. Safe Query Functions
**Location**: `src/lib/database/safe-query.sh`

Available functions:
- `safe_query()` - Generic parameterized query
- `pg_query_safe()` - PostgreSQL-specific safe query
- `pg_query_value()` - Returns single value safely

### 3. Billing Query Function
**Location**: `src/lib/billing/core.sh`

Already supports parameterized queries:
- `billing_db_query()` - Supports `:param` syntax

**Usage**:
```bash
billing_db_query "SELECT * FROM table WHERE id = :'id'" "tuples" "id" "$value"
```

---

## Fix Pattern Templates

### Template 1: Simple SELECT with One Parameter

```bash
# BEFORE (VULNERABLE)
function get_customer() {
  local customer_id="$1"

  billing_db_query "
    SELECT * FROM billing_customers
    WHERE customer_id = '${customer_id}'
  " "tuples"
}

# AFTER (SAFE)
function get_customer() {
  local customer_id="$1"

  # Step 1: Validate input
  if ! customer_id=$(validate_uuid "$customer_id" 2>/dev/null); then
    error "Invalid customer ID format"
    return 1
  fi

  # Step 2: Use parameterized query
  billing_db_query "
    SELECT * FROM billing_customers
    WHERE customer_id = :'customer_id'
  " "tuples" "customer_id" "$customer_id"
}
```

---

### Template 2: SELECT with Multiple Parameters

```bash
# BEFORE (VULNERABLE)
function get_usage() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"

  billing_db_query "
    SELECT SUM(quantity) FROM billing_usage_records
    WHERE customer_id = '${customer_id}'
    AND service_name = '${service}'
    AND recorded_at >= '${start_date}'
    AND recorded_at <= '${end_date}'
  " "tuples"
}

# AFTER (SAFE)
function get_usage() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"

  # Step 1: Validate ALL inputs
  if ! customer_id=$(validate_uuid "$customer_id" 2>/dev/null); then
    error "Invalid customer ID format"
    return 1
  fi

  validate_service_name "$service" || return 1
  validate_date_format "$start_date" || return 1
  validate_date_format "$end_date" || return 1

  # Step 2: Use parameterized query with ALL parameters
  billing_db_query "
    SELECT SUM(quantity) FROM billing_usage_records
    WHERE customer_id = :'customer_id'
    AND service_name = :'service_name'
    AND recorded_at >= :'start_date'
    AND recorded_at <= :'end_date'
  " "tuples" \
    "customer_id" "$customer_id" \
    "service_name" "$service" \
    "start_date" "$start_date" \
    "end_date" "$end_date"
}
```

---

### Template 3: INSERT Statement

```bash
# BEFORE (VULNERABLE)
function create_tenant() {
  local name="$1"
  local slug="$2"
  local plan="$3"
  local owner_id="$4"

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "INSERT INTO tenants.tenants (name, slug, plan_id, owner_user_id)
     VALUES ('$name', '$slug', '$plan', '$owner_id')
     RETURNING id"
}

# AFTER (SAFE)
function create_tenant() {
  local name="$1"
  local slug="$2"
  local plan="$3"
  local owner_id="$4"

  # Step 1: Validate ALL inputs
  validate_identifier "$slug" 100 || return 1
  validate_identifier "$plan" 50 || return 1

  if ! owner_id=$(validate_uuid "$owner_id" 2>/dev/null); then
    error "Invalid owner user ID format"
    return 1
  fi

  # Step 2: Use safe query function with parameters
  pg_query_value "
    INSERT INTO tenants.tenants (name, slug, plan_id, owner_user_id)
    VALUES (:'param1', :'param2', :'param3', :'param4')
    RETURNING id
  " "$name" "$slug" "$plan" "$owner_id"
}
```

---

### Template 4: UPDATE Statement

```bash
# BEFORE (VULNERABLE)
function suspend_tenant() {
  local tenant_id="$1"
  local reason="$2"

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE tenants.tenants
     SET status = 'suspended', suspended_at = NOW(),
         metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{suspension_reason}', '\"$reason\"'::jsonb)
     WHERE id = '$tenant_id' OR slug = '$tenant_id'"
}

# AFTER (SAFE)
function suspend_tenant() {
  local tenant_id="$1"
  local reason="$2"

  # Step 1: Validate inputs
  # tenant_id could be UUID or slug
  local validated_id="$tenant_id"
  if ! validated_id=$(validate_uuid "$tenant_id" 2>/dev/null); then
    if ! validated_id=$(validate_identifier "$tenant_id" 100 2>/dev/null); then
      error "Invalid tenant ID or slug format"
      return 1
    fi
  fi

  # Step 2: Use safe query with parameters
  pg_query_safe "
    UPDATE tenants.tenants
    SET status = 'suspended',
        suspended_at = NOW(),
        metadata = jsonb_set(
          COALESCE(metadata, '{}'::jsonb),
          '{suspension_reason}',
          to_jsonb(:'reason')
        )
    WHERE id = :'tenant_id' OR slug = :'tenant_id'
  " "$validated_id" "$reason"
}
```

---

### Template 5: DELETE Statement

```bash
# BEFORE (VULNERABLE)
function remove_member() {
  local tenant_id="$1"
  local user_id="$2"

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "DELETE FROM tenants.tenant_members tm
     USING tenants.tenants t
     WHERE tm.tenant_id = t.id
     AND (t.id = '$tenant_id' OR t.slug = '$tenant_id')
     AND tm.user_id = '$user_id'"
}

# AFTER (SAFE)
function remove_member() {
  local tenant_id="$1"
  local user_id="$2"

  # Step 1: Validate inputs
  local validated_tenant_id="$tenant_id"
  if ! validated_tenant_id=$(validate_uuid "$tenant_id" 2>/dev/null); then
    if ! validated_tenant_id=$(validate_identifier "$tenant_id" 100 2>/dev/null); then
      error "Invalid tenant ID or slug format"
      return 1
    fi
  fi

  if ! user_id=$(validate_uuid "$user_id" 2>/dev/null); then
    error "Invalid user ID format"
    return 1
  fi

  # Step 2: Use safe query
  pg_query_safe "
    DELETE FROM tenants.tenant_members tm
    USING tenants.tenants t
    WHERE tm.tenant_id = t.id
    AND (t.id = :'tenant_id' OR t.slug = :'tenant_id')
    AND tm.user_id = :'user_id'
  " "$validated_tenant_id" "$user_id"
}
```

---

### Template 6: Dynamic WHERE Clauses (Special Case)

```bash
# BEFORE (VULNERABLE) - Building WHERE clause with string concatenation
function export_usage() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"

  local where_clause="customer_id = '${customer_id}'"

  if [[ -n "$start_date" ]]; then
    where_clause+=" AND recorded_at >= '${start_date}'"
  fi

  if [[ -n "$end_date" ]]; then
    where_clause+=" AND recorded_at <= '${end_date}'"
  fi

  if [[ "$service" != "all" ]] && [[ -n "$service" ]]; then
    where_clause+=" AND service_name = '${service}'"
  fi

  billing_db_query "
    SELECT * FROM billing_usage_records
    WHERE ${where_clause}
  " "csv"
}

# AFTER (SAFE) - Use SQL conditional logic instead
function export_usage() {
  local customer_id="$1"
  local service="$2"
  local start_date="$3"
  local end_date="$4"

  # Step 1: Validate inputs
  if ! customer_id=$(validate_uuid "$customer_id" 2>/dev/null); then
    error "Invalid customer ID format"
    return 1
  fi

  if [[ "$service" != "all" ]] && [[ -n "$service" ]]; then
    validate_service_name "$service" || return 1
  fi

  if [[ -n "$start_date" ]]; then
    validate_date_format "$start_date" || return 1
  fi

  if [[ -n "$end_date" ]]; then
    validate_date_format "$end_date" || return 1
  fi

  # Step 2: Use COALESCE/conditional SQL logic
  billing_db_query "
    SELECT * FROM billing_usage_records
    WHERE customer_id = :'customer_id'
    AND (:'service' = 'all' OR :'service' = '' OR service_name = :'service')
    AND (:'start_date' = '' OR recorded_at >= :'start_date'::timestamp)
    AND (:'end_date' = '' OR recorded_at <= :'end_date'::timestamp)
  " "csv" \
    "customer_id" "$customer_id" \
    "service" "${service:-all}" \
    "start_date" "${start_date:-}" \
    "end_date" "${end_date:-}"
}
```

---

## File-by-File Instructions

### File 1: `src/lib/billing/usage.sh`

**Total Vulnerabilities**: ~60
**Risk Level**: CRITICAL

#### Section 1: usage_get_all_table() - Lines 286-338

**Vulnerabilities**:
- Line 305-312: Direct string interpolation in WHERE clause

**Fix**:
```bash
# Find line 305-312 (usage query)
# Replace:
usage=$(billing_db_query "
  SELECT COALESCE(SUM(quantity), 0)
  FROM billing_usage_records
  WHERE customer_id = '${customer_id}'
  AND service_name = '${service}'
  AND recorded_at >= '${start_date}'
  AND recorded_at <= '${end_date}';
" | tr -d ' ')

# With:
# Validate inputs (add at start of loop)
validate_customer_id "$customer_id" || return 1
validate_service_name "$service" || return 1

usage=$(billing_db_query "
  SELECT COALESCE(SUM(quantity), 0)
  FROM billing_usage_records
  WHERE customer_id = :'customer_id'
  AND service_name = :'service_name'
  AND recorded_at >= :'start_date'
  AND recorded_at <= :'end_date';
" "tuples" \
  "customer_id" "$customer_id" \
  "service_name" "$service" \
  "start_date" "$start_date" \
  "end_date" "$end_date" | tr -d ' ')
```

#### Section 2: usage_get_service_table() - Lines 341-379

**Vulnerabilities**:
- Lines 350-357: Same pattern as above

**Fix**: Apply same pattern (validate + parameterize)

#### Section 3: usage_export_csv() - Lines 1195-1228

**Vulnerabilities**:
- Lines 1202-1210: Dynamic WHERE clause construction

**Fix**: Use Template 6 (Dynamic WHERE Clauses)

#### Section 4: usage_aggregate_*() functions - Lines 833-924

**Vulnerabilities**:
- All three aggregate functions (hourly, daily, monthly) use string interpolation

**Fix Example** (usage_aggregate_daily):
```bash
# BEFORE
where_clause="customer_id = '${customer_id}'"
if [[ -n "$start_date" ]]; then
  where_clause+=" AND recorded_at >= '${start_date}'"
fi
if [[ -n "$end_date" ]]; then
  where_clause+=" AND recorded_at <= '${end_date}'"
fi

billing_db_query "
  SELECT ...
  WHERE ${where_clause}
"

# AFTER
# Validate
validate_customer_id "$customer_id" || return 1
if [[ -n "$start_date" ]]; then
  validate_date_format "$start_date" || return 1
fi
if [[ -n "$end_date" ]]; then
  validate_date_format "$end_date" || return 1
fi

billing_db_query "
  SELECT ...
  WHERE customer_id = :'customer_id'
  AND (:'start_date' = '' OR recorded_at >= :'start_date'::timestamp)
  AND (:'end_date' = '' OR recorded_at <= :'end_date'::timestamp)
" "tuples" \
  "customer_id" "$customer_id" \
  "start_date" "${start_date:-}" \
  "end_date" "${end_date:-}"
```

#### Section 5: usage_check_service_alert() - Lines 951-1007

**Vulnerabilities**:
- Lines 957-974: customer_id and service in WHERE clause

**Fix**: Parameterize both customer_id and service

---

### File 2: `src/lib/billing/reports.sh`

**Total Vulnerabilities**: ~30
**Risk Level**: CRITICAL

#### Section 1: report_usage_trends_table() - Lines 482-534

**Vulnerabilities**:
- Lines 486-489: Dynamic service_filter construction
- Line 506: service_filter injected into query

**Fix**:
```bash
# BEFORE
service_filter=""
if [[ "$service" != "all" ]]; then
  service_filter="AND service_name = '${service}'"
fi

billing_db_query "
  ...
  WHERE recorded_at >= NOW() - INTERVAL '${days} days'
  ${service_filter}
" "tuples"

# AFTER
# Validate service if not "all"
if [[ "$service" != "all" ]]; then
  validate_service_name "$service" || return 1
fi

# Validate days is integer
validate_integer "$days" || return 1

billing_db_query "
  ...
  WHERE recorded_at >= NOW() - INTERVAL '1 day' * :'days'
  AND (:'service' = 'all' OR service_name = :'service')
" "tuples" \
  "days" "$days" \
  "service" "$service"
```

#### Section 2: Other Reports

Most other reports already use parameterized queries correctly (e.g., report_mrr).
Review for any remaining string interpolation patterns.

---

### File 3: `src/lib/tenant/lifecycle.sh`

**Total Vulnerabilities**: ~25
**Risk Level**: CRITICAL

#### Section 1: tenant_provision() - Lines 17-105

**Vulnerabilities**:
- Line 34-35: slug_exists check
- Line 44-46: owner_id lookup
- Line 57-61: tenant INSERT
- Line 70-72: create_tenant_schema call
- Line 76-79: tenant_members INSERT

**Fix**:
All database operations must use safe_query or pg_query_safe with validation.

Example for slug_exists check:
```bash
# BEFORE
slug_exists=$(docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "SELECT COUNT(*) FROM tenants.tenants WHERE slug = '$tenant_slug'" | tr -d ' \n')

# AFTER
# Validate slug format
if ! tenant_slug=$(validate_identifier "$tenant_slug" 100 2>/dev/null); then
  error "Invalid tenant slug format"
  return 1
fi

# Use safe query
slug_exists=$(safe_query "
  SELECT COUNT(*) FROM tenants.tenants WHERE slug = :'param1'
" "$tenant_slug" | tr -d ' \n')
```

#### Section 2: tenant_lifecycle_suspend() - Lines 176-202

**Vulnerabilities**:
- Line 184-189: UPDATE with string interpolation
- Line 192-196: UPDATE auth.sessions

**Fix**: Use Template 4 (UPDATE Statement)

---

### File 4: `src/lib/tenant/core.sh`

**Total Vulnerabilities**: ~20
**Risk Level**: HIGH (some already fixed)

**Status**: This file is PARTIALLY FIXED. Some functions use `pg_query_safe()`, others don't.

#### Functions to Fix:
1. `tenant_list()` - Line 219: Direct psql call
2. `tenant_show()` - Line 249-254: String interpolation
3. `tenant_suspend()` - Line 267-274: String interpolation
4. `tenant_activate()` - Line 289-296: String interpolation
5. `tenant_member_remove()` - Line 434-444: String interpolation
6. `tenant_member_list()` - Line 457-469: String interpolation

#### Functions Already Safe:
- `tenant_create()` - Uses pg_query_value with validation
- `tenant_delete()` - Uses pg_query_safe with validation
- `tenant_member_add()` - Uses pg_query_safe with validation

**Pattern**: Apply fixes using safe_query or pg_query_safe consistently.

---

### File 5: `src/lib/tenant/routing.sh`

**Total Vulnerabilities**: ~15
**Risk Level**: HIGH

#### Section 1: generate_custom_domain_ssl() - Lines 207-266

**Vulnerabilities**:
- Line 220-222: is_verified check

**Fix**:
```bash
# BEFORE
verified=$(docker exec -i "$(docker_get_container_name postgres)" \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
  "SELECT is_verified FROM tenants.tenant_domains
   WHERE domain = '$domain' AND tenant_id = '$tenant_id'" | tr -d ' \n')

# AFTER
# Validate domain format (basic check)
if [[ ! "$domain" =~ ^[a-zA-Z0-9.-]+$ ]]; then
  error "Invalid domain format"
  return 1
fi

# Validate tenant_id
if ! tenant_id=$(validate_uuid "$tenant_id" 2>/dev/null); then
  error "Invalid tenant ID format"
  return 1
fi

# Use safe query
verified=$(safe_query "
  SELECT is_verified FROM tenants.tenant_domains
  WHERE domain = :'param1' AND tenant_id = :'param2'
" "$domain" "$tenant_id" | tr -d ' \n')
```

#### Section 2: get_tenant_url() - Lines 272-296

**Vulnerabilities**:
- Line 284-289: custom_domain lookup

**Fix**: Same pattern - validate slug, use safe_query

---

## Testing Your Fixes

### Manual Testing Checklist

For each fixed function:

1. ✅ **Input Validation Tests**
   ```bash
   # Test with valid input
   result=$(function_name "valid-input")
   # Should succeed

   # Test with SQL injection attempt
   result=$(function_name "'; DROP TABLE users; --" 2>&1)
   # Should fail validation gracefully
   ```

2. ✅ **Parameterization Tests**
   ```bash
   # Test with special characters
   result=$(function_name "test@example.com")
   # Should work correctly (special chars treated as literals)
   ```

3. ✅ **Edge Cases**
   ```bash
   # Test with empty string
   result=$(function_name "" 2>&1)
   # Should fail validation

   # Test with very long input
   result=$(function_name "$(printf 'a%.0s' {1..10000})" 2>&1)
   # Should fail validation (length check)
   ```

### Automated Testing

Create: `src/tests/security/test-sql-injection.sh`

```bash
#!/usr/bin/env bash

source "$(dirname "$0")/../../lib/utils/test-framework.sh"
source "$(dirname "$0")/../../lib/billing/usage.sh"
source "$(dirname "$0")/../../lib/tenant/core.sh"

# Test 1: Validate customer_id injection attempt
test_customer_id_sql_injection() {
  local malicious_id="1' OR '1'='1"

  # Should fail validation
  if validate_customer_id "$malicious_id" 2>/dev/null; then
    fail "Validation should reject SQL injection in customer_id"
  fi

  pass "SQL injection blocked in customer_id validation"
}

# Test 2: Validate service name injection
test_service_name_sql_injection() {
  local malicious_service="api'; DELETE FROM billing_usage_records; --"

  # Should fail validation
  if validate_service_name "$malicious_service" 2>/dev/null; then
    fail "Validation should reject SQL injection in service name"
  fi

  pass "SQL injection blocked in service name validation"
}

# Test 3: Parameterized query safety
test_parameterized_query_injection() {
  # Even if validation is bypassed, parameterized query should be safe
  local test_customer_id="test-customer-123"
  local test_service="api"

  # Create test data
  # ... (test data setup)

  # Attempt to inject via parameterized query
  # The injection attempt will be treated as a literal string, not SQL
  local result
  result=$(billing_db_query "
    SELECT COUNT(*) FROM billing_usage_records
    WHERE customer_id = :'customer_id'
    AND service_name = :'service_name'
  " "tuples" \
    "customer_id" "1' OR '1'='1" \
    "service_name" "$test_service")

  # Should return 0 (no match for literal string "1' OR '1'='1")
  if [[ "$result" -ne 0 ]]; then
    fail "Parameterized query did not prevent SQL injection"
  fi

  pass "Parameterized query successfully prevented SQL injection"
}

# Run tests
run_tests
```

Run with:
```bash
bash src/tests/security/test-sql-injection.sh
```

---

## Common Pitfalls

### Pitfall 1: Forgetting to Validate Before Parameterizing

```bash
# WRONG - No validation
billing_db_query "SELECT * FROM table WHERE id = :'id'" "tuples" "id" "$user_input"

# RIGHT - Validate first
if ! id=$(validate_uuid "$user_input" 2>/dev/null); then
  error "Invalid ID"
  return 1
fi
billing_db_query "SELECT * FROM table WHERE id = :'id'" "tuples" "id" "$id"
```

**Why**: Validation provides defense-in-depth. Even if parameterization fails, validation catches malicious input.

---

### Pitfall 2: Partial Parameterization

```bash
# WRONG - Only some parameters are safe
billing_db_query "
  SELECT * FROM table
  WHERE id = :'id'
  AND date >= '${start_date}'  # STILL VULNERABLE!
" "tuples" "id" "$id"

# RIGHT - All parameters must be parameterized
billing_db_query "
  SELECT * FROM table
  WHERE id = :'id'
  AND date >= :'start_date'
" "tuples" "id" "$id" "start_date" "$start_date"
```

---

### Pitfall 3: Building SQL Fragments

```bash
# WRONG - Building SQL fragments for reuse
filter_clause="AND status = '${status}'"
billing_db_query "SELECT * FROM table WHERE id = :'id' ${filter_clause}" "tuples" "id" "$id"

# RIGHT - Include all logic in parameterized query
billing_db_query "
  SELECT * FROM table
  WHERE id = :'id'
  AND (:'status' = '' OR status = :'status')
" "tuples" "id" "$id" "status" "${status:-}"
```

---

### Pitfall 4: Trusting "Internal" Variables

```bash
# WRONG - Assuming internal variables are safe
local service="api"  # Hard-coded
billing_db_query "SELECT * FROM table WHERE service = '${service}'" # STILL WRONG

# RIGHT - Always parameterize, even for "safe" values
billing_db_query "SELECT * FROM table WHERE service = :'service'" "tuples" "service" "$service"
```

**Why**: Future code changes might make "service" user-controllable. Always parameterize.

---

### Pitfall 5: Incorrect Parameter Syntax

```bash
# WRONG - Missing quotes around placeholder
billing_db_query "SELECT * FROM table WHERE id = :id" "tuples" "id" "$value"
# PostgreSQL error: syntax error at or near ":"

# RIGHT - Always use :'param' syntax
billing_db_query "SELECT * FROM table WHERE id = :'id'" "tuples" "id" "$value"
```

---

## Checklist for Each Fix

Before marking a vulnerability as "fixed":

- [ ] All user inputs validated with appropriate validation function
- [ ] All SQL uses `:param` syntax (no `${var}` in SQL strings)
- [ ] All parameters passed to billing_db_query or safe_query
- [ ] No dynamic SQL fragment construction
- [ ] Manual test with injection attempt passes
- [ ] Automated test added (if applicable)
- [ ] Code review completed
- [ ] Git commit with descriptive message

---

## Getting Help

If you encounter issues while fixing vulnerabilities:

1. Review this guide and the remediation report
2. Check existing fixed functions for examples (e.g., `tenant_create()` in core.sh)
3. Test your fix thoroughly before committing
4. Ask for code review if unsure

---

## Completion Tracking

Use this to track your progress:

**File 1: billing/usage.sh** (60 vulnerabilities)
- [ ] usage_get_all_table()
- [ ] usage_get_service_table()
- [ ] usage_get_all_json()
- [ ] usage_get_all_csv()
- [ ] usage_get_service_json()
- [ ] usage_get_service_csv()
- [ ] usage_aggregate_hourly()
- [ ] usage_aggregate_daily()
- [ ] usage_aggregate_monthly()
- [ ] usage_check_service_alert()
- [ ] usage_export_csv()
- [ ] usage_export_json()
- [ ] usage_get_peaks()

**File 2: billing/reports.sh** (30 vulnerabilities)
- [ ] report_usage_trends_table()
- [ ] report_usage_trends_csv()
- [ ] report_usage_trends_json()
- [ ] report_churn_table() (review - may be safe)
- [ ] report_aging_table() (review - may be safe)

**File 3: tenant/lifecycle.sh** (25 vulnerabilities)
- [ ] tenant_provision()
- [ ] create_tenant_owner()
- [ ] initialize_tenant_settings()
- [ ] tenant_lifecycle_suspend()
- [ ] tenant_lifecycle_activate()
- [ ] tenant_soft_delete()
- [ ] tenant_permanent_delete()
- [ ] tenant_migrate_plan()
- [ ] tenant_create_backup()
- [ ] tenant_health_check()

**File 4: tenant/core.sh** (20 vulnerabilities)
- [ ] tenant_list()
- [ ] tenant_show()
- [ ] tenant_suspend()
- [ ] tenant_activate()
- [ ] tenant_member_remove()
- [ ] tenant_member_list()
- [ ] tenant_domain_add()

**File 5: tenant/routing.sh** (15 vulnerabilities)
- [ ] generate_custom_domain_ssl()
- [ ] get_tenant_url()

**Total Progress**: _____ / 150 vulnerabilities fixed

---

**Last Updated**: 2026-01-31
**Next Review**: After 20% of vulnerabilities fixed (30 fixes)
