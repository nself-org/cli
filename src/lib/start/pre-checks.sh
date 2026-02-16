#!/usr/bin/env bash

# pre-checks.sh - Pre-flight checks for nself start
# Bash 3.2 compatible, cross-platform

# Source error messages library (namespaced to avoid clobbering caller's SCRIPT_DIR)
_START_PRECHECKS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${_START_PRECHECKS_DIR}/../utils/error-messages.sh" 2>/dev/null || true

# Check if services are already running
check_existing_services() {
  local project="${PROJECT_NAME:-nself}"
  local running_count=0

  # Use docker compose v2 syntax
  if command -v docker >/dev/null 2>&1; then
    running_count=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
  fi

  echo "$running_count"
}

# Check port availability with auto-increment
check_port_available() {
  local port="$1"
  local max_attempts="${2:-10}"
  local original_port="$port"
  local attempts=0

  while [ $attempts -lt $max_attempts ]; do
    # Cross-platform port check
    if command -v lsof >/dev/null 2>&1; then
      # macOS/BSD
      if ! lsof -i :$port >/dev/null 2>&1; then
        echo "$port"
        return 0
      fi
    elif command -v netstat >/dev/null 2>&1; then
      # Linux fallback
      if ! netstat -tuln 2>/dev/null | grep -q ":$port "; then
        echo "$port"
        return 0
      fi
    elif command -v ss >/dev/null 2>&1; then
      # Modern Linux
      if ! ss -tuln 2>/dev/null | grep -q ":$port "; then
        echo "$port"
        return 0
      fi
    else
      # Can't check, assume available
      echo "$port"
      return 0
    fi

    port=$((port + 1))
    attempts=$((attempts + 1))
  done

  # Port not available after max attempts
  return 1
}

# Check all required ports
check_all_ports() {
  local has_conflicts=false
  local conflict_list=""

  # Check main service ports (using arrays for Bash 3.2 compat)
  local ports=(80 443 5432 8080 4000 9000 9001 6379 1025 8025)
  local services=(nginx_http nginx_https postgres hasura auth minio minio_console redis mailpit_smtp mailpit_ui)

  local i=0
  while [ $i -lt ${#ports[@]} ]; do
    local port="${ports[$i]}"
    local service="${services[$i]}"

    # Get port from environment if set
    case "$service" in
      nginx_http) port="${NGINX_PORT:-80}" ;;
      nginx_https) port="${NGINX_HTTPS_PORT:-443}" ;;
      postgres) port="${POSTGRES_PORT:-5432}" ;;
      hasura) port="${HASURA_PORT:-8080}" ;;
      auth) port="${AUTH_PORT:-4000}" ;;
      minio) port="${MINIO_PORT:-9000}" ;;
      minio_console) port="${MINIO_CONSOLE_PORT:-9001}" ;;
      redis) port="${REDIS_PORT:-6379}" ;;
      mailpit_smtp) port="${MAILPIT_SMTP_PORT:-1025}" ;;
      mailpit_ui) port="${MAILPIT_UI_PORT:-8025}" ;;
    esac

    if ! check_port_available "$port" 1 >/dev/null; then
      has_conflicts=true
      conflict_list="${conflict_list}  - Port $port ($service)\n"
    fi

    i=$((i + 1))
  done

  if [ "$has_conflicts" = "true" ]; then
    echo "port_conflicts"
    # Store conflicts for later display if needed
    export PORT_CONFLICTS="$conflict_list"
    return 1
  fi

  echo "ports_available"
  return 0
}

# Check Docker availability
check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker_missing"
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    echo "docker_not_running"
    return 1
  fi

  echo "docker_ready"
  return 0
}

# Check environment files
check_env_files() {
  local env_file=""

  # Priority order for env files
  for file in .env.local .env.dev .env.development .env.staging .env.production .env; do
    if [ -f "$file" ]; then
      env_file="$file"
      break
    fi
  done

  if [ -z "$env_file" ]; then
    echo "no_env_file"
    return 1
  fi

  echo "$env_file"
  return 0
}

# Main pre-check function
run_pre_checks() {
  local verbose="${1:-false}"
  local checks_passed=true

  # Check Docker
  if [ "$verbose" = "true" ]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Checking Docker..."
  fi

  local docker_status=$(check_docker)
  if [ "$docker_status" != "docker_ready" ]; then
    if [ "$verbose" = "true" ]; then
      printf "\r${COLOR_RED}✗${COLOR_RESET} Docker not available\n"
    fi
    checks_passed=false
  elif [ "$verbose" = "true" ]; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Docker ready                    \n"
  fi

  # Check environment
  if [ "$verbose" = "true" ]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Checking environment..."
  fi

  local env_file=$(check_env_files)
  if [ "$?" -ne 0 ]; then
    if [ "$verbose" = "true" ]; then
      printf "\r${COLOR_RED}✗${COLOR_RESET} No environment file found\n"
    fi
    checks_passed=false
  elif [ "$verbose" = "true" ]; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Environment: $env_file          \n"
  fi

  # Check ports
  if [ "$verbose" = "true" ]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Checking port availability..."
  fi

  local port_status=$(check_all_ports)
  if [ "$port_status" != "ports_available" ]; then
    if [ "$verbose" = "true" ]; then
      printf "\r${COLOR_YELLOW}⚠${COLOR_RESET}  Some ports are in use            \n"
    fi
    # Don't fail on port conflicts, we can handle them
  elif [ "$verbose" = "true" ]; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Ports available                   \n"
  fi

  if [ "$checks_passed" = "true" ]; then
    return 0
  else
    return 1
  fi
}
