#!/usr/bin/env bash

# api.sh - Unified SSL/Trust API
# This is the single public interface that all commands use

# Get the library directory
SSL_API_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source all modules
source "$SSL_API_LIB_DIR/core/os-detection.sh"
source "$SSL_API_LIB_DIR/generators/mkcert.sh"
source "$SSL_API_LIB_DIR/trust/installer.sh"
source "$SSL_API_LIB_DIR/trust/verifier.sh"

# ============================================================================
# PUBLIC API - SSL Generation
# ============================================================================

# Generate SSL certificates for a project
# Usage: ssl_generate <project_dir> [domain]
# Returns: 0 on success, 1 on failure
ssl_generate() {
  local project_dir="${1:-.}"
  local domain="${2:-localhost}"

  # Ensure mkcert is available
  if ! mkcert_ensure_installed; then
    # Fallback to OpenSSL if mkcert not available
    return 1
  fi

  # Create certificate directories
  local cert_dir="$project_dir/ssl/certificates/$domain"
  mkdir -p "$cert_dir"

  # Generate using mkcert
  if mkcert_generate_localhost "$cert_dir"; then
    # Copy to nginx directory
    local nginx_ssl_dir="$project_dir/nginx/ssl/$domain"
    mkdir -p "$nginx_ssl_dir"
    cp -f "$cert_dir/fullchain.pem" "$nginx_ssl_dir/" 2>/dev/null
    cp -f "$cert_dir/privkey.pem" "$nginx_ssl_dir/" 2>/dev/null
    return 0
  fi

  return 1
}

# Check if SSL certificates exist for a project
# Usage: ssl_exists <project_dir> [domain]
ssl_exists() {
  local project_dir="${1:-.}"
  local domain="${2:-localhost}"
  local cert_file="$project_dir/ssl/certificates/$domain/fullchain.pem"
  local key_file="$project_dir/ssl/certificates/$domain/privkey.pem"

  [[ -f "$cert_file" ]] && [[ -f "$key_file" ]]
}

# Check if existing certificates are from mkcert
# Usage: ssl_is_mkcert <project_dir> [domain]
ssl_is_mkcert() {
  local project_dir="${1:-.}"
  local domain="${2:-localhost}"
  local cert_file="$project_dir/ssl/certificates/$domain/fullchain.pem"

  if [[ ! -f "$cert_file" ]]; then
    return 1
  fi

  mkcert_is_cert "$cert_file"
}

# ============================================================================
# PUBLIC API - Trust Management
# ============================================================================

# Install root CA to system trust store
# Usage: trust_install
# Returns: 0 on success, 1 on failure
# Note: May prompt for sudo password
trust_install() {
  # Ensure mkcert is available
  if ! mkcert_ensure_installed; then
    return 1
  fi

  trust_install_root_ca
}

# Check if root CA is installed
# Usage: trust_check
# Returns: 0 if installed, 1 if not
trust_check() {
  trust_is_installed
}

# Uninstall root CA from system trust store
# Usage: trust_uninstall
# Returns: 0 on success, 1 on failure
trust_uninstall() {
  trust_uninstall_root_ca
}

# Get detailed trust status
# Usage: trust_status
trust_status() {
  trust_get_status
}

# ============================================================================
# PUBLIC API - Combined Operations (for build command)
# ============================================================================

# Generate SSL certificates AND install trust if needed
# This is the main function for the build command
# Usage: ssl_setup_for_build <project_dir> [domain]
# Returns: 0 on success, 1 on failure
ssl_setup_for_build() {
  local project_dir="${1:-.}"
  local domain="${2:-localhost}"

  # Check if certificates already exist and are trusted
  if ssl_exists "$project_dir" "$domain" && ssl_is_mkcert "$project_dir" "$domain" && trust_check; then
    return 0 # Already setup
  fi

  # Generate certificates
  if ! ssl_generate "$project_dir" "$domain"; then
    return 1
  fi

  # Install trust if not already installed
  if ! trust_check; then
    trust_install || true # Don't fail if user cancels sudo
  fi

  return 0
}

# Quick check if SSL is fully configured (certs + trust)
# Usage: ssl_is_configured <project_dir> [domain]
# Returns: 0 if fully configured, 1 if not
ssl_is_configured() {
  local project_dir="${1:-.}"
  local domain="${2:-localhost}"

  ssl_exists "$project_dir" "$domain" && ssl_is_mkcert "$project_dir" "$domain" && trust_check
}

# ============================================================================
# Export all public API functions
# ============================================================================

export -f ssl_generate
export -f ssl_exists
export -f ssl_is_mkcert
export -f trust_install
export -f trust_check
export -f trust_uninstall
export -f trust_status
export -f ssl_setup_for_build
export -f ssl_is_configured
