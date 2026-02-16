#!/usr/bin/env bash
# rate-limit-distributed.sh - Distributed rate limiting with Redis
# Part of nself v0.7.0 - Sprint 6: RDS-002


# Distributed rate limit check using Redis
redis_rate_limit_check() {

set -euo pipefail

  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"
  local connection_name="${4:-main}"

  # Get Redis connection details
  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    # Fallback to local rate limiting if Redis unavailable
    source "$(dirname "${BASH_SOURCE[0]}")/../rate-limit/core.sh"
    rate_limit_check "$key" "$max_requests" "$window_seconds"
    return $?
  }

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    # Fallback to local
    source "$(dirname "${BASH_SOURCE[0]}")/../rate-limit/core.sh"
    rate_limit_check "$key" "$max_requests" "$window_seconds"
    return $?
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  # Lua script for atomic rate limiting (token bucket algorithm)
  local lua_script='
    local key = KEYS[1]
    local max_requests = tonumber(ARGV[1])
    local window = tonumber(ARGV[2])
    local now = tonumber(ARGV[3])

    local current = redis.call("GET", key)
    if current == false then
      redis.call("SET", key, max_requests - 1, "EX", window)
      return 1
    end

    current = tonumber(current)
    if current > 0 then
      redis.call("DECR", key)
      return 1
    end

    return 0
  '

  local rate_key="ratelimit:$key"
  local now=$(date +%s)

  # Execute Lua script
  local result=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    --eval <(echo "$lua_script") "$rate_key" , "$max_requests" "$window_seconds" "$now" 2>/dev/null)

  if [[ "$result" == "1" ]]; then
    return 0 # Allowed
  else
    return 1 # Rate limited
  fi
}

# Distributed sliding window rate limit
redis_rate_limit_sliding_window() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"
  local connection_name="${4:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  # Lua script for sliding window rate limiting
  local lua_script='
    local key = KEYS[1]
    local max_requests = tonumber(ARGV[1])
    local window = tonumber(ARGV[2])
    local now = tonumber(ARGV[3])
    local request_id = ARGV[4]

    -- Remove old entries outside the window
    redis.call("ZREMRANGEBYSCORE", key, 0, now - window)

    -- Count requests in current window
    local count = redis.call("ZCARD", key)

    if count < max_requests then
      redis.call("ZADD", key, now, request_id)
      redis.call("EXPIRE", key, window)
      return 1
    end

    return 0
  '

  local rate_key="ratelimit:sliding:$key"
  local now=$(date +%s)
  local request_id="$(date +%s%N)"

  local result=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    --eval <(echo "$lua_script") "$rate_key" , "$max_requests" "$window_seconds" "$now" "$request_id" 2>/dev/null)

  [[ "$result" == "1" ]] && return 0 || return 1
}

# Get distributed rate limit stats
redis_rate_limit_stats() {
  local key="$1"
  local window_seconds="${2:-60}"
  local connection_name="${3:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "{}"
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "{}"
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local rate_key="ratelimit:$key"
  local now=$(date +%s)

  # Get remaining requests
  local remaining=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    GET "$rate_key" 2>/dev/null || echo "0")

  # Get TTL
  local ttl=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    TTL "$rate_key" 2>/dev/null || echo "0")

  echo "{\"key\":\"$key\",\"remaining\":$remaining,\"ttl\":$ttl,\"window\":$window_seconds}"
}

# Reset distributed rate limit
redis_rate_limit_reset() {
  local key="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local rate_key="ratelimit:$key"
  local sliding_key="ratelimit:sliding:$key"

  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    DEL "$rate_key" "$sliding_key" >/dev/null 2>&1
}

# Distributed rate limit with burst support
redis_rate_limit_burst() {
  local key="$1"
  local max_requests="$2"
  local burst_size="$3"
  local window_seconds="$4"
  local connection_name="${5:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  # Lua script for burst protection
  local lua_script='
    local key = KEYS[1]
    local burst_key = KEYS[2]
    local max_requests = tonumber(ARGV[1])
    local burst_size = tonumber(ARGV[2])
    local window = tonumber(ARGV[3])
    local now = tonumber(ARGV[4])

    -- Check burst limit (1 second window)
    local burst_count = redis.call("GET", burst_key)
    if burst_count == false then
      redis.call("SET", burst_key, 1, "EX", 1)
    else
      burst_count = tonumber(burst_count)
      if burst_count >= burst_size then
        return 0  -- Burst limit exceeded
      end
      redis.call("INCR", burst_key)
    end

    -- Check normal rate limit
    local current = redis.call("GET", key)
    if current == false then
      redis.call("SET", key, max_requests - 1, "EX", window)
      return 1
    end

    current = tonumber(current)
    if current > 0 then
      redis.call("DECR", key)
      return 1
    end

    return 0
  '

  local rate_key="ratelimit:$key"
  local burst_key="ratelimit:burst:$key"
  local now=$(date +%s)

  local result=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    --eval <(echo "$lua_script") "$rate_key" "$burst_key" , "$max_requests" "$burst_size" "$window_seconds" "$now" 2>/dev/null)

  [[ "$result" == "1" ]] && return 0 || return 1
}

# Cluster-wide rate limiting (consistent hashing)
redis_rate_limit_cluster() {
  local key="$1"
  local max_requests="$2"
  local window_seconds="$3"
  local cluster_nodes="${4:-1}"

  # Use consistent hashing to determine which node handles this key
  local hash=$(echo -n "$key" | md5sum | cut -d' ' -f1)
  local node_id=$((0x${hash:0:8} % cluster_nodes))

  local connection_name="node_$node_id"

  # Try to use specific node, fallback to main
  redis_rate_limit_check "$key" "$max_requests" "$window_seconds" "$connection_name" 2>/dev/null ||
    redis_rate_limit_check "$key" "$max_requests" "$window_seconds" "main"
}

export -f redis_rate_limit_check redis_rate_limit_sliding_window redis_rate_limit_stats
export -f redis_rate_limit_reset redis_rate_limit_burst redis_rate_limit_cluster
