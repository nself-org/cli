#!/usr/bin/env bash
# test-urls.sh - Tests for nself urls
# Part of nself v0.9.9

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_FILE="$SCRIPT_DIR/../../cli/urls.sh"

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

  if grep -qi "usage\|command\|help" <<< "$help_output"; then
    printf "✓ Help text present\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ Help text missing\n"
    return 1
  fi
}

# Test: Service URL listing documented
test_url_listing() {
  TESTS_RUN=$((TESTS_RUN + 1))
  local help_output
  help_output=$(bash "$SOURCE_FILE" --help 2>&1 || bash "$SOURCE_FILE" help 2>&1 || true)

  if grep -qi "service\|access\|endpoint" <<< "$help_output"; then
    printf "✓ Service URL listing documented\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
    return 0
  else
    printf "✗ URL listing functionality not clear\n"
    return 1
  fi
}

# Run all tests
printf "Testing nself urls...\n"
test_syntax
test_help
test_url_listing

printf "\n"
printf "Tests passed: %d/%d\n" "$TESTS_PASSED" "$TESTS_RUN"

if [ "$TESTS_PASSED" -eq "$TESTS_RUN" ]; then
  printf "✅ All tests passed\n"
  exit 0
else
  printf "❌ Some tests failed\n"
  exit 1
fi
