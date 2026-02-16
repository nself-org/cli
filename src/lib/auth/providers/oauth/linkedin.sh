#!/usr/bin/env bash
# linkedin.sh - LinkedIn OAuth 2.0 provider (OAUTH-010)
# Part of nself v0.6.0 - Phase 1 Sprint 2


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# LinkedIn OAuth endpoints
readonly LINKEDIN_AUTH_ENDPOINT="https://www.linkedin.com/oauth/v2/authorization"
readonly LINKEDIN_TOKEN_ENDPOINT="https://www.linkedin.com/oauth/v2/accessToken"
readonly LINKEDIN_USERINFO_ENDPOINT="https://api.linkedin.com/v2/me"
readonly LINKEDIN_EMAIL_ENDPOINT="https://api.linkedin.com/v2/emailAddress?q=members&projection=(elements*(handle~))"

# Default scopes
readonly LINKEDIN_DEFAULT_SCOPES="r_liteprofile r_emailaddress"

# Get LinkedIn authorization URL
# Usage: linkedin_get_auth_url <client_id> <redirect_uri> [scopes]
linkedin_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$LINKEDIN_DEFAULT_SCOPES}"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"linkedin\"}"

  oauth_build_auth_url "$LINKEDIN_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange LinkedIn authorization code for tokens
# Usage: linkedin_exchange_code <client_id> <client_secret> <code> <redirect_uri>
linkedin_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"

  oauth_exchange_code "$LINKEDIN_TOKEN_ENDPOINT" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get LinkedIn user info
# Usage: linkedin_get_user_info <access_token>
# Note: LinkedIn requires separate API calls for profile and email
linkedin_get_user_info() {
  local access_token="$1"

  # Get basic profile
  local profile
  profile=$(oauth_get_user_info "$LINKEDIN_USERINFO_ENDPOINT" "$access_token")

  # Get email address (requires r_emailaddress scope)
  local email
  email=$(curl -s -X GET "$LINKEDIN_EMAIL_ENDPOINT" \
    -H "Authorization: Bearer ${access_token}" 2>/dev/null || echo "{}")

  # Combine profile and email
  echo "$profile" | jq --argjson email "$email" '. + {email: $email}'
}

# Refresh LinkedIn access token
# Usage: linkedin_refresh_token <client_id> <client_secret> <refresh_token>
# Note: LinkedIn doesn't support refresh tokens by default
linkedin_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"

  # LinkedIn OAuth 2.0 doesn't support refresh tokens by default
  # Users must re-authenticate when tokens expire
  echo "ERROR: LinkedIn does not support refresh tokens" >&2
  return 1
}

# Revoke LinkedIn token
# Usage: linkedin_revoke_token <token>
# Note: LinkedIn doesn't have a public revocation endpoint
linkedin_revoke_token() {
  local token="$1"

  # LinkedIn doesn't provide a token revocation endpoint
  # Tokens expire naturally after 60 days
  # This is a no-op for compatibility
  return 0
}

# Export functions
export -f linkedin_get_auth_url
export -f linkedin_exchange_code
export -f linkedin_get_user_info
export -f linkedin_refresh_token
export -f linkedin_revoke_token
