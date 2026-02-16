#!/usr/bin/env bash
# strategies.sh - Rate limit strategies (RATE-002)
# Part of nself v0.6.0 - Phase 1 Sprint 5
#
# Different rate limiting strategies and algorithms


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/core.sh" ]]; then
  source "$SCRIPT_DIR/core.sh"
fi

# ============================================================================
# Strategy Types
# ============================================================================

readonly STRATEGY_TOKEN_BUCKET="token_bucket"
readonly STRATEGY_LEAKY_BUCKET="leaky_bucket"
readonly STRATEGY_FIXED_WINDOW="fixed_window"
readonly STRATEGY_SLIDING_WINDOW="sliding_window"
readonly STRATEGY_SLIDING_LOG="sliding_log"

# ============================================================================
# Token Bucket Strategy
# ============================================================================

# Token bucket rate limiting (allows bursts)
# Usage: strategy_token_bucket <key> <max_requests> <window_seconds> <burst_size>
strategy_token_bucket() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"
  local burst_size="$4"

  # Use core token bucket implementation
  rate_limit_check "$key" "$max_requests" "$window_seconds" "$burst_size"
  return $?
}

# ============================================================================
# Leaky Bucket Strategy
# ============================================================================

# Leaky bucket rate limiting (smooth rate, no bursts)
# Usage: strategy_leaky_bucket <key> <max_requests> <window_seconds>
strategy_leaky_bucket() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"

  # Leaky bucket is similar to token bucket but with burst_size = 1
  # This prevents bursts and enforces smooth rate
  rate_limit_check "$key" "$max_requests" "$window_seconds" 1
  return $?
}

# ============================================================================
# Fixed Window Strategy
# ============================================================================

# Fixed window rate limiting (simple, but has edge case issues)
# Usage: strategy_fixed_window <key> <max_requests> <window_seconds>
strategy_fixed_window() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"

  # Use simple fixed window implementation
  rate_limit_check_simple "$key" "$max_requests" "$window_seconds"
  return $?
}

# ============================================================================
# Sliding Window Strategy
# ============================================================================

# Sliding window rate limiting (more accurate than fixed window)
# Usage: strategy_sliding_window <key> <max_requests> <window_seconds>
strategy_sliding_window() {
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

  # Get current window and previous window counts
  local current_epoch
  local window_start_epoch
  local prev_window_start_epoch

  current_epoch=$(date +%s)
  window_start_epoch=$((current_epoch - (current_epoch % window_seconds)))
  prev_window_start_epoch=$((window_start_epoch - window_seconds))

  # Count requests in current and previous windows
  local current_count
  local prev_count

  current_count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM rate_limit.log
     WHERE key = '$key'
       AND requested_at >= to_timestamp($window_start_epoch);" \
    2>/dev/null | xargs)

  prev_count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM rate_limit.log
     WHERE key = '$key'
       AND requested_at >= to_timestamp($prev_window_start_epoch)
       AND requested_at < to_timestamp($window_start_epoch);" \
    2>/dev/null | xargs)

  if [[ -z "$current_count" ]]; then current_count=0; fi
  if [[ -z "$prev_count" ]]; then prev_count=0; fi

  # Calculate weighted count using sliding window algorithm
  local time_in_window=$((current_epoch - window_start_epoch))
  local window_progress
  window_progress=$(echo "scale=6; $time_in_window / $window_seconds" | bc)

  local weighted_prev
  weighted_prev=$(echo "scale=6; $prev_count * (1 - $window_progress)" | bc)

  local total_count
  total_count=$(echo "scale=0; ($current_count + $weighted_prev) / 1" | bc)

  # Check if over limit
  if [[ $total_count -ge $max_requests ]]; then
    # Over limit
    echo "0"
    rate_limit_log "$key" "false" "0" >/dev/null 2>&1
    return 1
  else
    # Allowed
    local remaining=$((max_requests - total_count - 1))
    echo "$remaining"
    rate_limit_log "$key" "true" "$remaining" >/dev/null 2>&1
    return 0
  fi
}

# ============================================================================
# Sliding Log Strategy
# ============================================================================

# Sliding log rate limiting (most accurate, but more storage intensive)
# Usage: strategy_sliding_log <key> <max_requests> <window_seconds>
strategy_sliding_log() {
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

  # Count all requests in the sliding window
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
    echo "0"
    rate_limit_log "$key" "false" "0" >/dev/null 2>&1
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
# Adaptive Rate Limiting
# ============================================================================

# Adaptive rate limiting (adjusts based on success rate)
# Usage: strategy_adaptive <key> <base_max_requests> <window_seconds> <success_threshold>
strategy_adaptive() {
  local key="$1"
  local base_max_requests="$2"
  local window_seconds="$3"
  local success_threshold="${4:-0.9}" # 90% success rate threshold

  if [[ -z "$key" ]] || [[ -z "$base_max_requests" ]] || [[ -z "$window_seconds" ]]; then
    echo "ERROR: Key, max requests, and window required" >&2
    return 1
  fi

  # Get recent success rate
  local stats
  stats=$(rate_limit_get_stats "$key" 1) # Last hour

  local total
  local allowed
  total=$(echo "$stats" | jq -r '.total_requests // 0')
  allowed=$(echo "$stats" | jq -r '.allowed_requests // 0')

  # Calculate success rate
  local success_rate
  if [[ $total -gt 0 ]]; then
    success_rate=$(echo "scale=2; $allowed / $total" | bc)
  else
    success_rate=1.0
  fi

  # Adjust limit based on success rate
  local adjusted_max
  if (($(echo "$success_rate < $success_threshold" | bc -l))); then
    # Reduce limit if too many failures
    adjusted_max=$(echo "scale=0; ($base_max_requests * $success_rate) / 1" | bc)
    # Minimum 10% of base
    local minimum=$(echo "scale=0; ($base_max_requests * 0.1) / 1" | bc)
    if [[ $adjusted_max -lt $minimum ]]; then
      adjusted_max=$minimum
    fi
  else
    # Use base limit
    adjusted_max=$base_max_requests
  fi

  # Apply token bucket with adjusted limit
  rate_limit_check "$key" "$adjusted_max" "$window_seconds" "$adjusted_max"
  return $?
}

# ============================================================================
# Burst Protection Strategy
# ============================================================================

# Burst protection (detects and blocks sudden traffic spikes)
# Usage: strategy_burst_protection <key> <max_requests> <window_seconds> <burst_multiplier>
strategy_burst_protection() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"
  local burst_multiplier="${4:-2}" # Allow 2x burst temporarily

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

  # Check short-term burst (last 10 seconds)
  local burst_window=10
  local burst_count
  burst_count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM rate_limit.log
     WHERE key = '$key'
       AND requested_at >= NOW() - INTERVAL '$burst_window seconds';" \
    2>/dev/null | xargs)

  if [[ -z "$burst_count" ]]; then
    burst_count=0
  fi

  # Calculate burst threshold
  local burst_threshold
  burst_threshold=$(echo "scale=0; ($max_requests * $burst_multiplier * $burst_window / $window_seconds) / 1" | bc)

  # If burst detected, block immediately
  if [[ $burst_count -gt $burst_threshold ]]; then
    echo "0"
    rate_limit_log "$key" "false" "0" >/dev/null 2>&1
    return 1
  fi

  # Otherwise, use normal token bucket
  rate_limit_check "$key" "$max_requests" "$window_seconds" "$max_requests"
  return $?
}

# ============================================================================
# Strategy Selection
# ============================================================================

# Apply rate limit using specified strategy
# Usage: rate_limit_apply <strategy> <key> <max_requests> <window_seconds> [extra_params...]
rate_limit_apply() {
  local strategy="$1"
  local key="$2"
  local max_requests="$3"
  local window_seconds="$4"
  shift 4 || true

  case "$strategy" in
    "$STRATEGY_TOKEN_BUCKET")
      local burst_size="${1:-$max_requests}"
      strategy_token_bucket "$key" "$max_requests" "$window_seconds" "$burst_size"
      ;;
    "$STRATEGY_LEAKY_BUCKET")
      strategy_leaky_bucket "$key" "$max_requests" "$window_seconds"
      ;;
    "$STRATEGY_FIXED_WINDOW")
      strategy_fixed_window "$key" "$max_requests" "$window_seconds"
      ;;
    "$STRATEGY_SLIDING_WINDOW")
      strategy_sliding_window "$key" "$max_requests" "$window_seconds"
      ;;
    "$STRATEGY_SLIDING_LOG")
      strategy_sliding_log "$key" "$max_requests" "$window_seconds"
      ;;
    adaptive)
      local success_threshold="${1:-0.9}"
      strategy_adaptive "$key" "$max_requests" "$window_seconds" "$success_threshold"
      ;;
    burst_protection)
      local burst_multiplier="${1:-2}"
      strategy_burst_protection "$key" "$max_requests" "$window_seconds" "$burst_multiplier"
      ;;
    *)
      echo "ERROR: Unknown strategy: $strategy" >&2
      echo "Available: token_bucket, leaky_bucket, fixed_window, sliding_window, sliding_log, adaptive, burst_protection" >&2
      return 1
      ;;
  esac
}

# ============================================================================
# Export functions
# ============================================================================

export -f strategy_token_bucket
export -f strategy_leaky_bucket
export -f strategy_fixed_window
export -f strategy_sliding_window
export -f strategy_sliding_log
export -f strategy_adaptive
export -f strategy_burst_protection
export -f rate_limit_apply
