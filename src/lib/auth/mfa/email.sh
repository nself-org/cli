#!/usr/bin/env bash
# email.sh - Email-based MFA implementation (MFA-003)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Implements email-based multi-factor authentication
# Uses nself email service (MailPit in dev, SMTP in production)


# Email MFA configuration
readonly EMAIL_CODE_LENGTH=6

set -euo pipefail

readonly EMAIL_CODE_EXPIRY=600 # 10 minutes
readonly EMAIL_MAX_ATTEMPTS=5
readonly EMAIL_RATE_LIMIT=120 # 2 minutes between sends

# ============================================================================
# Email Code Generation
# ============================================================================

# Generate email verification code
# Usage: email_generate_code
# Returns: 6-digit numeric code
email_generate_code() {
  # Generate 6-digit random code
  if command -v openssl >/dev/null 2>&1; then
    printf "%06d" $((10#$(openssl rand -hex 3 | tr -d '\n' | head -c 6) % 1000000))
  else
    printf "%06d" $((RANDOM % 1000000))
  fi
}

# ============================================================================
# Email Sending
# ============================================================================

# Send email via MailPit (development)
# Usage: email_send_mailpit <email> <code>
email_send_mailpit() {
  local email="$1"
  local code="$2"

  local subject="Your verification code"
  local body="Your ${TOTP_ISSUER:-nself} verification code is: ${code}. Valid for $((EMAIL_CODE_EXPIRY / 60)) minutes."

  # In development, just print to console
  cat >&2 <<EOF

╔══════════════════════════════════════════╗
║        EMAIL VERIFICATION CODE           ║
╠══════════════════════════════════════════╣
║  To:      ${email}
║  Subject: ${subject}
║  Code:    ${code}
║  Valid:   $((EMAIL_CODE_EXPIRY / 60)) minutes
╚══════════════════════════════════════════╝

EOF

  return 0
}

# Send email via SMTP
# Usage: email_send_smtp <email> <code>
email_send_smtp() {
  local email="$1"
  local code="$2"

  local smtp_host="${SMTP_HOST:-localhost}"
  local smtp_port="${SMTP_PORT:-1025}"
  local smtp_user="${SMTP_USER:-}"
  local smtp_pass="${SMTP_PASSWORD:-}"
  local from_email="${SMTP_FROM:-noreply@nself.org}"

  local subject="Your verification code"
  local body="Your ${TOTP_ISSUER:-nself} verification code is: ${code}. Valid for $((EMAIL_CODE_EXPIRY / 60)) minutes."

  # Use Python to send email (most portable)
  if command -v python3 >/dev/null 2>&1; then
    python3 <<EOF
import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart

msg = MIMEMultipart()
msg['From'] = '${from_email}'
msg['To'] = '${email}'
msg['Subject'] = '${subject}'
msg.attach(MIMEText('${body}', 'plain'))

try:
    with smtplib.SMTP('${smtp_host}', ${smtp_port}) as server:
        if '${smtp_user}' and '${smtp_pass}':
            server.login('${smtp_user}', '${smtp_pass}')
        server.send_message(msg)
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    exit(1)
EOF
    return $?
  fi

  echo "ERROR: Python 3 required for SMTP email sending" >&2
  return 1
}

# Send email code (auto-detects mode)
# Usage: email_send_code <email> <code>
email_send_code() {
  local email="$1"
  local code="$2"

  local mode="${EMAIL_MODE:-dev}"

  case "$mode" in
    dev | development | mailpit)
      email_send_mailpit "$email" "$code"
      ;;
    smtp | production)
      email_send_smtp "$email" "$code"
      ;;
    *)
      echo "ERROR: Unknown email mode: $mode" >&2
      return 1
      ;;
  esac
}

# ============================================================================
# Email MFA Enrollment
# ============================================================================

# Enroll email for MFA
# Usage: email_mfa_enroll <user_id> <email>
# Returns: 0 if successful
email_mfa_enroll() {
  local user_id="$1"
  local email="$2"

  # Validate email format (basic validation)
  if ! echo "$email" | grep -qE '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$'; then
    echo "ERROR: Invalid email format" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create mfa_email table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.mfa_email (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  email TEXT NOT NULL,
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
CREATE INDEX IF NOT EXISTS idx_mfa_email_user_id ON auth.mfa_email(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_email_email ON auth.mfa_email(email);
EOSQL

  # Store email (not yet verified)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.mfa_email (user_id, email, enabled, verified)
     VALUES ('$user_id', '$email', FALSE, FALSE)
     ON CONFLICT (user_id) DO UPDATE SET
       email = EXCLUDED.email,
       enabled = FALSE,
       verified = FALSE,
       created_at = NOW();" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to enroll email" >&2
    return 1
  fi

  echo "✓ Email enrolled for MFA" >&2
  return 0
}

# Send email verification code
# Usage: email_send_verification <user_id>
# Returns: 0 if sent successfully
email_send_verification() {
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
    "SELECT EXTRACT(EPOCH FROM (NOW() - last_sent_at)) FROM auth.mfa_email WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -n "$last_sent" ]] && [[ "${last_sent%.*}" -lt "$EMAIL_RATE_LIMIT" ]]; then
    local wait_time=$((EMAIL_RATE_LIMIT - ${last_sent%.*}))
    echo "ERROR: Please wait ${wait_time} seconds before requesting another code" >&2
    return 1
  fi

  # Get email address
  local email
  email=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT email FROM auth.mfa_email WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$email" ]]; then
    echo "ERROR: Email not enrolled" >&2
    return 1
  fi

  # Generate code
  local code
  code=$(email_generate_code)

  # Calculate expiry
  local expires_at
  expires_at=$(date -u -d "+${EMAIL_CODE_EXPIRY} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${EMAIL_CODE_EXPIRY}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Store code
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_email SET
      code = '$code',
      code_expires_at = '$expires_at'::timestamptz,
      attempts = 0,
      last_sent_at = NOW()
     WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  # Send email
  if email_send_code "$email" "$code"; then
    echo "✓ Verification code sent to ${email}" >&2
    return 0
  else
    echo "ERROR: Failed to send email" >&2
    return 1
  fi
}

# Verify email code and enable MFA
# Usage: email_verify <user_id> <code>
# Returns: 0 if valid
email_verify() {
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
  local email_data
  email_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT code, code_expires_at, attempts FROM auth.mfa_email WHERE user_id = '$user_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$email_data" ]]; then
    echo "ERROR: No verification code found" >&2
    return 1
  fi

  local stored_code expires_at attempts
  read -r stored_code expires_at attempts <<<"$email_data"

  # Check max attempts
  if [[ "$attempts" -ge "$EMAIL_MAX_ATTEMPTS" ]]; then
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
      "UPDATE auth.mfa_email SET
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

    echo "✓ Email MFA enabled successfully" >&2
    return 0
  else
    # Increment attempts
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.mfa_email SET attempts = attempts + 1 WHERE user_id = '$user_id';" \
      >/dev/null 2>&1

    echo "ERROR: Invalid verification code" >&2
    return 1
  fi
}

# Disable email MFA
# Usage: email_mfa_disable <user_id>
email_mfa_disable() {
  local user_id="$1"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Disable email MFA
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.mfa_email SET enabled = FALSE WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  echo "✓ Email MFA disabled" >&2
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f email_generate_code
export -f email_send_code
export -f email_mfa_enroll
export -f email_send_verification
export -f email_verify
export -f email_mfa_disable
