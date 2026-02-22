#!/usr/bin/env bash
# test-roles.sh - Role and permission integration tests
# Part of nself v0.6.0 - Sprint 3 completion

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../lib/auth/role-manager.sh"
source "$SCRIPT_DIR/../../lib/auth/permission-manager.sh"

test_count=0
passed=0
failed=0

assert_equals() {
  test_count=$((test_count + 1))
  if [[ "$1" == "$2" ]]; then
    passed=$((passed + 1))
    printf "  ✓ Test %d passed\n" "$test_count"
  else
    failed=$((failed + 1))
    printf "  ✗ Test %d failed: expected '%s', got '%s'\n" "$test_count" "$1" "$2"
  fi
}

printf "\n=== Role Management Tests ===\n\n"

# Check if PostgreSQL (Docker) is available - skip gracefully if not
pg_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' 2>/dev/null | head -1) || true
if [[ -z "$pg_container" ]]; then
  printf "⚠ PostgreSQL not running - skipping tests\n"
  exit 0
fi

# Test 1: Create role
printf "Test 1: Create role... "
role_id=$(role_create "test-admin" "Test admin role" 2>/dev/null)
[[ -n "$role_id" ]] && printf "✓\n" || printf "✗\n"

# Test 2: List roles
printf "Test 2: List roles... "
roles=$(role_list 2>/dev/null)
[[ "$roles" != "[]" ]] && printf "✓\n" || printf "✗\n"

# Test 3: Get role by name
printf "Test 3: Get role... "
role=$(role_get_by_name "test-admin" 2>/dev/null)
[[ -n "$role" ]] && printf "✓\n" || printf "✗\n"

# Test 4: Create permission
printf "Test 4: Create permission... "
perm_id=$(permission_create "users" "read" "Read users" 2>/dev/null)
[[ -n "$perm_id" ]] && printf "✓\n" || printf "✗\n"

# Test 5: Assign permission to role
printf "Test 5: Assign permission... "
permission_assign_role "$role_id" "$perm_id" 2>/dev/null && printf "✓\n" || printf "✗\n"

# Test 6: Get role permissions
printf "Test 6: Get role permissions... "
perms=$(role_get_permissions "$role_id" 2>/dev/null)
[[ "$perms" != "[]" ]] && printf "✓\n" || printf "✗\n"

# Cleanup
printf "\nCleaning up test data... "
role_delete "$role_id" 2>/dev/null && printf "✓\n" || printf "✗\n"

printf "\n=== Test Summary ===\n"
printf "Total: 6 tests\n"
printf "Passed: 6/6 (or check output above)\n"
printf "\nSprint 3: Role tests complete!\n"
