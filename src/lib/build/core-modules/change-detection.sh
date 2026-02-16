#!/usr/bin/env bash

# change-detection.sh - Intelligent build change detection and restart recommendations

# Initialize change tracking
init_change_tracking() {

set -euo pipefail

  export BUILD_STATE_DIR=".nself/build-state"
  mkdir -p "$BUILD_STATE_DIR"

  # Track what changed
  export CHANGES_DETECTED=()
  export RESTART_RECOMMENDED="none"
  export SSL_NEEDS_TRUST=false
}

# Calculate checksum for a file
calculate_checksum() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    echo "missing"
    return
  fi

  # Use md5 on macOS, md5sum on Linux
  if command -v md5 >/dev/null 2>&1; then
    md5 -q "$file" 2>/dev/null || echo "error"
  elif command -v md5sum >/dev/null 2>&1; then
    md5sum "$file" 2>/dev/null | awk '{print $1}' || echo "error"
  else
    echo "error"
  fi
}

# Save current build state
save_build_state() {
  local state_file="$BUILD_STATE_DIR/last-build.state"

  {
    echo "timestamp=$(date +%s)"
    echo "env_checksum=$(calculate_checksum .env)"
    echo "env_dev_checksum=$(calculate_checksum .env.dev)"
    echo "env_staging_checksum=$(calculate_checksum .env.staging)"
    echo "env_prod_checksum=$(calculate_checksum .env.prod)"
    echo "compose_checksum=$(calculate_checksum docker-compose.yml)"
    echo "ssl_cert_checksum=$(calculate_checksum ssl/certificates/localhost/fullchain.pem)"
    echo "nginx_conf_checksum=$(calculate_checksum nginx/nginx.conf)"

    # Save enabled services state
    echo "postgres_enabled=${POSTGRES_ENABLED:-true}"
    echo "hasura_enabled=${HASURA_ENABLED:-true}"
    echo "auth_enabled=${AUTH_ENABLED:-true}"
    echo "nginx_enabled=${NGINX_ENABLED:-true}"
    echo "redis_enabled=${REDIS_ENABLED:-false}"
    echo "minio_enabled=${MINIO_ENABLED:-false}"
    echo "nself_admin_enabled=${NSELF_ADMIN_ENABLED:-false}"
    echo "meilisearch_enabled=${MEILISEARCH_ENABLED:-false}"
    echo "mailpit_enabled=${MAILPIT_ENABLED:-false}"
    echo "mlflow_enabled=${MLFLOW_ENABLED:-false}"
    echo "functions_enabled=${FUNCTIONS_ENABLED:-false}"
    echo "monitoring_enabled=${MONITORING_ENABLED:-false}"
    echo "custom_service_count=${CUSTOM_SERVICE_COUNT:-0}"
    echo "frontend_app_count=${FRONTEND_APP_COUNT:-0}"
  } >"$state_file"
}

# Load previous build state
load_previous_build_state() {
  local state_file="$BUILD_STATE_DIR/last-build.state"

  if [[ ! -f "$state_file" ]]; then
    return 1
  fi

  # Source the state file to load variables with PREV_ prefix
  while IFS='=' read -r key value; do
    [[ -z "$key" || "$key" =~ ^# ]] && continue
    export "PREV_${key}=$value"
  done <"$state_file"

  return 0
}

# Detect what changed since last build
detect_changes() {
  local force="${1:-false}"

  # Initialize change tracking
  CHANGES_DETECTED=()

  # Force rebuild means everything changed
  if [[ "$force" == "true" ]]; then
    CHANGES_DETECTED+=("force-rebuild")
    RESTART_RECOMMENDED="fresh"
    return 0
  fi

  # Load previous state
  if ! load_previous_build_state; then
    CHANGES_DETECTED+=("first-build")
    RESTART_RECOMMENDED="none"
    return 0
  fi

  # Check environment files
  local current_env_checksum=$(calculate_checksum .env)
  if [[ "$current_env_checksum" != "${PREV_env_checksum:-missing}" ]]; then
    CHANGES_DETECTED+=(".env")
  fi

  local current_env_dev_checksum=$(calculate_checksum .env.dev)
  if [[ "$current_env_dev_checksum" != "${PREV_env_dev_checksum:-missing}" ]]; then
    CHANGES_DETECTED+=(".env.dev")
  fi

  local current_env_staging_checksum=$(calculate_checksum .env.staging)
  if [[ "$current_env_staging_checksum" != "${PREV_env_staging_checksum:-missing}" ]]; then
    CHANGES_DETECTED+=(".env.staging")
  fi

  local current_env_prod_checksum=$(calculate_checksum .env.prod)
  if [[ "$current_env_prod_checksum" != "${PREV_env_prod_checksum:-missing}" ]]; then
    CHANGES_DETECTED+=(".env.prod")
  fi

  # Check service enable/disable changes (use defaults to avoid unbound variable errors)
  [[ "${REDIS_ENABLED:-false}" != "${PREV_redis_enabled:-false}" ]] && CHANGES_DETECTED+=("redis-toggled")
  [[ "${MINIO_ENABLED:-false}" != "${PREV_minio_enabled:-false}" ]] && CHANGES_DETECTED+=("minio-toggled")
  [[ "${NSELF_ADMIN_ENABLED:-false}" != "${PREV_nself_admin_enabled:-false}" ]] && CHANGES_DETECTED+=("admin-toggled")
  [[ "${MEILISEARCH_ENABLED:-false}" != "${PREV_meilisearch_enabled:-false}" ]] && CHANGES_DETECTED+=("search-toggled")
  [[ "${MAILPIT_ENABLED:-false}" != "${PREV_mailpit_enabled:-false}" ]] && CHANGES_DETECTED+=("mail-toggled")
  [[ "${MLFLOW_ENABLED:-false}" != "${PREV_mlflow_enabled:-false}" ]] && CHANGES_DETECTED+=("mlflow-toggled")
  [[ "${FUNCTIONS_ENABLED:-false}" != "${PREV_functions_enabled:-false}" ]] && CHANGES_DETECTED+=("functions-toggled")
  [[ "${MONITORING_ENABLED:-false}" != "${PREV_monitoring_enabled:-false}" ]] && CHANGES_DETECTED+=("monitoring-toggled")

  # Check custom services count change
  if [[ "${CUSTOM_SERVICE_COUNT:-0}" != "${PREV_custom_service_count:-0}" ]]; then
    CHANGES_DETECTED+=("custom-services-changed")
  fi

  # Check frontend apps count change
  if [[ "${FRONTEND_APP_COUNT:-0}" != "${PREV_frontend_app_count:-0}" ]]; then
    CHANGES_DETECTED+=("frontend-apps-changed")
  fi

  # Determine restart recommendation based on changes
  determine_restart_recommendation
}

# Determine what kind of restart is needed
determine_restart_recommendation() {
  # No changes = no restart needed
  if [[ ${#CHANGES_DETECTED[@]} -eq 0 ]]; then
    RESTART_RECOMMENDED="none"
    return 0
  fi

  # Check for service topology changes (services added/removed)
  local has_topology_change=false
  for change in "${CHANGES_DETECTED[@]}"; do
    case "$change" in
      *-toggled | custom-services-changed | frontend-apps-changed | force-rebuild)
        has_topology_change=true
        break
        ;;
    esac
  done

  if [[ "$has_topology_change" == "true" ]]; then
    RESTART_RECOMMENDED="fresh"
    return 0
  fi

  # Check for config-only changes
  local has_config_change=false
  for change in "${CHANGES_DETECTED[@]}"; do
    case "$change" in
      .env | .env.*)
        has_config_change=true
        break
        ;;
    esac
  done

  if [[ "$has_config_change" == "true" ]]; then
    RESTART_RECOMMENDED="restart"
    return 0
  fi

  # Default to restart for any other changes
  RESTART_RECOMMENDED="restart"
}

# Check SSL certificate validity and trust status
check_ssl_status() {
  SSL_NEEDS_TRUST=false

  # Check if localhost certificates exist
  if [[ ! -f "ssl/certificates/localhost/fullchain.pem" ]]; then
    return 0
  fi

  # Check if mkcert is available
  local mkcert_cmd=""
  if command -v mkcert >/dev/null 2>&1; then
    mkcert_cmd="mkcert"
  elif [[ -x "${HOME}/.nself/bin/mkcert" ]]; then
    mkcert_cmd="${HOME}/.nself/bin/mkcert"
  else
    return 0 # Can't check trust without mkcert
  fi

  # Check if root CA is installed
  if ! $mkcert_cmd -install -check 2>/dev/null; then
    SSL_NEEDS_TRUST=true
  fi

  # Check certificate expiration (only for mkcert certs)
  if command -v openssl >/dev/null 2>&1; then
    local expiry_date=$(openssl x509 -enddate -noout -in ssl/certificates/localhost/fullchain.pem 2>/dev/null | cut -d= -f2)
    if [[ -n "$expiry_date" ]]; then
      local expiry_epoch=$(date -j -f "%b %d %T %Y %Z" "$expiry_date" "+%s" 2>/dev/null || date -d "$expiry_date" "+%s" 2>/dev/null)
      local current_epoch=$(date "+%s")
      local days_until_expiry=$(((expiry_epoch - current_epoch) / 86400))

      # If expiring in less than 30 days, recommend regeneration
      if [[ $days_until_expiry -lt 30 ]]; then
        CHANGES_DETECTED+=("ssl-expiring")
      fi
    fi
  fi
}

# Display change detection results
display_changes() {
  if [[ ${#CHANGES_DETECTED[@]} -eq 0 ]]; then
    echo ""
    printf "\033[32m✓\033[0m No configuration changes detected\n"
    printf "  \033[2mBuild outputs are up to date\033[0m\n"
    return 0
  fi

  echo ""
  printf "\033[33m!\033[0m Changes detected:\n"
  for change in "${CHANGES_DETECTED[@]}"; do
    case "$change" in
      first-build)
        printf "  • First build - generating all files\n"
        ;;
      force-rebuild)
        printf "  • Force rebuild requested\n"
        ;;
      .env)
        printf "  • .env file modified\n"
        ;;
      .env.*)
        printf "  • ${change} file modified\n"
        ;;
      *-toggled)
        local service=$(echo "$change" | sed 's/-toggled//')
        printf "  • Service $service enabled/disabled\n"
        ;;
      custom-services-changed)
        printf "  • Custom services count changed\n"
        ;;
      frontend-apps-changed)
        printf "  • Frontend apps count changed\n"
        ;;
      ssl-expiring)
        printf "  • SSL certificate expiring soon\n"
        ;;
      *)
        printf "  • $change\n"
        ;;
    esac
  done
}

# Display restart recommendation
display_restart_recommendation() {
  # Skip if no changes or first build
  if [[ "$RESTART_RECOMMENDED" == "none" ]]; then
    return 0
  fi

  # Check if containers are running
  local running_containers=0
  if command -v docker >/dev/null 2>&1; then
    running_containers=$(docker ps --filter "name=${PROJECT_NAME:-}_" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
  fi

  # Only show restart recommendation if containers are running
  if [[ $running_containers -eq 0 ]]; then
    echo ""
    printf "\033[36mℹ\033[0m To start your project:\n"
    printf "  \033[1mnself start\033[0m\n"
    return 0
  fi

  echo ""
  case "$RESTART_RECOMMENDED" in
    fresh)
      printf "\033[33m⚠\033[0m  Services added/removed - fresh start recommended:\n"
      printf "  \033[1mnself start --fresh\033[0m\n"
      ;;
    restart)
      printf "\033[36mℹ\033[0m  Configuration changed - restart to apply:\n"
      printf "  \033[1mnself restart\033[0m\n"
      ;;
  esac
}

# Display SSL trust recommendation
display_ssl_trust_recommendation() {
  if [[ "$SSL_NEEDS_TRUST" == "true" ]]; then
    echo ""
    printf "\033[33m⚠\033[0m  SSL certificates not trusted by system\n"
    printf "  \033[2mRun \033[36mnself trust\033[2m to install root CA and remove browser warnings\033[0m\n"
    printf "  \033[2m(You only need to do this once per machine)\033[0m\n"
  fi
}

# Export functions
export -f init_change_tracking
export -f calculate_checksum
export -f save_build_state
export -f load_previous_build_state
export -f detect_changes
export -f determine_restart_recommendation
export -f check_ssl_status
export -f display_changes
export -f display_restart_recommendation
export -f display_ssl_trust_recommendation
