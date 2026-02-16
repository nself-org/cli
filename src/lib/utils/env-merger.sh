#!/usr/bin/env bash

# env-merger.sh - Environment merging and runtime file generation
# Cascades and merges environment files based on target environment
# Generates .env.runtime for docker-compose with fully resolved values
#
# IMPORTANT: DATABASE_URL Dual Strategy
# - .env files use @localhost:5432 (for external tools: plugins, nself db commands)
# - .env.runtime uses @postgres:5432 (for Docker internal networking)
# This file ALWAYS overrides DATABASE_URL in .env.runtime to use service names

# Get script directory (namespaced to avoid clobbering caller's SCRIPT_DIR)
_ENV_MERGER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source env-detection for helper functions
if [[ -f "$_ENV_MERGER_DIR/env-detection.sh" ]]; then
  source "$_ENV_MERGER_DIR/env-detection.sh"
fi

# Merge environment files and generate runtime configuration
# Usage: merge_environments <target_env> [output_file]
merge_environments() {
  local target_env="${1:-dev}"
  local output_file="${2:-.env.runtime}"
  local temp_file="/tmp/nself_env_merge_$$"

  # Clear temp file
  >"$temp_file"

  # Determine cascade order based on target environment
  local env_files=()

  # Always start with .env.dev as base (shared team defaults)
  env_files+=(".env.dev")

  case "$target_env" in
    staging | stage)
      # Staging: dev → staging → local overrides
      env_files+=(".env.staging")
      ;;
    prod | production)
      # Production: dev → staging → prod → secrets → local overrides
      env_files+=(".env.staging")
      env_files+=(".env.prod")
      env_files+=(".env.secrets")
      ;;
    dev | development | *)
      # Dev: just dev → local overrides (default)
      ;;
  esac

  # Always end with .env for local overrides (highest priority)
  env_files+=(".env")

  # Track loaded files for reporting
  local loaded_files=()

  # Load and merge each file
  for env_file in "${env_files[@]}"; do
    if [[ -f "$env_file" ]]; then
      # Read file and append to temp, later values override earlier ones
      while IFS= read -r line || [[ -n "$line" ]]; do
        # Skip empty lines and comments
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

        # Extract key if it's a key=value pair
        if [[ "$line" =~ ^([A-Z_][A-Z0-9_]*)= ]]; then
          local key="${BASH_REMATCH[1]}"
          # Remove any existing occurrence of this key
          sed -i.bak "/^${key}=/d" "$temp_file" 2>/dev/null || true
        fi

        # Add the line
        echo "$line" >>"$temp_file"
      done <"$env_file"

      loaded_files+=("$env_file")
    fi
  done

  # Add computed runtime values based on environment
  add_computed_values "$target_env" "$temp_file"

  # Add critical smart defaults (pass target_env for environment-specific defaults)
  add_smart_defaults "$temp_file" "$target_env"

  # Generate final runtime file with header
  {
    echo "# Generated runtime environment for: $target_env"
    echo "# Generated at: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "# Cascade order: ${loaded_files[*]}"
    echo ""

    # Ensure ENV is set correctly
    echo "ENV=$target_env"

    # Sort and output all values
    sort "$temp_file" | while IFS= read -r line; do
      # Skip ENV line as we already added it
      [[ "$line" =~ ^ENV= ]] && continue
      echo "$line"
    done
  } >"$output_file"

  # Cleanup
  rm -f "$temp_file" "$temp_file.bak"

  # Report what was loaded
  echo "Environment cascade for '$target_env':"
  for file in "${loaded_files[@]}"; do
    echo "  ✓ Loaded: $file"
  done
  echo "  → Generated: $output_file"
}

# Add smart defaults for critical runtime variables
add_smart_defaults() {
  local output_file="$1"
  local target_env="${2:-dev}"  # Accept environment parameter for context-aware defaults

  # Helper to check if variable exists in file
  var_exists() {
    grep -q "^${1}=" "$output_file" 2>/dev/null
  }

  # Helper to get variable value
  get_var() {
    grep "^${1}=" "$output_file" 2>/dev/null | cut -d= -f2- | tail -1
  }

  # PROJECT_NAME with smart default
  if ! var_exists "PROJECT_NAME"; then
    echo "PROJECT_NAME=myproject" >>"$output_file"
  fi

  local project_name="$(get_var PROJECT_NAME)"

  # DOCKER_NETWORK is critical - always ensure it's set
  if ! var_exists "DOCKER_NETWORK"; then
    echo "DOCKER_NETWORK=${project_name}_network" >>"$output_file"
  fi

  # Add POSTGRES_HOST if not set
  if ! grep -q "^POSTGRES_HOST=" "$output_file"; then
    echo "POSTGRES_HOST=postgres" >>"$output_file"
  fi

  # Database defaults
  var_exists "POSTGRES_USER" || echo "POSTGRES_USER=postgres" >>"$output_file"
  var_exists "POSTGRES_PASSWORD" || echo "POSTGRES_PASSWORD=postgres" >>"$output_file"
  var_exists "POSTGRES_DB" || echo "POSTGRES_DB=$project_name" >>"$output_file"
  var_exists "POSTGRES_PORT" || echo "POSTGRES_PORT=5432" >>"$output_file"

  # CRITICAL: Always override DATABASE_URL for Docker internal networking
  # .env files may have @localhost (correct for external tools like plugins)
  # But Docker services MUST use @postgres service name
  # Remove any existing DATABASE_URL and set the correct one
  sed -i.bak "/^DATABASE_URL=/d" "$output_file" 2>/dev/null || true

  local db_user="$(get_var POSTGRES_USER)"
  local db_pass="$(get_var POSTGRES_PASSWORD)"
  local db_name="$(get_var POSTGRES_DB)"
  echo "DATABASE_URL=postgresql://${db_user}:${db_pass}@postgres:5432/${db_name}" >>"$output_file"

  # Auth service defaults
  var_exists "AUTH_SMTP_USER" || echo "AUTH_SMTP_USER=" >>"$output_file"
  var_exists "AUTH_SMTP_PASS" || echo "AUTH_SMTP_PASS=" >>"$output_file"
  var_exists "AUTH_JWT_SECRET" || echo "AUTH_JWT_SECRET=change-this-secret-in-production" >>"$output_file"
  var_exists "AUTH_JWT_TYPE" || echo "AUTH_JWT_TYPE=HS256" >>"$output_file"
  var_exists "AUTH_JWT_KEY" || echo "AUTH_JWT_KEY=change-this-secret-in-production" >>"$output_file"

  # Redis defaults (empty password is okay)
  var_exists "REDIS_PASSWORD" || echo "REDIS_PASSWORD=" >>"$output_file"

  # Hasura defaults
  var_exists "HASURA_GRAPHQL_ADMIN_SECRET" || echo "HASURA_GRAPHQL_ADMIN_SECRET=change-this-secret" >>"$output_file"

  # CRITICAL: Hasura CORS domain (REQUIRED for Hasura to start)
  # Environment-specific defaults to prevent wildcard CORS in production
  if ! var_exists "HASURA_GRAPHQL_CORS_DOMAIN"; then
    local base_domain="$(get_var BASE_DOMAIN)"
    [ -z "$base_domain" ] && base_domain="localhost"

    case "$target_env" in
      prod | production)
        # Production: Only allow base domain, no wildcards
        echo "HASURA_GRAPHQL_CORS_DOMAIN=https://*.${base_domain}" >>"$output_file"
        ;;
      staging | stage)
        # Staging: Base domain + localhost for testing
        echo "HASURA_GRAPHQL_CORS_DOMAIN=https://*.${base_domain},http://localhost:3000" >>"$output_file"
        ;;
      dev | development | *)
        # Development: Permissive for convenience
        echo "HASURA_GRAPHQL_CORS_DOMAIN=http://localhost:*,http://*.${base_domain},https://*.${base_domain}" >>"$output_file"
        ;;
    esac
  fi

  # Hasura JWT configuration (JSON format required by auth service)
  if ! var_exists "HASURA_GRAPHQL_JWT_SECRET"; then
    local jwt_secret="$(get_var AUTH_JWT_SECRET)"
    local jwt_type="$(get_var AUTH_JWT_TYPE)"
    [ -z "$jwt_secret" ] && jwt_secret="change-this-secret-in-production"
    [ -z "$jwt_type" ] && jwt_type="HS256"
    echo "HASURA_GRAPHQL_JWT_SECRET={\"type\":\"$jwt_type\",\"key\":\"$jwt_secret\"}" >>"$output_file"
  fi

  # Base domain
  var_exists "BASE_DOMAIN" || echo "BASE_DOMAIN=localhost" >>"$output_file"

  # MeiliSearch env mapping (dev -> development, prod -> production)
  if ! var_exists "MEILI_ENV"; then
    local env_val="$(get_var ENV)"
    if [[ "$env_val" == "prod" || "$env_val" == "production" ]]; then
      echo "MEILI_ENV=production" >>"$output_file"
    else
      echo "MEILI_ENV=development" >>"$output_file"
    fi
  fi

  # Service ports
  var_exists "HASURA_PORT" || echo "HASURA_PORT=8080" >>"$output_file"
  var_exists "AUTH_PORT" || echo "AUTH_PORT=4000" >>"$output_file"
  var_exists "STORAGE_PORT" || echo "STORAGE_PORT=5000" >>"$output_file"
  var_exists "REDIS_PORT" || echo "REDIS_PORT=6379" >>"$output_file"
  var_exists "NGINX_PORT" || echo "NGINX_PORT=80" >>"$output_file"
  var_exists "NGINX_SSL_PORT" || echo "NGINX_SSL_PORT=443" >>"$output_file"
}

# Add computed values based on environment
add_computed_values() {
  local target_env="$1"
  local output_file="$2"

  # SSL Mode
  case "$target_env" in
    dev | development)
      echo "SSL_MODE=self-signed" >>"$output_file"
      echo "SSL_AUTO_TRUST=true" >>"$output_file"
      ;;
    staging | stage | prod | production)
      echo "SSL_MODE=lets-encrypt" >>"$output_file"
      echo "SSL_AUTO_TRUST=false" >>"$output_file"
      ;;
  esac

  # Database seeding
  case "$target_env" in
    dev | development)
      echo "SEED_DATABASE=true" >>"$output_file"
      echo "DEMO_USERS=true" >>"$output_file"
      echo "DEMO_CONTENT=true" >>"$output_file"
      echo "ENABLE_DEBUG=true" >>"$output_file"
      ;;
    staging | stage)
      echo "SEED_DATABASE=false" >>"$output_file"
      echo "DEMO_USERS=true" >>"$output_file"
      echo "DEMO_CONTENT=false" >>"$output_file"
      echo "ENABLE_DEBUG=false" >>"$output_file"
      ;;
    prod | production)
      echo "SEED_DATABASE=false" >>"$output_file"
      echo "DEMO_USERS=false" >>"$output_file"
      echo "DEMO_CONTENT=false" >>"$output_file"
      echo "ENABLE_DEBUG=false" >>"$output_file"
      ;;
  esac

  # Log levels
  case "$target_env" in
    dev | development)
      grep -q "^LOG_LEVEL=" "$output_file" || echo "LOG_LEVEL=debug" >>"$output_file"
      ;;
    staging | stage)
      grep -q "^LOG_LEVEL=" "$output_file" || echo "LOG_LEVEL=info" >>"$output_file"
      ;;
    prod | production)
      grep -q "^LOG_LEVEL=" "$output_file" || echo "LOG_LEVEL=warn" >>"$output_file"
      ;;
  esac

  # Node environment (for Node.js services)
  echo "NODE_ENV=$target_env" >>"$output_file"

  # Rails environment (for Ruby services)
  [[ "$target_env" == "prod" || "$target_env" == "production" ]] && echo "RAILS_ENV=production" >>"$output_file"
  [[ "$target_env" == "staging" || "$target_env" == "stage" ]] && echo "RAILS_ENV=staging" >>"$output_file"
  [[ "$target_env" == "dev" || "$target_env" == "development" ]] && echo "RAILS_ENV=development" >>"$output_file"

  # PHP environment
  echo "APP_ENV=$target_env" >>"$output_file"
}

# Extract all domains that need SSL certificates
extract_all_domains() {
  local env_file="${1:-.env.runtime}"
  local domains=()

  # Source the env file to get variables
  set -a
  source "$env_file" 2>/dev/null || true
  set +a

  # Base domain
  [[ -n "${BASE_DOMAIN:-}" ]] && domains+=("$BASE_DOMAIN")

  # System service routes
  for service in HASURA AUTH NSELF_ADMIN GRAFANA PROMETHEUS ALERTMANAGER; do
    local route_var="${service}_ROUTE"
    local route="${!route_var:-}"
    [[ -n "$route" && "$route" != "/" ]] && domains+=("${route}.${BASE_DOMAIN}")
  done

  # Custom service routes
  for i in {1..20}; do
    local route_var="CS_${i}_ROUTE"
    local public_var="CS_${i}_PUBLIC"
    local route="${!route_var:-}"
    local is_public="${!public_var:-false}"

    [[ "$is_public" == "true" && -n "$route" ]] && domains+=("${route}.${BASE_DOMAIN}")
  done

  # Frontend app routes
  for i in {1..10}; do
    local route_var="FRONTEND_APP_${i}_ROUTE"
    local route="${!route_var:-}"

    [[ -n "$route" ]] && domains+=("${route}.${BASE_DOMAIN}")
  done

  # Remove duplicates and return
  printf '%s\n' "${domains[@]}" | sort -u | tr '\n' ' '
}

# Validate environment configuration
validate_environment_config() {
  local target_env="$1"
  local runtime_file="${2:-.env.runtime}"
  local errors=()

  # Source runtime file
  set -a
  source "$runtime_file" 2>/dev/null || return 1
  set +a

  # Check ENV matches target
  if [[ "${ENV:-}" != "$target_env" ]]; then
    errors+=("ENV mismatch: expected '$target_env', got '${ENV:-}'")
  fi

  # Check required variables for production
  if [[ "$target_env" == "prod" || "$target_env" == "production" ]]; then
    # Check for hardcoded demo passwords
    if [[ "${POSTGRES_PASSWORD:-}" == *"demo"* || "${POSTGRES_PASSWORD:-}" == *"password"* ]]; then
      errors+=("Production database password appears to be a demo/test password")
    fi

    if [[ "${HASURA_GRAPHQL_ADMIN_SECRET:-}" == *"demo"* || "${HASURA_GRAPHQL_ADMIN_SECRET:-}" == *"secret"* ]]; then
      errors+=("Production Hasura admin secret appears to be insecure")
    fi

    # Check SSL is enabled
    if [[ "${SSL_ENABLED:-false}" != "true" ]]; then
      errors+=("SSL must be enabled in production")
    fi

    # Check debug is off
    if [[ "${DEBUG:-false}" == "true" ]]; then
      errors+=("Debug mode should be disabled in production")
    fi
  fi

  # Report errors
  if [[ ${#errors[@]} -gt 0 ]]; then
    echo "Environment validation failed:"
    for error in "${errors[@]}"; do
      echo "  ✗ $error"
    done
    return 1
  fi

  return 0
}

# Export functions
export -f merge_environments
export -f add_computed_values
export -f add_smart_defaults
export -f extract_all_domains
export -f validate_environment_config
