#!/usr/bin/env bash

# ssl.sh - Build module for SSL certificate generation
# This is a thin wrapper around the SSL API

# Get the root directory
BUILD_MODULE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

set -euo pipefail


# Source the unified SSL API
if [[ -f "$BUILD_MODULE_DIR/ssl/api.sh" ]]; then
  source "$BUILD_MODULE_DIR/ssl/api.sh"
else
  echo "Error: SSL API not found" >&2
  return 1
fi

# Build-specific SSL setup function
# This is called by the build orchestrator
# Usage: build_setup_ssl [domain]
build_setup_ssl() {
  local domain="${BASE_DOMAIN:-localhost}"
  local project_dir="."

  # Use the unified API
  if ssl_setup_for_build "$project_dir" "$domain"; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} SSL successfully configured                    \n"
    return 0
  else
    printf "\r${COLOR_YELLOW}✱${COLOR_RESET} SSL setup incomplete                          \n"
    return 1
  fi
}

# Check if SSL is already configured (for skip logic)
# Usage: build_needs_ssl
build_needs_ssl() {
  local domain="${BASE_DOMAIN:-localhost}"
  local project_dir="."

  # If SSL is fully configured, we don't need to regenerate
  if ssl_is_configured "$project_dir" "$domain"; then
    return 1 # Return 1 = no need to regenerate
  fi

  return 0 # Return 0 = need to generate/setup
}

# Export functions
export -f build_setup_ssl
export -f build_needs_ssl
