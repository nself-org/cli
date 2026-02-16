#!/usr/bin/env bash
# api.sh - Admin API endpoints
# Part of nself v0.9.0
#
# SECURITY: All queries use parameterized queries via safe-query.sh


# Source safe query library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${SCRIPT_DIR}/../database/safe-query.sh"

# Get admin statistics overview
# Usage: admin_stats_overview
# Returns: JSON object with system statistics
admin_stats_overview() {
  local container
  container=$(pg_get_container) || return 1

  # Use safe query - no user input needed, but consistent pattern
  local stats
  stats=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         (SELECT COUNT(*) FROM auth.users WHERE deleted_at IS NULL) AS total_users,
         (SELECT COUNT(*) FROM auth.sessions WHERE expires_at > NOW()) AS active_sessions,
         (SELECT COUNT(*) FROM auth.roles WHERE is_system = FALSE) AS custom_roles,
         (SELECT COUNT(*) FROM secrets.vault WHERE is_active = TRUE) AS total_secrets,
         (SELECT COUNT(*) FROM webhooks.endpoints WHERE enabled = TRUE) AS active_webhooks,
         (SELECT COUNT(*) FROM rate_limit.log WHERE requested_at >= NOW() - INTERVAL '1 hour') AS requests_last_hour
     ) s;" 2>/dev/null | xargs)

  if [[ -z "$stats" ]] || [[ "$stats" == "null" ]]; then
    echo "{}"
  else
    echo "$stats" | jq '.'
  fi
}

# List users with pagination
# Usage: admin_users_list [limit] [offset]
# Returns: JSON array of users
admin_users_list() {
  local limit="${1:-50}"
  local offset="${2:-0}"

  # Validate numeric inputs
  limit=$(validate_integer "$limit" 1 1000) || {
    echo "ERROR: Invalid limit" >&2
    return 1
  }
  offset=$(validate_integer "$offset" 0) || {
    echo "ERROR: Invalid offset" >&2
    return 1
  }

  # Use parameterized query with validated integers
  local query="SELECT json_agg(u) FROM (
       SELECT id, email, created_at, last_sign_in_at, email_verified,
              (SELECT json_agg(r.name) FROM auth.user_roles ur
               JOIN auth.roles r ON ur.role_id = r.id WHERE ur.user_id = users.id) AS roles
       FROM auth.users WHERE deleted_at IS NULL
       ORDER BY created_at DESC LIMIT :param1 OFFSET :param2
     ) u"

  pg_query_json_array "$query" "$limit" "$offset" | jq '.'
}

# Get recent activity logs
# Usage: admin_activity_recent [hours]
# Returns: JSON array of activity events
admin_activity_recent() {
  local hours="${1:-24}"

  # Validate numeric input
  hours=$(validate_integer "$hours" 1 720) || {
    echo "ERROR: Invalid hours (must be 1-720)" >&2
    return 1
  }

  # Use parameterized query - construct interval safely
  local query="SELECT json_agg(a) FROM (
       SELECT event_type, action, result, created_at
       FROM audit.events
       WHERE created_at >= NOW() - INTERVAL '1 hour' * :param1
       ORDER BY created_at DESC LIMIT 100
     ) a"

  pg_query_json_array "$query" "$hours" | jq '.'
}

# Get security events (rate limit violations)
# Usage: admin_security_events
# Returns: JSON array of security events
admin_security_events() {
  local container
  container=$(pg_get_container) || return 1

  # No user input, safe to use direct query
  local events
  events=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(e) FROM (
       SELECT 'rate_limit' AS type, key, COUNT(*) AS count
       FROM rate_limit.log
       WHERE allowed = FALSE AND requested_at >= NOW() - INTERVAL '1 hour'
       GROUP BY key
       ORDER BY count DESC LIMIT 10
     ) e;" 2>/dev/null | xargs)

  if [[ -z "$events" ]] || [[ "$events" == "null" ]]; then
    echo "[]"
  else
    echo "$events" | jq '.'
  fi
}

export -f admin_stats_overview admin_users_list admin_activity_recent admin_security_events
