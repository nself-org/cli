#!/usr/bin/env bash
# custom-oidc.sh - Custom OIDC provider with auto-discovery (OAUTH-013)
# Part of nself v0.6.0 - Phase 1 Sprint 2
#
# Supports any OIDC-compliant identity provider via:
# - Auto-discovery from .well-known/openid-configuration
# - Manual endpoint configuration
# - Flexible scope configuration


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source OAuth base
if [[ -f "$SCRIPT_DIR/oauth-base.sh" ]]; then
  source "$SCRIPT_DIR/oauth-base.sh"
fi

# Default scopes for OIDC
readonly OIDC_DEFAULT_SCOPES="openid profile email"

# ============================================================================
# OIDC Auto-Discovery
# ============================================================================

# Discover OIDC endpoints from issuer
# Usage: oidc_discover_endpoints <issuer_url>
# Returns: JSON with discovered endpoints
oidc_discover_endpoints() {
  local issuer_url="$1"

  oauth_discover_endpoints "$issuer_url"
}

# Extract endpoint from discovery JSON
# Usage: oidc_extract_endpoint <discovery_json> <endpoint_name>
oidc_extract_endpoint() {
  local discovery_json="$1"
  local endpoint_name="$2"

  echo "$discovery_json" | jq -r ".${endpoint_name} // empty" 2>/dev/null || echo ""
}

# ============================================================================
# OIDC Provider Configuration
# ============================================================================

# Configure custom OIDC provider (auto-discovery)
# Usage: oidc_configure_auto <provider_name> <issuer_url>
# Stores configuration in PostgreSQL or file
oidc_configure_auto() {
  local provider_name="$1"
  local issuer_url="$2"

  echo "Discovering OIDC endpoints for ${provider_name}..." >&2

  local discovery
  discovery=$(oidc_discover_endpoints "$issuer_url")

  if [[ -z "$discovery" ]] || [[ "$discovery" == "{}" ]]; then
    echo "ERROR: Failed to discover OIDC endpoints from ${issuer_url}" >&2
    return 1
  fi

  # Extract endpoints
  local auth_endpoint
  auth_endpoint=$(oidc_extract_endpoint "$discovery" "authorization_endpoint")

  local token_endpoint
  token_endpoint=$(oidc_extract_endpoint "$discovery" "token_endpoint")

  local userinfo_endpoint
  userinfo_endpoint=$(oidc_extract_endpoint "$discovery" "userinfo_endpoint")

  local revoke_endpoint
  revoke_endpoint=$(oidc_extract_endpoint "$discovery" "revocation_endpoint")

  # Validate required endpoints
  if [[ -z "$auth_endpoint" ]] || [[ -z "$token_endpoint" ]]; then
    echo "ERROR: Missing required OIDC endpoints (authorization_endpoint, token_endpoint)" >&2
    return 1
  fi

  # Store configuration (for now, in a temp file; should be in database)
  local config_file="/tmp/nself_oidc_${provider_name}.json"
  cat >"$config_file" <<EOF
{
  "provider": "${provider_name}",
  "issuer": "${issuer_url}",
  "authorization_endpoint": "${auth_endpoint}",
  "token_endpoint": "${token_endpoint}",
  "userinfo_endpoint": "${userinfo_endpoint}",
  "revocation_endpoint": "${revoke_endpoint}"
}
EOF

  echo "✓ OIDC provider '${provider_name}' configured successfully" >&2
  echo "$config_file"
}

# Configure custom OIDC provider (manual)
# Usage: oidc_configure_manual <provider_name> <auth_endpoint> <token_endpoint> <userinfo_endpoint> [revoke_endpoint]
oidc_configure_manual() {
  local provider_name="$1"
  local auth_endpoint="$2"
  local token_endpoint="$3"
  local userinfo_endpoint="$4"
  local revoke_endpoint="${5:-}"

  # Store configuration
  local config_file="/tmp/nself_oidc_${provider_name}.json"
  cat >"$config_file" <<EOF
{
  "provider": "${provider_name}",
  "authorization_endpoint": "${auth_endpoint}",
  "token_endpoint": "${token_endpoint}",
  "userinfo_endpoint": "${userinfo_endpoint}",
  "revocation_endpoint": "${revoke_endpoint}"
}
EOF

  echo "✓ OIDC provider '${provider_name}' configured successfully (manual)" >&2
  echo "$config_file"
}

# Load OIDC provider configuration
# Usage: oidc_load_config <provider_name>
oidc_load_config() {
  local provider_name="$1"

  local config_file="/tmp/nself_oidc_${provider_name}.json"

  if [[ ! -f "$config_file" ]]; then
    echo "ERROR: OIDC provider '${provider_name}' not configured" >&2
    return 1
  fi

  cat "$config_file"
}

# ============================================================================
# OIDC Authentication Flow
# ============================================================================

# Get custom OIDC authorization URL
# Usage: oidc_get_auth_url <provider_name> <client_id> <redirect_uri> [scopes]
oidc_get_auth_url() {
  local provider_name="$1"
  local client_id="$2"
  local redirect_uri="$3"
  local scopes="${4:-$OIDC_DEFAULT_SCOPES}"

  # Load provider configuration
  local config
  config=$(oidc_load_config "$provider_name")

  local auth_endpoint
  auth_endpoint=$(echo "$config" | jq -r '.authorization_endpoint')

  if [[ -z "$auth_endpoint" ]]; then
    echo "ERROR: No authorization endpoint configured for ${provider_name}" >&2
    return 1
  fi

  local state
  state=$(oauth_generate_state)

  oauth_store_state "$state" "{\"provider\": \"${provider_name}\"}"

  oauth_build_auth_url "$auth_endpoint" "$client_id" "$redirect_uri" "$scopes" "$state"
}

# Exchange custom OIDC authorization code for tokens
# Usage: oidc_exchange_code <provider_name> <client_id> <client_secret> <code> <redirect_uri>
oidc_exchange_code() {
  local provider_name="$1"
  local client_id="$2"
  local client_secret="$3"
  local code="$4"
  local redirect_uri="$5"

  # Load provider configuration
  local config
  config=$(oidc_load_config "$provider_name")

  local token_endpoint
  token_endpoint=$(echo "$config" | jq -r '.token_endpoint')

  if [[ -z "$token_endpoint" ]]; then
    echo "ERROR: No token endpoint configured for ${provider_name}" >&2
    return 1
  fi

  oauth_exchange_code "$token_endpoint" "$client_id" "$client_secret" "$code" "$redirect_uri"
}

# Get custom OIDC user info
# Usage: oidc_get_user_info <provider_name> <access_token>
oidc_get_user_info() {
  local provider_name="$1"
  local access_token="$2"

  # Load provider configuration
  local config
  config=$(oidc_load_config "$provider_name")

  local userinfo_endpoint
  userinfo_endpoint=$(echo "$config" | jq -r '.userinfo_endpoint // empty')

  if [[ -z "$userinfo_endpoint" ]]; then
    echo "ERROR: No userinfo endpoint configured for ${provider_name}" >&2
    return 1
  fi

  oauth_get_user_info "$userinfo_endpoint" "$access_token"
}

# Refresh custom OIDC access token
# Usage: oidc_refresh_token <provider_name> <client_id> <client_secret> <refresh_token>
oidc_refresh_token() {
  local provider_name="$1"
  local client_id="$2"
  local client_secret="$3"
  local refresh_token="$4"

  # Load provider configuration
  local config
  config=$(oidc_load_config "$provider_name")

  local token_endpoint
  token_endpoint=$(echo "$config" | jq -r '.token_endpoint')

  if [[ -z "$token_endpoint" ]]; then
    echo "ERROR: No token endpoint configured for ${provider_name}" >&2
    return 1
  fi

  oauth_refresh_token "$token_endpoint" "$client_id" "$client_secret" "$refresh_token"
}

# Revoke custom OIDC token
# Usage: oidc_revoke_token <provider_name> <client_id> <client_secret> <token>
oidc_revoke_token() {
  local provider_name="$1"
  local client_id="$2"
  local client_secret="$3"
  local token="$4"

  # Load provider configuration
  local config
  config=$(oidc_load_config "$provider_name")

  local revoke_endpoint
  revoke_endpoint=$(echo "$config" | jq -r '.revocation_endpoint // empty')

  if [[ -z "$revoke_endpoint" ]]; then
    # No revocation endpoint - tokens expire naturally
    return 0
  fi

  oauth_revoke_token "$revoke_endpoint" "$client_id" "$client_secret" "$token"
}

# ============================================================================
# Export functions
# ============================================================================

export -f oidc_discover_endpoints
export -f oidc_extract_endpoint
export -f oidc_configure_auto
export -f oidc_configure_manual
export -f oidc_load_config
export -f oidc_get_auth_url
export -f oidc_exchange_code
export -f oidc_get_user_info
export -f oidc_refresh_token
export -f oidc_revoke_token
