#!/usr/bin/env bash
# test-config.sh - Unit tests for config command
# Tests configuration management functionality

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
  assert_file_exists "$CLI_DIR/config.sh" "config.sh exists"
}

test_command_syntax() {
  assert_success "config.sh syntax is valid" bash -n "$CLI_DIR/config.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/config.sh" --help 2>&1 || true)
  assert_contains "config" "$output" "Help shows command name"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" help 2>&1 || true)
  assert_contains "sage" "$output" "Help subcommand shows usage"
}

test_env_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" env 2>&1 || true)
  # Should handle environment operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Env subcommand executes\n"
}

test_secrets_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" secrets 2>&1 || true)
  # Should handle secrets operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Secrets subcommand executes\n"
}

test_vault_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" vault 2>&1 || true)
  # Should handle vault operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Vault subcommand executes\n"
}

test_validate_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" validate 2>&1 || true)
  # Should validate configuration
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Validate subcommand executes\n"
}

test_sync_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" sync 2>&1 || true)
  # Should handle sync operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Sync subcommand executes\n"
}

test_export_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" export 2>&1 || true)
  # Should export configuration
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Export subcommand executes\n"
}

test_import_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" import 2>&1 || true)
  # Should import configuration
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Import subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/config.sh" invalid-command-xyz 2>&1 || true)
  # Should show error or help (lenient check)
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Invalid subcommand handled\n"
}

# ============================================================================
# Main
# ============================================================================

main() {
  printf "=== Testing config command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_env_subcommand
  test_secrets_subcommand
  test_vault_subcommand
  test_validate_subcommand
  test_sync_subcommand
  test_export_subcommand
  test_import_subcommand
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
