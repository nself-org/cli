#!/usr/bin/env bash

# profiles.sh - Monitoring profile management

# Get monitoring profile based on environment
get_monitoring_profile() {

set -euo pipefail

  local profile="${MONITORING_PROFILE:-auto}"

  if [[ "$profile" == "auto" ]]; then
    # Smart defaults based on ENV
    case "${ENV:-dev}" in
      dev)
        profile="minimal"
        ;;
      staging)
        profile="standard"
        ;;
      prod | production)
        profile="full"
        ;;
      *)
        profile="minimal"
        ;;
    esac
  fi

  echo "$profile"
}

# Apply monitoring profile settings
apply_monitoring_profile() {
  local profile="$1"

  case "$profile" in
    minimal)
      # Minimal: Just metrics (~1GB RAM)
      export MONITORING_METRICS=true
      # Only set profile defaults if individual service flags aren't already set
      export MONITORING_LOGS="${LOKI_ENABLED:-false}"
      export MONITORING_TRACING="${TEMPO_ENABLED:-false}"
      export MONITORING_ALERTS="${ALERTMANAGER_ENABLED:-false}"
      export MONITORING_EXPORTERS=false

      # Resource limits
      export PROMETHEUS_MEMORY_LIMIT="${PROMETHEUS_MEMORY_LIMIT:-512Mi}"
      export GRAFANA_MEMORY_LIMIT="${GRAFANA_MEMORY_LIMIT:-256Mi}"
      export MONITORING_CPU_LIMIT="${MONITORING_CPU_LIMIT:-500m}"
      export MONITORING_MEMORY_LIMIT="${MONITORING_MEMORY_LIMIT:-1Gi}"

      # Retention
      export PROMETHEUS_RETENTION="${PROMETHEUS_RETENTION:-7d}"
      ;;

    standard)
      # Standard: Metrics + Logs (~2GB RAM)
      export MONITORING_METRICS=true
      # Only set profile defaults if individual service flags aren't already set
      export MONITORING_LOGS="${LOKI_ENABLED:-true}"
      export MONITORING_TRACING="${TEMPO_ENABLED:-false}"
      export MONITORING_ALERTS="${ALERTMANAGER_ENABLED:-false}"
      export MONITORING_EXPORTERS=true

      # Resource limits
      export PROMETHEUS_MEMORY_LIMIT="${PROMETHEUS_MEMORY_LIMIT:-768Mi}"
      export GRAFANA_MEMORY_LIMIT="${GRAFANA_MEMORY_LIMIT:-384Mi}"
      export LOKI_MEMORY_LIMIT="${LOKI_MEMORY_LIMIT:-384Mi}"
      export MONITORING_CPU_LIMIT="${MONITORING_CPU_LIMIT:-1000m}"
      export MONITORING_MEMORY_LIMIT="${MONITORING_MEMORY_LIMIT:-2Gi}"

      # Retention
      export PROMETHEUS_RETENTION="${PROMETHEUS_RETENTION:-15d}"
      export LOKI_RETENTION="${LOKI_RETENTION:-7d}"
      ;;

    full)
      # Full: Complete observability (~3-4GB RAM)
      export MONITORING_METRICS=true
      # Only set profile defaults if individual service flags aren't already set
      export MONITORING_LOGS="${LOKI_ENABLED:-true}"
      export MONITORING_TRACING="${TEMPO_ENABLED:-true}"
      export MONITORING_ALERTS="${ALERTMANAGER_ENABLED:-true}"
      export MONITORING_EXPORTERS=true

      # Resource limits
      export PROMETHEUS_MEMORY_LIMIT="${PROMETHEUS_MEMORY_LIMIT:-1Gi}"
      export GRAFANA_MEMORY_LIMIT="${GRAFANA_MEMORY_LIMIT:-512Mi}"
      export LOKI_MEMORY_LIMIT="${LOKI_MEMORY_LIMIT:-512Mi}"
      export TEMPO_MEMORY_LIMIT="${TEMPO_MEMORY_LIMIT:-512Mi}"
      export MONITORING_CPU_LIMIT="${MONITORING_CPU_LIMIT:-2000m}"
      export MONITORING_MEMORY_LIMIT="${MONITORING_MEMORY_LIMIT:-4Gi}"

      # Retention
      export PROMETHEUS_RETENTION="${PROMETHEUS_RETENTION:-30d}"
      export LOKI_RETENTION="${LOKI_RETENTION:-14d}"
      export TEMPO_RETENTION="${TEMPO_RETENTION:-72h}"
      ;;

    custom)
      # Custom: Use individual settings from .env
      # Settings already loaded from environment
      ;;

    *)
      log_error "Unknown monitoring profile: $profile"
      return 1
      ;;
  esac

  # Auto-enable exporters based on services
  if [[ "${MONITORING_EXPORTERS}" == "true" ]] || [[ "${MONITORING_EXPORTERS}" == "auto" ]]; then
    # PostgreSQL exporter
    if [[ "${POSTGRES_EXPORTER_ENABLED:-auto}" == "auto" ]]; then
      export POSTGRES_EXPORTER_ENABLED=true
    fi

    # Redis exporter
    if [[ "${REDIS_EXPORTER_ENABLED:-auto}" == "auto" ]] && [[ "${REDIS_ENABLED:-}" == "true" ]]; then
      export REDIS_EXPORTER_ENABLED=true
    fi

    # Nginx exporter
    if [[ "${NGINX_EXPORTER_ENABLED:-auto}" == "auto" ]]; then
      export NGINX_EXPORTER_ENABLED=true
    fi

    # Node exporter (host metrics)
    if [[ "$profile" != "minimal" ]]; then
      export NODE_EXPORTER_ENABLED="${NODE_EXPORTER_ENABLED:-true}"
    fi
  fi
}

# Get profile description
get_profile_description() {
  local profile="$1"

  case "$profile" in
    minimal)
      echo "Minimal monitoring (Prometheus + Grafana + cAdvisor)"
      echo "  • Metrics collection and visualization"
      echo "  • Container metrics only"
      echo "  • ~1GB RAM usage"
      echo "  • Best for: Development environments"
      ;;
    standard)
      echo "Standard monitoring (Metrics + Logs)"
      echo "  • Metrics and log aggregation"
      echo "  • Container and host metrics"
      echo "  • Service-specific exporters"
      echo "  • ~2GB RAM usage"
      echo "  • Best for: Staging environments"
      ;;
    full)
      echo "Full observability (Metrics + Logs + Traces + Alerts)"
      echo "  • Complete monitoring stack"
      echo "  • Distributed tracing"
      echo "  • Advanced alerting"
      echo "  • All exporters enabled"
      echo "  • ~3-4GB RAM usage"
      echo "  • Best for: Production environments"
      ;;
    custom)
      echo "Custom configuration"
      echo "  • Components based on individual settings"
      echo "  • Metrics: ${MONITORING_METRICS:-false}"
      echo "  • Logs: ${MONITORING_LOGS:-false}"
      echo "  • Tracing: ${MONITORING_TRACING:-false}"
      echo "  • Alerts: ${MONITORING_ALERTS:-false}"
      echo "  • Exporters: ${MONITORING_EXPORTERS:-false}"
      ;;
  esac
}

# Validate monitoring configuration
validate_monitoring_config() {
  local profile="$(get_monitoring_profile)"

  # Check for required settings
  if [[ "${MONITORING_ENABLED}" != "true" ]]; then
    return 0 # Monitoring disabled, nothing to validate
  fi

  # Validate Grafana password
  if [[ -z "${GRAFANA_ADMIN_PASSWORD}" ]] || [[ "${GRAFANA_ADMIN_PASSWORD}" == "admin-password-change-me" ]]; then
    log_warning "GRAFANA_ADMIN_PASSWORD is not set or using default - please set a secure password"
  fi

  # Validate resource limits
  if [[ -n "${MONITORING_MEMORY_LIMIT}" ]]; then
    # Check if format is valid (e.g., 1Gi, 512Mi)
    if ! echo "${MONITORING_MEMORY_LIMIT}" | grep -qE '^[0-9]+[MG]i?$'; then
      log_error "Invalid MONITORING_MEMORY_LIMIT format: ${MONITORING_MEMORY_LIMIT}"
      return 1
    fi
  fi

  # Validate retention periods
  for var in PROMETHEUS_RETENTION LOKI_RETENTION TEMPO_RETENTION; do
    if [[ -n "${!var}" ]]; then
      if ! echo "${!var}" | grep -qE '^[0-9]+[smhdwy]$'; then
        log_warning "Invalid $var format: ${!var} (should be like 7d, 24h, etc)"
      fi
    fi
  done

  return 0
}

# Export functions
export -f get_monitoring_profile
export -f apply_monitoring_profile
export -f get_profile_description
export -f validate_monitoring_config
