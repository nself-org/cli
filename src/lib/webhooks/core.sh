#!/usr/bin/env bash
# core.sh - Webhook system core
# Part of nself v0.6.0 - Phase 2
#
# Webhook delivery and management system


# Webhook defaults
readonly WEBHOOK_TIMEOUT=30

set -euo pipefail

readonly WEBHOOK_MAX_RETRIES=3
readonly WEBHOOK_RETRY_DELAY=60

# ============================================================================
# Webhook Initialization
# ============================================================================

# Initialize webhook system
# Usage: webhook_init
webhook_init() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create webhooks schema and tables
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS webhooks;

-- Webhook endpoints
CREATE TABLE IF NOT EXISTS webhooks.endpoints (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  url TEXT NOT NULL,
  description TEXT,
  secret TEXT NOT NULL,
  enabled BOOLEAN DEFAULT TRUE,
  events TEXT[] NOT NULL,
  headers JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_endpoints_enabled ON webhooks.endpoints(enabled);
CREATE INDEX IF NOT EXISTS idx_endpoints_events ON webhooks.endpoints USING GIN(events);

-- Webhook deliveries
CREATE TABLE IF NOT EXISTS webhooks.deliveries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  endpoint_id UUID NOT NULL REFERENCES webhooks.endpoints(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL,
  response_code INTEGER,
  response_body TEXT,
  error_message TEXT,
  attempt INTEGER DEFAULT 1,
  max_attempts INTEGER DEFAULT 3,
  delivered_at TIMESTAMPTZ,
  next_retry_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deliveries_endpoint ON webhooks.deliveries(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_event ON webhooks.deliveries(event_type);
CREATE INDEX IF NOT EXISTS idx_deliveries_status ON webhooks.deliveries(status);
CREATE INDEX IF NOT EXISTS idx_deliveries_retry ON webhooks.deliveries(next_retry_at) WHERE status = 'pending';
EOSQL

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to initialize webhook system" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# Endpoint Management
# ============================================================================

# Create webhook endpoint
# Usage: webhook_create_endpoint <url> <events_json> [description] [secret]
webhook_create_endpoint() {
  local url="$1"
  local events_json="$2"
  local description="${3:-}"
  local secret="${4:-}"

  if [[ -z "$url" ]] || [[ -z "$events_json" ]]; then
    echo "ERROR: URL and events required" >&2
    return 1
  fi

  # Generate secret if not provided
  if [[ -z "$secret" ]]; then
    secret=$(openssl rand -hex 32)
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Convert JSON array to PostgreSQL array
  local events_array
  events_array=$(echo "$events_json" | jq -r 'map("\"" + . + "\"") | join(",")')
  events_array="{${events_array}}"

  # Escape values
  url=$(echo "$url" | sed "s/'/''/g")
  description=$(echo "$description" | sed "s/'/''/g")

  # Create endpoint
  local endpoint_id
  endpoint_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO webhooks.endpoints (url, description, secret, events)
     VALUES ('$url', '$description', '$secret', '$events_array')
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$endpoint_id" ]]; then
    echo "ERROR: Failed to create webhook endpoint" >&2
    return 1
  fi

  # Return endpoint info
  jq -n \
    --arg id "$endpoint_id" \
    --arg url "$url" \
    --arg secret "$secret" \
    --argjson events "$events_json" \
    '{id: $id, url: $url, secret: $secret, events: $events}'

  return 0
}

# List webhook endpoints
# Usage: webhook_list_endpoints
webhook_list_endpoints() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get endpoints
  local endpoints_json
  endpoints_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(e) FROM (
       SELECT id, url, description, enabled, events, created_at
       FROM webhooks.endpoints
       ORDER BY created_at DESC
     ) e;" \
    2>/dev/null | xargs)

  if [[ -z "$endpoints_json" ]] || [[ "$endpoints_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$endpoints_json"
  return 0
}

# Delete webhook endpoint
# Usage: webhook_delete_endpoint <endpoint_id>
webhook_delete_endpoint() {
  local endpoint_id="$1"

  if [[ -z "$endpoint_id" ]]; then
    echo "ERROR: Endpoint ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete endpoint
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM webhooks.endpoints WHERE id = '$endpoint_id';" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# Webhook Delivery
# ============================================================================

# Trigger webhook for event
# Usage: webhook_trigger <event_type> <payload_json>
webhook_trigger() {
  local event_type="$1"
  local payload_json="$2"

  if [[ -z "$event_type" ]] || [[ -z "$payload_json" ]]; then
    echo "ERROR: Event type and payload required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Find endpoints subscribed to this event
  local endpoints
  endpoints=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -A -F'|' -c \
    "SELECT id, url, secret, headers
     FROM webhooks.endpoints
     WHERE enabled = TRUE AND '$event_type' = ANY(events);" \
    2>/dev/null)

  if [[ -z "$endpoints" ]]; then
    # No endpoints for this event
    return 0
  fi

  # Escape payload for SQL
  local escaped_payload
  escaped_payload=$(echo "$payload_json" | sed "s/'/''/g")

  # Create delivery for each endpoint
  while IFS='|' read -r endpoint_id url secret headers_json; do
    # Create delivery record
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "INSERT INTO webhooks.deliveries (endpoint_id, event_type, payload, status)
       VALUES ('$endpoint_id', '$event_type', '$escaped_payload'::jsonb, 'pending');" \
      >/dev/null 2>&1

    # Deliver webhook in background
    webhook_deliver_async "$endpoint_id" "$url" "$secret" "$event_type" "$payload_json" "$headers_json" &
  done <<<"$endpoints"

  return 0
}

# Deliver webhook (async)
# Usage: webhook_deliver_async <endpoint_id> <url> <secret> <event_type> <payload_json> [headers_json]
webhook_deliver_async() {
  local endpoint_id="$1"
  local url="$2"
  local secret="$3"
  local event_type="$4"
  local payload_json="$5"
  local headers_json="${6:-{}}"

  # Generate webhook signature
  local signature
  signature=$(echo -n "$payload_json" | openssl dgst -sha256 -hmac "$secret" | cut -d' ' -f2)

  # Build headers
  local curl_headers=(
    -H "Content-Type: application/json"
    -H "X-Webhook-Event: $event_type"
    -H "X-Webhook-Signature: sha256=$signature"
    -H "X-Webhook-Timestamp: $(date -u +%s)"
    -H "User-Agent: nself-webhooks/1.0"
  )

  # Add custom headers
  if [[ "$headers_json" != "{}" ]]; then
    local header_count
    header_count=$(echo "$headers_json" | jq 'length')
    for ((i = 0; i < header_count; i++)); do
      local key
      local value
      key=$(echo "$headers_json" | jq -r "keys[$i]")
      value=$(echo "$headers_json" | jq -r ".\"$key\"")
      curl_headers+=(-H "$key: $value")
    done
  fi

  # Deliver webhook
  local response
  local http_code
  response=$(curl -s -w "\n%{http_code}" -X POST \
    "${curl_headers[@]}" \
    --max-time "$WEBHOOK_TIMEOUT" \
    -d "$payload_json" \
    "$url" 2>&1)

  http_code=$(echo "$response" | tail -1)
  local response_body=$(echo "$response" | head -n -1)

  # Update delivery status
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ "$http_code" -ge 200 ]] && [[ "$http_code" -lt 300 ]]; then
    # Success
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE webhooks.deliveries
       SET status = 'delivered',
           response_code = $http_code,
           delivered_at = NOW()
       WHERE endpoint_id = '$endpoint_id' AND event_type = '$event_type' AND status = 'pending'
       ORDER BY created_at DESC LIMIT 1;" \
      >/dev/null 2>&1
  else
    # Failed - mark for retry
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE webhooks.deliveries
       SET status = 'pending',
           response_code = $http_code,
           error_message = '${response_body//\'/\'\'}',
           attempt = attempt + 1,
           next_retry_at = NOW() + INTERVAL '$WEBHOOK_RETRY_DELAY seconds'
       WHERE endpoint_id = '$endpoint_id' AND event_type = '$event_type' AND status = 'pending'
         AND attempt < max_attempts
       ORDER BY created_at DESC LIMIT 1;" \
      >/dev/null 2>&1
  fi

  return 0
}

# ============================================================================
# Webhook Events
# ============================================================================

# Common webhook events
readonly WEBHOOK_EVENT_USER_CREATED="user.created"
readonly WEBHOOK_EVENT_USER_UPDATED="user.updated"
readonly WEBHOOK_EVENT_USER_DELETED="user.deleted"
readonly WEBHOOK_EVENT_USER_LOGIN="user.login"
readonly WEBHOOK_EVENT_USER_LOGOUT="user.logout"
readonly WEBHOOK_EVENT_SESSION_CREATED="session.created"
readonly WEBHOOK_EVENT_SESSION_REVOKED="session.revoked"
readonly WEBHOOK_EVENT_MFA_ENABLED="mfa.enabled"
readonly WEBHOOK_EVENT_MFA_DISABLED="mfa.disabled"
readonly WEBHOOK_EVENT_ROLE_ASSIGNED="role.assigned"
readonly WEBHOOK_EVENT_ROLE_REVOKED="role.revoked"

# ============================================================================
# Export functions
# ============================================================================

export -f webhook_init
export -f webhook_create_endpoint
export -f webhook_list_endpoints
export -f webhook_delete_endpoint
export -f webhook_trigger
export -f webhook_deliver_async
