#!/usr/bin/env bash
# jwt-manager.sh - JWT configuration and key management (JWT-001, JWT-002)
# Part of nself v0.6.0 - Phase 1 Sprint 3
#
# Implements JWT token configuration and RS256 key rotation


# JWT defaults
readonly JWT_ALGORITHM="RS256"

set -euo pipefail

readonly JWT_ACCESS_TOKEN_TTL=900      # 15 minutes
readonly JWT_REFRESH_TOKEN_TTL=2592000 # 30 days
readonly JWT_ISSUER="nself"

# ============================================================================
# JWT Configuration
# ============================================================================

# Initialize JWT configuration
# Usage: jwt_init
jwt_init() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create jwt_config table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.jwt_config (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  algorithm TEXT NOT NULL DEFAULT 'RS256',
  access_token_ttl INTEGER NOT NULL DEFAULT 900,
  refresh_token_ttl INTEGER NOT NULL DEFAULT 2592000,
  issuer TEXT NOT NULL DEFAULT 'nself',
  audience TEXT,
  allow_multiple_sessions BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default config if not exists
INSERT INTO auth.jwt_config (algorithm, access_token_ttl, refresh_token_ttl, issuer)
SELECT 'RS256', 900, 2592000, 'nself'
WHERE NOT EXISTS (SELECT 1 FROM auth.jwt_config);
EOSQL

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to initialize JWT configuration" >&2
    return 1
  fi

  return 0
}

# Get JWT configuration
# Usage: jwt_get_config
jwt_get_config() {
  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get config
  local config_json
  config_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(c) FROM (
       SELECT
         algorithm,
         access_token_ttl,
         refresh_token_ttl,
         issuer,
         audience,
         allow_multiple_sessions
       FROM auth.jwt_config
       LIMIT 1
     ) c;" \
    2>/dev/null | xargs)

  if [[ -z "$config_json" ]] || [[ "$config_json" == "null" ]]; then
    # Return defaults
    echo "{\"algorithm\":\"RS256\",\"access_token_ttl\":900,\"refresh_token_ttl\":2592000,\"issuer\":\"nself\"}"
    return 0
  fi

  echo "$config_json"
  return 0
}

# Update JWT configuration
# Usage: jwt_update_config <field> <value>
jwt_update_config() {
  local field="$1"
  local value="$2"

  if [[ -z "$field" ]] || [[ -z "$value" ]]; then
    echo "ERROR: Field and value required" >&2
    return 1
  fi

  # Validate field
  local valid_fields=("access_token_ttl" "refresh_token_ttl" "issuer" "audience" "allow_multiple_sessions")
  local is_valid=false
  for f in "${valid_fields[@]}"; do
    if [[ "$field" == "$f" ]]; then
      is_valid=true
      break
    fi
  done

  if [[ "$is_valid" == "false" ]]; then
    echo "ERROR: Invalid field: $field" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Escape value if string
  if [[ "$field" == "issuer" ]] || [[ "$field" == "audience" ]]; then
    value="'$(echo "$value" | sed "s/'/''/g")'"
  fi

  # Update config
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.jwt_config SET $field = $value, updated_at = NOW();" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to update JWT configuration" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# JWT Key Management
# ============================================================================

# Generate JWT key pair (RS256)
# Usage: jwt_generate_keypair
jwt_generate_keypair() {
  # Create keys directory if it doesn't exist
  local keys_dir="${NSELF_ROOT:-/tmp}/keys/jwt"
  mkdir -p "$keys_dir"

  # Generate private key (2048-bit RSA)
  openssl genrsa -out "$keys_dir/private.pem" 2048 2>/dev/null

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to generate private key" >&2
    return 1
  fi

  # Generate public key
  openssl rsa -in "$keys_dir/private.pem" -pubout -out "$keys_dir/public.pem" 2>/dev/null

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to generate public key" >&2
    return 1
  fi

  # Set proper permissions
  chmod 600 "$keys_dir/private.pem"
  chmod 644 "$keys_dir/public.pem"

  echo "✓ Generated JWT key pair" >&2
  echo "$keys_dir"
  return 0
}

# Store JWT key in database
# Usage: jwt_store_key <private_key_path> <public_key_path> [is_active]
jwt_store_key() {
  local private_key_path="$1"
  local public_key_path="$2"
  local is_active="${3:-true}"

  if [[ ! -f "$private_key_path" ]] || [[ ! -f "$public_key_path" ]]; then
    echo "ERROR: Key files not found" >&2
    return 1
  fi

  # Read keys
  local private_key
  private_key=$(cat "$private_key_path")

  local public_key
  public_key=$(cat "$public_key_path")

  # Escape keys
  private_key=$(echo "$private_key" | sed "s/'/''/g")
  public_key=$(echo "$public_key" | sed "s/'/''/g")

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create jwt_keys table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.jwt_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  private_key TEXT NOT NULL,
  public_key TEXT NOT NULL,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  rotated_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_jwt_keys_is_active ON auth.jwt_keys(is_active);
CREATE INDEX IF NOT EXISTS idx_jwt_keys_created_at ON auth.jwt_keys(created_at);
EOSQL

  # If setting as active, deactivate all other keys
  if [[ "$is_active" == "true" ]]; then
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.jwt_keys SET is_active = FALSE, rotated_at = NOW() WHERE is_active = TRUE;" \
      >/dev/null 2>&1
  fi

  # Store new key
  local key_id
  key_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.jwt_keys (private_key, public_key, is_active)
     VALUES ('$private_key', '$public_key', $is_active)
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$key_id" ]]; then
    echo "ERROR: Failed to store JWT key" >&2
    return 1
  fi

  echo "$key_id"
  return 0
}

# Get active JWT key
# Usage: jwt_get_active_key
jwt_get_active_key() {
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
       SELECT id, public_key, created_at
       FROM auth.jwt_keys
       WHERE is_active = TRUE
       ORDER BY created_at DESC
       LIMIT 1
     ) k;" \
    2>/dev/null | xargs)

  if [[ -z "$key_json" ]] || [[ "$key_json" == "null" ]]; then
    echo "ERROR: No active JWT key found" >&2
    return 1
  fi

  echo "$key_json"
  return 0
}

# Rotate JWT key
# Usage: jwt_rotate_key
jwt_rotate_key() {
  echo "Rotating JWT key..." >&2

  # Generate new key pair
  local keys_dir
  keys_dir=$(jwt_generate_keypair)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to generate new key pair" >&2
    return 1
  fi

  # Store new key and set as active
  local key_id
  key_id=$(jwt_store_key "$keys_dir/private.pem" "$keys_dir/public.pem" true)

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to store new key" >&2
    return 1
  fi

  echo "✓ JWT key rotated successfully" >&2
  echo "$key_id"
  return 0
}

# List JWT keys
# Usage: jwt_list_keys [include_private]
jwt_list_keys() {
  local include_private="${1:-false}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build SELECT clause
  local select_clause="id, public_key, is_active, created_at, rotated_at"
  if [[ "$include_private" == "true" ]]; then
    select_clause="id, private_key, public_key, is_active, created_at, rotated_at"
  fi

  # Get keys
  local keys_json
  keys_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(k) FROM (
       SELECT $select_clause
       FROM auth.jwt_keys
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
# JWT Token Generation (Placeholder - requires JWT library)
# ============================================================================

# Generate JWT access token
# Usage: jwt_generate_access_token <user_id> <claims_json>
jwt_generate_access_token() {
  local user_id="$1"
  local claims_json="${2:-{}}"

  # Get JWT config
  local config
  config=$(jwt_get_config)

  local ttl
  ttl=$(echo "$config" | jq -r '.access_token_ttl')

  local issuer
  issuer=$(echo "$config" | jq -r '.issuer')

  # Calculate expiry
  local exp
  exp=$(($(date +%s) + ttl))

  # Build claims
  local token_claims
  token_claims=$(echo "$claims_json" | jq --arg sub "$user_id" --arg iss "$issuer" --arg exp "$exp" \
    '. + {sub: $sub, iss: $iss, exp: ($exp | tonumber), iat: now}')

  # Retrieve active private key from database
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  local private_key_pem
  private_key_pem=$(docker exec -i "$container" psql \
    -U "${POSTGRES_USER:-postgres}" \
    -d "${POSTGRES_DB:-nself_db}" \
    -t -c \
    "SELECT private_key FROM auth.jwt_keys WHERE is_active = TRUE ORDER BY created_at DESC LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$private_key_pem" ]] || [[ "$private_key_pem" == "null" ]]; then
    echo "ERROR: No active JWT signing key found. Run 'nself auth setup' first." >&2
    return 1
  fi

  # Build RS256 header and payload (base64url-encoded)
  local header='{"alg":"RS256","typ":"JWT"}'
  local header_b64
  header_b64=$(printf '%s' "$header" | base64 | tr '+/' '-_' | tr -d '=')
  local payload_b64
  payload_b64=$(printf '%s' "$token_claims" | base64 | tr '+/' '-_' | tr -d '=')

  local signing_input="${header_b64}.${payload_b64}"

  # Write private key and signing input to temp files, sign with openssl
  local tmpkey tmpsig
  tmpkey=$(mktemp)
  tmpsig=$(mktemp)
  printf '%s' "$private_key_pem" > "$tmpkey"
  chmod 600 "$tmpkey"

  printf '%s' "$signing_input" | openssl dgst -sha256 -sign "$tmpkey" > "$tmpsig" 2>/dev/null
  local sign_rc=$?
  rm -f "$tmpkey"

  if [[ "$sign_rc" -ne 0 ]]; then
    rm -f "$tmpsig"
    echo "ERROR: JWT signing failed" >&2
    return 1
  fi

  local sig_b64
  sig_b64=$(base64 < "$tmpsig" | tr '+/' '-_' | tr -d '=')
  rm -f "$tmpsig"

  printf '%s.%s.%s' "$header_b64" "$payload_b64" "$sig_b64"
  return 0
}

# Generate JWT refresh token
# Usage: jwt_generate_refresh_token <user_id>
jwt_generate_refresh_token() {
  local user_id="$1"

  # Generate secure random token
  openssl rand -hex 32
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f jwt_init
export -f jwt_get_config
export -f jwt_update_config
export -f jwt_generate_keypair
export -f jwt_store_key
export -f jwt_get_active_key
export -f jwt_rotate_key
export -f jwt_list_keys
export -f jwt_generate_access_token
export -f jwt_generate_refresh_token
