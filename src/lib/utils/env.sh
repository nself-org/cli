#!/usr/bin/env bash


# Environment loading and management utilities

set -euo pipefail

# Export colors only when writing to a real terminal and NO_COLOR is not set
if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  export COLOR_RESET=$'\033[0m'
  export COLOR_BOLD=$'\033[1m'
  export COLOR_RED=$'\033[0;31m'
  export COLOR_GREEN=$'\033[0;32m'
  export COLOR_YELLOW=$'\033[0;33m'
  export COLOR_BLUE=$'\033[0;34m'
  export COLOR_MAGENTA=$'\033[0;35m'
  export COLOR_CYAN=$'\033[0;36m'
  export COLOR_DIM=$'\033[2m'
else
  export COLOR_RESET=""
  export COLOR_BOLD=""
  export COLOR_RED=""
  export COLOR_GREEN=""
  export COLOR_YELLOW=""
  export COLOR_BLUE=""
  export COLOR_MAGENTA=""
  export COLOR_CYAN=""
  export COLOR_DIM=""
fi

# Source the new environment detection module if available
ENV_UTILS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$ENV_UTILS_DIR/env-detection.sh" ]]; then
  source "$ENV_UTILS_DIR/env-detection.sh"
fi

# Safe environment file loader - parses line-by-line instead of sourcing
# Prevents unquoted values with spaces (e.g., CMD=pnpm dev) from being
# executed as shell commands. Sets _SAFE_SOURCE_ENV_COUNT with vars loaded.
safe_source_env() {
  local env_file="$1"
  [[ ! -f "$env_file" ]] && return 1

  _SAFE_SOURCE_ENV_COUNT=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    # Skip lines without = (not KEY=VALUE)
    [[ "$line" != *=* ]] && continue

    # Extract key (everything before first =)
    local key="${line%%=*}"
    # Trim leading/trailing whitespace from key
    key="${key#"${key%%[![:space:]]*}"}"
    key="${key%"${key##*[![:space:]]}"}"

    # Validate key format (must be valid env var name)
    [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] && continue

    # Extract value (everything after first =)
    local value="${line#*=}"

    # Strip surrounding double quotes
    if [[ "$value" =~ ^\"(.*)\"$ ]]; then
      value="${BASH_REMATCH[1]}"
    # Strip surrounding single quotes
    elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
      value="${BASH_REMATCH[1]}"
    fi

    export "$key=$value"
    _SAFE_SOURCE_ENV_COUNT=$((_SAFE_SOURCE_ENV_COUNT + 1))
  done < "$env_file"

  return 0
}

# Load environment files with correct priority order
# CONFIGURATION PHILOSOPHY:
#   • Smart Defaults: Everything works without changes
#   • Auto-Configuration: System adapts based on ENV
#   • Full Control: Power users can override ANY setting
#
# File Loading Order (later overrides earlier):
#   1) .env.dev     (team defaults, SHARED)
#   2) .env.staging (staging only config, SHARED) - if ENV=staging
#   3) .env.prod    (production only config, SHARED) - if ENV=prod
#   4) .env.secrets (production secrets, not shared) - if ENV=prod
#   5) .env         (LOCAL ONLY priority overrides) - HIGHEST PRIORITY
load_env_with_priority() {
  local silent="${1:-false}"
  local verbose="${VERBOSE:-false}"
  local total_vars=0
  local files_loaded=0

  # Show cascade visualization if verbose mode enabled
  if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
    printf "\n${COLOR_CYAN}Environment Cascade:${COLOR_RESET}\n"
  fi

  # Use the new cascading function if available
  if command -v cascade_env_vars >/dev/null 2>&1; then
    # Detect environment using NSELF_ENV or ENV
    local target_env=$(detect_environment)
    cascade_env_vars "$target_env"
    return 0
  fi

  # Fallback to original implementation if new module not available
  local loaded=false

  # STEP 1: Always load .env.dev as the base (team defaults)
  if [[ -f ".env.dev" ]]; then
    safe_source_env ".env.dev"
    local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
    loaded=true
    files_loaded=$((files_loaded + 1))
    total_vars=$((total_vars + loaded_count))
    if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
      printf "  %d. Loaded: .env.dev (%d vars)\n" "$files_loaded" "$loaded_count"
    fi
  fi

  # STEP 1.5: Load .env.local if it exists (legacy/alternative to .env)
  if [[ -f ".env.local" ]]; then
    safe_source_env ".env.local"
    local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
    loaded=true
    files_loaded=$((files_loaded + 1))
    total_vars=$((total_vars + loaded_count))
    if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
      printf "  %d. Loaded: .env.local (%d vars)\n" "$files_loaded" "$loaded_count"
    fi
  fi

  # STEP 2: Detect environment using NSELF_ENV first, then ENV
  local current_env="${NSELF_ENV:-${ENV:-dev}}"

  # Normalize environment names
  case "$current_env" in
    development | develop | devel)
      current_env="dev"
      export ENV="dev"
      ;;
    production | prod)
      current_env="prod"
      export ENV="prod"
      ;;
    staging | stage)
      current_env="staging"
      export ENV="staging"
      ;;
  esac

  # STEP 3: Load environment-specific overrides based on ENV
  case "$current_env" in
    staging | stage)
      # For staging: .env.dev -> .env.staging
      if [[ -f ".env.staging" ]]; then
        safe_source_env ".env.staging"
        local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
        loaded=true
        files_loaded=$((files_loaded + 1))
        total_vars=$((total_vars + loaded_count))
        if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
          printf "  %d. Loaded: .env.staging (%d vars)\n" "$files_loaded" "$loaded_count"
        fi
      fi
      ;;

    prod | production)
      # For production: .env.dev -> .env.staging -> .env.prod -> .env.secrets
      if [[ -f ".env.staging" ]]; then
        safe_source_env ".env.staging"
        local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
        loaded=true
        files_loaded=$((files_loaded + 1))
        total_vars=$((total_vars + loaded_count))
        if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
          printf "  %d. Loaded: .env.staging (%d vars)\n" "$files_loaded" "$loaded_count"
        fi
      fi

      if [[ -f ".env.prod" ]]; then
        safe_source_env ".env.prod"
        local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
        loaded=true
        files_loaded=$((files_loaded + 1))
        total_vars=$((total_vars + loaded_count))
        if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
          printf "  %d. Loaded: .env.prod (%d vars)\n" "$files_loaded" "$loaded_count"
        fi
      fi

      if [[ -f ".env.secrets" ]]; then
        safe_source_env ".env.secrets"
        local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
        loaded=true
        files_loaded=$((files_loaded + 1))
        total_vars=$((total_vars + loaded_count))
        if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
          printf "  %d. Loaded: .env.secrets (%d vars)\n" "$files_loaded" "$loaded_count"
        fi
      fi
      ;;

    dev | development | *)
      # For dev or any other env: just .env.dev (already loaded)
      ;;
  esac

  # STEP 4: Load .env as the FINAL override (HIGHEST PRIORITY)
  # This allows local overrides of ANY setting regardless of environment
  if [[ -f ".env" ]]; then
    safe_source_env ".env"
    local loaded_count=${_SAFE_SOURCE_ENV_COUNT:-0}
    loaded=true
    files_loaded=$((files_loaded + 1))
    total_vars=$((total_vars + loaded_count))
    if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
      printf "  %d. Loaded: .env (%d vars)\n" "$files_loaded" "$loaded_count"
    fi
  fi

  # Show summary if verbose
  if [[ "$verbose" == "true" ]] && [[ "$silent" != "true" ]]; then
    printf "  ${COLOR_GREEN}Final: %d files loaded, %d variables total${COLOR_RESET}\n\n" "$files_loaded" "$total_vars"
  fi

  # Ensure PROJECT_NAME is always set after loading
  ensure_project_context

  # CRITICAL: Construct and export derived environment variables for docker-compose
  # These variables are required by services in docker-compose.yml but aren't
  # directly set in .env files - they must be constructed from other variables

  # DATABASE_URL - Required by Hasura, Functions, and other services
  if [[ -n "${POSTGRES_USER:-}" ]] && [[ -n "${POSTGRES_PASSWORD:-}" ]] && [[ -n "${POSTGRES_DB:-}" ]]; then
    export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}"
  fi

  return 0
}

# Get environment variable with default
get_env_var() {
  local var_name="$1"
  local default_value="${2:-}"

  local value="${!var_name:-$default_value}"
  echo "$value"
}

# Set environment variable in file
set_env_var() {
  local var_name="$1"
  local value="$2"
  local file="${3:-.env}"

  # Check if variable already exists
  if grep -q "^${var_name}=" "$file" 2>/dev/null; then
    # Update existing variable
    if [[ "$OSTYPE" == "darwin"* ]]; then
      sed -i '' "s|^${var_name}=.*|${var_name}=${value}|" "$file"
    else
      sed -i "s|^${var_name}=.*|${var_name}=${value}|" "$file"
    fi
  else
    # Add new variable (ensure newline before if needed)
    # Check if file ends with newline
    if [[ -f "$file" ]] && [[ -s "$file" ]] && [[ $(tail -c 1 "$file" | wc -l) -eq 0 ]]; then
      echo "" >>"$file"
    fi
    echo "${var_name}=${value}" >>"$file"
  fi
}

# Check if environment variable is set
is_env_set() {
  local var_name="$1"
  [[ -n "${!var_name}" ]]
}

# Get environment type (dev/staging/prod)
get_environment() {
  local env="${ENV:-dev}"

  case "$env" in
    dev | development)
      echo "dev"
      ;;
    staging | stage)
      echo "staging"
      ;;
    prod | production)
      echo "prod"
      ;;
    *)
      echo "$env"
      ;;
  esac
}

# Check if running in production
is_production() {
  local env=$(get_environment)
  [[ "$env" == "prod" ]] || [[ "$env" == "production" ]]
}

# Check if running in development
is_development() {
  local env=$(get_environment)
  [[ "$env" == "dev" ]] || [[ "$env" == "development" ]]
}

# Export environment from file (uses safe parser to avoid command execution)
export_env_from_file() {
  local file="$1"

  if [[ ! -f "$file" ]]; then
    return 1
  fi

  safe_source_env "$file"
  return 0
}

# Validate required environment variables
validate_required_env() {
  local required_vars=("$@")
  local missing=()

  for var in "${required_vars[@]}"; do
    if ! is_env_set "$var"; then
      missing+=("$var")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "Missing required environment variables:"
    printf '  - %s\n' "${missing[@]}"
    return 1
  fi

  return 0
}

# Get all env vars with prefix
get_env_vars_with_prefix() {
  local prefix="$1"
  env | grep "^${prefix}" | cut -d= -f1
}

# Clear env vars with prefix
clear_env_vars_with_prefix() {
  local prefix="$1"

  for var in $(get_env_vars_with_prefix "$prefix"); do
    unset "$var"
  done
}

# Ensure PROJECT_NAME is set with auto-generation if needed
ensure_project_name() {
  if [[ -z "${PROJECT_NAME:-}" ]]; then
    # First, check if PROJECT_NAME already exists in .env — respect existing value
    if [[ -f ".env" ]]; then
      local file_value
      file_value=$(grep "^PROJECT_NAME=" ".env" 2>/dev/null | head -1 | cut -d= -f2-)
      if [[ -n "$file_value" ]]; then
        export PROJECT_NAME="$file_value"
        return 0
      fi
    fi

    # No value in shell or .env — fall back to directory name
    local dir_name=""
    if command -v pwd >/dev/null 2>&1; then
      dir_name=$(basename "$(pwd 2>/dev/null)" 2>/dev/null) || dir_name=""
    fi

    # If we couldn't get directory name, use a default
    if [[ -z "$dir_name" ]]; then
      dir_name="my-project"
    fi

    # Clean it up to be valid (alphanumeric and hyphens only)
    local clean_name=$(echo "$dir_name" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]/-/g' | sed 's/^-*//' | sed 's/-*$//')

    if [[ -z "$clean_name" ]] || [[ "$clean_name" == "." ]] || [[ "$clean_name" == "-" ]]; then
      clean_name="my-project"
    fi

    export PROJECT_NAME="$clean_name"

    # If we have a .env file, add it there too (only if not already present)
    if [[ -f ".env" ]] && ! grep -q "^PROJECT_NAME=" ".env"; then
      echo "PROJECT_NAME=$clean_name" >>".env"
    fi
  fi
}

# Ensure we have a valid project context
ensure_project_context() {
  # Ensure PROJECT_NAME is set
  ensure_project_name

  # Validate PROJECT_NAME format (Docker allows lowercase, numbers, underscore, hyphen)
  if [[ ! "$PROJECT_NAME" =~ ^[a-z][a-z0-9_-]*$ ]]; then
    # Use stderr to avoid breaking pipes/subshells
    echo "Warning: PROJECT_NAME '$PROJECT_NAME' contains invalid characters. Using 'my-project' instead." >&2
    export PROJECT_NAME="my-project"
  fi

  return 0
}

# Load environment for specific service
load_service_env() {
  local service="$1"
  local env_file=".env.${service}"

  if [[ -f "$env_file" ]]; then
    export_env_from_file "$env_file"
    return 0
  fi

  return 1
}

# Get cascaded environment variables for a specific environment
# Usage: get_cascaded_vars [environment]
# Returns: Exports all cascaded environment variables
# Example: get_cascaded_vars dev
#          get_cascaded_vars prod
get_cascaded_vars() {
  local target_env="${1:-${ENV:-dev}}"

  # Normalize environment names
  case "$target_env" in
    development | develop | devel)
      target_env="dev"
      ;;
    production | prod)
      target_env="prod"
      ;;
    staging | stage)
      target_env="staging"
      ;;
  esac

  # Always start with .env.dev as base (team defaults)
  [[ -f ".env.dev" ]] && safe_source_env ".env.dev"

  # Load environment-specific files based on target_env
  case "$target_env" in
    staging)
      # For staging: .env.dev -> .env.staging -> .env
      [[ -f ".env.staging" ]] && safe_source_env ".env.staging"
      ;;

    prod | production)
      # For production: .env.dev -> .env.staging -> .env.prod -> .env.secrets -> .env
      [[ -f ".env.staging" ]] && safe_source_env ".env.staging"
      [[ -f ".env.prod" ]] && safe_source_env ".env.prod"
      [[ -f ".env.secrets" ]] && safe_source_env ".env.secrets"
      ;;

    dev | development | *)
      # For dev: just .env.dev -> .env
      ;;
  esac

  # Always load .env last as highest priority override
  [[ -f ".env" ]] && safe_source_env ".env"

  # Export the determined environment
  export ENV="$target_env"

  return 0
}

# Print cascaded environment variables (for debugging)
# Usage: print_cascaded_vars [environment]
print_cascaded_vars() {
  local target_env="${1:-${ENV:-dev}}"

  # Save current environment
  local temp_env_file
  temp_env_file=$(mktemp /tmp/nself_env_before.XXXXXX)
  local temp_env_after
  temp_env_after=$(mktemp /tmp/nself_env_after.XXXXXX)
  trap "rm -f '$temp_env_file' '$temp_env_after'" EXIT
  env >"$temp_env_file"

  # Load cascaded vars
  get_cascaded_vars "$target_env"

  # Show only the vars that were set/changed
  env >"$temp_env_after"

  echo "# Cascaded environment for: $target_env"
  echo "# Cascade order:"
  echo "#   - .env.dev (base defaults)"
  case "$target_env" in
    staging)
      echo "#   - .env.staging"
      ;;
    prod | production)
      echo "#   - .env.staging"
      echo "#   - .env.prod"
      echo "#   - .env.secrets"
      ;;
  esac
  echo "#   - .env (local overrides)"
  echo ""

  # Show the differences
  comm -13 <(sort "$temp_env_file") <(sort "$temp_env_after") 2>/dev/null || {
    # Fallback if comm is not available
    diff "$temp_env_file" "$temp_env_after" | grep "^>" | sed 's/^> //'
  }

  # Cleanup
  rm -f "$temp_env_file" "$temp_env_after"

  return 0
}

# Export functions
export -f safe_source_env
export -f load_env_with_priority
export -f get_env_var
export -f set_env_var
export -f is_env_set
export -f get_environment
export -f is_production
export -f is_development
export -f export_env_from_file
export -f validate_required_env
export -f get_env_vars_with_prefix
export -f clear_env_vars_with_prefix
export -f load_service_env
export -f get_cascaded_vars
export -f print_cascaded_vars
