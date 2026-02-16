#!/usr/bin/env bash
# monitor.sh - Monitoring dashboard integration

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source utilities
source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/utils/env.sh"
source "$SCRIPT_DIR/../lib/config/defaults.sh"

# Load environment configuration
if [[ -f .env.local ]]; then
  set -a
  load_env_with_priority
  set +a
elif [[ -f .env ]]; then
  set -a
  load_env_with_priority
  set +a
fi

# Command function
cmd_monitor() {
  local mode="${1:-dashboard}"

  # Check for help
  if [[ "$mode" == "--help" ]] || [[ "$mode" == "-h" ]]; then
    show_monitor_help
    return 0
  fi

  # Check if monitoring is enabled
  if [[ "${MONITORING_ENABLED:-false}" != "true" ]]; then
    log_error "Monitoring is not enabled"
    echo ""
    echo "To enable monitoring, run:"
    echo "  nself metrics enable"
    return 1
  fi

  case "$mode" in
    dashboard | grafana)
      # Open Grafana dashboard
      open_grafana_dashboard
      ;;
    prometheus)
      # Open Prometheus UI
      open_prometheus_ui
      ;;
    loki)
      # Open Loki through Grafana
      open_loki_dashboard
      ;;
    alerts | alertmanager)
      # Open Alertmanager UI
      open_alertmanager_ui
      ;;
    services)
      # Show service status locally
      show_service_status
      ;;
    resources)
      # Show resource usage locally
      show_resource_usage
      ;;
    logs)
      # Tail logs locally
      tail_service_logs
      ;;
    *)
      log_error "Unknown monitor mode: $mode"
      show_monitor_help
      return 1
      ;;
  esac
}

# Show help
show_monitor_help() {
  echo "nself monitor - Monitoring dashboard integration"
  echo ""
  echo "Usage: nself monitor [mode] [options]"
  echo ""
  echo "Modes:"
  echo "  dashboard    Open Grafana dashboard (default)"
  echo "  grafana      Open Grafana dashboard (alias)"
  echo "  prometheus   Open Prometheus UI"
  echo "  loki         Open Loki in Grafana"
  echo "  alerts       Open Alertmanager UI"
  echo "  services     Show service status (CLI)"
  echo "  resources    Show resource usage (CLI)"
  echo "  logs         Tail service logs (CLI)"
  echo ""
  echo "Options:"
  echo "  -h, --help   Show this help message"
  echo ""
  echo "Examples:"
  echo "  nself monitor                # Open Grafana"
  echo "  nself monitor prometheus      # Open Prometheus"
  echo "  nself monitor services        # Show service status in CLI"
  echo "  nself monitor logs            # Tail logs in CLI"
  echo ""
  echo "Note: Monitoring must be enabled first with 'nself metrics enable'"
}

# Open Grafana dashboard
open_grafana_dashboard() {
  local url="https://grafana.${BASE_DOMAIN:-local.nself.org}"

  log_info "Opening Grafana dashboard..."
  echo "URL: $url"
  echo ""
  echo "Credentials:"
  echo "  Username: ${GRAFANA_ADMIN_USER:-admin}"
  if [[ -n "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
    echo "  Password: Check .env.local or .env.secrets for GRAFANA_ADMIN_PASSWORD"
  else
    log_error "GRAFANA_ADMIN_PASSWORD not set - monitoring may not be properly configured"
    echo "  Run 'nself metrics enable' to configure monitoring properly"
  fi
  echo ""

  # Try to open in browser
  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    echo ""
    echo "Please open manually: $url"
  fi
}

# Open Prometheus UI
open_prometheus_ui() {
  if [[ "${PROMETHEUS_WEB_ENABLE:-true}" != "true" ]]; then
    log_error "Prometheus web UI is disabled"
    echo "Set PROMETHEUS_WEB_ENABLE=true to enable it"
    return 1
  fi

  local url="https://prometheus.${BASE_DOMAIN:-local.nself.org}"

  log_info "Opening Prometheus UI..."
  echo "URL: $url"

  # Try to open in browser
  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    echo ""
    echo "Please open manually: $url"
  fi
}

# Open Loki dashboard
open_loki_dashboard() {
  if [[ "${MONITORING_LOGS:-false}" != "true" ]]; then
    log_error "Loki logging is not enabled"
    echo "Enable it with: nself metrics profile standard"
    return 1
  fi

  local url="https://grafana.${BASE_DOMAIN:-local.nself.org}/explore?orgId=1&left=%5B%22now-1h%22,%22now%22,%22Loki%22,%7B%7D%5D"

  log_info "Opening Loki in Grafana Explore..."
  echo "URL: $url"

  # Try to open in browser
  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    echo ""
    echo "Please open manually in Grafana → Explore → Loki"
  fi
}

# Open Alertmanager UI
open_alertmanager_ui() {
  if [[ "${MONITORING_ALERTS:-false}" != "true" ]]; then
    log_error "Alertmanager is not enabled"
    echo "Enable it with: nself metrics profile full"
    return 1
  fi

  local url="https://alerts.${BASE_DOMAIN:-local.nself.org}"

  log_info "Opening Alertmanager UI..."
  echo "URL: $url"

  # Try to open in browser
  if command -v open &>/dev/null; then
    open "$url"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$url"
  else
    echo ""
    echo "Please open manually: $url"
  fi
}

# Show service status
show_service_status() {
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║                     SERVICE STATUS                           ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo ""

  # Core services
  echo "Core Services:"
  check_container_status "nginx" "Nginx"
  check_container_status "postgres" "PostgreSQL"
  check_container_status "hasura" "Hasura"
  check_container_status "auth" "Auth"
  check_container_status "minio" "MinIO"
  check_container_status "storage" "Storage"

  # Optional services
  echo ""
  echo "Optional Services:"
  [[ "${FUNCTIONS_ENABLED:-}" == "true" ]] && check_container_status "functions" "Functions"
  [[ "${REDIS_ENABLED:-}" == "true" ]] && check_container_status "redis" "Redis"
  [[ "${ADMIN_ENABLED:-}" == "true" ]] && check_container_status "admin" "Admin UI"

  # Monitoring services
  if [[ "${MONITORING_ENABLED:-}" == "true" ]]; then
    echo ""
    echo "Monitoring Stack:"
    check_container_status "prometheus" "Prometheus"
    check_container_status "grafana" "Grafana"
    check_container_status "cadvisor" "cAdvisor"
    [[ "${MONITORING_LOGS:-}" == "true" ]] && check_container_status "loki" "Loki"
    [[ "${MONITORING_LOGS:-}" == "true" ]] && check_container_status "promtail" "Promtail"
    [[ "${MONITORING_TRACING:-}" == "true" ]] && check_container_status "tempo" "Tempo"
    [[ "${MONITORING_ALERTS:-}" == "true" ]] && check_container_status "alertmanager" "Alertmanager"
  fi
}

# Check container status
check_container_status() {
  local service="$1"
  local display_name="$2"
  local container_name="${PROJECT_NAME:-nself}_${service}"

  if docker ps --format "{{.Names}}" | grep -q "^${container_name}$"; then
    local stats=$(docker inspect "$container_name" --format='{{.State.Status}} | {{.State.Health.Status}}' 2>/dev/null || echo "unknown")
    local status=$(echo "$stats" | cut -d'|' -f1 | tr -d ' ')
    local health=$(echo "$stats" | cut -d'|' -f2 | tr -d ' ')

    if [[ "$health" == "healthy" ]] || [[ "$health" == "<novalue>" && "$status" == "running" ]]; then
      printf "  • %-20s $(color_text "● Running" "green")\n" "$display_name:"
    elif [[ "$health" == "unhealthy" ]]; then
      printf "  • %-20s $(color_text "● Unhealthy" "red")\n" "$display_name:"
    else
      printf "  • %-20s $(color_text "● Starting" "yellow")\n" "$display_name:"
    fi
  else
    printf "  • %-20s $(color_text "○ Stopped" "gray")\n" "$display_name:"
  fi
}

# Show resource usage
show_resource_usage() {
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║                    RESOURCE USAGE                            ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo ""

  # Get all nself containers
  local containers=$(docker ps --format "{{.Names}}" | grep "^${PROJECT_NAME:-nself}_" | sort)

  if [[ -z "$containers" ]]; then
    log_error "No nself containers are running"
    return 1
  fi

  # Header
  printf "%-30s %10s %15s %15s\n" "CONTAINER" "CPU %" "MEMORY" "MEMORY %"
  printf "%-30s %10s %15s %15s\n" "---------" "-----" "------" "--------"

  # Get stats for each container
  for container in $containers; do
    local stats=$(docker stats --no-stream --format "{{.Container}} {{.CPUPerc}} {{.MemUsage}} {{.MemPerc}}" "$container" 2>/dev/null)
    if [[ -n "$stats" ]]; then
      local name=$(echo "$stats" | awk '{print $1}' | sed "s/${PROJECT_NAME:-nself}_//")
      local cpu=$(echo "$stats" | awk '{print $2}')
      local mem=$(echo "$stats" | awk '{print $3}')
      local mem_perc=$(echo "$stats" | awk '{print $4}')

      # Color code based on usage
      local cpu_val=$(echo "$cpu" | sed 's/%//')
      if (($(echo "$cpu_val > 80" | bc -l))); then
        cpu=$(color_text "$cpu" "red")
      elif (($(echo "$cpu_val > 50" | bc -l))); then
        cpu=$(color_text "$cpu" "yellow")
      else
        cpu=$(color_text "$cpu" "green")
      fi

      printf "%-30s %10s %15s %15s\n" "$name" "$cpu" "$mem" "$mem_perc"
    fi
  done

  echo ""
  echo "Total containers: $(echo "$containers" | wc -l | tr -d ' ')"
}

# Tail service logs
tail_service_logs() {
  local service="${1:-}"

  if [[ -z "$service" ]]; then
    echo "Available services:"
    docker ps --format "{{.Names}}" | grep "^${PROJECT_NAME:-nself}_" | sed "s/${PROJECT_NAME:-nself}_/  • /" | sort
    echo ""
    echo "Usage: nself monitor logs <service>"
    echo "Example: nself monitor logs nginx"
    return 1
  fi

  local container_name="${PROJECT_NAME:-nself}_${service}"

  if ! docker ps --format "{{.Names}}" | grep -q "^${container_name}$"; then
    log_error "Service '$service' is not running"
    return 1
  fi

  log_info "Tailing logs for $service (Ctrl+C to stop)..."
  echo ""
  docker logs -f "$container_name" --tail 50
}

# Color text helper (cross-platform compatible)
color_text() {
  local text="$1"
  local color="$2"

  case "$color" in
    red)
      printf "\033[0;31m%s\033[0m" "$text"
      ;;
    green)
      printf "\033[0;32m%s\033[0m" "$text"
      ;;
    yellow)
      printf "\033[0;33m%s\033[0m" "$text"
      ;;
    blue)
      printf "\033[0;34m%s\033[0m" "$text"
      ;;
    cyan)
      printf "\033[0;36m%s\033[0m" "$text"
      ;;
    gray)
      printf "\033[0;90m%s\033[0m" "$text"
      ;;
    *)
      printf "%s" "$text"
      ;;
  esac
}

# Run full monitoring dashboard (legacy - redirect to Grafana)
run_monitoring_dashboard() {
  clear
  tput civis 2>/dev/null || true

  # Trap to restore cursor on exit
  trap 'tput cnorm 2>/dev/null || true; clear' EXIT INT TERM

  local refresh_interval="${MONITOR_INTERVAL:-2}"
  local paused=false
  local view="dashboard"

  while true; do
    # Clear screen for refresh
    clear

    # Show header
    show_dashboard_header "$view" "$paused"

    # Show appropriate view
    case "$view" in
      dashboard)
        show_dashboard_view
        ;;
      services)
        show_services_view
        ;;
      resources)
        show_resources_view
        ;;
      logs)
        show_logs_view
        ;;
      alerts)
        show_alerts_view
        ;;
    esac

    # Show footer with controls
    show_dashboard_footer

    # Handle input with timeout
    if read -t "$refresh_interval" -n 1 key; then
      case "$key" in
        q | Q)
          break
          ;;
        r | R)
          continue
          ;;
        s)
          view="services"
          ;;
        c)
          view="resources"
          ;;
        l)
          view="logs"
          ;;
        a)
          view="alerts"
          ;;
        " ")
          paused=$([[ "$paused" == "true" ]] && echo "false" || echo "true")
          ;;
      esac
    fi

    # Skip refresh if paused
    if [[ "$paused" == "true" ]]; then
      sleep 0.1
      continue
    fi
  done
}

# Show dashboard header
show_dashboard_header() {
  local view="$1"
  local paused="$2"

  local status_indicator="\033[0;32m●\033[0m"
  if [[ "$paused" == "true" ]]; then
    status_indicator="\033[0;33m⏸\033[0m"
  fi

  printf "\033[0;36m╔══════════════════════════════════════════════════════════════════════════════╗\033[0m\n"
  printf "\033[0;36m║\033[0m  \033[1mnself monitor\033[0m - %s view  %b %s                    \033[0;36m║\033[0m\n" "$view" "$status_indicator" "$(date '+%Y-%m-%d %H:%M:%S')"
  printf "\033[0;36m╚══════════════════════════════════════════════════════════════════════════════╝\033[0m\n"
  echo
}

# Show dashboard view
show_dashboard_view() {
  # Services summary
  printf "\033[0;36m▶ Services\033[0m\n"
  local services=$(docker compose ps --format "table {{.Service}}\t{{.Status}}" 2>/dev/null | tail -n +2)
  local running=$(echo "$services" | grep -c "Up" || echo "0")
  local total=$(echo "$services" | wc -l | xargs)
  echo "  Status: $running/$total services running"

  # Quick service list
  echo "$services" | head -5 | while IFS=$'\t' read -r service status; do
    local indicator="\033[0;31m✗\033[0m"
    if [[ "$status" == *"Up"* ]]; then
      indicator="\033[0;32m✓\033[0m"
    fi
    printf "  %b %-20s %s\n" "$indicator" "$service" "$status"
  done

  echo

  # Resources summary
  printf "\033[0;36m▶ Resources\033[0m\n"
  local cpu_usage=$(docker stats --no-stream --format "{{.CPUPerc}}" 2>/dev/null | sed 's/%//' | awk '{sum+=$1} END {printf "%.1f", sum}')
  local mem_usage=$(docker stats --no-stream --format "{{.MemUsage}}" 2>/dev/null | head -1)
  echo "  CPU Total: ${cpu_usage}%"
  echo "  Memory: $mem_usage"

  echo

  # Top consumers
  printf "\033[0;36m▶ Top Consumers\033[0m\n"
  docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemPerc}}" 2>/dev/null | head -6 | tail -n +2 | while IFS=$'\t' read -r name cpu mem; do
    printf "  %-30s CPU: %6s  MEM: %6s\n" "$name" "$cpu" "$mem"
  done

  echo

  # Recent alerts
  printf "\033[0;36m▶ Recent Alerts\033[0m\n"
  local alert_log="${TMPDIR:-/tmp}/nself-alerts.log"
  if [[ -f "$alert_log" ]]; then
    tail -3 "$alert_log" | while read -r line; do
      echo "  $line"
    done
  else
    echo "  No recent alerts"
  fi
}

# Show services view
show_services_view() {
  printf "\033[0;36m▶ Service Health\033[0m\n"
  echo

  docker compose ps --format "table {{.Service}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null | while IFS=$'\t' read -r service status ports; do
    if [[ "$service" == "Service" ]]; then
      printf "  %-20s %-30s %s\n" "SERVICE" "STATUS" "PORTS"
      printf "  %-20s %-30s %s\n" "-------" "------" "-----"
    else
      local indicator="\033[0;31m✗\033[0m"
      local health=""
      if [[ "$status" == *"Up"* ]]; then
        indicator="\033[0;32m✓\033[0m"
        if [[ "$status" == *"healthy"* ]]; then
          health="\033[0;32m[healthy]\033[0m"
        elif [[ "$status" == *"unhealthy"* ]]; then
          health="\033[0;31m[unhealthy]\033[0m"
          indicator="\033[0;33m⚠\033[0m"
        fi
      fi
      printf "  %b %-18s %-30s %s\n" "$indicator" "$service" "$status $health" "$ports"
    fi
  done

  echo
  printf "\033[0;36m▶ Container Restart Count\033[0m\n"
  docker compose ps --format "{{.Service}}" 2>/dev/null | while read -r service; do
    local restarts=$(docker inspect "$(docker compose ps -q "$service" 2>/dev/null)" --format='{{.RestartCount}}' 2>/dev/null || echo "0")
    if [[ "$restarts" -gt 0 ]]; then
      printf "  %-20s %d restarts\n" "$service" "$restarts"
    fi
  done
}

# Show resources view
show_resources_view() {
  printf "\033[0;36m▶ Resource Usage\033[0m\n"
  echo

  docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}" 2>/dev/null

  echo
  printf "\033[0;36m▶ System Resources\033[0m\n"

  # CPU info
  if [[ "$(uname)" == "Darwin" ]]; then
    local cpu_usage=$(top -l 1 | grep "CPU usage" | awk '{print $3}' | sed 's/%//')
    echo "  System CPU: ${cpu_usage}%"
  else
    local cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | sed 's/%us,//')
    echo "  System CPU: ${cpu_usage}%"
  fi

  # Memory info
  if [[ "$(uname)" == "Darwin" ]]; then
    local mem_info=$(top -l 1 | grep "PhysMem")
    echo "  System Memory: $mem_info"
  else
    local mem_info=$(free -h | grep "^Mem" | awk '{printf "%s / %s (%s used)", $3, $2, $3}')
    echo "  System Memory: $mem_info"
  fi

  # Disk info
  local disk_info=$(df -h . | tail -1 | awk '{printf "%s / %s (%s used)", $3, $2, $5}')
  echo "  Disk Usage: $disk_info"
}

# Show logs view
show_logs_view() {
  printf "\033[0;36m▶ Recent Logs\033[0m\n"
  echo

  # Show last 20 lines from all services
  docker compose logs --tail=20 --timestamps 2>/dev/null | tail -20
}

# Show alerts view
show_alerts_view() {
  printf "\033[0;36m▶ Active Alerts\033[0m\n"
  echo

  # Check for unhealthy services
  local unhealthy=$(docker compose ps --format "{{.Service}}\t{{.Status}}" 2>/dev/null | grep -E "unhealthy|Exit|Restarting" || true)
  if [[ -n "$unhealthy" ]]; then
    printf "  \033[0;31m⚠ Unhealthy Services:\033[0m\n"
    echo "$unhealthy" | while IFS=$'\t' read -r service status; do
      echo "    - $service: $status"
    done
    echo
  fi

  # Check disk space
  local disk_usage=$(df -h . | tail -1 | awk '{print $5}' | sed 's/%//')
  if [[ "$disk_usage" -gt 80 ]]; then
    printf "  \033[0;33m⚠ High Disk Usage: %s%%\033[0m\n" "$disk_usage"
    echo
  fi

  # Check memory
  if [[ "$(uname)" != "Darwin" ]]; then
    local mem_usage=$(free | grep "^Mem" | awk '{printf "%.0f", $3/$2 * 100}')
    if [[ "$mem_usage" -gt 80 ]]; then
      printf "  \033[0;33m⚠ High Memory Usage: %s%%\033[0m\n" "$mem_usage"
      echo
    fi
  fi

  # Show alert log
  local alert_log="${TMPDIR:-/tmp}/nself-alerts.log"
  if [[ -f "$alert_log" ]]; then
    printf "  \033[0;36mRecent Alert History:\033[0m\n"
    tail -10 /tmp/nself-alerts.log | while read -r line; do
      echo "    $line"
    done
  else
    echo "  No alerts logged"
  fi
}

# Show dashboard footer
show_dashboard_footer() {
  echo
  printf "\033[0;36m────────────────────────────────────────────────────────────────────────────────\033[0m\n"
  echo "Controls: [q]uit | [r]efresh | [s]ervices | [c]pu/resources | [l]ogs | [a]lerts | [space] pause"
}

# Monitor services live (standalone)
monitor_services_live() {
  while true; do
    clear
    show_command_header "nself monitor services" "Live service monitoring"
    show_services_view
    sleep "${MONITOR_INTERVAL:-2}"
  done
}

# Monitor resources live (standalone)
monitor_resources_live() {
  while true; do
    clear
    show_command_header "nself monitor resources" "Live resource monitoring"
    show_resources_view
    sleep "${MONITOR_INTERVAL:-2}"
  done
}

# Monitor logs live (standalone)
monitor_logs_live() {
  show_command_header "nself monitor logs" "Live log streaming"
  docker compose logs -f --tail=50
}

# Monitor alerts live (standalone)
monitor_alerts_live() {
  while true; do
    clear
    show_command_header "nself monitor alerts" "Live alert monitoring"
    show_alerts_view
    sleep "${MONITOR_INTERVAL:-5}"
  done
}

# Export for use as library
export -f cmd_monitor

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_monitor "$@"
  exit $?
fi
