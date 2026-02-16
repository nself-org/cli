#!/usr/bin/env bash

# config-validator.sh - Comprehensive configuration validation for build

# Source display utilities (namespaced to avoid clobbering caller's SCRIPT_DIR)
_BUILD_VALIDATOR_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

if [[ -f "$_BUILD_VALIDATOR_DIR/../utils/display.sh" ]]; then
  source "$_BUILD_VALIDATOR_DIR/../utils/display.sh"
fi

# Validate environment file syntax
validate_env_syntax() {
  local env_file="${1:-.env}"
  local issues=()
  local line_num=0

  if [[ ! -f "$env_file" ]]; then
    return 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    ((line_num++))

    # Skip empty lines and comments
    [[ -z "$line" ]] && continue
    [[ "$line" =~ ^[[:space:]]*# ]] && continue

    # Check for valid KEY=VALUE format
    if [[ ! "$line" =~ ^[A-Z_][A-Z0-9_]*= ]]; then
      if [[ "$line" =~ ^[[:space:]] ]]; then
        issues+=("Line $line_num: Leading whitespace found")
      elif [[ "$line" =~ = ]]; then
        issues+=("Line $line_num: Invalid variable name format")
      fi
      continue
    fi

    # Extract key and value
    local key="${line%%=*}"
    local value="${line#*=}"

    # Check for unquoted values with spaces
    if [[ "$value" =~ [[:space:]] ]]; then
      # Check if it's properly quoted
      if [[ ! "$value" =~ ^\".*\"$ ]] && [[ ! "$value" =~ ^\'.*\'$ ]]; then
        issues+=("Line $line_num: $key has unquoted value with spaces")
      fi
    fi

    # Check for special characters that need quoting
    if [[ "$value" =~ [#\$\`\\] ]]; then
      if [[ ! "$value" =~ ^\".*\"$ ]] && [[ ! "$value" =~ ^\'.*\'$ ]]; then
        issues+=("Line $line_num: $key contains special characters that should be quoted")
      fi
    fi
  done <"$env_file"

  if [[ ${#issues[@]} -gt 0 ]]; then
    echo "Environment file syntax issues:"
    for issue in "${issues[@]}"; do
      echo "  ⚠ $issue"
    done
    return 1
  fi

  return 0
}

# Validate required services configuration
validate_service_config() {
  local missing_configs=()

  # Check Postgres configuration if enabled
  if [[ "${POSTGRES_ENABLED:-true}" == "true" ]]; then
    [[ -z "${POSTGRES_USER:-}" ]] && missing_configs+=("POSTGRES_USER")
    [[ -z "${POSTGRES_PASSWORD:-}" ]] && missing_configs+=("POSTGRES_PASSWORD")
    [[ -z "${POSTGRES_DB:-}" ]] && missing_configs+=("POSTGRES_DB")
  fi

  # Check Hasura configuration if enabled
  if [[ "${HASURA_ENABLED:-false}" == "true" ]]; then
    [[ -z "${HASURA_GRAPHQL_ADMIN_SECRET:-}" ]] && missing_configs+=("HASURA_GRAPHQL_ADMIN_SECRET")
    [[ -z "${DATABASE_URL:-}" ]] && missing_configs+=("DATABASE_URL")

    # Validate DATABASE_URL format
    if [[ -n "${DATABASE_URL:-}" ]]; then
      if [[ ! "$DATABASE_URL" =~ ^postgres(ql)?:// ]]; then
        echo "  ⚠ DATABASE_URL should start with postgres:// or postgresql://"
      fi
    fi
  fi

  # Check Auth configuration if enabled
  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    [[ -z "${AUTH_JWT_SECRET:-}" ]] && missing_configs+=("AUTH_JWT_SECRET")

    # Check JWT secret length
    if [[ -n "${AUTH_JWT_SECRET:-}" ]] && [[ ${#AUTH_JWT_SECRET} -lt 32 ]]; then
      echo "  ⚠ AUTH_JWT_SECRET should be at least 32 characters"
    fi
  fi

  # Check MinIO configuration if enabled
  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    [[ -z "${MINIO_ROOT_USER:-}" ]] && missing_configs+=("MINIO_ROOT_USER")
    [[ -z "${MINIO_ROOT_PASSWORD:-}" ]] && missing_configs+=("MINIO_ROOT_PASSWORD")

    # Check MinIO password requirements
    if [[ -n "${MINIO_ROOT_PASSWORD:-}" ]] && [[ ${#MINIO_ROOT_PASSWORD} -lt 8 ]]; then
      echo "  ⚠ MINIO_ROOT_PASSWORD should be at least 8 characters"
    fi
  fi

  if [[ ${#missing_configs[@]} -gt 0 ]]; then
    echo "Missing required configurations:"
    for config in "${missing_configs[@]}"; do
      echo "  ✗ $config"
    done
    return 1
  fi

  return 0
}

# Validate consistency between env and docker-compose
validate_compose_consistency() {
  local compose_file="docker-compose.yml"
  local inconsistencies=()

  if [[ ! -f "$compose_file" ]]; then
    # No compose file yet, that's OK during build
    return 0
  fi

  # Check if services in env match what's in docker-compose
  local enabled_services=()

  [[ "${POSTGRES_ENABLED:-true}" == "true" ]] && enabled_services+=("postgres")
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && enabled_services+=("redis")
  [[ "${HASURA_ENABLED:-false}" == "true" ]] && enabled_services+=("hasura")
  [[ "${AUTH_ENABLED:-false}" == "true" ]] && enabled_services+=("auth")
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && enabled_services+=("minio")
  [[ "${MAILPIT_ENABLED:-false}" == "true" ]] && enabled_services+=("mailpit")
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && enabled_services+=("redis")
  [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]] && enabled_services+=("functions")
  [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]] && enabled_services+=("nself-admin")
  [[ "${MLFLOW_ENABLED:-false}" == "true" ]] && enabled_services+=("mlflow")

  # Check each enabled service exists in docker-compose
  for service in "${enabled_services[@]}"; do
    if ! grep -q "^  $service:" "$compose_file" 2>/dev/null; then
      inconsistencies+=("Service $service is enabled but not in docker-compose.yml")
    fi
  done

  # Check for orphaned services (in compose but not enabled)
  local compose_services=$(grep "^  [a-z_-]*:" "$compose_file" 2>/dev/null | sed 's/^  //;s/://' | sort -u)

  for service in $compose_services; do
    local var_name="$(echo "$service" | tr '[:lower:]' '[:upper:]')_ENABLED"
    var_name="${var_name//-/_}"

    # Skip core services that don't have enable flags
    if [[ "$service" == "nginx" ]] || [[ "$service" == "prometheus" ]] || [[ "$service" == "grafana" ]] || [[ "$service" == "loki" ]]; then
      continue
    fi

    # Check if service should be enabled
    local enabled_value="${!var_name:-false}"
    if [[ "$enabled_value" != "true" ]] && [[ "$service" != "postgres" ]]; then
      # postgres is enabled by default
      if [[ "$service" == "postgres" ]] && [[ "${POSTGRES_ENABLED:-true}" != "true" ]]; then
        inconsistencies+=("Service $service in docker-compose.yml but POSTGRES_ENABLED is false")
      elif [[ "$service" != "postgres" ]]; then
        inconsistencies+=("Service $service in docker-compose.yml but ${var_name} is not true")
      fi
    fi
  done

  if [[ ${#inconsistencies[@]} -gt 0 ]]; then
    echo "Configuration inconsistencies detected:"
    for issue in "${inconsistencies[@]}"; do
      echo "  ⚠ $issue"
    done
    return 1
  fi

  return 0
}

# Check for required directories and files
validate_project_structure() {
  local missing_dirs=()
  local missing_files=()

  # Check if nginx is enabled and config exists
  if [[ "${NGINX_ENABLED:-true}" == "true" ]] && [[ -f "docker-compose.yml" ]]; then
    [[ ! -d "nginx" ]] && missing_dirs+=("nginx/")
    [[ ! -f "nginx/nginx.conf" ]] && missing_files+=("nginx/nginx.conf")
  fi

  # Check SSL certificates if SSL is enabled
  if [[ "${SSL_ENABLED:-false}" == "true" ]] && [[ -f "docker-compose.yml" ]]; then
    [[ ! -d "ssl" ]] && missing_dirs+=("ssl/")
    if [[ -d "ssl" ]]; then
      local cert_count=$(ls -1 ssl/*.crt ssl/*.pem 2>/dev/null | wc -l)
      [[ $cert_count -eq 0 ]] && missing_files+=("SSL certificates")
    fi
  fi

  # Check for database init if postgres is enabled
  if [[ "${POSTGRES_ENABLED:-true}" == "true" ]] && [[ -f "docker-compose.yml" ]]; then
    [[ ! -d "postgres/init" ]] && missing_dirs+=("postgres/init/")
  fi

  if [[ ${#missing_dirs[@]} -gt 0 ]] || [[ ${#missing_files[@]} -gt 0 ]]; then
    echo "Missing project structure:"
    for dir in "${missing_dirs[@]}"; do
      echo "  ✗ Directory: $dir"
    done
    for file in "${missing_files[@]}"; do
      echo "  ✗ File: $file"
    done
    return 1
  fi

  return 0
}

# Main validation function
validate_build_config() {
  local validation_passed=true

  echo "Validating configuration..."
  echo

  # 1. Check env file syntax
  if ! validate_env_syntax; then
    validation_passed=false
    echo
  fi

  # 2. Check service configuration
  if ! validate_service_config; then
    validation_passed=false
    echo
  fi

  # 3. Check compose consistency (only if compose exists)
  if [[ -f "docker-compose.yml" ]]; then
    if ! validate_compose_consistency; then
      validation_passed=false
      echo
    fi

    # 4. Check project structure
    if ! validate_project_structure; then
      validation_passed=false
      echo
    fi
  fi

  if [[ "$validation_passed" == true ]]; then
    return 0
  else
    echo "Fix the issues above and run 'nself build' again"
    return 1
  fi
}

# Export functions
export -f validate_env_syntax
export -f validate_service_config
export -f validate_compose_consistency
export -f validate_project_structure
export -f validate_build_config
