#!/usr/bin/env bash
# spotify.sh - Spotify OAuth 2.0 provider
# Part of nself v0.8.0+


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Spotify OAuth endpoints
readonly SPOTIFY_AUTH_ENDPOINT="https://accounts.spotify.com/authorize"
readonly SPOTIFY_TOKEN_ENDPOINT="https://accounts.spotify.com/api/token"
readonly SPOTIFY_USERINFO_ENDPOINT="https://api.spotify.com/v1/me"

# Default scopes
readonly SPOTIFY_DEFAULT_SCOPES="user-read-email user-read-private"

# Get Spotify authorization URL
# Usage: spotify_get_auth_url <client_id> <redirect_uri> [scopes]
spotify_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$SPOTIFY_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"spotify\"}"

  oauth_build_auth_url "$SPOTIFY_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange Spotify authorization code for tokens
# Usage: spotify_exchange_code <client_id> <client_secret> <code> <redirect_uri>
spotify_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$SPOTIFY_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Spotify user info
# Usage: spotify_get_user_info <access_token>
spotify_get_user_info() {
  local access_token="$1"

  oauth_get_user_info "$SPOTIFY_USERINFO_ENDPOINT" "$access_token"
}

# Refresh Spotify access token
# Usage: spotify_refresh_token <client_id> <client_secret> <refresh_token>
spotify_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$SPOTIFY_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Export functions
export -f spotify_get_auth_url
export -f spotify_exchange_code
export -f spotify_get_user_info
export -f spotify_refresh_token
