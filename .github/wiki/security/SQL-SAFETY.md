# SQL Injection Prevention - Safe Query Patterns

**Version:** 1.0
**Last Updated:** 2026-01-30
**Criticality:** CRITICAL SECURITY REQUIREMENT

## Table of Contents

- [Overview](#overview)
- [The Problem: SQL Injection](#the-problem-sql-injection)
- [The Solution: Parameterized Queries](#the-solution-parameterized-queries)
- [Safe Query Library (safe-query.sh)](#safe-query-library-safe-querysh)
- [Usage Examples](#usage-examples)
- [Migration Guide](#migration-guide)
- [Testing for SQL Injection](#testing-for-sql-injection)
- [Code Review Checklist](#code-review-checklist)

---

## Overview

**CRITICAL**: All SQL queries that include user input MUST use parameterized queries to prevent SQL injection attacks.

nself provides `/src/lib/database/safe-query.sh` - a comprehensive library for safe database operations.

### Key Principle

**NEVER concatenate user input directly into SQL queries.**

```bash
# ❌ WRONG - SQL Injection vulnerability
local email="$1"
psql -c "SELECT * FROM users WHERE email = '$email'"

# ✅ RIGHT - Parameterized query
local email="$1"
pg_query_safe "SELECT * FROM users WHERE email = :'param1'" "$email"
```

---

## The Problem: SQL Injection

SQL injection is the #1 web application security risk (OWASP Top 10 - A03:2021).

### Attack Example

```bash
# Vulnerable code:
user_delete() {
  local user_id="$1"
  psql -c "DELETE FROM users WHERE id = '$user_id'"
}

# Attacker input:
user_id="1' OR '1'='1"

# Resulting SQL:
DELETE FROM users WHERE id = '1' OR '1'='1'
# ☠️ DELETES ALL USERS!
```

### Real-World Consequences

1. **Data Breach** - Attackers can read sensitive data
2. **Data Loss** - Attackers can delete or modify data
3. **Authentication Bypass** - Attackers can bypass login
4. **Privilege Escalation** - Attackers can gain admin access
5. **Complete System Compromise** - via `xp_cmdshell`, `pg_read_file()`, etc.

---

## The Solution: Parameterized Queries

Parameterized queries (also called prepared statements) separate SQL code from data.

### How It Works

PostgreSQL's `psql -v` flag allows safe parameter binding:

```bash
# Parameters are set as variables
psql -v email='user@example.com' -v limit=10

# SQL uses these variables (notice the colon prefix)
psql -c "SELECT * FROM users WHERE email = :'email' LIMIT :limit"
```

**The Key Difference:**

- `:'param'` - String parameter (automatically quoted and escaped)
- `:param` - Numeric parameter (no quotes)

PostgreSQL treats parameters as **data**, not **code**, preventing injection.

---

## Safe Query Library (safe-query.sh)

Location: `/src/lib/database/safe-query.sh`

### Core Functions

#### 1. Query Execution

```bash
# Execute parameterized query
pg_query_safe <query> [param1] [param2] ...

# Execute and return single value
pg_query_value <query> [param1] [param2] ...

# Execute and return JSON object
pg_query_json <query> [param1] [param2] ...

# Execute and return JSON array
pg_query_json_array <query> [param1] [param2] ...
```

#### 2. Input Validation

```bash
# Validate UUID (returns validated UUID or exits with error)
validate_uuid <uuid>

# Validate email
validate_email <email>

# Validate integer (with optional min/max)
validate_integer <value> [min] [max]

# Validate alphanumeric identifier
validate_identifier <value> [max_length]

# Validate JSON
validate_json <json_string>
```

#### 3. Query Builders (Common Patterns)

```bash
# SELECT by ID
pg_select_by_id <table> <id_column> <id_value> [columns]

# INSERT returning ID
pg_insert_returning_id <table> <columns> <values...>

# UPDATE by ID
pg_update_by_id <table> <id_column> <id_value> <update_columns> <values...>

# DELETE by ID
pg_delete_by_id <table> <id_column> <id_value>

# COUNT records
pg_count <table> [where_clause] [where_value]

# Check if record EXISTS
pg_exists <table> <column> <value>
```

#### 4. Transaction Support

```bash
pg_begin      # Begin transaction
pg_commit     # Commit transaction
pg_rollback   # Rollback transaction
```

---

## Usage Examples

### Example 1: Simple SELECT

```bash
#!/usr/bin/env bash
source "/path/to/safe-query.sh"

get_user_by_email() {
  local email="$1"

  # Validate input
  email=$(validate_email "$email") || return 1

  # Execute safe query
  local query="SELECT row_to_json(u) FROM (
       SELECT id, email, created_at
       FROM auth.users
       WHERE email = :'param1'
     ) u"

  pg_query_json "$query" "$email"
}
```

### Example 2: INSERT with Multiple Parameters

```bash
create_user() {
  local email="$1"
  local phone="$2"
  local password_hash="$3"

  # Validate inputs
  email=$(validate_email "$email") || return 1

  if [[ ! "$phone" =~ ^\+?[0-9]{10,15}$ ]]; then
    echo "ERROR: Invalid phone" >&2
    return 1
  fi

  # Use query builder
  local user_id
  user_id=$(pg_insert_returning_id "auth.users" \
    "email, phone, password_hash, created_at" \
    "$email" "$phone" "$password_hash" "NOW()")

  echo "$user_id"
}
```

### Example 3: UPDATE with Dynamic Columns

```bash
update_user() {
  local user_id="$1"
  local new_email="$2"
  local new_phone="$3"

  # Validate UUID
  user_id=$(validate_uuid "$user_id") || return 1

  # Validate new values
  new_email=$(validate_email "$new_email") || return 1

  # Use update builder
  pg_update_by_id "auth.users" "id" "$user_id" \
    "email, phone, updated_at" \
    "$new_email" "$new_phone" "NOW()"
}
```

### Example 4: Search with LIKE

```bash
search_users() {
  local search_term="$1"

  # Add wildcards in the parameter VALUE, not in SQL
  local pattern="%${search_term}%"

  local query="SELECT json_agg(u) FROM (
       SELECT id, email
       FROM auth.users
       WHERE email ILIKE :'param1'
       ORDER BY email
       LIMIT 50
     ) u"

  pg_query_json_array "$query" "$pattern"
}
```

### Example 5: Complex Query with Multiple Parameters

```bash
get_activity_logs() {
  local user_id="$1"
  local hours="$2"
  local limit="$3"

  # Validate inputs
  user_id=$(validate_uuid "$user_id") || return 1
  hours=$(validate_integer "$hours" 1 720) || return 1
  limit=$(validate_integer "$limit" 1 1000) || return 1

  local query="SELECT json_agg(a) FROM (
       SELECT event_type, action, created_at
       FROM audit.events
       WHERE user_id = :'param1'
         AND created_at >= NOW() - INTERVAL '1 hour' * :param2
       ORDER BY created_at DESC
       LIMIT :param3
     ) a"

  pg_query_json_array "$query" "$user_id" "$hours" "$limit"
}
```

### Example 6: Transaction Example

```bash
transfer_funds() {
  local from_account="$1"
  local to_account="$2"
  local amount="$3"

  # Validate inputs
  from_account=$(validate_uuid "$from_account") || return 1
  to_account=$(validate_uuid "$to_account") || return 1
  amount=$(validate_integer "$amount" 1) || return 1

  # Begin transaction
  pg_begin

  # Deduct from source
  local query1="UPDATE accounts SET balance = balance - :param2
                WHERE id = :'param1' AND balance >= :param2"
  if ! pg_query_safe "$query1" "$from_account" "$amount"; then
    pg_rollback
    echo "ERROR: Insufficient funds" >&2
    return 1
  fi

  # Add to destination
  local query2="UPDATE accounts SET balance = balance + :param2
                WHERE id = :'param1'"
  if ! pg_query_safe "$query2" "$to_account" "$amount"; then
    pg_rollback
    echo "ERROR: Transfer failed" >&2
    return 1
  fi

  # Commit transaction
  pg_commit
  return 0
}
```

---

## Migration Guide

### Step 1: Identify Vulnerable Queries

Search for SQL queries with string interpolation:

```bash
# Find all potentially vulnerable queries
grep -rn "psql.*-c.*\$" src/lib/

# Look for string interpolation in SQL
grep -rn "WHERE.*=.*\'\$" src/lib/
grep -rn "INSERT.*VALUES.*\$" src/lib/
```

### Step 2: Source the Safe Query Library

```bash
# Add to the top of your script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../database/safe-query.sh"
```

### Step 3: Convert Vulnerable Queries

#### Before (Vulnerable):

```bash
user_get_by_email() {
  local email="$1"
  local user=$(docker exec postgres psql -U postgres -d mydb -t -c \
    "SELECT row_to_json(u) FROM (
       SELECT id, email FROM users WHERE email = '$email'
     ) u" | xargs)
  echo "$user"
}
```

#### After (Safe):

```bash
user_get_by_email() {
  local email="$1"

  # Validate input
  email=$(validate_email "$email") || return 1

  # Use parameterized query
  local query="SELECT row_to_json(u) FROM (
       SELECT id, email FROM users WHERE email = :'param1'
     ) u"

  pg_query_json "$query" "$email"
}
```

### Step 4: Add Input Validation

Always validate user input before using it:

```bash
# UUIDs
user_id=$(validate_uuid "$user_id") || return 1

# Emails
email=$(validate_email "$email") || return 1

# Integers
limit=$(validate_integer "$limit" 1 1000) || return 1

# Identifiers (alphanumeric)
role_name=$(validate_identifier "$role_name" 100) || return 1

# JSON
metadata=$(validate_json "$metadata") || return 1
```

### Step 5: Test

Run tests with injection attempts:

```bash
# Test with malicious input
user_delete "1' OR '1'='1"  # Should fail validation

# Test with SQL keywords
user_create "admin'; DROP TABLE users; --"  # Should be safely escaped
```

---

## Testing for SQL Injection

### Manual Testing

```bash
# Test 1: Single quote bypass
nself auth user create "admin'--" "password"

# Test 2: UNION attack
nself auth user get "1' UNION SELECT password FROM users--"

# Test 3: Boolean-based blind
nself auth user get "1' AND 1=1--"
nself auth user get "1' AND 1=2--"

# Test 4: Time-based blind
nself auth user get "1'; SELECT pg_sleep(5)--"

# All of these should either:
# - Fail input validation
# - Be safely escaped (no SQL executed)
```

### Automated Testing

```bash
# Create test file: src/tests/security/test-sql-injection.sh

#!/usr/bin/env bash
source "src/lib/database/safe-query.sh"
source "src/lib/auth/user-manager.sh"

test_sql_injection_attempts() {
  local injection_payloads=(
    "1' OR '1'='1"
    "admin'--"
    "1'; DROP TABLE users--"
    "1' UNION SELECT password FROM users--"
    "' OR 1=1--"
  )

  for payload in "${injection_payloads[@]}"; do
    # Should fail gracefully
    if user_get_by_id "$payload" 2>/dev/null; then
      echo "❌ FAIL: Injection succeeded with: $payload"
      return 1
    else
      echo "✓ PASS: Injection blocked: $payload"
    fi
  done

  return 0
}

test_sql_injection_attempts
```

---

## Code Review Checklist

When reviewing code, check for:

### ❌ Anti-Patterns (Reject)

```bash
# Direct string interpolation in SQL
psql -c "SELECT * FROM users WHERE id = '$user_id'"

# Unvalidated user input
local email="$1"
psql -c "INSERT INTO users (email) VALUES ('$email')"

# Dynamic SQL construction
local where="name = '$name' AND email = '$email'"
psql -c "SELECT * FROM users WHERE $where"

# LIKE without escaping
psql -c "SELECT * FROM users WHERE name LIKE '%$search%'"
```

### ✅ Correct Patterns (Approve)

```bash
# Parameterized queries
pg_query_safe "SELECT * FROM users WHERE id = :'param1'" "$user_id"

# Input validation first
email=$(validate_email "$email") || return 1
pg_query_safe "INSERT INTO users (email) VALUES (:'param1')" "$email"

# Safe query builders
pg_select_by_id "users" "id" "$user_id"

# LIKE with parameterized pattern
local pattern="%${search}%"
pg_query_safe "SELECT * FROM users WHERE name LIKE :'param1'" "$pattern"
```

### Review Questions

- [ ] Does the query include user input?
- [ ] Is the input validated before use?
- [ ] Are parameterized queries used (:'param1' notation)?
- [ ] Are numeric parameters used correctly (:param without quotes)?
- [ ] Are dynamic column names validated (if any)?
- [ ] Is error handling present?
- [ ] Are transactions used where appropriate?

---

## Additional Resources

### OWASP Resources

- [OWASP Top 10 - A03: Injection](https://owasp.org/Top10/A03_2021-Injection/)
- [SQL Injection Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html)
- [Query Parameterization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Query_Parameterization_Cheat_Sheet.html)

### PostgreSQL Documentation

- [psql Variables](https://www.postgresql.org/docs/current/app-psql.html#APP-PSQL-VARIABLES)
- [Prepared Statements](https://www.postgresql.org/docs/current/sql-prepare.html)
- [Security Best Practices](https://www.postgresql.org/docs/current/security.html)

### nself Documentation

- `/src/lib/database/safe-query.sh` - Source code with inline documentation
- `/src/lib/auth/user-manager.sh` - Example of safe implementation
- `/src/lib/auth/role-manager.sh` - Example of safe implementation
- `/src/lib/admin/api.sh` - Example of safe implementation

---

## Summary

### Remember

1. **NEVER** concatenate user input into SQL
2. **ALWAYS** use `pg_query_safe()` and parameterized queries
3. **ALWAYS** validate user input with `validate_*()` functions
4. **USE** `:'param'` for strings, `:param` for numbers
5. **TEST** with SQL injection payloads

### Safe Query Workflow

```
User Input → Validation → Parameterized Query → Safe Execution
```

```bash
local email="$1"                                    # User input
email=$(validate_email "$email") || return 1        # Validation
pg_query_json "SELECT ... WHERE email = :'param1'"  # Parameterized
              "$email"                               # Safe execution
```

---

**Questions or Issues?**

- Review the source: `/src/lib/database/safe-query.sh`
- Check examples: `/src/lib/auth/*.sh`
- File security issues: GitHub Security Advisories

**This is a critical security requirement. No exceptions.**
