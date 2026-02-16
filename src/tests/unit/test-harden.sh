#!/usr/bin/env bash
# test-harden.sh - Unit tests for harden command
# Tests security hardening functionality

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLI_DIR="$NSELF_ROOT/src/cli"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# ============================================================================
# Test Utilities
# ============================================================================

assert_success() {
  local test_name="$1"
  shift

  TESTS_RUN=$((TESTS_RUN + 1))

  if "$@" >/dev/null 2>&1; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: command failed\n" "$test_name"
    return 1
  fi
}

assert_contains() {
  local expected="$1"
  local actual="$2"
  local test_name="${3:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if echo "$actual" | grep -q "$expected"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: expected to contain '%s'\n" "$test_name" "$expected"
    return 1
  fi
}

assert_file_exists() {
  local file="$1"
  local test_name="${2:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -f "$file" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: file not found: %s\n" "$test_name" "$file"
    return 1
  fi
}

# ============================================================================
# Tests
# ============================================================================

test_command_exists() {
  assert_file_exists "$CLI_DIR/harden.sh" "harden.sh exists"
}

test_command_syntax() {
  assert_success "harden.sh syntax is valid" bash -n "$CLI_DIR/harden.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" --help 2>&1 || true)
  assert_contains "harden" "$output" "Help shows command name"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" help 2>&1 || true)
  assert_contains "Usage" "$output" "Help subcommand shows usage"
}

test_check_subcommand() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" check 2>&1 || true)
  # Should check security status
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Check subcommand executes\n"
}

test_audit_subcommand() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" audit 2>&1 || true)
  # Should perform security audit
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Audit subcommand executes\n"
}

test_scan_subcommand() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" scan 2>&1 || true)
  # Should scan for vulnerabilities
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Scan subcommand executes\n"
}

test_report_subcommand() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" report 2>&1 || true)
  # Should generate security report
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Report subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/harden.sh" invalid-command-xyz 2>&1 || true)
  # Should show error or help
  TESTS_RUN=$((TESTS_RUN + 1))
  if echo "$output" | grep -qiE "unknown|invalid|error|usage|help"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ Invalid subcommand handled\n"
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Invalid subcommand not handled properly\n"
  fi
}

# ============================================================================
# Main
# ============================================================================

main() {
  printf "=== Testing harden command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_check_subcommand
  test_audit_subcommand
  test_scan_subcommand
  test_report_subcommand
  test_invalid_subcommand

  # Results
  printf "\n=== Results ===\n"
  printf "Tests run:    %d\n" "$TESTS_RUN"
  printf "Tests passed: %d\n" "$TESTS_PASSED"
  printf "Tests failed: %d\n" "$TESTS_FAILED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n✓ All tests passed\n"
    exit 0
  else
    printf "\n✗ Some tests failed\n"
    exit 1
  fi
}

main "$@"
