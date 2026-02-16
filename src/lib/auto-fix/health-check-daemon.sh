#!/usr/bin/env bash

# Health Check Daemon
# Runs periodic health checks and applies fixes automatically

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/service-health-monitor.sh"

# Configuration
CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-60}" # Check every 60 seconds by default
MAX_CONSECUTIVE_FAILURES=3
DAEMON_PID_FILE="/tmp/.nself_health_daemon_${PROJECT_NAME:-nself}.pid"
DAEMON_LOG_FILE="/tmp/.nself_health_daemon_${PROJECT_NAME:-nself}.log"

# Track consecutive failures using a file
CONSECUTIVE_FAILURES_FILE="/tmp/.nself_consecutive_failures_${PROJECT_NAME:-nself}"

# Check if daemon is already running
is_daemon_running() {
  if [[ -f "$DAEMON_PID_FILE" ]]; then
    local pid=$(cat "$DAEMON_PID_FILE")
    if ps -p "$pid" >/dev/null 2>&1; then
      return 0
    fi
  fi
  return 1
}

# Start the health check daemon
start_health_daemon() {
  if is_daemon_running; then
    log_info "Health daemon already running (PID: $(cat "$DAEMON_PID_FILE"))"
    return 0
  fi

  log_info "Starting health check daemon..."

  # Run in background
  (
    echo $$ >"$DAEMON_PID_FILE"

    while true; do
      # Run health check
      if monitor_all_services >>"$DAEMON_LOG_FILE" 2>&1; then
        # Reset failure counters on success
        rm -f "$CONSECUTIVE_FAILURES_FILE"
      else
        # Track failures
        for container in $(docker ps -a --filter "label=com.docker.compose.project=${PROJECT_NAME:-nself}" --format "{{.Names}}"); do
          local service_type=$(echo "$container" | sed "s/${PROJECT_NAME:-nself}_//")
          if ! check_service_health "$container" "$service_type"; then
            # Get current failure count
            local failures=0
            if [[ -f "$CONSECUTIVE_FAILURES_FILE" ]]; then
              failures=$(grep "^$container:" "$CONSECUTIVE_FAILURES_FILE" 2>/dev/null | cut -d: -f2 || echo "0")
            fi
            failures=$((failures + 1))

            # Update failure count
            if [[ -f "$CONSECUTIVE_FAILURES_FILE" ]]; then
              grep -v "^$container:" "$CONSECUTIVE_FAILURES_FILE" >"${CONSECUTIVE_FAILURES_FILE}.tmp" 2>/dev/null || true
              mv "${CONSECUTIVE_FAILURES_FILE}.tmp" "$CONSECUTIVE_FAILURES_FILE"
            fi
            echo "$container:$failures" >>"$CONSECUTIVE_FAILURES_FILE"

            # Alert if too many consecutive failures
            if [[ $failures -ge $MAX_CONSECUTIVE_FAILURES ]]; then
              echo "$(date '+%Y-%m-%d %H:%M:%S') WARNING: $container has failed $MAX_CONSECUTIVE_FAILURES consecutive health checks" >>"$DAEMON_LOG_FILE"
            fi
          else
            # Reset failures for this container
            if [[ -f "$CONSECUTIVE_FAILURES_FILE" ]]; then
              grep -v "^$container:" "$CONSECUTIVE_FAILURES_FILE" >"${CONSECUTIVE_FAILURES_FILE}.tmp" 2>/dev/null || true
              mv "${CONSECUTIVE_FAILURES_FILE}.tmp" "$CONSECUTIVE_FAILURES_FILE"
            fi
          fi
        done
      fi

      sleep "$CHECK_INTERVAL"
    done
  ) &

  local daemon_pid=$!
  echo $daemon_pid >"$DAEMON_PID_FILE"
  log_success "Health daemon started (PID: $daemon_pid)"

  # Give it a moment to start
  sleep 2

  if is_daemon_running; then
    return 0
  else
    log_error "Failed to start health daemon"
    return 1
  fi
}

# Stop the health check daemon
stop_health_daemon() {
  if ! is_daemon_running; then
    log_info "Health daemon not running"
    return 0
  fi

  local pid=$(cat "$DAEMON_PID_FILE")
  log_info "Stopping health daemon (PID: $pid)..."

  kill "$pid" 2>/dev/null
  rm -f "$DAEMON_PID_FILE"

  log_success "Health daemon stopped"
}

# Get daemon status
daemon_status() {
  if is_daemon_running; then
    local pid=$(cat "$DAEMON_PID_FILE")
    log_success "Health daemon is running (PID: $pid)"

    if [[ -f "$DAEMON_LOG_FILE" ]]; then
      echo ""
      echo "Recent activity:"
      tail -n 10 "$DAEMON_LOG_FILE"
    fi
    return 0
  else
    log_info "Health daemon is not running"
    return 1
  fi
}

# Export functions
export -f start_health_daemon
export -f stop_health_daemon
export -f daemon_status

# Handle command line arguments
case "${1:-}" in
  start)
    start_health_daemon
    ;;
  stop)
    stop_health_daemon
    ;;
  restart)
    stop_health_daemon
    start_health_daemon
    ;;
  status)
    daemon_status
    ;;
  *)
    if [[ -n "${1:-}" ]]; then
      echo "Usage: $0 {start|stop|restart|status}"
      exit 1
    fi
    ;;
esac
