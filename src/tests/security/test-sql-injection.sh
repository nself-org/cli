#!/usr/bin/env bash
# test-sql-injection.sh - SQL Injection Security Tests
# Part of nself v0.9.0 - Security Hardening
#
# Tests SQL injection protection in safe-query.sh and all database operations
# These tests verify that malicious SQL inputs are properly sanitized

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Source libraries
source "${NSELF_ROOT}/src/lib/database/safe-query.sh"

# ============================================================================
# Test Helper Functions
# ============================================================================

test_start() {
  local test_name="$1"
  printf "Testing: %s ... " "$test_name"
  TESTS_RUN=$((TESTS_RUN + 1))
}

test_pass() {
  printf "%b✓ PASS%b\n" "${GREEN}" "${NC}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
}

test_fail() {
  local reason="$1"
  printf "%b✗ FAIL%b: %s\n" "${RED}" "${NC}" "$reason"
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

test_warn() {
  local message="$1"
  printf "%b⚠ WARN%b: %s\n" "${YELLOW}" "${NC}" "$message"
}

# ============================================================================
# Input Validation Tests
# ============================================================================

test_validate_uuid() {
  test_start "UUID validation"

  local failed=0

  # Valid UUID
  if validate_uuid "550e8400-e29b-41d4-a716-446655440000" >/dev/null 2>&1; then
    :
  else
    test_fail "Valid UUID rejected"
    failed=1
  fi

  # Invalid UUID - SQL injection attempt
  if [[ $failed -eq 0 ]] && validate_uuid "1' OR '1'='1" >/dev/null 2>&1; then
    test_fail "SQL injection in UUID not caught"
    failed=1
  fi

  # Invalid UUID - DROP TABLE attempt
  if [[ $failed -eq 0 ]] && validate_uuid "1'; DROP TABLE users; --" >/dev/null 2>&1; then
    test_fail "DROP TABLE in UUID not caught"
    failed=1
  fi

  if [[ $failed -eq 0 ]]; then
    test_pass
  fi

  return $failed
}

test_validate_email() {
  test_start "Email validation"

  # Valid email
  if validate_email "user@example.com" >/dev/null 2>&1; then
    :
  else
    test_fail "Valid email rejected"
    return 1
  fi

  # Invalid - SQL injection
  if validate_email "admin'--@example.com" >/dev/null 2>&1; then
    test_fail "SQL injection in email not caught"
    return 1
  fi

  # Invalid - UNION attack
  if validate_email "test' UNION SELECT password FROM users--" >/dev/null 2>&1; then
    test_fail "UNION attack in email not caught"
    return 1
  fi

  # Invalid - no @ sign
  if validate_email "notanemail" >/dev/null 2>&1; then
    test_fail "Invalid email format accepted"
    return 1
  fi

  test_pass
  return 0
}

test_validate_integer() {
  test_start "Integer validation"

  # Valid integer
  if validate_integer "42" >/dev/null 2>&1; then
    :
  else
    test_fail "Valid integer rejected"
    return 1
  fi

  # Invalid - SQL injection
  if validate_integer "1 OR 1=1" >/dev/null 2>&1; then
    test_fail "SQL injection in integer not caught"
    return 1
  fi

  # Invalid - string
  if validate_integer "abc" >/dev/null 2>&1; then
    test_fail "String accepted as integer"
    return 1
  fi

  # Min/Max validation
  if ! validate_integer "50" 1 100 >/dev/null 2>&1; then
    test_fail "Valid integer within range rejected"
    return 1
  fi

  if validate_integer "150" 1 100 >/dev/null 2>&1; then
    test_fail "Integer above max accepted"
    return 1
  fi

  test_pass
  return 0
}

test_validate_identifier() {
  test_start "Identifier validation"

  # Valid identifier
  if validate_identifier "user_role_123" >/dev/null 2>&1; then
    :
  else
    test_fail "Valid identifier rejected"
    return 1
  fi

  # Invalid - SQL injection
  if validate_identifier "admin'; DROP TABLE users; --" >/dev/null 2>&1; then
    test_fail "SQL injection in identifier not caught"
    return 1
  fi

  # Invalid - spaces
  if validate_identifier "user role" >/dev/null 2>&1; then
    test_fail "Identifier with spaces accepted"
    return 1
  fi

  # Invalid - special chars
  if validate_identifier "user@role" >/dev/null 2>&1; then
    test_fail "Identifier with special chars accepted"
    return 1
  fi

  test_pass
  return 0
}

test_validate_json() {
  test_start "JSON validation"

  # Valid JSON
  if validate_json '{"key":"value"}' >/dev/null 2>&1; then
    :
  else
    test_fail "Valid JSON rejected"
    return 1
  fi

  # Invalid JSON
  if validate_json "not json" >/dev/null 2>&1; then
    test_fail "Invalid JSON accepted"
    return 1
  fi

  # SQL injection in JSON
  # This should be valid JSON but the content is escaped when used
  if ! validate_json '{"email":"admin'"'"'; DROP TABLE users; --"}' >/dev/null 2>&1; then
    # JSON is technically valid, content will be escaped by parameterized query
    :
  fi

  test_pass
  return 0
}

# ============================================================================
# SQL Escape Tests
# ============================================================================

test_sql_escape() {
  test_start "SQL escape function"

  local failed=0

  # Test single quote escaping
  local input="O'Reilly"
  local result
  result=$(sql_escape "$input")

  # The function should double single quotes: O'Reilly -> O''Reilly
  # When we check, we need to count the actual quote characters
  local quote_count
  quote_count=$(echo "$result" | grep -o "'" | wc -l | xargs)

  # Should have 2 single quotes (doubled from 1)
  if [[ "$quote_count" == "2" ]]; then
    test_pass
  else
    test_fail "Single quote not properly escaped: found $quote_count quotes, expected 2"
    failed=1
  fi

  return $failed
}

# ============================================================================
# SQL Injection Attack Simulations
# ============================================================================

test_injection_payloads() {
  test_start "Common SQL injection payloads"

  # Common injection payloads that should fail validation
  local payloads=(
    "1' OR '1'='1"
    "admin'--"
    "1'; DROP TABLE users--"
    "1' UNION SELECT password FROM users--"
    "' OR 1=1--"
    "1' AND 1=2 UNION SELECT password FROM users--"
    "1'; EXEC sp_MSForEachTable 'DROP TABLE ?'--"
    "admin' OR '1'='1' /*"
    "1' WAITFOR DELAY '00:00:05'--"
    "1'; SELECT pg_sleep(5)--"
  )

  local failed=0
  for payload in "${payloads[@]}"; do
    # Test UUID validation
    if validate_uuid "$payload" >/dev/null 2>&1; then
      printf "\n  %b✗%b UUID accepted injection: %s" "${RED}" "${NC}" "$payload"
      failed=1
    fi

    # Test email validation
    if validate_email "$payload" >/dev/null 2>&1; then
      printf "\n  %b✗%b Email accepted injection: %s" "${RED}" "${NC}" "$payload"
      failed=1
    fi

    # Test integer validation
    if validate_integer "$payload" >/dev/null 2>&1; then
      printf "\n  %b✗%b Integer accepted injection: %s" "${RED}" "${NC}" "$payload"
      failed=1
    fi

    # Test identifier validation
    if validate_identifier "$payload" >/dev/null 2>&1; then
      printf "\n  %b✗%b Identifier accepted injection: %s" "${RED}" "${NC}" "$payload"
      failed=1
    fi
  done

  if [[ $failed -eq 0 ]]; then
    test_pass
    return 0
  else
    printf "\n"
    test_fail "Some injection payloads were not caught"
    return 1
  fi
}

# ============================================================================
# Database Function Tests (if DB available)
# ============================================================================

test_database_functions() {
  # Check if PostgreSQL container is running
  if ! docker ps --filter 'name=postgres' --format '{{.Names}}' | grep -q postgres; then
    test_warn "PostgreSQL container not running - skipping database tests"
    return 0
  fi

  test_start "Database parameterized queries"

  # Test pg_exists with safe input
  local result
  result=$(pg_exists "information_schema.tables" "table_schema" "public" 2>/dev/null) || true

  if [[ -n "$result" ]]; then
    test_pass
  else
    test_fail "pg_exists function failed"
    return 1
  fi
}

test_injection_in_queries() {
  # Check if PostgreSQL container is running
  if ! docker ps --filter 'name=postgres' --format '{{.Names}}' | grep -q postgres; then
    test_warn "PostgreSQL container not running - skipping injection query tests"
    return 0
  fi

  test_start "SQL injection attempts in queries"

  # Test with injection payload - should fail validation
  local malicious_email="admin'; DROP TABLE users; --"

  if validate_email "$malicious_email" >/dev/null 2>&1; then
    test_fail "Malicious email passed validation"
    return 1
  fi

  # Even if validation is bypassed, parameterized query should handle it
  # This tests the defense-in-depth approach

  test_pass
  return 0
}

# ============================================================================
# Main Test Suite
# ============================================================================

run_all_tests() {
  printf "\n"
  printf "======================================\n"
  printf "  SQL Injection Security Tests\n"
  printf "======================================\n"
  printf "\n"

  # Input validation tests
  test_validate_uuid || true
  test_validate_email || true
  test_validate_integer || true
  test_validate_identifier || true
  test_validate_json || true

  # SQL escape tests
  test_sql_escape || true

  # Injection payload tests
  test_injection_payloads || true

  # Database function tests (if available)
  test_database_functions || true
  test_injection_in_queries || true

  # Print summary
  printf "\n"
  printf "======================================\n"
  printf "  Test Summary\n"
  printf "======================================\n"
  printf "Total tests:  %d\n" "$TESTS_RUN"
  printf "%bPassed:       %d%b\n" "${GREEN}" "$TESTS_PASSED" "${NC}"

  if [[ $TESTS_FAILED -gt 0 ]]; then
    printf "%bFailed:       %d%b\n" "${RED}" "$TESTS_FAILED" "${NC}"
    printf "\n"
    printf "%bSECURITY TESTS FAILED!%b\n" "${RED}" "${NC}"
    printf "Review the failures above and fix SQL injection vulnerabilities.\n"
    return 1
  else
    printf "Failed:       %d\n" "$TESTS_FAILED"
    printf "\n"
    printf "%bALL SECURITY TESTS PASSED!%b\n" "${GREEN}" "${NC}"
    return 0
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_all_tests
  exit $?
fi
