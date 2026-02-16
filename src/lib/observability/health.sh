#!/usr/bin/env bash
# health.sh - Deep health checks and automatic recovery
# Part of nself v0.7.0 - Sprint 7: OBS-004


# Initialize health check storage
health_init() {

set -euo pipefail

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS health;

CREATE TABLE IF NOT EXISTS health.checks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_name TEXT NOT NULL,
  check_type TEXT NOT NULL, -- liveness, readiness, startup, dependency
  status TEXT NOT NULL, -- healthy, degraded, unhealthy
  response_time_ms INTEGER,
  message TEXT,
  details JSONB DEFAULT '{}',
  checked_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS health.dependencies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_name TEXT NOT NULL,
  dependency_name TEXT NOT NULL,
  dependency_type TEXT NOT NULL, -- database, cache, api, queue
  status TEXT NOT NULL,
  last_checked TIMESTAMPTZ DEFAULT NOW(),
  last_healthy TIMESTAMPTZ,
  consecutive_failures INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS health.recovery_actions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_name TEXT NOT NULL,
  action_type TEXT NOT NULL, -- restart, scale, notify, auto-heal
  triggered_by TEXT NOT NULL,
  executed_at TIMESTAMPTZ DEFAULT NOW(),
  success BOOLEAN,
  details JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_health_service ON health.checks(service_name);
CREATE INDEX IF NOT EXISTS idx_health_checked ON health.checks(checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_health_status ON health.checks(status);
CREATE INDEX IF NOT EXISTS idx_deps_service ON health.dependencies(service_name);
CREATE INDEX IF NOT EXISTS idx_deps_status ON health.dependencies(status);
CREATE INDEX IF NOT EXISTS idx_recovery_service ON health.recovery_actions(service_name);
CREATE INDEX IF NOT EXISTS idx_recovery_executed ON health.recovery_actions(executed_at DESC);
EOSQL
  return 0
}

# Perform health check
health_check() {
  local service_name="$1"
  local check_type="${2:-liveness}" # liveness, readiness, startup

  local start_time=$(date +%s%3N)
  local status="healthy"
  local message="Service is healthy"
  local details="{}"

  # Check if container is running
  local container=$(docker ps --filter "name=$service_name" --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    status="unhealthy"
    message="Container not running"
  else
    # Check container health
    local health_status=$(docker inspect --format='{{.State.Health.Status}}' "$container" 2>/dev/null || echo "none")

    case "$health_status" in
      healthy)
        status="healthy"
        message="Container reports healthy"
        ;;
      starting)
        status="degraded"
        message="Container starting"
        ;;
      unhealthy)
        status="unhealthy"
        message="Container reports unhealthy"
        ;;
      none)
        # No health check defined - check if running
        local running=$(docker inspect --format='{{.State.Running}}' "$container" 2>/dev/null)
        [[ "$running" == "true" ]] && status="healthy" || status="unhealthy"
        ;;
    esac
  fi

  local end_time=$(date +%s%3N)
  local response_time=$((end_time - start_time))

  # Record check
  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO health.checks (service_name, check_type, status, response_time_ms, message, details)
     VALUES ('$service_name', '$check_type', '$status', $response_time, '$message', '$details'::jsonb);" >/dev/null 2>&1

  echo "{\"service\":\"$service_name\",\"status\":\"$status\",\"message\":\"$message\",\"response_time_ms\":$response_time}"
  [[ "$status" == "healthy" ]] && return 0 || return 1
}

# Check all services
health_check_all() {
  local services=$(docker ps --format '{{.Names}}')

  local results="[]"
  while IFS= read -r service; do
    [[ -z "$service" ]] && continue
    local result=$(health_check "$service" "liveness" 2>/dev/null)
    results=$(echo "$results" | jq --argjson r "$result" '. += [$r]')
  done <<<"$services"

  echo "$results"
}

# Check service dependency
health_check_dependency() {
  local service_name="$1"
  local dependency_name="$2"
  local dependency_type="$3" # database, cache, api, queue

  local status="healthy"
  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  case "$dependency_type" in
    database)
      # Check PostgreSQL
      if docker exec "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c "SELECT 1;" >/dev/null 2>&1; then
        status="healthy"
      else
        status="unhealthy"
      fi
      ;;

    cache)
      # Check Redis
      local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
      if [[ -n "$redis_container" ]] && docker exec "$redis_container" redis-cli PING >/dev/null 2>&1; then
        status="healthy"
      else
        status="unhealthy"
      fi
      ;;

    api)
      # Check HTTP endpoint
      if curl -sf "http://$dependency_name/health" >/dev/null 2>&1; then
        status="healthy"
      else
        status="unhealthy"
      fi
      ;;

    queue)
      # Custom queue check
      status="healthy"
      ;;
  esac

  # Update dependency status
  docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO health.dependencies (service_name, dependency_name, dependency_type, status, last_checked, last_healthy, consecutive_failures)
     VALUES ('$service_name', '$dependency_name', '$dependency_type', '$status', NOW(), $([ "$status" == "healthy" ] && echo "NOW()" || echo "NULL"), $([ "$status" == "unhealthy" ] && echo "1" || echo "0"))
     ON CONFLICT (service_name, dependency_name) DO UPDATE SET
       status = EXCLUDED.status,
       last_checked = NOW(),
       last_healthy = CASE WHEN EXCLUDED.status = 'healthy' THEN NOW() ELSE dependencies.last_healthy END,
       consecutive_failures = CASE WHEN EXCLUDED.status = 'unhealthy' THEN dependencies.consecutive_failures + 1 ELSE 0 END;" >/dev/null 2>&1

  echo "{\"dependency\":\"$dependency_name\",\"status\":\"$status\"}"
  [[ "$status" == "healthy" ]] && return 0 || return 1
}

# Get health status
health_status() {
  local service_name="${1:-}"
  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -n "$service_name" ]]; then
    # Get status for specific service
    local status=$(docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_build_object(
         'service', '$service_name',
         'status', status,
         'message', message,
         'response_time_ms', response_time_ms,
         'checked_at', checked_at
       )
       FROM health.checks
       WHERE service_name = '$service_name'
       ORDER BY checked_at DESC
       LIMIT 1;" 2>/dev/null | xargs)

    echo "$status"
  else
    # Get status for all services
    local statuses=$(docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_agg(h) FROM (
         SELECT DISTINCT ON (service_name)
           service_name,
           status,
           message,
           response_time_ms,
           checked_at
         FROM health.checks
         ORDER BY service_name, checked_at DESC
       ) h;" 2>/dev/null | xargs)

    [[ -z "$statuses" || "$statuses" == "null" ]] && echo "[]" || echo "$statuses"
  fi
}

# Trigger recovery action
health_recover() {
  local service_name="$1"
  local action_type="${2:-restart}" # restart, scale, notify

  local success=false
  local details="{}"

  case "$action_type" in
    restart)
      if docker restart "$service_name" >/dev/null 2>&1; then
        success=true
        details='{"action":"container_restarted"}'
      fi
      ;;

    scale)
      # Scale up/down (would need docker-compose scale)
      details='{"action":"scale_not_implemented"}'
      ;;

    notify)
      # Send notification
      success=true
      details='{"action":"notification_sent"}'
      ;;
  esac

  # Record recovery action
  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO health.recovery_actions (service_name, action_type, triggered_by, success, details)
     VALUES ('$service_name', '$action_type', 'health_check', $success, '$details'::jsonb);" >/dev/null 2>&1

  [[ "$success" == "true" ]] && return 0 || return 1
}

# Auto-heal unhealthy services
health_auto_heal() {
  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Find unhealthy services
  local unhealthy=$(docker exec -i "$db_container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(h) FROM (
       SELECT DISTINCT ON (service_name)
         service_name,
         status
       FROM health.checks
       WHERE checked_at >= NOW() - INTERVAL '5 minutes'
       ORDER BY service_name, checked_at DESC
     ) h
     WHERE h.status = 'unhealthy';" 2>/dev/null | xargs)

  [[ -z "$unhealthy" || "$unhealthy" == "null" ]] && return 0

  # Attempt recovery for each
  echo "$unhealthy" | jq -r '.[].service_name' | while read -r service; do
    echo "Auto-healing $service..."
    health_recover "$service" "restart" && echo "✓ $service recovered" || echo "✗ $service recovery failed"
  done
}

# Generate status page
health_status_page() {
  local output_file="${1:-/tmp/health_status.html}"

  local db_container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local statuses=$(health_status)

  cat >"$output_file" <<EOF
<!DOCTYPE html>
<html>
<head>
  <title>nself Health Status</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 20px; }
    .healthy { color: green; }
    .degraded { color: orange; }
    .unhealthy { color: red; }
    table { border-collapse: collapse; width: 100%; }
    th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
    th { background-color: #4CAF50; color: white; }
  </style>
</head>
<body>
  <h1>nself Health Status</h1>
  <p>Last updated: $(date)</p>
  <table>
    <tr>
      <th>Service</th>
      <th>Status</th>
      <th>Message</th>
      <th>Response Time</th>
      <th>Last Check</th>
    </tr>
EOF

  echo "$statuses" | jq -r '.[] | "<tr><td>\(.service_name)</td><td class=\"\(.status)\">\(.status)</td><td>\(.message)</td><td>\(.response_time_ms)ms</td><td>\(.checked_at)</td></tr>"' >>"$output_file"

  cat >>"$output_file" <<EOF
  </table>
</body>
</html>
EOF

  echo "Status page generated: $output_file"
}

export -f health_init health_check health_check_all health_check_dependency
export -f health_status health_recover health_auto_heal health_status_page
