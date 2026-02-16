#!/usr/bin/env bash
# tracing.sh - Distributed tracing system
# Part of nself v0.7.0 - Sprint 7: OBS-003


# Initialize tracing storage
tracing_init() {

set -euo pipefail

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS tracing;

CREATE TABLE IF NOT EXISTS tracing.traces (
  trace_id TEXT PRIMARY KEY,
  service_name TEXT NOT NULL,
  operation_name TEXT NOT NULL,
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  duration_ms INTEGER,
  status TEXT, -- ok, error, timeout
  tags JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tracing.spans (
  span_id TEXT PRIMARY KEY,
  trace_id TEXT NOT NULL REFERENCES tracing.traces(trace_id) ON DELETE CASCADE,
  parent_span_id TEXT,
  service_name TEXT NOT NULL,
  operation_name TEXT NOT NULL,
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  duration_ms INTEGER,
  tags JSONB DEFAULT '{}',
  logs JSONB DEFAULT '[]',
  status TEXT
);

CREATE TABLE IF NOT EXISTS tracing.span_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  span_id TEXT NOT NULL REFERENCES tracing.spans(span_id) ON DELETE CASCADE,
  timestamp TIMESTAMPTZ DEFAULT NOW(),
  event_name TEXT NOT NULL,
  attributes JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_traces_service ON tracing.traces(service_name);
CREATE INDEX IF NOT EXISTS idx_traces_start ON tracing.traces(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_traces_duration ON tracing.traces(duration_ms DESC);
CREATE INDEX IF NOT EXISTS idx_traces_status ON tracing.traces(status);
CREATE INDEX IF NOT EXISTS idx_spans_trace ON tracing.spans(trace_id);
CREATE INDEX IF NOT EXISTS idx_spans_parent ON tracing.spans(parent_span_id) WHERE parent_span_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_spans_service ON tracing.spans(service_name);
CREATE INDEX IF NOT EXISTS idx_span_events_span ON tracing.span_events(span_id);
EOSQL
  return 0
}

# Generate trace/span IDs
trace_generate_id() {
  echo "$(date +%s%N | md5sum | cut -d' ' -f1)"
}

span_generate_id() {
  echo "$(date +%s%N | md5sum | cut -d' ' -f1 | cut -c1-16)"
}

# Start a new trace
trace_start() {
  local service_name="$1"
  local operation_name="$2"
  local tags="${3:-{}}"

  local trace_id=$(trace_generate_id)
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO tracing.traces (trace_id, service_name, operation_name, start_time, tags)
     VALUES ('$trace_id', '$service_name', '$operation_name', NOW(), '$tags'::jsonb);" >/dev/null 2>&1

  echo "$trace_id"
}

# End a trace
trace_end() {
  local trace_id="$1"
  local status="${2:-ok}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE tracing.traces
     SET end_time = NOW(),
         duration_ms = EXTRACT(EPOCH FROM (NOW() - start_time)) * 1000,
         status = '$status'
     WHERE trace_id = '$trace_id';" >/dev/null 2>&1
}

# Start a span
span_start() {
  local trace_id="$1"
  local service_name="$2"
  local operation_name="$3"
  local parent_span_id="${4:-}"
  local tags="${5:-{}}"

  local span_id=$(span_generate_id)
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO tracing.spans (span_id, trace_id, parent_span_id, service_name, operation_name, start_time, tags)
     VALUES (
       '$span_id',
       '$trace_id',
       $([ -n "$parent_span_id" ] && echo "'$parent_span_id'" || echo "NULL"),
       '$service_name',
       '$operation_name',
       NOW(),
       '$tags'::jsonb
     );" >/dev/null 2>&1

  echo "$span_id"
}

# End a span
span_end() {
  local span_id="$1"
  local status="${2:-ok}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE tracing.spans
     SET end_time = NOW(),
         duration_ms = EXTRACT(EPOCH FROM (NOW() - start_time)) * 1000,
         status = '$status'
     WHERE span_id = '$span_id';" >/dev/null 2>&1
}

# Add span event/log
span_log() {
  local span_id="$1"
  local event_name="$2"
  local attributes="${3:-{}}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO tracing.span_events (span_id, event_name, attributes)
     VALUES ('$span_id', '$event_name', '$attributes'::jsonb);" >/dev/null 2>&1
}

# Get trace with all spans
trace_get() {
  local trace_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local trace=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'trace_id', t.trace_id,
       'service_name', t.service_name,
       'operation_name', t.operation_name,
       'start_time', t.start_time,
       'end_time', t.end_time,
       'duration_ms', t.duration_ms,
       'status', t.status,
       'tags', t.tags,
       'spans', (
         SELECT json_agg(s)
         FROM (
           SELECT
             span_id,
             parent_span_id,
             service_name,
             operation_name,
             start_time,
             end_time,
             duration_ms,
             tags,
             status
           FROM tracing.spans
           WHERE trace_id = t.trace_id
           ORDER BY start_time
         ) s
       )
     )
     FROM tracing.traces t
     WHERE t.trace_id = '$trace_id';" 2>/dev/null | xargs)

  [[ -z "$trace" || "$trace" == "null" ]] && echo "null" || echo "$trace"
}

# List recent traces
trace_list() {
  local service_name="${1:-}"
  local status="${2:-}"
  local limit="${3:-50}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local where_clauses=()
  [[ -n "$service_name" ]] && where_clauses+=("service_name = '$service_name'")
  [[ -n "$status" ]] && where_clauses+=("status = '$status'")

  local where_clause="TRUE"
  [[ ${#where_clauses[@]} -gt 0 ]] && where_clause=$(
    IFS=' AND '
    echo "${where_clauses[*]}"
  )

  local traces=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(t) FROM (
       SELECT
         trace_id,
         service_name,
         operation_name,
         start_time,
         duration_ms,
         status,
         (SELECT COUNT(*) FROM tracing.spans WHERE trace_id = traces.trace_id) AS span_count
       FROM tracing.traces
       WHERE $where_clause
       ORDER BY start_time DESC
       LIMIT $limit
     ) t;" 2>/dev/null | xargs)

  [[ -z "$traces" || "$traces" == "null" ]] && echo "[]" || echo "$traces"
}

# Find slow traces
trace_find_slow() {
  local threshold_ms="${1:-1000}"
  local time_range="${2:-1 hour}"
  local limit="${3:-20}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local traces=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(t) FROM (
       SELECT
         trace_id,
         service_name,
         operation_name,
         duration_ms,
         start_time,
         status
       FROM tracing.traces
       WHERE start_time >= NOW() - INTERVAL '$time_range'
         AND duration_ms > $threshold_ms
       ORDER BY duration_ms DESC
       LIMIT $limit
     ) t;" 2>/dev/null | xargs)

  [[ -z "$traces" || "$traces" == "null" ]] && echo "[]" || echo "$traces"
}

# Find error traces
trace_find_errors() {
  local time_range="${1:-1 hour}"
  local limit="${2:-50}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local traces=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(t) FROM (
       SELECT
         trace_id,
         service_name,
         operation_name,
         duration_ms,
         start_time,
         status
       FROM tracing.traces
       WHERE start_time >= NOW() - INTERVAL '$time_range'
         AND status = 'error'
       ORDER BY start_time DESC
       LIMIT $limit
     ) t;" 2>/dev/null | xargs)

  [[ -z "$traces" || "$traces" == "null" ]] && echo "[]" || echo "$traces"
}

# Get service statistics
trace_service_stats() {
  local service_name="$1"
  local time_range="${2:-1 hour}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local stats=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'service_name', '$service_name',
       'total_traces', COUNT(*),
       'avg_duration_ms', AVG(duration_ms),
       'p50_duration_ms', PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms),
       'p95_duration_ms', PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms),
       'p99_duration_ms', PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_ms),
       'max_duration_ms', MAX(duration_ms),
       'error_count', COUNT(*) FILTER (WHERE status = 'error'),
       'error_rate', ROUND(COUNT(*) FILTER (WHERE status = 'error') * 100.0 / COUNT(*), 2)
     )
     FROM tracing.traces
     WHERE service_name = '$service_name'
       AND start_time >= NOW() - INTERVAL '$time_range';" 2>/dev/null | xargs)

  echo "$stats"
}

# Visualize trace (simple text format)
trace_visualize() {
  local trace_id="$1"
  local trace=$(trace_get "$trace_id")

  [[ "$trace" == "null" ]] && {
    echo "Trace not found"
    return 1
  }

  local operation=$(echo "$trace" | jq -r '.operation_name')
  local duration=$(echo "$trace" | jq -r '.duration_ms')
  local status=$(echo "$trace" | jq -r '.status')

  echo "Trace: $trace_id"
  echo "Operation: $operation"
  echo "Duration: ${duration}ms"
  echo "Status: $status"
  echo ""
  echo "Spans:"

  echo "$trace" | jq -r '.spans[] | "\(.start_time) [\(.duration_ms)ms] \(.service_name):\(.operation_name) (\(.status))"'
}

export -f tracing_init trace_generate_id span_generate_id
export -f trace_start trace_end span_start span_end span_log
export -f trace_get trace_list trace_find_slow trace_find_errors
export -f trace_service_stats trace_visualize
