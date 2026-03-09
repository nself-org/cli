#!/usr/bin/env bash
set -euo pipefail


# security.sh - Security utility functions

# Generate secure password
generate_secure_password() {
  local length="${1:-25}"
  local password=""

  # Try OpenSSL first (most secure)
  if command -v openssl >/dev/null 2>&1; then
    password="$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-$length)"
  # Fallback to /dev/urandom
  elif [[ -r /dev/urandom ]]; then
    password="$(LC_ALL=C tr -dc 'A-Za-z0-9!@#$%^&*' </dev/urandom | head -c $length)"
  # Last resort - use $RANDOM
  else
    local chars='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*'
    for ((i = 0; i < length; i++)); do
      password="${password}${chars:RANDOM%${#chars}:1}"
    done
  fi

  # Validate password strength
  if [[ ${#password} -lt $length ]]; then
    log_warning "Generated password may be shorter than requested"
  fi

  echo "$password"
}

# Sanitize user input to prevent injection
sanitize_input() {
  local input="$1"
  # Remove dangerous characters
  echo "$input" | sed 's/[;&|`$()]//g'
}

# Validate container name
validate_container_name() {
  local name="$1"
  # Docker container names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*
  if [[ "$name" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]]; then
    return 0
  else
    return 1
  fi
}

# Redact known secret patterns from CLI output strings
# Usage: redacted=$(redact_secrets "$some_output")
# Replaces: nself_pro_* license keys, bearer tokens, API keys, passwords
# Bash 3.2+ compatible (no regex lookbehind, no declare -A)
redact_secrets() {
  local input="$1"
  local output="$input"

  # nself license keys: nself_pro_ + 32+ chars
  output=$(printf '%s' "$output" | sed 's/nself_pro_[A-Za-z0-9_-][A-Za-z0-9_-]*/nself_pro_[REDACTED]/g')

  # Bearer tokens in Authorization headers
  output=$(printf '%s' "$output" | sed 's/Bearer[[:space:]]*[A-Za-z0-9._-][A-Za-z0-9._-]*/Bearer [REDACTED]/g')

  # Generic API key patterns (sk-*, pk-*, rk-* — Stripe, OpenAI, etc.)
  output=$(printf '%s' "$output" | sed 's/\bsk-[A-Za-z0-9][A-Za-z0-9_-]*/sk-[REDACTED]/g')
  output=$(printf '%s' "$output" | sed 's/\bpk_live_[A-Za-z0-9][A-Za-z0-9_-]*/pk_live_[REDACTED]/g')
  output=$(printf '%s' "$output" | sed 's/\bpk_test_[A-Za-z0-9][A-Za-z0-9_-]*/pk_test_[REDACTED]/g')
  output=$(printf '%s' "$output" | sed 's/\brk_live_[A-Za-z0-9][A-Za-z0-9_-]*/rk_live_[REDACTED]/g')

  # env var assignments for known secret var names (VALUE redacted, not KEY)
  # Pattern: VARNAME=<value> where VARNAME contains key/secret/token/password
  output=$(printf '%s' "$output" | sed 's/\([A-Z_]*\(KEY\|SECRET\|TOKEN\|PASSWORD\|PASSWD\|API_KEY\|APIKEY\)[A-Z_]*\)=[^ ]\{1,\}/\1=[REDACTED]/g')

  printf '%s' "$output"
}

# Export functions
export -f generate_secure_password
export -f sanitize_input
export -f validate_container_name
export -f redact_secrets
