#!/usr/bin/env bash
# encryption.sh - Encryption key management (SEC-001)
# Part of nself v0.6.0 - Phase 1 Sprint 4
#
# Manages encryption keys for secrets vault


# Encryption defaults
readonly ENCRYPTION_ALGORITHM="aes-256-cbc"

set -euo pipefail

readonly KEY_SIZE=32 # 256 bits
readonly KEY_ROTATION_DAYS=90

# ============================================================================
# Encryption Key Generation
# ============================================================================

# Generate encryption key
# Usage: encryption_generate_key
# Returns: Base64-encoded 256-bit key
encryption_generate_key() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$KEY_SIZE"
  else
    head -c "$KEY_SIZE" /dev/urandom | base64
  fi
}

# Generate initialization vector (IV)
# Usage: encryption_generate_iv
# Returns: Hex-encoded IV
encryption_generate_iv() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
  else
    head -c 16 /dev/urandom | xxd -p | tr -d '\n'
  fi
}

# ============================================================================
# Key Storage
# ============================================================================

# Store encryption key in database
# Usage: encryption_store_key <key_data> [is_active]
encryption_store_key() {
  local key_data="$1"
  local is_active="${2:-true}"

  if [[ -z "$key_data" ]]; then
    echo "ERROR: Key data required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create encryption_keys table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS secrets;

CREATE TABLE IF NOT EXISTS secrets.encryption_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key_data TEXT NOT NULL,
  algorithm TEXT NOT NULL DEFAULT 'aes-256-cbc',
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  rotated_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_encryption_keys_active ON secrets.encryption_keys(is_active);
CREATE INDEX IF NOT EXISTS idx_encryption_keys_created ON secrets.encryption_keys(created_at);
EOSQL

  # Escape key data
  key_data=$(echo "$key_data" | sed "s/'/''/g")

  # Calculate expiry (90 days from now)
  local expires_at
  expires_at=$(date -u -d "+${KEY_ROTATION_DAYS} days" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${KEY_ROTATION_DAYS}d "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # If setting as active, deactivate all other keys
  if [[ "$is_active" == "true" ]]; then
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE secrets.encryption_keys SET is_active = FALSE, rotated_at = NOW() WHERE is_active = TRUE;" \
      >/dev/null 2>&1
  fi

  # Store new key
  local key_id
  key_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO secrets.encryption_keys (key_data, algorithm, is_active, expires_at)
     VALUES ('$key_data', '$ENCRYPTION_ALGORITHM', $is_active, '$expires_at'::timestamptz)
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Failed to store encryption key" >&2
    return 1
  fi

  echo "$key_id"
  return 0
}

# Get active encryption key
# Usage: encryption_get_active_key
# Returns: JSON with key data
encryption_get_active_key() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get active key
  local key_json
  key_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(k) FROM (
       SELECT id, key_data, algorithm, created_at, expires_at
       FROM secrets.encryption_keys
       WHERE is_active = TRUE
       ORDER BY created_at DESC
       LIMIT 1
     ) k;" \
    2>/dev/null | xargs)

  if [[ -z "$key_json" ]] || [[ "$key_json" == "null" ]]; then
    echo "ERROR: No active encryption key found" >&2
    return 1
  fi

  echo "$key_json"
  return 0
}

# Get encryption key by ID
# Usage: encryption_get_key <key_id>
encryption_get_key() {
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

  # Get key
  local key_json
  key_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(k) FROM (
       SELECT id, key_data, algorithm, is_active, created_at, rotated_at, expires_at
       FROM secrets.encryption_keys
       WHERE id = '$key_id'
     ) k;" \
    2>/dev/null | xargs)

  if [[ -z "$key_json" ]] || [[ "$key_json" == "null" ]]; then
    echo "ERROR: Key not found: $key_id" >&2
    return 1
  fi

  echo "$key_json"
  return 0
}

# ============================================================================
# Key Rotation
# ============================================================================

# Rotate encryption key
# Usage: encryption_rotate_key
# Returns: New key ID
encryption_rotate_key() {
  echo "Rotating encryption key..." >&2

  # Generate new key
  local new_key
  new_key=$(encryption_generate_key)

  if [[ -z "$new_key" ]]; then
    echo "ERROR: Failed to generate new key" >&2
    return 1
  fi

  # Store new key and set as active
  local key_id
  key_id=$(encryption_store_key "$new_key" true)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to store new key" >&2
    return 1
  fi

  echo "✓ Encryption key rotated successfully" >&2
  echo "$key_id"
  return 0
}

# Check if key needs rotation
# Usage: encryption_check_rotation
# Returns: 0 if rotation needed, 1 if not
encryption_check_rotation() {
  # Get active key
  local key_json
  key_json=$(encryption_get_active_key 2>/dev/null)

  if [[ $? -ne 0 ]]; then
    return 0 # No key exists, rotation needed
  fi

  # Get key creation date
  local created_at
  created_at=$(echo "$key_json" | jq -r '.created_at')

  if [[ -z "$created_at" ]] || [[ "$created_at" == "null" ]]; then
    return 0 # Invalid date, rotation needed
  fi

  # Calculate age in days
  local created_epoch
  local now_epoch
  created_epoch=$(date -d "$created_at" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "$created_at" +%s 2>/dev/null)
  now_epoch=$(date +%s)

  local age_days=$(((now_epoch - created_epoch) / 86400))

  if [[ $age_days -ge $KEY_ROTATION_DAYS ]]; then
    echo "Key is $age_days days old (threshold: $KEY_ROTATION_DAYS days)" >&2
    return 0 # Rotation needed
  fi

  return 1 # No rotation needed
}

# List all encryption keys
# Usage: encryption_list_keys
encryption_list_keys() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get keys (without key_data for security)
  local keys_json
  keys_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(k) FROM (
       SELECT id, algorithm, is_active, created_at, rotated_at, expires_at
       FROM secrets.encryption_keys
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

# ============================================================================
# Encryption/Decryption Operations
# ============================================================================

# Encrypt data
# Usage: encryption_encrypt <plaintext> [key_data]
# Returns: Base64-encoded encrypted data with IV
encryption_encrypt() {
  local plaintext="$1"
  local key_data="${2:-}"

  if [[ -z "$plaintext" ]]; then
    echo "ERROR: Plaintext required" >&2
    return 1
  fi

  # Get active key if not provided
  if [[ -z "$key_data" ]]; then
    local key_json
    key_json=$(encryption_get_active_key)

    if [[ $? -ne 0 ]]; then
      echo "ERROR: No encryption key available" >&2
      return 1
    fi

    key_data=$(echo "$key_json" | jq -r '.key_data')
  fi

  # Generate IV
  local iv
  iv=$(encryption_generate_iv)

  # Encrypt using OpenSSL
  local encrypted
  encrypted=$(echo -n "$plaintext" | openssl enc -aes-256-cbc -K "$(echo -n "$key_data" | base64 -d | xxd -p | tr -d '\n')" -iv "$iv" -base64 2>/dev/null)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Encryption failed" >&2
    return 1
  fi

  # Return IV:encrypted_data format
  echo "${iv}:${encrypted}"
  return 0
}

# Decrypt data
# Usage: encryption_decrypt <encrypted_data> [key_data]
# Returns: Decrypted plaintext
encryption_decrypt() {
  local encrypted_data="$1"
  local key_data="${2:-}"

  if [[ -z "$encrypted_data" ]]; then
    echo "ERROR: Encrypted data required" >&2
    return 1
  fi

  # Extract IV and ciphertext
  local iv="${encrypted_data%%:*}"
  local ciphertext="${encrypted_data#*:}"

  if [[ -z "$iv" ]] || [[ -z "$ciphertext" ]]; then
    echo "ERROR: Invalid encrypted data format" >&2
    return 1
  fi

  # Get active key if not provided
  if [[ -z "$key_data" ]]; then
    local key_json
    key_json=$(encryption_get_active_key)

    if [[ $? -ne 0 ]]; then
      echo "ERROR: No encryption key available" >&2
      return 1
    fi

    key_data=$(echo "$key_json" | jq -r '.key_data')
  fi

  # Decrypt using OpenSSL
  local decrypted
  decrypted=$(echo -n "$ciphertext" | openssl enc -d -aes-256-cbc -K "$(echo -n "$key_data" | base64 -d | xxd -p | tr -d '\n')" -iv "$iv" -base64 2>/dev/null)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Decryption failed" >&2
    return 1
  fi

  echo "$decrypted"
  return 0
}

# ============================================================================
# Initialization
# ============================================================================

# Initialize encryption system
# Usage: encryption_init
encryption_init() {
  # Check if active key exists
  local key_json
  key_json=$(encryption_get_active_key 2>/dev/null)

  if [[ $? -eq 0 ]]; then
    echo "Encryption system already initialized" >&2
    return 0
  fi

  echo "Initializing encryption system..." >&2

  # Generate and store first key
  local key
  key=$(encryption_generate_key)

  local key_id
  key_id=$(encryption_store_key "$key" true)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to initialize encryption system" >&2
    return 1
  fi

  echo "✓ Encryption system initialized with key: $key_id" >&2
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f encryption_generate_key
export -f encryption_generate_iv
export -f encryption_store_key
export -f encryption_get_active_key
export -f encryption_get_key
export -f encryption_rotate_key
export -f encryption_check_rotation
export -f encryption_list_keys
export -f encryption_encrypt
export -f encryption_decrypt
export -f encryption_init
