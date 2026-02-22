#!/usr/bin/env bash

# validation.sh - Input and configuration validation utilities

# Source display utilities
UTILS_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$UTILS_DIR/display.sh" 2>/dev/null || true

# Validate domain name
is_valid_domain() {
  local domain="$1"
  [[ "$domain" =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$ ]]
}

# Validate email address
is_valid_email() {
  local email="$1"
  [[ "$email" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]
}

# Validate port number
is_valid_port() {
  local port="$1"
  [[ "$port" =~ ^[0-9]+$ ]] && [[ $port -ge 1 ]] && [[ $port -le 65535 ]]
}

# Validate URL
is_valid_url() {
  local url="$1"
  [[ "$url" =~ ^https?://[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$ ]]
}

# Validate IP address
is_valid_ip() {
  local ip="$1"
  local IFS='.'
  local -a octets=($ip)

  [[ ${#octets[@]} -eq 4 ]] || return 1

  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^[0-9]+$ ]] || return 1
    [[ $octet -ge 0 && $octet -le 255 ]] || return 1
  done

  return 0
}

# Validate JSON
is_valid_json() {
  local json="$1"

  if command -v jq >/dev/null 2>&1; then
    echo "$json" | jq empty 2>/dev/null
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c "import json; json.loads('''$json''')" 2>/dev/null
  else
    # Basic check for JSON structure
    [[ "$json" =~ ^\{.*\}$ ]] || [[ "$json" =~ ^\[.*\]$ ]]
  fi
}

# Validate YAML
is_valid_yaml() {
  local yaml_file="$1"

  if command -v yq >/dev/null 2>&1; then
    yq eval '.' "$yaml_file" >/dev/null 2>&1
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c "import yaml; yaml.safe_load(open('$yaml_file'))" 2>/dev/null
  else
    # Basic syntax check
    ! grep -E '^\t' "$yaml_file" >/dev/null 2>&1 # No tabs in YAML
  fi
}

# Validate environment configuration
validate_env_config() {
  local env_file="${1:-.env.local}"
  local errors=0

  if [[ ! -f "$env_file" ]]; then
    log_error "Environment file not found: $env_file"
    return 1
  fi

  # Source the environment file
  source "$env_file"

  # Check required variables
  local -a required_vars=(
    "PROJECT_NAME"
    "BASE_DOMAIN"
    "POSTGRES_PASSWORD"
    "HASURA_GRAPHQL_ADMIN_SECRET"
  )

  for var in "${required_vars[@]}"; do
    if [[ -z "${!var:-}" ]]; then
      log_error "Missing required variable: $var"
      errors=$((errors + 1))
    fi
  done

  # Validate domain
  if [[ -n "${BASE_DOMAIN:-}" ]] && ! is_valid_domain "$BASE_DOMAIN"; then
    log_error "Invalid domain: $BASE_DOMAIN"
    errors=$((errors + 1))
  fi

  # Validate JWT secret if present
  if [[ -n "${HASURA_GRAPHQL_JWT_SECRET:-}" ]]; then
    if ! is_valid_json "$HASURA_GRAPHQL_JWT_SECRET"; then
      log_error "Invalid JSON in HASURA_GRAPHQL_JWT_SECRET"
      errors=$((errors + 1))
    fi
  fi

  # Check for inline comments
  if grep -E '^\s*[A-Z_]+=[^#]*#' "$env_file" >/dev/null 2>&1; then
    log_warning "Inline comments detected (can cause issues)"
    errors=$((errors + 1))
  fi

  # Check for weak passwords
  if [[ "${POSTGRES_PASSWORD:-}" == "changeme" ]] || [[ "${POSTGRES_PASSWORD:-}" == "password" ]]; then
    log_warning "Weak POSTGRES_PASSWORD detected"
  fi

  if [[ $errors -eq 0 ]]; then
    log_success "Environment configuration is valid"
    return 0
  else
    log_error "Environment configuration has $errors error(s)"
    return 1
  fi
}

# Validate Docker prerequisites
validate_docker_prerequisites() {
  local errors=0

  # Check Docker is installed
  if ! command -v docker >/dev/null 2>&1; then
    log_error "Docker is not installed"
    errors=$((errors + 1))
  fi

  # Check Docker Compose
  if ! docker compose version >/dev/null 2>&1; then
    log_error "Docker Compose v2 is not available"
    errors=$((errors + 1))
  fi

  # Check Docker is running
  if ! docker info >/dev/null 2>&1; then
    log_error "Docker daemon is not running"
    errors=$((errors + 1))
  fi

  return $errors
}

# Validate port availability
validate_ports() {
  local -a required_ports=(5432 8080 9000 6379 80 443)
  local errors=0

  for port in "${required_ports[@]}"; do
    if ! is_port_available "$port"; then
      log_warning "Port $port is already in use"

      # Check if it's our container
      if docker ps --filter "publish=$port" --format '{{.Names}}' | grep -q "^${PROJECT_NAME:-nself}_"; then
        log_info "Port $port is used by our container (OK)"
      else
        errors=$((errors + 1))
      fi
    fi
  done

  return $errors
}

# Validate file permissions
validate_permissions() {
  local errors=0

  # Check if we can write to current directory
  if ! touch .test_write 2>/dev/null; then
    log_error "Cannot write to current directory"
    errors=$((errors + 1))
  else
    rm -f .test_write
  fi

  # Check Docker socket permissions
  if [[ -S /var/run/docker.sock ]] && ! [[ -w /var/run/docker.sock ]]; then
    log_warning "No write access to Docker socket"
    log_info "You may need to add your user to the docker group"
  fi

  return $errors
}

# Export all functions
export -f is_valid_domain is_valid_email is_valid_port is_valid_url
export -f is_valid_ip is_valid_json is_valid_yaml
export -f validate_env_config validate_docker_prerequisites
export -f validate_ports validate_permissions
