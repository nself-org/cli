# 100% Test Coverage Plan

## Executive Summary

**Current Status:**
- Test Files: 76
- Source Files: 436
- Coverage: ~60% (445 existing tests + 250+ agent tests = ~695 tests)
- Target: 100% coverage with 1,200+ reliable, fast tests

**Timeline:** 4-6 weeks for complete coverage
**Strategy:** Fill gaps systematically, prioritize critical paths, ensure reliability

---

## Coverage Analysis

### Current Coverage (60%)

**Covered Areas:**
- ✅ Core initialization (init.sh + wizard)
- ✅ Database operations (safe queries, migrations)
- ✅ Authentication & authorization
- ✅ Multi-tenancy (billing, org, tenant)
- ✅ Security (secrets, vault, encryption)
- ✅ Monitoring & observability
- ✅ Email & webhooks
- ✅ Backup & recovery
- ✅ Some CLI commands (admin, config, build, start)

**Gaps (40% - Need Coverage):**

#### 1. CLI Commands (50+ commands missing tests)
Missing comprehensive tests for:
- `version.sh` - Version display
- `completion.sh` - Shell completions
- `audit.sh` - Audit logging
- `history.sh` - Command history
- `metrics.sh` - Performance metrics
- `doctor.sh` - System diagnostics
- `health.sh` - Health checks
- `status.sh` - Service status
- `logs.sh` - Log viewing
- `exec.sh` - Container execution
- `urls.sh` - Service URLs
- `down.sh`, `up.sh`, `stop.sh` - Service lifecycle
- `destroy.sh`, `clean.sh`, `reset.sh` - Cleanup commands
- `restore.sh`, `rollback.sh` - Recovery commands
- `scale.sh` - Scaling operations
- `perf.sh` - Performance commands
- `infra.sh` - Infrastructure management
- `service.sh` - Service management
- `auth.sh` - Auth command wrapper
- `db.sh` - Database command wrapper
- `deploy.sh` - Deployment wrapper
- `config.sh` - Config command wrapper

#### 2. Library Modules (100+ files missing full coverage)
- **docker/** - Container operations
- **nginx/** - Reverse proxy configuration
- **ssl/** - Certificate management (partial)
- **build/** - Build system
- **deployment/** - Deployment automation
- **services/** - Service initialization
- **logging/** - Logging utilities
- **errors/** - Error handling
- **help/** - Help system

#### 3. Service-Specific Tests
- **PostgreSQL Extensions** - 60+ extensions need validation
- **Hasura Metadata** - Schema validation
- **Auth Providers** - OAuth, SAML, LDAP
- **Storage Backends** - MinIO, S3, Azure, GCS
- **Search Engines** - MeiliSearch, Typesense, Sonic, etc.
- **Mail Providers** - MailPit, SMTP, SendGrid, etc.

#### 4. Network & Infrastructure
- **Nginx Routing** - All route configurations
- **SSL/TLS** - Certificate generation, validation, renewal
- **DNS Resolution** - Domain resolution, subdomain routing
- **Proxy Configuration** - Reverse proxy, load balancing

#### 5. Error Scenarios (Realistic Only)
- Missing dependencies (Docker, Git, etc.)
- Permission errors (file system, Docker socket)
- Configuration errors (invalid .env values)
- Resource limits (disk full, low memory)
- Network failures (timeouts, connection refused)

#### 6. Integration Tests
- End-to-end workflows
- Multi-service interactions
- Real-world scenarios
- Performance benchmarks

---

## Test Infrastructure Improvements

### 1. Mock/Stub Infrastructure

**File:** `/Users/admin/Sites/nself/src/tests/mocks/mock-infrastructure.sh`

```bash
#!/usr/bin/env bash
# Mock infrastructure for reliable testing

# Mock Docker API
mock_docker() {
  local operation="$1"
  shift

  case "$operation" in
    ps)
      printf "CONTAINER ID   IMAGE     STATUS\n"
      printf "abc123         nginx     Up 5 minutes\n"
      ;;
    inspect)
      printf '{"State":{"Running":true,"Health":{"Status":"healthy"}}}\n'
      ;;
    logs)
      printf "Mock container logs\n"
      ;;
    exec)
      printf "Mock exec output\n"
      ;;
    *)
      return 0
      ;;
  esac
}

# Mock network calls
mock_curl() {
  local url="$1"
  local response="${MOCK_RESPONSE:-{\"status\":\"ok\"}}"

  printf "%s\n" "$response"
  return 0
}

# Controllable time for timeout tests
mock_date() {
  local format="${1:-%s}"
  local mock_time="${MOCK_TIME:-$(date +%s)}"

  if [[ "$format" == "+%s" ]] || [[ "$format" == "%s" ]]; then
    printf "%s\n" "$mock_time"
  else
    command date "$format"
  fi
}

# Deterministic random data
mock_random() {
  local seed="${MOCK_SEED:-12345}"
  printf "%s\n" "$seed"
}

# Fast tmpfs-backed file operations
create_test_tmpfs() {
  local test_dir

  if [[ "$(uname)" == "Darwin" ]]; then
    test_dir=$(mktemp -d)
  else
    # Linux with tmpfs
    test_dir="/dev/shm/nself-test-$$"
    mkdir -p "$test_dir"
  fi

  printf "%s\n" "$test_dir"
}

# Export mock functions
export -f mock_docker
export -f mock_curl
export -f mock_date
export -f mock_random
export -f create_test_tmpfs
```

### 2. Enhanced Test Framework

**File:** `/Users/admin/Sites/nself/src/tests/lib/test-framework-enhanced.sh`

```bash
#!/usr/bin/env bash
# Enhanced test framework with reliability features

# Source existing framework
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../test_framework.sh"

# Timeout protection (30s default)
run_test_with_timeout() {
  local test_name="$1"
  local timeout="${2:-30}"

  if command -v timeout >/dev/null 2>&1; then
    timeout "$timeout" bash -c "$test_name"
    local result=$?
    if [[ $result -eq 124 ]]; then
      printf "TIMEOUT: %s exceeded %ss\n" "$test_name" "$timeout" >&2
      return 1
    fi
    return $result
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$timeout" bash -c "$test_name"
    return $?
  else
    # No timeout available - run directly
    bash -c "$test_name"
    return $?
  fi
}

# Retry logic for flaky operations
retry_test() {
  local test_func="$1"
  local max_attempts="${2:-3}"
  local attempt=1

  while [[ $attempt -le $max_attempts ]]; do
    if $test_func; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done

  printf "FAILED: %s after %d attempts\n" "$test_func" "$max_attempts" >&2
  return 1
}

# Cleanup guarantees
ensure_cleanup() {
  local cleanup_func="$1"

  # Register cleanup on EXIT
  trap "$cleanup_func" EXIT INT TERM
}

# Fast fail on critical errors
fail_fast() {
  local error_msg="$1"

  printf "\033[31mCRITICAL ERROR:\033[0m %s\n" "$error_msg" >&2
  exit 1
}

# Skip test gracefully
skip_test() {
  local reason="$1"

  printf "\033[33mSKIP:\033[0m %s\n" "$reason"
  return 0
}

# Check if running in CI
is_ci() {
  [[ -n "$CI" ]] || [[ -n "$GITHUB_ACTIONS" ]] || [[ -n "$GITLAB_CI" ]]
}

# Export enhanced functions
export -f run_test_with_timeout
export -f retry_test
export -f ensure_cleanup
export -f fail_fast
export -f skip_test
export -f is_ci
```

### 3. Coverage Tracking Script

**File:** `/Users/admin/Sites/nself/scripts/coverage-report.sh`

```bash
#!/usr/bin/env bash
# Generate comprehensive test coverage report

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COVERAGE_DIR="$PROJECT_ROOT/coverage"

# Create coverage directory
mkdir -p "$COVERAGE_DIR"

# Find all source files
mapfile -t source_files < <(find "$PROJECT_ROOT/src" -type f -name "*.sh" \
  ! -path "*/tests/*" \
  ! -path "*/templates/*" \
  ! -path "*/_deprecated/*" \
  | sort)

# Find all test files
mapfile -t test_files < <(find "$PROJECT_ROOT/src/tests" -type f \( -name "*.sh" -o -name "*.bats" \) | sort)

total_source_files=${#source_files[@]}
total_test_files=${#test_files[@]}

printf "# nself Test Coverage Report\n\n"
printf "Generated: %s\n\n" "$(date '+%Y-%m-%d %H:%M:%S')"

printf "## Summary\n\n"
printf "- **Source Files:** %d\n" "$total_source_files"
printf "- **Test Files:** %d\n" "$total_test_files"
printf "- **Coverage Ratio:** %.1f%%\n\n" "$(echo "scale=1; $total_test_files * 100 / $total_source_files" | bc)"

printf "## Coverage by Category\n\n"

# Analyze CLI coverage
cli_source=$(find "$PROJECT_ROOT/src/cli" -type f -name "*.sh" ! -path "*/_deprecated/*" | wc -l)
cli_tests=$(find "$PROJECT_ROOT/src/tests" -type f -name "*cli*.sh" -o -name "*cli*.bats" | wc -l)
printf "### CLI Commands\n"
printf "- Source files: %d\n" "$cli_source"
printf "- Test files: %d\n" "$cli_tests"
printf "- Coverage: %.1f%%\n\n" "$(echo "scale=1; $cli_tests * 100 / $cli_source" | bc)"

# Analyze library coverage
lib_source=$(find "$PROJECT_ROOT/src/lib" -type f -name "*.sh" | wc -l)
lib_tests=$(find "$PROJECT_ROOT/src/tests/unit" -type f -name "*.sh" | wc -l)
lib_tests=$((lib_tests + $(find "$PROJECT_ROOT/src/tests" -type f -name "*.bats" ! -path "*/integration/*" | wc -l)))
printf "### Library Modules\n"
printf "- Source files: %d\n" "$lib_source"
printf "- Test files: %d\n" "$lib_tests"
printf "- Coverage: %.1f%%\n\n" "$(echo "scale=1; $lib_tests * 100 / $lib_source" | bc)"

# List untested files
printf "## Untested Source Files\n\n"

for source_file in "${source_files[@]}"; do
  base_name=$(basename "$source_file" .sh)
  has_test=false

  for test_file in "${test_files[@]}"; do
    if [[ "$test_file" == *"$base_name"* ]]; then
      has_test=true
      break
    fi
  done

  if [[ "$has_test" == "false" ]]; then
    rel_path="${source_file#$PROJECT_ROOT/}"
    printf "- \`%s\`\n" "$rel_path"
  fi
done

printf "\n---\n\n"
printf "Run \`./scripts/generate-missing-tests.sh\` to create test stubs for untested files.\n"
```

---

## Test Creation Strategy

### Phase 1: Missing CLI Commands (Week 1-2)

Create tests for 50+ untested CLI commands:

**Priority 1 (Critical):**
- `doctor.sh` - System diagnostics
- `health.sh` - Health checks
- `status.sh` - Service status
- `version.sh` - Version display
- `urls.sh` - Service URLs

**Priority 2 (High):**
- `logs.sh` - Log viewing
- `exec.sh` - Container execution
- `audit.sh` - Audit logging
- `history.sh` - Command history
- `metrics.sh` - Performance metrics

**Priority 3 (Medium):**
- Service lifecycle: `down.sh`, `up.sh`, `stop.sh`
- Cleanup: `destroy.sh`, `clean.sh`, `reset.sh`
- Recovery: `restore.sh`, `rollback.sh`
- Scaling: `scale.sh`

**Priority 4 (Low):**
- Wrappers: `auth.sh`, `db.sh`, `deploy.sh`, `config.sh`, `infra.sh`, `service.sh`
- Advanced: `perf.sh`, `completion.sh`

### Phase 2: Library Module Coverage (Week 2-3)

Fill gaps in library modules:

**Critical Modules:**
- `docker/` - Container operations
- `build/` - Build system
- `nginx/` - Reverse proxy
- `ssl/` - Certificate management
- `errors/` - Error handling

**Important Modules:**
- `deployment/` - Deployment automation
- `services/` - Service initialization
- `logging/` - Logging utilities
- `help/` - Help system

### Phase 3: Service-Specific Tests (Week 3-4)

**PostgreSQL:**
- Test all 60+ extensions
- Connection pooling
- Backup/restore
- Replication

**Hasura:**
- Metadata validation
- Permission system
- Remote schemas
- Event triggers

**Auth:**
- All OAuth providers
- SAML integration
- LDAP integration
- MFA flows

**Storage:**
- MinIO operations
- S3 compatibility
- Azure Blob
- GCS integration

**Search:**
- MeiliSearch
- Typesense
- Sonic
- Elasticsearch

**Email:**
- MailPit (dev)
- SMTP
- SendGrid
- Mailgun

### Phase 4: Integration & E2E Tests (Week 4-5)

**End-to-End Workflows:**
- Full project initialization → build → start → stop
- Tenant creation → user creation → authentication → API call
- Backup → restore → verification
- Deployment → scaling → rollback

**Multi-Service Interactions:**
- Auth + Hasura + PostgreSQL
- Storage + Nginx + SSL
- Monitoring + Logging + Alerts
- Billing + Webhooks + Email

### Phase 5: Error Scenarios (Week 5-6)

**Realistic Errors Only:**
- Missing Docker installation
- Docker daemon not running
- Insufficient permissions
- Invalid configuration
- Network timeouts
- Disk space issues
- Memory constraints

### Phase 6: Performance & Benchmarks (Week 6)

**Performance Tests:**
- Build time benchmarks
- Start time benchmarks
- Query performance
- API response times

**Scalability Tests:**
- 1, 10, 100, 1000 tenants
- 1, 10, 100 concurrent users
- Large datasets

---

## Test Organization

### Directory Structure

```
src/tests/
├── unit/                          # Unit tests (150+ files)
│   ├── cli/                      # CLI command tests
│   │   ├── test-version.sh
│   │   ├── test-completion.sh
│   │   ├── test-audit.sh
│   │   ├── test-history.sh
│   │   ├── test-metrics.sh
│   │   ├── test-doctor.sh
│   │   ├── test-health.sh
│   │   ├── test-status.sh
│   │   ├── test-logs.sh
│   │   └── ... (50+ files)
│   ├── lib/                      # Library module tests
│   │   ├── test-docker.sh
│   │   ├── test-build.sh
│   │   ├── test-nginx.sh
│   │   ├── test-ssl.sh
│   │   ├── test-deployment.sh
│   │   ├── test-services.sh
│   │   ├── test-logging.sh
│   │   ├── test-errors.sh
│   │   └── ... (100+ files)
│   └── services/                 # Service-specific tests
│       ├── test-postgres-extensions.sh
│       ├── test-hasura-metadata.sh
│       ├── test-auth-providers.sh
│       ├── test-storage-backends.sh
│       ├── test-search-engines.sh
│       └── test-email-providers.sh
├── integration/                   # Integration tests (50+ files)
│   ├── test-full-workflow.sh
│   ├── test-auth-flow.sh
│   ├── test-backup-restore-flow.sh
│   ├── test-deployment-flow.sh
│   ├── test-multi-tenant-flow.sh
│   └── ...
├── e2e/                          # End-to-end tests (20+ files)
│   ├── test-project-lifecycle.sh
│   ├── test-user-journey.sh
│   ├── test-api-workflow.sh
│   └── ...
├── network/                      # Network & infrastructure (10+ files)
│   ├── test-nginx-routing.sh
│   ├── test-ssl-certificates.sh
│   ├── test-dns-resolution.sh
│   └── test-proxy-configuration.sh
├── errors/                       # Error scenario tests (15+ files)
│   ├── test-missing-dependencies.sh
│   ├── test-permission-errors.sh
│   ├── test-configuration-errors.sh
│   ├── test-resource-limits.sh
│   └── ...
├── performance/                  # Performance tests (10+ files)
│   ├── test-build-performance.sh
│   ├── test-start-performance.sh
│   ├── test-query-performance.sh
│   └── ...
├── mocks/                        # Mock infrastructure
│   └── mock-infrastructure.sh
└── lib/                          # Test utilities
    ├── test-framework.sh
    └── test-framework-enhanced.sh
```

---

## Reliability Patterns

### Pattern 1: Timeout Protection

```bash
test_with_timeout() {
  run_test_with_timeout "my_test_function" 30
  assert_success
}

my_test_function() {
  # Test code that might hang
  nself build
}
```

### Pattern 2: Retry Flaky Operations

```bash
test_network_operation() {
  retry_test check_network 3
  assert_success
}

check_network() {
  curl -sSf https://api.example.com/health
}
```

### Pattern 3: Skip Impossible Scenarios

```bash
test_external_service() {
  if ! check_network_available; then
    skip_test "Network not available (CI environment)"
    return 0
  fi

  # Actual test
  run_external_api_test
  assert_success
}
```

### Pattern 4: Fast Cleanup

```bash
test_with_cleanup() {
  local test_dir
  test_dir=$(create_test_tmpfs)

  ensure_cleanup "rm -rf '$test_dir'"

  # Test code
  touch "$test_dir/test.txt"
  assert_file_exists "$test_dir/test.txt"

  # Cleanup runs automatically on EXIT
}
```

### Pattern 5: Mock External Dependencies

```bash
test_docker_operation() {
  if ! docker ps >/dev/null 2>&1; then
    # Docker not available - use mock
    export -f mock_docker
    alias docker=mock_docker
  fi

  # Test code works with real or mocked Docker
  result=$(docker ps)
  assert_success
}
```

---

## Coverage Exclusions

Mark untestable code to exclude from coverage:

```bash
# Defensive programming - mark as untestable
# coverage:ignore-next
if [[ "$IMPOSSIBLE_SCENARIO" == "true" ]]; then
  log_error "This should never happen"
  exit 1
fi

# Debug logging - exclude from coverage
# coverage:ignore-start
if [[ "$DEBUG" == "true" ]]; then
  printf "Debug: %s\n" "$debug_info"
fi
# coverage:ignore-end
```

**What to Exclude:**
- Debug logging statements
- Defensive error handlers for impossible scenarios
- Platform-specific workarounds
- Development-only code

**What NOT to Exclude:**
- Business logic
- Public APIs
- Realistic error scenarios
- User-facing features

---

## CI/CD Optimization

### GitHub Actions Workflow

**File:** `.github/workflows/test-coverage.yml`

```yaml
name: Test Coverage

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]

jobs:
  coverage:
    runs-on: ubuntu-latest

    steps:
    - uses: actions/checkout@v3

    - name: Cache test data
      uses: actions/cache@v3
      with:
        path: |
          ~/.docker
          /tmp/nself-test
        key: test-cache-${{ hashFiles('src/**/*.sh') }}

    - name: Run tests with coverage
      run: |
        ./scripts/run-tests-with-coverage.sh

    - name: Upload coverage report
      uses: codecov/codecov-action@v3
      with:
        files: ./coverage/coverage.xml
        flags: unittests
        name: codecov-nself

    - name: Generate HTML report
      run: |
        ./scripts/coverage-html.sh

    - name: Upload HTML report
      uses: actions/upload-artifact@v3
      with:
        name: coverage-report
        path: coverage/html/
```

### Parallel Test Execution

**File:** `scripts/run-tests-parallel.sh`

```bash
#!/usr/bin/env bash
# Run tests in parallel for faster execution

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$PROJECT_ROOT/src/tests"

# Number of parallel jobs
JOBS=${JOBS:-4}

# Run unit tests in parallel
printf "Running unit tests in parallel (jobs=%d)...\n" "$JOBS"
find "$TEST_DIR/unit" -name "test-*.sh" -print0 | \
  xargs -0 -P "$JOBS" -I {} bash -c '{} || exit 255'

# Run integration tests sequentially (may have dependencies)
printf "Running integration tests sequentially...\n"
find "$TEST_DIR/integration" -name "test-*.sh" -exec {} \;

printf "All tests passed!\n"
```

---

## Success Metrics

### Coverage Targets

- **Overall Coverage:** 100%
- **CLI Commands:** 100% (all commands tested)
- **Library Modules:** 100% (all modules tested)
- **Critical Paths:** 100% (no gaps in critical functionality)
- **Error Scenarios:** 95% (realistic errors covered)

### Quality Targets

- **Test Reliability:** 99%+ (tests pass consistently)
- **Test Speed:** Full suite < 10 minutes in CI
- **CI Success Rate:** 95%+ (flaky tests eliminated)
- **Code Duplication:** < 5% (DRY tests)

### Performance Targets

- **Unit Tests:** < 5 minutes
- **Integration Tests:** < 3 minutes
- **E2E Tests:** < 2 minutes
- **Total Suite:** < 10 minutes

---

## Implementation Timeline

### Week 1: CLI Commands (Priority 1-2)
- Days 1-2: Critical commands (doctor, health, status, version, urls)
- Days 3-4: High priority (logs, exec, audit, history, metrics)
- Day 5: Review and refinement

### Week 2: CLI Commands (Priority 3-4) + Library Modules
- Days 1-2: Medium priority CLI (lifecycle, cleanup, recovery)
- Days 3-5: Critical library modules (docker, build, nginx, ssl)

### Week 3: Library Modules + Service Tests
- Days 1-3: Important library modules (deployment, services, logging)
- Days 4-5: PostgreSQL and Hasura tests

### Week 4: Service Tests + Integration Tests
- Days 1-2: Auth, Storage, Search, Email tests
- Days 3-5: Integration test workflows

### Week 5: E2E Tests + Error Scenarios
- Days 1-3: End-to-end workflows
- Days 4-5: Realistic error scenarios

### Week 6: Performance + Polish
- Days 1-2: Performance benchmarks
- Days 3-4: Coverage report and gaps
- Day 5: Documentation and final review

---

## Next Steps

1. **Run coverage analysis:**
   ```bash
   ./scripts/coverage-report.sh > coverage/current-coverage.md
   ```

2. **Generate test stubs:**
   ```bash
   ./scripts/generate-missing-tests.sh
   ```

3. **Start with critical CLI commands:**
   ```bash
   # Create test file
   touch src/tests/unit/cli/test-doctor.sh
   chmod +x src/tests/unit/cli/test-doctor.sh

   # Implement tests
   vim src/tests/unit/cli/test-doctor.sh
   ```

4. **Run and verify:**
   ```bash
   ./src/tests/unit/cli/test-doctor.sh
   ```

5. **Track progress:**
   ```bash
   ./scripts/coverage-report.sh
   ```

---

## Conclusion

This plan provides a systematic approach to achieving 100% test coverage with:

1. **Clear roadmap** - 6-week timeline with specific deliverables
2. **Prioritization** - Critical paths first, nice-to-haves last
3. **Reliability** - Enhanced test framework with timeout, retry, cleanup
4. **Speed** - Parallel execution, fast mocks, tmpfs for file I/O
5. **Maintainability** - Well-organized, DRY, documented tests

**Target:** 1,200+ tests covering 100% of critical code paths with 99%+ reliability in < 10 minutes total execution time.
