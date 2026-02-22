#!/usr/bin/env bash
# run-all-integration-tests.sh - Master integration test runner
#
# Executes all integration tests and provides comprehensive summary

set -euo pipefail

# Color definitions
readonly COLOR_RESET='\033[0m'
readonly COLOR_RED='\033[0;31m'
readonly COLOR_GREEN='\033[0;32m'
readonly COLOR_YELLOW='\033[0;33m'
readonly COLOR_BLUE='\033[0;34m'
readonly COLOR_BOLD='\033[1m'

# Test directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Test results
TOTAL_TESTS=0
TOTAL_PASSED=0
TOTAL_FAILED=0
TOTAL_SKIPPED=0
TOTAL_TIME=0

# Test execution tracking
declare -a TEST_FILES
declare -a TEST_RESULTS
declare -a TEST_TIMES
declare -a FAILED_TESTS

# ============================================================================
# Helper Functions
# ============================================================================

print_header() {
  local message="$1"
  printf "\n${COLOR_BOLD}${COLOR_BLUE}"
  printf "=================================================================\n"
  printf "%s\n" "$message"
  printf "=================================================================\n"
  printf "${COLOR_RESET}\n"
}

print_success() {
  printf "${COLOR_GREEN}✓${COLOR_RESET} %s\n" "$1"
}

print_failure() {
  printf "${COLOR_RED}✗${COLOR_RESET} %s\n" "$1"
}

print_warning() {
  printf "${COLOR_YELLOW}⚠${COLOR_RESET} %s\n" "$1"
}

print_info() {
  printf "${COLOR_BLUE}ℹ${COLOR_RESET} %s\n" "$1"
}

format_time() {
  local seconds="$1"
  printf "%02d:%02d" $((seconds / 60)) $((seconds % 60))
}

# ============================================================================
# Test Discovery
# ============================================================================

discover_tests() {
  print_header "Discovering Integration Tests"

  # Find all test files
  while IFS= read -r test_file; do
    if [[ -f "$test_file" ]] && [[ -x "$test_file" ]]; then
      TEST_FILES+=("$test_file")
      printf "  Found: %s\n" "$(basename "$test_file")"
    fi
  done < <(find "$TEST_DIR" -maxdepth 1 -name "test-*.sh" -type f | sort)

  printf "\nDiscovered %d integration tests\n" "${#TEST_FILES[@]}"
}

# ============================================================================
# Test Execution
# ============================================================================

run_test() {
  local test_file="$1"
  local test_name
  test_name=$(basename "$test_file" .sh)

  print_header "Running: $test_name"

  # Record start time
  local start_time
  start_time=$(date +%s)

  # Run test
  local output_file="/tmp/nself-integration-test-output-$$-$RANDOM.log"
  if bash "$test_file" > "$output_file" 2>&1; then
    local exit_code=0
  else
    local exit_code=$?
  fi

  # Record end time
  local end_time
  end_time=$(date +%s)
  local duration=$((end_time - start_time))

  # Parse test output
  local tests_run=0
  local tests_passed=0
  local tests_failed=0
  local tests_skipped=0

  if [[ -f "$output_file" ]]; then
    # Strip ANSI color codes before parsing to avoid matching color code numbers (e.g. [31m)
    local clean_output
    clean_output=$(sed 's/\x1b\[[0-9;]*m//g' "$output_file" 2>/dev/null || cat "$output_file")
    tests_run=$(printf '%s' "$clean_output" | grep "Total Tests:" 2>/dev/null | grep -o '[0-9]\+' | head -1 || echo "0")
    tests_passed=$(printf '%s' "$clean_output" | grep "Passed:" 2>/dev/null | grep -o '[0-9]\+' | head -1 || echo "0")
    tests_failed=$(printf '%s' "$clean_output" | grep "Failed:" 2>/dev/null | grep -o '[0-9]\+' | head -1 || echo "0")
    tests_skipped=$(printf '%s' "$clean_output" | grep "Skipped:" 2>/dev/null | grep -o '[0-9]\+' | head -1 || echo "0")
  fi

  # Update totals
  TOTAL_TESTS=$((TOTAL_TESTS + tests_run))
  TOTAL_PASSED=$((TOTAL_PASSED + tests_passed))
  TOTAL_FAILED=$((TOTAL_FAILED + tests_failed))
  TOTAL_SKIPPED=$((TOTAL_SKIPPED + tests_skipped))
  TOTAL_TIME=$((TOTAL_TIME + duration))

  # Store results
  TEST_RESULTS+=("$exit_code")
  TEST_TIMES+=("$duration")

  # Print result
  if [[ $exit_code -eq 0 ]]; then
    print_success "$test_name completed in $(format_time "$duration") (${tests_passed}/${tests_run} tests passed)"
  else
    print_failure "$test_name failed in $(format_time "$duration") (${tests_failed}/${tests_run} tests failed)"
    FAILED_TESTS+=("$test_name")

    # Show last 20 lines of output for failed tests
    printf "\n${COLOR_YELLOW}Last 20 lines of output:${COLOR_RESET}\n"
    tail -20 "$output_file"
    printf "\n"
  fi

  # Cleanup output file
  rm -f "$output_file"
}

run_all_tests() {
  print_header "Executing All Integration Tests"

  local test_count="${#TEST_FILES[@]}"
  local current=0

  for test_file in "${TEST_FILES[@]}"; do
    current=$((current + 1))
    printf "\n[%d/%d] " "$current" "$test_count"
    run_test "$test_file"
  done
}

# ============================================================================
# Results Summary
# ============================================================================

print_summary() {
  print_header "Integration Test Summary"

  # Overall statistics
  printf "\n${COLOR_BOLD}Overall Statistics:${COLOR_RESET}\n"
  printf "  Test Suites: %d\n" "${#TEST_FILES[@]}"
  printf "  Total Tests: %d\n" "$TOTAL_TESTS"
  printf "  Passed: ${COLOR_GREEN}%d${COLOR_RESET}\n" "$TOTAL_PASSED"
  printf "  Failed: ${COLOR_RED}%d${COLOR_RESET}\n" "$TOTAL_FAILED"
  printf "  Skipped: ${COLOR_YELLOW}%d${COLOR_RESET}\n" "$TOTAL_SKIPPED"
  printf "  Total Time: %s\n" "$(format_time "$TOTAL_TIME")"

  # Pass rate
  local pass_rate=0
  if [[ $TOTAL_TESTS -gt 0 ]]; then
    pass_rate=$(( (TOTAL_PASSED * 100) / TOTAL_TESTS ))
  fi

  printf "\n${COLOR_BOLD}Pass Rate: "
  if [[ $pass_rate -ge 90 ]]; then
    printf "${COLOR_GREEN}%d%%${COLOR_RESET}\n" "$pass_rate"
  elif [[ $pass_rate -ge 70 ]]; then
    printf "${COLOR_YELLOW}%d%%${COLOR_RESET}\n" "$pass_rate"
  else
    printf "${COLOR_RED}%d%%${COLOR_RESET}\n" "$pass_rate"
  fi

  # Individual test results
  printf "\n${COLOR_BOLD}Individual Test Results:${COLOR_RESET}\n"
  for i in "${!TEST_FILES[@]}"; do
    local test_name
    test_name=$(basename "${TEST_FILES[$i]}" .sh)
    local exit_code="${TEST_RESULTS[$i]}"
    local duration="${TEST_TIMES[$i]}"

    if [[ $exit_code -eq 0 ]]; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} %-40s %s\n" "$test_name" "$(format_time "$duration")"
    else
      printf "  ${COLOR_RED}✗${COLOR_RESET} %-40s %s\n" "$test_name" "$(format_time "$duration")"
    fi
  done

  # Failed tests detail
  if [[ ${#FAILED_TESTS[@]} -gt 0 ]]; then
    printf "\n${COLOR_BOLD}${COLOR_RED}Failed Tests:${COLOR_RESET}\n"
    for failed_test in "${FAILED_TESTS[@]}"; do
      printf "  - %s\n" "$failed_test"
    done
  fi

  # Recommendations
  printf "\n${COLOR_BOLD}Recommendations:${COLOR_RESET}\n"
  if [[ $TOTAL_FAILED -eq 0 ]]; then
    print_success "All integration tests passed! Ready for release."
  elif [[ $TOTAL_FAILED -le 2 ]]; then
    print_warning "Some tests failed. Review and fix before release."
  else
    print_failure "Multiple test failures. Significant issues need attention."
  fi

  # Next steps
  printf "\n${COLOR_BOLD}Next Steps:${COLOR_RESET}\n"
  if [[ $TOTAL_FAILED -gt 0 ]]; then
    printf "  1. Review failed test output above\n"
    printf "  2. Run individual tests with: bash %s/<test-name>.sh\n" "$TEST_DIR"
    printf "  3. Fix issues and re-run: bash %s\n" "$0"
  else
    printf "  1. Review any warnings or skipped tests\n"
    printf "  2. Run manual tests if needed\n"
    printf "  3. Proceed with deployment\n"
  fi

  printf "\n"
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
  # Parse arguments
  local run_specific=""
  local verbose=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --test)
        run_specific="$2"
        shift 2
        ;;
      --verbose|-v)
        verbose=true
        shift
        ;;
      --help|-h)
        cat <<EOF
Usage: $0 [OPTIONS]

Run all integration tests or a specific test suite.

Options:
  --test <name>     Run specific test (e.g., --test full-deployment)
  --verbose, -v     Show verbose output
  --help, -h        Show this help message

Examples:
  $0                                    # Run all tests
  $0 --test full-deployment             # Run specific test
  $0 --verbose                          # Run with verbose output

EOF
        exit 0
        ;;
      *)
        print_failure "Unknown option: $1"
        exit 1
        ;;
    esac
  done

  # Print banner
  print_header "nself Integration Test Suite v0.9.8"
  printf "Started at: %s\n" "$(date '+%Y-%m-%d %H:%M:%S')"

  # Discover tests
  discover_tests

  if [[ -z "$run_specific" ]]; then
    # Run all tests
    run_all_tests
  else
    # Run specific test
    local test_file="$TEST_DIR/test-${run_specific}.sh"
    if [[ ! -f "$test_file" ]]; then
      print_failure "Test not found: $run_specific"
      exit 1
    fi
    TEST_FILES=("$test_file")
    run_test "$test_file"
  fi

  # Print summary
  print_summary

  # Exit with appropriate code
  if [[ $TOTAL_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

# Run main if executed directly
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
