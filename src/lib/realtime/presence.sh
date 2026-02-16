#!/usr/bin/env bash
#
# presence.sh - Presence tracking for realtime system
#
# Manages user presence status and tracking
#


# Source dependencies
REALTIME_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$REALTIME_LIB_DIR/../utils/output.sh"
source "$REALTIME_LIB_DIR/../utils/docker.sh"

# ============================================================================
# Presence Management Functions
# ============================================================================

#######################################
# Track user presence
# Arguments:
#   $1 - User ID
#   $2 - Channel ID or slug
#   $3 - Status (online, away, offline) - default: online
#   $4 - Metadata (JSON) - optional
# Returns:
#   0 on success, 1 on failure
#######################################
presence_track() {
  local user_id="$1"
  local channel="${2:-}"
  local status="${3:-online}"
  local metadata="${4:-{}}"

  if [[ -z "$user_id" ]]; then
    error "User ID required"
    return 1
  fi

  # Validate status
  if [[ ! "$status" =~ ^(online|away|offline)$ ]]; then
    error "Invalid status: $status (must be online, away, or offline)"
    return 1
  fi

  local channel_id="NULL"
  if [[ -n "$channel" ]]; then
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    channel_id="'$channel_id'"
  fi

  # Escape metadata for SQL
  local escaped_metadata
  escaped_metadata=$(printf "%s" "$metadata" | sed "s/'/''/g")

  local sql="
    INSERT INTO realtime.presence (user_id, channel_id, status, metadata, last_seen_at)
    VALUES ('$user_id', $channel_id, '$status', '$escaped_metadata'::jsonb, NOW())
    ON CONFLICT (user_id, COALESCE(channel_id, '00000000-0000-0000-0000-000000000000'::uuid))
    DO UPDATE SET
        status = EXCLUDED.status,
        metadata = EXCLUDED.metadata,
        last_seen_at = NOW();
    "

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    return 0
  else
    error "Failed to track presence"
    return 1
  fi
}

#######################################
# Get user presence
# Arguments:
#   $1 - User ID
#   $2 - Channel ID or slug (optional - if not provided, gets global presence)
#   $3 - Format (json, table) - default: json
# Returns:
#   0 on success, 1 on failure
#######################################
presence_get() {
  local user_id="$1"
  local channel="${2:-}"
  local format="${3:-json}"

  if [[ -z "$user_id" ]]; then
    error "User ID required"
    return 1
  fi

  local where_clause="p.user_id = '$user_id'"

  if [[ -n "$channel" ]]; then
    local channel_id
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    where_clause="$where_clause AND p.channel_id = '$channel_id'"
  fi

  local sql="
    SELECT
        p.user_id,
        c.slug as channel,
        p.status,
        p.metadata,
        p.last_seen_at,
        EXTRACT(EPOCH FROM (NOW() - p.last_seen_at))::int as seconds_since_seen
    FROM realtime.presence p
    LEFT JOIN realtime.channels c ON p.channel_id = c.id
    WHERE $where_clause;
    "

  if [[ "$format" == "json" ]]; then
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT row_to_json(t) FROM ($sql) t;"
  else
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
  fi
}

#######################################
# List all online users
# Arguments:
#   $1 - Channel ID or slug (optional)
#   $2 - Format (table, json) - default: table
# Returns:
#   0 on success, 1 on failure
#######################################
presence_list_online() {
  local channel="${1:-}"
  local format="${2:-table}"

  local where_clause="p.status IN ('online', 'away')"

  if [[ -n "$channel" ]]; then
    local channel_id
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    where_clause="$where_clause AND p.channel_id = '$channel_id'"
  fi

  local sql="
    SELECT
        p.user_id,
        c.slug as channel,
        p.status,
        p.metadata,
        p.last_seen_at,
        EXTRACT(EPOCH FROM (NOW() - p.last_seen_at))::int as seconds_ago
    FROM realtime.presence p
    LEFT JOIN realtime.channels c ON p.channel_id = c.id
    WHERE $where_clause
    ORDER BY p.last_seen_at DESC;
    "

  if [[ "$format" == "json" ]]; then
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT json_agg(row_to_json(t)) FROM ($sql) t;"
  else
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
  fi
}

#######################################
# Count online users
# Arguments:
#   $1 - Channel ID or slug (optional)
# Returns:
#   0 on success, 1 on failure
#   Outputs count on success
#######################################
presence_count_online() {
  local channel="${1:-}"

  local where_clause="status IN ('online', 'away')"

  if [[ -n "$channel" ]]; then
    local channel_id
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    where_clause="$where_clause AND channel_id = '$channel_id'"
  fi

  local count
  count=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT COUNT(*) FROM realtime.presence WHERE $where_clause;" | tr -d ' \n')

  printf "%s\n" "$count"
  return 0
}

#######################################
# Set user offline
# Arguments:
#   $1 - User ID
#   $2 - Channel ID or slug (optional - if not provided, sets all channels offline)
# Returns:
#   0 on success, 1 on failure
#######################################
presence_set_offline() {
  local user_id="$1"
  local channel="${2:-}"

  if [[ -z "$user_id" ]]; then
    error "User ID required"
    return 1
  fi

  local where_clause="user_id = '$user_id'"

  if [[ -n "$channel" ]]; then
    local channel_id
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    where_clause="$where_clause AND channel_id = '$channel_id'"
  fi

  local sql="
    UPDATE realtime.presence
    SET status = 'offline', last_seen_at = NOW()
    WHERE $where_clause;
    "

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    return 0
  else
    error "Failed to set user offline"
    return 1
  fi
}

#######################################
# Cleanup stale presence records
# Arguments:
#   $1 - Timeout in seconds (default: 300 = 5 minutes)
# Returns:
#   0 on success, 1 on failure
#   Outputs number of cleaned records
#######################################
presence_cleanup() {
  local timeout="${1:-300}"

  info "Cleaning up presence records older than ${timeout}s..."

  local sql="
    UPDATE realtime.presence
    SET status = 'offline'
    WHERE status != 'offline'
      AND EXTRACT(EPOCH FROM (NOW() - last_seen_at)) > $timeout
    RETURNING user_id;
    "

  local result
  result=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" 2>&1 | wc -l | tr -d ' ')

  success "Cleaned up $result stale presence records"
  printf "%s\n" "$result"
  return 0
}

#######################################
# Get presence statistics
# Arguments:
#   None
# Returns:
#   0 on success, 1 on failure
#######################################
presence_stats() {
  local sql="
    SELECT
        COUNT(*) FILTER (WHERE status = 'online') as online,
        COUNT(*) FILTER (WHERE status = 'away') as away,
        COUNT(*) FILTER (WHERE status = 'offline') as offline,
        COUNT(DISTINCT user_id) as total_users,
        COUNT(DISTINCT channel_id) as total_channels
    FROM realtime.presence;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

#######################################
# Update presence metadata
# Arguments:
#   $1 - User ID
#   $2 - Channel ID or slug (optional)
#   $3 - Metadata (JSON)
# Returns:
#   0 on success, 1 on failure
#######################################
presence_update_metadata() {
  local user_id="$1"
  local channel="${2:-}"
  local metadata="$3"

  if [[ -z "$user_id" ]] || [[ -z "$metadata" ]]; then
    error "User ID and metadata required"
    return 1
  fi

  local where_clause="user_id = '$user_id'"

  if [[ -n "$channel" ]]; then
    local channel_id
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    where_clause="$where_clause AND channel_id = '$channel_id'"
  fi

  # Escape metadata for SQL
  local escaped_metadata
  escaped_metadata=$(printf "%s" "$metadata" | sed "s/'/''/g")

  local sql="
    UPDATE realtime.presence
    SET metadata = '$escaped_metadata'::jsonb, last_seen_at = NOW()
    WHERE $where_clause;
    "

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    return 0
  else
    error "Failed to update presence metadata"
    return 1
  fi
}
