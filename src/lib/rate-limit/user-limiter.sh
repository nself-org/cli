#!/usr/bin/env bash
# user-limiter.sh - User-based rate limiting (RATE-005)
# Part of nself v0.6.0 - Phase 1 Sprint 5
#
# Rate limiting by user ID


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/core.sh" ]]; then
  source "$SCRIPT_DIR/core.sh"
fi
if [[ -f "$SCRIPT_DIR/strategies.sh" ]]; then
  source "$SCRIPT_DIR/strategies.sh"
fi

# User rate limit defaults
readonly USER_RATE_LIMIT_MAX=1000    # 1000 requests
readonly USER_RATE_LIMIT_WINDOW=3600 # per hour

# ============================================================================
# User Rate Limiting
# ============================================================================

# Check rate limit for user
# Usage: user_rate_limit_check <user_id> [max_requests] [window_seconds] [strategy]
user_rate_limit_check() {
  local user_id="$1"
  local max_requests="${2:-$USER_RATE_LIMIT_MAX}"
  local window_seconds="${3:-$USER_RATE_LIMIT_WINDOW}"
  local strategy="${4:-token_bucket}"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Build rate limit key
  local key="user:${user_id}"

  # Apply rate limit with strategy
  rate_limit_apply "$strategy" "$key" "$max_requests" "$window_seconds"
  return $?
}

# Check rate limit for user + endpoint
# Usage: user_endpoint_rate_limit_check <user_id> <endpoint> [max_requests] [window_seconds]
user_endpoint_rate_limit_check() {
  local user_id="$1"
  local endpoint="$2"
  local max_requests="${3:-100}"
  local window_seconds="${4:-3600}"

  if [[ -z "$user_id" ]] || [[ -z "$endpoint" ]]; then
    echo "ERROR: User ID and endpoint required" >&2
    return 1
  fi

  # Normalize endpoint
  endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')

  # Build rate limit key
  local key="user:${user_id}:endpoint:${endpoint}"

  # Apply rate limit
  rate_limit_check "$key" "$max_requests" "$window_seconds" "$max_requests"
  return $?
}

# ============================================================================
# User Tier-Based Rate Limiting
# ============================================================================

# Check rate limit based on user tier
# Usage: user_tier_rate_limit_check <user_id> <tier> [endpoint]
user_tier_rate_limit_check() {
  local user_id="$1"
  local tier="$2"
  local endpoint="${3:-}"

  if [[ -z "$user_id" ]] || [[ -z "$tier" ]]; then
    echo "ERROR: User ID and tier required" >&2
    return 1
  fi

  # Define tier limits
  local max_requests
  local window_seconds

  case "$tier" in
    free)
      max_requests=100
      window_seconds=3600 # 100 req/hour
      ;;
    basic)
      max_requests=1000
      window_seconds=3600 # 1000 req/hour
      ;;
    pro)
      max_requests=10000
      window_seconds=3600 # 10000 req/hour
      ;;
    enterprise)
      max_requests=100000
      window_seconds=3600 # 100000 req/hour
      ;;
    unlimited)
      # No limit
      echo "999999"
      return 0
      ;;
    *)
      echo "ERROR: Unknown tier: $tier" >&2
      return 1
      ;;
  esac

  # Build key
  local key="user:${user_id}:tier:${tier}"
  if [[ -n "$endpoint" ]]; then
    endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')
    key="${key}:endpoint:${endpoint}"
  fi

  # Apply rate limit
  rate_limit_check "$key" "$max_requests" "$window_seconds" "$max_requests"
  return $?
}

# ============================================================================
# User Quota Management
# ============================================================================

# Set user quota
# Usage: user_quota_set <user_id> <max_requests> <window_seconds>
user_quota_set() {
  local user_id="$1"
  local max_requests="$2"
  local window_seconds="$3"

  if [[ -z "$user_id" ]] || [[ -z "$max_requests" ]] || [[ -z "$window_seconds" ]]; then
    echo "ERROR: User ID, max requests, and window required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create user_quotas table if needed
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS rate_limit.user_quotas (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL UNIQUE,
  max_requests INTEGER NOT NULL,
  window_seconds INTEGER NOT NULL,
  tier TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_quotas_user_id ON rate_limit.user_quotas(user_id);
EOSQL

  # Set quota
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO rate_limit.user_quotas (user_id, max_requests, window_seconds)
     VALUES ('$user_id', $max_requests, $window_seconds)
     ON CONFLICT (user_id) DO UPDATE SET
       max_requests = EXCLUDED.max_requests,
       window_seconds = EXCLUDED.window_seconds,
       updated_at = NOW();" \
    >/dev/null 2>&1

  return $?
}

# Get user quota
# Usage: user_quota_get <user_id>
user_quota_get() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get quota
  local quota_json
  quota_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(q) FROM (
       SELECT user_id, max_requests, window_seconds, tier, created_at, updated_at
       FROM rate_limit.user_quotas
       WHERE user_id = '$user_id'
     ) q;" \
    2>/dev/null | xargs)

  if [[ -z "$quota_json" ]] || [[ "$quota_json" == "null" ]]; then
    echo "{}"
    return 0
  fi

  echo "$quota_json"
  return 0
}

# Get user usage stats
# Usage: user_get_usage <user_id> [hours]
user_get_usage() {
  local user_id="$1"
  local hours="${2:-24}"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Build key
  local key="user:${user_id}"

  # Get stats
  rate_limit_get_stats "$key" "$hours"
  return $?
}

# Reset user rate limit
# Usage: user_rate_limit_reset <user_id>
user_rate_limit_reset() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Build key
  local key="user:${user_id}"

  # Reset
  rate_limit_reset "$key"
  return $?
}

# ============================================================================
# Export functions
# ============================================================================

export -f user_rate_limit_check
export -f user_endpoint_rate_limit_check
export -f user_tier_rate_limit_check
export -f user_quota_set
export -f user_quota_get
export -f user_get_usage
export -f user_rate_limit_reset
