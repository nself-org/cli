#!/usr/bin/env bash
# oauth-base.sh - Base OAuth 2.0 provider implementation
# Part of nself v0.6.0 - Phase 1 Sprint 1 (OAUTH-001, OAUTH-002)
#
# Provides common OAuth 2.0 / OIDC functionality that all providers inherit


# ============================================================================
# OAuth Constants
# ============================================================================

# OAuth grant types
readonly OAUTH_GRANT_AUTHORIZATION_CODE="authorization_code"

set -euo pipefail

readonly OAUTH_GRANT_REFRESH_TOKEN="refresh_token"

# OAuth response types
readonly OAUTH_RESPONSE_TYPE_CODE="code"

# Default OAuth scopes
readonly OAUTH_DEFAULT_SCOPES="openid profile email"

# ============================================================================
# OAuth Base Provider Interface
# ============================================================================
# All OAuth providers must implement these functions:
#   - provider_get_auth_url()      - Generate authorization URL
#   - provider_get_token_url()     - Get token endpoint URL
#   - provider_get_userinfo_url()  - Get userinfo endpoint URL
#   - provider_exchange_code()     - Exchange code for token
#   - provider_get_user_info()     - Get user profile
#   - provider_refresh_token()     - Refresh access token
#   - provider_revoke_token()      - Revoke access token

# ============================================================================
# OAuth URL Construction
# ============================================================================

# Build OAuth authorization URL
# Usage: oauth_build_auth_url <auth_endpoint> <client_id> <redirect_uri> <scopes> [state]
oauth_build_auth_url() {
  local auth_endpoint="$1"
  local client_id="$2"
  local redirect_uri="$3"
  local scopes="${4:-$OAUTH_DEFAULT_SCOPES}"
  local state="${5:-$(openssl rand -hex 16)}"

  # URL encode parameters
  local encoded_redirect_uri
  encoded_redirect_uri=$(printf "%s" "$redirect_uri" | jq -sRr @uri)

  local encoded_scopes
  encoded_scopes=$(printf "%s" "$scopes" | jq -sRr @uri)

  # Build URL
  echo "${auth_endpoint}?response_type=${OAUTH_RESPONSE_TYPE_CODE}&client_id=${client_id}&redirect_uri=${encoded_redirect_uri}&scope=${encoded_scopes}&state=${state}"
}

# Exchange authorization code for access token
# Usage: oauth_exchange_code <token_endpoint> <client_id> <client_secret> <code> <redirect_uri>
oauth_exchange_code() {
  local token_endpoint="$1"
  local client_id="$2"
  local client_secret="$3"
  local code="$4"
  local redirect_uri="$5"

  # Make POST request to token endpoint
  local response
  response=$(curl -s -X POST "$token_endpoint" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=${OAUTH_GRANT_AUTHORIZATION_CODE}" \
    -d "client_id=${client_id}" \
    -d "client_secret=${client_secret}" \
    -d "code=${code}" \
    -d "redirect_uri=${redirect_uri}")

  echo "$response"
}

# Refresh access token
# Usage: oauth_refresh_token <token_endpoint> <client_id> <client_secret> <refresh_token>
oauth_refresh_token() {
  local token_endpoint="$1"
  local client_id="$2"
  local client_secret="$3"
  local refresh_token="$4"

  # Make POST request to token endpoint
  local response
  response=$(curl -s -X POST "$token_endpoint" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=${OAUTH_GRANT_REFRESH_TOKEN}" \
    -d "client_id=${client_id}" \
    -d "client_secret=${client_secret}" \
    -d "refresh_token=${refresh_token}")

  echo "$response"
}

# Get user info from OAuth provider
# Usage: oauth_get_user_info <userinfo_endpoint> <access_token>
oauth_get_user_info() {
  local userinfo_endpoint="$1"
  local access_token="$2"

  # Make GET request to userinfo endpoint
  local response
  response=$(curl -s -X GET "$userinfo_endpoint" \
    -H "Authorization: Bearer ${access_token}")

  echo "$response"
}

# Revoke OAuth token
# Usage: oauth_revoke_token <revoke_endpoint> <client_id> <client_secret> <token>
oauth_revoke_token() {
  local revoke_endpoint="$1"
  local client_id="$2"
  local client_secret="$3"
  local token="$4"

  # Make POST request to revoke endpoint
  curl -s -X POST "$revoke_endpoint" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "client_id=${client_id}" \
    -d "client_secret=${client_secret}" \
    -d "token=${token}" \
    >/dev/null 2>&1
}

# ============================================================================
# OAuth State Management
# ============================================================================

# Generate OAuth state parameter (CSRF protection)
# Usage: oauth_generate_state
oauth_generate_state() {
  openssl rand -hex 16
}

# Store OAuth state
# Usage: oauth_store_state <state> <user_data>
oauth_store_state() {
  local state="$1"
  local user_data="${2:-{}}"

  # Store in temporary file (in production, use database)
  local state_file="/tmp/nself_oauth_state_${state}"
  echo "$user_data" >"$state_file"
}

# Verify OAuth state
# Usage: oauth_verify_state <state>
# Returns: 0 if valid, 1 if invalid
oauth_verify_state() {
  local state="$1"

  local state_file="/tmp/nself_oauth_state_${state}"

  if [[ -f "$state_file" ]]; then
    # Check if file is recent (less than 10 minutes old)
    local file_age
    file_age=$(($(date +%s) - $(stat -c %Y "$state_file" 2>/dev/null || stat -f %m "$state_file" 2>/dev/null)))

    if [[ $file_age -lt 600 ]]; then
      rm -f "$state_file"
      return 0
    else
      rm -f "$state_file"
      return 1
    fi
  else
    return 1
  fi
}

# ============================================================================
# OAuth Provider Discovery (OIDC)
# ============================================================================

# Discover OAuth endpoints via OIDC well-known configuration
# Usage: oauth_discover_endpoints <issuer_url>
# Returns: JSON with endpoints
oauth_discover_endpoints() {
  local issuer_url="$1"

  # Remove trailing slash
  issuer_url="${issuer_url%/}"

  # Fetch well-known configuration
  local config_url="${issuer_url}/.well-known/openid-configuration"

  local response
  response=$(curl -s "$config_url" 2>/dev/null || echo "{}")

  echo "$response"
}

# ============================================================================
# Export functions
# ============================================================================

export -f oauth_build_auth_url
export -f oauth_exchange_code
export -f oauth_refresh_token
export -f oauth_get_user_info
export -f oauth_revoke_token
export -f oauth_generate_state
export -f oauth_store_state
export -f oauth_verify_state
export -f oauth_discover_endpoints
