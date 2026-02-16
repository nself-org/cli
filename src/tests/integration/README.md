# nself Integration Test Suite v0.9.8

Comprehensive end-to-end integration tests for nself workflows.

## Overview

This test suite validates critical user workflows work correctly from start to finish. Unlike unit tests that test individual functions, integration tests verify complete user journeys.

## Test Suites

### 1. Full Deployment Workflow (`test-full-deployment.sh`)

Tests the complete deployment lifecycle:

- `nself init --simple` - Initialize project
- Modify `.env` configuration
- `nself build` - Generate configurations
- `nself start` - Start all services
- Wait for services to become healthy
- Verify `nself status` shows all running
- Test `nself urls` accessibility
- Verify database connection
- Test Hasura GraphQL endpoint
- Test Auth service endpoint
- `nself stop` - Clean shutdown
- `nself start` - Restart verification
- `nself restart` - Restart command test

**Duration**: ~5-7 minutes

### 2. Multi-Tenant Workflow (`test-multi-tenant-workflow.sh`)

Tests complete tenant lifecycle:

- Create tenants
- Add members to tenants
- Assign roles (admin, member, etc.)
- Test tenant data isolation
- Update tenant settings
- List all tenants
- Remove members
- Delete tenant
- Verify cleanup and isolation

**Duration**: ~3-4 minutes

### 3. Backup & Restore Workflow (`test-backup-restore-workflow.sh`)

Tests backup and restore functionality:

- Start services with test data
- Create full backup
- Verify backup file validity
- Modify database
- Restore from backup
- Verify data matches original
- Test incremental backup
- Test backup cleanup (retention policy)
- Verify automated backup scheduling

**Duration**: ~4-5 minutes

### 4. Database Migration Workflow (`test-migration-workflow.sh`)

Tests database migration system:

- Run initial migrations
- Verify schema created
- Create new migration
- Execute migration
- Verify schema changes
- Insert test data
- Create alter table migration
- Test migration rollback
- Verify rollback correctness
- Test fresh migrations (reset)
- Test migration locking

**Duration**: ~3-4 minutes

### 5. Monitoring Stack (`test-monitoring-stack.sh`)

Tests the complete monitoring bundle (10 services):

- Enable `MONITORING_ENABLED=true`
- Verify all 10 monitoring services start:
  - Prometheus
  - Grafana
  - Loki
  - Promtail
  - Tempo
  - Alertmanager
  - cAdvisor
  - Node Exporter
  - Postgres Exporter
  - Redis Exporter
- Verify Prometheus scraping metrics
- Test Grafana dashboards
- Verify Loki log aggregation
- Test Alertmanager configuration
- Verify exporters providing metrics
- Test individual service disable

**Duration**: ~6-8 minutes

### 6. Custom Services Workflow (`test-custom-services-workflow.sh`)

Tests custom service (CS_N) functionality:

- Configure CS_1 through CS_4
- Verify service directories generated
- Check docker-compose configuration
- Start all custom services
- Verify nginx routes
- Test service endpoints
- Verify logs accessible
- Modify service code and rebuild
- Test service restart
- Remove custom service
- Add new custom service
- Verify service isolation
- Test environment variables

**Duration**: ~4-5 minutes

## Quick Start

### Run All Integration Tests

```bash
cd src/tests/integration
./run-all-integration-tests.sh
```

### Run Specific Test Suite

```bash
cd src/tests/integration
./run-all-integration-tests.sh --test full-deployment
```

### Run Individual Test Directly

```bash
cd src/tests/integration
./test-full-deployment.sh
```

## Test Helper Utilities

### Integration Helpers (`utils/integration-helpers.sh`)

Provides reusable functions for all integration tests:

**Project Management:**
- `setup_test_project()` - Create isolated test environment
- `cleanup_test_project()` - Remove test project and containers
- `generate_test_project_name()` - Unique project names

**Service Health:**
- `wait_for_service_healthy()` - Wait for specific service
- `wait_for_all_services_healthy()` - Wait for all services
- `assert_service_running()` - Verify service is running

**Endpoint Verification:**
- `verify_endpoint_accessible()` - Check HTTP endpoint
- `verify_graphql_endpoint()` - Test GraphQL endpoint

**Data Management:**
- `create_test_data()` - Insert test data into database
- `verify_test_data()` - Check if test data exists
- `clear_test_data()` - Remove test data

**Service Mocking:**
- `mock_external_service()` - Create mock HTTP endpoint
- `stop_mock_service()` - Stop mock service

**Utilities:**
- `get_service_logs()` - Retrieve service logs
- `get_service_container_id()` - Get container ID
- `exec_in_service()` - Execute command in container
- `wait_for_port()` - Wait for port to be listening

## Writing New Integration Tests

### Test Template

```bash
#!/usr/bin/env bash
# test-my-workflow.sh - Description of what this tests

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="my-workflow"
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
test_01_setup() {
  describe "Test 1: Setup"
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Your setup code
  "$NSELF_ROOT/bin/nself" init --simple

  pass "Setup complete"
}

test_02_your_test() {
  describe "Test 2: Your test description"

  # Your test code

  pass "Test passed"
}

# Main runner
main() {
  start_suite "My Workflow Integration Test"

  test_01_setup
  test_02_your_test

  # Print summary
  printf "\n=================================================================\n"
  printf "Test Summary\n"
  printf "=================================================================\n"
  printf "Total Tests: %d\n" "$TESTS_RUN"
  printf "Passed: %d\n" "$TESTS_PASSED"
  printf "Failed: %d\n" "$TESTS_FAILED"
  printf "=================================================================\n\n"

  if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
```

### Best Practices

1. **Independence**: Each test should be completely independent
2. **Cleanup**: Always cleanup test environments (use trap)
3. **Unique Names**: Use unique project names to avoid conflicts
4. **Timeouts**: Add reasonable timeouts for service health checks
5. **Descriptive**: Use clear test descriptions
6. **Assertions**: Use helper assertions from test framework
7. **Logging**: Capture logs on failures for debugging
8. **Mock External**: Mock external services when possible

### Test Naming Convention

- Prefix: `test-`
- Format: `test-<workflow>-<aspect>.sh`
- Examples:
  - `test-full-deployment.sh`
  - `test-multi-tenant-workflow.sh`
  - `test-backup-restore-workflow.sh`

## CI/CD Integration

### GitHub Actions Workflow

Located at `.github/workflows/integration-tests.yml`

**Triggers:**
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop`
- Manual workflow dispatch

**Matrix Testing:**
- Ubuntu Latest
- macOS Latest

**Artifacts:**
- Test results uploaded as artifacts
- Retained for 7 days

### Running Specific Tests in CI

Use workflow dispatch with `test_suite` input:

```yaml
test_suite: full-deployment
```

## Test Environment

### Isolation

Each test runs in:
- Unique temporary directory (`/tmp/nself-integration-test-*`)
- Isolated Docker containers
- Separate Docker networks
- Independent databases

### Cleanup

Tests automatically cleanup:
- Docker containers
- Docker volumes
- Temporary directories
- Network resources

### Resource Requirements

**Minimum:**
- 4 GB RAM
- 10 GB disk space
- Docker and Docker Compose

**Recommended:**
- 8 GB RAM
- 20 GB disk space
- Fast SSD

## Debugging Integration Tests

### Run with Verbose Output

```bash
./run-all-integration-tests.sh --verbose
```

### Keep Test Environment for Inspection

```bash
# Edit test file and set:
CLEANUP_ON_EXIT=false

# Then run test
./test-full-deployment.sh

# Inspect environment
cd /tmp/nself-integration-test-*
docker-compose ps
docker-compose logs
```

### Check Service Logs

```bash
# During test execution
docker-compose logs -f <service_name>

# After test (if cleanup disabled)
cd /tmp/nself-integration-test-*
docker-compose logs <service_name> --tail=100
```

### Debug Failed Test

```bash
# Run specific test
./test-full-deployment.sh

# Check output in /tmp
ls -la /tmp/nself-integration-test-output-*

# Inspect test environment
cd /tmp/nself-integration-test-*
```

## Performance Benchmarks

| Test Suite | Duration | Containers | CPU | Memory |
|------------|----------|------------|-----|--------|
| Full Deployment | 5-7 min | 8-12 | Medium | 2-3 GB |
| Multi-Tenant | 3-4 min | 4-6 | Low | 1-2 GB |
| Backup/Restore | 4-5 min | 4-6 | Medium | 1-2 GB |
| Migrations | 3-4 min | 4-6 | Low | 1-2 GB |
| Monitoring Stack | 6-8 min | 14-20 | High | 3-4 GB |
| Custom Services | 4-5 min | 8-12 | Medium | 2-3 GB |

**Total (all tests)**: ~25-35 minutes

## Troubleshooting

### Docker Errors

```bash
# Clean Docker system
docker system prune -af --volumes

# Restart Docker daemon
sudo systemctl restart docker  # Linux
# macOS: Restart Docker Desktop
```

### Port Conflicts

```bash
# Check ports in use
lsof -i :5432  # PostgreSQL
lsof -i :8080  # Hasura

# Kill conflicting processes
kill -9 <PID>
```

### Permission Issues

```bash
# Fix test file permissions
chmod +x src/tests/integration/test-*.sh
chmod +x src/tests/integration/run-all-integration-tests.sh
chmod +x src/tests/integration/utils/integration-helpers.sh
```

### Cleanup Stuck Containers

```bash
# Remove all test containers
docker ps -a | grep nself-integration-test | awk '{print $1}' | xargs docker rm -f

# Remove test directories
rm -rf /tmp/nself-integration-test-*
```

## Contributing

### Adding New Tests

1. Create test file: `test-<workflow>.sh`
2. Use template above
3. Make executable: `chmod +x test-<workflow>.sh`
4. Test locally: `./test-<workflow>.sh`
5. Add to CI workflow paths if needed
6. Update this README

### Test Coverage Goals

- All critical user workflows
- End-to-end scenarios users actually perform
- Error handling and recovery
- Data integrity verification
- Service interaction validation

## Support

For issues with integration tests:

1. Check test output logs
2. Review service logs
3. Run with verbose mode
4. Disable cleanup for inspection
5. Create GitHub issue with:
   - Test name
   - Error output
   - Environment details
   - Steps to reproduce

## Version

**Integration Test Suite Version**: 0.9.8
**Last Updated**: 2026-01-31
**Compatibility**: nself v0.9.9+
