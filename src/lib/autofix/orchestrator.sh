#!/usr/bin/env bash


# Main autofix orchestrator - coordinates error analysis and fixes
# Designed for minimal output and maximum effectiveness

# Source dependencies
AUTOFIX_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$AUTOFIX_DIR/error-analyzer.sh"
source "$AUTOFIX_DIR/state-tracker.sh"
source "$AUTOFIX_DIR/pre-checks.sh"

# Source all fixes
for fix_file in "$AUTOFIX_DIR"/fixes/*.sh; do
  [[ -f "$fix_file" ]] && source "$fix_file"
done

# Explicitly source new fix modules if they exist
[[ -f "$AUTOFIX_DIR/fixes/healthcheck.sh" ]] && source "$AUTOFIX_DIR/fixes/healthcheck.sh"
[[ -f "$AUTOFIX_DIR/fixes/bullmq.sh" ]] && source "$AUTOFIX_DIR/fixes/bullmq.sh"

# Explicitly source schema fixes if it exists
[[ -f "$AUTOFIX_DIR/fixes/schema.sh" ]] && source "$AUTOFIX_DIR/fixes/schema.sh"

# Source nginx-specific fixes
[[ -f "$AUTOFIX_DIR/../auto-fix/nginx-fix.sh" ]] && source "$AUTOFIX_DIR/../auto-fix/nginx-fix.sh"

# Main entry point for autofix
autofix_service() {
  local service_name="$1"
  local service_logs="$2"
  local verbose="${3:-false}"

  # Get clean service name
  local clean_name="${service_name#${PROJECT_NAME:-nself}_}"

  # Check attempt count
  local attempts=$(get_service_attempts "$service_name")
  if [[ $attempts -ge 3 ]]; then
    echo
    log_info "Unable to auto-fix $clean_name. Check configuration and try again."

    if [[ "$verbose" == "true" ]]; then
      echo "Last error logs:"
      echo "$service_logs" | head -10
    fi
    return 1
  fi

  # Analyze the error
  local error_code=$(analyze_error "$service_logs")
  local error_msg=$(get_error_message "$error_code")

  # Special handling for services that are just restarting due to dependencies
  if [[ "$error_code" == "NGINX_UPSTREAM_NOT_FOUND" ]] || [[ "$error_code" == "SCHEMA_NOT_FOUND" ]]; then
    # For schema issues, ensure schemas exist first
    if [[ "$error_code" == "SCHEMA_NOT_FOUND" ]]; then
      # Create schemas and restart the specific service
      fix_missing_schemas "$service_name"
      return 99 # Signal retry
    fi

    # These often need dependency fixes
    if [[ -f "$AUTOFIX_DIR/fixes/dependencies.sh" ]]; then
      source "$AUTOFIX_DIR/fixes/dependencies.sh"
      fix_service_dependencies "$service_name"
      return 99 # Signal retry
    fi
  fi

  # Clear the spinner line
  printf "\r                                                    \r"

  # Show what went wrong with attempt number
  log_error "$clean_name: $error_msg ($((attempts + 1)))"

  if [[ "$verbose" == "true" ]]; then
    echo "Error details: $error_code"
    echo "Log excerpt: $(echo "$service_logs" | grep -E "error|failed" | head -2)"
  fi

  # Apply the appropriate fix and get description
  local fix_result=1
  local fix_description=""

  case "$error_code" in
    POSTGRES_PORT_5433)
      fix_postgres_port_5433 "$service_name" "$attempts"
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    POSTGRES_NOT_RUNNING)
      fix_postgres_not_running
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    POSTGRES_AUTH_FAILED)
      fix_postgres_auth_failed
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    POSTGRES_CONNECTION)
      fix_postgres_connection
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    DATABASE_NOT_FOUND)
      fix_database_not_found
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    REDIS_CONNECTION)
      fix_redis_connection
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    ELASTICSEARCH_CONNECTION)
      fix_elasticsearch_connection
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    PORT_IN_USE)
      fix_port_in_use "$service_name"
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    OUT_OF_MEMORY)
      fix_out_of_memory
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    NETWORK_DNS)
      fix_network_dns
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    PERMISSION_DENIED)
      fix_permission_denied "$service_name"
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    MISSING_ENV_VARS)
      fix_missing_env_vars
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    MISSING_FILES)
      # Try to generate missing files
      docker compose stop "$service_name" >/dev/null 2>&1
      docker compose rm -f "$service_name" >/dev/null 2>&1
      nself build --force >/dev/null 2>&1
      fix_result=0
      fix_description="Regenerated missing files"
      ;;
    SSL_CERT_ERROR)
      fix_ssl_cert_error
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    SCHEMA_NOT_FOUND)
      fix_missing_schemas "$service_name"
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    MISSING_NODE_MODULES)
      fix_missing_node_modules "$service_name"
      fix_result=$?
      fix_description=$(get_last_fix_description)
      ;;
    NGINX_UPSTREAM_NOT_FOUND | NGINX_*)
      # Use comprehensive nginx fix function if available
      if declare -f fix_nginx_restart_loop >/dev/null 2>&1; then
        fix_nginx_restart_loop "$service_name" "$verbose"
        fix_result=$?
        fix_description="Fixed nginx configuration issues"
      else
        # Fallback to basic fix
        fix_nginx_upstream
        fix_result=$?
        fix_description=$(get_last_fix_description)
      fi
      ;;
    NO_SHELL_IN_CONTAINER)
      # Container has no shell - just recreate it
      docker compose stop "$service_name" >/dev/null 2>&1
      docker compose rm -f "$service_name" >/dev/null 2>&1
      docker compose up -d "$service_name" >/dev/null 2>&1
      fix_result=$?
      fix_description="Recreated $service_name container"
      ;;
    MISSING_HEALTHCHECK_TOOLS)
      if declare -f fix_service_healthcheck >/dev/null 2>&1; then
        fix_service_healthcheck "$service_name"
        fix_result=$?
        fix_description=$(get_last_fix_description)
      else
        fix_missing_healthcheck_tools "$service_name"
        fix_result=$?
        fix_description=$(get_last_fix_description)
      fi
      ;;
    BULLMQ_MISSING_MODULES)
      if declare -f fix_bullmq_worker >/dev/null 2>&1; then
        fix_bullmq_worker "$service_name" "MISSING_NODE_MODULES"
        fix_result=$?
        fix_description=$(get_last_fix_description)
      else
        fix_missing_node_modules "$service_name"
        fix_result=$?
        fix_description=$(get_last_fix_description)
      fi
      ;;
    BULLMQ_REDIS_CONNECTION)
      if declare -f fix_bullmq_worker >/dev/null 2>&1; then
        fix_bullmq_worker "$service_name" "REDIS_CONNECTION"
        fix_result=$?
        fix_description=$(get_last_fix_description)
      else
        fix_redis_connection
        fix_result=$?
        fix_description=$(get_last_fix_description)
      fi
      ;;
    *)
      # Generic fix: recreate service
      docker compose stop "$service_name" >/dev/null 2>&1
      docker compose rm -f "$service_name" >/dev/null 2>&1
      fix_result=0
      fix_description="Recreated $clean_name service"
      ;;
  esac

  # Record attempt
  record_fix_attempt "$service_name" "$error_code"

  if [[ $fix_result -eq 0 ]]; then
    log_success "$fix_description, retrying..."
    return 99 # Signal retry
  else
    log_error "Fix failed"
    return 1 # Failed
  fi
}

# Run pre-checks and fix common issues before Docker
run_autofix_prechecks() {
  run_pre_checks "${1:-}"
  return $?
}
