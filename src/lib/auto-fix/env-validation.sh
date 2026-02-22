#!/usr/bin/env bash

# env-validation.sh - Comprehensive environment variable validation with auto-fix

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true

# Validate and fix PROJECT_NAME
validate_project_name() {
  local name="$1"
  local fixed_name=""

  # Check if empty
  if [[ -z "$name" ]]; then
    log_error "PROJECT_NAME is empty"
    fixed_name="my-project"
    return 1
  fi

  # Remove invalid characters and convert to lowercase
  fixed_name=$(echo "$name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//')

  # Ensure it starts with a letter
  if [[ ! "$fixed_name" =~ ^[a-z] ]]; then
    fixed_name="project-$fixed_name"
  fi

  # Limit length to 30 characters
  if [[ ${#fixed_name} -gt 30 ]]; then
    fixed_name="${fixed_name:0:30}"
  fi

  if [[ "$name" != "$fixed_name" ]]; then
    log_warning "PROJECT_NAME '$name' is invalid"
    echo "$fixed_name"
    return 1
  fi

  return 0
}

# Validate and fix port number
validate_port() {
  local port="$1"
  local service="$2"
  local default_port="$3"

  # Check if empty or invalid
  if [[ -z "$port" ]] || ! [[ "$port" =~ ^[0-9]+$ ]] || [[ $port -lt 1 ]] || [[ $port -gt 65535 ]]; then
    log_warning "${service}_PORT '$port' is invalid, using default $default_port"
    echo "$default_port"
    return 1
  fi

  # Check if port is in use
  if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
    # Find next available port
    local new_port=$((port + 1))
    while [[ $new_port -le 65535 ]] && lsof -Pi :$new_port -sTCP:LISTEN -t >/dev/null 2>&1; do
      new_port=$((new_port + 1))
    done

    if [[ $new_port -le 65535 ]]; then
      log_warning "${service}_PORT $port is in use, using $new_port instead"
      echo "$new_port"
      return 1
    fi
  fi

  echo "$port"
  return 0
}

# Validate and fix domain
validate_domain() {
  local domain="$1"

  # Check if empty
  if [[ -z "$domain" ]]; then
    log_warning "BASE_DOMAIN is empty, using default"
    echo "local.nself.org"
    return 1
  fi

  # Remove invalid characters
  local fixed_domain=$(echo "$domain" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9.-]//g')

  # Ensure it's a valid domain format
  if ! [[ "$fixed_domain" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$ ]]; then
    log_warning "BASE_DOMAIN '$domain' is invalid, using default"
    echo "local.nself.org"
    return 1
  fi

  if [[ "$domain" != "$fixed_domain" ]]; then
    log_warning "BASE_DOMAIN '$domain' contains invalid characters"
    echo "$fixed_domain"
    return 1
  fi

  echo "$domain"
  return 0
}

# Validate and fix environment type
validate_env_type() {
  local env="$1"
  local valid_envs=("dev" "staging" "prod" "test")

  # Convert to lowercase
  env=$(echo "$env" | tr '[:upper:]' '[:lower:]')

  # Check if valid
  for valid in "${valid_envs[@]}"; do
    if [[ "$env" == "$valid" ]]; then
      echo "$env"
      return 0
    fi
  done

  log_warning "ENV '$env' is invalid, using 'dev'"
  echo "dev"
  return 1
}

# Validate and fix database name
validate_db_name() {
  local name="$1"

  # Check if empty
  if [[ -z "$name" ]]; then
    echo "nhost"
    return 0
  fi

  # Remove invalid characters and convert to lowercase
  local fixed_name=$(echo "$name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_]/_/g')

  # Ensure it starts with a letter
  if [[ ! "$fixed_name" =~ ^[a-z] ]]; then
    fixed_name="db_$fixed_name"
  fi

  if [[ "$name" != "$fixed_name" ]]; then
    log_warning "POSTGRES_DB '$name' contains invalid characters"
    echo "$fixed_name"
    return 1
  fi

  echo "$name"
  return 0
}

# Validate and fix username
validate_username() {
  local name="$1"
  local service="$2"

  # Check if empty
  if [[ -z "$name" ]]; then
    echo "postgres"
    return 0
  fi

  # Remove invalid characters
  local fixed_name=$(echo "$name" | sed 's/[^a-zA-Z0-9_]//g')

  # Ensure it starts with a letter
  if [[ ! "$fixed_name" =~ ^[a-zA-Z] ]]; then
    fixed_name="user_$fixed_name"
  fi

  if [[ "$name" != "$fixed_name" ]]; then
    log_warning "${service}_USER '$name' contains invalid characters"
    echo "$fixed_name"
    return 1
  fi

  echo "$name"
  return 0
}

# Auto-fix environment file
auto_fix_env_file() {
  local env_file="${1:-.env}"
  local timestamp="$(date +%Y%m%d_%H%M%S)"
  local backup_dir="_backup/${timestamp}"
  local temp_file="${env_file}.tmp"
  local changes_made=0

  if [[ ! -f "$env_file" ]]; then
    log_error "Environment file not found: $env_file"
    return 1
  fi

  # Create backup directory and backup file
  mkdir -p "$backup_dir"
  local backup_file="${backup_dir}/$(basename "$env_file")"
  cp "$env_file" "$backup_file"
  log_info "Created backup: $backup_file"

  # Read and process each line
  >"$temp_file"
  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip comments and empty lines
    if [[ "$line" =~ ^[[:space:]]*# ]] || [[ -z "$line" ]]; then
      echo "$line" >>"$temp_file"
      continue
    fi

    # Parse key=value
    if [[ "$line" =~ ^([A-Z_]+)=(.*)$ ]]; then
      key="${BASH_REMATCH[1]}"
      value="${BASH_REMATCH[2]}"
      new_value="$value"

      # Validate and fix based on key
      case "$key" in
        PROJECT_NAME)
          if new_val=$(validate_project_name "$value"); then
            :
          else
            new_value="$new_val"
            changes_made=1
          fi
          ;;

        POSTGRES_PORT)
          new_value=$(validate_port "$value" "POSTGRES" "5432")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        HASURA_PORT)
          new_value=$(validate_port "$value" "HASURA" "8080")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        AUTH_PORT)
          new_value=$(validate_port "$value" "AUTH" "4000")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        STORAGE_PORT)
          new_value=$(validate_port "$value" "STORAGE" "5001")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        REDIS_PORT)
          new_value=$(validate_port "$value" "REDIS" "6379")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        PROMETHEUS_PORT)
          new_value=$(validate_port "$value" "PROMETHEUS" "9090")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        GRAFANA_PORT)
          new_value=$(validate_port "$value" "GRAFANA" "3000")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        LOKI_PORT)
          new_value=$(validate_port "$value" "LOKI" "3100")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        MAILPIT_PORT)
          new_value=$(validate_port "$value" "MAILPIT" "1025")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        BASE_DOMAIN)
          new_value=$(validate_domain "$value")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        ENV)
          new_value=$(validate_env_type "$value")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        POSTGRES_DB)
          new_value=$(validate_db_name "$value")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;

        POSTGRES_USER)
          new_value=$(validate_username "$value" "POSTGRES")
          [[ "$value" != "$new_value" ]] && changes_made=1
          ;;
      esac

      # Write the line (fixed or original)
      echo "${key}=${new_value}" >>"$temp_file"
    else
      # Unknown format, keep as is
      echo "$line" >>"$temp_file"
    fi
  done <"$env_file"

  # Apply changes if any were made
  if [[ $changes_made -eq 1 ]]; then
    mv "$temp_file" "$env_file"
    log_success "Fixed $changes_made validation issue(s) in $env_file"
    log_info "Original file backed up to: $backup_file"
    return 0
  else
    rm "$temp_file"
    rm "$backup_file" # Remove backup if no changes
    log_success "All environment variables are valid"
    return 0
  fi
}

# Validate all environment files
validate_all_env() {
  local errors=0

  # Check for .env or .env.local
  if [[ -f ".env" ]]; then
    log_info "Validating .env..."
    if ! auto_fix_env_file ".env"; then
      errors=$((errors + 1))
    fi
  fi

  if [[ -f ".env.local" ]]; then
    log_info "Validating .env.local..."
    if ! auto_fix_env_file ".env.local"; then
      errors=$((errors + 1))
    fi
  fi

  # Check for environment-specific files
  for env_file in .env.dev .env.staging .env.prod; do
    if [[ -f "$env_file" ]]; then
      log_info "Validating $env_file..."
      if ! auto_fix_env_file "$env_file"; then
        errors=$((errors + 1))
      fi
    fi
  done

  return $errors
}

# Export functions if sourced
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
  export -f validate_project_name
  export -f validate_port
  export -f validate_domain
  export -f validate_env_type
  export -f validate_db_name
  export -f validate_username
  export -f auto_fix_env_file
  export -f validate_all_env
fi
