#!/usr/bin/env bash

# build-orchestrator.sh - Main build orchestration with proper env handling
# Loads env to detect WHAT to build, but outputs use runtime vars for HOW

# Source change detection module
ORCHESTRATOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$ORCHESTRATOR_DIR/change-detection.sh" ]]; then
  source "$ORCHESTRATOR_DIR/change-detection.sh"
fi

# Main orchestration function
orchestrate_build() {
  local project_name="${1:-$(basename "$PWD")}"
  local env="${2:-dev}"
  local force="${3:-false}"
  local verbose="${4:-false}"

  # Export basic settings
  export PROJECT_NAME="${PROJECT_NAME:-$project_name}"
  export ENV="${ENV:-$env}"
  export VERBOSE="$verbose"

  # Initialize change tracking
  if command -v init_change_tracking >/dev/null 2>&1; then
    init_change_tracking
  fi

  # Load environment files to detect what services to build
  # This tells us WHAT to provision, not HOW to configure it
  load_env_for_detection

  # Validate and fix environment variables (including PROJECT_NAME)
  if command -v validate_environment >/dev/null 2>&1; then
    validate_environment || true
  fi

  # Detect all services and apps
  detect_all_services

  # Detect what changed since last build
  if command -v detect_changes >/dev/null 2>&1; then
    detect_changes "$force"
    display_changes
  fi

  # Now build everything with runtime variables
  build_all_components "$force"

  # Check SSL trust status
  if command -v check_ssl_status >/dev/null 2>&1; then
    check_ssl_status
  fi

  # Save current build state for next run
  if command -v save_build_state >/dev/null 2>&1; then
    save_build_state
  fi

  # Display recommendations
  if command -v display_restart_recommendation >/dev/null 2>&1; then
    display_restart_recommendation
  fi

  if command -v display_ssl_trust_recommendation >/dev/null 2>&1; then
    display_ssl_trust_recommendation
  fi

  return 0
}

# Load environment files ONLY for service detection
load_env_for_detection() {
  local env="${ENV:-dev}"

  # Load files in cascade order for proper detection
  # .env.dev -> .env.[env] -> .env
  if [[ -f ".env.dev" ]]; then
    set -a
    source ".env.dev" 2>/dev/null || true
    set +a
  fi

  # Load environment-specific file
  case "$env" in
    staging)
      if [[ -f ".env.staging" ]]; then
        set -a
        source ".env.staging" 2>/dev/null || true
        set +a
      fi
      ;;
    prod | production)
      if [[ -f ".env.prod" ]]; then
        set -a
        source ".env.prod" 2>/dev/null || true
        set +a
      fi
      ;;
  esac

  # Load local overrides last
  if [[ -f ".env" ]]; then
    set -a
    source ".env" 2>/dev/null || true
    set +a
  fi
}

# Detect all services that need to be built
detect_all_services() {
  # Core services detection
  export POSTGRES_ENABLED="${POSTGRES_ENABLED:-true}"
  export HASURA_ENABLED="${HASURA_ENABLED:-true}"
  export AUTH_ENABLED="${AUTH_ENABLED:-true}"
  export NGINX_ENABLED="${NGINX_ENABLED:-true}"

  # Optional services
  export NSELF_ADMIN_ENABLED="${NSELF_ADMIN_ENABLED:-false}"
  export MINIO_ENABLED="${MINIO_ENABLED:-${STORAGE_ENABLED:-false}}"
  export REDIS_ENABLED="${REDIS_ENABLED:-false}"
  export MEILISEARCH_ENABLED="${MEILISEARCH_ENABLED:-false}"
  export MAILPIT_ENABLED="${MAILPIT_ENABLED:-false}"
  export MLFLOW_ENABLED="${MLFLOW_ENABLED:-false}"
  export FUNCTIONS_ENABLED="${FUNCTIONS_ENABLED:-false}"

  # Monitoring bundle
  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    export PROMETHEUS_ENABLED="true"
    export GRAFANA_ENABLED="true"
    export LOKI_ENABLED="true"
    export PROMTAIL_ENABLED="true"
    export TEMPO_ENABLED="true"
    export ALERTMANAGER_ENABLED="true"
    export CADVISOR_ENABLED="true"
    export NODE_EXPORTER_ENABLED="true"
    export POSTGRES_EXPORTER_ENABLED="true"
    export REDIS_EXPORTER_ENABLED="true"
  fi

  # Detect custom services (CS_N)
  detect_custom_services

  # Detect frontend apps
  detect_frontend_apps
}

# Detect custom services
detect_custom_services() {
  export CUSTOM_SERVICES=""
  export CUSTOM_SERVICE_COUNT=0

  for i in {1..20}; do
    local cs_var="CS_${i}"
    local cs_value="${!cs_var:-}"

    if [[ -n "$cs_value" ]]; then
      CUSTOM_SERVICE_COUNT=$((CUSTOM_SERVICE_COUNT + 1))

      # Parse service definition
      IFS=':' read -r name template port <<<"$cs_value"

      # Export service details for build
      export "CS_${i}_NAME=$name"
      export "CS_${i}_TEMPLATE=$template"
      export "CS_${i}_PORT=${port:-$((8000 + i))}"

      # Add to list
      CUSTOM_SERVICES="$CUSTOM_SERVICES $name"
    fi
  done
}

# Detect frontend applications
detect_frontend_apps() {
  export FRONTEND_APPS=""
  export FRONTEND_APP_COUNT=0

  for i in {1..10}; do
    # Support both NAME and SYSTEM_NAME
    local app_name_var="FRONTEND_APP_${i}_NAME"
    local app_system_var="FRONTEND_APP_${i}_SYSTEM_NAME"
    local app_name="${!app_name_var:-${!app_system_var:-}}"

    if [[ -n "$app_name" ]]; then
      FRONTEND_APP_COUNT=$((FRONTEND_APP_COUNT + 1))

      # Export app details for build
      export "FRONTEND_APP_${i}_NAME=$app_name"
      local port_var="FRONTEND_APP_${i}_PORT"
      export "FRONTEND_APP_${i}_PORT=${!port_var:-$((3000 + i - 1))}"

      # Check for remote schema configuration
      local schema_var="FRONTEND_APP_${i}_REMOTE_SCHEMA_NAME"
      if [[ -n "${!schema_var:-}" ]]; then
        export "$schema_var=${!schema_var}"
      fi

      # Add to list
      FRONTEND_APPS="$FRONTEND_APPS $app_name"
    fi
  done
}

# Build all components
build_all_components() {
  local force="${1:-false}"

  # Check if this is a no-op build (no changes detected)
  local skip_build=false
  if [[ ${#CHANGES_DETECTED[@]} -eq 0 ]] && [[ "$force" != "true" ]]; then
    skip_build=true
  fi

  # Always show what we're building (or would build)
  if [[ "$skip_build" == "true" ]]; then
    echo ""
    echo "Project configuration (up to date):"
  else
    echo ""
    echo "Building components for:"
  fi

  echo "  • Core services: 4"
  echo "  • Optional services: $(count_enabled_optional)"
  echo "  • Custom services: $CUSTOM_SERVICE_COUNT"
  echo "  • Frontend apps: $FRONTEND_APP_COUNT"
  [[ "$MONITORING_ENABLED" == "true" ]] && echo "  • Monitoring: 10 services"
  echo ""

  # If no changes and not forcing, we can skip most work
  if [[ "$skip_build" == "true" ]]; then
    # Still ensure directories exist
    setup_directories >/dev/null 2>&1
    return 0
  fi

  # Create directory structure
  setup_directories

  # Generate SSL certificates (checks if needed internally)
  generate_ssl_certificates "$force"

  # Copy custom service templates (only if new or forced)
  copy_custom_service_templates "$force"

  # Generate nginx configuration with runtime vars
  if [[ -f "$(dirname "${BASH_SOURCE[0]}")/nginx-generator.sh" ]]; then
    source "$(dirname "${BASH_SOURCE[0]}")/nginx-generator.sh"
  fi
  generate_nginx_config "$force"

  # Generate docker-compose with runtime vars
  if command -v generate_docker_compose >/dev/null 2>&1; then
    generate_docker_compose
  else
    # Fallback to compose-generate script
    local compose_script="${NSELF_ROOT:-/usr/local/lib/nself}/src/services/docker/compose-generate.sh"
    if [[ -f "$compose_script" ]]; then
      bash "$compose_script"
    fi
  fi

  # Generate database initialization
  if command -v generate_database_init >/dev/null 2>&1; then
    generate_database_init "$force"
  else
    # Basic database init
    mkdir -p postgres/init
    cat >postgres/init/00-init.sql <<'EOF'
-- Database initialization
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
EOF
  fi

  return 0
}

# Setup directory structure
setup_directories() {
  mkdir -p nginx/{conf.d,sites,includes,routes} 2>/dev/null || true
  mkdir -p ssl/certificates 2>/dev/null || true
  mkdir -p postgres/init 2>/dev/null || true
  mkdir -p services 2>/dev/null || true
  mkdir -p monitoring/{prometheus,grafana,loki,alertmanager,tempo} 2>/dev/null || true
  mkdir -p .volumes/{postgres,redis,minio,grafana,prometheus} 2>/dev/null || true
}

# Generate SSL certificates
# Bug #35 fix: Use setup_ssl_certificates() from ssl-generation.sh if available.
# The original version only generated a CN=localhost cert without SAN, and never
# created domain-specific certs (e.g., ssl/certificates/nself-org/).
generate_ssl_certificates() {
  local force="${1:-false}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  # Prefer the full SSL generator from ssl-generation.sh (handles SAN, mkcert, domain certs)
  if command -v setup_ssl_certificates >/dev/null 2>&1; then
    setup_ssl_certificates "$force"
    return $?
  fi

  # Fallback: generate self-signed certs with SAN for all domains
  mkdir -p ssl/certificates/localhost 2>/dev/null || true

  if [[ "$force" == "true" ]] || [[ ! -f "ssl/certificates/localhost/fullchain.pem" ]]; then
    # Build SAN list for all configured domains
    local san_entries="DNS:localhost,DNS:*.localhost,IP:127.0.0.1,IP:::1"

    if [[ "$base_domain" != "localhost" ]]; then
      san_entries="${san_entries},DNS:${base_domain},DNS:*.${base_domain}"
    fi

    # Always include local.nself.org for development
    san_entries="${san_entries},DNS:local.nself.org,DNS:*.local.nself.org"

    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout ssl/certificates/localhost/privkey.pem \
      -out ssl/certificates/localhost/fullchain.pem \
      -subj "/C=US/ST=State/L=City/O=Local Development/CN=${base_domain}" \
      -addext "subjectAltName=${san_entries}" \
      2>/dev/null || true
  fi

  # Create domain-specific cert directories with symlinks to the same cert
  # Nginx configs reference ssl/certificates/<domain-dir>/fullchain.pem
  if [[ "$base_domain" != "localhost" ]]; then
    local cert_dir_name
    if [[ "$base_domain" == "nself.org" ]] || [[ "$base_domain" == *".nself.org" ]]; then
      cert_dir_name="nself-org"
    else
      cert_dir_name=$(printf '%s' "$base_domain" | tr '.' '-')
    fi

    mkdir -p "ssl/certificates/${cert_dir_name}" 2>/dev/null || true
    if [[ ! -f "ssl/certificates/${cert_dir_name}/fullchain.pem" ]]; then
      cp ssl/certificates/localhost/fullchain.pem "ssl/certificates/${cert_dir_name}/fullchain.pem" 2>/dev/null || true
      cp ssl/certificates/localhost/privkey.pem "ssl/certificates/${cert_dir_name}/privkey.pem" 2>/dev/null || true
    fi
  fi

  # Always ensure nself-org certs exist for local dev
  mkdir -p ssl/certificates/nself-org 2>/dev/null || true
  if [[ ! -f "ssl/certificates/nself-org/fullchain.pem" ]]; then
    cp ssl/certificates/localhost/fullchain.pem ssl/certificates/nself-org/fullchain.pem 2>/dev/null || true
    cp ssl/certificates/localhost/privkey.pem ssl/certificates/nself-org/privkey.pem 2>/dev/null || true
  fi
}

# Copy custom service templates
copy_custom_service_templates() {
  local force="${1:-false}"
  local nself_root="${NSELF_ROOT:-/usr/local/lib/nself}"

  for i in {1..20}; do
    local cs_name_var="CS_${i}_NAME"
    local cs_template_var="CS_${i}_TEMPLATE"
    local cs_port_var="CS_${i}_PORT"

    local name="${!cs_name_var:-}"
    local template="${!cs_template_var:-}"
    local port="${!cs_port_var:-}"

    if [[ -n "$name" ]] && [[ -n "$template" ]]; then
      local service_dir="services/$name"

      # Skip if already exists and not forcing
      if [[ -d "$service_dir" ]] && [[ "$force" != "true" ]]; then
        continue
      fi

      # Find and copy template
      for lang in js python go rust; do
        local template_dir="$nself_root/src/templates/services/$lang/$template"
        if [[ -d "$template_dir" ]]; then
          echo "  → Copying template '$template' to services/$name"
          mkdir -p "$service_dir"
          cp -r "$template_dir"/* "$service_dir/" 2>/dev/null || true

          # Replace placeholders
          find "$service_dir" -type f \( -name "*.js" -o -name "*.ts" -o -name "*.py" \
            -o -name "*.go" -o -name "*.json" -o -name "*.yml" -o -name "Dockerfile*" \) \
            -exec sed -i.bak \
            -e "s/{{SERVICE_NAME}}/$name/g" \
            -e "s/{{SERVICE_PORT}}/$port/g" \
            -e "s/{{PROJECT_NAME}}/\${PROJECT_NAME}/g" \
            {} \; 2>/dev/null || true

          # Remove .template extensions
          find "$service_dir" -name "*.template" -exec bash -c 'mv "$1" "${1%.template}"' _ {} \;

          # Cleanup backup files
          find "$service_dir" -name "*.bak" -delete 2>/dev/null || true

          break
        fi
      done
    fi
  done
}

# Count enabled optional services
count_enabled_optional() {
  local count=0
  [[ "$NSELF_ADMIN_ENABLED" == "true" ]] && count=$((count + 1))
  [[ "$MINIO_ENABLED" == "true" ]] && count=$((count + 1))
  [[ "$REDIS_ENABLED" == "true" ]] && count=$((count + 1))
  [[ "$MEILISEARCH_ENABLED" == "true" ]] && count=$((count + 1))
  [[ "$MAILPIT_ENABLED" == "true" ]] && count=$((count + 1))
  [[ "$MLFLOW_ENABLED" == "true" ]] && count=$((count + 1))
  [[ "$FUNCTIONS_ENABLED" == "true" ]] && count=$((count + 1))
  echo $count
}

# Export functions
export -f orchestrate_build
export -f load_env_for_detection
export -f detect_all_services
export -f detect_custom_services
export -f detect_frontend_apps
export -f build_all_components
export -f setup_directories
export -f generate_ssl_certificates
export -f copy_custom_service_templates
export -f count_enabled_optional
