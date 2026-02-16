#!/usr/bin/env bash
# metrics.sh - Enhanced metrics collection
# Part of nself v0.7.0 - Sprint 7: OBS-001


# Initialize metrics storage
metrics_init() {

set -euo pipefail

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS metrics;

CREATE TABLE IF NOT EXISTS metrics.custom_metrics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  metric_type TEXT NOT NULL, -- counter, gauge, histogram, summary
  value NUMERIC NOT NULL,
  labels JSONB DEFAULT '{}',
  timestamp TIMESTAMPTZ DEFAULT NOW(),
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metrics.business_metrics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  metric_name TEXT NOT NULL,
  metric_value NUMERIC NOT NULL,
  dimensions JSONB DEFAULT '{}',
  recorded_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metrics.performance_metrics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  endpoint TEXT NOT NULL,
  method TEXT NOT NULL,
  response_time_ms INTEGER NOT NULL,
  status_code INTEGER NOT NULL,
  user_id UUID,
  timestamp TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS metrics.resource_utilization (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  resource_type TEXT NOT NULL, -- cpu, memory, disk, network
  service_name TEXT NOT NULL,
  value NUMERIC NOT NULL,
  unit TEXT NOT NULL,
  timestamp TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_custom_metrics_name ON metrics.custom_metrics(name);
CREATE INDEX IF NOT EXISTS idx_custom_metrics_timestamp ON metrics.custom_metrics(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_business_metrics_name ON metrics.business_metrics(metric_name);
CREATE INDEX IF NOT EXISTS idx_business_metrics_recorded ON metrics.business_metrics(recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_performance_endpoint ON metrics.performance_metrics(endpoint);
CREATE INDEX IF NOT EXISTS idx_performance_timestamp ON metrics.performance_metrics(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_resource_type ON metrics.resource_utilization(resource_type);
CREATE INDEX IF NOT EXISTS idx_resource_service ON metrics.resource_utilization(service_name);
CREATE INDEX IF NOT EXISTS idx_resource_timestamp ON metrics.resource_utilization(timestamp DESC);
EOSQL
  return 0
}

# Record custom metric
metrics_record() {
  local name="$1"
  local value="$2"
  local metric_type="${3:-gauge}" # counter, gauge, histogram, summary
  local labels="${4:-{}}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO metrics.custom_metrics (name, metric_type, value, labels)
     VALUES ('$name', '$metric_type', $value, '$labels'::jsonb);" >/dev/null 2>&1
}

# Record business metric
metrics_business_record() {
  local metric_name="$1"
  local value="$2"
  local dimensions="${3:-{}}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO metrics.business_metrics (metric_name, metric_value, dimensions)
     VALUES ('$metric_name', $value, '$dimensions'::jsonb);" >/dev/null 2>&1
}

# Record performance metric
metrics_performance_record() {
  local endpoint="$1"
  local method="$2"
  local response_time_ms="$3"
  local status_code="$4"
  local user_id="${5:-NULL}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO metrics.performance_metrics (endpoint, method, response_time_ms, status_code, user_id)
     VALUES ('$endpoint', '$method', $response_time_ms, $status_code, $([ -n "$user_id" ] && echo "'$user_id'" || echo "NULL"));" >/dev/null 2>&1
}

# Record resource utilization
metrics_resource_record() {
  local resource_type="$1" # cpu, memory, disk, network
  local service_name="$2"
  local value="$3"
  local unit="$4" # percent, MB, GB, Mbps

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO metrics.resource_utilization (resource_type, service_name, value, unit)
     VALUES ('$resource_type', '$service_name', $value, '$unit');" >/dev/null 2>&1
}

# Get metric statistics
metrics_stats() {
  local metric_name="$1"
  local time_range="${2:-1 hour}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local stats=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'metric', '$metric_name',
       'count', COUNT(*),
       'sum', SUM(value),
       'avg', AVG(value),
       'min', MIN(value),
       'max', MAX(value),
       'stddev', STDDEV(value),
       'time_range', '$time_range'
     )
     FROM metrics.custom_metrics
     WHERE name = '$metric_name'
       AND timestamp >= NOW() - INTERVAL '$time_range';" 2>/dev/null | xargs)

  echo "$stats"
}

# Get performance metrics summary
metrics_performance_summary() {
  local endpoint="${1:-}"
  local time_range="${2:-1 hour}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local where_clause=""
  [[ -n "$endpoint" ]] && where_clause="AND endpoint = '$endpoint'"

  local summary=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'total_requests', COUNT(*),
       'avg_response_time_ms', AVG(response_time_ms),
       'p50_response_time_ms', PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY response_time_ms),
       'p95_response_time_ms', PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time_ms),
       'p99_response_time_ms', PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY response_time_ms),
       'max_response_time_ms', MAX(response_time_ms),
       'errors', COUNT(*) FILTER (WHERE status_code >= 400),
       'error_rate', ROUND(COUNT(*) FILTER (WHERE status_code >= 400) * 100.0 / COUNT(*), 2)
     )
     FROM metrics.performance_metrics
     WHERE timestamp >= NOW() - INTERVAL '$time_range'
       $where_clause;" 2>/dev/null | xargs)

  echo "$summary"
}

# Get resource utilization trends
metrics_resource_trends() {
  local service_name="${1:-}"
  local resource_type="${2:-}"
  local time_range="${3:-1 hour}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local where_clauses=()
  [[ -n "$service_name" ]] && where_clauses+=("service_name = '$service_name'")
  [[ -n "$resource_type" ]] && where_clauses+=("resource_type = '$resource_type'")

  local where_clause=""
  [[ ${#where_clauses[@]} -gt 0 ]] && where_clause="AND ${where_clauses[*]}"

  local trends=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(t) FROM (
       SELECT
         DATE_TRUNC('minute', timestamp) AS time_bucket,
         resource_type,
         service_name,
         AVG(value) AS avg_value,
         MAX(value) AS max_value,
         unit
       FROM metrics.resource_utilization
       WHERE timestamp >= NOW() - INTERVAL '$time_range'
         $where_clause
       GROUP BY DATE_TRUNC('minute', timestamp), resource_type, service_name, unit
       ORDER BY time_bucket DESC
       LIMIT 100
     ) t;" 2>/dev/null | xargs)

  [[ -z "$trends" || "$trends" == "null" ]] && echo "[]" || echo "$trends"
}

# Collect current system metrics
metrics_collect_system() {
  # CPU usage
  local cpu_usage=$(ps aux | awk '{sum += $3} END {print sum}')
  metrics_resource_record "cpu" "system" "$cpu_usage" "percent"

  # Memory usage
  local mem_usage=$(free | awk '/Mem:/ {printf "%.2f", ($3/$2)*100}')
  metrics_resource_record "memory" "system" "$mem_usage" "percent"

  # Disk usage
  local disk_usage=$(df -h / | awk 'NR==2 {print $5}' | tr -d '%')
  metrics_resource_record "disk" "system" "$disk_usage" "percent"
}

# Collect Docker container metrics
metrics_collect_containers() {
  docker stats --no-stream --format "{{.Container}},{{.CPUPerc}},{{.MemPerc}}" | while IFS=, read -r container cpu mem; do
    cpu_val=$(echo "$cpu" | tr -d '%')
    mem_val=$(echo "$mem" | tr -d '%')

    metrics_resource_record "cpu" "$container" "$cpu_val" "percent"
    metrics_resource_record "memory" "$container" "$mem_val" "percent"
  done
}

# Export metrics to Prometheus format
metrics_export_prometheus() {
  local output_file="${1:-/tmp/nself_metrics.prom}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get all recent metrics
  local metrics=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT name, metric_type, value, labels
     FROM metrics.custom_metrics
     WHERE timestamp >= NOW() - INTERVAL '5 minutes'
     ORDER BY timestamp DESC;" 2>/dev/null)

  # Convert to Prometheus format
  {
    echo "# HELP nself_custom_metrics Custom metrics from nself"
    echo "# TYPE nself_custom_metrics gauge"

    echo "$metrics" | while read -r line; do
      [[ -z "$line" ]] && continue
      local name=$(echo "$line" | awk '{print $1}')
      local value=$(echo "$line" | awk '{print $3}')
      local labels=$(echo "$line" | awk '{print $4}')

      echo "nself_${name}{${labels}} ${value}"
    done
  } >"$output_file"
}

export -f metrics_init metrics_record metrics_business_record metrics_performance_record
export -f metrics_resource_record metrics_stats metrics_performance_summary
export -f metrics_resource_trends metrics_collect_system metrics_collect_containers
export -f metrics_export_prometheus
