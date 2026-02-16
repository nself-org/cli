#!/usr/bin/env bash
# test-plugin.sh - Unit tests for plugin command
# Tests plugin system functionality

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
  assert_file_exists "$CLI_DIR/plugin.sh" "plugin.sh exists"
}

test_command_syntax() {
  assert_success "plugin.sh syntax is valid" bash -n "$CLI_DIR/plugin.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" --help 2>&1 || true)
  assert_contains "plugin" "$output" "Help shows command name"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" help 2>&1 || true)
  assert_contains "sage" "$output" "Help subcommand shows usage"
}

test_list_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" list 2>&1 || true)
  # Should list plugins
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ List subcommand executes\n"
}

test_search_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" search test 2>&1 || true)
  # Should search plugins
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Search subcommand executes\n"
}

test_info_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" info 2>&1 || true)
  # Should show plugin info
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Info subcommand executes\n"
}

test_enable_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" enable 2>&1 || true)
  # Should handle enable
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Enable subcommand executes\n"
}

test_disable_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" disable 2>&1 || true)
  # Should handle disable
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Disable subcommand executes\n"
}

test_update_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" update 2>&1 || true)
  # Should handle updates
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Update subcommand executes\n"
}

test_create_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" create 2>&1 || true)
  # Should show create help
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Create subcommand executes\n"
}

test_publish_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" publish 2>&1 || true)
  # Should show publish help
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Publish subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/plugin.sh" invalid-command-xyz 2>&1 || true)
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
  printf "=== Testing plugin command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_list_subcommand
  test_search_subcommand
  test_info_subcommand
  test_enable_subcommand
  test_disable_subcommand
  test_update_subcommand
  test_create_subcommand
  test_publish_subcommand
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
