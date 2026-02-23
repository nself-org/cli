#!/usr/bin/env bash
# backup-codes.sh - MFA backup codes implementation (MFA-005)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements backup codes for MFA recovery
# One-time use codes for account recovery


# Backup codes configuration
readonly BACKUP_CODE_COUNT=10

set -euo pipefail

readonly BACKUP_CODE_LENGTH=8
readonly BACKUP_CODE_FORMAT="XXXX-XXXX" # 4 chars hyphen 4 chars

# ============================================================================
# Backup Code Generation
# ============================================================================

# Generate a single backup code
# Usage: backup_code_generate_one
# Returns: 8-character alphanumeric code (format: XXXX-XXXX)
backup_code_generate_one() {
  # Generate 8 random alphanumeric characters (no ambiguous characters)
  local chars="ABCDEFGHJKLMNPQRSTUVWXYZ23456789" # No I, O, 0, 1
  local code=""

  for ((i = 0; i < 8; i++)); do
    if command -v openssl >/dev/null 2>&1; then
      local random_index=$(($(openssl rand -hex 1 | tr -d '\n' | awk '{print "0x" $0}') % ${#chars}))
      code="${code}${chars:$random_index:1}"
    else
      code="${code}${chars:$((RANDOM % ${#chars})):1}"
    fi
  done

  # Format as XXXX-XXXX
  echo "${code:0:4}-${code:4:4}"
}

# Generate multiple backup codes
# Usage: backup_codes_generate [count]
# Returns: Array of backup codes
backup_codes_generate() {
  local count="${1:-$BACKUP_CODE_COUNT}"

  for ((i = 0; i < count; i++)); do
    backup_code_generate_one
  done
}

# ============================================================================
# Backup Codes Storage
# ============================================================================

# Create backup codes for user
# Usage: backup_codes_create <user_id>
# Returns: JSON array of backup codes
backup_codes_create() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_backup_codes table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_backup_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  code_hash TEXT NOT NULL,
  used BOOLEAN DEFAULT FALSE,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(code_hash)
);
CREATE INDEX IF NOT EXISTS idx_mfa_backup_codes_user_id ON auth.mfa_backup_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_backup_codes_hash ON auth.mfa_backup_codes(code_hash);
EOSQL

  # Revoke any existing unused codes
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.mfa_backup_codes WHERE user_id = '$user_id' AND used = FALSE;" \
    >/dev/null 2>&1

  # Generate new codes
  local codes=()
  for ((i = 0; i < BACKUP_CODE_COUNT; i++)); do
    local code
    code=$(backup_code_generate_one)
    codes+=("$code")

    # Hash code for storage (SHA-256)
    local code_hash
    if command -v openssl >/dev/null 2>&1; then
      code_hash=$(printf "%s" "$code" | openssl dgst -sha256 | cut -d' ' -f2)
    else
      code_hash=$(printf "%s" "$code" | sha256sum | cut -d' ' -f1)
    fi

    # Store in database
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "INSERT INTO auth.mfa_backup_codes (user_id, code, code_hash, used)
       VALUES ('$user_id', '***', '$code_hash', FALSE);" \
      >/dev/null 2>&1
  done

  # Return codes as JSON array
  printf '{\n  "codes": [\n'
  for ((i = 0; i < ${#codes[@]}; i++)); do
    printf '    "%s"' "${codes[$i]}"
    if [[ $i -lt $((${#codes[@]} - 1)) ]]; then
      printf ',\n'
    else
      printf '\n'
    fi
  done
  printf '  ],\n'
  printf '  "count": %d,\n' "${#codes[@]}"
  printf '  "warning": "Store these codes in a safe place. Each code can only be used once."\n'
  printf '}\n'

  return 0
}

# ============================================================================
# Backup Code Verification
# ============================================================================

# Verify and consume a backup code
# Usage: backup_code_verify <user_id> <code>
# Returns: 0 if valid and unused, 1 otherwise
backup_code_verify() {
  local user_id="$1"
  local code="$2"

  # Remove spaces and convert to uppercase
  code=$(echo "$code" | tr -d ' ' | tr '[:lower:]' '[:upper:]')

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Hash the provided code
  local code_hash
  if command -v openssl >/dev/null 2>&1; then
    code_hash=$(printf "%s" "$code" | openssl dgst -sha256 | cut -d' ' -f2)
  else
    code_hash=$(printf "%s" "$code" | sha256sum | cut -d' ' -f1)
  fi

  # Check if code exists and is unused
  local backup_code_id
  backup_code_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM auth.mfa_backup_codes
     WHERE user_id = '$user_id'
       AND code_hash = '$code_hash'
       AND used = FALSE
     LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$backup_code_id" ]]; then
    echo "ERROR: Invalid or already used backup code" >&2
    return 1
  fi

  # Mark code as used
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_backup_codes
     SET used = TRUE,
         used_at = NOW()
     WHERE id = '$backup_code_id';" \
    >/dev/null 2>&1

  echo "✓ Backup code verified and consumed" >&2
  return 0
}

# ============================================================================
# Backup Code Management
# ============================================================================

# Get backup code status for user
# Usage: backup_codes_status <user_id>
# Returns: JSON with code statistics
backup_codes_status() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get code statistics
  local stats
  stats=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT
       COUNT(*) FILTER (WHERE used = FALSE) as unused,
       COUNT(*) FILTER (WHERE used = TRUE) as used,
       COUNT(*) as total
     FROM auth.mfa_backup_codes
     WHERE user_id = '$user_id';" \
    2>/dev/null | xargs)

  if [[ -z "$stats" ]]; then
    echo '{"unused": 0, "used": 0, "total": 0}'
    return 0
  fi

  local unused used total
  read -r unused used total <<<"$stats"

  cat <<EOF
{
  "unused": ${unused:-0},
  "used": ${used:-0},
  "total": ${total:-0}
}
EOF

  return 0
}

# Regenerate backup codes (revokes old unused ones)
# Usage: backup_codes_regenerate <user_id>
# Returns: JSON array of new backup codes
backup_codes_regenerate() {
  local user_id="$1"

  echo "⚠ Regenerating backup codes will invalidate all unused codes" >&2
  backup_codes_create "$user_id"
}

# List backup codes (only shows if used, not the codes themselves)
# Usage: backup_codes_list <user_id>
# Returns: JSON array of backup code status
backup_codes_list() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get all backup codes
  local codes_json
  codes_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -A -F'|' -c \
    "SELECT
       id,
       used,
       created_at,
       used_at
     FROM auth.mfa_backup_codes
     WHERE user_id = '$user_id'
     ORDER BY created_at DESC;" \
    2>/dev/null)

  if [[ -z "$codes_json" ]]; then
    echo '{"codes": []}'
    return 0
  fi

  printf '{\n  "codes": [\n'
  local first=true
  while IFS='|' read -r id used created_at used_at; do
    if [[ "$first" == "false" ]]; then
      printf ',\n'
    fi
    first=false

    printf '    {\n'
    printf '      "id": "%s",\n' "$id"
    printf '      "used": %s,\n' "$used"
    printf '      "created_at": "%s"' "$created_at"
    if [[ -n "$used_at" ]]; then
      printf ',\n      "used_at": "%s"\n' "$used_at"
    else
      printf '\n'
    fi
    printf '    }'
  done <<<"$codes_json"
  printf '\n  ]\n}\n'

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f backup_code_generate_one
export -f backup_codes_generate
export -f backup_codes_create
export -f backup_code_verify
export -f backup_codes_status
export -f backup_codes_regenerate
export -f backup_codes_list
