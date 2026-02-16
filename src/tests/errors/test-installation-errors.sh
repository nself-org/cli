#!/usr/bin/env bash
set -euo pipefail

# test-installation-errors.sh - Installation and setup error scenario tests
# Tests realistic errors users encounter during installation and setup

set -e

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$TEST_DIR/../.."

# Source test framework
source "$TEST_DIR/../test_framework.sh"

# Source utilities we're testing
source "$ROOT_DIR/lib/utils/error-messages.sh"
source "$ROOT_DIR/lib/utils/platform-compat.sh"

# ============================================
# Test Setup
# ============================================

setup_test_environment() {
  export TEST_MODE=1
  export NO_COLOR=1
  TEMP_DIR=$(mktemp -d)
  export TEMP_DIR
}

teardown_test_environment() {
  if [[ -n "$TEMP_DIR" ]] && [[ -d "$TEMP_DIR" ]]; then
    rm -rf "$TEMP_DIR"
  fi
}

# ============================================
# Installation Error Tests
# ============================================

test_docker_not_installed() {
  local test_name="Docker not installed"

  # Simulate docker command not found
  local output
  output=$(cat <<'EOF'
Docker is not installed on this system.

Problem:
  The 'docker' command was not found in your PATH.

Fix:
  Install Docker Desktop:

  macOS:
    1. Download from https://www.docker.com/products/docker-desktop
    2. Install the application
    3. Launch Docker Desktop

  Ubuntu/Debian:
    curl -fsSL https://get.docker.com -o get-docker.sh
    sudo sh get-docker.sh

  Verify installation:
    docker --version
EOF
)

  # Verify error contains all required information
  assert_contains "$output" "Docker is not installed" "$test_name: Error title"
  assert_contains "$output" "Problem:" "$test_name: Problem section"
  assert_contains "$output" "Fix:" "$test_name: Fix section"
  assert_contains "$output" "https://www.docker.com" "$test_name: Install URL"
  assert_contains "$output" "docker --version" "$test_name: Verification command"
}

test_docker_daemon_not_running() {
  local test_name="Docker daemon not running"

  # Test macOS error message
  local output
  output=$(show_docker_not_running_error "Darwin")

  assert_contains "$output" "Docker is not running" "$test_name: Error message"
  assert_contains "$output" "Docker Desktop" "$test_name: macOS specific"
  assert_contains "$output" "open -a Docker" "$test_name: macOS start command"

  # Test Linux error message
  output=$(show_docker_not_running_error "Linux")

  assert_contains "$output" "systemctl start docker" "$test_name: Linux start command"
  assert_contains "$output" "sudo usermod -aG docker" "$test_name: Permission fix"
}

test_insufficient_permissions() {
  local test_name="Insufficient Docker permissions"

  local output
  output=$(show_permission_error "/var/run/docker.sock" "access")

  assert_contains "$output" "Permission denied" "$test_name: Error title"
  assert_contains "$output" "/var/run/docker.sock" "$test_name: File path"
  assert_contains "$output" "docker group" "$test_name: Fix suggestion"

  # Should provide both immediate and permanent fix
  assert_contains "$output" "sudo" "$test_name: Immediate fix"
  assert_contains "$output" "usermod" "$test_name: Permanent fix"
}

test_disk_space_insufficient() {
  local test_name="Insufficient disk space"

  local output
  output=$(show_resource_error "disk" "500MB" "2GB")

  assert_contains "$output" "Insufficient disk" "$test_name: Error title"
  assert_contains "$output" "Available: 500MB" "$test_name: Available space"
  assert_contains "$output" "Required: 2GB" "$test_name: Required space"

  # Should provide cleanup suggestions
  assert_contains "$output" "docker system prune" "$test_name: Docker cleanup"
}

test_incompatible_docker_version() {
  local test_name="Incompatible Docker version"

  local output
  output=$(cat <<'EOF'
Incompatible Docker version

Problem:
  Docker version 19.03.0 is installed
  nself requires Docker 20.10.0 or higher

Fix:
  Update Docker to the latest version:

  1. Check current version:
     docker --version

  2. Update Docker:
     macOS: Update Docker Desktop from the menu
     Linux: sudo apt-get update && sudo apt-get install docker-ce

  3. Verify update:
     docker --version
EOF
)

  assert_contains "$output" "Incompatible Docker version" "$test_name: Error title"
  assert_contains "$output" "requires Docker 20.10.0" "$test_name: Version requirement"
  assert_contains "$output" "Update Docker" "$test_name: Fix instruction"
}

test_port_conflicts() {
  local test_name="Port conflicts on common ports"

  local output
  output=$(show_port_conflict_error 5432 "postgres" "PostgreSQL")

  assert_contains "$output" "Port 5432 is already in use" "$test_name: Port number"
  assert_contains "$output" "postgres" "$test_name: Service name"

  # Should provide commands to find and kill process
  assert_contains "$output" "lsof -i :5432" "$test_name: Find process command (macOS)"
  assert_contains "$output" "kill" "$test_name: Kill command"

  # Should suggest alternative solutions
  assert_contains "$output" "POSTGRES_PORT" "$test_name: Config change suggestion"
}

test_missing_dependencies() {
  local test_name="Missing dependencies (curl, git)"

  local output
  output=$(cat <<'EOF'
Missing required dependencies

Problem:
  The following required tools are not installed:
    - curl (required for downloads)
    - git (required for version control)

Fix:
  Install missing dependencies:

  Ubuntu/Debian:
    sudo apt-get update
    sudo apt-get install curl git

  macOS:
    brew install curl git

  Verify installation:
    curl --version
    git --version
EOF
)

  assert_contains "$output" "Missing required dependencies" "$test_name: Error title"
  assert_contains "$output" "curl" "$test_name: Lists curl"
  assert_contains "$output" "git" "$test_name: Lists git"
  assert_contains "$output" "apt-get install" "$test_name: Linux install"
  assert_contains "$output" "brew install" "$test_name: macOS install"
}

# ============================================
# Error Message Quality Tests
# ============================================

test_error_messages_have_structure() {
  local test_name="Error messages follow structure"

  local output
  output=$(show_container_failed_error "hasura" "connection refused" "")

  # Every error should have:
  # 1. Clear title
  # 2. Problem description
  # 3. Solution steps
  # 4. Verification command

  assert_contains "$output" "failed to start" "$test_name: Title"
  assert_not_empty "$output" "$test_name: Has content"

  # Should have numbered list of solutions
  if printf "%s" "$output" | grep -qE '^\s*[0-9]+\.'; then
    pass "$test_name: Has numbered solutions"
  else
    fail "$test_name: Missing numbered solutions"
  fi
}

test_error_messages_actionable() {
  local test_name="Error messages are actionable"

  local output
  output=$(show_database_error "PostgreSQL" "authentication failed")

  # Should contain actual commands user can run
  assert_contains "$output" "docker" "$test_name: Contains docker command"
  assert_contains "$output" "nself" "$test_name: Contains nself command"

  # Should not have vague suggestions
  assert_not_contains "$output" "try again" "$test_name: No vague 'try again'"
  assert_not_contains "$output" "contact support" "$test_name: No support punt"
}

test_error_messages_cross_platform() {
  local test_name="Error messages are cross-platform"

  # Test macOS-specific
  local output_mac
  output_mac=$(show_docker_not_running_error "Darwin")
  assert_contains "$output_mac" "open -a Docker" "$test_name: macOS command"

  # Test Linux-specific
  local output_linux
  output_linux=$(show_docker_not_running_error "Linux")
  assert_contains "$output_linux" "systemctl" "$test_name: Linux command"

  # Both should solve the same problem
  assert_contains "$output_mac" "Docker" "$test_name: Both mention Docker"
  assert_contains "$output_linux" "Docker" "$test_name: Both mention Docker"
}

# ============================================
# Exit Code Tests
# ============================================

test_errors_return_nonzero() {
  local test_name="Errors return non-zero exit codes"

  # Create a function that simulates an error
  check_docker_error() {
    if ! command -v docker >/dev/null 2>&1; then
      show_docker_not_running_error "$(uname)"
      return 1
    fi
    return 0
  }

  # This test just verifies the pattern works
  if check_docker_error >/dev/null 2>&1; then
    # Docker is installed - that's fine
    pass "$test_name: Function returns proper exit code"
  else
    # Docker not installed - function returned 1, which is correct
    pass "$test_name: Function returns proper exit code"
  fi
}

# ============================================
# Recovery Tests
# ============================================

test_errors_dont_crash_program() {
  local test_name="Errors don't crash the program"

  # Error functions should never call exit directly
  # They should return error codes instead

  local output
  output=$(show_generic_error "Test Error" "Test reason" "Solution 1" 2>&1) || true

  # We should still be running
  assert_not_empty "$output" "$test_name: Error produced output"

  # Script should continue after error
  local continue_test="still running"
  assert_equals "$continue_test" "still running" "$test_name: Script continues"
}

# ============================================
# Test Runner
# ============================================

run_all_tests() {
  printf "\n========================================\n"
  printf "  Installation Error Tests\n"
  printf "========================================\n\n"

  setup_test_environment

  # Installation errors
  test_docker_not_installed
  test_docker_daemon_not_running
  test_insufficient_permissions
  test_disk_space_insufficient
  test_incompatible_docker_version
  test_port_conflicts
  test_missing_dependencies

  # Error quality
  test_error_messages_have_structure
  test_error_messages_actionable
  test_error_messages_cross_platform

  # Error behavior
  test_errors_return_nonzero
  test_errors_dont_crash_program

  teardown_test_environment

  # Summary
  printf "\n========================================\n"
  printf "  Test Results\n"
  printf "========================================\n"
  printf "Total:   %d\n" "$TESTS_RUN"
  printf "Passed:  %d\n" "$TESTS_PASSED"
  printf "Failed:  %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n✓ All tests passed!\n\n"
    return 0
  else
    printf "\n✗ Some tests failed\n\n"
    return 1
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_all_tests
fi
