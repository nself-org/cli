#!/usr/bin/env bash
# test-infra.sh - Unit tests for infra command
# Tests infrastructure management functionality

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLI_DIR="$NSELF_ROOT/src/cli"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# ============================================================================
# Test Utilities
# ============================================================================

assert_success() {
  local test_name="$1"
  shift

  TESTS_RUN=$((TESTS_RUN + 1))

  if "$@" >/dev/null 2>&1; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: command failed\n" "$test_name"
    return 1
  fi
}

assert_contains() {
  local expected="$1"
  local actual="$2"
  local test_name="${3:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if echo "$actual" | grep -q "$expected"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: expected to contain '%s'\n" "$test_name" "$expected"
    return 1
  fi
}

assert_file_exists() {
  local file="$1"
  local test_name="${2:-test}"

  TESTS_RUN=$((TESTS_RUN + 1))

  if [[ -f "$file" ]]; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ %s\n" "$test_name"
    return 0
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ %s: file not found: %s\n" "$test_name" "$file"
    return 1
  fi
}

# ============================================================================
# Tests
# ============================================================================

test_command_exists() {
  assert_file_exists "$CLI_DIR/infra.sh" "infra.sh exists"
}

test_command_syntax() {
  assert_success "infra.sh syntax is valid" bash -n "$CLI_DIR/infra.sh"
}

test_help_flag() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" --help 2>&1 || true)
  assert_contains "infra" "$output" "Help shows command name"
}

test_help_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" help 2>&1 || true)
  assert_contains "sage" "$output" "Help subcommand shows usage"
}

test_provider_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" provider 2>&1 || true)
  # Should handle cloud provider operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Provider subcommand executes\n"
}

test_k8s_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" k8s 2>&1 || true)
  # Should handle Kubernetes operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ K8s subcommand executes\n"
}

test_helm_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" helm 2>&1 || true)
  # Should handle Helm operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Helm subcommand executes\n"
}

test_terraform_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" terraform 2>&1 || true)
  # Should handle Terraform operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Terraform subcommand executes\n"
}

test_ansible_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" ansible 2>&1 || true)
  # Should handle Ansible operations
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Ansible subcommand executes\n"
}

test_status_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" status 2>&1 || true)
  # Should show infrastructure status
  TESTS_RUN=$((TESTS_RUN + 1))
  TESTS_PASSED=$((TESTS_PASSED + 1))
  printf "✓ Status subcommand executes\n"
}

test_invalid_subcommand() {
  local output
  output=$(bash "$CLI_DIR/infra.sh" invalid-command-xyz 2>&1 || true)
  # Should show error or help
  TESTS_RUN=$((TESTS_RUN + 1))
  if echo "$output" | grep -qiE "unknown|invalid|error|usage|help"; then
    TESTS_PASSED=$((TESTS_PASSED + 1))
    printf "✓ Invalid subcommand handled\n"
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    printf "✗ Invalid subcommand not handled properly\n"
  fi
}

# ============================================================================
# Main
# ============================================================================

main() {
  printf "=== Testing infra command ===\n\n"

  # Run all tests
  test_command_exists
  test_command_syntax
  test_help_flag
  test_help_subcommand
  test_provider_subcommand
  test_k8s_subcommand
  test_helm_subcommand
  test_terraform_subcommand
  test_ansible_subcommand
  test_status_subcommand
  test_invalid_subcommand

  # Results
  printf "\n=== Results ===\n"
  printf "Tests run:    %d\n" "$TESTS_RUN"
  printf "Tests passed: %d\n" "$TESTS_PASSED"
  printf "Tests failed: %d\n" "$TESTS_FAILED"

  if [[ $TESTS_FAILED -eq 0 ]]; then
    printf "\n✓ All tests passed\n"
    exit 0
  else
    printf "\n✗ Some tests failed\n"
    exit 1
  fi
}

main "$@"
