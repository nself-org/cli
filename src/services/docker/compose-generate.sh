#!/usr/bin/env bash
# compose-generate.sh - Generate docker-compose.yml configuration
# Refactored modular version using separate service modules
set -euo pipefail

# Always start with tracing off unless explicitly in debug mode
set +x

# Error handler with more details (only in debug mode)
if [[ "${DEBUG:-false}" == "true" ]]; then
  trap 'echo "Error on line $LINENO: $BASH_COMMAND" >&2' ERR
  set -x
else
  trap '' ERR
fi

# Get script directory (macOS compatible)
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  COMPOSE_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  # When called via bash script.sh, $0 is the script path
  COMPOSE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
fi

# Source display utilities first (for logging functions)
if [[ -f "$COMPOSE_SCRIPT_DIR/../../lib/utils/display.sh" ]]; then
  source "$COMPOSE_SCRIPT_DIR/../../lib/utils/display.sh"
fi

# Source platform compatibility utilities
if [[ -f "$COMPOSE_SCRIPT_DIR/../../lib/utils/platform-compat.sh" ]]; then
  source "$COMPOSE_SCRIPT_DIR/../../lib/utils/platform-compat.sh"
fi

# Source environment utilities for functions only
if [[ -f "$COMPOSE_SCRIPT_DIR/../../lib/utils/env.sh" ]]; then
  source "$COMPOSE_SCRIPT_DIR/../../lib/utils/env.sh"
fi

# IMPORTANT: Build is environment-agnostic for service CONFIGURATION
# Configuration values use ${VAR:-default} in generated docker-compose.yml
# so the same file works across environments (dev, staging, prod).
#
# However, STRUCTURAL variables (which services exist: CS_N, MONITORING_ENABLED, etc.)
# must be known at build time. These are normally inherited from the parent process,
# but we load .env files as a fallback if they're missing (Bug #19 definitive fix).

# Set smart defaults that can be overridden by explicitly set env vars
export PROJECT_NAME="${PROJECT_NAME:-myproject}"
export ENV="${ENV:-dev}"
export BASE_DOMAIN="${BASE_DOMAIN:-localhost}"

# CRITICAL (Bug #19 definitive fix): Load env files if structural variables are missing
# CS_N variables define which custom services to create and have no defaults.
# If the parent process didn't export them, we must load them directly.
# This also covers MONITORING_ENABLED, *_ENABLED flags, and FRONTEND_APP_N variables.
if [[ -z "${CS_1:-}" ]] && { [[ -f ".env.dev" ]] || [[ -f ".env" ]]; }; then
  for _envfile in .env.dev .env; do
    if [[ -f "$_envfile" ]]; then
      set -a
      source "$_envfile" 2>/dev/null || true
      set +a
    fi
  done
  # Re-derive defaults with potentially updated values
  export PROJECT_NAME="${PROJECT_NAME:-myproject}"
  export ENV="${ENV:-dev}"
  export BASE_DOMAIN="${BASE_DOMAIN:-localhost}"
fi

# Database defaults
export POSTGRES_USER="${POSTGRES_USER:-postgres}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
export POSTGRES_DB="${POSTGRES_DB:-${PROJECT_NAME}}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"

# Service port defaults
export HASURA_PORT="${HASURA_PORT:-8080}"
export AUTH_PORT="${AUTH_PORT:-4000}"
export STORAGE_PORT="${STORAGE_PORT:-5000}"
export REDIS_PORT="${REDIS_PORT:-6379}"
export MINIO_PORT="${MINIO_PORT:-9000}"
export MINIO_CONSOLE_PORT="${MINIO_CONSOLE_PORT:-9001}"

# Source smart defaults to handle JWT construction
if [[ -f "$COMPOSE_SCRIPT_DIR/../../services/auth/smart-defaults.sh" ]]; then
  source "$COMPOSE_SCRIPT_DIR/../../services/auth/smart-defaults.sh"
  apply_smart_defaults
fi

# Source auth config for multi-app support
if [[ -f "$COMPOSE_SCRIPT_DIR/../../services/auth/multi-app.sh" ]]; then
  source "$COMPOSE_SCRIPT_DIR/../../services/auth/multi-app.sh"
fi

# Source all service modules
MODULES_DIR="$COMPOSE_SCRIPT_DIR/compose-modules"
if [[ -d "$MODULES_DIR" ]]; then
  for module in "$MODULES_DIR"/*.sh; do
    [[ -f "$module" ]] && source "$module"
  done
else
  echo "Error: compose-modules directory not found" >&2
  exit 1
fi

# Compose database URLs from individual variables
# CRITICAL: Always use port 5432 for internal container-to-container communication
# The POSTGRES_PORT variable is for external host access only
construct_database_urls() {
  # Use smart defaults with ternary operators
  local db_user="${POSTGRES_USER:-postgres}"
  local db_pass="${POSTGRES_PASSWORD:-postgres}"
  local db_host="postgres"  # Container name for internal networking
  local db_port="5432"      # Always use 5432 internally
  local db_name="${POSTGRES_DB:-${PROJECT_NAME:-myproject}}"

  # URL encode password to handle special characters
  local encoded_pass=$(url_encode "$db_pass")

  # Construct database URLs with defaults
  export DATABASE_URL="postgresql://${db_user}:${encoded_pass}@${db_host}:${db_port}/${db_name}"
  export POSTGRES_URL="postgresql://${db_user}:${encoded_pass}@${db_host}:${db_port}/${db_name}"

  # Auth-specific database URL (can point to a different database)
  local auth_db="${AUTH_DATABASE_NAME:-${db_name}}"
  export AUTH_DATABASE_URL="postgresql://${db_user}:${encoded_pass}@${db_host}:${db_port}/${auth_db}"

  # Storage database URL (for Hasura Storage)
  local storage_db="${STORAGE_DATABASE_NAME:-${db_name}}"
  export STORAGE_DATABASE_URL="postgresql://${db_user}:${encoded_pass}@${db_host}:${db_port}/${storage_db}"
}

# URL encode function for password handling
url_encode() {
  local string="${1}"
  local strlen=${#string}
  local encoded=""
  local pos c o

  for (( pos=0 ; pos<strlen ; pos++ )); do
    c=${string:$pos:1}
    case "$c" in
      [-_.~a-zA-Z0-9] ) o="${c}" ;;
      * ) printf -v o '%%%02x' "'$c" ;;
    esac
    encoded+="${o}"
  done
  echo "${encoded}"
}

# Ensure DOCKER_NETWORK is expanded for Docker Compose
DOCKER_NETWORK="${PROJECT_NAME}_network"
export DOCKER_NETWORK

# Set environment-specific defaults
# Support both ENV and ENVIRONMENT for backward compatibility
ENVIRONMENT="${ENV:-${ENVIRONMENT:-development}}"
export NODE_ENV="${NODE_ENV:-$ENVIRONMENT}"

# Set defaults based on environment
case "$ENVIRONMENT" in
  production|prod)
    export LOG_LEVEL="${LOG_LEVEL:-warn}"
    ;;
  staging|stage)
    export LOG_LEVEL="${LOG_LEVEL:-info}"
    ;;
  *)
    export LOG_LEVEL="${LOG_LEVEL:-debug}"
    ;;
esac

# Backup existing docker-compose.yml only if it will be changed
backup_existing_compose() {
  if [[ -f docker-compose.yml ]]; then
    local backup_dir=".volumes/backups/compose"
    mkdir -p "$backup_dir"
    local timestamp=$(date +%Y%m%d_%H%M%S)
    cp docker-compose.yml "$backup_dir/docker-compose.yml.$timestamp"

    # Keep only last 10 backups
    ls -t "$backup_dir"/docker-compose.yml.* 2>/dev/null | tail -n +11 | xargs rm -f 2>/dev/null || true

    # Mark that docker-compose was changed
    export DOCKER_COMPOSE_CHANGED="true"
  fi
}

# Generate the complete docker-compose.yml
generate_docker_compose() {
  # Set default DOCKER_NETWORK if not set
  : ${DOCKER_NETWORK:="${PROJECT_NAME}_network"}

  # Construct database URLs
  construct_database_urls

  # Backup existing file
  backup_existing_compose

  # Start generating the compose file
  cat > docker-compose.yml <<EOF
# Generated by nself build - DO NOT EDIT MANUALLY
# Project: ${PROJECT_NAME}
# Date: $(date '+%Y-%m-%d %H:%M:%S')
# Build Version: $(cat "$COMPOSE_SCRIPT_DIR/../VERSION" 2>/dev/null || echo "unknown")

name: ${PROJECT_NAME}

networks:
  ${DOCKER_NETWORK}:
    name: ${DOCKER_NETWORK}
    driver: bridge

volumes:
  postgres_data:
    driver: local
  nginx_cache:
    driver: local
EOF

  # Add conditional volumes based on enabled services
  [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] && echo "  nself_admin_data:" >> docker-compose.yml
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && echo "  redis_data:" >> docker-compose.yml
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && echo "  minio_data:" >> docker-compose.yml

  # Search service volumes (provider-agnostic)
  local search_enabled="${SEARCH_ENABLED:-false}"
  local search_provider="${SEARCH_PROVIDER:-meilisearch}"

  # Legacy support for old variables
  [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] && search_enabled="true" && search_provider="meilisearch"
  [[ "${TYPESENSE_ENABLED:-false}" == "true" ]] && search_enabled="true" && search_provider="typesense"

  if [[ "$search_enabled" == "true" ]]; then
    case "$search_provider" in
      meilisearch)
        echo "  meilisearch_data:" >> docker-compose.yml
        ;;
      typesense)
        echo "  typesense_data:" >> docker-compose.yml
        ;;
    esac
  fi

  [[ "${SONIC_ENABLED:-false}" == "true" ]] && echo "  sonic_data:" >> docker-compose.yml
  [[ "${MLFLOW_ENABLED:-false}" == "true" ]] && echo "  mlflow_data:" >> docker-compose.yml
  [[ "${GRAFANA_ENABLED:-false}" == "true" ]] && echo "  grafana_data:" >> docker-compose.yml
  [[ "${PROMETHEUS_ENABLED:-false}" == "true" ]] && echo "  prometheus_data:" >> docker-compose.yml
  [[ "${LOKI_ENABLED:-false}" == "true" ]] && echo "  loki_data:" >> docker-compose.yml
  [[ "${TEMPO_ENABLED:-false}" == "true" ]] && echo "  tempo_data:" >> docker-compose.yml
  [[ "${ALERTMANAGER_ENABLED:-false}" == "true" ]] && echo "  alertmanager_data:" >> docker-compose.yml
  [[ "${PGADMIN_ENABLED:-false}" == "true" ]] && echo "  pgadmin_data:" >> docker-compose.yml
  [[ "${PORTAINER_ENABLED:-false}" == "true" ]] && echo "  portainer_data:" >> docker-compose.yml

  # Start services section
  echo "" >> docker-compose.yml
  echo "services:" >> docker-compose.yml

  # Generate services in sorted order (matches nself status display)
  # Order: Core → Optional → Monitoring → Custom

  # ============================================
  # Core Services (in display order)
  # ============================================
  echo "  # ============================================" >> docker-compose.yml
  echo "  # Core Services" >> docker-compose.yml
  echo "  # ============================================" >> docker-compose.yml

  generate_postgres_service >> docker-compose.yml
  generate_hasura_service >> docker-compose.yml
  generate_auth_service >> docker-compose.yml
  generate_nginx_service >> docker-compose.yml

  # ============================================
  # Optional Services (in display order)
  # ============================================
  if [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] || \
     [[ "${MINIO_ENABLED:-false}" == "true" ]] || \
     [[ "${REDIS_ENABLED:-false}" == "true" ]] || \
     [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]] || \
     [[ "${MAILPIT_ENABLED:-false}" == "true" ]] || \
     [[ "${SEARCH_ENABLED:-false}" == "true" ]] || \
     [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] || \
     [[ "${TYPESENSE_ENABLED:-false}" == "true" ]] || \
     [[ "${MLFLOW_ENABLED:-false}" == "true" ]]; then
    echo "" >> docker-compose.yml
    echo "  # ============================================" >> docker-compose.yml
    echo "  # Optional Services" >> docker-compose.yml
    echo "  # ============================================" >> docker-compose.yml
    generate_utility_services >> docker-compose.yml
  fi

  # ============================================
  # Monitoring Services (in priority order)
  # ============================================
  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    echo "" >> docker-compose.yml
    echo "  # ============================================" >> docker-compose.yml
    echo "  # Monitoring Services" >> docker-compose.yml
    echo "  # ============================================" >> docker-compose.yml
    # CRITICAL (Bug #20/#29 fix): Generate monitoring in isolation so errors
    # don't prevent custom services from being generated. Monitoring failures
    # are non-fatal — custom services MUST always be generated.
    generate_monitoring_services >> docker-compose.yml 2>/dev/null || true
    generate_monitoring_exporters >> docker-compose.yml 2>/dev/null || true
  fi

  # ============================================
  # Custom Services (alphabetical)
  # ============================================
  generate_template_custom_services >> docker-compose.yml

  # ============================================
  # Plugin Services (Dockerized plugins)
  # ============================================
  generate_all_plugin_services >> docker-compose.yml

  # Frontend apps are not Docker containers, skip from compose file
  generate_frontend_apps >> docker-compose.yml

  echo "" >> docker-compose.yml
  echo "# End of generated docker-compose.yml" >> docker-compose.yml

  # Protect generated file: it may contain variable names for secrets
  chmod 600 docker-compose.yml 2>/dev/null || true

  # Explicitly return success
  return 0
}

# Main execution
main() {
  # Sanitize and set defaults for critical variables
  if [[ -z "$PROJECT_NAME" ]] || [[ "$PROJECT_NAME" =~ [[:space:]] ]]; then
    PROJECT_NAME=$(echo "${PROJECT_NAME:-myproject}" | tr -d ' ' | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]//g')
    [[ -z "$PROJECT_NAME" ]] && PROJECT_NAME="myproject"
  fi

  # Check if docker-compose.yml exists to detect first build
  local compose_exists=false
  [[ -f "docker-compose.yml" ]] && compose_exists=true

  # Show verbose output if requested
  if [[ "${VERBOSE:-false}" == "true" ]]; then
    echo "Generating docker-compose.yml for project: ${PROJECT_NAME}"
  fi

  # Generate the compose file (redirect based on verbosity)
  if [[ "${VERBOSE:-false}" == "true" ]]; then
    if ! generate_docker_compose; then
      echo "Error: Failed to generate docker-compose.yml" >&2
      return 1
    fi
  else
    if ! generate_docker_compose 2>&1 | grep -i 'error\|fatal\|unbound' >&2; then
      : # Errors (if any) were already shown
    fi
    # Verify the file was actually generated
    if [[ ! -f "docker-compose.yml" ]] || [[ ! -s "docker-compose.yml" ]]; then
      echo "Error: Failed to generate docker-compose.yml" >&2
      return 1
    fi
  fi

  # If this was first time, mark it
  if [[ "$compose_exists" == "false" ]]; then
    export DOCKER_COMPOSE_CHANGED="true"
  fi

  # Validate the generated file (skip if docker not available)
  if command -v docker >/dev/null 2>&1; then
    if [[ "${VERBOSE:-false}" == "true" ]]; then
      if safe_timeout 5 docker compose config >/dev/null 2>&1; then
        echo "✓ docker-compose.yml validation passed"
      else
        echo "⚠ docker-compose.yml validation warnings (expected)"
      fi
    else
      safe_timeout 5 docker compose config >/dev/null 2>&1 || true
    fi
  fi

  # Show success in verbose mode
  if [[ "${VERBOSE:-false}" == "true" ]]; then
    echo "✓ docker-compose.yml generated successfully"
  fi

  # Always return success if we got this far
  return 0
}

# Run main function
main "$@"