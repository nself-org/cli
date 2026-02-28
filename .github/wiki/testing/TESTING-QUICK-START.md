# Testing Quick Start Guide

## Overview

This guide helps you quickly create reliable, fast tests for nself using our enhanced test framework.

## Prerequisites

```bash
# Ensure test infrastructure is in place
ls src/tests/lib/test-framework-enhanced.sh
ls src/tests/mocks/mock-infrastructure.sh

# Make scripts executable
chmod +x scripts/*.sh
chmod +x src/tests/unit/**/*.sh
```

## Quick Start (3 Steps)

### 1. Generate Test Stubs

```bash
# Generate stubs for all untested files
./scripts/generate-missing-tests.sh

# This creates test skeleton files you can fill in
```

### 2. Implement Tests

Edit the generated test file and replace `skip_test "Not implemented yet"` with actual tests.

**Example:** `src/tests/unit/cli/test-version.sh`

```bash
test_version_display() {
  printf "Test: version command displays version\n"

  local output
  output=$(bash "$PROJECT_ROOT/src/cli/version.sh" 2>&1)

  assert_success
  assert_contains "$output" "nself version"
  assert_contains "$output" "0.9"
}
```

### 3. Run Tests

```bash
# Run specific test
./src/tests/unit/cli/test-version.sh

# Run all unit tests
find src/tests/unit -name "test-*.sh" -exec {} \;

# Check coverage
./scripts/coverage-report.sh
```

---

## Test Template Patterns

### Pattern 1: Simple Unit Test

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source enhanced framework
source "$SCRIPT_DIR/../../lib/test-framework-enhanced.sh"

test_simple_function() {
  printf "Test: simple function works\n"

  local result
  result=$(my_function "input")

  assert_equals "expected_output" "$result"
}

main() {
  local failed=0
  run_and_track_test "simple_function" test_simple_function || failed=$((failed + 1))
  print_test_summary
  return $failed
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

### Pattern 2: Test with Setup/Teardown

```bash
setup_test() {
  # Create isolated environment
  TEST_DIR=$(create_test_tmpfs "my-test")
  ensure_cleanup "rm -rf '$TEST_DIR'"
  cd "$TEST_DIR" || exit 1
}

teardown_test() {
  run_all_cleanups
}

test_with_cleanup() {
  printf "Test: creates files and cleans up\n"

  touch "$TEST_DIR/test.txt"
  assert_file_exists "$TEST_DIR/test.txt"

  # Cleanup runs automatically
}

main() {
  local failed=0

  setup_test
  run_and_track_test "with_cleanup" test_with_cleanup || failed=$((failed + 1))
  teardown_test

  print_test_summary
  return $failed
}
```

### Pattern 3: Test with Timeout

```bash
test_slow_operation() {
  printf "Test: slow operation completes within timeout\n"

  local result
  result=$(run_test_with_timeout "slow_operation" 10)

  assert_success
}

slow_operation() {
  sleep 2
  printf "completed\n"
}
```

### Pattern 4: Test with Retry

```bash
test_flaky_network() {
  printf "Test: network call with retry\n"

  retry_test check_network 3
  assert_success
}

check_network() {
  # Might fail occasionally - will retry up to 3 times
  curl -sSf https://api.example.com/health
}
```

### Pattern 5: Test with Mocks

```bash
test_docker_operation() {
  printf "Test: Docker operation (with mock fallback)\n"

  # Use real Docker if available, otherwise mock
  if ! has_real_docker; then
    alias docker=mock_docker
  fi

  local result
  result=$(docker ps)

  assert_success
  assert_contains "$result" "CONTAINER"
}
```

### Pattern 6: Skip Test When Unavailable

```bash
test_requires_docker() {
  printf "Test: requires Docker\n"

  if ! docker ps >/dev/null 2>&1; then
    skip_test "Docker not available"
    return 0
  fi

  # Actual test
  docker run --rm alpine echo "test"
  assert_success
}
```

### Pattern 7: Expect Failure

```bash
test_invalid_input_fails() {
  printf "Test: invalid input should fail\n"

  expect_failure validate_input "invalid@@@"
  assert_success
}

validate_input() {
  local input="$1"
  [[ "$input" =~ ^[a-zA-Z0-9]+$ ]] || return 1
}
```

---

## Enhanced Framework Features

### Assertions

```bash
# Success/failure
assert_success                    # Last command succeeded
assert_failure                    # Last command failed

# Equality
assert_equals "expected" "$actual"
assert_not_equals "not_this" "$actual"

# String matching
assert_contains "$haystack" "needle"
assert_not_contains "$haystack" "needle"

# File system
assert_file_exists "/path/to/file"
assert_file_not_exists "/path/to/file"
assert_dir_exists "/path/to/dir"
assert_dir_not_exists "/path/to/dir"
```

### Test Execution

```bash
# Timeout protection
run_test_with_timeout "my_test" 30    # 30 second timeout

# Retry flaky tests
retry_test "network_test" 3           # Retry up to 3 times
retry_with_backoff "api_test" 3 1    # With exponential backoff

# Run in isolation
run_isolated_test "my_test"          # Isolated temp directory

# Measure performance
measure_test_time "performance_test"
benchmark_test "speed_test" 10       # Run 10 times, average
```

### Cleanup Management

```bash
# Register cleanup function
ensure_cleanup "rm -rf /tmp/test-data"
ensure_cleanup "docker rm -f test_container"

# Multiple cleanups run in reverse order (LIFO)
ensure_cleanup "cleanup_step_1"
ensure_cleanup "cleanup_step_2"
ensure_cleanup "cleanup_step_3"

# All run automatically on EXIT, INT, TERM
```

### Environment Detection

```bash
# Platform detection
if is_macos; then
  # macOS-specific test
fi

if is_linux; then
  # Linux-specific test
fi

if is_wsl; then
  # WSL-specific test
fi

if is_ci; then
  # CI environment - may need to skip certain tests
fi

# Get platform name
platform=$(get_platform)  # "macos", "linux", "wsl", "unknown"
```

### Mock Infrastructure

```bash
# Source mocks
source "$SCRIPT_DIR/../../mocks/mock-infrastructure.sh"

# Use Docker mock
if ! has_real_docker; then
  alias docker=mock_docker
fi

# Use network mock
MOCK_HTTP_RESPONSE='{"status":"ok"}'
result=$(mock_curl "https://api.example.com")

# Use controllable time
MOCK_TIME=$(date +%s)
current=$(mock_date +%s)
advance_mock_time 60  # Advance by 60 seconds
later=$(mock_date +%s)

# Deterministic random
MOCK_RANDOM_SEED=12345
value=$(mock_random 100)  # Same value every time

# Fast temp directory (tmpfs on Linux)
test_dir=$(create_test_tmpfs "my-test")
```

---

## Complete Test Example

**File:** `src/tests/unit/cli/test-doctor.sh`

```bash
#!/usr/bin/env bash
# Comprehensive test for doctor command
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

source "$SCRIPT_DIR/../../lib/test-framework-enhanced.sh"
source "$SCRIPT_DIR/../../mocks/mock-infrastructure.sh"

CLI_COMMAND="$PROJECT_ROOT/src/cli/doctor.sh"

setup_test() {
  TEST_DIR=$(create_test_tmpfs "test-doctor")
  ensure_cleanup "rm -rf '$TEST_DIR'"
  cd "$TEST_DIR" || exit 1

  # Create minimal .env
  cat > .env <<EOF
PROJECT_NAME=test-project
POSTGRES_PASSWORD=test-pass
HASURA_GRAPHQL_ADMIN_SECRET=test-secret
EOF
}

teardown_test() {
  run_all_cleanups
}

test_doctor_command_exists() {
  printf "Test: doctor command exists\n"
  assert_file_exists "$CLI_COMMAND"
}

test_doctor_checks_docker() {
  printf "Test: doctor checks Docker availability\n"

  if ! has_real_docker; then
    skip_test "Docker not available, using mock"
    alias docker=mock_docker
  fi

  local output
  output=$(bash "$CLI_COMMAND" 2>&1 || true)

  # Should check for Docker
  assert_contains "$output" "Docker"
}

test_doctor_checks_env_file() {
  printf "Test: doctor checks .env file\n"

  local output
  output=$(bash "$CLI_COMMAND" 2>&1 || true)

  # Should mention environment file
  assert_contains "$output" "env" || assert_contains "$output" "ENV"
}

test_doctor_with_timeout() {
  printf "Test: doctor completes within timeout\n"

  run_test_with_timeout "bash '$CLI_COMMAND' >/dev/null 2>&1" 30
  # Don't assert success - command may fail checks, but shouldn't hang
}

test_doctor_error_handling() {
  printf "Test: doctor handles missing dependencies\n"

  # Temporarily hide Docker
  alias docker='false'

  local output
  output=$(bash "$CLI_COMMAND" 2>&1 || true)

  # Should report issue but not crash
  assert_contains "$output" "not found" || assert_contains "$output" "missing"

  unalias docker 2>/dev/null || true
}

main() {
  printf "\n"
  printf "=%.0s" {1..80}
  printf "\n"
  printf "Testing: doctor command\n"
  printf "=%.0s" {1..80}
  printf "\n\n"

  local failed=0

  setup_test
  run_and_track_test "command_exists" test_doctor_command_exists || failed=$((failed + 1))
  teardown_test

  setup_test
  run_and_track_test "checks_docker" test_doctor_checks_docker || failed=$((failed + 1))
  teardown_test

  setup_test
  run_and_track_test "checks_env_file" test_doctor_checks_env_file || failed=$((failed + 1))
  teardown_test

  setup_test
  run_and_track_test "with_timeout" test_doctor_with_timeout || failed=$((failed + 1))
  teardown_test

  setup_test
  run_and_track_test "error_handling" test_doctor_error_handling || failed=$((failed + 1))
  teardown_test

  print_test_summary
  return $failed
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
```

---

## Common Testing Scenarios

### Testing CLI Commands

```bash
# Test help output
output=$(bash "$CLI_COMMAND" --help 2>&1)
assert_contains "$output" "Usage"

# Test with arguments
result=$(bash "$CLI_COMMAND" arg1 arg2 2>&1)
assert_success

# Test error conditions
output=$(bash "$CLI_COMMAND" --invalid 2>&1 || true)
assert_failure
```

### Testing Functions

```bash
# Source the module
source "$PROJECT_ROOT/src/lib/module/functions.sh"

# Test function
result=$(my_function "input")
assert_equals "expected" "$result"

# Test with environment variables
export MY_VAR="test"
result=$(function_using_env)
assert_contains "$result" "test"
```

### Testing Docker Operations

```bash
# Use real Docker if available
if has_real_docker; then
  docker run --rm alpine echo "test"
else
  # Use mock
  alias docker=mock_docker
  result=$(docker ps)
fi

assert_success
```

### Testing File Operations

```bash
# Create test file
echo "content" > "$TEST_DIR/test.txt"
assert_file_exists "$TEST_DIR/test.txt"

# Read and verify
content=$(cat "$TEST_DIR/test.txt")
assert_equals "content" "$content"

# Cleanup happens automatically
```

### Testing Network Calls

```bash
# With retry for flaky connections
retry_test check_api 3

check_api() {
  curl -sSf https://api.example.com/health
}

# Or use mock
MOCK_HTTP_RESPONSE='{"healthy":true}'
result=$(mock_curl "https://api.example.com/health")
assert_contains "$result" "healthy"
```

---

## Best Practices

### 1. Test One Thing Per Test

❌ **Bad:**
```bash
test_everything() {
  # Tests multiple things
  test_function_a
  test_function_b
  test_function_c
}
```

✅ **Good:**
```bash
test_function_a() {
  # Tests only function_a
}

test_function_b() {
  # Tests only function_b
}
```

### 2. Use Descriptive Names

❌ **Bad:**
```bash
test_1() { ... }
test_2() { ... }
```

✅ **Good:**
```bash
test_version_displays_correct_format() { ... }
test_version_handles_missing_file() { ... }
```

### 3. Always Clean Up

❌ **Bad:**
```bash
test_creates_files() {
  touch /tmp/test-file
  # No cleanup!
}
```

✅ **Good:**
```bash
test_creates_files() {
  local test_file="/tmp/test-file-$$"
  ensure_cleanup "rm -f '$test_file'"
  touch "$test_file"
}
```

### 4. Skip Gracefully

❌ **Bad:**
```bash
test_requires_docker() {
  docker ps  # Fails hard if Docker unavailable
}
```

✅ **Good:**
```bash
test_requires_docker() {
  if ! has_real_docker; then
    skip_test "Docker not available"
    return 0
  fi
  docker ps
}
```

### 5. Use Timeouts for Slow Tests

❌ **Bad:**
```bash
test_might_hang() {
  long_running_operation  # Could hang forever
}
```

✅ **Good:**
```bash
test_might_hang() {
  run_test_with_timeout "long_running_operation" 30
}
```

---

## Troubleshooting

### Tests Timeout

```bash
# Increase timeout
run_test_with_timeout "slow_test" 60  # 60 seconds

# Or skip in CI
if is_ci; then
  skip_test "Too slow for CI"
fi
```

### Tests Are Flaky

```bash
# Add retry logic
retry_test "flaky_test" 3

# Or with exponential backoff
retry_with_backoff "network_test" 3 1
```

### Docker Not Available

```bash
# Use mock
if ! has_real_docker; then
  alias docker=mock_docker
fi
```

### File Permission Errors

```bash
# Use isolated test directory
TEST_DIR=$(create_test_tmpfs "my-test")
# Has correct permissions automatically
```

---

## Next Steps

1. **Generate stubs:** `./scripts/generate-missing-tests.sh`
2. **Implement tests:** Edit generated files, replace `skip_test`
3. **Run tests:** `./src/tests/unit/cli/test-*.sh`
4. **Check coverage:** `./scripts/coverage-report.sh`
5. **Iterate:** Fix failures, add more tests, repeat

## Resources

- **Full Plan:** `docs/testing/100-PERCENT-COVERAGE-PLAN.md`
- **Framework:** `src/tests/lib/test-framework-enhanced.sh`
- **Mocks:** `src/tests/mocks/mock-infrastructure.sh`
- **Examples:** `src/tests/unit/` and `src/tests/integration/`

---

**Happy Testing!** 🧪
