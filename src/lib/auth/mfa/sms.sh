#!/usr/bin/env bash
# sms.sh - SMS-based MFA implementation (MFA-002)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements SMS-based multi-factor authentication
# Supports Twilio, AWS SNS, and development mode


# SMS MFA configuration
readonly SMS_CODE_LENGTH=6

set -euo pipefail

readonly SMS_CODE_EXPIRY=300 # 5 minutes
readonly SMS_MAX_ATTEMPTS=3
readonly SMS_RATE_LIMIT=60 # 1 minute between sends

# ============================================================================
# SMS Code Generation
# ============================================================================

# Generate SMS verification code
# Usage: sms_generate_code
# Returns: 6-digit numeric code
sms_generate_code() {
  # Generate 6-digit random code
  if command -v openssl >/dev/null 2>&1; then
    printf "%06d" $((10#$(openssl rand -hex 3 | tr -d '\n' | head -c 6) % 1000000))
  else
    printf "%06d" $((RANDOM % 1000000))
  fi
}

# ============================================================================
# SMS Sending (Multi-Provider Support)
# ============================================================================

# Send SMS via Twilio
# Usage: sms_send_twilio <phone> <code>
sms_send_twilio() {
  local phone="$1"
  local code="$2"

  local account_sid="${TWILIO_ACCOUNT_SID:-}"
  local auth_token="${TWILIO_AUTH_TOKEN:-}"
  local from_number="${TWILIO_PHONE_NUMBER:-}"

  if [[ -z "$account_sid" ]] || [[ -z "$auth_token" ]] || [[ -z "$from_number" ]]; then
    echo "ERROR: Twilio credentials not configured" >&2
    return 1
  fi

  local message="Your ${TOTP_ISSUER:-nself} verification code is: ${code}. Valid for ${SMS_CODE_EXPIRY} seconds."

  curl -s -X POST "https://api.twilio.com/2010-04-01/Accounts/${account_sid}/Messages.json" \
    -u "${account_sid}:${auth_token}" \
    -d "From=${from_number}" \
    -d "To=${phone}" \
    -d "Body=${message}" \
    >/dev/null 2>&1

  return $?
}

# Send SMS via AWS SNS
# Usage: sms_send_aws_sns <phone> <code>
sms_send_aws_sns() {
  local phone="$1"
  local code="$2"

  if ! command -v aws >/dev/null 2>&1; then
    echo "ERROR: AWS CLI not installed" >&2
    return 1
  fi

  local message="Your ${TOTP_ISSUER:-nself} verification code is: ${code}. Valid for ${SMS_CODE_EXPIRY} seconds."

  aws sns publish \
    --phone-number "$phone" \
    --message "$message" \
    >/dev/null 2>&1

  return $?
}

# Send SMS (development mode - prints to console)
# Usage: sms_send_dev <phone> <code>
sms_send_dev() {
  local phone="$1"
  local code="$2"

  cat >&2 <<EOF

╔══════════════════════════════════════════╗
║         SMS VERIFICATION CODE            ║
╠══════════════════════════════════════════╣
║  Phone: ${phone}
║  Code:  ${code}
║  Valid: ${SMS_CODE_EXPIRY} seconds
╚══════════════════════════════════════════╝

EOF

  return 0
}

# Send SMS code (auto-detects provider)
# Usage: sms_send_code <phone> <code>
sms_send_code() {
  local phone="$1"
  local code="$2"

  local provider="${SMS_PROVIDER:-dev}"

  case "$provider" in
    twilio)
      sms_send_twilio "$phone" "$code"
      ;;
    aws | sns)
      sms_send_aws_sns "$phone" "$code"
      ;;
    dev | development)
      sms_send_dev "$phone" "$code"
      ;;
    *)
      echo "ERROR: Unknown SMS provider: $provider" >&2
      return 1
      ;;
  esac
}

# ============================================================================
# SMS MFA Enrollment
# ============================================================================

# Enroll phone number for SMS MFA
# Usage: sms_enroll <user_id> <phone>
# Returns: 0 if successful
sms_enroll() {
  local user_id="$1"
  local phone="$2"

  # Validate phone number format (basic validation)
  if ! echo "$phone" | grep -qE '^\+?[1-9][0-9]{7,14}$'; then
    echo "ERROR: Invalid phone number format" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_sms table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_sms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  phone TEXT NOT NULL,
  code TEXT,
  code_expires_at TIMESTAMPTZ,
  attempts INTEGER DEFAULT 0,
  enabled BOOLEAN DEFAULT FALSE,
  verified BOOLEAN DEFAULT FALSE,
  last_sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  verified_at TIMESTAMPTZ,
  UNIQUE(user_id)
);
CREATE INDEX IF NOT EXISTS idx_mfa_sms_user_id ON auth.mfa_sms(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_sms_phone ON auth.mfa_sms(phone);
EOSQL

  # Store phone number (not yet verified)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.mfa_sms (user_id, phone, enabled, verified)
     VALUES ('$user_id', '$phone', FALSE, FALSE)
     ON CONFLICT (user_id) DO UPDATE SET
       phone = EXCLUDED.phone,
       enabled = FALSE,
       verified = FALSE,
       created_at = NOW();" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to enroll phone number" >&2
    return 1
  fi

  echo "✓ Phone number enrolled for SMS MFA" >&2
  return 0
}

# Send SMS verification code
# Usage: sms_send_verification <user_id>
# Returns: 0 if sent successfully
sms_send_verification() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Check rate limiting
  local last_sent
  last_sent=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT EXTRACT(EPOCH FROM (NOW() - last_sent_at)) FROM auth.mfa_sms WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -n "$last_sent" ]] && [[ "${last_sent%.*}" -lt "$SMS_RATE_LIMIT" ]]; then
    local wait_time=$((SMS_RATE_LIMIT - ${last_sent%.*}))
    echo "ERROR: Please wait ${wait_time} seconds before requesting another code" >&2
    return 1
  fi

  # Get phone number
  local phone
  phone=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT phone FROM auth.mfa_sms WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$phone" ]]; then
    echo "ERROR: Phone number not enrolled" >&2
    return 1
  fi

  # Generate code
  local code
  code=$(sms_generate_code)

  # Calculate expiry
  local expires_at
  expires_at=$(date -u -d "+${SMS_CODE_EXPIRY} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${SMS_CODE_EXPIRY}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Store code
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_sms SET
      code = '$code',
      code_expires_at = '$expires_at'::timestamptz,
      attempts = 0,
      last_sent_at = NOW()
     WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  # Send SMS
  if sms_send_code "$phone" "$code"; then
    echo "✓ Verification code sent to ${phone}" >&2
    return 0
  else
    echo "ERROR: Failed to send SMS" >&2
    return 1
  fi
}

# Verify SMS code and enable MFA
# Usage: sms_verify <user_id> <code>
# Returns: 0 if valid
sms_verify() {
  local user_id="$1"
  local code="$2"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get stored code and expiry
  local sms_data
  sms_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT code, code_expires_at, attempts FROM auth.mfa_sms WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$sms_data" ]]; then
    echo "ERROR: No verification code found" >&2
    return 1
  fi

  local stored_code expires_at attempts
  read -r stored_code expires_at attempts <<<"$sms_data"

  # Check max attempts
  if [[ "$attempts" -ge "$SMS_MAX_ATTEMPTS" ]]; then
    echo "ERROR: Maximum verification attempts exceeded" >&2
    return 1
  fi

  # Check expiry
  local now
  now=$(date -u "+%Y-%m-%d %H:%M:%S")

  if [[ "$now" > "$expires_at" ]]; then
    echo "ERROR: Verification code expired" >&2
    return 1
  fi

  # Verify code
  if [[ "$code" == "$stored_code" ]]; then
    # Mark as verified and enabled
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.mfa_sms SET
        enabled = TRUE,
        verified = TRUE,
        verified_at = NOW(),
        code = NULL,
        code_expires_at = NULL,
        attempts = 0
       WHERE user_id = '$user_id';" \
      >/dev/null 2>&1

    # Update user MFA status
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.users SET mfa_enabled = TRUE WHERE id = '$user_id';" \
      >/dev/null 2>&1

    echo "✓ SMS MFA enabled successfully" >&2
    return 0
  else
    # Increment attempts
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.mfa_sms SET attempts = attempts + 1 WHERE user_id = '$user_id';" \
      >/dev/null 2>&1

    echo "ERROR: Invalid verification code" >&2
    return 1
  fi
}

# Disable SMS MFA
# Usage: sms_disable <user_id>
sms_disable() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Disable SMS MFA
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_sms SET enabled = FALSE WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  echo "✓ SMS MFA disabled" >&2
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f sms_generate_code
export -f sms_send_code
export -f sms_enroll
export -f sms_send_verification
export -f sms_verify
export -f sms_disable
