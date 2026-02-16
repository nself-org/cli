#!/usr/bin/env bash

# os-detection.sh - Cross-platform OS detection and platform-specific routing

# Detect the operating system
detect_os() {

set -euo pipefail

  local os_type=""
  local os_variant=""

  case "$(uname -s)" in
    Darwin*)
      os_type="macos"
      os_variant=$(sw_vers -productVersion 2>/dev/null || echo "unknown")
      ;;
    Linux*)
      os_type="linux"

      # Check if running in WSL
      if grep -qiE "(microsoft|wsl)" /proc/version 2>/dev/null; then
        os_type="wsl"
        os_variant=$(grep -oP '(?<=WSL)\d+' /proc/version 2>/dev/null || echo "1")
      else
        # Detect Linux distro
        if [[ -f /etc/os-release ]]; then
          os_variant=$(. /etc/os-release && echo "$ID")
        elif [[ -f /etc/redhat-release ]]; then
          os_variant="rhel"
        elif [[ -f /etc/debian_version ]]; then
          os_variant="debian"
        else
          os_variant="unknown"
        fi
      fi
      ;;
    CYGWIN* | MINGW* | MSYS*)
      os_type="windows"
      os_variant="mingw"
      ;;
    *)
      os_type="unknown"
      os_variant="unknown"
      ;;
  esac

  export NSELF_OS_TYPE="$os_type"
  export NSELF_OS_VARIANT="$os_variant"

  echo "$os_type"
  return 0
}

# Get the OS type (cached)
get_os_type() {
  if [[ -z "${NSELF_OS_TYPE:-}" ]]; then
    detect_os >/dev/null
  fi
  echo "${NSELF_OS_TYPE}"
}

# Get the OS variant (distro name, macOS version, etc.)
get_os_variant() {
  if [[ -z "${NSELF_OS_VARIANT:-}" ]]; then
    detect_os >/dev/null
  fi
  echo "${NSELF_OS_VARIANT}"
}

# Check if running on macOS
is_macos() {
  [[ "$(get_os_type)" == "macos" ]]
}

# Check if running on Linux
is_linux() {
  [[ "$(get_os_type)" == "linux" ]]
}

# Check if running on WSL
is_wsl() {
  [[ "$(get_os_type)" == "wsl" ]]
}

# Check if running on Windows (MinGW/Cygwin)
is_windows() {
  [[ "$(get_os_type)" == "windows" ]]
}

# Get platform-specific command for a given operation
# Usage: get_platform_command "trust_install" "default_command"
get_platform_command() {
  local operation="$1"
  local default_cmd="${2:-}"
  local os=$(get_os_type)

  case "$operation" in
    trust_install)
      case "$os" in
        macos) echo "security add-trusted-cert" ;;
        linux) echo "update-ca-certificates" ;;
        wsl) echo "update-ca-certificates" ;;
        *) echo "$default_cmd" ;;
      esac
      ;;
    trust_check)
      case "$os" in
        macos) echo "security find-certificate" ;;
        linux) echo "trust list" ;;
        wsl) echo "trust list" ;;
        *) echo "$default_cmd" ;;
      esac
      ;;
    package_manager)
      case "$(get_os_variant)" in
        ubuntu | debian) echo "apt-get" ;;
        fedora | rhel | centos) echo "dnf" ;;
        arch) echo "pacman" ;;
        alpine) echo "apk" ;;
        *) echo "$default_cmd" ;;
      esac
      ;;
    *)
      echo "$default_cmd"
      ;;
  esac
}

# Export functions
export -f detect_os
export -f get_os_type
export -f get_os_variant
export -f is_macos
export -f is_linux
export -f is_wsl
export -f is_windows
export -f get_platform_command
