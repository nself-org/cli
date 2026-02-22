#!/usr/bin/env bash
# dashboard.sh - Monitoring dashboard utilities


# Terminal control functions
term_save_cursor() {

set -euo pipefail

  tput sc 2>/dev/null || true
}

term_restore_cursor() {
  tput rc 2>/dev/null || true
}

term_clear_line() {
  tput el 2>/dev/null || true
}

term_move_up() {
  local lines="${1:-1}"
  tput cuu "$lines" 2>/dev/null || true
}

# Draw progress bar
draw_progress_bar() {
  local percent="$1"
  local width="${2:-40}"
  local label="${3:-}"

  local filled=$((percent * width / 100))
  local empty=$((width - filled))

  printf "%s [" "$label"
  printf "%${filled}s" | tr ' ' '█'
  printf "%${empty}s" | tr ' ' '░'
  printf "] %3d%%\n" "$percent"
}

# Get service health color
get_health_color() {
  local status="$1"

  if [[ "$status" == *"healthy"* ]]; then
    echo "[0;32m" # Green
  elif [[ "$status" == *"unhealthy"* ]]; then
    echo "[0;31m" # Red
  elif [[ "$status" == *"starting"* ]]; then
    echo "[0;33m" # Yellow
  elif [[ "$status" == *"Up"* ]]; then
    echo "[0;32m" # Green
  else
    echo "[0;31m" # Red
  fi
}

# Get resource usage color
get_usage_color() {
  local usage="$1"

  if [[ "$usage" -lt 50 ]]; then
    echo "[0;32m" # Green
  elif [[ "$usage" -lt 80 ]]; then
    echo "[0;33m" # Yellow
  else
    echo "[0;31m" # Red
  fi
}

# Format bytes to human readable
format_bytes() {
  local bytes="$1"
  local units=("B" "KB" "MB" "GB" "TB")
  local unit=0

  while [[ "$bytes" -ge 1024 ]] && [[ "$unit" -lt 4 ]]; do
    bytes=$((bytes / 1024))
    unit=$((unit + 1))
  done

  echo "${bytes}${units[$unit]}"
}

# Get container metrics
get_container_metrics() {
  local container="$1"

  local stats=$(docker stats --no-stream --format "{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}" "$container" 2>/dev/null || echo "0%\t0/0\t0/0\t0/0")
  echo "$stats"
}

# Get system metrics
get_system_metrics() {
  local metrics=""

  if [[ "$(uname)" == "Darwin" ]]; then
    # macOS
    local cpu=$(top -l 1 | grep "CPU usage" | awk '{print $3}' | sed 's/%//')
    local mem=$(top -l 1 | grep "PhysMem" | awk '{print $2}')
    metrics="CPU: ${cpu}%|Memory: $mem"
  else
    # Linux
    local cpu=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | sed 's/%us,//')
    local mem=$(free -h | grep "^Mem" | awk '{printf "%s/%s", $3, $2}')
    local swap=$(free -h | grep "^Swap" | awk '{printf "%s/%s", $3, $2}')
    metrics="CPU: ${cpu}%|Memory: $mem|Swap: $swap"
  fi

  echo "$metrics"
}

# Check for alerts
check_alerts() {
  local alerts=()

  # Check for unhealthy services
  local unhealthy=$(docker compose ps --format "{{.Service}}" --filter "health=unhealthy" 2>/dev/null | wc -l | xargs)
  if [[ "$unhealthy" -gt 0 ]]; then
    alerts+=("[0;31m⚠[0m $unhealthy unhealthy services")
  fi

  # Check disk space
  local disk_usage=$(df -h . | tail -1 | awk '{print $5}' | sed 's/%//')
  if [[ "$disk_usage" -gt 90 ]]; then
    alerts+=("[0;31m⚠[0m Critical disk usage: ${disk_usage}%")
  elif [[ "$disk_usage" -gt 80 ]]; then
    alerts+=("[0;33m⚠[0m High disk usage: ${disk_usage}%")
  fi

  # Check for restart loops
  local restarting=$(docker compose ps --format "{{.Service}}\t{{.Status}}" 2>/dev/null | grep -c "Restarting" || echo "0")
  if [[ "$restarting" -gt 0 ]]; then
    alerts+=("[0;31m⚠[0m $restarting services in restart loop")
  fi

  # Log alerts
  if [[ ${#alerts[@]} -gt 0 ]]; then
    for alert in "${alerts[@]}"; do
      echo "$(date '+%Y-%m-%d %H:%M:%S') $alert" >>/tmp/nself-alerts.log
    done
  fi

  printf "%s\n" "${alerts[@]}"
}

# Draw service grid
draw_service_grid() {
  local services=$(docker compose ps --format "{{.Service}}" 2>/dev/null)
  local cols=4
  local count=0

  printf "[0;36m┌────────────────┬────────────────┬────────────────┬────────────────┐[0m\n"

  local row=""
  echo "$services" | while read -r service; do
    local status=$(docker inspect "$(docker compose ps -q "$service" 2>/dev/null)" --format='{{.State.Status}}' 2>/dev/null || echo "down")
    local health=$(docker inspect "$(docker compose ps -q "$service" 2>/dev/null)" --format='{{.State.Health.Status}}' 2>/dev/null || echo "")

    local color=$(get_health_color "$status $health")
    local icon="●"
    [[ "$status" != "running" ]] && icon="○"

    row="${row}│ ${color}${icon}[0m ${service:0:12}"
    count=$((count + 1))

    if [[ $((count % cols)) -eq 0 ]]; then
      # Pad and close row
      while [[ $((count % cols)) -ne 0 ]]; do
        row="${row}│                "
        count=$((count + 1))
      done
      printf "${row}│\n"
      row=""
    fi
  done

  # Handle incomplete last row
  if [[ -n "$row" ]]; then
    while [[ $((count % cols)) -ne 0 ]]; do
      row="${row}│                "
      count=$((count + 1))
    done
    printf "${row}│\n"
  fi

  printf "[0;36m└────────────────┴────────────────┴────────────────┴────────────────┘[0m\n"
}

# Export functions
export -f term_save_cursor
export -f term_restore_cursor
export -f term_clear_line
export -f term_move_up
export -f draw_progress_bar
export -f get_health_color
export -f get_usage_color
export -f format_bytes
export -f get_container_metrics
export -f get_system_metrics
export -f check_alerts
export -f draw_service_grid
