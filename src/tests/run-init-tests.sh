#!/usr/bin/env bash
# run-all-tests.sh - Run all init tests locally
#
# This script runs all test suites for the init command
# Usage: ./run-all-tests.sh [--quick] [--verbose]

set -euo pipefail

# Test configuration
QUICK_MODE=false
VERBOSE=false
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INIT_DIR="$TEST_DIR/../lib/init"
CLI_DIR="$TEST_DIR/../cli"

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --quick)
      QUICK_MODE=true
      shift
      ;;
    --verbose)
      VERBOSE=true
      shift
      ;;
    --help)
      echo "Usage: $0 [--quick] [--verbose]"
      echo "  --quick    Skip integration tests"
      echo "  --verbose  Show detailed output"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Colors
RED="\033[31m"
GREEN="\033[32m"
YELLOW="\033[33m"
BLUE="\033[34m"
RESET="\033[0m"

# Helper functions
print_header() {
  printf "\n${BLUE}═══════════════════════════════════════════${RESET}\n"
  printf "${BLUE}  %s${RESET}\n" "$1"
  printf "${BLUE}═══════════════════════════════════════════${RESET}\n"
}

print_success() {
  printf "${GREEN}✓ %s${RESET}\n" "$1"
}

print_error() {
  printf "${RED}✗ %s${RESET}\n" "$1"
}

print_warning() {
  printf "${YELLOW}⚠ %s${RESET}\n" "$1"
}

# Track results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Run unit tests
run_unit_tests() {
  print_header "Running Unit Tests"

  if [[ -f "$TEST_DIR/unit/test-init.sh" ]]; then
    if bash "$TEST_DIR/unit/test-init.sh"; then
      print_success "Unit tests passed"
      PASSED_TESTS=$((PASSED_TESTS + 1))
    else
      print_error "Unit tests failed"
      FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
  else
    print_warning "Unit test file not found"
  fi
}

# Run integration tests
run_integration_tests() {
  print_header "Running Integration Tests"

  if [[ "$QUICK_MODE" == true ]]; then
    print_warning "Skipping integration tests (--quick mode)"
    return
  fi

  # Test basic init
  local temp_dir="/tmp/nself-test-$$"
  mkdir -p "$temp_dir"
  cd "$temp_dir"

  echo "Testing basic init..."
  if bash "$CLI_DIR/init.sh" --quiet; then
    if [[ -f .env ]] && [[ -f .env.example ]] && [[ -f .gitignore ]]; then
      print_success "Basic init test passed"
      PASSED_TESTS=$((PASSED_TESTS + 1))
    else
      print_error "Basic init test failed: missing files"
      FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
  else
    print_error "Basic init test failed: command error"
    FAILED_TESTS=$((FAILED_TESTS + 1))
  fi
  TOTAL_TESTS=$((TOTAL_TESTS + 1))

  # Cleanup
  cd /
  rm -rf "$temp_dir"
}

# Run shellcheck if available
run_shellcheck() {
  print_header "Running ShellCheck"

  if ! command -v shellcheck >/dev/null 2>&1; then
    print_warning "ShellCheck not installed, skipping"
    return
  fi

  local files_to_check=(
    "$CLI_DIR/init.sh"
    "$INIT_DIR/*.sh"
  )

  local has_errors=false
  for pattern in "${files_to_check[@]}"; do
    for file in $pattern; do
      if [[ -f "$file" ]]; then
        if [[ "$VERBOSE" == true ]]; then
          echo "Checking $file..."
        fi
        if ! shellcheck -S warning "$file" 2>/dev/null; then
          has_errors=true
        fi
      fi
    done
  done

  if [[ "$has_errors" == false ]]; then
    print_success "ShellCheck passed"
    PASSED_TESTS=$((PASSED_TESTS + 1))
  else
    print_error "ShellCheck found issues"
    FAILED_TESTS=$((FAILED_TESTS + 1))
  fi
  TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

# Check for Bash 4+ features
check_bash_compatibility() {
  print_header "Checking Bash 3.2 Compatibility"

  local has_issues=false
  local files_to_check="$INIT_DIR/*.sh $CLI_DIR/init.sh"

  # Check for declare -A (associative arrays)
  if grep -h "declare -A" $files_to_check 2>/dev/null | grep -v "#"; then
    print_error "Found associative arrays (Bash 4+)"
    has_issues=true
  fi

  # Check for ${var^^} or ${var,,}
  if grep -hE '\$\{[^}]*(\^\^|,,)[^}]*\}' $files_to_check 2>/dev/null | grep -v "#"; then
    print_error "Found case conversion (Bash 4+)"
    has_issues=true
  fi

  # Check for mapfile/readarray
  if grep -hE '\b(mapfile|readarray)\b' $files_to_check 2>/dev/null | grep -v "#"; then
    print_error "Found mapfile/readarray (Bash 4+)"
    has_issues=true
  fi

  if [[ "$has_issues" == false ]]; then
    print_success "Bash 3.2 compatible"
    PASSED_TESTS=$((PASSED_TESTS + 1))
  else
    FAILED_TESTS=$((FAILED_TESTS + 1))
  fi
  TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

# Check file permissions
check_permissions() {
  print_header "Checking File Permissions"

  local temp_dir="/tmp/nself-perm-test-$$"
  mkdir -p "$temp_dir"
  cd "$temp_dir"

  # Run init quietly
  bash "$CLI_DIR/init.sh" --quiet

  # Check .env permissions
  local env_perms
  env_perms=$(stat -c "%a" .env 2>/dev/null || stat -f "%OLp" .env 2>/dev/null)

  if [[ "$env_perms" == "600" ]]; then
    print_success ".env has correct permissions (600)"
    PASSED_TESTS=$((PASSED_TESTS + 1))
  else
    print_error ".env has wrong permissions: $env_perms (expected 600)"
    FAILED_TESTS=$((FAILED_TESTS + 1))
  fi
  TOTAL_TESTS=$((TOTAL_TESTS + 1))

  # Cleanup
  cd /
  rm -rf "$temp_dir"
}

# Main execution
main() {
  echo "╔══════════════════════════════════════════╗"
  echo "║     nself init Test Runner              ║"
  echo "╚══════════════════════════════════════════╝"

  # Run all test suites
  run_unit_tests
  run_integration_tests
  run_shellcheck
  check_bash_compatibility
  check_permissions

  # Print summary
  print_header "Test Summary"
  echo "Total tests: $TOTAL_TESTS"
  printf "Passed: ${GREEN}%s${RESET}\n" "$PASSED_TESTS"
  if [[ $FAILED_TESTS -gt 0 ]]; then
    printf "Failed: ${RED}%s${RESET}\n" "$FAILED_TESTS"
  else
    printf "Failed: %s\n" "$FAILED_TESTS"
  fi

  # Exit code
  if [[ $FAILED_TESTS -gt 0 ]]; then
    exit 1
  else
    printf "\n${GREEN}All tests passed!${RESET}\n"
    exit 0
  fi
}

# Run main
main
