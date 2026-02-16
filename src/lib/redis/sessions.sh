#!/usr/bin/env bash
# sessions.sh - Distributed session management with Redis
# Part of nself v0.7.0 - Sprint 6: RDS-003


# Default session TTL (30 days)
readonly SESSION_DEFAULT_TTL=2592000

set -euo pipefail


# Create session in Redis
redis_session_create() {
  local session_id="$1"
  local user_id="$2"
  local session_data="${3:-{}}"
  local ttl="${4:-$SESSION_DEFAULT_TTL}"
  local connection_name="${5:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "ERROR: Redis not available" >&2
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "ERROR: Connection not found" >&2
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  # Create session data with metadata
  local now=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  local expires_at=$(date -u -d "+$ttl seconds" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v "+${ttl}S" +"%Y-%m-%dT%H:%M:%SZ")

  local session_json=$(jq -n \
    --arg sid "$session_id" \
    --arg uid "$user_id" \
    --arg created "$now" \
    --arg expires "$expires_at" \
    --argjson data "$session_data" \
    '{
      session_id: $sid,
      user_id: $uid,
      created_at: $created,
      last_accessed_at: $created,
      expires_at: $expires,
      data: $data
    }')

  local session_key="session:$session_id"

  # Store in Redis with TTL
  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SET "$session_key" "$session_json" EX "$ttl" >/dev/null 2>&1

  # Add to user's session set
  local user_sessions_key="user:$user_id:sessions"
  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SADD "$user_sessions_key" "$session_id" >/dev/null 2>&1

  echo "$session_id"
}

# Get session from Redis
redis_session_get() {
  local session_id="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "null"
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "null"
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local session_key="session:$session_id"

  local session=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    GET "$session_key" 2>/dev/null)

  [[ -z "$session" || "$session" == "null" ]] && {
    echo "null"
    return 1
  }

  # Update last accessed timestamp
  local now=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  local updated_session=$(echo "$session" | jq --arg now "$now" '.last_accessed_at = $now')

  # Get current TTL and reset it
  local ttl=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    TTL "$session_key" 2>/dev/null || echo "$SESSION_DEFAULT_TTL")

  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SET "$session_key" "$updated_session" EX "$ttl" >/dev/null 2>&1

  echo "$updated_session"
}

# Update session data
redis_session_update() {
  local session_id="$1"
  local updates="$2"
  local connection_name="${3:-main}"

  local session=$(redis_session_get "$session_id" "$connection_name")
  [[ "$session" == "null" ]] && {
    echo "ERROR: Session not found" >&2
    return 1
  }

  # Merge updates into session data
  local updated_session=$(echo "$session" | jq --argjson updates "$updates" '.data += $updates')

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local session_key="session:$session_id"

  # Get current TTL
  local ttl=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    TTL "$session_key" 2>/dev/null || echo "$SESSION_DEFAULT_TTL")

  # Update session
  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SET "$session_key" "$updated_session" EX "$ttl" >/dev/null 2>&1

  echo "$updated_session"
}

# Delete session from Redis
redis_session_delete() {
  local session_id="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && return 1

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && return 1

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  # Get session to find user_id
  local session=$(redis_session_get "$session_id" "$connection_name" 2>/dev/null)
  if [[ "$session" != "null" ]]; then
    local user_id=$(echo "$session" | jq -r '.user_id')
    local user_sessions_key="user:$user_id:sessions"

    # Remove from user's session set
    docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
      SREM "$user_sessions_key" "$session_id" >/dev/null 2>&1
  fi

  # Delete session
  local session_key="session:$session_id"
  docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    DEL "$session_key" >/dev/null 2>&1
}

# List user sessions
redis_session_list_user() {
  local user_id="$1"
  local connection_name="${2:-main}"

  local redis_container=$(docker ps --filter 'name=redis' --format '{{.Names}}' | head -1)
  [[ -z "$redis_container" ]] && {
    echo "[]"
    return 1
  }

  local conn=$(redis_connection_get "$connection_name" 2>/dev/null)
  [[ -z "$conn" || "$conn" == "null" ]] && {
    echo "[]"
    return 1
  }

  local host=$(echo "$conn" | jq -r '.host')
  local port=$(echo "$conn" | jq -r '.port')
  local database=$(echo "$conn" | jq -r '.database')

  local user_sessions_key="user:$user_id:sessions"

  # Get all session IDs for user
  local session_ids=$(docker exec "$redis_container" redis-cli -h "$host" -p "$port" -n "$database" \
    SMEMBERS "$user_sessions_key" 2>/dev/null)

  [[ -z "$session_ids" ]] && {
    echo "[]"
    return 0
  }

  # Fetch each session
  local sessions="[]"
  while IFS= read -r sid; do
    [[ -z "$sid" ]] && continue
    local session=$(redis_session_get "$sid" "$connection_name" 2>/dev/null)
    [[ "$session" != "null" ]] && sessions=$(echo "$sessions" | jq --argjson s "$session" '. += [$s]')
  done <<<"$session_ids"

  echo "$sessions"
}

# Revoke all user sessions
redis_session_revoke_user() {
  local user_id="$1"
  local connection_name="${2:-main}"

  local sessions=$(redis_session_list_user "$user_id" "$connection_name")
  local count=0

  echo "$sessions" | jq -r '.[].session_id' | while read -r sid; do
    redis_session_delete "$sid" "$connection_name"
    count=$((count + 1))
  done

  echo "$count"
}

# Session replication across nodes
redis_session_replicate() {
  local session_id="$1"
  local source_connection="${2:-main}"
  local target_connections="${3:-}" # Comma-separated list

  # Get session from source
  local session=$(redis_session_get "$session_id" "$source_connection" 2>/dev/null)
  [[ "$session" == "null" ]] && {
    echo "ERROR: Session not found" >&2
    return 1
  }

  local user_id=$(echo "$session" | jq -r '.user_id')
  local session_data=$(echo "$session" | jq -r '.data')
  local ttl=$(echo "$session" | jq -r '.expires_at')

  # Calculate remaining TTL
  local now=$(date +%s)
  local expires=$(date -d "$ttl" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%SZ" "$ttl" +%s)
  local remaining_ttl=$((expires - now))

  [[ $remaining_ttl -lt 0 ]] && {
    echo "ERROR: Session expired" >&2
    return 1
  }

  # Replicate to target connections
  IFS=',' read -ra targets <<<"$target_connections"
  for target in "${targets[@]}"; do
    [[ -z "$target" ]] && continue
    redis_session_create "$session_id" "$user_id" "$session_data" "$remaining_ttl" "$target" >/dev/null 2>&1
  done

  echo "Replicated to ${#targets[@]} nodes"
}

# Automatic failover - find session in any connection
redis_session_find() {
  local session_id="$1"

  # Try all configured connections
  local connections=$(redis_connection_list | jq -r '.[].name')

  while IFS= read -r conn; do
    [[ -z "$conn" ]] && continue
    local session=$(redis_session_get "$session_id" "$conn" 2>/dev/null)
    if [[ "$session" != "null" ]]; then
      echo "$session"
      return 0
    fi
  done <<<"$connections"

  echo "null"
  return 1
}

export -f redis_session_create redis_session_get redis_session_update redis_session_delete
export -f redis_session_list_user redis_session_revoke_user redis_session_replicate redis_session_find
