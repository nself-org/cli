#!/usr/bin/env bash


CONFIG_VALIDATOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${CONFIG_VALIDATOR_DIR}/../utils/display.sh"
source "${CONFIG_VALIDATOR_DIR}/../utils/output-formatter.sh"

VALIDATION_ERRORS=()
VALIDATION_WARNINGS=()
AUTO_FIXES=()

# Enhanced: Handle empty file
validate_file_not_empty() {
  local env_file="$1"

  if [[ ! -s "$env_file" ]]; then
    VALIDATION_ERRORS+=("Environment file is empty: $env_file")
    AUTO_FIXES+=("create_minimal_config:$env_file")
    return 1
  fi

  # Check if file only has comments
  local non_comment_lines=$(grep -v '^#' "$env_file" | grep -v '^[[:space:]]*$' | wc -l)
  if [[ $non_comment_lines -eq 0 ]]; then
    VALIDATION_ERRORS+=("Environment file contains no configuration (only comments): $env_file")
    AUTO_FIXES+=("create_minimal_config:$env_file")
    return 1
  fi

  return 0
}

# Enhanced: Detect and fix whitespace issues
validate_whitespace() {
  local env_file="$1"
  local has_issues=false

  while IFS= read -r line; do
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    # Check for leading/trailing whitespace in values
    if [[ "$line" =~ ^([A-Z_]+)=(.*)$ ]]; then
      local key="${BASH_REMATCH[1]}"
      local value="${BASH_REMATCH[2]}"

      # Check for leading whitespace in value
      if [[ "$value" =~ ^[[:space:]]+ ]]; then
        VALIDATION_WARNINGS+=("$key has leading whitespace in value")
        AUTO_FIXES+=("trim_whitespace:$env_file:$key")
        has_issues=true
      fi

      # Check for trailing whitespace
      if [[ "$value" =~ [[:space:]]+$ ]]; then
        VALIDATION_WARNINGS+=("$key has trailing whitespace in value")
        AUTO_FIXES+=("trim_whitespace:$env_file:$key")
        has_issues=true
      fi

      # Check for tabs instead of spaces
      if [[ "$line" =~ $'\t' ]]; then
        VALIDATION_WARNINGS+=("Line contains tabs: $key")
        AUTO_FIXES+=("replace_tabs:$env_file:$key")
        has_issues=true
      fi
    fi
  done <"$env_file"

  return $([[ "$has_issues" == true ]] && echo 1 || echo 0)
}

# Enhanced: Detect quote mismatches
validate_quotes() {
  local env_file="$1"
  local has_issues=false

  while IFS= read -r line; do
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    if [[ "$line" =~ ^([A-Z_]+)=(.*)$ ]]; then
      local key="${BASH_REMATCH[1]}"
      local value="${BASH_REMATCH[2]}"

      # Count quotes
      local single_quotes=$(echo "$value" | grep -o "'" | wc -l)
      local double_quotes=$(echo "$value" | grep -o '"' | wc -l)

      # Check for odd number of quotes (mismatch)
      if [[ $((single_quotes % 2)) -ne 0 ]]; then
        VALIDATION_ERRORS+=("$key has mismatched single quotes")
        AUTO_FIXES+=("fix_quote_mismatch:$env_file:$key:single")
        has_issues=true
      fi

      if [[ $((double_quotes % 2)) -ne 0 ]]; then
        VALIDATION_ERRORS+=("$key has mismatched double quotes")
        AUTO_FIXES+=("fix_quote_mismatch:$env_file:$key:double")
        has_issues=true
      fi

      # Check for mixed quotes
      if [[ "$value" =~ ^[\'\"] ]] && [[ "$value" =~ [\'\"]$ ]]; then
        local first_char="${value:0:1}"
        local last_char="${value: -1}"
        if [[ "$first_char" != "$last_char" ]]; then
          VALIDATION_ERRORS+=("$key has mixed quote styles")
          AUTO_FIXES+=("fix_mixed_quotes:$env_file:$key")
          has_issues=true
        fi
      fi
    fi
  done <"$env_file"

  return $([[ "$has_issues" == true ]] && echo 1 || echo 0)
}

# Enhanced: Detect duplicate variables
validate_duplicates() {
  local env_file="$1"
  local -a seen_keys=()
  local has_duplicates=false

  while IFS= read -r line; do
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    if [[ "$line" =~ ^([A-Z_]+)= ]]; then
      local key="${BASH_REMATCH[1]}"

      # Check if we've seen this key before
      for seen in "${seen_keys[@]}"; do
        if [[ "$seen" == "$key" ]]; then
          VALIDATION_ERRORS+=("Duplicate variable found: $key")
          AUTO_FIXES+=("remove_duplicate:$env_file:$key")
          has_duplicates=true
          break
        fi
      done

      seen_keys+=("$key")
    fi
  done <"$env_file"

  return $([[ "$has_duplicates" == true ]] && echo 1 || echo 0)
}

# Enhanced: Validate port numbers
validate_port_number() {
  local var_name="$1"
  local port="$2"

  if [[ -z "$port" ]]; then
    return 0 # Port not set, use default
  fi

  # Check if port is numeric
  if ! [[ "$port" =~ ^[0-9]+$ ]]; then
    VALIDATION_ERRORS+=("$var_name is not a valid number: '$port'")
    AUTO_FIXES+=("fix_port_number:$var_name:$port")
    return 1
  fi

  # Check port range (1-65535)
  if [[ $port -lt 1 ]] || [[ $port -gt 65535 ]]; then
    VALIDATION_ERRORS+=("$var_name is outside valid range (1-65535): $port")
    AUTO_FIXES+=("fix_port_range:$var_name:$port")
    return 1
  fi

  # Warn about privileged ports
  if [[ $port -lt 1024 ]]; then
    VALIDATION_WARNINGS+=("$var_name uses privileged port (requires root): $port")
  fi

  # Check for commonly problematic ports
  local problematic_ports=(80 443 3000 3306 5432 6379 8080 8443 9000)
  for prob_port in "${problematic_ports[@]}"; do
    if [[ $port -eq $prob_port ]]; then
      # Check if port is actually in use
      if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        VALIDATION_WARNINGS+=("$var_name uses commonly occupied port $port (currently in use)")
        AUTO_FIXES+=("suggest_alternative_port:$var_name:$port")
      fi
      break
    fi
  done

  return 0
}

# Enhanced: Handle inline comments
validate_inline_comments() {
  local env_file="$1"
  local has_inline_comments=false

  while IFS= read -r line; do
    # Skip empty lines and full-line comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    # Check for inline comments (but not in quoted strings)
    if [[ "$line" =~ ^([A-Z_]+)=([^#]*)(#.*)$ ]]; then
      local key="${BASH_REMATCH[1]}"
      local value="${BASH_REMATCH[2]}"
      local comment="${BASH_REMATCH[3]}"

      # Check if the # is inside quotes
      local quoted_value=$(echo "$value" | sed "s/'[^']*'//g" | sed 's/"[^"]*"//g')
      if [[ "$quoted_value" != *"#"* ]]; then
        VALIDATION_WARNINGS+=("$key has inline comment (will be removed)")
        AUTO_FIXES+=("remove_inline_comment:$env_file:$key")
        has_inline_comments=true
      fi
    fi
  done <"$env_file"

  return $([[ "$has_inline_comments" == true ]] && echo 1 || echo 0)
}

# Enhanced: Validate special characters in passwords
validate_password_enhanced() {
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

  # Check for problematic special characters
  if [[ "$password" =~ [\'\"\`\$\!\&\*\(\)\{\}\[\]\;\<\>\|\\] ]]; then
    VALIDATION_WARNINGS+=("$var_name contains special characters that may need escaping")
    AUTO_FIXES+=("escape_password_special:$var_name:$password")
    return 2
  fi

  # Check for spaces
  if [[ "$password" =~ [[:space:]] ]]; then
    VALIDATION_ERRORS+=("$var_name contains spaces")
    AUTO_FIXES+=("remove_password_spaces:$var_name:$password")
    return 1
  fi

  # Check for common weak passwords
  local weak_passwords=("password" "123456" "admin" "secret" "changeme" "default" "test" "demo")
  local password_lower=$(echo "$password" | tr '[:upper:]' '[:lower:]')
  for weak in "${weak_passwords[@]}"; do
    if [[ "$password_lower" == "$weak" ]]; then
      VALIDATION_ERRORS+=("$var_name uses a weak/common password")
      AUTO_FIXES+=("replace_weak_password:$var_name")
      return 1
    fi
  done

  return 0
}

# New: Validate IP addresses
validate_ip_address() {
  local var_name="$1"
  local ip="$2"

  if [[ -z "$ip" ]]; then
    return 0
  fi

  # Check for valid IPv4
  if [[ "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
    IFS='.' read -r -a octets <<<"$ip"
    for octet in "${octets[@]}"; do
      if [[ $octet -gt 255 ]]; then
        VALIDATION_ERRORS+=("$var_name has invalid IP address (octet >255): $ip")
        AUTO_FIXES+=("fix_ip_address:$var_name:$ip")
        return 1
      fi
    done
  elif [[ "$ip" != "localhost" ]] && [[ "$ip" != "*" ]]; then
    # Not a valid hostname pattern either
    VALIDATION_ERRORS+=("$var_name has invalid IP/hostname: $ip")
    AUTO_FIXES+=("fix_ip_address:$var_name:$ip")
    return 1
  fi

  return 0
}

# New: Validate memory format
validate_memory_format() {
  local var_name="$1"
  local memory="$2"

  if [[ -z "$memory" ]]; then
    return 0
  fi

  # Check for valid memory format (e.g., 512M, 2G, 1024K)
  if ! [[ "$memory" =~ ^[0-9]+[KMG]?$ ]]; then
    VALIDATION_ERRORS+=("$var_name has invalid memory format: $memory (use format like 512M or 2G)")
    AUTO_FIXES+=("fix_memory_format:$var_name:$memory")
    return 1
  fi

  return 0
}

# New: Validate timezone
validate_timezone() {
  local var_name="$1"
  local timezone="$2"

  if [[ -z "$timezone" ]]; then
    return 0
  fi

  # Check common timezone formats
  if [[ "$timezone" =~ ^[A-Z][a-z]+/[A-Z][a-z]+$ ]] || [[ "$timezone" == "UTC" ]] || [[ "$timezone" == "GMT" ]]; then
    return 0
  fi

  VALIDATION_WARNINGS+=("$var_name may have invalid timezone format: $timezone")
  AUTO_FIXES+=("validate_timezone:$var_name:$timezone")
  return 2
}

# New: Validate Docker naming conventions
validate_docker_name() {
  local var_name="$1"
  local name="$2"

  if [[ -z "$name" ]]; then
    return 0
  fi

  # Docker names must be lowercase and can contain letters, digits, underscores, periods and dashes
  if ! [[ "$name" =~ ^[a-z0-9][a-z0-9._-]*$ ]]; then
    VALIDATION_ERRORS+=("$var_name doesn't follow Docker naming conventions: $name")
    AUTO_FIXES+=("fix_docker_name:$var_name:$name")
    return 1
  fi

  # Check length (Docker has a 63 character limit for container names)
  if [[ ${#name} -gt 63 ]]; then
    VALIDATION_ERRORS+=("$var_name is too long for Docker (>63 chars): $name")
    AUTO_FIXES+=("truncate_docker_name:$var_name:$name")
    return 1
  fi

  return 0
}

# New: Validate file paths exist
validate_file_path() {
  local var_name="$1"
  local path="$2"

  if [[ -z "$path" ]]; then
    return 0
  fi

  # SECURITY: Expand path safely - only handle ~ and $HOME, not arbitrary commands
  # Replace leading ~ with $HOME, and expand known env vars without eval
  path="${path/#\~/$HOME}"
  path="${path/\$HOME/$HOME}"
  path="${path/\${HOME\}/$HOME}"

  if [[ ! -e "$path" ]]; then
    VALIDATION_WARNINGS+=("$var_name references non-existent path: $path")
    AUTO_FIXES+=("create_missing_path:$var_name:$path")
    return 2
  fi

  return 0
}

# New: Validate SSL configuration
validate_ssl_config() {
  local ssl_mode="${SSL_MODE:-}"
  local ssl_cert="${SSL_CERT_PATH:-}"
  local ssl_key="${SSL_KEY_PATH:-}"

  if [[ "$ssl_mode" == "custom" ]]; then
    if [[ -z "$ssl_cert" ]] || [[ -z "$ssl_key" ]]; then
      VALIDATION_ERRORS+=("SSL_MODE is 'custom' but SSL_CERT_PATH or SSL_KEY_PATH not set")
      AUTO_FIXES+=("fix_ssl_config")
      return 1
    fi

    validate_file_path "SSL_CERT_PATH" "$ssl_cert"
    validate_file_path "SSL_KEY_PATH" "$ssl_key"
  fi

  return 0
}

# Original validation functions for backward compatibility
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

  # Use enhanced version
  validate_password_enhanced "$var_name" "$password" "$min_length"
  return $?
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

# Main validation runner
run_validation() {
  local env_file="${1:-.env.local}"

  VALIDATION_ERRORS=()
  VALIDATION_WARNINGS=()
  AUTO_FIXES=()

  if [[ ! -f "$env_file" ]]; then
    format_error "Environment file not found: $env_file" "Run 'nself init' first"
    return 1
  fi

  format_section "Validating Configuration"

  # File-level validations
  validate_file_not_empty "$env_file"
  validate_whitespace "$env_file"
  validate_quotes "$env_file"
  validate_duplicates "$env_file"
  validate_inline_comments "$env_file"

  # Load environment variables safely
  if [[ -s "$env_file" ]]; then
    while IFS='=' read -r key value; do
      # Skip empty lines and comments
      [[ -z "$key" || "$key" =~ ^[[:space:]]*# ]] && continue
      # Remove leading/trailing whitespace from key
      key=$(echo "$key" | xargs)
      # Remove inline comments from value
      value=$(echo "$value" | sed 's/#.*//')
      # Remove leading/trailing whitespace from value
      value=$(echo "$value" | xargs)
      # Export the variable
      export "$key=$value"
    done <"$env_file"
  fi

  # Variable-level validations
  validate_project_name "${PROJECT_NAME:-}"
  validate_docker_name "PROJECT_NAME" "${PROJECT_NAME:-}"

  # Enhanced password validations
  validate_password_enhanced "POSTGRES_PASSWORD" "${POSTGRES_PASSWORD:-}" 8
  validate_password_enhanced "HASURA_GRAPHQL_ADMIN_SECRET" "${HASURA_GRAPHQL_ADMIN_SECRET:-}" 10
  validate_password_enhanced "REDIS_PASSWORD" "${REDIS_PASSWORD:-}" 6

  validate_jwt_key "${HASURA_JWT_KEY:-}"

  # Port validations
  validate_port_number "POSTGRES_PORT" "${POSTGRES_PORT:-}"
  validate_port_number "REDIS_PORT" "${REDIS_PORT:-}"
  validate_port_number "HASURA_PORT" "${HASURA_PORT:-}"
  validate_port_number "AUTH_PORT" "${AUTH_PORT:-}"
  validate_port_number "STORAGE_PORT" "${STORAGE_PORT:-}"
  validate_port_number "DASHBOARD_PORT" "${DASHBOARD_PORT:-}"
  validate_port_number "NGINX_HTTP_PORT" "${NGINX_HTTP_PORT:-}"
  validate_port_number "NGINX_HTTPS_PORT" "${NGINX_HTTPS_PORT:-}"

  # Boolean validations
  validate_boolean "REDIS_ENABLED" "${REDIS_ENABLED:-}"
  validate_boolean "FUNCTIONS_ENABLED" "${FUNCTIONS_ENABLED:-}"
  validate_boolean "DASHBOARD_ENABLED" "${DASHBOARD_ENABLED:-}"
  validate_boolean "SERVICES_ENABLED" "${SERVICES_ENABLED:-}"
  validate_boolean "NESTJS_ENABLED" "${NESTJS_ENABLED:-}"
  validate_boolean "GOLANG_ENABLED" "${GOLANG_ENABLED:-}"
  validate_boolean "PYTHON_ENABLED" "${PYTHON_ENABLED:-}"
  validate_boolean "BULLMQ_ENABLED" "${BULLMQ_ENABLED:-}"
  validate_boolean "BULLMQ_DASHBOARD_ENABLED" "${BULLMQ_DASHBOARD_ENABLED:-}"
  validate_boolean "DB_ENV_SEEDS" "${DB_ENV_SEEDS:-}"
  validate_boolean "HASURA_GRAPHQL_ENABLE_CONSOLE" "${HASURA_GRAPHQL_ENABLE_CONSOLE:-}"
  validate_boolean "HASURA_GRAPHQL_DEV_MODE" "${HASURA_GRAPHQL_DEV_MODE:-}"

  # Service list validations
  validate_service_list "NESTJS_SERVICES" "${NESTJS_SERVICES:-}" "nestjs"
  validate_service_list "GOLANG_SERVICES" "${GOLANG_SERVICES:-}" "golang"
  validate_service_list "PYTHON_SERVICES" "${PYTHON_SERVICES:-}" "python"
  validate_service_list "BULLMQ_WORKERS" "${BULLMQ_WORKERS:-}" "worker"

  # Other validations
  validate_postgres_extensions "${POSTGRES_EXTENSIONS:-}"
  validate_ip_address "POSTGRES_HOST" "${POSTGRES_HOST:-}"
  validate_memory_format "POSTGRES_MEMORY" "${POSTGRES_MEMORY:-}"
  validate_memory_format "REDIS_MEMORY" "${REDIS_MEMORY:-}"
  validate_timezone "TZ" "${TZ:-}"
  validate_ssl_config

  # Check for port conflicts
  validate_ports

  # Check Docker resources
  validate_docker_resources

  # Check dependencies
  validate_dependencies

  # Display results
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

  # Show auto-fix availability
  if [[ ${#AUTO_FIXES[@]} -gt 0 ]]; then
    echo ""
    format_info "Auto-fixes available: ${#AUTO_FIXES[@]}"
    echo "Run with --apply-fixes to automatically resolve these issues"
  fi

  return $([[ ${#VALIDATION_ERRORS[@]} -gt 0 ]] && echo 1 || echo 0)
}

# Export all functions
export -f validate_file_not_empty
export -f validate_whitespace
export -f validate_quotes
export -f validate_duplicates
export -f validate_inline_comments
export -f validate_port_number
export -f validate_password_enhanced
export -f validate_ip_address
export -f validate_memory_format
export -f validate_timezone
export -f validate_docker_name
export -f validate_file_path
export -f validate_ssl_config
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
