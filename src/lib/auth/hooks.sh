#!/usr/bin/env bash
# hooks.sh - Authentication hooks system (HOOK-001 to HOOK-006)
# Part of nself v0.6.0 - Phase 1 Sprint 3
#
# Implements pluggable hooks for authentication lifecycle events
# Allows custom logic injection at key points in auth flows


# Hook types
readonly HOOK_PRE_SIGNUP="pre_signup"

set -euo pipefail

readonly HOOK_POST_SIGNUP="post_signup"
readonly HOOK_PRE_LOGIN="pre_login"
readonly HOOK_POST_LOGIN="post_login"
readonly HOOK_CUSTOM_CLAIMS="custom_claims"
readonly HOOK_PRE_MFA="pre_mfa"
readonly HOOK_POST_MFA="post_mfa"

# ============================================================================
# Hook Registration
# ============================================================================

# Register a hook
# Usage: hook_register <hook_type> <hook_name> <handler_path> [enabled]
hook_register() {
  local hook_type="$1"
  local hook_name="$2"
  local handler_path="$3"
  local enabled="${4:-true}"

  if [[ -z "$hook_type" ]] || [[ -z "$hook_name" ]] || [[ -z "$handler_path" ]]; then
    echo "ERROR: Hook type, name, and handler path required" >&2
    return 1
  fi

  # Validate hook type
  local valid_types=("$HOOK_PRE_SIGNUP" "$HOOK_POST_SIGNUP" "$HOOK_PRE_LOGIN" "$HOOK_POST_LOGIN" "$HOOK_CUSTOM_CLAIMS" "$HOOK_PRE_MFA" "$HOOK_POST_MFA")
  local is_valid=false
  for type in "${valid_types[@]}"; do
    if [[ "$hook_type" == "$type" ]]; then
      is_valid=true
      break
    fi
  done

  if [[ "$is_valid" == "false" ]]; then
    echo "ERROR: Invalid hook type: $hook_type" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create hooks table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.hooks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  hook_type TEXT NOT NULL,
  name TEXT NOT NULL,
  handler_path TEXT NOT NULL,
  enabled BOOLEAN DEFAULT TRUE,
  priority INTEGER DEFAULT 100,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(hook_type, name)
);
CREATE INDEX IF NOT EXISTS idx_hooks_type ON auth.hooks(hook_type);
CREATE INDEX IF NOT EXISTS idx_hooks_enabled ON auth.hooks(enabled);
CREATE INDEX IF NOT EXISTS idx_hooks_priority ON auth.hooks(priority);
EOSQL

  # Escape handler path
  handler_path=$(echo "$handler_path" | sed "s/'/''/g")

  # Register hook
  local hook_id
  hook_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.hooks (hook_type, name, handler_path, enabled)
     VALUES ('$hook_type', '$hook_name', '$handler_path', $enabled)
     ON CONFLICT (hook_type, name) DO UPDATE SET
       handler_path = EXCLUDED.handler_path,
       enabled = EXCLUDED.enabled,
       updated_at = NOW()
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$hook_id" ]]; then
    echo "ERROR: Failed to register hook" >&2
    return 1
  fi

  echo "$hook_id"
  return 0
}

# Unregister a hook
# Usage: hook_unregister <hook_type> <hook_name>
hook_unregister() {
  local hook_type="$1"
  local hook_name="$2"

  if [[ -z "$hook_type" ]] || [[ -z "$hook_name" ]]; then
    echo "ERROR: Hook type and name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Delete hook
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.hooks WHERE hook_type = '$hook_type' AND name = '$hook_name';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to unregister hook" >&2
    return 1
  fi

  return 0
}

# Enable/disable hook
# Usage: hook_set_enabled <hook_type> <hook_name> <enabled>
hook_set_enabled() {
  local hook_type="$1"
  local hook_name="$2"
  local enabled="$3"

  if [[ -z "$hook_type" ]] || [[ -z "$hook_name" ]]; then
    echo "ERROR: Hook type and name required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Update enabled status
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.hooks SET enabled = $enabled, updated_at = NOW()
     WHERE hook_type = '$hook_type' AND name = '$hook_name';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to update hook status" >&2
    return 1
  fi

  return 0
}

# List hooks
# Usage: hook_list [hook_type]
hook_list() {
  local hook_type="${1:-}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build query
  local where_clause=""
  if [[ -n "$hook_type" ]]; then
    where_clause="WHERE hook_type = '$hook_type'"
  fi

  # Get hooks
  local hooks_json
  hooks_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(h) FROM (
       SELECT id, hook_type, name, handler_path, enabled, priority, created_at
       FROM auth.hooks
       $where_clause
       ORDER BY hook_type, priority, name
     ) h;" \
    2>/dev/null | xargs)

  if [[ -z "$hooks_json" ]] || [[ "$hooks_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$hooks_json"
  return 0
}

# ============================================================================
# Hook Execution
# ============================================================================

# Execute hooks of a specific type
# Usage: hook_execute <hook_type> <context_json>
# Returns: Modified context JSON or original if no hooks
hook_execute() {
  local hook_type="$1"
  local context_json="${2:-{}}"

  if [[ -z "$hook_type" ]]; then
    echo "ERROR: Hook type required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get enabled hooks of this type
  local hooks
  hooks=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -A -F'|' -c \
    "SELECT id, name, handler_path
     FROM auth.hooks
     WHERE hook_type = '$hook_type' AND enabled = TRUE
     ORDER BY priority, name;" \
    2>/dev/null)

  if [[ -z "$hooks" ]]; then
    # No hooks registered, return original context
    echo "$context_json"
    return 0
  fi

  # Execute each hook in order
  local current_context="$context_json"

  while IFS='|' read -r hook_id hook_name handler_path; do
    # Execute hook handler
    local result
    result=$(hook_execute_handler "$handler_path" "$current_context" 2>&1)

    if [[ $? -eq 0 ]]; then
      # Hook succeeded, use modified context
      current_context="$result"
    else
      # Hook failed, log error but continue
      echo "WARNING: Hook '$hook_name' failed: $result" >&2
      # Optionally: return error to stop execution
      # return 1
    fi
  done <<<"$hooks"

  echo "$current_context"
  return 0
}

# Execute a hook handler
# Usage: hook_execute_handler <handler_path> <context_json>
hook_execute_handler() {
  local handler_path="$1"
  local context_json="$2"

  # Check if handler exists and is executable
  if [[ ! -f "$handler_path" ]]; then
    echo "ERROR: Handler not found: $handler_path" >&2
    return 1
  fi

  if [[ ! -x "$handler_path" ]]; then
    echo "ERROR: Handler not executable: $handler_path" >&2
    return 1
  fi

  # Execute handler with context as argument
  "$handler_path" "$context_json"
  return $?
}

# ============================================================================
# Pre-Signup Hook
# ============================================================================

# Execute pre-signup hooks
# Usage: hook_pre_signup <user_data_json>
# Context: {email, phone, metadata}
# Returns: Modified user data or error
hook_pre_signup() {
  local user_data="$1"

  hook_execute "$HOOK_PRE_SIGNUP" "$user_data"
  return $?
}

# ============================================================================
# Post-Signup Hook
# ============================================================================

# Execute post-signup hooks
# Usage: hook_post_signup <user_json>
# Context: {user_id, email, phone, created_at}
# Returns: Modified user data or original
hook_post_signup() {
  local user_data="$1"

  hook_execute "$HOOK_POST_SIGNUP" "$user_data"
  return $?
}

# ============================================================================
# Pre-Login Hook
# ============================================================================

# Execute pre-login hooks
# Usage: hook_pre_login <login_data_json>
# Context: {email, ip_address, user_agent}
# Returns: Modified login data or error (abort login)
hook_pre_login() {
  local login_data="$1"

  hook_execute "$HOOK_PRE_LOGIN" "$login_data"
  return $?
}

# ============================================================================
# Post-Login Hook
# ============================================================================

# Execute post-login hooks
# Usage: hook_post_login <session_data_json>
# Context: {user_id, session_id, access_token, refresh_token}
# Returns: Modified session data
hook_post_login() {
  local session_data="$1"

  hook_execute "$HOOK_POST_LOGIN" "$session_data"
  return $?
}

# ============================================================================
# Custom Claims Hook
# ============================================================================

# Execute custom claims hooks
# Usage: hook_custom_claims <claims_json>
# Context: {user_id, roles, permissions, metadata}
# Returns: Modified claims to include in JWT
hook_custom_claims() {
  local claims_data="$1"

  hook_execute "$HOOK_CUSTOM_CLAIMS" "$claims_data"
  return $?
}

# ============================================================================
# Hook Context Helpers
# ============================================================================

# Create hook context
# Usage: hook_create_context <data_json>
hook_create_context() {
  local data_json="$1"

  # Add common context fields
  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  echo "$data_json" | jq --arg ts "$timestamp" '. + {timestamp: $ts}'
  return 0
}

# Validate hook context
# Usage: hook_validate_context <context_json> <required_fields_array>
# Example: hook_validate_context "$context" '["email", "user_id"]'
hook_validate_context() {
  local context_json="$1"
  local required_fields="$2"

  # Check if all required fields are present
  local field_count
  field_count=$(echo "$required_fields" | jq 'length')

  for ((i = 0; i < field_count; i++)); do
    local field
    field=$(echo "$required_fields" | jq -r ".[$i]")

    local value
    value=$(echo "$context_json" | jq -r ".$field // empty")

    if [[ -z "$value" ]]; then
      echo "ERROR: Missing required field: $field" >&2
      return 1
    fi
  done

  return 0
}

# ============================================================================
# Hook Logging
# ============================================================================

# Log hook execution
# Usage: hook_log <hook_type> <hook_name> <status> <message>
hook_log() {
  local hook_type="$1"
  local hook_name="$2"
  local status="$3"
  local message="${4:-}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 0 # Fail silently for logging
  fi

  # Create hook_logs table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.hook_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  hook_type TEXT NOT NULL,
  hook_name TEXT NOT NULL,
  status TEXT NOT NULL,
  message TEXT,
  executed_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_hook_logs_executed_at ON auth.hook_logs(executed_at);
CREATE INDEX IF NOT EXISTS idx_hook_logs_type ON auth.hook_logs(hook_type);
EOSQL

  # Escape message
  message=$(echo "$message" | sed "s/'/''/g")

  # Log hook execution
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.hook_logs (hook_type, hook_name, status, message)
     VALUES ('$hook_type', '$hook_name', '$status', '$message');" \
    >/dev/null 2>&1

  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f hook_register
export -f hook_unregister
export -f hook_set_enabled
export -f hook_list
export -f hook_execute
export -f hook_execute_handler
export -f hook_pre_signup
export -f hook_post_signup
export -f hook_pre_login
export -f hook_post_login
export -f hook_custom_claims
export -f hook_create_context
export -f hook_validate_context
export -f hook_log
