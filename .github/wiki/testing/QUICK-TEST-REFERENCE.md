# Quick Test Reference - v0.9.8

## New Comprehensive Test Suites

### Run Individual Test Suites

```bash
# Billing System (150 tests) - ~15 seconds
bash src/tests/integration/test-billing-comprehensive.sh

# OAuth Providers (80 tests) - ~8 seconds
bash src/tests/integration/test-oauth-providers-comprehensive.sh

# White-Label System (100 tests) - ~10 seconds
bash src/tests/integration/test-whitelabel-comprehensive.sh

# Backup/Restore (50 tests) - ~5 seconds
bash src/tests/integration/test-backup-restore-comprehensive.sh

# Rate Limiting (30 tests) - ~3 seconds
bash src/tests/integration/test-rate-limit-comprehensive.sh
```

### Run All New Tests

```bash
# Run all 410 new comprehensive tests (~41 seconds)
for test in src/tests/integration/test-*-comprehensive.sh; do
  bash "$test"
done
```

### Run All Tests (Existing + New)

```bash
# Run complete test suite (855+ tests)
bash src/tests/run-tests.sh
```

---

## Test Coverage Summary

| Feature | Tests | File |
|---------|-------|------|
| **Billing** | 150 | `test-billing-comprehensive.sh` |
| **OAuth** | 80 | `test-oauth-providers-comprehensive.sh` |
| **White-Label** | 100 | `test-whitelabel-comprehensive.sh` |
| **Backup/Restore** | 50 | `test-backup-restore-comprehensive.sh` |
| **Rate Limiting** | 30 | `test-rate-limit-comprehensive.sh` |
| **TOTAL NEW** | **410** | 5 new files |
| **TOTAL ALL** | **855+** | 41 files |

---

## What Each Suite Tests

### 1. Billing (150 tests)

- ✅ Subscription lifecycle (create, upgrade, downgrade, cancel, reactivate, pause, resume)
- ✅ Usage tracking & metering (API calls, storage, bandwidth, compute, etc.)
- ✅ Invoice generation & payment processing
- ✅ Quota enforcement (soft/hard limits, burst allowance)
- ✅ Cost allocation & reporting
- ✅ Stripe integration edge cases

### 2. OAuth Providers (80 tests)

- ✅ All 13 providers: Google, GitHub, Microsoft, Facebook, Apple, Slack, Discord, Twitch, Twitter, LinkedIn, GitLab, Bitbucket, Spotify
- ✅ Authorization flows (complete OAuth cycle)
- ✅ Token refresh mechanisms
- ✅ Account linking scenarios
- ✅ Provider failure handling
- ✅ PKCE support for mobile apps
- ✅ State validation & security

### 3. White-Label (100 tests)

- ✅ Branding configuration (logos, colors, fonts)
- ✅ Domain management (custom domains, SSL, DNS)
- ✅ Email template customization
- ✅ Theme management (dark/light modes, CSS variables)
- ✅ Multi-tenant isolation

### 4. Backup/Restore (50 tests)

- ✅ Backup creation (full, incremental, encrypted)
- ✅ Cloud providers (S3, GCS, Azure, Backblaze B2)
- ✅ Intelligent pruning (age, count, size, GFS rotation)
- ✅ 3-2-1 rule verification
- ✅ Cross-environment restore
- ✅ Corruption handling

### 5. Rate Limiting (30 tests)

- ✅ Nginx integration (limit_req, limit_conn)
- ✅ Whitelist/blacklist management
- ✅ Per-zone rate limits (API, Auth, GraphQL, etc.)
- ✅ Redis backend
- ✅ Violation handling & resets

---

## Test Output Format

All tests use consistent output formatting:

```
=== Test Suite Name ===

--- Section Name ---

Test 1: Description... ✓ passed
Test 2: Description... ✓ passed
Test 3: Description... ✗ failed
  Expected: value1
  Actual: value2

Test Summary
Total Tests: 150
Passed: 149
Failed: 1
Success Rate: 99.3%

✓ All tests passed!
```

---

## CI/CD Integration

All tests are designed to run in CI/CD pipelines:

- **Cross-platform:** macOS, Linux, WSL
- **POSIX-compliant:** No Bash 4+ features
- **Fast execution:** Mock mode completes in seconds
- **No external dependencies:** Tests use mocks by default
- **Exit codes:** 0 = success, 1 = failure

### GitHub Actions Example

```yaml
- name: Run Billing Tests
  run: bash src/tests/integration/test-billing-comprehensive.sh

- name: Run OAuth Tests
  run: bash src/tests/integration/test-oauth-providers-comprehensive.sh

- name: Run White-Label Tests
  run: bash src/tests/integration/test-whitelabel-comprehensive.sh

- name: Run Backup Tests
  run: bash src/tests/integration/test-backup-restore-comprehensive.sh

- name: Run Rate Limiting Tests
  run: bash src/tests/integration/test-rate-limit-comprehensive.sh
```

---

## Mock vs. Real Testing

Tests currently use **mock mode** for rapid development:

- Mock Stripe API calls
- Mock OAuth provider responses
- Mock cloud storage uploads
- Mock Redis operations

**To switch to real testing:**
1. Set environment variables (e.g., `STRIPE_API_KEY`)
2. Tests will automatically detect and use real APIs
3. Requires real service credentials

---

## Development Workflow

### 1. Before Implementing a Feature

```bash
# Run relevant test suite to see expected behavior
bash src/tests/integration/test-billing-comprehensive.sh
```

### 2. During Implementation

```bash
# Run tests frequently to verify progress
bash src/tests/integration/test-billing-comprehensive.sh
```

### 3. After Implementation

```bash
# Verify all tests pass
bash src/tests/integration/test-billing-comprehensive.sh

# Run full test suite
bash src/tests/run-tests.sh
```

---

## Debugging Failed Tests

If a test fails:

1. **Check the output** - Test framework shows expected vs actual values
2. **Run test in isolation** - Comment out other tests to focus on one
3. **Enable verbose mode** - Add `set -x` at top of test file
4. **Check test prerequisites** - Ensure required files/services exist

---

## Adding New Tests

To add tests to existing suites:

1. Add test function following naming convention: `test_feature_name()`
2. Increment `TOTAL_TESTS` variable at top of file
3. Follow existing test patterns (describe → run → pass/fail)
4. Use `printf` instead of `echo -e` (POSIX compliance)
5. Make test cross-platform compatible

Example:

```bash
test_new_feature() {
  describe "Test new feature"

  local result
  result=$(some_command)

  if printf "%s" "$result" | grep -q "expected"; then
    pass "New feature working"
  else
    fail "New feature failed"
  fi
}
```

---

## Test Maintenance

- **Update tests** when features change
- **Add edge cases** as bugs are discovered
- **Keep mocks simple** - Focus on behavior, not implementation
- **Document complex tests** - Add comments for non-obvious logic
- **Run tests before commits** - Ensure nothing breaks

---

## Coverage Goals

| Metric | Target | Current | Status |
|--------|--------|---------|--------|
| Total Tests | 700+ | 855+ | ✅ Exceeded (122%) |
| Coverage % | 80% | ~85% | ✅ Exceeded |
| Billing Tests | 150 | 150 | ✅ Complete |
| OAuth Tests | 80 | 80 | ✅ Complete |
| White-Label Tests | 100 | 100 | ✅ Complete |
| Backup Tests | 50 | 50 | ✅ Complete |
| Rate Limit Tests | 30 | 30 | ✅ Complete |

---

## Frequently Asked Questions

**Q: Why are tests so fast?**
A: Tests use mocks by default, avoiding slow external API calls.

**Q: How do I run tests with real services?**
A: Set appropriate environment variables (e.g., `STRIPE_API_KEY`, `AWS_ACCESS_KEY_ID`).

**Q: Can I run tests in parallel?**
A: Yes, test files are independent and can run in parallel.

**Q: What if a test fails in CI but passes locally?**
A: Check for platform-specific differences (macOS vs Linux). All tests should be cross-platform.

**Q: How do I add a new test suite?**
A: Copy an existing comprehensive test file, update the test functions, and add to run-all-tests.sh (invoked via run-tests.sh).

---

**Last Updated:** 2025-01-31
**Version:** v0.9.8
**Total Tests:** 855+
**Coverage:** ~85%
