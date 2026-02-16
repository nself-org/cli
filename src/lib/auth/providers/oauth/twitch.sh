#!/usr/bin/env bash
# twitch.sh - Twitch OAuth 2.0 provider (OAUTH-012)
# Part of nself v0.6.0 - Phase 1 Sprint 2


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Twitch OAuth endpoints
readonly TWITCH_AUTH_ENDPOINT="https://id.twitch.tv/oauth2/authorize"
readonly TWITCH_TOKEN_ENDPOINT="https://id.twitch.tv/oauth2/token"
readonly TWITCH_USERINFO_ENDPOINT="https://api.twitch.tv/helix/users"
readonly TWITCH_REVOKE_ENDPOINT="https://id.twitch.tv/oauth2/revoke"

# Default scopes
readonly TWITCH_DEFAULT_SCOPES="user:read:email"

# Get Twitch authorization URL
# Usage: twitch_get_auth_url <client_id> <redirect_uri> [scopes]
twitch_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$TWITCH_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"twitch\"}"

  # Twitch requires force_verify=true for re-authentication
  local auth_url
  auth_url=$(oauth_build_auth_url "$TWITCH_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state")

  echo "${auth_url}&force_verify=false"
}

# Exchange Twitch authorization code for tokens
# Usage: twitch_exchange_code <client_id> <client_secret> <code> <redirect_uri>
twitch_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$TWITCH_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Twitch user info
# Usage: twitch_get_user_info <access_token> <client_id>
# Note: Twitch requires Client-ID header in addition to bearer token
twitch_get_user_info() {
  local access_token="$1"
  local client_id="${2:-}"

  if [[ -z "$client_id" ]]; then
    echo "ERROR: Twitch requires client_id for user info requests" >&2
    return 1
  fi

  # Twitch requires both Authorization and Client-ID headers
  local response
  response=$(curl -s -X GET "$TWITCH_USERINFO_ENDPOINT" \
    -H "Authorization: Bearer ${access_token}" \
    -H "Client-Id: ${client_id}")

  echo "$response"
}

# Refresh Twitch access token
# Usage: twitch_refresh_token <client_id> <client_secret> <refresh_token>
twitch_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$TWITCH_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke Twitch token
# Usage: twitch_revoke_token <client_id> <token>
twitch_revoke_token() {
  local client_id="$1"
  local token="$2"

  curl -s -X POST "$TWITCH_REVOKE_ENDPOINT" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "client_id=${client_id}" \
    -d "token=${token}" \
    >/dev/null 2>&1
}

# Export functions
export -f twitch_get_auth_url
export -f twitch_exchange_code
export -f twitch_get_user_info
export -f twitch_refresh_token
export -f twitch_revoke_token
