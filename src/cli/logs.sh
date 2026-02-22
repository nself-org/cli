#!/usr/bin/env bash
set -euo pipefail

# logs.sh - Clean and readable logging for nself services

set +e # Don't exit on error for logging

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/docker.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/audit-logging.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/logging.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/parallel-logs.sh" 2>/dev/null || true
# Color output functions

# Function to format service names
format_service_name() {
  local name="$1"
  # Remove project prefix and clean up
  name="${name#${PROJECT_NAME}_}"
  name="${name#${PROJECT_NAME}-}"
  # Truncate long names to 12 chars (better for longer service names)
  if [[ ${#name} -gt 12 ]]; then
    echo "${name:0:12}"
  else
    printf "%-12s" "$name"
  fi
}

# Function to clean up timestamps
format_timestamp() {
  local timestamp="$1"
  # Convert Docker timestamp to readable format
  echo "$timestamp" | sed -E 's/^([0-9]{4}-[0-9]{2}-[0-9]{2})T([0-9]{2}:[0-9]{2}:[0-9]{2}).*$/\1 \2/'
}

# Function to colorize and clean log output
# Args: [service_name] - optional, used for single-service logs where service isn't in output
clean_and_colorize() {
  local fixed_service="$1" # If provided, use this instead of extracting from line

  while IFS= read -r line; do
    if [[ -z "$line" ]]; then continue; fi

    local service
    local rest

    if [[ -n "$fixed_service" ]]; then
      # Single-service mode: service name provided, line is just "timestamp message"
      service=$(format_service_name "$fixed_service")
      rest="$line"
    else
      # Multi-service mode: extract service name from first field
      local service_raw=$(echo "$line" | sed 's/ .*//')
      service=$(format_service_name "$service_raw")
      rest=$(echo "$line" | sed 's/^[^ ]* //')
    fi
    local timestamp=$(echo "$rest" | grep -o '^[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}T[0-9]\{2\}:[0-9]\{2\}:[0-9]\{2\}[^ ]*' || echo "")
    local message

    if [[ -n "$timestamp" ]]; then
      message=$(echo "$rest" | sed "s/^$timestamp *//")
      timestamp=$(format_timestamp "$timestamp")
    else
      message="$rest"
      timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    fi

    # Strip common service-embedded timestamps from message start
    # Matches: ISO timestamps, syslog-style, and bracketed timestamps
    message=$(echo "$message" | sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}:[0-9]{2}[^ ]* ?//')
    message=$(echo "$message" | sed -E 's/^\[[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}:[0-9]{2}[^\]]*\] ?//')
    message=$(echo "$message" | sed -E 's/^[A-Z][a-z]{2} [0-9]{1,2} [0-9]{2}:[0-9]{2}:[0-9]{2} ?//')

    # Skip noisy/repeated log entries
    if [[ "$QUIET_MODE" == "true" ]]; then
      # Filter out common noise
      if echo "$message" | grep -q -E "(healthz|health check|GET /healthz|Ready to accept connections|checkpoint)"; then
        continue
      fi
    fi

    # Apply search filter
    if [[ -n "$SEARCH_PATTERN" ]] && ! echo "$message" | grep -i -q "$SEARCH_PATTERN"; then
      continue
    fi

    # Apply level filter
    if [[ -n "$FILTER_LEVEL" ]]; then
      case "$FILTER_LEVEL" in
        error | ERROR)
          if ! echo "$message" | grep -i -q -E "(error|fatal|ERROR|FATAL)"; then continue; fi
          ;;
        warn | WARNING)
          if ! echo "$message" | grep -i -q -E "(warn|warning|WARN|WARNING)"; then continue; fi
          ;;
      esac
    fi

    # Show errors only mode
    if [[ "$SHOW_ERRORS_ONLY" == "true" ]]; then
      if ! echo "$message" | grep -i -q -E "(error|fatal|ERROR|FATAL|exception|Exception)"; then
        continue
      fi
    fi

    # Format output based on mode
    if [[ "$COMPACT_MODE" == "true" ]]; then
      # Compact: [service] message
      printf "\033[1;36m[%s]\033[0m %s\n" "$service" "$(echo "$message" | sed 's/^[[:space:]]*//')"
    else
      # Full: timestamp [service] message with colors
      local colored_message="$message"
      colored_message=$(echo "$colored_message" | sed -E 's/(ERROR|error|FATAL|fatal)/\x1b[1;31m\1\x1b[0m/g')
      colored_message=$(echo "$colored_message" | sed -E 's/(WARN|warn|WARNING|warning)/\x1b[1;33m\1\x1b[0m/g')
      colored_message=$(echo "$colored_message" | sed -E 's/(INFO|info)/\x1b[1;32m\1\x1b[0m/g')

      printf "\033[1;90m%s\033[0m \033[1;36m[%-12s]\033[0m %s\n" \
        "$timestamp" "$service" "$colored_message"
    fi
  done
}

# Function to get summary of recent errors
show_error_summary() {
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found"
    return 1
  fi

  show_header "Recent Errors (last 100 lines per service)"
  echo ""

  local services=($(compose config --services 2>/dev/null))
  # Sort services in display order
  local sorted_services=($(sort_services "${services[@]}"))
  local error_found=false

  for service in "${sorted_services[@]}"; do
    # Replace hyphens with underscores in container name (Docker naming convention)
    local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

    if ! compose ps "$service" --filter "status=running" >/dev/null 2>&1; then
      continue
    fi

    local errors=$(docker logs --tail 100 "$container_name" 2>&1 |
      grep -i -E "(error|fatal|exception|failed|problem)" |
      head -5)

    if [[ -n "$errors" ]]; then
      if [[ "$error_found" == "false" ]]; then
        error_found=true
      fi
      printf "\033[1;31m● $service\033[0m\n"
      echo "$errors" | while read -r line; do
        echo "  $(echo "$line" | sed 's/^.*[0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9][^ ]* *//' | cut -c1-100)"
      done
      echo ""
    fi
  done

  if [[ "$error_found" == "false" ]]; then
    log_success "No recent errors found in running services"
  fi
}

# Function to get logs from specific service (Docker or plugin)
get_service_logs() {
  local service="$1"
  # Replace hyphens with underscores in container name (Docker naming convention)
  local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

  # Check if it's a plugin FIRST (before Docker, since compose ps may not
  # reliably return non-zero for unknown services in all environments)
  local plugin_dir="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
  local plugin_log="$HOME/.nself/runtime/logs/${service}.log"
  if [[ -f "$plugin_dir/$service/plugin.json" ]]; then
    # It's a plugin - show plugin logs
    if [[ ! -f "$plugin_log" ]]; then
      log_warning "No logs for plugin '$service' (not started yet)"
      printf "Start with: nself plugin start %s\n" "$service"
      return 1
    fi
    if [[ "$FOLLOW_MODE" == "true" ]]; then
      tail -f "$plugin_log" | clean_and_colorize "$service"
    else
      tail -"$TAIL_LINES" "$plugin_log" | clean_and_colorize "$service"
    fi
    return 0
  fi

  # Not a plugin — check if service exists in Docker
  if ! compose ps "$service" >/dev/null 2>&1; then
    log_error "Service '$service' not found"
    log_info "Available services: $(compose config --services 2>/dev/null | tr '\n' ' ')"
    # Show available plugins too
    if [[ -d "$plugin_dir" ]]; then
      local plugins=""
      for _pj in "$plugin_dir"/*/plugin.json; do
        [[ -f "$_pj" ]] || continue
        local _pn
        _pn=$(dirname "$_pj")
        _pn=$(basename "$_pn")
        [[ "$_pn" == "_shared" ]] && continue
        plugins="$plugins $_pn"
      done
      if [[ -n "$plugins" ]]; then
        log_info "Available plugins:$plugins"
      fi
    fi
    return 1
  fi

  # Check if service is running
  if ! compose ps "$service" --filter "status=running" >/dev/null 2>&1; then
    log_warning "Service '$service' is not running"
    log_info "Showing last logs before shutdown..."
  fi

  # Build docker logs command
  local docker_args=("logs")

  if [[ "$FOLLOW_MODE" == "true" ]]; then
    docker_args+=("--follow")
  fi

  docker_args+=("--tail" "$TAIL_LINES" "--timestamps" "$container_name")

  # Execute and process logs safely without eval
  # Pass service name for single-service mode
  docker "${docker_args[@]}" 2>&1 | clean_and_colorize "$service"
}

# Function to get logs from all services
get_all_logs() {
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found"
    log_info "Run 'nself build' to generate project structure"
    return 1
  fi

  local services=($(compose config --services 2>/dev/null))

  # Sort services in display order (Core → Optional → Monitoring → Custom)
  local sorted_services=($(sort_services "${services[@]}"))
  local running_services=()

  # Filter to only running services
  for service in "${sorted_services[@]}"; do
    if compose ps "$service" --filter "status=running" >/dev/null 2>&1; then
      running_services+=("$service")
    fi
  done

  if [[ ${#running_services[@]} -eq 0 ]]; then
    log_warning "No services are currently running"
    log_info "Run 'nself start' to start services"
    return 1
  fi

  # Performance: Use parallel log tailing for multiple services (v0.9.8)
  if [[ "$FOLLOW_MODE" == "true" ]] && [[ ${#running_services[@]} -gt 3 ]] && command -v parallel_tail_logs_colored >/dev/null 2>&1; then
    if [[ "$COMPACT_MODE" != "true" ]]; then
      show_header "Following logs from ${#running_services[@]} services (parallel mode)"
      echo ""
    fi
    # Use parallel log tailing with color coding
    parallel_tail_logs_colored "${running_services[@]}"
    return $?
  fi

  if [[ "$COMPACT_MODE" != "true" ]]; then
    if [[ $TAIL_LINES -eq 10 ]]; then
      show_header "Recent logs from ${#running_services[@]} services"
    else
      show_header "Logs from ${#running_services[@]} services (last ${TAIL_LINES} lines each)"
    fi
    echo ""
  fi

  # Build compose logs command safely
  local compose_args=("logs")

  if [[ "$FOLLOW_MODE" == "true" ]]; then
    compose_args+=("--follow")
  fi

  compose_args+=("--tail=$TAIL_LINES" "--timestamps")
  compose_args+=("${running_services[@]}")

  # Execute and process logs safely without eval
  compose "${compose_args[@]}" 2>&1 | clean_and_colorize
}

# Function to show service status with log info
show_service_status() {
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found"
    return 1
  fi

  show_header "Service Status & Recent Activity"
  echo ""

  local services=($(compose config --services 2>/dev/null))
  # Sort services in display order
  local sorted_services=($(sort_services "${services[@]}"))
  local running=0
  local with_errors=0

  for service in "${sorted_services[@]}"; do
    # Replace hyphens with underscores in container name (Docker naming convention)
    local container_name="${PROJECT_NAME:-nself}_${service//-/_}"
    local clean_name=$(format_service_name "${PROJECT_NAME:-nself}_${service}")

    if compose ps "$service" --filter "status=running" >/dev/null 2>&1; then
      running=$((running + 1))
      # Count errors but exclude deprecation warnings
      local recent_errors=$(docker logs --tail 50 "$container_name" 2>&1 |
        grep -i -E "(error|fatal)" |
        grep -v -i -E "(deprecat|deprecated)" |
        wc -l | tr -d ' ')
      recent_errors=$(echo "$recent_errors" | tr -d '\n\r ')

      if [[ $recent_errors -gt 0 ]]; then
        with_errors=$((with_errors + 1))
        printf "\033[1;31m● %-12s\033[0m running (%s recent errors)\n" "$clean_name" "$recent_errors"
      else
        printf "\033[1;32m● %-12s\033[0m running\n" "$clean_name"
      fi
    else
      printf "\033[1;37m○ %-12s\033[0m stopped\n" "$clean_name"
    fi
  done

  echo ""
  log_info "$running/${#services[@]} services running"
  if [[ $with_errors -gt 0 ]]; then
    log_warning "$with_errors services have recent errors"
    log_info "Use 'nself logs --errors' to see error details"
  fi
}

# Function to show top log producers
show_top_talkers() {
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found"
    return 1
  fi

  show_header "Most Active Services (last 100 lines)"
  echo ""

  local services=($(compose config --services 2>/dev/null))
  local service_data=""

  for service in "${services[@]}"; do
    # Replace hyphens with underscores in container name (Docker naming convention)
    local container_name="${PROJECT_NAME:-nself}_${service//-/_}"
    local clean_name=$(format_service_name "${PROJECT_NAME:-nself}_${service}")

    if compose ps "$service" --filter "status=running" >/dev/null 2>&1; then
      local line_count=$(docker logs --tail 100 "$container_name" 2>&1 | wc -l | tr -d ' ')
      service_data="${service_data}${line_count}:${clean_name}\n"
    fi
  done

  # Sort by line count and show top 5
  printf "$service_data\n" | sort -rn | head -5 | while IFS=: read -r count service; do
    if [[ -n "$service" ]]; then
      if [[ $count -gt 20 ]]; then
        printf "\033[1;33m● %-12s\033[0m %s lines\n" "$service" "$count"
      else
        printf "\033[1;32m● %-12s\033[0m %s lines\n" "$service" "$count"
      fi
    fi
  done
}

# Function to show audit logs
show_audit_logs() {
  local filter_category="${1:-}"
  local filter_action="${2:-}"
  local limit="${3:-50}"

  if [[ "$NSELF_AUDIT_ENABLED" != "true" ]]; then
    log_warning "Audit logging is not enabled"
    log_info "Set NSELF_AUDIT_ENABLED=true in .env to enable"
    return 1
  fi

  show_header "Audit Trail"
  echo ""

  if [[ -n "$filter_category" ]]; then
    audit_query "$filter_category" "$filter_action"
  else
    # Show recent audit events
    local audit_path="${NSELF_AUDIT_DIR}/${NSELF_AUDIT_FILE}"
    if [[ -f "$audit_path" ]]; then
      log_info "Recent audit events (last ${limit}):"
      echo ""
      grep -v '^#' "$audit_path" 2>/dev/null | tail -"$limit" | while IFS='|' read -r timestamp event_id category action user hostname pid details checksum; do
        printf "%b[%s]%b %s - %s (user: %s)\n" \
          "${COLOR_CYAN}" "$category" "${COLOR_RESET}" \
          "${timestamp%% UTC*}" "$action" "$user"
      done
    else
      log_warning "No audit logs found"
    fi
  fi
}

# Function to show help
show_help() {
  echo "nself logs - Clean and readable service logs"
  echo ""
  echo "Usage: nself logs [options] [service]"
  echo ""
  echo "Quick Options:"
  echo "  (no args)                 Show last 10 lines from all services"
  echo "  --more                    Show last 50 lines from all services"
  echo "  --all                     Show last 100 lines from all services"
  echo "  -f, --follow              Follow log output (live mode)"
  echo "  -e, --errors              Show only errors and exceptions"
  echo "  -q, --quiet               Filter out noise (healthchecks, etc.)"
  echo ""
  echo "Detailed Options:"
  echo "  -n, --tail LINES          Number of lines to show (default: 10)"
  echo "  -s, --search PATTERN      Search for pattern in logs"
  echo "  -l, --level LEVEL         Filter by level (error, warn)"
  echo "  -c, --compact             Compact output format"
  echo "  --status                  Show service status overview"
  echo "  --summary                 Show recent errors summary"
  echo "  --top                     Show most active services"
  echo "  -h, --help                Show this help message"
  echo ""
  echo "Audit Logs:"
  echo "  --audit [category]        Show audit trail (security events)"
  echo "  --audit-stats             Show audit log statistics"
  echo "  --audit-export FILE       Export audit logs to file"
  echo ""
  echo "Examples:"
  echo "  nself logs                      # Last 10 lines, all services"
  echo "  nself logs --more               # Last 50 lines, all services"
  echo "  nself logs postgres             # Last 10 lines, postgres only"
  echo "  nself logs epg                  # Plugin logs (auto-detected)"
  echo "  nself logs -f                   # Follow all services (live)"
  echo "  nself logs -e                   # Show only errors"
  echo "  nself logs -q                   # Quiet mode (filter noise)"
  echo "  nself logs --summary            # Recent errors by service"
  echo "  nself logs -n 25 -s database    # Last 25 lines containing 'database'"
  echo "  nself logs --audit              # Show audit trail"
  echo "  nself logs --audit AUTH         # Show authentication events"
  echo "  nself logs --audit-export audit.csv  # Export to CSV"
  echo ""
  echo "Output format:"
  echo "  • Service names are cleaned and colored"
  echo "  • Timestamps are readable (YYYY-MM-DD HH:MM:SS)"
  echo "  • Errors are highlighted in red, warnings in yellow"
  echo "  • Default: 10 lines (quick overview)"
}

# Main function
main() {
  local show_status=false
  local show_summary=false
  local show_top=false
  local show_audit=false
  local audit_category=""
  local audit_stats=false
  local audit_export=""

  # Initialize variables
  FOLLOW_MODE=false
  TAIL_LINES=10
  SEARCH_PATTERN=""
  FILTER_LEVEL=""
  SHOW_ERRORS_ONLY=false
  COMPACT_MODE=false
  QUIET_MODE=false
  SERVICE_NAME=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case $1 in
      -f | --follow)
        FOLLOW_MODE=true
        shift
        ;;
      -n | --tail | --lines)
        TAIL_LINES="$2"
        if ! [[ "$TAIL_LINES" =~ ^[0-9]+$ ]]; then
          log_error "Invalid tail lines: $TAIL_LINES"
          exit 1
        fi
        shift 2
        ;;
      --more)
        TAIL_LINES=50
        shift
        ;;
      --all)
        TAIL_LINES=100
        shift
        ;;
      -s | --search)
        SEARCH_PATTERN="$2"
        shift 2
        ;;
      -l | --level)
        FILTER_LEVEL="$2"
        shift 2
        ;;
      -e | --errors)
        SHOW_ERRORS_ONLY=true
        shift
        ;;
      -c | --compact)
        COMPACT_MODE=true
        shift
        ;;
      -q | --quiet)
        QUIET_MODE=true
        shift
        ;;
      --status)
        show_status=true
        shift
        ;;
      --summary)
        show_summary=true
        shift
        ;;
      --top)
        show_top=true
        shift
        ;;
      --audit)
        show_audit=true
        if [[ -n "${2:-}" ]] && [[ ! "$2" =~ ^- ]]; then
          audit_category="$2"
          shift
        fi
        shift
        ;;
      --audit-stats)
        audit_stats=true
        shift
        ;;
      --audit-export)
        audit_export="$2"
        shift 2
        ;;
      -h | --help)
        show_help
        exit 0
        ;;
      -*)
        log_error "Unknown option: $1"
        log_info "Use 'nself logs --help' for usage information"
        exit 1
        ;;
      *)
        SERVICE_NAME="$1"
        shift
        ;;
    esac
  done

  # Load environment
  load_env_with_priority

  # Handle audit logging modes first
  if [[ "$audit_stats" == "true" ]]; then
    audit_stats
    exit 0
  fi

  if [[ -n "$audit_export" ]]; then
    local format="${audit_export##*.}"
    [[ "$format" != "csv" && "$format" != "json" && "$format" != "txt" ]] && format="txt"
    audit_export "$audit_export" "$format"
    exit 0
  fi

  if [[ "$show_audit" == "true" ]]; then
    show_audit_logs "$audit_category"
    exit 0
  fi

  # Show command header (not for help mode)
  if [[ "$show_status" != "true" && "$show_summary" != "true" && "$show_top" != "true" ]]; then
    show_command_header "nself logs" "View and monitor service logs"
  fi

  # Handle special modes
  if [[ "$show_status" == "true" ]]; then
    show_service_status
    exit 0
  fi

  if [[ "$show_summary" == "true" ]]; then
    show_error_summary
    exit 0
  fi

  if [[ "$show_top" == "true" ]]; then
    show_top_talkers
    exit 0
  fi

  # Setup signal handlers for follow mode
  if [[ "$FOLLOW_MODE" == "true" ]]; then
    trap 'printf "\n\nLog following stopped\n"; exit 0' INT
    if [[ "$COMPACT_MODE" != "true" ]]; then
      show_header "Following logs... (Press Ctrl+C to stop)"
      echo ""
    fi
  fi

  # Main log display
  if [[ -n "$SERVICE_NAME" ]]; then
    get_service_logs "$SERVICE_NAME"
  else
    get_all_logs
  fi
}

# Run main function
main "$@"
