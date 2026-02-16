#!/usr/bin/env bash
# device-manager.sh - Device management and tracking
# Part of nself v0.6.0 - Phase 2


device_init() {

set -euo pipefail

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.devices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL UNIQUE,
  device_name TEXT,
  device_type TEXT,
  os TEXT,
  browser TEXT,
  ip_address INET,
  user_agent TEXT,
  fingerprint TEXT,
  trusted BOOLEAN DEFAULT FALSE,
  last_seen TIMESTAMPTZ DEFAULT NOW(),
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, device_id)
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON auth.devices(user_id);
CREATE INDEX IF NOT EXISTS idx_devices_device_id ON auth.devices(device_id);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON auth.devices(last_seen);
EOSQL
  return 0
}

device_register() {
  local user_id="$1"
  local device_id="$2"
  local device_info="${3:-{}}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  local device_name=$(echo "$device_info" | jq -r '.name // "Unknown Device"')
  local device_type=$(echo "$device_info" | jq -r '.type // "unknown"')
  local os=$(echo "$device_info" | jq -r '.os // ""')
  local browser=$(echo "$device_info" | jq -r '.browser // ""')
  local ip=$(echo "$device_info" | jq -r '.ip // ""')
  local ua=$(echo "$device_info" | jq -r '.user_agent // ""' | sed "s/'/''/g")

  local id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.devices (user_id, device_id, device_name, device_type, os, browser, ip_address, user_agent)
     VALUES ('$user_id', '$device_id', '$device_name', '$device_type', '$os', '$browser', '$ip', '$ua')
     ON CONFLICT (user_id, device_id) DO UPDATE SET
       last_seen = NOW(), ip_address = EXCLUDED.ip_address
     RETURNING id;" 2>/dev/null | xargs)

  echo "$id"
}

device_list_user() {
  local user_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local devices=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(d) FROM (
       SELECT id, device_id, device_name, device_type, os, browser, trusted, last_seen, created_at
       FROM auth.devices WHERE user_id = '$user_id' ORDER BY last_seen DESC
     ) d;" 2>/dev/null | xargs)

  [[ -z "$devices" || "$devices" == "null" ]] && echo "[]" || echo "$devices"
}

device_trust() {
  local device_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.devices SET trusted = TRUE WHERE device_id = '$device_id';" >/dev/null 2>&1
}

device_revoke() {
  local device_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.devices WHERE device_id = '$device_id';" >/dev/null 2>&1
}

device_is_trusted() {
  local user_id="$1"
  local device_id="$2"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local trusted=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT trusted FROM auth.devices WHERE user_id = '$user_id' AND device_id = '$device_id';" 2>/dev/null | xargs)

  [[ "$trusted" == "t" ]] && return 0 || return 1
}

export -f device_init device_register device_list_user device_trust device_revoke device_is_trusted
