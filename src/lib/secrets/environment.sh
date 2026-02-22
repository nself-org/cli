#!/usr/bin/env bash
# environment.sh - Environment-based secrets (SEC-006)
# Part of nself v0.6.0 - Phase 1 Sprint 4
#
# Manages secrets per environment (dev, staging, prod)


# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

if [[ -f "$SCRIPT_DIR/vault.sh" ]]; then
  source "$SCRIPT_DIR/vault.sh"
fi
if [[ -f "$SCRIPT_DIR/audit.sh" ]]; then
  source "$SCRIPT_DIR/audit.sh"
fi

# Environment types
readonly ENV_DEFAULT="default"
readonly ENV_DEV="dev"
readonly ENV_STAGING="staging"
readonly ENV_PROD="prod"

# ============================================================================
# Environment Management
# ============================================================================

# Set secret for specific environment
# Usage: env_set_secret <key_name> <value> <environment> [description] [expires_days]
env_set_secret() {
  local key_name="$1"
  local value="$2"
  local environment="$3"
  local description="${4:-}"
  local expires_days="${5:-}"

  if [[ -z "$key_name" ]] || [[ -z "$value" ]] || [[ -z "$environment" ]]; then
    echo "ERROR: Key name, value, and environment required" >&2
    return 1
  fi

  # Validate environment
  if ! env_validate_environment "$environment"; then
    echo "ERROR: Invalid environment: $environment" >&2
    echo "Valid environments: $ENV_DEFAULT, $ENV_DEV, $ENV_STAGING, $ENV_PROD" >&2
    return 1
  fi

  # Store secret
  local secret_id
  secret_id=$(vault_set "$key_name" "$value" "$environment" "$description" "$expires_days")

  if [[ $? -ne 0 ]]; then
    audit_log "set" "$key_name" "$environment" "failure" "" "" "Failed to store secret"
    return 1
  fi

  # Log audit
  audit_log "set" "$key_name" "$environment" "success" "$secret_id"

  echo "$secret_id"
  return 0
}

# Get secret for specific environment
# Usage: env_get_secret <key_name> <environment>
env_get_secret() {
  local key_name="$1"
  local environment="$2"

  if [[ -z "$key_name" ]] || [[ -z "$environment" ]]; then
    echo "ERROR: Key name and environment required" >&2
    return 1
  fi

  # Validate environment
  if ! env_validate_environment "$environment"; then
    echo "ERROR: Invalid environment: $environment" >&2
    return 1
  fi

  # Get secret
  local value
  value=$(vault_get "$key_name" "$environment")

  if [[ $? -ne 0 ]]; then
    audit_log "get" "$key_name" "$environment" "failure" "" "" "Secret not found"
    return 1
  fi

  # Log audit
  audit_log "get" "$key_name" "$environment" "success"

  echo "$value"
  return 0
}

# Delete secret for specific environment
# Usage: env_delete_secret <key_name> <environment>
env_delete_secret() {
  local key_name="$1"
  local environment="$2"

  if [[ -z "$key_name" ]] || [[ -z "$environment" ]]; then
    echo "ERROR: Key name and environment required" >&2
    return 1
  fi

  # Validate environment
  if ! env_validate_environment "$environment"; then
    echo "ERROR: Invalid environment: $environment" >&2
    return 1
  fi

  # Delete secret
  if ! vault_delete "$key_name" "$environment"; then
    audit_log "delete" "$key_name" "$environment" "failure" "" "" "Failed to delete secret"
    return 1
  fi

  # Log audit
  audit_log "delete" "$key_name" "$environment" "success"

  return 0
}

# List secrets for environment
# Usage: env_list_secrets <environment>
env_list_secrets() {
  local environment="$1"

  if [[ -z "$environment" ]]; then
    echo "ERROR: Environment required" >&2
    return 1
  fi

  # Validate environment
  if ! env_validate_environment "$environment"; then
    echo "ERROR: Invalid environment: $environment" >&2
    return 1
  fi

  vault_list "$environment"
  return $?
}

# ============================================================================
# Environment Comparison
# ============================================================================

# Compare secrets across environments
# Usage: env_compare <env1> <env2>
env_compare() {
  local env1="$1"
  local env2="$2"

  if [[ -z "$env1" ]] || [[ -z "$env2" ]]; then
    echo "ERROR: Two environments required for comparison" >&2
    return 1
  fi

  # Get secrets for both environments
  local secrets1
  local secrets2
  secrets1=$(vault_list "$env1")
  secrets2=$(vault_list "$env2")

  # Extract key names
  local keys1
  local keys2
  keys1=$(echo "$secrets1" | jq -r '.[].key_name' | sort)
  keys2=$(echo "$secrets2" | jq -r '.[].key_name' | sort)

  # Find differences
  local only_in_env1=()
  local only_in_env2=()
  local common=()

  while IFS= read -r key; do
    if echo "$keys2" | grep -q "^${key}$"; then
      common+=("$key")
    else
      only_in_env1+=("$key")
    fi
  done <<<"$keys1"

  while IFS= read -r key; do
    if ! echo "$keys1" | grep -q "^${key}$"; then
      only_in_env2+=("$key")
    fi
  done <<<"$keys2"

  # Build comparison JSON
  local comparison
  comparison=$(jq -n \
    --arg env1 "$env1" \
    --arg env2 "$env2" \
    --argjson only1 "$(printf '%s\n' "${only_in_env1[@]}" | jq -R . | jq -s .)" \
    --argjson only2 "$(printf '%s\n' "${only_in_env2[@]}" | jq -R . | jq -s .)" \
    --argjson common "$(printf '%s\n' "${common[@]}" | jq -R . | jq -s .)" \
    '{
      environment_1: $env1,
      environment_2: $env2,
      only_in_env1: $only1,
      only_in_env2: $only2,
      common: $common,
      total_env1: ($only1 | length) + ($common | length),
      total_env2: ($only2 | length) + ($common | length)
    }')

  echo "$comparison"
  return 0
}

# ============================================================================
# Environment Sync
# ============================================================================

# Sync secret from one environment to another
# Usage: env_sync_secret <key_name> <source_env> <target_env>
env_sync_secret() {
  local key_name="$1"
  local source_env="$2"
  local target_env="$3"

  if [[ -z "$key_name" ]] || [[ -z "$source_env" ]] || [[ -z "$target_env" ]]; then
    echo "ERROR: Key name, source environment, and target environment required" >&2
    return 1
  fi

  # Validate environments
  if ! env_validate_environment "$source_env" || ! env_validate_environment "$target_env"; then
    echo "ERROR: Invalid environment" >&2
    return 1
  fi

  # Get secret from source
  local value
  value=$(env_get_secret "$key_name" "$source_env")

  if [[ $? -ne 0 ]]; then
    echo "ERROR: Failed to get secret from $source_env" >&2
    return 1
  fi

  # Set in target
  env_set_secret "$key_name" "$value" "$target_env" "Synced from $source_env"

  return $?
}

# Bulk sync all secrets from one environment to another
# Usage: env_sync_all <source_env> <target_env> [overwrite]
env_sync_all() {
  local source_env="$1"
  local target_env="$2"
  local overwrite="${3:-false}"

  if [[ -z "$source_env" ]] || [[ -z "$target_env" ]]; then
    echo "ERROR: Source and target environments required" >&2
    return 1
  fi

  echo "Syncing secrets from $source_env to $target_env..." >&2

  # Get source secrets
  local secrets
  secrets=$(vault_list "$source_env")

  if [[ "$secrets" == "[]" ]]; then
    echo "No secrets to sync" >&2
    return 0
  fi

  # Extract key names
  local count
  count=$(echo "$secrets" | jq 'length')

  local synced=0
  local skipped=0
  local failed=0

  for ((i = 0; i < count; i++)); do
    local key_name
    key_name=$(echo "$secrets" | jq -r ".[$i].key_name")

    # Check if exists in target
    if [[ "$overwrite" == "false" ]]; then
      local target_value
      target_value=$(vault_get "$key_name" "$target_env" 2>/dev/null)

      if [[ $? -eq 0 ]]; then
        echo "Skipping $key_name (already exists in $target_env)" >&2
        skipped=$((skipped + 1))
        continue
      fi
    fi

    # Sync secret
    if env_sync_secret "$key_name" "$source_env" "$target_env" 2>/dev/null; then
      synced=$((synced + 1))
    else
      failed=$((failed + 1))
      echo "WARNING: Failed to sync: $key_name" >&2
    fi
  done

  echo "✓ Synced $synced secrets ($skipped skipped, $failed failed)" >&2
  return 0
}

# ============================================================================
# Environment Promotion
# ============================================================================

# Promote secrets from dev → staging → prod
# Usage: env_promote <key_name> <from_env> <to_env>
env_promote() {
  local key_name="$1"
  local from_env="$2"
  local to_env="$3"

  if [[ -z "$key_name" ]] || [[ -z "$from_env" ]] || [[ -z "$to_env" ]]; then
    echo "ERROR: Key name, source, and target environment required" >&2
    return 1
  fi

  # Validate promotion path
  local valid_promotion=false

  if [[ "$from_env" == "$ENV_DEV" ]] && [[ "$to_env" == "$ENV_STAGING" ]]; then
    valid_promotion=true
  elif [[ "$from_env" == "$ENV_STAGING" ]] && [[ "$to_env" == "$ENV_PROD" ]]; then
    valid_promotion=true
  elif [[ "$from_env" == "$ENV_DEV" ]] && [[ "$to_env" == "$ENV_PROD" ]]; then
    echo "WARNING: Direct dev → prod promotion (bypassing staging)" >&2
    valid_promotion=true
  fi

  if [[ "$valid_promotion" == "false" ]]; then
    echo "ERROR: Invalid promotion path: $from_env → $to_env" >&2
    echo "Valid paths: dev → staging, staging → prod" >&2
    return 1
  fi

  # Sync secret
  env_sync_secret "$key_name" "$from_env" "$to_env"

  return $?
}

# ============================================================================
# Environment Validation
# ============================================================================

# Validate environment name
# Usage: env_validate_environment <environment>
env_validate_environment() {
  local environment="$1"

  case "$environment" in
    "$ENV_DEFAULT" | "$ENV_DEV" | "$ENV_STAGING" | "$ENV_PROD")
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# List all environments
# Usage: env_list_environments
env_list_environments() {
  echo "[$ENV_DEFAULT, $ENV_DEV, $ENV_STAGING, $ENV_PROD]" | jq '.'
  return 0
}

# ============================================================================
# Environment Status
# ============================================================================

# Get environment status (secret count, last update)
# Usage: env_get_status <environment>
env_get_status() {
  local environment="$1"

  if [[ -z "$environment" ]]; then
    echo "ERROR: Environment required" >&2
    return 1
  fi

  # Get PostgreSQL container
  local container
  container=$(docker ps --filter 'name=postgres' --format '{{.Names}}' | head -1)

  if [[ -z "$container" ]]; then
    echo "ERROR: PostgreSQL container not found" >&2
    return 1
  fi

  # Get status
  local status_json
  status_json=$(docker exec -i "$container" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself_db}" -t -c \
    "SELECT row_to_json(s) FROM (
       SELECT
         '$environment' AS environment,
         COUNT(*) AS total_secrets,
         COUNT(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at < NOW()) AS expired_secrets,
         MAX(updated_at) AS last_update,
         array_agg(DISTINCT key_name) AS secret_keys
       FROM secrets.vault
       WHERE environment = '$environment' AND is_active = TRUE
     ) s;" \
    2>/dev/null | xargs)

  if [[ -z "$status_json" ]] || [[ "$status_json" == "null" ]]; then
    echo "{\"environment\": \"$environment\", \"total_secrets\": 0}"
    return 0
  fi

  echo "$status_json"
  return 0
}

# ============================================================================
# Export functions
# ============================================================================

export -f env_set_secret
export -f env_get_secret
export -f env_delete_secret
export -f env_list_secrets
export -f env_compare
export -f env_sync_secret
export -f env_sync_all
export -f env_promote
export -f env_validate_environment
export -f env_list_environments
export -f env_get_status
