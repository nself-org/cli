#!/usr/bin/env bash

# auth-manager.sh - Core authentication service manager
# Part of nself v0.6.0 - Phase 1 Sprint 1
#
# Responsibilities:
#   - Provider registration and management
#   - Session creation and validation
#   - Token generation and verification
#   - User authentication operations


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source dependencies
if ! declare -f log_error >/dev/null 2>&1; then
  source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
fi

if ! declare -f detect_environment >/dev/null 2>&1; then
  source "$NSELF_ROOT/src/lib/utils/env-detection.sh" 2>/dev/null || true
fi

# Source database utilities
if [[ -f "$NSELF_ROOT/src/lib/services/postgres.sh" ]]; then
  source "$NSELF_ROOT/src/lib/services/postgres.sh" 2>/dev/null || true
fi

# Source password utilities
if [[ -f "$SCRIPT_DIR/password-utils.sh" ]]; then
  source "$SCRIPT_DIR/password-utils.sh"
fi

# Source magic link utilities
if [[ -f "$SCRIPT_DIR/magic-link.sh" ]]; then
  source "$SCRIPT_DIR/magic-link.sh"
fi

# ============================================================================
# Constants
# ============================================================================

# Session token expiry (15 minutes)
readonly AUTH_SESSION_EXPIRY_SECONDS=900

# Refresh token expiry (30 days)
readonly AUTH_REFRESH_TOKEN_EXPIRY_SECONDS=2592000

# Supported auth methods
readonly AUTH_METHODS=("email" "phone" "oauth" "anonymous" "magic_link")

# Supported OAuth providers (Sprint 1 subset)
readonly OAUTH_PROVIDERS_SPRINT_1=("google" "github" "apple" "facebook" "twitter")

# Database schema
readonly AUTH_SCHEMA="auth"

# Provider registry (associative arrays simulated for Bash 3.2 compatibility)
# We'll use parallel arrays: PROVIDER_NAMES, PROVIDER_TYPES, PROVIDER_ENABLED
declare -a PROVIDER_NAMES=()
declare -a PROVIDER_TYPES=()
declare -a PROVIDER_ENABLED=()
declare -a PROVIDER_CONFIG=()

# ============================================================================
# Initialization
# ============================================================================

# Initialize auth service
# Sets up database schema, loads providers, etc.
auth_init() {
  log_info "Initializing auth service..."

  # Check if database is available
  if ! auth_check_database; then
    log_error "Database not available. Run 'nself start' first."
    return 1
  fi

  # Check if auth schema exists
  if ! auth_schema_exists; then
    log_info "Auth schema not found. Creating..."
    if ! auth_create_schema; then
      log_error "Failed to create auth schema"
      return 1
    fi
  fi

  # Load providers from database
  auth_load_providers

  log_info "✓ Auth service initialized"
  return 0
}

# Check if database is available
auth_check_database() {
  # Check if PostgreSQL container is running
  local postgres_running
  postgres_running=$(docker ps --filter "name=postgres" --format "{{.Names}}" 2>/dev/null || echo "")

  if [[ -z "$postgres_running" ]]; then
    return 1
  fi

  return 0
}

# Check if auth schema exists
auth_schema_exists() {
  local result
  result=$(docker exec -i "$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)" \
    psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = '${AUTH_SCHEMA}');" \
    2>/dev/null || echo "f")

  result=$(echo "$result" | tr -d ' \n')

  if [[ "$result" == "t" ]]; then
    return 0
  else
    return 1
  fi
}

# Create auth database schema
auth_create_schema() {
  log_info "Creating auth schema and tables..."

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # Create schema and tables
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL'
-- Create auth schema
CREATE SCHEMA IF NOT EXISTS auth;

-- Users table
CREATE TABLE IF NOT EXISTS auth.users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT UNIQUE,
  phone TEXT UNIQUE,
  email_verified BOOLEAN DEFAULT FALSE,
  phone_verified BOOLEAN DEFAULT FALSE,
  password_hash TEXT,
  mfa_enabled BOOLEAN DEFAULT FALSE,
  mfa_type TEXT[],
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  last_sign_in_at TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);

-- Sessions table
CREATE TABLE IF NOT EXISTS auth.sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  token TEXT UNIQUE NOT NULL,
  refresh_token TEXT UNIQUE,
  ip_address INET,
  user_agent TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  revoked_at TIMESTAMPTZ
);

-- Providers table (OAuth/SSO provider configs)
CREATE TABLE IF NOT EXISTS auth.providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT UNIQUE NOT NULL,
  type TEXT NOT NULL,
  enabled BOOLEAN DEFAULT TRUE,
  config JSONB NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- User providers (link users to OAuth accounts)
CREATE TABLE IF NOT EXISTS auth.user_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  provider_id UUID REFERENCES auth.providers(id) ON DELETE CASCADE,
  provider_user_id TEXT NOT NULL,
  provider_data JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(provider_id, provider_user_id)
);

-- MFA secrets table
CREATE TABLE IF NOT EXISTS auth.mfa_secrets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  secret_encrypted TEXT NOT NULL,
  backup_codes_encrypted TEXT[],
  webauthn_credential JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  last_used_at TIMESTAMPTZ
);

-- Refresh tokens table
CREATE TABLE IF NOT EXISTS auth.refresh_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  token TEXT UNIQUE NOT NULL,
  parent_token_id UUID REFERENCES auth.refresh_tokens(id),
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  revoked_at TIMESTAMPTZ
);

-- Audit log table
CREATE TABLE IF NOT EXISTS auth.audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id),
  event_type TEXT NOT NULL,
  event_data JSONB,
  ip_address INET,
  user_agent TEXT,
  success BOOLEAN DEFAULT TRUE,
  error_message TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON auth.sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON auth.sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON auth.sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_providers_user_id ON auth.user_providers(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON auth.audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON auth.audit_log(created_at);

-- Grant permissions to default user
GRANT ALL ON SCHEMA auth TO ${POSTGRES_USER:-postgres};
GRANT ALL ON ALL TABLES IN SCHEMA auth TO ${POSTGRES_USER:-postgres};
GRANT ALL ON ALL SEQUENCES IN SCHEMA auth TO ${POSTGRES_USER:-postgres};
EOSQL

  if [[ $? -eq 0 ]]; then
    log_info "✓ Auth schema created successfully"
    return 0
  else
    log_error "Failed to create auth schema"
    return 1
  fi
}

# ============================================================================
# Provider Management
# ============================================================================

# Load providers from database
auth_load_providers() {
  log_info "Loading auth providers..."

  # Clear existing provider arrays
  PROVIDER_NAMES=()
  PROVIDER_TYPES=()
  PROVIDER_ENABLED=()
  PROVIDER_CONFIG=()

  # Query database for providers
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_warning "PostgreSQL container not found, skipping provider load"
    return 1
  fi

  # Get providers as tab-separated values
  local providers
  providers=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT name, type, enabled, config::text FROM auth.providers;" 2>/dev/null || echo "")

  if [[ -z "$providers" ]]; then
    log_info "No providers configured yet"
    return 0
  fi

  # Parse providers (tab-separated: name, type, enabled, config)
  while IFS=$'\t' read -r name type enabled config; do
    # Trim whitespace
    name=$(echo "$name" | xargs)
    type=$(echo "$type" | xargs)
    enabled=$(echo "$enabled" | xargs)
    config=$(echo "$config" | xargs)

    if [[ -n "$name" ]]; then
      PROVIDER_NAMES+=("$name")
      PROVIDER_TYPES+=("$type")
      PROVIDER_ENABLED+=("$enabled")
      PROVIDER_CONFIG+=("$config")
    fi
  done <<<"$providers"

  log_info "✓ Loaded ${#PROVIDER_NAMES[@]} provider(s)"
  return 0
}

# List all providers
auth_list_providers() {
  # Reload from database to get fresh data
  auth_load_providers

  if [[ ${#PROVIDER_NAMES[@]} -eq 0 ]]; then
    echo "No providers configured"
    return 0
  fi

  # Print header
  printf "%-20s %-10s %-10s\n" "NAME" "TYPE" "STATUS"
  printf "%-20s %-10s %-10s\n" "----" "----" "------"

  # Print providers
  for i in "${!PROVIDER_NAMES[@]}"; do
    local status
    if [[ "${PROVIDER_ENABLED[$i]}" == "t" ]]; then
      status="enabled"
    else
      status="disabled"
    fi

    printf "%-20s %-10s %-10s\n" "${PROVIDER_NAMES[$i]}" "${PROVIDER_TYPES[$i]}" "$status"
  done

  return 0
}

# Add a new provider
# Usage: auth_add_provider <name> <type> <config_json>
auth_add_provider() {
  local name="$1"
  local type="$2"
  local config="${3:-{}}"

  if [[ -z "$name" ]] || [[ -z "$type" ]]; then
    log_error "Provider name and type required"
    return 1
  fi

  log_info "Adding provider: $name ($type)"

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # Insert provider into database
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.providers (name, type, config) VALUES ('$name', '$type', '$config'::jsonb) ON CONFLICT (name) DO UPDATE SET type = '$type', config = '$config'::jsonb, updated_at = NOW();" \
    >/dev/null 2>&1

  if [[ $? -eq 0 ]]; then
    log_info "✓ Provider '$name' added successfully"
    # Reload providers
    auth_load_providers
    return 0
  else
    log_error "Failed to add provider '$name'"
    return 1
  fi
}

# Remove a provider
# Usage: auth_remove_provider <name>
auth_remove_provider() {
  local name="$1"

  if [[ -z "$name" ]]; then
    log_error "Provider name required"
    return 1
  fi

  log_info "Removing provider: $name"

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # Delete provider from database
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.providers WHERE name = '$name';" \
    >/dev/null 2>&1

  if [[ $? -eq 0 ]]; then
    log_info "✓ Provider '$name' removed successfully"
    # Reload providers
    auth_load_providers
    return 0
  else
    log_error "Failed to remove provider '$name'"
    return 1
  fi
}

# Enable a provider
# Usage: auth_enable_provider <name>
auth_enable_provider() {
  local name="$1"

  if [[ -z "$name" ]]; then
    log_error "Provider name required"
    return 1
  fi

  log_info "Enabling provider: $name"

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # Update provider enabled status
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.providers SET enabled = TRUE, updated_at = NOW() WHERE name = '$name';" \
    >/dev/null 2>&1

  if [[ $? -eq 0 ]]; then
    log_info "✓ Provider '$name' enabled"
    # Reload providers
    auth_load_providers
    return 0
  else
    log_error "Failed to enable provider '$name'"
    return 1
  fi
}

# Disable a provider
# Usage: auth_disable_provider <name>
auth_disable_provider() {
  local name="$1"

  if [[ -z "$name" ]]; then
    log_error "Provider name required"
    return 1
  fi

  log_info "Disabling provider: $name"

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # Update provider enabled status
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.providers SET enabled = FALSE, updated_at = NOW() WHERE name = '$name';" \
    >/dev/null 2>&1

  if [[ $? -eq 0 ]]; then
    log_info "✓ Provider '$name' disabled"
    # Reload providers
    auth_load_providers
    return 0
  else
    log_error "Failed to disable provider '$name'"
    return 1
  fi
}

# Get provider config
# Usage: auth_get_provider_config <name>
auth_get_provider_config() {
  local name="$1"

  if [[ -z "$name" ]]; then
    log_error "Provider name required"
    return 1
  fi

  # Find provider in arrays
  for i in "${!PROVIDER_NAMES[@]}"; do
    if [[ "${PROVIDER_NAMES[$i]}" == "$name" ]]; then
      echo "${PROVIDER_CONFIG[$i]}"
      return 0
    fi
  done

  log_error "Provider '$name' not found"
  return 1
}

# ============================================================================
# Session Management (Placeholders for AUTH-004+)
# ============================================================================

# Create a new session
# Usage: auth_create_session <user_id> [ip_address] [user_agent]
auth_create_session() {
  local user_id="$1"
  local ip_address="${2:-}"
  local user_agent="${3:-}"

  # TODO (v1.0): Implement session management (AUTH-004)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-002)
  log_warning "auth_create_session not yet implemented (AUTH-004)"
  return 1
}

# Validate a session token
# Usage: auth_validate_session <token>
auth_validate_session() {
  local token="$1"

  # TODO (v1.0): Implement session management (AUTH-004)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-002)
  log_warning "auth_validate_session not yet implemented (AUTH-004)"
  return 1
}

# Revoke a session
# Usage: auth_revoke_session <session_id>
auth_revoke_session() {
  local session_id="$1"

  # TODO (v1.0): Implement session management (AUTH-004)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-002)
  log_warning "auth_revoke_session not yet implemented (AUTH-004)"
  return 1
}

# List user sessions
# Usage: auth_list_sessions <user_id>
auth_list_sessions() {
  local user_id="$1"

  # TODO (v1.0): Implement session management (AUTH-004)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-002)
  log_warning "auth_list_sessions not yet implemented (AUTH-004)"
  return 1
}

# ============================================================================
# Authentication Operations (Placeholders for AUTH-004+)
# ============================================================================

# Login with email/password
# Usage: auth_login_email <email> <password>
# Returns: JSON with session token on success
auth_login_email() {
  local email="$1"
  local password="$2"

  if [[ -z "$email" ]] || [[ -z "$password" ]]; then
    log_error "Email and password required"
    return 1
  fi

  log_info "Authenticating user: $email"

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found. Run 'nself start' first."
    return 1
  fi

  # Get user from database
  local user_data
  user_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id, password_hash, email_verified, disabled_at, deleted_at FROM auth.users WHERE email = '$email' LIMIT 1;" \
    2>/dev/null || echo "")

  if [[ -z "$user_data" ]]; then
    log_error "Invalid email or password"
    return 1
  fi

  # Parse user data (tab-separated: id, password_hash, email_verified, disabled_at, deleted_at)
  local user_id password_hash email_verified disabled_at deleted_at
  read -r user_id password_hash email_verified disabled_at deleted_at <<<"$(echo "$user_data" | xargs)"

  # Check if user is disabled or deleted
  if [[ -n "$disabled_at" ]] || [[ -n "$deleted_at" ]]; then
    log_error "Account is disabled"
    return 1
  fi

  # Verify password
  if ! verify_password "$password" "$password_hash"; then
    log_error "Invalid email or password"
    return 1
  fi

  # Create session
  local session_token
  session_token=$(auth_generate_token 32)

  local refresh_token
  refresh_token=$(auth_generate_token 48)

  local expires_at
  expires_at=$(date -u -d "+${AUTH_SESSION_EXPIRY_SECONDS} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${AUTH_SESSION_EXPIRY_SECONDS}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Insert session into database
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.sessions (user_id, token, refresh_token, expires_at) VALUES ('$user_id', '$session_token', '$refresh_token', '$expires_at'::timestamptz);" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    log_error "Failed to create session"
    return 1
  fi

  # Update last_sign_in_at
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.users SET last_sign_in_at = NOW() WHERE id = '$user_id';" \
    >/dev/null 2>&1

  # Log audit event
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.audit_log (user_id, event_type, success) VALUES ('$user_id', 'login', true);" \
    >/dev/null 2>&1

  log_info "✓ Login successful"

  # Return session info as JSON
  echo "{\"user_id\": \"$user_id\", \"token\": \"$session_token\", \"refresh_token\": \"$refresh_token\", \"expires_at\": \"$expires_at\"}"
  return 0
}

# Signup with email/password
# Usage: auth_signup_email <email> <password>
# Returns: JSON with user ID on success
auth_signup_email() {
  local email="$1"
  local password="$2"

  if [[ -z "$email" ]] || [[ -z "$password" ]]; then
    log_error "Email and password required"
    return 1
  fi

  log_info "Creating new user: $email"

  # Validate email format
  if ! echo "$email" | grep -qE '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$'; then
    log_error "Invalid email format"
    return 1
  fi

  # Validate password strength
  if ! validate_password_strength "$password"; then
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found. Run 'nself start' first."
    return 1
  fi

  # Check if user already exists
  local existing_user
  existing_user=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM auth.users WHERE email = '$email' LIMIT 1;" \
    2>/dev/null || echo "")

  if [[ -n "$existing_user" ]]; then
    log_error "User with this email already exists"
    return 1
  fi

  # Hash password
  local password_hash
  password_hash=$(hash_password "$password")

  if [[ -z "$password_hash" ]]; then
    log_error "Failed to hash password"
    return 1
  fi

  # Insert user into database
  local user_id
  user_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.users (email, password_hash) VALUES ('$email', '$password_hash') RETURNING id;" \
    2>/dev/null || echo "")

  user_id=$(echo "$user_id" | xargs)

  if [[ -z "$user_id" ]]; then
    log_error "Failed to create user"
    return 1
  fi

  # Log audit event
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.audit_log (user_id, event_type, success) VALUES ('$user_id', 'signup', true);" \
    >/dev/null 2>&1

  log_info "✓ User created successfully"

  # Return user info as JSON
  echo "{\"user_id\": \"$user_id\", \"email\": \"$email\"}"
  return 0
}

# Login with magic link
# Usage: auth_login_magic_link <email> [token]
# If token provided, verify it. Otherwise, create and send magic link.
auth_login_magic_link() {
  local email="${1:-}"
  local token="${2:-}"

  if [[ -z "$email" ]] && [[ -z "$token" ]]; then
    log_error "Email or token required"
    return 1
  fi

  # If token provided, verify and login
  if [[ -n "$token" ]]; then
    log_info "Verifying magic link..."

    local verified_email
    verified_email=$(verify_magic_link "$token")

    if [[ $? -ne 0 ]]; then
      log_error "Invalid or expired magic link"
      return 1
    fi

    # Get or create user
    local container
    container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

    local user_id
    user_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT id FROM auth.users WHERE email = '$verified_email' LIMIT 1;" \
      2>/dev/null | xargs || echo "")

    # Create user if doesn't exist (passwordless signup)
    if [[ -z "$user_id" ]]; then
      log_info "Creating new user via magic link..."
      user_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "INSERT INTO auth.users (email, email_verified) VALUES ('$verified_email', true) RETURNING id;" \
        2>/dev/null | xargs || echo "")
    else
      # Mark email as verified
      docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
        "UPDATE auth.users SET email_verified = true WHERE id = '$user_id';" \
        >/dev/null 2>&1
    fi

    # Create session (same as email/password login)
    local session_token
    session_token=$(auth_generate_token 32)

    local refresh_token
    refresh_token=$(auth_generate_token 48)

    local expires_at
    expires_at=$(date -u -d "+${AUTH_SESSION_EXPIRY_SECONDS} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
      date -u -v+${AUTH_SESSION_EXPIRY_SECONDS}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "INSERT INTO auth.sessions (user_id, token, refresh_token, expires_at) VALUES ('$user_id', '$session_token', '$refresh_token', '$expires_at'::timestamptz);" \
      >/dev/null 2>&1

    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.users SET last_sign_in_at = NOW() WHERE id = '$user_id';" \
      >/dev/null 2>&1

    log_info "✓ Magic link login successful"

    echo "{\"user_id\": \"$user_id\", \"token\": \"$session_token\", \"refresh_token\": \"$refresh_token\", \"expires_at\": \"$expires_at\"}"
    return 0

  else
    # Create and send magic link
    log_info "Sending magic link to: $email"

    local magic_token
    magic_token=$(create_magic_link "$email")

    if [[ $? -ne 0 ]]; then
      log_error "Failed to create magic link"
      return 1
    fi

    # In production, this would send an email with the link
    # For now, just display the token for development
    log_info "✓ Magic link created"
    log_info "Token: $magic_token"
    log_info "Use: nself auth login --magic-link --token=$magic_token"

    echo "{\"email\": \"$email\", \"token\": \"$magic_token\", \"message\": \"Magic link sent to email\"}"
    return 0
  fi
}

# Login with phone/SMS
# Usage: auth_login_phone <phone> [otp_code]
# If otp_code provided, verify it. Otherwise, send OTP to phone.
auth_login_phone() {
  local phone="$1"
  local otp_code="${2:-}"

  if [[ -z "$phone" ]]; then
    log_error "Phone number required"
    return 1
  fi

  # Normalize phone number (basic validation)
  if ! echo "$phone" | grep -qE '^\+?[1-9][0-9]{7,14}$'; then
    log_error "Invalid phone number format (use E.164: +1234567890)"
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # If OTP code provided, verify and login
  if [[ -n "$otp_code" ]]; then
    log_info "Verifying OTP code..."

    # Create phone_otps table if it doesn't exist
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.phone_otps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  phone TEXT NOT NULL,
  code TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_phone_otps_phone ON auth.phone_otps(phone);
EOSQL

    # Verify OTP
    local otp_data
    otp_data=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT phone, expires_at, verified_at FROM auth.phone_otps WHERE phone = '$phone' AND code = '$otp_code' ORDER BY created_at DESC LIMIT 1;" \
      2>/dev/null || echo "")

    if [[ -z "$otp_data" ]]; then
      log_error "Invalid OTP code"
      return 1
    fi

    local verified_phone expires_at verified_at
    read -r verified_phone expires_at verified_at <<<"$(echo "$otp_data" | xargs)"

    # Check if already used
    if [[ -n "$verified_at" ]]; then
      log_error "OTP code already used"
      return 1
    fi

    # Check if expired (5 minutes)
    local now
    now=$(date -u "+%Y-%m-%d %H:%M:%S")

    if [[ "$now" > "$expires_at" ]]; then
      log_error "OTP code expired"
      return 1
    fi

    # Mark as verified
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "UPDATE auth.phone_otps SET verified_at = NOW() WHERE phone = '$phone' AND code = '$otp_code';" \
      >/dev/null 2>&1

    # Get or create user
    local user_id
    user_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
      "SELECT id FROM auth.users WHERE phone = '$phone' LIMIT 1;" \
      2>/dev/null | xargs || echo "")

    if [[ -z "$user_id" ]]; then
      user_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
        "INSERT INTO auth.users (phone, phone_verified) VALUES ('$phone', true) RETURNING id;" \
        2>/dev/null | xargs || echo "")
    else
      docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
        "UPDATE auth.users SET phone_verified = true WHERE id = '$user_id';" \
        >/dev/null 2>&1
    fi

    # Create session
    local session_token
    session_token=$(auth_generate_token 32)

    local refresh_token
    refresh_token=$(auth_generate_token 48)

    local session_expires_at
    session_expires_at=$(date -u -d "+${AUTH_SESSION_EXPIRY_SECONDS} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
      date -u -v+${AUTH_SESSION_EXPIRY_SECONDS}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "INSERT INTO auth.sessions (user_id, token, refresh_token, expires_at) VALUES ('$user_id', '$session_token', '$refresh_token', '$session_expires_at'::timestamptz);" \
      >/dev/null 2>&1

    log_info "✓ Phone login successful"

    echo "{\"user_id\": \"$user_id\", \"token\": \"$session_token\", \"refresh_token\": \"$refresh_token\", \"expires_at\": \"$session_expires_at\"}"
    return 0

  else
    # Generate and send OTP code
    log_info "Sending OTP to: $phone"

    # Generate 6-digit OTP
    local otp
    otp=$(openssl rand -hex 3 2>/dev/null | cut -c1-6 || printf "%06d" $((RANDOM % 1000000)))

    # Calculate expiry (5 minutes)
    local otp_expires_at
    otp_expires_at=$(date -u -d "+300 seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
      date -u -v+300S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

    # Create table if needed
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.phone_otps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  phone TEXT NOT NULL,
  code TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_phone_otps_phone ON auth.phone_otps(phone);
EOSQL

    # Store OTP
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "INSERT INTO auth.phone_otps (phone, code, expires_at) VALUES ('$phone', '$otp', '$otp_expires_at'::timestamptz);" \
      >/dev/null 2>&1

    # In production, this would send SMS via Twilio/AWS SNS
    # For now, display OTP for development
    log_info "✓ OTP code generated"
    log_info "OTP: $otp (valid for 5 minutes)"
    log_info "Use: nself auth login --phone=$phone --otp=$otp"

    echo "{\"phone\": \"$phone\", \"message\": \"OTP sent to phone\", \"otp_dev\": \"$otp\"}"
    return 0
  fi
}

# Login anonymously
# Usage: auth_login_anonymous
# Creates anonymous user account with no credentials
auth_login_anonymous() {
  log_info "Creating anonymous session..."

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found"
    return 1
  fi

  # Create anonymous user
  local user_id
  user_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.users (email) VALUES (NULL) RETURNING id;" \
    2>/dev/null | xargs || echo "")

  if [[ -z "$user_id" ]]; then
    log_error "Failed to create anonymous user"
    return 1
  fi

  # Create session
  local session_token
  session_token=$(auth_generate_token 32)

  local refresh_token
  refresh_token=$(auth_generate_token 48)

  local expires_at
  expires_at=$(date -u -d "+${AUTH_SESSION_EXPIRY_SECONDS} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${AUTH_SESSION_EXPIRY_SECONDS}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.sessions (user_id, token, refresh_token, expires_at) VALUES ('$user_id', '$session_token', '$refresh_token', '$expires_at'::timestamptz);" \
    >/dev/null 2>&1

  # Log audit event
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.audit_log (user_id, event_type, success) VALUES ('$user_id', 'anonymous_login', true);" \
    >/dev/null 2>&1

  log_info "✓ Anonymous session created"

  echo "{\"user_id\": \"$user_id\", \"token\": \"$session_token\", \"refresh_token\": \"$refresh_token\", \"expires_at\": \"$expires_at\", \"anonymous\": true}"
  return 0
}

# Login with OAuth provider
# Usage: auth_login_oauth <provider>
auth_login_oauth() {
  local provider="$1"

  # TODO (v1.0+): Implement OAuth login flow (OAUTH-003+)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-004)
  log_warning "auth_login_oauth not yet implemented (OAUTH-003+)"
  return 1
}

# ============================================================================
# Utility Functions
# ============================================================================

# Generate a random token
# Usage: auth_generate_token [length]
auth_generate_token() {
  local length="${1:-32}"
  openssl rand -hex "$length" 2>/dev/null || head -c "$length" /dev/urandom | xxd -p
}

# Hash a password
# Usage: auth_hash_password <password>
auth_hash_password() {
  local password="$1"

  # TODO (v1.0): Implement bcrypt password hashing (SECURITY CRITICAL)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-003)
  log_warning "auth_hash_password not yet implemented"
  return 1
}

# Verify a password
# Usage: auth_verify_password <password> <hash>
auth_verify_password() {
  local password="$1"
  local hash="$2"

  # TODO (v1.0): Implement bcrypt password verification (SECURITY CRITICAL)
  # See: .ai/roadmap/v1.0/deferred-features.md (AUTH-003)
  log_warning "auth_verify_password not yet implemented"
  return 1
}

# ============================================================================
# Export functions
# ============================================================================

# Make functions available when sourced
export -f auth_init
export -f auth_check_database
export -f auth_schema_exists
export -f auth_create_schema
export -f auth_load_providers
export -f auth_list_providers
export -f auth_add_provider
export -f auth_remove_provider
export -f auth_enable_provider
export -f auth_disable_provider
export -f auth_get_provider_config
export -f auth_create_session
export -f auth_validate_session
export -f auth_revoke_session
export -f auth_list_sessions
export -f auth_login_email
export -f auth_signup_email
export -f auth_login_magic_link
export -f auth_login_phone
export -f auth_login_anonymous
export -f auth_login_oauth
export -f auth_generate_token
export -f auth_hash_password
export -f auth_verify_password

# If executed directly (for testing)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  auth_init
fi
