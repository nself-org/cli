#!/usr/bin/env bash
# magic-link.sh - Magic link (passwordless email) authentication
# Part of nself v0.6.0 - Phase 1 Sprint 1 (AUTH-005)


# Magic link expiry (15 minutes)
readonly MAGIC_LINK_EXPIRY_SECONDS=900

set -euo pipefail


# Generate a magic link token
# Usage: generate_magic_link_token
generate_magic_link_token() {
  openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p
}

# Create and send magic link
# Usage: create_magic_link <email>
# Returns: Magic link token
create_magic_link() {
  local email="$1"

  if [[ -z "$email" ]]; then
    echo "ERROR: Email required" >&2
    return 1
  fi

  # Generate token
  local token
  token=$(generate_magic_link_token)

  # Calculate expiry
  local expires_at
  expires_at=$(date -u -d "+${MAGIC_LINK_EXPIRY_SECONDS} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${MAGIC_LINK_EXPIRY_SECONDS}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Store token in database (create table if needed)
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create magic_links table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.magic_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL,
  token TEXT UNIQUE NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_magic_links_token ON auth.magic_links(token);
CREATE INDEX IF NOT EXISTS idx_magic_links_email ON auth.magic_links(email);
EOSQL

  # Insert magic link
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.magic_links (email, token, expires_at) VALUES ('$email', '$token', '$expires_at'::timestamptz);" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to create magic link" >&2
    return 1
  fi

  # Return token
  echo "$token"
  return 0
}

# Verify magic link token
# Usage: verify_magic_link <token>
# Returns: Email on success
verify_magic_link() {
  local token="$1"

  if [[ -z "$token" ]]; then
    echo "ERROR: Token required" >&2
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get magic link from database
  local link_data
  link_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT email, expires_at, used_at FROM auth.magic_links WHERE token = '$token' LIMIT 1;" \
    2>/dev/null || echo "")

  if [[ -z "$link_data" ]]; then
    echo "ERROR: Invalid or expired magic link" >&2
    return 1
  fi

  # Parse link data
  local email expires_at used_at
  read -r email expires_at used_at <<<"$(echo "$link_data" | xargs)"

  # Check if already used
  if [[ -n "$used_at" ]]; then
    echo "ERROR: Magic link already used" >&2
    return 1
  fi

  # Check if expired
  local now
  now=$(date -u "+%Y-%m-%d %H:%M:%S")

  if [[ "$now" > "$expires_at" ]]; then
    echo "ERROR: Magic link expired" >&2
    return 1
  fi

  # Mark as used
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.magic_links SET used_at = NOW() WHERE token = '$token';" \
    >/dev/null 2>&1

  # Return email
  echo "$email"
  return 0
}

# Export functions
export -f create_magic_link
export -f verify_magic_link
export -f generate_magic_link_token
