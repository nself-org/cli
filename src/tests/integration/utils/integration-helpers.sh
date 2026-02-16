#!/usr/bin/env bash
set -euo pipefail
# integration-helpers.sh - Core utilities for integration testing
#
# Provides helper functions for setting up, managing, and cleaning up
# integration test environments

# Ensure NSELF_ROOT is set
NSELF_ROOT="${NSELF_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)}"
export NSELF_ROOT

# Test framework location
TEST_FRAMEWORK="$NSELF_ROOT/src/tests/test_framework.sh"

# Load test framework if available
if [[ -f "$TEST_FRAMEWORK" ]]; then
  source "$TEST_FRAMEWORK"
fi

# Default timeout for service health checks (seconds)
readonly DEFAULT_HEALTH_TIMEOUT=120
readonly DEFAULT_HEALTH_INTERVAL=2

# Test project name prefix
readonly TEST_PROJECT_PREFIX="nself-integration-test"

# ============================================================================
# Project Setup & Cleanup
# ============================================================================

# setup_test_project - Create isolated test environment
# Usage: setup_test_project [project_name]
setup_test_project() {
  local project_name="${1:-$(generate_test_project_name)}"
  local test_dir="/tmp/$project_name"

  # Clean up any existing test directory
  if [[ -d "$test_dir" ]]; then
    cleanup_test_project "$test_dir"
  fi

  # Create fresh test directory
  mkdir -p "$test_dir"
  cd "$test_dir"

  printf "Test environment: %s\n" "$test_dir"
  echo "$test_dir"
}

# cleanup_test_project - Remove test project and all containers
# Usage: cleanup_test_project [test_dir]
cleanup_test_project() {
  local test_dir="${1:-$PWD}"

  printf "Cleaning up test environment: %s\n" "$test_dir"

  # Stop and remove all containers for this project
  if [[ -f "$test_dir/docker-compose.yml" ]]; then
    cd "$test_dir"
    docker-compose down -v --remove-orphans >/dev/null 2>&1 || true
  fi

  # Remove test directory
  if [[ "$test_dir" == /tmp/${TEST_PROJECT_PREFIX}* ]]; then
    rm -rf "$test_dir"
  else
    printf "WARNING: Not removing directory (doesn't match test prefix): %s\n" "$test_dir"
  fi
}

# generate_test_project_name - Create unique test project name
generate_test_project_name() {
  echo "${TEST_PROJECT_PREFIX}-$$-$(date +%s)"
}

# ============================================================================
# Service Health Checks
# ============================================================================

# wait_for_service_healthy - Wait for service to become healthy
# Usage: wait_for_service_healthy <service_name> [timeout] [interval]
wait_for_service_healthy() {
  local service_name="$1"
  local timeout="${2:-$DEFAULT_HEALTH_TIMEOUT}"
  local interval="${3:-$DEFAULT_HEALTH_INTERVAL}"
  local elapsed=0

  printf "Waiting for %s to become healthy (timeout: %ds)...\n" "$service_name" "$timeout"

  while [[ $elapsed -lt $timeout ]]; do
    local health_status
    health_status=$(docker-compose ps -q "$service_name" 2>/dev/null | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null || echo "unknown")

    if [[ "$health_status" == "healthy" ]]; then
      printf "✓ %s is healthy (took %ds)\n" "$service_name" "$elapsed"
      return 0
    fi

    # Check if container is even running
    local running_status
    running_status=$(docker-compose ps -q "$service_name" 2>/dev/null | xargs docker inspect -f '{{.State.Running}}' 2>/dev/null || echo "false")

    if [[ "$running_status" != "true" ]]; then
      printf "✗ %s is not running\n" "$service_name"
      return 1
    fi

    sleep "$interval"
    elapsed=$((elapsed + interval))
  done

  printf "✗ %s health check timeout after %ds\n" "$service_name" "$timeout"
  return 1
}

# wait_for_all_services_healthy - Wait for all services to be healthy
# Usage: wait_for_all_services_healthy [timeout]
wait_for_all_services_healthy() {
  local timeout="${1:-$DEFAULT_HEALTH_TIMEOUT}"
  local interval="${2:-$DEFAULT_HEALTH_INTERVAL}"
  local elapsed=0

  printf "Waiting for all services to become healthy...\n"

  while [[ $elapsed -lt $timeout ]]; do
    local unhealthy_count=0
    local total_count=0

    # Get all service names
    while IFS= read -r service_name; do
      [[ -z "$service_name" ]] && continue
      total_count=$((total_count + 1))

      local health_status
      health_status=$(docker-compose ps -q "$service_name" 2>/dev/null | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null || echo "unknown")

      if [[ "$health_status" != "healthy" ]]; then
        unhealthy_count=$((unhealthy_count + 1))
      fi
    done < <(docker-compose config --services 2>/dev/null)

    if [[ $unhealthy_count -eq 0 ]]; then
      printf "✓ All %d services are healthy (took %ds)\n" "$total_count" "$elapsed"
      return 0
    fi

    printf "  %d/%d services healthy...\n" "$((total_count - unhealthy_count))" "$total_count"
    sleep "$interval"
    elapsed=$((elapsed + interval))
  done

  printf "✗ Services health check timeout after %ds\n" "$timeout"
  return 1
}

# ============================================================================
# Endpoint Verification
# ============================================================================

# verify_endpoint_accessible - Check if endpoint is accessible
# Usage: verify_endpoint_accessible <url> [timeout] [expected_status]
verify_endpoint_accessible() {
  local url="$1"
  local timeout="${2:-30}"
  local expected_status="${3:-200}"
  local elapsed=0

  printf "Verifying endpoint: %s\n" "$url"

  while [[ $elapsed -lt $timeout ]]; do
    local http_code
    http_code=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo "000")

    if [[ "$http_code" == "$expected_status" ]]; then
      printf "✓ Endpoint accessible: %s (HTTP %s)\n" "$url" "$http_code"
      return 0
    fi

    sleep 2
    elapsed=$((elapsed + 2))
  done

  printf "✗ Endpoint not accessible: %s (timeout)\n" "$url"
  return 1
}

# verify_graphql_endpoint - Check if GraphQL endpoint is working
# Usage: verify_graphql_endpoint <url> [admin_secret]
verify_graphql_endpoint() {
  local url="$1"
  local admin_secret="${2:-}"

  printf "Verifying GraphQL endpoint: %s\n" "$url"

  local headers=""
  if [[ -n "$admin_secret" ]]; then
    headers="-H 'x-hasura-admin-secret: $admin_secret'"
  fi

  local response
  response=$(curl -s $headers \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"query":"{ __schema { queryType { name } } }"}' \
    "$url" 2>/dev/null || echo "")

  if echo "$response" | grep -q "queryType"; then
    printf "✓ GraphQL endpoint is working\n"
    return 0
  else
    printf "✗ GraphQL endpoint error: %s\n" "$response"
    return 1
  fi
}

# ============================================================================
# Data Management
# ============================================================================

# create_test_data - Insert test data into database
# Usage: create_test_data <table_name> <json_data>
create_test_data() {
  local table_name="$1"
  local json_data="$2"

  printf "Creating test data in table: %s\n" "$table_name"

  # Execute via docker-compose exec
  docker-compose exec -T postgres psql -U postgres -d "${POSTGRES_DB:-nself}" -c \
    "INSERT INTO $table_name SELECT * FROM json_populate_recordset(NULL::$table_name, '$json_data'::json);" \
    >/dev/null 2>&1

  if [[ $? -eq 0 ]]; then
    printf "✓ Test data created\n"
    return 0
  else
    printf "✗ Failed to create test data\n"
    return 1
  fi
}

# verify_test_data - Check if test data exists
# Usage: verify_test_data <table_name> <condition>
verify_test_data() {
  local table_name="$1"
  local condition="$2"

  printf "Verifying test data in table: %s\n" "$table_name"

  local count
  count=$(docker-compose exec -T postgres psql -U postgres -d "${POSTGRES_DB:-nself}" -t -c \
    "SELECT COUNT(*) FROM $table_name WHERE $condition;" 2>/dev/null | tr -d ' ')

  if [[ $count -gt 0 ]]; then
    printf "✓ Test data verified (%s records)\n" "$count"
    return 0
  else
    printf "✗ No test data found\n"
    return 1
  fi
}

# clear_test_data - Remove test data from database
# Usage: clear_test_data <table_name> [condition]
clear_test_data() {
  local table_name="$1"
  local condition="${2:-1=1}"

  printf "Clearing test data from table: %s\n" "$table_name"

  docker-compose exec -T postgres psql -U postgres -d "${POSTGRES_DB:-nself}" -c \
    "DELETE FROM $table_name WHERE $condition;" >/dev/null 2>&1

  if [[ $? -eq 0 ]]; then
    printf "✓ Test data cleared\n"
    return 0
  else
    printf "✗ Failed to clear test data\n"
    return 1
  fi
}

# ============================================================================
# Service Mocking
# ============================================================================

# mock_external_service - Create mock HTTP endpoint
# Usage: mock_external_service <port> <response_file>
mock_external_service() {
  local port="$1"
  local response_file="$2"

  printf "Starting mock service on port %s\n" "$port"

  # Use Python simple HTTP server for mocking
  python3 -m http.server "$port" --directory "$(dirname "$response_file")" >/dev/null 2>&1 &
  local mock_pid=$!

  # Wait for server to start
  sleep 2

  if kill -0 "$mock_pid" 2>/dev/null; then
    printf "✓ Mock service running (PID: %s)\n" "$mock_pid"
    echo "$mock_pid"
    return 0
  else
    printf "✗ Failed to start mock service\n"
    return 1
  fi
}

# stop_mock_service - Stop mock HTTP endpoint
# Usage: stop_mock_service <pid>
stop_mock_service() {
  local pid="$1"

  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null
    printf "✓ Mock service stopped (PID: %s)\n" "$pid"
    return 0
  else
    printf "Mock service not running (PID: %s)\n" "$pid"
    return 1
  fi
}

# ============================================================================
# Assertion Helpers
# ============================================================================

# assert_integration - Extended assertion for integration tests
# Usage: assert_integration <command> <message>
assert_integration() {
  local command="$1"
  local message="$2"

  printf "Testing: %s\n" "$message"

  if eval "$command" >/dev/null 2>&1; then
    printf "  ✓ %s\n" "$message"
    return 0
  else
    printf "  ✗ %s\n" "$message"
    return 1
  fi
}

# assert_service_running - Check if service is running
# Usage: assert_service_running <service_name>
assert_service_running() {
  local service_name="$1"

  local status
  status=$(docker-compose ps -q "$service_name" 2>/dev/null | xargs docker inspect -f '{{.State.Running}}' 2>/dev/null || echo "false")

  if [[ "$status" == "true" ]]; then
    printf "✓ Service running: %s\n" "$service_name"
    return 0
  else
    printf "✗ Service not running: %s\n" "$service_name"
    return 1
  fi
}

# assert_file_generated - Check if build generated a file
# Usage: assert_file_generated <file_path>
assert_file_generated() {
  local file_path="$1"

  if [[ -f "$file_path" ]]; then
    printf "✓ File generated: %s\n" "$file_path"
    return 0
  else
    printf "✗ File not generated: %s\n" "$file_path"
    return 1
  fi
}

# ============================================================================
# Utility Functions
# ============================================================================

# get_service_logs - Get last N lines of service logs
# Usage: get_service_logs <service_name> [lines]
get_service_logs() {
  local service_name="$1"
  local lines="${2:-50}"

  docker-compose logs --tail="$lines" "$service_name" 2>/dev/null
}

# get_service_container_id - Get container ID for service
# Usage: get_service_container_id <service_name>
get_service_container_id() {
  local service_name="$1"

  docker-compose ps -q "$service_name" 2>/dev/null
}

# exec_in_service - Execute command in service container
# Usage: exec_in_service <service_name> <command>
exec_in_service() {
  local service_name="$1"
  shift
  local command="$*"

  docker-compose exec -T "$service_name" sh -c "$command" 2>/dev/null
}

# wait_for_port - Wait for port to be listening
# Usage: wait_for_port <host> <port> [timeout]
wait_for_port() {
  local host="$1"
  local port="$2"
  local timeout="${3:-30}"
  local elapsed=0

  printf "Waiting for %s:%s to be available...\n" "$host" "$port"

  while [[ $elapsed -lt $timeout ]]; do
    if nc -z "$host" "$port" >/dev/null 2>&1; then
      printf "✓ Port %s:%s is available\n" "$host" "$port"
      return 0
    fi

    sleep 1
    elapsed=$((elapsed + 1))
  done

  printf "✗ Port %s:%s timeout\n" "$host" "$port"
  return 1
}

# ============================================================================
# Export Functions
# ============================================================================

# Make all functions available to test scripts
export -f setup_test_project
export -f cleanup_test_project
export -f generate_test_project_name
export -f wait_for_service_healthy
export -f wait_for_all_services_healthy
export -f verify_endpoint_accessible
export -f verify_graphql_endpoint
export -f create_test_data
export -f verify_test_data
export -f clear_test_data
export -f mock_external_service
export -f stop_mock_service
export -f assert_integration
export -f assert_service_running
export -f assert_file_generated
export -f get_service_logs
export -f get_service_container_id
export -f exec_in_service
export -f wait_for_port
