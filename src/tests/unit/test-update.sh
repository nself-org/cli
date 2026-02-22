#!/usr/bin/env bash
# test-update.sh - Tests for nself update
# Part of nself v0.9.9

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_FILE="$SCRIPT_DIR/../../cli/update.sh"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0

# Test: Syntax validation
test_syntax() {
  TESTS_RUN=$((TESTS_RUN + 1))
  if bash -n "$SOURCE_FILE" 2>/dev/null; then
    printf "✓ Syntax valid\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ Syntax validation failed\n"
    return 1
  fi
}

# Test: Help text exists
test_help() {
  TESTS_RUN=$((TESTS_RUN + 1))
  local help_output
  # Try --help flag first, then help subcommand
  help_output=$(bash "$SOURCE_FILE" --help 2>&1 || bash "$SOURCE_FILE" help 2>&1 || true)

  if echo "$help_output" | grep -qi "usage\|command\|help"; then
    printf "✓ Help text present\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ Help text missing\n"
    return 1
  fi
}

# Test: Update functionality documented
test_update_functionality() {
  TESTS_RUN=$((TESTS_RUN + 1))
  local help_output
  help_output=$(bash "$SOURCE_FILE" --help 2>&1 || bash "$SOURCE_FILE" help 2>&1 || true)

  if echo "$help_output" | grep -qi "nself\|version\|upgrade"; then
    printf "✓ Update functionality documented\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ Update functionality not clear\n"
    return 1
  fi
}

# Run all tests
printf "Testing nself update...\n"
test_syntax
test_help
test_update_functionality

printf "\n"
printf "Tests passed: %d/%d\n" "$TESTS_PASSED" "$TESTS_RUN"

if [ "$TESTS_PASSED" -eq "$TESTS_RUN" ]; then
  printf "✅ All tests passed\n"
  exit 0
else
  printf "❌ Some tests failed\n"
  exit 1
fi
