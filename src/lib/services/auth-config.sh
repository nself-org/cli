#!/usr/bin/env bash

# auth-config.sh - Generate auth configuration for multi-app support

# Get all allowed redirect URLs for auth service
auth::get_allowed_redirect_urls() {

set -euo pipefail

  local base_domain="${BASE_DOMAIN:-localhost}"
  local urls=()

  # Add main auth client URL
  local auth_client_url="${AUTH_CLIENT_URL:-http://localhost:3000}"
  urls+=("${auth_client_url}/*")

  # Add all frontend app URLs
  local app_count="${FRONTEND_APP_COUNT:-0}"
  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      local route_var="FRONTEND_APP_${i}_ROUTE"
      local route="${!route_var:-}"

      if [[ -n "$route" ]]; then
        # Handle both full domain and subdomain formats
        local full_route=""
        if [[ "$route" == *".${base_domain}" ]]; then
          full_route="$route"
        else
          full_route="${route}.${base_domain}"
        fi

        # Add both http and https variants for development flexibility
        urls+=("http://${full_route}/*")
        urls+=("https://${full_route}/*")

        # Add the auth subdomain for this app
        local app_prefix="${full_route%%.*}"
        local app_domain="${full_route#*.}"
        urls+=("http://auth.${app_prefix}.${app_domain}/*")
        urls+=("https://auth.${app_prefix}.${app_domain}/*")

        # Add the API subdomain for this app
        local remote_url_var="FRONTEND_APP_${i}_REMOTE_SCHEMA_URL"
        local remote_url="${!remote_url_var:-}"
        if [[ -n "$remote_url" ]]; then
          # Parse the API route
          local api_route=""
          if [[ "$remote_url" =~ ^https?:// ]]; then
            api_route=$(echo "$remote_url" | sed -E 's|https?://([^/]+).*|\1|')
          else
            api_route="${remote_url}.${base_domain}"
          fi

          if [[ -n "$api_route" ]]; then
            urls+=("http://${api_route}/*")
            urls+=("https://${api_route}/*")
          fi
        fi
      fi
    done
  fi

  # Add localhost variants for development
  urls+=("http://localhost:*/*")
  urls+=("http://127.0.0.1:*/*")

  # Add any custom redirect URLs if specified
  if [[ -n "${AUTH_ALLOWED_REDIRECT_URLS:-}" ]]; then
    IFS=',' read -ra CUSTOM_URLS <<<"$AUTH_ALLOWED_REDIRECT_URLS"
    for url in "${CUSTOM_URLS[@]}"; do
      urls+=("$url")
    done
  fi

  # Remove duplicates and join with commas
  printf '%s\n' "${urls[@]}" | sort -u | paste -sd ',' -
}

# Get all allowed email redirect URLs
auth::get_allowed_email_redirect_urls() {
  # Similar to redirect URLs but for email links
  auth::get_allowed_redirect_urls
}

# Generate auth environment variables with multi-app support
auth::generate_env_vars() {
  local redirect_urls=$(auth::get_allowed_redirect_urls)
  local email_redirect_urls=$(auth::get_allowed_email_redirect_urls)

  cat <<EOF
AUTH_ALLOWED_REDIRECT_URLS=${redirect_urls}
AUTH_ALLOWED_EMAIL_REDIRECT_URLS=${email_redirect_urls}
EOF
}

# Get all app-specific auth routes for SSL certificate generation
auth::get_ssl_domains() {
  local base_domain="${BASE_DOMAIN:-localhost}"
  local domains=()

  # Add main auth route
  local auth_route="${AUTH_ROUTE:-auth.${base_domain}}"
  domains+=("$auth_route")

  # Add per-app auth routes
  local app_count="${FRONTEND_APP_COUNT:-0}"
  if [[ "$app_count" -gt 0 ]]; then
    for ((i = 1; i <= app_count; i++)); do
      local route_var="FRONTEND_APP_${i}_ROUTE"
      local route="${!route_var:-}"

      if [[ -n "$route" ]]; then
        local app_prefix="${route%%.*}"
        local app_domain="${route#*.}"
        local auth_route="auth.${app_prefix}.${app_domain}"

        # Don't duplicate the main auth route
        if [[ "$auth_route" != "${AUTH_ROUTE:-auth.${base_domain}}" ]]; then
          domains+=("$auth_route")
        fi
      fi
    done
  fi

  printf '%s\n' "${domains[@]}" | sort -u
}

# Export functions
export -f auth::get_allowed_redirect_urls
export -f auth::get_allowed_email_redirect_urls
export -f auth::generate_env_vars
export -f auth::get_ssl_domains
