#!/usr/bin/env bash
# apple.sh - Apple OAuth provider (Sign in with Apple)
# Part of nself v0.6.0 - Phase 1 Sprint 2


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

[[ -f "$SCRIPT_DIR/../oauth-core.sh" ]] && source "$SCRIPT_DIR/../oauth-core.sh"

readonly APPLE_AUTH_ENDPOINT="https://appleid.apple.com/auth/authorize"
readonly APPLE_TOKEN_ENDPOINT="https://appleid.apple.com/auth/token"
readonly APPLE_DEFAULT_SCOPES="name email"

apple_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$APPLE_DEFAULT_SCOPES}"
  local state=$(oauth_generate_state)
  oauth_store_state "$state" "{\"provider\": \"apple\"}"
  local auth_url=$(oauth_build_auth_url "$APPLE_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state")
  echo "${auth_url}&response_mode=form_post"
}

apple_exchange_code() {
  oauth_exchange_code "$APPLE_TOKEN_ENDPOINT" "$1" "$2" "$3" "$4"
}

apple_get_user_info() {
  local id_token=$(echo "$1" | jq -r '.id_token // empty')
  local payload=$(echo "$id_token" | cut -d. -f2 | base64 -d 2>/dev/null)
  local user_id=$(echo "$payload" | jq -r '.sub // empty')
  local email=$(echo "$payload" | jq -r '.email // empty')
  jq -n --arg id "$user_id" --arg email "$email" '{id: $id, email: $email, provider: "apple"}'
}

export -f apple_get_auth_url apple_exchange_code apple_get_user_info
