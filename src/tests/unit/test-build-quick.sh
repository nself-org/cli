#!/usr/bin/env bash
set -euo pipefail
# Quick test runner for build modules with better error handling

# Determine the nself root directory
if [[ -n "${GITHUB_WORKSPACE:-}" ]]; then
  NSELF_ROOT="${GITHUB_WORKSPACE:-}"
else
  # Local development - find the root by going up from test directory
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
fi

# Test framework
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
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

# Test platform detection
test_platform() {
  echo "Testing Platform Detection..."

  if source "$NSELF_ROOT/src/lib/build/platform.sh" 2>/dev/null; then
    detect_build_platform
    if [[ -n "$PLATFORM" ]]; then
      test_result "pass" "Platform detection works ($PLATFORM)"
    else
      test_result "fail" "Platform not detected"
    fi

    # Test safe arithmetic
    local counter=0
    safe_increment counter
    if [[ $counter -eq 1 ]]; then
      test_result "pass" "Safe increment works"
    else
      test_result "fail" "Safe increment failed"
    fi

    # Test system detection
    local cores=$(get_cpu_cores)
    if [[ $cores -ge 1 ]]; then
      test_result "pass" "CPU detection works ($cores cores)"
    else
      test_result "fail" "CPU detection failed"
    fi

  else
    test_result "fail" "Platform module failed to load"
  fi
}

# Test validation module
test_validation() {
  echo ""
  echo "Testing Validation Module..."

  if source "$NSELF_ROOT/src/lib/build/validation.sh" 2>/dev/null; then
    test_result "pass" "Validation module loads"

    # Test set_default function
    unset TEST_VAR
    set_default "TEST_VAR" "default_value"
    if [[ "$TEST_VAR" == "default_value" ]]; then
      test_result "pass" "set_default function works"
    else
      test_result "fail" "set_default function failed"
    fi

  else
    test_result "fail" "Validation module failed to load"
  fi
}

# Test SSL module
test_ssl() {
  echo ""
  echo "Testing SSL Module..."

  if source "$NSELF_ROOT/src/lib/build/ssl.sh" 2>/dev/null; then
    test_result "pass" "SSL module loads"

    # Test certificate path function
    if declare -f generate_ssl_certificates >/dev/null 2>&1; then
      test_result "pass" "SSL generation function exists"
    else
      test_result "fail" "SSL generation function missing"
    fi

  else
    test_result "fail" "SSL module failed to load"
  fi
}

# Test nginx module
test_nginx() {
  echo ""
  echo "Testing Nginx Module..."

  if source "$NSELF_ROOT/src/lib/build/nginx.sh" 2>/dev/null; then
    test_result "pass" "Nginx module loads"

    if declare -f generate_nginx_config >/dev/null 2>&1; then
      test_result "pass" "Nginx generation function exists"
    else
      test_result "fail" "Nginx generation function missing"
    fi

  else
    test_result "fail" "Nginx module failed to load"
  fi
}

# Test docker-compose module
test_docker_compose() {
  echo ""
  echo "Testing Docker Compose Module..."

  if source "$NSELF_ROOT/src/lib/build/docker-compose.sh" 2>/dev/null; then
    test_result "pass" "Docker Compose module loads"

    if declare -f generate_docker_compose >/dev/null 2>&1; then
      test_result "pass" "Docker Compose generation function exists"
    else
      test_result "fail" "Docker Compose generation function missing"
    fi

  else
    test_result "fail" "Docker Compose module failed to load"
  fi
}

# Test core module
test_core() {
  echo ""
  echo "Testing Core Module..."

  if source "$NSELF_ROOT/src/lib/build/core.sh" 2>/dev/null; then
    test_result "pass" "Core module loads"

    if declare -f orchestrate_build >/dev/null 2>&1; then
      test_result "pass" "Main orchestrate_build function exists"
    else
      test_result "fail" "Main orchestrate_build function missing"
    fi

    if declare -f detect_app_port >/dev/null 2>&1; then
      test_result "pass" "Port detection function exists"
    else
      test_result "fail" "Port detection function missing"
    fi

  else
    test_result "fail" "Core module failed to load"
  fi
}

# Test build wrapper
test_wrapper() {
  echo ""
  echo "Testing Build Wrapper..."

  # Check file exists first
  if [[ ! -f "$NSELF_ROOT/src/cli/build.sh" ]]; then
    test_result "fail" "Build wrapper not found"
    return
  fi

  # Executable check - skip in CI (permissions may not transfer)
  if [[ -x "$NSELF_ROOT/src/cli/build.sh" ]]; then
    test_result "pass" "Build wrapper is executable"
  elif [[ -n "${GITHUB_WORKSPACE:-}" ]] || [[ -n "${CI:-}" ]]; then
    test_result "skip" "Build wrapper executable check (CI environment)"
  else
    test_result "fail" "Build wrapper not executable"
  fi

  # Test help option - this is the real functional test
  local help_test_result=false
  if command -v timeout >/dev/null 2>&1; then
    timeout 10 bash "$NSELF_ROOT/src/cli/build.sh" --help >/dev/null 2>&1 && help_test_result=true
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout 10 bash "$NSELF_ROOT/src/cli/build.sh" --help >/dev/null 2>&1 && help_test_result=true
  else
    # No timeout available, just run the test
    bash "$NSELF_ROOT/src/cli/build.sh" --help >/dev/null 2>&1 && help_test_result=true
  fi

  if [[ "$help_test_result" == "true" ]]; then
    test_result "pass" "Build wrapper help works"
  else
    test_result "fail" "Build wrapper help failed or hung"
  fi
}

# Main test runner
echo "================================"
echo "Quick Build Module Test Runner"
echo "================================"

test_platform
test_validation
test_ssl
test_nginx
test_docker_compose
test_core
test_wrapper

echo ""
echo "================================"
echo "Test Results:"
echo "  Passed: $TESTS_PASSED"
echo "  Failed: $TESTS_FAILED"
echo "  Skipped: $TESTS_SKIPPED"
echo "================================"

if [[ $TESTS_FAILED -gt 0 ]]; then
  echo "Some tests failed. Check the output above."
  exit 1
else
  echo "All tests passed!"
  exit 0
fi
