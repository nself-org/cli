#!/usr/bin/env bash

# mkcert.sh - mkcert certificate generation (cross-platform)

# Source OS detection
MKCERT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

set -euo pipefail

source "$MKCERT_LIB_DIR/core/os-detection.sh"

# Get mkcert command (system or bundled)
get_mkcert_command() {
  local mkcert_cmd=""

  # Try system mkcert first
  if command -v mkcert >/dev/null 2>&1; then
    mkcert_cmd="mkcert"
  elif [[ -x "${HOME}/.nself/bin/mkcert" ]]; then
    mkcert_cmd="${HOME}/.nself/bin/mkcert"
  elif [[ -x "/usr/local/bin/mkcert" ]]; then
    mkcert_cmd="/usr/local/bin/mkcert"
  fi

  echo "$mkcert_cmd"
}

# Check if mkcert is available
mkcert_is_available() {
  local mkcert_cmd=$(get_mkcert_command)
  [[ -n "$mkcert_cmd" ]] && [[ -x "$mkcert_cmd" ]]
}

# Install mkcert if not available
mkcert_ensure_installed() {
  if mkcert_is_available; then
    return 0
  fi

  local os_type=$(get_os_type)
  local install_dir="${HOME}/.nself/bin"
  mkdir -p "$install_dir"

  case "$os_type" in
    macos)
      # Try homebrew first
      if command -v brew >/dev/null 2>&1; then
        brew install mkcert 2>/dev/null || return 1
      else
        # Download binary
        local arch=$(uname -m)
        local download_url="https://github.com/FiloSottile/mkcert/releases/latest/download/mkcert-v*-darwin-${arch}"
        curl -sSL "$download_url" -o "$install_dir/mkcert" 2>/dev/null || return 1
        chmod +x "$install_dir/mkcert"
      fi
      ;;
    linux | wsl)
      # Try package manager first
      local pkg_mgr=$(get_platform_command "package_manager")
      if [[ "$pkg_mgr" == "apt-get" ]]; then
        sudo apt-get install -y mkcert 2>/dev/null || {
          # Download binary as fallback
          local arch=$(uname -m)
          [[ "$arch" == "x86_64" ]] && arch="amd64"
          curl -sSL "https://github.com/FiloSottile/mkcert/releases/latest/download/mkcert-v*-linux-${arch}" \
            -o "$install_dir/mkcert" 2>/dev/null || return 1
          chmod +x "$install_dir/mkcert"
        }
      else
        # Download binary
        local arch=$(uname -m)
        [[ "$arch" == "x86_64" ]] && arch="amd64"
        curl -sSL "https://github.com/FiloSottile/mkcert/releases/latest/download/mkcert-v*-linux-${arch}" \
          -o "$install_dir/mkcert" 2>/dev/null || return 1
        chmod +x "$install_dir/mkcert"
      fi
      ;;
    *)
      return 1
      ;;
  esac

  mkcert_is_available
}

# Get CAROOT directory
mkcert_get_caroot() {
  local mkcert_cmd=$(get_mkcert_command)
  if [[ -z "$mkcert_cmd" ]]; then
    echo "${HOME}/.local/share/mkcert"
    return 1
  fi

  local caroot=$($mkcert_cmd -CAROOT 2>/dev/null)
  if [[ -z "$caroot" ]]; then
    echo "${HOME}/.local/share/mkcert"
    return 1
  fi

  echo "$caroot"
  return 0
}

# Generate localhost certificate bundle
# Usage: mkcert_generate_localhost <output_dir>
mkcert_generate_localhost() {
  local output_dir="${1:-.}"
  local mkcert_cmd=$(get_mkcert_command)

  if [[ -z "$mkcert_cmd" ]]; then
    return 1
  fi

  mkdir -p "$output_dir"

  # Generate certificate for localhost and common aliases
  local domains=(
    "localhost"
    "*.localhost"
    "127.0.0.1"
    "::1"
    "local.nself.org"
    "*.local.nself.org"
  )

  # Generate in temp location first
  local temp_dir=$(mktemp -d)
  cd "$temp_dir" || return 1

  $mkcert_cmd "${domains[@]}" >/dev/null 2>&1

  # Find generated files (mkcert names them automatically)
  local cert_file=$(find . -name "*.pem" -not -name "*-key.pem" | head -1)
  local key_file=$(find . -name "*-key.pem" | head -1)

  if [[ -f "$cert_file" ]] && [[ -f "$key_file" ]]; then
    cp "$cert_file" "$output_dir/fullchain.pem"
    cp "$key_file" "$output_dir/privkey.pem"
    chmod 644 "$output_dir/fullchain.pem"
    chmod 600 "$output_dir/privkey.pem"
    cd - >/dev/null
    rm -rf "$temp_dir"
    return 0
  fi

  cd - >/dev/null
  rm -rf "$temp_dir"
  return 1
}

# Generate wildcard certificate
# Usage: mkcert_generate_wildcard <domain> <output_dir>
mkcert_generate_wildcard() {
  local domain="$1"
  local output_dir="${2:-.}"
  local mkcert_cmd=$(get_mkcert_command)

  if [[ -z "$mkcert_cmd" ]]; then
    return 1
  fi

  mkdir -p "$output_dir"

  local temp_dir=$(mktemp -d)
  cd "$temp_dir" || return 1

  $mkcert_cmd "$domain" "*.$domain" >/dev/null 2>&1

  local cert_file=$(find . -name "*.pem" -not -name "*-key.pem" | head -1)
  local key_file=$(find . -name "*-key.pem" | head -1)

  if [[ -f "$cert_file" ]] && [[ -f "$key_file" ]]; then
    cp "$cert_file" "$output_dir/fullchain.pem"
    cp "$key_file" "$output_dir/privkey.pem"
    chmod 644 "$output_dir/fullchain.pem"
    chmod 600 "$output_dir/privkey.pem"
    cd - >/dev/null
    rm -rf "$temp_dir"
    return 0
  fi

  cd - >/dev/null
  rm -rf "$temp_dir"
  return 1
}

# Check if a certificate is from mkcert
# Usage: mkcert_is_cert <cert_file>
mkcert_is_cert() {
  local cert_file="$1"

  if [[ ! -f "$cert_file" ]]; then
    return 1
  fi

  openssl x509 -in "$cert_file" -text -noout 2>/dev/null | grep -q "mkcert development CA"
}

# Export functions
export -f get_mkcert_command
export -f mkcert_is_available
export -f mkcert_ensure_installed
export -f mkcert_get_caroot
export -f mkcert_generate_localhost
export -f mkcert_generate_wildcard
export -f mkcert_is_cert
