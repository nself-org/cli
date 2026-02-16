#!/usr/bin/env bash
# custom-claims.sh - Custom JWT claims with roles/permissions (ROLE-006)
# Part of nself v0.6.0 - Phase 1 Sprint 3
#
# Integrates RBAC system with JWT tokens via custom claims


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/role-manager.sh" ]]; then
  source "$SCRIPT_DIR/role-manager.sh"
fi
if [[ -f "$SCRIPT_DIR/permission-manager.sh" ]]; then
  source "$SCRIPT_DIR/permission-manager.sh"
fi
if [[ -f "$SCRIPT_DIR/hooks.sh" ]]; then
  source "$SCRIPT_DIR/hooks.sh"
fi

# ============================================================================
# Custom Claims Generation
# ============================================================================

# Generate custom claims for user
# Usage: claims_generate <user_id>
# Returns: JSON with custom claims
claims_generate() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get user roles
  local roles_json
  roles_json=$(role_get_user_roles "$user_id" 2>/dev/null || echo "[]")

  # Get user permissions (aggregated from roles)
  local permissions_json
  permissions_json=$(permission_get_user_permissions "$user_id" 2>/dev/null || echo "[]")

  # Get user metadata
  local metadata_json="{}"
  if [[ -f "$SCRIPT_DIR/user-metadata.sh" ]]; then
    source "$SCRIPT_DIR/user-metadata.sh"
    metadata_json=$(metadata_get_all "$user_id" 2>/dev/null || echo "{}")
  fi

  # Build base claims
  local claims_json
  claims_json=$(
    cat <<EOF
{
  "user_id": "$user_id",
  "roles": $roles_json,
  "permissions": $permissions_json,
  "metadata": $metadata_json
}
EOF
  )

  # Execute custom claims hooks for additional transformations
  claims_json=$(hook_custom_claims "$claims_json" 2>/dev/null || echo "$claims_json")

  echo "$claims_json"
  return 0
}

# Generate Hasura claims
# Usage: claims_generate_hasura <user_id> [default_role]
# Returns: Hasura-compatible JWT claims
claims_generate_hasura() {
  local user_id="$1"
  local default_role="${2:-user}"

  # Get user roles
  local roles_array
  roles_array=$(role_get_user_roles "$user_id" 2>/dev/null | jq -r '.[].name' 2>/dev/null || echo "")

  # Build Hasura roles array
  local hasura_roles="[\"$default_role\"]"
  if [[ -n "$roles_array" ]]; then
    local roles_list=""
    while IFS= read -r role; do
      if [[ -n "$role" ]]; then
        roles_list="${roles_list},\"$role\""
      fi
    done <<<"$roles_array"

    if [[ -n "$roles_list" ]]; then
      hasura_roles="[${roles_list#,}]"
    fi
  fi

  # Build Hasura custom claims
  cat <<EOF
{
  "https://hasura.io/jwt/claims": {
    "x-hasura-allowed-roles": $hasura_roles,
    "x-hasura-default-role": "$default_role",
    "x-hasura-user-id": "$user_id"
  }
}
EOF

  return 0
}

# ============================================================================
# Claims Validation
# ============================================================================

# Validate custom claims
# Usage: claims_validate <claims_json>
# Returns: 0 if valid, 1 if invalid
claims_validate() {
  local claims_json="$1"

  if [[ -z "$claims_json" ]]; then
    echo "ERROR: Claims JSON required" >&2
    return 1
  fi

  # Check required fields
  local user_id
  user_id=$(echo "$claims_json" | jq -r '.user_id // empty' 2>/dev/null)

  if [[ -z "$user_id" ]]; then
    echo "ERROR: Missing user_id in claims" >&2
    return 1
  fi

  # Validate roles structure
  local roles
  roles=$(echo "$claims_json" | jq '.roles' 2>/dev/null)

  if [[ -z "$roles" ]] || [[ "$roles" == "null" ]]; then
    echo "ERROR: Missing or invalid roles in claims" >&2
    return 1
  fi

  # Validate permissions structure
  local permissions
  permissions=$(echo "$claims_json" | jq '.permissions' 2>/dev/null)

  if [[ -z "$permissions" ]] || [[ "$permissions" == "null" ]]; then
    echo "ERROR: Missing or invalid permissions in claims" >&2
    return 1
  fi

  return 0
}

# ============================================================================
# Claims Refresh
# ============================================================================

# Refresh claims for user (regenerate with current data)
# Usage: claims_refresh <user_id>
claims_refresh() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Regenerate claims
  claims_generate "$user_id"
  return $?
}

# ============================================================================
# Claims Storage
# ============================================================================

# Store custom claims (for caching)
# Usage: claims_store <user_id> <claims_json> [ttl_seconds]
claims_store() {
  local user_id="$1"
  local claims_json="$2"
  local ttl="${3:-300}" # 5 minute default

  if [[ -z "$user_id" ]] || [[ -z "$claims_json" ]]; then
    echo "ERROR: User ID and claims JSON required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Create claims cache table
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" <<'EOSQL' >/dev/null 2>&1
CREATE TABLE IF NOT EXISTS auth.claims_cache (
  user_id UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
  claims JSONB NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_claims_cache_expires_at ON auth.claims_cache(expires_at);
EOSQL

  # Calculate expiry
  local expires_at
  expires_at=$(date -u -d "+${ttl} seconds" "+%Y-%m-%d %H:%M:%S" 2>/dev/null ||
    date -u -v+${ttl}S "+%Y-%m-%d %H:%M:%S" 2>/dev/null)

  # Escape claims JSON
  claims_json=$(echo "$claims_json" | sed "s/'/''/g")

  # Store claims
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "INSERT INTO auth.claims_cache (user_id, claims, expires_at)
     VALUES ('$user_id', '$claims_json'::jsonb, '$expires_at'::timestamptz)
     ON CONFLICT (user_id) DO UPDATE SET
       claims = EXCLUDED.claims,
       expires_at = EXCLUDED.expires_at,
       created_at = NOW();" \
    >/dev/null 2>&1

  return $?
}

# Get cached claims
# Usage: claims_get_cached <user_id>
claims_get_cached() {
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

  # Get cached claims if not expired
  local claims_json
  claims_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT claims FROM auth.claims_cache
     WHERE user_id = '$user_id'
       AND expires_at > NOW()
     LIMIT 1;" \
    2>/dev/null | xargs)

  if [[ -z "$claims_json" ]] || [[ "$claims_json" == "null" ]]; then
    return 1 # No cached claims
  fi

  echo "$claims_json"
  return 0
}

# Invalidate cached claims
# Usage: claims_invalidate <user_id>
claims_invalidate() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "ERROR: User ID required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    return 0 # Fail silently
  fi

  # Delete cached claims
  docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -c \
    "DELETE FROM auth.claims_cache WHERE user_id = '$user_id';" \
    >/dev/null 2>&1

  return 0
}

# ============================================================================
# Claims Helpers
# ============================================================================

# Get user permissions as simple array
# Usage: claims_get_permissions_array <user_id>
# Returns: ["resource:action", "resource:action", ...]
claims_get_permissions_array() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "[]"
    return 0
  fi

  local permissions_json
  permissions_json=$(permission_get_user_permissions "$user_id" 2>/dev/null || echo "[]")

  # Transform to array of "resource:action" strings
  echo "$permissions_json" | jq -r '[.[] | "\(.resource):\(.action)"]' 2>/dev/null || echo "[]"
  return 0
}

# Get user roles as simple array
# Usage: claims_get_roles_array <user_id>
# Returns: ["role1", "role2", ...]
claims_get_roles_array() {
  local user_id="$1"

  if [[ -z "$user_id" ]]; then
    echo "[]"
    return 0
  fi

  local roles_json
  roles_json=$(role_get_user_roles "$user_id" 2>/dev/null || echo "[]")

  # Transform to array of role names
  echo "$roles_json" | jq -r '[.[] | .name]' 2>/dev/null || echo "[]"
  return 0
}

# Check if user has role
# Usage: claims_has_role <user_id> <role_name>
# Returns: 0 if has role, 1 if not
claims_has_role() {
  local user_id="$1"
  local role_name="$2"

  if [[ -z "$user_id" ]] || [[ -z "$role_name" ]]; then
    return 1
  fi

  local roles_array
  roles_array=$(claims_get_roles_array "$user_id")

  echo "$roles_array" | jq -e --arg role "$role_name" '. | index($role) != null' >/dev/null 2>&1
  return $?
}

# Check if user has permission
# Usage: claims_has_permission <user_id> <resource> <action>
# Returns: 0 if has permission, 1 if not
claims_has_permission() {
  local user_id="$1"
  local resource="$2"
  local action="$3"

  if [[ -z "$user_id" ]] || [[ -z "$resource" ]] || [[ -z "$action" ]]; then
    return 1
  fi

  permission_check_user "$user_id" "$resource" "$action"
  return $?
}

# ============================================================================
# Export functions
# ============================================================================

export -f claims_generate
export -f claims_generate_hasura
export -f claims_validate
export -f claims_refresh
export -f claims_store
export -f claims_get_cached
export -f claims_invalidate
export -f claims_get_permissions_array
export -f claims_get_roles_array
export -f claims_has_role
export -f claims_has_permission
