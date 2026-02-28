# Test Coverage Guide

Complete guide to nself's test coverage system and how to achieve 100% coverage.

## Table of Contents

- [Overview](#overview)
- [Coverage System](#coverage-system)
- [Running Coverage](#running-coverage)
- [Viewing Reports](#viewing-reports)
- [Improving Coverage](#improving-coverage)
- [Coverage Requirements](#coverage-requirements)
- [CI/CD Integration](#cicd-integration)
- [Troubleshooting](#troubleshooting)

## Overview

nself aims for **100% test coverage** to ensure reliability and quality. Our coverage system tracks:

- **Line Coverage**: Percentage of code lines executed by tests
- **Branch Coverage**: Percentage of decision branches taken
- **Function Coverage**: Percentage of functions called

**Current Status**: 100% ✅

## Coverage System

### Architecture

```
coverage/
├── data/               # Raw coverage data from test runs
│   ├── unit/          # Unit test coverage
│   ├── integration/   # Integration test coverage
│   ├── security/      # Security test coverage
│   └── e2e/           # End-to-end test coverage
├── reports/           # Generated reports
│   ├── coverage.txt   # Text report
│   ├── coverage.json  # JSON data
│   ├── badge.svg      # Coverage badge
│   └── html/          # Interactive HTML report
└── .coverage-history.json  # Historical trend data
```

### Tools

- **kcov**: Bash code coverage collection
- **lcov**: Coverage data merging
- **Custom scripts**: Report generation and verification

## Running Coverage

### Full Coverage Collection

```bash
# Run all tests with coverage tracking
./src/scripts/coverage/collect-coverage.sh

# Generate reports
./src/scripts/coverage/generate-coverage-report.sh

# Verify requirements
./src/scripts/coverage/verify-coverage.sh
```

### Quick Coverage Check

```bash
# One-line coverage check
./src/scripts/coverage/collect-coverage.sh && \
./src/scripts/coverage/generate-coverage-report.sh && \
./src/scripts/coverage/verify-coverage.sh
```

### Coverage for Specific Suite

```bash
# Unit tests only
COVERAGE_ENABLED=true kcov coverage/data/unit ./src/tests/run-init-tests.sh

# Integration tests only
COVERAGE_ENABLED=true kcov coverage/data/integration ./src/tests/integration/run-all.sh
```

## Viewing Reports

### Text Report

```bash
# View in terminal
cat coverage/reports/coverage.txt
```

Example output:
```
========================================
nself Test Coverage Report
========================================

Overall Coverage:     100.0%  (target: 100%)
Line Coverage:        100.0%  (5,234 / 5,234 lines)
Branch Coverage:      98.5%   (1,234 / 1,253 branches)
Function Coverage:    100.0%  (432 / 432 functions)

Progress: [==================================================] 100.0%

Test Statistics:
  Total Tests:      700
  Passed:          700  (100.0%)
  Failed:            0
  Skipped:           0
```

### HTML Report

```bash
# Open interactive HTML report
open coverage/reports/html/index.html
```

Features:
- File browser with coverage percentages
- Line-by-line coverage highlighting
- Branch coverage visualization
- Test execution counts
- Uncovered code identification

### JSON Report

```bash
# View JSON data
cat coverage/reports/coverage.json

# Use with jq
jq '.overall' coverage/reports/coverage.json
```

## Improving Coverage

### 1. Identify Uncovered Code

```bash
# Generate report to see uncovered lines
./src/scripts/coverage/generate-coverage-report.sh

# Open HTML report
open coverage/reports/html/index.html
```

In the HTML report:
- **Green lines**: Covered by tests
- **Red lines**: Not covered
- **Yellow lines**: Partially covered (branch coverage)

### 2. Write Tests for Uncovered Code

Example: Adding tests for uncovered function

```bash
# File: src/lib/auth/oauth.sh
oauth_validate_token() {
    local token="$1"

    # Line 1: Covered ✅
    if [[ -z "$token" ]]; then
        return 1
    fi

    # Line 2: NOT covered ❌
    if [[ ${#token} -lt 32 ]]; then
        return 1
    fi

    # Line 3: Covered ✅
    return 0
}
```

Add test:

```bash
# File: src/tests/unit/test-oauth.sh
test_oauth_validate_token_short() {
    local result

    # Test the uncovered branch
    oauth_validate_token "short_token"
    result=$?

    assert_equals 1 "$result" "Should reject short token"
}
```

### 3. Run Coverage Again

```bash
# Collect coverage
./src/scripts/coverage/collect-coverage.sh

# Verify improvement
./src/scripts/coverage/verify-coverage.sh
```

### 4. Common Patterns

#### Error Handling

```bash
# Ensure error paths are tested
function process_data() {
    local data="$1"

    # Test both success and failure
    if validate_data "$data"; then
        # Success path - needs test ✅
        return 0
    else
        # Error path - needs test ✅
        return 1
    fi
}
```

Test both paths:
```bash
test_process_data_success() {
    process_data "valid"
    assert_equals 0 $?
}

test_process_data_failure() {
    process_data "invalid"
    assert_equals 1 $?
}
```

#### Edge Cases

```bash
# Test boundary conditions
test_edge_cases() {
    # Empty input
    my_function ""

    # Very long input
    my_function "$(printf 'a%.0s' {1..10000})"

    # Special characters
    my_function "test@#$%"
}
```

#### Branch Coverage

```bash
# Test all branches
function calculate_price() {
    local quantity="$1"

    if [[ $quantity -lt 10 ]]; then
        echo "100"  # Test with quantity=5 ✅
    elif [[ $quantity -lt 100 ]]; then
        echo "90"   # Test with quantity=50 ✅
    else
        echo "80"   # Test with quantity=200 ✅
    fi
}
```

## Coverage Requirements

### Minimum Thresholds

```bash
# Set in verify-coverage.sh
REQUIRED_LINE_COVERAGE=100.0     # Enforced ✅
REQUIRED_BRANCH_COVERAGE=95.0    # Warning only ⚠️
REQUIRED_FUNCTION_COVERAGE=100.0  # Warning only ⚠️
```

### Enforcement

- **Line coverage < 100%**: CI fails ❌
- **Branch coverage < 95%**: Warning only ⚠️
- **Function coverage < 100%**: Warning only ⚠️

### Override (Emergency Only)

```bash
# Skip coverage verification (NOT recommended)
SKIP_COVERAGE_CHECK=true ./src/scripts/coverage/verify-coverage.sh
```

**Only use in emergencies!** Coverage requirements exist for a reason.

## CI/CD Integration

### GitHub Actions

Coverage runs automatically on:
- Every push to `main` or `develop`
- Every pull request

Workflow: `.github/workflows/coverage.yml`

### PR Coverage Comments

Pull requests automatically get a coverage comment:

```markdown
## 📊 Coverage Report

**Overall Coverage:** 100.0%
**Target:** 100%
**Gap:** 0.0%

🎉 **100% Coverage Achieved!**
```

### Coverage Badge

Badge updates automatically on merge to `main`:

![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)

### Preventing Coverage Decreases

```bash
# Pre-commit hook (optional)
# .git/hooks/pre-commit

./src/scripts/coverage/collect-coverage.sh
./src/scripts/coverage/verify-coverage.sh

if [ $? -ne 0 ]; then
    echo "❌ Coverage verification failed"
    echo "Fix: Add tests to maintain coverage"
    exit 1
fi
```

## Troubleshooting

### "Coverage data not found"

**Problem**: No coverage data collected

**Solution**:
```bash
# Ensure tools are installed
command -v kcov || sudo apt-get install kcov

# Run collection first
./src/scripts/coverage/collect-coverage.sh
```

### "Coverage below requirement"

**Problem**: Coverage dropped below 100%

**Solution**:
```bash
# View uncovered code
open coverage/reports/html/index.html

# Find red (uncovered) lines
# Write tests for those lines
# Run coverage again
```

### "Test fails but coverage passes"

**Problem**: Test suite fails but coverage is collected

**Solution**:
```bash
# Fix failing tests first
./src/tests/run-tests.sh

# Then collect coverage
./src/scripts/coverage/collect-coverage.sh
```

### "kcov not found"

**Problem**: Coverage tool not installed

**Solution**:
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install kcov

# macOS
brew install kcov

# Or use manual instrumentation (slower)
COVERAGE_ENABLED=true bash test-file.sh
```

### "Coverage decreased in PR"

**Problem**: PR reduces coverage

**Solution**:
```bash
# Show coverage diff
./src/scripts/coverage/coverage-diff.sh

# Identify new uncovered code
# Add tests for new code
# Ensure new code is covered
```

## Best Practices

### 1. Write Tests First (TDD)

```bash
# 1. Write failing test
test_new_feature() {
    new_feature
    assert_equals 0 $?
}

# 2. Implement feature
new_feature() {
    # Implementation
    return 0
}

# 3. Verify test passes
# Coverage automatically at 100%
```

### 2. Test Every Branch

```bash
# Bad: Only tests happy path
test_process() {
    process_data "valid"
    assert_equals 0 $?
}

# Good: Tests all paths
test_process_valid() {
    process_data "valid"
    assert_equals 0 $?
}

test_process_invalid() {
    process_data "invalid"
    assert_equals 1 $?
}

test_process_empty() {
    process_data ""
    assert_equals 1 $?
}
```

### 3. Use Coverage to Find Gaps

```bash
# After implementing feature
./src/scripts/coverage/collect-coverage.sh
./src/scripts/coverage/generate-coverage-report.sh

# Review HTML report
# Add tests for any red lines
```

### 4. Maintain Coverage

```bash
# Before committing
./src/scripts/coverage/verify-coverage.sh

# Only commit if verification passes
```

## Coverage History

View coverage trends:

```bash
# Show trend chart
./src/scripts/coverage/track-coverage-history.sh show

# Generate trend report
./src/scripts/coverage/track-coverage-history.sh report
```

Example output:
```
Coverage Trend:

Commit    Date           Coverage   Change
--------------------------------------------------
af3ad41   2026-01-31     100.0%     +35.0%
5184aa5   2026-01-30      65.0%      +5.0%
b0af0e0   2026-01-29      60.0%      +0.0%
```

## Resources

- **Coverage Scripts**: `/src/scripts/coverage/`
- **Test Suites**: `/src/tests/`
- **CI Workflow**: `/.github/workflows/coverage.yml`
- **Coverage Data**: `/coverage/`

## Getting Help

Coverage issues? Check:

1. **HTML Report**: `coverage/reports/html/index.html`
2. **Text Report**: `coverage/reports/coverage.txt`
3. **CI Logs**: GitHub Actions workflow logs
4. **This Guide**: You're reading it!

## Summary

**Goal**: 100% test coverage

**How to achieve**:
1. Run: `./src/scripts/coverage/collect-coverage.sh`
2. Review: `open coverage/reports/html/index.html`
3. Write tests for red lines
4. Verify: `./src/scripts/coverage/verify-coverage.sh`
5. Repeat until 100%

**Maintain**:
- CI enforces coverage
- Pre-commit hooks (optional)
- Regular trend review

🎯 **Target: 100%** ✅ **Status: ACHIEVED!** 🎉
