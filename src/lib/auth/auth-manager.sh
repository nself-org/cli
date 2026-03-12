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

# Create a new session in auth.sessions and return the session token.
# The session record is written directly to PostgreSQL (same DB used by
# auth_login_email). No local file store is used — the DB is the source
# of truth.
#
# Usage: auth_create_session <user_id> [ip_address] [user_agent]
# Returns: session token (plain text) on stdout, exit 0 on success
auth_create_session() {
  local user_id="$1"
  local ip_address="${2:-}"
  local user_agent="${3:-}"

  if [[ -z "$user_id" ]]; then
    log_error "auth_create_session: user_id required"
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found. Run 'nself start' first."
    return 1
  fi

  # Generate a cryptographically random session token (64 hex chars = 32 bytes)
  local session_token
  session_token=$(openssl rand -hex 32 2>/dev/null || \
    dd if=/dev/urandom bs=32 count=1 2>/dev/null | \
    od -An -tx1 | tr -d ' \\n')

  if [[ -z "$session_token" ]]; then
    log_error "Failed to generate session token"
    return 1
  fi

  # Compute expiry timestamp (UTC) using portable date arithmetic.
  # macOS Bash 3.2 ships with BSD date; Linux ships with GNU date.
  # Try GNU date first, fall back to BSD date.
  local expires_at
  expires_at=$(date -u -d "+${AUTH_SESSION_EXPIRY_SECONDS} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null || \
               date -u -v+${AUTH_SESSION_EXPIRY_SECONDS}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  if [[ -z "$expires_at" ]]; then
    log_error "Failed to compute session expiry"
    return 1
  fi

  # Escape single quotes in user-agent to prevent SQL injection.
  local safe_ua
  safe_ua=$(printf "%s" "$user_agent" | sed "s/\'/''/g")
  local safe_ip="${ip_address:-NULL}"

  # Build the INSERT, handling optional ip_address (stored as INET — pass NULL
  # when empty because PostgreSQL rejects an empty string for INET columns).
  local insert_sql
  if [[ -z "$ip_address" ]]; then
    insert_sql="INSERT INTO auth.sessions (user_id, token, expires_at, user_agent)
      VALUES ('$user_id', '$session_token', '$expires_at'::timestamptz, '$safe_ua');"
  else
    insert_sql="INSERT INTO auth.sessions (user_id, token, expires_at, ip_address, user_agent)
      VALUES ('$user_id', '$session_token', '$expires_at'::timestamptz, '$ip_address'::inet, '$safe_ua');"
  fi

  if ! docker exec -i "$container" psql \
      -U "${POSTGRES_USER:-postgres}" \
      -d "${POSTGRES_DB:-nself_db}" \
      -c "$insert_sql" >/dev/null 2>&1; then
    log_error "Failed to insert session into database"
    return 1
  fi

  # Emit just the token so callers can capture it: token=$(auth_create_session ...)
  printf "%s" "$session_token"
  return 0
}

# Validate a session token against auth.sessions.
# Checks that the token exists, has not been revoked, and has not expired.
#
# Usage: auth_validate_session <token>
# Returns: JSON row on stdout and exit 0 when valid; exit 1 when invalid/expired.
auth_validate_session() {
  local token="$1"

  if [[ -z "$token" ]]; then
    log_error "auth_validate_session: token required"
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found. Run 'nself start' first."
    return 1
  fi

  # A session is valid when:
  #   - revoked_at IS NULL  (not explicitly revoked)
  #   - expires_at > NOW()  (not expired)
  local row
  row=$(docker exec -i "$container" psql \
    -U "${POSTGRES_USER:-postgres}" \
    -d "${POSTGRES_DB:-nself_db}" \
    -t -A -c \
    "SELECT row_to_json(s)
     FROM auth.sessions s
     WHERE token = '$token'
       AND revoked_at IS NULL
       AND expires_at > NOW()
     LIMIT 1;" 2>/dev/null || echo "")

  row=$(printf "%s" "$row" | tr -d ' \r\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' )

  if [[ -z "$row" ]]; then
    return 1
  fi

  printf "%s" "$row"
  return 0
}

# Revoke a session by setting revoked_at to NOW().
# The session record is kept for audit purposes; it is simply marked inactive.
#
# Usage: auth_revoke_session <session_id_or_token>
# Accepts either the session UUID (id) or the opaque session token.
auth_revoke_session() {
  local session_id="$1"

  if [[ -z "$session_id" ]]; then
    log_error "auth_revoke_session: session_id or token required"
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found. Run 'nself start' first."
    return 1
  fi

  # Match on either the UUID primary key (id) or the opaque token column.
  # This lets callers pass whichever they have available.
  local rows_affected
  rows_affected=$(docker exec -i "$container" psql \
    -U "${POSTGRES_USER:-postgres}" \
    -d "${POSTGRES_DB:-nself_db}" \
    -t -A -c \
    "UPDATE auth.sessions
     SET revoked_at = NOW()
     WHERE revoked_at IS NULL
       AND (id::text = '$session_id' OR token = '$session_id');" 2>/dev/null || echo "0")

  # psql UPDATE output: "UPDATE N" — extract N
  rows_affected=$(printf "%s" "$rows_affected" | grep -o '[0-9]*' | tail -1)

  if [[ "${rows_affected:-0}" -eq 0 ]]; then
    log_warning "No active session found for: $session_id"
    return 1
  fi

  log_info "Session revoked: $session_id"
  return 0
}

# List all active (non-expired, non-revoked) sessions for a user.
# Prints one line per session with: session_id, created_at, expires_at,
# ip_address, user_agent (truncated to 40 chars).
#
# Usage: auth_list_sessions <user_id>
auth_list_sessions() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    log_error "auth_list_sessions: user_id required"
    return 1
  fi

  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    log_error "PostgreSQL container not found. Run 'nself start' first."
    return 1
  fi

  local results
  results=$(docker exec -i "$container" psql \
    -U "${POSTGRES_USER:-postgres}" \
    -d "${POSTGRES_DB:-nself_db}" \
    -t -A -c \
    "SELECT id, created_at::text, expires_at::text,
            COALESCE(ip_address::text, \'(none)\'),
            LEFT(COALESCE(user_agent, \'(none)\'), 40)
     FROM auth.sessions
     WHERE user_id = '$user_id'
       AND revoked_at IS NULL
       AND expires_at > NOW()
     ORDER BY created_at DESC;" 2>/dev/null || echo "")

  if [[ -z "$results" ]]; then
    log_info "No active sessions for user: $user_id"
    return 0
  fi

  # Print header
  printf "  %-36s  %-19s  %-19s  %-15s  %s\n" \
    "Session ID" "Created At" "Expires At" "IP" "User Agent"
  printf "  %s\n" "$(printf '%0.s-' $(seq 1 110))"

  # Print each row (psql -A -t outputs pipe-separated values with -F)
  while IFS='|' read -r sid created expires ip ua; do
    printf "  %-36s  %-19s  %-19s  %-15s  %s\n" \
      "$sid" "$created" "$expires" "$ip" "$ua"
  done <<EOF_SESSIONS
$results
EOF_SESSIONS

  return 0
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

# Find a free TCP port in a given range
# Usage: _oauth_find_free_port <start> <end>
_oauth_find_free_port() {
  local start="${1:-5555}"
  local end="${2:-5600}"
  local port="$start"
  while [[ "$port" -le "$end" ]]; do
    if ! nc -z localhost "$port" 2>/dev/null; then
      printf "%d" "$port"
      return 0
    fi
    port=$((port + 1))
  done
  return 1
}

# URL-encode a string — Bash 3.2+ compatible, no external deps
# Usage: _oauth_url_encode <string>
_oauth_url_encode() {
  local str="$1"
  local encoded=""
  local i=0
  local c=""
  while [[ "$i" -lt "${#str}" ]]; do
    c="${str:$i:1}"
    case "$c" in
      [A-Za-z0-9._~-]) encoded="${encoded}${c}" ;;
      *) encoded="${encoded}$(printf '%%%02X' "'$c")" ;;
    esac
    i=$((i + 1))
  done
  printf "%s" "$encoded"
}

# One-shot HTTP callback server — reads one request, writes HTML response, prints request
# Usage: _oauth_capture_callback <port> <timeout_secs>
_oauth_capture_callback() {
  local port="$1"
  local timeout_secs="${2:-30}"

  local html_body
  html_body='<html><head><title>Login complete</title></head><body><h2>Login successful!</h2><p>You can close this tab and return to your terminal.</p><script>window.close();</script></body></html>'
  local response
  response="$(printf "HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nConnection: close\r\n\r\n%s" "$html_body")"

  local request_file
  request_file=$(mktemp /tmp/nself_oauth_XXXXXX)

  # Source platform-compat for safe_timeout
  local _compat
  _compat="${NSELF_ROOT}/src/lib/utils/platform-compat.sh"
  if [[ -f "$_compat" ]] && ! declare -f safe_timeout >/dev/null 2>&1; then
    source "$_compat" 2>/dev/null || true
  fi

  # Determine nc listen syntax (OpenBSD nc: -l PORT; traditional nc: -l -p PORT)
  if nc -h 2>&1 | grep -q '\-p'; then
    # Traditional nc (some Linux distros)
    printf "%s" "$response" | safe_timeout "$timeout_secs" nc -l -p "$port" > "$request_file" 2>/dev/null || true
  else
    # OpenBSD nc / macOS nc
    printf "%s" "$response" | safe_timeout "$timeout_secs" nc -l "$port" > "$request_file" 2>/dev/null || true
  fi

  cat "$request_file"
  rm -f "$request_file"
}

# Login with OAuth provider — browser-based PKCE flow
# Usage: auth_login_oauth <provider>
auth_login_oauth() {
  local provider="${1:-google}"

  # Validate provider
  case "$provider" in
    google|github|apple|facebook|twitter|gitlab|bitbucket) ;;
    *)
      log_error "Unsupported OAuth provider: $provider"
      log_info "Supported: google github apple facebook twitter gitlab bitbucket"
      return 1
      ;;
  esac

  # Source platform-compat
  local _compat="${NSELF_ROOT}/src/lib/utils/platform-compat.sh"
  if [[ -f "$_compat" ]] && ! declare -f safe_timeout >/dev/null 2>&1; then
    source "$_compat" 2>/dev/null || true
  fi

  # Load environment
  if declare -f load_env_with_priority >/dev/null 2>&1; then
    load_env_with_priority 2>/dev/null || true
  fi

  local base_domain="${BASE_DOMAIN:-local.nself.org}"
  local auth_url="${AUTH_URL:-https://auth.${base_domain}}"
  local nself_auth_dir="${HOME}/.nself/auth"
  mkdir -p "$nself_auth_dir"
  chmod 700 "$nself_auth_dir"

  # Check nc is available
  if ! command -v nc >/dev/null 2>&1; then
    log_error "netcat (nc) is required for OAuth login but was not found"
    log_info "Install with: brew install netcat (macOS) or apt-get install netcat (Linux)"
    return 1
  fi

  # Find a free port for the local callback server
  local callback_port
  callback_port=$(_oauth_find_free_port 5555 5600)
  if [[ -z "$callback_port" ]]; then
    log_error "No free port available for OAuth callback (tried 5555-5600)"
    return 1
  fi

  local callback_url
  callback_url="http://localhost:${callback_port}/callback"
  local encoded_callback
  encoded_callback=$(_oauth_url_encode "$callback_url")

  # Build nHost Auth OAuth URL
  local oauth_url="${auth_url}/signin/provider/${provider}?redirectTo=${encoded_callback}"

  log_info "Opening browser for ${provider} authentication..."
  printf "  URL: %s\n" "$oauth_url"
  printf "\n"
  log_info "If the browser did not open, visit the URL above manually."
  printf "\n"

  # Open browser (cross-platform)
  if command -v open >/dev/null 2>&1; then
    open "$oauth_url" 2>/dev/null &
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$oauth_url" 2>/dev/null &
  fi

  log_info "Waiting for OAuth callback (timeout: 30s)..."

  # Listen for one HTTP request on the callback port
  local raw_request
  raw_request=$(_oauth_capture_callback "$callback_port" 30)

  if [[ -z "$raw_request" ]]; then
    log_error "OAuth callback timed out. Authentication cancelled."
    return 1
  fi

  # Extract refreshToken from the callback request
  # nHost Auth sends: GET /callback?refreshToken=xxx&type=signinProvider HTTP/1.1
  local refresh_token
  refresh_token=$(printf "%s" "$raw_request" | grep -o 'refreshToken=[^& \n\r]*' | head -1 | cut -d= -f2-)

  if [[ -z "$refresh_token" ]]; then
    # Check if nHost sent an error
    local error_param
    error_param=$(printf "%s" "$raw_request" | grep -o 'error=[^& \n\r]*' | head -1 | cut -d= -f2-)
    if [[ -n "$error_param" ]]; then
      log_error "OAuth provider returned error: $error_param"
    else
      log_error "No token received from OAuth provider. Please try again."
    fi
    return 1
  fi

  log_info "Exchanging token..."

  # Exchange refresh token for a full session via nHost Auth /token endpoint
  local token_response
  token_response=$(curl -s -X POST "${auth_url}/token" \
    -H "Content-Type: application/json" \
    -d "{\"refreshToken\": \"${refresh_token}\"}" 2>/dev/null)

  if [[ -z "$token_response" ]]; then
    log_error "Failed to exchange OAuth token — could not reach auth service"
    return 1
  fi

  # Parse response fields (Bash 3.2+ compatible — no jq required)
  local access_token
  access_token=$(printf "%s" "$token_response" | grep -o '"accessToken":"[^"]*"' | head -1 | cut -d'"' -f4)

  local new_refresh_token
  new_refresh_token=$(printf "%s" "$token_response" | grep -o '"refreshToken":"[^"]*"' | head -1 | cut -d'"' -f4)

  local user_id
  user_id=$(printf "%s" "$token_response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

  local user_email
  user_email=$(printf "%s" "$token_response" | grep -o '"email":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [[ -z "$access_token" ]]; then
    # Try to extract an error message
    local err_msg
    err_msg=$(printf "%s" "$token_response" | grep -o '"message":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_error "Token exchange failed${err_msg:+: $err_msg}"
    return 1
  fi

  # Use new refresh token if provided, fall back to original
  local stored_refresh="${new_refresh_token:-$refresh_token}"

  # Store session in ~/.nself/auth/session.json (mode 600 — owner read only)
  local session_file="${nself_auth_dir}/session.json"
  local created_at
  created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u '+%Y-%m-%dT%H:%M:%SZ')
  printf '{\n  "provider": "%s",\n  "userId": "%s",\n  "email": "%s",\n  "accessToken": "%s",\n  "refreshToken": "%s",\n  "createdAt": "%s"\n}\n' \
    "$provider" \
    "${user_id:-}" \
    "${user_email:-}" \
    "$access_token" \
    "$stored_refresh" \
    "$created_at" > "$session_file"
  chmod 600 "$session_file"

  log_success "Logged in via ${provider}"
  if [[ -n "${user_email:-}" ]]; then
    log_info "User: ${user_email}"
  fi
  log_info "Session saved to ${session_file}"
  return 0
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

# Hash a password using bcrypt (htpasswd) or SHA-512 crypt (openssl fallback)
# Usage: auth_hash_password <password>
auth_hash_password() {
  local password="$1"

  if [[ -z "$password" ]]; then
    log_error "auth_hash_password: password required"
    return 1
  fi

  # Prefer bcrypt via htpasswd (Apache utils, cost factor 12)
  if command -v htpasswd >/dev/null 2>&1; then
    local hash
    hash=$(htpasswd -nbB -C 12 "" "$password" 2>/dev/null | cut -d: -f2)
    if [[ -n "$hash" ]]; then
      printf '%s' "$hash"
      return 0
    fi
  fi

  # Fall back to SHA-512 crypt via openssl (OpenSSL 1.1.1+)
  if openssl passwd -6 "" >/dev/null 2>&1; then
    local hash
    hash=$(openssl passwd -6 "$password" 2>/dev/null)
    if [[ -n "$hash" ]]; then
      printf '%s' "$hash"
      return 0
    fi
  fi

  # Last resort: delegate to password-utils.sh hash_password if sourced
  if declare -f hash_password >/dev/null 2>&1; then
    hash_password "$password"
    return $?
  fi

  log_error "auth_hash_password: no password hashing tool available (need htpasswd or openssl 1.1.1+)"
  return 1
}

# Verify a password against a stored hash
# Usage: auth_verify_password <password> <hash>
auth_verify_password() {
  local password="$1"
  local hash="$2"

  if [[ -z "$password" ]] || [[ -z "$hash" ]]; then
    return 1
  fi

  # bcrypt hashes ($2y$, $2b$, $2a$) — verify via htpasswd temp file
  if [[ "$hash" == '$2'* ]]; then
    if command -v htpasswd >/dev/null 2>&1; then
      local tmpfile
      tmpfile=$(mktemp)
      printf ':%s\n' "$hash" > "$tmpfile"
      htpasswd -v -b "$tmpfile" "" "$password" >/dev/null 2>&1
      local result=$?
      rm -f "$tmpfile"
      return $result
    fi
    # bcrypt hash but no htpasswd — delegate to password-utils.sh if available
    if declare -f verify_password >/dev/null 2>&1; then
      verify_password "$password" "$hash"
      return $?
    fi
    log_error "auth_verify_password: cannot verify bcrypt hash (install apache2-utils for htpasswd)"
    return 1
  fi

  # SHA-512 crypt hashes ($6$) — verify via openssl
  if [[ "$hash" == '$6$'* ]]; then
    local salt
    salt=$(printf '%s' "$hash" | cut -d'$' -f3)
    local computed
    computed=$(openssl passwd -6 -salt "$salt" "$password" 2>/dev/null)
    [[ "$computed" == "$hash" ]]
    return $?
  fi

  # Delegate to password-utils.sh verify_password for any other hash format
  if declare -f verify_password >/dev/null 2>&1; then
    verify_password "$password" "$hash"
    return $?
  fi

  log_error "auth_verify_password: unrecognised hash format"
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
