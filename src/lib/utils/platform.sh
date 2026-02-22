#!/bin/bash


# Platform detection and compatibility utilities

detect_platform() {

set -euo pipefail

  case "$OSTYPE" in
    darwin*) PLATFORM="macos" ;;
    linux*) PLATFORM="linux" ;;
    msys*) PLATFORM="windows" ;;
    cygwin*) PLATFORM="windows" ;;
    win32*) PLATFORM="windows" ;;
    *) PLATFORM="unknown" ;;
  esac

  export PLATFORM
}

detect_arch() {
  local arch=$(uname -m)
  case "$arch" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64) ARCH="arm64" ;;
    armv7l) ARCH="arm" ;;
    *) ARCH="$arch" ;;
  esac

  export ARCH
}

detect_package_manager() {
  if command -v brew &>/dev/null; then
    PKG_MANAGER="brew"
  elif command -v apt-get &>/dev/null; then
    PKG_MANAGER="apt"
  elif command -v yum &>/dev/null; then
    PKG_MANAGER="yum"
  elif command -v dnf &>/dev/null; then
    PKG_MANAGER="dnf"
  elif command -v pacman &>/dev/null; then
    PKG_MANAGER="pacman"
  elif command -v choco &>/dev/null; then
    PKG_MANAGER="choco"
  elif command -v winget &>/dev/null; then
    PKG_MANAGER="winget"
  else
    PKG_MANAGER="unknown"
  fi

  export PKG_MANAGER
}

get_docker_command() {
  if [[ "$PLATFORM" == "windows" ]]; then
    if command -v docker.exe &>/dev/null; then
      echo "docker.exe"
    else
      echo "docker"
    fi
  else
    echo "docker"
  fi
}

get_docker_compose_command() {
  # Try docker compose (v2) first
  if docker compose version &>/dev/null 2>&1; then
    echo "docker compose"
  # Fall back to docker-compose (v1)
  elif command -v docker-compose &>/dev/null; then
    echo "docker-compose"
  # Windows specific
  elif [[ "$PLATFORM" == "windows" ]]; then
    if docker.exe compose version &>/dev/null 2>&1; then
      echo "docker.exe compose"
    elif command -v docker-compose.exe &>/dev/null; then
      echo "docker-compose.exe"
    fi
  else
    echo ""
  fi
}

check_docker_desktop() {
  case "$PLATFORM" in
    macos)
      if [[ -d "/Applications/Docker.app" ]]; then
        return 0
      fi
      ;;
    windows)
      if [[ -d "/c/Program Files/Docker/Docker" ]] || [[ -d "$PROGRAMFILES/Docker/Docker" ]]; then
        return 0
      fi
      ;;
    linux)
      # Check for Docker Desktop on Linux
      if systemctl list-units --all | grep -q "docker-desktop"; then
        return 0
      fi
      ;;
  esac
  return 1
}

start_docker_platform_specific() {
  case "$PLATFORM" in
    macos)
      if check_docker_desktop; then
        echo "Starting Docker Desktop on macOS..."
        open -a Docker
        # Wait for Docker to be ready
        local count=0
        while ! docker info &>/dev/null && [ $count -lt 30 ]; do
          sleep 2
          count=$((count + 1))
        done
        if docker info &>/dev/null; then
          return 0
        fi
      fi
      ;;
    windows)
      if check_docker_desktop; then
        echo "Starting Docker Desktop on Windows..."
        if command -v powershell &>/dev/null; then
          powershell -Command "Start-Process 'C:\\Program Files\\Docker\\Docker\\Docker Desktop.exe'"
        fi
        # Wait for Docker to be ready
        local count=0
        while ! docker info &>/dev/null && [ $count -lt 30 ]; do
          sleep 2
          count=$((count + 1))
        done
        if docker info &>/dev/null; then
          return 0
        fi
      fi
      ;;
    linux)
      # Try to start Docker service
      if command -v systemctl &>/dev/null; then
        sudo systemctl start docker 2>/dev/null || true
        sleep 2
        if docker info &>/dev/null; then
          return 0
        fi
      elif command -v service &>/dev/null; then
        sudo service docker start 2>/dev/null || true
        sleep 2
        if docker info &>/dev/null; then
          return 0
        fi
      fi
      ;;
  esac
  return 1
}

get_temp_dir() {
  case "$PLATFORM" in
    windows)
      echo "${TEMP:-/tmp}"
      ;;
    *)
      echo "${TMPDIR:-/tmp}"
      ;;
  esac
}

normalize_path() {
  local path="$1"

  case "$PLATFORM" in
    windows)
      # Convert Windows paths for Git Bash/WSL
      if [[ "$path" =~ ^[A-Za-z]: ]]; then
        # Convert C:\path to /c/path
        echo "$path" | sed 's|\\|/|g' | sed 's|^\([A-Za-z]\):|/\L\1|'
      else
        echo "$path"
      fi
      ;;
    *)
      echo "$path"
      ;;
  esac
}

check_wsl() {
  if [[ -f /proc/version ]] && grep -qi microsoft /proc/version; then
    export IS_WSL=true
    return 0
  fi
  export IS_WSL=false
  return 1
}

get_host_ip() {
  case "$PLATFORM" in
    macos)
      # On Mac, use host.docker.internal or local IP
      echo "host.docker.internal"
      ;;
    linux)
      if check_wsl; then
        # In WSL, get Windows host IP
        cat /etc/resolv.conf | grep nameserver | awk '{print $2}'
      else
        # On Linux, get docker0 interface IP
        ip -4 addr show docker0 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || echo "172.17.0.1"
      fi
      ;;
    windows)
      echo "host.docker.internal"
      ;;
    *)
      echo "localhost"
      ;;
  esac
}

# Memory check platform-specific
get_available_memory_gb() {
  case "$PLATFORM" in
    macos)
      local mem_bytes=$(sysctl -n hw.memsize 2>/dev/null || echo 0)
      echo $((mem_bytes / 1073741824))
      ;;
    linux)
      local mem_kb=$(grep MemTotal /proc/meminfo 2>/dev/null | awk '{print $2}' || echo 0)
      echo $((mem_kb / 1048576))
      ;;
    windows)
      if command -v wmic &>/dev/null; then
        local mem_bytes=$(wmic computersystem get TotalPhysicalMemory -value | grep '=' | cut -d= -f2 | tr -d '\r')
        echo $((mem_bytes / 1073741824))
      else
        echo 0
      fi
      ;;
    *)
      echo 0
      ;;
  esac
}

# Disk space check platform-specific
get_available_disk_gb() {
  local path="${1:-.}"

  case "$PLATFORM" in
    macos | linux)
      df -BG "$path" 2>/dev/null | awk 'NR==2 {print int($4)}' || echo 0
      ;;
    windows)
      if command -v wmic &>/dev/null; then
        local drive=$(echo "$path" | cut -c1)
        wmic logicaldisk where "DeviceID='${drive}:'" get FreeSpace -value | grep '=' | cut -d= -f2 | awk '{print int($1/1073741824)}' || echo 0
      else
        df -BG "$path" 2>/dev/null | awk 'NR==2 {print int($4)}' || echo 0
      fi
      ;;
    *)
      echo 0
      ;;
  esac
}

# Initialize platform detection when sourced
if [[ -z "$PLATFORM" ]]; then
  detect_platform
  detect_arch
  detect_package_manager
  check_wsl
fi

export -f detect_platform
export -f detect_arch
export -f detect_package_manager
export -f get_docker_command
export -f get_docker_compose_command
export -f check_docker_desktop
export -f start_docker_platform_specific
export -f get_temp_dir
export -f normalize_path
export -f check_wsl
export -f get_host_ip
export -f get_available_memory_gb
export -f get_available_disk_gb
