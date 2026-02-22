#!/usr/bin/env bash


# quick-check.sh - Fast essential checks only

# Quick environment check
quick_env_check() {

set -euo pipefail

  if [[ ! -f ".env.local" ]]; then
    log_error "No .env.local found"
    log_info "Run: nself init"
    return 1
  fi

  # Quick check for critical vars only
  set -a
  source .env.local
  set +a
  if [[ -z "$BASE_DOMAIN" ]]; then
    log_error "BASE_DOMAIN not set in .env.local"
    return 1
  fi

  return 0
}

# Quick port check - only check if ports are free
quick_port_check() {
  # Use configured ports if present; otherwise default
  local http_port="${NGINX_HTTP_PORT:-80}"
  local https_port="${NGINX_HTTPS_PORT:-443}"
  local pg_port="${POSTGRES_PORT:-5432}"
  local hasura_port="${HASURA_PORT:-8080}"

  local critical_ports=("$http_port" "$https_port" "$pg_port" "$hasura_port")
  local conflicts=()

  for port in "${critical_ports[@]}"; do
    if ! is_port_available $port; then
      conflicts+=($port)
    fi
  done

  if [[ ${#conflicts[@]} -gt 0 ]]; then
    log_warning "Port conflicts detected: ${conflicts[*]}"
    return 1
  fi

  return 0
}

# Quick Docker check
quick_docker_check() {
  if ! docker info >/dev/null 2>&1; then
    log_error "Docker is not running"

    # Offer to start Docker on macOS
    if [[ "$OSTYPE" == "darwin"* ]] && [[ -d "/Applications/Docker.app" ]]; then
      read -p "Start Docker Desktop? [Y/n]: " -n 1 -r
      echo ""
      if [[ ! $REPLY =~ ^[Nn]$ ]]; then
        open -a Docker
        log_info "Starting Docker Desktop..."

        # Wait up to 30 seconds
        local count=0
        while [[ $count -lt 30 ]]; do
          if docker info >/dev/null 2>&1; then
            log_success "Docker started successfully"
            return 0
          fi
          sleep 1
          count=$((count + 1))
          printf "."
        done
        echo ""
        log_error "Docker failed to start in 30 seconds"
      fi
    fi
    return 1
  fi
  return 0
}

# Run essential checks only
run_essential_checks() {
  local failed=0
  local interactive="${1:-true}" # Default to interactive mode

  # Docker must be running
  if ! quick_docker_check; then
    failed=$((failed + 1))
  fi

  # Environment must exist
  if ! quick_env_check; then
    failed=$((failed + 1))
  fi

  # Check ports (non-critical)
  if ! quick_port_check; then
    if [[ "$interactive" == "true" ]]; then
      offer_port_solutions
    else
      # In non-interactive mode, just return failure
      return 1
    fi
  fi

  return $failed
}

# Offer solutions for port conflicts
offer_port_solutions() {
  echo ""
  log_info "Port conflict options:"
  echo "  1) Stop conflicting services"
  echo "  2) Use alternative ports (auto-configure)"
  echo "  3) Continue anyway (may fail)"
  echo "  4) Cancel"

  read -p "Choose option [1-4]: " -n 1 -r
  echo ""

  case $REPLY in
    1)
      stop_conflicting_services_interactive
      ;;
    2)
      configure_alternative_ports
      ;;
    3)
      log_warning "Continuing with port conflicts..."
      ;;
    4)
      log_info "Cancelled"
      exit 0
      ;;
    *)
      log_error "Invalid option"
      exit 1
      ;;
  esac
}

# Interactive service stopping
stop_conflicting_services_interactive() {
  log_info "Checking what's using the ports..."

  local ports=(80 443 5432 8080)
  for port in "${ports[@]}"; do
    if ! is_port_available $port; then
      local process=$(get_port_process $port)
      echo ""
      log_warning "Port $port is used by: $process"

      # Special handling for Docker containers
      if [[ "$process" == "com.docker"* ]] || [[ "$process" == "docker"* ]]; then
        local container=$(docker ps --format "table {{.Names}}\t{{.Ports}}" 2>/dev/null | grep ":$port->" | awk '{print $1}')
        if [[ -n "$container" ]]; then
          read -p "Stop container '$container'? [y/N]: " -n 1 -r
          echo ""
          if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker stop "$container"
            log_success "Stopped $container"
          fi
        fi
      elif [[ "$port" == "5432" ]] && [[ "$process" == "postgres"* ]]; then
        log_info "PostgreSQL is running on port 5432"
        log_info "Consider using a different port for this project"
      else
        log_info "You may need to stop '$process' manually"
      fi
    fi
  done
}

# Configure alternative ports
configure_alternative_ports() {
  log_info "Finding alternative ports..."

  local port_updates=""
  local ports_to_check=(
    "80:NGINX_HTTP_PORT"
    "443:NGINX_HTTPS_PORT"
    "5432:POSTGRES_PORT"
    "8080:HASURA_PORT"
  )

  for port_config in "${ports_to_check[@]}"; do
    IFS=':' read -r port var <<<"$port_config"

    if ! is_port_available $port; then
      local alt_port=$(find_alternative_port $port)
      if [[ -n "$alt_port" ]]; then
        log_info "Port $port → $alt_port ($var)"
        port_updates="${port_updates}\n${var}=${alt_port}"
      fi
    fi
  done

  if [[ -n "$port_updates" ]]; then
    log_info "Updating .env.local with alternative ports..."
    # Normalize line endings
    touch .env.local

    # macOS/BSD-compatible sed -i usage requires a backup extension
    while IFS='=' read -r var newval; do
      [[ -z "$var" ]] && continue
      # Remove leading newline if present
      var="$(echo "$var" | tr -d '\r\n')"
      newval="$(echo "$newval" | tr -d '\r\n')"
      if grep -qE "^${var}=" .env.local; then
        sed -i.bak "s|^${var}=.*$|${var}=${newval}|" .env.local
      else
        echo "${var}=${newval}" >>.env.local
      fi
    done <<<"$(printf "$port_updates\n")"
    rm -f .env.local.bak

    log_success "Port configuration updated"

    # Auto rebuild so new ports take effect immediately
    log_info "Rebuilding configuration with new ports..."

    # Try multiple ways to find and run build
    local build_success=false

    # Method 1: Use nself command directly
    if command -v nself >/dev/null 2>&1; then
      if nself build >/dev/null 2>&1; then
        build_success=true
      fi
    fi

    # Method 2: Find build.sh relative to this script
    if [[ "$build_success" == "false" ]]; then
      local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
      if [[ -f "$script_dir/../../cli/build.sh" ]]; then
        if bash "$script_dir/../../cli/build.sh" >/dev/null 2>&1; then
          build_success=true
        fi
      fi
    fi

    # Method 3: Check NSELF_ROOT
    if [[ "$build_success" == "false" ]] && [[ -n "${NSELF_ROOT:-}" ]]; then
      if [[ -x "$NSELF_ROOT/src/cli/build.sh" ]]; then
        if bash "$NSELF_ROOT/src/cli/build.sh" >/dev/null 2>&1; then
          build_success=true
        fi
      fi
    fi

    if [[ "$build_success" == "true" ]]; then
      log_success "Configuration rebuilt with new ports"
    else
      log_warning "Could not auto-rebuild; run 'nself build' manually"
    fi
  fi
}

export -f quick_env_check
export -f quick_port_check
export -f quick_docker_check
export -f run_essential_checks
export -f offer_port_solutions
export -f stop_conflicting_services_interactive
export -f configure_alternative_ports
