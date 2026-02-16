#!/usr/bin/env bash
# core.sh - Redis core functionality
# Part of nself v0.7.0 - Sprint 6: RDS-001


# Redis connection defaults
readonly REDIS_DEFAULT_HOST="${REDIS_HOST:-localhost}"

set -euo pipefail

readonly REDIS_DEFAULT_PORT="${REDIS_PORT:-6379}"
readonly REDIS_DEFAULT_DB="${REDIS_DB:-0}"
readonly REDIS_DEFAULT_TIMEOUT="${REDIS_TIMEOUT:-5}"
readonly REDIS_DEFAULT_POOL_SIZE="${REDIS_POOL_SIZE:-10}"

# Initialize Redis
redis_init() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  # Create Redis configuration table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS redis_config;

CREATE TABLE IF NOT EXISTS redis_config.connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  host TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 6379,
  database INTEGER NOT NULL DEFAULT 0,
  password TEXT,
  tls_enabled BOOLEAN DEFAULT FALSE,
  cluster_mode BOOLEAN DEFAULT FALSE,
  sentinel_mode BOOLEAN DEFAULT FALSE,
  max_connections INTEGER DEFAULT 10,
  connection_timeout INTEGER DEFAULT 5,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redis_config.pools (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id UUID NOT NULL REFERENCES redis_config.connections(id) ON DELETE CASCADE,
  pool_size INTEGER NOT NULL DEFAULT 10,
  min_idle INTEGER NOT NULL DEFAULT 2,
  max_idle INTEGER NOT NULL DEFAULT 5,
  idle_timeout INTEGER DEFAULT 300,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redis_config.health_checks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id UUID NOT NULL REFERENCES redis_config.connections(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  response_time_ms INTEGER,
  error_message TEXT,
  checked_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_connections_name ON redis_config.connections(name);
CREATE INDEX IF NOT EXISTS idx_connections_active ON redis_config.connections(is_active);
CREATE INDEX IF NOT EXISTS idx_health_checks_connection ON redis_config.health_checks(connection_id);
CREATE INDEX IF NOT EXISTS idx_health_checks_checked_at ON redis_config.health_checks(checked_at DESC);
EOSQL
  return 0
}

# Add Redis connection
redis_connection_add() {
  local name="$1"
  local host="${2:-$REDIS_DEFAULT_HOST}"
  local port="${3:-$REDIS_DEFAULT_PORT}"
  local database="${4:-$REDIS_DEFAULT_DB}"
  local password="${5:-}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local conn_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO redis_config.connections (name, host, port, database, password)
     VALUES ('$name', '$host', $port, $database, $([ -n "$password" ] && echo "'$password'" || echo "NULL"))
     RETURNING id;" 2>/dev/null | xargs)

  echo "$conn_id"
}

# Get Redis connection
redis_connection_get() {
  local name="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local conn=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'id', id,
       'name', name,
       'host', host,
       'port', port,
       'database', database,
       'cluster_mode', cluster_mode,
       'sentinel_mode', sentinel_mode,
       'max_connections', max_connections,
       'connection_timeout', connection_timeout
     ) FROM redis_config.connections WHERE name = '$name' AND is_active = TRUE;" 2>/dev/null | xargs)

  echo "$conn"
}

# List Redis connections
redis_connection_list() {
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local conns=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(c) FROM (
       SELECT id, name, host, port, database, cluster_mode, is_active, created_at
       FROM redis_config.connections
       ORDER BY created_at DESC
     ) c;" 2>/dev/null | xargs)

  [[ -z "$conns" || "$conns" == "null" ]] && echo "[]" || echo "$conns"
}

# Delete Redis connection
redis_connection_delete() {
  local name="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM redis_config.connections WHERE name = '$name';" >/dev/null 2>&1
}

# Test Redis connection
redis_connection_test() {
  local name="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get connection details
  local conn=$(redis_connection_get "$name")
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "ERROR: Connection not found" >&2
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')
  local timeout=$(echo "$conn" | jq -r '.connection_timeout')

  # Try to ping Redis
  local start_time=$(date +%s%3N)

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  if [[ -n "$redis_container" ]]; then
    if docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" PING >/dev/null 2>&1; then
      local end_time=$(date +%s%3N)
      local response_time=$((end_time - start_time))

      # Log health check success
      docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
        "INSERT INTO redis_config.health_checks (connection_id, status, response_time_ms)
         SELECT id, 'healthy', $response_time FROM redis_config.connections WHERE name = '$name';" >/dev/null 2>&1

      echo "{\"status\":\"healthy\",\"response_time_ms\":$response_time}"
      return 0
    fi
  fi

  # Log health check failure
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO redis_config.health_checks (connection_id, status, error_message)
     SELECT id, 'unhealthy', 'Connection timeout' FROM redis_config.connections WHERE name = '$name';" >/dev/null 2>&1

  echo "{\"status\":\"unhealthy\",\"error\":\"Connection timeout\"}"
  return 1
}

# Get Redis health status
redis_health_status() {
  local name="${1:-}"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -n "$name" ]]; then
    # Get health for specific connection
    local health=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_build_object(
         'connection', c.name,
         'status', h.status,
         'response_time_ms', h.response_time_ms,
         'error', h.error_message,
         'checked_at', h.checked_at
       )
       FROM redis_config.health_checks h
       JOIN redis_config.connections c ON h.connection_id = c.id
       WHERE c.name = '$name'
       ORDER BY h.checked_at DESC LIMIT 1;" 2>/dev/null | xargs)

    echo "$health"
  else
    # Get health for all connections
    local health=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT json_agg(h) FROM (
         SELECT DISTINCT ON (c.name)
           c.name AS connection,
           h.status,
           h.response_time_ms,
           h.error_message AS error,
           h.checked_at
         FROM redis_config.health_checks h
         JOIN redis_config.connections c ON h.connection_id = c.id
         WHERE c.is_active = TRUE
         ORDER BY c.name, h.checked_at DESC
       ) h;" 2>/dev/null | xargs)

    [[ -z "$health" || "$health" == "null" ]] && echo "[]" || echo "$health"
  fi
}

# Configure connection pool
redis_pool_configure() {
  local connection_name="$1"
  local pool_size="${2:-$REDIS_DEFAULT_POOL_SIZE}"
  local min_idle="${3:-2}"
  local max_idle="${4:-5}"
  local idle_timeout="${5:-300}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  # Get connection ID
  local conn_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM redis_config.connections WHERE name = '$connection_name';" 2>/dev/null | xargs)

  [[ -z "$conn_id" ]] && {
    echo "ERROR: Connection not found" >&2
    return 1
  }

  # Create or update pool configuration
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO redis_config.pools (connection_id, pool_size, min_idle, max_idle, idle_timeout)
     VALUES ('$conn_id', $pool_size, $min_idle, $max_idle, $idle_timeout)
     ON CONFLICT (connection_id) DO UPDATE SET
       pool_size = EXCLUDED.pool_size,
       min_idle = EXCLUDED.min_idle,
       max_idle = EXCLUDED.max_idle,
       idle_timeout = EXCLUDED.idle_timeout;" >/dev/null 2>&1
}

# Get pool configuration
redis_pool_get() {
  local connection_name="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local pool=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_build_object(
       'connection', c.name,
       'pool_size', p.pool_size,
       'min_idle', p.min_idle,
       'max_idle', p.max_idle,
       'idle_timeout', p.idle_timeout
     )
     FROM redis_config.pools p
     JOIN redis_config.connections c ON p.connection_id = c.id
     WHERE c.name = '$connection_name';" 2>/dev/null | xargs)

  echo "$pool"
}

export -f redis_init redis_connection_add redis_connection_get redis_connection_list redis_connection_delete
export -f redis_connection_test redis_health_status redis_pool_configure redis_pool_get
