#!/usr/bin/env bash


# lb-health.sh - Load balancer health monitoring

# Health check configuration
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-10}"

set -euo pipefail

HEALTH_CHECK_TIMEOUT="${HEALTH_CHECK_TIMEOUT:-5}"
UNHEALTHY_THRESHOLD="${UNHEALTHY_THRESHOLD:-3}"
HEALTHY_THRESHOLD="${HEALTHY_THRESHOLD:-2}"

# Backend pools
BACKEND_POOLS_FILE="${BACKEND_POOLS_FILE:-/tmp/nself-backend-pools}"

# Initialize backend pool
init_backend_pool() {
  local pool_name="$1"
  local backends="$2" # comma-separated list

  echo "$pool_name:$backends:active" >>"$BACKEND_POOLS_FILE"
}

# Health check endpoint
health_check_endpoint() {
  local endpoint="$1"
  local timeout="${2:-$HEALTH_CHECK_TIMEOUT}"

  # Try HTTP health check
  local response_code=$(curl -s -o /dev/null -w "%{http_code}" \
    --connect-timeout "$timeout" \
    --max-time "$timeout" \
    "$endpoint/health" 2>/dev/null || echo "000")

  if [[ "$response_code" == "200" ]]; then
    return 0
  else
    return 1
  fi
}

# Monitor backend health
monitor_backend_health() {
  local backend="$1"
  local pool="$2"
  local consecutive_failures=0
  local consecutive_successes=0
  local is_healthy=true

  while true; do
    if health_check_endpoint "$backend"; then
      # Health check passed
      consecutive_successes=$((consecutive_successes + 1))
      consecutive_failures=0

      if [[ "$is_healthy" == "false" ]] && [[ $consecutive_successes -ge $HEALTHY_THRESHOLD ]]; then
        # Mark as healthy
        mark_backend_healthy "$backend" "$pool"
        is_healthy=true
      fi
    else
      # Health check failed
      consecutive_failures=$((consecutive_failures + 1))
      consecutive_successes=0

      if [[ "$is_healthy" == "true" ]] && [[ $consecutive_failures -ge $UNHEALTHY_THRESHOLD ]]; then
        # Mark as unhealthy
        mark_backend_unhealthy "$backend" "$pool"
        is_healthy=false
      fi
    fi

    sleep "$HEALTH_CHECK_INTERVAL"
  done
}

# Mark backend as healthy
mark_backend_healthy() {
  local backend="$1"
  local pool="$2"

  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Backend $backend in pool $pool marked HEALTHY"

  # Update nginx upstream
  update_nginx_upstream "$pool" "$backend" "up"

  # Send notification
  send_health_notification "$backend" "$pool" "healthy"
}

# Mark backend as unhealthy
mark_backend_unhealthy() {
  local backend="$1"
  local pool="$2"

  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Backend $backend in pool $pool marked UNHEALTHY"

  # Update nginx upstream
  update_nginx_upstream "$pool" "$backend" "down"

  # Send notification
  send_health_notification "$backend" "$pool" "unhealthy"
}

# Update nginx upstream configuration
update_nginx_upstream() {
  local pool="$1"
  local backend="$2"
  local status="$3" # up or down

  # Generate nginx upstream config
  local config_file="/tmp/upstream_${pool}.conf"

  # Get all backends for pool
  local backends=$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' ' ')

  echo "upstream $pool {" >"$config_file"

  for b in $backends; do
    if [[ "$b" == "$backend" ]] && [[ "$status" == "down" ]]; then
      echo "    server $b down;" >>"$config_file"
    else
      echo "    server $b;" >>"$config_file"
    fi
  done

  echo "    keepalive 32;" >>"$config_file"
  echo "}" >>"$config_file"

  # Copy to nginx and reload
  docker cp "$config_file" nginx:/etc/nginx/conf.d/upstream_${pool}.conf 2>/dev/null || true
  docker exec nginx nginx -t 2>/dev/null && docker exec nginx nginx -s reload 2>/dev/null || true
}

# Send health notification
send_health_notification() {
  local backend="$1"
  local pool="$2"
  local status="$3"

  # Log to file
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Backend $backend in pool $pool is $status" >>/tmp/lb-health.log

  # Send webhook if configured
  if [[ -n "${LB_HEALTH_WEBHOOK:-}" ]]; then
    curl -X POST "$LB_HEALTH_WEBHOOK" \
      -H "Content-Type: application/json" \
      -d "{\"backend\":\"$backend\",\"pool\":\"$pool\",\"status\":\"$status\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" \
      2>/dev/null || true
  fi
}

# Get pool statistics
get_pool_statistics() {
  local pool="$1"

  # Count healthy/unhealthy backends
  local total=0
  local healthy=0
  local unhealthy=0

  local backends=$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' ' ')

  for backend in $backends; do
    total=$((total + 1))

    if health_check_endpoint "$backend"; then
      healthy=$((healthy + 1))
    else
      unhealthy=$((unhealthy + 1))
    fi
  done

  echo "{\"pool\":\"$pool\",\"total\":$total,\"healthy\":$healthy,\"unhealthy\":$unhealthy}"
}

# Implement connection draining
connection_drain() {
  local backend="$1"
  local drain_timeout="${2:-30}"

  echo "Starting connection drain for $backend (timeout: ${drain_timeout}s)"

  # Mark backend as draining in nginx
  update_nginx_upstream_drain "$backend"

  # Wait for connections to drain
  local elapsed=0
  while [[ $elapsed -lt $drain_timeout ]]; do
    local active_connections=$(get_active_connections "$backend")

    if [[ $active_connections -eq 0 ]]; then
      echo "Connection drain complete for $backend"
      return 0
    fi

    echo "Waiting for $active_connections connections to drain..."
    sleep 5
    elapsed=$((elapsed + 5))
  done

  echo "Connection drain timeout reached for $backend"
}

# Get active connections for backend
get_active_connections() {
  local backend="$1"

  # Parse nginx status
  local connections=$(docker exec nginx sh -c "netstat -an | grep '$backend' | grep ESTABLISHED | wc -l" 2>/dev/null || echo "0")

  echo "$connections"
}

# Implement sticky sessions
configure_sticky_sessions() {
  local pool="$1"
  local method="${2:-ip_hash}" # ip_hash, cookie, or consistent_hash

  local config_file="/tmp/sticky_${pool}.conf"

  case "$method" in
    ip_hash)
      cat >"$config_file" <<EOF
upstream $pool {
    ip_hash;
$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' '\n' | sed 's/^/    server /')
}
EOF
      ;;
    cookie)
      cat >"$config_file" <<EOF
upstream $pool {
    sticky cookie srv_id expires=1h path=/;
$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' '\n' | sed 's/^/    server /')
}
EOF
      ;;
    consistent_hash)
      cat >"$config_file" <<EOF
upstream $pool {
    hash \$request_uri consistent;
$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' '\n' | sed 's/^/    server /')
}
EOF
      ;;
  esac

  # Apply configuration
  docker cp "$config_file" nginx:/etc/nginx/conf.d/sticky_${pool}.conf 2>/dev/null || true
  docker exec nginx nginx -s reload 2>/dev/null || true
}

# Implement health check dashboard
display_health_dashboard() {
  clear

  echo "════════════════════════════════════════════════════════════════════════════════"
  echo "                         Load Balancer Health Dashboard                          "
  echo "════════════════════════════════════════════════════════════════════════════════"
  echo ""

  # Get all pools
  local pools=$(cut -d':' -f1 "$BACKEND_POOLS_FILE" | sort -u)

  for pool in $pools; do
    echo "Pool: $pool"
    echo "────────────────────────────────────────────────────────────────────────────────"

    local stats=$(get_pool_statistics "$pool")
    local total=$(echo "$stats" | jq '.total')
    local healthy=$(echo "$stats" | jq '.healthy')
    local unhealthy=$(echo "$stats" | jq '.unhealthy')

    # Display health bar
    local health_percent=0
    if [[ $total -gt 0 ]]; then
      health_percent=$((healthy * 100 / total))
    fi

    echo -n "Health: "
    for ((i = 0; i < 20; i++)); do
      local threshold=$((i * 5))
      if [[ $health_percent -gt $threshold ]]; then
        echo -n "█"
      else
        echo -n "░"
      fi
    done
    printf " %3d%% (%d/%d healthy)\n" "$health_percent" "$healthy" "$total"

    # List backends
    local backends=$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' ' ')

    for backend in $backends; do
      echo -n "  • $backend: "

      if health_check_endpoint "$backend"; then
        printf "\033[32m● Healthy\033[0m\n"
      else
        printf "\033[31m○ Unhealthy\033[0m\n"
      fi
    done

    echo ""
  done

  echo "════════════════════════════════════════════════════════════════════════════════"
  echo "Updated: $(date '+%Y-%m-%d %H:%M:%S')"
}

# Run health monitoring
run_health_monitoring() {
  # Initialize pools
  touch "$BACKEND_POOLS_FILE"

  # Start monitoring each backend
  local pools=$(cut -d':' -f1 "$BACKEND_POOLS_FILE" | sort -u)

  for pool in $pools; do
    local backends=$(grep "^$pool:" "$BACKEND_POOLS_FILE" | cut -d':' -f2 | tr ',' ' ')

    for backend in $backends; do
      # Start background monitor for each backend
      monitor_backend_health "$backend" "$pool" &
    done
  done

  # Wait for monitors
  wait
}

# Export functions
export -f init_backend_pool
export -f health_check_endpoint
export -f monitor_backend_health
export -f get_pool_statistics
export -f connection_drain
export -f configure_sticky_sessions
export -f display_health_dashboard
export -f run_health_monitoring
