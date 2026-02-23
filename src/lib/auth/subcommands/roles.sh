#!/usr/bin/env bash
# roles.sh - Role management CLI (ROLE-001)
# Part of nself v0.6.0 - Phase 1 Sprint 3
#
# Command-line interface for role and permission management

set -euo pipefail

# Get script directory and nself root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source required libraries
if [[ -z "${EXIT_SUCCESS:-}" ]]; then
  source "$NSELF_ROOT/src/lib/config/constants.sh" 2>/dev/null || true
fi
source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/role-manager.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/auth/permission-manager.sh" 2>/dev/null || true

# ============================================================================
# Role CLI Commands
# ============================================================================

# Main roles command handler
cmd_roles() {
  local subcommand="${1:-}"

  if [[ -z "$subcommand" ]]; then
    cmd_roles_help
    return 0
  fi

  shift

  case "$subcommand" in
    create)
      cmd_roles_create "$@"
      ;;
    list)
      cmd_roles_list "$@"
      ;;
    get)
      cmd_roles_get "$@"
      ;;
    update)
      cmd_roles_update "$@"
      ;;
    delete)
      cmd_roles_delete "$@"
      ;;
    assign)
      cmd_roles_assign "$@"
      ;;
    revoke)
      cmd_roles_revoke "$@"
      ;;
    permissions)
      cmd_roles_permissions "$@"
      ;;
    default)
      cmd_roles_default "$@"
      ;;
    help|--help|-h)
      cmd_roles_help
      ;;
    *)
      echo "ERROR: Unknown roles command: $subcommand" >&2
      cmd_roles_help
      return 1
      ;;
  esac
}

# ============================================================================
# Role CRUD Commands
# ============================================================================

# Create role
cmd_roles_create() {
  local role_name=""
  local description=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name=*)
        role_name="${1#*=}"
        shift
        ;;
      --description=*)
        description="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$role_name" ]]; then
    echo "ERROR: Role name required. Use --name=<name>" >&2
    return 1
  fi

  print_info "Creating role: $role_name"

  local role_id
  role_id=$(role_create "$role_name" "$description" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Failed to create role"
    echo "$role_id" >&2
    return 1
  fi

  print_success "Role created successfully"
  echo "  Role ID: $role_id"
  echo "  Name: $role_name"
  if [[ -n "$description" ]]; then
    echo "  Description: $description"
  fi

  return 0
}

# List roles
cmd_roles_list() {
  local format="table"
  local limit=50

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --format=*)
        format="${1#*=}"
        shift
        ;;
      --limit=*)
        limit="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  print_info "Listing roles..."

  local roles_json
  roles_json=$(role_list "$limit" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Failed to list roles"
    return 1
  fi

  if [[ "$format" == "json" ]]; then
    echo "$roles_json" | jq '.'
  else
    # Table format
    echo ""
    printf "%-36s  %-20s  %-10s  %-50s\n" "ID" "Name" "Default" "Description"
    printf "%-36s  %-20s  %-10s  %-50s\n" "------------------------------------" "--------------------" "----------" "--------------------------------------------------"

    echo "$roles_json" | jq -r '.[] | "\(.id)  \(.name)  \(if .is_default then "Yes" else "No" end)  \(.description // "")"' 2>/dev/null
    echo ""
  fi

  return 0
}

# Get role
cmd_roles_get() {
  local role_name=""
  local format="json"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name=*)
        role_name="${1#*=}"
        shift
        ;;
      --format=*)
        format="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$role_name" ]]; then
    echo "ERROR: Role name required. Use --name=<name>" >&2
    return 1
  fi

  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  if [[ "$format" == "json" ]]; then
    echo "$role_json" | jq '.'
  else
    print_info "Role details:"
    echo "$role_json" | jq -r '"
  ID: \(.id)
  Name: \(.name)
  Description: \(.description // "N/A")
  Default: \(if .is_default then "Yes" else "No" end)
  System: \(if .is_system then "Yes" else "No" end)
  Created: \(.created_at)"'
  fi

  return 0
}

# Delete role
cmd_roles_delete() {
  local role_name=""
  local confirm=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name=*)
        role_name="${1#*=}"
        shift
        ;;
      --confirm)
        confirm=true
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$role_name" ]]; then
    echo "ERROR: Role name required. Use --name=<name>" >&2
    return 1
  fi

  if [[ "$confirm" != "true" ]]; then
    print_warning "This will permanently delete the role '$role_name'"
    echo "Add --confirm to proceed"
    return 1
  fi

  # Get role ID
  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  local role_id
  role_id=$(echo "$role_json" | jq -r '.id')

  print_info "Deleting role: $role_name"

  if role_delete "$role_id"; then
    print_success "Role deleted successfully"
    return 0
  else
    print_error "Failed to delete role"
    return 1
  fi
}

# ============================================================================
# Role Assignment Commands
# ============================================================================

# Assign role to user
cmd_roles_assign() {
  local user_id=""
  local role_name=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --user=*)
        user_id="${1#*=}"
        shift
        ;;
      --role=*)
        role_name="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$user_id" ]] || [[ -z "$role_name" ]]; then
    echo "ERROR: User ID and role name required" >&2
    echo "Usage: nself roles assign --user=<user_id> --role=<role_name>" >&2
    return 1
  fi

  # Get role ID
  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  local role_id
  role_id=$(echo "$role_json" | jq -r '.id')

  print_info "Assigning role '$role_name' to user $user_id"

  if role_assign_user "$user_id" "$role_id"; then
    print_success "Role assigned successfully"
    return 0
  else
    print_error "Failed to assign role"
    return 1
  fi
}

# Revoke role from user
cmd_roles_revoke() {
  local user_id=""
  local role_name=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --user=*)
        user_id="${1#*=}"
        shift
        ;;
      --role=*)
        role_name="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$user_id" ]] || [[ -z "$role_name" ]]; then
    echo "ERROR: User ID and role name required" >&2
    echo "Usage: nself roles revoke --user=<user_id> --role=<role_name>" >&2
    return 1
  fi

  # Get role ID
  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  local role_id
  role_id=$(echo "$role_json" | jq -r '.id')

  print_info "Revoking role '$role_name' from user $user_id"

  if role_revoke_user "$user_id" "$role_id"; then
    print_success "Role revoked successfully"
    return 0
  else
    print_error "Failed to revoke role"
    return 1
  fi
}

# ============================================================================
# Permission Commands
# ============================================================================

# Manage role permissions
cmd_roles_permissions() {
  local action="${1:-}"

  if [[ -z "$action" ]]; then
    echo "ERROR: Action required. Use: add, remove, list" >&2
    return 1
  fi

  shift

  case "$action" in
    add)
      cmd_roles_permissions_add "$@"
      ;;
    remove)
      cmd_roles_permissions_remove "$@"
      ;;
    list)
      cmd_roles_permissions_list "$@"
      ;;
    *)
      echo "ERROR: Unknown action: $action" >&2
      return 1
      ;;
  esac
}

# Add permission to role
cmd_roles_permissions_add() {
  local role_name=""
  local resource=""
  local action=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --role=*)
        role_name="${1#*=}"
        shift
        ;;
      --resource=*)
        resource="${1#*=}"
        shift
        ;;
      --action=*)
        action="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$role_name" ]] || [[ -z "$resource" ]] || [[ -z "$action" ]]; then
    echo "ERROR: Role name, resource, and action required" >&2
    echo "Usage: nself roles permissions add --role=<name> --resource=<resource> --action=<action>" >&2
    return 1
  fi

  # Get role ID
  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  local role_id
  role_id=$(echo "$role_json" | jq -r '.id')

  # Create permission if it doesn't exist
  local perm_id
  perm_id=$(permission_create "$resource" "$action" "Permission: $resource:$action" 2>&1 || \
            permission_list "$resource" 1 0 2>/dev/null | jq -r ".[0].id" 2>/dev/null)

  if [[ -z "$perm_id" ]]; then
    print_error "Failed to create or find permission"
    return 1
  fi

  print_info "Adding permission $resource:$action to role '$role_name'"

  if permission_assign_role "$role_id" "$perm_id"; then
    print_success "Permission added successfully"
    return 0
  else
    print_error "Failed to add permission"
    return 1
  fi
}

# List role permissions
cmd_roles_permissions_list() {
  local role_name=""
  local format="table"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --role=*)
        role_name="${1#*=}"
        shift
        ;;
      --format=*)
        format="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$role_name" ]]; then
    echo "ERROR: Role name required. Use --role=<name>" >&2
    return 1
  fi

  # Get role ID
  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  local role_id
  role_id=$(echo "$role_json" | jq -r '.id')

  # Get permissions
  local perms_json
  perms_json=$(permission_get_role_permissions "$role_id" 2>&1)

  if [[ "$format" == "json" ]]; then
    echo "$perms_json" | jq '.'
  else
    print_info "Permissions for role '$role_name':"
    echo ""
    printf "%-20s  %-20s  %-50s\n" "Resource" "Action" "Description"
    printf "%-20s  %-20s  %-50s\n" "--------------------" "--------------------" "--------------------------------------------------"

    echo "$perms_json" | jq -r '.[] | "\(.resource)  \(.action)  \(.description // "")"' 2>/dev/null
    echo ""
  fi

  return 0
}

# ============================================================================
# Default Role Commands
# ============================================================================

# Manage default role
cmd_roles_default() {
  local action="${1:-}"

  if [[ -z "$action" ]]; then
    # Show current default
    local default_json
    default_json=$(role_get_default 2>&1)

    if [[ -z "$default_json" ]] || [[ "$default_json" == "{}" ]]; then
      print_info "No default role set"
    else
      print_info "Current default role:"
      echo "$default_json" | jq -r '"  Name: \(.name)\n  Description: \(.description // "N/A")"'
    fi
    return 0
  fi

  shift

  case "$action" in
    set)
      cmd_roles_default_set "$@"
      ;;
    get)
      cmd_roles_default
      ;;
    *)
      echo "ERROR: Unknown action: $action" >&2
      return 1
      ;;
  esac
}

# Set default role
cmd_roles_default_set() {
  local role_name=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --role=*)
        role_name="${1#*=}"
        shift
        ;;
      *)
        echo "ERROR: Unknown option: $1" >&2
        return 1
        ;;
    esac
  done

  if [[ -z "$role_name" ]]; then
    echo "ERROR: Role name required. Use --role=<name>" >&2
    return 1
  fi

  # Get role ID
  local role_json
  role_json=$(role_get_by_name "$role_name" 2>&1)

  if [[ $? -ne 0 ]]; then
    print_error "Role not found: $role_name"
    return 1
  fi

  local role_id
  role_id=$(echo "$role_json" | jq -r '.id')

  print_info "Setting '$role_name' as default role"

  if role_set_default "$role_id"; then
    print_success "Default role updated"
    return 0
  else
    print_error "Failed to set default role"
    return 1
  fi
}

# ============================================================================
# Help
# ============================================================================

cmd_roles_help() {
  cat <<EOF
nself roles - Role and permission management

USAGE:
  nself roles <command> [options]

COMMANDS:
  create          Create a new role
  list            List all roles
  get             Get role details
  delete          Delete a role
  assign          Assign role to user
  revoke          Revoke role from user
  permissions     Manage role permissions
  default         Manage default role
  help            Show this help message

EXAMPLES:
  # Create a role
  nself roles create --name=editor --description="Content editor role"

  # List all roles
  nself roles list

  # Get role details
  nself roles get --name=editor

  # Assign role to user
  nself roles assign --user=<user_id> --role=editor

  # Add permission to role
  nself roles permissions add --role=editor --resource=posts --action=create

  # List role permissions
  nself roles permissions list --role=editor

  # Set default role
  nself roles default set --role=user

For more information, visit: https://docs.nself.org/auth/roles
EOF
}

# Export main command
export -f cmd_roles

# Run command if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_roles "$@"
fi
