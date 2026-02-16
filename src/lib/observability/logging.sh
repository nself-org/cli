#!/usr/bin/env bash
# logging.sh - Advanced structured logging
# Part of nself v0.7.0 - Sprint 7: OBS-002


# Log levels
readonly LOG_LEVEL_DEBUG=10

set -euo pipefail

readonly LOG_LEVEL_INFO=20
readonly LOG_LEVEL_WARN=30
readonly LOG_LEVEL_ERROR=40
readonly LOG_LEVEL_FATAL=50

# Initialize logging storage
logging_init() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS logs;

CREATE TABLE IF NOT EXISTS logs.entries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  timestamp TIMESTAMPTZ DEFAULT NOW(),
  level TEXT NOT NULL,
  level_numeric INTEGER NOT NULL,
  logger_name TEXT NOT NULL,
  message TEXT NOT NULL,
  context JSONB DEFAULT '{}',
  trace_id TEXT,
  span_id TEXT,
  user_id UUID,
  service_name TEXT,
  hostname TEXT,
  pid INTEGER,
  thread_id TEXT,
  file_name TEXT,
  line_number INTEGER,
  function_name TEXT,
  exception TEXT,
  stack_trace TEXT
);

CREATE TABLE IF NOT EXISTS logs.log_retention (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  log_level TEXT NOT NULL,
  retention_days INTEGER NOT NULL DEFAULT 30,
  enabled BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS logs.alert_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  condition TEXT NOT NULL, -- SQL WHERE clause
  threshold INTEGER NOT NULL DEFAULT 1,
  window_minutes INTEGER NOT NULL DEFAULT 5,
  severity TEXT NOT NULL,
  notification_channels JSONB DEFAULT '[]',
  enabled BOOLEAN DEFAULT TRUE,
  last_triggered TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs.entries(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_level ON logs.entries(level_numeric DESC);
CREATE INDEX IF NOT EXISTS idx_logs_logger ON logs.entries(logger_name);
CREATE INDEX IF NOT EXISTS idx_logs_trace ON logs.entries(trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_logs_user ON logs.entries(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_logs_service ON logs.entries(service_name);
CREATE INDEX IF NOT EXISTS idx_logs_context ON logs.entries USING GIN (context);

-- Default retention policies
INSERT INTO logs.log_retention (log_level, retention_days)
VALUES
  ('DEBUG', 7),
  ('INFO', 30),
  ('WARN', 90),
  ('ERROR', 365),
  ('FATAL', 365)
ON CONFLICT DO NOTHING;
EOSQL
  return 0
}

# Log entry with structured data
log_entry() {
  local level="$1"
  local level_numeric="$2"
  local logger_name="$3"
  local message="$4"
  local context="${5:-{}}"
  local trace_id="${6:-}"
  local user_id="${7:-}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local service_name="${SERVICE_NAME:-nself}"
  local hostname=$(hostname)
  local pid=$$

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO logs.entries (level, level_numeric, logger_name, message, context, trace_id, user_id, service_name, hostname, pid)
     VALUES (
       '$level',
       $level_numeric,
       '$logger_name',
       '$(echo "$message" | sed "s/'/''/g")',
       '$context'::jsonb,
       $([ -n "$trace_id" ] && echo "'$trace_id'" || echo "NULL"),
       $([ -n "$user_id" ] && echo "'$user_id'" || echo "NULL"),
       '$service_name',
       '$hostname',
       $pid
     );" >/dev/null 2>&1
}

# Convenience logging functions
log_debug() {
  local logger="$1"
  local message="$2"
  local context="${3:-{}}"
  log_entry "DEBUG" "$LOG_LEVEL_DEBUG" "$logger" "$message" "$context"
}

log_info() {
  local logger="$1"
  local message="$2"
  local context="${3:-{}}"
  log_entry "INFO" "$LOG_LEVEL_INFO" "$logger" "$message" "$context"
}

log_warn() {
  local logger="$1"
  local message="$2"
  local context="${3:-{}}"
  log_entry "WARN" "$LOG_LEVEL_WARN" "$logger" "$message" "$context"
}

log_error() {
  local logger="$1"
  local message="$2"
  local context="${3:-{}}"
  log_entry "ERROR" "$LOG_LEVEL_ERROR" "$logger" "$message" "$context"
}

log_fatal() {
  local logger="$1"
  local message="$2"
  local context="${3:-{}}"
  log_entry "FATAL" "$LOG_LEVEL_FATAL" "$logger" "$message" "$context"
}

# Search logs with filters
log_search() {
  local level="${1:-}"
  local logger="${2:-}"
  local message_pattern="${3:-}"
  local time_range="${4:-1 hour}"
  local limit="${5:-100}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local where_clauses=("timestamp >= NOW() - INTERVAL '$time_range'")
  [[ -n "$level" ]] && where_clauses+=("level = '$level'")
  [[ -n "$logger" ]] && where_clauses+=("logger_name = '$logger'")
  [[ -n "$message_pattern" ]] && where_clauses+=("message ILIKE '%$message_pattern%'")

  local where_clause=$(
    IFS=' AND '
    echo "${where_clauses[*]}"
  )

  local results=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(l) FROM (
       SELECT
         id,
         timestamp,
         level,
         logger_name,
         message,
         context,
         trace_id,
         user_id,
         service_name
       FROM logs.entries
       WHERE $where_clause
       ORDER BY timestamp DESC
       LIMIT $limit
     ) l;" 2>/dev/null | xargs)

  [[ -z "$results" || "$results" == "null" ]] && echo "[]" || echo "$results"
}

# Aggregate logs by level
log_aggregate_by_level() {
  local time_range="${1:-1 hour}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local aggregation=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(a) FROM (
       SELECT
         level,
         COUNT(*) AS count,
         MIN(timestamp) AS first_seen,
         MAX(timestamp) AS last_seen
       FROM logs.entries
       WHERE timestamp >= NOW() - INTERVAL '$time_range'
       GROUP BY level
       ORDER BY level_numeric DESC
     ) a;" 2>/dev/null | xargs)

  [[ -z "$aggregation" || "$aggregation" == "null" ]] && echo "[]" || echo "$aggregation"
}

# Get log statistics
log_stats() {
  local time_range="${1:-1 hour}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local stats=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'total_entries', COUNT(*),
       'debug_count', COUNT(*) FILTER (WHERE level = 'DEBUG'),
       'info_count', COUNT(*) FILTER (WHERE level = 'INFO'),
       'warn_count', COUNT(*) FILTER (WHERE level = 'WARN'),
       'error_count', COUNT(*) FILTER (WHERE level = 'ERROR'),
       'fatal_count', COUNT(*) FILTER (WHERE level = 'FATAL'),
       'unique_loggers', COUNT(DISTINCT logger_name),
       'unique_services', COUNT(DISTINCT service_name),
       'time_range', '$time_range'
     )
     FROM logs.entries
     WHERE timestamp >= NOW() - INTERVAL '$time_range';" 2>/dev/null | xargs)

  echo "$stats"
}

# Create alert rule
log_alert_create() {
  local name="$1"
  local condition="$2" # SQL WHERE clause
  local threshold="$3"
  local window_minutes="$4"
  local severity="$5"
  local channels="${6:-[]}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO logs.alert_rules (name, condition, threshold, window_minutes, severity, notification_channels)
     VALUES (
       '$name',
       '$condition',
       $threshold,
       $window_minutes,
       '$severity',
       '$channels'::jsonb
     )
     ON CONFLICT (name) DO UPDATE SET
       condition = EXCLUDED.condition,
       threshold = EXCLUDED.threshold,
       window_minutes = EXCLUDED.window_minutes,
       severity = EXCLUDED.severity,
       notification_channels = EXCLUDED.notification_channels,
       enabled = TRUE;" >/dev/null 2>&1
}

# Check alert rules
log_alert_check() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get all active rules
  local rules=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(r) FROM (
       SELECT id, name, condition, threshold, window_minutes, severity, notification_channels
       FROM logs.alert_rules
       WHERE enabled = TRUE
     ) r;" 2>/dev/null | xargs)

  [[ -z "$rules" || "$rules" == "null" ]] && return 0

  # Check each rule
  echo "$rules" | jq -c '.[]' | while read -r rule; do
    local rule_id=$(echo "$rule" | jq -r '.id')
    local rule_name=$(echo "$rule" | jq -r '.name')
    local condition=$(echo "$rule" | jq -r '.condition')
    local threshold=$(echo "$rule" | jq -r '.threshold')
    local window=$(echo "$rule" | jq -r '.window_minutes')

    # Count matching log entries
    local count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT COUNT(*)
       FROM logs.entries
       WHERE timestamp >= NOW() - INTERVAL '$window minutes'
         AND ($condition);" 2>/dev/null | xargs)

    if [[ $count -ge $threshold ]]; then
      echo "ALERT: $rule_name triggered ($count entries in last $window minutes)"

      # Update last_triggered
      docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
        "UPDATE logs.alert_rules SET last_triggered = NOW() WHERE id = '$rule_id';" >/dev/null 2>&1
    fi
  done
}

# Apply retention policies
log_cleanup() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get retention policies
  local policies=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(p) FROM (
       SELECT log_level, retention_days
       FROM logs.log_retention
       WHERE enabled = TRUE
     ) p;" 2>/dev/null | xargs)

  [[ -z "$policies" || "$policies" == "null" ]] && return 0

  # Apply each policy
  echo "$policies" | jq -c '.[]' | while read -r policy; do
    local level=$(echo "$policy" | jq -r '.log_level')
    local days=$(echo "$policy" | jq -r '.retention_days')

    local deleted=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "WITH deleted AS (
         DELETE FROM logs.entries
         WHERE level = '$level'
           AND timestamp < NOW() - INTERVAL '$days days'
         RETURNING id
       )
       SELECT COUNT(*) FROM deleted;" 2>/dev/null | xargs)

    echo "Cleaned $deleted $level logs older than $days days"
  done
}

# Export logs
log_export() {
  local format="${1:-json}" # json, csv
  local output_file="${2:-/tmp/nself_logs.json}"
  local time_range="${3:-24 hours}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ "$format" == "json" ]]; then
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_agg(l) FROM (
         SELECT * FROM logs.entries
         WHERE timestamp >= NOW() - INTERVAL '$time_range'
         ORDER BY timestamp DESC
       ) l;" 2>/dev/null | xargs >"$output_file"
  else
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "COPY (
         SELECT timestamp, level, logger_name, message, service_name
         FROM logs.entries
         WHERE timestamp >= NOW() - INTERVAL '$time_range'
         ORDER BY timestamp DESC
       ) TO STDOUT WITH CSV HEADER;" 2>/dev/null >"$output_file"
  fi

  echo "Exported logs to $output_file"
}

export -f logging_init log_entry log_debug log_info log_warn log_error log_fatal
export -f log_search log_aggregate_by_level log_stats log_alert_create log_alert_check
export -f log_cleanup log_export
