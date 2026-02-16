#!/usr/bin/env bash
# permission-manager.sh - Permission management system (ROLE-003)
# Part of nself v0.6.0 - Phase 1 Sprint 3
#
# Implements granular permission management for RBAC


# ============================================================================
# Permission CRUD Operations
# ============================================================================

# Create a new permission
# Usage: permission_create <resource> <action> <description>
# Example: permission_create "users" "create" "Create new users"
permission_create() {

set -euo pipefail

  local resource="$1"
  local action="$2"
  local description="${3:-}"

  if [[ -z "$resource" ]] || [[ -z "$action" ]]; then
    echo "ERROR: Resource and action required" >&2
    return 1
  fi

  # Validate names (alphanumeric, underscore, hyphen)
  if ! echo "$resource" | grep -qE '^[a-zA-Z0-9_-]+$'; then
    echo "ERROR: Invalid resource name" >&2
    return 1
  fi

  if ! echo "$action" | grep -qE '^[a-zA-Z0-9_-]+$'; then
    echo "ERROR: Invalid action name" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create permissions table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.permissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  resource TEXT NOT NULL,
  action TEXT NOT NULL,
  description TEXT,
  is_system BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(resource, action)
);
CREATE INDEX IF NOT EXISTS idx_permissions_resource ON auth.permissions(resource);
CREATE INDEX IF NOT EXISTS idx_permissions_action ON auth.permissions(action);
EOSQL

  # Check if permission exists
  local existing_perm
  existing_perm=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT id FROM auth.permissions WHERE resource = '$resource' AND action = '$action' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -n "$existing_perm" ]]; then
    echo "ERROR: Permission '$resource:$action' already exists" >&2
    return 1
  fi

  # Escape description
  description=$(echo "$description" | sed "s/'/''/g")

  # Create permission
  local perm_id
  perm_id=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "INSERT INTO auth.permissions (resource, action, description)
     VALUES ('$resource', '$action', '$description')
     RETURNING id;" \
    2>/dev/null | xargs)

  if [[ -z "$perm_id" ]]; then
    echo "ERROR: Failed to create permission" >&2
    return 1
  fi

  echo "$perm_id"
  return 0
}

# Get permission by ID
# Usage: permission_get_by_id <permission_id>
permission_get_by_id() {
  local perm_id="$1"

  if [[ -z "$perm_id" ]]; then
    echo "ERROR: Permission ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get permission
  local perm_json
  perm_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(p) FROM (
       SELECT id, resource, action, description, is_system, created_at
       FROM auth.permissions
       WHERE id = '$perm_id'
     ) p;" \
    2>/dev/null | xargs)

  if [[ -z "$perm_json" ]] || [[ "$perm_json" == "null" ]]; then
    echo "ERROR: Permission not found" >&2
    return 1
  fi

  echo "$perm_json"
  return 0
}

# Delete permission
# Usage: permission_delete <permission_id>
permission_delete() {
  local perm_id="$1"

  if [[ -z "$perm_id" ]]; then
    echo "ERROR: Permission ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Check if system permission
  local is_system
  is_system=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT is_system FROM auth.permissions WHERE id = '$perm_id' LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ "$is_system" == "t" ]]; then
    echo "ERROR: Cannot delete system permission" >&2
    return 1
  fi

  # Delete permission
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.permissions WHERE id = '$perm_id';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to delete permission" >&2
    return 1
  fi

  return 0
}

# List permissions
# Usage: permission_list [resource] [limit] [offset]
permission_list() {
  local resource="${1:-}"
  local limit="${2:-100}"
  local offset="${3:-0}"

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Build query
  local where_clause=""
  if [[ -n "$resource" ]]; then
    where_clause="WHERE resource = '$resource'"
  fi

  # Get permissions
  local perms_json
  perms_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(p) FROM (
       SELECT id, resource, action, description, is_system
       FROM auth.permissions
       $where_clause
       ORDER BY resource, action
       LIMIT $limit OFFSET $offset
     ) p;" \
    2>/dev/null | xargs)

  if [[ -z "$perms_json" ]] || [[ "$perms_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$perms_json"
  return 0
}

# ============================================================================
# Role-Permission Association
# ============================================================================

# Assign permission to role
# Usage: permission_assign_role <role_id> <permission_id>
permission_assign_role() {
  local role_id="$1"
  local perm_id="$2"

  if [[ -z "$role_id" ]] || [[ -z "$perm_id" ]]; then
    echo "ERROR: Role ID and permission ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create role_permissions table if it doesn't exist
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.role_permissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  role_id UUID REFERENCES auth.roles(id) ON DELETE CASCADE,
  permission_id UUID REFERENCES auth.permissions(id) ON DELETE CASCADE,
  granted_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(role_id, permission_id)
);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON auth.role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission_id ON auth.role_permissions(permission_id);
EOSQL

  # Assign permission
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.role_permissions (role_id, permission_id)
     VALUES ('$role_id', '$perm_id')
     ON CONFLICT (role_id, permission_id) DO NOTHING;" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to assign permission" >&2
    return 1
  fi

  return 0
}

# Revoke permission from role
# Usage: permission_revoke_role <role_id> <permission_id>
permission_revoke_role() {
  local role_id="$1"
  local perm_id="$2"

  if [[ -z "$role_id" ]] || [[ -z "$perm_id" ]]; then
    echo "ERROR: Role ID and permission ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Revoke permission
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.role_permissions
     WHERE role_id = '$role_id' AND permission_id = '$perm_id';" \
    >/dev/null 2>&1

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to revoke permission" >&2
    return 1
  fi

  return 0
}

# Get role permissions
# Usage: permission_get_role_permissions <role_id>
permission_get_role_permissions() {
  local role_id="$1"

  if [[ -z "$role_id" ]]; then
    echo "ERROR: Role ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get permissions
  local perms_json
  perms_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(p) FROM (
       SELECT p.id, p.resource, p.action, p.description, rp.granted_at
       FROM auth.role_permissions rp
       JOIN auth.permissions p ON rp.permission_id = p.id
       WHERE rp.role_id = '$role_id'
       ORDER BY p.resource, p.action
     ) p;" \
    2>/dev/null | xargs)

  if [[ -z "$perms_json" ]] || [[ "$perms_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$perms_json"
  return 0
}

# ============================================================================
# Permission Checking
# ============================================================================

# Check if user has permission
# Usage: permission_check_user <user_id> <resource> <action>
# Returns: 0 if has permission, 1 if not
permission_check_user() {
  local user_id="$1"
  local resource="$2"
  local action="$3"

  if [[ -z "$user_id" ]] || [[ -z "$resource" ]] || [[ -z "$action" ]]; then
    echo "ERROR: User ID, resource, and action required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Check permission through user roles
  local has_perm
  has_perm=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT COUNT(*)
     FROM auth.user_roles ur
     JOIN auth.role_permissions rp ON ur.role_id = rp.role_id
     JOIN auth.permissions p ON rp.permission_id = p.id
     WHERE ur.user_id = '$user_id'
       AND p.resource = '$resource'
       AND p.action = '$action';" \
    2>/dev/null | xargs)

  if [[ "${has_perm:-0}" -gt 0 ]]; then
    return 0 # Has permission
  else
    return 1 # No permission
  fi
}

# Get all user permissions (aggregated from roles)
# Usage: permission_get_user_permissions <user_id>
permission_get_user_permissions() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get all permissions from user's roles
  local perms_json
  perms_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT json_agg(DISTINCT p.*) FROM (
       SELECT p.id, p.resource, p.action, p.description
       FROM auth.user_roles ur
       JOIN auth.role_permissions rp ON ur.role_id = rp.role_id
       JOIN auth.permissions p ON rp.permission_id = p.id
       WHERE ur.user_id = '$user_id'
       ORDER BY p.resource, p.action
     ) p;" \
    2>/dev/null | xargs)

  if [[ -z "$perms_json" ]] || [[ "$perms_json" == "null" ]]; then
    echo "[]"
    return 0
  fi

  echo "$perms_json"
  return 0
}

# ============================================================================
# Bulk Operations
# ============================================================================

# Create standard CRUD permissions for a resource
# Usage: permission_create_crud <resource>
# Creates: create, read, update, delete permissions
permission_create_crud() {
  local resource="$1"

  if [[ -z "$resource" ]]; then
    echo "ERROR: Resource required" >&2
    return 1
  fi

  local actions=("create" "read" "update" "delete")
  local descriptions=(
    "Create new $resource"
    "Read $resource"
    "Update $resource"
    "Delete $resource"
  )

  for i in "${!actions[@]}"; do
    permission_create "$resource" "${actions[$i]}" "${descriptions[$i]}" 2>/dev/null || true
  done

  echo "✓ Created CRUD permissions for $resource" >&2
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f permission_create
export -f permission_get_by_id
export -f permission_delete
export -f permission_list
export -f permission_assign_role
export -f permission_revoke_role
export -f permission_get_role_permissions
export -f permission_check_user
export -f permission_get_user_permissions
export -f permission_create_crud
