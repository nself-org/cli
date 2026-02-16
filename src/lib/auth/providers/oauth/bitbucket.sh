#!/usr/bin/env bash
# bitbucket.sh - Bitbucket OAuth 2.0 provider
# Part of nself v0.6.0 - Phase 1 Sprint 2


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

[[ -f "$SCRIPT_DIR/../oauth-core.sh" ]] && source "$SCRIPT_DIR/../oauth-core.sh"

readonly BITBUCKET_AUTH_ENDPOINT="https://bitbucket.org/site/oauth2/authorize"
readonly BITBUCKET_TOKEN_ENDPOINT="https://bitbucket.org/site/oauth2/access_token"
readonly BITBUCKET_USERINFO_ENDPOINT="https://api.bitbucket.org/2.0/user"
readonly BITBUCKET_DEFAULT_SCOPES="account"

bitbucket_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$BITBUCKET_DEFAULT_SCOPES}"
  local state=$(oauth_generate_state)
  oauth_store_state "$state" "{\"provider\": \"bitbucket\"}"
  oauth_build_auth_url "$BITBUCKET_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state"
}

bitbucket_exchange_code() {
  oauth_exchange_code "$BITBUCKET_TOKEN_ENDPOINT" "$1" "$2" "$3" "$4"
}

bitbucket_get_user_info() {
  local response=$(curl -s -H "Authorization: Bearer $1" "$BITBUCKET_USERINFO_ENDPOINT")
  local user_id=$(echo "$response" | jq -r '.uuid')
  local username=$(echo "$response" | jq -r '.username')
  local display_name=$(echo "$response" | jq -r '.display_name')

  # Get email from separate endpoint
  local email_response=$(curl -s -H "Authorization: Bearer $1" "${BITBUCKET_USERINFO_ENDPOINT}/emails")
  local email=$(echo "$email_response" | jq -r '.values[0].email // empty')

  jq -n --arg id "$user_id" --arg username "$username" --arg email "$email" --arg name "$display_name" \
    '{id: $id, username: $username, email: $email, name: $name, provider: "bitbucket"}'
}

export -f bitbucket_get_auth_url bitbucket_exchange_code bitbucket_get_user_info
