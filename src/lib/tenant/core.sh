#!/usr/bin/env bash

#
# Tenant Core Library
# Multi-tenant management functions
#

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../utils/output.sh"
source "$SCRIPT_DIR/../utils/docker.sh"

# Source safe query library for SQL injection prevention
if [[ -f "$SCRIPT_DIR/../database/safe-query.sh" ]]; then
  source "$SCRIPT_DIR/../database/safe-query.sh"
fi

# ============================================================================
# Tenant Initialization
# ============================================================================

tenant_init() {
  info "Initializing multi-tenancy system..."

  # Check if PostgreSQL is running
  if ! docker_container_running "postgres"; then
    error "PostgreSQL is not running. Start it with: nself start"
    return 1
  fi

  # Run migration
  info "Running tenant system migration..."
  local migration_file="$ROOT_DIR/postgres/migrations/008_create_tenant_system.sql"

  if [[ ! -f "$migration_file" ]]; then
    error "Migration file not found: $migration_file"
    return 1
  fi

  # Execute migration
  if docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" <"$migration_file" >/dev/null 2>&1; then
    success "Multi-tenancy system initialized"
  else
    error "Failed to initialize multi-tenancy system"
    return 1
  fi

  # Create default tenant for existing data
  info "Creating default tenant for existing data..."
  tenant_create_default

  success "Multi-tenancy initialization complete"
}

# ============================================================================
# Tenant CRUD Operations
# ============================================================================

tenant_create() {
  local name="$1"
  local slug=""
  local plan="free"
  local owner_id=""

  # Parse arguments
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --slug)
        slug="$2"
        shift 2
        ;;
      --plan)
        plan="$2"
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

  # Generate slug if not provided
  if [[ -z "$slug" ]]; then
    slug=$(printf "%s" "$name" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | sed 's/[^a-z0-9-]//g')
  fi

  # Get owner ID if not provided (use first user in auth.users)
  if [[ -z "$owner_id" ]]; then
    owner_id=$(docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT id FROM auth.users LIMIT 1" | tr -d ' \n')
  fi

  if [[ -z "$owner_id" ]]; then
    error "No users found. Create a user first."
    return 1
  fi

  info "Creating tenant: $name (slug: $slug)"

  # Validate slug format (alphanumeric and hyphens only)
  if ! slug=$(validate_identifier "$slug" 100 2>/dev/null); then
    error "Invalid slug format (use only letters, numbers, hyphens, underscores)"
    return 1
  fi

  # Validate plan (should be alphanumeric identifier)
  if ! plan=$(validate_identifier "$plan" 50 2>/dev/null); then
    error "Invalid plan format"
    return 1
  fi

  # Validate owner_id as UUID
  if ! owner_id=$(validate_uuid "$owner_id" 2>/dev/null); then
    error "Invalid owner user ID format"
    return 1
  fi

  # Create tenant (SAFE - parameterized query)
  local result
  result=$(pg_query_value "
    INSERT INTO tenants.tenants (name, slug, plan_id, owner_user_id)
    VALUES (:'param1', :'param2', :'param3', :'param4')
    RETURNING id || ' ' || slug
  " "$name" "$slug" "$plan" "$owner_id" 2>&1)

  if [[ $? -ne 0 ]]; then
    error "Failed to create tenant: $result"
    return 1
  fi

  local tenant_id
  tenant_id=$(printf "%s" "$result" | awk '{print $1}')

  # Create tenant schema
  info "Creating tenant schema..."
  local schema_sql="SELECT tenants.create_tenant_schema('$tenant_id');"
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$schema_sql" >/dev/null 2>&1

  # Add owner as member
  local member_sql="
    INSERT INTO tenants.tenant_members (tenant_id, user_id, role)
    VALUES ('$tenant_id', '$owner_id', 'owner');
    "
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$member_sql" >/dev/null 2>&1

  success "Tenant created: $slug (ID: $tenant_id)"
  printf "  Owner: %s\n" "$owner_id"
  printf "  Plan: %s\n" "$plan"
}

tenant_create_default() {
  # Create default tenant for existing installation
  local default_name="Default Organization"
  local default_slug="default"

  # Check if default tenant exists
  local exists
  exists=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT COUNT(*) FROM tenants.tenants WHERE slug = '$default_slug'" | tr -d ' \n')

  if [[ "$exists" -gt 0 ]]; then
    info "Default tenant already exists"
    return 0
  fi

  # Create default tenant
  tenant_create "$default_name" --slug "$default_slug" --plan "enterprise"
}

tenant_list() {
  local json_output=false

  # Parse arguments
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

  # Query tenants
  local sql="
    SELECT
        id,
        slug,
        name,
        status,
        plan_id,
        created_at
    FROM tenants.tenants
    ORDER BY created_at DESC;
    "

  if [[ "$json_output" == true ]]; then
    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
      "SELECT json_agg(row_to_json(t)) FROM ($sql) t;"
  else
    printf "%-38s %-20s %-30s %-12s %-12s\n" "ID" "SLUG" "NAME" "STATUS" "PLAN"
    printf "%.0s-" {1..120}
    printf "\n"

    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql" | while read -r line; do
      printf "%s\n" "$line"
    done
  fi
}

tenant_show() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  # Query tenant details
  local sql="
    SELECT
        t.id,
        t.slug,
        t.name,
        t.status,
        t.plan_id,
        t.max_users,
        t.max_storage_gb,
        t.max_api_requests_per_month,
        t.created_at,
        t.owner_user_id,
        COUNT(tm.id) as member_count
    FROM tenants.tenants t
    LEFT JOIN tenants.tenant_members tm ON t.id = tm.tenant_id
    WHERE t.id = '$tenant_id' OR t.slug = '$tenant_id'
    GROUP BY t.id;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

tenant_suspend() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  info "Suspending tenant: $tenant_id"

  local sql="
    UPDATE tenants.tenants
    SET status = 'suspended', suspended_at = NOW()
    WHERE id = '$tenant_id' OR slug = '$tenant_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Tenant suspended: $tenant_id"
}

tenant_activate() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  info "Activating tenant: $tenant_id"

  local sql="
    UPDATE tenants.tenants
    SET status = 'active', suspended_at = NULL
    WHERE id = '$tenant_id' OR slug = '$tenant_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Tenant activated: $tenant_id"
}

tenant_delete() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  # Confirm deletion
  printf "Are you sure you want to delete tenant '%s'? This cannot be undone. (yes/no): " "$tenant_id"
  read -r confirmation

  if [[ "$confirmation" != "yes" ]]; then
    info "Deletion cancelled"
    return 0
  fi

  info "Deleting tenant: $tenant_id"

  # Validate tenant_id (could be UUID or slug)
  # Try UUID first, if that fails, validate as identifier
  local validated_id="$tenant_id"
  if ! validated_id=$(validate_uuid "$tenant_id" 2>/dev/null); then
    if ! validated_id=$(validate_identifier "$tenant_id" 100 2>/dev/null); then
      error "Invalid tenant ID or slug format"
      return 1
    fi
  fi

  # Drop tenant schema (SAFE - parameterized query)
  pg_query_safe "SELECT tenants.drop_tenant_schema(:'param1')" "$validated_id" >/dev/null 2>&1

  # Delete tenant (cascades to all related tables) - SAFE - parameterized query
  pg_query_safe "DELETE FROM tenants.tenants WHERE id = :'param1' OR slug = :'param1'" "$validated_id" >/dev/null 2>&1

  success "Tenant deleted: $tenant_id"
}

tenant_stats() {
  info "Tenant Statistics"
  printf "\n"

  # Overall stats
  local stats_sql="
    SELECT
        COUNT(*) as total_tenants,
        COUNT(*) FILTER (WHERE status = 'active') as active_tenants,
        COUNT(*) FILTER (WHERE status = 'suspended') as suspended_tenants,
        COUNT(*) FILTER (WHERE status = 'deleted') as deleted_tenants
    FROM tenants.tenants;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$stats_sql"

  printf "\n"

  # Tenants by plan
  local plan_sql="
    SELECT
        plan_id,
        COUNT(*) as count
    FROM tenants.tenants
    WHERE status = 'active'
    GROUP BY plan_id
    ORDER BY count DESC;
    "

  printf "Tenants by Plan:\n"
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$plan_sql"
}

# ============================================================================
# Tenant Member Management
# ============================================================================

tenant_member_add() {
  local tenant_id="$1"
  local user_id="$2"
  local role="${3:-member}"

  if [[ -z "$tenant_id" || -z "$user_id" ]]; then
    error "Tenant ID and User ID required"
    return 1
  fi

  info "Adding user $user_id to tenant $tenant_id as $role"

  # Validate inputs
  # tenant_id could be UUID or slug
  local validated_tenant_id="$tenant_id"
  if ! validated_tenant_id=$(validate_uuid "$tenant_id" 2>/dev/null); then
    if ! validated_tenant_id=$(validate_identifier "$tenant_id" 100 2>/dev/null); then
      error "Invalid tenant ID or slug format"
      return 1
    fi
  fi

  # user_id should be UUID
  if ! user_id=$(validate_uuid "$user_id" 2>/dev/null); then
    error "Invalid user ID format"
    return 1
  fi

  # role should be alphanumeric identifier
  if ! role=$(validate_identifier "$role" 50 2>/dev/null); then
    error "Invalid role format"
    return 1
  fi

  # Add member (SAFE - parameterized query)
  pg_query_safe "
    INSERT INTO tenants.tenant_members (tenant_id, user_id, role)
    SELECT t.id, :'param2', :'param3'
    FROM tenants.tenants t
    WHERE t.id = :'param1' OR t.slug = :'param1'
    ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = :'param3'
  " "$validated_tenant_id" "$user_id" "$role" >/dev/null 2>&1

  success "User added to tenant"
}

tenant_member_remove() {
  local tenant_id="$1"
  local user_id="$2"

  if [[ -z "$tenant_id" || -z "$user_id" ]]; then
    error "Tenant ID and User ID required"
    return 1
  fi

  info "Removing user $user_id from tenant $tenant_id"

  local sql="
    DELETE FROM tenants.tenant_members tm
    USING tenants.tenants t
    WHERE tm.tenant_id = t.id
    AND (t.id = '$tenant_id' OR t.slug = '$tenant_id')
    AND tm.user_id = '$user_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "User removed from tenant"
}

tenant_member_list() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  local sql="
    SELECT
        tm.user_id,
        tm.role,
        tm.joined_at
    FROM tenants.tenant_members tm
    INNER JOIN tenants.tenants t ON tm.tenant_id = t.id
    WHERE t.id = '$tenant_id' OR t.slug = '$tenant_id'
    ORDER BY tm.joined_at ASC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

# ============================================================================
# Tenant Domain Management
# ============================================================================

tenant_domain_add() {
  local tenant_id="$1"
  local domain="$2"

  if [[ -z "$tenant_id" || -z "$domain" ]]; then
    error "Tenant ID and domain required"
    return 1
  fi

  info "Adding domain $domain to tenant $tenant_id"

  # Generate verification token
  local token
  token=$(openssl rand -hex 32)

  local sql="
    INSERT INTO tenants.tenant_domains (tenant_id, domain, verification_token)
    SELECT t.id, '$domain', '$token'
    FROM tenants.tenants t
    WHERE t.id = '$tenant_id' OR t.slug = '$tenant_id';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Domain added: $domain"
  printf "  Verification token: %s\n" "$token"
  printf "  Add this TXT record to your DNS:\n"
  printf "    nself-verify=%s\n" "$token"
}

tenant_domain_verify() {
  local tenant_id="$1"
  local domain="$2"

  if [[ -z "$tenant_id" || -z "$domain" ]]; then
    error "Tenant ID and domain required"
    return 1
  fi

  info "Verifying domain: $domain"

  # Get verification token
  local token_sql="
    SELECT td.verification_token
    FROM tenants.tenant_domains td
    INNER JOIN tenants.tenants t ON td.tenant_id = t.id
    WHERE (t.id = '$tenant_id' OR t.slug = '$tenant_id')
    AND td.domain = '$domain';
    "

  local token
  token=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$token_sql" | tr -d ' \n')

  if [[ -z "$token" ]]; then
    error "Domain not found"
    return 1
  fi

  # Check DNS TXT record
  local txt_record
  txt_record=$(dig +short TXT "$domain" | grep "nself-verify=" | tr -d '"' | cut -d= -f2)

  if [[ "$txt_record" == "$token" ]]; then
    # Mark as verified
    local verify_sql="
        UPDATE tenants.tenant_domains td
        SET is_verified = true, verified_at = NOW()
        FROM tenants.tenants t
        WHERE td.tenant_id = t.id
        AND (t.id = '$tenant_id' OR t.slug = '$tenant_id')
        AND td.domain = '$domain';
        "

    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$verify_sql" >/dev/null 2>&1

    success "Domain verified: $domain"
  else
    error "Domain verification failed"
    printf "  Expected: nself-verify=%s\n" "$token"
    printf "  Found: %s\n" "$txt_record"
    return 1
  fi
}

tenant_domain_remove() {
  local tenant_id="$1"
  local domain="$2"

  if [[ -z "$tenant_id" || -z "$domain" ]]; then
    error "Tenant ID and domain required"
    return 1
  fi

  info "Removing domain: $domain"

  local sql="
    DELETE FROM tenants.tenant_domains td
    USING tenants.tenants t
    WHERE td.tenant_id = t.id
    AND (t.id = '$tenant_id' OR t.slug = '$tenant_id')
    AND td.domain = '$domain';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Domain removed: $domain"
}

tenant_domain_list() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  local sql="
    SELECT
        td.domain,
        td.is_primary,
        td.is_verified,
        td.verified_at,
        td.created_at
    FROM tenants.tenant_domains td
    INNER JOIN tenants.tenants t ON td.tenant_id = t.id
    WHERE t.id = '$tenant_id' OR t.slug = '$tenant_id'
    ORDER BY td.is_primary DESC, td.created_at ASC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}

# ============================================================================
# Tenant Settings Management
# ============================================================================

tenant_setting_set() {
  local tenant_id="$1"
  local key="$2"
  local value="$3"

  if [[ -z "$tenant_id" || -z "$key" || -z "$value" ]]; then
    error "Tenant ID, key, and value required"
    return 1
  fi

  info "Setting $key = $value for tenant $tenant_id"

  # Convert value to JSON
  local json_value
  json_value=$(printf '%s' "$value" | jq -R .)

  local sql="
    INSERT INTO tenants.tenant_settings (tenant_id, key, value)
    SELECT t.id, '$key', '$json_value'::jsonb
    FROM tenants.tenants t
    WHERE t.id = '$tenant_id' OR t.slug = '$tenant_id'
    ON CONFLICT (tenant_id, key) DO UPDATE SET value = '$json_value'::jsonb, updated_at = NOW();
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" >/dev/null 2>&1

  success "Setting updated"
}

tenant_setting_get() {
  local tenant_id="$1"
  local key="$2"

  if [[ -z "$tenant_id" || -z "$key" ]]; then
    error "Tenant ID and key required"
    return 1
  fi

  local sql="
    SELECT ts.value
    FROM tenants.tenant_settings ts
    INNER JOIN tenants.tenants t ON ts.tenant_id = t.id
    WHERE (t.id = '$tenant_id' OR t.slug = '$tenant_id')
    AND ts.key = '$key';
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c "$sql"
}

tenant_setting_list() {
  local tenant_id="$1"

  if [[ -z "$tenant_id" ]]; then
    error "Tenant ID required"
    return 1
  fi

  local sql="
    SELECT
        ts.key,
        ts.value,
        ts.updated_at
    FROM tenants.tenant_settings ts
    INNER JOIN tenants.tenants t ON ts.tenant_id = t.id
    WHERE t.id = '$tenant_id' OR t.slug = '$tenant_id'
    ORDER BY ts.key ASC;
    "

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql"
}
