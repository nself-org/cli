#!/usr/bin/env bash
# hardening.sh - Security hardening functions

set -euo pipefail

SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SECURITY_LIB_DIR/../../.." && pwd)"

source "$NSELF_ROOT/src/lib/utils/cli-output.sh" 2>/dev/null || true

# Rotate weak secrets
harden_secrets() {
  cli_info "Rotating weak secrets..."

  local env_file=".env"

  if [[ ! -f "$env_file" ]]; then
    cli_warning "Environment file not found: $env_file"
    return 1
  fi

  # Source secret generation
  if [[ -f "$NSELF_ROOT/src/lib/init/secrets-gen.sh" ]]; then
    source "$NSELF_ROOT/src/lib/init/secrets-gen.sh"
  else
    cli_error "Secret generation module not found"
    return 1
  fi

  local env="${ENV:-dev}"
  local rotated=0

  # Secret generation lengths based on environment
  local postgres_length=32
  local hasura_length=64
  local jwt_length=64
  local minio_length=32

  if [[ "$env" == "production" ]] || [[ "$env" == "prod" ]]; then
    postgres_length=48
    hasura_length=96
    jwt_length=96
    minio_length=48
  fi

  # Define weak patterns (parallel arrays - Bash 3.2 compatible)
  local secret_keys=(
    "POSTGRES_PASSWORD"
    "HASURA_GRAPHQL_ADMIN_SECRET"
    "HASURA_JWT_KEY"
    "MINIO_ROOT_PASSWORD"
    "GRAFANA_ADMIN_PASSWORD"
    "MEILISEARCH_MASTER_KEY"
  )

  local weak_patterns=(
    "minioadmin|changeme|admin|password123|secret|dev-password|test|demo|postgres"
    "hasura|admin-secret|secret|changeme|test|demo"
    "minioadmin|changeme|secret|test|demo|development-secret"
    "minioadmin|changeme|admin|password|secret|test|demo"
    "admin|grafana|changeme|password|secret|test|demo"
    "changeme|secret|test|demo"
  )

  # Check and rotate each secret
  local i=0
  for secret_key in "${secret_keys[@]}"; do
    local weak_pattern="${weak_patterns[$i]}"

    if grep -qiE "^${secret_key}=.*($weak_pattern)" "$env_file" 2>/dev/null; then
      # Generate new strong secret
      local new_value
      case "$secret_key" in
        *PASSWORD*)
          new_value=$(generate_random_secret "$postgres_length" "alphanumeric")
          ;;
        *SECRET*|*KEY*)
          new_value=$(generate_random_secret "$hasura_length" "hex")
          ;;
        *)
          new_value=$(generate_random_secret "$postgres_length" "hex")
          ;;
      esac

      # Replace the secret (platform-safe sed)
      if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s|^${secret_key}=.*|${secret_key}=${new_value}|" "$env_file"
      else
        sed -i "s|^${secret_key}=.*|${secret_key}=${new_value}|" "$env_file"
      fi

      cli_success "Rotated: $secret_key"
      rotated=$((rotated + 1))
    elif grep -qE "^${secret_key}=.{1,23}$" "$env_file" 2>/dev/null; then
      # Also check for short secrets (less than 24 chars)
      local new_value
      case "$secret_key" in
        *PASSWORD*)
          new_value=$(generate_random_secret "$postgres_length" "alphanumeric")
          ;;
        *SECRET*|*KEY*)
          new_value=$(generate_random_secret "$hasura_length" "hex")
          ;;
        *)
          new_value=$(generate_random_secret "$postgres_length" "hex")
          ;;
      esac

      if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s|^${secret_key}=.*|${secret_key}=${new_value}|" "$env_file"
      else
        sed -i "s|^${secret_key}=.*|${secret_key}=${new_value}|" "$env_file"
      fi

      cli_success "Strengthened: $secret_key"
      rotated=$((rotated + 1))
    fi

    i=$((i + 1))
  done

  if [[ $rotated -eq 0 ]]; then
    cli_success "All secrets are already strong"
  else
    cli_success "Rotated $rotated weak secret(s)"
  fi
}

# Fix CORS configuration
harden_cors() {
  cli_info "Hardening CORS configuration..."

  local env="${ENV:-dev}"
  local env_file=".env"

  if [[ ! -f "$env_file" ]]; then
    cli_warning "Environment file not found: $env_file"
    return 1
  fi

  if [[ "$env" == "production" ]]; then
    # Remove wildcard CORS
    if grep -q "CORS.*\\*" "$env_file" 2>/dev/null; then
      # Use platform-safe sed
      if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' '/CORS.*\*/d' "$env_file"
      else
        sed -i '/CORS.*\*/d' "$env_file"
      fi

      # Add secure CORS domain
      local base_domain="${BASE_DOMAIN:-localhost}"
      printf "HASURA_GRAPHQL_CORS_DOMAIN=https://*.%s\n" "$base_domain" >> "$env_file"
    fi
  fi

  cli_success "CORS configuration hardened"
}

# Interactive wizard
harden_interactive() {
  cli_header "Security Hardening Wizard"
  echo ""

  cli_info "Running security audit..."
  if [[ -f "$NSELF_ROOT/src/lib/security/audit-checks.sh" ]]; then
    source "$NSELF_ROOT/src/lib/security/audit-checks.sh"
    run_security_audit || true
  fi

  echo ""
  printf "Apply automatic hardening fixes? (y/N): "
  read -r response

  response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
  if [[ "$response" =~ ^[yY] ]]; then
    harden_all
  else
    cli_info "Hardening cancelled"
  fi
}

# Harden everything
harden_all() {
  cli_header "Applying Security Hardening"

  harden_secrets
  harden_cors

  echo ""
  cli_success "Security hardening complete!"
  cli_info "Rebuild and restart: nself build && nself restart"
}

# Export functions
export -f harden_secrets
export -f harden_cors
export -f harden_interactive
export -f harden_all
