#!/usr/bin/env bash
# core.sh - Rate limiting core (RATE-001)
# Part of nself v0.6.0 - Phase 1 Sprint 5
#
# Core rate limiting functionality


# Rate limit defaults
readonly RATE_LIMIT_WINDOW=60        # 60 seconds (1 minute)

set -euo pipefail

readonly RATE_LIMIT_MAX_REQUESTS=100 # 100 requests per window
readonly RATE_LIMIT_BURST=20         # Allow burst of 20 requests

# ============================================================================
# Rate Limiter Initialization
# ============================================================================

# Initialize rate limiter
# Usage: rate_limit_init
rate_limit_init() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create rate_limit schema and tables
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS rate_limit;

-- Rate limit buckets (token bucket algorithm)
CREATE TABLE IF NOT EXISTS rate_limit.buckets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL UNIQUE,
  tokens FLOAT NOT NULL DEFAULT 0,
  capacity FLOAT NOT NULL,
  refill_rate FLOAT NOT NULL,
  last_refill TIMESTAMPTZ DEFAULT NOW(),
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_buckets_key ON rate_limit.buckets(key);
CREATE INDEX IF NOT EXISTS idx_buckets_last_refill ON rate_limit.buckets(last_refill);

-- Rate limit rules
CREATE TABLE IF NOT EXISTS rate_limit.rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  rule_type TEXT NOT NULL,
  pattern TEXT NOT NULL,
  max_requests INTEGER NOT NULL,
  window_seconds INTEGER NOT NULL,
  burst_size INTEGER,
  enabled BOOLEAN DEFAULT TRUE,
  priority INTEGER DEFAULT 100,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rules_rule_type ON rate_limit.rules(rule_type);
CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rate_limit.rules(enabled);
CREATE INDEX IF NOT EXISTS idx_rules_priority ON rate_limit.rules(priority);

-- Rate limit log (for monitoring)
CREATE TABLE IF NOT EXISTS rate_limit.log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL,
  rule_id UUID,
  allowed BOOLEAN NOT NULL,
  tokens_remaining FLOAT,
  requested_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_log_key ON rate_limit.log(key);
CREATE INDEX IF NOT EXISTS idx_log_requested_at ON rate_limit.log(requested_at);
CREATE INDEX IF NOT EXISTS idx_log_allowed ON rate_limit.log(allowed);
EOSQL

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to initialize rate limiter" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# Token Bucket Algorithm
# ============================================================================

# Check rate limit using token bucket algorithm
# Usage: rate_limit_check <key> <max_requests> <window_seconds> [burst_size]
# Returns: 0 if allowed, 1 if limited
rate_limit_check() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"
  local burst_size="${4:-$max_requests}"

  if [[ -z "$key" ]] || [[ -z "$max_requests" ]] || [[ -z "$window_seconds" ]]; then
    echo "ERROR: Key, max requests, and window required" >&2
    return 1
  fi

  # Calculate refill rate (tokens per second)
  local refill_rate
  refill_rate=$(echo "scale=6; $max_requests / $window_seconds" | bc)

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape key
  key=$(echo "$key" | sed "s/'/''/g")

  # Get or create bucket
  local bucket_json
  bucket_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO rate_limit.buckets (key, tokens, capacity, refill_rate)
     VALUES ('$key', $burst_size, $burst_size, $refill_rate)
     ON CONFLICT (key) DO UPDATE SET
       capacity = EXCLUDED.capacity,
       refill_rate = EXCLUDED.refill_rate
     RETURNING row_to_json(buckets.*);" \
    2>/dev/null | xargs)

  if [[ -z "$bucket_json" ]] || [[ "$bucket_json" == "null" ]]; then
    echo "ERROR: Failed to get bucket" >&2
    return 1
  fi

  # Extract bucket data
  local tokens
  local last_refill
  tokens=$(echo "$bucket_json" | jq -r '.tokens')
  last_refill=$(echo "$bucket_json" | jq -r '.last_refill')

  # Calculate elapsed time and refill tokens
  local now_epoch
  local last_refill_epoch
  now_epoch=$(date +%s)
  last_refill_epoch=$(date -d "$last_refill" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "$last_refill" +%s 2>/dev/null)

  local elapsed=$((now_epoch - last_refill_epoch))
  local refilled
  refilled=$(echo "scale=6; $tokens + ($refill_rate * $elapsed)" | bc)

  # Cap at burst size
  if (($(echo "$refilled > $burst_size" | bc -l))); then
    refilled=$burst_size
  fi

  # Check if we have tokens
  local allowed=false
  local new_tokens=$refilled

  if (($(echo "$refilled >= 1" | bc -l))); then
    allowed=true
    new_tokens=$(echo "scale=6; $refilled - 1" | bc)
  fi

  # Update bucket
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE rate_limit.buckets
     SET tokens = $new_tokens,
         last_refill = NOW()
     WHERE key = '$key';" \
    >/dev/null 2>&1

  # Log the attempt
  rate_limit_log "$key" "$allowed" "$new_tokens" >/dev/null 2>&1

  # Return result
  if [[ "$allowed" == "true" ]]; then
    # Output remaining tokens for rate limit headers
    echo "$new_tokens"
    return 0
  else
    # Output 0 tokens
    echo "0"
    return 1
  fi
}

# Simple rate limit check (fixed window)
# Usage: rate_limit_check_simple <key> <max_requests> <window_seconds>
# Returns: 0 if allowed, 1 if limited
rate_limit_check_simple() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"

  if [[ -z "$key" ]] || [[ -z "$max_requests" ]] || [[ -z "$window_seconds" ]]; then
    echo "ERROR: Key, max requests, and window required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape key
  key=$(echo "$key" | sed "s/'/''/g")

  # Count requests in window
  local count
  count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM rate_limit.log
     WHERE key = '$key'
       AND requested_at >= NOW() - INTERVAL '$window_seconds seconds';" \
    2>/dev/null | xargs)

  if [[ -z "$count" ]]; then
    count=0
  fi

  # Check if over limit
  if [[ $count -ge $max_requests ]]; then
    # Over limit
    local remaining=0
    echo "$remaining"
    rate_limit_log "$key" "false" "$remaining" >/dev/null 2>&1
    return 1
  else
    # Allowed
    local remaining=$((max_requests - count - 1))
    echo "$remaining"
    rate_limit_log "$key" "true" "$remaining" >/dev/null 2>&1
    return 0
  fi
}

# ============================================================================
# Rate Limit Logging
# ============================================================================

# Log rate limit attempt
# Usage: rate_limit_log <key> <allowed> <tokens_remaining> [rule_id]
rate_limit_log() {
  local key="$1"
  local allowed="$2"
  local tokens_remaining="$3"
  local rule_id="${4:-}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 0 # Fail silently for logging
  fi

  # Escape key
  key=$(echo "$key" | sed "s/'/''/g")

  # Build rule_id SQL
  local rule_id_sql="NULL"
  if [[ -n "$rule_id" ]]; then
    rule_id_sql="'$rule_id'"
  fi

  # Insert log entry
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO rate_limit.log (key, rule_id, allowed, tokens_remaining)
     VALUES ('$key', $rule_id_sql, $allowed, $tokens_remaining);" \
    >/dev/null 2>&1

  return 0
}

# Get rate limit stats
# Usage: rate_limit_get_stats <key> [hours]
rate_limit_get_stats() {
  local key="$1"
  local hours="${2:-24}"

  if [[ -z "$key" ]]; then
    echo "ERROR: Key required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape key
  key=$(echo "$key" | sed "s/'/''/g")

  # Get stats
  local stats_json
  stats_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         '$key' AS key,
         COUNT(*) AS total_requests,
         SUM(CASE WHEN allowed = true THEN 1 ELSE 0 END) AS allowed_requests,
         SUM(CASE WHEN allowed = false THEN 1 ELSE 0 END) AS limited_requests,
         AVG(tokens_remaining) AS avg_tokens_remaining,
         MIN(requested_at) AS first_request,
         MAX(requested_at) AS last_request
       FROM rate_limit.log
       WHERE key = '$key'
         AND requested_at >= NOW() - INTERVAL '$hours hours'
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$stats_json" ]] || [[ "$stats_json" == "null" ]]; then
    echo "{\"key\": \"$key\", \"total_requests\": 0}"
    return 0
  fi

  echo "$stats_json"
  return 0
}

# ============================================================================
# Rate Limit Cleanup
# ============================================================================

# Clean old rate limit logs
# Usage: rate_limit_cleanup [days_to_keep]
rate_limit_cleanup() {
  local days_to_keep="${1:-7}"

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
       DELETE FROM rate_limit.log
       WHERE requested_at < NOW() - INTERVAL '$days_to_keep days'
       RETURNING id
     )
     SELECT COUNT(*) FROM deleted;" \
    2>/dev/null | xargs)

  echo "Deleted $deleted old rate limit logs" >&2
  return 0
}

# Reset rate limit for a key
# Usage: rate_limit_reset <key>
rate_limit_reset() {
  local key="$1"

  if [[ -z "$key" ]]; then
    echo "ERROR: Key required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape key
  key=$(echo "$key" | sed "s/'/''/g")

  # Reset bucket to full capacity
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE rate_limit.buckets
     SET tokens = capacity,
         last_refill = NOW()
     WHERE key = '$key';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to reset rate limit" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# Rate Limit Headers
# ============================================================================

# Generate rate limit headers
# Usage: rate_limit_headers <max_requests> <window_seconds> <tokens_remaining>
rate_limit_headers() {
  local max_requests="$1"
  local window_seconds="$2"
  local tokens_remaining="$3"

  # Calculate reset time (current time + window)
  local reset_time
  reset_time=$(($(date +%s) + window_seconds))

  # Output headers
  cat <<EOF
{
  "X-RateLimit-Limit": "$max_requests",
  "X-RateLimit-Remaining": "$tokens_remaining",
  "X-RateLimit-Reset": "$reset_time",
  "X-RateLimit-Window": "$window_seconds"
}
EOF

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f rate_limit_init
export -f rate_limit_check
export -f rate_limit_check_simple
export -f rate_limit_log
export -f rate_limit_get_stats
export -f rate_limit_cleanup
export -f rate_limit_reset
export -f rate_limit_headers
