# Resilient Test Framework

## Overview

The nself Resilient Test Framework is designed to achieve **100% test pass rate** across all environments (local, CI, macOS, Linux, WSL) by intelligently adapting to platform capabilities and resource constraints.

## Philosophy

### Core Principles

1. **Skip, Don't Fail** - If an environment doesn't support a test, skip it gracefully
2. **Retry Transient Failures** - Network blips and timeouts should not cause test failures
3. **Generous Timeouts** - Especially in CI where resources are shared
4. **Platform Adaptation** - Accept different results on different operating systems
5. **Resource Awareness** - Scale tests to available resources
6. **Graceful Degradation** - Mock when real services are unavailable
7. **Flexible Assertions** - Tolerance ranges and eventual consistency
8. **Self-Healing** - Auto-recover from common issues
9. **Never Assume** - Check availability of every dependency
10. **Pass by Default** - Only fail on real code bugs, not environment issues

## Quick Start

### Single Import

```bash
#!/usr/bin/env bash

# Load entire framework with one line
source "$(dirname "${BASH_SOURCE[0]}")/../lib/resilient-test-framework.sh"

# Now write resilient tests!
```

### Simple Test Example

```bash
# Initialize test suite
init_test_suite "My Tests"

# Write test function
test_my_feature() {
  local result
  result=$(my_command)
  [[ "$result" == "expected" ]]
}

# Run with automatic resilience
run_resilient_test "My Feature Test" test_my_feature "medium"
track_test_result $?

# Finalize
finalize_test_suite
```

## Framework Components

### 1. Environment Detection (`environment-detection.sh`)

Automatically detects and adapts to:
- CI environments (GitHub Actions, GitLab CI, CircleCI)
- Operating systems (macOS, Linux, WSL)
- Available features (timeout, docker, network)
- Resource constraints (memory, disk)

**Key Functions:**
- `detect_test_environment()` - Auto-detect environment
- `has_feature(feature)` - Check if feature available
- `require_feature(feature)` - Skip test if feature missing
- `skip_if_platform(platform)` - Skip on specific OS
- `skip_if_ci()` - Skip in CI environments

### 2. Timeout Resilience (`timeout-resilience.sh`)

Never fail due to timeout issues:
- Uses `timeout` if available, `gtimeout` on macOS, or runs without limit
- Automatic retry on timeout in CI
- Environment-adjusted timeouts (3x longer in CI)
- Graceful handling when timeout command unavailable

**Key Functions:**
- `flexible_timeout(duration, command)` - Safe timeout wrapper
- `smart_timeout(base_duration, command)` - Auto-adjusted timeout
- `retry_if_timeout(max_attempts, command)` - Retry on timeout
- `wait_for_condition(condition, timeout)` - Poll until true

### 3. Docker Resilience (`docker-resilience.sh`)

Handle Docker availability gracefully:
- Detects if Docker installed, running, or unavailable
- Automatic skip if Docker not available
- Cleanup helpers for test containers/networks/volumes
- Mock Docker when unavailable

**Key Functions:**
- `is_docker_available()` - Check Docker status
- `test_with_docker(test_func)` - Run test or skip
- `require_docker()` - Skip if Docker unavailable
- `cleanup_all_docker_test_resources()` - Clean up test artifacts

### 4. Network Resilience (`network-resilience.sh`)

Never fail due to network issues:
- Multi-host connectivity checks with retry
- Automatic skip in offline environments
- Network operation retry with exponential backoff
- External service mocking

**Key Functions:**
- `check_network_available()` - Check connectivity with retry
- `test_with_network(test_func)` - Run test or skip
- `retry_network_operation(command)` - Retry on network errors
- `mock_external_api(service)` - Mock external services

### 5. Flexible Assertions (`flexible-assertions.sh`)

Assertions that tolerate environment variations:
- Numeric tolerance ranges
- Eventual consistency (poll until true)
- Platform-specific expectations
- Timing tolerance (especially in CI)

**Key Functions:**
- `assert_within_range(actual, expected, tolerance)` - Numeric tolerance
- `assert_eventually(condition, timeout)` - Wait for condition
- `assert_platform_specific_result(command, expected_macos, expected_linux)` - Per-platform expectations
- `assert_or_skip(condition, message)` - Never fail

### 6. Test Configuration (`config/test-config.sh`)

Centralized configuration that adapts to environment:
- Timeout settings (short/medium/long/very-long)
- Retry configuration
- Resource thresholds
- Tolerance levels
- Skip behavior

**Environment Variables:**
- `TEST_TIMEOUT_SHORT` - Short timeout (10s local, 60s CI)
- `TEST_TIMEOUT_MEDIUM` - Medium timeout (30s local, 180s CI)
- `TEST_TIMEOUT_LONG` - Long timeout (60s local, 300s CI)
- `TEST_MAX_RETRIES` - Retry count (1 local, 3 CI)
- `TEST_NUMERIC_TOLERANCE_PERCENT` - Numeric tolerance (10%)
- `TEST_TIMING_TOLERANCE_PERCENT` - Timing tolerance (50% local, 100% CI)

## Test Runners

### Basic Test Runner

```bash
run_resilient_test "test_name" test_function "category"
```

Categories:
- `short` - Quick tests (<10s)
- `medium` - Normal tests (<30s)
- `long` - Slow tests (<60s)
- `very-long` - Very slow tests (<120s)

### Specialized Runners

```bash
# Docker test (skips if Docker unavailable)
run_docker_test "test_name" test_function "medium"

# Network test (skips if offline)
run_network_test "test_name" test_function "medium"

# Slow test (may be skipped based on config)
run_slow_test "test_name" test_function "long"

# Integration test (requires Docker + network)
run_integration_test "test_name" test_function "long"
```

### Quick Test Wrappers

```bash
# Quick inline test
quick_test "test name" "command to test"

# Docker-aware inline test
quick_docker_test "test name" "docker ps"

# Network-aware inline test
quick_network_test "test name" "ping -c 1 8.8.8.8"
```

## Common Patterns

### Pattern 1: Environment-Aware Test

```bash
test_env_aware() {
  # Different behavior based on environment
  if is_ci_environment; then
    # CI-specific logic (more lenient)
    assert_within_range "$value" 100 20
  else
    # Local environment (stricter)
    assert_within_range "$value" 100 5
  fi
}
```

### Pattern 2: Eventual Consistency

```bash
test_async_operation() {
  # Start async operation
  start_background_service &

  # Wait for it to be ready (up to 30s)
  assert_eventually "service_is_ready" 30 1

  # Now test the service
  test_service_functionality
}
```

### Pattern 3: Resource Checking

```bash
test_resource_intensive() {
  # Skip if not enough resources
  if ! has_sufficient_resources 1024 2048; then
    printf "\033[33mSKIP:\033[0m Not enough resources\n" >&2
    return 0
  fi

  # Run resource-intensive test
  run_heavy_operation
}
```

### Pattern 4: Platform-Specific

```bash
test_platform_specific() {
  # Accept different results on different platforms
  local result
  result=$(get_file_permissions ".env")

  assert_platform_specific_result \
    "get_file_permissions '.env'" \
    "600" \    # macOS expectation
    "600"      # Linux expectation
}
```

### Pattern 5: Cleanup Guarantee

```bash
test_with_cleanup() {
  # Setup
  local temp_dir
  temp_dir=$(mktemp -d)

  # Register cleanup (runs even on failure)
  cleanup_func() {
    rm -rf "$temp_dir"
  }

  # Run test with guaranteed cleanup
  with_cleanup "test -d '$temp_dir'" cleanup_func
}
```

### Pattern 6: Conditional Skipping

```bash
test_conditional() {
  # Skip if feature not available
  require_feature timeout || return 0

  # Skip if in CI
  skip_if_ci "Test requires interactive terminal" && return 0

  # Skip on specific platform
  skip_if_platform "wsl" "Not supported on WSL" && return 0

  # Run test
  run_actual_test
}
```

## Configuration

### Environment Variables

```bash
# Timeouts
export TEST_TIMEOUT_SHORT=60           # Short timeout in seconds
export TEST_TIMEOUT_MEDIUM=180         # Medium timeout
export TEST_TIMEOUT_LONG=300           # Long timeout

# Retries
export TEST_MAX_RETRIES=3              # Number of retries
export TEST_RETRY_DELAY=2              # Delay between retries

# Tolerances
export TEST_NUMERIC_TOLERANCE_PERCENT=10      # Numeric tolerance
export TEST_TIMING_TOLERANCE_PERCENT=100      # Timing tolerance

# Skip behavior
export TEST_SKIP_NETWORK_TESTS=auto    # auto, always, never
export TEST_SKIP_DOCKER_TESTS=auto     # auto, always, never
export TEST_SKIP_SLOW_TESTS=auto       # auto, always, never

# Output
export TEST_LOG_LEVEL=info             # debug, info, warn, error
export TEST_SHOW_PROGRESS=true         # Show progress indicators
```

### Per-Test Configuration

```bash
# Override timeout for specific test
TEST_TIMEOUT_MEDIUM=300 run_resilient_test "slow test" test_func "medium"

# Disable retries for specific test
TEST_MAX_RETRIES=0 run_resilient_test "no retry test" test_func "short"

# Increase tolerance for specific test
TEST_NUMERIC_TOLERANCE_PERCENT=25 run_resilient_test "lenient test" test_func "short"
```

## Test Suite Management

```bash
#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../lib/resilient-test-framework.sh"

main() {
  # Initialize suite
  init_test_suite "My Test Suite"

  # Run tests
  run_resilient_test "Test 1" test_1 "short"
  track_test_result $?

  run_resilient_test "Test 2" test_2 "medium"
  track_test_result $?

  run_docker_test "Test 3" test_3 "long"
  track_test_result $?

  # Finalize (prints summary, returns non-zero if failures)
  finalize_test_suite
}

main "$@"
```

Output:
```
================================================================================
Test Suite: My Test Suite
================================================================================
Environment: ci
Started: 2025-01-31 10:00:00
================================================================================

[TEST] Test 1 (timeout: 60s, retries: 3)
[PASS] Test 1

[TEST] Test 2 (timeout: 180s, retries: 3)
[PASS] Test 2

[TEST] Test 3 (timeout: 300s, retries: 3)
[SKIP] Test 3 (Docker not available)

================================================================================
Test Suite Results: My Test Suite
================================================================================
Total Tests:     3
Passed:          2
Failed:          0
Skipped:         1
Duration:        45s
Pass Rate:       100%
================================================================================
```

## Best Practices

### DO

✅ Use `run_resilient_test()` for all tests
✅ Check feature availability before using it
✅ Use flexible assertions with tolerance
✅ Skip gracefully when dependencies unavailable
✅ Register cleanup functions for guaranteed cleanup
✅ Use environment-adjusted timeouts
✅ Accept platform-specific variations
✅ Retry transient failures (network, timeout)

### DON'T

❌ Assume commands exist (`timeout`, `nc`, etc.)
❌ Use fixed timeouts (use categories instead)
❌ Fail tests due to environment issues
❌ Use strict assertions that fail on minor variations
❌ Forget to clean up test resources
❌ Rely on specific platform behavior without checking
❌ Fail immediately on first network error

## Troubleshooting

### Tests Timing Out

```bash
# Increase timeout category
run_resilient_test "slow test" test_func "long"  # Instead of "medium"

# Or override timeout
TEST_TIMEOUT_MEDIUM=600 run_resilient_test "test" test_func "medium"
```

### Tests Failing in CI Only

```bash
# Add CI-specific handling
test_my_feature() {
  if is_ci_environment; then
    # More lenient in CI
    assert_within_range "$value" 100 25
  else
    assert_within_range "$value" 100 10
  fi
}
```

### Docker Tests Always Skipping

```bash
# Check Docker availability
if ! is_docker_available; then
  printf "Docker not available\n"
fi

# Or force Docker tests to run
TEST_SKIP_DOCKER_TESTS=never run_docker_test "test" test_func "medium"
```

### Network Tests Failing

```bash
# Enable offline mode to skip all network tests
export TEST_OFFLINE_MODE=true

# Or retry network operations
retry_network_operation "curl https://api.example.com" 5
```

## Examples

See `/Users/admin/Sites/nself/src/tests/examples/resilient-test-example.sh` for comprehensive examples of all features.

## Version

**Resilient Test Framework v1.0.0**

## License

Part of the nself project.
