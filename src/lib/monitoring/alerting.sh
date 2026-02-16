#!/usr/bin/env bash


# alerting.sh - Automated alerting system for nself

# Source platform compatibility utilities
ALERTING_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$ALERTING_DIR/../utils/platform-compat.sh" 2>/dev/null || true

# Alert configuration
ALERT_CONFIG_FILE="${ALERT_CONFIG_FILE:-./alerts.conf}"
ALERT_LOG="${ALERT_LOG:-./logs/alerts.log}"
ALERT_STATE_FILE="${ALERT_STATE_FILE:-/tmp/nself-alert-state}"

# Alert thresholds
CPU_ALERT_THRESHOLD="${CPU_ALERT_THRESHOLD:-90}"
MEMORY_ALERT_THRESHOLD="${MEMORY_ALERT_THRESHOLD:-90}"
DISK_ALERT_THRESHOLD="${DISK_ALERT_THRESHOLD:-90}"
SERVICE_DOWN_THRESHOLD="${SERVICE_DOWN_THRESHOLD:-60}" # seconds

# Alert channels
ALERT_EMAIL="${ALERT_EMAIL:-}"
ALERT_SLACK_WEBHOOK="${ALERT_SLACK_WEBHOOK:-}"
ALERT_DISCORD_WEBHOOK="${ALERT_DISCORD_WEBHOOK:-}"
ALERT_PAGERDUTY_KEY="${ALERT_PAGERDUTY_KEY:-}"

# Initialize alerting
init_alerting() {
  mkdir -p "$(dirname "$ALERT_LOG")"
  touch "$ALERT_LOG"

  # Initialize state file
  if [[ ! -f "$ALERT_STATE_FILE" ]]; then
    echo "{}" >"$ALERT_STATE_FILE"
  fi
}

# Log alert
log_alert() {
  local severity="$1"
  local message="$2"
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [$severity] $message" >>"$ALERT_LOG"
}

# Check if alert should be sent (rate limiting)
should_send_alert() {
  local alert_key="$1"
  local cooldown="${2:-300}" # 5 minutes default

  # Get last alert time
  local last_alert=$(grep "\"$alert_key\":" "$ALERT_STATE_FILE" 2>/dev/null | cut -d':' -f2 | tr -d ' ",')

  if [[ -z "$last_alert" ]]; then
    # Never sent this alert
    return 0
  fi

  local now=$(date +%s)
  local elapsed=$((now - last_alert))

  if [[ $elapsed -gt $cooldown ]]; then
    return 0
  else
    return 1
  fi
}

# Update alert state
update_alert_state() {
  local alert_key="$1"
  local timestamp=$(date +%s)

  # Update state file (simple JSON update)
  local current_state=$(cat "$ALERT_STATE_FILE")
  echo "$current_state" | sed "s/\"$alert_key\":[0-9]*/\"$alert_key\":$timestamp/" >"$ALERT_STATE_FILE.tmp"

  # Add if not exists
  if ! grep -q "\"$alert_key\":" "$ALERT_STATE_FILE.tmp"; then
    echo "$current_state" | sed "s/}$/,\"$alert_key\":$timestamp}/" >"$ALERT_STATE_FILE.tmp"
    if ! grep -q "{" "$ALERT_STATE_FILE.tmp"; then
      echo "{\"$alert_key\":$timestamp}" >"$ALERT_STATE_FILE.tmp"
    fi
  fi

  mv "$ALERT_STATE_FILE.tmp" "$ALERT_STATE_FILE"
}

# Send email alert
send_email_alert() {
  local subject="$1"
  local body="$2"

  if [[ -z "$ALERT_EMAIL" ]]; then
    return 1
  fi

  echo "$body" | mail -s "nself Alert: $subject" "$ALERT_EMAIL" 2>/dev/null || {
    log_alert "ERROR" "Failed to send email alert"
    return 1
  }

  log_alert "INFO" "Email alert sent: $subject"
}

# Send Slack alert
send_slack_alert() {
  local message="$1"
  local severity="${2:-warning}"

  if [[ -z "$ALERT_SLACK_WEBHOOK" ]]; then
    return 1
  fi

  local color=""
  case "$severity" in
    critical) color="danger" ;;
    warning) color="warning" ;;
    info) color="good" ;;
    *) color="#808080" ;;
  esac

  local payload=$(
    cat <<EOF
{
  "attachments": [{
    "color": "$color",
    "title": "nself Alert",
    "text": "$message",
    "footer": "nself monitoring",
    "ts": $(date +%s)
  }]
}
EOF
  )

  curl -X POST "$ALERT_SLACK_WEBHOOK" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    2>/dev/null || {
    log_alert "ERROR" "Failed to send Slack alert"
    return 1
  }

  log_alert "INFO" "Slack alert sent: $message"
}

# Send Discord alert
send_discord_alert() {
  local message="$1"
  local severity="${2:-warning}"

  if [[ -z "$ALERT_DISCORD_WEBHOOK" ]]; then
    return 1
  fi

  local color=""
  case "$severity" in
    critical) color="15158332" ;; # Red
    warning) color="16776960" ;;  # Yellow
    info) color="3066993" ;;      # Green
    *) color="8421504" ;;         # Gray
  esac

  local payload=$(
    cat <<EOF
{
  "embeds": [{
    "title": "nself Alert",
    "description": "$message",
    "color": $color,
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  }]
}
EOF
  )

  curl -X POST "$ALERT_DISCORD_WEBHOOK" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    2>/dev/null || {
    log_alert "ERROR" "Failed to send Discord alert"
    return 1
  }

  log_alert "INFO" "Discord alert sent: $message"
}

# Send PagerDuty alert
send_pagerduty_alert() {
  local message="$1"
  local severity="${2:-warning}"
  local dedup_key="${3:-nself-alert}"

  if [[ -z "$ALERT_PAGERDUTY_KEY" ]]; then
    return 1
  fi

  local event_action="trigger"
  if [[ "$severity" == "resolved" ]]; then
    event_action="resolve"
  fi

  local payload=$(
    cat <<EOF
{
  "routing_key": "$ALERT_PAGERDUTY_KEY",
  "event_action": "$event_action",
  "dedup_key": "$dedup_key",
  "payload": {
    "summary": "$message",
    "severity": "$severity",
    "source": "nself",
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  }
}
EOF
  )

  curl -X POST "https://events.pagerduty.com/v2/enqueue" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    2>/dev/null || {
    log_alert "ERROR" "Failed to send PagerDuty alert"
    return 1
  }

  log_alert "INFO" "PagerDuty alert sent: $message"
}

# Send alert to all configured channels
send_alert() {
  local message="$1"
  local severity="${2:-warning}"
  local alert_key="${3:-general}"

  # Check rate limiting
  if ! should_send_alert "$alert_key"; then
    log_alert "INFO" "Alert suppressed due to rate limiting: $alert_key"
    return 0
  fi

  # Update state
  update_alert_state "$alert_key"

  # Send to all configured channels
  send_email_alert "$severity Alert" "$message"
  send_slack_alert "$message" "$severity"
  send_discord_alert "$message" "$severity"
  send_pagerduty_alert "$message" "$severity" "$alert_key"
}

# Monitor system resources
monitor_resources() {
  # CPU check
  local cpu_usage=""
  if [[ "$(uname)" == "Linux" ]]; then
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)
  elif [[ "$(uname)" == "Darwin" ]]; then
    cpu_usage=$(top -l 1 | grep "CPU usage" | awk '{print $3}' | cut -d'%' -f1)
  fi

  if [[ -n "$cpu_usage" ]]; then
    local cpu_int=${cpu_usage%.*}
    if [[ $cpu_int -ge $CPU_ALERT_THRESHOLD ]]; then
      send_alert "CPU usage critical: ${cpu_usage}%" "critical" "cpu_high"
    fi
  fi

  # Memory check
  if command -v free >/dev/null 2>&1; then
    local total_mem=$(free -m | grep "^Mem:" | awk '{print $2}')
    local used_mem=$(free -m | grep "^Mem:" | awk '{print $3}')
    local mem_percent=$((used_mem * 100 / total_mem))

    if [[ $mem_percent -ge $MEMORY_ALERT_THRESHOLD ]]; then
      send_alert "Memory usage critical: ${mem_percent}%" "critical" "memory_high"
    fi
  fi

  # Disk check
  local disk_usage=$(df -h . | tail -1 | awk '{print $5}' | cut -d'%' -f1)
  if [[ $disk_usage -ge $DISK_ALERT_THRESHOLD ]]; then
    send_alert "Disk usage critical: ${disk_usage}%" "critical" "disk_high"
  fi
}

# Monitor service health
monitor_services() {
  local services=$(docker ps --format "{{.Names}}" 2>/dev/null)

  for service in $services; do
    # Check health status
    local health=$(docker inspect "$service" --format='{{.State.Health.Status}}' 2>/dev/null || echo "none")

    if [[ "$health" == "unhealthy" ]]; then
      send_alert "Service unhealthy: $service" "critical" "service_unhealthy_$service"
    fi

    # Check restart count
    local restarts=$(docker inspect "$service" --format='{{.RestartCount}}' 2>/dev/null || echo "0")
    if [[ $restarts -gt 5 ]]; then
      send_alert "Service restarting frequently: $service (${restarts} restarts)" "warning" "service_restarts_$service"
    fi
  done

  # Check for stopped critical services
  local critical_services="postgres redis nginx hasura"
  for service in $critical_services; do
    if ! docker ps --format "{{.Names}}" | grep -q "$service"; then
      send_alert "Critical service not running: $service" "critical" "service_down_$service"
    fi
  done
}

# Monitor backups
monitor_backups() {
  # Check last backup time (cross-platform: try BSD stat, fallback to GNU stat)
  local last_backup
  if [[ "$OSTYPE" == "darwin"* ]]; then
    last_backup=$(find ./backups -name "*.tar.gz" -type f -exec stat -f %m {} \; 2>/dev/null | sort -n | tail -1)
  else
    last_backup=$(find ./backups -name "*.tar.gz" -type f -exec stat -c %Y {} \; 2>/dev/null | sort -n | tail -1)
  fi

  if [[ -n "$last_backup" ]]; then
    local now=$(date +%s)
    local age=$((now - last_backup))
    local days=$((age / 86400))

    if [[ $days -gt 7 ]]; then
      send_alert "No backup in $days days" "warning" "backup_old"
    fi
  else
    send_alert "No backups found" "critical" "backup_missing"
  fi
}

# Run monitoring loop
run_monitoring() {
  init_alerting

  log_alert "INFO" "Monitoring started"

  while true; do
    monitor_resources
    monitor_services
    monitor_backups

    # Wait before next check
    sleep "${MONITOR_INTERVAL:-60}"
  done
}

# Export functions
export -f init_alerting
export -f send_alert
export -f monitor_resources
export -f monitor_services
export -f run_monitoring
