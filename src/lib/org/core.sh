#!/usr/bin/env bash

#
# Organization Core Library
# Organization and team management functions
#

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../utils/output.sh"
source "$SCRIPT_DIR/../utils/docker.sh"

# ============================================================================
# Organization Initialization
# ============================================================================

org_init() {
  info "Initializing organization system..."

  if ! docker_container_running "postgres"; then
    error "PostgreSQL is not running. Start it with: nself start"
    return 1
  fi

  local migration_file="$ROOT_DIR/postgres/migrations/010_create_organization_system.sql"

  if [[ ! -f "$migration_file" ]]; then
    error "Migration file not found: $migration_file"
    return 1
  fi

  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" <"$migration_file" >/dev/null 2>&1; then
    success "Organization system initialized"
  else
    error "Failed to initialize organization system"
    return 1
  fi

  # Create default organization
  info "Creating default organization..."
  org_create_default

  success "Organization initialization complete"
}

org_create_default() {
  local default_name="Default Organization"
  local default_slug="default"

  local exists
  exists=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT COUNT(*) FROM organizations.organizations WHERE slug = '$default_slug'" | tr -d ' \n')

  if [[ "$exists" -gt 0 ]]; then
    info "Default organization already exists"
    return 0
  fi

  org_create "$default_name" --slug "$default_slug"
}

# ============================================================================
# Organization CRUD
# ============================================================================

org_create() {
  local name="$1"
  local slug=""
  local owner_id=""

  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --slug)
        slug="$2"
        shift 2
        ;;
      --owner)
        owner_id="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ -z "$slug" ]]; then
    slug=$(printf "%s" "$name" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | sed 's/[^a-z0-9-]//g')
  fi

  if [[ -z "$owner_id" ]]; then
    owner_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM auth.users LIMIT 1" | tr -d ' \n')
  fi

  if [[ -z "$owner_id" ]]; then
    error "No users found. Create a user first."
    return 1
  fi

  info "Creating organization: $name (slug: $slug)"

  local sql="
    INSERT INTO organizations.organizations (name, slug, owner_user_id)
    VALUES ('$name', '$slug', '$owner_id')
    RETURNING id, slug;
    "

  local result
  result=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" 2>&1)

  if [[ $? -ne 0 ]]; then
    error "Failed to create organization: $result"
    return 1
  fi

  local org_id
  org_id=$(printf "%s" "$result" | awk '{print $1}')

  # Add owner as member
  local member_sql="
    INSERT INTO organizations.org_members (org_id, user_id, role)
    VALUES ('$org_id', '$owner_id', 'owner');
    "
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$member_sql" >/dev/null 2>&1

  success "Organization created: $slug (ID: $org_id)"
}

org_list() {
  local json_output=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --json)
        json_output=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  local sql="
    SELECT
        id,
        slug,
        name,
        status,
        billing_plan,
        created_at
    FROM organizations.organizations
    ORDER BY created_at DESC;
    "

  if [[ "$json_output" == true ]]; then
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT json_agg(row_to_json(o)) FROM ($sql) o;"
  else
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
  fi
}

org_show() {
  local org_id="$1"

  if [[ -z "$org_id" ]]; then
    error "Organization ID required"
    return 1
  fi

  local sql="
    SELECT
        o.*,
        COUNT(DISTINCT om.user_id) as member_count,
        COUNT(DISTINCT t.id) as team_count
    FROM organizations.organizations o
    LEFT JOIN organizations.org_members om ON o.id = om.org_id
    LEFT JOIN organizations.teams t ON o.id = t.org_id
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    GROUP BY o.id;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

org_delete() {
  local org_id="$1"

  if [[ -z "$org_id" ]]; then
    error "Organization ID required"
    return 1
  fi

  printf "Are you sure you want to delete organization '%s'? (yes/no): " "$org_id"
  read -r confirmation

  if [[ "$confirmation" != "yes" ]]; then
    info "Deletion cancelled"
    return 0
  fi

  info "Deleting organization: $org_id"

  local sql="DELETE FROM organizations.organizations WHERE id = '$org_id' OR slug = '$org_id';"
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Organization deleted: $org_id"
}

# ============================================================================
# Organization Member Management
# ============================================================================

org_member_add() {
  local org_id="$1"
  local user_id="$2"
  local role="${3:-member}"

  if [[ -z "$org_id" || -z "$user_id" ]]; then
    error "Organization ID and User ID required"
    return 1
  fi

  info "Adding user $user_id to organization $org_id as $role"

  local sql="
    INSERT INTO organizations.org_members (org_id, user_id, role)
    SELECT o.id, '$user_id', '$role'
    FROM organizations.organizations o
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    ON CONFLICT (org_id, user_id) DO UPDATE SET role = '$role';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "User added to organization"
}

org_member_remove() {
  local org_id="$1"
  local user_id="$2"

  if [[ -z "$org_id" || -z "$user_id" ]]; then
    error "Organization ID and User ID required"
    return 1
  fi

  info "Removing user $user_id from organization $org_id"

  local sql="
    DELETE FROM organizations.org_members om
    USING organizations.organizations o
    WHERE om.org_id = o.id
    AND (o.id = '$org_id' OR o.slug = '$org_id')
    AND om.user_id = '$user_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "User removed from organization"
}

org_member_list() {
  local org_id="$1"

  if [[ -z "$org_id" ]]; then
    error "Organization ID required"
    return 1
  fi

  local sql="
    SELECT
        om.user_id,
        om.role,
        om.joined_at
    FROM organizations.org_members om
    INNER JOIN organizations.organizations o ON om.org_id = o.id
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    ORDER BY om.joined_at ASC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

org_member_change_role() {
  local org_id="$1"
  local user_id="$2"
  local new_role="$3"

  if [[ -z "$org_id" || -z "$user_id" || -z "$new_role" ]]; then
    error "Organization ID, User ID, and role required"
    return 1
  fi

  info "Changing role of $user_id in $org_id to $new_role"

  local sql="
    UPDATE organizations.org_members om
    SET role = '$new_role'
    FROM organizations.organizations o
    WHERE om.org_id = o.id
    AND (o.id = '$org_id' OR o.slug = '$org_id')
    AND om.user_id = '$user_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Role updated"
}

# ============================================================================
# Team Management
# ============================================================================

org_team_create() {
  local org_id="$1"
  local team_name="$2"

  if [[ -z "$org_id" || -z "$team_name" ]]; then
    error "Organization ID and team name required"
    return 1
  fi

  local team_slug
  team_slug=$(printf "%s" "$team_name" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | sed 's/[^a-z0-9-]//g')

  info "Creating team: $team_name"

  local sql="
    INSERT INTO organizations.teams (org_id, name, slug)
    SELECT o.id, '$team_name', '$team_slug'
    FROM organizations.organizations o
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    RETURNING id;
    "

  local team_id
  team_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" | tr -d ' \n')

  success "Team created: $team_name (ID: $team_id)"
}

org_team_list() {
  local org_id="$1"

  if [[ -z "$org_id" ]]; then
    error "Organization ID required"
    return 1
  fi

  local sql="
    SELECT
        t.id,
        t.slug,
        t.name,
        COUNT(tm.user_id) as member_count,
        t.created_at
    FROM organizations.teams t
    INNER JOIN organizations.organizations o ON t.org_id = o.id
    LEFT JOIN organizations.team_members tm ON t.id = tm.team_id
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    GROUP BY t.id
    ORDER BY t.created_at DESC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

org_team_show() {
  local team_id="$1"

  if [[ -z "$team_id" ]]; then
    error "Team ID required"
    return 1
  fi

  local sql="
    SELECT
        t.*,
        COUNT(tm.user_id) as member_count
    FROM organizations.teams t
    LEFT JOIN organizations.team_members tm ON t.id = tm.team_id
    WHERE t.id = '$team_id' OR t.slug = '$team_id'
    GROUP BY t.id;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

org_team_delete() {
  local team_id="$1"

  if [[ -z "$team_id" ]]; then
    error "Team ID required"
    return 1
  fi

  info "Deleting team: $team_id"

  local sql="DELETE FROM organizations.teams WHERE id = '$team_id' OR slug = '$team_id';"
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Team deleted"
}

org_team_add_member() {
  local team_id="$1"
  local user_id="$2"
  local role="${3:-member}"

  if [[ -z "$team_id" || -z "$user_id" ]]; then
    error "Team ID and User ID required"
    return 1
  fi

  info "Adding user $user_id to team $team_id as $role"

  local sql="
    INSERT INTO organizations.team_members (team_id, user_id, role)
    SELECT t.id, '$user_id', '$role'
    FROM organizations.teams t
    WHERE t.id = '$team_id' OR t.slug = '$team_id'
    ON CONFLICT (team_id, user_id) DO UPDATE SET role = '$role';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "User added to team"
}

org_team_remove_member() {
  local team_id="$1"
  local user_id="$2"

  if [[ -z "$team_id" || -z "$user_id" ]]; then
    error "Team ID and User ID required"
    return 1
  fi

  info "Removing user $user_id from team $team_id"

  local sql="
    DELETE FROM organizations.team_members tm
    USING organizations.teams t
    WHERE tm.team_id = t.id
    AND (t.id = '$team_id' OR t.slug = '$team_id')
    AND tm.user_id = '$user_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "User removed from team"
}

# ============================================================================
# Role Management
# ============================================================================

org_role_create() {
  local org_id="$1"
  local role_name="$2"

  if [[ -z "$org_id" || -z "$role_name" ]]; then
    error "Organization ID and role name required"
    return 1
  fi

  info "Creating role: $role_name"

  local sql="
    INSERT INTO permissions.roles (org_id, name)
    SELECT o.id, '$role_name'
    FROM organizations.organizations o
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    RETURNING id;
    "

  local role_id
  role_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" | tr -d ' \n')

  success "Role created: $role_name (ID: $role_id)"
}

org_role_list() {
  local org_id="$1"

  if [[ -z "$org_id" ]]; then
    error "Organization ID required"
    return 1
  fi

  local sql="
    SELECT
        r.id,
        r.name,
        r.description,
        r.is_builtin,
        COUNT(rp.permission_id) as permission_count,
        r.created_at
    FROM permissions.roles r
    INNER JOIN organizations.organizations o ON r.org_id = o.id
    LEFT JOIN permissions.role_permissions rp ON r.id = rp.role_id
    WHERE o.id = '$org_id' OR o.slug = '$org_id'
    GROUP BY r.id
    ORDER BY r.created_at DESC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

org_role_assign() {
  local org_id="$1"
  local user_id="$2"
  local role_name="$3"

  if [[ -z "$org_id" || -z "$user_id" || -z "$role_name" ]]; then
    error "Organization ID, User ID, and role name required"
    return 1
  fi

  info "Assigning role $role_name to user $user_id"

  local sql="
    INSERT INTO permissions.user_roles (user_id, role_id, org_id)
    SELECT '$user_id', r.id, o.id
    FROM permissions.roles r
    INNER JOIN organizations.organizations o ON r.org_id = o.id
    WHERE (o.id = '$org_id' OR o.slug = '$org_id')
    AND r.name = '$role_name'
    ON CONFLICT DO NOTHING;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Role assigned"
}

org_role_revoke() {
  local org_id="$1"
  local user_id="$2"
  local role_name="$3"

  if [[ -z "$org_id" || -z "$user_id" || -z "$role_name" ]]; then
    error "Organization ID, User ID, and role name required"
    return 1
  fi

  info "Revoking role $role_name from user $user_id"

  local sql="
    DELETE FROM permissions.user_roles ur
    USING permissions.roles r, organizations.organizations o
    WHERE ur.role_id = r.id
    AND r.org_id = o.id
    AND ur.user_id = '$user_id'
    AND (o.id = '$org_id' OR o.slug = '$org_id')
    AND r.name = '$role_name';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Role revoked"
}

# ============================================================================
# Permission Management
# ============================================================================

org_permission_grant() {
  local role_name="$1"
  local permission_name="$2"

  if [[ -z "$role_name" || -z "$permission_name" ]]; then
    error "Role name and permission name required"
    return 1
  fi

  info "Granting permission $permission_name to role $role_name"

  local sql="
    INSERT INTO permissions.role_permissions (role_id, permission_id)
    SELECT r.id, p.id
    FROM permissions.roles r, permissions.permissions p
    WHERE r.name = '$role_name'
    AND p.name = '$permission_name'
    ON CONFLICT DO NOTHING;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Permission granted"
}

org_permission_revoke() {
  local role_name="$1"
  local permission_name="$2"

  if [[ -z "$role_name" || -z "$permission_name" ]]; then
    error "Role name and permission name required"
    return 1
  fi

  info "Revoking permission $permission_name from role $role_name"

  local sql="
    DELETE FROM permissions.role_permissions rp
    USING permissions.roles r, permissions.permissions p
    WHERE rp.role_id = r.id
    AND rp.permission_id = p.id
    AND r.name = '$role_name'
    AND p.name = '$permission_name';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Permission revoked"
}

org_permission_list() {
  local sql="
    SELECT
        name,
        resource_type,
        action,
        description
    FROM permissions.permissions
    ORDER BY resource_type, action;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}
