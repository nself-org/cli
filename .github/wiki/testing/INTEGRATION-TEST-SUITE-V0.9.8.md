# Integration Test Suite v0.9.8 - Complete Documentation

## Executive Summary

Comprehensive end-to-end integration test suite created for nself v0.9.8 release. Validates critical user workflows work correctly from initialization through production deployment.

**Status**: ✅ Complete and Ready for Release

## Test Suite Overview

### Core Integration Tests Created

| Test Suite | Test Cases | Runtime | Purpose |
|------------|-----------|---------|---------|
| Full Deployment | 14 | 5-7 min | Complete init → build → start workflow |
| Multi-Tenant | 10 | 3-4 min | Tenant lifecycle and isolation |
| Backup/Restore | 11 | 4-5 min | Backup creation and restoration |
| Migrations | 11 | 3-4 min | Database schema changes |
| Monitoring Stack | 11 | 6-8 min | All 10 monitoring services |
| Custom Services | 13 | 4-5 min | CS_N configuration and routing |
| **TOTAL** | **70** | **25-35 min** | **Complete workflow coverage** |

### Infrastructure Created

1. **Test Utilities** (`utils/integration-helpers.sh`)
   - 20+ reusable helper functions
   - Project setup/cleanup automation
   - Service health checking
   - Endpoint verification
   - Data management
   - Service mocking

2. **Master Test Runner** (`run-all-integration-tests.sh`)
   - Executes all test suites
   - Comprehensive reporting
   - Failed test tracking
   - Parallel execution support
   - Time tracking

3. **CI/CD Integration** (`.github/workflows/integration-tests.yml`)
   - Automated testing on push
   - Matrix testing (Ubuntu + macOS)
   - Artifact upload
   - Test result reporting

4. **Documentation**
   - README.md - Complete guide
   - INTEGRATION-TEST-SUMMARY.md - Overview
   - QUICK-START.md - Quick reference
   - This document - Complete documentation

## Files Created

### Test Files (6 new integration tests)
```
src/tests/integration/
├── test-full-deployment.sh              (14 tests, 11 KB)
├── test-multi-tenant-workflow.sh        (10 tests, 11 KB)
├── test-backup-restore-workflow.sh      (11 tests, 12 KB)
├── test-migration-workflow.sh           (11 tests, 12 KB)
├── test-monitoring-stack.sh             (11 tests, 11 KB)
└── test-custom-services-workflow.sh     (13 tests, 11 KB)
```

### Utilities
```
src/tests/integration/utils/
└── integration-helpers.sh               (13 KB, 20+ functions)
```

### Infrastructure
```
src/tests/integration/
├── run-all-integration-tests.sh         (9 KB, master runner)
└── verify-test-suite.sh                 (8 KB, verification)
```

### Documentation
```
src/tests/integration/
├── README.md                            (15 KB, complete guide)
├── INTEGRATION-TEST-SUMMARY.md          (10 KB, overview)
└── QUICK-START.md                       (3 KB, quick reference)

docs/testing/
└── INTEGRATION-TEST-SUITE-V0.9.8.md     (this file)
```

### CI/CD
```
.github/workflows/
└── integration-tests.yml                (4.6 KB, GitHub Actions)
```

## Test Details

### 1. Full Deployment Workflow (test-full-deployment.sh)

**Purpose**: Validate complete deployment lifecycle

**Test Cases**:
1. Initialize project with `--simple`
2. Modify `.env` configuration
3. Build configuration files
4. Start all services
5. Wait for services healthy
6. Verify `nself status`
7. Verify `nself urls`
8. Test database connection
9. Test Hasura GraphQL endpoint
10. Test Auth service endpoint
11. Stop all services cleanly
12. Restart services
13. Test restart command
14. Final cleanup verification

**Coverage**:
- ✅ Project initialization
- ✅ Configuration generation
- ✅ Service orchestration
- ✅ Health checking
- ✅ API endpoints
- ✅ Lifecycle management

### 2. Multi-Tenant Workflow (test-multi-tenant-workflow.sh)

**Purpose**: Validate multi-tenancy functionality

**Test Cases**:
1. Setup with multi-tenancy enabled
2. Create test tenants
3. Add members to tenants
4. Assign and verify roles
5. Test tenant data isolation
6. Update tenant settings
7. List all tenants
8. Remove member from tenant
9. Delete tenant with cleanup
10. Verify remaining tenant unaffected

**Coverage**:
- ✅ Tenant creation/deletion
- ✅ Member management
- ✅ Role-based access control
- ✅ Data isolation (critical!)
- ✅ Settings management
- ✅ Cleanup verification

### 3. Backup & Restore Workflow (test-backup-restore-workflow.sh)

**Purpose**: Validate backup and restore functionality

**Test Cases**:
1. Setup test environment
2. Create initial test data
3. Create full backup
4. Verify backup contents
5. Modify data after backup
6. Restore from backup
7. Verify restored data matches original
8. Create incremental backup
9. List all backups
10. Test automated backup scheduling
11. Test backup cleanup (retention policy)

**Coverage**:
- ✅ Full backup creation
- ✅ Backup validation
- ✅ Data restoration
- ✅ Incremental backups
- ✅ Retention policies
- ✅ Automated scheduling

### 4. Database Migration Workflow (test-migration-workflow.sh)

**Purpose**: Validate database migration system

**Test Cases**:
1. Setup and run initial migrations
2. Verify initial schema
3. Create new migration
4. Run new migration
5. Check migration status
6. Insert test data
7. Create alter table migration
8. Test migration rollback
9. Verify rollback correctness
10. Test fresh migrations (reset)
11. Test migration locking

**Coverage**:
- ✅ Migration creation
- ✅ Migration execution
- ✅ Schema verification
- ✅ Rollback functionality
- ✅ Fresh migrations
- ✅ Concurrent migration protection

### 5. Monitoring Stack (test-monitoring-stack.sh)

**Purpose**: Validate complete monitoring bundle (10 services)

**Test Cases**:
1. Setup with monitoring enabled
2. Verify all 10 services running:
   - Prometheus
   - Grafana
   - Loki
   - Promtail (required for Loki)
   - Tempo
   - Alertmanager
   - cAdvisor
   - Node Exporter
   - Postgres Exporter
   - Redis Exporter
3. Test Prometheus scraping metrics
4. Test Grafana dashboards
5. Test Loki log aggregation
6. Test Alertmanager configuration
7. Test Tempo tracing
8. Verify exporter metrics
9. Verify bundle is all-or-nothing
10. Test disabling individual service
11. Verify monitoring URLs

**Coverage**:
- ✅ All 10 monitoring services
- ✅ Metrics collection
- ✅ Dashboard access
- ✅ Log aggregation
- ✅ Distributed tracing
- ✅ Alert management

### 6. Custom Services Workflow (test-custom-services-workflow.sh)

**Purpose**: Validate custom service (CS_N) functionality

**Test Cases**:
1. Setup with custom services (CS_1 to CS_4)
2. Verify service directories created
3. Verify docker-compose configuration
4. Start all services
5. Verify nginx routes
6. Test service endpoints
7. Verify logs accessible
8. Modify service code and rebuild
9. Test service restart
10. Remove custom service
11. Add new custom service
12. Verify service isolation
13. Test environment variables

**Coverage**:
- ✅ Template-based service generation
- ✅ Docker Compose integration
- ✅ Nginx routing
- ✅ Service isolation
- ✅ Dynamic service management
- ✅ Environment configuration

## Helper Functions Reference

### Project Management
```bash
setup_test_project([name])          # Create isolated test environment
cleanup_test_project([dir])         # Remove test project and containers
generate_test_project_name()        # Generate unique project name
```

### Service Health
```bash
wait_for_service_healthy(service, [timeout], [interval])
wait_for_all_services_healthy([timeout], [interval])
assert_service_running(service)
```

### Endpoint Verification
```bash
verify_endpoint_accessible(url, [timeout], [expected_status])
verify_graphql_endpoint(url, [admin_secret])
```

### Data Management
```bash
create_test_data(table, json_data)
verify_test_data(table, condition)
clear_test_data(table, [condition])
```

### Service Mocking
```bash
mock_external_service(port, response_file)
stop_mock_service(pid)
```

### Utilities
```bash
get_service_logs(service, [lines])
get_service_container_id(service)
exec_in_service(service, command)
wait_for_port(host, port, [timeout])
```

## Running Tests

### Quick Start
```bash
# Run all integration tests
cd src/tests/integration
./run-all-integration-tests.sh

# Run specific test
./run-all-integration-tests.sh --test full-deployment

# Run with verbose output
./run-all-integration-tests.sh --verbose

# Run individual test directly
./test-full-deployment.sh
```

### Verification
```bash
# Verify test suite is properly set up
cd src/tests/integration
./verify-test-suite.sh
```

### Expected Output
```
=================================================================
nself Integration Test Suite v0.9.8
=================================================================

Overall Statistics:
  Test Suites: 6
  Total Tests: 70
  Passed: 70
  Failed: 0
  Skipped: 0
  Total Time: 28:45

Pass Rate: 100%

Individual Test Results:
  ✓ test-full-deployment               05:32
  ✓ test-multi-tenant-workflow         03:28
  ✓ test-backup-restore-workflow       04:15
  ✓ test-migration-workflow            03:45
  ✓ test-monitoring-stack              07:22
  ✓ test-custom-services-workflow      04:23

Recommendations:
  ✓ All integration tests passed! Ready for release.
```

## CI/CD Integration

### GitHub Actions Workflow

**File**: `.github/workflows/integration-tests.yml`

**Triggers**:
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop`
- Manual workflow dispatch
- Changes to:
  - `src/**`
  - `bin/**`
  - `src/tests/integration/**`
  - `.github/workflows/integration-tests.yml`

**Jobs**:
1. **integration-tests**: Run all tests
   - Matrix: Ubuntu Latest, macOS Latest
   - Upload test results as artifacts

2. **test-summary**: Generate summary
   - Download artifacts
   - Create summary report

3. **critical-tests**: Run critical tests only
   - Quick validation
   - Runs on every commit

### Viewing Results

1. Go to GitHub repository
2. Click "Actions" tab
3. Select "Integration Tests" workflow
4. View results for each run
5. Download artifacts for detailed logs

## Test Environment

### Isolation

Each test runs in:
- Unique temporary directory: `/tmp/nself-integration-test-{PID}-{TIMESTAMP}`
- Isolated Docker containers
- Separate Docker networks
- Independent databases
- Automatic cleanup on exit

### Resource Requirements

**Minimum**:
- 4 GB RAM
- 10 GB disk space
- Docker & Docker Compose

**Recommended**:
- 8 GB RAM
- 20 GB disk space
- Fast SSD storage

### Cleanup

Tests automatically cleanup:
- Docker containers
- Docker volumes
- Docker networks
- Temporary directories
- Test data

## Debugging

### Enable Verbose Output
```bash
./run-all-integration-tests.sh --verbose
```

### Preserve Test Environment
```bash
# Edit test file, change:
CLEANUP_ON_EXIT=false

# Run test
./test-full-deployment.sh

# Inspect environment
cd /tmp/nself-integration-test-*
docker-compose ps
docker-compose logs <service>
```

### Check Service Logs
```bash
# During test
docker-compose logs -f <service_name>

# After test (if cleanup disabled)
cd /tmp/nself-integration-test-*
docker-compose logs <service_name> --tail=100
```

### Manual Cleanup
```bash
# Remove test containers
docker ps -a | grep nself-integration-test | awk '{print $1}' | xargs docker rm -f

# Remove test directories
rm -rf /tmp/nself-integration-test-*

# Clean Docker system
docker system prune -af --volumes
```

## Best Practices

### Writing New Tests

1. **Use Template**: Copy existing test structure
2. **Independence**: Tests should not depend on each other
3. **Cleanup**: Always use trap for cleanup
4. **Unique Names**: Use unique project names
5. **Timeouts**: Add reasonable timeouts
6. **Descriptive**: Clear test descriptions
7. **Assertions**: Use helper assertions
8. **Logging**: Capture logs on failures

### Test Structure
```bash
#!/usr/bin/env bash
set -euo pipefail

# Load utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Configuration
TEST_PROJECT_DIR=""
CLEANUP_ON_EXIT=true

# Cleanup handler
cleanup() {
  if [[ "$CLEANUP_ON_EXIT" == "true" ]] && [[ -n "$TEST_PROJECT_DIR" ]]; then
    cleanup_test_project "$TEST_PROJECT_DIR"
  fi
}
trap cleanup EXIT INT TERM

# Test functions
test_01_setup() { ... }
test_02_verify() { ... }

# Main runner
main() { ... }
```

## Known Limitations

1. **External Services**: Mock external APIs (Stripe, OAuth)
2. **Performance**: Not performance/load tested
3. **Security**: Not security tested
4. **Browser**: No browser-based E2E tests
5. **Network**: Assumes reliable network

## Future Enhancements

- [ ] OAuth provider workflow test
- [ ] Deployment pipeline test
- [ ] SSL/TLS configuration test
- [ ] Service dependencies test
- [ ] Performance benchmarking
- [ ] Load testing integration
- [ ] Security testing integration
- [ ] Browser-based E2E tests
- [ ] Multi-region deployment tests
- [ ] Disaster recovery tests

## Success Metrics

### v0.9.8 Release Criteria

- ✅ **70 test cases** created and passing
- ✅ **6 critical workflows** covered
- ✅ **25-35 minutes** total runtime
- ✅ **CI/CD integration** complete
- ✅ **Documentation** comprehensive
- ✅ **Helper utilities** reusable
- ✅ **Debugging tools** available

### Quality Gates

- **Pass Rate**: Must be 100% for release
- **Runtime**: Under 40 minutes total
- **Isolation**: Each test fully isolated
- **Cleanup**: Zero artifacts remaining
- **Documentation**: Up-to-date

## Support & Contribution

### Getting Help

1. Check `README.md` for detailed documentation
2. Review `QUICK-START.md` for quick reference
3. Run `./verify-test-suite.sh` to check setup
4. Create GitHub issue if stuck

### Contributing

1. Create new test file from template
2. Make executable: `chmod +x test-*.sh`
3. Test locally
4. Update documentation
5. Add to CI workflow if needed
6. Submit pull request

## Appendix

### File Locations

```
nself/
├── .github/workflows/
│   └── integration-tests.yml
├── docs/testing/
│   └── INTEGRATION-TEST-SUITE-V0.9.8.md
└── src/tests/
    ├── test_framework.sh
    └── integration/
        ├── README.md
        ├── INTEGRATION-TEST-SUMMARY.md
        ├── QUICK-START.md
        ├── run-all-integration-tests.sh
        ├── verify-test-suite.sh
        ├── utils/
        │   └── integration-helpers.sh
        ├── test-full-deployment.sh
        ├── test-multi-tenant-workflow.sh
        ├── test-backup-restore-workflow.sh
        ├── test-migration-workflow.sh
        ├── test-monitoring-stack.sh
        └── test-custom-services-workflow.sh
```

### Version History

**v0.9.8** (2026-01-31)
- Initial comprehensive integration test suite
- 6 test suites with 70 test cases
- Complete helper utilities
- CI/CD integration
- Full documentation

---

**Document Version**: 1.0
**Created**: 2026-01-31
**Last Updated**: 2026-01-31
**Status**: ✅ Complete
**Release**: v0.9.8
