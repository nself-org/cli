# Test Reliability Guide

> How to write bulletproof, zero-flakiness tests for nself

## Overview

This guide covers best practices for writing reliable tests that:
- **Pass 100% of the time** (no flakiness)
- **Run fast** (full suite <5 minutes)
- **Are deterministic** (same input = same output)
- **Are isolated** (don't interfere with each other)
- **Work cross-platform** (macOS, Linux, WSL)
- **Are debuggable** (clear failure messages)

## Quick Reference

```bash
# Source the reliable test framework
source src/tests/lib/reliable-test-framework.sh

# Run test with timeout protection
run_test_with_timeout my_test_function 30

# Run test with guaranteed cleanup
with_cleanup my_test_function cleanup_function

# Retry flaky operations
retry_on_failure flaky_test 3

# Use mocks instead of real services
source src/tests/mocks/docker-mock.sh
source src/tests/mocks/network-mock.sh
source src/tests/mocks/time-mock.sh
```

## Core Principles

### 1. Timeout Protection

**Always** wrap tests in timeouts to prevent hangs:

```bash
# ✅ GOOD - Test will timeout after 30s
run_test_with_timeout test_my_feature 30

# ❌ BAD - Test could hang forever
test_my_feature
```

### 2. Guaranteed Cleanup

**Always** ensure cleanup runs, even on failure:

```bash
# ✅ GOOD - Cleanup always runs
setup_resources() {
  TEST_DIR=$(mktemp -d)
  # ... setup
}

cleanup_resources() {
  rm -rf "$TEST_DIR"
}

with_cleanup test_feature cleanup_resources

# ❌ BAD - Cleanup might not run if test fails
setup_resources
test_feature
cleanup_resources  # Might not execute!
```

### 3. Test Isolation

**Never** share state between tests:

```bash
# ✅ GOOD - Unique resources per test
test_feature_a() {
  local test_dir=$(create_isolated_test_dir)
  local test_port=$(get_random_port)
  # ... test using unique resources
}

# ❌ BAD - Shared state can cause conflicts
SHARED_DIR="/tmp/tests"
test_feature_a() {
  # ... uses SHARED_DIR (collision with other tests!)
}
```

### 4. Use Mocks, Not Real Services

**Replace** external dependencies with mocks:

```bash
# ✅ GOOD - Fast, deterministic
source src/tests/mocks/docker-mock.sh
docker run --name test-container nginx
# Instant, no Docker required

# ❌ BAD - Slow, requires Docker
docker run --name test-container nginx
# 5+ seconds, requires Docker running
```

## Using the Reliable Test Framework

### Timeout Protection

```bash
source src/tests/lib/reliable-test-framework.sh

# Run test with 30s timeout
run_test_with_timeout my_test 30

# Run command with timeout and capture output
output=$(run_with_timeout_capture 10 nself build)
```

### Guaranteed Cleanup

```bash
# Cleanup always runs, even on failure/interrupt
with_cleanup test_function cleanup_function

# Create isolated test directory with auto-cleanup
test_dir=$(create_isolated_test_dir)
# ... test runs
# cleanup_isolated_test_dir called automatically on exit
```

### Retry Logic

```bash
# Retry test up to 3 times on failure
retry_on_failure flaky_test 3

# Retry until condition is met
retry_until \
  "check_service_ready" \
  "start_service" \
  10 \  # max attempts
  2     # delay between attempts
```

### Test Isolation

```bash
# Get unique resources for each test
project_name=$(get_unique_project_name "myapp")
db_name=$(get_unique_db_name "test_db")
port=$(get_random_port)

# Create isolated test directory
test_dir=$(create_isolated_test_dir)
cd "$test_dir"
# ... test runs in isolation
```

### Environment Detection

```bash
# Skip test if dependency unavailable
require_command docker || return 0
require_docker || return 0
require_network || return 0

# Skip on specific platform
skip_on_platform "macos" "This test requires Linux" || return 0

# Run only on specific platform
run_on_platform "linux" "This test only runs on Linux" || return 0
```

### Enhanced Assertions

```bash
# Assert with detailed context
assert_with_context \
  "expected_value" \
  "$actual_value" \
  "Configuration should be correct"

# Assert file contains pattern with context
assert_file_contains_with_context \
  "/etc/config.yml" \
  "database: postgres" \
  "Config should specify database"
```

### Wait Functions

```bash
# Wait for condition with timeout
wait_for_condition "service_is_ready" 30 1

# Wait for file to exist
wait_for_file "/var/log/app.log" 10

# Wait for port to be available
wait_for_port 8080 30
```

## Using Mocks

### Docker Mock

```bash
source src/tests/mocks/docker-mock.sh

# Use docker commands normally - they're mocked!
docker run --name test-app nginx
docker ps
docker stop test-app
docker rm test-app

# Cleanup mock state
cleanup_docker_mock
```

### Network Mock

```bash
source src/tests/mocks/network-mock.sh

# Register mock HTTP responses
register_mock_response \
  "https://api.example.com/users" \
  200 \
  '{"users": [{"id": 1, "name": "Alice"}]}'

# Use curl/wget normally - they return mock data!
response=$(curl -s https://api.example.com/users)

# Simulate network issues
simulate_network_delay 1000  # 1 second delay
simulate_network_timeout      # Timeout error
simulate_connection_refused   # Connection refused

# Cleanup
cleanup_network_mock
```

### Time Mock

```bash
source src/tests/mocks/time-mock.sh

# Enable time mocking
enable_time_mock

# Control time
set_mock_time 1704067200  # Set to specific timestamp
advance_time_by 60        # Fast-forward 60 seconds

# Use date/sleep normally - they use mock time!
current_time=$(date +%s)
sleep 300  # Instant! Time advances without actual delay

# Fast-forward for timeout tests
set_time_multiplier 10.0  # 10x speed
sleep 60  # Completes in 6 seconds

# Cleanup
disable_time_mock
```

### Filesystem Mock

```bash
source src/tests/mocks/filesystem-mock.sh

# Initialize mock filesystem
init_filesystem_mock

# Create files in mock filesystem
create_mock_file "/etc/app.conf" "setting=value"

# Read files from mock filesystem
content=$(read_mock_file "/etc/app.conf")

# Check existence
if mock_file_exists "/etc/app.conf"; then
  echo "File exists"
fi

# Snapshot and restore
snapshot_mock_fs "before_test"
# ... make changes
restore_mock_fs "before_test"

# Cleanup
cleanup_filesystem_mock
```

## Common Patterns

### Pattern 1: Testing Init Command

```bash
test_init_creates_env_file() {
  # Setup: isolated directory with cleanup
  local test_dir=$(create_isolated_test_dir)
  cd "$test_dir"

  # Act: run init with timeout
  run_with_timeout_capture 30 nself init

  # Assert: check results
  assert_file_exists .env "Init should create .env file"
  assert_file_permissions .env 600 ".env should be private"

  # Cleanup: automatic via create_isolated_test_dir
}
```

### Pattern 2: Testing Build with Docker Mock

```bash
test_build_creates_docker_compose() {
  source src/tests/mocks/docker-mock.sh

  local test_dir=$(create_isolated_test_dir)
  cd "$test_dir"

  # Create minimal .env
  cat > .env <<EOF
PROJECT_NAME=test-app
ENV=dev
EOF

  # Run build
  run_with_timeout_capture 30 nself build

  # Verify docker-compose.yml created
  assert_file_exists docker-compose.yml
  assert_file_contains_with_context \
    docker-compose.yml \
    "postgres:" \
    "Should include PostgreSQL service"

  # No real Docker involved!
  cleanup_docker_mock
}
```

### Pattern 3: Testing Network Operations

```bash
test_api_endpoint() {
  source src/tests/mocks/network-mock.sh

  # Setup mock API response
  register_mock_response \
    "https://api.nself.org/status" \
    200 \
    '{"status": "ok", "version": "1.0.0"}'

  # Test function that calls API
  local result=$(check_api_status)

  # Assert
  assert_with_context "ok" "$result" "API status should be ok"

  cleanup_network_mock
}
```

### Pattern 4: Testing Timeouts

```bash
test_timeout_handling() {
  source src/tests/mocks/time-mock.sh

  enable_time_mock
  set_time_multiplier 100.0  # 100x speed

  # Function that waits 60 seconds
  local start=$(get_mock_time)
  wait_for_service_ready  # Would normally take 60s
  local end=$(get_mock_time)

  # Verify time advanced (but test completed quickly)
  assert_time_advanced "$start" 60

  disable_time_mock
}
```

## Fixing Flaky Tests

### Identify Flaky Tests

```bash
# Run flakiness detector
bash scripts/find-flaky-tests.sh --iterations 10

# Output:
# SEVERELY FLAKY: test-deploy.sh (45% pass rate)
# MODERATELY FLAKY: test-tenant.sh (65% pass rate)
# SLIGHTLY FLAKY: test-auth.sh (90% pass rate)
```

### Common Causes & Fixes

#### 1. Race Conditions

```bash
# ❌ BAD - Race condition
start_service &
test_service  # Might run before service is ready

# ✅ GOOD - Wait for service
start_service &
wait_for_port 8080 30
test_service
```

#### 2. Shared Resources

```bash
# ❌ BAD - Tests share port
TEST_PORT=8080
test_a() {
  start_service $TEST_PORT  # Conflict if test_b runs concurrently
}

# ✅ GOOD - Unique port per test
test_a() {
  local port=$(get_random_port)
  start_service $port
}
```

#### 3. Timing Assumptions

```bash
# ❌ BAD - Assumes operation completes in 1s
start_long_operation
sleep 1
check_operation_result  # Might not be done yet!

# ✅ GOOD - Poll until ready
start_long_operation
wait_for_condition "operation_is_complete" 30 1
check_operation_result
```

#### 4. External Dependencies

```bash
# ❌ BAD - Depends on external service
response=$(curl https://api.example.com/data)

# ✅ GOOD - Use mock
source src/tests/mocks/network-mock.sh
register_mock_response "https://api.example.com/data" 200 '{"status":"ok"}'
response=$(curl https://api.example.com/data)
```

#### 5. Non-Deterministic Output

```bash
# ❌ BAD - Output includes timestamp
log_message "Started at $(date)"
assert_contains "$output" "Started at"  # Timestamp changes!

# ✅ GOOD - Use deterministic timestamps in tests
source src/tests/mocks/time-mock.sh
enable_time_mock
set_mock_time 1704067200
log_message "Started at $(date)"
# Always same output!
```

## Performance Optimization

### Analyze Performance

```bash
# Run performance analysis
bash scripts/test-performance-analysis.sh --save-history --show-trend

# Output:
# SLOWEST TESTS:
#  1. test-full-deploy.sh - 45s
#  2. test-docker-build.sh - 30s
#  3. test-network-ops.sh - 15s
```

### Optimization Strategies

#### 1. Use Mocks

```bash
# Before: 30s (real Docker)
docker build -t myapp .
docker run myapp

# After: <1s (mocked)
source src/tests/mocks/docker-mock.sh
docker build -t myapp .
docker run myapp
```

#### 2. Parallelize

```bash
# Before: Sequential (60s total)
test_feature_a  # 20s
test_feature_b  # 20s
test_feature_c  # 20s

# After: Parallel (20s total)
test_feature_a &
test_feature_b &
test_feature_c &
wait
```

#### 3. Cache Setup

```bash
# Before: Setup on every test
test_a() {
  setup_expensive_environment  # 10s
  # ... test
}

test_b() {
  setup_expensive_environment  # 10s
  # ... test
}

# After: Shared setup
setup_once() {
  setup_expensive_environment  # 10s once
}

test_a() {
  # ... test using pre-setup environment
}
```

#### 4. Skip Unnecessary Waits

```bash
# Before: Fixed sleep (always waits 10s)
start_service
sleep 10
test_service

# After: Poll (completes as soon as ready)
start_service
wait_for_port 8080 10  # Returns immediately when ready
test_service
```

## Cross-Platform Compatibility

### Platform Detection

```bash
platform=$(detect_platform)
# Returns: "linux", "macos", "wsl", or "unknown"

if [[ "$platform" == "macos" ]]; then
  # macOS-specific test
fi
```

### Skip on Platform

```bash
test_linux_feature() {
  # Skip if not on Linux
  run_on_platform "linux" || return 0

  # ... Linux-specific test
}
```

### Handle Missing Commands

```bash
test_with_timeout() {
  # Use timeout if available, otherwise skip or adapt
  if command -v timeout >/dev/null 2>&1; then
    timeout 30 my_command
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout 30 my_command
  else
    echo "SKIP: timeout command not available"
    return 0
  fi
}
```

## Debugging Failed Tests

### 1. Check Test Logs

```bash
# Run test with verbose output
VERBOSE=true bash src/tests/unit/test-init.sh
```

### 2. Run Test in Isolation

```bash
# Run single test function
bash -c 'source src/tests/unit/test-init.sh; test_init_creates_env_file'
```

### 3. Add Debug Output

```bash
test_feature() {
  echo "DEBUG: Starting test"
  echo "DEBUG: TEST_DIR=$TEST_DIR"

  # ... test code

  echo "DEBUG: Test completed"
}
```

### 4. Check CI Logs

- Go to GitHub Actions
- Find failed workflow
- Check "Annotations" for specific failures
- Download test artifacts for detailed logs

## Pre-Commit Testing

```bash
# Install pre-commit hook
cat > .git/hooks/pre-commit <<'EOF'
#!/bin/bash
# Run fast tests before commit
bash scripts/run-fast-tests.sh
if [ $? -ne 0 ]; then
  echo "Tests failed - commit blocked"
  exit 1
fi
EOF

chmod +x .git/hooks/pre-commit
```

## Best Practices Checklist

- [ ] Tests use `reliable-test-framework.sh`
- [ ] All tests have timeout protection
- [ ] Cleanup is guaranteed (use `with_cleanup`)
- [ ] Tests are isolated (unique resources per test)
- [ ] Mocks used instead of real services
- [ ] No `sleep` without polling
- [ ] No hardcoded timestamps
- [ ] Cross-platform compatible
- [ ] Clear failure messages
- [ ] Tests run in <5 seconds each

## Resources

- Reliable Test Framework: `src/tests/lib/reliable-test-framework.sh`
- Mocks:
  - Docker: `src/tests/mocks/docker-mock.sh`
  - Network: `src/tests/mocks/network-mock.sh`
  - Time: `src/tests/mocks/time-mock.sh`
  - Filesystem: `src/tests/mocks/filesystem-mock.sh`
- Scripts:
  - Find Flaky Tests: `scripts/find-flaky-tests.sh`
  - Performance Analysis: `scripts/test-performance-analysis.sh`
- Workflows:
  - Optimized Tests: `.github/workflows/optimized-tests.yml`

## Summary

**Golden Rules:**
1. **Use mocks, not real services**
2. **Always use timeouts**
3. **Always guarantee cleanup**
4. **Isolate test resources**
5. **Make tests deterministic**
6. **Run fast (<5s per test)**
7. **Zero flakiness tolerance**

Follow these practices and your tests will be:
- ✅ Reliable (100% pass rate)
- ✅ Fast (<5 min full suite)
- ✅ Deterministic
- ✅ Debuggable
- ✅ Cross-platform
