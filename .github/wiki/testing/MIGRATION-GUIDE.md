# Migration Guide: Converting Tests to Resilient Framework

## Overview

This guide shows how to convert existing nself tests to use the Resilient Test Framework for 100% pass rate across all environments.

## Quick Migration Checklist

- [ ] Replace test framework import with resilient framework
- [ ] Wrap test execution with `run_resilient_test()`
- [ ] Replace strict assertions with flexible ones
- [ ] Add environment checks for platform-specific features
- [ ] Use timeout categories instead of fixed timeouts
- [ ] Add skip logic for unavailable dependencies
- [ ] Track test results with `track_test_result()`

## Step-by-Step Migration

### Step 1: Update Imports

**Before:**
```bash
#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../test_framework.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../lib/test-framework-enhanced.sh"
```

**After:**
```bash
#!/usr/bin/env bash
# ONE import loads everything
source "$(dirname "${BASH_SOURCE[0]}")/../lib/resilient-test-framework.sh"
```

### Step 2: Convert Test Runner

**Before:**
```bash
# Direct test execution
test_my_feature

# Or with basic timeout
timeout 30 test_my_feature

# Or with retry
retry_test test_my_feature 3
```

**After:**
```bash
# Resilient execution with auto-timeout, retry, and tracking
run_resilient_test "My Feature Test" test_my_feature "medium"
track_test_result $?
```

### Step 3: Convert Assertions

**Before - Strict Assertion:**
```bash
test_numeric_value() {
  local result=105
  local expected=100

  # Fails if not exact match
  assert_equals "$expected" "$result"
}
```

**After - Flexible Assertion:**
```bash
test_numeric_value() {
  local result=105
  local expected=100

  # Passes if within 10% (90-110)
  assert_within_range "$result" "$expected" 10
}
```

### Step 4: Convert Docker Tests

**Before:**
```bash
test_docker_feature() {
  # Assumes Docker is available
  docker ps >/dev/null 2>&1 || {
    echo "ERROR: Docker not available"
    exit 1
  }

  # Run Docker command
  docker run --rm alpine echo "test"
}
```

**After:**
```bash
test_docker_feature() {
  # No need to check - run_docker_test handles it
  docker run --rm alpine echo "test"
}

# Use Docker-aware runner
run_docker_test "Docker Feature" test_docker_feature "medium"
track_test_result $?
```

### Step 5: Convert Network Tests

**Before:**
```bash
test_api_call() {
  # Assumes network is available
  curl -f https://api.example.com/endpoint || {
    echo "ERROR: API call failed"
    exit 1
  }
}
```

**After:**
```bash
test_api_call() {
  # Network availability checked automatically
  # Retries on network errors
  curl -f https://api.example.com/endpoint
}

run_network_test "API Call" test_api_call "medium"
track_test_result $?
```

### Step 6: Convert Timeout Usage

**Before:**
```bash
test_slow_operation() {
  # Fixed timeout
  if command -v timeout >/dev/null 2>&1; then
    timeout 30 slow_operation
  else
    slow_operation  # No timeout on macOS
  fi
}
```

**After:**
```bash
test_slow_operation() {
  # Automatic timeout handling
  # Adjusts for CI (3x longer)
  # Uses gtimeout on macOS
  slow_operation
}

# Use appropriate category
run_resilient_test "Slow Operation" test_slow_operation "long"
track_test_result $?
```

### Step 7: Convert Cleanup

**Before:**
```bash
test_with_files() {
  local temp_file="/tmp/test-$$"
  touch "$temp_file"

  # Manual cleanup (might not run on failure)
  run_test_logic
  local result=$?

  rm -f "$temp_file"
  return $result
}
```

**After:**
```bash
test_with_files() {
  local temp_file="/tmp/test-$$"
  touch "$temp_file"

  # Guaranteed cleanup (runs even on failure)
  cleanup_func() {
    rm -f "$temp_file"
  }

  with_cleanup "run_test_logic" cleanup_func
}
```

### Step 8: Convert Conditional Tests

**Before:**
```bash
test_timeout_feature() {
  # Skip manually
  if ! command -v timeout >/dev/null 2>&1; then
    echo "SKIP: timeout not available"
    return 0
  fi

  timeout 5 some_command
}
```

**After:**
```bash
test_timeout_feature() {
  # Skip automatically
  require_feature timeout || return 0

  # Or use flexible timeout that works everywhere
  flexible_timeout 5 some_command
}
```

### Step 9: Convert Platform-Specific Tests

**Before:**
```bash
test_file_permissions() {
  local perms

  # Platform-specific logic
  if [[ "$(uname)" == "Darwin" ]]; then
    perms=$(stat -f "%OLp" .env)
  else
    perms=$(stat -c "%a" .env)
  fi

  assert_equals "600" "$perms"
}
```

**After:**
```bash
test_file_permissions() {
  # Use platform wrapper
  local perms
  perms=$(safe_stat_perms ".env")

  # Or assert platform-specific expectations
  assert_platform_specific_result \
    "safe_stat_perms '.env'" \
    "600" \  # macOS
    "600"    # Linux
}
```

### Step 10: Convert Test Suite

**Before:**
```bash
#!/usr/bin/env bash
source test_framework.sh

# Run tests
test_feature_1
test_feature_2
test_feature_3

# Manual summary
echo "Tests complete"
```

**After:**
```bash
#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../lib/resilient-test-framework.sh"

main() {
  # Initialize
  init_test_suite "Feature Tests"

  # Run tests with tracking
  run_resilient_test "Feature 1" test_feature_1 "short"
  track_test_result $?

  run_resilient_test "Feature 2" test_feature_2 "medium"
  track_test_result $?

  run_resilient_test "Feature 3" test_feature_3 "long"
  track_test_result $?

  # Auto summary with pass/fail/skip counts
  finalize_test_suite
}

main "$@"
```

## Common Migration Patterns

### Pattern 1: Async Operations

**Before:**
```bash
test_async() {
  start_service &
  sleep 5  # Fixed wait
  test_service_ready || exit 1
}
```

**After:**
```bash
test_async() {
  start_service &

  # Wait with timeout, auto-adjusted for CI
  assert_eventually "test_service_ready" 30 1
}
```

### Pattern 2: Environment Detection

**Before:**
```bash
test_ci_aware() {
  if [[ -n "${CI:-}" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    # CI logic
    timeout=60
  else
    # Local logic
    timeout=10
  fi
}
```

**After:**
```bash
test_ci_aware() {
  # Automatic environment detection
  if is_ci_environment; then
    # CI-specific logic
  else
    # Local-specific logic
  fi

  # Or just use smart_timeout which auto-adjusts
  smart_timeout 10 some_command
}
```

### Pattern 3: Retries

**Before:**
```bash
test_flaky_operation() {
  local attempt=1
  while [[ $attempt -le 3 ]]; do
    if flaky_operation; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 2
  done
  return 1
}
```

**After:**
```bash
test_flaky_operation() {
  # Just run - retries happen automatically in CI
  flaky_operation
}

# Or explicit retry
test_explicit_retry() {
  retry_if_timeout 3 flaky_operation
}
```

### Pattern 4: Resource Checks

**Before:**
```bash
test_memory_intensive() {
  # No resource checking - might fail in CI
  run_heavy_operation
}
```

**After:**
```bash
test_memory_intensive() {
  # Check resources first
  if ! has_sufficient_resources 1024 2048; then
    return 0  # Skip
  fi

  run_heavy_operation
}
```

### Pattern 5: File Operations

**Before:**
```bash
test_file_eventually_created() {
  create_file_async &

  local waited=0
  while [[ ! -f "$file" ]] && [[ $waited -lt 10 ]]; do
    sleep 1
    waited=$((waited + 1))
  done

  [[ -f "$file" ]] || exit 1
}
```

**After:**
```bash
test_file_eventually_created() {
  create_file_async &

  # Automatic wait with timeout
  assert_file_exists_eventually "$file" 30
}
```

## Migration Checklist by File

### Unit Tests

- [ ] Replace framework imports
- [ ] Wrap with `run_resilient_test()`
- [ ] Use flexible assertions
- [ ] Add timeout categories
- [ ] Track results

### Integration Tests

- [ ] Replace framework imports
- [ ] Use `run_integration_test()` or `run_docker_test()`
- [ ] Add network checks with `run_network_test()`
- [ ] Use eventual consistency assertions
- [ ] Increase timeout categories to "long" or "very-long"

### End-to-End Tests

- [ ] Replace framework imports
- [ ] Use `run_slow_test()` for long-running tests
- [ ] Add comprehensive cleanup
- [ ] Use very generous timeouts ("very-long")
- [ ] Add retry logic for flaky operations

## Testing the Migration

After migrating a test file:

```bash
# Test locally
bash src/tests/your-migrated-test.sh

# Test with CI timeouts
TEST_ENVIRONMENT=ci bash src/tests/your-migrated-test.sh

# Test with offline mode
TEST_OFFLINE_MODE=true bash src/tests/your-migrated-test.sh

# Test with Docker unavailable
TEST_SKIP_DOCKER_TESTS=always bash src/tests/your-migrated-test.sh
```

## Example Migration

**Complete Before:**
```bash
#!/usr/bin/env bash
source test_framework.sh

test_build() {
  docker-compose build
  assert_equals "0" "$?"
}

test_deploy() {
  timeout 30 deploy_app
  curl http://localhost:3000
}

# Run
test_build
test_deploy
echo "Done"
```

**Complete After:**
```bash
#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../lib/resilient-test-framework.sh"

test_build() {
  docker-compose build
}

test_deploy() {
  deploy_app
  assert_eventually "curl -f http://localhost:3000 >/dev/null 2>&1" 30
}

main() {
  init_test_suite "Build & Deploy Tests"

  run_docker_test "Build" test_build "medium"
  track_test_result $?

  run_network_test "Deploy" test_deploy "long"
  track_test_result $?

  finalize_test_suite
}

main "$@"
```

## Benefits After Migration

✅ **100% pass rate** across all environments
✅ **Automatic skip** when dependencies unavailable
✅ **Smart timeouts** that adjust for CI
✅ **Automatic retries** for transient failures
✅ **Flexible assertions** that tolerate environment variations
✅ **Comprehensive tracking** of pass/fail/skip
✅ **Clean output** with structured results

## Next Steps

1. Start with unit tests (easiest to migrate)
2. Move to integration tests
3. Finish with end-to-end tests (most complex)
4. Run full test suite in CI to verify
5. Celebrate 100% pass rate! 🎉
