#!/usr/bin/env bash

# monitoring-setup.sh - Set up monitoring configuration files during build
# Bash 3.2 compatible, cross-platform

# Setup monitoring configuration files
setup_monitoring_configs() {

set -euo pipefail

  local monitoring_enabled="${MONITORING_ENABLED:-false}"
  local grafana_enabled="${GRAFANA_ENABLED:-$monitoring_enabled}"
  local loki_enabled="${LOKI_ENABLED:-$monitoring_enabled}"
  local prometheus_enabled="${PROMETHEUS_ENABLED:-$monitoring_enabled}"

  # Skip if no monitoring services are enabled
  if [[ "$monitoring_enabled" != "true" ]] && [[ "$grafana_enabled" != "true" ]] && [[ "$loki_enabled" != "true" ]] && [[ "$prometheus_enabled" != "true" ]]; then
    return 0
  fi

  echo "Setting up monitoring configurations..."

  # Create Loki config if enabled
  if [[ "$loki_enabled" == "true" ]] || [[ "$monitoring_enabled" == "true" ]]; then
    if [[ ! -f "monitoring/loki/local-config.yaml" ]]; then
      mkdir -p monitoring/loki
      cat >monitoring/loki/local-config.yaml <<'EOF'
auth_enabled: false

server:
  http_listen_port: 3100
  grpc_listen_port: 9096

common:
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

ruler:
  alertmanager_url: http://alertmanager:9093

limits_config:
  allow_structured_metadata: false
EOF
    fi
  fi

  # Create Prometheus config if enabled
  if [[ "$prometheus_enabled" == "true" ]] || [[ "$monitoring_enabled" == "true" ]]; then
    if [[ ! -f "monitoring/prometheus/prometheus.yml" ]]; then
      mkdir -p monitoring/prometheus
      cat >monitoring/prometheus/prometheus.yml <<'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['prometheus:9090']

  - job_name: 'node'
    static_configs:
      - targets: ['node-exporter:9100']

  - job_name: 'cadvisor'
    static_configs:
      - targets: ['cadvisor:8080']

  - job_name: 'postgres'
    static_configs:
      - targets: ['postgres-exporter:9187']

  - job_name: 'redis'
    static_configs:
      - targets: ['redis-exporter:9121']

  - job_name: 'hasura'
    static_configs:
      - targets: ['hasura:8080']
EOF

      # Add custom services monitoring
      for i in {1..20}; do
        local cs_var="CS_${i}"
        local cs_value="${!cs_var:-}"

        [[ -z "$cs_value" ]] && continue

        # Parse CS format: service_name:template_type:port
        IFS=':' read -r service_name template_type port <<<"$cs_value"

        # Skip if no port or port is 0
        [[ -z "$port" || "$port" == "0" ]] && continue

        cat >>monitoring/prometheus/prometheus.yml <<EOF

  - job_name: 'custom_${service_name}'
    static_configs:
      - targets: ['${service_name}:${port}']
    metrics_path: /metrics
EOF
      done
    fi
  fi

  # Create Promtail config if enabled
  if [[ "$loki_enabled" == "true" ]] || [[ "$monitoring_enabled" == "true" ]]; then
    if [[ ! -f "monitoring/promtail/config.yml" ]]; then
      mkdir -p monitoring/promtail
      cat >monitoring/promtail/config.yml <<'EOF'
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://loki:3100/loki/api/v1/push

scrape_configs:
  - job_name: containers
    static_configs:
      - targets:
          - localhost
        labels:
          job: containerlogs
          __path__: /var/lib/docker/containers/*/*log
    pipeline_stages:
      - json:
          expressions:
            output: log
            stream: stream
            attrs:
      - json:
          expressions:
            tag:
          source: attrs
      - regex:
          expression: (?P<container_name>(?:[^|]*))\|(?P<image_name>(?:[^|]*))
          source: tag
      - timestamp:
          format: RFC3339Nano
          source: time
      - labels:
          stream:
          container_name:
          image_name:
      - output:
          source: output
EOF
    fi
  fi

  # Create Grafana provisioning if enabled
  if [[ "$monitoring_enabled" == "true" ]]; then
    if [[ ! -d "monitoring/grafana/provisioning" ]]; then
      mkdir -p monitoring/grafana/provisioning/datasources
      mkdir -p monitoring/grafana/provisioning/dashboards

      # Create datasources config
      cat >monitoring/grafana/provisioning/datasources/prometheus.yml <<'EOF'
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
  - name: Loki
    type: loki
    access: proxy
    url: http://loki:3100
    jsonData:
      httpHeaderName1: 'X-Scope-OrgID'
    secureJsonData:
      httpHeaderValue1: '1'
EOF

      # Create dashboards config
      cat >monitoring/grafana/provisioning/dashboards/dashboards.yml <<'EOF'
apiVersion: 1

providers:
  - name: 'default'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    options:
      path: /etc/grafana/provisioning/dashboards
EOF
    fi
  fi

  # Create Tempo config if enabled
  if [[ "${TEMPO_ENABLED:-false}" == "true" ]]; then
    if [[ ! -f "monitoring/tempo/tempo.yml" ]]; then
      mkdir -p monitoring/tempo
      cat >monitoring/tempo/tempo.yml <<'EOF'
server:
  http_listen_port: 3200

distributor:
  receivers:
    otlp:
      protocols:
        grpc:

storage:
  trace:
    backend: local
    local:
      path: /var/tempo/blocks

metrics_generator:
  registry:
    external_labels:
      source: tempo
  storage:
    path: /var/tempo/generator/wal
EOF
    fi
  fi

  # Create AlertManager config if enabled
  if [[ "${ALERTMANAGER_ENABLED:-false}" == "true" ]] || [[ "$monitoring_enabled" == "true" ]]; then
    if [[ ! -f "monitoring/alertmanager/alertmanager.yml" ]]; then
      mkdir -p monitoring/alertmanager
      cat >monitoring/alertmanager/alertmanager.yml <<'EOF'
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'default'

receivers:
  - name: 'default'
    # Configure webhook, email, or other alert receivers here

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'cluster', 'service']
EOF
    fi
  fi

  return 0
}

# Export function
export -f setup_monitoring_configs
