#!/usr/bin/env bash

# service-detection.sh - Smart service detection using ternary patterns
# Environment-agnostic service detection that works across dev/staging/prod

# Detect which services are enabled using smart defaults
detect_enabled_services() {

set -euo pipefail

  # Core services - always enabled unless explicitly disabled
  export POSTGRES_ENABLED="${POSTGRES_ENABLED:-true}"
  export HASURA_ENABLED="${HASURA_ENABLED:-true}"
  export AUTH_ENABLED="${AUTH_ENABLED:-true}"
  export NGINX_ENABLED="${NGINX_ENABLED:-true}"

  # Optional services - disabled by default unless explicitly enabled
  export NSELF_ADMIN_ENABLED="${NSELF_ADMIN_ENABLED:-false}"
  export MINIO_ENABLED="${MINIO_ENABLED:-${STORAGE_ENABLED:-false}}"
  export REDIS_ENABLED="${REDIS_ENABLED:-false}"
  export MEILISEARCH_ENABLED="${MEILISEARCH_ENABLED:-false}"
  export MAILPIT_ENABLED="${MAILPIT_ENABLED:-false}"
  export MLFLOW_ENABLED="${MLFLOW_ENABLED:-false}"
  export FUNCTIONS_ENABLED="${FUNCTIONS_ENABLED:-false}"

  # Monitoring bundle - all or nothing
  export MONITORING_ENABLED="${MONITORING_ENABLED:-false}"
  if [[ "$MONITORING_ENABLED" == "true" ]]; then
    export PROMETHEUS_ENABLED="${PROMETHEUS_ENABLED:-true}"
    export GRAFANA_ENABLED="${GRAFANA_ENABLED:-true}"
    export LOKI_ENABLED="${LOKI_ENABLED:-true}"
    export PROMTAIL_ENABLED="${PROMTAIL_ENABLED:-true}"
    export TEMPO_ENABLED="${TEMPO_ENABLED:-true}"
    export ALERTMANAGER_ENABLED="${ALERTMANAGER_ENABLED:-true}"
    export CADVISOR_ENABLED="${CADVISOR_ENABLED:-true}"
    export NODE_EXPORTER_ENABLED="${NODE_EXPORTER_ENABLED:-true}"
    export POSTGRES_EXPORTER_ENABLED="${POSTGRES_EXPORTER_ENABLED:-true}"
    export REDIS_EXPORTER_ENABLED="${REDIS_EXPORTER_ENABLED:-true}"
  fi
}

# Detect custom services (CS_N pattern)
detect_custom_services() {
  local custom_services=()
  local cs_count=0

  # Check for CS_N variables (up to 20)
  for i in {1..20}; do
    local cs_var="CS_${i}"
    local cs_value="${!cs_var:-}"

    if [[ -n "$cs_value" ]]; then
      custom_services+=("$cs_value")
      cs_count=$((cs_count + 1))

      # Parse service definition
      IFS=':' read -r name template port <<<"$cs_value"

      # Export parsed values for build system
      export "CS_${i}_NAME=$name"
      export "CS_${i}_TEMPLATE=$template"
      export "CS_${i}_PORT=${port:-$((8000 + i))}"

      # Set route using ternary pattern (env-specific or default)
      local route_var="CS_${i}_ROUTE"
      local env_upper="$(echo "$ENV" | tr '[:lower:]' '[:upper:]')"
      local env_route_var="CS_${i}_${env_upper}_ROUTE"

      # Check for environment-specific route first
      if [[ -n "${!env_route_var:-}" ]]; then
        export "$route_var=${!env_route_var}"
      elif [[ -n "${!route_var:-}" ]]; then
        # Use explicitly set route
        export "$route_var=${!route_var}"
      else
        # Default route based on service name
        export "$route_var=${name//_/-}"
      fi
    fi
  done

  export CUSTOM_SERVICES_COUNT="$cs_count"
  export CUSTOM_SERVICES="${custom_services[*]}"
}

# Detect frontend applications
detect_frontend_apps() {
  local frontend_apps=()
  local app_count=0

  # Check for FRONTEND_APP_N variables (up to 10)
  for i in {1..10}; do
    # Support both NAME and SYSTEM_NAME for compatibility
    local app_name_var="FRONTEND_APP_${i}_NAME"
    local app_system_name_var="FRONTEND_APP_${i}_SYSTEM_NAME"
    local app_name="${!app_name_var:-${!app_system_name_var:-}}"

    if [[ -n "$app_name" ]]; then
      frontend_apps+=("$app_name")
      app_count=$((app_count + 1))

      # Set defaults using ternary patterns
      local port_var="FRONTEND_APP_${i}_PORT"
      local route_var="FRONTEND_APP_${i}_ROUTE"
      local framework_var="FRONTEND_APP_${i}_FRAMEWORK"

      # Port with smart default
      export "$port_var=${!port_var:-$((3000 + i - 1))}"

      # Route with environment awareness
      local env_upper="$(echo "$ENV" | tr '[:lower:]' '[:upper:]')"
      local env_route_var="FRONTEND_APP_${i}_${env_upper}_ROUTE"
      if [[ -n "${!env_route_var:-}" ]]; then
        export "$route_var=${!env_route_var}"
      elif [[ -n "${!route_var:-}" ]]; then
        export "$route_var=${!route_var}"
      else
        export "$route_var=$app_name"
      fi

      # Framework default
      export "$framework_var=${!framework_var:-react}"
    fi
  done

  # Also check legacy APP_N pattern for backward compatibility
  for i in {1..10}; do
    local app_var="APP_${i}"
    local app_value="${!app_var:-}"

    if [[ -n "$app_value" ]]; then
      # Parse legacy format if not already counted
      if [[ ! " ${frontend_apps[*]} " =~ " ${app_value} " ]]; then
        app_count=$((app_count + 1))
        frontend_apps+=("$app_value")

        # Convert to new format
        export "FRONTEND_APP_${app_count}_NAME=$app_value"
        export "FRONTEND_APP_${app_count}_PORT=$((3000 + app_count - 1))"
        export "FRONTEND_APP_${app_count}_ROUTE=$app_value"
      fi
    fi
  done

  export FRONTEND_APPS_COUNT="$app_count"
  export FRONTEND_APPS="${frontend_apps[*]}"
}

# Get environment-specific domain
get_env_domain() {
  local env="${1:-${ENV:-dev}}"

  # Use ternary pattern for domain selection
  case "$env" in
    prod | production)
      echo "${PROD_DOMAIN:-${PRODUCTION_DOMAIN:-${BASE_DOMAIN:-localhost}}}"
      ;;
    staging | stage)
      echo "${STAGING_DOMAIN:-${BASE_DOMAIN:-localhost}}"
      ;;
    dev | development | *)
      echo "${DEV_DOMAIN:-${BASE_DOMAIN:-localhost}}"
      ;;
  esac
}

# Get environment-specific port
get_env_port() {
  local service="$1"
  local default_port="$2"
  local env="${3:-${ENV:-dev}}"

  # Convert to uppercase for variable lookup
  local service_upper=$(echo "$service" | tr '[:lower:]' '[:upper:]' | tr '-' '_')
  local env_upper=$(echo "$env" | tr '[:lower:]' '[:upper:]')

  # Check for environment-specific port
  local env_port_var="${service_upper}_${env_upper}_PORT"
  local port_var="${service_upper}_PORT"

  # Use ternary pattern: env-specific > service-specific > default
  echo "${!env_port_var:-${!port_var:-$default_port}}"
}

# Generate service configuration with environment awareness
generate_service_config() {
  local service="$1"
  local env="${ENV:-dev}"

  # Detect all services
  detect_enabled_services
  detect_custom_services
  detect_frontend_apps

  # Get environment-specific values
  local domain=$(get_env_domain "$env")
  local hasura_port=$(get_env_port "hasura" 8080 "$env")
  local auth_port=$(get_env_port "auth" 4000 "$env")

  # Export computed values
  export EFFECTIVE_DOMAIN="$domain"
  export EFFECTIVE_HASURA_PORT="$hasura_port"
  export EFFECTIVE_AUTH_PORT="$auth_port"

  # Generate service-specific config
  case "$service" in
    summary)
      echo "Environment: $env"
      echo "Domain: $domain"
      echo "Core Services: $(count_enabled_services core)"
      echo "Optional Services: $(count_enabled_services optional)"
      echo "Custom Services: ${CUSTOM_SERVICES_COUNT:-0}"
      echo "Frontend Apps: ${FRONTEND_APPS_COUNT:-0}"
      [[ "$MONITORING_ENABLED" == "true" ]] && echo "Monitoring: 10 services"
      ;;
    *)
      # Return service-specific config
      ;;
  esac
}

# Count enabled services by category
count_enabled_services() {
  local category="$1"
  local count=0

  case "$category" in
    core)
      [[ "$POSTGRES_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$HASURA_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$AUTH_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$NGINX_ENABLED" == "true" ]] && count=$((count + 1))
      ;;
    optional)
      [[ "$NSELF_ADMIN_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$MINIO_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$REDIS_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$MEILISEARCH_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$MAILPIT_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$MLFLOW_ENABLED" == "true" ]] && count=$((count + 1))
      [[ "$FUNCTIONS_ENABLED" == "true" ]] && count=$((count + 1))
      ;;
  esac

  echo "$count"
}

# Export all functions
export -f detect_enabled_services
export -f detect_custom_services
export -f detect_frontend_apps
export -f get_env_domain
export -f get_env_port
export -f generate_service_config
export -f count_enabled_services
