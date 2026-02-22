#!/usr/bin/env bash
# test-vault.sh - Tests for nself vault (deprecated wrapper)
# Part of nself v0.9.9

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_FILE="$SCRIPT_DIR/../../cli/vault.sh"

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

# Test: Deprecation warning present
test_deprecation_warning() {
  TESTS_RUN=$((TESTS_RUN + 1))
  local output
  output=$(bash "$SOURCE_FILE" 2>&1 || true)

  if grep -qi "deprecat" <<< "$output"; then
    printf "✓ Deprecation warning present\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ Deprecation warning missing\n"
    return 1
  fi
}

# Test: Mentions new command
test_new_command_mentioned() {
  TESTS_RUN=$((TESTS_RUN + 1))
  local output
  output=$(bash "$SOURCE_FILE" 2>&1 || true)

  if grep -q "config vault" <<< "$output"; then
    printf "✓ Mentions 'nself config vault'\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ New command not mentioned\n"
    return 1
  fi
}

# Run all tests
printf "Testing nself vault (deprecated)...\n"
test_syntax
test_deprecation_warning
test_new_command_mentioned

printf "\n"
printf "Tests passed: %d/%d\n" "$TESTS_PASSED" "$TESTS_RUN"

if [ "$TESTS_PASSED" -eq "$TESTS_RUN" ]; then
  printf "✅ All tests passed\n"
  exit 0
else
  printf "❌ Some tests failed\n"
  exit 1
fi
