# Branch Coverage Guide

Complete guide to achieving and maintaining 100% branch coverage in nself.

## Table of Contents

- [Overview](#overview)
- [What is Branch Coverage?](#what-is-branch-coverage)
- [Why 100% Coverage?](#why-100-coverage)
- [Branch Types](#branch-types)
- [Testing Patterns](#testing-patterns)
- [Tools & Scripts](#tools--scripts)
- [CI/CD Integration](#cicd-integration)
- [Best Practices](#best-practices)

## Overview

Branch coverage measures whether every possible execution path (branch) in your code has been tested. Unlike line coverage, which only checks if a line was executed, branch coverage ensures that BOTH the true and false paths of every conditional have been tested.

**Current Status:**
```bash
# Check current coverage
bash scripts/branch-coverage-analysis.sh

# See untested branches
bash scripts/show-untested-branches.sh
```

## What is Branch Coverage?

### Simple If/Else Example

```bash
# Code with 2 branches
if [[ "$ENV" == "prod" ]]; then
  deploy_to_production  # Branch 1
else
  deploy_to_dev         # Branch 2
fi
```

**100% branch coverage requires:**
- ✅ Test with `ENV=prod` (Branch 1)
- ✅ Test with `ENV=dev` (Branch 2)

### Case Statement Example

```bash
# Code with 4 branches
case "$command" in
  start)   cmd_start ;;     # Branch 1
  stop)    cmd_stop ;;      # Branch 2
  restart) cmd_restart ;;   # Branch 3
  *)       show_help ;;     # Branch 4 (default)
esac
```

**100% branch coverage requires:**
- ✅ Test with `command=start`
- ✅ Test with `command=stop`
- ✅ Test with `command=restart`
- ✅ Test with `command=invalid` (default case)

### Logical Operator Example

```bash
# Code with 3 branches
if command -v docker >/dev/null && command -v docker-compose >/dev/null; then
  proceed  # Branch 1: Both true
else
  error    # Branch 2: First false OR Branch 3: Second false
fi
```

**100% branch coverage requires:**
- ✅ Test with both commands available (Branch 1)
- ✅ Test with first command missing (Branch 2 - short-circuit)
- ✅ Test with second command missing (Branch 3)

## Why 100% Coverage?

### Benefits

1. **Catch Hidden Bugs**
   - Untested branches often contain bugs
   - Edge cases are easy to miss
   - Error handling paths need testing

2. **Prevent Regressions**
   - Changes that break code paths are caught immediately
   - Refactoring is safer
   - Confidence in deployments

3. **Documentation**
   - Tests document expected behavior
   - Show how code handles different scenarios
   - Examples for future developers

4. **Platform Compatibility**
   - Test macOS vs Linux paths
   - Handle missing optional commands
   - Graceful degradation

### Common Untested Branches

❌ **Error handling paths**
```bash
if check_something; then
  proceed
else
  # This path is often untested!
  handle_error
fi
```

❌ **Platform-specific code**
```bash
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS path - might not be tested on Linux CI
  use_bsd_commands
else
  # Linux path - might not be tested on macOS
  use_gnu_commands
fi
```

❌ **Optional command availability**
```bash
if command -v timeout >/dev/null; then
  timeout 30 operation
else
  # Fallback path - often untested
  operation
fi
```

## Branch Types

### 1. If/Else Statements

```bash
# Pattern
if [[ condition ]]; then
  true_branch
else
  false_branch
fi

# Test template
test_if_else_both_branches() {
  # Test TRUE path
  setup_condition_true
  run_code
  assert_true_branch_executed

  # Test FALSE path
  setup_condition_false
  run_code
  assert_false_branch_executed
}
```

### 2. Case Statements

```bash
# Pattern
case "$var" in
  option1) branch1 ;;
  option2) branch2 ;;
  option3) branch3 ;;
  *)       default ;;
esac

# Test template
test_case_all_branches() {
  for option in option1 option2 option3 invalid; do
    var="$option"
    run_code
    assert_correct_branch_executed
  done
}
```

### 3. Logical AND (&&)

```bash
# Pattern
if cmd1 && cmd2; then
  both_true
else
  one_or_both_false
fi

# Test template
test_and_operator() {
  # Both true
  mock_cmd1 "true"
  mock_cmd2 "true"
  assert_both_true

  # First false (short-circuit)
  mock_cmd1 "false"
  assert_one_or_both_false

  # Second false
  mock_cmd1 "true"
  mock_cmd2 "false"
  assert_one_or_both_false
}
```

### 4. Logical OR (||)

```bash
# Pattern
if cmd1 || cmd2; then
  at_least_one_true
else
  both_false
fi

# Test template
test_or_operator() {
  # First true (short-circuit)
  mock_cmd1 "true"
  assert_at_least_one_true

  # Second true
  mock_cmd1 "false"
  mock_cmd2 "true"
  assert_at_least_one_true

  # Both false (graceful degradation)
  mock_cmd1 "false"
  mock_cmd2 "false"
  assert_both_false_handled_gracefully
}
```

### 5. Return Paths

```bash
# Pattern
function_with_returns() {
  if [[ error ]]; then
    return 1
  elif [[ skip ]]; then
    return 2
  else
    return 0
  fi
}

# Test template
test_return_paths() {
  # Success path
  run_function && result=$? || result=$?
  assert_equals 0 "$result"

  # Error path
  setup_error
  run_function && result=$? || result=$?
  assert_equals 1 "$result"

  # Skip path
  setup_skip
  run_function && result=$? || result=$?
  assert_equals 2 "$result"
}
```

## Testing Patterns

### Pattern 1: Platform-Specific Branches

**Code:**
```bash
if [[ "$OSTYPE" == "darwin"* ]]; then
  use_bsd_stat
else
  use_gnu_stat
fi
```

**Test:**
```bash
test_platform_specific() {
  # Test macOS
  mock_platform "macos"
  run_code
  assert_bsd_stat_used

  # Test Linux
  mock_platform "linux"
  run_code
  assert_gnu_stat_used
}
```

### Pattern 2: Command Availability

**Code:**
```bash
if command -v timeout >/dev/null 2>&1; then
  timeout 30 operation
else
  operation  # No timeout available
fi
```

**Test:**
```bash
test_optional_command() {
  # WITH timeout
  mock_command_exists "timeout" "true"
  run_code
  assert_timeout_used

  # WITHOUT timeout (graceful degradation)
  mock_command_exists "timeout" "false"
  run_code
  assert_success  # Still succeeds!
}
```

### Pattern 3: Error Handling

**Code:**
```bash
if docker info >/dev/null 2>&1; then
  start_containers
else
  show_docker_not_running_error
fi
```

**Test:**
```bash
test_error_handling() {
  # Success path
  mock_docker_running "true"
  run_code
  assert_containers_started

  # Error path (handled gracefully)
  mock_docker_running "false"
  run_code
  assert_error_message_shown
  # Test PASSES - error was handled
}
```

### Pattern 4: Graceful Degradation

**Code:**
```bash
if [[ -n "$REDIS_URL" ]] && redis_available; then
  use_redis_cache
else
  use_memory_cache  # Fallback
fi
```

**Test:**
```bash
test_graceful_degradation() {
  # With Redis
  mock_redis_available "true"
  run_code
  assert_redis_used

  # Without Redis (degrade gracefully)
  mock_redis_available "false"
  run_code
  assert_memory_cache_used
  assert_success  # Still works!
}
```

### Pattern 5: Nested Conditionals

**Code:**
```bash
if [[ "$ENV" == "prod" ]]; then
  if confirm_deployment; then
    deploy
  else
    cancel
  fi
else
  deploy_without_confirmation
fi
```

**Test:**
```bash
test_nested_branches() {
  # Path 1: prod + confirmed
  ENV="prod" confirm="yes"
  run_code
  assert_deployed

  # Path 2: prod + cancelled
  ENV="prod" confirm="no"
  run_code
  assert_cancelled

  # Path 3: non-prod
  ENV="dev"
  run_code
  assert_deployed_without_confirmation
}
```

## Tools & Scripts

### 1. Branch Coverage Analysis

```bash
# Analyze all branches in codebase
bash scripts/branch-coverage-analysis.sh
```

**Output:**
```
=== Branch Coverage Analysis ===

Analyzing if/else statements...
  Found 245 if statements (490 branches)

Analyzing case statements...
  Found 38 case statements (156 branches)

Analyzing logical operators...
  Found 89 && operators
  Found 67 || operators
  Total logical branches: 312

=== Branch Coverage Summary ===
Total Branches: 1,253
Tested Branches: 752
Coverage: 60%

Goal: 100% branch coverage
Remaining: 501 branches to test
```

### 2. Show Untested Branches

```bash
# List specific untested branches
bash scripts/show-untested-branches.sh
```

**Output:**
```
=== Untested Branches Analysis ===

Source Files Without Test Coverage:
  ✗ src/lib/config/vault.sh
  ✗ src/cli/whitelabel.sh

  Total: 2 files without tests

Potential Untested Conditionals:
  ✗ src/lib/init/wizard.sh - 12 if statements (no tests)
      Line 45: if [[ "$mode" == "simple" ]]; then
      Line 67: if validate_domain "$domain"; then
      Line 89: if check_port_available "$port"; then
      ... and 9 more
```

### 3. Environment Control Mocks

```bash
# Source mocking infrastructure
source src/tests/mocks/environment-control.sh

# Mock platform
mock_platform "macos"

# Mock command availability
mock_command_exists "timeout" "false"

# Mock Docker state
mock_docker_running "true"

# Mock file existence
mock_file_exists ".env" "true" "ENV=dev"

# Save and restore environment
env_save=$(save_environment)
# ... make changes ...
restore_environment "$env_save"
```

### 4. Branch Coverage Test Template

```bash
# Use template for new tests
cp src/tests/unit/test-branch-coverage-template.sh \
   src/tests/unit/test-my-module-branch-coverage.sh

# Edit to test your module
vim src/tests/unit/test-my-module-branch-coverage.sh
```

## CI/CD Integration

### GitHub Actions Workflow

```yaml
# .github/workflows/branch-coverage.yml
name: Branch Coverage Check

on:
  push:
    paths:
      - 'src/lib/**/*.sh'
      - 'src/tests/**/*.sh'

jobs:
  check-coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run coverage analysis
        run: bash scripts/branch-coverage-analysis.sh

      - name: Check threshold
        run: |
          COVERAGE=$(jq -r '.coverage_percent' .coverage/branch-coverage.json)
          if (( $(echo "$COVERAGE < 80" | bc -l) )); then
            echo "Coverage ${COVERAGE}% below 80% threshold"
            exit 1
          fi
```

### Enforcing Coverage

```bash
# In your pre-commit hook
COVERAGE=$(bash scripts/branch-coverage-analysis.sh | grep "Coverage:" | awk '{print $2}' | tr -d '%')

if (( COVERAGE < 80 )); then
  echo "❌ Branch coverage ${COVERAGE}% is below 80%"
  echo "Run: bash scripts/show-untested-branches.sh"
  exit 1
fi
```

## Best Practices

### 1. Test All Branches Immediately

When writing code with conditionals, write tests for ALL branches immediately:

```bash
# ❌ DON'T write code and test later
if [[ condition ]]; then
  do_something
else
  do_something_else
fi
# TODO: Test this later

# ✅ DO write tests immediately
test_both_branches() {
  # True path
  setup_true
  assert_something

  # False path
  setup_false
  assert_something_else
}
```

### 2. Mock for Control

Use mocks to force specific code paths:

```bash
# ✅ Control which branch executes
test_platform_specific() {
  # Force macOS path
  OSTYPE="darwin22.0" run_code

  # Force Linux path
  OSTYPE="linux-gnu" run_code
}
```

### 3. Never Fail on Environment

Tests should pass on all platforms:

```bash
# ❌ DON'T fail on missing optional commands
test_timeout() {
  timeout 5 operation  # Fails on macOS!
  assert_success
}

# ✅ DO handle gracefully
test_timeout() {
  if command -v timeout >/dev/null; then
    timeout 5 operation
  else
    operation  # No timeout available
  fi
  assert_success
}
```

### 4. Test Error Paths

Error handling is code too - test it:

```bash
# ✅ Test that errors are handled properly
test_error_handling() {
  # Trigger error
  mock_docker_running "false"

  # Should handle gracefully (not crash)
  run_code 2>&1 | grep "Docker not running"

  # Test PASSES because error was handled
  assert_success
}
```

### 5. Document Expected Behavior

Each test documents how code behaves:

```bash
test_deployment_confirmation() {
  # DOCUMENTS: Production requires confirmation
  ENV="prod" CONFIRM="yes"
  run_deploy
  assert_deployed

  # DOCUMENTS: Can be cancelled
  ENV="prod" CONFIRM="no"
  run_deploy
  assert_cancelled

  # DOCUMENTS: Dev auto-deploys
  ENV="dev"
  run_deploy
  assert_deployed_without_confirmation
}
```

### 6. Incremental Coverage

Don't aim for 100% immediately:

```
Week 1: 60% coverage (basic paths)
Week 2: 70% coverage (error handling)
Week 3: 80% coverage (platform-specific)
Week 4: 90% coverage (edge cases)
Week 5: 100% coverage (complete)
```

### 7. Coverage != Quality

100% branch coverage doesn't mean bug-free:

```bash
# ✅ Has 100% branch coverage
if [[ "$x" -gt 0 ]]; then
  result="positive"
else
  result="negative"
fi

# ❌ But doesn't test x=0 edge case!
```

Use branch coverage as a **baseline**, not the goal:
- ✅ Start with 100% branch coverage
- ✅ Then add edge case tests
- ✅ Then add integration tests
- ✅ Then add property-based tests

## Roadmap

### Current (v0.9.7)
- ✅ Branch coverage analysis script
- ✅ Untested branches reporting
- ✅ Environment control mocks
- ✅ Test templates and patterns
- ✅ CI/CD workflow

### Next (v0.9.8)
- 🎯 60% branch coverage baseline
- 🎯 Coverage for all init modules
- 🎯 Coverage for critical CLI commands

### Future (v1.0)
- 🎯 80% branch coverage target
- 🎯 Coverage for all core modules
- 🎯 Automated coverage regression detection

### Long-term (v1.x)
- 🎯 100% branch coverage
- 🎯 Coverage enforcement in CI
- 🎯 Branch coverage badges

## Examples

See working examples:
- `src/tests/unit/test-branch-coverage-template.sh` - Template with 10 patterns
- `src/tests/unit/test-validation-branch-coverage.sh` - Real validation module coverage
- `src/tests/mocks/environment-control.sh` - Mock infrastructure

## Contributing

When adding new code:

1. **Write the code**
2. **Identify all branches** (if/else, case, &&, ||)
3. **Write tests for EACH branch**
4. **Run coverage analysis**
5. **Verify 100% coverage** for your module

```bash
# Add new code
vim src/lib/mymodule.sh

# Create branch coverage tests
vim src/tests/unit/test-mymodule-branch-coverage.sh

# Verify coverage
bash scripts/branch-coverage-analysis.sh
```

## Resources

- [Testing Guide](./README.md)
- [Test Framework](./RESILIENT-TEST-FRAMEWORK.md)
- CI/CD Guide
- [Platform Compatibility](../contributing/CROSS-PLATFORM-COMPATIBILITY.md)

---

**Remember:** Branch coverage is about testing ALL the paths your code can take, not just the happy path. Every untested branch is a potential bug waiting to happen!
