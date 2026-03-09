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
      if ! lsof -iTCP:$port -sTCP:LISTEN >/dev/null 2>&1; then
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

# Get the name of the process holding a port
# Bash 3.2 compatible
get_port_holder() {
  local port="$1"
  local holder=""

  if command -v lsof >/dev/null 2>&1; then
    # macOS/Linux with lsof: get command name from LISTEN row
    holder=$(lsof -iTCP:$port -sTCP:LISTEN -n -P 2>/dev/null | awk 'NR==2{print $1}')
  elif command -v ss >/dev/null 2>&1; then
    holder=$(ss -tlnp 2>/dev/null | grep ":$port " | sed -E 's/.*users:\(\("([^"]+)".*/\1/' | head -1)
  elif command -v netstat >/dev/null 2>&1; then
    holder=$(netstat -tlnp 2>/dev/null | grep ":$port " | awk '{print $7}' | cut -d/ -f2 | head -1)
  fi

  if [ -z "$holder" ]; then
    holder="unknown process"
  fi
  printf "%s" "$holder"
}

# Check if a single port is in use on the host (TCP LISTEN)
# Returns 0 if available, 1 if in use
host_port_in_use() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:$port -sTCP:LISTEN -t >/dev/null 2>&1
    return $?
  elif command -v ss >/dev/null 2>&1; then
    ss -tln 2>/dev/null | grep -q ":$port "
    return $?
  elif command -v netstat >/dev/null 2>&1; then
    netstat -tln 2>/dev/null | grep -q ":$port "
    return $?
  else
    # nc fallback: try connecting, if it succeeds something is listening
    nc -z 127.0.0.1 "$port" 2>/dev/null
    return $?
  fi
}

# Preflight port conflict check
# Reads configured ports from environment, checks each one, and reports
# clearly which process holds the port and which .env variable to change.
# Returns 0 if all ports are free, 1 if any conflict detected.
# Bash 3.2 compatible: uses parallel arrays, avoids pipeline subshell issue.
preflight_port_check() {
  # Allow users to bypass the port check when they know their environment is safe.
  # Useful when running in Docker-in-Docker, custom networking, or other setups
  # where the pre-flight check produces false positives.
  # Usage: NSELF_SKIP_PORT_CHECK=true nself start
  if [ "${NSELF_SKIP_PORT_CHECK:-}" = "true" ]; then
    return 0
  fi

  local failed=0

  # Get current compose project name to exclude own containers.
  # Fall back to "nself" (the default project name used by check_existing_services).
  local own_project="${COMPOSE_PROJECT_NAME:-${PROJECT_NAME:-}}"
  if [ -z "$own_project" ] && [ -f ".env" ]; then
    own_project=$(grep "^PROJECT_NAME=" .env 2>/dev/null | head -1 | cut -d= -f2- || true)
  fi
  # Default: match the project name used when containers were started
  if [ -z "$own_project" ]; then
    own_project="nself"
  fi

  # Build list of ports held by this project's own containers (space-separated).
  # This prevents false-positive conflicts when nself is already partially running.
  local own_ports=""
  if command -v docker >/dev/null 2>&1; then
    own_ports=$(docker ps --filter "label=com.docker.compose.project=$own_project" --format "{{.Ports}}" 2>/dev/null \
      | grep -oE '(0\.0\.0\.0|127\.0\.0\.1):([0-9]+)' | grep -oE '[0-9]+$' | sort -u | tr '\n' ' ' || true)
  fi

  # Bash 3.2 compatible: check each port individually (no associative arrays)
  _check_port() {
    local env_var="$1" default_port="$2" service_name="$3" env_hint="$4"
    local actual_port
    actual_port=$(printenv "$env_var" 2>/dev/null || true)
    if [ -z "$actual_port" ]; then
      actual_port="$default_port"
    fi
    # Skip non-numeric
    case "$actual_port" in
      ''|*[!0-9]*) return 0 ;;
    esac
    # Skip ports owned by this project's containers
    case " $own_ports " in
      *" $actual_port "*) return 0 ;;
    esac
    if host_port_in_use "$actual_port"; then
      local holder
      holder=$(get_port_holder "$actual_port")
      # docker-proxy holds ports on behalf of Docker containers. Check if the
      # binding is localhost-only — if so it is safe and is not a conflict.
      case "$holder" in
        docker-proxy|docker|containerd*)
          local docker_binding
          docker_binding=$(docker ps --format "{{.Ports}}" 2>/dev/null \
            | grep -oE '(127\.0\.0\.1):'"$actual_port"':' | head -1 || true)
          if [ -n "$docker_binding" ]; then
            # Port is held by a Docker container bound to 127.0.0.1 — safe, skip
            return 0
          fi
          ;;
      esac
      printf "${COLOR_RED}ERROR${COLOR_RESET}: Port %s is already in use by '%s' (needed by %s)\n" \
        "$actual_port" "$holder" "$service_name"
      printf "       To change it: %s\n" "$env_hint"
      printf "       If this port is used by your own Docker containers, run: NSELF_SKIP_PORT_CHECK=true nself start\n"
      return 1
    fi
    return 0
  }

  _check_port "NGINX_PORT"          "80"   "nginx HTTP"    "set NGINX_PORT=<port> in .env"          || failed=1
  _check_port "NGINX_SSL_PORT"      "443"  "nginx HTTPS"   "set NGINX_SSL_PORT=<port> in .env"      || failed=1
  _check_port "POSTGRES_PORT"       "5432" "postgres"      "set POSTGRES_PORT=<port> in .env"       || failed=1
  _check_port "REDIS_PORT"          "6379" "redis"         "set REDIS_PORT=<port> in .env"          || failed=1
  _check_port "HASURA_PORT"         "8080" "hasura"        "set HASURA_PORT=<port> in .env"         || failed=1
  _check_port "MINIO_PORT"          "9000" "minio"         "set MINIO_PORT=<port> in .env"          || failed=1
  _check_port "MINIO_CONSOLE_PORT"  "9001" "minio console" "set MINIO_CONSOLE_PORT=<port> in .env"  || failed=1
  _check_port "MAILPIT_SMTP_PORT"   "1025" "mailpit SMTP"  "set MAILPIT_SMTP_PORT=<port> in .env"   || failed=1
  _check_port "MAILPIT_UI_PORT"     "8025" "mailpit UI"    "set MAILPIT_UI_PORT=<port> in .env"     || failed=1

  return $failed
}

export -f get_port_holder host_port_in_use preflight_port_check
