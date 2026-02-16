#!/usr/bin/env bash
# ip-limiter.sh - IP-based rate limiting (RATE-004)
# Part of nself v0.6.0 - Phase 1 Sprint 5
#
# Rate limiting by IP address


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/core.sh" ]]; then
  source "$SCRIPT_DIR/core.sh"
fi
if [[ -f "$SCRIPT_DIR/strategies.sh" ]]; then
  source "$SCRIPT_DIR/strategies.sh"
fi

# IP rate limit defaults
readonly IP_RATE_LIMIT_MAX=100   # 100 requests
readonly IP_RATE_LIMIT_WINDOW=60 # per minute

# ============================================================================
# IP Rate Limiting
# ============================================================================

# Check rate limit for IP address
# Usage: ip_rate_limit_check <ip_address> [max_requests] [window_seconds] [strategy]
ip_rate_limit_check() {
  local ip_address="$1"
  local max_requests="${2:-$IP_RATE_LIMIT_MAX}"
  local window_seconds="${3:-$IP_RATE_LIMIT_WINDOW}"
  local strategy="${4:-token_bucket}"

  if [[ -z "$ip_address" ]]; then
    echo "ERROR: IP address required" >&2
    return 1
  fi

  # Build rate limit key
  local key="ip:${ip_address}"

  # Apply rate limit with strategy
  rate_limit_apply "$strategy" "$key" "$max_requests" "$window_seconds"
  return $?
}

# Check rate limit for IP + endpoint
# Usage: ip_endpoint_rate_limit_check <ip_address> <endpoint> [max_requests] [window_seconds]
ip_endpoint_rate_limit_check() {
  local ip_address="$1"
  local endpoint="$2"
  local max_requests="${3:-50}"
  local window_seconds="${4:-60}"

  if [[ -z "$ip_address" ]] || [[ -z "$endpoint" ]]; then
    echo "ERROR: IP address and endpoint required" >&2
    return 1
  fi

  # Normalize endpoint (remove query params, trailing slashes)
  endpoint=$(echo "$endpoint" | sed 's/\?.*$//' | sed 's:/*$::')

  # Build rate limit key
  local key="ip:${ip_address}:endpoint:${endpoint}"

  # Apply rate limit
  rate_limit_check "$key" "$max_requests" "$window_seconds" "$max_requests"
  return $?
}

# ============================================================================
# IP Whitelist/Bypass
# ============================================================================

# Check if IP is whitelisted
# Usage: ip_is_whitelisted <ip_address>
ip_is_whitelisted() {
  local ip_address="$1"

  if [[ -z "$ip_address" ]]; then
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 1
  fi

  # Check whitelist table
  local count
  count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM rate_limit.whitelist
     WHERE ip_address = '$ip_address' AND enabled = TRUE;" \
    2>/dev/null | xargs)

  if [[ -z "$count" ]] || [[ $count -eq 0 ]]; then
    return 1 # Not whitelisted
  fi

  return 0 # Whitelisted
}

# Add IP to whitelist
# Usage: ip_whitelist_add <ip_address> [description]
ip_whitelist_add() {
  local ip_address="$1"
  local description="${2:-}"

  if [[ -z "$ip_address" ]]; then
    echo "ERROR: IP address required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create whitelist table if needed
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS rate_limit.whitelist (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ip_address INET NOT NULL UNIQUE,
  description TEXT,
  enabled BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_whitelist_ip ON rate_limit.whitelist(ip_address);
EOSQL

  # Escape description
  description=$(echo "$description" | sed "s/'/''/g")

  # Add to whitelist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO rate_limit.whitelist (ip_address, description)
     VALUES ('$ip_address', '$description')
     ON CONFLICT (ip_address) DO UPDATE SET
       enabled = TRUE,
       description = EXCLUDED.description;" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to add IP to whitelist" >&2
    return 1
  fi

  return 0
}

# Remove IP from whitelist
# Usage: ip_whitelist_remove <ip_address>
ip_whitelist_remove() {
  local ip_address="$1"

  if [[ -z "$ip_address" ]]; then
    echo "ERROR: IP address required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Remove from whitelist (soft delete)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE rate_limit.whitelist SET enabled = FALSE WHERE ip_address = '$ip_address';" \
    >/dev/null 2>&1

  return 0
}

# List whitelisted IPs
# Usage: ip_whitelist_list
ip_whitelist_list() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get whitelist
  local whitelist_json
  whitelist_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(w) FROM (
       SELECT ip_address, description, enabled, created_at
       FROM rate_limit.whitelist
       ORDER BY created_at DESC
     ) w;" \
    2>/dev/null | xargs)

  if [[ -z "$whitelist_json" ]] || [[ "$whitelist_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$whitelist_json"
  return 0
}

# ============================================================================
# IP Blocking
# ============================================================================

# Block IP address
# Usage: ip_block <ip_address> [reason] [duration_seconds]
ip_block() {
  local ip_address="$1"
  local reason="${2:-Exceeded rate limit}"
  local duration_seconds="${3:-3600}" # 1 hour default

  if [[ -z "$ip_address" ]]; then
    echo "ERROR: IP address required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create blocklist table if needed
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS rate_limit.blocklist (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ip_address INET NOT NULL,
  reason TEXT,
  blocked_at TIMESTAMPTZ DEFAULT NOW(),
  expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_blocklist_ip ON rate_limit.blocklist(ip_address);
CREATE INDEX IF NOT EXISTS idx_blocklist_expires ON rate_limit.blocklist(expires_at);
EOSQL

  # Escape reason
  reason=$(echo "$reason" | sed "s/'/''/g")

  # Calculate expiry
  local expires_at
  expires_at=$(date -u -d "+${duration_seconds} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${duration_seconds}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Add to blocklist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO rate_limit.blocklist (ip_address, reason, expires_at)
     VALUES ('$ip_address', '$reason', '$expires_at'::timestamptz);" \
    >/dev/null 2>&1

  return 0
}

# Check if IP is blocked
# Usage: ip_is_blocked <ip_address>
ip_is_blocked() {
  local ip_address="$1"

  if [[ -z "$ip_address" ]]; then
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 1
  fi

  # Check blocklist (only active blocks)
  local count
  count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM rate_limit.blocklist
     WHERE ip_address = '$ip_address'
       AND (expires_at IS NULL OR expires_at > NOW());" \
    2>/dev/null | xargs)

  if [[ -z "$count" ]] || [[ $count -eq 0 ]]; then
    return 1 # Not blocked
  fi

  return 0 # Blocked
}

# Unblock IP address
# Usage: ip_unblock <ip_address>
ip_unblock() {
  local ip_address="$1"

  if [[ -z "$ip_address" ]]; then
    echo "ERROR: IP address required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Remove from blocklist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM rate_limit.blocklist WHERE ip_address = '$ip_address';" \
    >/dev/null 2>&1

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f ip_rate_limit_check
export -f ip_endpoint_rate_limit_check
export -f ip_is_whitelisted
export -f ip_whitelist_add
export -f ip_whitelist_remove
export -f ip_whitelist_list
export -f ip_block
export -f ip_is_blocked
export -f ip_unblock
