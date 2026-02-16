#!/usr/bin/env bash
# oauth-linking.sh - OAuth provider account linking
# Part of nself v0.8.0+
#
# Allows users to link/unlink multiple OAuth providers to their account
# Example: Link both Google and GitHub to the same user account


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source dependencies
if [[ -f "$NSELF_ROOT/src/lib/utils/display.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/display.sh"
fi

if [[ -f "$NSELF_ROOT/src/lib/utils/platform-compat.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/platform-compat.sh"
fi

# ============================================================================
# Database Connection
# ============================================================================

# Get database connection string
get_db_connection() {
  local db_user="${POSTGRES_USER:-postgres}"
  local db_password="${POSTGRES_PASSWORD:-postgres}"
  local db_name="${POSTGRES_DB:-nself}"
  local db_host="${POSTGRES_HOST:-postgres}"
  local db_port="${POSTGRES_PORT:-5432}"

  printf "postgresql://%s:%s@%s:%s/%s" "$db_user" "$db_password" "$db_host" "$db_port" "$db_name"
}

# Execute PostgreSQL query
psql_exec() {
  local query="$1"
  local db_conn
  db_conn=$(get_db_connection)

  psql "$db_conn" -t -A -c "$query" 2>/dev/null
}

# ============================================================================
# OAuth Provider Linking
# ============================================================================

# Link OAuth provider to existing user
# Usage: oauth_link_provider <user_id> <provider> <provider_user_id> <email> <access_token> <refresh_token> <expires_in>
oauth_link_provider() {
  local user_id="$1"
  local provider="$2"
  local provider_user_id="$3"
  local email="$4"
  local access_token="$5"
  local refresh_token="${6:-}"
  local expires_in="${7:-}"

  # Check if provider is already linked
  local existing_link
  existing_link=$(psql_exec "
    SELECT id FROM auth.oauth_provider_accounts
    WHERE user_id = '$user_id' AND provider = '$provider';
  ")

  if [[ -n "$existing_link" ]]; then
    log_error "Provider $provider is already linked to this user"
    return 1
  fi

  # Check if provider account is linked to another user
  local other_user
  other_user=$(psql_exec "
    SELECT user_id FROM auth.oauth_provider_accounts
    WHERE provider = '$provider' AND provider_user_id = '$provider_user_id';
  ")

  if [[ -n "$other_user" ]] && [[ "$other_user" != "$user_id" ]]; then
    log_error "This $provider account is already linked to another user"
    return 1
  fi

  # Calculate expiration time
  local expires_at_clause=""
  if [[ -n "$expires_in" ]]; then
    expires_at_clause=", token_expires_at = NOW() + INTERVAL '${expires_in} seconds'"
  fi

  local refresh_token_clause=""
  if [[ -n "$refresh_token" ]]; then
    refresh_token_clause=", refresh_token = '$refresh_token'"
  fi

  # Insert OAuth account
  local query="
    INSERT INTO auth.oauth_provider_accounts (
      user_id,
      provider,
      provider_user_id,
      provider_account_email,
      access_token
      ${refresh_token_clause:+$refresh_token_clause}
      ${expires_at_clause:+$expires_at_clause}
    )
    VALUES (
      '$user_id',
      '$provider',
      '$provider_user_id',
      '$email',
      '$access_token'
    )
    RETURNING id;
  "

  local account_id
  account_id=$(psql_exec "$query")

  if [[ -z "$account_id" ]]; then
    log_error "Failed to link $provider provider"
    return 1
  fi

  # Log audit event
  psql_exec "
    INSERT INTO auth.oauth_audit_log (user_id, provider, event_type)
    VALUES ('$user_id', '$provider', 'link');
  "

  log_success "Successfully linked $provider provider to user $user_id"
  return 0
}

# Unlink OAuth provider from user
# Usage: oauth_unlink_provider <user_id> <provider>
oauth_unlink_provider() {
  local user_id="$1"
  local provider="$2"

  # Check if provider is linked
  local existing_link
  existing_link=$(psql_exec "
    SELECT id FROM auth.oauth_provider_accounts
    WHERE user_id = '$user_id' AND provider = '$provider';
  ")

  if [[ -z "$existing_link" ]]; then
    log_error "Provider $provider is not linked to this user"
    return 1
  fi

  # Check if this is the only auth method
  local auth_count
  auth_count=$(psql_exec "
    SELECT COUNT(*) FROM (
      SELECT 1 FROM auth.oauth_provider_accounts WHERE user_id = '$user_id'
      UNION ALL
      SELECT 1 FROM auth.users WHERE id = '$user_id' AND password_hash IS NOT NULL
    ) AS auth_methods;
  ")

  if [[ "$auth_count" -le 1 ]]; then
    log_error "Cannot unlink $provider: This is the only authentication method for this user"
    log_warning "User must have at least one auth method (password or OAuth provider)"
    return 1
  fi

  # Delete OAuth account
  psql_exec "
    DELETE FROM auth.oauth_provider_accounts
    WHERE user_id = '$user_id' AND provider = '$provider';
  "

  # Log audit event
  psql_exec "
    INSERT INTO auth.oauth_audit_log (user_id, provider, event_type)
    VALUES ('$user_id', '$provider', 'unlink');
  "

  log_success "Successfully unlinked $provider provider from user $user_id"
  return 0
}

# List OAuth providers linked to user
# Usage: oauth_list_providers <user_id>
oauth_list_providers() {
  local user_id="$1"

  local query="
    SELECT
      provider,
      provider_account_email,
      created_at,
      token_expires_at
    FROM auth.oauth_provider_accounts
    WHERE user_id = '$user_id'
    ORDER BY created_at DESC;
  "

  local result
  result=$(psql_exec "$query")

  if [[ -z "$result" ]]; then
    log_info "No OAuth providers linked to user $user_id"
    return 0
  fi

  printf "\n%bLinked OAuth Providers:%b\n\n" "${COLOR_BOLD}" "${COLOR_RESET}"

  while IFS='|' read -r provider email linked_at expires_at; do
    if [[ -n "$provider" ]]; then
      printf "  %b%s%b\n" "${COLOR_CYAN}" "$provider" "${COLOR_RESET}"
      printf "    Email: %s\n" "$email"
      printf "    Linked: %s\n" "$linked_at"

      if [[ -n "$expires_at" ]] && [[ "$expires_at" != "null" ]]; then
        printf "    Token expires: %s\n" "$expires_at"
      fi

      printf "\n"
    fi
  done <<<"$result"
}

# Check if user can authenticate with a provider
# Usage: oauth_can_authenticate <user_id> <provider>
oauth_can_authenticate() {
  local user_id="$1"
  local provider="$2"

  local result
  result=$(psql_exec "
    SELECT 1 FROM auth.oauth_provider_accounts
    WHERE user_id = '$user_id' AND provider = '$provider'
    LIMIT 1;
  ")

  if [[ -n "$result" ]]; then
    return 0
  else
    return 1
  fi
}

# Get primary authentication method for user
# Usage: oauth_get_primary_auth <user_id>
oauth_get_primary_auth() {
  local user_id="$1"

  # Check if user has password
  local has_password
  has_password=$(psql_exec "
    SELECT 1 FROM auth.users
    WHERE id = '$user_id' AND password_hash IS NOT NULL
    LIMIT 1;
  ")

  if [[ -n "$has_password" ]]; then
    printf "password"
    return 0
  fi

  # Get first OAuth provider
  local first_provider
  first_provider=$(psql_exec "
    SELECT provider FROM auth.oauth_provider_accounts
    WHERE user_id = '$user_id'
    ORDER BY created_at ASC
    LIMIT 1;
  ")

  if [[ -n "$first_provider" ]]; then
    printf "oauth:%s" "$first_provider"
    return 0
  fi

  printf "none"
  return 1
}

# ============================================================================
# Account Merging
# ============================================================================

# Merge two user accounts (combine OAuth providers)
# Usage: oauth_merge_accounts <from_user_id> <to_user_id>
oauth_merge_accounts() {
  local from_user_id="$1"
  local to_user_id="$2"

  log_info "Merging user accounts: $from_user_id -> $to_user_id"

  # Check if accounts exist
  local from_exists
  from_exists=$(psql_exec "SELECT 1 FROM auth.users WHERE id = '$from_user_id' LIMIT 1;")

  local to_exists
  to_exists=$(psql_exec "SELECT 1 FROM auth.users WHERE id = '$to_user_id' LIMIT 1;")

  if [[ -z "$from_exists" ]] || [[ -z "$to_exists" ]]; then
    log_error "One or both user accounts do not exist"
    return 1
  fi

  if [[ "$from_user_id" == "$to_user_id" ]]; then
    log_error "Cannot merge account with itself"
    return 1
  fi

  # Get OAuth providers from source account
  local from_providers
  from_providers=$(psql_exec "
    SELECT provider FROM auth.oauth_provider_accounts
    WHERE user_id = '$from_user_id';
  ")

  if [[ -z "$from_providers" ]]; then
    log_warning "Source account has no OAuth providers to merge"
    return 0
  fi

  # Check for conflicts
  while read -r provider; do
    if [[ -z "$provider" ]]; then
      continue
    fi

    local conflict
    conflict=$(psql_exec "
      SELECT 1 FROM auth.oauth_provider_accounts
      WHERE user_id = '$to_user_id' AND provider = '$provider'
      LIMIT 1;
    ")

    if [[ -n "$conflict" ]]; then
      log_error "Cannot merge: Both accounts have $provider provider linked"
      log_warning "Please unlink $provider from one account first"
      return 1
    fi
  done <<<"$from_providers"

  # Transfer OAuth providers
  psql_exec "
    UPDATE auth.oauth_provider_accounts
    SET user_id = '$to_user_id'
    WHERE user_id = '$from_user_id';
  "

  # Transfer audit logs
  psql_exec "
    UPDATE auth.oauth_audit_log
    SET user_id = '$to_user_id'
    WHERE user_id = '$from_user_id';
  "

  # Log merge event
  psql_exec "
    INSERT INTO auth.oauth_audit_log (user_id, provider, event_type, metadata)
    VALUES (
      '$to_user_id',
      'system',
      'account_merge',
      jsonb_build_object('merged_from', '$from_user_id')
    );
  "

  log_success "Successfully merged accounts"
  log_info "OAuth providers transferred from $from_user_id to $to_user_id"

  # Note: Original account is not deleted, just OAuth providers are transferred
  log_warning "Original account $from_user_id still exists but has no OAuth providers"
  log_info "You may want to disable or delete it manually"

  return 0
}

# ============================================================================
# CLI Interface
# ============================================================================

show_usage() {
  cat <<EOF
Usage: oauth-linking.sh <command> [options]

OAuth provider account linking for nself

COMMANDS:
  link <user_id> <provider>           Link OAuth provider to user
  unlink <user_id> <provider>         Unlink OAuth provider from user
  list <user_id>                      List linked providers for user
  check <user_id> <provider>          Check if provider is linked
  primary <user_id>                   Get primary auth method
  merge <from_user_id> <to_user_id>  Merge two user accounts
  help                                Show this help message

EXAMPLES:
  # List providers for user
  oauth-linking.sh list 123e4567-e89b-12d3-a456-426614174000

  # Check if user can login with Google
  oauth-linking.sh check 123e4567-e89b-12d3-a456-426614174000 google

  # Unlink GitHub from user
  oauth-linking.sh unlink 123e4567-e89b-12d3-a456-426614174000 github

  # Merge accounts (combine OAuth providers)
  oauth-linking.sh merge old-user-id new-user-id

NOTES:
  - Users must have at least one authentication method
  - Cannot unlink the only OAuth provider if user has no password
  - Account merging transfers OAuth providers but doesn't delete source account
EOF
}

# ============================================================================
# Main Entry Point
# ============================================================================

main() {
  local command="${1:-help}"

  case "$command" in
    link)
      if [[ $# -lt 3 ]]; then
        log_error "Usage: oauth-linking.sh link <user_id> <provider>"
        exit 1
      fi
      oauth_link_provider "$2" "$3" "${4:-}" "${5:-}" "${6:-}" "${7:-}" "${8:-}"
      ;;
    unlink)
      if [[ $# -lt 3 ]]; then
        log_error "Usage: oauth-linking.sh unlink <user_id> <provider>"
        exit 1
      fi
      oauth_unlink_provider "$2" "$3"
      ;;
    list)
      if [[ $# -lt 2 ]]; then
        log_error "Usage: oauth-linking.sh list <user_id>"
        exit 1
      fi
      oauth_list_providers "$2"
      ;;
    check)
      if [[ $# -lt 3 ]]; then
        log_error "Usage: oauth-linking.sh check <user_id> <provider>"
        exit 1
      fi
      if oauth_can_authenticate "$2" "$3"; then
        log_success "User $2 can authenticate with $3"
      else
        log_error "User $2 cannot authenticate with $3"
        exit 1
      fi
      ;;
    primary)
      if [[ $# -lt 2 ]]; then
        log_error "Usage: oauth-linking.sh primary <user_id>"
        exit 1
      fi
      local primary
      primary=$(oauth_get_primary_auth "$2")
      log_info "Primary authentication method: $primary"
      ;;
    merge)
      if [[ $# -lt 3 ]]; then
        log_error "Usage: oauth-linking.sh merge <from_user_id> <to_user_id>"
        exit 1
      fi
      oauth_merge_accounts "$2" "$3"
      ;;
    help | --help | -h)
      show_usage
      ;;
    *)
      log_error "Unknown command: $command"
      printf "\n"
      show_usage
      exit 1
      ;;
  esac
}

# Export functions for library usage
export -f oauth_link_provider
export -f oauth_unlink_provider
export -f oauth_list_providers
export -f oauth_can_authenticate
export -f oauth_get_primary_auth
export -f oauth_merge_accounts

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
