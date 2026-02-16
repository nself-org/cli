#!/usr/bin/env bash
# user-manager.sh - User CRUD operations (USER-001) - SECURE VERSION
# Part of nself v0.9.0 - Security Hardening
#
# Implements comprehensive user management operations with SQL injection protection
# All queries use parameterized queries via safe-query.sh


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source password utilities
if [[ -f "$SCRIPT_DIR/password-utils.sh" ]]; then
  source "$SCRIPT_DIR/password-utils.sh"
fi

# Source safe query library
source "$SCRIPT_DIR/../database/safe-query.sh"

# ============================================================================
# User Creation
# ============================================================================

# Create a new user
# Usage: user_create <email> [password] [phone] [metadata_json]
# Returns: User ID
user_create() {
  local email="$1"
  local password="${2:-}"
  local phone="${3:-}"
  local metadata_json="${4:-{}}"

  # Validate email
  email=$(validate_email "$email") || return 1

  # Validate phone if provided
  if [[ -n "$phone" ]]; then
    # Basic phone validation (customize as needed)
    if [[ ! "$phone" =~ ^\+?[0-9]{10,15}$ ]]; then
      echo "ERROR: Invalid phone format" >&2
      return 1
    fi
  fi

  # Validate metadata JSON if provided
  if [[ "$metadata_json" != "{}" ]]; then
    metadata_json=$(validate_json "$metadata_json") || return 1
  fi

  # Check if user already exists using safe query
  local existing_check
  existing_check=$(pg_exists "auth.users" "email" "$email")

  if [[ "$existing_check" == "t" ]]; then
    echo "ERROR: User with email '$email' already exists" >&2
    return 1
  fi

  # Hash password if provided
  local password_hash=""
  if [[ -n "$password" ]]; then
    password_hash=$(hash_password "$password")
    if [[ -z "$password_hash" ]]; then
      echo "ERROR: Failed to hash password" >&2
      return 1
    fi
  fi

  # Create user using safe parameterized query
  local user_id
  if [[ -n "$phone" ]] && [[ -n "$password_hash" ]]; then
    # All fields provided
    user_id=$(pg_insert_returning_id "auth.users" \
      "email, phone, password_hash, created_at" \
      "$email" "$phone" "$password_hash" "NOW()")
  elif [[ -n "$phone" ]]; then
    # Email and phone only
    user_id=$(pg_insert_returning_id "auth.users" \
      "email, phone, created_at" \
      "$email" "$phone" "NOW()")
  elif [[ -n "$password_hash" ]]; then
    # Email and password only
    user_id=$(pg_insert_returning_id "auth.users" \
      "email, password_hash, created_at" \
      "$email" "$password_hash" "NOW()")
  else
    # Email only
    user_id=$(pg_insert_returning_id "auth.users" \
      "email, created_at" \
      "$email" "NOW()")
  fi

  if [[ -z "$user_id" ]]; then
    echo "ERROR: Failed to create user" >&2
    return 1
  fi

  # Store metadata if provided
  if [[ "$metadata_json" != "{}" ]]; then
    local query="INSERT INTO auth.user_metadata (user_id, metadata)
                 VALUES (:'param1', :'param2'::jsonb)"
    pg_query_safe "$query" "$user_id" "$metadata_json" >/dev/null 2>&1
  fi

  echo "$user_id"
  return 0
}

# ============================================================================
# User Retrieval
# ============================================================================

# Get user by ID
# Usage: user_get_by_id <user_id>
# Returns: JSON user object
user_get_by_id() {
  local user_id="$1"

  # Validate UUID
  user_id=$(validate_uuid "$user_id") || return 1

  # Use safe query helper
  local query="SELECT row_to_json(u) FROM (
       SELECT
         id,
         email,
         phone,
         mfa_enabled,
         email_verified,
         phone_verified,
         created_at,
         last_sign_in_at
       FROM auth.users
       WHERE id = :'param1'
     ) u"

  local result
  result=$(pg_query_json "$query" "$user_id")

  if [[ "$result" == "{}" ]]; then
    echo "ERROR: User not found" >&2
    return 1
  fi

  echo "$result"
  return 0
}

# Get user by email
# Usage: user_get_by_email <email>
# Returns: JSON user object
user_get_by_email() {
  local email="$1"

  # Validate email
  email=$(validate_email "$email") || return 1

  # Use safe query
  local query="SELECT row_to_json(u) FROM (
       SELECT
         id,
         email,
         phone,
         mfa_enabled,
         email_verified,
         phone_verified,
         created_at,
         last_sign_in_at
       FROM auth.users
       WHERE email = :'param1'
     ) u"

  local result
  result=$(pg_query_json "$query" "$email")

  if [[ "$result" == "{}" ]]; then
    echo "ERROR: User not found" >&2
    return 1
  fi

  echo "$result"
  return 0
}

# ============================================================================
# User Update
# ============================================================================

# Update user
# Usage: user_update <user_id> [email] [phone] [password]
user_update() {
  local user_id="$1"
  local new_email="${2:-}"
  local new_phone="${3:-}"
  local new_password="${4:-}"

  # Validate user_id
  user_id=$(validate_uuid "$user_id") || return 1

  # Check if user exists
  local exists
  exists=$(pg_exists "auth.users" "id" "$user_id")
  if [[ "$exists" != "t" ]]; then
    echo "ERROR: User not found" >&2
    return 1
  fi

  # Build update columns and values
  local columns=()
  local values=()

  if [[ -n "$new_email" ]]; then
    new_email=$(validate_email "$new_email") || return 1
    columns+=("email")
    values+=("$new_email")
    columns+=("email_verified")
    values+=("FALSE")
  fi

  if [[ -n "$new_phone" ]]; then
    # Validate phone
    if [[ ! "$new_phone" =~ ^\+?[0-9]{10,15}$ ]]; then
      echo "ERROR: Invalid phone format" >&2
      return 1
    fi
    columns+=("phone")
    values+=("$new_phone")
    columns+=("phone_verified")
    values+=("FALSE")
  fi

  if [[ -n "$new_password" ]]; then
    local password_hash
    password_hash=$(hash_password "$new_password")
    if [[ -z "$password_hash" ]]; then
      echo "ERROR: Failed to hash password" >&2
      return 1
    fi
    columns+=("password_hash")
    values+=("$password_hash")
  fi

  if [[ ${#columns[@]} -eq 0 ]]; then
    echo "ERROR: No fields to update" >&2
    return 1
  fi

  # Convert arrays to comma-separated strings
  local columns_str
  columns_str=$(
    IFS=,
    echo "${columns[*]}"
  )

  # Execute update
  pg_update_by_id "auth.users" "id" "$user_id" "$columns_str" "${values[@]}"

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to update user" >&2
    return 1
  fi

  printf "✓ User updated successfully\n" >&2
  return 0
}

# ============================================================================
# User Deletion
# ============================================================================

# Delete user (soft delete)
# Usage: user_delete <user_id> [hard_delete]
user_delete() {
  local user_id="$1"
  local hard_delete="${2:-false}"

  # Validate UUID
  user_id=$(validate_uuid "$user_id") || return 1

  if [[ "$hard_delete" == "true" ]]; then
    # Hard delete - permanently remove user
    pg_delete_by_id "auth.users" "id" "$user_id"

    if [[ $? -ne 0 ]]; then
      echo "ERROR: Failed to delete user" >&2
      return 1
    fi

    printf "✓ User permanently deleted\n" >&2
  else
    # Soft delete - add deleted_at timestamp
    local container
    container=$(pg_get_container) || return 1

    # First, create deleted_at column if it doesn't exist
    docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
      "ALTER TABLE auth.users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;" \
      >/dev/null 2>&1

    # Mark as deleted using safe query
    pg_update_by_id "auth.users" "id" "$user_id" "deleted_at" "NOW()"

    if [[ $? -ne 0 ]]; then
      echo "ERROR: Failed to delete user" >&2
      return 1
    fi

    printf "✓ User marked as deleted (soft delete)\n" >&2
  fi

  return 0
}

# Restore deleted user
# Usage: user_restore <user_id>
user_restore() {
  local user_id="$1"

  # Validate UUID
  user_id=$(validate_uuid "$user_id") || return 1

  # Restore user using safe update
  pg_update_by_id "auth.users" "id" "$user_id" "deleted_at" "NULL"

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to restore user" >&2
    return 1
  fi

  printf "✓ User restored successfully\n" >&2
  return 0
}

# ============================================================================
# User Listing & Search
# ============================================================================

# List all users
# Usage: user_list [limit] [offset] [include_deleted]
# Returns: JSON array of users
user_list() {
  local limit="${1:-50}"
  local offset="${2:-0}"
  local include_deleted="${3:-false}"

  # Validate numeric inputs
  limit=$(validate_integer "$limit" 1 1000) || return 1
  offset=$(validate_integer "$offset" 0) || return 1

  # Build query based on include_deleted flag
  local query
  if [[ "$include_deleted" != "true" ]]; then
    query="SELECT json_agg(u) FROM (
       SELECT
         id,
         email,
         phone,
         mfa_enabled,
         email_verified,
         phone_verified,
         created_at,
         last_sign_in_at,
         deleted_at
       FROM auth.users
       WHERE deleted_at IS NULL
       ORDER BY created_at DESC
       LIMIT :param1 OFFSET :param2
     ) u"
  else
    query="SELECT json_agg(u) FROM (
       SELECT
         id,
         email,
         phone,
         mfa_enabled,
         email_verified,
         phone_verified,
         created_at,
         last_sign_in_at,
         deleted_at
       FROM auth.users
       ORDER BY created_at DESC
       LIMIT :param1 OFFSET :param2
     ) u"
  fi

  pg_query_json_array "$query" "$limit" "$offset"
  return 0
}

# Search users
# Usage: user_search <query> [limit]
# Returns: JSON array of users
user_search() {
  local search_query="$1"
  local limit="${2:-50}"

  if [[ -z "$search_query" ]]; then
    echo "ERROR: Search query required" >&2
    return 1
  fi

  # Validate limit
  limit=$(validate_integer "$limit" 1 1000) || return 1

  # Use ILIKE with parameterized query
  # Add wildcards in the parameter value, not in SQL
  local search_pattern="%${search_query}%"

  local query="SELECT json_agg(u) FROM (
       SELECT
         id,
         email,
         phone,
         mfa_enabled,
         email_verified,
         phone_verified,
         created_at,
         last_sign_in_at
       FROM auth.users
       WHERE deleted_at IS NULL
         AND (email ILIKE :'param1' OR phone ILIKE :'param1')
       ORDER BY created_at DESC
       LIMIT :param2
     ) u"

  pg_query_json_array "$query" "$search_pattern" "$limit"
  return 0
}

# Count total users
# Usage: user_count [include_deleted]
# Returns: Integer count
user_count() {
  local include_deleted="${1:-false}"

  if [[ "$include_deleted" != "true" ]]; then
    pg_count "auth.users" "deleted_at IS NULL"
  else
    pg_count "auth.users"
  fi
}

# ============================================================================
# Export functions
# ============================================================================

export -f user_create
export -f user_get_by_id
export -f user_get_by_email
export -f user_update
export -f user_delete
export -f user_restore
export -f user_list
export -f user_search
export -f user_count
