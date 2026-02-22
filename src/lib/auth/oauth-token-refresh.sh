#!/usr/bin/env bash
# oauth-token-refresh.sh - OAuth token refresh background service
# Part of nself v0.8.0+
#
# Automatically refreshes OAuth access tokens before they expire
# Processes oauth_token_refresh_queue table


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source dependencies
if [[ -f "$NSELF_ROOT/src/lib/utils/display.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/display.sh"
fi

if [[ -f "$NSELF_ROOT/src/lib/utils/env-detection.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/env-detection.sh"
fi

# Source OAuth base
if [[ -f "$SCRIPT_DIR/providers/oauth/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/providers/oauth/oauth-base.sh"
fi

# Source all OAuth providers
for provider_file in "$SCRIPT_DIR/providers/oauth"/*.sh; do
  if [[ "$provider_file" != *"oauth-base.sh" ]]; then
    source "$provider_file"
  fi
done

# ============================================================================
# Configuration
# ============================================================================

# Refresh interval (seconds) - how often to check for tokens to refresh
readonly REFRESH_CHECK_INTERVAL="${OAUTH_REFRESH_CHECK_INTERVAL:-300}" # 5 minutes

# Maximum refresh attempts before giving up
readonly MAX_REFRESH_ATTEMPTS="${OAUTH_MAX_REFRESH_ATTEMPTS:-3}"

# Refresh window (minutes) - refresh tokens this many minutes before expiry
readonly REFRESH_WINDOW_MINUTES="${OAUTH_REFRESH_WINDOW_MINUTES:-5}"

# Log file
readonly LOG_FILE="${OAUTH_REFRESH_LOG_FILE:-/var/log/nself/oauth-refresh.log}"

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
# Logging
# ============================================================================

log_refresh() {
  local level="$1"
  shift
  local message="$*"
  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  # Console output
  case "$level" in
    INFO)
      log_info "$message"
      ;;
    WARN)
      log_warning "$message"
      ;;
    ERROR)
      log_error "$message"
      ;;
    SUCCESS)
      log_success "$message"
      ;;
  esac

  # File output
  if [[ -n "$LOG_FILE" ]]; then
    mkdir -p "$(dirname "$LOG_FILE")"
    printf "[%s] [%s] %s\n" "$timestamp" "$level" "$message" >>"$LOG_FILE"
  fi
}

# ============================================================================
# Token Refresh Logic
# ============================================================================

# Get tokens pending refresh
get_pending_refresh_tokens() {
  local query="
    SELECT
      trq.id,
      trq.oauth_account_id,
      opa.provider,
      opa.refresh_token,
      trq.attempts
    FROM auth.oauth_token_refresh_queue trq
    JOIN auth.oauth_provider_accounts opa ON opa.id = trq.oauth_account_id
    WHERE trq.scheduled_at <= NOW()
      AND trq.attempts < trq.max_attempts
      AND opa.refresh_token IS NOT NULL
    ORDER BY trq.scheduled_at ASC
    LIMIT 50;
  "

  psql_exec "$query"
}

# Refresh token for a specific provider
refresh_provider_token() {
  local provider="$1"
  local refresh_token="$2"

  # Get provider credentials from environment
  local provider_upper
  provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

  local client_id_var="OAUTH_${provider_upper}_CLIENT_ID"
  local client_secret_var="OAUTH_${provider_upper}_CLIENT_SECRET"

  local client_id="${!client_id_var:-}"
  local client_secret="${!client_secret_var:-}"

  if [[ -z "$client_id" ]] || [[ -z "$client_secret" ]]; then
    log_refresh ERROR "Missing credentials for provider: $provider"
    return 1
  fi

  # Call provider-specific refresh function
  case "$provider" in
    google)
      google_refresh_token "$client_id" "$client_secret" "$refresh_token"
      ;;
    github)
      # GitHub doesn't support token refresh
      printf '{"error": "not_supported"}'
      return 1
      ;;
    microsoft)
      oauth_refresh_token "https://login.microsoftonline.com/common/oauth2/v2.0/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    slack)
      oauth_refresh_token "https://slack.com/api/oauth.v2.access" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    discord)
      oauth_refresh_token "https://discord.com/api/oauth2/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    gitlab)
      oauth_refresh_token "https://gitlab.com/oauth/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    bitbucket)
      oauth_refresh_token "https://bitbucket.org/site/oauth2/access_token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    twitter)
      oauth_refresh_token "https://api.twitter.com/2/oauth2/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    facebook)
      oauth_refresh_token "https://graph.facebook.com/v18.0/oauth/access_token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    apple)
      oauth_refresh_token "https://appleid.apple.com/auth/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    linkedin)
      oauth_refresh_token "https://www.linkedin.com/oauth/v2/accessToken" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    twitch)
      oauth_refresh_token "https://id.twitch.tv/oauth2/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    spotify)
      oauth_refresh_token "https://accounts.spotify.com/api/token" \
        "$client_id" "$client_secret" "$refresh_token"
      ;;
    *)
      log_refresh ERROR "Unknown provider: $provider"
      return 1
      ;;
  esac
}

# Update OAuth account with new tokens
update_oauth_account() {
  local account_id="$1"
  local access_token="$2"
  local refresh_token="$3"
  local expires_in="$4"

  local expires_at_clause=""
  if [[ -n "$expires_in" ]] && [[ "$expires_in" != "null" ]]; then
    expires_at_clause=", token_expires_at = NOW() + INTERVAL '${expires_in} seconds'"
  fi

  local refresh_token_clause=""
  if [[ -n "$refresh_token" ]] && [[ "$refresh_token" != "null" ]]; then
    refresh_token_clause=", refresh_token = '$refresh_token'"
  fi

  local query="
    UPDATE auth.oauth_provider_accounts
    SET access_token = '$access_token'
        $refresh_token_clause
        $expires_at_clause,
        updated_at = NOW()
    WHERE id = '$account_id';
  "

  psql_exec "$query"
}

# Mark refresh as complete
mark_refresh_complete() {
  local queue_id="$1"

  local query="
    DELETE FROM auth.oauth_token_refresh_queue
    WHERE id = '$queue_id';
  "

  psql_exec "$query"
}

# Mark refresh as failed
mark_refresh_failed() {
  local queue_id="$1"
  local error_message="$2"

  # Escape single quotes
  error_message="${error_message//\'/\'\'}"

  local query="
    UPDATE auth.oauth_token_refresh_queue
    SET attempts = attempts + 1,
        last_attempt_at = NOW(),
        error_message = '$error_message'
    WHERE id = '$queue_id';
  "

  psql_exec "$query"
}

# Process single token refresh
process_token_refresh() {
  local queue_id="$1"
  local account_id="$2"
  local provider="$3"
  local refresh_token="$4"
  local attempts="$5"

  log_refresh INFO "Refreshing token for provider: $provider (attempt $((attempts + 1)))"

  # Call refresh function
  local response
  if ! response=$(refresh_provider_token "$provider" "$refresh_token" 2>&1); then
    log_refresh ERROR "Token refresh failed for $provider: $response"
    mark_refresh_failed "$queue_id" "$response"
    return 1
  fi

  # Parse response (JSON)
  local access_token
  local new_refresh_token
  local expires_in

  access_token=$(echo "$response" | jq -r '.access_token // empty' 2>/dev/null)
  new_refresh_token=$(echo "$response" | jq -r '.refresh_token // empty' 2>/dev/null)
  expires_in=$(echo "$response" | jq -r '.expires_in // empty' 2>/dev/null)

  if [[ -z "$access_token" ]]; then
    log_refresh ERROR "Invalid token response for $provider: missing access_token"
    mark_refresh_failed "$queue_id" "Invalid response: missing access_token"
    return 1
  fi

  # Update OAuth account
  if ! update_oauth_account "$account_id" "$access_token" "$new_refresh_token" "$expires_in"; then
    log_refresh ERROR "Failed to update OAuth account: $account_id"
    mark_refresh_failed "$queue_id" "Database update failed"
    return 1
  fi

  # Mark as complete
  mark_refresh_complete "$queue_id"

  log_refresh SUCCESS "Token refreshed successfully for $provider"
  return 0
}

# ============================================================================
# Main Refresh Loop
# ============================================================================

# Process all pending refreshes
process_pending_refreshes() {
  local pending_tokens
  pending_tokens=$(get_pending_refresh_tokens)

  if [[ -z "$pending_tokens" ]]; then
    return 0
  fi

  local processed=0
  local failed=0

  while IFS='|' read -r queue_id account_id provider refresh_token attempts; do
    if [[ -z "$queue_id" ]]; then
      continue
    fi

    if process_token_refresh "$queue_id" "$account_id" "$provider" "$refresh_token" "$attempts"; then
      processed=$((processed + 1))
    else
      failed=$((failed + 1))
    fi
  done <<<"$pending_tokens"

  if [[ $processed -gt 0 ]]; then
    log_refresh INFO "Processed $processed token refreshes ($failed failed)"
  fi
}

# Run refresh service in daemon mode
run_refresh_daemon() {
  log_refresh INFO "Starting OAuth token refresh service..."
  log_refresh INFO "Check interval: ${REFRESH_CHECK_INTERVAL}s"
  log_refresh INFO "Refresh window: ${REFRESH_WINDOW_MINUTES} minutes before expiry"

  while true; do
    process_pending_refreshes || true
    sleep "$REFRESH_CHECK_INTERVAL"
  done
}

# Run once and exit (for cron jobs)
run_once() {
  log_refresh INFO "Running OAuth token refresh (one-time)"
  process_pending_refreshes
}

# ============================================================================
# CLI Interface
# ============================================================================

show_usage() {
  cat <<EOF
Usage: oauth-token-refresh.sh [OPTIONS]

OAuth token refresh background service for nself

OPTIONS:
  daemon        Run as daemon (continuous background process)
  once          Run once and exit (for cron jobs)
  status        Show refresh queue status
  test          Test token refresh for a specific provider
  help          Show this help message

EXAMPLES:
  # Run as daemon
  oauth-token-refresh.sh daemon

  # Run once (for cron)
  */5 * * * * /path/to/oauth-token-refresh.sh once

  # Check status
  oauth-token-refresh.sh status

ENVIRONMENT VARIABLES:
  OAUTH_REFRESH_CHECK_INTERVAL    Check interval in seconds (default: 300)
  OAUTH_MAX_REFRESH_ATTEMPTS      Max retry attempts (default: 3)
  OAUTH_REFRESH_WINDOW_MINUTES    Refresh window in minutes (default: 5)
  OAUTH_REFRESH_LOG_FILE          Log file path (default: /var/log/nself/oauth-refresh.log)
EOF
}

# Show refresh queue status
show_status() {
  local query="
    SELECT
      COUNT(*) as total,
      COUNT(*) FILTER (WHERE scheduled_at <= NOW()) as pending,
      COUNT(*) FILTER (WHERE attempts >= max_attempts) as failed
    FROM auth.oauth_token_refresh_queue;
  "

  log_info "OAuth Token Refresh Queue Status"
  printf "\n"

  local result
  result=$(psql_exec "$query")

  IFS='|' read -r total pending failed <<<"$result"

  printf "  Total queued: %s\n" "$total"
  printf "  Pending refresh: %s\n" "$pending"
  printf "  Failed (max attempts): %s\n" "$failed"
  printf "\n"

  # Show next 5 pending
  local next_query="
    SELECT
      opa.provider,
      trq.scheduled_at,
      trq.attempts
    FROM auth.oauth_token_refresh_queue trq
    JOIN auth.oauth_provider_accounts opa ON opa.id = trq.oauth_account_id
    ORDER BY trq.scheduled_at ASC
    LIMIT 5;
  "

  log_info "Next 5 scheduled refreshes:"
  printf "\n"

  psql_exec "$next_query" | while IFS='|' read -r provider scheduled_at attempts; do
    if [[ -n "$provider" ]]; then
      printf "  %s - %s (attempt %s)\n" "$provider" "$scheduled_at" "$attempts"
    fi
  done

  printf "\n"
}

# ============================================================================
# Main Entry Point
# ============================================================================

main() {
  local command="${1:-help}"

  case "$command" in
    daemon)
      run_refresh_daemon
      ;;
    once)
      run_once
      ;;
    status)
      show_status
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

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
