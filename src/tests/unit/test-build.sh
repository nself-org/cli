#!/usr/bin/env bash
set -euo pipefail
# test-build.sh - Unit tests for build modules

# Test framework
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

# Test helpers
assert_equals() {
  local expected="$1"
  local actual="$2"
  local message="${3:-}"

  if [[ "$expected" == "$actual" ]]; then
    printf "${GREEN}✓${NC} %s\n" "$message"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC} %s\n" "$message"
    echo "  Expected: '$expected'"
    echo "  Actual: '$actual'"
    ((TESTS_FAILED++))
  fi
}

assert_true() {
  local condition="$1"
  local message="${2:-}"

  if eval "$condition"; then
    printf "${GREEN}✓${NC} %s\n" "$message"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC} %s (condition: %s)\n" "$message" "$condition"
    ((TESTS_FAILED++))
  fi
}

assert_false() {
  local condition="$1"
  local message="${2:-}"

  if ! eval "$condition"; then
    printf "${GREEN}✓${NC} %s\n" "$message"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC} %s (condition: %s)\n" "$message" "$condition"
    ((TESTS_FAILED++))
  fi
}

assert_file_exists() {
  local file="$1"
  local message="${2:-File should exist: $file}"

  if [[ -f "$file" ]]; then
    printf "${GREEN}✓${NC} %s\n" "$message"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC} %s\n" "$message"
    ((TESTS_FAILED++))
  fi
}

assert_dir_exists() {
  local dir="$1"
  local message="${2:-Directory should exist: $dir}"

  if [[ -d "$dir" ]]; then
    printf "${GREEN}✓${NC} %s\n" "$message"
    ((TESTS_PASSED++))
  else
    printf "${RED}✗${NC} %s\n" "$message"
    ((TESTS_FAILED++))
  fi
}

# Setup test environment
setup_test_env() {
  # Create temp directory
  export TEST_DIR=$(mktemp -d)
  export ORIGINAL_DIR=$(pwd)

  # Get nself root directory
  local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  export NSELF_ROOT="$(cd "$script_dir/../../.." && pwd)"

  cd "$TEST_DIR"

  # Create minimal .env
  cat >.env <<EOF
PROJECT_NAME=testproject
BASE_DOMAIN=localhost
ENV=dev
POSTGRES_ENABLED=true
NGINX_ENABLED=true
EOF

  # Source build modules using the global NSELF_ROOT
  source "$NSELF_ROOT/src/lib/build/platform.sh"
  source "$NSELF_ROOT/src/lib/build/validation.sh"
  source "$NSELF_ROOT/src/lib/build/output.sh"
}

# Cleanup test environment
cleanup_test_env() {
  cd "$ORIGINAL_DIR"
  rm -rf "$TEST_DIR"
}

# Test platform detection
test_platform_detection() {
  echo ""
  echo "Testing Platform Detection..."

  detect_build_platform

  assert_true "[[ -n \"\$PLATFORM\" ]]" "Platform should be detected"

  case "$(uname -s)" in
    Darwin*)
      assert_equals "darwin" "$PLATFORM" "Platform should be darwin on macOS"
      assert_equals "true" "$IS_MAC" "IS_MAC should be true on macOS"
      ;;
    Linux*)
      assert_equals "linux" "$PLATFORM" "Platform should be linux on Linux"
      assert_equals "true" "$IS_LINUX" "IS_LINUX should be true on Linux"
      ;;
  esac
}

# Test safe arithmetic
test_safe_arithmetic() {
  echo ""
  echo "Testing Safe Arithmetic..."

  local counter=0
  safe_increment counter
  assert_equals "1" "$counter" "Counter should increment to 1"

  safe_increment counter
  assert_equals "2" "$counter" "Counter should increment to 2"

  local result=$(safe_math "5 + 3")
  assert_equals "8" "$result" "5 + 3 should equal 8"

  result=$(safe_math "10 - 4")
  assert_equals "6" "$result" "10 - 4 should equal 6"
}

# Test CPU and memory detection
test_system_detection() {
  echo ""
  echo "Testing System Detection..."

  local cores=$(get_cpu_cores)
  assert_true "[[ \$cores -ge 1 ]]" "Should detect at least 1 CPU core"

  local memory=$(get_memory_mb)
  assert_true "[[ \$memory -ge 256 ]]" "Should detect at least 256MB memory"
}

# Test variable validation
test_variable_validation() {
  echo ""
  echo "Testing Variable Validation..."

  # Test with empty PROJECT_NAME
  unset PROJECT_NAME
  validate_environment >/dev/null 2>&1

  assert_true "[[ -n \"\$PROJECT_NAME\" ]]" "PROJECT_NAME should be set after validation"

  # Test boolean validation
  export SSL_ENABLED="yes"
  validate_boolean_vars >/dev/null 2>&1
  assert_equals "true" "$SSL_ENABLED" "Boolean 'yes' should convert to 'true'"

  export NGINX_ENABLED="0"
  validate_boolean_vars >/dev/null 2>&1
  assert_equals "false" "$NGINX_ENABLED" "Boolean '0' should convert to 'false'"
}

# Test port conflict detection
test_port_conflicts() {
  echo ""
  echo "Testing Port Conflict Detection..."

  # This test would need to be adjusted based on actual port usage
  # For now, just test the function exists and runs
  check_port_conflicts >/dev/null 2>&1
  assert_equals "0" "$?" "Port conflict check should complete"
}

# Test service dependencies
test_service_dependencies() {
  echo ""
  echo "Testing Service Dependencies..."

  # Test Hasura enables PostgreSQL
  export HASURA_ENABLED="true"
  export POSTGRES_ENABLED="false"
  validate_service_dependencies >/dev/null 2>&1

  assert_equals "true" "$POSTGRES_ENABLED" "PostgreSQL should be enabled when Hasura is enabled"

  # Test Auth enables PostgreSQL
  export AUTH_ENABLED="true"
  export POSTGRES_ENABLED="false"
  validate_service_dependencies >/dev/null 2>&1

  assert_equals "true" "$POSTGRES_ENABLED" "PostgreSQL should be enabled when Auth is enabled"
}

# Test SSL certificate generation
test_ssl_generation() {
  echo ""
  echo "Testing SSL Certificate Generation..."

  # Source SSL module
  source "$NSELF_ROOT/src/lib/build/ssl.sh"

  # Test directory creation
  generate_ssl_certificates "true" >/dev/null 2>&1

  assert_dir_exists "ssl/certificates/localhost" "SSL localhost directory should be created"
  assert_dir_exists "nginx/ssl" "Nginx SSL directory should be created"

  # Test certificate files (will be self-signed in test)
  if command -v openssl >/dev/null 2>&1; then
    assert_file_exists "ssl/certificates/localhost/fullchain.pem" "Certificate should be generated"
    assert_file_exists "ssl/certificates/localhost/privkey.pem" "Private key should be generated"
  fi
}

# Test docker-compose generation
test_docker_compose_generation() {
  echo ""
  echo "Testing Docker Compose Generation..."

  # Source docker-compose module
  source "$NSELF_ROOT/src/lib/build/docker-compose.sh"

  # Generate docker-compose.yml
  generate_docker_compose "true" >/dev/null 2>&1

  assert_file_exists "docker-compose.yml" "docker-compose.yml should be generated"

  # Check content
  if [[ -f "docker-compose.yml" ]]; then
    assert_true "grep -q 'services:' docker-compose.yml" "docker-compose.yml should contain services"
    assert_true "grep -q 'nginx' docker-compose.yml" "docker-compose.yml should contain nginx service"
    assert_true "grep -q 'postgres' docker-compose.yml" "docker-compose.yml should contain postgres service"
  fi
}

# Test nginx configuration generation
test_nginx_generation() {
  echo ""
  echo "Testing Nginx Configuration Generation..."

  # Source nginx module
  source "$NSELF_ROOT/src/lib/build/nginx.sh"

  # Generate nginx config
  generate_nginx_config "true" >/dev/null 2>&1

  assert_file_exists "nginx/nginx.conf" "nginx.conf should be generated"
  assert_file_exists "nginx/conf.d/default.conf" "default.conf should be generated"

  # Check SSL config if enabled
  if [[ "${SSL_ENABLED:-true}" == "true" ]]; then
    assert_file_exists "nginx/includes/ssl.conf" "ssl.conf should be generated"
  fi
}

# Test database initialization
test_database_init() {
  echo ""
  echo "Testing Database Initialization..."

  # Source database module
  source "$NSELF_ROOT/src/lib/build/database.sh"

  # Generate init-db.sql
  generate_database_init "true" >/dev/null 2>&1

  assert_file_exists "init-db.sql" "init-db.sql should be generated"

  # Check content
  if [[ -f "init-db.sql" ]]; then
    assert_true "grep -q 'CREATE EXTENSION' init-db.sql" "init-db.sql should contain extensions"
  fi
}

# Test output functions
test_output_functions() {
  echo ""
  echo "Testing Output Functions..."

  # Test color setup
  setup_colors
  assert_true "[[ -n \"\$COLOR_GREEN\" ]] || [[ -t 1 ]]" "Colors should be set or terminal not available"

  # Test output functions don't error
  show_info "Test info" >/dev/null 2>&1
  assert_equals "0" "$?" "show_info should work"

  show_warning "Test warning" >/dev/null 2>&1
  assert_equals "0" "$?" "show_warning should work"

  show_error "Test error" >/dev/null 2>&1
  assert_equals "0" "$?" "show_error should work"
}

# Run all tests
run_all_tests() {
  echo "================================"
  echo "Running Build Module Unit Tests"
  echo "================================"

  setup_test_env

  test_platform_detection
  test_safe_arithmetic
  test_system_detection
  test_variable_validation
  test_port_conflicts
  test_service_dependencies
  test_ssl_generation
  test_docker_compose_generation
  test_nginx_generation
  test_database_init
  test_output_functions

  cleanup_test_env

  echo ""
  echo "================================"
  echo "Test Results:"
  echo "  Passed: $TESTS_PASSED"
  echo "  Failed: $TESTS_FAILED"
  echo "  Skipped: $TESTS_SKIPPED"
  echo "================================"

  if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  run_all_tests
fi
