#!/usr/bin/env bash
# facebook.sh - Facebook OAuth 2.0 provider (OAUTH-006)
# Part of nself v0.6.0 - Phase 1 Sprint 1


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Facebook OAuth endpoints
readonly FACEBOOK_AUTH_ENDPOINT="https://www.facebook.com/v12.0/dialog/oauth"
readonly FACEBOOK_TOKEN_ENDPOINT="https://graph.facebook.com/v12.0/oauth/access_token"
readonly FACEBOOK_USERINFO_ENDPOINT="https://graph.facebook.com/me"

# Default scopes
readonly FACEBOOK_DEFAULT_SCOPES="public_profile email"

# Get Facebook authorization URL
# Usage: facebook_get_auth_url <client_id> <redirect_uri> [scopes]
facebook_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$FACEBOOK_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"facebook\"}"

  oauth_build_auth_url "$FACEBOOK_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange Facebook authorization code for tokens
# Usage: facebook_exchange_code <client_id> <client_secret> <code> <redirect_uri>
facebook_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$FACEBOOK_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Facebook user info
# Usage: facebook_get_user_info <access_token>
facebook_get_user_info() {
  local access_token="$1"

  # Facebook requires fields parameter
  local response
  response=$(curl -s -X GET "${FACEBOOK_USERINFO_ENDPOINT}?fields=id,name,email,picture&access_token=${access_token}")

  echo "$response"
}

# Refresh Facebook access token
# Usage: facebook_refresh_token <client_id> <client_secret> <refresh_token>
facebook_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$FACEBOOK_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke Facebook token
# Usage: facebook_revoke_token <client_id> <client_secret> <token>
facebook_revoke_token() {
  local client_id="$1"
  local client_secret="$2"
  local token="$3"

  curl -s -X DELETE "https://graph.facebook.com/v12.0/me/permissions" \
    -H "Authorization: Bearer ${token}" \
    >/dev/null 2>&1
}

# Export functions
export -f facebook_get_auth_url
export -f facebook_exchange_code
export -f facebook_get_user_info
export -f facebook_refresh_token
export -f facebook_revoke_token
