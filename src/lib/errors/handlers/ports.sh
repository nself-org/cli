#!/usr/bin/env bash


# ports.sh - Port conflict detection and resolution (Bash 3.2 compatible)

# Required ports for nself services using parallel arrays
REQUIRED_PORTS_KEYS=(nginx_http nginx_https postgres hasura auth minio minio_console redis mailpit_smtp mailpit_ui)

set -euo pipefail

REQUIRED_PORTS_VALUES=(80 443 5432 8080 4000 9000 9001 6379 1025 8025)

# Simple string-based caches
PORT_STATUS=""
PORT_CONFLICTS=""
PORT_ALTERNATIVES=""

# Check if a port is available
is_port_available() {
  local port="$1"

  # Check using lsof (macOS/Linux)
  if command -v lsof >/dev/null 2>&1; then
    if lsof -iTCP:$port -sTCP:LISTEN >/dev/null 2>&1; then
      return 1 # Port is in use
    fi
  # Fallback to netstat
  elif command -v netstat >/dev/null 2>&1; then
    if netstat -an | grep -q ":$port.*LISTEN"; then
      return 1 # Port is in use
    fi
  # Try nc (netcat)
  elif command -v nc >/dev/null 2>&1; then
    if nc -z localhost $port 2>/dev/null; then
      return 1 # Port is in use
    fi
  fi

  return 0 # Port is available
}

# Get process using a port
get_port_process() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:$port -sTCP:LISTEN 2>/dev/null | tail -1 | awk '{print $1}'
  elif command -v netstat >/dev/null 2>&1; then
    netstat -tulpn 2>/dev/null | grep ":$port" | awk '{print $NF}' | cut -d'/' -f2
  else
    echo "unknown"
  fi
}

# Find alternative port
find_alternative_port() {
  local base_port="$1"
  local max_attempts="${2:-10}"

  # Try incrementing the port
  for i in $(seq 1 $max_attempts); do
    local test_port=$((base_port + i))

    # Skip well-known ports
    if [[ $test_port -ge 1024 ]] && is_port_available $test_port; then
      echo $test_port
      return 0
    fi
  done

  # Try random high ports
  for i in $(seq 1 5); do
    local test_port=$((30000 + RANDOM % 30000))
    if is_port_available $test_port; then
      echo $test_port
      return 0
    fi
  done

  return 1
}

# Scan all required ports
scan_port_conflicts() {
  log_info "Scanning for port conflicts..."

  local conflicts_found=0

  # Bash 3.2 compatible using parallel arrays
  PORT_STATUS=""
  PORT_CONFLICTS=""
  PORT_ALTERNATIVES=""

  for i in "${!REQUIRED_PORTS_KEYS[@]}"; do
    local service="${REQUIRED_PORTS_KEYS[$i]}"
    local port="${REQUIRED_PORTS_VALUES[$i]}"

    if is_port_available $port; then
      PORT_STATUS="${PORT_STATUS}${service}=available\n"
      log_debug "✓ Port $port ($service) is available"
    else
      local process=$(get_port_process $port)
      PORT_STATUS="${PORT_STATUS}${service}=conflict\n"
      PORT_CONFLICTS="${PORT_CONFLICTS}${service}=${process}\n"
      conflicts_found=$((conflicts_found + 1))

      # Find alternative
      if alt_port=$(find_alternative_port $port); then
        PORT_ALTERNATIVES="${PORT_ALTERNATIVES}${service}=${alt_port}\n"
        log_warning "✗ Port $port ($service) is in use by: $process"
        log_info "  Alternative port available: $alt_port"
      else
        log_error "✗ Port $port ($service) is in use and no alternative found"
      fi
    fi
  done

  if [[ $conflicts_found -gt 0 ]]; then
    register_error "PORT_CONFLICTS" \
      "$conflicts_found port(s) are already in use" \
      $ERROR_MAJOR \
      "true" \
      "fix_port_conflicts"

    return 1
  else
    log_success "All required ports are available"
    return 0
  fi
}

# Fix port conflicts
fix_port_conflicts() {
  log_info "Resolving port conflicts..."

  local fixes_applied=0
  local env_updates=""

  # Bash 3.2 compatible using parallel arrays
  for i in "${!REQUIRED_PORTS_KEYS[@]}"; do
    local service="${REQUIRED_PORTS_KEYS[$i]}"
    local status=$(echo "$PORT_STATUS" | awk -F= -v k="$service" '$1==k{print $2}' | head -1)

    if [[ "$status" == "conflict" ]]; then
      local original_port="${REQUIRED_PORTS_VALUES[$i]}"
      local alt_port=$(echo "$PORT_ALTERNATIVES" | awk -F= -v k="$service" '$1==k{print $2}' | head -1)
      local process=$(echo "$PORT_CONFLICTS" | awk -F= -v k="$service" '$1==k{print $2}' | head -1)

      if [[ -n "$alt_port" ]]; then
        log_info "Updating $service port: $original_port → $alt_port"

        # Build environment variable updates
        case "$service" in
          nginx_http)
            env_updates="${env_updates}\nNGINX_HTTP_PORT=$alt_port"
            ;;
          nginx_https)
            env_updates="${env_updates}\nNGINX_HTTPS_PORT=$alt_port"
            ;;
          postgres)
            env_updates="${env_updates}\nPOSTGRES_PORT=$alt_port"
            ;;
          hasura)
            env_updates="${env_updates}\nHASURA_PORT=$alt_port"
            ;;
          auth)
            env_updates="${env_updates}\nAUTH_PORT=$alt_port"
            ;;
          minio)
            env_updates="${env_updates}\nMINIO_PORT=$alt_port"
            ;;
          redis)
            env_updates="${env_updates}\nREDIS_PORT=$alt_port"
            ;;
          mailpit_smtp)
            env_updates="${env_updates}\nMAILPIT_SMTP_PORT=$alt_port"
            ;;
          mailpit_ui)
            env_updates="${env_updates}\nMAILPIT_UI_PORT=$alt_port"
            ;;
        esac

        fixes_applied=$((fixes_applied + 1))
      else
        log_error "Cannot fix port conflict for $service (no alternative found)"
      fi
    fi
  done

  # Update .env.local with new ports
  if [[ -n "$env_updates" ]] && [[ -f ".env.local" ]]; then
    log_info "Updating .env.local with new port configurations..."

    # Backup current .env.local to _backup/timestamp structure
    local timestamp="$(date +%Y%m%d_%H%M%S)"
    local backup_dir="_backup/${timestamp}"
    mkdir -p "$backup_dir"
    cp .env.local "${backup_dir}/$(basename .env.local)"

    # Append port overrides (ensure proper newlines)
    # Ensure file ends with a newline
    if [[ -f .env.local ]]; then
      # Add newline if file doesn't end with one
      if [[ -n "$(tail -c 1 .env.local 2>/dev/null)" ]]; then
        echo "" >>.env.local
      fi
    fi
    echo "" >>.env.local
    echo "# Port overrides due to conflicts (added $(date))" >>.env.local
    printf "$env_updates\n" >>.env.local

    log_success "Updated .env.local with $fixes_applied port override(s)"
    log_info "Backup saved as .env.local.backup.*"

    # Mark that we need to rebuild
    touch .needs-rebuild

    return 0
  elif [[ $fixes_applied -eq 0 ]]; then
    log_error "Could not fix any port conflicts"
    return 1
  fi

  return 0
}

# Offer to stop conflicting services (Bash 3.2 compatible)
offer_stop_conflicts() {
  if [[ -z "$PORT_CONFLICTS" ]]; then
    return 0
  fi

  log_info "The following processes are using required ports:"
  echo ""

  for i in "${!REQUIRED_PORTS_KEYS[@]}"; do
    local service="${REQUIRED_PORTS_KEYS[$i]}"
    local port="${REQUIRED_PORTS_VALUES[$i]}"
    local process=$(echo "$PORT_CONFLICTS" | awk -F= -v k="$service" '$1==k{print $2}' | head -1)
    if [[ -n "$process" ]]; then
      echo "  • Port $port: $process"
    fi
  done

  echo ""
  read -p "Would you like to stop these processes? [y/N]: " -n 1 -r
  echo ""

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    stop_conflicting_processes
  fi
}

# Stop conflicting processes
stop_conflicting_processes() {
  log_info "Attempting to stop conflicting processes..."

  local stopped=0

  for service in "${!PORT_CONFLICTS[@]}"; do
    local port="${REQUIRED_PORTS[$service]}"
    local process="${PORT_CONFLICTS[$service]}"

    # Special handling for common services
    case "$process" in
      docker*)
        log_info "Stopping Docker container on port $port..."
        # Find and stop the container
        local container=$(docker ps --format "table {{.Names}}\t{{.Ports}}" | grep ":$port->" | awk '{print $1}')
        if [[ -n "$container" ]]; then
          if docker stop "$container"; then
            log_success "Stopped container: $container"
            stopped=$((stopped + 1))
          fi
        fi
        ;;
      postgres | postgresql)
        log_info "PostgreSQL is running on port $port"
        log_warning "Consider using a different port for this project"
        ;;
      nginx)
        log_info "Nginx is running on port $port"
        log_warning "This might be another nself project or system nginx"
        ;;
      *)
        log_info "Process '$process' is using port $port"
        log_warning "Please stop it manually if it's safe to do so"
        ;;
    esac
  done

  if [[ $stopped -gt 0 ]]; then
    log_success "Stopped $stopped conflicting process(es)"
    # Re-scan to update status
    scan_port_conflicts
  fi
}

# Check for multiple nself projects
check_multiple_projects() {
  log_info "Checking for other running nself projects..."

  # Look for nself containers
  local nself_containers=$(docker ps --filter "label=nself.project" --format "{{.Names}}" 2>/dev/null)

  if [[ -n "$nself_containers" ]]; then
    log_warning "Found other nself projects running:"
    echo "$nself_containers" | while read container; do
      echo "  • $container"
    done

    echo ""
    read -p "Stop other nself projects? [y/N]: " -n 1 -r
    echo ""

    if [[ $REPLY =~ ^[Yy]$ ]]; then
      log_info "Stopping other nself projects..."
      echo "$nself_containers" | while read container; do
        docker stop "$container" >/dev/null 2>&1
      done
      log_success "Stopped other projects"
    fi
  fi
}

# Export functions
export -f is_port_available
export -f get_port_process
export -f find_alternative_port
export -f scan_port_conflicts
export -f fix_port_conflicts
export -f offer_stop_conflicts
export -f stop_conflicting_processes
export -f check_multiple_projects
