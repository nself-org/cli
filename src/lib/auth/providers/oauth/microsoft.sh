#!/usr/bin/env bash
# microsoft.sh - Microsoft OAuth 2.0 provider (OAUTH-009)
# Part of nself v0.6.0 - Phase 1 Sprint 2


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Microsoft OAuth endpoints (common tenant for multi-tenant apps)
readonly MICROSOFT_AUTH_ENDPOINT="https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
readonly MICROSOFT_TOKEN_ENDPOINT="https://login.microsoftonline.com/common/oauth2/v2.0/token"
readonly MICROSOFT_USERINFO_ENDPOINT="https://graph.microsoft.com/v1.0/me"
readonly MICROSOFT_REVOKE_ENDPOINT="https://login.microsoftonline.com/common/oauth2/v2.0/logout"

# Default scopes
readonly MICROSOFT_DEFAULT_SCOPES="openid profile email User.Read"

# Get Microsoft authorization URL
# Usage: microsoft_get_auth_url <client_id> <redirect_uri> [scopes] [tenant_id]
microsoft_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$MICROSOFT_DEFAULT_SCOPES}"
  local tenant_id="${4:-common}"

  # Use tenant-specific endpoint if provided
  local auth_endpoint="https://login.microsoftonline.com/${tenant_id}/oauth2/v2.0/authorize"

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"microsoft\", \"tenant\": \"${tenant_id}\"}"

  oauth_build_auth_url "$auth_endpoint" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange Microsoft authorization code for tokens
# Usage: microsoft_exchange_code <client_id> <client_secret> <code> <redirect_uri> [tenant_id]
microsoft_exchange_code() {
  local client_id="$1"
  local client_secret="$2"
  local code="$3"
  local redirect_uri="$4"
  local tenant_id="${5:-common}"

  # Use tenant-specific endpoint
  local token_endpoint="https://login.microsoftonline.com/${tenant_id}/oauth2/v2.0/token"

  oauth_exchange_code "$token_endpoint" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get Microsoft user info
# Usage: microsoft_get_user_info <access_token>
microsoft_get_user_info() {
  local access_token="$1"

  oauth_get_user_info "$MICROSOFT_USERINFO_ENDPOINT" "$access_token"
}

# Refresh Microsoft access token
# Usage: microsoft_refresh_token <client_id> <client_secret> <refresh_token> [tenant_id]
microsoft_refresh_token() {
  local client_id="$1"
  local client_secret="$2"
  local refresh_token="$3"
  local tenant_id="${4:-common}"

  # Use tenant-specific endpoint
  local token_endpoint="https://login.microsoftonline.com/${tenant_id}/oauth2/v2.0/token"

  oauth_refresh_token "$token_endpoint" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke Microsoft token (logout)
# Usage: microsoft_revoke_token <token>
# Note: Microsoft uses logout endpoint rather than token revocation
microsoft_revoke_token() {
  local token="$1"

  # Microsoft doesn't have a standard revocation endpoint
  # Tokens are revoked via logout or naturally expire
  # This is a no-op for compatibility
  return 0
}

# Export functions
export -f microsoft_get_auth_url
export -f microsoft_exchange_code
export -f microsoft_get_user_info
export -f microsoft_refresh_token
export -f microsoft_revoke_token
