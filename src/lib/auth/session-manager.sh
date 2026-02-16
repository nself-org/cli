#!/usr/bin/env bash
# session-manager.sh - Session management system (SESS-001 to SESS-006)
# Part of nself v0.6.0 - Phase 1 Sprint 3
#
# Implements session lifecycle, refresh token rotation, and session revocation


# Session defaults
readonly SESSION_DEFAULT_TTL=900   # 15 minutes

set -euo pipefail

readonly REFRESH_TOKEN_TTL=2592000 # 30 days
readonly MAX_SESSIONS_PER_USER=10

# ============================================================================
# Session Creation
# ============================================================================

# Create a new session
# Usage: session_create <user_id> <access_token> <refresh_token> [metadata_json]
session_create() {
  local user_id="$1"
  local access_token="$2"
  local refresh_token="$3"
  local metadata_json="${4:-{}}"

  if [[ -z "$user_id" ]] || [[ -z "$access_token" ]] || [[ -z "$refresh_token" ]]; then
    echo "ERROR: User ID, access token, and refresh token required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Ensure sessions table exists (might already exist from Sprint 1)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  access_token TEXT NOT NULL,
  refresh_token TEXT UNIQUE NOT NULL,
  refresh_token_hash TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  refresh_expires_at TIMESTAMPTZ NOT NULL,
  ip_address TEXT,
  user_agent TEXT,
  metadata JSONB DEFAULT '{}',
  revoked BOOLEAN DEFAULT FALSE,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  last_active_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON auth.sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_refresh_token ON auth.sessions(refresh_token);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON auth.sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_revoked ON auth.sessions(revoked);
EOSQL

  # Calculate expiry times
  local access_expires_at
  access_expires_at=$(date -u -d "+${SESSION_DEFAULT_TTL} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${SESSION_DEFAULT_TTL}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  local refresh_expires_at
  refresh_expires_at=$(date -u -d "+${REFRESH_TOKEN_TTL} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${REFRESH_TOKEN_TTL}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Hash refresh token for storage
  local refresh_token_hash
  if command -v openssl >/dev/null 2>&1; then
    refresh_token_hash=$(echo -n "$refresh_token" | openssl dgst -sha256 | cut -d' ' -f2)
  else
    refresh_token_hash=$(echo -n "$refresh_token" | sha256sum | cut -d' ' -f1)
  fi

  # Escape metadata
  metadata_json=$(echo "$metadata_json" | sed "s/'/''/g")

  # Create session
  local session_id
  session_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.sessions (user_id, access_token, refresh_token, refresh_token_hash, expires_at, refresh_expires_at, metadata)
     VALUES ('$user_id', '$access_token', '$refresh_token', '$refresh_token_hash', '$access_expires_at'::timestamptz, '$refresh_expires_at'::timestamptz, '$metadata_json'::jsonb)
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$session_id" ]]; then
    echo "ERROR: Failed to create session" >&2
    return 1
  fi

  # Clean up old sessions if user has too many
  session_cleanup_user_sessions "$user_id" "$MAX_SESSIONS_PER_USER" 2>/dev/null

  echo "$session_id"
  return 0
}

# ============================================================================
# Session Retrieval
# ============================================================================

# Get session by ID
# Usage: session_get_by_id <session_id>
session_get_by_id() {
  local session_id="$1"

  if [[ -z "$session_id" ]]; then
    echo "ERROR: Session ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get session
  local session_json
  session_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         id,
         user_id,
         expires_at,
         refresh_expires_at,
         ip_address,
         user_agent,
         metadata,
         revoked,
         revoked_at,
         created_at,
         last_active_at
       FROM auth.sessions
       WHERE id = '$session_id'
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$session_json" ]] || [[ "$session_json" == "null" ]]; then
    echo "ERROR: Session not found" >&2
    return 1
  fi

  echo "$session_json"
  return 0
}

# Get session by refresh token
# Usage: session_get_by_refresh_token <refresh_token>
session_get_by_refresh_token() {
  local refresh_token="$1"

  if [[ -z "$refresh_token" ]]; then
    echo "ERROR: Refresh token required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get session
  local session_json
  session_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         id,
         user_id,
         expires_at,
         refresh_expires_at,
         revoked,
         created_at
       FROM auth.sessions
       WHERE refresh_token = '$refresh_token'
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$session_json" ]] || [[ "$session_json" == "null" ]]; then
    echo "ERROR: Session not found" >&2
    return 1
  fi

  echo "$session_json"
  return 0
}

# List user sessions
# Usage: session_list_user <user_id> [include_revoked]
session_list_user() {
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

  # Get sessions
  local sessions_json
  sessions_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(s) FROM (
       SELECT
         id,
         expires_at,
         refresh_expires_at,
         ip_address,
         user_agent,
         revoked,
         revoked_at,
         created_at,
         last_active_at
       FROM auth.sessions
       $where_clause
       ORDER BY last_active_at DESC
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$sessions_json" ]] || [[ "$sessions_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$sessions_json"
  return 0
}

# ============================================================================
# Session Update
# ============================================================================

# Update session last active timestamp
# Usage: session_update_activity <session_id>
session_update_activity() {
  local session_id="$1"

  if [[ -z "$session_id" ]]; then
    echo "ERROR: Session ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Update last active
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.sessions SET last_active_at = NOW() WHERE id = '$session_id';" \
    >/dev/null 2>&1

  return $?
}

# Update session metadata
# Usage: session_update_metadata <session_id> <metadata_json>
session_update_metadata() {
  local session_id="$1"
  local metadata_json="$2"

  if [[ -z "$session_id" ]] || [[ -z "$metadata_json" ]]; then
    echo "ERROR: Session ID and metadata required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape metadata
  metadata_json=$(echo "$metadata_json" | sed "s/'/''/g")

  # Update metadata
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.sessions SET metadata = '$metadata_json'::jsonb WHERE id = '$session_id';" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# Refresh Token Rotation
# ============================================================================

# Rotate refresh token
# Usage: session_rotate_refresh_token <old_refresh_token>
# Returns: New refresh token
session_rotate_refresh_token() {
  local old_refresh_token="$1"

  if [[ -z "$old_refresh_token" ]]; then
    echo "ERROR: Old refresh token required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get session by old refresh token
  local session_data
  session_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id, user_id, revoked, refresh_expires_at FROM auth.sessions WHERE refresh_token = '$old_refresh_token' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$session_data" ]]; then
    echo "ERROR: Invalid refresh token" >&2
    return 1
  fi

  local session_id user_id revoked refresh_expires_at
  read -r session_id user_id revoked refresh_expires_at <<<"$session_data"

  # Check if session is revoked
  if [[ "$revoked" == "t" ]]; then
    echo "ERROR: Session has been revoked" >&2
    return 1
  fi

  # Check if refresh token is expired
  local now
  now=$(date -u "+%Y-%m-%d %H:%M:%S")

  if [[ "$now" > "$refresh_expires_at" ]]; then
    echo "ERROR: Refresh token expired" >&2
    return 1
  fi

  # Generate new refresh token
  local new_refresh_token
  new_refresh_token=$(openssl rand -hex 32)

  # Hash new refresh token
  local new_refresh_token_hash
  if command -v openssl >/dev/null 2>&1; then
    new_refresh_token_hash=$(echo -n "$new_refresh_token" | openssl dgst -sha256 | cut -d' ' -f2)
  else
    new_refresh_token_hash=$(echo -n "$new_refresh_token" | sha256sum | cut -d' ' -f1)
  fi

  # Update session with new refresh token
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.sessions
     SET refresh_token = '$new_refresh_token',
         refresh_token_hash = '$new_refresh_token_hash',
         last_active_at = NOW()
     WHERE id = '$session_id';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to rotate refresh token" >&2
    return 1
  fi

  echo "$new_refresh_token"
  return 0
}

# ============================================================================
# Session Revocation
# ============================================================================

# Revoke a single session
# Usage: session_revoke <session_id>
session_revoke() {
  local session_id="$1"

  if [[ -z "$session_id" ]]; then
    echo "ERROR: Session ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Revoke session
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.sessions
     SET revoked = TRUE,
         revoked_at = NOW()
     WHERE id = '$session_id';" \
    >/dev/null 2>&1

  return $?
}

# Revoke all user sessions
# Usage: session_revoke_all_user <user_id>
session_revoke_all_user() {
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

  # Revoke all sessions
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.sessions
     SET revoked = TRUE,
         revoked_at = NOW()
     WHERE user_id = '$user_id' AND revoked = FALSE;" \
    >/dev/null 2>&1

  return $?
}

# Revoke all user sessions except current
# Usage: session_revoke_all_except <user_id> <current_session_id>
session_revoke_all_except() {
  local user_id="$1"
  local current_session_id="$2"

  if [[ -z "$user_id" ]] || [[ -z "$current_session_id" ]]; then
    echo "ERROR: User ID and current session ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Revoke all other sessions
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.sessions
     SET revoked = TRUE,
         revoked_at = NOW()
     WHERE user_id = '$user_id'
       AND id != '$current_session_id'
       AND revoked = FALSE;" \
    >/dev/null 2>&1

  return $?
}

# ============================================================================
# Session Cleanup
# ============================================================================

# Clean up expired sessions
# Usage: session_cleanup_expired
session_cleanup_expired() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete expired sessions
  local deleted_count
  deleted_count=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "DELETE FROM auth.sessions
     WHERE refresh_expires_at < NOW()
     RETURNING id;" \
    2>/dev/null | wc -l | xargs)

  echo "Cleaned up $deleted_count expired sessions" >&2
  return 0
}

# Clean up user sessions (keep only N most recent)
# Usage: session_cleanup_user_sessions <user_id> <max_sessions>
session_cleanup_user_sessions() {
  local user_id="$1"
  local max_sessions="${2:-$MAX_SESSIONS_PER_USER}"

  if [[ -z "$user_id" ]]; then
    return 0
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 0
  fi

  # Delete old sessions beyond max
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.sessions
     WHERE id IN (
       SELECT id FROM auth.sessions
       WHERE user_id = '$user_id'
       ORDER BY last_active_at DESC
       OFFSET $max_sessions
     );" \
    >/dev/null 2>&1

  return 0
}

# ============================================================================
# Session Validation
# ============================================================================

# Validate session
# Usage: session_validate <session_id>
# Returns: 0 if valid, 1 if invalid
session_validate() {
  local session_id="$1"

  if [[ -z "$session_id" ]]; then
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 1
  fi

  # Check session validity
  local is_valid
  is_valid=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM auth.sessions
     WHERE id = '$session_id'
       AND revoked = FALSE
       AND expires_at > NOW();" \
    2>/dev/null | xargs)

  if [[ "${is_valid:-0}" -gt 0 ]]; then
    return 0 # Valid
  else
    return 1 # Invalid
  fi
}

# ============================================================================
# Export functions
# ============================================================================

export -f session_create
export -f session_get_by_id
export -f session_get_by_refresh_token
export -f session_list_user
export -f session_update_activity
export -f session_update_metadata
export -f session_rotate_refresh_token
export -f session_revoke
export -f session_revoke_all_user
export -f session_revoke_all_except
export -f session_cleanup_expired
export -f session_cleanup_user_sessions
export -f session_validate
