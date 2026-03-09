#!/usr/bin/env bash
# plugin_install.sh — Rust plugin binary download, verification, and install
# Bash 3.2 compatible. No echo -e, no ${var,,}, no declare -A.
#
# Called by plugin.sh when plugin.json declares language=rust.
# Handles:
#   - Arch detection (uname -m + uname -s)
#   - Signed URL request from ping_api
#   - Binary download to ~/.nself/plugins/<name>/bin/<binary_name>
#   - SHA-256 verification
#   - chmod +x

set -o pipefail

NSELF_PLUGINS_DIR="${NSELF_PLUGIN_DIR:-${HOME}/.nself/plugins}"
NSELF_PING_URL="${NSELF_PING_API_URL:-https://ping.nself.org}"

# ---------------------------------------------------------------------------
# _plugin_detect_arch
# Returns a string like "linux-x86_64", "linux-arm64", "darwin-arm64"
# ---------------------------------------------------------------------------
_plugin_detect_arch() {
  local os machine arch_str
  os=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')
  machine=$(uname -m 2>/dev/null)

  case "$machine" in
    x86_64|amd64)  machine="x86_64" ;;
    aarch64|arm64) machine="arm64" ;;
    armv7*)        machine="armv7" ;;
    *)             machine="$machine" ;;
  esac

  case "$os" in
    linux)   arch_str="linux-${machine}" ;;
    darwin)  arch_str="darwin-${machine}" ;;
    mingw*|cygwin*|msys*) arch_str="windows-${machine}" ;;
    *)       arch_str="${os}-${machine}" ;;
  esac

  printf '%s' "$arch_str"
}

# ---------------------------------------------------------------------------
# _plugin_sha256 <file>
# Print the SHA-256 hex digest of a file. Cross-platform (macOS + Linux).
# ---------------------------------------------------------------------------
_plugin_sha256() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" 2>/dev/null | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" 2>/dev/null | cut -d' ' -f1
  else
    printf ''
  fi
}

# ---------------------------------------------------------------------------
# plugin_install_rust_binary <plugin_name> <binary_name> [license_key]
# Downloads the binary for the current arch and verifies SHA-256.
# Returns 0 on success, 1 on failure.
# ---------------------------------------------------------------------------
plugin_install_rust_binary() {
  local plugin_name="$1"
  local binary_name="$2"
  local license_key="${3:-}"

  if [ -z "$license_key" ]; then
    # Try env var then key file
    if [ -n "${NSELF_PLUGIN_LICENSE_KEY:-}" ]; then
      license_key="$NSELF_PLUGIN_LICENSE_KEY"
    elif [ -f "${HOME}/.nself/license/key" ]; then
      license_key=$(tr -d '[:space:]' < "${HOME}/.nself/license/key" 2>/dev/null)
    fi
  fi

  if [ -z "$license_key" ]; then
    printf '\033[0;31m[ERROR]\033[0m License key required to download Rust plugin binary.\n' >&2
    return 1
  fi

  if ! command -v curl >/dev/null 2>&1; then
    printf '\033[0;31m[ERROR]\033[0m curl is required to download plugin binaries.\n' >&2
    return 1
  fi

  local arch
  arch=$(_plugin_detect_arch)

  printf '\033[0;34m[INFO]\033[0m Detected arch: %s\n' "$arch"
  printf '\033[0;34m[INFO]\033[0m Requesting download URL for plugin: %s\n' "$plugin_name"

  # Request signed download URL from ping_api
  local response http_status body
  response=$(curl -s -w '\n%{http_code}' \
    --max-time 30 \
    --connect-timeout 10 \
    -H "X-License-Key: ${license_key}" \
    "${NSELF_PING_URL}/plugins/${plugin_name}/download-url?arch=${arch}" 2>/dev/null)

  http_status=$(printf '%s' "$response" | tail -1)
  body=$(printf '%s' "$response" | head -1)

  if [ "$http_status" != "200" ]; then
    local err_msg
    err_msg=$(printf '%s' "$body" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)
    printf '\033[0;31m[ERROR]\033[0m Failed to get download URL (HTTP %s): %s\n' "$http_status" "${err_msg:-unknown}" >&2
    return 1
  fi

  # Extract download URL and expected SHA-256 from response
  local download_url expected_sha
  download_url=$(printf '%s' "$body" | grep -o '"url":"[^"]*"' | head -1 | cut -d'"' -f4)
  expected_sha=$(printf '%s' "$body" | grep -o '"sha256":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [ -z "$download_url" ]; then
    printf '\033[0;31m[ERROR]\033[0m No download URL in response.\n' >&2
    return 1
  fi

  # Prepare destination directory
  local bin_dir="${NSELF_PLUGINS_DIR}/${plugin_name}/bin"
  mkdir -p "$bin_dir" 2>/dev/null
  local dest="${bin_dir}/${binary_name}"

  printf '\033[0;34m[INFO]\033[0m Downloading %s binary...\n' "$plugin_name"

  # Download binary
  local http_dl_status
  http_dl_status=$(curl -s -w '%{http_code}' \
    --max-time 120 \
    --connect-timeout 15 \
    -L \
    -o "$dest" \
    "$download_url" 2>/dev/null)

  if [ "$http_dl_status" != "200" ]; then
    printf '\033[0;31m[ERROR]\033[0m Binary download failed (HTTP %s).\n' "$http_dl_status" >&2
    rm -f "$dest" 2>/dev/null
    return 1
  fi

  # Verify SHA-256 if provided
  if [ -n "$expected_sha" ]; then
    local actual_sha
    actual_sha=$(_plugin_sha256 "$dest")
    if [ -z "$actual_sha" ]; then
      printf '\033[0;33m[WARNING]\033[0m Cannot verify SHA-256 (no sha256sum or shasum available).\n'
    elif [ "$actual_sha" != "$expected_sha" ]; then
      printf '\033[0;31m[ERROR]\033[0m SHA-256 mismatch for %s.\n' "$binary_name" >&2
      printf '  Expected: %s\n' "$expected_sha" >&2
      printf '  Got:      %s\n' "$actual_sha" >&2
      rm -f "$dest" 2>/dev/null
      return 1
    else
      printf '\033[0;32m[SUCCESS]\033[0m SHA-256 verified.\n'
    fi
  fi

  # Make executable
  chmod +x "$dest" 2>/dev/null

  printf '\033[0;32m[SUCCESS]\033[0m Installed: %s\n' "$dest"
  return 0
}
