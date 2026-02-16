#!/usr/bin/env bash
# slack.sh - Slack OAuth 2.0 provider (OAUTH-011)
# Part of nself v0.6.0 - Phase 1 Sprint 2


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Slack OAuth endpoints
readonly SLACK_AUTH_ENDPOINT="https://slack.com/oauth/v2/authorize"
readonly SLACK_TOKEN_ENDPOINT="https://slack.com/api/oauth.v2.access"
readonly SLACK_USERINFO_ENDPOINT="https://slack.com/api/users.identity"
readonly SLACK_REVOKE_ENDPOINT="https://slack.com/api/auth.revoke"

# Default scopes (for user identity)
readonly SLACK_DEFAULT_SCOPES="users:read users:read.email"

# Get Slack authorization URL
# Usage: slack_get_auth_url <client_id> <redirect_uri> [scopes] [team_id]
slack_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$SLACK_DEFAULT_SCOPES}"
  local team_id="${4:-}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"slack\"}"

  # Build base URL
  local auth_url
  auth_url=$(oauth_build_auth_url "$SLACK_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state")

  # Add team_id if provided
  if [[ -n "$team_id" ]]; then
    auth_url="${auth_url}&team=${team_id}"
  fi

  echo "$auth_url"
}

# Exchange Slack authorization code for tokens
# Usage: slack_exchange_code <client_id> <client_secret> <code> <redirect_uri>
slack_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$SLACK_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Slack user info
# Usage: slack_get_user_info <access_token>
slack_get_user_info() {
  local access_token="$1"

  oauth_get_user_info "$SLACK_USERINFO_ENDPOINT" "$access_token"
}

# Refresh Slack access token
# Usage: slack_refresh_token <client_id> <client_secret> <refresh_token>
# Note: Slack supports refresh tokens when requested during authorization
slack_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$SLACK_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke Slack token
# Usage: slack_revoke_token <token>
slack_revoke_token() {
  local token="$1"

  curl -s -X POST "$SLACK_REVOKE_ENDPOINT" \
    -H "Authorization: Bearer ${token}" \
    >/dev/null 2>&1
}

# Export functions
export -f slack_get_auth_url
export -f slack_exchange_code
export -f slack_get_user_info
export -f slack_refresh_token
export -f slack_revoke_token
