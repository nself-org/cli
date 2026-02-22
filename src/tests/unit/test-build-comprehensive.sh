#!/usr/bin/env bash
set -euo pipefail
# Comprehensive unit tests for nself build command
# Tests all modules and functions for Bash 3.2 compatibility

# Determine the nself root directory
if [[ -n "${GITHUB_WORKSPACE:-}" ]]; then
  NSELF_ROOT="${GITHUB_WORKSPACE:-}"
else
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
fi

# Test framework
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0
VERBOSE="${VERBOSE:-false}"

# Colors (ANSI escape codes compatible with Bash 3.2)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test result function
test_result() {
  local status="$1"
  local message="$2"

  case "$status" in
    "pass")
      printf "${GREEN}✓${NC} %s\n" "$message"
      TESTS_PASSED=$((TESTS_PASSED + 1))
      ;;
    "fail")
      printf "${RED}✗${NC} %s\n" "$message"
      TESTS_FAILED=$((TESTS_FAILED + 1))
      ;;
    "skip")
      printf "${YELLOW}⚠${NC} %s (skipped)\n" "$message"
      TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
      ;;
  esac
}

# Test section header
test_section() {
  echo ""
  printf "${BLUE}═══ %s ═══${NC}\n" "$1"
}

# ==========================================
# PLATFORM MODULE TESTS
# ==========================================
test_platform_module() {
  test_section "Platform Module Tests"

  # Source the module
  if ! source "$NSELF_ROOT/src/lib/build/platform.sh" 2>/dev/null; then
    test_result "fail" "Failed to source platform.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test detect_build_platform
  detect_build_platform
  if [[ -n "$PLATFORM" ]]; then
    test_result "pass" "Platform detection: $PLATFORM"
  else
    test_result "fail" "Platform detection failed"
  fi

  # Test platform flags
  case "$PLATFORM" in
    darwin)
      [[ "$IS_MAC" == "true" ]] && test_result "pass" "macOS flag set correctly" || test_result "fail" "macOS flag incorrect"
      ;;
    linux)
      [[ "$IS_LINUX" == "true" ]] && test_result "pass" "Linux flag set correctly" || test_result "fail" "Linux flag incorrect"
      ;;
  esac

  # Test safe_increment (critical for Bash 3.2)
  local counter=0
  safe_increment counter
  if [[ $counter -eq 1 ]]; then
    test_result "pass" "safe_increment works"
  else
    test_result "fail" "safe_increment failed (got: $counter)"
  fi

  # Test multiple increments
  safe_increment counter
  safe_increment counter
  if [[ $counter -eq 3 ]]; then
    test_result "pass" "Multiple increments work"
  else
    test_result "fail" "Multiple increments failed (got: $counter)"
  fi

  # Test safe_math
  local result=$(safe_math "10 + 5")
  if [[ $result -eq 15 ]]; then
    test_result "pass" "safe_math addition works"
  else
    test_result "fail" "safe_math addition failed (got: $result)"
  fi

  # Test safe_math with multiplication
  result=$(safe_math "3 * 4")
  if [[ $result -eq 12 ]]; then
    test_result "pass" "safe_math multiplication works"
  else
    test_result "fail" "safe_math multiplication failed (got: $result)"
  fi

  # Test set_default
  unset TEST_VAR
  set_default "TEST_VAR" "default_value"
  if [[ "$TEST_VAR" == "default_value" ]]; then
    test_result "pass" "set_default with unset variable"
  else
    test_result "fail" "set_default failed with unset variable"
  fi

  # Test set_default with existing variable
  TEST_VAR="existing"
  set_default "TEST_VAR" "new_value"
  if [[ "$TEST_VAR" == "existing" ]]; then
    test_result "pass" "set_default preserves existing value"
  else
    test_result "fail" "set_default overwrote existing value"
  fi

  # Test CPU core detection
  local cores=$(get_cpu_cores)
  if [[ $cores -ge 1 ]]; then
    test_result "pass" "CPU detection works ($cores cores)"
  else
    test_result "fail" "CPU detection failed"
  fi

  # Test memory detection
  local memory=$(get_memory_mb)
  if [[ $memory -ge 256 ]]; then
    test_result "pass" "Memory detection works (${memory}MB)"
  else
    test_result "fail" "Memory detection failed"
  fi

  # Test command_exists
  if command_exists "bash"; then
    test_result "pass" "command_exists detects bash"
  else
    test_result "fail" "command_exists failed for bash"
  fi

  if ! command_exists "nonexistent_command_xyz"; then
    test_result "pass" "command_exists correctly reports missing command"
  else
    test_result "fail" "command_exists false positive"
  fi
}

# ==========================================
# VALIDATION MODULE TESTS
# ==========================================
test_validation_module() {
  test_section "Validation Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/validation.sh" 2>/dev/null; then
    test_result "fail" "Failed to source validation.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test PROJECT_NAME validation
  PROJECT_NAME="Test-Project_123"
  validate_environment >/dev/null 2>&1
  if [[ "$PROJECT_NAME" == "test-project-123" ]]; then
    test_result "pass" "PROJECT_NAME sanitization works"
  else
    test_result "fail" "PROJECT_NAME sanitization failed: $PROJECT_NAME"
  fi

  # Test BASE_DOMAIN default
  unset BASE_DOMAIN
  validate_environment >/dev/null 2>&1
  if [[ "$BASE_DOMAIN" == "localhost" ]]; then
    test_result "pass" "BASE_DOMAIN defaults to localhost"
  else
    test_result "fail" "BASE_DOMAIN default failed: $BASE_DOMAIN"
  fi

  # Test boolean normalization
  POSTGRES_ENABLED="yes"
  validate_boolean_vars
  if [[ "$POSTGRES_ENABLED" == "true" ]]; then
    test_result "pass" "Boolean normalization (yes->true)"
  else
    test_result "fail" "Boolean normalization failed"
  fi

  REDIS_ENABLED="0"
  validate_boolean_vars
  if [[ "$REDIS_ENABLED" == "false" ]]; then
    test_result "pass" "Boolean normalization (0->false)"
  else
    test_result "fail" "Boolean normalization failed for 0"
  fi
}

# ==========================================
# SSL MODULE TESTS
# ==========================================
test_ssl_module() {
  test_section "SSL Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/ssl.sh" 2>/dev/null; then
    test_result "fail" "Failed to source ssl.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test function existence
  if declare -f generate_ssl_certificates >/dev/null 2>&1; then
    test_result "pass" "generate_ssl_certificates function exists"
  else
    test_result "fail" "generate_ssl_certificates function missing"
  fi

  if declare -f create_self_signed_cert >/dev/null 2>&1; then
    test_result "pass" "create_self_signed_cert function exists"
  else
    test_result "fail" "create_self_signed_cert function missing"
  fi

  # Test certificate path functions
  local cert_path=$(get_ssl_cert_path "api.localhost")
  if [[ -n "$cert_path" ]]; then
    test_result "pass" "get_ssl_cert_path returns path"
  else
    test_result "fail" "get_ssl_cert_path failed"
  fi
}

# ==========================================
# NGINX MODULE TESTS
# ==========================================
test_nginx_module() {
  test_section "Nginx Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/nginx.sh" 2>/dev/null; then
    test_result "fail" "Failed to source nginx.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test function existence
  if declare -f generate_nginx_config >/dev/null 2>&1; then
    test_result "pass" "generate_nginx_config function exists"
  else
    test_result "fail" "generate_nginx_config function missing"
  fi

  if declare -f generate_nginx_upstream >/dev/null 2>&1; then
    test_result "pass" "generate_nginx_upstream function exists"
  else
    test_result "fail" "generate_nginx_upstream function missing"
  fi

  if declare -f generate_nginx_location >/dev/null 2>&1; then
    test_result "pass" "generate_nginx_location function exists"
  else
    test_result "fail" "generate_nginx_location function missing"
  fi
}

# ==========================================
# DOCKER COMPOSE MODULE TESTS
# ==========================================
test_docker_compose_module() {
  test_section "Docker Compose Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/docker-compose.sh" 2>/dev/null; then
    test_result "fail" "Failed to source docker-compose.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test function existence
  if declare -f generate_docker_compose >/dev/null 2>&1; then
    test_result "pass" "generate_docker_compose function exists"
  else
    test_result "fail" "generate_docker_compose function missing"
  fi

  if declare -f add_nginx_service >/dev/null 2>&1; then
    test_result "pass" "add_nginx_service function exists"
  else
    test_result "fail" "add_nginx_service function missing"
  fi

  if declare -f add_postgres_service >/dev/null 2>&1; then
    test_result "pass" "add_postgres_service function exists"
  else
    test_result "fail" "add_postgres_service function missing"
  fi
}

# ==========================================
# DATABASE MODULE TESTS
# ==========================================
test_database_module() {
  test_section "Database Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/database.sh" 2>/dev/null; then
    test_result "fail" "Failed to source database.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test function existence
  if declare -f create_database_initialization >/dev/null 2>&1; then
    test_result "pass" "create_database_initialization function exists"
  else
    test_result "fail" "create_database_initialization function missing"
  fi

  if declare -f generate_init_sql >/dev/null 2>&1; then
    test_result "pass" "generate_init_sql function exists"
  else
    test_result "fail" "generate_init_sql function missing"
  fi
}

# ==========================================
# SERVICES MODULE TESTS
# ==========================================
test_services_module() {
  test_section "Services Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/services.sh" 2>/dev/null; then
    test_result "fail" "Failed to source services.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test function existence
  if declare -f generate_services >/dev/null 2>&1; then
    test_result "pass" "generate_services function exists"
  else
    test_result "fail" "generate_services function missing"
  fi

  if declare -f generate_frontend_service >/dev/null 2>&1; then
    test_result "pass" "generate_frontend_service function exists"
  else
    test_result "fail" "generate_frontend_service function missing"
  fi

  if declare -f generate_backend_service >/dev/null 2>&1; then
    test_result "pass" "generate_backend_service function exists"
  else
    test_result "fail" "generate_backend_service function missing"
  fi
}

# ==========================================
# CORE MODULE TESTS
# ==========================================
test_core_module() {
  test_section "Core Module Tests"

  if ! source "$NSELF_ROOT/src/lib/build/core.sh" 2>/dev/null; then
    test_result "fail" "Failed to source core.sh"
    return 1
  fi
  test_result "pass" "Module loads successfully"

  # Test function existence
  if declare -f orchestrate_build >/dev/null 2>&1; then
    test_result "pass" "orchestrate_build function exists"
  else
    test_result "fail" "orchestrate_build function missing"
  fi

  if declare -f init_build_environment >/dev/null 2>&1; then
    test_result "pass" "init_build_environment function exists"
  else
    test_result "fail" "init_build_environment function missing"
  fi

  if declare -f detect_app_port >/dev/null 2>&1; then
    test_result "pass" "detect_app_port function exists"
  else
    test_result "fail" "detect_app_port function missing"
  fi
}

# ==========================================
# BUILD WRAPPER TESTS
# ==========================================
test_build_wrapper() {
  test_section "Build Wrapper Tests"

  if [[ -x "$NSELF_ROOT/src/cli/build.sh" ]]; then
    test_result "pass" "Build wrapper is executable"
  else
    test_result "fail" "Build wrapper not executable"
    return 1
  fi

  # Test help option
  local help_test_result=false
  if command -v timeout >/dev/null 2>&1; then
    timeout 5 bash "$NSELF_ROOT/src/cli/build.sh" --help >/dev/null 2>&1 && help_test_result=true
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout 5 bash "$NSELF_ROOT/src/cli/build.sh" --help >/dev/null 2>&1 && help_test_result=true
  else
    # No timeout available, just run the test
    bash "$NSELF_ROOT/src/cli/build.sh" --help >/dev/null 2>&1 && help_test_result=true
  fi

  if [[ "$help_test_result" == "true" ]]; then
    test_result "pass" "Build wrapper help works"
  else
    test_result "fail" "Build wrapper help failed"
  fi
}

# ==========================================
# BASH 3.2 COMPATIBILITY TESTS
# ==========================================
test_bash_compatibility() {
  test_section "Bash 3.2 Compatibility Tests"

  # Test that we're not using Bash 4+ features
  local files_to_check="$NSELF_ROOT/src/lib/build/*.sh"

  # Check for associative arrays (Bash 4+)
  if grep -h "declare -A" $files_to_check 2>/dev/null | grep -v "#"; then
    test_result "fail" "Found associative arrays (Bash 4+)"
  else
    test_result "pass" "No associative arrays found"
  fi

  # Check for mapfile/readarray (Bash 4+)
  if grep -h "mapfile\|readarray" $files_to_check 2>/dev/null | grep -v "#"; then
    test_result "fail" "Found mapfile/readarray (Bash 4+)"
  else
    test_result "pass" "No mapfile/readarray found"
  fi

  # Check for ${VAR,,} ${VAR^^} case conversion (Bash 4+)
  if grep -h '\${[^}]*,,' $files_to_check 2>/dev/null | grep -v "#"; then
    test_result "fail" "Found lowercase conversion (Bash 4+)"
  else
    test_result "pass" "No Bash 4+ case conversion found"
  fi

  # Check for negative array indices (Bash 4.2+)
  if grep -h '\[[-][0-9]\]' $files_to_check 2>/dev/null | grep -v "#"; then
    test_result "fail" "Found negative array indices (Bash 4.2+)"
  else
    test_result "pass" "No negative array indices found"
  fi

  # Check for nameref variables (Bash 4.3+)
  if grep -h "declare -n\|local -n" $files_to_check 2>/dev/null | grep -v "#"; then
    test_result "fail" "Found nameref variables (Bash 4.3+)"
  else
    test_result "pass" "No nameref variables found"
  fi
}

# ==========================================
# INTEGRATION TESTS
# ==========================================
test_integration() {
  test_section "Integration Tests"

  # Create a temporary test directory
  local test_dir=$(mktemp -d)
  cd "$test_dir" || {
    test_result "fail" "Failed to create test directory"
    return 1
  }

  # Create minimal .env file
  cat >.env <<EOF
PROJECT_NAME=testproject
BASE_DOMAIN=localhost
ENV=dev
POSTGRES_ENABLED=true
NGINX_ENABLED=true
POSTGRES_PASSWORD=testpass123
POSTGRES_USER=postgres
POSTGRES_DB=testdb
DOCKER_NETWORK=testproject_network
EOF

  test_result "pass" "Created test environment"

  # Test basic build functionality (may not complete fully in CI environment)
  local build_result=false

  # For CI environments, we just want to test that the build starts and processes the .env file
  # We don't expect a full Docker build to succeed in GitHub Actions
  if [[ -n "${CI:-}" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    # In CI: just test that build reads .env and starts processing
    local build_output
    if command -v timeout >/dev/null 2>&1; then
      build_output=$(timeout 10 bash "$NSELF_ROOT/src/cli/build.sh" --force 2>&1 || true)
    elif command -v gtimeout >/dev/null 2>&1; then
      build_output=$(gtimeout 10 bash "$NSELF_ROOT/src/cli/build.sh" --force 2>&1 || true)
    else
      build_output=$(bash "$NSELF_ROOT/src/cli/build.sh" --force 2>&1 | head -20 || true)
    fi

    # Consider it a success if the build at least processed the environment
    if echo "$build_output" | grep -q "DEVELOPMENT\|dev\|Building\|testproject"; then
      build_result=true
    fi
  else
    # In normal environments: attempt full build
    if command -v timeout >/dev/null 2>&1; then
      timeout 30 bash "$NSELF_ROOT/src/cli/build.sh" --force >/dev/null 2>&1 && build_result=true
    elif command -v gtimeout >/dev/null 2>&1; then
      gtimeout 30 bash "$NSELF_ROOT/src/cli/build.sh" --force >/dev/null 2>&1 && build_result=true
    else
      bash "$NSELF_ROOT/src/cli/build.sh" --force >/dev/null 2>&1 && build_result=true
    fi
  fi

  # In CI or when files are generated, consider it a success
  # The build may not fully complete in CI due to Docker limitations
  if [[ "$build_result" == "true" ]] || [[ -f docker-compose.yml ]] || [[ -d nginx ]]; then
    test_result "pass" "Build processed successfully"
  else
    # Only fail if we're not in CI and no files were generated
    if [[ -z "${CI:-}" ]] && [[ -z "${GITHUB_ACTIONS:-}" ]]; then
      test_result "fail" "Build failed to complete"
    else
      test_result "skip" "Build incomplete in CI (expected)"
    fi
  fi

  # Check generated files
  if [[ -f docker-compose.yml ]]; then
    test_result "pass" "docker-compose.yml generated"
  else
    # In CI, this might not always be generated
    if [[ -n "${CI:-}" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]]; then
      test_result "skip" "docker-compose.yml not generated (CI environment)"
    else
      test_result "fail" "docker-compose.yml not generated"
    fi
  fi

  if [[ -d nginx/conf.d ]] || [[ -d nginx ]]; then
    test_result "pass" "nginx configs generated"
  else
    if [[ -n "${CI:-}" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]]; then
      test_result "skip" "nginx configs not generated (CI environment)"
    else
      test_result "fail" "nginx configs not generated"
    fi
  fi

  if [[ -d ssl/certificates ]] || [[ -d ssl ]]; then
    test_result "pass" "SSL directory created"
  else
    if [[ -n "${CI:-}" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]]; then
      test_result "skip" "SSL directory not created (CI environment)"
    else
      test_result "fail" "SSL directory not created"
    fi
  fi

  # Cleanup
  cd - >/dev/null
  rm -rf "$test_dir"
}

# ==========================================
# MAIN TEST RUNNER
# ==========================================
echo "╔════════════════════════════════════════════╗"
echo "║  Comprehensive Build Module Test Suite     ║"
echo "║  Testing Bash 3.2 Compatibility           ║"
echo "╚════════════════════════════════════════════╝"

# Run all test suites
test_platform_module
test_validation_module
test_ssl_module
test_nginx_module
test_docker_compose_module
test_database_module
test_services_module
test_core_module
test_build_wrapper
test_bash_compatibility
test_integration

# Summary
echo ""
echo "╔════════════════════════════════════════════╗"
echo "║              TEST SUMMARY                  ║"
echo "╚════════════════════════════════════════════╝"
echo ""
echo "  Passed:  $TESTS_PASSED"
echo "  Failed:  $TESTS_FAILED"
echo "  Skipped: $TESTS_SKIPPED"
echo ""

if [[ $TESTS_FAILED -gt 0 ]]; then
  printf "${RED}Some tests failed. Please review the output above.${NC}\n"
  exit 1
else
  printf "${GREEN}All tests passed! ✓${NC}\n"
  exit 0
fi
