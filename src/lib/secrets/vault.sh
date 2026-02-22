#!/usr/bin/env bash
# vault.sh - Encrypted secrets vault (SEC-002, SEC-003, SEC-004)
# Part of nself v0.6.0 - Phase 1 Sprint 4
#
# Manages encrypted storage, rotation, and versioning of secrets

# Note: Library files should not use set -euo pipefail as it can cause
# issues when sourced by other scripts. Use explicit error checking instead.

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$SCRIPT_DIR/encryption.sh" ]]; then
  source "$SCRIPT_DIR/encryption.sh"
fi

# Source safe query library for SQL injection prevention
if [[ -f "$SCRIPT_DIR/../database/safe-query.sh" ]]; then
  source "$SCRIPT_DIR/../database/safe-query.sh"
fi

# ============================================================================
# Vault Initialization
# ============================================================================

# Initialize secrets vault
# Usage: vault_init
vault_init() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create vault tables
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE SCHEMA IF NOT EXISTS secrets;

CREATE TABLE IF NOT EXISTS secrets.vault (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key_name TEXT NOT NULL,
  encrypted_value TEXT NOT NULL,
  encryption_key_id UUID NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  environment TEXT NOT NULL DEFAULT 'default',
  description TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  created_by UUID,
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  rotated_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  is_active BOOLEAN DEFAULT TRUE,
  UNIQUE(key_name, environment, version)
);

CREATE INDEX IF NOT EXISTS idx_vault_key_name ON secrets.vault(key_name);
CREATE INDEX IF NOT EXISTS idx_vault_environment ON secrets.vault(environment);
CREATE INDEX IF NOT EXISTS idx_vault_active ON secrets.vault(is_active);
CREATE INDEX IF NOT EXISTS idx_vault_created_at ON secrets.vault(created_at);

-- Version history table
CREATE TABLE IF NOT EXISTS secrets.vault_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  vault_id UUID NOT NULL REFERENCES secrets.vault(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  encrypted_value TEXT NOT NULL,
  encryption_key_id UUID NOT NULL,
  changed_by UUID,
  changed_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vault_versions_vault_id ON secrets.vault_versions(vault_id);
CREATE INDEX IF NOT EXISTS idx_vault_versions_changed_at ON secrets.vault_versions(changed_at);
EOSQL

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to initialize secrets vault" >&2
    return 1
  fi

  # Initialize encryption system if needed
  encryption_init >/dev/null 2>&1

  echo "✓ Secrets vault initialized" >&2
  return 0
}

# ============================================================================
# Secret Storage (SEC-002)
# ============================================================================

# Store secret
# Usage: vault_set <key_name> <value> [environment] [description] [expires_days]
vault_set() {
  local key_name="$1"
  local value="$2"
  local environment="${3:-default}"
  local description="${4:-}"
  local expires_days="${5:-}"

  if [[ -z "$key_name" ]] || [[ -z "$value" ]]; then
    echo "ERROR: Key name and value required" >&2
    return 1
  fi

  # Get active encryption key
  local key_json
  key_json=$(encryption_get_active_key)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: No encryption key available. Run: encryption_init" >&2
    return 1
  fi

  local encryption_key_id
  encryption_key_id=$(echo "$key_json" | jq -r '.id')

  # Encrypt the value
  local encrypted_value
  encrypted_value=$(encryption_encrypt "$value")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to encrypt secret" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Validate key_name (alphanumeric, underscore, hyphen only)
  key_name=$(validate_identifier "$key_name" 100) || {
    echo "ERROR: Invalid key name format (use only letters, numbers, underscore, hyphen)" >&2
    return 1
  }

  # Validate environment
  environment=$(validate_identifier "$environment" 50) || {
    echo "ERROR: Invalid environment format" >&2
    return 1
  }

  # Validate encryption_key_id as UUID
  encryption_key_id=$(validate_uuid "$encryption_key_id") || {
    echo "ERROR: Invalid encryption key ID" >&2
    return 1
  }

  # Check if secret exists (SAFE - parameterized query)
  local existing_id
  existing_id=$(pg_query_value "
    SELECT id FROM secrets.vault
    WHERE key_name = :'param1' AND environment = :'param2' AND is_active = TRUE
    LIMIT 1
  " "$key_name" "$environment")

  # Calculate expiry (NULL or timestamptz)
  local expires_at=""
  if [[ -n "$expires_days" ]]; then
    expires_at=$(date -u -d "+${expires_days} days" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
      date -u -v+${expires_days}d "+%Y-%m-%d %H:%M:%S" 2>/dev/null)
  fi

  if [[ -n "$existing_id" ]]; then
    # Validate existing_id as UUID
    existing_id=$(validate_uuid "$existing_id") || {
      echo "ERROR: Invalid existing secret ID" >&2
      return 1
    }

    # Archive current version (SAFE - parameterized query)
    pg_query_safe "
      INSERT INTO secrets.vault_versions (vault_id, version, encrypted_value, encryption_key_id)
      SELECT id, version, encrypted_value, encryption_key_id
      FROM secrets.vault
      WHERE id = :'param1'
    " "$existing_id"

    # Update with new version (SAFE - parameterized query)
    if [[ -n "$expires_at" ]]; then
      pg_query_safe "
        UPDATE secrets.vault SET
          encrypted_value = :'param1',
          encryption_key_id = :'param2',
          version = version + 1,
          description = :'param3',
          updated_at = NOW(),
          expires_at = :'param4'::timestamptz
        WHERE id = :'param5'
      " "$encrypted_value" "$encryption_key_id" "$description" "$expires_at" "$existing_id"
    else
      pg_query_safe "
        UPDATE secrets.vault SET
          encrypted_value = :'param1',
          encryption_key_id = :'param2',
          version = version + 1,
          description = :'param3',
          updated_at = NOW(),
          expires_at = NULL
        WHERE id = :'param4'
      " "$encrypted_value" "$encryption_key_id" "$description" "$existing_id"
    fi

    echo "$existing_id"
  else
    # Create new secret (SAFE - parameterized query)
    local secret_id
    if [[ -n "$expires_at" ]]; then
      secret_id=$(pg_query_value "
        INSERT INTO secrets.vault (key_name, encrypted_value, encryption_key_id, environment, description, expires_at)
        VALUES (:'param1', :'param2', :'param3', :'param4', :'param5', :'param6'::timestamptz)
        RETURNING id
      " "$key_name" "$encrypted_value" "$encryption_key_id" "$environment" "$description" "$expires_at")
    else
      secret_id=$(pg_query_value "
        INSERT INTO secrets.vault (key_name, encrypted_value, encryption_key_id, environment, description)
        VALUES (:'param1', :'param2', :'param3', :'param4', :'param5')
        RETURNING id
      " "$key_name" "$encrypted_value" "$encryption_key_id" "$environment" "$description")
    fi

    if [[ -z "$secret_id" ]]; then
      echo "ERROR: Failed to store secret" >&2
      return 1
    fi

    echo "$secret_id"
  fi

  return 0
}

# Get secret (decrypted)
# Usage: vault_get <key_name> [environment] [version]
vault_get() {
  local key_name="$1"
  local environment="${2:-default}"
  local version="${3:-}"

  if [[ -z "$key_name" ]]; then
    echo "ERROR: Key name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Validate key_name
  key_name=$(validate_identifier "$key_name" 100) || {
    echo "ERROR: Invalid key name format" >&2
    return 1
  }

  # Validate environment
  environment=$(validate_identifier "$environment" 50) || {
    echo "ERROR: Invalid environment format" >&2
    return 1
  }

  # Validate version if provided
  if [[ -n "$version" ]]; then
    version=$(validate_integer "$version" 1) || {
      echo "ERROR: Invalid version number" >&2
      return 1
    }
  fi

  # Get encrypted secret (SAFE - parameterized query)
  local result
  if [[ -n "$version" ]]; then
    result=$(pg_query_value "
      SELECT encrypted_value || '|' || encryption_key_id
      FROM secrets.vault
      WHERE key_name = :'param1' AND environment = :'param2' AND version = :'param3'
      LIMIT 1
    " "$key_name" "$environment" "$version")
  else
    result=$(pg_query_value "
      SELECT encrypted_value || '|' || encryption_key_id
      FROM secrets.vault
      WHERE key_name = :'param1' AND environment = :'param2' AND is_active = TRUE
      LIMIT 1
    " "$key_name" "$environment")
  fi

  if [[ -z "$result" ]]; then
    echo "ERROR: Secret not found: $key_name (environment: $environment)" >&2
    return 1
  fi

  local encrypted_value="${result%%|*}"
  local encryption_key_id="${result#*|}"

  # Get encryption key
  local key_json
  key_json=$(encryption_get_key "$encryption_key_id")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Encryption key not found" >&2
    return 1
  fi

  local key_data
  key_data=$(echo "$key_json" | jq -r '.key_data')

  # Decrypt value
  local decrypted
  decrypted=$(encryption_decrypt "$encrypted_value" "$key_data")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to decrypt secret" >&2
    return 1
  fi

  echo "$decrypted"
  return 0
}

# Delete secret
# Usage: vault_delete <key_name> [environment]
vault_delete() {
  local key_name="$1"
  local environment="${2:-default}"

  if [[ -z "$key_name" ]]; then
    echo "ERROR: Key name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Validate key_name
  key_name=$(validate_identifier "$key_name" 100) || {
    echo "ERROR: Invalid key name format" >&2
    return 1
  }

  # Validate environment
  environment=$(validate_identifier "$environment" 50) || {
    echo "ERROR: Invalid environment format" >&2
    return 1
  }

  # Soft delete (mark as inactive) - SAFE - parameterized query
  pg_query_safe "
    UPDATE secrets.vault
    SET is_active = FALSE, updated_at = NOW()
    WHERE key_name = :'param1' AND environment = :'param2' AND is_active = TRUE
  " "$key_name" "$environment"

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to delete secret" >&2
    return 1
  fi

  return 0
}

# List secrets
# Usage: vault_list [environment]
vault_list() {
  local environment="${1:-}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Validate environment if provided
  if [[ -n "$environment" ]]; then
    environment=$(validate_identifier "$environment" 50) || {
      echo "ERROR: Invalid environment format" >&2
      return 1
    }
  fi

  # Get secrets (without values) - SAFE - parameterized query
  local secrets_json
  if [[ -n "$environment" ]]; then
    secrets_json=$(pg_query_value "
      SELECT COALESCE(json_agg(s), '[]'::json)
      FROM (
        SELECT id, key_name, environment, version, description, created_at, updated_at, expires_at
        FROM secrets.vault
        WHERE is_active = TRUE AND environment = :'param1'
        ORDER BY key_name, environment
      ) s
    " "$environment")
  else
    secrets_json=$(pg_query_value "
      SELECT COALESCE(json_agg(s), '[]'::json)
      FROM (
        SELECT id, key_name, environment, version, description, created_at, updated_at, expires_at
        FROM secrets.vault
        WHERE is_active = TRUE
        ORDER BY key_name, environment
      ) s
    ")
  fi

  if [[ -z "$secrets_json" ]] || [[ "$secrets_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$secrets_json"
  return 0
}

# ============================================================================
# Secret Rotation (SEC-003)
# ============================================================================

# Rotate secret (re-encrypt with new key)
# Usage: vault_rotate <key_name> [environment]
vault_rotate() {
  local key_name="$1"
  local environment="${2:-default}"

  if [[ -z "$key_name" ]]; then
    echo "ERROR: Key name required" >&2
    return 1
  fi

  # Get current secret value (decrypted)
  local value
  value=$(vault_get "$key_name" "$environment")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to get secret for rotation" >&2
    return 1
  fi

  # Get active encryption key
  local key_json
  key_json=$(encryption_get_active_key)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: No encryption key available" >&2
    return 1
  fi

  local new_encryption_key_id
  new_encryption_key_id=$(echo "$key_json" | jq -r '.id')

  # Re-encrypt with current active key
  local new_encrypted_value
  new_encrypted_value=$(encryption_encrypt "$value")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to re-encrypt secret" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Validate key_name
  key_name=$(validate_identifier "$key_name" 100) || {
    echo "ERROR: Invalid key name format" >&2
    return 1
  }

  # Validate environment
  environment=$(validate_identifier "$environment" 50) || {
    echo "ERROR: Invalid environment format" >&2
    return 1
  }

  # Validate encryption_key_id as UUID
  new_encryption_key_id=$(validate_uuid "$new_encryption_key_id") || {
    echo "ERROR: Invalid encryption key ID" >&2
    return 1
  }

  # Update secret with new encryption (SAFE - parameterized query)
  pg_query_safe "
    UPDATE secrets.vault SET
      encrypted_value = :'param1',
      encryption_key_id = :'param2',
      rotated_at = NOW(),
      updated_at = NOW()
    WHERE key_name = :'param3' AND environment = :'param4' AND is_active = TRUE
  " "$new_encrypted_value" "$new_encryption_key_id" "$key_name" "$environment"

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to rotate secret" >&2
    return 1
  fi

  return 0
}

# Rotate all secrets (after key rotation)
# Usage: vault_rotate_all
vault_rotate_all() {
  echo "Rotating all secrets..." >&2

  # Get all active secrets
  local secrets
  secrets=$(vault_list)

  if [[ "$secrets" == "[]" ]]; then
    echo "No secrets to rotate" >&2
    return 0
  fi

  # Extract key names and environments
  local count
  count=$(echo "$secrets" | jq 'length')

  local rotated=0
  local failed=0

  for ((i = 0; i < count; i++)); do
    local key_name
    local environment
    key_name=$(echo "$secrets" | jq -r ".[$i].key_name")
    environment=$(echo "$secrets" | jq -r ".[$i].environment")

    if vault_rotate "$key_name" "$environment" 2>/dev/null; then
      rotated=$((rotated + 1))
    else
      failed=$((failed + 1))
      echo "WARNING: Failed to rotate: $key_name (environment: $environment)" >&2
    fi
  done

  echo "✓ Rotated $rotated secrets ($failed failed)" >&2
  return 0
}

# ============================================================================
# Version Management (SEC-004)
# ============================================================================

# Get secret version history
# Usage: vault_get_versions <key_name> [environment]
vault_get_versions() {
  local key_name="$1"
  local environment="${2:-default}"

  if [[ -z "$key_name" ]]; then
    echo "ERROR: Key name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Validate key_name
  key_name=$(validate_identifier "$key_name" 100) || {
    echo "ERROR: Invalid key name format" >&2
    return 1
  }

  # Validate environment
  environment=$(validate_identifier "$environment" 50) || {
    echo "ERROR: Invalid environment format" >&2
    return 1
  }

  # Get vault ID (SAFE - parameterized query)
  local vault_id
  vault_id=$(pg_query_value "
    SELECT id FROM secrets.vault
    WHERE key_name = :'param1' AND environment = :'param2'
    LIMIT 1
  " "$key_name" "$environment")

  if [[ -z "$vault_id" ]]; then
    echo "ERROR: Secret not found" >&2
    return 1
  fi

  # Validate vault_id as UUID
  vault_id=$(validate_uuid "$vault_id") || {
    echo "ERROR: Invalid vault ID format" >&2
    return 1
  }

  # Get version history (SAFE - parameterized query)
  local versions_json
  versions_json=$(pg_query_value "
    SELECT COALESCE(json_agg(v), '[]'::json)
    FROM (
      SELECT version, changed_at, changed_by
      FROM secrets.vault_versions
      WHERE vault_id = :'param1'
      ORDER BY version DESC
    ) v
  " "$vault_id")

  if [[ -z "$versions_json" ]] || [[ "$versions_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$versions_json"
  return 0
}

# Rollback to previous version
# Usage: vault_rollback <key_name> <version> [environment]
vault_rollback() {
  local key_name="$1"
  local target_version="$2"
  local environment="${3:-default}"

  if [[ -z "$key_name" ]] || [[ -z "$target_version" ]]; then
    echo "ERROR: Key name and version required" >&2
    return 1
  fi

  # Get the old version's value
  local old_value
  old_value=$(vault_get "$key_name" "$environment" "$target_version")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Version $target_version not found" >&2
    return 1
  fi

  # Set as current (creates new version)
  vault_set "$key_name" "$old_value" "$environment" "Rolled back to version $target_version"

  return $?
}

# ============================================================================
# Export functions
# ============================================================================

export -f vault_init
export -f vault_set
export -f vault_get
export -f vault_delete
export -f vault_list
export -f vault_rotate
export -f vault_rotate_all
export -f vault_get_versions
export -f vault_rollback
