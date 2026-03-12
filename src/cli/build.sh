#!/usr/bin/env bash
set -euo pipefail

# build.sh - nself build command wrapper
# This is now a thin wrapper that delegates to modular components

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
LIB_DIR="$SCRIPT_DIR/../lib/build"
SECURITY_LIB_DIR="$SCRIPT_DIR/../lib/security"

# Check if build library exists
if [[ ! -d "$LIB_DIR" ]]; then
  echo "Error: Build library not found at $LIB_DIR" >&2
  exit 1
fi

# Debug output (only for build debugging, not application DEBUG)
if [[ "${BUILD_DEBUG:-}" == "true" ]]; then
  echo "DEBUG: LIB_DIR=$LIB_DIR" >&2
  echo "DEBUG: Loading core.sh..." >&2
fi

# Source the core build module
if [[ -f "$LIB_DIR/core.sh" ]]; then
  source "$LIB_DIR/core.sh"
else
  echo "Error: Core build module not found at $LIB_DIR/core.sh" >&2
  exit 1
fi

# Source security module for secure-by-default validation
if [[ -f "$SECURITY_LIB_DIR/secure-defaults.sh" ]]; then
  source "$SECURITY_LIB_DIR/secure-defaults.sh"
fi

# Display utilities are already sourced via core.sh

# Show help for build command
show_build_help() {
  echo "nself build - Generate project infrastructure and configuration"
  echo ""
  echo "Usage: nself build [OPTIONS]"
  echo ""
  echo "Description:"
  echo "  Generates Docker Compose files, SSL certificates, nginx configuration,"
  echo "  and all necessary infrastructure based on your .env settings."
  echo ""
  echo "Options:"
  echo "  -f, --force           Force rebuild of all components"
  echo "  --no-cache            Disable build cache (force full rebuild)"
  echo "  -h, --help            Show this help message"
  echo "  -v, --verbose         Show detailed output (environment cascade)"
  echo "  --debug               Enable debug mode"
  echo "  --security-report     Show comprehensive security analysis"
  echo "  --allow-insecure      Allow insecure config (DEV ONLY, not for prod)"
  echo "  --check               Validate config security without building (no Docker required)"
  echo ""
  echo "Examples:"
  echo "  nself build                    # Build with current configuration"
  echo "  nself build --force            # Force rebuild everything"
  echo "  nself build --no-cache         # Disable cache, rebuild all"
  echo "  nself build --verbose          # Show environment cascade details"
  echo "  nself build --security-report  # Get comprehensive security score"
  echo "  nself build --debug            # Build with debug output"
  echo ""
  echo "Files Generated:"
  echo "  • docker-compose.yml           • nginx/ configuration"
  echo "  • SSL certificates             • Database initialization"
  echo "  • Service templates            • Environment validation"
  echo ""
  echo "Security (Secure by Default):"
  echo "  • Validates all required passwords are set"
  echo "  • Ensures internal services bind to 127.0.0.1 only"
  echo "  • Auto-generates secrets for dev environments"
  echo "  • Blocks insecure configurations in production"
  echo ""
  echo "Performance (v0.9.8):"
  echo "  • Smart caching - 5x faster incremental builds"
  echo "  • Only rebuilds changed components"
  echo "  • Use --no-cache to force full rebuild"
  echo ""
  echo "Notes:"
  echo "  • Automatically detects configuration changes"
  echo "  • Only rebuilds what's necessary (unless --force)"
  echo "  • Validates security before building"
  echo "  • Creates trusted SSL certificates for HTTPS"
}

# Main build command function
cmd_build() {
  local force_rebuild=false
  local verbose=false
  local no_cache=false
  local allow_insecure=false
  local security_report=false
  local check_only=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case $1 in
      -f | --force)
        force_rebuild=true
        shift
        ;;
      --no-cache)
        no_cache=true
        force_rebuild=true
        shift
        ;;
      -h | --help)
        show_build_help
        return 0
        ;;
      -v | --verbose)
        verbose=true
        export VERBOSE=true
        shift
        ;;
      --debug)
        export BUILD_DEBUG=true
        export VERBOSE=true
        verbose=true
        shift
        ;;
      --allow-insecure)
        allow_insecure=true
        shift
        ;;
      --security-report)
        security_report=true
        shift
        ;;
      --check)
        check_only=true
        shift
        ;;
      *)
        echo "Error: Unknown option: $1" >&2
        echo "Use 'nself build --help' for usage information" >&2
        return 1
        ;;
    esac
  done

  # Export no-cache flag for core.sh
  export NSELF_NO_CACHE="$no_cache"
  export NSELF_ALLOW_INSECURE="$allow_insecure"

  # --check mode: validate config security without building
  if [[ "$check_only" == "true" ]]; then
    local check_failed=false

    # Load .env if present
    if [[ -f ".env" ]]; then
      # shellcheck disable=SC1091
      { set +u; source ".env"; } 2>/dev/null || true
    fi

    # Validate POSTGRES_PASSWORD minimum length (16 chars)
    local pg_pass="${POSTGRES_PASSWORD:-}"
    if [[ -z "$pg_pass" ]]; then
      echo "Error: POSTGRES_PASSWORD is empty (minimum 16 characters required)" >&2
      check_failed=true
    elif [[ "${#pg_pass}" -lt 16 ]]; then
      echo "Error: POSTGRES_PASSWORD too short (${#pg_pass} chars, minimum 16 required)" >&2
      check_failed=true
    fi

    # Validate HASURA_GRAPHQL_ADMIN_SECRET minimum length (32 chars)
    local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"
    if [[ -z "$admin_secret" ]]; then
      echo "Error: HASURA_GRAPHQL_ADMIN_SECRET is empty (minimum 32 characters required)" >&2
      check_failed=true
    elif [[ "${#admin_secret}" -lt 32 ]]; then
      echo "Error: HASURA_GRAPHQL_ADMIN_SECRET (admin secret) too short (${#admin_secret} chars, minimum 32 required)" >&2
      check_failed=true
    fi

    # Validate HASURA_GRAPHQL_JWT_SECRET format (must be JSON object or empty)
    local jwt_secret="${HASURA_GRAPHQL_JWT_SECRET:-}"
    if [[ -n "$jwt_secret" ]]; then
      # JWT secret must be a JSON object with "type" and "key" fields
      if ! echo "$jwt_secret" | grep -q '^{'; then
        echo "Warning: HASURA_GRAPHQL_JWT_SECRET should be a JSON object (e.g. {\"type\":\"HS256\",\"key\":\"...\"}) not a plain string" >&2
        check_failed=true
      fi
    fi

    if [[ "$check_failed" == "true" ]]; then
      return 1
    fi

    echo "Security check passed — configuration is valid"
    return 0
  fi

  # Check Docker availability
  if ! command -v docker >/dev/null 2>&1; then
    echo "Error: Docker is not installed" >&2
    echo "Install Docker from https://docker.com" >&2
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    echo "Error: Docker daemon is not running" >&2
    echo "Start Docker Desktop or run: sudo systemctl start docker" >&2
    return 1
  fi

  # Get project name from env or use basename
  local project_name="${PROJECT_NAME:-$(basename "$PWD")}"
  local env="${ENV:-dev}"

  # Load full environment cascade (secure-by-default requirement)
  # This ensures security validation sees all configured values from the cascade
  # Cascade order: .env.dev → .env.staging/prod → .env.secrets → .env
  # Critical: Must load BEFORE security validation to detect actual values
  # When verbose mode is enabled, show the cascade visualization
  if [[ "$verbose" == "true" ]]; then
    load_env_with_priority false  # Don't suppress output in verbose mode
  else
    load_env_with_priority true   # Suppress normal output
  fi
  env="${ENV:-$env}"

  # Sanitize environment variables (PROJECT_NAME, booleans, etc.) before security checks.
  # validate_environment auto-fixes invalid values and writes them back to .env so that
  # security validation and all downstream code sees clean values.
  if command -v validate_environment >/dev/null 2>&1; then
    validate_environment >/dev/null 2>&1 || true
  fi

  # ============================================================
  # SECURE BY DEFAULT: Security validation before build
  # ============================================================
  if command -v security::validate_build >/dev/null 2>&1; then
    if ! security::validate_build "$allow_insecure"; then
      # Security validation failed
      if [[ "$allow_insecure" == "true" ]] && [[ "$env" != "prod" ]] && [[ "$env" != "production" ]]; then
        # Dev environment with --allow-insecure flag - warn but continue
        echo ""
        echo "⚠ Continuing with insecure configuration (--allow-insecure flag)"
        echo ""
      else
        return 1
      fi
    fi

    # Generate comprehensive security report if requested
    if [[ "$security_report" == "true" ]]; then
      if command -v security::generate_report >/dev/null 2>&1; then
        security::generate_report
      fi
    fi
  fi

  # Verify build system is properly initialized
  if ! command -v orchestrate_build >/dev/null 2>&1; then
    echo "Error: Build system not properly initialized" >&2
    echo "Missing orchestrate_build function from core.sh" >&2
    return 1
  fi

  # Run the orchestrated build
  local build_result
  orchestrate_build "$project_name" "$env" "$force_rebuild" "$verbose"
  build_result=$?

  return $build_result
}

# Export the main command
export -f cmd_build

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_build "$@"
fi
