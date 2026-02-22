#!/usr/bin/env bash
# run-all-tests.sh - Run all unit tests and generate report
# Usage: bash src/tests/unit/run-all-tests.sh

set -euo pipefail

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Counters
TOTAL_FILES=0
PASSED_FILES=0
FAILED_FILES=0
TOTAL_TESTS=0
TOTAL_PASSED=0
TOTAL_FAILED=0

# Array to store failed tests
declare -a FAILED_TESTS

printf "${YELLOW}=== Running All Unit Tests ===${NC}\n\n"

# Run all tests
for test_file in "$SCRIPT_DIR"/test-*.sh; do
  test_name=$(basename "$test_file" .sh)

  # Skip run-all-tests.sh itself
  if [[ "$test_name" == "run-all-tests" ]]; then
    continue
  fi

  TOTAL_FILES=$((TOTAL_FILES + 1))

  printf "Running ${YELLOW}%s${NC}...\n" "$test_name"

  # Run test and capture output
  if output=$(bash "$test_file" 2>&1); then
    PASSED_FILES=$((PASSED_FILES + 1))
    printf "${GREEN}✓${NC} %s passed\n\n" "$test_name"

    # Extract test counts
    tests_run=$(echo "$output" | grep "Tests run:" | awk '{print $3}')
    tests_passed=$(echo "$output" | grep "Tests passed:" | awk '{print $3}')
    tests_failed=$(echo "$output" | grep "Tests failed:" | awk '{print $3}')

    TOTAL_TESTS=$((TOTAL_TESTS + tests_run))
    TOTAL_PASSED=$((TOTAL_PASSED + tests_passed))
    TOTAL_FAILED=$((TOTAL_FAILED + tests_failed))
  else
    FAILED_FILES=$((FAILED_FILES + 1))
    printf "${RED}✗${NC} %s failed\n" "$test_name"
    echo "$output"
    printf "\n"
    FAILED_TESTS+=("$test_name")

    # Extract test counts even on failure
    tests_run=$(echo "$output" | grep "Tests run:" | awk '{print $3}' || echo "0")
    tests_passed=$(echo "$output" | grep "Tests passed:" | awk '{print $3}' || echo "0")
    tests_failed=$(echo "$output" | grep "Tests failed:" | awk '{print $3}' || echo "0")

    TOTAL_TESTS=$((TOTAL_TESTS + tests_run))
    TOTAL_PASSED=$((TOTAL_PASSED + tests_passed))
    TOTAL_FAILED=$((TOTAL_FAILED + tests_failed))
  fi
done

# Print summary
printf "${YELLOW}=== Summary ===${NC}\n"
printf "Test Files:   %d total, ${GREEN}%d passed${NC}, ${RED}%d failed${NC}\n" \
  "$TOTAL_FILES" "$PASSED_FILES" "$FAILED_FILES"
printf "Test Cases:   %d total, ${GREEN}%d passed${NC}, ${RED}%d failed${NC}\n" \
  "$TOTAL_TESTS" "$TOTAL_PASSED" "$TOTAL_FAILED"

if [[ ${#FAILED_TESTS[@]} -gt 0 ]]; then
  printf "\n${RED}Failed test files:${NC}\n"
  for failed in "${FAILED_TESTS[@]}"; do
    printf "  - %s\n" "$failed"
  done
fi

# Exit with appropriate code
if [[ $FAILED_FILES -eq 0 ]]; then
  printf "\n${GREEN}✓ All tests passed!${NC}\n"
  exit 0
else
  printf "\n${RED}✗ Some tests failed${NC}\n"
  exit 1
fi
