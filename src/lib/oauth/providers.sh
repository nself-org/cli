#!/usr/bin/env bash
set -uo pipefail
# providers.sh - OAuth provider management library
# Part of nself - POSIX-compliant, Bash 3.2+ compatible
#
# Provides functions for managing OAuth provider configurations


# Prevent double-sourcing
[[ "${OAUTH_PROVIDERS_SOURCED:-}" == "1" ]] && return 0
export OAUTH_PROVIDERS_SOURCED=1

# Source dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

if ! declare -f log_error >/dev/null 2>&1; then
  source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
fi

if [[ -f "$NSELF_ROOT/src/lib/utils/platform-compat.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/platform-compat.sh"
fi

# ============================================================================
# Provider Metadata
# ============================================================================

# Get provider display name
oauth_get_provider_name() {
  local provider="$1"

  case "$provider" in
    google)
      echo "Google"
      ;;
    github)
      echo "GitHub"
      ;;
    microsoft)
      echo "Microsoft"
      ;;
    slack)
      echo "Slack"
      ;;
    facebook)
      echo "Facebook"
      ;;
    twitter)
      echo "Twitter"
      ;;
    linkedin)
      echo "LinkedIn"
      ;;
    *)
      echo "$provider"
      ;;
  esac
}

# Get provider authorization URL
oauth_get_auth_url() {
  local provider="$1"

  case "$provider" in
    google)
      echo "https://accounts.google.com/o/oauth2/v2/auth"
      ;;
    github)
      echo "https://github.com/login/oauth/authorize"
      ;;
    microsoft)
      local tenant_id="${2:-common}"
      echo "https://login.microsoftonline.com/${tenant_id}/oauth2/v2.0/authorize"
      ;;
    slack)
      echo "https://slack.com/oauth/v2/authorize"
      ;;
    facebook)
      echo "https://www.facebook.com/v12.0/dialog/oauth"
      ;;
    twitter)
      echo "https://twitter.com/i/oauth2/authorize"
      ;;
    linkedin)
      echo "https://www.linkedin.com/oauth/v2/authorization"
      ;;
    *)
      echo ""
      ;;
  esac
}

# Get provider token URL
oauth_get_token_url() {
  local provider="$1"

  case "$provider" in
    google)
      echo "https://oauth2.googleapis.com/token"
      ;;
    github)
      echo "https://github.com/login/oauth/access_token"
      ;;
    microsoft)
      local tenant_id="${2:-common}"
      echo "https://login.microsoftonline.com/${tenant_id}/oauth2/v2.0/token"
      ;;
    slack)
      echo "https://slack.com/api/oauth.v2.access"
      ;;
    facebook)
      echo "https://graph.facebook.com/v12.0/oauth/access_token"
      ;;
    twitter)
      echo "https://api.twitter.com/2/oauth2/token"
      ;;
    linkedin)
      echo "https://www.linkedin.com/oauth/v2/accessToken"
      ;;
    *)
      echo ""
      ;;
  esac
}

# Get provider userinfo URL
oauth_get_userinfo_url() {
  local provider="$1"

  case "$provider" in
    google)
      echo "https://www.googleapis.com/oauth2/v2/userinfo"
      ;;
    github)
      echo "https://api.github.com/user"
      ;;
    microsoft)
      echo "https://graph.microsoft.com/v1.0/me"
      ;;
    slack)
      echo "https://slack.com/api/users.identity"
      ;;
    facebook)
      echo "https://graph.facebook.com/me"
      ;;
    twitter)
      echo "https://api.twitter.com/2/users/me"
      ;;
    linkedin)
      echo "https://api.linkedin.com/v2/me"
      ;;
    *)
      echo ""
      ;;
  esac
}

# Get default scopes for provider
oauth_get_default_scopes() {
  local provider="$1"

  case "$provider" in
    google)
      echo "openid profile email"
      ;;
    github)
      echo "read:user user:email"
      ;;
    microsoft)
      echo "openid profile email User.Read"
      ;;
    slack)
      echo "openid profile email"
      ;;
    facebook)
      echo "email public_profile"
      ;;
    twitter)
      echo "tweet.read users.read"
      ;;
    linkedin)
      echo "r_liteprofile r_emailaddress"
      ;;
    *)
      echo "openid profile email"
      ;;
  esac
}

# ============================================================================
# Provider Configuration
# ============================================================================

# Check if provider is enabled
oauth_is_provider_enabled() {
  local provider="$1"
  local env_file="${2:-}"

  if [[ -z "$env_file" ]]; then
    # Auto-detect env file
    if [[ -f "$NSELF_ROOT/.env.local" ]]; then
      env_file="$NSELF_ROOT/.env.local"
    elif [[ -f "$NSELF_ROOT/.env.dev" ]]; then
      env_file="$NSELF_ROOT/.env.dev"
    elif [[ -f "$NSELF_ROOT/.env" ]]; then
      env_file="$NSELF_ROOT/.env"
    else
      return 1
    fi
  fi

  local provider_upper
  provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

  if grep -q "^OAUTH_${provider_upper}_ENABLED=true" "$env_file" 2>/dev/null; then
    return 0
  fi

  return 1
}

# Get provider configuration
oauth_get_provider_config() {
  local provider="$1"
  local env_file="${2:-}"

  if [[ -z "$env_file" ]]; then
    # Auto-detect env file
    if [[ -f "$NSELF_ROOT/.env.local" ]]; then
      env_file="$NSELF_ROOT/.env.local"
    elif [[ -f "$NSELF_ROOT/.env.dev" ]]; then
      env_file="$NSELF_ROOT/.env.dev"
    elif [[ -f "$NSELF_ROOT/.env" ]]; then
      env_file="$NSELF_ROOT/.env"
    else
      return 1
    fi
  fi

  local provider_upper
  provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

  # Check if enabled
  if ! oauth_is_provider_enabled "$provider" "$env_file"; then
    return 1
  fi

  # Get configuration values
  local client_id
  local client_secret
  local callback_url
  local tenant_id

  client_id=$(grep "^OAUTH_${provider_upper}_CLIENT_ID=" "$env_file" 2>/dev/null | cut -d'=' -f2- || echo "")
  client_secret=$(grep "^OAUTH_${provider_upper}_CLIENT_SECRET=" "$env_file" 2>/dev/null | cut -d'=' -f2- || echo "")
  callback_url=$(grep "^OAUTH_${provider_upper}_CALLBACK_URL=" "$env_file" 2>/dev/null | cut -d'=' -f2- || echo "")
  tenant_id=$(grep "^OAUTH_${provider_upper}_TENANT_ID=" "$env_file" 2>/dev/null | cut -d'=' -f2- || echo "")

  # Return as JSON-like format (for parsing)
  cat <<EOF
provider=$provider
client_id=$client_id
client_secret=$client_secret
callback_url=$callback_url
tenant_id=$tenant_id
EOF
}

# List all enabled providers
oauth_list_enabled_providers() {
  local env_file="${1:-}"

  if [[ -z "$env_file" ]]; then
    # Auto-detect env file
    if [[ -f "$NSELF_ROOT/.env.local" ]]; then
      env_file="$NSELF_ROOT/.env.local"
    elif [[ -f "$NSELF_ROOT/.env.dev" ]]; then
      env_file="$NSELF_ROOT/.env.dev"
    elif [[ -f "$NSELF_ROOT/.env" ]]; then
      env_file="$NSELF_ROOT/.env"
    else
      return 0
    fi
  fi

  local providers=("google" "github" "microsoft" "slack" "facebook" "twitter" "linkedin")
  local enabled_providers=()

  for provider in "${providers[@]}"; do
    if oauth_is_provider_enabled "$provider" "$env_file"; then
      enabled_providers+=("$provider")
    fi
  done

  # Return space-separated list
  printf "%s\n" "${enabled_providers[@]}"
}

# ============================================================================
# Callback URL Generation
# ============================================================================

# Generate callback URL for provider
oauth_generate_callback_url() {
  local provider="$1"
  local base_domain="${2:-localhost}"
  local port="${3:-3100}"

  echo "http://${base_domain}:${port}/oauth/${provider}/callback"
}

# ============================================================================
# Token Storage and Refresh - SECURE CREDENTIAL HANDLING
# ============================================================================

# Store OAuth tokens with secure file permissions
oauth_store_tokens() {
  local provider="$1"
  local user_id="$2"
  local access_token="$3"
  local refresh_token="${4:-}"
  local expires_in="${5:-3600}"

  local token_dir="$NSELF_ROOT/.nself/oauth-tokens"
  mkdir -p "$token_dir"

  local token_file="$token_dir/${provider}_${user_id}.json"

  # Set restrictive umask BEFORE creating token file to ensure secure permissions
  local old_umask
  old_umask=$(umask)
  umask 077

  # Create token file with restrictive permissions
  cat >"$token_file" <<EOF
{
  "provider": "$provider",
  "user_id": "$user_id",
  "access_token": "$access_token",
  "refresh_token": "$refresh_token",
  "expires_in": $expires_in,
  "created_at": $(date +%s)
}
EOF

  # Verify file has restrictive permissions immediately after creation
  chmod 600 "$token_file" || true

  # Restore original umask
  umask "$old_umask"

  # Optional: Encrypt token file at rest using openssl (if available)
  if command -v openssl >/dev/null 2>&1; then
    # You can implement encryption here if needed:
    # openssl enc -aes-256-cbc -in "$token_file" -out "${token_file}.enc" -pass pass:"$encryption_key"
    # rm -f "$token_file"
    true
  fi
}

# Retrieve OAuth tokens
oauth_get_tokens() {
  local provider="$1"
  local user_id="$2"

  local token_file="$NSELF_ROOT/.nself/oauth-tokens/${provider}_${user_id}.json"

  if [[ -f "$token_file" ]]; then
    # Verify file permissions are secure before reading
    local file_perms
    file_perms=$(stat -c "%a" "$token_file" 2>/dev/null || stat -f "%OLp" "$token_file" 2>/dev/null || echo "")
    if [[ "$file_perms" != "600" ]] && [[ "$file_perms" != "rw-------" ]]; then
      # Fix insecure permissions
      chmod 600 "$token_file"
    fi

    cat "$token_file"
    return 0
  fi

  return 1
}

# Check if token is expired
oauth_is_token_expired() {
  local token_file="$1"

  if [[ ! -f "$token_file" ]]; then
    return 0 # Treat missing file as expired
  fi

  local created_at
  local expires_in
  local now

  created_at=$(grep '"created_at"' "$token_file" | grep -o '[0-9]*' | head -1)
  expires_in=$(grep '"expires_in"' "$token_file" | grep -o '[0-9]*' | head -1)
  now=$(date +%s)

  local expiry_time=$((created_at + expires_in))

  if [[ $now -ge $expiry_time ]]; then
    return 0 # Expired
  fi

  return 1 # Not expired
}

# ============================================================================
# Profile Data Normalization
# ============================================================================

# Normalize profile data across providers
oauth_normalize_profile() {
  local provider="$1"
  local raw_profile="$2"

  # This would typically use jq to parse JSON and extract fields
  # For now, return raw profile
  echo "$raw_profile"
}

# ============================================================================
# Validation
# ============================================================================

# Validate provider name
oauth_validate_provider() {
  local provider="$1"

  local valid_providers=("google" "github" "microsoft" "slack" "facebook" "twitter" "linkedin")

  for valid_provider in "${valid_providers[@]}"; do
    if [[ "$provider" == "$valid_provider" ]]; then
      return 0
    fi
  done

  return 1
}

# Validate provider configuration
oauth_validate_config() {
  local provider="$1"
  local env_file="${2:-}"

  if ! oauth_validate_provider "$provider"; then
    log_error "Invalid provider: $provider"
    return 1
  fi

  if ! oauth_is_provider_enabled "$provider" "$env_file"; then
    log_error "Provider not enabled: $provider"
    return 1
  fi

  local config
  config=$(oauth_get_provider_config "$provider" "$env_file")

  local client_id
  client_id=$(echo "$config" | grep "^client_id=" | cut -d'=' -f2-)

  local client_secret
  client_secret=$(echo "$config" | grep "^client_secret=" | cut -d'=' -f2-)

  if [[ -z "$client_id" ]] || [[ -z "$client_secret" ]]; then
    log_error "Missing client credentials for provider: $provider"
    return 1
  fi

  return 0
}

# ============================================================================
# Export Functions
# ============================================================================

export -f oauth_get_provider_name
export -f oauth_get_auth_url
export -f oauth_get_token_url
export -f oauth_get_userinfo_url
export -f oauth_get_default_scopes
export -f oauth_is_provider_enabled
export -f oauth_get_provider_config
export -f oauth_list_enabled_providers
export -f oauth_generate_callback_url
export -f oauth_store_tokens
export -f oauth_get_tokens
export -f oauth_is_token_expired
export -f oauth_normalize_profile
export -f oauth_validate_provider
export -f oauth_validate_config
