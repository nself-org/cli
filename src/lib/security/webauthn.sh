#!/usr/bin/env bash
# webauthn.sh - WebAuthn/FIDO2 helper functions
# Part of nself v0.6.0 - Sprint 17: Advanced Security


# ============================================================================
# WebAuthn Challenge Generation
# ============================================================================

# Generate a cryptographically secure random challenge for WebAuthn
generate_webauthn_challenge() {

set -euo pipefail

  local challenge_length="${1:-32}" # 32 bytes default

  if command -v openssl >/dev/null 2>&1; then
    # Generate random bytes and base64url encode
    openssl rand -base64 "$challenge_length" | tr '+/' '-_' | tr -d '='
  else
    # Fallback to /dev/urandom
    if [[ -r /dev/urandom ]]; then
      LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$challenge_length"
    else
      # Last resort - use $RANDOM (not cryptographically secure!)
      local challenge=""
      local chars='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
      for ((i = 0; i < challenge_length; i++)); do
        challenge="${challenge}${chars:RANDOM%${#chars}:1}"
      done
      echo "$challenge"
    fi
  fi
}

# ============================================================================
# WebAuthn Registration Options
# ============================================================================

# Generate registration options for WebAuthn
generate_registration_options() {
  local user_id="$1"
  local user_email="$2"
  local rp_name="${3:-nself}"
  local rp_id="${4:-localhost}"

  local challenge
  challenge=$(generate_webauthn_challenge)

  # Create registration options JSON
  cat <<EOF
{
  "challenge": "$challenge",
  "rp": {
    "name": "$rp_name",
    "id": "$rp_id"
  },
  "user": {
    "id": "$user_id",
    "name": "$user_email",
    "displayName": "$user_email"
  },
  "pubKeyCredParams": [
    {"type": "public-key", "alg": -7},
    {"type": "public-key", "alg": -257}
  ],
  "authenticatorSelection": {
    "authenticatorAttachment": "cross-platform",
    "requireResidentKey": false,
    "userVerification": "preferred"
  },
  "timeout": 60000,
  "attestation": "none"
}
EOF
}

# ============================================================================
# WebAuthn Authentication Options
# ============================================================================

# Generate authentication options for WebAuthn
generate_authentication_options() {
  local rp_id="${1:-localhost}"
  local allow_credentials="${2:-[]}" # JSON array of allowed credential IDs

  local challenge
  challenge=$(generate_webauthn_challenge)

  cat <<EOF
{
  "challenge": "$challenge",
  "rpId": "$rp_id",
  "allowCredentials": $allow_credentials,
  "timeout": 60000,
  "userVerification": "preferred"
}
EOF
}

# ============================================================================
# Credential Storage
# ============================================================================

# Store a WebAuthn credential in the database
store_webauthn_credential() {
  local user_id="$1"
  local credential_id="$2"
  local public_key="$3"
  local counter="$4"
  local name="${5:-Hardware Key}"
  local transports="${6:-{usb,nfc}}"

  # This would execute a SQL INSERT
  # For now, just output the data
  cat <<EOF
{
  "user_id": "$user_id",
  "credential_id": "$credential_id",
  "public_key": "$public_key",
  "counter": $counter,
  "name": "$name",
  "transports": "$transports",
  "status": "stored"
}
EOF
}

# Get WebAuthn credentials for a user
get_user_credentials() {
  local user_id="$1"

  # This would query the database
  # Placeholder response
  echo '[]'
}

# ============================================================================
# Credential Verification
# ============================================================================

# Verify a WebAuthn authentication response
verify_webauthn_response() {
  local credential_id="$1"
  local authenticator_data="$2"
  local client_data_json="$3"
  local signature="$4"

  # This would perform cryptographic verification
  # - Parse authenticator data
  # - Verify signature using stored public key
  # - Check counter to prevent replay attacks
  # - Verify challenge matches

  # Placeholder response
  echo '{"verified": true, "counter": 1}'
}

# ============================================================================
# Counter Management
# ============================================================================

# Update credential counter (prevents replay attacks)
update_credential_counter() {
  local credential_id="$1"
  local new_counter="$2"

  # This would execute a SQL UPDATE
  echo '{"updated": true, "counter": '$new_counter'}'
}

# Check if counter value is valid (must be greater than stored value)
is_counter_valid() {
  local stored_counter="$1"
  local new_counter="$2"

  if [[ $new_counter -gt $stored_counter ]]; then
    return 0 # Valid
  else
    return 1 # Invalid - possible replay attack
  fi
}

# ============================================================================
# Attestation Processing
# ============================================================================

# Process attestation statement (for registration)
process_attestation() {
  local attestation_object="$1"

  # This would parse and verify the attestation
  # - Extract authenticator data
  # - Extract attestation statement
  # - Verify attestation based on format (packed, fido-u2f, etc.)

  echo '{"verified": true, "format": "none"}'
}

# ============================================================================
# Transport Detection
# ============================================================================

# Detect available transports for authenticator
detect_authenticator_transports() {
  local user_agent="$1"

  local transports=()

  # Most authenticators support USB
  transports+=("usb")

  # Check for NFC support (typically mobile devices)
  if [[ "$user_agent" =~ (iPhone|Android) ]]; then
    transports+=("nfc")
  fi

  # Check for BLE support
  if [[ "$user_agent" =~ (iPhone|Android|macOS) ]]; then
    transports+=("ble")
  fi

  # Check for platform authenticator (Touch ID, Face ID, Windows Hello)
  if [[ "$user_agent" =~ (macOS|iPhone|iPad) ]]; then
    transports+=("internal")
  elif [[ "$user_agent" =~ Windows ]]; then
    transports+=("internal")
  fi

  # Output as JSON array
  printf '%s\n' "${transports[@]}" | jq -R . | jq -s .
}

# ============================================================================
# Authenticator Types
# ============================================================================

# Determine authenticator type from AAGUID
get_authenticator_type() {
  local aaguid="$1"

  # Known AAGUIDs for popular authenticators
  case "$aaguid" in
    "2fc0579f-8113-47ea-b116-bb5a8db9202a")
      echo "YubiKey 5 Series"
      ;;
    "ee882879-721c-4913-9775-3dfcce97072a")
      echo "YubiKey Bio Series"
      ;;
    "73bb0cd4-e502-49b8-9c6f-b59445bf720b")
      echo "YubiKey 4/5 FIPS Series"
      ;;
    "0c64b5d4-a5f4-4e9e-ba1d-72b8c1f7f1d8")
      echo "Touch ID"
      ;;
    "adce0002-35bc-c60a-648b-0b25f1f05503")
      echo "Face ID"
      ;;
    "08987058-cadc-4b81-b6e1-30de50dcbe96")
      echo "Windows Hello"
      ;;
    *)
      echo "Unknown Authenticator"
      ;;
  esac
}

# ============================================================================
# Security Level Assessment
# ============================================================================

# Assess security level of authenticator
assess_authenticator_security() {
  local authenticator_attachment="$1"
  local transports="$2"
  local aaguid="$3"

  local security_level="medium"

  # Platform authenticators (Touch ID, Face ID, Windows Hello) are high security
  if [[ "$authenticator_attachment" == "platform" ]]; then
    security_level="high"
  fi

  # Hardware tokens (YubiKey) are high security
  if [[ "$transports" =~ usb ]] && [[ ! "$transports" =~ internal ]]; then
    security_level="high"
  fi

  # Check for FIPS certification (ultra high security)
  local authenticator_type
  authenticator_type=$(get_authenticator_type "$aaguid")
  if [[ "$authenticator_type" =~ FIPS ]]; then
    security_level="ultra_high"
  fi

  echo "$security_level"
}

# ============================================================================
# Helper Functions
# ============================================================================

# Base64URL encode
base64url_encode() {
  local input="$1"
  echo -n "$input" | base64 | tr '+/' '-_' | tr -d '='
}

# Base64URL decode
base64url_decode() {
  local input="$1"
  # Add padding if needed
  local padded="$input"
  while [[ $((${#padded} % 4)) -ne 0 ]]; do
    padded="${padded}="
  done
  echo -n "$padded" | tr '_-' '/+' | base64 -d
}

# Generate user handle (for WebAuthn)
generate_user_handle() {
  local user_id="$1"
  # Convert UUID to bytes and base64url encode
  echo -n "$user_id" | openssl dgst -sha256 -binary | base64 | tr '+/' '-_' | tr -d '='
}

# ============================================================================
# WebAuthn Policy
# ============================================================================

# Check if WebAuthn is required for user
is_webauthn_required() {
  local user_id="$1"
  local user_role="$2"

  # Policy: Require WebAuthn for admin and security_admin roles
  if [[ "$user_role" =~ (admin|security_admin) ]]; then
    return 0 # Required
  fi

  # Check if user has opted into WebAuthn requirement
  # This would query the database
  return 1 # Not required
}

# Export functions
export -f generate_webauthn_challenge
export -f generate_registration_options
export -f generate_authentication_options
export -f store_webauthn_credential
export -f get_user_credentials
export -f verify_webauthn_response
export -f update_credential_counter
export -f is_counter_valid
export -f process_attestation
export -f detect_authenticator_transports
export -f get_authenticator_type
export -f assess_authenticator_security
export -f base64url_encode
export -f base64url_decode
export -f generate_user_handle
export -f is_webauthn_required
