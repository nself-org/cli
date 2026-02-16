#!/usr/bin/env bash


# port-scanner.sh - Fast port availability checking before Docker startup

# Source utilities
UTILS_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$UTILS_DIR/display.sh" 2>/dev/null || true

# Extract all ports from docker-compose.yml
get_required_ports() {
  local compose_file="${1:-docker-compose.yml}"

  if [[ ! -f "$compose_file" ]]; then
    return 0
  fi

  # Extract all published ports from docker-compose config
  # Look for patterns like "8080:8080" or published: 8080
  docker compose config 2>/dev/null |
    grep -E '"[0-9]+:[0-9]+"|published: [0-9]+' |
    sed 's/.*"\([0-9]*\):.*/\1/; s/.*published: \([0-9]*\).*/\1/' |
    sort -u
}

# Check if a port is in use and by what
check_port_usage() {
  local port="$1"

  # Check with lsof (most reliable on macOS)
  if command -v lsof >/dev/null 2>&1; then
    local pid=$(lsof -ti:$port -sTCP:LISTEN 2>/dev/null | head -1)
    if [[ -n "$pid" ]]; then
      local process_name=$(ps -p $pid -o comm= 2>/dev/null)
      local full_path=$(ps -p $pid -o command= 2>/dev/null | cut -d' ' -f1)
      echo "$pid|$process_name|$full_path"
      return 1
    fi
  fi

  # Fallback to nc
  if nc -z localhost "$port" 2>/dev/null; then
    echo "unknown|unknown|unknown"
    return 1
  fi

  return 0
}

# Pre-check all required ports
precheck_all_ports() {
  local compose_file="${1:-docker-compose.yml}"
  local conflicts=()

  # Get all required ports
  local ports=$(get_required_ports "$compose_file")

  if [[ -z "$ports" ]]; then
    return 0
  fi

  # Check each port
  for port in $ports; do
    if ! check_port_usage "$port" >/dev/null 2>&1; then
      local usage=$(check_port_usage "$port")
      conflicts+=("$port:$usage")
    fi
  done

  if [[ ${#conflicts[@]} -gt 0 ]]; then
    echo "${conflicts[@]}"
    return 1
  fi

  return 0
}

# Get alternative port suggestion
suggest_alternative_port() {
  local base_port="$1"
  local port=$((base_port + 1))

  while [[ $port -lt 65535 ]]; do
    if check_port_usage "$port" >/dev/null 2>&1; then
      echo "$port"
      return 0
    fi
    ((port++))
  done

  return 1
}

# Auto-fix port in .env.local
fix_port_in_env() {
  local service="$1"
  local old_port="$2"
  local new_port="$3"

  # Map common ports to their env variables
  case "$old_port" in
    5432)
      sed -i.bak "s/^POSTGRES_PORT=.*/POSTGRES_PORT=$new_port/" .env.local
      if ! grep -q "^POSTGRES_PORT=" .env.local; then
        echo "POSTGRES_PORT=$new_port" >>.env.local
      fi
      ;;
    8080)
      sed -i.bak "s/^HASURA_PORT=.*/HASURA_PORT=$new_port/" .env.local
      if ! grep -q "^HASURA_PORT=" .env.local; then
        echo "HASURA_PORT=$new_port" >>.env.local
      fi
      ;;
    4000)
      sed -i.bak "s/^AUTH_PORT=.*/AUTH_PORT=$new_port/" .env.local
      if ! grep -q "^AUTH_PORT=" .env.local; then
        echo "AUTH_PORT=$new_port" >>.env.local
      fi
      ;;
    6379)
      # Redis port
      sed -i.bak "s/^REDIS_PORT=.*/REDIS_PORT=$new_port/" .env.local
      if ! grep -q "^REDIS_PORT=" .env.local; then
        echo "REDIS_PORT=$new_port" >>.env.local
      fi
      ;;
    1025)
      # Mailpit SMTP port
      sed -i.bak "s/^MAILPIT_SMTP_PORT=.*/MAILPIT_SMTP_PORT=$new_port/" .env.local
      if ! grep -q "^MAILPIT_SMTP_PORT=" .env.local; then
        echo "MAILPIT_SMTP_PORT=$new_port" >>.env.local
      fi
      ;;
    8025)
      # Mailpit UI port
      sed -i.bak "s/^MAILPIT_UI_PORT=.*/MAILPIT_UI_PORT=$new_port/" .env.local
      if ! grep -q "^MAILPIT_UI_PORT=" .env.local; then
        echo "MAILPIT_UI_PORT=$new_port" >>.env.local
      fi
      ;;
    5000 | 5001)
      # Storage port - add if not exists
      sed -i.bak "s/^STORAGE_PORT=.*/STORAGE_PORT=$new_port/" .env.local
      if ! grep -q "^STORAGE_PORT=" .env.local; then
        echo "STORAGE_PORT=$new_port" >>.env.local
      fi
      ;;
    9000)
      sed -i.bak "s/^MINIO_PORT=.*/MINIO_PORT=$new_port/" .env.local
      if ! grep -q "^MINIO_PORT=" .env.local; then
        echo "MINIO_PORT=$new_port" >>.env.local
      fi
      ;;
    *)
      # Generic port update - just append
      echo "# Port $old_port changed to $new_port" >>.env.local
      ;;
  esac

  # Clean up backup
  rm -f .env.local.bak
}

export -f get_required_ports check_port_usage precheck_all_ports suggest_alternative_port fix_port_in_env
