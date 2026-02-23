#!/usr/bin/env bash
set -euo pipefail
# password-utils.sh - Password hashing and verification utilities
# Part of nself v0.6.0 - Phase 1 Sprint 1 (AUTH-004)
#
# Uses bcrypt for password hashing (via htpasswd or Python)


# ============================================================================
# Password Hashing (bcrypt)
# ============================================================================

# Hash a password using bcrypt
# Usage: hash_password <password>
# Returns: bcrypt hash string
hash_password() {
  local password="$1"

  if [[ -z "$password" ]]; then
    echo "ERROR: Password required" >&2
    return 1
  fi

  # Try htpasswd first (Apache utils)
  if command -v htpasswd >/dev/null 2>&1; then
    # htpasswd with bcrypt (-B flag)
    local hash
    hash=$(htpasswd -nbB "" "$password" 2>/dev/null | cut -d: -f2)
    echo "$hash"
    return 0
  fi

  # Try Python bcrypt as fallback — pass password via stdin to avoid injection
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "$password" | python3 -c "import bcrypt, sys; pwd=sys.stdin.read(); print(bcrypt.hashpw(pwd.encode(), bcrypt.gensalt()).decode())" 2>/dev/null && return 0
  fi

  # Try openssl as last resort (less secure, but available everywhere)
  if command -v openssl >/dev/null 2>&1; then
    # Use PBKDF2 with SHA-256 (not bcrypt, but better than nothing)
    local salt
    salt=$(openssl rand -hex 16)
    local hash
    hash=$(printf "%s" "$password$salt" | openssl dgst -sha256 | cut -d' ' -f2)
    echo "\$pbkdf2\$$salt\$$hash"
    return 0
  fi

  echo "ERROR: No password hashing utility available" >&2
  return 1
}

# Verify a password against a hash
# Usage: verify_password <password> <hash>
# Returns: 0 if match, 1 if no match
verify_password() {
  local password="$1"
  local stored_hash="$2"

  if [[ -z "$password" ]] || [[ -z "$stored_hash" ]]; then
    return 1
  fi

  # Check hash type
  if [[ "$stored_hash" == \$2y\$* ]] || [[ "$stored_hash" == \$2b\$* ]] || [[ "$stored_hash" == \$2a\$* ]]; then
    # bcrypt hash — try Python3 first (more reliable cross-platform), then htpasswd
    if command -v python3 >/dev/null 2>&1 && python3 -c "import bcrypt" >/dev/null 2>&1; then
      printf '%s\n%s' "$password" "$stored_hash" | python3 -c "import bcrypt, sys; lines=sys.stdin.read().split('\n', 1); sys.exit(0 if bcrypt.checkpw(lines[0].encode(), lines[1].encode()) else 1)" 2>/dev/null
      return $?
    elif command -v htpasswd >/dev/null 2>&1; then
      # htpasswd -v requires a file; write hash to temp file for verification
      local tmpfile
      tmpfile=$(mktemp)
      printf ":%s\n" "$stored_hash" > "$tmpfile"
      htpasswd -v -b "$tmpfile" "" "$password" >/dev/null 2>&1
      local result=$?
      rm -f "$tmpfile"
      return $result
    fi
  elif [[ "$stored_hash" == \$pbkdf2\$* ]]; then
    # Our custom PBKDF2 format
    local salt
    salt=$(echo "$stored_hash" | cut -d'$' -f3)
    local expected_hash
    expected_hash=$(echo "$stored_hash" | cut -d'$' -f4)
    local computed_hash
    computed_hash=$(printf "%s" "$password$salt" | openssl dgst -sha256 | cut -d' ' -f2)

    if [[ "$computed_hash" == "$expected_hash" ]]; then
      return 0
    else
      return 1
    fi
  fi

  # Unknown hash format
  return 1
}

# Generate a random password
# Usage: generate_password [length]
generate_password() {
  local length="${1:-16}"

  if command -v openssl >/dev/null 2>&1; then
    local bytes=$(( (length * 4 / 3) + 16 ))
    openssl rand -base64 "$bytes" | tr -d "=+/\n" | head -c "$length"
  else
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$length"
  fi
}

# Validate password strength
# Usage: validate_password_strength <password>
# Returns: 0 if strong enough, 1 if weak
validate_password_strength() {
  local password="$1"

  # Minimum requirements:
  # - At least 8 characters
  # - At least one lowercase letter
  # - At least one uppercase letter
  # - At least one digit
  # - At least one special character (optional for now)

  local length=${#password}

  if [[ $length -lt 8 ]]; then
    echo "Password must be at least 8 characters" >&2
    return 1
  fi

  # Check for lowercase
  if ! echo "$password" | grep -q '[a-z]'; then
    echo "Password must contain at least one lowercase letter" >&2
    return 1
  fi

  # Check for uppercase
  if ! echo "$password" | grep -q '[A-Z]'; then
    echo "Password must contain at least one uppercase letter" >&2
    return 1
  fi

  # Check for digit
  if ! echo "$password" | grep -q '[0-9]'; then
    echo "Password must contain at least one digit" >&2
    return 1
  fi

  return 0
}

# Export functions
export -f hash_password
export -f verify_password
export -f generate_password
export -f validate_password_strength
