#!/usr/bin/env bash
# user-metadata.sh - User metadata system (USER-004)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements flexible key-value metadata storage for users
# Supports versioning and JSON data types


# ============================================================================
# Metadata Storage
# ============================================================================

# Set user metadata
# Usage: metadata_set <user_id> <key> <value> [value_type]
# value_type: string (default), number, boolean, json
metadata_set() {

set -euo pipefail

  local user_id="$1"
  local key="$2"
  local value="$3"
  local value_type="${4:-string}"

  if [[ -z "$user_id" ]] || [[ -z "$key" ]]; then
    echo "ERROR: User ID and key required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create user_metadata table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.user_metadata (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  key TEXT NOT NULL,
  value TEXT,
  value_type TEXT DEFAULT 'string',
  version INTEGER DEFAULT 1,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, key)
);
CREATE INDEX IF NOT EXISTS idx_user_metadata_user_id ON auth.user_metadata(user_id);
CREATE INDEX IF NOT EXISTS idx_user_metadata_key ON auth.user_metadata(key);

-- Metadata history table for versioning
CREATE TABLE IF NOT EXISTS auth.user_metadata_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  metadata_id UUID REFERENCES auth.user_metadata(id) ON DELETE CASCADE,
  user_id UUID,
  key TEXT,
  value TEXT,
  value_type TEXT,
  version INTEGER,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_metadata_history_metadata_id ON auth.user_metadata_history(metadata_id);
EOSQL

  # Escape single quotes
  value=$(echo "$value" | sed "s/'/''/g")

  # Insert or update metadata
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<EOSQL >/dev/null 2>&1
-- Save current version to history if exists
INSERT INTO auth.user_metadata_history (metadata_id, user_id, key, value, value_type, version, created_at)
SELECT id, user_id, key, value, value_type, version, updated_at
FROM auth.user_metadata
WHERE user_id = '$user_id' AND key = '$key';

-- Insert or update metadata
INSERT INTO auth.user_metadata (user_id, key, value, value_type, version, updated_at)
VALUES ('$user_id', '$key', '$value', '$value_type', 1, NOW())
ON CONFLICT (user_id, key) DO UPDATE SET
  value = EXCLUDED.value,
  value_type = EXCLUDED.value_type,
  version = user_metadata.version + 1,
  updated_at = NOW();
EOSQL

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to set metadata" >&2
    return 1
  fi

  echo "✓ Metadata key '$key' updated" >&2
  return 0
}

# Get user metadata
# Usage: metadata_get <user_id> <key>
# Returns: Metadata value
metadata_get() {
  local user_id="$1"
  local key="$2"

  if [[ -z "$user_id" ]] || [[ -z "$key" ]]; then
    echo "ERROR: User ID and key required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get metadata value
  local value
  value=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT value FROM auth.user_metadata
     WHERE user_id = '$user_id' AND key = '$key'
     LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$value" ]]; then
    return 1
  fi

  echo "$value"
  return 0
}

# Get all user metadata
# Usage: metadata_get_all <user_id>
# Returns: JSON object with all metadata
metadata_get_all() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get all metadata as JSON
  local metadata_json
  metadata_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_object_agg(key, value)
     FROM auth.user_metadata
     WHERE user_id = '$user_id';" \
    2>/dev/null | xargs)

  if [[ -z "$metadata_json" ]] || [[ "$metadata_json" == "null" ]]; then
    echo "{}"
    return 0
  fi

  echo "$metadata_json"
  return 0
}

# Delete user metadata
# Usage: metadata_delete <user_id> <key>
metadata_delete() {
  local user_id="$1"
  local key="$2"

  if [[ -z "$user_id" ]] || [[ -z "$key" ]]; then
    echo "ERROR: User ID and key required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete metadata
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.user_metadata
     WHERE user_id = '$user_id' AND key = '$key';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to delete metadata" >&2
    return 1
  fi

  echo "✓ Metadata key '$key' deleted" >&2
  return 0
}

# ============================================================================
# Bulk Metadata Operations
# ============================================================================

# Set multiple metadata keys at once
# Usage: metadata_set_bulk <user_id> <json_metadata>
# Example: metadata_set_bulk "uuid" '{"key1": "value1", "key2": "value2"}'
metadata_set_bulk() {
  local user_id="$1"
  local json_metadata="$2"

  if [[ -z "$user_id" ]] || [[ -z "$json_metadata" ]]; then
    echo "ERROR: User ID and JSON metadata required" >&2
    return 1
  fi

  # Get all keys from JSON
  local keys
  keys=$(echo "$json_metadata" | jq -r 'keys[]' 2>/dev/null || echo "")

  if [[ -z "$keys" ]]; then
    echo "ERROR: No metadata keys found" >&2
    return 1
  fi

  # Set each key
  local count=0
  while IFS= read -r key; do
    if [[ -n "$key" ]]; then
      local value
      value=$(echo "$json_metadata" | jq -r ".$key")

      if metadata_set "$user_id" "$key" "$value" "string" 2>/dev/null; then
        ((count++))
      fi
    fi
  done <<<"$keys"

  echo "✓ Set $count metadata keys" >&2
  return 0
}

# Delete all metadata for user
# Usage: metadata_delete_all <user_id>
metadata_delete_all() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete all metadata
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.user_metadata WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to delete metadata" >&2
    return 1
  fi

  echo "✓ All metadata deleted" >&2
  return 0
}

# ============================================================================
# Metadata Versioning
# ============================================================================

# Get metadata version history
# Usage: metadata_get_history <user_id> <key> [limit]
# Returns: JSON array of historical versions
metadata_get_history() {
  local user_id="$1"
  local key="$2"
  local limit="${3:-10}"

  if [[ -z "$user_id" ]] || [[ -z "$key" ]]; then
    echo "ERROR: User ID and key required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get metadata ID first
  local metadata_id
  metadata_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM auth.user_metadata
     WHERE user_id = '$user_id' AND key = '$key'
     LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$metadata_id" ]]; then
    echo "[]"
    return 0
  fi

  # Get history
  local history_json
  history_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(h) FROM (
       SELECT
         key,
         value,
         value_type,
         version,
         created_at
       FROM auth.user_metadata_history
       WHERE metadata_id = '$metadata_id'
       ORDER BY version DESC
       LIMIT $limit
     ) h;" \
    2>/dev/null | xargs)

  if [[ -z "$history_json" ]] || [[ "$history_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$history_json"
  return 0
}

# Restore metadata to previous version
# Usage: metadata_restore_version <user_id> <key> <version>
metadata_restore_version() {
  local user_id="$1"
  local key="$2"
  local version="$3"

  if [[ -z "$user_id" ]] || [[ -z "$key" ]] || [[ -z "$version" ]]; then
    echo "ERROR: User ID, key, and version required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get metadata ID
  local metadata_id
  metadata_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM auth.user_metadata
     WHERE user_id = '$user_id' AND key = '$key'
     LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$metadata_id" ]]; then
    echo "ERROR: Metadata not found" >&2
    return 1
  fi

  # Get historical value
  local historical_data
  historical_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT value, value_type FROM auth.user_metadata_history
     WHERE metadata_id = '$metadata_id' AND version = $version
     LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$historical_data" ]]; then
    echo "ERROR: Version $version not found" >&2
    return 1
  fi

  local value value_type
  read -r value value_type <<<"$historical_data"

  # Restore value
  metadata_set "$user_id" "$key" "$value" "$value_type"
  return $?
}

# ============================================================================
# Metadata Search
# ============================================================================

# Search users by metadata
# Usage: metadata_search <key> <value> [limit]
# Returns: JSON array of user IDs
metadata_search() {
  local key="$1"
  local value="$2"
  local limit="${3:-50}"

  if [[ -z "$key" ]] || [[ -z "$value" ]]; then
    echo "ERROR: Key and value required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Search metadata
  local results_json
  results_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(user_id)
     FROM auth.user_metadata
     WHERE key = '$key' AND value ILIKE '%$value%'
     LIMIT $limit;" \
    2>/dev/null | xargs)

  if [[ -z "$results_json" ]] || [[ "$results_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$results_json"
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f metadata_set
export -f metadata_get
export -f metadata_get_all
export -f metadata_delete
export -f metadata_set_bulk
export -f metadata_delete_all
export -f metadata_get_history
export -f metadata_restore_version
export -f metadata_search
