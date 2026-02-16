#!/usr/bin/env bash
set -euo pipefail

# run-edge-case-tests.sh - Run all edge case tests
# Comprehensive test suite for boundary values and edge cases

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
FAILED_FILES=()

# ============================================
# Helper Functions
# ============================================

print_header() {
  printf "\n"
  printf "${BLUE}========================================${NC}\n"
  printf "${BLUE}  %s${NC}\n" "$1"
  printf "${BLUE}========================================${NC}\n"
  printf "\n"
}

run_test_file() {
  local test_file="$1"
  local test_name=$(basename "$test_file" .sh)

  printf "${YELLOW}Running:${NC} %s\n" "$test_name"

  if bash "$test_file"; then
    printf "${GREEN}✓ %s passed${NC}\n\n" "$test_name"
    PASSED_TESTS=$((PASSED_TESTS + 1))
    return 0
  else
    printf "${RED}✗ %s failed${NC}\n\n" "$test_name"
    FAILED_TESTS=$((FAILED_TESTS + 1))
    FAILED_FILES+=("$test_name")
    return 1
  fi
}

# ============================================
# Main Test Execution
# ============================================

main() {
  print_header "nself Edge Case Tests"

  printf "Running comprehensive edge case tests...\n"
  printf "Location: %s\n" "$SCRIPT_DIR"
  printf "\n"

  # Boundary values
  print_header "Boundary Value Tests"
  run_test_file "$SCRIPT_DIR/test-boundary-values.sh"
  TOTAL_TESTS=$((TOTAL_TESTS + 1))

  # State transitions
  print_header "State Transition Tests"
  run_test_file "$SCRIPT_DIR/test-state-transitions.sh"
  TOTAL_TESTS=$((TOTAL_TESTS + 1))

  # Summary
  print_header "Test Summary"

  printf "Total test files:  %d\n" "$TOTAL_TESTS"
  printf "${GREEN}Passed:            %d${NC}\n" "$PASSED_TESTS"

  if [[ $FAILED_TESTS -gt 0 ]]; then
    printf "${RED}Failed:            %d${NC}\n" "$FAILED_TESTS"
    printf "\n"
    printf "${RED}Failed test files:${NC}\n"
    for failed_file in "${FAILED_FILES[@]}"; do
      printf "  - %s\n" "$failed_file"
    done
    printf "\n"
    exit 1
  else
    printf "Failed:            0\n"
    printf "\n"
    printf "${GREEN}========================================${NC}\n"
    printf "${GREEN}  ✓ All edge case tests passed!${NC}\n"
    printf "${GREEN}========================================${NC}\n"
    printf "\n"
    exit 0
  fi
}

# Run tests
main "$@"
