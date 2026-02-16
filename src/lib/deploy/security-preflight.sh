#!/usr/bin/env bash
set -euo pipefail

# security-preflight.sh - Production security validation before deployment
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
PREFLIGHT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_ROOT="$(dirname "$PREFLIGHT_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/env.sh" 2>/dev/null || true

# Minimum password lengths
MIN_PASSWORD_LENGTH=16
MIN_SECRET_LENGTH=32

# ============================================================
# Security Pre-flight Checks for Production Deployment
# ============================================================

# Run all security checks
security::preflight() {
  local env_name="${1:-prod}"
  local env_dir="${2:-.environments/$env_name}"
  local strict="${3:-false}" # Strict mode fails on warnings too

  printf "\n${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "${COLOR_CYAN}     Production Security Pre-flight Checks${COLOR_RESET}\n"
  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n\n"

  local errors=0
  local warnings=0

  # Load environment
  if [[ -f "$env_dir/.env" ]]; then
    set -a
    source "$env_dir/.env" 2>/dev/null || true
    set +a
  fi

  if [[ -f "$env_dir/.env.secrets" ]]; then
    set -a
    source "$env_dir/.env.secrets" 2>/dev/null || true
    set +a
  fi

  # 1. Check for required secrets
  printf "${COLOR_BOLD}1. Required Secrets${COLOR_RESET}\n"
  security::check_secrets
  errors=$((errors + $?))
  printf "\n"

  # 2. Check password strength
  printf "${COLOR_BOLD}2. Password Strength${COLOR_RESET}\n"
  security::check_password_strength
  local pw_result=$?
  if [[ "$strict" == "true" ]]; then
    errors=$((errors + pw_result))
  else
    warnings=$((warnings + pw_result))
  fi
  printf "\n"

  # 3. Check service bindings
  printf "${COLOR_BOLD}3. Service Security Bindings${COLOR_RESET}\n"
  security::check_service_bindings "$env_dir"
  errors=$((errors + $?))
  printf "\n"

  # 4. Check SSL configuration
  printf "${COLOR_BOLD}4. SSL Configuration${COLOR_RESET}\n"
  security::check_ssl_config "$env_dir"
  local ssl_result=$?
  if [[ "$strict" == "true" ]]; then
    errors=$((errors + ssl_result))
  else
    warnings=$((warnings + ssl_result))
  fi
  printf "\n"

  # 5. Check for insecure defaults
  printf "${COLOR_BOLD}5. Insecure Defaults${COLOR_RESET}\n"
  security::check_insecure_defaults
  errors=$((errors + $?))
  printf "\n"

  # 6. Check admin services in production
  printf "${COLOR_BOLD}6. Production Service Settings${COLOR_RESET}\n"
  security::check_production_services
  local svc_result=$?
  warnings=$((warnings + svc_result))
  printf "\n"

  # Summary
  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "${COLOR_BOLD}Summary${COLOR_RESET}\n"
  printf "  Errors:   %d\n" "$errors"
  printf "  Warnings: %d\n" "$warnings"
  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n\n"

  if [[ $errors -gt 0 ]]; then
    log_error "DEPLOYMENT BLOCKED: $errors critical security issue(s) found"
    printf "\n"
    printf "To fix these issues:\n"
    printf "  1. Generate secure secrets: ${COLOR_CYAN}nself secrets generate${COLOR_RESET}\n"
    printf "  2. Update your environment: ${COLOR_CYAN}$env_dir/.env.secrets${COLOR_RESET}\n"
    printf "  3. Run this check again:    ${COLOR_CYAN}nself deploy check $env_name${COLOR_RESET}\n"
    printf "\n"
    return 1
  elif [[ $warnings -gt 0 ]]; then
    log_warning "$warnings warning(s) - review before deploying"
    return 0
  else
    log_success "All security checks passed"
    return 0
  fi
}

# Check that all required secrets are set
security::check_secrets() {
  local errors=0

  # Required secrets for production
  local required_secrets=(
    "POSTGRES_PASSWORD:PostgreSQL password"
    "HASURA_GRAPHQL_ADMIN_SECRET:Hasura admin secret"
    "AUTH_JWT_SECRET:JWT signing secret"
  )

  # Conditional secrets
  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    required_secrets+=("REDIS_PASSWORD:Redis password")
  fi

  if [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]]; then
    required_secrets+=("MEILISEARCH_MASTER_KEY:MeiliSearch master key")
  fi

  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    required_secrets+=("MINIO_ROOT_PASSWORD:MinIO root password")
  fi

  for secret_spec in "${required_secrets[@]}"; do
    local var_name="${secret_spec%%:*}"
    local description="${secret_spec#*:}"
    local value="${!var_name:-}"

    if [[ -z "$value" ]]; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} %s (%s): ${COLOR_RED}Not set${COLOR_RESET}\n" "$var_name" "$description"
      errors=$((errors + 1))
    elif [[ "$value" == "changeme" ]] || [[ "$value" == "password" ]] || [[ "$value" == "secret" ]]; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} %s: ${COLOR_RED}Using insecure default value${COLOR_RESET}\n" "$var_name"
      errors=$((errors + 1))
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} %s: Configured\n" "$var_name"
    fi
  done

  return $errors
}

# Check password/secret strength
security::check_password_strength() {
  local warnings=0

  local secrets_to_check=(
    "POSTGRES_PASSWORD:$MIN_PASSWORD_LENGTH"
    "HASURA_GRAPHQL_ADMIN_SECRET:$MIN_SECRET_LENGTH"
    "AUTH_JWT_SECRET:$MIN_SECRET_LENGTH"
    "REDIS_PASSWORD:$MIN_PASSWORD_LENGTH"
    "MEILISEARCH_MASTER_KEY:$MIN_SECRET_LENGTH"
    "MINIO_ROOT_PASSWORD:$MIN_PASSWORD_LENGTH"
  )

  for spec in "${secrets_to_check[@]}"; do
    local var_name="${spec%%:*}"
    local min_length="${spec#*:}"
    local value="${!var_name:-}"

    if [[ -n "$value" ]]; then
      local length=${#value}
      if [[ $length -lt $min_length ]]; then
        printf "  ${COLOR_YELLOW}!${COLOR_RESET} %s: %d chars (recommended: >=%d)\n" "$var_name" "$length" "$min_length"
        warnings=$((warnings + 1))
      else
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} %s: %d chars ${COLOR_DIM}(strong)${COLOR_RESET}\n" "$var_name" "$length"
      fi
    fi
  done

  return $warnings
}

# Check that services are bound to localhost only
security::check_service_bindings() {
  local env_dir="$1"
  local errors=0

  # Check docker-compose.yml for service bindings
  local compose_file="docker-compose.yml"
  if [[ ! -f "$compose_file" ]]; then
    compose_file="$env_dir/docker-compose.yml"
  fi

  if [[ -f "$compose_file" ]]; then
    # Check PostgreSQL binding
    if grep -q "5432:5432" "$compose_file" 2>/dev/null && ! grep -q "127.0.0.1.*5432:5432" "$compose_file" 2>/dev/null; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} PostgreSQL: Bound to 0.0.0.0 (should be 127.0.0.1)\n"
      errors=$((errors + 1))
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} PostgreSQL: Localhost binding\n"
    fi

    # Check Redis binding
    if grep -q "6379:6379" "$compose_file" 2>/dev/null && ! grep -q "127.0.0.1.*6379:6379" "$compose_file" 2>/dev/null; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} Redis: Bound to 0.0.0.0 (should be 127.0.0.1)\n"
      errors=$((errors + 1))
    elif grep -q "6379:6379" "$compose_file" 2>/dev/null; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Redis: Localhost binding\n"
    fi
  else
    printf "  ${COLOR_YELLOW}!${COLOR_RESET} Cannot verify service bindings (no docker-compose.yml)\n"
  fi

  return $errors
}

# Check SSL configuration
security::check_ssl_config() {
  local env_dir="$1"
  local warnings=0

  local ssl_enabled="${SSL_ENABLED:-true}"
  local env_type="${ENV:-dev}"

  if [[ "$env_type" == "prod" ]] || [[ "$env_type" == "production" ]]; then
    if [[ "$ssl_enabled" != "true" ]]; then
      printf "  ${COLOR_RED}✗${COLOR_RESET} SSL_ENABLED: Should be 'true' in production\n"
      warnings=$((warnings + 1))
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} SSL_ENABLED: true\n"
    fi

    # Check for Let's Encrypt configuration
    if [[ -n "${DNS_PROVIDER:-}" ]] && [[ -n "${DNS_API_TOKEN:-}" ]]; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Let's Encrypt: Configured (DNS provider: %s)\n" "${DNS_PROVIDER}"
    elif [[ -n "${LETSENCRYPT_EMAIL:-}" ]]; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Let's Encrypt: Configured (HTTP validation)\n"
    else
      printf "  ${COLOR_YELLOW}!${COLOR_RESET} Let's Encrypt: Not configured (will use self-signed)\n"
      printf "    ${COLOR_DIM}Set LETSENCRYPT_EMAIL or DNS_PROVIDER for automatic certificates${COLOR_RESET}\n"
      warnings=$((warnings + 1))
    fi
  else
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} Non-production environment (SSL optional)\n"
  fi

  return $warnings
}

# Check for insecure default values
security::check_insecure_defaults() {
  local errors=0

  # List of variables that should NOT have these default values
  local insecure_patterns=(
    "password"
    "changeme"
    "secret"
    "admin"
    "12345"
    "qwerty"
    "default"
  )

  local vars_to_check=(
    "POSTGRES_PASSWORD"
    "HASURA_GRAPHQL_ADMIN_SECRET"
    "AUTH_JWT_SECRET"
    "REDIS_PASSWORD"
    "MINIO_ROOT_PASSWORD"
  )

  for var_name in "${vars_to_check[@]}"; do
    local value="${!var_name:-}"
    local value_lower
    value_lower=$(printf "%s" "$value" | tr '[:upper:]' '[:lower:]')

    for pattern in "${insecure_patterns[@]}"; do
      if [[ "$value_lower" == "$pattern" ]] || [[ "$value_lower" == *"$pattern"* && ${#value} -lt 16 ]]; then
        printf "  ${COLOR_RED}✗${COLOR_RESET} %s: Contains insecure pattern '%s'\n" "$var_name" "$pattern"
        errors=$((errors + 1))
        break
      fi
    done
  done

  if [[ $errors -eq 0 ]]; then
    printf "  ${COLOR_GREEN}✓${COLOR_RESET} No insecure default values detected\n"
  fi

  return $errors
}

# Check production service settings
security::check_production_services() {
  local warnings=0
  local env_type="${ENV:-dev}"

  if [[ "$env_type" == "prod" ]] || [[ "$env_type" == "production" ]]; then
    # Check if admin/debug services are disabled
    if [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]]; then
      printf "  ${COLOR_YELLOW}!${COLOR_RESET} NSELF_ADMIN_ENABLED: Should be 'false' in production\n"
      warnings=$((warnings + 1))
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} NSELF_ADMIN_ENABLED: false\n"
    fi

    if [[ "${MAILPIT_ENABLED:-false}" == "true" ]]; then
      printf "  ${COLOR_YELLOW}!${COLOR_RESET} MAILPIT_ENABLED: Development mail service enabled in production\n"
      warnings=$((warnings + 1))
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} MAILPIT_ENABLED: false\n"
    fi

    if [[ "${HASURA_DEV_MODE:-false}" == "true" ]]; then
      printf "  ${COLOR_YELLOW}!${COLOR_RESET} HASURA_DEV_MODE: Should be 'false' in production\n"
      warnings=$((warnings + 1))
    else
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} HASURA_DEV_MODE: false\n"
    fi
  else
    printf "  ${COLOR_DIM}Non-production environment - skipping production service checks${COLOR_RESET}\n"
  fi

  return $warnings
}

# Generate secure secrets
security::generate_secrets() {
  local env_dir="${1:-.environments/prod}"
  local secrets_file="$env_dir/.env.secrets"

  printf "Generating secure secrets...\n\n"

  # Create directory if needed
  mkdir -p "$env_dir"

  # Generate secrets
  local postgres_pw=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
  local hasura_secret=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
  local jwt_secret=$(openssl rand -base64 48 | tr -d '/+=' | head -c 64)
  local redis_pw=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
  local minio_pw=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)
  local meili_key=$(openssl rand -base64 32 | tr -d '/+=' | head -c 44)

  cat >"$secrets_file" <<EOF
# Production Secrets - Generated $(date)
# NEVER commit this file to version control
# Permissions should be 600 (chmod 600 $secrets_file)

# Database
POSTGRES_PASSWORD=$postgres_pw

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=$hasura_secret

# Authentication
AUTH_JWT_SECRET=$jwt_secret
AUTH_SECRET_KEY=$jwt_secret

# Redis (if enabled)
REDIS_PASSWORD=$redis_pw

# MinIO (if enabled)
MINIO_ROOT_PASSWORD=$minio_pw

# MeiliSearch (if enabled)
MEILISEARCH_MASTER_KEY=$meili_key
EOF

  # Set secure permissions
  chmod 600 "$secrets_file"

  printf "  ${COLOR_GREEN}✓${COLOR_RESET} POSTGRES_PASSWORD:           %d chars\n" "${#postgres_pw}"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} HASURA_GRAPHQL_ADMIN_SECRET: %d chars\n" "${#hasura_secret}"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} AUTH_JWT_SECRET:             %d chars\n" "${#jwt_secret}"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} REDIS_PASSWORD:              %d chars\n" "${#redis_pw}"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} MINIO_ROOT_PASSWORD:         %d chars\n" "${#minio_pw}"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} MEILISEARCH_MASTER_KEY:      %d chars\n" "${#meili_key}"

  printf "\n"
  log_success "Secrets saved to: $secrets_file"
  printf "  ${COLOR_DIM}Permissions set to 600 (owner read/write only)${COLOR_RESET}\n"
}

# Export functions
export -f security::preflight
export -f security::check_secrets
export -f security::check_password_strength
export -f security::check_service_bindings
export -f security::check_ssl_config
export -f security::check_insecure_defaults
export -f security::check_production_services
export -f security::generate_secrets
