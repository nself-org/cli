#!/usr/bin/env bash
set -euo pipefail

# test-service-failures.sh - Service startup and runtime error tests
# Tests realistic service failure scenarios

set -e

# Get script directory
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$TEST_DIR/../.."

# Source test framework
source "$TEST_DIR/../test_framework.sh"

# Source utilities
source "$ROOT_DIR/lib/utils/error-messages.sh"

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
# Port Conflict Errors
# ============================================

test_port_already_in_use() {
  local test_name="Port already in use"

  local output
  output=$(show_port_conflict_error 5432 "postgres" "PostgreSQL")

  assert_contains "$output" "Port 5432 is already in use" "$test_name: Port number"
  assert_contains "$output" "postgres" "$test_name: Service name"

  # Should provide platform-specific commands
  assert_contains "$output" "lsof" "$test_name: macOS/Linux command"

  # Should provide solutions
  assert_contains "$output" "kill" "$test_name: Kill process"
  assert_contains "$output" "POSTGRES_PORT" "$test_name: Change port config"
}

test_multiple_port_conflicts() {
  local test_name="Multiple services with port conflicts"

  local output
  output=$(cat <<'EOF'
Multiple port conflicts detected

Problem:
  The following services failed to start due to port conflicts:
    - postgres (port 5432)
    - redis (port 6379)
    - hasura (port 8080)

Fix:
  Option 1: Kill conflicting processes
    # Find processes
    lsof -i :5432
    lsof -i :6379
    lsof -i :8080

    # Kill processes (replace PID with actual process ID)
    kill -9 <PID>

  Option 2: Change ports in .env
    POSTGRES_PORT=5433
    REDIS_PORT=6380
    HASURA_PORT=8081

  Option 3: Stop other nself instances
    nself stop  # in other project directory

  Then restart:
    nself start
EOF
)

  assert_contains "$output" "Multiple port conflicts" "$test_name: Error title"
  assert_contains "$output" "5432" "$test_name: Lists postgres port"
  assert_contains "$output" "6379" "$test_name: Lists redis port"
  assert_contains "$output" "Option 1:" "$test_name: Multiple solutions"
}

# ============================================
# Container Startup Failures
# ============================================

test_container_fails_to_start() {
  local test_name="Container fails to start"

  local output
  output=$(show_container_failed_error "hasura" "database connection failed" "")

  assert_contains "$output" "Container 'hasura' failed to start" "$test_name: Error title"
  assert_contains "$output" "database connection failed" "$test_name: Reason"
  assert_contains "$output" "nself logs hasura" "$test_name: View logs command"
  assert_contains "$output" "nself restart" "$test_name: Restart command"
}

test_dependency_not_ready() {
  local test_name="Dependency service not ready"

  local output
  output=$(cat <<'EOF'
Service dependency not ready

Problem:
  Container 'hasura' is waiting for dependency 'postgres'
  postgres is still starting up (not healthy yet)

Status:
  postgres: starting (health: 0/30 checks passed)
  hasura: waiting

Fix:
  This is usually temporary. The service will start automatically when
  the dependency is ready.

  1. Wait 30-60 seconds for postgres to become healthy

  2. Check postgres status:
     docker ps | grep postgres
     nself logs postgres

  3. If postgres fails to start:
     nself restart postgres

  4. If still failing after 2 minutes:
     nself doctor
EOF
)

  assert_contains "$output" "dependency not ready" "$test_name: Error title"
  assert_contains "$output" "waiting for dependency" "$test_name: Explains relationship"
  assert_contains "$output" "temporarily" "$test_name: Reassures user"
  assert_contains "$output" "automatically" "$test_name: Auto-recovery mentioned"
}

test_health_check_timeout() {
  local test_name="Health check timeout"

  local output
  output=$(show_health_check_error "hasura" "timeout")

  assert_contains "$output" "unhealthy" "$test_name: Status"
  assert_contains "$output" "nself logs hasura" "$test_name: Logs command"

  # Should provide troubleshooting steps
  assert_contains "$output" "restart" "$test_name: Restart suggestion"
}

test_missing_docker_image() {
  local test_name="Docker image not found"

  local output
  output=$(cat <<'EOF'
Docker image not found

Problem:
  Image 'nhost/hasura-auth:latest' could not be pulled
  Possible causes:
    - Network connection issue
    - Image doesn't exist
    - Registry authentication required

Fix:
  1. Check internet connection:
     ping docker.io

  2. Try pulling manually:
     docker pull nhost/hasura-auth:latest

  3. If authentication required:
     docker login

  4. Check if image name is correct in docker-compose.yml

  5. Try rebuilding:
     nself build
     nself start
EOF
)

  assert_contains "$output" "Image.*not found" "$test_name: Error title"
  assert_contains "$output" "Possible causes" "$test_name: Lists causes"
  assert_contains "$output" "docker pull" "$test_name: Manual pull command"
  assert_contains "$output" "docker login" "$test_name: Auth command"
}

test_build_failure() {
  local test_name="Docker build failure"

  local output
  output=$(show_build_error "custom-api" "RUN npm install" "npm ERR! code ENOENT")

  assert_contains "$output" "Build failed" "$test_name: Error title"
  assert_contains "$output" "custom-api" "$test_name: Service name"
  assert_contains "$output" "RUN npm install" "$test_name: Failed step"

  # Should provide build-specific solutions
  assert_contains "$output" "docker builder prune" "$test_name: Clear cache"
}

# ============================================
# Resource Limit Errors
# ============================================

test_out_of_memory() {
  local test_name="Container out of memory"

  local output
  output=$(show_resource_error "memory" "1GB" "2GB")

  assert_contains "$output" "Insufficient memory" "$test_name: Error title"
  assert_contains "$output" "Available: 1GB" "$test_name: Available"
  assert_contains "$output" "Required: 2GB" "$test_name: Required"

  # Should suggest how to free memory
  if printf "%s" "$output" | grep -qE '(docker.*prune|stop.*containers|increase)'; then
    pass "$test_name: Suggests memory solutions"
  else
    fail "$test_name: Missing memory solutions"
  fi
}

test_disk_full() {
  local test_name="Disk full during operation"

  local output
  output=$(cat <<'EOF'
Disk space exhausted

Problem:
  Container failed to write data - disk is full
  Available: 0 MB
  Required: 500 MB minimum

Fix:
  Free up disk space:

  1. Clean Docker system:
     docker system prune -a --volumes

  2. Remove old containers:
     docker container prune

  3. Remove old images:
     docker image prune -a

  4. Clean nself logs:
     nself logs --clean

  5. Check disk usage:
     df -h
     du -sh /var/lib/docker

  After cleanup:
     nself start
EOF
)

  assert_contains "$output" "Disk space exhausted" "$test_name: Error title"
  assert_contains "$output" "docker system prune" "$test_name: Cleanup command"
  assert_contains "$output" "df -h" "$test_name: Check usage command"
}

# ============================================
# Network Errors
# ============================================

test_network_connection_failed() {
  local test_name="Network connection failed"

  local output
  output=$(show_network_error "hasura" "http://postgres:5432" "connection refused")

  assert_contains "$output" "Network connection failed" "$test_name: Error title"
  assert_contains "$output" "hasura" "$test_name: Service name"
  assert_contains "$output" "connection refused" "$test_name: Error message"

  # Should suggest network troubleshooting
  assert_contains "$output" "docker network" "$test_name: Network command"
}

test_dns_resolution_failure() {
  local test_name="DNS resolution failure"

  local output
  output=$(cat <<'EOF'
DNS resolution failed

Problem:
  Service 'hasura' cannot resolve hostname 'postgres'
  Error: Name or service not known

Fix:
  1. Check if postgres container is running:
     docker ps | grep postgres

  2. Verify Docker network:
     docker network inspect <project>_default

  3. Restart networking:
     nself stop
     docker network prune
     nself start

  4. Check docker-compose.yml network configuration

  5. Verify service names match:
     grep "container_name:" docker-compose.yml
EOF
)

  assert_contains "$output" "DNS resolution failed" "$test_name: Error title"
  assert_contains "$output" "docker network inspect" "$test_name: Inspect network"
  assert_contains "$output" "docker network prune" "$test_name: Cleanup network"
}

# ============================================
# Configuration Errors at Runtime
# ============================================

test_environment_variable_missing_at_runtime() {
  local test_name="Missing environment variable at runtime"

  local output
  output=$(cat <<'EOF'
Missing environment variable

Problem:
  Container 'hasura' exited immediately
  Error: HASURA_GRAPHQL_ADMIN_SECRET is not set

Fix:
  1. Check .env file has the variable:
     grep HASURA_GRAPHQL_ADMIN_SECRET .env

  2. If missing, add it:
     HASURA_GRAPHQL_ADMIN_SECRET=$(openssl rand -base64 32)

  3. Rebuild configuration:
     nself build

  4. Restart service:
     nself restart hasura

  Note: Some variables are required even if they have defaults in docker-compose.yml
EOF
)

  assert_contains "$output" "Missing environment variable" "$test_name: Error title"
  assert_contains "$output" "HASURA_GRAPHQL_ADMIN_SECRET" "$test_name: Variable name"
  assert_contains "$output" "openssl rand" "$test_name: Generation command"
  assert_contains "$output" "nself build" "$test_name: Rebuild step"
}

# ============================================
# Volume and Permission Errors
# ============================================

test_volume_mount_permission_denied() {
  local test_name="Volume mount permission denied"

  local output
  output=$(cat <<'EOF'
Volume mount permission error

Problem:
  Container 'postgres' cannot write to mounted volume
  Error: Permission denied: /var/lib/postgresql/data

Fix:
  1. Check volume permissions:
     ls -la data/postgres/

  2. Fix ownership:
     sudo chown -R $(id -u):$(id -g) data/

  3. Or fix permissions:
     sudo chmod -R 755 data/

  4. For SELinux systems:
     sudo chcon -Rt svirt_sandbox_file_t data/

  5. Restart service:
     nself restart postgres

  Prevention:
    Let Docker create volumes automatically instead of
    binding to local directories for database data.
EOF
)

  assert_contains "$output" "Volume mount permission" "$test_name: Error title"
  assert_contains "$output" "chown" "$test_name: Fix ownership"
  assert_contains "$output" "chmod" "$test_name: Fix permissions"
  assert_contains "$output" "SELinux" "$test_name: SELinux case"
}

# ============================================
# Test Runner
# ============================================

run_all_tests() {
  printf "\n========================================\n"
  printf "  Service Failure Tests\n"
  printf "========================================\n\n"

  setup_test_environment

  # Port conflicts
  test_port_already_in_use
  test_multiple_port_conflicts

  # Container failures
  test_container_fails_to_start
  test_dependency_not_ready
  test_health_check_timeout
  test_missing_docker_image
  test_build_failure

  # Resource limits
  test_out_of_memory
  test_disk_full

  # Network errors
  test_network_connection_failed
  test_dns_resolution_failure

  # Runtime configuration
  test_environment_variable_missing_at_runtime

  # Permissions
  test_volume_mount_permission_denied

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
