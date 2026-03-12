#!/usr/bin/env bats
# observability_plugin_test.bats
# Tests for nSelf's built-in observability/monitoring stack.
# Verifies that Prometheus metrics are exported correctly when monitoring is enabled.
# Requires nself services running with monitoring bundle enabled.

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if ! docker_available; then
    skip "Docker not available"
  fi
}

monitoring_running() {
  # Prometheus default port via nself nginx proxy
  curl -fsS "http://localhost:9090/-/healthy" >/dev/null 2>&1 ||
  nself status 2>/dev/null | grep -q "prometheus"
}

skip_if_monitoring_off() {
  if ! monitoring_running; then
    skip "Monitoring bundle not running — enable with MONITORING_ENABLED=true in .env"
  fi
}

# ---------------------------------------------------------------------------
# Monitoring enable/disable (dry-run / help)
# ---------------------------------------------------------------------------

@test "nself monitoring --help exits 0" {
  run nself monitoring --help
  assert_success
}

@test "nself monitoring enable --help exits 0" {
  run nself monitoring enable --help
  assert_success
}

@test "nself monitoring disable --help exits 0" {
  run nself monitoring disable --help
  assert_success
}

@test "nself monitoring status --help exits 0" {
  run nself monitoring status --help
  assert_success
}

# ---------------------------------------------------------------------------
# Node Exporter — host metrics
# ---------------------------------------------------------------------------

@test "node-exporter: /metrics endpoint returns prometheus format" {
  skip_if_no_docker
  skip_if_monitoring_off
  run curl -fsS "http://localhost:9100/metrics"
  assert_success
  # Prometheus text format: lines starting with # HELP or metric_name{
  assert_output --regexp "# HELP|node_"
}

# ---------------------------------------------------------------------------
# Prometheus — scrape endpoint
# ---------------------------------------------------------------------------

@test "prometheus: /-/healthy returns 200" {
  skip_if_no_docker
  skip_if_monitoring_off
  run curl -fsS "http://localhost:9090/-/healthy"
  assert_success
}

@test "prometheus: /metrics endpoint exports go_ metrics" {
  skip_if_no_docker
  skip_if_monitoring_off
  run curl -fsS "http://localhost:9090/metrics"
  assert_success
  assert_output --regexp "go_"
}

@test "prometheus: /api/v1/targets shows nself targets" {
  skip_if_no_docker
  skip_if_monitoring_off
  run curl -fsS "http://localhost:9090/api/v1/targets"
  assert_success
  assert_output --regexp "activeTargets|scrapePool"
}

# ---------------------------------------------------------------------------
# Postgres Exporter
# ---------------------------------------------------------------------------

@test "postgres-exporter: /metrics returns pg_ metrics" {
  skip_if_no_docker
  skip_if_monitoring_off
  # Postgres exporter runs on port 9187
  run curl -fsS "http://localhost:9187/metrics"
  assert_success
  assert_output --regexp "pg_|postgres_"
}

# ---------------------------------------------------------------------------
# Grafana
# ---------------------------------------------------------------------------

@test "grafana: /api/health returns ok" {
  skip_if_no_docker
  skip_if_monitoring_off
  run curl -fsS "http://localhost:3000/api/health"
  assert_success
  assert_output --regexp "ok|database"
}

# ---------------------------------------------------------------------------
# nself monitoring command
# ---------------------------------------------------------------------------

@test "nself monitoring status shows enabled services" {
  skip_if_no_docker
  skip_if_monitoring_off
  run nself monitoring status
  assert_success
  assert_output --regexp "prometheus|grafana|loki"
}
