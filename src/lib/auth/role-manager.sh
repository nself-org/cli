#!/usr/bin/env bash
# role-manager.sh - Role management system (ROLE-001, ROLE-002) - SECURE VERSION
# Part of nself v0.9.0 - Security Hardening
#
# Implements role-based access control (RBAC) with SQL injection protection
# All queries use parameterized queries via safe-query.sh


# Source safe query library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../database/safe-query.sh"

# ============================================================================
# Role CRUD Operations
# ============================================================================

# Create a new role
# Usage: role_create <role_name> <description>
role_create() {
  local role_name="$1"
  local description="${2:-}"

  # Validate role name
  role_name=$(validate_identifier "$role_name" 100) || return 1

  # Get PostgreSQL container
  local container
  container=$(pg_get_container) || return 1

  # Create roles table if it doesn't exist (DDL - safe, no user input)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  is_default BOOLEAN DEFAULT FALSE,
  is_system BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_roles_name ON auth.roles(name);
CREATE INDEX IF NOT EXISTS idx_roles_is_default ON auth.roles(is_default);
EOSQL

  # Check if role already exists using safe query
  local exists
  exists=$(pg_exists "auth.roles" "name" "$role_name")

  if [[ "$exists" == "t" ]]; then
    echo "ERROR: Role '$role_name' already exists" >&2
    return 1
  fi

  # Create role using safe parameterized query
  local role_id
  role_id=$(pg_insert_returning_id "auth.roles" "name, description" "$role_name" "$description")

  if [[ -z "$role_id" ]]; then
    echo "ERROR: Failed to create role" >&2
    return 1
  fi

  echo "$role_id"
  return 0
}

# Get role by ID
# Usage: role_get_by_id <role_id>
role_get_by_id() {
  local role_id="$1"

  # Validate UUID
  role_id=$(validate_uuid "$role_id") || return 1

  # Use safe query
  local query="SELECT row_to_json(r) FROM (
       SELECT id, name, description, is_default, is_system, created_at, updated_at
       FROM auth.roles
       WHERE id = :'param1'
     ) r"

  local result
  result=$(pg_query_json "$query" "$role_id")

  if [[ "$result" == "{}" ]]; then
    echo "ERROR: Role not found" >&2
    return 1
  fi

  echo "$result"
  return 0
}

# Get role by name
# Usage: role_get_by_name <role_name>
role_get_by_name() {
  local role_name="$1"

  # Validate role name
  role_name=$(validate_identifier "$role_name" 100) || return 1

  # Use safe query
  local query="SELECT row_to_json(r) FROM (
       SELECT id, name, description, is_default, is_system, created_at, updated_at
       FROM auth.roles
       WHERE name = :'param1'
     ) r"

  local result
  result=$(pg_query_json "$query" "$role_name")

  if [[ "$result" == "{}" ]]; then
    echo "ERROR: Role not found" >&2
    return 1
  fi

  echo "$result"
  return 0
}

# Update role
# Usage: role_update <role_id> <name> <description>
role_update() {
  local role_id="$1"
  local new_name="${2:-}"
  local new_description="${3:-}"

  # Validate UUID
  role_id=$(validate_uuid "$role_id") || return 1

  # Check if role is system role (safe query, no user input in WHERE)
  local query="SELECT is_system FROM auth.roles WHERE id = :'param1' LIMIT 1"
  local is_system
  is_system=$(pg_query_value "$query" "$role_id")

  if [[ "$is_system" == "t" ]]; then
    echo "ERROR: Cannot modify system role" >&2
    return 1
  fi

  # Build update columns and values
  local columns=()
  local values=()

  if [[ -n "$new_name" ]]; then
    new_name=$(validate_identifier "$new_name" 100) || return 1
    columns+=("name")
    values+=("$new_name")
  fi

  if [[ -n "$new_description" ]]; then
    # Description can contain any text, will be parameterized
    columns+=("description")
    values+=("$new_description")
  fi

  if [[ ${#columns[@]} -eq 0 ]]; then
    echo "ERROR: No fields to update" >&2
    return 1
  fi

  # Add updated_at
  columns+=("updated_at")
  values+=("NOW()")

  # Convert arrays to comma-separated strings
  local columns_str
  columns_str=$(
    IFS=,
    echo "${columns[*]}"
  )

  # Update role
  pg_update_by_id "auth.roles" "id" "$role_id" "$columns_str" "${values[@]}"

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to update role" >&2
    return 1
  fi

  return 0
}

# Delete role
# Usage: role_delete <role_id>
role_delete() {
  local role_id="$1"

  # Validate UUID
  role_id=$(validate_uuid "$role_id") || return 1

  # Check if role is system role
  local query="SELECT is_system FROM auth.roles WHERE id = :'param1' LIMIT 1"
  local is_system
  is_system=$(pg_query_value "$query" "$role_id")

  if [[ "$is_system" == "t" ]]; then
    echo "ERROR: Cannot delete system role" >&2
    return 1
  fi

  # Delete role using safe query
  pg_delete_by_id "auth.roles" "id" "$role_id"

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to delete role" >&2
    return 1
  fi

  return 0
}

# List all roles
# Usage: role_list [limit] [offset]
role_list() {
  local limit="${1:-50}"
  local offset="${2:-0}"

  # Validate numeric inputs
  limit=$(validate_integer "$limit" 1 1000) || return 1
  offset=$(validate_integer "$offset" 0) || return 1

  # Get roles using safe query
  local query="SELECT json_agg(r) FROM (
       SELECT id, name, description, is_default, is_system, created_at, updated_at
       FROM auth.roles
       ORDER BY is_system DESC, name ASC
       LIMIT :param1 OFFSET :param2
     ) r"

  pg_query_json_array "$query" "$limit" "$offset"
  return 0
}

# ============================================================================
# Default Role Management
# ============================================================================

# Set role as default
# Usage: role_set_default <role_id>
role_set_default() {
  local role_id="$1"

  # Validate UUID
  role_id=$(validate_uuid "$role_id") || return 1

  local container
  container=$(pg_get_container) || return 1

  # Start transaction
  pg_begin

  # Unset all default roles (safe - no user input)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "UPDATE auth.roles SET is_default = FALSE;" \
    >/dev/null 2>&1

  # Set this role as default using safe query
  pg_update_by_id "auth.roles" "id" "$role_id" "is_default" "TRUE"

  local result=$?

  if [[ $result -ne 0 ]]; then
    pg_rollback
    echo "ERROR: Failed to set default role" >&2
    return 1
  fi

  pg_commit
  return 0
}

# Get default role
# Usage: role_get_default
role_get_default() {
  local container
  container=$(pg_get_container) || return 1

  # Get default role (safe - no user input)
  local role_json
  role_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(r) FROM (
       SELECT id, name, description, is_default, is_system
       FROM auth.roles
       WHERE is_default = TRUE
       LIMIT 1
     ) r;" 2>/dev/null | xargs)

  if [[ -z "$role_json" ]] || [[ "$role_json" == "null" ]]; then
    echo "{}"
    return 0
  fi

  echo "$role_json"
  return 0
}

# ============================================================================
# User-Role Assignment
# ============================================================================

# Assign role to user
# Usage: role_assign_user <user_id> <role_id>
role_assign_user() {
  local user_id="$1"
  local role_id="$2"

  # Validate UUIDs
  user_id=$(validate_uuid "$user_id") || return 1
  role_id=$(validate_uuid "$role_id") || return 1

  local container
  container=$(pg_get_container) || return 1

  # Create user_roles table if it doesn't exist (DDL - safe)
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.user_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
  role_id UUID REFERENCES auth.roles(id) ON DELETE CASCADE,
  assigned_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, role_id)
);
CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON auth.user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON auth.user_roles(role_id);
EOSQL

  # Assign role using safe query
  local query="INSERT INTO auth.user_roles (user_id, role_id)
               VALUES (:'param1', :'param2')
               ON CONFLICT (user_id, role_id) DO NOTHING"

  pg_query_safe "$query" "$user_id" "$role_id" >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to assign role" >&2
    return 1
  fi

  return 0
}

# Revoke role from user
# Usage: role_revoke_user <user_id> <role_id>
role_revoke_user() {
  local user_id="$1"
  local role_id="$2"

  # Validate UUIDs
  user_id=$(validate_uuid "$user_id") || return 1
  role_id=$(validate_uuid "$role_id") || return 1

  # Revoke role using safe query
  local query="DELETE FROM auth.user_roles
               WHERE user_id = :'param1' AND role_id = :'param2'"

  pg_query_safe "$query" "$user_id" "$role_id" >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to revoke role" >&2
    return 1
  fi

  return 0
}

# Get user roles
# Usage: role_get_user_roles <user_id>
role_get_user_roles() {
  local user_id="$1"

  # Validate UUID
  user_id=$(validate_uuid "$user_id") || return 1

  # Get user roles using safe query
  local query="SELECT json_agg(r) FROM (
       SELECT r.id, r.name, r.description, ur.assigned_at
       FROM auth.user_roles ur
       JOIN auth.roles r ON ur.role_id = r.id
       WHERE ur.user_id = :'param1'
       ORDER BY r.name
     ) r"

  pg_query_json_array "$query" "$user_id"
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f role_create
export -f role_get_by_id
export -f role_get_by_name
export -f role_update
export -f role_delete
export -f role_list
export -f role_set_default
export -f role_get_default
export -f role_assign_user
export -f role_revoke_user
export -f role_get_user_roles
