#!/usr/bin/env bash
# discord.sh - Discord OAuth 2.0 provider (OAUTH-008)
# Part of nself v0.6.0 - Phase 1 Sprint 2


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Discord OAuth endpoints
readonly DISCORD_AUTH_ENDPOINT="https://discord.com/api/oauth2/authorize"
readonly DISCORD_TOKEN_ENDPOINT="https://discord.com/api/oauth2/token"
readonly DISCORD_USERINFO_ENDPOINT="https://discord.com/api/users/@me"
readonly DISCORD_REVOKE_ENDPOINT="https://discord.com/api/oauth2/token/revoke"

# Default scopes
readonly DISCORD_DEFAULT_SCOPES="identify email"

# Get Discord authorization URL
# Usage: discord_get_auth_url <client_id> <redirect_uri> [scopes]
discord_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$DISCORD_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"discord\"}"

  oauth_build_auth_url "$DISCORD_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange Discord authorization code for tokens
# Usage: discord_exchange_code <client_id> <client_secret> <code> <redirect_uri>
discord_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$DISCORD_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Discord user info
# Usage: discord_get_user_info <access_token>
discord_get_user_info() {
  local access_token="$1"

  oauth_get_user_info "$DISCORD_USERINFO_ENDPOINT" "$access_token"
}

# Refresh Discord access token
# Usage: discord_refresh_token <client_id> <client_secret> <refresh_token>
discord_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$DISCORD_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke Discord token
# Usage: discord_revoke_token <client_id> <client_secret> <token>
discord_revoke_token() {
  local client_id="$1"
  local client_secret="$2"
  local token="$3"

  # Discord requires Basic auth for revocation
  local auth_header
  auth_header=$(printf "%s:%s" "$client_id" "$client_secret" | base64)

  curl -s -X POST "$DISCORD_REVOKE_ENDPOINT" \
    -H "Authorization: Basic ${auth_header}" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "token=${token}" \
    >/dev/null 2>&1
}

# Export functions
export -f discord_get_auth_url
export -f discord_exchange_code
export -f discord_get_user_info
export -f discord_refresh_token
export -f discord_revoke_token
