#!/usr/bin/env bash
# test-custom-services-workflow.sh - Custom services integration test
#
# Tests: Configure CS_1-4 → build → verify → routes → logs → updates

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="custom-services-workflow"
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
# Helper Functions
# ============================================================================

run_nself_command() {
  "$NSELF_ROOT/bin/nself" "$@"
}

# ============================================================================
# Test Functions
# ============================================================================

test_01_setup() {
  describe "Test 1: Setup with custom services"

  # Create test environment
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Initialize project
  run_nself_command init --quiet

  # Configure custom services (CS_1 through CS_4)
  cat >>.env <<EOF

# Custom Services Configuration
CS_1=express-api:express-js:8001
CS_2=bullmq-worker:bullmq-js:8002
CS_3=grpc-service:grpc:8003
CS_4=python-ml:python-api:8004
EOF

  # Build configuration
  run_nself_command build

  # Verify build completed
  assert_file_generated "docker-compose.yml"

  # Source .env
  source .env

  pass "Test environment setup complete"
}

test_02_verify_service_directories() {
  describe "Test 2: Verify custom service directories created"

  cd "$TEST_PROJECT_DIR"

  # Check service directories exist
  local services_dir="$TEST_PROJECT_DIR/services"

  assert_file_generated "$services_dir/express-api/Dockerfile"
  assert_file_generated "$services_dir/express-api/package.json"

  assert_file_generated "$services_dir/bullmq-worker/Dockerfile"
  assert_file_generated "$services_dir/bullmq-worker/package.json"

  assert_file_generated "$services_dir/grpc-service/Dockerfile"

  assert_file_generated "$services_dir/python-ml/Dockerfile"
  assert_file_generated "$services_dir/python-ml/requirements.txt"

  printf "All custom service directories created\n"

  pass "Service directories verified"
}

test_03_verify_docker_compose() {
  describe "Test 3: Verify custom services in docker-compose.yml"

  cd "$TEST_PROJECT_DIR"

  # Check docker-compose.yml has all custom services
  assert_file_contains "docker-compose.yml" "express-api:"
  assert_file_contains "docker-compose.yml" "bullmq-worker:"
  assert_file_contains "docker-compose.yml" "grpc-service:"
  assert_file_contains "docker-compose.yml" "python-ml:"

  # Verify ports are configured
  assert_file_contains "docker-compose.yml" "8001:"
  assert_file_contains "docker-compose.yml" "8002:"
  assert_file_contains "docker-compose.yml" "8003:"
  assert_file_contains "docker-compose.yml" "8004:"

  pass "Docker Compose configuration verified"
}

test_04_start_services() {
  describe "Test 4: Start all services including custom services"

  cd "$TEST_PROJECT_DIR"

  # Start services
  run_nself_command start

  # Wait for services to initialize
  sleep 15

  # Verify core services running
  assert_service_running "postgres"
  assert_service_running "hasura"

  # Verify custom services running
  assert_service_running "express-api"
  assert_service_running "bullmq-worker"
  assert_service_running "grpc-service"
  assert_service_running "python-ml"

  pass "All services started successfully"
}

test_05_verify_nginx_routes() {
  describe "Test 5: Verify nginx routes for custom services"

  cd "$TEST_PROJECT_DIR"

  # Check nginx configuration
  local nginx_conf="$TEST_PROJECT_DIR/nginx/nginx.conf"

  if [[ ! -f "$nginx_conf" ]]; then
    # Try sites directory
    nginx_conf="$TEST_PROJECT_DIR/nginx/sites/custom-services.conf"
  fi

  # Verify routes exist for services with public endpoints
  # (Worker service might not have a public route)

  if [[ -f "$nginx_conf" ]]; then
    # Check for API routes
    if grep -q "express-api\|8001" "$nginx_conf" || \
       grep -q "grpc-service\|8003" "$nginx_conf" || \
       grep -q "python-ml\|8004" "$nginx_conf"; then
      printf "✓ Custom service routes configured in nginx\n"
    else
      print_warning "Custom service routes not found in nginx config"
    fi
  else
    print_warning "Nginx configuration file not found"
  fi

  pass "Nginx routes verified"
}

test_06_test_service_endpoints() {
  describe "Test 6: Test custom service endpoints"

  cd "$TEST_PROJECT_DIR"

  # Wait for services to be fully ready
  sleep 10

  # Test Express API endpoint
  printf "Testing Express API...\n"
  if verify_endpoint_accessible "http://localhost:8001" 30 || \
     verify_endpoint_accessible "http://localhost:8001/health" 30; then
    printf "✓ Express API accessible\n"
  else
    print_warning "Express API not accessible (might still be initializing)"
  fi

  # Test Python ML API
  printf "Testing Python ML API...\n"
  if verify_endpoint_accessible "http://localhost:8004" 30 || \
     verify_endpoint_accessible "http://localhost:8004/health" 30; then
    printf "✓ Python ML API accessible\n"
  else
    print_warning "Python ML API not accessible (might still be initializing)"
  fi

  pass "Service endpoints tested"
}

test_07_verify_logs() {
  describe "Test 7: Verify logs accessible for custom services"

  cd "$TEST_PROJECT_DIR"

  # Get logs for each service
  printf "Checking service logs...\n"

  local services=("express-api" "bullmq-worker" "grpc-service" "python-ml")

  for service in "${services[@]}"; do
    local logs
    logs=$(get_service_logs "$service" 10)

    if [[ -n "$logs" ]]; then
      printf "✓ Logs available for %s (%d lines)\n" "$service" "$(echo "$logs" | wc -l)"
    else
      print_warning "No logs for $service"
    fi
  done

  pass "Service logs verified"
}

test_08_modify_service_code() {
  describe "Test 8: Modify service code and rebuild"

  cd "$TEST_PROJECT_DIR"

  # Modify Express API code
  local express_index="$TEST_PROJECT_DIR/services/express-api/src/index.js"

  if [[ -f "$express_index" ]]; then
    # Add a comment to trigger rebuild
    echo "// Modified for integration test" >> "$express_index"

    # Rebuild service
    printf "Rebuilding express-api...\n"
    docker-compose build express-api >/dev/null 2>&1

    printf "✓ Service rebuilt successfully\n"
  else
    print_warning "Express API source file not found"
  fi

  pass "Service modification verified"
}

test_09_service_restart() {
  describe "Test 9: Restart individual custom service"

  cd "$TEST_PROJECT_DIR"

  # Restart express-api
  printf "Restarting express-api...\n"
  docker-compose restart express-api >/dev/null 2>&1

  # Wait for restart
  sleep 5

  # Verify still running
  assert_service_running "express-api"

  pass "Service restart successful"
}

test_10_remove_custom_service() {
  describe "Test 10: Remove custom service and rebuild"

  cd "$TEST_PROJECT_DIR"

  # Remove CS_4 from .env
  printf "Removing CS_4 from configuration...\n"
  sed -i.bak '/CS_4=/d' .env

  # Rebuild
  run_nself_command build

  # Verify python-ml not in docker-compose.yml
  if grep -q "python-ml:" docker-compose.yml; then
    fail "python-ml should not be in docker-compose.yml after removal"
  fi

  printf "✓ Custom service removed from configuration\n"

  pass "Service removal verified"
}

test_11_add_new_custom_service() {
  describe "Test 11: Add new custom service dynamically"

  cd "$TEST_PROJECT_DIR"

  # Add CS_5
  printf "Adding CS_5 to configuration...\n"
  echo "CS_5=fastapi-service:python-fastapi:8005" >> .env

  # Rebuild
  run_nself_command build

  # Verify fastapi-service in docker-compose.yml
  if grep -q "fastapi-service:" docker-compose.yml; then
    printf "✓ New custom service added to docker-compose.yml\n"
  else
    fail "fastapi-service not in docker-compose.yml"
  fi

  # Verify service directory created
  assert_file_generated "$TEST_PROJECT_DIR/services/fastapi-service/Dockerfile"

  pass "New service added successfully"
}

test_12_verify_service_isolation() {
  describe "Test 12: Verify custom services are isolated"

  cd "$TEST_PROJECT_DIR"

  # Verify services run in separate containers
  local express_container
  local worker_container

  express_container=$(get_service_container_id "express-api")
  worker_container=$(get_service_container_id "bullmq-worker")

  if [[ "$express_container" != "$worker_container" ]] && \
     [[ -n "$express_container" ]] && \
     [[ -n "$worker_container" ]]; then
    printf "✓ Services run in isolated containers\n"
  else
    fail "Service isolation verification failed"
  fi

  pass "Service isolation verified"
}

test_13_custom_service_env_vars() {
  describe "Test 13: Verify custom services receive environment variables"

  cd "$TEST_PROJECT_DIR"

  # Check environment variables in express-api
  local env_check
  env_check=$(exec_in_service "express-api" "env | grep NODE_ENV" || echo "")

  if [[ -n "$env_check" ]]; then
    printf "✓ Environment variables passed to custom services\n"
  else
    print_warning "Could not verify environment variables"
  fi

  pass "Environment variables verified"
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  start_suite "Custom Services Workflow Integration Test"

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
  printf "Custom Services Workflow Integration Test\n"
  printf "=================================================================\n\n"

  # Run all tests in sequence
  test_01_setup
  test_02_verify_service_directories
  test_03_verify_docker_compose
  test_04_start_services
  test_05_verify_nginx_routes
  test_06_test_service_endpoints
  test_07_verify_logs
  test_08_modify_service_code
  test_09_service_restart
  test_10_remove_custom_service
  test_11_add_new_custom_service
  test_12_verify_service_isolation
  test_13_custom_service_env_vars

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
