#!/usr/bin/env bash
# gitlab.sh - GitLab OAuth 2.0 provider
# Part of nself v0.6.0 - Phase 1 Sprint 2


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

[[ -f "$SCRIPT_DIR/../oauth-core.sh" ]] && source "$SCRIPT_DIR/../oauth-core.sh"

readonly GITLAB_AUTH_ENDPOINT="https://gitlab.com/oauth/authorize"
readonly GITLAB_TOKEN_ENDPOINT="https://gitlab.com/oauth/token"
readonly GITLAB_USERINFO_ENDPOINT="https://gitlab.com/api/v4/user"
readonly GITLAB_DEFAULT_SCOPES="read_user"

gitlab_get_auth_url() {
  local client_id="$1"
  local redirect_uri="$2"
  local scopes="${3:-$GITLAB_DEFAULT_SCOPES}"
  local gitlab_url="${4:-https://gitlab.com}"
  local state=$(oauth_generate_state)
  oauth_store_state "$state" "{\"provider\": \"gitlab\", \"gitlab_url\": \"$gitlab_url\"}"
  oauth_build_auth_url "${gitlab_url}/oauth/authorize" "$client_id" "$redirect_uri" "$scopes" "$state"
}

gitlab_exchange_code() {
  local gitlab_url="${5:-https://gitlab.com}"
  oauth_exchange_code "${gitlab_url}/oauth/token" "$1" "$2" "$3" "$4"
}

gitlab_get_user_info() {
  local gitlab_url="${2:-https://gitlab.com}"
  local response=$(curl -s -H "Authorization: Bearer $1" "${gitlab_url}/api/v4/user")
  local user_id=$(echo "$response" | jq -r '.id')
  local username=$(echo "$response" | jq -r '.username')
  local email=$(echo "$response" | jq -r '.email')
  local name=$(echo "$response" | jq -r '.name')
  jq -n --arg id "$user_id" --arg username "$username" --arg email "$email" --arg name "$name" \
    '{id: $id, username: $username, email: $email, name: $name, provider: "gitlab"}'
}

export -f gitlab_get_auth_url gitlab_exchange_code gitlab_get_user_info
