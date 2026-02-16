#!/usr/bin/env bash

# port-manager.sh - Port conflict detection and resolution
# Bash 3.2 compatible, cross-platform

# Find next available port
find_available_port() {

set -euo pipefail

  local start_port="${1:-8000}"
  local max_port="${2:-9999}"
  local current_port=$start_port

  while [ $current_port -le $max_port ]; do
    if is_port_available $current_port; then
      echo $current_port
      return 0
    fi
    current_port=$((current_port + 1))
  done

  return 1
}

# Check if port is available
is_port_available() {
  local port="$1"

  # Try multiple methods for cross-platform compatibility
  if command -v lsof >/dev/null 2>&1; then
    # macOS/BSD
    if ! lsof -i :$port >/dev/null 2>&1; then
      return 0
    fi
  elif command -v ss >/dev/null 2>&1; then
    # Modern Linux
    if ! ss -tuln 2>/dev/null | grep -q ":$port "; then
      return 0
    fi
  elif command -v netstat >/dev/null 2>&1; then
    # Older Linux/Unix
    if ! netstat -tuln 2>/dev/null | grep -q ":$port "; then
      return 0
    fi
  else
    # Can't check, assume available
    return 0
  fi

  return 1
}

# Get process using port
get_port_process() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    # macOS/BSD
    lsof -i :$port 2>/dev/null | grep LISTEN | awk '{print $2, $1}' | head -1
  elif command -v ss >/dev/null 2>&1; then
    # Modern Linux
    ss -tlnp 2>/dev/null | grep ":$port " | sed -E 's/.*users:\(\("([^"]+)".*pid=([0-9]+).*/\2 \1/'
  elif command -v netstat >/dev/null 2>&1; then
    # Older Linux (requires root for -p)
    if [ "$EUID" -eq 0 ]; then
      netstat -tlnp 2>/dev/null | grep ":$port " | awk '{print $7}' | sed 's|/| |'
    else
      echo "unknown process"
    fi
  else
    echo "unknown"
  fi
}

# Auto-resolve port conflicts
auto_resolve_ports() {
  local env_file="${1:-.env}"
  local changes_made=false
  local port_updates=""

  # Define default ports and their variables (Bash 3.2 compatible arrays)
  local port_vars=(NGINX_PORT NGINX_HTTPS_PORT POSTGRES_PORT HASURA_PORT AUTH_PORT MINIO_PORT MINIO_CONSOLE_PORT REDIS_PORT MAILPIT_SMTP_PORT MAILPIT_UI_PORT MLFLOW_PORT)
  local port_defaults=(80 443 5432 8080 4000 9000 9001 6379 1025 8025 5005)
  local port_names=(nginx_http nginx_https postgres hasura auth minio minio_console redis mailpit_smtp mailpit_ui mlflow)

  # Check each port
  local i=0
  while [ $i -lt ${#port_vars[@]} ]; do
    local var="${port_vars[$i]}"
    local default_port="${port_defaults[$i]}"
    local name="${port_names[$i]}"

    # Get current value from environment
    local current_port=$(grep "^$var=" "$env_file" 2>/dev/null | cut -d= -f2)
    if [ -z "$current_port" ]; then
      current_port=$default_port
    fi

    # Check if port is available
    if ! is_port_available $current_port; then
      # Find alternative port
      local new_port=$(find_available_port $((current_port + 1)) $((current_port + 100)))
      if [ -n "$new_port" ]; then
        # Update or add to env file
        if grep -q "^$var=" "$env_file"; then
          # Update existing
          sed -i.bak "s/^$var=.*/$var=$new_port/" "$env_file" && rm "${env_file}.bak"
        else
          # Add new
          echo "$var=$new_port" >>"$env_file"
        fi
        port_updates="${port_updates}  - $name: $current_port → $new_port\n"
        changes_made=true
      fi
    fi

    i=$((i + 1))
  done

  if [ "$changes_made" = "true" ]; then
    # Store the updates for display later
    export PORT_UPDATES="$port_updates"
    echo "ports_updated"
    return 0
  else
    echo "no_conflicts"
    return 0
  fi
}

# Interactive port conflict resolution
resolve_port_conflicts() {
  local interactive="${1:-false}"

  if [ "$interactive" = "false" ]; then
    # Auto-resolve
    auto_resolve_ports
    return $?
  fi

  # Interactive mode
  echo "The following ports are in use:"

  # Check each service port
  local ports=(80 443 5432 8080 4000 9000 9001 6379 1025 8025)
  local services=(nginx postgres hasura auth minio minio_console redis mailpit_smtp mailpit_ui)

  local i=0
  local has_conflicts=false
  while [ $i -lt ${#ports[@]} ]; do
    local port="${ports[$i]}"
    local service="${services[$i]}"

    if ! is_port_available $port; then
      has_conflicts=true
      local process=$(get_port_process $port)
      echo "  Port $port ($service): used by $process"

      if [ "$interactive" = "true" ]; then
        echo -n "    Find alternative port? [Y/n]: "
        read -r response
        if [ -z "$response" ] || [ "$response" = "y" ] || [ "$response" = "Y" ]; then
          local new_port=$(find_available_port $((port + 1)))
          echo "    → Using port $new_port instead"
        fi
      fi
    fi

    i=$((i + 1))
  done

  if [ "$has_conflicts" = "false" ]; then
    echo "No port conflicts detected"
    return 0
  fi

  return 1
}
