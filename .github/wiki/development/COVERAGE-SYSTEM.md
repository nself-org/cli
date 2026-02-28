# nself Coverage System

Complete documentation of nself's test coverage infrastructure for achieving and maintaining 100% coverage.

## Overview

nself has implemented a comprehensive test coverage system to track, report, and enforce 100% test coverage across the entire codebase.

**Current Status**: ✅ 100% Coverage Achieved

## System Architecture

### Components

```
Coverage System
├── Collection (collect-coverage.sh)
│   ├── Runs all test suites
│   ├── Uses kcov for bash coverage
│   └── Aggregates coverage data
│
├── Reporting (generate-coverage-report.sh)
│   ├── Text reports
│   ├── HTML reports
│   ├── JSON data
│   └── SVG badges
│
├── Verification (verify-coverage.sh)
│   ├── Enforces requirements
│   ├── Fails CI if below threshold
│   └── Shows gap analysis
│
├── History Tracking (track-coverage-history.sh)
│   ├── Stores coverage per commit
│   ├── Generates trend charts
│   └── Tracks progress over time
│
└── Diff Analysis (coverage-diff.sh)
    ├── Compares branches
    ├── Shows PR impact
    └── Identifies coverage changes
```

### File Structure

```
nself/
├── src/scripts/coverage/                 # Coverage scripts
│   ├── collect-coverage.sh              # Main collection script
│   ├── generate-coverage-report.sh      # Report generator
│   ├── verify-coverage.sh               # Requirement enforcer
│   ├── track-coverage-history.sh        # History tracker
│   ├── coverage-diff.sh                 # Diff analyzer
│   ├── install-coverage-tools.sh        # Tool installer
│   ├── pre-commit-hook.sh               # Git hook template
│   ├── README.md                        # Script documentation
│   └── QUICK-REFERENCE.md               # Quick commands
│
├── coverage/                            # Generated coverage data
│   ├── data/                            # Raw coverage from tests
│   │   ├── unit/                        # Unit test coverage
│   │   ├── integration/                 # Integration coverage
│   │   ├── security/                    # Security coverage
│   │   └── e2e/                         # E2E coverage
│   ├── reports/                         # Generated reports
│   │   ├── coverage.txt                 # Text report
│   │   ├── coverage.json                # JSON data
│   │   ├── badge.svg                    # Badge
│   │   ├── summary.txt                  # Quick summary
│   │   ├── trend.txt                    # Trend report
│   │   └── html/                        # HTML reports
│   │       └── index.html               # Main HTML report
│   └── .coverage-history.json           # Historical data
│
├── docs/development/                    # Coverage documentation
│   ├── COVERAGE-GUIDE.md                # Complete guide
│   ├── COVERAGE-DASHBOARD.md            # Live dashboard
│   └── COVERAGE-SYSTEM.md               # This file
│
└── .github/workflows/
    └── coverage.yml                     # CI/CD workflow
```

## Coverage Metrics

### Tracked Metrics

1. **Line Coverage**: Percentage of code lines executed by tests
2. **Branch Coverage**: Percentage of decision branches taken
3. **Function Coverage**: Percentage of functions called
4. **Test Count**: Total number of tests
5. **Pass Rate**: Percentage of tests passing
6. **Execution Time**: Test suite duration

### Requirements

| Metric | Required | Warning | Action |
|--------|----------|---------|--------|
| Line Coverage | 100.0% | - | CI fails ❌ |
| Branch Coverage | 95.0% | < 95% | Warning only ⚠️ |
| Function Coverage | 100.0% | < 100% | Warning only ⚠️ |

## Workflow

### Standard Workflow

```bash
# 1. Collect coverage from all test suites
./src/scripts/coverage/collect-coverage.sh

# 2. Generate reports in multiple formats
./src/scripts/coverage/generate-coverage-report.sh

# 3. Track in history for trend analysis
./src/scripts/coverage/track-coverage-history.sh track

# 4. Verify requirements (fails if < 100%)
./src/scripts/coverage/verify-coverage.sh
```

### Developer Workflow

```bash
# Make code changes
vim src/lib/auth/oauth.sh

# Run relevant tests
./src/tests/run-init-tests.sh

# Check coverage
./src/scripts/coverage/collect-coverage.sh
./src/scripts/coverage/verify-coverage.sh

# If coverage < 100%, add tests
vim src/tests/unit/test-oauth.sh

# Verify again
./src/scripts/coverage/collect-coverage.sh
./src/scripts/coverage/verify-coverage.sh

# Commit when coverage is good
git commit -am "feat: add oauth feature with tests"
```

### CI/CD Workflow

1. **On Push/PR**: Coverage workflow runs automatically
2. **Collection**: All test suites run with coverage tracking
3. **Reporting**: Reports generated and uploaded as artifacts
4. **Verification**: CI fails if coverage < 100%
5. **PR Comment**: Coverage summary posted to PR
6. **Badge Update**: Coverage badge updated on main branch
7. **History**: Coverage entry added to history

## Report Types

### 1. Text Report

Terminal-friendly summary with:
- Overall coverage percentage
- Line/branch/function breakdown
- Progress bar visualization
- Test statistics
- Coverage by module
- Gap analysis

**Location**: `coverage/reports/coverage.txt`

**Usage**:
```bash
cat coverage/reports/coverage.txt
```

### 2. HTML Report

Interactive browser-based report with:
- File browser with coverage percentages
- Line-by-line coverage highlighting
- Branch coverage visualization
- Test execution counts
- Uncovered code identification

**Location**: `coverage/reports/html/index.html`

**Usage**:
```bash
open coverage/reports/html/index.html
```

### 3. JSON Report

Machine-readable data for:
- Automation
- Integration with other tools
- Custom reporting
- API consumption

**Location**: `coverage/reports/coverage.json`

**Usage**:
```bash
jq '.overall' coverage/reports/coverage.json
```

### 4. Coverage Badge

SVG badge showing:
- Current coverage percentage
- Color-coded (green = 100%, yellow = 80-99%, red < 80%)

**Location**: `coverage/reports/badge.svg`

**Usage**:
```markdown
![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)
```

## CI/CD Integration

### GitHub Actions

**Workflow**: `.github/workflows/coverage.yml`

**Triggers**:
- Push to `main` or `develop`
- Pull requests
- Manual workflow dispatch

**Jobs**:
1. **coverage** - Main coverage job
   - Install coverage tools
   - Run tests with coverage
   - Generate reports
   - Verify requirements
   - Upload to Codecov
   - Upload artifacts
   - Comment on PR
   - Update badge

2. **coverage-summary** - Summary job
   - Download artifacts
   - Display summary
   - Report final status

### PR Integration

Pull requests automatically receive:

1. **Coverage Comment**:
   ```markdown
   ## 📊 Coverage Report

   **Overall Coverage:** 100.0%
   **Target:** 100%
   **Gap:** 0.0%

   🎉 **100% Coverage Achieved!**
   ```

2. **Coverage Diff**:
   - Shows coverage change vs base branch
   - Identifies new uncovered code
   - Prevents coverage decreases

3. **Status Check**:
   - ✅ Pass if coverage >= 100%
   - ❌ Fail if coverage < 100%

### Pre-Commit Hook

Optional git hook to verify coverage before commit.

**Installation**:
```bash
cp src/scripts/coverage/pre-commit-hook.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

**Behavior**:
- Runs coverage verification
- Blocks commit if coverage < 100%
- Can be bypassed with `SKIP_COVERAGE_CHECK=true`

## History Tracking

### Coverage History

Coverage data is tracked over time in `.coverage-history.json`:

```json
{
  "version": "1.0",
  "created": "2026-01-31T21:45:00Z",
  "commits": [
    {
      "sha": "af3ad41",
      "date": "2026-01-31",
      "coverage": 100.0,
      "tests": 700
    },
    {
      "sha": "5184aa5",
      "date": "2026-01-30",
      "coverage": 65.0,
      "tests": 445
    }
  ]
}
```

### Trend Analysis

View coverage trends:

```bash
# Show trend chart
./src/scripts/coverage/track-coverage-history.sh show

# Generate trend report
./src/scripts/coverage/track-coverage-history.sh report
```

**Output**:
```
Coverage Trend:

Commit    Date           Coverage   Change
--------------------------------------------------
af3ad41   2026-01-31     100.0%     +35.0%
5184aa5   2026-01-30      65.0%      +5.0%
b0af0e0   2026-01-29      60.0%      +0.0%
```

## Coverage Tools

### Required Tools

1. **Bash 3.2+** - Shell execution (built-in)
2. **bc** - Mathematical calculations (built-in on most systems)

### Optional Tools (Enhanced Functionality)

1. **kcov** - Bash code coverage collection
   ```bash
   # Ubuntu/Debian
   sudo apt-get install kcov

   # macOS
   brew install kcov
   ```

2. **lcov** - Coverage data merging
   ```bash
   sudo apt-get install lcov  # Linux
   brew install lcov          # macOS
   ```

3. **jq** - JSON processing
   ```bash
   sudo apt-get install jq    # Linux
   brew install jq            # macOS
   ```

### Tool Installation

Quick install all tools:

```bash
./src/scripts/coverage/install-coverage-tools.sh
```

### Fallback Mode

If coverage tools aren't available:
- Scripts use manual instrumentation
- Coverage still works (slower)
- Reduced accuracy but functional

## Best Practices

### 1. Test Before Commit

```bash
# Always verify coverage before committing
./src/scripts/coverage/verify-coverage.sh
```

### 2. View Uncovered Code

```bash
# Generate HTML report to see what's uncovered
./src/scripts/coverage/generate-coverage-report.sh
open coverage/reports/html/index.html

# Red lines = uncovered code
# Write tests for red lines
```

### 3. Maintain 100%

```bash
# Coverage should never decrease
# Add tests when adding code
# Review coverage diff in PRs
./src/scripts/coverage/coverage-diff.sh diff main HEAD
```

### 4. Use Pre-Commit Hook

```bash
# Install hook to catch coverage issues early
cp src/scripts/coverage/pre-commit-hook.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### 5. Review Coverage Dashboard

```bash
# Regularly check dashboard for gaps
cat docs/development/COVERAGE-DASHBOARD.md
```

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| kcov not found | Tool not installed | Run `install-coverage-tools.sh` |
| No coverage data | Collection not run | Run `collect-coverage.sh` first |
| Coverage < 100% | Missing tests | View HTML report, add tests |
| Test failures | Code errors | Fix tests before collecting coverage |
| Wrong permissions | File mode issues | Check script permissions (755) |

### Debug Mode

Run scripts with debug output:

```bash
bash -x ./src/scripts/coverage/collect-coverage.sh
```

### Skip Verification

Emergency bypass (NOT recommended):

```bash
SKIP_COVERAGE_CHECK=true git commit
```

## Maintenance

### Regular Tasks

1. **Weekly**: Review coverage dashboard
2. **Per PR**: Check coverage diff
3. **Per Release**: Verify 100% coverage
4. **Monthly**: Review trend history

### Updating Scripts

When modifying coverage scripts:

1. Test on Ubuntu and macOS
2. Ensure backward compatibility
3. Update documentation
4. Test CI/CD integration
5. Update quick reference

## Resources

### Documentation

- [Coverage Guide](COVERAGE-GUIDE.md) - Complete usage guide
- [Coverage Dashboard](COVERAGE-DASHBOARD.md) - Live status dashboard
- [Script README](../../src/scripts/coverage/README.md) - Script documentation
- [Quick Reference](../../src/scripts/coverage/QUICK-REFERENCE.md) - Quick commands

### Scripts

- `collect-coverage.sh` - Collection
- `generate-coverage-report.sh` - Reporting
- `verify-coverage.sh` - Verification
- `track-coverage-history.sh` - History
- `coverage-diff.sh` - Diff analysis

### External Tools

- [kcov](https://github.com/SimonKagstrom/kcov) - Bash coverage
- [lcov](https://github.com/linux-test-project/lcov) - Coverage merging
- [Codecov](https://codecov.io) - Coverage hosting

## Future Enhancements

### Planned Features

- [ ] Mutation testing integration
- [ ] Performance benchmarking coverage
- [ ] Load testing coverage
- [ ] Chaos engineering coverage
- [ ] Coverage heat maps
- [ ] Interactive trend visualization
- [ ] Coverage prediction models
- [ ] Automated coverage improvement suggestions

## Summary

nself's coverage system provides:

✅ **Comprehensive tracking** across all test suites
✅ **Multiple report formats** (text, HTML, JSON, badge)
✅ **CI/CD integration** with automatic verification
✅ **Historical tracking** for trend analysis
✅ **PR integration** with coverage diffs
✅ **Developer tools** for local verification
✅ **100% coverage** achieved and maintained

**Goal**: Maintain 100% test coverage to ensure reliability and quality.

**Status**: 🎉 **100% Coverage Achieved!** ✅
