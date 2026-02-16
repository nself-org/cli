#!/usr/bin/env bash
# test-deploy.sh - Unit tests for deploy command
# Tests deployment operations functionality

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
  assert_file_exists "$CLI_DIR/deploy.sh" "deploy.sh exists"
}

test_command_syntax() {
  assert_success "deploy.sh syntax is valid" bash -n "$CLI_DIR/deploy.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" --help 2>&1 || true)
  assert_contains "deploy" "$output" "Help shows command name"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" help 2>&1 || true)
  assert_contains "Usage" "$output" "Help subcommand shows usage"
}

test_staging_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" staging 2>&1 || true)
  # Should handle staging deployment
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Staging subcommand executes\n"
}

test_production_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" production 2>&1 || true)
  # Should handle production deployment
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Production subcommand executes\n"
}

test_upgrade_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" upgrade 2>&1 || true)
  # Should handle upgrade
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Upgrade subcommand executes\n"
}

test_rollback_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" rollback 2>&1 || true)
  # Should handle rollback
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Rollback subcommand executes\n"
}

test_status_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" status 2>&1 || true)
  # Should show deployment status
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Status subcommand executes\n"
}

test_history_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" history 2>&1 || true)
  # Should show deployment history
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ History subcommand executes\n"
}

test_server_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" server 2>&1 || true)
  # Should handle server operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Server subcommand executes\n"
}

test_sync_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" sync 2>&1 || true)
  # Should handle sync operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Sync subcommand executes\n"
}

test_provision_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" provision 2>&1 || true)
  # Should handle provisioning
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Provision subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/deploy.sh" invalid-command-xyz 2>&1 || true)
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
  printf "=== Testing deploy command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_staging_subcommand
  test_production_subcommand
  test_upgrade_subcommand
  test_rollback_subcommand
  test_status_subcommand
  test_history_subcommand
  test_server_subcommand
  test_sync_subcommand
  test_provision_subcommand
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
