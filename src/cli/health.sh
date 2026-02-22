#!/usr/bin/env bash

# health.sh - Health check management and monitoring
# v0.4.6 - Feedback implementation

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"
source "$CLI_SCRIPT_DIR/../lib/plugin/runtime.sh" 2>/dev/null || true

# Color fallbacks
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"
: "${COLOR_BOLD:=\033[1m}"

# Show help
show_health_help() {
  cat <<'EOF'
nself health - Health check management and monitoring

Usage: nself health [subcommand] [options]

Subcommands:
  check                 Run all health checks (default)
  service <name>        Check specific service health
  endpoint <url>        Check custom endpoint
  watch                 Continuous health monitoring
  history               Show health check history
  config                Configure health check settings

Options:
  --timeout N           Health check timeout in seconds (default: 30)
  --interval N          Check interval for watch mode (default: 10)
  --retries N           Number of retries on failure (default: 3)
  --env NAME            Check health in specific environment
  --json                Output in JSON format
  --quiet               Only output on failure
  -h, --help            Show this help message

Examples:
  nself health                      # Check all services + plugins
  nself health check                # Same as above
  nself health service postgres     # Check PostgreSQL only
  nself health watch                # Continuous monitoring
  nself health --env staging        # Check staging health
  nself health --json               # JSON output for tooling

Note: Installed plugins are automatically included in health checks.
  Plugin health is checked via GET /health on the plugin's configured port.
EOF
}

# Initialize health check environment
init_health() {
  load_env_with_priority

  HEALTH_DIR="${HEALTH_DIR:-.nself/health}"
  mkdir -p "$HEALTH_DIR"

  PROJECT_NAME="${PROJECT_NAME:-nself}"
  BASE_DOMAIN="${BASE_DOMAIN:-local.nself.org}"
  HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-30}"
  HEALTH_RETRIES="${HEALTH_RETRIES:-3}"
}

# Check single service health
check_service_health() {
  local service="$1"
  local container="${PROJECT_NAME}_${service}"
  local status="unknown"
  local details=""
  local response_time=0

  local start_time=$(date +%s%N)

  # Check if container exists and is running
  if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "^${container}$"; then
    # Check health status from Docker
    local health=$(docker inspect --format='{{.State.Health.Status}}' "$container" 2>/dev/null || echo "none")

    case "$health" in
      healthy)
        status="healthy"
        details="Container healthy"
        ;;
      unhealthy)
        status="unhealthy"
        details=$(docker inspect --format='{{range .State.Health.Log}}{{.Output}}{{end}}' "$container" 2>/dev/null | tail -1)
        ;;
      starting)
        status="starting"
        details="Health check pending"
        ;;
      none)
        # No health check defined, check if running
        local running=$(docker inspect --format='{{.State.Running}}' "$container" 2>/dev/null)
        if [[ "$running" == "true" ]]; then
          status="running"
          details="No health check defined"
        else
          status="stopped"
          details="Container not running"
        fi
        ;;
    esac
  else
    status="not_found"
    details="Container does not exist"
  fi

  local end_time=$(date +%s%N)
  response_time=$(((end_time - start_time) / 1000000))

  # Return JSON
  printf '{"service": "%s", "status": "%s", "details": "%s", "response_time_ms": %d}' \
    "$service" "$status" "$details" "$response_time"
}

# Check endpoint health
check_endpoint_health() {
  local url="$1"
  local name="${2:-custom}"
  local status="unknown"
  local http_code=0
  local response_time=0

  local start_time=$(date +%s%N)

  # Perform HTTP request
  local result=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$HEALTH_TIMEOUT" "$url" 2>/dev/null || echo "000")
  http_code="$result"

  local end_time=$(date +%s%N)
  response_time=$(((end_time - start_time) / 1000000))

  case "$http_code" in
    2* | 3*)
      status="healthy"
      ;;
    4*)
      status="client_error"
      ;;
    5*)
      status="server_error"
      ;;
    000)
      status="unreachable"
      ;;
    *)
      status="unknown"
      ;;
  esac

  printf '{"endpoint": "%s", "url": "%s", "status": "%s", "http_code": %d, "response_time_ms": %d}' \
    "$name" "$url" "$status" "$http_code" "$response_time"
}

# Run all health checks
cmd_check() {
  local json_mode="${JSON_OUTPUT:-false}"
  local quiet_mode="${QUIET_MODE:-false}"
  local target_env="${TARGET_ENV:-}"

  init_health

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself health" "Health Check"
    echo ""
  fi

  # Define services to check
  local services=(
    "postgres"
    "hasura"
    "auth"
    "nginx"
  )

  # Add optional services if enabled
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && services+=("redis")
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && services+=("minio")
  [[ "${MAILPIT_ENABLED:-false}" == "true" ]] && services+=("mailpit")
  [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]] && services+=("meilisearch")

  local healthy=0
  local unhealthy=0
  local total=${#services[@]}
  local results="["
  local first=true

  if [[ "$json_mode" != "true" ]]; then
    printf "${COLOR_CYAN}➞ Service Health${COLOR_RESET}\n"
    echo ""
    printf "  %-20s %-12s %-8s %s\n" "Service" "Status" "Time" "Details"
    printf "  %-20s %-12s %-8s %s\n" "-------" "------" "----" "-------"
  fi

  for service in "${services[@]}"; do
    local result=$(check_service_health "$service")
    local status=$(echo "$result" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
    local response=$(echo "$result" | grep -o '"response_time_ms": *[0-9]*' | sed 's/"response_time_ms": *//')
    local details=$(echo "$result" | grep -o '"details": *"[^"]*"' | sed 's/"details": *"\([^"]*\)"/\1/')

    if [[ "$first" != "true" ]]; then
      results+=","
    fi
    first=false
    results+="$result"

    local status_color="$COLOR_RESET"
    local status_icon="?"

    case "$status" in
      healthy | running)
        healthy=$((healthy + 1))
        status_color="$COLOR_GREEN"
        status_icon="✓"
        ;;
      unhealthy | stopped | not_found)
        unhealthy=$((unhealthy + 1))
        status_color="$COLOR_RED"
        status_icon="✗"
        ;;
      starting)
        status_color="$COLOR_YELLOW"
        status_icon="○"
        ;;
    esac

    if [[ "$json_mode" != "true" ]]; then
      if [[ "$quiet_mode" != "true" ]] || [[ "$status" != "healthy" && "$status" != "running" ]]; then
        printf "  %-20s ${status_color}%-12s${COLOR_RESET} %-8s %s\n" \
          "$service" "$status_icon $status" "${response}ms" "$details"
      fi
    fi
  done

  results+="]"

  # Check endpoints
  local endpoints=(
    "api:https://api.${BASE_DOMAIN}/healthz"
    "auth:https://auth.${BASE_DOMAIN}/healthz"
  )

  if [[ "$json_mode" != "true" ]]; then
    echo ""
    printf "${COLOR_CYAN}➞ Endpoint Health${COLOR_RESET}\n"
    echo ""
    printf "  %-20s %-12s %-8s %s\n" "Endpoint" "Status" "Time" "HTTP"
    printf "  %-20s %-12s %-8s %s\n" "--------" "------" "----" "----"
  fi

  for entry in "${endpoints[@]}"; do
    local name="${entry%%:*}"
    local url="${entry#*:}"

    local result=$(check_endpoint_health "$url" "$name")
    local status=$(echo "$result" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
    local http_code=$(echo "$result" | grep -o '"http_code": *[0-9]*' | sed 's/"http_code": *//')
    local response=$(echo "$result" | grep -o '"response_time_ms": *[0-9]*' | sed 's/"response_time_ms": *//')

    local status_color="$COLOR_RESET"
    case "$status" in
      healthy) status_color="$COLOR_GREEN" ;;
      client_error | server_error) status_color="$COLOR_RED" ;;
      unreachable) status_color="$COLOR_RED" ;;
    esac

    if [[ "$json_mode" != "true" ]]; then
      if [[ "$quiet_mode" != "true" ]] || [[ "$status" != "healthy" ]]; then
        printf "  %-20s ${status_color}%-12s${COLOR_RESET} %-8s %s\n" \
          "$name" "$status" "${response}ms" "HTTP $http_code"
      fi
    fi
  done

  # Check plugin health (if any plugins are installed)
  local plugin_dir="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
  local plugin_healthy=0
  local plugin_total=0
  if [[ -d "$plugin_dir" ]] && command -v check_plugin_health >/dev/null 2>&1; then
    local has_plugins=false
    for _pjson in "$plugin_dir"/*/plugin.json; do
      if [[ -f "$_pjson" ]]; then
        local _pname
        _pname=$(dirname "$_pjson")
        _pname=$(basename "$_pname")
        if [[ "$_pname" != "_shared" ]]; then
          has_plugins=true
          break
        fi
      fi
    done

    if [[ "$has_plugins" == "true" ]]; then
      if [[ "$json_mode" != "true" ]]; then
        echo ""
        printf "${COLOR_CYAN}➞ Plugin Health${COLOR_RESET}\n"
        echo ""
        printf "  %-20s %-12s %s\n" "Plugin" "Status" "Details"
        printf "  %-20s %-12s %s\n" "------" "------" "-------"
      fi

      for _pjson in "$plugin_dir"/*/plugin.json; do
        [[ -f "$_pjson" ]] || continue
        local _pname
        _pname=$(dirname "$_pjson")
        _pname=$(basename "$_pname")
        [[ "$_pname" == "_shared" ]] && continue

        plugin_total=$((plugin_total + 1))
        total=$((total + 1))

        if is_plugin_running "$_pname" 2>/dev/null; then
          local _port=""
          local _env_file="$plugin_dir/$_pname/ts/.env"
          if [[ -f "$_env_file" ]]; then
            _port=$(grep "^PORT=" "$_env_file" | cut -d= -f2)
          fi

          # Try health endpoint
          local _plugin_healthy=false
          if [[ -n "$_port" ]] && command -v curl >/dev/null 2>&1; then
            if curl -sf "http://localhost:$_port/health" >/dev/null 2>&1; then
              _plugin_healthy=true
            fi
          fi

          if [[ "$_plugin_healthy" == "true" ]]; then
            plugin_healthy=$((plugin_healthy + 1))
            healthy=$((healthy + 1))
            if [[ "$json_mode" != "true" ]]; then
              printf "  %-20s ${COLOR_GREEN}%-12s${COLOR_RESET} %s\n" "$_pname" "✓ healthy" "port $_port"
            fi
          else
            if [[ "$json_mode" != "true" ]]; then
              printf "  %-20s ${COLOR_YELLOW}%-12s${COLOR_RESET} %s\n" "$_pname" "○ running" "port ${_port:-unknown}"
            fi
          fi
        else
          unhealthy=$((unhealthy + 1))
          if [[ "$json_mode" != "true" ]]; then
            printf "  %-20s ${COLOR_RED}%-12s${COLOR_RESET} %s\n" "$_pname" "✗ stopped" "not running"
          fi
        fi
      done
    fi
  fi

  # Summary
  if [[ "$json_mode" == "true" ]]; then
    printf '{"timestamp": "%s", "healthy": %d, "unhealthy": %d, "total": %d, "services": %s}\n' \
      "$(date -Iseconds)" "$healthy" "$unhealthy" "$total" "$results"
  else
    echo ""
    printf "${COLOR_CYAN}➞ Summary${COLOR_RESET}\n"
    printf "  Healthy: %d/%d\n" "$healthy" "$total"
    if [[ $plugin_total -gt 0 ]]; then
      printf "  Plugins: %d/%d\n" "$plugin_healthy" "$plugin_total"
    fi

    if [[ "$unhealthy" -eq 0 ]]; then
      echo ""
      log_success "All services healthy"
    else
      echo ""
      log_warning "$unhealthy service(s) need attention"
    fi
  fi

  # Record history
  local timestamp=$(date +%Y%m%d_%H%M%S)
  printf '{"timestamp": "%s", "healthy": %d, "unhealthy": %d, "total": %d}\n' \
    "$(date -Iseconds)" "$healthy" "$unhealthy" "$total" >>"${HEALTH_DIR}/history.jsonl"

  [[ "$unhealthy" -eq 0 ]]
}

# Check specific service
cmd_service() {
  local service="$1"
  local json_mode="${JSON_OUTPUT:-false}"

  if [[ -z "$service" ]]; then
    log_error "Service name required"
    return 1
  fi

  init_health

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself health" "Service: $service"
    echo ""
  fi

  local result=$(check_service_health "$service")
  local status=$(echo "$result" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')

  if [[ "$json_mode" == "true" ]]; then
    echo "$result"
  else
    local status_color="$COLOR_RESET"
    case "$status" in
      healthy | running) status_color="$COLOR_GREEN" ;;
      unhealthy | stopped | not_found) status_color="$COLOR_RED" ;;
      starting) status_color="$COLOR_YELLOW" ;;
    esac

    printf "  Status: ${status_color}%s${COLOR_RESET}\n" "$status"

    # Get additional details
    local container="${PROJECT_NAME}_${service}"
    if docker ps --format "{{.Names}}" 2>/dev/null | grep -q "^${container}$"; then
      echo ""
      printf "${COLOR_CYAN}➞ Container Details${COLOR_RESET}\n"

      local image=$(docker inspect --format='{{.Config.Image}}' "$container" 2>/dev/null)
      local created=$(docker inspect --format='{{.Created}}' "$container" 2>/dev/null | cut -d'T' -f1)
      local restart_count=$(docker inspect --format='{{.RestartCount}}' "$container" 2>/dev/null)

      printf "  Image: %s\n" "$image"
      printf "  Created: %s\n" "$created"
      printf "  Restarts: %s\n" "$restart_count"

      # Port mappings
      echo ""
      printf "${COLOR_CYAN}➞ Port Mappings${COLOR_RESET}\n"
      docker port "$container" 2>/dev/null | sed 's/^/  /' || printf "  None\n"
    fi
  fi

  [[ "$status" == "healthy" || "$status" == "running" ]]
}

# Check custom endpoint
cmd_endpoint() {
  local url="$1"
  local json_mode="${JSON_OUTPUT:-false}"

  if [[ -z "$url" ]]; then
    log_error "URL required"
    return 1
  fi

  init_health

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself health" "Endpoint Check"
    echo ""
  fi

  local result=$(check_endpoint_health "$url" "custom")

  if [[ "$json_mode" == "true" ]]; then
    echo "$result"
  else
    local status=$(echo "$result" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
    local http_code=$(echo "$result" | grep -o '"http_code": *[0-9]*' | sed 's/"http_code": *//')
    local response=$(echo "$result" | grep -o '"response_time_ms": *[0-9]*' | sed 's/"response_time_ms": *//')

    printf "  URL: %s\n" "$url"
    printf "  Status: %s\n" "$status"
    printf "  HTTP Code: %s\n" "$http_code"
    printf "  Response Time: %sms\n" "$response"
  fi
}

# Continuous health monitoring
cmd_watch() {
  local interval="${HEALTH_INTERVAL:-10}"
  local json_mode="${JSON_OUTPUT:-false}"

  init_health

  show_command_header "nself health" "Continuous Monitoring"
  echo ""
  log_info "Checking health every ${interval}s (Ctrl+C to stop)"
  echo ""

  trap 'echo ""; log_info "Monitoring stopped"; exit 0' INT

  while true; do
    local timestamp=$(date '+%H:%M:%S')

    if [[ "$json_mode" == "true" ]]; then
      QUIET_MODE=true JSON_OUTPUT=true cmd_check 2>/dev/null
    else
      printf "\r${COLOR_DIM}[%s]${COLOR_RESET} Checking..." "$timestamp"

      local healthy=0
      local total=0

      for service in postgres hasura auth nginx; do
        total=$((total + 1))
        local result=$(check_service_health "$service")
        local status=$(echo "$result" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
        [[ "$status" == "healthy" || "$status" == "running" ]] && healthy=$((healthy + 1))
      done

      local status_color="$COLOR_GREEN"
      [[ "$healthy" -lt "$total" ]] && status_color="$COLOR_YELLOW"
      [[ "$healthy" -lt $((total / 2)) ]] && status_color="$COLOR_RED"

      printf "\r${COLOR_DIM}[%s]${COLOR_RESET} ${status_color}%d/%d healthy${COLOR_RESET}    \n" \
        "$timestamp" "$healthy" "$total"
    fi

    sleep "$interval"
  done
}

# Show health history
cmd_history() {
  local json_mode="${JSON_OUTPUT:-false}"
  local limit="${LIMIT:-20}"

  init_health

  local history_file="${HEALTH_DIR}/history.jsonl"

  if [[ ! -f "$history_file" ]]; then
    log_info "No health history recorded yet"
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself health" "Health History"
    echo ""

    printf "  %-25s %-10s %-12s\n" "Timestamp" "Healthy" "Unhealthy"
    printf "  %-25s %-10s %-12s\n" "---------" "-------" "---------"

    tail -n "$limit" "$history_file" | while read -r line; do
      local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')
      local healthy=$(echo "$line" | grep -o '"healthy": *[0-9]*' | sed 's/"healthy": *//')
      local unhealthy=$(echo "$line" | grep -o '"unhealthy": *[0-9]*' | sed 's/"unhealthy": *//')

      local status_color="$COLOR_GREEN"
      [[ "$unhealthy" -gt 0 ]] && status_color="$COLOR_YELLOW"

      printf "  %-25s ${status_color}%-10s${COLOR_RESET} %-12s\n" "$ts" "$healthy" "$unhealthy"
    done
  else
    printf '{"history": ['
    tail -n "$limit" "$history_file" | tr '\n' ',' | sed 's/,$//'
    printf ']}\n'
  fi
}

# Configure health settings
cmd_config() {
  local json_mode="${JSON_OUTPUT:-false}"

  init_health

  show_command_header "nself health" "Health Configuration"
  echo ""

  printf "${COLOR_CYAN}➞ Current Settings${COLOR_RESET}\n"
  printf "  Timeout: %s seconds\n" "$HEALTH_TIMEOUT"
  printf "  Retries: %s\n" "$HEALTH_RETRIES"
  printf "  History: %s\n" "${HEALTH_DIR}/history.jsonl"
  echo ""

  log_info "Configure via environment variables:"
  echo "  HEALTH_TIMEOUT=30"
  echo "  HEALTH_RETRIES=3"
  echo "  HEALTH_INTERVAL=10"
}

# Main command handler
cmd_health() {
  local subcommand="${1:-check}"

  # Check for help first
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]]; then
    show_health_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --timeout)
        HEALTH_TIMEOUT="$2"
        shift 2
        ;;
      --interval)
        HEALTH_INTERVAL="$2"
        shift 2
        ;;
      --retries)
        HEALTH_RETRIES="$2"
        shift 2
        ;;
      --env)
        TARGET_ENV="$2"
        shift 2
        ;;
      --limit)
        LIMIT="$2"
        shift 2
        ;;
      --json)
        JSON_OUTPUT=true
        shift
        ;;
      --quiet)
        QUIET_MODE=true
        shift
        ;;
      -h | --help)
        show_health_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments
  set -- "${args[@]}"
  subcommand="${1:-check}"

  case "$subcommand" in
    check)
      cmd_check
      ;;
    service)
      shift
      cmd_service "$@"
      ;;
    endpoint)
      shift
      cmd_endpoint "$@"
      ;;
    watch)
      cmd_watch
      ;;
    history)
      cmd_history
      ;;
    config)
      cmd_config
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_health_help
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_health

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_health_help
      exit 0
    fi
  done
  pre_command "health" || exit $?
  cmd_health "$@"
  exit_code=$?
  post_command "health" $exit_code
  exit $exit_code
fi
