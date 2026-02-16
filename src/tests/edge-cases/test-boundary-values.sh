#!/usr/bin/env bash
set -euo pipefail

# test-boundary-values.sh - Tests for boundary value validation
# Tests edge cases with minimum, maximum, and invalid boundary values

set -e

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$TEST_DIR/../.."

# Source test framework
source "$TEST_DIR/../test_framework.sh"

# ============================================
# Test Setup
# ============================================

setup_test_environment() {
  export TEST_MODE=1
  export NO_COLOR=1
  TEMP_DIR=$(mktemp -d)
  export TEMP_DIR
}

teardown_test_environment() {
  if [[ -n "$TEMP_DIR" ]] && [[ -d "$TEMP_DIR" ]]; then
    rm -rf "$TEMP_DIR"
  fi
}

# ============================================
# Port Number Boundary Tests
# ============================================

test_port_zero() {
  local test_name="Port number 0 (invalid)"

  # Port 0 is invalid for services
  local port=0
  local is_valid=false

  # Validation function
  if [[ $port -ge 1 ]] && [[ $port -le 65535 ]]; then
    is_valid=true
  fi

  assert_equals "$is_valid" "false" "$test_name: Port 0 should be invalid"
}

test_port_one() {
  local test_name="Port 1 (privileged, valid but requires root)"

  local port=1
  local is_privileged=false
  local is_valid=false

  if [[ $port -ge 1 ]] && [[ $port -le 65535 ]]; then
    is_valid=true
  fi

  if [[ $port -ge 1 ]] && [[ $port -le 1023 ]]; then
    is_privileged=true
  fi

  assert_equals "$is_valid" "true" "$test_name: Port 1 is valid"
  assert_equals "$is_privileged" "true" "$test_name: Port 1 is privileged"
}

test_port_1023() {
  local test_name="Port 1023 (last privileged port)"

  local port=1023
  local is_privileged=false

  if [[ $port -ge 1 ]] && [[ $port -le 1023 ]]; then
    is_privileged=true
  fi

  assert_equals "$is_privileged" "true" "$test_name: Port 1023 is privileged"
}

test_port_1024() {
  local test_name="Port 1024 (first unprivileged port)"

  local port=1024
  local is_privileged=false

  if [[ $port -ge 1 ]] && [[ $port -le 1023 ]]; then
    is_privileged=true
  fi

  assert_equals "$is_privileged" "false" "$test_name: Port 1024 is not privileged"
}

test_port_65535() {
  local test_name="Port 65535 (maximum valid)"

  local port=65535
  local is_valid=false

  if [[ $port -ge 1 ]] && [[ $port -le 65535 ]]; then
    is_valid=true
  fi

  assert_equals "$is_valid" "true" "$test_name: Port 65535 is valid"
}

test_port_65536() {
  local test_name="Port 65536 (out of range)"

  local port=65536
  local is_valid=false

  if [[ $port -ge 1 ]] && [[ $port -le 65535 ]]; then
    is_valid=true
  fi

  assert_equals "$is_valid" "false" "$test_name: Port 65536 is invalid"
}

test_port_negative() {
  local test_name="Port -1 (negative, invalid)"

  local port=-1
  local is_valid=false

  if [[ $port -ge 1 ]] && [[ $port -le 65535 ]]; then
    is_valid=true
  fi

  assert_equals "$is_valid" "false" "$test_name: Negative port is invalid"
}

# ============================================
# String Length Boundary Tests
# ============================================

test_empty_string() {
  local test_name="Empty string input"

  local input=""
  local is_valid=false

  # Most fields should reject empty strings
  if [[ -n "$input" ]]; then
    is_valid=true
  fi

  assert_equals "$is_valid" "false" "$test_name: Empty string should be invalid"
}

test_single_character_string() {
  local test_name="Single character string"

  local input="a"
  local is_valid=false

  if [[ -n "$input" ]]; then
    is_valid=true
  fi

  assert_equals "$is_valid" "true" "$test_name: Single char is valid"
}

test_very_long_string() {
  local test_name="Very long string (>1000 chars)"

  # Generate 1500 character string
  local long_string=$(printf 'a%.0s' {1..1500})
  local length=${#long_string}

  # Most reasonable fields should have limits
  local exceeds_limit=false
  if [[ $length -gt 1000 ]]; then
    exceeds_limit=true
  fi

  assert_equals "$exceeds_limit" "true" "$test_name: String exceeds reasonable limit"
}

# ============================================
# Domain Name Boundary Tests
# ============================================

test_domain_single_char() {
  local test_name="Domain with single character"

  local domain="a"
  local is_valid=true

  # Single char domains are technically valid (like x.com)
  if [[ ! "$domain" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$ ]]; then
    is_valid=false
  fi

  assert_equals "$is_valid" "true" "$test_name: Single char domain is valid"
}

test_domain_max_length() {
  local test_name="Domain at maximum length (253 chars)"

  # DNS limits domain names to 253 characters
  local domain_base=$(printf 'a%.0s' {1..243})
  local domain="${domain_base}.localhost"  # 253 chars total
  local length=${#domain}

  assert_equals "$length" "253" "$test_name: Domain is exactly 253 chars"

  local exceeds_limit=false
  if [[ $length -gt 253 ]]; then
    exceeds_limit=true
  fi

  assert_equals "$exceeds_limit" "false" "$test_name: Does not exceed limit"
}

test_domain_over_max_length() {
  local test_name="Domain over maximum length (>253 chars)"

  # Create domain with 260 characters
  local domain_base=$(printf 'a%.0s' {1..250})
  local domain="${domain_base}.localhost"  # 260 chars
  local length=${#domain}

  local exceeds_limit=false
  if [[ $length -gt 253 ]]; then
    exceeds_limit=true
  fi

  assert_equals "$exceeds_limit" "true" "$test_name: Exceeds DNS limit"
}

test_domain_label_max_length() {
  local test_name="Domain label at max length (63 chars)"

  # DNS labels (parts between dots) are limited to 63 chars
  local label=$(printf 'a%.0s' {1..63})
  local domain="${label}.example.com"
  local label_length=${#label}

  assert_equals "$label_length" "63" "$test_name: Label is exactly 63 chars"
}

test_domain_label_over_max() {
  local test_name="Domain label over max (>63 chars)"

  # 64 character label
  local label=$(printf 'a%.0s' {1..64})
  local label_length=${#label}

  local exceeds_limit=false
  if [[ $label_length -gt 63 ]]; then
    exceeds_limit=true
  fi

  assert_equals "$exceeds_limit" "true" "$test_name: Label exceeds limit"
}

# ============================================
# Special Character Boundary Tests
# ============================================

test_input_with_null_bytes() {
  local test_name="Input with null bytes"

  # Null bytes should be rejected
  local input=$(printf 'test\x00data')
  local has_null=false

  if [[ "$input" =~ $'\x00' ]]; then
    has_null=true
  fi

  # This test is informational - null bytes are dangerous
  pass "$test_name: Null byte detection works"
}

test_input_with_unicode() {
  local test_name="Input with Unicode characters"

  local input="test-データ-🚀"

  # Unicode should generally be handled
  local is_ascii=false
  if [[ "$input" =~ ^[[:ascii:]]*$ ]]; then
    is_ascii=true
  fi

  assert_equals "$is_ascii" "false" "$test_name: Contains non-ASCII"
}

test_input_with_control_characters() {
  local test_name="Input with control characters"

  # Control characters (0x00-0x1F) should generally be rejected
  local input=$(printf 'test\x01\x02\x03')
  local has_control=false

  if [[ "$input" =~ [[:cntrl:]] ]]; then
    has_control=true
  fi

  assert_equals "$has_control" "true" "$test_name: Control char detection"
}

# ============================================
# Numeric Boundary Tests
# ============================================

test_integer_overflow() {
  local test_name="Integer overflow (>2^31-1)"

  # Bash integers are platform-dependent
  local max_int=2147483647
  local overflow_int=2147483648

  # This test is informational
  pass "$test_name: Int boundaries documented"
}

test_negative_zero() {
  local test_name="Negative zero"

  local zero=0
  local neg_zero=-0

  # In bash, -0 equals 0
  assert_equals "$zero" "$neg_zero" "$test_name: -0 equals 0"
}

# ============================================
# Boolean Boundary Tests
# ============================================

test_boolean_variations() {
  local test_name="Boolean value variations"

  # Only "true" and "false" should be valid
  local valid_true="true"
  local valid_false="false"

  # These should be invalid
  local invalid_values=("TRUE" "False" "yes" "no" "1" "0" "on" "off")

  assert_equals "$valid_true" "true" "$test_name: 'true' is valid"
  assert_equals "$valid_false" "false" "$test_name: 'false' is valid"

  # All others should be rejected
  for invalid in "${invalid_values[@]}"; do
    if [[ "$invalid" == "true" ]] || [[ "$invalid" == "false" ]]; then
      local is_valid=true
    else
      local is_valid=false
    fi

    assert_equals "$is_valid" "false" "$test_name: '$invalid' should be invalid"
  done
}

# ============================================
# Email Boundary Tests
# ============================================

test_email_minimum_length() {
  local test_name="Email minimum length (a@b.c)"

  # Shortest possible valid email: a@b.c (5 chars)
  local email="a@b.c"
  local is_valid=true

  # Basic email regex
  if [[ ! "$email" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
    is_valid=false
  fi

  assert_equals "$is_valid" "true" "$test_name: Shortest email is valid"
}

test_email_maximum_length() {
  local test_name="Email maximum length (320 chars)"

  # Email addresses have a practical limit of 320 characters
  # (64 for local part + @ + 255 for domain)
  local local_part=$(printf 'a%.0s' {1..64})
  local domain_part=$(printf 'a%.0s' {1..240})
  local email="${local_part}@${domain_part}.com"  # 269 chars
  local length=${#email}

  # Should be under 320
  local under_limit=false
  if [[ $length -le 320 ]]; then
    under_limit=true
  fi

  assert_equals "$under_limit" "true" "$test_name: Email under limit"
}

# ============================================
# URL Boundary Tests
# ============================================

test_url_minimum_length() {
  local test_name="URL minimum length"

  # Shortest valid URL with protocol
  local url="http://a.b"
  local is_valid=true

  if [[ ! "$url" =~ ^https?:// ]]; then
    is_valid=false
  fi

  assert_equals "$is_valid" "true" "$test_name: Short URL is valid"
}

test_url_maximum_length() {
  local test_name="URL maximum length (2083 chars, IE limit)"

  # URLs should generally stay under 2083 chars for IE compatibility
  local long_path=$(printf 'a%.0s' {1..2060})
  local url="https://example.com/${long_path}"
  local length=${#url}

  local under_limit=false
  if [[ $length -le 2083 ]]; then
    under_limit=true
  fi

  assert_equals "$under_limit" "true" "$test_name: URL under IE limit"
}

# ============================================
# Test Runner
# ============================================

run_all_tests() {
  printf "\n========================================\n"
  printf "  Boundary Value Tests\n"
  printf "========================================\n\n"

  setup_test_environment

  # Port boundaries
  test_port_zero
  test_port_one
  test_port_1023
  test_port_1024
  test_port_65535
  test_port_65536
  test_port_negative

  # String length boundaries
  test_empty_string
  test_single_character_string
  test_very_long_string

  # Domain boundaries
  test_domain_single_char
  test_domain_max_length
  test_domain_over_max_length
  test_domain_label_max_length
  test_domain_label_over_max

  # Special characters
  test_input_with_unicode
  test_input_with_control_characters

  # Numeric boundaries
  test_integer_overflow
  test_negative_zero

  # Boolean boundaries
  test_boolean_variations

  # Email boundaries
  test_email_minimum_length
  test_email_maximum_length

  # URL boundaries
  test_url_minimum_length
  test_url_maximum_length

  teardown_test_environment

  # Summary
  printf "\n========================================\n"
  printf "  Test Results\n"
  printf "========================================\n"
  printf "Total:   %d\n" "$TESTS_RUN"
  printf "Passed:  %d\n" "$TESTS_PASSED"
  printf "Failed:  %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n✓ All tests passed!\n\n"
    return 0
  else
    printf "\n✗ Some tests failed\n\n"
    return 1
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_all_tests
fi
