#!/usr/bin/env bash
# test-full-deployment.sh - Complete deployment workflow integration test
#
# Tests the full lifecycle: init → build → start → verify → stop → restart

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="full-deployment"
TEST_PROJECT_DIR=""
CLEANUP_ON_EXIT=true

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
# Test Functions
# ============================================================================

test_01_init_simple() {
  describe "Test 1: Initialize project with --simple flag"

  # Create test environment
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Run nself init
  printf "Running: nself init --simple\n"
  "$NSELF_ROOT/bin/nself" init --simple

  # Verify critical files exist
  assert_file_exists ".env" ".env file should be created"
  assert_file_exists ".env.example" ".env.example should be created"
  assert_file_exists ".gitignore" ".gitignore should be created"

  # Verify .env has required variables
  assert_file_contains ".env" "PROJECT_NAME=" "PROJECT_NAME should be set"
  assert_file_contains ".env" "ENV=" "ENV should be set"
  assert_file_contains ".env" "BASE_DOMAIN=" "BASE_DOMAIN should be set"
  assert_file_contains ".env" "POSTGRES_PASSWORD=" "POSTGRES_PASSWORD should be set"
  assert_file_contains ".env" "HASURA_GRAPHQL_ADMIN_SECRET=" "HASURA_GRAPHQL_ADMIN_SECRET should be set"

  pass "Project initialized successfully"
}

test_02_modify_env() {
  describe "Test 2: Modify .env with test configuration"

  cd "$TEST_PROJECT_DIR"

  # Get project name from .env
  local project_name
  project_name=$(grep "^PROJECT_NAME=" .env | cut -d'=' -f2)

  # Enable some optional services for testing
  cat >>.env <<EOF

# Integration test configuration
REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
MAILPIT_ENABLED=true
EOF

  assert_file_contains ".env" "REDIS_ENABLED=true" "REDIS_ENABLED should be set"
  assert_file_contains ".env" "MINIO_ENABLED=true" "MINIO_ENABLED should be set"

  pass "Configuration modified successfully"
}

test_03_build() {
  describe "Test 3: Build configuration files"

  cd "$TEST_PROJECT_DIR"

  # Run nself build
  printf "Running: nself build\n"
  "$NSELF_ROOT/bin/nself" build

  # Verify docker-compose.yml generated
  assert_file_generated "docker-compose.yml" "docker-compose.yml should be generated"

  # Verify nginx configuration generated
  assert_file_generated "nginx/nginx.conf" "nginx.conf should be generated"

  # Verify postgres initialization
  assert_file_generated "postgres/init/00-init.sql" "postgres init script should be generated"

  # Verify SSL certificates
  assert_file_generated "ssl/cert.pem" "SSL certificate should be generated"
  assert_file_generated "ssl/key.pem" "SSL key should be generated"

  # Verify docker-compose has required services
  assert_file_contains "docker-compose.yml" "postgres:" "postgres service should be defined"
  assert_file_contains "docker-compose.yml" "hasura:" "hasura service should be defined"
  assert_file_contains "docker-compose.yml" "auth:" "auth service should be defined"
  assert_file_contains "docker-compose.yml" "nginx:" "nginx service should be defined"

  # Verify optional services
  assert_file_contains "docker-compose.yml" "redis:" "redis service should be defined"
  assert_file_contains "docker-compose.yml" "minio:" "minio service should be defined"

  pass "Build completed successfully"
}

test_04_start() {
  describe "Test 4: Start all services"

  cd "$TEST_PROJECT_DIR"

  # Run nself start
  printf "Running: nself start\n"
  "$NSELF_ROOT/bin/nself" start

  # Wait a bit for services to initialize
  sleep 5

  # Verify containers are running
  assert_service_running "postgres" "postgres should be running"
  assert_service_running "hasura" "hasura should be running"
  assert_service_running "auth" "auth should be running"
  assert_service_running "nginx" "nginx should be running"

  pass "Services started successfully"
}

test_05_health_checks() {
  describe "Test 5: Wait for all services to become healthy"

  cd "$TEST_PROJECT_DIR"

  # Wait for critical services
  wait_for_service_healthy "postgres" 60 || {
    printf "Postgres logs:\n"
    get_service_logs "postgres" 100
    fail "postgres failed health check"
  }

  wait_for_service_healthy "hasura" 60 || {
    printf "Hasura logs:\n"
    get_service_logs "hasura" 100
    fail "hasura failed health check"
  }

  wait_for_service_healthy "auth" 60 || {
    printf "Auth logs:\n"
    get_service_logs "auth" 100
    fail "auth failed health check"
  }

  pass "All services are healthy"
}

test_06_status_check() {
  describe "Test 6: Verify status command shows all running"

  cd "$TEST_PROJECT_DIR"

  # Run nself status
  printf "Running: nself status\n"
  local status_output
  status_output=$("$NSELF_ROOT/bin/nself" status 2>&1)

  # Check for running services
  echo "$status_output" | grep -q "postgres" || fail "postgres not in status output"
  echo "$status_output" | grep -q "hasura" || fail "hasura not in status output"
  echo "$status_output" | grep -q "auth" || fail "auth not in status output"

  pass "Status check successful"
}

test_07_urls() {
  describe "Test 7: Verify URLs are accessible"

  cd "$TEST_PROJECT_DIR"

  # Run nself urls
  printf "Running: nself urls\n"
  local urls_output
  urls_output=$("$NSELF_ROOT/bin/nself" urls 2>&1)

  # Verify URLs are listed
  echo "$urls_output" | grep -q "http" || fail "No URLs in output"

  pass "URLs listed successfully"
}

test_08_database_connection() {
  describe "Test 8: Test database connection"

  cd "$TEST_PROJECT_DIR"

  # Get database credentials from .env
  source .env

  # Test connection using docker-compose exec
  printf "Testing database connection...\n"
  local db_result
  db_result=$(docker-compose exec -T postgres psql -U postgres -d "$POSTGRES_DB" -c "SELECT 1;" 2>&1)

  echo "$db_result" | grep -q "1 row" || {
    printf "Database connection failed: %s\n" "$db_result"
    fail "Database connection test failed"
  }

  pass "Database connection successful"
}

test_09_hasura_graphql() {
  describe "Test 9: Test Hasura GraphQL endpoint"

  cd "$TEST_PROJECT_DIR"

  # Get Hasura admin secret
  source .env

  # Wait for Hasura to be ready
  sleep 5

  # Test GraphQL introspection query
  local graphql_url="http://localhost:8080/v1/graphql"
  printf "Testing GraphQL endpoint: %s\n" "$graphql_url"

  verify_graphql_endpoint "$graphql_url" "$HASURA_GRAPHQL_ADMIN_SECRET" || {
    printf "Hasura logs:\n"
    get_service_logs "hasura" 100
    fail "GraphQL endpoint test failed"
  }

  pass "Hasura GraphQL endpoint working"
}

test_10_auth_endpoint() {
  describe "Test 10: Test Auth endpoint"

  cd "$TEST_PROJECT_DIR"

  # Test auth health endpoint
  local auth_url="http://localhost:4000/healthz"
  printf "Testing Auth endpoint: %s\n" "$auth_url"

  verify_endpoint_accessible "$auth_url" 30 200 || {
    printf "Auth logs:\n"
    get_service_logs "auth" 100
    fail "Auth endpoint test failed"
  }

  pass "Auth endpoint working"
}

test_11_stop() {
  describe "Test 11: Stop all services cleanly"

  cd "$TEST_PROJECT_DIR"

  # Run nself stop
  printf "Running: nself stop\n"
  "$NSELF_ROOT/bin/nself" stop

  # Wait for shutdown
  sleep 5

  # Verify containers stopped
  local running_count
  running_count=$(docker-compose ps -q 2>/dev/null | wc -l | tr -d ' ')

  if [[ $running_count -eq 0 ]]; then
    pass "All services stopped successfully"
  else
    fail "Some services still running: $running_count containers"
  fi
}

test_12_restart() {
  describe "Test 12: Restart services"

  cd "$TEST_PROJECT_DIR"

  # Start services again
  printf "Running: nself start\n"
  "$NSELF_ROOT/bin/nself" start

  sleep 5

  # Verify services running
  assert_service_running "postgres" "postgres should be running after restart"
  assert_service_running "hasura" "hasura should be running after restart"

  # Wait for health
  wait_for_service_healthy "postgres" 60 || fail "postgres not healthy after restart"

  pass "Services restarted successfully"
}

test_13_full_restart() {
  describe "Test 13: Test restart command"

  cd "$TEST_PROJECT_DIR"

  # Run nself restart
  printf "Running: nself restart\n"
  "$NSELF_ROOT/bin/nself" restart

  sleep 5

  # Verify services running
  assert_service_running "postgres" "postgres should be running after restart command"
  assert_service_running "hasura" "hasura should be running after restart command"

  pass "Restart command successful"
}

test_14_final_cleanup() {
  describe "Test 14: Final cleanup"

  cd "$TEST_PROJECT_DIR"

  # Stop everything
  "$NSELF_ROOT/bin/nself" stop

  # Verify clean shutdown
  sleep 3

  local running_count
  running_count=$(docker-compose ps -q 2>/dev/null | wc -l | tr -d ' ')

  if [[ $running_count -eq 0 ]]; then
    pass "Final cleanup successful"
  else
    fail "Final cleanup incomplete: $running_count containers still running"
  fi
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  start_suite "Full Deployment Workflow Integration Test"

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
  printf "Full Deployment Workflow Integration Test\n"
  printf "=================================================================\n\n"

  # Run all tests in sequence
  test_01_init_simple
  test_02_modify_env
  test_03_build
  test_04_start
  test_05_health_checks
  test_06_status_check
  test_07_urls
  test_08_database_connection
  test_09_hasura_graphql
  test_10_auth_endpoint
  test_11_stop
  test_12_restart
  test_13_full_restart
  test_14_final_cleanup

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
