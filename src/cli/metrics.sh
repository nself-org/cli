#!/usr/bin/env bash
# metrics.sh - Complete monitoring stack management

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source utilities
source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/utils/env.sh"
source "$SCRIPT_DIR/../lib/utils/platform-compat.sh"
source "$SCRIPT_DIR/../lib/config/defaults.sh"
source "$SCRIPT_DIR/../lib/monitoring/profiles.sh"

# Load environment configuration
if [[ -f .env.local ]]; then
  set -a
  load_env_with_priority
  set +a
elif [[ -f .env ]]; then
  set -a
  load_env_with_priority
  set +a
fi

# Command function
cmd_metrics() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    enable)
      enable_monitoring "$@"
      ;;
    disable)
      disable_monitoring "$@"
      ;;
    status)
      show_monitoring_status "$@"
      ;;
    profile)
      manage_monitoring_profile "$@"
      ;;
    config)
      configure_monitoring "$@"
      ;;
    dashboard | dashboards)
      open_dashboards "$@"
      ;;
    --help | -h | help)
      show_metrics_help
      ;;
    *)
      log_error "Unknown action: $action"
      show_metrics_help
      return 1
      ;;
  esac
}

# Show help
show_metrics_help() {
  echo "nself metrics - Complete monitoring stack management"
  echo ""
  echo "Usage: nself metrics [action] [options]"
  echo ""
  echo "Actions:"
  echo "  enable [profile]    Enable monitoring with optional profile"
  echo "  disable             Disable monitoring stack"
  echo "  status              Show monitoring status and components"
  echo "  profile [name]      View or change monitoring profile"
  echo "  config              Configure monitoring settings"
  echo "  dashboard           Open Grafana dashboard"
  echo ""
  echo "Profiles:"
  echo "  minimal   Metrics only (Prometheus + Grafana, ~1GB RAM)"
  echo "  standard  Metrics + Logs (adds Loki, ~2GB RAM)"
  echo "  full      Complete observability (adds Tempo + Alertmanager, ~3-4GB RAM)"
  echo "  custom    Use individual component settings from .env"
  echo "  auto      Smart defaults based on ENV (dev→minimal, staging→standard, prod→full)"
  echo ""
  echo "Options:"
  echo "  --profile <name>    Specify monitoring profile"
  echo "  --force             Force enable/disable without prompts"
  echo "  --no-restart        Don't restart services after changes"
  echo ""
  echo "Examples:"
  echo "  nself metrics enable              # Enable with smart defaults"
  echo "  nself metrics enable full         # Enable full monitoring"
  echo "  nself metrics status              # Show current monitoring setup"
  echo "  nself metrics profile standard    # Switch to standard profile"
  echo "  nself metrics dashboard           # Open Grafana"
  echo ""
  echo "Environment Variables:"
  echo "  MONITORING_ENABLED     Enable/disable monitoring (true/false)"
  echo "  MONITORING_PROFILE     Set profile (minimal/standard/full/custom/auto)"
  echo "  ENV                    Environment (dev/staging/prod) - affects auto profile"
  echo ""
  echo "Note: Monitoring is disabled by default for simplicity."
  echo "      Enable it explicitly when you need observability."
}

# Enable monitoring
enable_monitoring() {
  local profile=""
  local force=false
  local no_restart=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --profile)
        profile="$2"
        shift 2
        ;;
      --force)
        force=true
        shift
        ;;
      --no-restart)
        no_restart=true
        shift
        ;;
      -*)
        # Unknown option
        shift
        ;;
      *)
        if [[ -z "$profile" ]]; then
          profile="$1"
        fi
        shift
        ;;
    esac
  done

  # Set profile if specified
  if [[ -n "$profile" ]]; then
    export MONITORING_PROFILE="$profile"
  fi

  # Get effective profile
  local effective_profile="$(get_monitoring_profile)"

  log_info "Enabling monitoring with profile: $effective_profile"
  echo ""
  get_profile_description "$effective_profile"
  echo ""

  # Confirm unless forced
  if [[ "$force" != "true" ]]; then
    echo -n "Continue? [Y/n] "
    read -r response
    if [[ "$response" =~ ^[Nn] ]]; then
      log_info "Cancelled"
      return 0
    fi
  fi

  # Update .env.local
  update_env_file "MONITORING_ENABLED" "true"
  if [[ -n "$profile" ]]; then
    update_env_file "MONITORING_PROFILE" "$profile"
  fi

  # Ensure secure Grafana password is set
  if [[ -z "${GRAFANA_ADMIN_PASSWORD:-}" ]] || [[ "${GRAFANA_ADMIN_PASSWORD}" == "admin-password-change-me" ]]; then
    log_info "Generating secure Grafana admin password..."
    local grafana_password
    if command -v openssl >/dev/null 2>&1; then
      grafana_password=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
    elif [[ -r /dev/urandom ]]; then
      grafana_password=$(tr -dc 'A-Za-z0-9!@#$%^&*' < /dev/urandom | head -c 32)
    else
      # Last resort: use $RANDOM (weak but better than hardcoded)
      grafana_password=""
      local chars="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*"
      for ((i=0; i<32; i++)); do
        grafana_password="${grafana_password}${chars:RANDOM%${#chars}:1}"
      done
    fi
    update_env_file "GRAFANA_ADMIN_PASSWORD" "$grafana_password"
    export GRAFANA_ADMIN_PASSWORD="$grafana_password"
    log_success "Generated secure Grafana password (32 chars)"
  fi

  # Apply profile settings
  apply_monitoring_profile "$effective_profile"

  # Create monitoring configurations
  create_monitoring_configs

  # Generate docker-compose overrides
  generate_monitoring_compose

  # Restart services unless disabled
  if [[ "$no_restart" != "true" ]]; then
    log_info "Restarting services with monitoring enabled..."
    "$SCRIPT_DIR/build.sh"
    "$SCRIPT_DIR/up.sh"
  fi

  log_success "Monitoring enabled successfully!"
  echo ""
  echo "Access your monitoring stack:"
  echo "  • Grafana:    https://grafana.${BASE_DOMAIN:-local.nself.org}"

  if [[ "${MONITORING_METRICS:-}" == "true" ]]; then
    echo "  • Prometheus: https://prometheus.${BASE_DOMAIN:-local.nself.org}"
  fi
  if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
    echo "  • Loki:       https://loki.${BASE_DOMAIN:-local.nself.org}"
  fi
  if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
    echo "  • Tempo:      https://tempo.${BASE_DOMAIN:-local.nself.org}"
  fi
  if [[ "${MONITORING_ALERTS:-}" == "true" ]]; then
    echo "  • Alerts:     https://alerts.${BASE_DOMAIN:-local.nself.org}"
  fi

  echo ""
  echo "Grafana credentials:"
  echo "  Username: ${GRAFANA_ADMIN_USER:-admin}"
  if [[ -n "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
    echo "  Password: ${GRAFANA_ADMIN_PASSWORD}"
    echo ""
    log_info "Store these credentials securely - password will not be shown again"
  else
    log_error "GRAFANA_ADMIN_PASSWORD not set - this should not happen"
  fi
}

# Disable monitoring
disable_monitoring() {
  local force=false
  local no_restart=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force)
        force=true
        shift
        ;;
      --no-restart)
        no_restart=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  log_info "Disabling monitoring stack..."

  # Confirm unless forced
  if [[ "$force" != "true" ]]; then
    echo -n "This will stop all monitoring services. Continue? [Y/n] "
    read -r response
    if [[ "$response" =~ ^[Nn] ]]; then
      log_info "Cancelled"
      return 0
    fi
  fi

  # Update .env.local
  update_env_file "MONITORING_ENABLED" "false"

  # Stop monitoring containers
  if [[ "$no_restart" != "true" ]]; then
    log_info "Stopping monitoring services..."
    docker-compose -f docker-compose.yml -f docker-compose.monitoring.yml down || true
  fi

  log_success "Monitoring disabled"
}

# Show monitoring status
show_monitoring_status() {
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║                    MONITORING STATUS                         ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo ""

  # Check if enabled
  if [[ "${MONITORING_ENABLED:-false}" != "true" ]]; then
    echo "Status: $(color_text "DISABLED" "yellow")"
    echo ""
    echo "To enable monitoring, run:"
    echo "  nself metrics enable"
    return 0
  fi

  echo "Status: $(color_text "ENABLED" "green")"

  # Get current profile
  local profile="$(get_monitoring_profile)"
  echo "Profile: $(color_text "$profile" "cyan")"
  echo ""

  # Show profile details
  get_profile_description "$profile"
  echo ""

  # Check running services
  echo "Service Status:"

  # Always check core services
  check_service_status "prometheus" "Prometheus"
  check_service_status "grafana" "Grafana"
  check_service_status "cadvisor" "cAdvisor"

  # Check profile-specific services
  if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
    check_service_status "loki" "Loki"
    check_service_status "promtail" "Promtail"
  fi

  if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
    check_service_status "tempo" "Tempo"
  fi

  if [[ "${MONITORING_ALERTS:-}" == "true" ]]; then
    check_service_status "alertmanager" "Alertmanager"
  fi

  # Check exporters
  if [[ "${MONITORING_EXPORTERS:-}" == "true" ]]; then
    echo ""
    echo "Exporters:"
    [[ "${NODE_EXPORTER_ENABLED:-}" == "true" ]] && check_service_status "node-exporter" "Node Exporter"
    [[ "${POSTGRES_EXPORTER_ENABLED:-}" == "true" ]] && check_service_status "postgres-exporter" "PostgreSQL Exporter"
    [[ "${REDIS_EXPORTER_ENABLED:-}" == "true" ]] && check_service_status "redis-exporter" "Redis Exporter"
    [[ "${BLACKBOX_EXPORTER_ENABLED:-}" == "true" ]] && check_service_status "blackbox-exporter" "Blackbox Exporter"
  fi

  echo ""
  echo "Resource Usage:"
  show_monitoring_resources

  echo ""
  echo "Access Points:"
  echo "  • Grafana:    https://grafana.${BASE_DOMAIN:-local.nself.org}"
  [[ "${PROMETHEUS_WEB_ENABLE:-true}" == "true" ]] && echo "  • Prometheus: https://prometheus.${BASE_DOMAIN:-local.nself.org}"
}

# Check service status
check_service_status() {
  local service="$1"
  local display_name="$2"
  local container_name="${PROJECT_NAME:-nself}_${service}"

  if docker ps --format "table {{.Names}}" | grep -q "^${container_name}$"; then
    echo "  • $display_name: $(color_text "Running" "green")"
  else
    echo "  • $display_name: $(color_text "Stopped" "red")"
  fi
}

# Show monitoring resource usage
show_monitoring_resources() {
  local total_cpu=0
  local total_memory=0

  # Get stats for monitoring containers
  local containers=(prometheus grafana loki tempo alertmanager cadvisor node-exporter promtail)

  for container in "${containers[@]}"; do
    local container_name="${PROJECT_NAME:-nself}_${container}"
    if docker ps --format "{{.Names}}" | grep -q "^${container_name}$"; then
      local stats=$(docker stats --no-stream --format "{{.CPUPerc}} {{.MemUsage}}" "$container_name" 2>/dev/null || echo "0% 0MiB")
      if [[ -n "$stats" ]]; then
        local cpu=$(echo "$stats" | awk '{print $1}' | sed 's/%//')
        local mem=$(echo "$stats" | awk '{print $2}' | sed 's/MiB.*//')

        # Add to totals (simplified calculation)
        total_cpu=$(echo "$total_cpu + $cpu" | bc 2>/dev/null || echo "$total_cpu")
        total_memory=$(echo "$total_memory + $mem" | bc 2>/dev/null || echo "$total_memory")
      fi
    fi
  done

  echo "  • Total CPU:    ~${total_cpu}%"
  echo "  • Total Memory: ~${total_memory}MiB"
  echo "  • Disk Usage:   $(du -sh .nself/monitoring 2>/dev/null | awk '{print $1}' || echo "N/A")"
}

# Manage monitoring profile
manage_monitoring_profile() {
  local new_profile="${1:-}"

  if [[ -z "$new_profile" ]]; then
    # Show current profile
    local current_profile="$(get_monitoring_profile)"
    echo "Current monitoring profile: $(color_text "$current_profile" "cyan")"
    echo ""
    get_profile_description "$current_profile"
    echo ""
    echo "Available profiles:"
    echo "  • minimal  - Metrics only (~1GB RAM)"
    echo "  • standard - Metrics + Logs (~2GB RAM)"
    echo "  • full     - Complete observability (~3-4GB RAM)"
    echo "  • custom   - Use individual settings"
    echo "  • auto     - Smart defaults based on ENV"
    return 0
  fi

  # Validate profile
  case "$new_profile" in
    minimal | standard | full | custom | auto) ;;
    *)
      log_error "Invalid profile: $new_profile"
      echo "Valid profiles: minimal, standard, full, custom, auto"
      return 1
      ;;
  esac

  log_info "Switching to profile: $new_profile"

  # Update configuration
  update_env_file "MONITORING_PROFILE" "$new_profile"

  # Apply profile
  apply_monitoring_profile "$new_profile"

  # Regenerate configs
  create_monitoring_configs
  generate_monitoring_compose

  log_success "Profile updated to: $new_profile"
  echo ""
  echo "Run 'nself build && nself up' to apply changes"
}

# Configure monitoring
configure_monitoring() {
  local setting="${1:-}"
  local value="${2:-}"

  if [[ -z "$setting" ]]; then
    echo "Monitoring Configuration:"
    echo ""
    echo "Core Settings:"
    echo "  MONITORING_ENABLED:     ${MONITORING_ENABLED:-false}"
    echo "  MONITORING_PROFILE:     ${MONITORING_PROFILE:-auto}"
    echo ""
    echo "Components:"
    echo "  MONITORING_METRICS:     ${MONITORING_METRICS:-true}"
    echo "  MONITORING_LOGS:        ${MONITORING_LOGS:-false}"
    echo "  MONITORING_TRACING:     ${MONITORING_TRACING:-false}"
    echo "  MONITORING_ALERTS:      ${MONITORING_ALERTS:-false}"
    echo "  MONITORING_EXPORTERS:   ${MONITORING_EXPORTERS:-false}"
    echo ""
    echo "Grafana:"
    echo "  GRAFANA_ADMIN_USER:     ${GRAFANA_ADMIN_USER:-admin}"
    echo "  GRAFANA_ROUTE:          ${GRAFANA_ROUTE:-grafana.\$BASE_DOMAIN}"
    echo ""
    echo "Resource Limits:"
    echo "  MONITORING_CPU_LIMIT:    ${MONITORING_CPU_LIMIT:-2000m}"
    echo "  MONITORING_MEMORY_LIMIT: ${MONITORING_MEMORY_LIMIT:-4Gi}"
    echo ""
    echo "To change a setting:"
    echo "  nself metrics config <setting> <value>"
    return 0
  fi

  # Update setting
  update_env_file "$setting" "$value"
  log_success "Updated $setting = $value"
  echo "Run 'nself build && nself up' to apply changes"
}

# Open dashboards
open_dashboards() {
  local dashboard="${1:-grafana}"

  case "$dashboard" in
    grafana)
      local url="https://grafana.${BASE_DOMAIN:-local.nself.org}"
      ;;
    prometheus)
      local url="https://prometheus.${BASE_DOMAIN:-local.nself.org}"
      ;;
    loki)
      local url="https://loki.${BASE_DOMAIN:-local.nself.org}"
      ;;
    alerts | alertmanager)
      local url="https://alerts.${BASE_DOMAIN:-local.nself.org}"
      ;;
    *)
      log_error "Unknown dashboard: $dashboard"
      return 1
      ;;
  esac

  log_info "Opening $dashboard dashboard..."
  echo "URL: $url"

  # Try to open in browser
  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    echo "Please open manually: $url"
  fi
}

# Create monitoring configuration files
create_monitoring_configs() {
  local config_dir=".nself/monitoring"
  mkdir -p "$config_dir"

  # Create Prometheus config
  create_prometheus_config "$config_dir/prometheus.yml"

  # Create Grafana provisioning
  create_grafana_provisioning "$config_dir"

  # Create Loki config if enabled
  if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
    create_loki_config "$config_dir/loki.yml"
    create_promtail_config "$config_dir/promtail.yml"
  fi

  # Create Alertmanager config if enabled
  if [[ "${MONITORING_ALERTS:-}" == "true" ]]; then
    create_alertmanager_config "$config_dir/alertmanager.yml"
    create_alert_rules "$config_dir/alerts.yml"
  fi

  # Create Tempo config if enabled
  if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
    create_tempo_config "$config_dir/tempo.yml"
  fi
}

# Create Prometheus configuration
create_prometheus_config() {
  local config_file="$1"

  cat >"$config_file" <<EOF
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL:-15s}
  evaluation_interval: 15s
  external_labels:
    monitor: 'nself-monitor'
    environment: '${ENV:-dev}'

# Alert manager configuration
EOF

  if [[ "${MONITORING_ALERTS:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF
alerting:
  alertmanagers:
    - static_configs:
        - targets:
            - alertmanager:${ALERTMANAGER_PORT:-9093}

# Load alert rules
rule_files:
  - /etc/prometheus/alerts.yml

EOF
  fi

  cat >>"$config_file" <<EOF
# Scrape configurations
scrape_configs:
  # Prometheus itself
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  # Grafana metrics
  - job_name: 'grafana'
    static_configs:
      - targets: ['grafana:${GRAFANA_PORT:-3000}']

  # cAdvisor for container metrics
  - job_name: 'cadvisor'
    static_configs:
      - targets: ['cadvisor:8080']
    metric_relabel_configs:
      - source_labels: [name]
        regex: '${PROJECT_NAME:-nself}_(.*)'
        target_label: container_name
        replacement: '\${1}'
EOF

  # Add Node Exporter if enabled
  if [[ "${NODE_EXPORTER_ENABLED:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF

  # Node Exporter for host metrics
  - job_name: 'node'
    static_configs:
      - targets: ['node-exporter:9100']
EOF
  fi

  # Add PostgreSQL Exporter if enabled
  if [[ "${POSTGRES_EXPORTER_ENABLED:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF

  # PostgreSQL metrics
  - job_name: 'postgres'
    static_configs:
      - targets: ['postgres-exporter:9187']
EOF
  fi

  # Add Redis Exporter if enabled
  if [[ "${REDIS_EXPORTER_ENABLED:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF

  # Redis metrics
  - job_name: 'redis'
    static_configs:
      - targets: ['redis-exporter:9121']
EOF
  fi

  # Add Blackbox Exporter if enabled
  if [[ "${BLACKBOX_EXPORTER_ENABLED:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF

  # Blackbox Exporter for endpoint monitoring
  - job_name: 'blackbox'
    metrics_path: /probe
    params:
      module: [http_2xx]
    static_configs:
      - targets:
          - https://api.${BASE_DOMAIN:-local.nself.org}
          - https://auth.${BASE_DOMAIN:-local.nself.org}
          - https://storage.${BASE_DOMAIN:-local.nself.org}
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: blackbox-exporter:9115
EOF
  fi

  # Add Docker metrics
  cat >>"$config_file" <<EOF

  # Docker daemon metrics
  - job_name: 'docker'
    static_configs:
      - targets: ['host.docker.internal:9323']
EOF

  # Add Loki if enabled
  if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF

  # Loki metrics
  - job_name: 'loki'
    static_configs:
      - targets: ['loki:${LOKI_PORT:-3100}']
EOF
  fi

  # Add Tempo if enabled
  if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
    cat >>"$config_file" <<EOF

  # Tempo metrics
  - job_name: 'tempo'
    static_configs:
      - targets: ['tempo:${TEMPO_PORT:-3200}']
EOF
  fi
}

# Create Grafana provisioning
create_grafana_provisioning() {
  local config_dir="$1"

  # Create provisioning directories
  mkdir -p "$config_dir/grafana/provisioning/datasources"
  mkdir -p "$config_dir/grafana/provisioning/dashboards"
  mkdir -p "$config_dir/grafana/dashboards"

  # Create datasources config
  cat >"$config_dir/grafana/provisioning/datasources/datasources.yml" <<EOF
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:${PROMETHEUS_PORT:-9090}
    isDefault: true
    editable: true
EOF

  # Add Loki datasource if enabled
  if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
    cat >>"$config_dir/grafana/provisioning/datasources/datasources.yml" <<EOF

  - name: Loki
    type: loki
    access: proxy
    url: http://loki:${LOKI_PORT:-3100}
    editable: true
EOF
  fi

  # Add Tempo datasource if enabled
  if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
    cat >>"$config_dir/grafana/provisioning/datasources/datasources.yml" <<EOF

  - name: Tempo
    type: tempo
    access: proxy
    url: http://tempo:${TEMPO_PORT:-3200}
    editable: true
EOF
  fi

  # Create dashboard provisioning config
  cat >"$config_dir/grafana/provisioning/dashboards/dashboards.yml" <<EOF
apiVersion: 1

providers:
  - name: 'nself'
    orgId: 1
    folder: 'nself'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /etc/grafana/provisioning/dashboards
EOF

  # Create default dashboards
  create_grafana_dashboards "$config_dir/grafana/dashboards"
}

# Create Grafana dashboards
create_grafana_dashboards() {
  local dashboard_dir="$1"

  # Create Container Overview Dashboard
  cat >"$dashboard_dir/container-overview.json" <<'EOF'
{
  "dashboard": {
    "title": "nself Container Overview",
    "panels": [
      {
        "title": "Container CPU Usage",
        "targets": [
          {
            "expr": "rate(container_cpu_usage_seconds_total{container_label_com_docker_compose_project=\"${PROJECT_NAME}\"}[5m]) * 100"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 0}
      },
      {
        "title": "Container Memory Usage",
        "targets": [
          {
            "expr": "container_memory_usage_bytes{container_label_com_docker_compose_project=\"${PROJECT_NAME}\"}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 0}
      }
    ]
  }
}
EOF
}

# Create Loki configuration
create_loki_config() {
  local config_file="$1"

  cat >"$config_file" <<EOF
auth_enabled: false

server:
  http_listen_port: ${LOKI_PORT:-3100}
  grpc_listen_port: 9096

common:
  path_prefix: ${LOKI_DATA_PATH:-/tmp/loki}
  storage:
    filesystem:
      chunks_directory: ${LOKI_DATA_PATH:-/tmp/loki}/chunks
      rules_directory: ${LOKI_DATA_PATH:-/tmp/loki}/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2020-10-24
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h

ruler:
  alertmanager_url: http://alertmanager:${ALERTMANAGER_PORT:-9093}

limits_config:
  enforce_metric_name: false
  reject_old_samples: true
  reject_old_samples_max_age: ${LOKI_RETENTION:-7d}
  max_entries_limit_per_query: 5000
  ingestion_rate_mb: 10
  ingestion_burst_size_mb: 20
  max_line_size: ${LOKI_MAX_LINE_SIZE:-256kb}
EOF
}

# Create Promtail configuration
create_promtail_config() {
  local config_file="$1"

  cat >"$config_file" <<EOF
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: ${PROMTAIL_POS_FILE:-/tmp/positions.yaml}

clients:
  - url: http://loki:${LOKI_PORT:-3100}/loki/api/v1/push

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
            time: time
      - json:
          expressions:
            tag: attrs.tag
          source: stream
      - regex:
          expression: '^(?P<container_name>/${PROJECT_NAME:-nself}_[^/]+)'
          source: tag
      - labels:
          container_name:
      - output:
          source: output
EOF
}

# Create Alertmanager configuration
create_alertmanager_config() {
  local config_file="$1"

  cat >"$config_file" <<EOF
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
EOF

  # Add webhook if configured
  if [[ -n "${ALERTMANAGER_WEBHOOK_URL:-}" ]]; then
    cat >>"$config_file" <<EOF
    webhook_configs:
      - url: '${ALERTMANAGER_WEBHOOK_URL}'
        send_resolved: true
EOF
  fi

  # Add email if configured
  if [[ -n "${ALERTMANAGER_EMAIL_TO:-}" ]]; then
    cat >>"$config_file" <<EOF
    email_configs:
      - to: '${ALERTMANAGER_EMAIL_TO}'
        from: '${ALERTMANAGER_EMAIL_FROM:-alerts@local.nself.org}'
        smarthost: '${AUTH_SMTP_HOST:-mailpit}:${AUTH_SMTP_PORT:-1025}'
        auth_username: '${AUTH_SMTP_USER:-}'
        auth_password: '${AUTH_SMTP_PASS:-}'
        send_resolved: true
EOF
  fi

  # Add PagerDuty if configured
  if [[ -n "${ALERTMANAGER_PAGERDUTY_KEY:-}" ]]; then
    cat >>"$config_file" <<EOF
    pagerduty_configs:
      - service_key: '${ALERTMANAGER_PAGERDUTY_KEY}'
        send_resolved: true
EOF
  fi

  cat >>"$config_file" <<EOF

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'cluster', 'service']
EOF
}

# Create alert rules
create_alert_rules() {
  local config_file="$1"

  cat >"$config_file" <<EOF
groups:
  - name: nself_alerts
    interval: 30s
    rules:
EOF

  # High CPU alert
  if [[ "${ALERTS_HIGH_CPU:-true}" == "true" ]]; then
    cat >>"$config_file" <<EOF
      - alert: HighCPUUsage
        expr: rate(container_cpu_usage_seconds_total[5m]) * 100 > 80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage detected"
          description: "Container {{ \$labels.name }} CPU usage is above 80% (current: {{ \$value }}%)"

EOF
  fi

  # High Memory alert
  if [[ "${ALERTS_HIGH_MEMORY:-true}" == "true" ]]; then
    cat >>"$config_file" <<EOF
      - alert: HighMemoryUsage
        expr: (container_memory_usage_bytes / container_spec_memory_limit_bytes) * 100 > 80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage detected"
          description: "Container {{ \$labels.name }} memory usage is above 80% (current: {{ \$value }}%)"

EOF
  fi

  # Service down alert
  if [[ "${ALERTS_SERVICE_DOWN:-true}" == "true" ]]; then
    cat >>"$config_file" <<EOF
      - alert: ServiceDown
        expr: up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Service is down"
          description: "{{ \$labels.job }} service {{ \$labels.instance }} is down"

EOF
  fi

  # Disk space alert
  if [[ "${ALERTS_DISK_SPACE:-true}" == "true" ]]; then
    cat >>"$config_file" <<EOF
      - alert: DiskSpaceLow
        expr: (node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100 < 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Disk space is running low"
          description: "Less than 10% disk space remaining ({{ \$value }}% free)"

EOF
  fi

  # High error rate alert
  if [[ "${ALERTS_HIGH_ERROR_RATE:-true}" == "true" ]]; then
    cat >>"$config_file" <<EOF
      - alert: HighErrorRate
        expr: rate(nginx_http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          description: "Error rate is above 5% (current: {{ \$value }})"
EOF
  fi
}

# Create Tempo configuration
create_tempo_config() {
  local config_file="$1"

  cat >"$config_file" <<EOF
server:
  http_listen_port: ${TEMPO_PORT:-3200}

distributor:
  receivers:
    otlp:
      protocols:
        http:
        grpc:

ingester:
  trace_idle_period: 10s
  max_block_bytes: 1_000_000
  max_block_duration: 5m

compactor:
  compaction:
    compaction_window: 1h
    max_block_bytes: 100_000_000
    block_retention: ${TEMPO_RETENTION:-72h}

storage:
  trace:
    backend: local
    local:
      path: ${TEMPO_DATA_PATH:-/tmp/tempo}/blocks
    wal:
      path: ${TEMPO_DATA_PATH:-/tmp/tempo}/wal

overrides:
  defaults:
    metrics:
      enable_exemplars: true
    ingestion:
      rate_limit_bytes: 15000000
      burst_size_bytes: 20000000
      max_traces_per_user: 10000
EOF
}

# Generate monitoring Docker Compose override
generate_monitoring_compose() {
  local compose_file="docker-compose.monitoring.yml"

  cat >"$compose_file" <<EOF
version: '3.8'

services:
EOF

  # Always add core monitoring services
  if [[ "${MONITORING_ENABLED:-}" == "true" ]]; then
    # Prometheus
    cat >>"$compose_file" <<EOF
  prometheus:
    image: prom/prometheus:latest
    container_name: \${PROJECT_NAME:-nself}_prometheus
    volumes:
      - ./.nself/monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - ./.nself/monitoring/alerts.yml:/etc/prometheus/alerts.yml:ro
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=\${PROMETHEUS_RETENTION:-15d}'
      - '--web.console.libraries=/usr/share/prometheus/console_libraries'
      - '--web.console.templates=/usr/share/prometheus/consoles'
      - '--web.enable-lifecycle'
    ports:
      - "\${PROMETHEUS_PORT:-9090}:9090"
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: \${PROMETHEUS_MEMORY_LIMIT:-1Gi}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.prometheus.rule=Host(\`prometheus.\${BASE_DOMAIN:-local.nself.org}\`)"
      - "traefik.http.routers.prometheus.tls=true"

  grafana:
    image: grafana/grafana:latest
    container_name: \${PROJECT_NAME:-nself}_grafana
    environment:
      - GF_SECURITY_ADMIN_USER=\${GRAFANA_ADMIN_USER:-admin}
      - GF_SECURITY_ADMIN_PASSWORD=\${GRAFANA_ADMIN_PASSWORD}
      - GF_AUTH_ANONYMOUS_ENABLED=\${GRAFANA_ANONYMOUS_ENABLED:-false}
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
      - GF_USERS_DEFAULT_THEME=\${GRAFANA_DEFAULT_THEME:-dark}
      - GF_INSTALL_PLUGINS=grafana-piechart-panel
    volumes:
      - grafana_data:/var/lib/grafana
      - ./.nself/monitoring/grafana/provisioning:/etc/grafana/provisioning:ro
      - ./.nself/monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards:ro
    ports:
      - "\${GRAFANA_PORT:-3000}:3000"
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: \${GRAFANA_MEMORY_LIMIT:-512Mi}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.grafana.rule=Host(\`grafana.\${BASE_DOMAIN:-local.nself.org}\`)"
      - "traefik.http.routers.grafana.tls=true"

  cadvisor:
    image: gcr.io/cadvisor/cadvisor:latest
    container_name: \${PROJECT_NAME:-nself}_cadvisor
    volumes:
      - /:/rootfs:ro
      - /var/run:/var/run:ro
      - /sys:/sys:ro
      - /var/lib/docker:/var/lib/docker:ro
      - /dev/disk:/dev/disk:ro
    devices:
      - /dev/kmsg
    privileged: true
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 200Mi

EOF

    # Add Loki and Promtail if logs enabled (either by profile or individual flag)
    if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  loki:
    image: grafana/loki:latest
    container_name: \${PROJECT_NAME:-nself}_loki
    ports:
      - "\${LOKI_PORT:-3100}:3100"
    volumes:
      - ./.nself/monitoring/loki.yml:/etc/loki/local-config.yaml:ro
      - loki_data:/loki
    command: -config.file=/etc/loki/local-config.yaml
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: \${LOKI_MEMORY_LIMIT:-512Mi}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.loki.rule=Host(\`loki.\${BASE_DOMAIN:-local.nself.org}\`)"
      - "traefik.http.routers.loki.tls=true"

  promtail:
    image: grafana/promtail:latest
    container_name: \${PROJECT_NAME:-nself}_promtail
    volumes:
      - ./.nself/monitoring/promtail.yml:/etc/promtail/config.yml:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
      - /var/run/docker.sock:/var/run/docker.sock
    command: -config.file=/etc/promtail/config.yml
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 100Mi

EOF
    fi

    # Add Tempo if tracing enabled (either by profile or individual flag)
    if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  tempo:
    image: grafana/tempo:latest
    container_name: \${PROJECT_NAME:-nself}_tempo
    command: [ "-config.file=/etc/tempo.yml" ]
    volumes:
      - ./.nself/monitoring/tempo.yml:/etc/tempo.yml:ro
      - tempo_data:${TEMPO_DATA_PATH:-/tmp/tempo}
    ports:
      - "\${TEMPO_PORT:-3200}:3200"
      - "4317:4317"  # OTLP gRPC
      - "4318:4318"  # OTLP HTTP
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: \${TEMPO_MEMORY_LIMIT:-512Mi}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.tempo.rule=Host(\`tempo.\${BASE_DOMAIN:-local.nself.org}\`)"
      - "traefik.http.routers.tempo.tls=true"

EOF
    fi

    # Add Alertmanager if alerts enabled
    if [[ "${MONITORING_ALERTS:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  alertmanager:
    image: prom/alertmanager:latest
    container_name: \${PROJECT_NAME:-nself}_alertmanager
    volumes:
      - ./.nself/monitoring/alertmanager.yml:/etc/alertmanager/config.yml:ro
      - alertmanager_data:/alertmanager
    command:
      - '--config.file=/etc/alertmanager/config.yml'
      - '--storage.path=/alertmanager'
    ports:
      - "\${ALERTMANAGER_PORT:-9093}:9093"
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 100Mi
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.alertmanager.rule=Host(\`alerts.\${BASE_DOMAIN:-local.nself.org}\`)"
      - "traefik.http.routers.alertmanager.tls=true"

EOF
    fi

    # Add exporters
    if [[ "${NODE_EXPORTER_ENABLED:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  node-exporter:
    image: prom/node-exporter:latest
    container_name: \${PROJECT_NAME:-nself}_node_exporter
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /:/rootfs:ro
    command:
      - '--path.procfs=/host/proc'
      - '--path.sysfs=/host/sys'
      - '--path.rootfs=/rootfs'
      - '--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)(\$\$|/)'
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 50Mi

EOF
    fi

    if [[ "${POSTGRES_EXPORTER_ENABLED:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  postgres-exporter:
    image: prometheuscommunity/postgres-exporter:latest
    container_name: \${PROJECT_NAME:-nself}_postgres_exporter
    environment:
      DATA_SOURCE_NAME: "postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:\${POSTGRES_PORT:-5432}/\${POSTGRES_DB:-nhost}?sslmode=disable"
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 50Mi

EOF
    fi

    if [[ "${REDIS_EXPORTER_ENABLED:-}" == "true" ]] && [[ "${REDIS_ENABLED:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  redis-exporter:
    image: oliver006/redis_exporter:latest
    container_name: \${PROJECT_NAME:-nself}_redis_exporter
    environment:
      REDIS_ADDR: "redis:\${REDIS_PORT:-6379}"
      REDIS_PASSWORD: "\${REDIS_PASSWORD:-}"
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 30Mi

EOF
    fi

    if [[ "${BLACKBOX_EXPORTER_ENABLED:-}" == "true" ]]; then
      cat >>"$compose_file" <<EOF
  blackbox-exporter:
    image: prom/blackbox-exporter:latest
    container_name: \${PROJECT_NAME:-nself}_blackbox_exporter
    volumes:
      - ./.nself/monitoring/blackbox.yml:/etc/blackbox_exporter/config.yml:ro
    networks:
      - \${DOCKER_NETWORK:-nself_network}
    restart: unless-stopped
    mem_limit: 50Mi

EOF
    fi
  fi

  # Add volumes
  cat >>"$compose_file" <<EOF

volumes:
  prometheus_data:
  grafana_data:
EOF

  if [[ "${MONITORING_LOGS:-}" == "true" || "${LOKI_ENABLED:-}" == "true" ]]; then
    cat >>"$compose_file" <<EOF
  loki_data:
EOF
  fi

  if [[ "${MONITORING_TRACING:-}" == "true" || "${TEMPO_ENABLED:-}" == "true" ]]; then
    cat >>"$compose_file" <<EOF
  tempo_data:
EOF
  fi

  if [[ "${MONITORING_ALERTS:-}" == "true" ]]; then
    cat >>"$compose_file" <<EOF
  alertmanager_data:
EOF
  fi

  cat >>"$compose_file" <<EOF

networks:
  \${DOCKER_NETWORK:-nself_network}:
    external: true
EOF

  log_success "Generated docker-compose.monitoring.yml"
}

# Update env file helper
update_env_file() {
  local key="$1"
  local value="$2"
  local env_file=".env.local"

  # Create .env.local if it doesn't exist
  if [[ ! -f "$env_file" ]]; then
    touch "$env_file"
  fi

  # Update or add the setting
  if grep -q "^${key}=" "$env_file"; then
    # Update existing
    safe_sed_inline "$env_file" "s/^${key}=.*/${key}=${value}/"
    rm -f "${env_file}.bak"
  else
    # Add new
    echo "${key}=${value}" >>"$env_file"
  fi
}

# Color text helper (cross-platform compatible)
color_text() {
  local text="$1"
  local color="$2"

  case "$color" in
    red)
      printf "\033[0;31m%s\033[0m" "$text"
      ;;
    green)
      printf "\033[0;32m%s\033[0m" "$text"
      ;;
    yellow)
      printf "\033[0;33m%s\033[0m" "$text"
      ;;
    blue)
      printf "\033[0;34m%s\033[0m" "$text"
      ;;
    cyan)
      printf "\033[0;36m%s\033[0m" "$text"
      ;;
    *)
      printf "%s" "$text"
      ;;
  esac
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_metrics "$@"
fi
