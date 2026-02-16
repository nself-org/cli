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

# Export functions
export -f generate_secure_password
export -f sanitize_input
export -f validate_container_name
