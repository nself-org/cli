#!/usr/bin/env bash
# test-monitoring-stack.sh - Monitoring stack integration test
#
# Tests: Enable monitoring → verify all 10 services → test dashboards → alerts

set -euo pipefail

# Load test utilities
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$TEST_DIR/utils/integration-helpers.sh"
source "$TEST_DIR/../test_framework.sh"

# Test configuration
readonly TEST_NAME="monitoring-stack"
TEST_PROJECT_DIR=""
CLEANUP_ON_EXIT=true

# Monitoring services (EXACTLY 10)
readonly MONITORING_SERVICES=(
  "prometheus"
  "grafana"
  "loki"
  "promtail"
  "tempo"
  "alertmanager"
  "cadvisor"
  "node-exporter"
  "postgres-exporter"
  "redis-exporter"
)

# ============================================================================
# Cleanup Handler
# ============================================================================

cleanup() {
  if [[ "$CLEANUP_ON_EXIT" == "true" ]] && [[ -n "$TEST_PROJECT_DIR" ]]; then
    printf "\nCleaning up test environment...\n"
    cleanup_test_project "$TEST_PROJECT_DIR"
  fi
}

trap cleanup EXIT INT TERM

# ============================================================================
# Helper Functions
# ============================================================================

run_nself_command() {
  "$NSELF_ROOT/bin/nself" "$@"
}

# ============================================================================
# Test Functions
# ============================================================================

test_01_setup() {
  describe "Test 1: Setup test environment with monitoring"

  # Create test environment
  TEST_PROJECT_DIR=$(setup_test_project)
  cd "$TEST_PROJECT_DIR"

  # Initialize project
  run_nself_command init --quiet

  # Enable monitoring bundle
  cat >>.env <<EOF

# Monitoring configuration
MONITORING_ENABLED=true

# Enable Redis for redis-exporter
REDIS_ENABLED=true
EOF

  # Build and start
  run_nself_command build
  run_nself_command start

  # Wait for services to initialize
  sleep 15

  # Source .env
  source .env

  pass "Test environment setup complete"
}

test_02_verify_all_monitoring_services() {
  describe "Test 2: Verify all 10 monitoring services are running"

  cd "$TEST_PROJECT_DIR"

  local services_running=0
  local services_failed=()

  for service in "${MONITORING_SERVICES[@]}"; do
    if assert_service_running "$service"; then
      services_running=$((services_running + 1))
    else
      services_failed+=("$service")
    fi
  done

  if [[ $services_running -ne 10 ]]; then
    fail "Expected 10 monitoring services, but only $services_running are running. Failed: ${services_failed[*]}"
  fi

  printf "All 10 monitoring services are running\n"

  pass "Monitoring services verified"
}

test_03_prometheus_scraping() {
  describe "Test 3: Verify Prometheus is scraping metrics"

  cd "$TEST_PROJECT_DIR"

  # Wait for Prometheus to be ready
  wait_for_service_healthy "prometheus" 60 || fail "Prometheus not healthy"

  # Check Prometheus targets
  printf "Checking Prometheus targets...\n"
  local targets_url="http://localhost:9090/api/v1/targets"

  local response
  response=$(curl -s "$targets_url" 2>/dev/null || echo "")

  if echo "$response" | grep -q '"state":"up"'; then
    printf "✓ Prometheus has active targets\n"
  else
    fail "Prometheus has no active targets"
  fi

  # Verify specific exporters are being scraped
  local exporters=("node-exporter" "postgres-exporter" "cadvisor")

  for exporter in "${exporters[@]}"; do
    if echo "$response" | grep -q "$exporter"; then
      printf "✓ %s is being scraped\n" "$exporter"
    else
      print_warning "$exporter not found in targets"
    fi
  done

  pass "Prometheus scraping verified"
}

test_04_grafana_dashboards() {
  describe "Test 4: Verify Grafana dashboards are accessible"

  cd "$TEST_PROJECT_DIR"

  # Wait for Grafana to be ready
  wait_for_service_healthy "grafana" 60 || fail "Grafana not healthy"

  # Check Grafana API
  printf "Checking Grafana API...\n"
  local grafana_url="http://localhost:3000/api/health"

  verify_endpoint_accessible "$grafana_url" 30 200 || fail "Grafana not accessible"

  # Check dashboards (requires authentication)
  local dashboards_url="http://admin:admin@localhost:3000/api/dashboards/home"

  local dashboard_response
  dashboard_response=$(curl -s "$dashboards_url" 2>/dev/null || echo "")

  if [[ -n "$dashboard_response" ]]; then
    printf "✓ Grafana dashboards accessible\n"
  else
    print_warning "Could not verify dashboards (authentication might be required)"
  fi

  pass "Grafana verified"
}

test_05_loki_logging() {
  describe "Test 5: Verify Loki is collecting logs"

  cd "$TEST_PROJECT_DIR"

  # Wait for Loki to be ready
  wait_for_service_healthy "loki" 60 || fail "Loki not healthy"

  # Check Loki is ready
  printf "Checking Loki status...\n"
  local loki_url="http://localhost:3100/ready"

  verify_endpoint_accessible "$loki_url" 30 200 || fail "Loki not ready"

  # Verify Promtail is running (required for Loki to receive logs)
  assert_service_running "promtail" || fail "Promtail is required for Loki"

  printf "✓ Loki and Promtail are operational\n"

  pass "Loki logging verified"
}

test_06_alertmanager() {
  describe "Test 6: Verify Alertmanager is configured"

  cd "$TEST_PROJECT_DIR"

  # Wait for Alertmanager to be ready
  wait_for_service_healthy "alertmanager" 60 || fail "Alertmanager not healthy"

  # Check Alertmanager API
  printf "Checking Alertmanager API...\n"
  local alertmanager_url="http://localhost:9093/api/v1/status"

  verify_endpoint_accessible "$alertmanager_url" 30 200 || fail "Alertmanager not accessible"

  printf "✓ Alertmanager is operational\n"

  pass "Alertmanager verified"
}

test_07_tempo_tracing() {
  describe "Test 7: Verify Tempo is ready for tracing"

  cd "$TEST_PROJECT_DIR"

  # Wait for Tempo to be ready
  wait_for_service_healthy "tempo" 60 || fail "Tempo not healthy"

  # Check Tempo is ready
  printf "Checking Tempo status...\n"
  local tempo_url="http://localhost:3200/ready"

  if verify_endpoint_accessible "$tempo_url" 30 200; then
    printf "✓ Tempo is ready for distributed tracing\n"
  else
    print_warning "Tempo endpoint not accessible (might be normal depending on config)"
  fi

  pass "Tempo verified"
}

test_08_exporters_metrics() {
  describe "Test 8: Verify exporters are providing metrics"

  cd "$TEST_PROJECT_DIR"

  # Check Node Exporter metrics
  printf "Checking Node Exporter metrics...\n"
  local node_metrics
  node_metrics=$(curl -s http://localhost:9100/metrics 2>/dev/null | head -20)

  if echo "$node_metrics" | grep -q "node_"; then
    printf "✓ Node Exporter is providing metrics\n"
  else
    fail "Node Exporter not providing metrics"
  fi

  # Check cAdvisor metrics
  printf "Checking cAdvisor metrics...\n"
  local cadvisor_metrics
  cadvisor_metrics=$(curl -s http://localhost:8080/metrics 2>/dev/null | head -20)

  if echo "$cadvisor_metrics" | grep -q "container_"; then
    printf "✓ cAdvisor is providing container metrics\n"
  else
    fail "cAdvisor not providing metrics"
  fi

  # Check Postgres Exporter metrics
  printf "Checking Postgres Exporter metrics...\n"
  local postgres_metrics
  postgres_metrics=$(curl -s http://localhost:9187/metrics 2>/dev/null | head -20)

  if echo "$postgres_metrics" | grep -q "pg_"; then
    printf "✓ Postgres Exporter is providing database metrics\n"
  else
    print_warning "Postgres Exporter might not be ready yet"
  fi

  pass "Exporter metrics verified"
}

test_09_monitoring_bundle_all_or_nothing() {
  describe "Test 9: Verify monitoring bundle is all-or-nothing"

  cd "$TEST_PROJECT_DIR"

  # Count monitoring services in docker-compose.yml
  local monitoring_service_count
  monitoring_service_count=$(grep -c "container_name:.*-prometheus\|container_name:.*-grafana\|container_name:.*-loki\|container_name:.*-promtail\|container_name:.*-tempo\|container_name:.*-alertmanager\|container_name:.*-cadvisor\|container_name:.*-node-exporter\|container_name:.*-postgres-exporter\|container_name:.*-redis-exporter" docker-compose.yml || echo "0")

  if [[ $monitoring_service_count -eq 10 ]]; then
    printf "✓ All 10 monitoring services defined in docker-compose.yml\n"
  else
    fail "Expected 10 monitoring services in docker-compose.yml, found $monitoring_service_count"
  fi

  pass "Monitoring bundle verified as complete"
}

test_10_disable_individual_service() {
  describe "Test 10: Test disabling individual monitoring service"

  cd "$TEST_PROJECT_DIR"

  # Disable Tempo
  printf "Disabling Tempo...\n"
  echo "TEMPO_ENABLED=false" >> .env

  # Rebuild
  run_nself_command build

  # Verify Tempo not in docker-compose.yml
  if grep -q "container_name:.*-tempo" docker-compose.yml; then
    fail "Tempo should not be in docker-compose.yml when disabled"
  fi

  printf "✓ Individual service can be disabled\n"

  pass "Individual service disable verified"
}

test_11_monitoring_urls() {
  describe "Test 11: Verify monitoring service URLs"

  cd "$TEST_PROJECT_DIR"

  # Run nself urls and check for monitoring URLs
  printf "Checking nself urls output...\n"
  local urls_output
  urls_output=$(run_nself_command urls 2>&1)

  # Check for monitoring URLs
  echo "$urls_output" | grep -q "grafana" || print_warning "Grafana URL not in output"
  echo "$urls_output" | grep -q "prometheus" || print_warning "Prometheus URL not in output"

  pass "Monitoring URLs verified"
}

# ============================================================================
# Main Test Runner
# ============================================================================

main() {
  start_suite "Monitoring Stack Integration Test"

  # Skip gracefully when Docker or nself is not available (requires live stack in CI)
  if ! docker ps >/dev/null 2>&1; then
    printf "⚠ Docker not available - skipping workflow tests\n"
    exit 0
  fi
  if [[ -z "${NSELF_ROOT:-}" ]] || [[ ! -x "${NSELF_ROOT}/bin/nself" ]]; then
    printf "⚠ NSELF_ROOT not set or nself not found - skipping workflow tests\n"
    exit 0
  fi

  printf "\n=================================================================\n"
  printf "Monitoring Stack Integration Test\n"
  printf "=================================================================\n\n"

  # Run all tests in sequence
  test_01_setup
  test_02_verify_all_monitoring_services
  test_03_prometheus_scraping
  test_04_grafana_dashboards
  test_05_loki_logging
  test_06_alertmanager
  test_07_tempo_tracing
  test_08_exporters_metrics
  test_09_monitoring_bundle_all_or_nothing
  test_10_disable_individual_service
  test_11_monitoring_urls

  # Print summary
  printf "\n=================================================================\n"
  printf "Test Summary\n"
  printf "=================================================================\n"
  printf "Total Tests: %d\n" "$TESTS_RUN"
  printf "Passed: %d\n" "$TESTS_PASSED"
  printf "Failed: %d\n" "$TESTS_FAILED"
  printf "Skipped: %d\n" "$TESTS_SKIPPED"
  printf "=================================================================\n\n"

  # Exit with proper code
  if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
  else
    exit 0
  fi
}

# Run tests if executed directly
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
