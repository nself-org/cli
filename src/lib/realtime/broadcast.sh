#!/usr/bin/env bash
#
# broadcast.sh - Message broadcasting for realtime system
#
# Manages broadcast messages to channels
#


# Source dependencies
REALTIME_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$REALTIME_LIB_DIR/../utils/output.sh"
source "$REALTIME_LIB_DIR/../utils/docker.sh"

# ============================================================================
# Broadcast Functions
# ============================================================================

#######################################
# Broadcast a message to a channel
# Arguments:
#   $1 - Channel ID or slug
#   $2 - Event type
#   $3 - Payload (JSON)
#   $4 - Sender user ID (optional)
# Returns:
#   0 on success, 1 on failure
#   Outputs message ID on success
#######################################
broadcast_send() {
  local channel="$1"
  local event_type="$2"
  local payload="$3"
  local sender_id="${4:-}"

  if [[ -z "$channel" ]] || [[ -z "$event_type" ]] || [[ -z "$payload" ]]; then
    error "Channel, event type, and payload required"
    return 1
  fi

  # Get channel ID
  local channel_id
  channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

  if [[ -z "$channel_id" ]]; then
    error "Channel not found: $channel"
    return 1
  fi

  # Escape payload for SQL
  local escaped_payload
  escaped_payload=$(printf "%s" "$payload" | sed "s/'/''/g")

  local sender_clause="NULL"
  if [[ -n "$sender_id" ]]; then
    sender_clause="'$sender_id'"
  fi

  local sql="
    INSERT INTO realtime.messages (channel_id, event_type, payload, sender_id, sent_at)
    VALUES ('$channel_id', '$event_type', '$escaped_payload'::jsonb, $sender_clause, NOW())
    RETURNING id;
    "

  local message_id
  if ! message_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" 2>&1 | tr -d ' \n'); then
    error "Failed to broadcast message"
    return 1
  fi

  # Notify channel via PostgreSQL NOTIFY
  local notify_sql="SELECT pg_notify('realtime:channel:$channel_id', '$message_id');"
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$notify_sql" >/dev/null 2>&1 || true

  success "Message broadcast to channel (ID: $message_id)"
  printf "%s\n" "$message_id"
  return 0
}

#######################################
# Get recent messages from a channel
# Arguments:
#   $1 - Channel ID or slug
#   $2 - Limit (default: 50)
#   $3 - Format (table, json) - default: json
# Returns:
#   0 on success, 1 on failure
#######################################
broadcast_get_messages() {
  local channel="$1"
  local limit="${2:-50}"
  local format="${3:-json}"

  if [[ -z "$channel" ]]; then
    error "Channel ID or slug required"
    return 1
  fi

  # Get channel ID
  local channel_id
  channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

  if [[ -z "$channel_id" ]]; then
    error "Channel not found: $channel"
    return 1
  fi

  local sql="
    SELECT
        m.id,
        m.event_type,
        m.payload,
        m.sender_id,
        m.sent_at
    FROM realtime.messages m
    WHERE m.channel_id = '$channel_id'
    ORDER BY m.sent_at DESC
    LIMIT $limit;
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
# Get message replay (messages since timestamp)
# Arguments:
#   $1 - Channel ID or slug
#   $2 - Since timestamp (ISO 8601 or UNIX timestamp)
#   $3 - Format (table, json) - default: json
# Returns:
#   0 on success, 1 on failure
#######################################
broadcast_replay() {
  local channel="$1"
  local since="$2"
  local format="${3:-json}"

  if [[ -z "$channel" ]] || [[ -z "$since" ]]; then
    error "Channel and timestamp required"
    return 1
  fi

  # Get channel ID
  local channel_id
  channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

  if [[ -z "$channel_id" ]]; then
    error "Channel not found: $channel"
    return 1
  fi

  # Convert UNIX timestamp to PostgreSQL timestamp if needed
  local timestamp_clause
  if [[ "$since" =~ ^[0-9]+$ ]]; then
    timestamp_clause="TO_TIMESTAMP($since)"
  else
    timestamp_clause="'$since'::timestamp"
  fi

  local sql="
    SELECT
        m.id,
        m.event_type,
        m.payload,
        m.sender_id,
        m.sent_at
    FROM realtime.messages m
    WHERE m.channel_id = '$channel_id'
      AND m.sent_at >= $timestamp_clause
    ORDER BY m.sent_at ASC;
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
# Delete old broadcast messages
# Arguments:
#   $1 - Retention period in hours (default: 24)
# Returns:
#   0 on success, 1 on failure
#   Outputs number of deleted messages
#######################################
broadcast_cleanup() {
  local retention_hours="${1:-24}"

  info "Cleaning up broadcast messages older than ${retention_hours}h..."

  local sql="
    DELETE FROM realtime.messages
    WHERE sent_at < NOW() - INTERVAL '$retention_hours hours'
    RETURNING id;
    "

  local result
  result=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" 2>&1 | wc -l | tr -d ' ')

  success "Deleted $result old messages"
  printf "%s\n" "$result"
  return 0
}

#######################################
# Get broadcast statistics
# Arguments:
#   $1 - Channel ID or slug (optional - if not provided, gets global stats)
# Returns:
#   0 on success, 1 on failure
#######################################
broadcast_stats() {
  local channel="${1:-}"

  local where_clause=""
  if [[ -n "$channel" ]]; then
    local channel_id
    channel_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM realtime.channels WHERE id::text = '$channel' OR slug = '$channel';" | tr -d ' \n')

    if [[ -z "$channel_id" ]]; then
      error "Channel not found: $channel"
      return 1
    fi
    where_clause="WHERE channel_id = '$channel_id'"
  fi

  local sql="
    SELECT
        COUNT(*) as total_messages,
        COUNT(*) FILTER (WHERE sent_at > NOW() - INTERVAL '1 hour') as messages_last_hour,
        COUNT(*) FILTER (WHERE sent_at > NOW() - INTERVAL '24 hours') as messages_last_day,
        COUNT(DISTINCT event_type) as unique_event_types,
        COUNT(DISTINCT sender_id) as unique_senders
    FROM realtime.messages
    $where_clause;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

#######################################
# Delete a specific message
# Arguments:
#   $1 - Message ID
# Returns:
#   0 on success, 1 on failure
#######################################
broadcast_delete_message() {
  local message_id="$1"

  if [[ -z "$message_id" ]]; then
    error "Message ID required"
    return 1
  fi

  local sql="DELETE FROM realtime.messages WHERE id = '$message_id';"

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    success "Message deleted"
    return 0
  else
    error "Failed to delete message"
    return 1
  fi
}

#######################################
# List event types
# Arguments:
#   $1 - Channel ID or slug (optional)
#   $2 - Time period in hours (default: 24)
# Returns:
#   0 on success, 1 on failure
#######################################
broadcast_list_events() {
  local channel="${1:-}"
  local period="${2:-24}"

  local where_clause="WHERE sent_at > NOW() - INTERVAL '$period hours'"

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
    SELECT
        event_type,
        COUNT(*) as count,
        MAX(sent_at) as last_sent
    FROM realtime.messages
    $where_clause
    GROUP BY event_type
    ORDER BY count DESC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}
