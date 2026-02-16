#!/usr/bin/env bash


# graceful-degradation.sh - Graceful degradation strategies for service failures

# Degradation configuration
DEGRADATION_CONFIG="${DEGRADATION_CONFIG:-./degradation.conf}"

set -euo pipefail

DEGRADATION_STATE="${DEGRADATION_STATE:-/tmp/nself-degradation-state}"

# Service priority levels
PRIORITY_CRITICAL=1
PRIORITY_HIGH=2
PRIORITY_MEDIUM=3
PRIORITY_LOW=4

# Initialize degradation system
init_degradation() {
  # Create state file
  echo "{\"mode\": \"normal\", \"disabled_features\": []}" >"$DEGRADATION_STATE"
}

# Get service priority
get_service_priority() {
  local service="$1"

  case "$service" in
    postgres | postgresql)
      echo $PRIORITY_CRITICAL
      ;;
    nginx | hasura | auth)
      echo $PRIORITY_HIGH
      ;;
    redis | cache)
      echo $PRIORITY_MEDIUM
      ;;
    *)
      echo $PRIORITY_LOW
      ;;
  esac
}

# Check system load
check_system_load() {
  local cpu_threshold=80
  local mem_threshold=85

  # Get CPU usage
  local cpu_usage=""
  if [[ "$(uname)" == "Linux" ]]; then
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)
  elif [[ "$(uname)" == "Darwin" ]]; then
    cpu_usage=$(top -l 1 | grep "CPU usage" | awk '{print $3}' | cut -d'%' -f1)
  fi

  # Get memory usage
  local mem_usage=0
  if command -v free >/dev/null 2>&1; then
    local total_mem=$(free -m | grep "^Mem:" | awk '{print $2}')
    local used_mem=$(free -m | grep "^Mem:" | awk '{print $3}')
    mem_usage=$((used_mem * 100 / total_mem))
  fi

  # Determine if degradation needed
  if [[ ${cpu_usage%.*} -gt $cpu_threshold ]] || [[ $mem_usage -gt $mem_threshold ]]; then
    return 0 # Degradation needed
  else
    return 1 # System OK
  fi
}

# Enable degraded mode
enable_degraded_mode() {
  local level="${1:-medium}"

  echo "Enabling degraded mode: $level"

  case "$level" in
    minimal)
      # Minimal functionality - critical services only
      disable_feature "dashboard"
      disable_feature "monitoring"
      disable_feature "backups"
      reduce_connection_pools
      disable_caching
      ;;
    light)
      # Light degradation
      disable_feature "dashboard"
      disable_feature "monitoring"
      reduce_connection_pools
      ;;
    medium)
      # Medium degradation
      disable_feature "monitoring"
      reduce_worker_processes
      enable_read_only_mode "non-critical"
      ;;
    heavy)
      # Heavy degradation
      enable_read_only_mode "all"
      disable_all_writes
      serve_cached_content_only
      ;;
  esac

  # Update state
  echo "{\"mode\": \"degraded\", \"level\": \"$level\", \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" >"$DEGRADATION_STATE"
}

# Disable feature
disable_feature() {
  local feature="$1"

  echo "Disabling feature: $feature"

  case "$feature" in
    dashboard)
      docker stop dashboard 2>/dev/null || true
      ;;
    monitoring)
      docker stop prometheus grafana 2>/dev/null || true
      ;;
    backups)
      # Disable scheduled backups
      crontab -l | grep -v "nself backup" | crontab - 2>/dev/null || true
      ;;
    caching)
      # Disable Redis caching
      docker stop redis 2>/dev/null || true
      ;;
  esac

  # Log disabled feature
  local current_state=$(cat "$DEGRADATION_STATE")
  echo "$current_state" | jq ".disabled_features += [\"$feature\"]" >"$DEGRADATION_STATE"
}

# Reduce connection pools
reduce_connection_pools() {
  echo "Reducing database connection pools"

  # Update PostgreSQL max connections
  docker exec postgres psql -U postgres -c "ALTER SYSTEM SET max_connections = 50;" 2>/dev/null || true
  docker exec postgres psql -U postgres -c "SELECT pg_reload_conf();" 2>/dev/null || true

  # Update Hasura connection pool
  docker exec hasura sh -c 'export HASURA_GRAPHQL_PG_CONNECTIONS=10' 2>/dev/null || true
}

# Reduce worker processes
reduce_worker_processes() {
  echo "Reducing worker processes"

  # Reduce nginx workers
  docker exec nginx sh -c "sed -i 's/worker_processes.*/worker_processes 1;/' /etc/nginx/nginx.conf" 2>/dev/null || true
  docker exec nginx nginx -s reload 2>/dev/null || true
}

# Enable read-only mode
enable_read_only_mode() {
  local scope="${1:-all}"

  echo "Enabling read-only mode for: $scope"

  if [[ "$scope" == "all" ]]; then
    # Make database read-only
    docker exec postgres psql -U postgres -c "ALTER DATABASE postgres SET default_transaction_read_only = on;" 2>/dev/null || true
  fi

  # Update application config
  echo "READ_ONLY_MODE=true" >>.env.local
}

# Circuit breaker pattern
circuit_breaker() {
  local service="$1"
  local failure_threshold=5
  local timeout=60

  # Check failure count
  local failures=$(grep "\"$service\":" "$DEGRADATION_STATE" 2>/dev/null | grep -o "failures\":[0-9]*" | cut -d':' -f2)

  if [[ -z "$failures" ]]; then
    failures=0
  fi

  if [[ $failures -ge $failure_threshold ]]; then
    echo "Circuit breaker OPEN for $service"

    # Stop routing traffic to service
    docker exec nginx sh -c "echo 'return 503;' > /etc/nginx/conf.d/${service}-circuit-breaker.conf" 2>/dev/null || true
    docker exec nginx nginx -s reload 2>/dev/null || true

    # Schedule circuit breaker reset
    (
      sleep $timeout
      reset_circuit_breaker "$service"
    ) &

    return 0
  else
    return 1
  fi
}

# Reset circuit breaker
reset_circuit_breaker() {
  local service="$1"

  echo "Resetting circuit breaker for $service"

  # Remove circuit breaker config
  docker exec nginx sh -c "rm -f /etc/nginx/conf.d/${service}-circuit-breaker.conf" 2>/dev/null || true
  docker exec nginx nginx -s reload 2>/dev/null || true

  # Reset failure count
  local current_state=$(cat "$DEGRADATION_STATE")
  echo "$current_state" | jq "del(.\"$service\")" >"$DEGRADATION_STATE"
}

# Implement bulkhead pattern
bulkhead_isolation() {
  local service="$1"
  local max_resources="${2:-50}" # Percentage

  echo "Implementing bulkhead isolation for $service (max $max_resources% resources)"

  # Limit CPU for service
  docker update --cpus="0.$max_resources" "$service" 2>/dev/null || true

  # Limit memory
  local total_mem=$(free -m | grep "^Mem:" | awk '{print $2}')
  local service_mem=$((total_mem * max_resources / 100))
  docker update --memory="${service_mem}m" "$service" 2>/dev/null || true
}

# Rate limiting
apply_rate_limiting() {
  local requests_per_second="${1:-10}"

  echo "Applying rate limiting: $requests_per_second requests/second"

  # Configure nginx rate limiting
  cat >/tmp/rate_limit.conf <<EOF
limit_req_zone \$binary_remote_addr zone=global:10m rate=${requests_per_second}r/s;
limit_req zone=global burst=20 nodelay;
limit_req_status 429;
EOF

  docker cp /tmp/rate_limit.conf nginx:/etc/nginx/conf.d/
  docker exec nginx nginx -s reload 2>/dev/null || true
}

# Fallback to static content
serve_static_fallback() {
  echo "Serving static fallback content"

  # Create maintenance page
  cat >/tmp/maintenance.html <<EOF
<!DOCTYPE html>
<html>
<head>
  <title>Service Temporarily Unavailable</title>
  <style>
    body { font-family: Arial; text-align: center; padding: 50px; }
    h1 { color: #333; }
  </style>
</head>
<body>
  <h1>Service Temporarily Unavailable</h1>
  <p>We're experiencing high load. Please try again in a few moments.</p>
</body>
</html>
EOF

  docker cp /tmp/maintenance.html nginx:/usr/share/nginx/html/

  # Configure nginx to serve maintenance page
  cat >/tmp/maintenance.conf <<EOF
location / {
  try_files /maintenance.html @backend;
}
location @backend {
  proxy_pass http://upstream;
  proxy_connect_timeout 2s;
  proxy_read_timeout 2s;
  error_page 502 503 504 /maintenance.html;
}
EOF

  docker cp /tmp/maintenance.conf nginx:/etc/nginx/conf.d/
  docker exec nginx nginx -s reload 2>/dev/null || true
}

# Auto-scale based on load
auto_scale() {
  local service="$1"
  local current_replicas=$(docker service ls --filter "name=$service" --format "{{.Replicas}}" 2>/dev/null | cut -d'/' -f1)

  if [[ -z "$current_replicas" ]]; then
    current_replicas=1
  fi

  # Check if scaling needed
  if check_system_load; then
    # Scale down
    local new_replicas=$((current_replicas - 1))
    if [[ $new_replicas -lt 1 ]]; then
      new_replicas=1
    fi

    echo "Scaling down $service to $new_replicas replicas"
    docker service scale "$service=$new_replicas" 2>/dev/null || true
  else
    # Scale up if resources available
    local new_replicas=$((current_replicas + 1))
    local max_replicas=5

    if [[ $new_replicas -le $max_replicas ]]; then
      echo "Scaling up $service to $new_replicas replicas"
      docker service scale "$service=$new_replicas" 2>/dev/null || true
    fi
  fi
}

# Restore normal mode
restore_normal_mode() {
  echo "Restoring normal mode"

  # Re-enable features
  local disabled_features=$(cat "$DEGRADATION_STATE" | jq -r '.disabled_features[]' 2>/dev/null)

  for feature in $disabled_features; do
    case "$feature" in
      dashboard)
        docker start dashboard 2>/dev/null || true
        ;;
      monitoring)
        docker start prometheus grafana 2>/dev/null || true
        ;;
      caching)
        docker start redis 2>/dev/null || true
        ;;
    esac
  done

  # Restore connection pools
  docker exec postgres psql -U postgres -c "ALTER SYSTEM SET max_connections = 200;" 2>/dev/null || true
  docker exec postgres psql -U postgres -c "SELECT pg_reload_conf();" 2>/dev/null || true

  # Remove rate limiting
  docker exec nginx sh -c "rm -f /etc/nginx/conf.d/rate_limit.conf" 2>/dev/null || true
  docker exec nginx nginx -s reload 2>/dev/null || true

  # Update state
  echo "{\"mode\": \"normal\", \"disabled_features\": []}" >"$DEGRADATION_STATE"
}

# Monitor and apply degradation
monitor_degradation() {
  while true; do
    if check_system_load; then
      # System under load
      local current_mode=$(cat "$DEGRADATION_STATE" | jq -r '.mode' 2>/dev/null)

      if [[ "$current_mode" != "degraded" ]]; then
        enable_degraded_mode "medium"
      fi
    else
      # System OK
      local current_mode=$(cat "$DEGRADATION_STATE" | jq -r '.mode' 2>/dev/null)

      if [[ "$current_mode" == "degraded" ]]; then
        restore_normal_mode
      fi
    fi

    sleep 30
  done
}

# Export functions
export -f init_degradation
export -f enable_degraded_mode
export -f circuit_breaker
export -f bulkhead_isolation
export -f apply_rate_limiting
export -f auto_scale
export -f restore_normal_mode
export -f monitor_degradation
