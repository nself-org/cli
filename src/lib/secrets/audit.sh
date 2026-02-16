#!/usr/bin/env bash
# audit.sh - Secret access auditing (SEC-005)
# Part of nself v0.6.0 - Phase 1 Sprint 4
#
# Tracks all secret access for security and compliance


# ============================================================================
# Audit Initialization
# ============================================================================

# Initialize audit system
# Usage: audit_init
audit_init() {

set -euo pipefail

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create audit table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS secrets;

CREATE TABLE IF NOT EXISTS secrets.audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  secret_id UUID,
  key_name TEXT NOT NULL,
  environment TEXT NOT NULL,
  action TEXT NOT NULL,
  actor_id UUID,
  actor_type TEXT,
  ip_address TEXT,
  user_agent TEXT,
  result TEXT NOT NULL,
  error_message TEXT,
  accessed_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_secret_id ON secrets.audit_log(secret_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_key_name ON secrets.audit_log(key_name);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON secrets.audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor_id ON secrets.audit_log(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_accessed_at ON secrets.audit_log(accessed_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_result ON secrets.audit_log(result);
EOSQL

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to initialize audit system" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# Audit Logging
# ============================================================================

# Log secret access
# Usage: audit_log <action> <key_name> <environment> <result> [secret_id] [actor_id] [error_message]
# Actions: get, set, delete, rotate, list
# Results: success, failure, unauthorized
audit_log() {
  local action="$1"
  local key_name="$2"
  local environment="$3"
  local result="$4"
  local secret_id="${5:-}"
  local actor_id="${6:-}"
  local error_message="${7:-}"

  if [[ -z "$action" ]] || [[ -z "$key_name" ]] || [[ -z "$environment" ]] || [[ -z "$result" ]]; then
    return 0 # Fail silently for audit logging
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 0 # Fail silently
  fi

  # Escape values
  key_name=$(echo "$key_name" | sed "s/'/''/g")
  environment=$(echo "$environment" | sed "s/'/''/g")
  error_message=$(echo "$error_message" | sed "s/'/''/g")

  # Build secret_id SQL
  local secret_id_sql="NULL"
  if [[ -n "$secret_id" ]]; then
    secret_id_sql="'$secret_id'"
  fi

  # Build actor_id SQL
  local actor_id_sql="NULL"
  local actor_type_sql="'system'"
  if [[ -n "$actor_id" ]]; then
    actor_id_sql="'$actor_id'"
    actor_type_sql="'user'"
  fi

  # Build error message SQL
  local error_sql="NULL"
  if [[ -n "$error_message" ]]; then
    error_sql="'$error_message'"
  fi

  # Log the access
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO secrets.audit_log (secret_id, key_name, environment, action, actor_id, actor_type, result, error_message)
     VALUES ($secret_id_sql, '$key_name', '$environment', '$action', $actor_id_sql, $actor_type_sql, '$result', $error_sql);" \
    >/dev/null 2>&1

  return 0
}

# ============================================================================
# Audit Queries
# ============================================================================

# Get audit logs
# Usage: audit_get_logs [key_name] [environment] [action] [limit]
audit_get_logs() {
  local key_name="${1:-}"
  local environment="${2:-}"
  local action="${3:-}"
  local limit="${4:-100}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build where clause
  local where_parts=()
  if [[ -n "$key_name" ]]; then
    where_parts+=("key_name = '$key_name'")
  fi
  if [[ -n "$environment" ]]; then
    where_parts+=("environment = '$environment'")
  fi
  if [[ -n "$action" ]]; then
    where_parts+=("action = '$action'")
  fi

  local where_clause=""
  if [[ ${#where_parts[@]} -gt 0 ]]; then
    where_clause="WHERE $(
      IFS=' AND '
      echo "${where_parts[*]}"
    )"
  fi

  # Get logs
  local logs_json
  logs_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(l) FROM (
       SELECT id, secret_id, key_name, environment, action, actor_id, actor_type, result, error_message, accessed_at
       FROM secrets.audit_log
       $where_clause
       ORDER BY accessed_at DESC
       LIMIT $limit
     ) l;" \
    2>/dev/null | xargs)

  if [[ -z "$logs_json" ]] || [[ "$logs_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$logs_json"
  return 0
}

# Get audit summary for a secret
# Usage: audit_get_summary <key_name> [environment]
audit_get_summary() {
  local key_name="$1"
  local environment="${2:-default}"

  if [[ -z "$key_name" ]]; then
    echo "ERROR: Key name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get summary
  local summary_json
  summary_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         key_name,
         environment,
         COUNT(*) AS total_accesses,
         SUM(CASE WHEN action = 'get' THEN 1 ELSE 0 END) AS read_count,
         SUM(CASE WHEN action = 'set' THEN 1 ELSE 0 END) AS write_count,
         SUM(CASE WHEN action = 'delete' THEN 1 ELSE 0 END) AS delete_count,
         SUM(CASE WHEN action = 'rotate' THEN 1 ELSE 0 END) AS rotate_count,
         SUM(CASE WHEN result = 'success' THEN 1 ELSE 0 END) AS success_count,
         SUM(CASE WHEN result = 'failure' THEN 1 ELSE 0 END) AS failure_count,
         MAX(accessed_at) AS last_accessed,
         array_agg(DISTINCT actor_id) FILTER (WHERE actor_id IS NOT NULL) AS unique_actors
       FROM secrets.audit_log
       WHERE key_name = '$key_name' AND environment = '$environment'
       GROUP BY key_name, environment
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$summary_json" ]] || [[ "$summary_json" == "null" ]]; then
    echo "{}"
    return 0
  fi

  echo "$summary_json"
  return 0
}

# Get recent access activity
# Usage: audit_get_recent_activity [limit]
audit_get_recent_activity() {
  local limit="${1:-20}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get recent activity
  local activity_json
  activity_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(a) FROM (
       SELECT
         key_name,
         environment,
         action,
         result,
         actor_type,
         accessed_at
       FROM secrets.audit_log
       ORDER BY accessed_at DESC
       LIMIT $limit
     ) a;" \
    2>/dev/null | xargs)

  if [[ -z "$activity_json" ]] || [[ "$activity_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$activity_json"
  return 0
}

# Get failed access attempts
# Usage: audit_get_failures [key_name] [limit]
audit_get_failures() {
  local key_name="${1:-}"
  local limit="${2:-50}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build where clause
  local where_clause="WHERE result = 'failure' OR result = 'unauthorized'"
  if [[ -n "$key_name" ]]; then
    where_clause="$where_clause AND key_name = '$key_name'"
  fi

  # Get failures
  local failures_json
  failures_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(f) FROM (
       SELECT
         key_name,
         environment,
         action,
         result,
         error_message,
         actor_id,
         actor_type,
         accessed_at
       FROM secrets.audit_log
       $where_clause
       ORDER BY accessed_at DESC
       LIMIT $limit
     ) f;" \
    2>/dev/null | xargs)

  if [[ -z "$failures_json" ]] || [[ "$failures_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$failures_json"
  return 0
}

# ============================================================================
# Audit Analysis
# ============================================================================

# Get access patterns (who accessed what, when)
# Usage: audit_get_access_patterns [days]
audit_get_access_patterns() {
  local days="${1:-7}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get patterns
  local patterns_json
  patterns_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(p) FROM (
       SELECT
         key_name,
         environment,
         COUNT(*) AS access_count,
         COUNT(DISTINCT actor_id) AS unique_actors,
         array_agg(DISTINCT action) AS actions,
         MIN(accessed_at) AS first_access,
         MAX(accessed_at) AS last_access
       FROM secrets.audit_log
       WHERE accessed_at >= NOW() - INTERVAL '$days days'
       GROUP BY key_name, environment
       ORDER BY access_count DESC
     ) p;" \
    2>/dev/null | xargs)

  if [[ -z "$patterns_json" ]] || [[ "$patterns_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$patterns_json"
  return 0
}

# Detect suspicious activity
# Usage: audit_detect_suspicious [threshold]
audit_detect_suspicious() {
  local threshold="${1:-10}" # Failed attempts threshold

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Detect patterns
  local suspicious_json
  suspicious_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(s) FROM (
       SELECT
         key_name,
         environment,
         COUNT(*) AS failed_attempts,
         array_agg(DISTINCT actor_id) FILTER (WHERE actor_id IS NOT NULL) AS actors,
         MAX(accessed_at) AS last_attempt
       FROM secrets.audit_log
       WHERE result IN ('failure', 'unauthorized')
         AND accessed_at >= NOW() - INTERVAL '1 hour'
       GROUP BY key_name, environment
       HAVING COUNT(*) >= $threshold
       ORDER BY failed_attempts DESC
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$suspicious_json" ]] || [[ "$suspicious_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$suspicious_json"
  return 0
}

# ============================================================================
# Audit Cleanup
# ============================================================================

# Clean old audit logs
# Usage: audit_cleanup [days_to_keep]
audit_cleanup() {
  local days_to_keep="${1:-90}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete old logs
  local deleted
  deleted=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "WITH deleted AS (
       DELETE FROM secrets.audit_log
       WHERE accessed_at < NOW() - INTERVAL '$days_to_keep days'
       RETURNING id
     )
     SELECT COUNT(*) FROM deleted;" \
    2>/dev/null | xargs)

  echo "Deleted $deleted old audit logs" >&2
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f audit_init
export -f audit_log
export -f audit_get_logs
export -f audit_get_summary
export -f audit_get_recent_activity
export -f audit_get_failures
export -f audit_get_access_patterns
export -f audit_detect_suspicious
export -f audit_cleanup
