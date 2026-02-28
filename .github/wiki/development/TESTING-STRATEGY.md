# nself Testing Strategy

Comprehensive testing approach for production-ready quality.

**Last Updated**: January 31, 2026
**Coverage Target**: 80%+ (v0.9.8), 85%+ (v1.0)

---

## Testing Pyramid

```
           /\
          /E2E\         20 tests
         /------\
        /  INT   \      150 tests  
       /----------\
      /    UNIT    \    500+ tests
     /--------------\
```

### Test Distribution

| Type | Count | Coverage | Execution Time |
|------|-------|----------|----------------|
| Unit Tests | 500+ | 85% | < 5 min |
| Integration Tests | 150+ | 75% | < 15 min |
| E2E Tests | 20+ | 70% | < 30 min |
| **Total** | **700+** | **80%** | **< 50 min** |

---

## Test Categories

### 1. Unit Tests (500+)

**Purpose**: Test individual functions and modules in isolation

**Coverage**: 85%

**Tools**: Bash test framework, ShellSpec (optional)

**Examples:**
- Configuration parsing
- Environment variable handling
- String manipulation
- File operations
- Validation logic

**Location**: `src/tests/unit/`

### 2. Integration Tests (150+)

**Purpose**: Test service interactions and workflows

**Coverage**: 75%

**Tools**: Docker Compose, PostgreSQL, Hasura

**Examples:**
- Database migrations
- Multi-tenant isolation (RLS)
- Authentication flows
- GraphQL queries
- Backup/restore operations

**Location**: `src/tests/integration/`

### 3. End-to-End Tests (20+)

**Purpose**: Test complete user workflows

**Coverage**: 70%

**Tools**: Real deployments, actual services

**Examples:**
- Init → Build → Start → Deploy workflow
- User signup → Login → API call flow
- Billing subscription lifecycle
- White-label tenant setup
- Production deployment

**Location**: `src/tests/e2e/`

### 4. Security Tests (30+)

**Purpose**: Validate security measures

**Tools**: Security scanner, SQL injection tester

**Examples:**
- SQL injection prevention
- XSS protection
- Command injection prevention
- Secrets detection
- Permission checks

**Location**: `src/tests/security/`

### 5. Performance Tests (20+)

**Purpose**: Ensure performance targets

**Tools**: Time measurements, benchmarking

**Examples:**
- Build time < 30s
- Start time < 60s
- Query performance < 100ms
- API latency < 200ms

**Location**: `src/tests/performance/`

---

## Test Execution

### Local Development

```bash
# Run all tests
bash src/tests/run-tests.sh

# Run specific test category
bash src/tests/unit/run-unit-tests.sh
bash src/tests/integration/run-integration-tests.sh
bash src/tests/e2e/run-e2e-tests.sh

# Run single test file
bash src/tests/unit/test-init.sh
```

### CI/CD (GitHub Actions)

**Triggers:**
- Every push
- Every pull request
- Daily scheduled run

**Matrix**:
- Ubuntu 22.04 + Bash 5.x
- Ubuntu 22.04 + Bash 3.2
- macOS Latest + Bash 3.2

**Workflows**:
1. Portability Check
2. Unit Tests
3. Integration Tests
4. Security Scan
5. Platform Compatibility

---

## Coverage Goals

### Component-Specific Targets

| Component | Target | Current | Status |
|-----------|--------|---------|--------|
| Init/Build | 90% | 88% | ✅ Good |
| Authentication | 90% | 90% | ✅ Excellent |
| Multi-Tenancy | 100% | 100% | ✅ Excellent |
| Database | 85% | 85% | ✅ Good |
| GraphQL | 75% | 75% | ✅ Good |
| Billing | 80% | 70% | ⚠️ Needs Work |
| White-Label | 80% | 65% | ⚠️ Needs Work |
| OAuth | 75% | 75% | ✅ Good |
| Storage | 80% | 80% | ✅ Good |
| Deploy | 80% | 70% | ⚠️ Needs Work |
| Monitoring | 70% | 60% | ⚠️ Needs Work |

---

## Quality Gates

### Pre-Commit

- [ ] Code lints successfully
- [ ] New code has tests
- [ ] All tests pass locally

### Pre-PR

- [ ] All tests pass
- [ ] Coverage maintained or improved
- [ ] No security issues introduced

### Pre-Release

- [ ] All tests passing (100%)
- [ ] Coverage ≥ 80%
- [ ] Performance benchmarks met
- [ ] Cross-platform validated
- [ ] Security audit clean

---

## Test Writing Guidelines

### Unit Test Example

```bash
test_validate_port() {
  local result
  
  # Test valid port
  validate_port "3000" && result=$? || result=$?
  assert_equals "0" "$result" "Valid port should return 0"
  
  # Test invalid port
  validate_port "99999" && result=$? || result=$?
  assert_not_equals "0" "$result" "Invalid port should fail"
}
```

### Integration Test Example

```bash
test_tenant_isolation() {
  # Create two tenants
  local tenant_a=$(create_tenant "Tenant A")
  local tenant_b=$(create_tenant "Tenant B")
  
  # Create data in tenant A
  create_tenant_data "$tenant_a" "secret data"
  
  # Verify tenant B cannot see it
  local data=$(query_as_tenant "$tenant_b" "SELECT * FROM data")
  assert_empty "$data" "Tenant B should not see Tenant A data"
}
```

---

## v0.9.9 Test Plan

### New Tests to Add (250+)

**Billing (150 tests):**
- Subscription creation
- Plan upgrades/downgrades
- Usage tracking
- Invoice generation
- Payment failures
- Webhook handling

**White-Label (100 tests):**
- Custom domains
- Branding customization
- Email template overrides
- Theme configuration
- Logo management

---

## Continuous Improvement

### Metrics Tracked

- Test count
- Coverage percentage
- Test execution time
- Flaky test rate
- Bug escape rate

### Monthly Review

- Identify coverage gaps
- Remove redundant tests
- Fix flaky tests
- Improve test speed
- Update test data

---

**See Also:**
- [Quality Metrics](./QUALITY-METRICS.md)
- [Performance Benchmarks](./PERFORMANCE-BENCHMARKS.md)
- CI/CD Configuration
