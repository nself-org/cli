#!/usr/bin/env bash

# verifier.sh - Multi-OS trust verification for SSL certificates

# Source dependencies
VERIFIER_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

set -euo pipefail

source "$VERIFIER_LIB_DIR/core/os-detection.sh"
source "$VERIFIER_LIB_DIR/generators/mkcert.sh"

# Check if root CA is installed in system trust store
# Returns 0 if trusted, 1 if not trusted
trust_is_installed() {
  local mkcert_cmd=$(get_mkcert_command)
  local os_type=$(get_os_type)

  if [[ -z "$mkcert_cmd" ]]; then
    return 1
  fi

  # Check if CAROOT exists
  local caroot=$($mkcert_cmd -CAROOT 2>/dev/null)
  if [[ -z "$caroot" ]] || [[ ! -f "$caroot/rootCA.pem" ]]; then
    return 1
  fi

  case "$os_type" in
    macos)
      trust_is_installed_macos "$caroot"
      ;;
    linux)
      trust_is_installed_linux "$caroot"
      ;;
    wsl)
      trust_is_installed_wsl "$caroot"
      ;;
    *)
      # Fallback - check if CAROOT exists
      [[ -f "$caroot/rootCA.pem" ]]
      ;;
  esac
}

# Check if trusted on macOS
trust_is_installed_macos() {
  local caroot="$1"

  # Check macOS Keychain
  if security find-certificate -c "mkcert" -a 2>/dev/null | grep -q "mkcert"; then
    return 0
  fi

  # Alternative check - look for the specific cert
  if security find-certificate -c "mkcert development CA" 2>/dev/null | grep -q "mkcert"; then
    return 0
  fi

  return 1
}

# Check if trusted on Linux
trust_is_installed_linux() {
  local caroot="$1"
  local os_variant=$(get_os_variant)

  case "$os_variant" in
    ubuntu | debian | linuxmint)
      [[ -f /usr/local/share/ca-certificates/mkcert-root-ca.crt ]]
      ;;
    fedora | rhel | centos | rocky | alma)
      [[ -f /etc/pki/ca-trust/source/anchors/mkcert-root-ca.pem ]]
      ;;
    arch | manjaro)
      [[ -f /etc/ca-certificates/trust-source/anchors/mkcert-root-ca.pem ]]
      ;;
    opensuse*)
      [[ -f /etc/pki/trust/anchors/mkcert-root-ca.pem ]]
      ;;
    alpine)
      [[ -f /usr/local/share/ca-certificates/mkcert-root-ca.crt ]]
      ;;
    *)
      # Generic check - see if the CA is in the system bundle
      if command -v trust >/dev/null 2>&1; then
        trust list | grep -q "mkcert"
      else
        # Fallback to checking if CA file exists
        [[ -f "$caroot/rootCA.pem" ]]
      fi
      ;;
  esac
}

# Check if trusted on WSL
trust_is_installed_wsl() {
  local caroot="$1"

  # Check Linux side
  if trust_is_installed_linux "$caroot"; then
    return 0
  fi

  # Check Windows side
  local windows_certutil="/mnt/c/Windows/System32/certutil.exe"
  if [[ -x "$windows_certutil" ]]; then
    if "$windows_certutil" -store -user Root 2>/dev/null | grep -q "mkcert"; then
      return 0
    fi
  fi

  return 1
}

# Get trust status details
# Returns JSON-like output with trust information
trust_get_status() {
  local mkcert_cmd=$(get_mkcert_command)
  local os_type=$(get_os_type)
  local os_variant=$(get_os_variant)

  if [[ -z "$mkcert_cmd" ]]; then
    echo "mkcert_available=false"
    return 1
  fi

  local caroot=$($mkcert_cmd -CAROOT 2>/dev/null)
  local ca_exists="false"
  local trust_installed="false"

  if [[ -f "$caroot/rootCA.pem" ]]; then
    ca_exists="true"
  fi

  if trust_is_installed; then
    trust_installed="true"
  fi

  cat <<EOF
os_type=$os_type
os_variant=$os_variant
mkcert_available=true
mkcert_command=$mkcert_cmd
caroot=$caroot
ca_exists=$ca_exists
trust_installed=$trust_installed
EOF

  return 0
}

# Check if a specific certificate file is trusted
# Usage: trust_verify_certificate <cert_file>
trust_verify_certificate() {
  local cert_file="$1"
  local os_type=$(get_os_type)

  if [[ ! -f "$cert_file" ]]; then
    return 1
  fi

  case "$os_type" in
    macos)
      # Use security verify-cert on macOS
      security verify-cert -c "$cert_file" 2>/dev/null
      ;;
    linux | wsl)
      # Use openssl verify on Linux/WSL
      openssl verify "$cert_file" 2>/dev/null | grep -q "OK"
      ;;
    *)
      # Fallback - just check if it's a valid cert
      openssl x509 -in "$cert_file" -noout 2>/dev/null
      ;;
  esac
}

# Export functions
export -f trust_is_installed
export -f trust_is_installed_macos
export -f trust_is_installed_linux
export -f trust_is_installed_wsl
export -f trust_get_status
export -f trust_verify_certificate
