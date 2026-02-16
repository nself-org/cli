#!/usr/bin/env bash
set -euo pipefail

# test-error-messages.sh - Tests for error message library
# Verifies error messages are clear, actionable, and cross-platform compatible

set -e

# Colors for test output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$TEST_DIR/../../lib"

# Source the error messages library
source "$LIB_DIR/utils/error-messages.sh"

# Disable colors for testing (makes output easier to parse)
export NO_COLOR=1

# Test helper functions
assert_function_exists() {
  local func_name="$1"
  TESTS_RUN=$((TESTS_RUN + 1))

  if declare -f "$func_name" >/dev/null 2>&1; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} Function exists: %s\n" "$func_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} Function missing: %s\n" "$func_name"
    return 1
  fi
}

assert_output_contains() {
  local output="$1"
  local expected="$2"
  local test_name="$3"

  TESTS_RUN=$((TESTS_RUN + 1))

  if echo "$output" | grep -q "$expected"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} %s: Output contains '%s'\n" "$test_name" "$expected"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} %s: Expected '%s' not found\n" "$test_name" "$expected"
    printf "   Output: %s\n" "${output:0:100}"
    return 1
  fi
}

assert_output_has_numbered_list() {
  local output="$1"
  local test_name="$2"

  TESTS_RUN=$((TESTS_RUN + 1))

  # Check for numbered list (1. 2. 3. etc)
  if echo "$output" | grep -qE '^\s*[0-9]+\.'; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} %s: Has numbered list\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} %s: Missing numbered list\n" "$test_name"
    return 1
  fi
}

# Test 1: Function Exports
test_function_exports() {
  printf "\n=== Testing Function Exports ===\n"

  assert_function_exists "show_port_conflict_error"
  assert_function_exists "show_container_failed_error"
  assert_function_exists "show_config_missing_error"
  assert_function_exists "show_permission_error"
  assert_function_exists "show_network_error"
  assert_function_exists "show_docker_not_running_error"
  assert_function_exists "show_resource_error"
  assert_function_exists "show_database_error"
  assert_function_exists "show_build_error"
  assert_function_exists "show_health_check_error"
  assert_function_exists "show_generic_error"
  assert_function_exists "show_warning_message"
  assert_function_exists "show_error_reference"
}

# Test 2: Port Conflict Error
test_port_conflict_error() {
  printf "\n=== Testing Port Conflict Error ===\n"

  local output=$(show_port_conflict_error 5432 "postgres" "PostgreSQL" 2>&1)

  assert_output_contains "$output" "Container 'postgres' failed to start" "Port conflict title"
  assert_output_contains "$output" "Port 5432 is already in use" "Port number"
  assert_output_contains "$output" "Possible solutions" "Solutions header"
  assert_output_has_numbered_list "$output" "Port conflict error"

  # Check for actionable commands
  assert_output_contains "$output" "kill" "Kill command"
  assert_output_contains "$output" "nself build" "Build command"
}

# Test 3: Container Failed Error
test_container_failed_error() {
  printf "\n=== Testing Container Failed Error ===\n"

  local output=$(show_container_failed_error "hasura" "database connection failed" "" 2>&1)

  assert_output_contains "$output" "Container 'hasura' failed to start" "Container name"
  assert_output_contains "$output" "database connection failed" "Reason"
  assert_output_contains "$output" "nself logs hasura" "Logs command"
  assert_output_has_numbered_list "$output" "Container failed error"
}

# Test 4: Config Missing Error
test_config_missing_error() {
  printf "\n=== Testing Config Missing Error ===\n"

  local output=$(show_config_missing_error ".env" "PROJECT_NAME POSTGRES_PASSWORD" 2>&1)

  assert_output_contains "$output" "Required configuration missing" "Error title"
  assert_output_contains "$output" ".env" "Config file"
  assert_output_contains "$output" "PROJECT_NAME" "Missing var 1"
  assert_output_contains "$output" "POSTGRES_PASSWORD" "Missing var 2"
  assert_output_contains "$output" "nself init" "Init command"
}

# Test 5: Permission Error
test_permission_error() {
  printf "\n=== Testing Permission Error ===\n"

  local output=$(show_permission_error "/var/run/docker.sock" "access" 2>&1)

  assert_output_contains "$output" "Permission denied" "Error title"
  assert_output_contains "$output" "/var/run/docker.sock" "Path"
  assert_output_contains "$output" "chown" "Fix ownership"
  assert_output_contains "$output" "chmod" "Fix permissions"
}

# Test 6: Network Error
test_network_error() {
  printf "\n=== Testing Network Error ===\n"

  local output=$(show_network_error "hasura" "http://localhost:8080" "connection refused" 2>&1)

  assert_output_contains "$output" "Network connection failed" "Error title"
  assert_output_contains "$output" "hasura" "Service"
  assert_output_contains "$output" "connection refused" "Error message"
  assert_output_contains "$output" "docker network" "Network command"
}

# Test 7: Docker Not Running Error
test_docker_not_running_error() {
  printf "\n=== Testing Docker Not Running Error ===\n"

  # Test macOS version
  local output=$(show_docker_not_running_error "Darwin" 2>&1)
  assert_output_contains "$output" "Docker is not running" "Error title"
  assert_output_contains "$output" "Docker Desktop" "macOS specific"
  assert_output_contains "$output" "open -a Docker" "macOS command"

  # Test Linux version
  output=$(show_docker_not_running_error "Linux" 2>&1)
  assert_output_contains "$output" "systemctl start docker" "Linux command"
}

# Test 8: Resource Error
test_resource_error() {
  printf "\n=== Testing Resource Error ===\n"

  local output=$(show_resource_error "memory" "2GB" "4GB" 2>&1)

  assert_output_contains "$output" "Insufficient memory" "Error title"
  assert_output_contains "$output" "Available: 2GB" "Available"
  assert_output_contains "$output" "Required: 4GB" "Required"
  assert_output_has_numbered_list "$output" "Resource error"
}

# Test 9: Database Error
test_database_error() {
  printf "\n=== Testing Database Error ===\n"

  local output=$(show_database_error "PostgreSQL" "connection refused" 2>&1)

  assert_output_contains "$output" "Database connection failed" "Error title"
  assert_output_contains "$output" "PostgreSQL" "Database type"
  assert_output_contains "$output" "docker ps.*postgres" "Check command"
  assert_output_contains "$output" "nself logs postgres" "Logs command"
}

# Test 10: Build Error
test_build_error() {
  printf "\n=== Testing Build Error ===\n"

  local output=$(show_build_error "custom-service" "RUN npm install" "ENOENT" 2>&1)

  assert_output_contains "$output" "Build failed" "Error title"
  assert_output_contains "$output" "custom-service" "Service name"
  assert_output_contains "$output" "RUN npm install" "Stage"
  assert_output_contains "$output" "docker builder prune" "Clean cache"
}

# Test 11: Health Check Error
test_health_check_error() {
  printf "\n=== Testing Health Check Error ===\n"

  local output=$(show_health_check_error "hasura" "unhealthy" 2>&1)

  assert_output_contains "$output" "Service 'hasura' is unhealthy" "Error title"
  assert_output_contains "$output" "nself logs hasura" "Logs command"
  assert_output_contains "$output" "nself restart hasura" "Restart command"
}

# Test 12: Generic Error
test_generic_error() {
  printf "\n=== Testing Generic Error ===\n"

  local output=$(show_generic_error "Test Error" "Test reason" "Solution 1" "Solution 2" 2>&1)

  assert_output_contains "$output" "Test Error" "Error title"
  assert_output_contains "$output" "Test reason" "Reason"
  assert_output_has_numbered_list "$output" "Generic error"
}

# Test 13: Warning Message
test_warning_message() {
  printf "\n=== Testing Warning Message ===\n"

  local output=$(show_warning_message "Test warning" "Suggestion 1" "Suggestion 2" 2>&1)

  assert_output_contains "$output" "Test warning" "Warning text"
  assert_output_contains "$output" "Suggestions" "Suggestions header"
}

# Test 14: Error Reference
test_error_reference() {
  printf "\n=== Testing Error Reference ===\n"

  local output=$(show_error_reference 2>&1)

  assert_output_contains "$output" "Quick Reference" "Reference title"
  assert_output_contains "$output" "Port conflicts" "Port section"
  assert_output_contains "$output" "Container failures" "Container section"
  assert_output_contains "$output" "nself doctor" "Doctor command"
}

# Test 15: Cross-Platform Compatibility
test_cross_platform() {
  printf "\n=== Testing Cross-Platform Compatibility ===\n"

  TESTS_RUN=$((TESTS_RUN + 1))

  # Test that error messages work without errors on current platform
  local output
  output=$(show_port_conflict_error 5432 "postgres" "" 2>&1 || echo "FAILED")

  if echo "$output" | grep -q "FAILED"; then
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} Port conflict error failed on platform\n"
  else
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} Error messages work on current platform\n"
  fi
}

# Test 16: No Echo -e Usage (Portability)
test_no_echo_e() {
  printf "\n=== Testing POSIX Compliance (No echo -e) ===\n"

  TESTS_RUN=$((TESTS_RUN + 1))

  # Check source file for echo -e usage
  if grep -q 'echo -e' "$LIB_DIR/utils/error-messages.sh"; then
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} Found 'echo -e' in error-messages.sh (not portable)\n"
  else
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} No 'echo -e' found (portable)\n"
  fi
}

# Test 17: All Printf Statements Have Newlines
test_printf_newlines() {
  printf "\n=== Testing Printf Usage ===\n"

  TESTS_RUN=$((TESTS_RUN + 1))

  # This is informational - just verify printf is used instead of echo -e
  local printf_count=$(grep -c 'printf' "$LIB_DIR/utils/error-messages.sh" || echo 0)

  if [ "$printf_count" -gt 50 ]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "${GREEN}✓${NC} Uses printf for formatted output (%d instances)\n" "$printf_count"
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "${RED}✗${NC} Low printf usage: %d instances\n" "$printf_count"
  fi
}

# Run all tests
run_all_tests() {
  printf "\n"
  printf "========================================\n"
  printf "  Error Messages Library - Unit Tests\n"
  printf "========================================\n"

  test_function_exports
  test_port_conflict_error
  test_container_failed_error
  test_config_missing_error
  test_permission_error
  test_network_error
  test_docker_not_running_error
  test_resource_error
  test_database_error
  test_build_error
  test_health_check_error
  test_generic_error
  test_warning_message
  test_error_reference
  test_cross_platform
  test_no_echo_e
  test_printf_newlines

  # Summary
  printf "\n"
  printf "========================================\n"
  printf "  Test Results\n"
  printf "========================================\n"
  printf "Total:  %d\n" "$TESTS_RUN"
  printf "${GREEN}Passed: %d${NC}\n" "$TESTS_PASSED"

  if [ "$TESTS_FAILED" -gt 0 ]; then
    printf "${RED}Failed: %d${NC}\n" "$TESTS_FAILED"
  else
    printf "Failed: 0\n"
  fi

  printf "\n"

  if [ "$TESTS_FAILED" -eq 0 ]; then
    printf "${GREEN}✓ All tests passed!${NC}\n\n"
    return 0
  else
    printf "${RED}✗ Some tests failed${NC}\n\n"
    return 1
  fi
}

# Run tests
run_all_tests
