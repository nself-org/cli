#!/usr/bin/env bash


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${SCRIPT_DIR}/../utils/display.sh"
source "${SCRIPT_DIR}/../utils/output-formatter.sh"

VALIDATION_ERRORS=()
VALIDATION_WARNINGS=()
AUTO_FIXES=()

validate_project_name() {
  local name="$1"

  if [[ -z "$name" ]]; then
    VALIDATION_ERRORS+=("PROJECT_NAME is required but not set")
    AUTO_FIXES+=("set_default_project_name")
    return 1
  fi

  if [[ "$name" =~ [[:space:]] ]]; then
    VALIDATION_ERRORS+=("PROJECT_NAME contains spaces: '$name'")
    AUTO_FIXES+=("fix_project_name_spaces:$name")
    return 1
  fi

  if [[ "$name" =~ [^a-zA-Z0-9-_] ]]; then
    VALIDATION_ERRORS+=("PROJECT_NAME contains invalid characters: '$name'")
    AUTO_FIXES+=("fix_project_name_chars:$name")
    return 1
  fi

  if [[ ${#name} -gt 50 ]]; then
    VALIDATION_WARNINGS+=("PROJECT_NAME is very long (${#name} chars): '$name'")
    AUTO_FIXES+=("truncate_project_name:$name")
    return 2
  fi

  if [[ "$name" =~ ^[0-9] ]]; then
    VALIDATION_ERRORS+=("PROJECT_NAME cannot start with a number: '$name'")
    AUTO_FIXES+=("fix_project_name_start:$name")
    return 1
  fi

  return 0
}

validate_password() {
  local var_name="$1"
  local password="$2"
  local min_length="${3:-8}"

  if [[ -z "$password" ]]; then
    VALIDATION_ERRORS+=("$var_name is required but not set")
    AUTO_FIXES+=("generate_password:$var_name:$min_length")
    return 1
  fi

  if [[ ${#password} -lt $min_length ]]; then
    VALIDATION_ERRORS+=("$var_name is too short (${#password} chars, minimum $min_length)")
    AUTO_FIXES+=("extend_password:$var_name:$password:$min_length")
    return 1
  fi

  if [[ "$password" =~ [\'\"] ]]; then
    VALIDATION_WARNINGS+=("$var_name contains quotes which may cause issues")
    AUTO_FIXES+=("escape_password_quotes:$var_name:$password")
    return 2
  fi

  if [[ "$password" == "password" ]] || [[ "$password" == "123456" ]] || [[ "$password" == "admin" ]]; then
    VALIDATION_ERRORS+=("$var_name uses a weak/common password")
    AUTO_FIXES+=("replace_weak_password:$var_name")
    return 1
  fi

  return 0
}

validate_jwt_key() {
  local key="$1"

  if [[ -z "$key" ]]; then
    VALIDATION_ERRORS+=("HASURA_JWT_KEY is required but not set")
    AUTO_FIXES+=("generate_jwt_key")
    return 1
  fi

  if [[ ${#key} -lt 32 ]]; then
    VALIDATION_ERRORS+=("HASURA_JWT_KEY is too short (${#key} chars, minimum 32)")
    AUTO_FIXES+=("extend_jwt_key:$key")
    return 1
  fi

  return 0
}

validate_boolean() {
  local var_name="$1"
  local value="$2"

  if [[ -z "$value" ]]; then
    return 0
  fi

  local lower_value=$(echo "$value" | tr '[:upper:]' '[:lower:]')

  if [[ "$lower_value" != "true" ]] && [[ "$lower_value" != "false" ]]; then
    VALIDATION_ERRORS+=("$var_name has invalid boolean value: '$value'")
    AUTO_FIXES+=("fix_boolean:$var_name:$value")
    return 1
  fi

  if [[ "$value" != "$lower_value" ]]; then
    VALIDATION_WARNINGS+=("$var_name uses mixed case: '$value' (will be normalized)")
    AUTO_FIXES+=("normalize_boolean:$var_name:$value")
    return 2
  fi

  return 0
}

validate_service_list() {
  local var_name="$1"
  local services="$2"
  local service_type="$3"

  if [[ -z "$services" ]]; then
    return 0
  fi

  if [[ "$services" =~ ^, ]] || [[ "$services" =~ ,$ ]]; then
    VALIDATION_ERRORS+=("$var_name has leading/trailing commas: '$services'")
    AUTO_FIXES+=("fix_service_commas:$var_name:$services")
    return 1
  fi

  if [[ "$services" =~ ,, ]]; then
    VALIDATION_ERRORS+=("$var_name has empty values (consecutive commas): '$services'")
    AUTO_FIXES+=("fix_service_empty:$var_name:$services")
    return 1
  fi

  if [[ "$services" =~ [[:space:]] ]]; then
    VALIDATION_WARNINGS+=("$var_name contains spaces: '$services'")
    AUTO_FIXES+=("remove_service_spaces:$var_name:$services")
  fi

  IFS=',' read -r -a SERVICE_ARRAY <<<"$services"
  local seen=()

  for service in "${SERVICE_ARRAY[@]}"; do
    service=$(echo "$service" | tr -d ' ')

    if [[ "$service" =~ - ]]; then
      VALIDATION_ERRORS+=("Service name contains hyphen (use underscore): '$service'")
      AUTO_FIXES+=("fix_service_hyphen:$var_name:$service")
    fi

    if [[ "$service" =~ ^[0-9]+$ ]]; then
      VALIDATION_ERRORS+=("Service name is numeric only: '$service'")
      AUTO_FIXES+=("fix_numeric_service:$var_name:$service:$service_type")
    fi

    if [[ "$service" =~ [^a-zA-Z0-9_-] ]]; then
      VALIDATION_ERRORS+=("Service name contains invalid characters: '$service'")
      AUTO_FIXES+=("fix_service_chars:$var_name:$service")
    fi

    for s in "${seen[@]}"; do
      if [[ "$s" == "$service" ]]; then
        VALIDATION_ERRORS+=("Duplicate service name: '$service'")
        AUTO_FIXES+=("remove_duplicate_service:$var_name:$service")
        break
      fi
    done
    seen+=("$service")
  done

  return 0
}

validate_postgres_extensions() {
  local extensions="$1"

  if [[ -z "$extensions" ]]; then
    return 0
  fi

  local valid_extensions=(
    "uuid-ossp" "pgcrypto" "citext" "timescaledb" "postgis"
    "pgvector" "hstore" "pg_trgm" "btree_gin" "btree_gist"
    "pg_stat_statements" "postgres_fdw" "file_fdw"
  )

  IFS=',' read -r -a EXT_ARRAY <<<"$extensions"

  for ext in "${EXT_ARRAY[@]}"; do
    ext=$(echo "$ext" | tr -d ' ')
    local found=false

    for valid in "${valid_extensions[@]}"; do
      if [[ "$ext" == "$valid" ]]; then
        found=true
        break
      fi
    done

    if [[ "$found" == false ]]; then
      VALIDATION_WARNINGS+=("Unknown Postgres extension: '$ext'")
      AUTO_FIXES+=("validate_extension:$ext")
    fi
  done

  return 0
}

validate_ports() {
  local used_ports=()

  [[ "${POSTGRES_ENABLED:-true}" == "true" ]] && used_ports+=("${POSTGRES_PORT:-5432}")
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && used_ports+=("${REDIS_PORT:-6379}")
  [[ "${HASURA_ENABLED:-true}" == "true" ]] && used_ports+=("${HASURA_PORT:-8080}")
  [[ "${DASHBOARD_ENABLED:-false}" == "true" ]] && used_ports+=("${DASHBOARD_PORT:-3000}")
  [[ "${BULLMQ_DASHBOARD_ENABLED:-false}" == "true" ]] && used_ports+=("${BULLMQ_DASHBOARD_PORT:-3001}")

  local seen=()
  for port in "${used_ports[@]}"; do
    for s in "${seen[@]}"; do
      if [[ "$s" == "$port" ]]; then
        VALIDATION_ERRORS+=("Port conflict detected: $port is used by multiple services")
        AUTO_FIXES+=("fix_port_conflict:$port")
        break
      fi
    done
    seen+=("$port")

    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
      VALIDATION_WARNINGS+=("Port $port is already in use on the system")
      AUTO_FIXES+=("suggest_port_change:$port")
    fi
  done

  return 0
}

validate_docker_resources() {
  if ! docker info >/dev/null 2>&1; then
    VALIDATION_ERRORS+=("Docker is not running or not accessible")
    AUTO_FIXES+=("start_docker")
    return 1
  fi

  local mem_bytes=$(docker info --format '{{.MemTotal}}' 2>/dev/null)
  local mem_gb=$((mem_bytes / 1073741824))

  if [[ $mem_gb -lt 4 ]]; then
    VALIDATION_WARNINGS+=("Docker has only ${mem_gb}GB memory allocated (recommend 4GB+)")
    AUTO_FIXES+=("suggest_docker_memory")
  fi

  local disk_available=$(df -k /var/lib/docker 2>/dev/null | awk 'NR==2 {print $4}')
  local disk_gb=$((disk_available / 1048576))

  if [[ $disk_gb -lt 10 ]]; then
    VALIDATION_WARNINGS+=("Low disk space for Docker: ${disk_gb}GB available")
    AUTO_FIXES+=("suggest_disk_cleanup")
  fi

  return 0
}

validate_dependencies() {
  local deps_needed=()

  if [[ "${GOLANG_ENABLED:-false}" == "true" ]] && ! command -v go &>/dev/null; then
    VALIDATION_WARNINGS+=("Go is not installed but GOLANG_ENABLED=true")
    deps_needed+=("go")
  fi

  if [[ "${PYTHON_ENABLED:-false}" == "true" ]] && ! command -v python3 &>/dev/null; then
    VALIDATION_WARNINGS+=("Python3 is not installed but PYTHON_ENABLED=true")
    deps_needed+=("python3")
  fi

  if [[ "${NESTJS_ENABLED:-false}" == "true" ]] && ! command -v node &>/dev/null; then
    VALIDATION_WARNINGS+=("Node.js is not installed but NESTJS_ENABLED=true")
    deps_needed+=("nodejs")
  fi

  if [[ ${#deps_needed[@]} -gt 0 ]]; then
    AUTO_FIXES+=("install_dependencies:${deps_needed[*]}")
  fi

  return 0
}

run_validation() {
  local env_file="${1:-.env.local}"

  VALIDATION_ERRORS=()
  VALIDATION_WARNINGS=()
  AUTO_FIXES=()

  if [[ ! -f "$env_file" ]]; then
    format_error "Environment file not found: $env_file" "Run 'nself init' first"
    return 1
  fi

  # Load environment variables safely
  while IFS='=' read -r key value; do
    # Skip empty lines and comments
    [[ -z "$key" || "$key" =~ ^# ]] && continue
    # Remove leading/trailing whitespace from key
    key=$(echo "$key" | xargs)
    # Export the variable
    export "$key=$value"
  done <"$env_file"

  format_section "Validating Configuration"

  validate_project_name "${PROJECT_NAME:-}"
  validate_password "POSTGRES_PASSWORD" "${POSTGRES_PASSWORD:-}" 8
  validate_password "HASURA_GRAPHQL_ADMIN_SECRET" "${HASURA_GRAPHQL_ADMIN_SECRET:-}" 10
  validate_jwt_key "${HASURA_JWT_KEY:-}"

  validate_boolean "REDIS_ENABLED" "${REDIS_ENABLED:-}"
  validate_boolean "FUNCTIONS_ENABLED" "${FUNCTIONS_ENABLED:-}"
  validate_boolean "DASHBOARD_ENABLED" "${DASHBOARD_ENABLED:-}"
  validate_boolean "SERVICES_ENABLED" "${SERVICES_ENABLED:-}"
  validate_boolean "NESTJS_ENABLED" "${NESTJS_ENABLED:-}"
  validate_boolean "GOLANG_ENABLED" "${GOLANG_ENABLED:-}"
  validate_boolean "PYTHON_ENABLED" "${PYTHON_ENABLED:-}"
  validate_boolean "BULLMQ_ENABLED" "${BULLMQ_ENABLED:-}"
  validate_boolean "BULLMQ_DASHBOARD_ENABLED" "${BULLMQ_DASHBOARD_ENABLED:-}"

  validate_service_list "NESTJS_SERVICES" "${NESTJS_SERVICES:-}" "nestjs"
  validate_service_list "GOLANG_SERVICES" "${GOLANG_SERVICES:-}" "golang"
  validate_service_list "PYTHON_SERVICES" "${PYTHON_SERVICES:-}" "python"
  validate_service_list "BULLMQ_WORKERS" "${BULLMQ_WORKERS:-}" "worker"

  validate_postgres_extensions "${POSTGRES_EXTENSIONS:-}"
  validate_ports
  validate_docker_resources
  validate_dependencies

  if [[ ${#VALIDATION_ERRORS[@]} -gt 0 ]]; then
    format_section "Validation Errors" 40
    for error in "${VALIDATION_ERRORS[@]}"; do
      printf "${RED}✗${RESET} %s\n" "$error"
    done
  fi

  if [[ ${#VALIDATION_WARNINGS[@]} -gt 0 ]]; then
    format_section "Validation Warnings" 40
    for warning in "${VALIDATION_WARNINGS[@]}"; do
      printf "${YELLOW}⚠${RESET} %s\n" "$warning"
    done
  fi

  if [[ ${#VALIDATION_ERRORS[@]} -eq 0 ]] && [[ ${#VALIDATION_WARNINGS[@]} -eq 0 ]]; then
    format_success "All validations passed!"
    return 0
  fi

  return $([[ ${#VALIDATION_ERRORS[@]} -gt 0 ]] && echo 1 || echo 0)
}

export -f validate_project_name
export -f validate_password
export -f validate_jwt_key
export -f validate_boolean
export -f validate_service_list
export -f validate_postgres_extensions
export -f validate_ports
export -f validate_docker_resources
export -f validate_dependencies
export -f run_validation
