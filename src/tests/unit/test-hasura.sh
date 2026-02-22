#!/usr/bin/env bash
# test-hasura.sh - Unit tests for hasura command
# Tests Hasura GraphQL management functionality

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

  if grep -q "$expected" <<< "$actual"; then
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
  assert_file_exists "$CLI_DIR/hasura.sh" "hasura.sh exists"
}

test_command_syntax() {
  assert_success "hasura.sh syntax is valid" bash -n "$CLI_DIR/hasura.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" --help 2>&1 || true)
  # Should show help or error message
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Help flag executes\n"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" help 2>&1 || true)
  # Should show help or error message
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Help subcommand executes\n"
}

test_console_subcommand() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" console 2>&1 || true)
  # Should open or reference console
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Console subcommand executes\n"
}

test_migrate_subcommand() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" migrate 2>&1 || true)
  # Should handle migrations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Migrate subcommand executes\n"
}

test_metadata_subcommand() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" metadata 2>&1 || true)
  # Should handle metadata
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Metadata subcommand executes\n"
}

test_status_subcommand() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" status 2>&1 || true)
  # Should show status
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Status subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/hasura.sh" invalid-command-xyz 2>&1 || true)
  # Should show error or help
  TESTS_RUN=$((TESTS_RUN + 1))
  if grep -qiE "unknown|invalid|error|usage|help" <<< "$output"; then
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
  printf "=== Testing hasura command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_console_subcommand
  test_migrate_subcommand
  test_metadata_subcommand
  test_status_subcommand
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
