#!/usr/bin/env bash
set -euo pipefail
# monitoring-services.sh - Generate monitoring and search service definitions
# This module handles MLflow, search engines, logging, and monitoring services
# SECURITY: All monitoring services bind to 127.0.0.1 only - access via nginx proxy

# Generate Grafana monitoring service
# SECURITY: Grafana binds to 127.0.0.1 only - access via nginx proxy
generate_grafana_service() {
  local enabled="${GRAFANA_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Grafana - Monitoring Dashboard
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  grafana:
    image: grafana/grafana:${GRAFANA_VERSION:-latest}
    container_name: \${PROJECT_NAME}_grafana
    restart: unless-stopped
    user: "472:472"
    networks:
      - ${DOCKER_NETWORK}
    environment:
      GF_SECURITY_ADMIN_USER: \${GRAFANA_ADMIN_USER}
      GF_SECURITY_ADMIN_PASSWORD: \${GRAFANA_ADMIN_PASSWORD}
      GF_INSTALL_PLUGINS: \${GRAFANA_PLUGINS:-}
      GF_SERVER_ROOT_URL: \${GRAFANA_ROOT_URL:-http://localhost:3000}
      GF_ANALYTICS_REPORTING_ENABLED: "false"
      GF_ANALYTICS_CHECK_FOR_UPDATES: "false"
    volumes:
      - grafana_data:/var/lib/grafana
      - ./monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards:ro
      - ./monitoring/grafana/datasources:/etc/grafana/provisioning/datasources:ro
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${GRAFANA_PORT:-3000}:3000"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/api/health"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF
}

# Generate Prometheus service
# SECURITY: Prometheus binds to 127.0.0.1 only - access via nginx proxy
generate_prometheus_service() {
  local enabled="${PROMETHEUS_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Prometheus - Metrics Collection
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  prometheus:
    image: prom/prometheus:${PROMETHEUS_VERSION:-latest}
    container_name: \${PROJECT_NAME}_prometheus
    restart: unless-stopped
    user: "65534:65534"
    networks:
      - ${DOCKER_NETWORK}
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/usr/share/prometheus/console_libraries'
      - '--web.console.templates=/usr/share/prometheus/consoles'
      - '--web.enable-lifecycle'
    volumes:
      - prometheus_data:/prometheus
      - ./monitoring/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${PROMETHEUS_PORT:-9090}:9090"
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:9090/-/healthy"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF
}

# Generate Loki logging service
# SECURITY: Loki binds to 127.0.0.1 only - access via nginx proxy
generate_loki_service() {
  local enabled="${LOKI_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Loki - Log Aggregation
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  loki:
    image: grafana/loki:${LOKI_VERSION:-2.9.0}
    container_name: \${PROJECT_NAME}_loki
    restart: unless-stopped
    user: "10001:10001"
    networks:
      - ${DOCKER_NETWORK}
    command: -config.file=/etc/loki/local-config.yaml
    volumes:
      - loki_data:/loki
      - ./monitoring/loki/local-config.yaml:/etc/loki/local-config.yaml:ro
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${LOKI_PORT:-3100}:3100"
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3100/ready"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF
}

# Generate Promtail log collector
generate_promtail_service() {
  local enabled="${PROMTAIL_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Promtail - Log Collector for Loki
  promtail:
    image: grafana/promtail:${PROMTAIL_VERSION:-2.9.0}
    container_name: \${PROJECT_NAME}_promtail
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      - loki
    command: -config.file=/etc/promtail/config.yml
    volumes:
      - /var/log:/var/log:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - ./monitoring/promtail/config.yml:/etc/promtail/config.yml:ro
    healthcheck:
      test: ["CMD-SHELL", "kill -0 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
EOF
}

# Main function to generate all monitoring services
generate_monitoring_services() {
  [[ "${MONITORING_ENABLED:-false}" != "true" ]] && return 0

  # Generate core monitoring services in display order (4 of 10)
  generate_prometheus_service
  generate_grafana_service
  generate_loki_service
  generate_promtail_service
  # Note: Tempo, Alertmanager, and exporters are in monitoring-exporters.sh
}

# Export functions
export -f generate_grafana_service
export -f generate_prometheus_service
export -f generate_loki_service
export -f generate_promtail_service
export -f generate_monitoring_services