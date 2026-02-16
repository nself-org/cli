#!/usr/bin/env bash
# google.sh - Google OAuth 2.0 provider (OAUTH-003)
# Part of nself v0.6.0 - Phase 1 Sprint 1


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Google OAuth endpoints
readonly GOOGLE_AUTH_ENDPOINT="https://accounts.google.com/o/oauth2/v2/auth"
readonly GOOGLE_TOKEN_ENDPOINT="https://oauth2.googleapis.com/token"
readonly GOOGLE_USERINFO_ENDPOINT="https://www.googleapis.com/oauth2/v2/userinfo"
readonly GOOGLE_REVOKE_ENDPOINT="https://oauth2.googleapis.com/revoke"

# Default scopes
readonly GOOGLE_DEFAULT_SCOPES="openid profile email"

# Get Google authorization URL
# Usage: google_get_auth_url <client_id> <redirect_uri> [scopes]
google_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$GOOGLE_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"google\"}"

  oauth_build_auth_url "$GOOGLE_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange Google authorization code for tokens
# Usage: google_exchange_code <client_id> <client_secret> <code> <redirect_uri>
google_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$GOOGLE_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Google user info
# Usage: google_get_user_info <access_token>
google_get_user_info() {
  local access_token="$1"

  oauth_get_user_info "$GOOGLE_USERINFO_ENDPOINT" "$access_token"
}

# Refresh Google access token
# Usage: google_refresh_token <client_id> <client_secret> <refresh_token>
google_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$GOOGLE_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke Google token
# Usage: google_revoke_token <token>
google_revoke_token() {
  local token="$1"

  curl -s -X POST "$GOOGLE_REVOKE_ENDPOINT" \
    -d "token=${token}" \
    >/dev/null 2>&1
}

# Export functions
export -f google_get_auth_url
export -f google_exchange_code
export -f google_get_user_info
export -f google_refresh_token
export -f google_revoke_token
