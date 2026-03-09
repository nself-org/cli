#!/usr/bin/env bash
# test-count.sh — count @test cases across all bats files and enforce minimums
# Usage: bash scripts/test-count.sh [--ci]
# Bash 3.2+ compatible

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$SCRIPT_DIR/../src"
CI_MODE="${1:-}"

# Minimums (update when new test files are added — see docs/test-coverage.md)
MIN_BATS_FILES=66
MIN_TEST_CASES=1700

# Count
file_count=$(find "$SRC_DIR/tests" -name "*.bats" | wc -l | tr -d ' ')
case_count=$(grep -r '^@test' "$SRC_DIR/tests" --include="*.bats" | wc -l | tr -d ' ')

printf "nself CLI test suite\n"
printf "  .bats files : %s (minimum: %s)\n" "$file_count" "$MIN_BATS_FILES"
printf "  @test cases : %s (minimum: %s)\n" "$case_count" "$MIN_TEST_CASES"

if [ "$CI_MODE" = "--ci" ]; then
  failed=0

  if [ "$file_count" -lt "$MIN_BATS_FILES" ]; then
    printf "FAIL: bats file count %s < minimum %s\n" "$file_count" "$MIN_BATS_FILES" >&2
    failed=1
  fi

  if [ "$case_count" -lt "$MIN_TEST_CASES" ]; then
    printf "FAIL: @test case count %s < minimum %s\n" "$case_count" "$MIN_TEST_CASES" >&2
    failed=1
  fi

  if [ "$failed" -eq 1 ]; then
    printf "\nTest count below baseline — see docs/test-coverage.md\n" >&2
    exit 1
  fi

  printf "PASS: test counts above baseline\n"
fi
