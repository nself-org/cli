#!/usr/bin/env bash


# metrics-dashboard.sh - Real-time performance metrics dashboard

# Metrics configuration
METRICS_INTERVAL="${METRICS_INTERVAL:-5}"

set -euo pipefail

METRICS_HISTORY_SIZE="${METRICS_HISTORY_SIZE:-100}"
METRICS_FILE="${METRICS_FILE:-/tmp/nself-metrics.json}"

# Initialize metrics collection
init_metrics() {
  # Create metrics file
  echo '{"metrics": [], "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' >"$METRICS_FILE"
}

# Collect system metrics
collect_system_metrics() {
  local cpu_usage=""
  local mem_usage=""
  local disk_usage=""
  local network_rx=""
  local network_tx=""

  # CPU usage
  if [[ "$(uname)" == "Linux" ]]; then
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)
  elif [[ "$(uname)" == "Darwin" ]]; then
    cpu_usage=$(top -l 1 | grep "CPU usage" | awk '{print $3}' | cut -d'%' -f1)
  fi

  # Memory usage
  if command -v free >/dev/null 2>&1; then
    local total_mem=$(free -m | grep "^Mem:" | awk '{print $2}')
    local used_mem=$(free -m | grep "^Mem:" | awk '{print $3}')
    mem_usage=$((used_mem * 100 / total_mem))
  elif [[ "$(uname)" == "Darwin" ]]; then
    mem_usage=$(memory_pressure 2>/dev/null | grep "System-wide memory free percentage" | awk '{print 100-$5}' | cut -d'%' -f1)
  fi

  # Disk usage
  disk_usage=$(df -h . | tail -1 | awk '{print $5}' | cut -d'%' -f1)

  # Network usage (simplified)
  if [[ "$(uname)" == "Linux" ]]; then
    network_rx=$(cat /sys/class/net/eth0/statistics/rx_bytes 2>/dev/null || echo "0")
    network_tx=$(cat /sys/class/net/eth0/statistics/tx_bytes 2>/dev/null || echo "0")
  else
    network_rx="0"
    network_tx="0"
  fi

  echo "{\"cpu\": $cpu_usage, \"memory\": $mem_usage, \"disk\": $disk_usage, \"network_rx\": $network_rx, \"network_tx\": $network_tx}"
}

# Collect Docker metrics
collect_docker_metrics() {
  local containers=$(docker ps --format "{{.Names}}" 2>/dev/null)
  local metrics_json="{"
  local first=true

  for container in $containers; do
    if [[ "$first" != "true" ]]; then
      metrics_json+=","
    fi
    first=false

    # Get container stats
    local stats=$(docker stats "$container" --no-stream --format "{{.CPUPerc}}|{{.MemPerc}}" 2>/dev/null || echo "0%|0%")
    IFS='|' read -r cpu mem <<<"$stats"

    # Remove % signs
    cpu=${cpu%\%}
    mem=${mem%\%}

    metrics_json+="\"$container\": {\"cpu\": $cpu, \"memory\": $mem}"
  done

  metrics_json+="}"
  echo "$metrics_json"
}

# Collect service health metrics
collect_health_metrics() {
  local total=0
  local healthy=0
  local unhealthy=0
  local starting=0

  local containers=$(docker ps --format "{{.Names}}" 2>/dev/null)

  for container in $containers; do
    total=$((total + 1))

    local health=$(docker inspect "$container" --format='{{.State.Health.Status}}' 2>/dev/null || echo "none")

    case "$health" in
      healthy | none)
        healthy=$((healthy + 1))
        ;;
      unhealthy)
        unhealthy=$((unhealthy + 1))
        ;;
      starting)
        starting=$((starting + 1))
        ;;
    esac
  done

  echo "{\"total\": $total, \"healthy\": $healthy, \"unhealthy\": $unhealthy, \"starting\": $starting}"
}

# Store metrics
store_metrics() {
  local system_metrics="$1"
  local docker_metrics="$2"
  local health_metrics="$3"
  local timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  # Read existing metrics
  local existing_metrics=$(cat "$METRICS_FILE" 2>/dev/null || echo '{"metrics": []}')

  # Add new metrics
  local new_metric="{\"timestamp\": \"$timestamp\", \"system\": $system_metrics, \"docker\": $docker_metrics, \"health\": $health_metrics}"

  # Update metrics file (keep last N entries)
  echo "$existing_metrics" | jq ".metrics += [$new_metric] | .metrics = .metrics[-$METRICS_HISTORY_SIZE:] | .timestamp = \"$timestamp\"" >"$METRICS_FILE"
}

# Display metrics dashboard
display_dashboard() {
  clear

  echo "════════════════════════════════════════════════════════════════════════════════"
  echo "                         nself Performance Metrics Dashboard                     "
  echo "════════════════════════════════════════════════════════════════════════════════"
  echo ""

  # Get latest metrics
  local latest_metrics=$(cat "$METRICS_FILE" | jq '.metrics[-1]' 2>/dev/null)

  if [[ -n "$latest_metrics" ]] && [[ "$latest_metrics" != "null" ]]; then
    # System metrics
    echo "System Resources"
    echo "────────────────────────────────────────────────────────────────────────────────"

    local cpu=$(echo "$latest_metrics" | jq '.system.cpu' 2>/dev/null || echo "0")
    local mem=$(echo "$latest_metrics" | jq '.system.memory' 2>/dev/null || echo "0")
    local disk=$(echo "$latest_metrics" | jq '.system.disk' 2>/dev/null || echo "0")

    # CPU bar
    echo -n "CPU:    "
    draw_bar "$cpu" 100
    printf " %3d%%\n" "$cpu"

    # Memory bar
    echo -n "Memory: "
    draw_bar "$mem" 100
    printf " %3d%%\n" "$mem"

    # Disk bar
    echo -n "Disk:   "
    draw_bar "$disk" 100
    printf " %3d%%\n" "$disk"

    echo ""

    # Service health
    echo "Service Health"
    echo "────────────────────────────────────────────────────────────────────────────────"

    local total=$(echo "$latest_metrics" | jq '.health.total' 2>/dev/null || echo "0")
    local healthy=$(echo "$latest_metrics" | jq '.health.healthy' 2>/dev/null || echo "0")
    local unhealthy=$(echo "$latest_metrics" | jq '.health.unhealthy' 2>/dev/null || echo "0")

    if [[ $total -gt 0 ]]; then
      local health_percent=$((healthy * 100 / total))
      echo -n "Health: "
      draw_health_bar "$health_percent"
      printf " %3d%% (%d/%d services healthy)\n" "$health_percent" "$healthy" "$total"

      if [[ $unhealthy -gt 0 ]]; then
        echo "        ⚠ $unhealthy service(s) unhealthy"
      fi
    else
      echo "No services running"
    fi

    echo ""

    # Container metrics
    echo "Container Performance"
    echo "────────────────────────────────────────────────────────────────────────────────"
    printf "%-20s %-20s %-20s\n" "Container" "CPU" "Memory"

    # Parse container metrics
    local container_metrics=$(echo "$latest_metrics" | jq -r '.docker | to_entries[] | "\(.key)|\(.value.cpu)|\(.value.memory)"' 2>/dev/null)

    while IFS='|' read -r name cpu mem; do
      printf "%-20s " "$name"

      # CPU mini bar
      draw_mini_bar "$cpu" 100
      printf " %3d%%  " "$cpu"

      # Memory mini bar
      draw_mini_bar "$mem" 100
      printf " %3d%%\n" "$mem"
    done <<<"$container_metrics"

    echo ""
    echo "────────────────────────────────────────────────────────────────────────────────"
    echo "Updated: $(date '+%Y-%m-%d %H:%M:%S') | Refresh: ${METRICS_INTERVAL}s | Press Ctrl+C to exit"
  else
    echo "Collecting metrics..."
  fi
}

# Draw progress bar
draw_bar() {
  local value=$1
  local max=$2
  local width=40
  local filled=$((value * width / max))
  local empty=$((width - filled))

  # Choose color based on value
  if [[ $value -lt 50 ]]; then
    printf "\033[32m" # Green
  elif [[ $value -lt 80 ]]; then
    printf "\033[33m" # Yellow
  else
    printf "\033[31m" # Red
  fi

  # Draw filled part
  for ((i = 0; i < filled; i++)); do
    echo -n "█"
  done

  # Reset color and draw empty part
  printf "\033[0m"
  for ((i = 0; i < empty; i++)); do
    echo -n "░"
  done
}

# Draw mini bar
draw_mini_bar() {
  local value=$1
  local max=$2
  local width=10
  local filled=$((value * width / max))
  local empty=$((width - filled))

  # Choose color
  if [[ $value -lt 50 ]]; then
    printf "\033[32m" # Green
  elif [[ $value -lt 80 ]]; then
    printf "\033[33m" # Yellow
  else
    printf "\033[31m" # Red
  fi

  # Draw bar
  for ((i = 0; i < filled; i++)); do
    echo -n "▰"
  done
  printf "\033[0m"
  for ((i = 0; i < empty; i++)); do
    echo -n "▱"
  done
}

# Draw health bar
draw_health_bar() {
  local value=$1

  if [[ $value -ge 90 ]]; then
    printf "\033[32m" # Green
  elif [[ $value -ge 70 ]]; then
    printf "\033[33m" # Yellow
  else
    printf "\033[31m" # Red
  fi

  draw_bar "$value" 100
  printf "\033[0m"
}

# Run metrics dashboard
run_dashboard() {
  init_metrics

  # Trap Ctrl+C to exit cleanly
  trap "echo ''; echo 'Dashboard stopped.'; exit 0" INT

  while true; do
    # Collect metrics
    local system_metrics=$(collect_system_metrics)
    local docker_metrics=$(collect_docker_metrics)
    local health_metrics=$(collect_health_metrics)

    # Store metrics
    store_metrics "$system_metrics" "$docker_metrics" "$health_metrics"

    # Display dashboard
    display_dashboard

    # Wait for next update
    sleep "$METRICS_INTERVAL"
  done
}

# Export functions
export -f init_metrics
export -f collect_system_metrics
export -f collect_docker_metrics
export -f collect_health_metrics
export -f run_dashboard
