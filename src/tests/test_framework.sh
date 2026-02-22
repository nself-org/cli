#!/usr/bin/env bash
set -euo pipefail
# test_framework.sh - Core testing utilities for nself
#
# This framework enables modular testing where each function can be
# tested in isolation. Once a function passes its tests, we can rely
# on it while developing other parts of the system.

# Guard against double-sourcing (readonly variables would fail on re-source)
[[ -n "${_TEST_FRAMEWORK_LOADED:-}" ]] && return 0
readonly _TEST_FRAMEWORK_LOADED=1

# Color definitions for test output
readonly COLOR_RESET='\033[0m'
readonly COLOR_RED='\033[0;31m'
readonly COLOR_GREEN='\033[0;32m'
readonly COLOR_YELLOW='\033[0;33m'
readonly COLOR_BLUE='\033[0;34m'
readonly COLOR_MAGENTA='\033[0;35m'

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Test context
CURRENT_TEST=""
TEST_OUTPUT=""

# ============================================
# Core Assertion Functions
# ============================================

# assert_equals - Check if two values are equal
assert_equals() {
  local actual="$1"
  local expected="$2"
  local message="${3:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$actual" == "$expected" ]]; then
    pass "$message"
    return 0
  else
    fail "$message: expected '$expected', got '$actual'"
    return 1
  fi
}

# assert_not_equals - Check if two values are not equal
assert_not_equals() {
  local actual="$1"
  local expected="$2"
  local message="${3:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$actual" != "$expected" ]]; then
    pass "$message"
    return 0
  else
    fail "$message: values should not be equal: '$actual'"
    return 1
  fi
}

# assert_true - Check if command returns true (0)
assert_true() {
  local command="$1"
  local message="${2:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if eval "$command" >/dev/null 2>&1; then
    pass "$message"
    return 0
  else
    fail "$message: command returned false"
    return 1
  fi
}

# assert_false - Check if command returns false (non-zero)
assert_false() {
  local command="$1"
  local message="${2:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if ! eval "$command" >/dev/null 2>&1; then
    pass "$message"
    return 0
  else
    fail "$message: command returned true"
    return 1
  fi
}

# assert_contains - Check if string contains substring
assert_contains() {
  local haystack="$1"
  local needle="$2"
  local message="${3:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$haystack" == *"$needle"* ]]; then
    pass "$message"
    return 0
  else
    fail "$message: '$needle' not found in output"
    return 1
  fi
}

# assert_not_contains - Check if string does not contain substring
assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local message="${3:-Assertion}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ "$haystack" != *"$needle"* ]]; then
    pass "$message"
    return 0
  else
    fail "$message: '$needle' should not be in output"
    return 1
  fi
}

# assert_file_exists - Check if file exists
assert_file_exists() {
  local file="$1"
  local message="${2:-File should exist}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -f "$file" ]]; then
    pass "$message: $file"
    return 0
  else
    fail "$message: $file not found"
    return 1
  fi
}

# assert_file_not_exists - Check if file does not exist
assert_file_not_exists() {
  local file="$1"
  local message="${2:-File should not exist}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ ! -f "$file" ]]; then
    pass "$message: $file"
    return 0
  else
    fail "$message: $file exists"
    return 1
  fi
}

# assert_directory_exists - Check if directory exists
assert_directory_exists() {
  local dir="$1"
  local message="${2:-Directory should exist}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -d "$dir" ]]; then
    pass "$message: $dir"
    return 0
  else
    fail "$message: $dir not found"
    return 1
  fi
}

# assert_exit_code - Check command exit code
assert_exit_code() {
  local expected="$1"
  local command="$2"
  local message="${3:-Exit code check}"

  TESTS_RUN=$((TESTS_RUN + 1))

  eval "$command" >/dev/null 2>&1
  local actual=$?

  if [[ $actual -eq $expected ]]; then
    pass "$message: exit code $expected"
    return 0
  else
    fail "$message: expected exit code $expected, got $actual"
    return 1
  fi
}

# ============================================
# Test Result Functions
# ============================================

# pass - Mark test as passed
pass() {
  local message="$1"
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} %s\n" "$message"
}

# fail - Mark test as failed
fail() {
  local message="$1"
  TESTS_FAILED=$((TESTS_FAILED + 1))
  printf "  ${COLOR_RED}✗${COLOR_RESET} %s\n" "$message"
  return 1
}

# skip - Mark test as skipped
skip() {
  local message="$1"
  TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
  printf "  ${COLOR_YELLOW}⊘${COLOR_RESET} %s (skipped)\n" "$message"
}

# ============================================
# Test Setup and Teardown
# ============================================

# setup_test - Run before each test
setup_test() {
  # Create temporary test directory
  TEST_DIR="/tmp/nself_test_$$"
  mkdir -p "$TEST_DIR"
  cd "$TEST_DIR" || exit 1

  # Set test environment
  export TEST_MODE=true
  export NSELF_CONFIG_DIR="$TEST_DIR/.config"
  mkdir -p "$NSELF_CONFIG_DIR"
}

# teardown_test - Run after each test
teardown_test() {
  # Clean up test directory
  if [[ -n "$TEST_DIR" ]] && [[ -d "$TEST_DIR" ]]; then
    rm -rf "$TEST_DIR"
  fi

  # Unset test environment
  unset TEST_MODE
  unset NSELF_CONFIG_DIR
}

# ============================================
# Test Runner
# ============================================

# run_tests - Main test runner
run_tests() {
  local test_file="${BASH_SOURCE[1]}"
  local test_name=$(basename "$test_file" .sh)

  # Find all test functions
  local test_functions=($(declare -F | awk '{print $3}' | grep '^test_'))

  if [[ ${#test_functions[@]} -eq 0 ]]; then
    echo "No tests found in $test_file"
    return 1
  fi

  printf "${COLOR_BLUE}Running %s${COLOR_RESET}\n" "$test_name"
  echo "Found ${#test_functions[@]} tests"
  echo

  # Run each test
  for test_func in "${test_functions[@]}"; do
    CURRENT_TEST="$test_func"
    echo "→ $test_func"

    # Setup test environment
    setup_test

    # Run test in subshell to isolate
    (
      set +e # Don't exit on error
      $test_func
    )
    local test_result=$?

    # Teardown test environment
    teardown_test

    # Track failed test
    if [[ $test_result -ne 0 ]] && [[ $test_result -ne 255 ]]; then
      ((TESTS_FAILED++))
    fi
  done

  # Show summary
  show_test_summary

  # Return failure if any tests failed
  [[ $TESTS_FAILED -eq 0 ]]
}

# show_test_summary - Display test results summary
show_test_summary() {
  echo
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "Test Summary:"
  printf "  ${COLOR_GREEN}Passed:${COLOR_RESET}  %s\n" "$TESTS_PASSED"

  if [[ $TESTS_FAILED -gt 0 ]]; then
    printf "  ${COLOR_RED}Failed:${COLOR_RESET}  %s\n" "$TESTS_FAILED"
  else
    printf "  Failed:  %s\n" "$TESTS_FAILED"
  fi

  if [[ $TESTS_SKIPPED -gt 0 ]]; then
    printf "  ${COLOR_YELLOW}Skipped:${COLOR_RESET} %s\n" "$TESTS_SKIPPED"
  fi

  echo "  Total:   $TESTS_RUN"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "${COLOR_GREEN}✓ All tests passed!${COLOR_RESET}\n"
  else
    printf "${COLOR_RED}✗ Some tests failed${COLOR_RESET}\n"
  fi
}

# ============================================
# Helper Functions
# ============================================

# require_command - Check if command exists
require_command() {
  local command="$1"

  if ! command -v "$command" >/dev/null 2>&1; then
    skip "$command not available"
    return 1
  fi
  return 0
}

# require_network - Check if network is available
require_network() {
  if ! ping -c 1 google.com >/dev/null 2>&1; then
    skip "Network not available"
    return 1
  fi
  return 0
}

# mock_function - Create a mock function for testing
mock_function() {
  local function_name="$1"
  local mock_output="$2"
  local mock_exit_code="${3:-0}"

  eval "
    $function_name() {
        echo '$mock_output'
        return $mock_exit_code
    }
    "
}

# capture_output - Capture command output for testing
capture_output() {
  local command="$1"
  TEST_OUTPUT=$(eval "$command" 2>&1)
  return $?
}

# ============================================
# Export Functions
# ============================================

# Export all assertion functions
export -f assert_equals
export -f assert_not_equals
export -f assert_true
export -f assert_false
export -f assert_contains
export -f assert_not_contains
export -f assert_file_exists
export -f assert_file_not_exists
export -f assert_directory_exists
export -f assert_exit_code

# Export test result functions
export -f pass
export -f fail
export -f skip

# Export helper functions
export -f require_command
export -f require_network
export -f mock_function
export -f capture_output

# Export setup/teardown
export -f setup_test
export -f teardown_test

# Export runner
export -f run_tests
export -f show_test_summary

# ============================================
# Additional Functions (v0.4.3 compatibility)
# ============================================

# run_test - Run a single test function with description
run_test() {
  local test_func="$1"
  local description="${2:-$test_func}"

  CURRENT_TEST="$test_func"

  # Run test in subshell
  (
    set +e
    $test_func
  )
  local result=$?

  if [[ $result -ne 0 ]]; then
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

# Alias functions for backward compatibility
pass_test() {
  local message="${1:-Test passed}"
  TESTS_PASSED=$((TESTS_PASSED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
  printf "  \033[32m✓\033[0m %s\n" "$message"
}

fail_test() {
  local message="${1:-Test failed}"
  TESTS_FAILED=$((TESTS_FAILED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
  printf "  \033[31m✗\033[0m %s\n" "$message"
  return 1
}

skip_test() {
  local message="${1:-Test skipped}"
  TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
  TESTS_RUN=$((TESTS_RUN + 1))
  printf "  \033[33m⊘\033[0m %s (skipped)\n" "$message"
}

print_test_summary() {
  printf "\n"
  printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
  printf "Test Summary:\n"
  printf "  \033[32mPassed:\033[0m  %d\n" "$TESTS_PASSED"

  if [[ $TESTS_FAILED -gt 0 ]]; then
    printf "  \033[31mFailed:\033[0m  %d\n" "$TESTS_FAILED"
  else
    printf "  Failed:  %d\n" "$TESTS_FAILED"
  fi

  if [[ $TESTS_SKIPPED -gt 0 ]]; then
    printf "  \033[33mSkipped:\033[0m %d\n" "$TESTS_SKIPPED"
  fi

  printf "  Total:   %d\n" "$TESTS_RUN"
  printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\033[32m✓ All tests passed!\033[0m\n"
    return 0
  else
    printf "\033[31m✗ Some tests failed\033[0m\n"
    return 1
  fi
}

# Additional file assertions
assert_file_contains() {
  local file="$1"
  local pattern="$2"
  local message="${3:-File should contain pattern}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -f "$file" ]] && grep -q "$pattern" "$file" 2>/dev/null; then
    pass "$message"
    return 0
  else
    fail "$message: '$pattern' not found in $file"
    return 1
  fi
}

assert_dir_exists() {
  assert_directory_exists "$@"
}

# print_test_header - Print a section header for tests
print_test_header() {
  local title="$1"
  printf "\n"
  printf "╔════════════════════════════════════════════════════════════╗\n"
  printf "║ %s\n" "$title"
  printf "╚════════════════════════════════════════════════════════════╝\n"
  printf "\n"
}

# ============================================
# BDD-Style Functions for Integration Tests
# ============================================

# describe - Print a description of the current test
describe() {
  local description="$1"
  printf "\n  \033[34m→\033[0m %s\n" "$description"
}

# start_suite - Initialize a named test suite
# Usage: start_suite "Suite Name"
start_suite() {
  local suite_name="${1:-Test Suite}"
  TESTS_RUN=0
  TESTS_PASSED=0
  TESTS_FAILED=0
  TESTS_SKIPPED=0
  printf "\n\033[1m=== %s ===\033[0m\n\n" "$suite_name"
}

# run - Run a command and capture output
run() {
  TEST_OUTPUT=$("$@" 2>&1) || true
  TEST_EXIT_CODE=$?
  return $TEST_EXIT_CODE
}

# run_expect_fail - Run a command expecting it to fail
run_expect_fail() {
  TEST_OUTPUT=$("$@" 2>&1) || true
  TEST_EXIT_CODE=$?
  if [[ $TEST_EXIT_CODE -eq 0 ]]; then
    fail "Command should have failed but succeeded: $*"
    return 1
  fi
  return 0
}

# assert_file_permissions - Check file permissions (cross-platform)
assert_file_permissions() {
  local file="$1"
  local expected="$2"
  local message="${3:-File should have correct permissions}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ ! -f "$file" ]]; then
    fail "$message: file '$file' does not exist"
    return 1
  fi

  local actual
  # Cross-platform permission check
  if stat --version 2>/dev/null | grep -q GNU; then
    actual=$(stat -c "%a" "$file" 2>/dev/null)
  else
    actual=$(stat -f "%OLp" "$file" 2>/dev/null)
  fi

  if [[ "$actual" == "$expected" ]]; then
    pass "$message"
    return 0
  else
    fail "$message: expected '$expected', got '$actual'"
    return 1
  fi
}

# assert_file_not_contains - Check if file does not contain pattern
assert_file_not_contains() {
  local file="$1"
  local pattern="$2"
  local message="${3:-File should not contain pattern}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ ! -f "$file" ]]; then
    fail "$message: file '$file' does not exist"
    return 1
  fi

  if ! grep -q "$pattern" "$file" 2>/dev/null; then
    pass "$message"
    return 0
  else
    fail "$message: '$pattern' found in $file"
    return 1
  fi
}

# Export new functions
export -f run_test
export -f pass_test
export -f fail_test
export -f skip_test
export -f print_test_summary
export -f print_test_header
export -f assert_file_contains
export -f assert_dir_exists

# Export BDD-style functions
export -f describe
export -f run
export -f run_expect_fail
export -f assert_file_permissions
export -f assert_file_not_contains
export -f start_suite
