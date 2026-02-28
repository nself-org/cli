# Branch Coverage Quick Reference

**One-page cheat sheet for branch coverage testing in nself**

## Quick Commands

```bash
# Analyze coverage
bash scripts/branch-coverage-analysis.sh

# Show untested branches
bash scripts/show-untested-branches.sh

# Run template tests
bash src/tests/unit/test-branch-coverage-template.sh

# Run validation tests (example)
bash src/tests/unit/test-validation-branch-coverage.sh

# View reports
cat .coverage/branch-coverage-report.txt
jq . .coverage/branch-coverage.json
```

## Branch Types Checklist

When testing a function, ensure you cover:

- [ ] **If/Else:** Both true AND false paths
- [ ] **Case:** ALL case branches including default (*)
- [ ] **&&:** Both true, first false, second false
- [ ] **||:** First true, second true, both false
- [ ] **Return:** Every return statement path
- [ ] **File checks:** File exists AND file missing
- [ ] **Platform:** macOS AND Linux paths
- [ ] **Commands:** Command exists AND command missing
- [ ] **Errors:** Success path AND error path

## Test Template

```bash
#!/usr/bin/env bash
set -euo pipefail

# Load framework
source "$(dirname "${BASH_SOURCE[0]}")/../lib/reliable-test-framework.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../mocks/environment-control.sh"

# Track coverage
declare -i BRANCHES_TESTED=0

test_both_branches() {
  # TRUE branch
  setup_true_condition
  run_function
  assert_equals "expected" "$result"
  BRANCHES_TESTED=$((BRANCHES_TESTED + 1))

  # FALSE branch
  setup_false_condition
  run_function
  assert_equals "expected" "$result"
  BRANCHES_TESTED=$((BRANCHES_TESTED + 1))
}

# Run and report
test_both_branches
printf "Branches Tested: %d\n" "$BRANCHES_TESTED"
```

## Common Patterns

### If/Else
```bash
# Code
if [[ condition ]]; then
  branch_a
else
  branch_b
fi

# Test
test_condition_true()  # Tests branch_a
test_condition_false() # Tests branch_b
```

### Case
```bash
# Code
case "$var" in
  a) action_a ;;
  b) action_b ;;
  *) default ;;
esac

# Test
test_case_a()       # var=a
test_case_b()       # var=b
test_case_default() # var=invalid
```

### AND (&&)
```bash
# Code
if check_a && check_b; then
  both_ok
fi

# Test
test_both_true()      # a=true, b=true
test_first_false()    # a=false (short-circuit)
test_second_false()   # a=true, b=false
```

### OR (||)
```bash
# Code
if check_a || check_b; then
  at_least_one_ok
fi

# Test
test_first_true()     # a=true (short-circuit)
test_second_true()    # a=false, b=true
test_both_false()     # a=false, b=false
```

## Mock Functions

```bash
# Platform
mock_platform "macos"        # or "linux"

# Commands
mock_command_exists "docker" "true"   # or "false"

# Docker
mock_docker_running "true"   # or "false"

# Files
mock_file_exists "/path" "true" "content"

# Env vars
mock_env_var "DEBUG" "true"

# Network
mock_network_available "false"

# Cleanup
cleanup_mocks
```

## Assertions

```bash
# Equals
assert_equals "expected" "$actual" "description"

# Success (exit code 0)
assert_success "description"

# Contains
assert_contains "$output" "substring" "description"

# With context (detailed error)
assert_with_context "expected" "$actual" "description"
```

## Resilient Testing Rules

### ✅ DO

```bash
# Test all branches
test_success_path()
test_error_path()

# Handle missing commands
if command -v timeout >/dev/null 2>&1; then
  timeout 5 cmd
else
  cmd  # No timeout available
fi

# Test degrades gracefully
mock_redis "false"
assert_fallback_works
assert_success  # Still works without Redis
```

### ❌ DON'T

```bash
# Only test happy path
test_success_only()  # Missing error path!

# Fail on environment
timeout 5 cmd  # Fails on macOS without timeout

# Assume commands exist
use_command_without_check  # May not exist
```

## Workflow

### 1. Write Code
```bash
vim src/lib/mymodule.sh
```

### 2. Identify Branches
```bash
# Count branches in your module
grep "if \[" src/lib/mymodule.sh | wc -l
grep "case" src/lib/mymodule.sh | wc -l
grep " && " src/lib/mymodule.sh | wc -l
grep " || " src/lib/mymodule.sh | wc -l
```

### 3. Write Tests
```bash
# Copy template
cp src/tests/unit/test-branch-coverage-template.sh \
   src/tests/unit/test-mymodule-branch-coverage.sh

# Edit to test your branches
vim src/tests/unit/test-mymodule-branch-coverage.sh
```

### 4. Run Tests
```bash
# Run your tests
bash src/tests/unit/test-mymodule-branch-coverage.sh

# Check coverage
bash scripts/branch-coverage-analysis.sh
```

### 5. Verify
```bash
# All tests pass?
# All branches covered?
# Works on macOS and Linux?
```

## Coverage Targets

| Phase | Target | Status |
|-------|--------|--------|
| Current | 59% | ✅ Baseline |
| v0.9.8 | 60% | 🎯 Next |
| v0.9.9 | 70% | 🔜 Soon |
| v1.0 | 80% | 📅 Planned |
| v1.x | 100% | 🚀 Goal |

## Common Branch Patterns in nself

### Platform Detection
```bash
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS (BSD)
else
  # Linux (GNU)
fi
```

### Command Availability
```bash
if command -v timeout >/dev/null 2>&1; then
  # Has timeout
else
  # No timeout
fi
```

### Docker Check
```bash
if docker info >/dev/null 2>&1; then
  # Docker running
else
  # Docker not running
fi
```

### File Existence
```bash
if [[ -f ".env" ]]; then
  # File exists
else
  # File missing
fi
```

### Environment Mode
```bash
case "$ENV" in
  dev|development) ;;
  prod|production) ;;
  staging) ;;
  *) error ;;
esac
```

## Tips

1. **Start Small:** Test one function completely before moving on
2. **Use Template:** Copy and modify the template for consistency
3. **Mock Everything:** Control test environment completely
4. **Test Errors:** Error paths are code too - test them
5. **Be Resilient:** Tests should pass on all platforms
6. **Document:** Each test documents expected behavior
7. **Automate:** Run coverage analysis regularly

## Help

- Full guide: `docs/testing/BRANCH-COVERAGE-GUIDE.md`
- Implementation: `docs/testing/BRANCH-COVERAGE-IMPLEMENTATION.md`
- Template: `src/tests/unit/test-branch-coverage-template.sh`
- Example: `src/tests/unit/test-validation-branch-coverage.sh`
- Mocks: `src/tests/mocks/environment-control.sh`

## Troubleshooting

**Q: Test fails on macOS but passes on Linux?**
A: Probably using a GNU-specific command. Use platform-compat.sh wrappers.

**Q: How do I test a function with many branches?**
A: Break into smaller functions or test systematically with a table of inputs.

**Q: Coverage analysis is slow?**
A: It processes thousands of files. Consider targeting specific modules.

**Q: Mock isn't working?**
A: Ensure you source environment-control.sh and call cleanup_mocks after tests.

**Q: How do I know which branches aren't tested?**
A: Run `bash scripts/show-untested-branches.sh`

---

**Remember:** Every branch is a code path. Every code path should be tested. Use this reference to systematically cover all branches in nself.
