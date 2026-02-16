#!/usr/bin/env bash

# health-check.sh - Deployment health validation
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
DEPLOY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$DEPLOY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true
source "$DEPLOY_LIB_DIR/ssh.sh" 2>/dev/null || true

# Health check configuration
HEALTH_CHECK_TIMEOUT="${HEALTH_CHECK_TIMEOUT:-120}"
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-5}"
HEALTH_CHECK_RETRIES="${HEALTH_CHECK_RETRIES:-3}"

# Check deployment health on remote server
health::check_deployment() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  log_info "Checking deployment health on $host..."

  local errors=0

  # Check Docker services
  health::check_docker_services "$host" "$deploy_path" "$user" "$port" "$key_file"
  errors=$((errors + $?))

  # Check nginx
  health::check_nginx "$host" "$user" "$port" "$key_file"
  errors=$((errors + $?))

  # Check database connectivity
  health::check_database "$host" "$deploy_path" "$user" "$port" "$key_file"
  errors=$((errors + $?))

  # Check HTTP endpoints
  health::check_http_endpoints "$host" "$user" "$port" "$key_file"
  errors=$((errors + $?))

  if [[ $errors -eq 0 ]]; then
    log_success "All health checks passed"
    return 0
  else
    log_error "$errors health check(s) failed"
    return 1
  fi
}

# Check Docker services are running
health::check_docker_services() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  printf "  Checking Docker services... "

  local check_script="
    cd '$deploy_path' 2>/dev/null || exit 1

    # Count running vs expected services
    total=\$(docker compose ps -a --format '{{.Name}}' 2>/dev/null | wc -l)
    running=\$(docker compose ps --format '{{.Name}}' --filter 'status=running' 2>/dev/null | wc -l)
    healthy=\$(docker compose ps --format '{{.Health}}' 2>/dev/null | grep -c 'healthy' || echo 0)

    echo \"total=\$total\"
    echo \"running=\$running\"
    echo \"healthy=\$healthy\"
  "

  local result
  result=$(ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null)

  if [[ -z "$result" ]]; then
    printf "${COLOR_RED}FAILED${COLOR_RESET} (could not connect)\n"
    return 1
  fi

  local total running healthy
  total=$(echo "$result" | grep "^total=" | cut -d'=' -f2)
  running=$(echo "$result" | grep "^running=" | cut -d'=' -f2)
  healthy=$(echo "$result" | grep "^healthy=" | cut -d'=' -f2)

  if [[ "$running" -eq "$total" ]] && [[ "$total" -gt 0 ]]; then
    printf "${COLOR_GREEN}OK${COLOR_RESET} (%s/%s running)\n" "$running" "$total"
    return 0
  else
    printf "${COLOR_YELLOW}WARNING${COLOR_RESET} (%s/%s running)\n" "$running" "$total"
    return 1
  fi
}

# Check nginx is responding
health::check_nginx() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"
  local key_file="${4:-}"

  printf "  Checking nginx... "

  local check_script="
    # Check nginx process
    if pgrep -x nginx >/dev/null 2>&1 || docker ps --format '{{.Names}}' | grep -q nginx; then
      # Try to get health endpoint
      if curl -sf -o /dev/null -w '%{http_code}' http://localhost/health 2>/dev/null | grep -q '200'; then
        echo 'healthy'
      elif curl -sf -o /dev/null -w '%{http_code}' http://localhost/ 2>/dev/null | grep -qE '200|301|302'; then
        echo 'responding'
      else
        echo 'running'
      fi
    else
      echo 'not_running'
    fi
  "

  local result
  result=$(ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null)

  case "$result" in
    healthy)
      printf "${COLOR_GREEN}OK${COLOR_RESET} (healthy)\n"
      return 0
      ;;
    responding)
      printf "${COLOR_GREEN}OK${COLOR_RESET} (responding)\n"
      return 0
      ;;
    running)
      printf "${COLOR_YELLOW}OK${COLOR_RESET} (running, no health endpoint)\n"
      return 0
      ;;
    not_running)
      printf "${COLOR_RED}FAILED${COLOR_RESET} (not running)\n"
      return 1
      ;;
    *)
      printf "${COLOR_RED}FAILED${COLOR_RESET} (unknown status)\n"
      return 1
      ;;
  esac
}

# Check database connectivity
health::check_database() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  printf "  Checking database... "

  local check_script="
    cd '$deploy_path' 2>/dev/null || exit 1

    # Check PostgreSQL container
    if docker compose ps postgres 2>/dev/null | grep -q 'running'; then
      # Try to connect
      if docker compose exec -T postgres pg_isready -U postgres 2>/dev/null; then
        echo 'healthy'
      else
        echo 'running'
      fi
    else
      echo 'not_running'
    fi
  "

  local result
  result=$(ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null)

  case "$result" in
    *healthy*)
      printf "${COLOR_GREEN}OK${COLOR_RESET} (accepting connections)\n"
      return 0
      ;;
    *running*)
      printf "${COLOR_YELLOW}OK${COLOR_RESET} (starting up)\n"
      return 0
      ;;
    *not_running*)
      printf "${COLOR_RED}FAILED${COLOR_RESET} (not running)\n"
      return 1
      ;;
    *)
      printf "${COLOR_YELLOW}UNKNOWN${COLOR_RESET}\n"
      return 0
      ;;
  esac
}

# Check HTTP endpoints
health::check_http_endpoints() {
  local host="$1"
  local user="${2:-root}"
  local port="${3:-22}"
  local key_file="${4:-}"

  printf "  Checking HTTP endpoints... "

  local check_script="
    endpoints_checked=0
    endpoints_healthy=0

    # Check common health endpoints
    for endpoint in 'http://localhost/health' 'http://localhost:8080/healthz' 'https://localhost/health'; do
      response=\$(curl -sf -o /dev/null -w '%{http_code}' --max-time 5 -k \"\$endpoint\" 2>/dev/null || echo '000')
      endpoints_checked=\$((endpoints_checked + 1))
      if [ \"\$response\" = '200' ]; then
        endpoints_healthy=\$((endpoints_healthy + 1))
      fi
    done

    echo \"checked=\$endpoints_checked\"
    echo \"healthy=\$endpoints_healthy\"
  "

  local result
  result=$(ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null)

  local checked healthy
  checked=$(echo "$result" | grep "^checked=" | cut -d'=' -f2)
  healthy=$(echo "$result" | grep "^healthy=" | cut -d'=' -f2)

  if [[ "${healthy:-0}" -gt 0 ]]; then
    printf "${COLOR_GREEN}OK${COLOR_RESET} (%s/%s healthy)\n" "$healthy" "$checked"
    return 0
  else
    printf "${COLOR_YELLOW}WARNING${COLOR_RESET} (no health endpoints responding)\n"
    return 0 # Not a critical failure
  fi
}

# Wait for services to become healthy
health::wait_for_healthy() {
  local host="$1"
  local deploy_path="$2"
  local timeout="${3:-$HEALTH_CHECK_TIMEOUT}"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  log_info "Waiting for services to become healthy (timeout: ${timeout}s)..."

  local start_time
  start_time=$(date +%s)
  local elapsed=0

  while [[ $elapsed -lt $timeout ]]; do
    # Check service health
    local health_result
    health_result=$(ssh::exec "$host" "
      cd '$deploy_path' 2>/dev/null || exit 1
      running=\$(docker compose ps --format '{{.Name}}' --filter 'status=running' 2>/dev/null | wc -l)
      total=\$(docker compose ps -a --format '{{.Name}}' 2>/dev/null | wc -l)
      echo \"\$running/\$total\"
    " "$user" "$port" "$key_file" 2>/dev/null)

    local running total
    running=$(echo "$health_result" | cut -d'/' -f1)
    total=$(echo "$health_result" | cut -d'/' -f2)

    # Show progress
    printf "\r  Services: %s/%s running (%ds elapsed)" "$running" "$total" "$elapsed"

    if [[ "$running" -eq "$total" ]] && [[ "$total" -gt 0 ]]; then
      printf "\n"
      log_success "All $total services are running"
      return 0
    fi

    sleep "$HEALTH_CHECK_INTERVAL"
    elapsed=$(($(date +%s) - start_time))
  done

  printf "\n"
  log_error "Timeout waiting for services to become healthy"
  return 1
}

# Check specific service health
health::check_service() {
  local host="$1"
  local service_name="$2"
  local deploy_path="$3"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  local check_script="
    cd '$deploy_path' 2>/dev/null || exit 1
    status=\$(docker compose ps '$service_name' --format '{{.Status}}' 2>/dev/null)
    health=\$(docker compose ps '$service_name' --format '{{.Health}}' 2>/dev/null)
    echo \"status=\$status\"
    echo \"health=\$health\"
  "

  ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null
}

# Get unhealthy services list
health::get_unhealthy_services() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  local check_script="
    cd '$deploy_path' 2>/dev/null || exit 1
    docker compose ps -a --format '{{.Name}} {{.Status}}' 2>/dev/null | grep -v 'running' || true
  "

  ssh::exec "$host" "$check_script" "$user" "$port" "$key_file" 2>/dev/null
}

# Perform comprehensive health check with report
health::full_report() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  printf "\n${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "${COLOR_CYAN}        Deployment Health Report${COLOR_RESET}\n"
  printf "${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n\n"

  printf "Host: %s\n" "$host"
  printf "Path: %s\n" "$deploy_path"
  printf "Time: %s\n\n" "$(date)"

  # Run all checks
  health::check_deployment "$host" "$deploy_path" "$user" "$port" "$key_file"
  local result=$?

  # Get service list
  printf "\n${COLOR_CYAN}Service Status:${COLOR_RESET}\n"
  local services
  services=$(ssh::exec "$host" "
    cd '$deploy_path' 2>/dev/null && docker compose ps --format 'table {{.Name}}\t{{.Status}}\t{{.Health}}' 2>/dev/null
  " "$user" "$port" "$key_file" 2>/dev/null)

  if [[ -n "$services" ]]; then
    echo "$services" | while IFS= read -r line; do
      printf "  %s\n" "$line"
    done
  fi

  printf "\n${COLOR_CYAN}═══════════════════════════════════════════════════${COLOR_RESET}\n"

  return $result
}

# Export functions
export -f health::check_deployment
export -f health::check_docker_services
export -f health::check_nginx
export -f health::check_database
export -f health::check_http_endpoints
export -f health::wait_for_healthy
export -f health::check_service
export -f health::get_unhealthy_services
export -f health::full_report
