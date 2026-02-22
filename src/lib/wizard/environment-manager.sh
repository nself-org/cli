#!/usr/bin/env bash

# environment-manager.sh - Environment management for multi-env support

# Source templates for create_environment_file function
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

CLI_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")/cli"
if [[ -f "$CLI_DIR/wizard/templates.sh" ]]; then
  source "$CLI_DIR/wizard/templates.sh"
fi

# Merge environment files with override support
merge_env_files() {
  local base_file="$1"
  local override_file="$2"
  local output_file="$3"

  # Start with base file
  cp "$base_file" "$output_file"

  # Apply overrides
  if [[ -f "$override_file" ]]; then
    while IFS='=' read -r key value; do
      # Skip comments and empty lines
      [[ "$key" =~ ^#.*$ ]] && continue
      [[ -z "$key" ]] && continue

      # Remove existing key from output
      sed -i.bak "/^$key=/d" "$output_file"

      # Add new value
      echo "$key=$value" >>"$output_file"
    done <"$override_file"

    rm -f "${output_file}.bak"
  fi
}

# Compile environment based on target
compile_environment() {
  local target="${1:-development}"
  local output=".env"

  case "$target" in
    development)
      # Development uses .env.local directly
      if [[ -f ".env.local" ]]; then
        cp .env.local "$output"
        log_success "Compiled development environment"
      else
        log_error ".env.local not found"
        return 1
      fi
      ;;

    staging)
      # Staging merges .env.staging over .env.local
      if [[ -f ".env.local" ]] && [[ -f ".env.staging" ]]; then
        merge_env_files ".env.local" ".env.staging" "$output"
        log_success "Compiled staging environment"
      else
        log_error "Missing .env.local or .env.staging"
        return 1
      fi
      ;;

    production)
      # Production merges .env.secrets over .env.prod
      if [[ -f ".env.prod" ]] && [[ -f ".env.secrets" ]]; then
        merge_env_files ".env.prod" ".env.secrets" "$output"

        # Set restrictive permissions
        chmod 600 "$output"
        log_success "Compiled production environment"
      else
        log_error "Missing .env.prod or .env.secrets"
        return 1
      fi
      ;;

    *)
      log_error "Unknown environment: $target"
      return 1
      ;;
  esac
}

# Validate environment configuration
validate_environment() {
  local env_file="${1:-.env}"
  local errors=0

  # Source the environment
  set -a
  source "$env_file"
  set +a

  # Check required variables
  local required_vars=(
    "PROJECT_NAME"
    "BASE_DOMAIN"
    "POSTGRES_DB"
    "POSTGRES_USER"
    "POSTGRES_PASSWORD"
  )

  for var in "${required_vars[@]}"; do
    if [[ -z "${!var}" ]]; then
      log_error "Missing required variable: $var"
      errors=$((errors + 1))
    fi
  done

  # Check for security issues in production
  if [[ "${ENVIRONMENT:-development}" == "production" ]]; then
    # Check for default passwords
    if [[ "$POSTGRES_PASSWORD" == "localpass123" ]]; then
      log_error "Default password detected in production!"
      errors=$((errors + 1))
    fi

    # Check SSL is enabled
    if [[ "${SSL_MODE:-none}" == "none" ]]; then
      log_error "SSL not enabled for production!"
      errors=$((errors + 1))
    fi

    # Check debug mode
    if [[ "${DEBUG:-false}" == "true" ]]; then
      log_warning "Debug mode enabled in production"
    fi
  fi

  # Check port conflicts
  local ports=(
    "${POSTGRES_PORT:-5432}"
    "${REDIS_PORT:-6379}"
    "${HASURA_PORT:-8080}"
    "${AUTH_PORT:-4000}"
    "${MINIO_PORT:-9000}"
  )

  local seen_ports=()
  for port in "${ports[@]}"; do
    if [[ " ${seen_ports[*]} " =~ " ${port} " ]]; then
      log_error "Port conflict detected: $port"
      errors=$((errors + 1))
    fi
    seen_ports+=("$port")
  done

  if [[ $errors -eq 0 ]]; then
    log_success "Environment validation passed"
    return 0
  else
    log_error "Environment validation failed with $errors errors"
    return 1
  fi
}

# Show environment information
show_environment_info() {
  local env_file="${1:-.env}"

  if [[ ! -f "$env_file" ]]; then
    log_error "Environment file not found: $env_file"
    return 1
  fi

  # Source the environment
  set -a
  source "$env_file"
  set +a

  echo "Environment Configuration"
  echo "========================"
  echo ""
  echo "Core:"
  echo "  Project:     ${PROJECT_NAME:-not set}"
  echo "  Domain:      ${BASE_DOMAIN:-not set}"
  echo "  Environment: ${ENVIRONMENT:-development}"
  echo ""

  echo "Database:"
  echo "  Type:     PostgreSQL ${POSTGRES_VERSION:-15}"
  echo "  Database: ${POSTGRES_DB:-not set}"
  echo "  User:     ${POSTGRES_USER:-not set}"
  echo "  Host:     ${POSTGRES_HOST:-postgres}"
  echo "  Port:     ${POSTGRES_PORT:-5432}"
  echo ""

  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    echo "Redis:"
    echo "  Host: ${REDIS_HOST:-redis}"
    echo "  Port: ${REDIS_PORT:-6379}"
    echo ""
  fi

  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    echo "Storage (MinIO):"
    echo "  Port:    ${MINIO_PORT:-9000}"
    echo "  Console: ${MINIO_CONSOLE_PORT:-9001}"
    echo ""
  fi

  if [[ "${SEARCH_ENABLED:-false}" == "true" ]]; then
    echo "Search:"
    echo "  Engine: ${SEARCH_ENGINE:-postgres}"
    echo "  Host:   ${SEARCH_HOST:-search}"
    echo "  Port:   ${SEARCH_PORT:-7700}"
    echo ""
  fi

  if [[ "${ADMIN_ENABLED:-false}" == "true" ]]; then
    echo "Admin UI:"
    echo "  Port:  ${ADMIN_PORT:-3021}"
    echo "  Route: ${ADMIN_ROUTE:-admin.\${BASE_DOMAIN}}"
    echo ""
  fi

  echo "SSL:"
  echo "  Mode:  ${SSL_MODE:-local}"
  echo "  Email: ${SSL_EMAIL:-not set}"
}

# Create environment files for all environments
create_all_environments() {
  local project_name="$1"
  local base_domain="$2"

  log_info "Creating environment files..."

  # Development (.env.local)
  if [[ ! -f ".env.local" ]]; then
    create_environment_file "development" "$project_name" "$base_domain"
    log_success "Created .env.local"
  else
    log_info ".env.local already exists, skipping"
  fi

  # Staging (.env.staging)
  if [[ ! -f ".env.staging" ]]; then
    create_environment_file "staging" "$project_name" "$base_domain"
    log_success "Created .env.staging"
  else
    log_info ".env.staging already exists, skipping"
  fi

  # Production (.env.prod and .env.secrets)
  if [[ ! -f ".env.prod" ]]; then
    create_environment_file "production" "$project_name" "$base_domain"
    log_success "Created .env.prod and .env.secrets"
  else
    log_info ".env.prod already exists, skipping"
  fi
}

# Export functions
export -f merge_env_files
export -f compile_environment
export -f validate_environment
export -f show_environment_info
export -f create_all_environments
