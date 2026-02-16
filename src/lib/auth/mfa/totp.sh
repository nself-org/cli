#!/usr/bin/env bash
# totp.sh - TOTP (Time-based One-Time Password) implementation (MFA-001)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements RFC 6238 TOTP for multi-factor authentication
# Compatible with Google Authenticator, Authy, etc.


# TOTP configuration
readonly TOTP_PERIOD=30      # Time step in seconds

set -euo pipefail

readonly TOTP_DIGITS=6       # Code length
readonly TOTP_WINDOW=1       # Allow ±1 time window
readonly TOTP_ISSUER="nself" # Issuer name for QR codes

# ============================================================================
# TOTP Secret Generation
# ============================================================================

# Generate TOTP secret (base32 encoded)
# Usage: totp_generate_secret
# Returns: 32-character base32 secret
totp_generate_secret() {
  # Generate 20 random bytes (160 bits) and encode as base32
  # This is compatible with Google Authenticator standard
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 20 | tr -d '\n' | tr -d '=' | tr '+/' 'AB' | head -c 32
  else
    head -c 20 /dev/urandom | base64 | tr -d '\n' | tr -d '=' | tr '+/' 'AB' | head -c 32
  fi
}

# ============================================================================
# TOTP Code Generation & Verification
# ============================================================================

# Generate TOTP code
# Usage: totp_generate_code <secret> [timestamp]
# Returns: 6-digit TOTP code
totp_generate_code() {
  local secret="$1"
  local timestamp="${2:-$(date +%s)}"

  # Try oathtool first (most reliable)
  if command -v oathtool >/dev/null 2>&1; then
    oathtool --totp --base32 "$secret" --now="$timestamp" 2>/dev/null || echo ""
    return 0
  fi

  # Try Python pyotp as fallback
  if command -v python3 >/dev/null 2>&1; then
    python3 <<EOF 2>/dev/null || echo ""
import pyotp
import sys
try:
    totp = pyotp.TOTP('$secret')
    print(totp.at($timestamp))
except:
    sys.exit(1)
EOF
    return 0
  fi

  # If neither available, return error
  echo "ERROR: TOTP generation requires oathtool or Python pyotp" >&2
  return 1
}

# Verify TOTP code
# Usage: totp_verify_code <secret> <code> [timestamp]
# Returns: 0 if valid, 1 if invalid
totp_verify_code() {
  local secret="$1"
  local code="$2"
  local timestamp="${3:-$(date +%s)}"

  # Remove spaces and leading zeros from code
  code=$(echo "$code" | tr -d ' ' | sed 's/^0*//')

  # Check current window and ±TOTP_WINDOW windows
  for ((i = -TOTP_WINDOW; i <= TOTP_WINDOW; i++)); do
    local check_time=$((timestamp + (i * TOTP_PERIOD)))
    local expected_code
    expected_code=$(totp_generate_code "$secret" "$check_time")

    # Remove leading zeros for comparison
    expected_code=$(echo "$expected_code" | sed 's/^0*//')

    if [[ "$code" == "$expected_code" ]]; then
      return 0
    fi
  done

  return 1
}

# ============================================================================
# TOTP Enrollment & Storage
# ============================================================================

# Enroll user in TOTP MFA
# Usage: totp_enroll <user_id> <email>
# Returns: JSON with secret and provisioning URI
totp_enroll() {
  local user_id="$1"
  local email="$2"

  # Generate secret
  local secret
  secret=$(totp_generate_secret)

  if [[ -z "$secret" ]]; then
    echo "ERROR: Failed to generate TOTP secret" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_totp table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_totp (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  secret TEXT NOT NULL,
  enabled BOOLEAN DEFAULT FALSE,
  verified BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  verified_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  UNIQUE(user_id)
);
CREATE INDEX IF NOT EXISTS idx_mfa_totp_user_id ON auth.mfa_totp(user_id);
EOSQL

  # Store secret (not yet enabled or verified)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.mfa_totp (user_id, secret, enabled, verified)
     VALUES ('$user_id', '$secret', FALSE, FALSE)
     ON CONFLICT (user_id) DO UPDATE SET
       secret = EXCLUDED.secret,
       enabled = FALSE,
       verified = FALSE,
       created_at = NOW();" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to store TOTP secret" >&2
    return 1
  fi

  # Generate provisioning URI for QR code
  local uri
  uri="otpauth://totp/${TOTP_ISSUER}:${email}?secret=${secret}&issuer=${TOTP_ISSUER}&algorithm=SHA1&digits=${TOTP_DIGITS}&period=${TOTP_PERIOD}"

  # Return JSON
  cat <<EOF
{
  "secret": "$secret",
  "uri": "$uri",
  "qr_command": "qrencode -o totp_qr.png '$uri'"
}
EOF

  return 0
}

# Verify and enable TOTP MFA
# Usage: totp_verify_enrollment <user_id> <code>
# Returns: 0 if successful
totp_verify_enrollment() {
  local user_id="$1"
  local code="$2"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get secret from database
  local secret
  secret=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT secret FROM auth.mfa_totp WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$secret" ]]; then
    echo "ERROR: TOTP not enrolled for user" >&2
    return 1
  fi

  # Verify code
  if totp_verify_code "$secret" "$code"; then
    # Mark as verified and enabled
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.mfa_totp SET
        enabled = TRUE,
        verified = TRUE,
        verified_at = NOW()
       WHERE user_id = '$user_id';" \
      >/dev/null 2>&1

    # Update user MFA status
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.users SET mfa_enabled = TRUE WHERE id = '$user_id';" \
      >/dev/null 2>&1

    echo "✓ TOTP MFA enabled successfully" >&2
    return 0
  else
    echo "ERROR: Invalid TOTP code" >&2
    return 1
  fi
}

# Authenticate with TOTP code
# Usage: totp_authenticate <user_id> <code>
# Returns: 0 if valid
totp_authenticate() {
  local user_id="$1"
  local code="$2"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get secret and check if enabled
  local totp_data
  totp_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT secret, enabled FROM auth.mfa_totp WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$totp_data" ]]; then
    echo "ERROR: TOTP not enrolled for user" >&2
    return 1
  fi

  local secret enabled
  read -r secret enabled <<<"$totp_data"

  if [[ "$enabled" != "t" ]]; then
    echo "ERROR: TOTP not enabled for user" >&2
    return 1
  fi

  # Verify code
  if totp_verify_code "$secret" "$code"; then
    # Update last used timestamp
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.mfa_totp SET last_used_at = NOW() WHERE user_id = '$user_id';" \
      >/dev/null 2>&1

    return 0
  else
    return 1
  fi
}

# Disable TOTP MFA
# Usage: totp_disable <user_id>
totp_disable() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Disable TOTP
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_totp SET enabled = FALSE WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  # Check if user has any other MFA methods enabled
  local has_other_mfa
  has_other_mfa=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*) FROM auth.mfa_totp WHERE user_id = '$user_id' AND enabled = TRUE;" \
    2>/dev/null | xargs)

  # If no other MFA methods, disable MFA for user
  if [[ "$has_other_mfa" == "0" ]]; then
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.users SET mfa_enabled = FALSE WHERE id = '$user_id';" \
      >/dev/null 2>&1
  fi

  echo "✓ TOTP MFA disabled" >&2
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f totp_generate_secret
export -f totp_generate_code
export -f totp_verify_code
export -f totp_enroll
export -f totp_verify_enrollment
export -f totp_authenticate
export -f totp_disable
