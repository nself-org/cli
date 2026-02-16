#!/usr/bin/env bash

# installer.sh - Multi-OS trust installation for SSL certificates

# Source dependencies
TRUST_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

set -euo pipefail

source "$TRUST_LIB_DIR/core/os-detection.sh"
source "$TRUST_LIB_DIR/generators/mkcert.sh"

# Install root CA to system trust store
# Returns 0 on success, 1 on failure
trust_install_root_ca() {
  local mkcert_cmd=$(get_mkcert_command)
  local os_type=$(get_os_type)

  if [[ -z "$mkcert_cmd" ]]; then
    return 1
  fi

  case "$os_type" in
    macos)
      trust_install_macos "$mkcert_cmd"
      ;;
    linux)
      trust_install_linux "$mkcert_cmd"
      ;;
    wsl)
      trust_install_wsl "$mkcert_cmd"
      ;;
    *)
      # Fallback to mkcert's built-in installer
      $mkcert_cmd -install 2>&1
      ;;
  esac
}

# macOS-specific trust installation
trust_install_macos() {
  local mkcert_cmd="$1"

  # Use mkcert's built-in installation for macOS
  # This handles:
  # - System keychain
  # - Firefox NSS database (if Firefox is installed)
  $mkcert_cmd -install 2>&1

  local result=$?

  # Verify installation
  if [[ $result -eq 0 ]]; then
    # Double-check it's in the keychain
    local caroot=$($mkcert_cmd -CAROOT 2>/dev/null)
    if [[ -f "$caroot/rootCA.pem" ]]; then
      if security find-certificate -c "mkcert" -a 2>/dev/null | grep -q "mkcert"; then
        return 0
      fi
    fi
  fi

  return $result
}

# Linux-specific trust installation
trust_install_linux() {
  local mkcert_cmd="$1"
  local os_variant=$(get_os_variant)
  local caroot=$($mkcert_cmd -CAROOT 2>/dev/null)

  if [[ ! -f "$caroot/rootCA.pem" ]]; then
    # Generate CA if it doesn't exist
    $mkcert_cmd -install 2>&1 || return 1
  fi

  case "$os_variant" in
    ubuntu | debian | linuxmint)
      # Debian-based distributions
      sudo cp "$caroot/rootCA.pem" /usr/local/share/ca-certificates/mkcert-root-ca.crt 2>/dev/null
      sudo update-ca-certificates 2>/dev/null
      ;;
    fedora | rhel | centos | rocky | alma)
      # Red Hat-based distributions
      sudo cp "$caroot/rootCA.pem" /etc/pki/ca-trust/source/anchors/mkcert-root-ca.pem 2>/dev/null
      sudo update-ca-trust 2>/dev/null
      ;;
    arch | manjaro)
      # Arch-based distributions
      sudo cp "$caroot/rootCA.pem" /etc/ca-certificates/trust-source/anchors/mkcert-root-ca.pem 2>/dev/null
      sudo trust extract-compat 2>/dev/null
      ;;
    opensuse*)
      # openSUSE
      sudo cp "$caroot/rootCA.pem" /etc/pki/trust/anchors/mkcert-root-ca.pem 2>/dev/null
      sudo update-ca-certificates 2>/dev/null
      ;;
    alpine)
      # Alpine Linux
      sudo cp "$caroot/rootCA.pem" /usr/local/share/ca-certificates/mkcert-root-ca.crt 2>/dev/null
      sudo update-ca-certificates 2>/dev/null
      ;;
    *)
      # Generic Linux - try mkcert's built-in
      $mkcert_cmd -install 2>&1
      ;;
  esac

  # Install for Firefox/Chrome/etc. (NSS)
  if command -v certutil >/dev/null 2>&1; then
    local nss_dirs=(
      "$HOME/.pki/nssdb"
      "$HOME/.mozilla/firefox/*.default-release"
      "$HOME/.mozilla/firefox/*.default"
    )

    for nss_dir in "${nss_dirs[@]}"; do
      # Expand glob patterns
      for dir in $nss_dir; do
        if [[ -d "$dir" ]]; then
          certutil -A -d "sql:$dir" -t "C,," -n "mkcert" -i "$caroot/rootCA.pem" 2>/dev/null || true
        fi
      done
    done
  fi

  return 0
}

# WSL-specific trust installation
trust_install_wsl() {
  local mkcert_cmd="$1"

  # In WSL, install for both Linux and Windows

  # Install for Linux side
  trust_install_linux "$mkcert_cmd"

  # Try to install for Windows side if possible
  local windows_certutil="/mnt/c/Windows/System32/certutil.exe"
  if [[ -x "$windows_certutil" ]]; then
    local caroot=$($mkcert_cmd -CAROOT 2>/dev/null)
    if [[ -f "$caroot/rootCA.pem" ]]; then
      # Convert to Windows path
      local win_ca_path=$(wslpath -w "$caroot/rootCA.pem" 2>/dev/null)
      if [[ -n "$win_ca_path" ]]; then
        "$windows_certutil" -addstore -user Root "$win_ca_path" 2>/dev/null || true
      fi
    fi
  fi

  return 0
}

# Uninstall root CA from system trust store
trust_uninstall_root_ca() {
  local mkcert_cmd=$(get_mkcert_command)
  local os_type=$(get_os_type)

  if [[ -z "$mkcert_cmd" ]]; then
    return 1
  fi

  case "$os_type" in
    macos)
      $mkcert_cmd -uninstall 2>&1
      ;;
    linux)
      trust_uninstall_linux "$mkcert_cmd"
      ;;
    wsl)
      trust_uninstall_linux "$mkcert_cmd"
      # Also try to uninstall from Windows
      local windows_certutil="/mnt/c/Windows/System32/certutil.exe"
      if [[ -x "$windows_certutil" ]]; then
        "$windows_certutil" -delstore -user Root "mkcert" 2>/dev/null || true
      fi
      ;;
    *)
      $mkcert_cmd -uninstall 2>&1
      ;;
  esac
}

# Linux-specific trust uninstallation
trust_uninstall_linux() {
  local mkcert_cmd="$1"
  local os_variant=$(get_os_variant)

  case "$os_variant" in
    ubuntu | debian | linuxmint)
      sudo rm -f /usr/local/share/ca-certificates/mkcert-root-ca.crt 2>/dev/null
      sudo update-ca-certificates --fresh 2>/dev/null
      ;;
    fedora | rhel | centos | rocky | alma)
      sudo rm -f /etc/pki/ca-trust/source/anchors/mkcert-root-ca.pem 2>/dev/null
      sudo update-ca-trust 2>/dev/null
      ;;
    arch | manjaro)
      sudo rm -f /etc/ca-certificates/trust-source/anchors/mkcert-root-ca.pem 2>/dev/null
      sudo trust extract-compat 2>/dev/null
      ;;
    opensuse*)
      sudo rm -f /etc/pki/trust/anchors/mkcert-root-ca.pem 2>/dev/null
      sudo update-ca-certificates 2>/dev/null
      ;;
    alpine)
      sudo rm -f /usr/local/share/ca-certificates/mkcert-root-ca.crt 2>/dev/null
      sudo update-ca-certificates 2>/dev/null
      ;;
    *)
      $mkcert_cmd -uninstall 2>&1
      ;;
  esac

  # Remove from NSS databases
  if command -v certutil >/dev/null 2>&1; then
    local nss_dirs=(
      "$HOME/.pki/nssdb"
      "$HOME/.mozilla/firefox/*.default-release"
      "$HOME/.mozilla/firefox/*.default"
    )

    for nss_dir in "${nss_dirs[@]}"; do
      for dir in $nss_dir; do
        if [[ -d "$dir" ]]; then
          certutil -D -d "sql:$dir" -n "mkcert" 2>/dev/null || true
        fi
      done
    done
  fi

  return 0
}

# Export functions
export -f trust_install_root_ca
export -f trust_install_macos
export -f trust_install_linux
export -f trust_install_wsl
export -f trust_uninstall_root_ca
export -f trust_uninstall_linux
