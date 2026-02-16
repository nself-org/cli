#!/usr/bin/env bash
# webauthn.sh - WebAuthn/FIDO2 support
# Part of nself v0.6.0 - Phase 1 Sprint 2 (deferred item)


webauthn_init() {

set -euo pipefail

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  [[ -z "$container" ]] && {
    echo "ERROR: PostgreSQL not found" >&2
    return 1
  }

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.webauthn_credentials (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  credential_id TEXT NOT NULL UNIQUE,
  public_key TEXT NOT NULL,
  counter BIGINT DEFAULT 0,
  device_name TEXT,
  device_type TEXT DEFAULT 'platform',
  transports TEXT[],
  created_at TIMESTAMPTZ DEFAULT NOW(),
  last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_webauthn_user ON auth.webauthn_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_webauthn_cred ON auth.webauthn_credentials(credential_id);

CREATE TABLE IF NOT EXISTS auth.webauthn_challenges (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  challenge TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_webauthn_challenges_expires ON auth.webauthn_challenges(expires_at);
EOSQL
  return 0
}

webauthn_generate_challenge() {
  local user_id="${1:-}"
  local challenge=$(openssl rand -base64 32 | tr -d '\n')
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local expires=$(date -u -d "+5 minutes" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+5M "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  local user_clause="NULL"
  [[ -n "$user_id" ]] && user_clause="'$user_id'"

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.webauthn_challenges (user_id, challenge, expires_at)
     VALUES ($user_clause, '$challenge', '$expires'::timestamptz);" >/dev/null 2>&1

  echo "$challenge"
}

webauthn_verify_challenge() {
  local challenge="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local valid=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.webauthn_challenges
     WHERE challenge = '$challenge' AND expires_at > NOW();" 2>/dev/null | xargs)

  [[ "$valid" -gt 0 ]] && return 0 || return 1
}

webauthn_register_credential() {
  local user_id="$1"
  local credential_id="$2"
  local public_key="$3"
  local device_name="${4:-Security Key}"

  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)
  public_key=$(echo "$public_key" | sed "s/'/''/g")

  local id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.webauthn_credentials (user_id, credential_id, public_key, device_name)
     VALUES ('$user_id', '$credential_id', '$public_key', '$device_name')
     RETURNING id;" 2>/dev/null | xargs)

  echo "$id"
}

webauthn_get_credentials() {
  local user_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  local creds=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(c) FROM (
       SELECT id, credential_id, device_name, device_type, created_at, last_used_at
       FROM auth.webauthn_credentials WHERE user_id = '$user_id' ORDER BY created_at DESC
     ) c;" 2>/dev/null | xargs)

  [[ -z "$creds" || "$creds" == "null" ]] && echo "[]" || echo "$creds"
}

webauthn_update_counter() {
  local credential_id="$1"
  local counter="$2"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.webauthn_credentials
     SET counter = $counter, last_used_at = NOW()
     WHERE credential_id = '$credential_id';" >/dev/null 2>&1
}

webauthn_remove_credential() {
  local credential_id="$1"
  local container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.webauthn_credentials WHERE credential_id = '$credential_id';" >/dev/null 2>&1
}

export -f webauthn_init webauthn_generate_challenge webauthn_verify_challenge
export -f webauthn_register_credential webauthn_get_credentials
export -f webauthn_update_counter webauthn_remove_credential
