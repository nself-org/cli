#!/usr/bin/env bash
# apikey-manager.sh - API key management system (KEY-001 to KEY-007)
# Part of nself v0.6.0 - Phase 1 Sprint 4
#
# Implements API key CRUD, scopes, expiration, rotation, and usage tracking


# API key defaults
readonly APIKEY_PREFIX="nself"

set -euo pipefail

readonly APIKEY_LENGTH=32
readonly APIKEY_DEFAULT_EXPIRY=31536000 # 1 year

# ============================================================================
# API Key Generation
# ============================================================================

# Generate API key
# Usage: apikey_generate [prefix]
# Returns: API key string
apikey_generate() {
  local prefix="${1:-$APIKEY_PREFIX}"

  # Generate random key
  local random_part
  if command -v openssl >/dev/null 2>&1; then
    random_part=$(openssl rand -hex $APIKEY_LENGTH | tr -d '\n')
  else
    random_part=$(head -c $APIKEY_LENGTH /dev/urandom | xxd -p | tr -d '\n')
  fi

  echo "${prefix}_${random_part}"
  return 0
}

# Hash API key for storage
# Usage: apikey_hash <api_key>
apikey_hash() {
  local api_key="$1"

  if command -v openssl >/dev/null 2>&1; then
    printf "%s" "$api_key" | openssl dgst -sha256 | cut -d' ' -f2
  else
    printf "%s" "$api_key" | sha256sum | cut -d' ' -f1
  fi
}

# ============================================================================
# API Key CRUD Operations
# ============================================================================

# Create API key
# Usage: apikey_create <user_id> <name> [scopes_json] [expires_in_seconds]
# Returns: API key string (show only once!)
apikey_create() {
  local user_id="$1"
  local name="$2"
  local scopes_json="${3:-[]}"
  local expires_in="${4:-$APIKEY_DEFAULT_EXPIRY}"

  if [[ -z "$user_id" ]] || [[ -z "$name" ]]; then
    echo "ERROR: User ID and name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create api_keys table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  key_hash TEXT UNIQUE NOT NULL,
  key_prefix TEXT NOT NULL,
  scopes JSONB DEFAULT '[]',
  expires_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  usage_count INTEGER DEFAULT 0,
  revoked BOOLEAN DEFAULT FALSE,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, name)
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON auth.api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON auth.api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_expires_at ON auth.api_keys(expires_at);
CREATE INDEX IF NOT EXISTS idx_api_keys_revoked ON auth.api_keys(revoked);
EOSQL

  # Generate API key
  local api_key
  api_key=$(apikey_generate)

  # Hash API key
  local key_hash
  key_hash=$(apikey_hash "$api_key")

  # Extract prefix (first part before underscore)
  local key_prefix="${api_key%%_*}"

  # Calculate expiry
  local expires_at=""
  if [[ "$expires_in" != "0" ]] && [[ "$expires_in" != "never" ]]; then
    expires_at=$(date -u -d "+${expires_in} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
      date -u -v+${expires_in}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)
  fi

  # Escape name and scopes
  name=$(echo "$name" | sed "s/'/''/g")
  scopes_json=$(echo "$scopes_json" | sed "s/'/''/g")

  # Build INSERT query
  local expires_clause=""
  if [[ -n "$expires_at" ]]; then
    expires_clause=", expires_at = '$expires_at'::timestamptz"
  fi

  # Create API key record
  local key_id
  key_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.api_keys (user_id, name, key_hash, key_prefix, scopes${expires_clause:+, expires_at})
     VALUES ('$user_id', '$name', '$key_hash', '$key_prefix', '$scopes_json'::jsonb${expires_at:+, '$expires_at'::timestamptz})
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Failed to create API key" >&2
    return 1
  fi

  # Return the API key (only time it's shown in plain text!)
  cat <<EOF
{
  "id": "$key_id",
  "api_key": "$api_key",
  "name": "$name",
  "warning": "Store this key securely. It will not be shown again."
}
EOF

  return 0
}

# Get API key by ID
# Usage: apikey_get_by_id <key_id>
apikey_get_by_id() {
  local key_id="$1"

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Key ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get API key (excluding hash)
  local key_json
  key_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(k) FROM (
       SELECT
         id,
         user_id,
         name,
         key_prefix,
         scopes,
         expires_at,
         last_used_at,
         usage_count,
         revoked,
         revoked_at,
         created_at
       FROM auth.api_keys
       WHERE id = '$key_id'
     ) k;" \
    2>/dev/null | xargs)

  if [[ -z "$key_json" ]] || [[ "$key_json" == "null" ]]; then
    echo "ERROR: API key not found" >&2
    return 1
  fi

  echo "$key_json"
  return 0
}

# List user API keys
# Usage: apikey_list_user <user_id> [include_revoked]
apikey_list_user() {
  local user_id="$1"
  local include_revoked="${2:-false}"

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

  # Build where clause
  local where_clause="WHERE user_id = '$user_id'"
  if [[ "$include_revoked" != "true" ]]; then
    where_clause="$where_clause AND revoked = FALSE"
  fi

  # Get keys
  local keys_json
  keys_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(k) FROM (
       SELECT
         id,
         name,
         key_prefix,
         scopes,
         expires_at,
         last_used_at,
         usage_count,
         revoked,
         created_at
       FROM auth.api_keys
       $where_clause
       ORDER BY created_at DESC
     ) k;" \
    2>/dev/null | xargs)

  if [[ -z "$keys_json" ]] || [[ "$keys_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$keys_json"
  return 0
}

# Delete API key
# Usage: apikey_delete <key_id>
apikey_delete() {
  local key_id="$1"

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Key ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete key
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.api_keys WHERE id = '$key_id';" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# API Key Scopes
# ============================================================================

# Update API key scopes
# Usage: apikey_update_scopes <key_id> <scopes_json>
# Example scopes: ["read:users", "write:posts", "admin:*"]
apikey_update_scopes() {
  local key_id="$1"
  local scopes_json="$2"

  if [[ -z "$key_id" ]] || [[ -z "$scopes_json" ]]; then
    echo "ERROR: Key ID and scopes required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape scopes
  scopes_json=$(echo "$scopes_json" | sed "s/'/''/g")

  # Update scopes
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.api_keys SET scopes = '$scopes_json'::jsonb WHERE id = '$key_id';" \
    >/dev/null 2>&1

  return $?
}

# Check if API key has scope
# Usage: apikey_has_scope <key_id> <scope>
# Returns: 0 if has scope, 1 if not
apikey_has_scope() {
  local key_id="$1"
  local scope="$2"

  if [[ -z "$key_id" ]] || [[ -z "$scope" ]]; then
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 1
  fi

  # Check if scope exists or if key has wildcard
  local has_scope
  has_scope=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM auth.api_keys
     WHERE id = '$key_id'
       AND (scopes @> '[\"$scope\"]'::jsonb
            OR scopes @> '[\"*\"]'::jsonb
            OR scopes @> '[\"admin:*\"]'::jsonb);" \
    2>/dev/null | xargs)

  if [[ "${has_scope:-0}" -gt 0 ]]; then
    return 0
  else
    return 1
  fi
}

# ============================================================================
# API Key Expiration
# ============================================================================

# Update API key expiration
# Usage: apikey_update_expiry <key_id> <expires_in_seconds>
apikey_update_expiry() {
  local key_id="$1"
  local expires_in="$2"

  if [[ -z "$key_id" ]] || [[ -z "$expires_in" ]]; then
    echo "ERROR: Key ID and expiry required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Calculate new expiry
  local expires_at
  if [[ "$expires_in" == "0" ]] || [[ "$expires_in" == "never" ]]; then
    expires_at="NULL"
  else
    expires_at="'$(date -u -d "+${expires_in} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
      date -u -v+${expires_in}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)'::timestamptz"
  fi

  # Update expiry
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.api_keys SET expires_at = $expires_at WHERE id = '$key_id';" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# API Key Rotation
# ============================================================================

# Rotate API key (generate new key, revoke old)
# Usage: apikey_rotate <key_id>
# Returns: New API key
apikey_rotate() {
  local key_id="$1"

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Key ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get old key info
  local key_data
  key_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT user_id, name, scopes, expires_at FROM auth.api_keys WHERE id = '$key_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$key_data" ]]; then
    echo "ERROR: API key not found" >&2
    return 1
  fi

  local user_id name scopes expires_at
  read -r user_id name scopes expires_at <<<"$key_data"

  # Calculate remaining TTL
  local expires_in="0"
  if [[ -n "$expires_at" ]] && [[ "$expires_at" != "null" ]]; then
    local now
    now=$(date -u +%s)
    local exp
    exp=$(date -u -d "$expires_at" +%s 2>/dev/null || date -u -j -f "%Y-%m-%d %H:%M:%S" "$expires_at" +%s 2>/dev/null)
    expires_in=$((exp - now))
  fi

  # Revoke old key
  apikey_revoke "$key_id" 2>/dev/null

  # Create new key with same settings
  apikey_create "$user_id" "$name (rotated)" "$scopes" "$expires_in"
  return $?
}

# ============================================================================
# API Key Revocation
# ============================================================================

# Revoke API key
# Usage: apikey_revoke <key_id>
apikey_revoke() {
  local key_id="$1"

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Key ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Revoke key
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.api_keys SET revoked = TRUE, revoked_at = NOW() WHERE id = '$key_id';" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# API Key Usage Tracking
# ============================================================================

# Record API key usage
# Usage: apikey_record_usage <api_key>
apikey_record_usage() {
  local api_key="$1"

  if [[ -z "$api_key" ]]; then
    return 0
  fi

  # Hash API key
  local key_hash
  key_hash=$(apikey_hash "$api_key")

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 0
  fi

  # Update usage
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.api_keys
     SET usage_count = usage_count + 1,
         last_used_at = NOW()
     WHERE key_hash = '$key_hash';" \
    >/dev/null 2>&1

  return 0
}

# Get API key usage stats
# Usage: apikey_get_usage_stats <key_id>
apikey_get_usage_stats() {
  local key_id="$1"

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Key ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get stats
  local stats_json
  stats_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         usage_count,
         last_used_at,
         created_at,
         EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400 as days_old
       FROM auth.api_keys
       WHERE id = '$key_id'
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$stats_json" ]] || [[ "$stats_json" == "null" ]]; then
    echo "{}"
    return 0
  fi

  echo "$stats_json"
  return 0
}

# ============================================================================
# API Key Validation
# ============================================================================

# Validate API key
# Usage: apikey_validate <api_key>
# Returns: 0 if valid, 1 if invalid
apikey_validate() {
  local api_key="$1"

  if [[ -z "$api_key" ]]; then
    return 1
  fi

  # Hash API key
  local key_hash
  key_hash=$(apikey_hash "$api_key")

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 1
  fi

  # Check if key is valid
  local is_valid
  is_valid=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM auth.api_keys
     WHERE key_hash = '$key_hash'
       AND revoked = FALSE
       AND (expires_at IS NULL OR expires_at > NOW());" \
    2>/dev/null | xargs)

  if [[ "${is_valid:-0}" -gt 0 ]]; then
    # Record usage
    apikey_record_usage "$api_key" 2>/dev/null &
    return 0
  else
    return 1
  fi
}

# ============================================================================
# Export functions
# ============================================================================

export -f apikey_generate
export -f apikey_hash
export -f apikey_create
export -f apikey_get_by_id
export -f apikey_list_user
export -f apikey_delete
export -f apikey_update_scopes
export -f apikey_has_scope
export -f apikey_update_expiry
export -f apikey_rotate
export -f apikey_revoke
export -f apikey_record_usage
export -f apikey_get_usage_stats
export -f apikey_validate
