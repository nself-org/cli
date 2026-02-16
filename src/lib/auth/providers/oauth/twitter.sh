#!/usr/bin/env bash
# twitter.sh - Twitter/X OAuth 2.0 provider
# Part of nself v0.6.0 - Phase 1 Sprint 2


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

[[ -f "$SCRIPT_DIR/../oauth-core.sh" ]] && source "$SCRIPT_DIR/../oauth-core.sh"

readonly TWITTER_AUTH_ENDPOINT="https://twitter.com/i/oauth2/authorize"
readonly TWITTER_TOKEN_ENDPOINT="https://api.twitter.com/2/oauth2/token"
readonly TWITTER_USERINFO_ENDPOINT="https://api.twitter.com/2/users/me"
readonly TWITTER_DEFAULT_SCOPES="tweet.read users.read"

twitter_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$TWITTER_DEFAULT_SCOPES}"
  local state=$(oauth_generate_state)
  local code_challenge=$(oauth_generate_pkce_challenge)
  oauth_store_state "$state" "{\"provider\": \"twitter\", \"code_challenge\": \"$code_challenge\"}"
  local auth_url=$(oauth_build_auth_url "$TWITTER_AUTH_ENDPOINT" "$client_id" "$redirect_uri" "$scopes" "$state")
  echo "${auth_url}&code_challenge=${code_challenge}&code_challenge_method=S256"
}

twitter_exchange_code() {
  oauth_exchange_code "$TWITTER_TOKEN_ENDPOINT" "$1" "$2" "$3" "$4"
}

twitter_get_user_info() {
  local response=$(curl -s -H "Authorization: Bearer $1" "$TWITTER_USERINFO_ENDPOINT?user.fields=profile_image_url")
  local user_id=$(echo "$response" | jq -r '.data.id')
  local username=$(echo "$response" | jq -r '.data.username')
  local name=$(echo "$response" | jq -r '.data.name')
  jq -n --arg id "$user_id" --arg username "$username" --arg name "$name" \
    '{id: $id, username: $username, name: $name, provider: "twitter"}'
}

export -f twitter_get_auth_url twitter_exchange_code twitter_get_user_info
