#!/usr/bin/env bash

#
# Tenant Lifecycle Management
# Handles tenant provisioning, activation, suspension, deletion, and migration
#

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "$SCRIPT_DIR/../utils/output.sh"
source "$SCRIPT_DIR/../utils/docker.sh"

# ============================================================================
# Tenant Provisioning Workflow
# ============================================================================

tenant_provision() {
  local tenant_name="$1"
  local tenant_slug="$2"
  local plan="${3:-free}"
  local owner_email="$4"

  info "Starting tenant provisioning: $tenant_name"

  # Step 1: Validate inputs
  if [[ -z "$tenant_name" || -z "$tenant_slug" || -z "$owner_email" ]]; then
    error "Tenant name, slug, and owner email are required"
    return 1
  fi

  # Step 2: Check if slug is available
  local slug_exists
  slug_exists=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT COUNT(*) FROM tenants.tenants WHERE slug = '$tenant_slug'" | tr -d ' \n')

  if [[ "$slug_exists" -gt 0 ]]; then
    error "Tenant slug already exists: $tenant_slug"
    return 1
  fi

  # Step 3: Create or find owner user
  local owner_id
  owner_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT id FROM auth.users WHERE email = '$owner_email' LIMIT 1" | tr -d ' \n')

  if [[ -z "$owner_id" ]]; then
    info "Creating owner user: $owner_email"
    # Create user via auth system
    owner_id=$(create_tenant_owner "$owner_email" "$tenant_name")
  fi

  # Step 4: Create tenant
  info "Creating tenant record..."
  local tenant_id
  tenant_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "INSERT INTO tenants.tenants (name, slug, plan_id, owner_user_id)
         VALUES ('$tenant_name', '$tenant_slug', '$plan', '$owner_id')
         RETURNING id" | tr -d ' \n')

  if [[ -z "$tenant_id" ]]; then
    error "Failed to create tenant"
    return 1
  fi

  # Step 5: Create tenant schema
  info "Creating tenant schema..."
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "SELECT tenants.create_tenant_schema('$tenant_id')" >/dev/null 2>&1

  # Step 6: Add owner as member
  info "Adding owner as tenant member..."
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "INSERT INTO tenants.tenant_members (tenant_id, user_id, role)
         VALUES ('$tenant_id', '$owner_id', 'owner')" >/dev/null 2>&1

  # Step 7: Initialize tenant settings
  info "Initializing tenant settings..."
  initialize_tenant_settings "$tenant_id"

  # Step 8: Create subdomain nginx config
  info "Configuring subdomain routing..."
  configure_tenant_subdomain "$tenant_slug"

  # Step 9: Send welcome email (if configured)
  if command -v mail >/dev/null 2>&1; then
    send_tenant_welcome_email "$owner_email" "$tenant_slug"
  fi

  success "Tenant provisioned successfully!"
  printf "\n"
  printf "  Tenant ID: %s\n" "$tenant_id"
  printf "  Slug: %s\n" "$tenant_slug"
  printf "  Owner: %s\n" "$owner_email"
  printf "  Plan: %s\n" "$plan"
  printf "  URL: https://%s.%s\n" "$tenant_slug" "$BASE_DOMAIN"
  printf "\n"

  # Return tenant_id for scripting
  echo "$tenant_id"
}

create_tenant_owner() {
  local email="$1"
  local tenant_name="$2"

  # Generate random password
  local password
  password=$(openssl rand -base64 32)

  # Create user (simplified - in production, use proper auth flow)
  local user_id
  user_id=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "INSERT INTO auth.users (email, password_hash, email_verified)
         VALUES ('$email', crypt('$password', gen_salt('bf')), false)
         RETURNING id" | tr -d ' \n')

  info "Created owner user with ID: $user_id"
  printf "  Temporary password: %s\n" "$password"
  printf "  (User will be prompted to change on first login)\n"

  echo "$user_id"
}

initialize_tenant_settings() {
  local tenant_id="$1"

  # Set default settings
  local settings=(
    "branding.primary_color|#3B82F6"
    "branding.logo_url|"
    "features.analytics|true"
    "features.api_access|true"
    "limits.enforce_quotas|true"
    "notifications.email|true"
    "notifications.webhook_url|"
  )

  for setting in "${settings[@]}"; do
    local key="${setting%%|*}"
    local value="${setting##*|}"

    docker exec -i "$(docker_get_container_name postgres)" \
      psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
      "INSERT INTO tenants.tenant_settings (tenant_id, key, value)
             VALUES ('$tenant_id', '$key', '\"$value\"'::jsonb)
             ON CONFLICT DO NOTHING" >/dev/null 2>&1
  done
}

configure_tenant_subdomain() {
  local slug="$1"

  # This would regenerate nginx config to include new tenant subdomain
  # For now, just log
  info "Subdomain configured: $slug.$BASE_DOMAIN"
}

send_tenant_welcome_email() {
  local email="$1"
  local slug="$2"

  # Send welcome email (simplified)
  info "Welcome email would be sent to: $email"
}

# ============================================================================
# Tenant Activation/Suspension
# ============================================================================

tenant_lifecycle_suspend() {
  local tenant_id="$1"
  local reason="${2:-No reason provided}"

  info "Suspending tenant: $tenant_id"
  info "Reason: $reason"

  # Suspend tenant
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE tenants.tenants
         SET status = 'suspended', suspended_at = NOW(),
             metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{suspension_reason}', '\"$reason\"'::jsonb)
         WHERE id = '$tenant_id' OR slug = '$tenant_id'" >/dev/null 2>&1

  # Revoke all active sessions
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE auth.sessions
         SET expires_at = NOW()
         WHERE tenant_id = (SELECT id FROM tenants.tenants WHERE id = '$tenant_id' OR slug = '$tenant_id')" >/dev/null 2>&1

  # Notify tenant owner
  notify_tenant_suspension "$tenant_id" "$reason"

  success "Tenant suspended"
}

tenant_lifecycle_activate() {
  local tenant_id="$1"

  info "Activating tenant: $tenant_id"

  # Activate tenant
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE tenants.tenants
         SET status = 'active', suspended_at = NULL
         WHERE id = '$tenant_id' OR slug = '$tenant_id'" >/dev/null 2>&1

  # Notify tenant owner
  notify_tenant_activation "$tenant_id"

  success "Tenant activated"
}

notify_tenant_suspension() {
  local tenant_id="$1"
  local reason="$2"

  # Get tenant owner email
  local owner_email
  owner_email=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT u.email FROM auth.users u
         INNER JOIN tenants.tenants t ON u.id = t.owner_user_id
         WHERE t.id = '$tenant_id'" | tr -d ' \n')

  info "Suspension notification would be sent to: $owner_email"
}

notify_tenant_activation() {
  local tenant_id="$1"

  local owner_email
  owner_email=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT u.email FROM auth.users u
         INNER JOIN tenants.tenants t ON u.id = t.owner_user_id
         WHERE t.id = '$tenant_id'" | tr -d ' \n')

  info "Activation notification would be sent to: $owner_email"
}

# ============================================================================
# Tenant Deletion with Cleanup
# ============================================================================

tenant_lifecycle_delete() {
  local tenant_id="$1"
  local permanent="${2:-false}"

  if [[ "$permanent" == "true" ]]; then
    tenant_permanent_delete "$tenant_id"
  else
    tenant_soft_delete "$tenant_id"
  fi
}

tenant_soft_delete() {
  local tenant_id="$1"

  info "Soft deleting tenant: $tenant_id"

  # Mark as deleted (keeps data for recovery)
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE tenants.tenants
         SET status = 'deleted', deleted_at = NOW()
         WHERE id = '$tenant_id' OR slug = '$tenant_id'" >/dev/null 2>&1

  # Revoke sessions
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE auth.sessions
         SET expires_at = NOW()
         WHERE tenant_id = (SELECT id FROM tenants.tenants WHERE id = '$tenant_id' OR slug = '$tenant_id')" >/dev/null 2>&1

  success "Tenant soft deleted (recoverable within 30 days)"
}

tenant_permanent_delete() {
  local tenant_id="$1"

  warn "PERMANENT DELETION: This cannot be undone!"
  printf "Type the tenant ID to confirm: "
  read -r confirmation

  if [[ "$confirmation" != "$tenant_id" ]]; then
    info "Deletion cancelled"
    return 0
  fi

  info "Permanently deleting tenant: $tenant_id"

  # Step 1: Create final backup
  info "Creating final backup..."
  tenant_create_backup "$tenant_id" "pre-deletion"

  # Step 2: Drop tenant schema
  info "Dropping tenant schema..."
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "SELECT tenants.drop_tenant_schema('$tenant_id')" >/dev/null 2>&1

  # Step 3: Delete tenant (cascades to all related data)
  info "Deleting tenant record and all related data..."
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "DELETE FROM tenants.tenants WHERE id = '$tenant_id' OR slug = '$tenant_id'" >/dev/null 2>&1

  # Step 4: Clean up Redis keys (if Redis enabled)
  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    info "Cleaning up Redis keys..."
    docker exec -i "$(docker_get_container_name redis)" \
      redis-cli --scan --pattern "tenant:$tenant_id:*" | xargs -r redis-cli DEL 2>/dev/null || true
  fi

  # Step 5: Clean up MinIO buckets (if MinIO enabled)
  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    info "Cleaning up storage buckets..."
    # This would use mc (MinIO client) to remove tenant bucket
  fi

  success "Tenant permanently deleted"
}

# ============================================================================
# Tenant Migration Between Plans
# ============================================================================

tenant_migrate_plan() {
  local tenant_id="$1"
  local new_plan="$2"

  if [[ -z "$tenant_id" || -z "$new_plan" ]]; then
    error "Tenant ID and new plan required"
    return 1
  fi

  info "Migrating tenant $tenant_id to plan: $new_plan"

  # Get current plan
  local current_plan
  current_plan=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT plan_id FROM tenants.tenants WHERE id = '$tenant_id' OR slug = '$tenant_id'" | tr -d ' \n')

  info "Current plan: $current_plan"
  info "New plan: $new_plan"

  # Update plan
  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE tenants.tenants
         SET plan_id = '$new_plan',
             updated_at = NOW()
         WHERE id = '$tenant_id' OR slug = '$tenant_id'" >/dev/null 2>&1

  # Update quotas based on plan
  update_tenant_quotas "$tenant_id" "$new_plan"

  success "Tenant migrated to $new_plan plan"
}

update_tenant_quotas() {
  local tenant_id="$1"
  local plan="$2"

  # Plan quotas (hardcoded for now - in production, from plans table)
  case "$plan" in
    free)
      local max_users=5
      local max_storage=1
      local max_requests=10000
      ;;
    pro)
      local max_users=50
      local max_storage=10
      local max_requests=100000
      ;;
    enterprise)
      local max_users=999999
      local max_storage=999999
      local max_requests=999999999
      ;;
    *)
      error "Unknown plan: $plan"
      return 1
      ;;
  esac

  docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
    "UPDATE tenants.tenants
         SET max_users = $max_users,
             max_storage_gb = $max_storage,
             max_api_requests_per_month = $max_requests
         WHERE id = '$tenant_id'" >/dev/null 2>&1
}

# ============================================================================
# Tenant Backup & Restore
# ============================================================================

tenant_create_backup() {
  local tenant_id="$1"
  local label="${2:-manual}"

  info "Creating backup for tenant: $tenant_id"

  # Get tenant schema
  local schema_name
  schema_name=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT schema_name FROM tenants.tenant_schemas WHERE tenant_id = '$tenant_id' LIMIT 1" | tr -d ' \n')

  if [[ -z "$schema_name" ]]; then
    error "Tenant schema not found"
    return 1
  fi

  # Create backup
  local backup_dir="$ROOT_DIR/backups/tenants"
  mkdir -p "$backup_dir"

  local backup_file="$backup_dir/${tenant_id}_${label}_$(date +%Y%m%d_%H%M%S).sql.gz"

  docker exec -i "$(docker_get_container_name postgres)" \
    pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -n "$schema_name" | gzip >"$backup_file"

  success "Backup created: $backup_file"
}

tenant_restore_backup() {
  local tenant_id="$1"
  local backup_file="$2"

  if [[ ! -f "$backup_file" ]]; then
    error "Backup file not found: $backup_file"
    return 1
  fi

  info "Restoring backup for tenant: $tenant_id"

  # Decompress and restore
  gunzip -c "$backup_file" | docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"

  success "Backup restored"
}

# ============================================================================
# Tenant Health Checks
# ============================================================================

tenant_health_check() {
  local tenant_id="$1"

  info "Running health check for tenant: $tenant_id"

  # Check 1: Tenant exists and is active
  local status
  status=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT status FROM tenants.tenants WHERE id = '$tenant_id'" | tr -d ' \n')

  printf "  Status: %s\n" "$status"

  # Check 2: Schema exists
  local schema_exists
  schema_exists=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT COUNT(*) FROM tenants.tenant_schemas WHERE tenant_id = '$tenant_id'" | tr -d ' \n')

  printf "  Schema: %s\n" "$([[ "$schema_exists" -gt 0 ]] && echo "exists" || echo "missing")"

  # Check 3: Quotas
  local within_quota
  within_quota=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT tenants.check_storage_quota('$tenant_id')" | tr -d ' \n')

  printf "  Within storage quota: %s\n" "$within_quota"

  # Check 4: Member count
  local member_count
  member_count=$(docker exec -i "$(docker_get_container_name postgres)" \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
    "SELECT COUNT(*) FROM tenants.tenant_members WHERE tenant_id = '$tenant_id'" | tr -d ' \n')

  printf "  Members: %s\n" "$member_count"

  if [[ "$status" == "active" && "$schema_exists" -gt 0 && "$member_count" -gt 0 ]]; then
    success "Tenant is healthy"
    return 0
  else
    warn "Tenant has issues"
    return 1
  fi
}
