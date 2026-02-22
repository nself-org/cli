#!/usr/bin/env bash

# validation.sh - Environment validation for build

# Source auto-fix utilities
VALIDATION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$VALIDATION_DIR/../auto-fix/env-quotes-fix.sh" ]]; then
  source "$VALIDATION_DIR/../auto-fix/env-quotes-fix.sh"
fi

# Source output utilities for show_warning, show_error, show_info
if [[ -f "$VALIDATION_DIR/output.sh" ]]; then
  source "$VALIDATION_DIR/output.sh"
fi

# Source platform utilities for set_default and other functions
if [[ -f "$VALIDATION_DIR/platform.sh" ]]; then
  source "$VALIDATION_DIR/platform.sh"
fi

# Validate environment configuration
validate_environment() {
  local validation_passed=true
  local errors=()
  local warnings=()
  local fixes=()

  # Fix unquoted values with spaces first (before loading)
  if command -v auto_fix_env_quotes >/dev/null 2>&1; then
    auto_fix_env_quotes
  fi

  # Check required variables
  if [[ -z "${PROJECT_NAME:-}" ]]; then
    PROJECT_NAME="$(basename "$PWD")"
    fixes+=("Set PROJECT_NAME to '$PROJECT_NAME'")
    export PROJECT_NAME
  fi

  if [[ -z "${BASE_DOMAIN:-}" ]]; then
    BASE_DOMAIN="localhost"
    fixes+=("Set BASE_DOMAIN to 'localhost'")
    export BASE_DOMAIN
  fi

  # Validate PROJECT_NAME format (Bash 3.2 compatible)
  # Docker and DNS require lowercase alphanumeric with hyphens only
  # Must start and end with alphanumeric, can contain hyphens in the middle
  local original_project_name="$PROJECT_NAME"

  # First, convert to lowercase and replace invalid characters
  PROJECT_NAME=$(echo "$PROJECT_NAME" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g')

  # Remove leading/trailing hyphens and collapse multiple hyphens
  PROJECT_NAME=$(echo "$PROJECT_NAME" | sed 's/^-*//;s/-*$//' | sed 's/--*/-/g')

  # Ensure it starts with a letter or number (prepend 'app' if needed)
  if ! echo "$PROJECT_NAME" | grep -q '^[a-z0-9]'; then
    PROJECT_NAME="app-$PROJECT_NAME"
  fi

  # Ensure it ends with a letter or number (append '1' if needed)
  if ! echo "$PROJECT_NAME" | grep -q '[a-z0-9]$'; then
    PROJECT_NAME="${PROJECT_NAME}1"
  fi

  # Ensure minimum length (Docker requires at least 2 chars)
  if [ ${#PROJECT_NAME} -lt 2 ]; then
    PROJECT_NAME="app-${PROJECT_NAME}"
  fi

  # Truncate if too long (Docker has a 63 char limit for container names)
  if [ ${#PROJECT_NAME} -gt 30 ]; then
    PROJECT_NAME="${PROJECT_NAME:0:30}"
    # Ensure it still ends with alphanumeric after truncation
    PROJECT_NAME=$(echo "$PROJECT_NAME" | sed 's/-*$//')
  fi

  # Final validation
  if ! echo "$PROJECT_NAME" | grep -q '^[a-z0-9][a-z0-9-]*[a-z0-9]$'; then
    # Fallback to a safe default
    PROJECT_NAME="myproject"
    warnings+=("PROJECT_NAME '$original_project_name' was invalid, using default 'myproject'")
  elif [[ "$PROJECT_NAME" != "$original_project_name" ]]; then
    fixes+=("Fixed PROJECT_NAME from '$original_project_name' to '$PROJECT_NAME'")
  fi

  export PROJECT_NAME

  # Validate domain format
  if [[ "$BASE_DOMAIN" == *" "* ]]; then
    BASE_DOMAIN="${BASE_DOMAIN// /}"
    fixes+=("Removed spaces from BASE_DOMAIN")
    export BASE_DOMAIN
  fi

  # Skip port conflicts check during build (runtime concern)
  # Port availability can change between build and start

  # Validate boolean values
  validate_boolean_vars

  # Validate custom services
  validate_custom_services

  # Apply fixes to .env if needed
  if [[ ${#fixes[@]} -gt 0 ]]; then
    apply_validation_fixes
  fi

  # Show validation results
  if [[ ${#errors[@]} -gt 0 ]]; then
    for error in "${errors[@]}"; do
      show_error "$error"
    done
    validation_passed=false
  fi

  if [[ ${#warnings[@]} -gt 0 ]]; then
    for warning in "${warnings[@]}"; do
      show_warning "$warning"
    done
  fi

  if [[ ${#fixes[@]} -gt 0 ]]; then
    show_info "Applied ${#fixes[@]} automatic fixes"
  fi

  [[ "$validation_passed" == true ]]
}

# Check for port conflicts
check_port_conflicts() {
  local ports_to_check=(
    "NGINX_PORT:80"
    "NGINX_SSL_PORT:443"
    "POSTGRES_PORT:5432"
    "HASURA_PORT:8080"
    "AUTH_PORT:4000"
    "STORAGE_PORT:5000"
    "REDIS_PORT:6379"
  )

  for port_var in "${ports_to_check[@]}"; do
    local var_name="${port_var%:*}"
    local default_port="${port_var#*:}"
    # Use eval for Bash 3.2 compatibility
    eval "local port=\${$var_name:-$default_port}"

    # Check if port is in use (3s timeout to avoid hanging on NFS/macOS issues)
    if command -v lsof >/dev/null 2>&1; then
      if timeout 3 lsof -Pi :"$port" -t >/dev/null 2>&1; then
        warnings+=("Port $port (${var_name}) is already in use")
      fi
    fi
  done
}

# Validate CS_N custom service format
validate_custom_services() {
  local used_ports=()
  local service_names=()

  # Collect ports from core services
  used_ports+=("${NGINX_PORT:-80}")
  used_ports+=("${NGINX_SSL_PORT:-443}")
  used_ports+=("${POSTGRES_PORT:-5432}")
  used_ports+=("${HASURA_PORT:-8080}")
  used_ports+=("${AUTH_PORT:-4000}")
  used_ports+=("${REDIS_PORT:-6379}")
  used_ports+=("${MINIO_PORT:-9000}")
  used_ports+=("${MINIO_CONSOLE_PORT:-9001}")

  # Validate CS_N variables
  for i in {1..20}; do
    local cs_var="CS_${i}"
    local cs_value="${!cs_var:-}"

    [[ -z "$cs_value" ]] && continue

    # Parse CS format: service_name:template_type:port
    IFS=':' read -r service_name template_type port <<<"$cs_value"

    # Validate format
    if [[ -z "$service_name" || -z "$template_type" ]]; then
      errors+=("CS_${i} has invalid format. Expected: service_name:template_type:port")
      continue
    fi

    # Validate service name format (lowercase, alphanumeric with underscores)
    if ! echo "$service_name" | grep -q '^[a-z][a-z0-9_]*$'; then
      errors+=("CS_${i} service name '$service_name' must start with lowercase letter and contain only lowercase letters, numbers, and underscores")
    fi

    # Check for duplicate service names
    if [[ ${#service_names[@]} -gt 0 ]] && [[ " ${service_names[*]} " =~ " ${service_name} " ]]; then
      errors+=("CS_${i} duplicate service name: $service_name")
    else
      service_names+=("$service_name")
    fi

    # Validate port if specified
    if [[ -n "$port" && "$port" != "0" ]]; then
      # Check if port is a valid number
      if ! [[ "$port" =~ ^[0-9]+$ ]] || [[ "$port" -lt 1 || "$port" -gt 65535 ]]; then
        errors+=("CS_${i} invalid port: $port (must be 1-65535)")
      fi

      # Check for port conflicts
      if [[ ${#used_ports[@]} -gt 0 ]] && [[ " ${used_ports[*]} " =~ " ${port} " ]]; then
        warnings+=("CS_${i} port $port is already in use by another service")
      else
        used_ports+=("$port")
      fi
    fi

    # Validate template type exists
    local template_base="${NSELF_TEMPLATES:-${NSELF_ROOT:-/usr/local/lib/nself}/src/templates}/services"
    local template_found=false

    for lang_dir in "$template_base"/*; do
      if [[ -d "$lang_dir/$template_type" ]]; then
        template_found=true
        break
      fi
    done

    if [[ "$template_found" != "true" ]]; then
      warnings+=("CS_${i} template '$template_type' not found in service templates")
    fi
  done
}

# Validate boolean variables
validate_boolean_vars() {
  local bool_vars=(
    "SSL_ENABLED"
    "NGINX_ENABLED"
    "POSTGRES_ENABLED"
    "HASURA_ENABLED"
    "AUTH_ENABLED"
    "STORAGE_ENABLED"
    "REDIS_ENABLED"
    "FUNCTIONS_ENABLED"
    "NESTJS_ENABLED"
    "NSELF_ADMIN_ENABLED"
  )

  for var in "${bool_vars[@]}"; do
    # Use eval for Bash 3.2 compatibility
    eval "local value=\${$var:-}"
    if [[ -n "$value" ]] && [[ "$value" != "true" ]] && [[ "$value" != "false" ]]; then
      # Convert common boolean representations
      local value_lower=$(echo "$value" | tr '[:upper:]' '[:lower:]')
      case "$value_lower" in
        1 | yes | y | on | enabled)
          eval "$var=true"
          fixes+=("Fixed $var to 'true'")
          ;;
        0 | no | n | off | disabled)
          eval "$var=false"
          fixes+=("Fixed $var to 'false'")
          ;;
        *)
          eval "$var=false"
          warnings+=("Invalid boolean value for $var: '$value' (set to 'false')")
          ;;
      esac
    fi
  done
}

# Apply validation fixes to .env files
apply_validation_fixes() {
  local env_file=".env"

  # Determine which env file to update
  if [[ -f ".env.local" ]]; then
    env_file=".env.local"
  elif [[ -f ".env.${ENV:-dev}" ]]; then
    env_file=".env.${ENV:-dev}"
  fi

  # If no env file exists, nothing to fix
  if [[ ! -f "$env_file" ]]; then
    return 0
  fi

  # Backup the file to _backup/timestamp structure
  local timestamp="$(date +%Y%m%d_%H%M%S)"
  local backup_dir="_backup/${timestamp}"
  mkdir -p "$backup_dir"
  cp "$env_file" "${backup_dir}/$(basename "$env_file")" 2>/dev/null || true

  # Apply fixes (carefully)
  local temp_file=$(mktemp)

  # Read the original file and apply fixes
  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip empty lines and comments (Bash 3.2 compatible)
    if [[ -z "$line" ]] || echo "$line" | grep -q '^[[:space:]]*#'; then
      echo "$line" >>"$temp_file"
      continue
    fi

    # Parse key=value pairs (Bash 3.2 compatible)
    if echo "$line" | grep -q '^[A-Z_][A-Z_]*='; then
      local key=$(echo "$line" | cut -d'=' -f1)
      local value=$(echo "$line" | cut -d'=' -f2-)

      # Check if we have a new value for this key
      # Use eval for Bash 3.2 compatibility
      eval "local new_value=\${$key:-}"
      if [[ -n "$new_value" ]]; then
        echo "${key}=${new_value}" >>"$temp_file"
      else
        echo "$line" >>"$temp_file"
      fi
    else
      echo "$line" >>"$temp_file"
    fi
  done <"$env_file"

  # Add any new variables that weren't in the file (Bash 3.2 compatible)
  for fix in "${fixes[@]}"; do
    if echo "$fix" | grep -q 'Set [A-Z_][A-Z_]* to'; then
      local key=$(echo "$fix" | sed 's/Set \([A-Z_][A-Z_]*\) to.*/\1/')
      if ! grep -q "^${key}=" "$temp_file"; then
        eval "local new_val=\${$key:-}"
        echo "${key}=${new_val}" >>"$temp_file"
      fi
    fi
  done

  # Move temp file to original
  mv "$temp_file" "$env_file"
}

# Validate service dependencies
validate_service_dependencies() {
  # Use eval for Bash 3.2 compatibility with indirect variable references
  # Check Hasura dependencies
  if [[ "${HASURA_ENABLED:-false}" == "true" ]]; then
    if [[ "${POSTGRES_ENABLED:-false}" != "true" ]]; then
      POSTGRES_ENABLED=true
      fixes+=("Enabled PostgreSQL (required by Hasura)")
    fi
  fi

  # Check Auth dependencies
  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    if [[ "${POSTGRES_ENABLED:-false}" != "true" ]]; then
      POSTGRES_ENABLED=true
      fixes+=("Enabled PostgreSQL (required by Auth)")
    fi
  fi

  # MinIO is self-contained and doesn't require other services
  # No dependency checks needed
}

# Export functions
export -f validate_environment
export -f check_port_conflicts
export -f validate_boolean_vars
export -f validate_custom_services
export -f apply_validation_fixes
export -f validate_service_dependencies
