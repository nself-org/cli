#!/usr/bin/env bash
# endpoint-limiter.sh - Endpoint-based rate limiting (RATE-006)
# Part of nself v0.6.0 - Phase 1 Sprint 5
#
# Rate limiting by API endpoint/route


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/core.sh" ]]; then
  source "$SCRIPT_DIR/core.sh"
fi
if [[ -f "$SCRIPT_DIR/strategies.sh" ]]; then
  source "$SCRIPT_DIR/strategies.sh"
fi

# Endpoint rate limit defaults
readonly ENDPOINT_RATE_LIMIT_MAX=500   # 500 requests
readonly ENDPOINT_RATE_LIMIT_WINDOW=60 # per minute

# ============================================================================
# Endpoint Rate Limiting
# ============================================================================

# Check rate limit for endpoint
# Usage: endpoint_rate_limit_check <endpoint> [max_requests] [window_seconds] [strategy]
endpoint_rate_limit_check() {
  local endpoint="$1"
  local max_requests="${2:-$ENDPOINT_RATE_LIMIT_MAX}"
  local window_seconds="${3:-$ENDPOINT_RATE_LIMIT_WINDOW}"
  local strategy="${4:-token_bucket}"

  if [[ -z "$endpoint" ]]; then
    echo "ERROR: Endpoint required" >&2
    return 1
  fi

  # Normalize endpoint (remove query params, trailing slashes)
  endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')

  # Build rate limit key
  local key="endpoint:${endpoint}"

  # Apply rate limit with strategy
  rate_limit_apply "$strategy" "$key" "$max_requests" "$window_seconds"
  return $?
}

# Check rate limit by HTTP method + endpoint
# Usage: endpoint_method_rate_limit_check <method> <endpoint> [max_requests] [window_seconds]
endpoint_method_rate_limit_check() {
  local method="$1"
  local endpoint="$2"
  local max_requests="${3:-200}"
  local window_seconds="${4:-60}"

  if [[ -z "$method" ]] || [[ -z "$endpoint" ]]; then
    echo "ERROR: Method and endpoint required" >&2
    return 1
  fi

  # Normalize
  method=$(echo "$method" | tr '[:lower:]' '[:upper:]')
  endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')

  # Build rate limit key
  local key="endpoint:${method}:${endpoint}"

  # Apply rate limit
  rate_limit_check "$key" "$max_requests" "$window_seconds" "$max_requests"
  return $?
}

# ============================================================================
# Endpoint Rules
# ============================================================================

# Create endpoint rate limit rule
# Usage: endpoint_rule_create <rule_name> <pattern> <max_requests> <window_seconds> [priority]
endpoint_rule_create() {
  local rule_name="$1"
  local pattern="$2"
  local max_requests="$3"
  local window_seconds="$4"
  local priority="${5:-100}"

  if [[ -z "$rule_name" ]] || [[ -z "$pattern" ]] || [[ -z "$max_requests" ]] || [[ -z "$window_seconds" ]]; then
    echo "ERROR: Rule name, pattern, max requests, and window required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape values
  pattern=$(echo "$pattern" | sed "s/'/''/g")

  # Create rule
  local rule_id
  rule_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO rate_limit.rules (name, rule_type, pattern, max_requests, window_seconds, priority)
     VALUES ('$rule_name', 'endpoint', '$pattern', $max_requests, $window_seconds, $priority)
     ON CONFLICT (name) DO UPDATE SET
       pattern = EXCLUDED.pattern,
       max_requests = EXCLUDED.max_requests,
       window_seconds = EXCLUDED.window_seconds,
       priority = EXCLUDED.priority,
       updated_at = NOW()
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$rule_id" ]]; then
    echo "ERROR: Failed to create rule" >&2
    return 1
  fi

  echo "$rule_id"
  return 0
}

# Get matching endpoint rules
# Usage: endpoint_rule_match <endpoint>
endpoint_rule_match() {
  local endpoint="$1"

  if [[ -z "$endpoint" ]]; then
    echo "ERROR: Endpoint required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Normalize endpoint
  endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')

  # Escape endpoint
  endpoint=$(echo "$endpoint" | sed "s/'/''/g")

  # Find matching rules (pattern matching)
  local rules_json
  rules_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(r) FROM (
       SELECT id, name, pattern, max_requests, window_seconds, burst_size, priority
       FROM rate_limit.rules
       WHERE rule_type = 'endpoint'
         AND enabled = TRUE
         AND '$endpoint' ~ pattern
       ORDER BY priority, created_at
     ) r;" \
    2>/dev/null | xargs)

  if [[ -z "$rules_json" ]] || [[ "$rules_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$rules_json"
  return 0
}

# Apply endpoint rules
# Usage: endpoint_rules_apply <endpoint> <client_key>
endpoint_rules_apply() {
  local endpoint="$1"
  local client_key="$2"

  if [[ -z "$endpoint" ]] || [[ -z "$client_key" ]]; then
    echo "ERROR: Endpoint and client key required" >&2
    return 1
  fi

  # Get matching rules
  local rules
  rules=$(endpoint_rule_match "$endpoint")

  if [[ "$rules" == "[]" ]]; then
    # No specific rules, use default
    endpoint_rate_limit_check "$endpoint"
    return $?
  fi

  # Apply first matching rule (highest priority)
  local rule
  rule=$(echo "$rules" | jq -r '.[0]')

  local rule_id
  local max_requests
  local window_seconds
  local burst_size

  rule_id=$(echo "$rule" | jq -r '.id')
  max_requests=$(echo "$rule" | jq -r '.max_requests')
  window_seconds=$(echo "$rule" | jq -r '.window_seconds')
  burst_size=$(echo "$rule" | jq -r '.burst_size // .max_requests')

  # Build composite key
  local key="${client_key}:endpoint:${endpoint}"

  # Apply rate limit
  local tokens_remaining
  tokens_remaining=$(rate_limit_check "$key" "$max_requests" "$window_seconds" "$burst_size")
  local result=$?

  # Log with rule ID
  rate_limit_log "$key" "$([ $result -eq 0 ] && echo true || echo false)" "$tokens_remaining" "$rule_id"

  echo "$tokens_remaining"
  return $result
}

# ============================================================================
# Endpoint Statistics
# ============================================================================

# Get endpoint usage stats
# Usage: endpoint_get_usage <endpoint> [hours]
endpoint_get_usage() {
  local endpoint="$1"
  local hours="${2:-24}"

  if [[ -z "$endpoint" ]]; then
    echo "ERROR: Endpoint required" >&2
    return 1
  fi

  # Normalize endpoint
  endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')

  # Build key
  local key="endpoint:${endpoint}"

  # Get stats
  rate_limit_get_stats "$key" "$hours"
  return $?
}

# Get top rate-limited endpoints
# Usage: endpoint_get_top_limited [limit] [hours]
endpoint_get_top_limited() {
  local limit="${1:-10}"
  local hours="${2:-24}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get top limited endpoints
  local top_json
  top_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(t) FROM (
       SELECT
         key,
         COUNT(*) FILTER (WHERE allowed = false) AS limited_count,
         COUNT(*) AS total_count,
         ROUND(100.0 * COUNT(*) FILTER (WHERE allowed = false) / COUNT(*), 2) AS limited_percent
       FROM rate_limit.log
       WHERE key LIKE 'endpoint:%'
         AND requested_at >= NOW() - INTERVAL '$hours hours'
       GROUP BY key
       ORDER BY limited_count DESC
       LIMIT $limit
     ) t;" \
    2>/dev/null | xargs)

  if [[ -z "$top_json" ]] || [[ "$top_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$top_json"
  return 0
}

# List all endpoint rules
# Usage: endpoint_rule_list
endpoint_rule_list() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get rules
  local rules_json
  rules_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(r) FROM (
       SELECT id, name, pattern, max_requests, window_seconds, burst_size, enabled, priority, created_at
       FROM rate_limit.rules
       WHERE rule_type = 'endpoint'
       ORDER BY priority, name
     ) r;" \
    2>/dev/null | xargs)

  if [[ -z "$rules_json" ]] || [[ "$rules_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$rules_json"
  return 0
}

# Enable/disable endpoint rule
# Usage: endpoint_rule_set_enabled <rule_name> <enabled>
endpoint_rule_set_enabled() {
  local rule_name="$1"
  local enabled="$2"

  if [[ -z "$rule_name" ]]; then
    echo "ERROR: Rule name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Update rule
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE rate_limit.rules SET enabled = $enabled, updated_at = NOW() WHERE name = '$rule_name';" \
    >/dev/null 2>&1

  return $?
}

# Delete endpoint rule
# Usage: endpoint_rule_delete <rule_name>
endpoint_rule_delete() {
  local rule_name="$1"

  if [[ -z "$rule_name" ]]; then
    echo "ERROR: Rule name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete rule
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM rate_limit.rules WHERE name = '$rule_name';" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# Export functions
# ============================================================================

export -f endpoint_rate_limit_check
export -f endpoint_method_rate_limit_check
export -f endpoint_rule_create
export -f endpoint_rule_match
export -f endpoint_rules_apply
export -f endpoint_get_usage
export -f endpoint_get_top_limited
export -f endpoint_rule_list
export -f endpoint_rule_set_enabled
export -f endpoint_rule_delete
