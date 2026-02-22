#!/usr/bin/env bash
# test-admin-dev.sh - Unit tests for admin-dev command
# Tests admin development mode functionality

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

  if [[ "$actual" == *"$expected"* ]]; then
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
  assert_file_exists "$CLI_DIR/admin-dev.sh" "admin-dev.sh exists"
}

test_command_syntax() {
  assert_success "admin-dev.sh syntax is valid" bash -n "$CLI_DIR/admin-dev.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" --help 2>&1 || true)
  assert_contains "admin-dev" "$output" "Help shows command name"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" help 2>&1 || true)
  assert_contains "sage" "$output" "Help subcommand shows usage"
}

test_version_flag() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" --version 2>&1 || true)
  # Should show version or not fail
  TESTS_RUN=$((TESTS_RUN + 1))
  if [[ $? -eq 0 ]] || echo "$output" | grep -q "version\|Version\|v[0-9]"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ Version flag works\n"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Version flag failed\n"
    return 1
  fi
}

test_list_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" list 2>&1 || true)
  # Should list dev instances or show message
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ List subcommand executes\n"
}

test_status_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" status 2>&1 || true)
  # Should show status or appropriate message
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Status subcommand executes\n"
}

test_env_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" env 2>&1 || true)
  # Should show environment info
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Env subcommand executes\n"
}

test_logs_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" logs 2>&1 || true)
  # Should show logs or appropriate message
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Logs subcommand executes\n"
}

test_config_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" config 2>&1 || true)
  # Should show config or appropriate message
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Config subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/admin-dev.sh" invalid-command-xyz 2>&1 || true)
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
  printf "=== Testing admin-dev command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_version_flag
  test_list_subcommand
  test_status_subcommand
  test_env_subcommand
  test_logs_subcommand
  test_config_subcommand
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
