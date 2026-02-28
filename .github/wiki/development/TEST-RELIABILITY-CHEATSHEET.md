# Test Reliability Cheat Sheet

> Quick reference for writing bulletproof tests

## Basic Template

```bash
#!/usr/bin/env bash
source src/tests/lib/reliable-test-framework.sh
source src/tests/mocks/docker-mock.sh  # Optional

test_my_feature() {
  # Your test code here
  echo "✓ Test passed"
}

run_test_with_timeout test_my_feature 30
```

## Common Patterns

### Pattern: Test with Auto-Cleanup

```bash
test_dir="/tmp/test-$$"
mkdir -p "$test_dir"

# ... test code ...

rm -rf "$test_dir"  # Manual cleanup
```

Or use:
```bash
test_dir=$(create_isolated_test_dir)
# ... test code ...
# Cleanup automatic!
```

### Pattern: Docker Test (No Docker Needed)

```bash
source src/tests/mocks/docker-mock.sh

docker run --name test nginx  # Instant!
docker ps | grep -q "test"    # Works!
docker stop test
docker rm test
```

### Pattern: HTTP Request Test (No Network)

```bash
source src/tests/mocks/network-mock.sh

register_mock_response "https://api.example.com/status" 200 '{"ok":true}'
response=$(curl -s https://api.example.com/status)  # Instant!
echo "$response" | grep -q "ok"
```

### Pattern: Timeout Test (Instant)

```bash
source src/tests/mocks/time-mock.sh

enable_time_mock
set_time_multiplier 100.0  # 100x speed

sleep 60  # Completes in 0.6 seconds

disable_time_mock
```

### Pattern: Retry Flaky Operation

```bash
ATTEMPT=0
flaky_func() {
  ATTEMPT=$((ATTEMPT + 1))
  [[ $ATTEMPT -ge 3 ]] && return 0 || return 1
}

retry_on_failure flaky_func 5  # Retries up to 5 times
```

### Pattern: Wait for Async

```bash
# Start async operation
do_something_async &

# Wait for file
wait_for_file "/tmp/ready.txt" 10

# Wait for port
wait_for_port 8080 30
```

### Pattern: Test Isolation

```bash
# Unique resources per test
project=$(get_unique_project_name "myapp")
db=$(get_unique_db_name "testdb")
port=$(get_random_port)
```

## Quick Reference

| Task | Command |
|------|---------|
| Source framework | `source src/tests/lib/reliable-test-framework.sh` |
| Run with timeout | `run_test_with_timeout my_test 30` |
| Guaranteed cleanup | `with_cleanup test_func cleanup_func` |
| Docker mock | `source src/tests/mocks/docker-mock.sh` |
| Network mock | `source src/tests/mocks/network-mock.sh` |
| Time mock | `source src/tests/mocks/time-mock.sh` |
| Filesystem mock | `source src/tests/mocks/filesystem-mock.sh` |
| Retry flaky | `retry_on_failure test 3` |
| Wait for file | `wait_for_file "/path" 10` |
| Wait for port | `wait_for_port 8080 30` |
| Random port | `port=$(get_random_port)` |
| Unique name | `name=$(get_unique_project_name "app")` |
| Platform detect | `platform=$(detect_platform)` |

## Analysis Tools

```bash
# Find flaky tests
bash scripts/find-flaky-tests.sh --iterations 10

# Analyze performance
bash scripts/test-performance-analysis.sh

# Install pre-commit hook
bash scripts/install-pre-commit-hook.sh

# Run examples
bash src/tests/examples/reliable-test-example.sh
```

## Pre-Commit Hook

```bash
# Install
bash scripts/install-pre-commit-hook.sh

# Skip (temporary)
git commit --no-verify
```

## Common Mistakes to Avoid

❌ **Don't use real services**
```bash
docker run ...  # Slow, requires Docker
```

✅ **Use mocks**
```bash
source src/tests/mocks/docker-mock.sh
docker run ...  # Instant, no Docker
```

---

❌ **Don't use sleep without polling**
```bash
sleep 10  # Always waits 10s
```

✅ **Use wait functions**
```bash
wait_for_file "/tmp/ready" 10  # Returns as soon as ready
```

---

❌ **Don't share resources**
```bash
TEST_PORT=8080  # Collision!
```

✅ **Use unique resources**
```bash
TEST_PORT=$(get_random_port)  # Unique!
```

---

❌ **Don't forget cleanup**
```bash
setup_resources
run_test  # If fails, cleanup might not run!
cleanup_resources
```

✅ **Guarantee cleanup**
```bash
with_cleanup run_test cleanup_resources
```

## Best Practices

1. ✅ Always use timeout protection
2. ✅ Always guarantee cleanup
3. ✅ Use mocks, not real services
4. ✅ Isolate test resources
5. ✅ Poll instead of sleep
6. ✅ Make tests deterministic
7. ✅ Run fast (<5s per test)
8. ✅ Zero tolerance for flakiness

## Full Documentation

- **Quick Start:** `docs/development/TEST-RELIABILITY-Quick-Start.md`
- **Best Practices:** `docs/development/TEST-RELIABILITY-GUIDE.md`
- **Implementation:** `docs/development/TEST-RELIABILITY-IMPLEMENTATION.md`
- **Complete Summary:** `TEST-RELIABILITY-COMPLETE.md`
