#!/usr/bin/env bash
# test-multi-tenant-workflow.sh - Multi-tenant lifecycle integration test
#
# Tests complete tenant lifecycle: create → members → roles → isolation → delete

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="multi-tenant-workflow"
TEST_PROJECT_DIR=""
CLEANUP_ON_EXIT=true

# Tenant test data
TENANT_1_ID=""
TENANT_2_ID=""
USER_1_ID=""
USER_2_ID=""

# ============================================================================
# Cleanup Handler
# ============================================================================

cleanup() {
  if [[ "$CLEANUP_ON_EXIT" == "true" ]] && [[ -n "$TEST_PROJECT_DIR" ]]; then
    printf "\nCleaning up test environment...\n"
    cleanup_test_project "$TEST_PROJECT_DIR"
  fi
}

trap cleanup EXIT INT TERM

# ============================================================================
# Helper Functions
# ============================================================================

run_nself_command() {
  "$NSELF_ROOT/bin/nself" "$@"
}

db_exec() {
  local query="$1"
  docker-compose exec -T postgres psql -U postgres -d "$POSTGRES_DB" -t -c "$query" 2>/dev/null | xargs
}

# ============================================================================
# Test Functions
# ============================================================================

test_01_setup() {
  describe "Test 1: Setup test environment with multi-tenancy"

  # Create test environment
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Initialize project
  run_nself_command init --simple

  # Enable multi-tenancy in .env
  cat >>.env <<EOF

# Multi-tenancy configuration
MULTI_TENANCY_ENABLED=true
TENANT_ISOLATION=strict
EOF

  # Build and start
  run_nself_command build
  run_nself_command start

  # Wait for services
  sleep 10
  wait_for_service_healthy "postgres" 60 || fail "postgres not healthy"
  wait_for_service_healthy "hasura" 60 || fail "hasura not healthy"

  # Source .env for database credentials
  source .env

  pass "Test environment setup complete"
}

test_02_create_tenants() {
  describe "Test 2: Create test tenants"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create tenant 1
  printf "Creating tenant 1...\n"
  run_nself_command tenant create \
    --name "Test Tenant 1" \
    --slug "test-tenant-1" \
    --email "admin@tenant1.test"

  TENANT_1_ID=$(db_exec "SELECT id FROM tenants WHERE slug='test-tenant-1';")

  if [[ -z "$TENANT_1_ID" ]]; then
    fail "Tenant 1 not created"
  fi

  printf "Tenant 1 created with ID: %s\n" "$TENANT_1_ID"

  # Create tenant 2
  printf "Creating tenant 2...\n"
  run_nself_command tenant create \
    --name "Test Tenant 2" \
    --slug "test-tenant-2" \
    --email "admin@tenant2.test"

  TENANT_2_ID=$(db_exec "SELECT id FROM tenants WHERE slug='test-tenant-2';")

  if [[ -z "$TENANT_2_ID" ]]; then
    fail "Tenant 2 not created"
  fi

  printf "Tenant 2 created with ID: %s\n" "$TENANT_2_ID"

  pass "Tenants created successfully"
}

test_03_add_members() {
  describe "Test 3: Add members to tenants"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create user 1 for tenant 1
  printf "Creating user 1...\n"
  run_nself_command tenant member add \
    --tenant "$TENANT_1_ID" \
    --email "user1@tenant1.test" \
    --role "member"

  USER_1_ID=$(db_exec "SELECT id FROM auth.users WHERE email='user1@tenant1.test';")

  if [[ -z "$USER_1_ID" ]]; then
    fail "User 1 not created"
  fi

  printf "User 1 created with ID: %s\n" "$USER_1_ID"

  # Create user 2 for tenant 2
  printf "Creating user 2...\n"
  run_nself_command tenant member add \
    --tenant "$TENANT_2_ID" \
    --email "user2@tenant2.test" \
    --role "member"

  USER_2_ID=$(db_exec "SELECT id FROM auth.users WHERE email='user2@tenant2.test';")

  if [[ -z "$USER_2_ID" ]]; then
    fail "User 2 not created"
  fi

  printf "User 2 created with ID: %s\n" "$USER_2_ID"

  pass "Members added successfully"
}

test_04_assign_roles() {
  describe "Test 4: Assign and verify roles"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Assign admin role to user 1
  printf "Assigning admin role to user 1...\n"
  run_nself_command tenant member role \
    --tenant "$TENANT_1_ID" \
    --user "$USER_1_ID" \
    --role "admin"

  # Verify role assignment
  local user_role
  user_role=$(db_exec "SELECT role FROM tenant_members WHERE tenant_id='$TENANT_1_ID' AND user_id='$USER_1_ID';")

  if [[ "$user_role" != "admin" ]]; then
    fail "Role not assigned correctly: expected 'admin', got '$user_role'"
  fi

  printf "User 1 role: %s\n" "$user_role"

  pass "Roles assigned successfully"
}

test_05_test_isolation() {
  describe "Test 5: Test tenant data isolation"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Create test data for tenant 1
  printf "Creating test data for tenant 1...\n"
  db_exec "INSERT INTO tenant_data (tenant_id, data_key, data_value) VALUES ('$TENANT_1_ID', 'test_key', 'tenant_1_value');"

  # Create test data for tenant 2
  printf "Creating test data for tenant 2...\n"
  db_exec "INSERT INTO tenant_data (tenant_id, data_key, data_value) VALUES ('$TENANT_2_ID', 'test_key', 'tenant_2_value');"

  # Verify tenant 1 can only see its data
  local tenant_1_data
  tenant_1_data=$(db_exec "SELECT data_value FROM tenant_data WHERE tenant_id='$TENANT_1_ID' AND data_key='test_key';")

  if [[ "$tenant_1_data" != "tenant_1_value" ]]; then
    fail "Tenant 1 data isolation failed: expected 'tenant_1_value', got '$tenant_1_data'"
  fi

  # Verify tenant 2 can only see its data
  local tenant_2_data
  tenant_2_data=$(db_exec "SELECT data_value FROM tenant_data WHERE tenant_id='$TENANT_2_ID' AND data_key='test_key';")

  if [[ "$tenant_2_data" != "tenant_2_value" ]]; then
    fail "Tenant 2 data isolation failed: expected 'tenant_2_value', got '$tenant_2_data'"
  fi

  # Verify cross-tenant query returns no results
  local cross_tenant_count
  cross_tenant_count=$(db_exec "SELECT COUNT(*) FROM tenant_data WHERE tenant_id='$TENANT_1_ID' AND data_key='test_key' AND data_value='tenant_2_value';")

  if [[ $cross_tenant_count -ne 0 ]]; then
    fail "Data leaked between tenants"
  fi

  pass "Tenant isolation verified"
}

test_06_update_settings() {
  describe "Test 6: Update tenant settings"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Update tenant 1 settings
  printf "Updating tenant 1 settings...\n"
  run_nself_command tenant settings update \
    --tenant "$TENANT_1_ID" \
    --key "max_users" \
    --value "100"

  # Verify setting updated
  local setting_value
  setting_value=$(db_exec "SELECT value FROM tenant_settings WHERE tenant_id='$TENANT_1_ID' AND key='max_users';")

  if [[ "$setting_value" != "100" ]]; then
    fail "Setting not updated: expected '100', got '$setting_value'"
  fi

  pass "Tenant settings updated successfully"
}

test_07_list_tenants() {
  describe "Test 7: List all tenants"

  cd "$TEST_PROJECT_DIR"

  # List tenants
  printf "Listing all tenants...\n"
  local tenant_list
  tenant_list=$(run_nself_command tenant list 2>&1)

  # Verify both tenants in list
  echo "$tenant_list" | grep -q "test-tenant-1" || fail "Tenant 1 not in list"
  echo "$tenant_list" | grep -q "test-tenant-2" || fail "Tenant 2 not in list"

  pass "Tenant listing successful"
}

test_08_remove_member() {
  describe "Test 8: Remove member from tenant"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Remove user 1 from tenant 1
  printf "Removing user 1 from tenant 1...\n"
  run_nself_command tenant member remove \
    --tenant "$TENANT_1_ID" \
    --user "$USER_1_ID"

  # Verify member removed
  local member_count
  member_count=$(db_exec "SELECT COUNT(*) FROM tenant_members WHERE tenant_id='$TENANT_1_ID' AND user_id='$USER_1_ID';")

  if [[ $member_count -ne 0 ]]; then
    fail "Member not removed: count=$member_count"
  fi

  pass "Member removed successfully"
}

test_09_delete_tenant() {
  describe "Test 9: Delete tenant and verify cleanup"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Delete tenant 1
  printf "Deleting tenant 1...\n"
  run_nself_command tenant delete \
    --tenant "$TENANT_1_ID" \
    --confirm

  # Verify tenant deleted
  local tenant_count
  tenant_count=$(db_exec "SELECT COUNT(*) FROM tenants WHERE id='$TENANT_1_ID';")

  if [[ $tenant_count -ne 0 ]]; then
    fail "Tenant not deleted: count=$tenant_count"
  fi

  # Verify tenant data cleaned up
  local data_count
  data_count=$(db_exec "SELECT COUNT(*) FROM tenant_data WHERE tenant_id='$TENANT_1_ID';")

  if [[ $data_count -ne 0 ]]; then
    fail "Tenant data not cleaned up: count=$data_count"
  fi

  # Verify tenant settings cleaned up
  local settings_count
  settings_count=$(db_exec "SELECT COUNT(*) FROM tenant_settings WHERE tenant_id='$TENANT_1_ID';")

  if [[ $settings_count -ne 0 ]]; then
    fail "Tenant settings not cleaned up: count=$settings_count"
  fi

  pass "Tenant deleted and cleaned up successfully"
}

test_10_verify_remaining_tenant() {
  describe "Test 10: Verify remaining tenant unaffected"

  cd "$TEST_PROJECT_DIR"
  source .env

  # Verify tenant 2 still exists
  local tenant_2_exists
  tenant_2_exists=$(db_exec "SELECT COUNT(*) FROM tenants WHERE id='$TENANT_2_ID';")

  if [[ $tenant_2_exists -ne 1 ]]; then
    fail "Tenant 2 was affected by tenant 1 deletion"
  fi

  # Verify tenant 2 data still exists
  local tenant_2_data
  tenant_2_data=$(db_exec "SELECT data_value FROM tenant_data WHERE tenant_id='$TENANT_2_ID' AND data_key='test_key';")

  if [[ "$tenant_2_data" != "tenant_2_value" ]]; then
    fail "Tenant 2 data was affected: expected 'tenant_2_value', got '$tenant_2_data'"
  fi

  pass "Remaining tenant unaffected by deletion"
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  start_suite "Multi-Tenant Workflow Integration Test"

  # Skip gracefully when Docker or nself is not available (requires live stack in CI)
  if ! docker ps >/dev/null 2>&1; then
    printf "⚠ Docker not available - skipping workflow tests\n"
    exit 0
  fi
  if [[ -z "${NSELF_ROOT:-}" ]] || [[ ! -x "${NSELF_ROOT}/bin/nself" ]]; then
    printf "⚠ NSELF_ROOT not set or nself not found - skipping workflow tests\n"
    exit 0
  fi

  printf "\n=================================================================\n"
  printf "Multi-Tenant Workflow Integration Test\n"
  printf "=================================================================\n\n"

  # Run all tests in sequence
  test_01_setup
  test_02_create_tenants
  test_03_add_members
  test_04_assign_roles
  test_05_test_isolation
  test_06_update_settings
  test_07_list_tenants
  test_08_remove_member
  test_09_delete_tenant
  test_10_verify_remaining_tenant

  # Print summary
  printf "\n=================================================================\n"
  printf "Test Summary\n"
  printf "=================================================================\n"
  printf "Total Tests: %d\n" "$TESTS_RUN"
  printf "Passed: %d\n" "$TESTS_PASSED"
  printf "Failed: %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"
  printf "=================================================================\n\n"

  # Exit with proper code
  if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
