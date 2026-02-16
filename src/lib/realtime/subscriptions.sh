#!/usr/bin/env bash
#
# subscriptions.sh - Database subscriptions (Change Data Capture)
#
# Manages database table subscriptions using PostgreSQL LISTEN/NOTIFY
#


# Source dependencies
REALTIME_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$REALTIME_LIB_DIR/../utils/output.sh"
source "$REALTIME_LIB_DIR/../utils/docker.sh"

# ============================================================================
# Subscription Functions
# ============================================================================

#######################################
# Subscribe to table changes
# Arguments:
#   $1 - Table name (schema.table format)
#   $2 - Events (INSERT, UPDATE, DELETE - comma-separated, default: all)
#   $3 - Filter (WHERE clause, optional)
# Returns:
#   0 on success, 1 on failure
#   Outputs subscription ID on success
#######################################
subscribe_table() {
  local table="$1"
  local events="${2:-INSERT,UPDATE,DELETE}"
  local filter="${3:-}"

  if [[ -z "$table" ]]; then
    error "Table name required"
    return 1
  fi

  # Validate table format
  if [[ ! "$table" =~ ^[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
    error "Invalid table format: $table (use schema.table)"
    return 1
  fi

  local schema
  local table_name
  schema=$(echo "$table" | cut -d'.' -f1)
  table_name=$(echo "$table" | cut -d'.' -f2)

  # Convert events to array
  local events_upper
  events_upper=$(echo "$events" | tr '[:lower:]' '[:upper:]')

  info "Creating subscription for table: $table"
  info "Events: $events_upper"
  if [[ -n "$filter" ]]; then
    info "Filter: $filter"
  fi

  # Escape filter for SQL
  local escaped_filter=""
  if [[ -n "$filter" ]]; then
    escaped_filter=$(printf "%s" "$filter" | sed "s/'/''/g")
  fi

  # Create trigger function if not exists
  local sql="
    -- Create trigger function
    CREATE OR REPLACE FUNCTION realtime.notify_${schema}_${table_name}()
    RETURNS trigger AS \$\$
    DECLARE
        payload jsonb;
        channel text;
    BEGIN
        channel := 'realtime:table:${schema}.${table_name}';

        -- Build payload
        payload := jsonb_build_object(
            'timestamp', EXTRACT(EPOCH FROM NOW()),
            'operation', TG_OP,
            'schema', TG_TABLE_SCHEMA,
            'table', TG_TABLE_NAME
        );

        -- Add old and new data
        IF TG_OP = 'DELETE' THEN
            payload := payload || jsonb_build_object('old', row_to_json(OLD));
        ELSIF TG_OP = 'UPDATE' THEN
            payload := payload || jsonb_build_object(
                'old', row_to_json(OLD),
                'new', row_to_json(NEW)
            );
        ELSE
            payload := payload || jsonb_build_object('new', row_to_json(NEW));
        END IF;

        -- Notify
        PERFORM pg_notify(channel, payload::text);

        RETURN NULL;
    END;
    \$\$ LANGUAGE plpgsql;

    -- Record subscription
    INSERT INTO realtime.subscriptions (table_name, events, filter, created_at)
    VALUES ('${schema}.${table_name}', ARRAY['$(echo "$events_upper" | sed "s/,/','/g")'], $(if [[ -n "$escaped_filter" ]]; then echo "'$escaped_filter'"; else echo "NULL"; fi), NOW())
    ON CONFLICT (table_name) DO UPDATE SET
        events = EXCLUDED.events,
        filter = EXCLUDED.filter,
        updated_at = NOW()
    RETURNING id;
    "

  local sub_id
  if ! sub_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" 2>&1 | tr -d ' \n'); then
    error "Failed to create subscription"
    return 1
  fi

  # Create triggers for each event type
  IFS=',' read -ra EVENT_ARRAY <<<"$events_upper"
  for event in "${EVENT_ARRAY[@]}"; do
    event=$(echo "$event" | tr -d ' ')
    local trigger_sql="
        DROP TRIGGER IF EXISTS realtime_${event}_${table_name} ON ${schema}.${table_name};
        CREATE TRIGGER realtime_${event}_${table_name}
        AFTER $event ON ${schema}.${table_name}
        FOR EACH ROW
        EXECUTE FUNCTION realtime.notify_${schema}_${table_name}();
        "

    if ! docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$trigger_sql" >/dev/null 2>&1; then
      warn "Failed to create $event trigger"
    fi
  done

  success "Subscription created (ID: $sub_id)"
  printf "%s\n" "$sub_id"
  return 0
}

#######################################
# Unsubscribe from table changes
# Arguments:
#   $1 - Table name (schema.table format) or subscription ID
# Returns:
#   0 on success, 1 on failure
#######################################
unsubscribe_table() {
  local identifier="$1"

  if [[ -z "$identifier" ]]; then
    error "Table name or subscription ID required"
    return 1
  fi

  # Get table name
  local table_name
  if [[ "$identifier" =~ ^[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
    table_name="$identifier"
  else
    table_name=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT table_name FROM realtime.subscriptions WHERE id = '$identifier';" | tr -d ' ')

    if [[ -z "$table_name" ]]; then
      error "Subscription not found: $identifier"
      return 1
    fi
  fi

  local schema
  local tbl
  schema=$(echo "$table_name" | cut -d'.' -f1)
  tbl=$(echo "$table_name" | cut -d'.' -f2)

  info "Removing subscription for: $table_name"

  # Drop triggers
  local trigger_sql="
    DROP TRIGGER IF EXISTS realtime_INSERT_${tbl} ON ${schema}.${tbl};
    DROP TRIGGER IF EXISTS realtime_UPDATE_${tbl} ON ${schema}.${tbl};
    DROP TRIGGER IF EXISTS realtime_DELETE_${tbl} ON ${schema}.${tbl};
    DROP FUNCTION IF EXISTS realtime.notify_${schema}_${tbl}();
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$trigger_sql" >/dev/null 2>&1 || true

  # Delete subscription record
  local sql="DELETE FROM realtime.subscriptions WHERE table_name = '$table_name';"

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    success "Subscription removed"
    return 0
  else
    error "Failed to remove subscription"
    return 1
  fi
}

#######################################
# List all subscriptions
# Arguments:
#   $1 - Format (table, json) - default: table
# Returns:
#   0 on success, 1 on failure
#######################################
list_subscriptions() {
  local format="${1:-table}"

  local sql="
    SELECT
        id,
        table_name,
        events,
        filter,
        created_at,
        updated_at
    FROM realtime.subscriptions
    ORDER BY created_at DESC;
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
# Test subscription by triggering a change
# Arguments:
#   $1 - Table name (schema.table format)
# Returns:
#   0 on success, 1 on failure
#######################################
test_subscription() {
  local table="$1"

  if [[ -z "$table" ]]; then
    error "Table name required"
    return 1
  fi

  info "Testing subscription for: $table"
  info "This will attempt to query the table..."

  local sql="SELECT COUNT(*) FROM $table LIMIT 1;"

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1; then
    success "Subscription test successful"
    info "Monitor logs to see NOTIFY events"
    return 0
  else
    error "Failed to test subscription"
    return 1
  fi
}

#######################################
# Listen to table changes (blocking)
# Arguments:
#   $1 - Table name (schema.table format)
#   $2 - Duration in seconds (optional, default: infinite)
# Returns:
#   0 on success, 1 on failure
#######################################
listen_table() {
  local table="$1"
  local duration="${2:-0}"

  if [[ -z "$table" ]]; then
    error "Table name required"
    return 1
  fi

  local channel="realtime:table:$table"

  info "Listening to changes on: $table"
  info "Channel: $channel"
  info "Press Ctrl+C to stop"
  printf "\n"

  local listen_sql="LISTEN \"$channel\";"

  if [[ $duration -gt 0 ]]; then
    info "Will listen for $duration seconds..."
    if command -v timeout >/dev/null 2>&1; then
      timeout "$duration" docker exec -i "$(docker_get_container_name postgres)" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$listen_sql; SELECT pg_sleep(999999);" 2>&1 || true
    else
      docker exec -i "$(docker_get_container_name postgres)" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$listen_sql; SELECT pg_sleep($duration);" 2>&1 || true
    fi
  else
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$listen_sql; SELECT pg_sleep(999999);" 2>&1 || true
  fi

  return 0
}

#######################################
# Get subscription statistics
# Arguments:
#   None
# Returns:
#   0 on success, 1 on failure
#######################################
subscription_stats() {
  local sql="
    SELECT
        COUNT(*) as total_subscriptions,
        COUNT(DISTINCT table_name) as unique_tables,
        COUNT(*) FILTER (WHERE 'INSERT' = ANY(events)) as insert_subs,
        COUNT(*) FILTER (WHERE 'UPDATE' = ANY(events)) as update_subs,
        COUNT(*) FILTER (WHERE 'DELETE' = ANY(events)) as delete_subs,
        COUNT(*) FILTER (WHERE filter IS NOT NULL) as filtered_subs
    FROM realtime.subscriptions;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}
