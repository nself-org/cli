#!/usr/bin/env bash
#
# channels.sh - Channel management for realtime system
#
# Manages realtime channels (creation, deletion, membership)
#


# Source dependencies
REALTIME_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$REALTIME_LIB_DIR/../utils/output.sh"
source "$REALTIME_LIB_DIR/../utils/docker.sh"

# ============================================================================
# Channel Management Functions
# ============================================================================

#######################################
# Create a new channel
# Arguments:
#   $1 - Channel name
#   $2 - Channel type (public, private, presence) - default: public
#   $3 - Metadata (JSON) - optional
# Returns:
#   0 on success, 1 on failure
#   Outputs channel ID on success
#######################################
channel_create() {
  local name="$1"
  local type="${2:-public}"
  local metadata="${3:-{}}"

  if [[ -z "$name" ]]; then
    error "Channel name required"
    return 1
  fi

  # Validate type
  if [[ ! "$type" =~ ^(public|private|presence)$ ]]; then
    error "Invalid channel type: $type (must be public, private, or presence)"
    return 1
  fi

  # Generate slug from name
  local slug
  slug=$(printf "%s" "$name" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | sed 's/[^a-z0-9-]//g')

  info "Creating channel: $name (type: $type)"

  # Escape metadata for SQL
  local escaped_metadata
  escaped_metadata=$(printf "%s" "$metadata" | sed "s/'/''/g")

  local sql="
    INSERT INTO realtime.channels (name, slug, type, metadata)
    VALUES ('$name', '$slug', '$type', '$escaped_metadata'::jsonb)
    ON CONFLICT (slug) DO UPDATE SET
        name = EXCLUDED.name,
        type = EXCLUDED.type,
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
    RETURNING id, slug;
    "

  local result
  if ! result=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" 2>&1); then
    error "Failed to create channel: $result"
    return 1
  fi

  local channel_id
  channel_id=$(printf "%s" "$result" | awk '{print $1}' | tr -d ' \n')

  success "Channel created: $slug (ID: $channel_id)"
  printf "%s\n" "$channel_id"
  return 0
}

#######################################
# List all channels
# Arguments:
#   $1 - Format (table, json, csv) - default: table
#   $2 - Filter type (all, public, private, presence) - default: all
# Returns:
#   0 on success, 1 on failure
#######################################
channel_list() {
  local format="${1:-table}"
  local filter="${2:-all}"

  local where_clause=""
  if [[ "$filter" != "all" ]]; then
    where_clause="WHERE c.type = '$filter'"
  fi

  local sql="
    SELECT
        c.id,
        c.slug,
        c.name,
        c.type,
        COUNT(DISTINCT cm.user_id) as members,
        COUNT(DISTINCT p.user_id) as online,
        c.created_at,
        c.updated_at
    FROM realtime.channels c
    LEFT JOIN realtime.channel_members cm ON c.id = cm.channel_id
    LEFT JOIN realtime.presence p ON c.id = p.channel_id AND p.status != 'offline'
    $where_clause
    GROUP BY c.id
    ORDER BY c.created_at DESC;
    "

  case "$format" in
    json)
      docker exec -i "$(docker_get_container_name postgres)" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
        "SELECT json_agg(row_to_json(t)) FROM ($sql) t;"
      ;;
    csv)
      docker exec -i "$(docker_get_container_name postgres)" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" --csv -c "$sql"
      ;;
    *)
      docker exec -i "$(docker_get_container_name postgres)" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
      ;;
  esac
}

#######################################
# Get channel details
# Arguments:
#   $1 - Channel ID or slug
#   $2 - Format (json, table) - default: json
# Returns:
#   0 on success, 1 on failure
#######################################
channel_get() {
  local identifier="$1"
  local format="${2:-json}"

  if [[ -z "$identifier" ]]; then
    error "Channel ID or slug required"
    return 1
  fi

  local sql="
    SELECT
        c.id,
        c.slug,
        c.name,
        c.type,
        c.metadata,
        COUNT(DISTINCT cm.user_id) as total_members,
        COUNT(DISTINCT p.user_id) as online_members,
        c.created_at,
        c.updated_at
    FROM realtime.channels c
    LEFT JOIN realtime.channel_members cm ON c.id = cm.channel_id
    LEFT JOIN realtime.presence p ON c.id = p.channel_id AND p.status != 'offline'
    WHERE c.id::text = '$identifier' OR c.slug = '$identifier'
    GROUP BY c.id;
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
# Delete a channel
# Arguments:
#   $1 - Channel ID or slug
#   $2 - Force delete (skip confirmation) - default: false
# Returns:
#   0 on success, 1 on failure
#######################################
channel_delete() {
  local identifier="$1"
  local force="${2:-false}"

  if [[ -z "$identifier" ]]; then
    error "Channel ID or slug required"
    return 1
  fi

  # Get channel details first
  local channel_info
  if ! channel_info=$(channel_get "$identifier" "json" 2>/dev/null); then
    error "Channel not found: $identifier"
    return 1
  fi

  local channel_name
  channel_name=$(printf "%s" "$channel_info" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)

  # Confirm deletion unless forced
  if [[ "$force" != "true" ]]; then
    printf "Delete channel '%s'? This will remove all members and message history. [y/N] " "$channel_name"
    read -r response
    response=$(echo "$response" | tr '[:upper:]' '[:lower:]')

    if [[ "$response" != "y" ]]; then
      info "Deletion cancelled"
      return 0
    fi
  fi

  info "Deleting channel: $channel_name"

  local sql="DELETE FROM realtime.channels WHERE id::text = '$identifier' OR slug = '$identifier';"

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    success "Channel deleted: $channel_name"
    return 0
  else
    error "Failed to delete channel"
    return 1
  fi
}

#######################################
# Add member to channel
# Arguments:
#   $1 - Channel ID or slug
#   $2 - User ID
#   $3 - Role (member, moderator, admin) - default: member
# Returns:
#   0 on success, 1 on failure
#######################################
channel_add_member() {
  local channel="$1"
  local user_id="$2"
  local role="${3:-member}"

  if [[ -z "$channel" ]] || [[ -z "$user_id" ]]; then
    error "Channel and user_id required"
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
    INSERT INTO realtime.channel_members (channel_id, user_id, role)
    VALUES ('$channel_id', '$user_id', '$role')
    ON CONFLICT (channel_id, user_id) DO UPDATE SET
        role = EXCLUDED.role,
        joined_at = NOW();
    "

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    success "Member added to channel"
    return 0
  else
    error "Failed to add member to channel"
    return 1
  fi
}

#######################################
# Remove member from channel
# Arguments:
#   $1 - Channel ID or slug
#   $2 - User ID
# Returns:
#   0 on success, 1 on failure
#######################################
channel_remove_member() {
  local channel="$1"
  local user_id="$2"

  if [[ -z "$channel" ]] || [[ -z "$user_id" ]]; then
    error "Channel and user_id required"
    return 1
  fi

  local sql="
    DELETE FROM realtime.channel_members cm
    USING realtime.channels c
    WHERE cm.channel_id = c.id
      AND (c.id::text = '$channel' OR c.slug = '$channel')
      AND cm.user_id = '$user_id';
    "

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    success "Member removed from channel"
    return 0
  else
    error "Failed to remove member from channel"
    return 1
  fi
}

#######################################
# List channel members
# Arguments:
#   $1 - Channel ID or slug
#   $2 - Format (table, json) - default: table
# Returns:
#   0 on success, 1 on failure
#######################################
channel_list_members() {
  local channel="$1"
  local format="${2:-table}"

  if [[ -z "$channel" ]]; then
    error "Channel ID or slug required"
    return 1
  fi

  local sql="
    SELECT
        cm.user_id,
        cm.role,
        CASE
            WHEN p.status = 'online' THEN 'online'
            WHEN p.status = 'away' THEN 'away'
            ELSE 'offline'
        END as status,
        cm.joined_at,
        p.last_seen_at
    FROM realtime.channel_members cm
    INNER JOIN realtime.channels c ON cm.channel_id = c.id
    LEFT JOIN realtime.presence p ON cm.user_id = p.user_id AND c.id = p.channel_id
    WHERE c.id::text = '$channel' OR c.slug = '$channel'
    ORDER BY cm.joined_at DESC;
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
