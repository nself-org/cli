#!/usr/bin/env bash

# env-detection.sh - Environment detection and management utilities
# Provides centralized environment detection with NSELF_ENV support

# Detect the current environment based on multiple sources
# Priority order:
#   1. TARGET_ENV (from command line flags)
#   2. NSELF_ENV (machine-level environment variable)
#   3. ENV from .env file (read from file, not environment variable)
#   4. Default to "dev"
detect_environment() {

set -euo pipefail

  local env=""

  # First check command-line TARGET_ENV (highest priority)
  if [[ -n "${TARGET_ENV:-}" ]]; then
    env="$TARGET_ENV"
  # Then check machine-level NSELF_ENV
  elif [[ -n "${NSELF_ENV:-}" ]]; then
    env="$NSELF_ENV"
  # CRITICAL (Bug #16 fix): Read ENV from .env file, not environment variable
  # This ensures we use the environment specified in .env, not a stale ENV var
  elif [[ -f ".env" ]]; then
    env=$(grep "^ENV=" .env 2>/dev/null | cut -d'=' -f2 | tr -d ' ' | tr -d '"' | tr -d "'")
  fi

  # Fallback: check ENV variable if .env didn't have it
  if [[ -z "$env" ]] && [[ -n "${ENV:-}" ]]; then
    env="$ENV"
  fi

  # Default to dev if still empty
  if [[ -z "$env" ]]; then
    env="dev"
  fi

  # Normalize environment names
  case "$env" in
    development | develop | devel | dev)
      echo "dev"
      ;;
    production | prod)
      echo "prod"
      ;;
    staging | stage)
      echo "staging"
      ;;
    *)
      echo "$env"
      ;;
  esac
}

# Get environment file cascade for a specific environment
# Returns the ordered list of env files to load
get_env_file_cascade() {
  local target_env="${1:-$(detect_environment)}"
  local files=()

  # Always start with .env.dev as base (team defaults)
  files+=(".env.dev")

  # Add environment-specific files based on target
  case "$target_env" in
    dev | development)
      # For dev environment, add .env.local for per-developer overrides
      files+=(".env.local")
      ;;
    staging)
      files+=(".env.staging")
      ;;
    prod | production)
      files+=(".env.staging") # Staging configs are base for prod
      files+=(".env.prod")
      files+=(".env.secrets")
      ;;
  esac

  # Always end with .env for local overrides (highest priority)
  files+=(".env")

  # Return space-separated list
  echo "${files[@]}"
}

# Load environment variables with proper cascading
# Usage: cascade_env_vars [target_env]
cascade_env_vars() {
  local target_env="${1:-$(detect_environment)}"
  local files=$(get_env_file_cascade "$target_env")
  local loaded=false

  for file in $files; do
    if [[ -f "$file" ]]; then
      if command -v safe_source_env >/dev/null 2>&1; then
        safe_source_env "$file"
      else
        set -a
        # Temporarily disable set -u to handle env files that reference
        # undefined variables (e.g. ${STAGING_SECRET} placeholders).
        # Use a group command so exports propagate to the current shell.
        { set +u; source "$file"; } 2>/dev/null || true
        set +a
      fi
      loaded=true
    fi
  done

  # Export the determined environment
  export ENV="$target_env"

  # Ensure PROJECT_NAME is set
  ensure_project_context

  return 0
}

# Run command with specific environment
# Usage: with_env <environment> <command>
# Example: with_env prod docker compose ps
with_env() {
  local target_env="$1"
  shift

  # Save current environment
  local original_env="${ENV:-}"
  local temp_env_file
  temp_env_file=$(mktemp /tmp/nself_env_backup.XXXXXX)
  trap "rm -f '$temp_env_file'" EXIT
  env >"$temp_env_file"

  # Load target environment
  cascade_env_vars "$target_env"

  # Run command
  "$@"
  local exit_code=$?

  # Restore original environment
  # Clear all current env vars
  while IFS='=' read -r key value; do
    unset "$key" 2>/dev/null || true
  done < <(env | cut -d= -f1)

  # Restore from backup
  set -a
  source "$temp_env_file" 2>/dev/null || true
  set +a

  # Cleanup
  rm -f "$temp_env_file"

  return $exit_code
}

# Get environment-specific domain
# Usage: get_domain_for_env [environment]
get_domain_for_env() {
  local target_env="${1:-$(detect_environment)}"

  # Check for environment-specific domain variables
  case "$target_env" in
    dev)
      echo "${DEV_DOMAIN:-${BASE_DOMAIN:-localhost}}"
      ;;
    staging)
      echo "${STAGING_DOMAIN:-${BASE_DOMAIN:-localhost}}"
      ;;
    prod | production)
      echo "${PROD_DOMAIN:-${PRODUCTION_DOMAIN:-${BASE_DOMAIN:-localhost}}}"
      ;;
    *)
      echo "${BASE_DOMAIN:-localhost}"
      ;;
  esac
}

# Get environment-specific route
# Usage: get_route_for_env <service> [environment]
# Example: get_route_for_env "api" "prod"
get_route_for_env() {
  local service="$1"
  local target_env="${2:-$(detect_environment)}"
  local domain=$(get_domain_for_env "$target_env")

  # Convert to uppercase for Bash 3.2 compatibility
  local service_upper=$(echo "$service" | tr '[:lower:]' '[:upper:]')
  local env_upper=$(echo "$target_env" | tr '[:lower:]' '[:upper:]')

  # Check for service-specific route overrides
  local route_var="${service_upper}_ROUTE"
  local env_route_var="${service_upper}_${env_upper}_ROUTE"

  # First check environment-specific route
  if [[ -n "${!env_route_var:-}" ]]; then
    echo "${!env_route_var}"
  # Then check general route
  elif [[ -n "${!route_var:-}" ]]; then
    echo "${!route_var}"
  # Default to subdomain pattern
  else
    echo "${service}.${domain}"
  fi
}

# Print effective environment (for debugging)
# Usage: print_effective_env [environment]
print_effective_env() {
  local target_env="${1:-$(detect_environment)}"

  echo "# Effective environment: $target_env"
  echo "# Machine NSELF_ENV: ${NSELF_ENV:-not set}"
  echo "# ENV variable: ${ENV:-not set}"
  echo "# Detected as: $(detect_environment)"
  echo "#"
  echo "# Cascade order:"
  local files=$(get_env_file_cascade "$target_env")
  for file in $files; do
    if [[ -f "$file" ]]; then
      echo "#   ✓ $file (exists)"
    else
      echo "#   ✗ $file (missing)"
    fi
  done
  echo "#"
  echo "# Effective domain: $(get_domain_for_env "$target_env")"
  echo "# API route: $(get_route_for_env "api" "$target_env")"
  echo ""
}

# Check if running in specific environment
is_env() {
  local check_env="$1"
  local current_env=$(detect_environment)
  [[ "$current_env" == "$check_env" ]]
}

# Export functions
export -f detect_environment
export -f get_env_file_cascade
export -f cascade_env_vars
export -f with_env
export -f get_domain_for_env
export -f get_route_for_env
export -f print_effective_env
export -f is_env
