#!/usr/bin/env bash

# parallel-logs.sh - Parallel log tailing with proper cleanup
# Part of nself v0.9.8 - Performance Optimization
# POSIX-compliant, no Bash 4+ features

# Temporary directory for log streams
LOG_STREAM_DIR="/tmp/nself-logs-$$"

set -euo pipefail

LOG_PIDS=()

# Cleanup function for parallel logs
cleanup_parallel_logs() {
  # Kill all background log processes
  for pid in "${LOG_PIDS[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done

  # Clean up temporary files
  if [[ -d "$LOG_STREAM_DIR" ]]; then
    rm -rf "$LOG_STREAM_DIR"
  fi

  LOG_PIDS=()
}

# Trap for clean exit
setup_log_traps() {
  trap cleanup_parallel_logs EXIT INT TERM
}

# Tail logs from single service in background
tail_service_logs_bg() {
  local service="$1"
  local tail_lines="${2:-50}"
  local follow="${3:-false}"
  local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

  # Create stream file
  mkdir -p "$LOG_STREAM_DIR"
  local stream_file="$LOG_STREAM_DIR/${service}.log"

  # Build docker logs command
  local docker_cmd="docker logs"

  if [[ "$follow" == "true" ]]; then
    docker_cmd="$docker_cmd --follow"
  fi

  docker_cmd="$docker_cmd --tail $tail_lines --timestamps $container_name"

  # Run in background, prefix with service name
  (
    $docker_cmd 2>&1 | while IFS= read -r line; do
      printf "%s %s\n" "$service" "$line"
    done > "$stream_file"
  ) &

  # Store PID
  LOG_PIDS+=($!)
}

# Tail logs from multiple services in parallel
parallel_tail_logs() {
  local services=("$@")
  local tail_lines="${TAIL_LINES:-50}"
  local follow_mode="${FOLLOW_MODE:-false}"

  # Setup cleanup
  setup_log_traps

  # Start tailing all services
  for service in "${services[@]}"; do
    tail_service_logs_bg "$service" "$tail_lines" "$follow_mode"
  done

  # Wait a moment for logs to start flowing
  sleep 1

  # Merge and display logs with timestamps
  if [[ "$follow_mode" == "true" ]]; then
    # Follow mode: continuously merge streams
    while true; do
      for stream_file in "$LOG_STREAM_DIR"/*.log; do
        if [[ -f "$stream_file" ]]; then
          # Read new lines and clear file
          if [[ -s "$stream_file" ]]; then
            cat "$stream_file"
            > "$stream_file"  # Truncate after reading
          fi
        fi
      done
      sleep 0.5
    done
  else
    # Non-follow mode: wait for completion
    wait "${LOG_PIDS[@]}"

    # Display all logs sorted by timestamp
    for stream_file in "$LOG_STREAM_DIR"/*.log; do
      if [[ -f "$stream_file" ]]; then
        cat "$stream_file"
      fi
    done | sort -k2,3
  fi

  # Cleanup
  cleanup_parallel_logs
}

# Color-coded parallel logs
parallel_tail_logs_colored() {
  local services=("$@")

  # Assign colors to services (cycle through available colors)
  local colors=(
    '\033[0;36m'  # Cyan
    '\033[0;32m'  # Green
    '\033[0;33m'  # Yellow
    '\033[0;35m'  # Magenta
    '\033[0;34m'  # Blue
    '\033[1;36m'  # Bright Cyan
    '\033[1;32m'  # Bright Green
    '\033[1;33m'  # Bright Yellow
  )
  local reset='\033[0m'

  local color_idx=0
  local service_colors=""

  # Build color mapping
  for service in "${services[@]}"; do
    local color="${colors[$color_idx]}"
    service_colors="${service_colors}${service}=${color}\n"
    color_idx=$(( (color_idx + 1) % ${#colors[@]} ))
  done

  # Export for subprocess use
  export SERVICE_COLOR_MAP="$service_colors"

  # Tail logs and apply colors
  parallel_tail_logs "$@" | while IFS= read -r line; do
    # Extract service name (first field)
    local service=$(echo "$line" | awk '{print $1}')
    local rest=$(echo "$line" | cut -d' ' -f2-)

    # Get color for service
    local color=$(printf '%b' "$SERVICE_COLOR_MAP" | grep "^${service}=" | cut -d'=' -f2)
    if [[ -z "$color" ]]; then
      color="$reset"
    fi

    # Print colored
    printf "${color}[%-12s]${reset} %s\n" "$service" "$rest"
  done
}

# Smart log aggregation (combines logs from services with similar patterns)
aggregate_logs_smart() {
  local services=("$@")
  local tail_lines="${TAIL_LINES:-50}"

  # Get logs from all services
  for service in "${services[@]}"; do
    local container_name="${PROJECT_NAME:-nself}_${service//-/_}"
    docker logs --tail "$tail_lines" --timestamps "$container_name" 2>&1 | \
      sed "s/^/${service} /"
  done | sort -k2,3 | uniq
}

# Get log summary (error counts, warnings, etc.)
get_parallel_log_summary() {
  local services=("$@")
  local tail_lines="${TAIL_LINES:-100}"

  printf "Service Log Summary (last %d lines)\n" "$tail_lines"
  printf "%-20s %8s %8s %8s\n" "Service" "Errors" "Warnings" "Total"
  printf "%s\n" "$(printf '%.0s─' {1..50})"

  for service in "${services[@]}"; do
    local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

    # Get counts in parallel
    local logs=$(docker logs --tail "$tail_lines" "$container_name" 2>&1)

    local error_count=$(echo "$logs" | grep -i -c "error" 2>/dev/null || echo "0")
    local warn_count=$(echo "$logs" | grep -i -c "warn" 2>/dev/null || echo "0")
    local total_lines=$(echo "$logs" | wc -l | tr -d ' ')

    printf "%-20s %8d %8d %8d\n" "$service" "$error_count" "$warn_count" "$total_lines"
  done
}

# Export functions
export -f cleanup_parallel_logs
export -f setup_log_traps
export -f tail_service_logs_bg
export -f parallel_tail_logs
export -f parallel_tail_logs_colored
export -f aggregate_logs_smart
export -f get_parallel_log_summary
