#!/usr/bin/env bash
# run-all-unit-tests.sh - Run all unit tests and generate coverage report
# Part of nself v0.9.9

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$SCRIPT_DIR/unit"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Track failed test files
FAILED_FILES=()

printf "${GREEN}Running all nself unit tests...${NC}\n\n"

# Find all test files
TEST_FILES=$(find "$TEST_DIR" -name "test-*.sh" | sort)
TOTAL_FILES=$(echo "$TEST_FILES" | wc -l | tr -d ' ')

printf "Found %d test files\n\n" "$TOTAL_FILES"

# Run each test
for test_file in $TEST_FILES; do
  test_name=$(basename "$test_file" .sh)
  printf "Running %s...\n" "$test_name"

  TOTAL_TESTS=$((TOTAL_TESTS + 1))

  # Run test and capture output
  if output=$(bash "$test_file" 2>&1); then
    PASSED_TESTS=$((PASSED_TESTS + 1))
    printf "${GREEN}✓${NC} %s passed\n\n" "$test_name"
  else
    FAILED_TESTS=$((FAILED_TESTS + 1))
    FAILED_FILES+=("$test_name")
    printf "${RED}✗${NC} %s failed\n" "$test_name"
    printf "%s\n\n" "$output"
  fi
done

# Generate coverage report
printf "\n${YELLOW}=== Test Coverage Report ===${NC}\n\n"

# Count total CLI commands
CLI_DIR="$SCRIPT_DIR/../cli"
TOTAL_COMMANDS=$(find "$CLI_DIR" -name "*.sh" | wc -l | tr -d ' ')

# Calculate coverage percentage
COVERAGE=$(awk "BEGIN {printf \"%.1f\", ($TOTAL_FILES / $TOTAL_COMMANDS) * 100}")

printf "Total CLI commands: %d\n" "$TOTAL_COMMANDS"
printf "Total test files: %d\n" "$TOTAL_FILES"
printf "Coverage: %s%%\n\n" "$COVERAGE"

printf "${YELLOW}=== Test Results ===${NC}\n\n"
printf "Tests run: %d\n" "$TOTAL_TESTS"
printf "${GREEN}Passed: %d${NC}\n" "$PASSED_TESTS"
printf "${RED}Failed: %d${NC}\n\n" "$FAILED_TESTS"

# Show failed tests if any
if [ "$FAILED_TESTS" -gt 0 ]; then
  printf "${RED}Failed tests:${NC}\n"
  for failed in "${FAILED_FILES[@]}"; do
    printf "  - %s\n" "$failed"
  done
  printf "\n"
fi

# Final status
if [ "$FAILED_TESTS" -eq 0 ]; then
  printf "${GREEN}✅ All tests passed!${NC}\n"
  exit 0
else
  printf "${RED}❌ Some tests failed${NC}\n"
  exit 1
fi
