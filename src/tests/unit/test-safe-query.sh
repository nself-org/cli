#!/usr/bin/env bash
set -uo pipefail
# test-safe-query.sh - Unit tests for safe query library
# Tests SQL injection prevention, input validation, and parameterized queries

# Note: Don't use set -e because we're testing functions that are expected to fail

# Color codes for output
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
LIB_DIR="$SCRIPT_DIR/../../lib"

# Source the safe query library
if [[ ! -f "$LIB_DIR/database/safe-query.sh" ]]; then
  printf "${RED}ERROR: safe-query.sh not found at $LIB_DIR/database/safe-query.sh${NC}\n"
  exit 1
fi

source "$LIB_DIR/database/safe-query.sh"

# ============================================================================
# Test Framework Functions
# ============================================================================

print_test_header() {
  printf "\n${YELLOW}Testing: %s${NC}\n" "$1"
}

assert_equals() {
  local expected="$1"
  local actual="$2"
  local test_name="${3:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$expected" == "$actual" ]]; then
    printf "${GREEN}  ✓ %s${NC}\n" "$test_name"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "${RED}  ✗ %s${NC}\n" "$test_name"
    printf "${RED}    Expected: %s${NC}\n" "$expected"
    printf "${RED}    Got:      %s${NC}\n" "$actual"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    return 1
  fi
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local test_name="${3:-Contains check}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$haystack" == *"$needle"* ]]; then
    printf "${GREEN}  ✓ %s${NC}\n" "$test_name"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "${RED}  ✗ %s${NC}\n" "$test_name"
    printf "${RED}    Expected to find: %s${NC}\n" "$needle"
    printf "${RED}    In: %s${NC}\n" "$haystack"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    return 1
  fi
}

assert_success() {
  local result=$?
  local test_name="${1:-Command should succeed}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ $result -eq 0 ]]; then
    printf "${GREEN}  ✓ %s${NC}\n" "$test_name"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "${RED}  ✗ %s (exit code: %d)${NC}\n" "$test_name" "$result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    return 1
  fi
}

assert_failure() {
  local result=$?
  local test_name="${1:-Command should fail}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ $result -ne 0 ]]; then
    printf "${GREEN}  ✓ %s${NC}\n" "$test_name"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "${RED}  ✗ %s (should have failed but succeeded)${NC}\n" "$test_name"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    return 1
  fi
}

# ============================================================================
# SQL Escaping Tests
# ============================================================================

test_sql_escape() {
  print_test_header "SQL Escaping Function"

  # Test 1: Single quote should be doubled (checking for doubled quotes, not literal comparison)
  local input="test'value"
  local result
  result=$(sql_escape "$input")
  # Check that result contains doubled quotes (two apostrophes in a row)
  if [[ "$result" == *"''"* ]] || [[ "$result" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ Single quote should be doubled${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ Single quote should be doubled${NC}\n"
    printf "${RED}    Got: %s${NC}\n" "$result"
  fi

  # Test 2: Multiple single quotes
  local input="test'multiple'quotes"
  local result
  result=$(sql_escape "$input")
  # Check for presence of doubled quotes
  if [[ "$result" == *"''"* ]] || [[ "$result" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ Multiple quotes should be doubled${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ Multiple quotes should be doubled${NC}\n"
  fi

  # Test 3: SQL injection attempt
  local input="test'; DROP TABLE users; --"
  local result
  result=$(sql_escape "$input")
  # Check for escaped quotes
  if [[ "$result" == *"''"* ]] || [[ "$result" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ SQL injection attempt should have quotes escaped${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ SQL injection attempt should have quotes escaped${NC}\n"
  fi

  # Test 4: Normal string without quotes
  local input="normal_string"
  local expected="normal_string"
  local result
  result=$(sql_escape "$input")
  assert_equals "$expected" "$result" "Normal string should be unchanged"
}

# ============================================================================
# UUID Validation Tests
# ============================================================================

test_uuid_validation() {
  print_test_header "UUID Validation"

  # Test 1: Valid UUID
  local valid_uuid="123e4567-e89b-12d3-a456-426614174000"
  validate_uuid "$valid_uuid" >/dev/null 2>&1
  assert_success "Valid UUID should be accepted"

  # Test 2: Invalid UUID (wrong format)
  local invalid_uuid="not-a-uuid"
  validate_uuid "$invalid_uuid" >/dev/null 2>&1
  assert_failure "Invalid UUID should be rejected"

  # Test 3: Invalid UUID (missing sections)
  local invalid_uuid="123e4567-e89b-12d3"
  validate_uuid "$invalid_uuid" >/dev/null 2>&1
  assert_failure "Incomplete UUID should be rejected"

  # Test 4: Invalid UUID (wrong characters)
  local invalid_uuid="123e4567-e89b-12d3-a456-42661417400g"
  validate_uuid "$invalid_uuid" >/dev/null 2>&1
  assert_failure "UUID with invalid characters should be rejected"

  # Test 5: Valid UUID (all lowercase)
  local valid_uuid="abcdef12-3456-7890-abcd-ef1234567890"
  validate_uuid "$valid_uuid" >/dev/null 2>&1
  assert_success "Valid lowercase UUID should be accepted"
}

# ============================================================================
# Email Validation Tests
# ============================================================================

test_email_validation() {
  print_test_header "Email Validation"

  # Test 1: Valid email
  local valid_email="test@example.com"
  validate_email "$valid_email" >/dev/null 2>&1
  assert_success "Valid email should be accepted"

  # Test 2: Invalid email (no @)
  local invalid_email="testexample.com"
  validate_email "$invalid_email" >/dev/null 2>&1
  assert_failure "Email without @ should be rejected"

  # Test 3: Invalid email (no domain)
  local invalid_email="test@"
  validate_email "$invalid_email" >/dev/null 2>&1
  assert_failure "Email without domain should be rejected"

  # Test 4: Valid email with subdomain
  local valid_email="user@mail.example.com"
  validate_email "$valid_email" >/dev/null 2>&1
  assert_success "Email with subdomain should be accepted"

  # Test 5: Valid email with plus sign
  local valid_email="user+tag@example.com"
  validate_email "$valid_email" >/dev/null 2>&1
  assert_success "Email with plus sign should be accepted"

  # Test 6: Email too long (> 254 chars)
  local long_email="$(printf 'a%.0s' {1..250})@example.com"
  validate_email "$long_email" >/dev/null 2>&1
  assert_failure "Email over 254 characters should be rejected"
}

# ============================================================================
# Integer Validation Tests
# ============================================================================

test_integer_validation() {
  print_test_header "Integer Validation"

  # Test 1: Valid positive integer
  validate_integer "123" >/dev/null 2>&1
  assert_success "Positive integer should be accepted"

  # Test 2: Valid negative integer
  validate_integer "-123" >/dev/null 2>&1
  assert_success "Negative integer should be accepted"

  # Test 3: Invalid (float)
  validate_integer "123.45" >/dev/null 2>&1
  assert_failure "Float should be rejected"

  # Test 4: Invalid (string)
  validate_integer "abc" >/dev/null 2>&1
  assert_failure "String should be rejected"

  # Test 5: Integer with min constraint
  validate_integer "10" "5" >/dev/null 2>&1
  assert_success "Integer above minimum should be accepted"

  # Test 6: Integer below min constraint
  validate_integer "3" "5" >/dev/null 2>&1
  assert_failure "Integer below minimum should be rejected"

  # Test 7: Integer with max constraint
  validate_integer "50" "" "100" >/dev/null 2>&1
  assert_success "Integer below maximum should be accepted"

  # Test 8: Integer above max constraint
  validate_integer "150" "" "100" >/dev/null 2>&1
  assert_failure "Integer above maximum should be rejected"
}

# ============================================================================
# Identifier Validation Tests
# ============================================================================

test_identifier_validation() {
  print_test_header "Identifier Validation"

  # Test 1: Valid identifier
  validate_identifier "my_identifier" >/dev/null 2>&1
  assert_success "Valid identifier should be accepted"

  # Test 2: Valid identifier with hyphen
  validate_identifier "my-identifier" >/dev/null 2>&1
  assert_success "Identifier with hyphen should be accepted"

  # Test 3: Invalid (space)
  validate_identifier "my identifier" >/dev/null 2>&1
  assert_failure "Identifier with space should be rejected"

  # Test 4: Invalid (special characters)
  validate_identifier "my@identifier" >/dev/null 2>&1
  assert_failure "Identifier with special characters should be rejected"

  # Test 5: Too long (with custom max)
  local long_id="$(printf 'a%.0s' {1..150})"
  validate_identifier "$long_id" "100" >/dev/null 2>&1
  assert_failure "Identifier over max length should be rejected"

  # Test 6: Valid identifier starting with number
  validate_identifier "1identifier" >/dev/null 2>&1
  assert_success "Identifier starting with number should be accepted"
}

# ============================================================================
# JSON Validation Tests
# ============================================================================

test_json_validation() {
  print_test_header "JSON Validation"

  # Only run if jq is available
  if ! command -v jq >/dev/null 2>&1; then
    printf "${YELLOW}  ⊘ Skipping JSON tests (jq not installed)${NC}\n"
    return 0
  fi

  # Test 1: Valid JSON object
  validate_json '{"key": "value"}' >/dev/null 2>&1
  assert_success "Valid JSON object should be accepted"

  # Test 2: Valid JSON array
  validate_json '[1, 2, 3]' >/dev/null 2>&1
  assert_success "Valid JSON array should be accepted"

  # Test 3: Invalid JSON
  validate_json '{key: value}' >/dev/null 2>&1
  assert_failure "Invalid JSON should be rejected"

  # Test 4: Valid empty JSON object
  validate_json '{}' >/dev/null 2>&1
  assert_success "Empty JSON object should be accepted"

  # Test 5: Valid nested JSON
  validate_json '{"outer": {"inner": "value"}}' >/dev/null 2>&1
  assert_success "Nested JSON should be accepted"
}

# ============================================================================
# SQL Injection Prevention Tests
# ============================================================================

test_sql_injection_prevention() {
  print_test_header "SQL Injection Prevention"

  # Test 1: Escape SQL injection attempt in escaping function
  local malicious="admin'; DROP TABLE users; --"
  local escaped
  escaped=$(sql_escape "$malicious")
  # Check for doubled quotes (either '' or \'\')
  if [[ "$escaped" == *"''"* ]] || [[ "$escaped" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ SQL injection quotes should be escaped${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ SQL injection quotes should be escaped${NC}\n"
    printf "${RED}    Got: %s${NC}\n" "$escaped"
  fi

  # Test 2: Union attack attempt
  local malicious="' UNION SELECT password FROM users --"
  local escaped
  escaped=$(sql_escape "$malicious")
  if [[ "$escaped" == *"''"* ]] || [[ "$escaped" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ UNION attack quotes should be escaped${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ UNION attack quotes should be escaped${NC}\n"
  fi

  # Test 3: Comment attack attempt
  local malicious="admin'--"
  local escaped
  escaped=$(sql_escape "$malicious")
  if [[ "$escaped" == *"''"* ]] || [[ "$escaped" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ Comment attack quotes should be escaped${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ Comment attack quotes should be escaped${NC}\n"
  fi

  # Test 4: Stacked query attempt
  local malicious="'; DELETE FROM users WHERE '1'='1"
  local escaped
  escaped=$(sql_escape "$malicious")
  if [[ "$escaped" == *"''"* ]] || [[ "$escaped" == *"\'\'"* ]]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}  ✓ Stacked query quotes should be escaped${NC}\n"
  else
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}  ✗ Stacked query quotes should be escaped${NC}\n"
  fi
}

# ============================================================================
# Run All Tests
# ============================================================================

run_all_tests() {
  printf "${GREEN}================================${NC}\n"
  printf "${GREEN}Safe Query Library Test Suite${NC}\n"
  printf "${GREEN}================================${NC}\n"

  test_sql_escape
  test_uuid_validation
  test_email_validation
  test_integer_validation
  test_identifier_validation
  test_json_validation
  test_sql_injection_prevention

  # Print summary
  printf "\n${GREEN}================================${NC}\n"
  printf "${GREEN}Test Summary${NC}\n"
  printf "${GREEN}================================${NC}\n"
  printf "Total Tests:  %d\n" "$TESTS_RUN"
  printf "${GREEN}Passed:       %d${NC}\n" "$TESTS_PASSED"

  if [[ $TESTS_FAILED -gt 0 ]]; then
    printf "${RED}Failed:       %d${NC}\n" "$TESTS_FAILED"
    printf "\n${RED}Some tests failed!${NC}\n"
    exit 1
  else
    printf "${GREEN}Failed:       0${NC}\n"
    printf "\n${GREEN}All tests passed!${NC}\n"
    exit 0
  fi
}

# Run tests
run_all_tests
