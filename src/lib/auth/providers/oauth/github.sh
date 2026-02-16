#!/usr/bin/env bash
# github.sh - GitHub OAuth 2.0 provider (OAUTH-004)
# Part of nself v0.6.0 - Phase 1 Sprint 1


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# GitHub OAuth endpoints
readonly GITHUB_AUTH_ENDPOINT="https://github.com/login/oauth/authorize"
readonly GITHUB_TOKEN_ENDPOINT="https://github.com/login/oauth/access_token"
readonly GITHUB_USERINFO_ENDPOINT="https://api.github.com/user"

# Default scopes
readonly GITHUB_DEFAULT_SCOPES="read:user user:email"

# Get GitHub authorization URL
# Usage: github_get_auth_url <client_id> <redirect_uri> [scopes]
github_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$GITHUB_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"github\"}"

  oauth_build_auth_url "$GITHUB_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange GitHub authorization code for tokens
# Usage: github_exchange_code <client_id> <client_secret> <code> <redirect_uri>
github_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$GITHUB_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get GitHub user info
# Usage: github_get_user_info <access_token>
github_get_user_info() {
  local access_token="$1"

  oauth_get_user_info "$GITHUB_USERINFO_ENDPOINT" "$access_token"
}

# Refresh GitHub access token
# Usage: github_refresh_token <client_id> <client_secret> <refresh_token>
github_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  oauth_refresh_token "$GITHUB_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke GitHub token
# Usage: github_revoke_token <client_id> <client_secret> <token>
github_revoke_token() {
  local client_id="$1"
  local client_secret="$2"
  local token="$3"

  curl -s -X DELETE "https://api.github.com/applications/${client_id}/grants" \
    -u "${client_id}:${client_secret}" \
    -d "{\"access_token\": \"${token}\"}" \
    >/dev/null 2>&1
}

# Export functions
export -f github_get_auth_url
export -f github_exchange_code
export -f github_get_user_info
export -f github_refresh_token
export -f github_revoke_token
