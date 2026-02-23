#!/usr/bin/env bash
# oauth.sh - OAuth management CLI
# Part of nself - Comprehensive OAuth handlers management
#
# Commands:
#   nself oauth enable --providers google,github,slack
#   nself oauth disable --providers google
#   nself oauth config <provider> [--client-id=<id>] [--client-secret=<secret>]
#   nself oauth test <provider>
#   nself oauth list
#   nself oauth status
#   nself oauth install
#
# Usage: nself oauth <subcommand> [options]

set -euo pipefail

# Get script directory for sourcing dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source dependencies
if ! declare -f log_error >/dev/null 2>&1; then
  source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true
fi

if ! declare -f detect_environment >/dev/null 2>&1; then
  source "$NSELF_ROOT/src/lib/utils/env-detection.sh" 2>/dev/null || true
fi

# Source platform compatibility utilities
if [[ -f "$NSELF_ROOT/src/lib/utils/platform-compat.sh" ]]; then
  source "$NSELF_ROOT/src/lib/utils/platform-compat.sh"
fi

# Constants
if [[ -z "${EXIT_SUCCESS:-}" ]]; then
  source "$NSELF_ROOT/src/lib/config/constants.sh" 2>/dev/null || true
fi

# OAuth library
if [[ -f "$NSELF_ROOT/src/lib/oauth/providers.sh" ]]; then
  source "$NSELF_ROOT/src/lib/oauth/providers.sh"
fi

# OAuth linking library
if [[ -f "$NSELF_ROOT/src/lib/auth/oauth-linking.sh" ]]; then
  source "$NSELF_ROOT/src/lib/auth/oauth-linking.sh"
fi

# ============================================================================
# Helper Functions
# ============================================================================

# Get .env file path
get_env_file() {
  local env_file="$NSELF_ROOT/.env.dev"
  if [[ -f "$NSELF_ROOT/.env.local" ]]; then
    env_file="$NSELF_ROOT/.env.local"
  elif [[ -f "$NSELF_ROOT/.env" ]]; then
    env_file="$NSELF_ROOT/.env"
  fi
  echo "$env_file"
}

# Check if OAuth service is installed
is_oauth_service_installed() {
  local services_dir="$NSELF_ROOT/services"

  if [[ -d "$services_dir/oauth-handlers" ]] || [[ -d "$services_dir/oauth" ]]; then
    return 0
  fi

  return 1
}

# Get OAuth service directory
get_oauth_service_dir() {
  local services_dir="$NSELF_ROOT/services"

  if [[ -d "$services_dir/oauth-handlers" ]]; then
    echo "$services_dir/oauth-handlers"
  elif [[ -d "$services_dir/oauth" ]]; then
    echo "$services_dir/oauth"
  else
    echo "$services_dir/oauth-handlers"
  fi
}

# ============================================================================
# Command Functions
# ============================================================================

# Show usage information
oauth_usage() {
  cat <<EOF
Usage: nself oauth <subcommand> [options]

OAuth providers management for nself

SUBCOMMANDS:
  enable             Enable OAuth providers
  disable            Disable OAuth providers
  config             Configure OAuth provider credentials
  test               Test OAuth provider configuration
  list               List all OAuth providers
  status             Show OAuth service status
  install            Install OAuth handlers service
  accounts           Manage user OAuth accounts
  refresh            Manage token refresh service
  link               Link OAuth provider to user account
  unlink             Unlink OAuth provider from user account

ENABLE OPTIONS:
  --providers=<list>    Comma-separated list of providers (google,github,slack,microsoft)

DISABLE OPTIONS:
  --providers=<list>    Comma-separated list of providers to disable

CONFIG OPTIONS:
  <provider>                Provider name (google, github, slack, microsoft)
  --client-id=<id>         OAuth client ID
  --client-secret=<secret> OAuth client secret
  --tenant-id=<id>         Tenant ID (Microsoft only)
  --callback-url=<url>     Custom callback URL

TEST OPTIONS:
  <provider>                Provider name to test

EXAMPLES:
  # Install OAuth handlers service
  nself oauth install

  # Enable multiple providers
  nself oauth enable --providers google,github,slack

  # Configure Google OAuth
  nself oauth config google \\
    --client-id=xxx.apps.googleusercontent.com \\
    --client-secret=GOCSPX-xxx

  # Configure Microsoft OAuth with tenant
  nself oauth config microsoft \\
    --client-id=xxx \\
    --client-secret=xxx \\
    --tenant-id=your-tenant-id

  # Test provider configuration
  nself oauth test google

  # List all providers
  nself oauth list

  # Show OAuth service status
  nself oauth status

  # List OAuth accounts for user
  nself oauth accounts 123e4567-e89b-12d3-a456-426614174000

  # Manage token refresh service
  nself oauth refresh status
  nself oauth refresh start
  nself oauth refresh once

  # Link provider to user account
  nself oauth link 123e4567-e89b-12d3-a456-426614174000 github

  # Unlink provider from user account
  nself oauth unlink 123e4567-e89b-12d3-a456-426614174000 github

For more information, see: docs/cli/oauth.md
EOF
}

# ============================================================================
# Install Command
# ============================================================================

cmd_oauth_install() {
  log_info "Installing OAuth handlers service..."

  # Check if template exists
  local template_dir="$NSELF_ROOT/src/templates/services/oauth-handlers"
  if [[ ! -d "$template_dir" ]]; then
    log_error "OAuth handlers template not found at: $template_dir"
    exit 1
  fi

  # Determine service directory
  local service_dir
  service_dir=$(get_oauth_service_dir)

  # Check if already installed
  if [[ -d "$service_dir" ]]; then
    log_warning "OAuth handlers service already installed at: $service_dir"
    log_info "Use 'nself oauth config' to configure providers"
    return 0
  fi

  # Create services directory if needed
  mkdir -p "$(dirname "$service_dir")"

  # Copy template
  log_info "Copying OAuth handlers template..."
  cp -r "$template_dir" "$service_dir"

  # Remove .template extensions
  log_info "Processing template files..."
  find "$service_dir" -name "*.template" -type f | while read -r file; do
    local new_file="${file%.template}"
    mv "$file" "$new_file"
  done

  # Get environment values
  local project_name
  local base_domain
  local port

  project_name=$(grep -E "^PROJECT_NAME=" "$(get_env_file)" 2>/dev/null | cut -d'=' -f2 || echo "nself-project")
  base_domain=$(grep -E "^BASE_DOMAIN=" "$(get_env_file)" 2>/dev/null | cut -d'=' -f2 || echo "localhost")
  port="3100"

  # Replace placeholders
  log_info "Configuring service..."
  find "$service_dir" -type f | while read -r file; do
    safe_sed_inline "$file" \
      -e "s/{{SERVICE_NAME}}/oauth-handlers/g" \
      -e "s/{{PROJECT_NAME}}/$project_name/g" \
      -e "s/{{BASE_DOMAIN}}/$base_domain/g" \
      -e "s/{{PORT}}/$port/g" \
      -e "s/{{AUTH_PORT}}/1337/g" \
      -e "s/{{HASURA_GRAPHQL_ADMIN_SECRET}}/nhost-admin-secret/g"
  done

  log_success "OAuth handlers service installed at: $service_dir"
  log_info "Next steps:"
  printf "  1. Configure OAuth providers: %bnself oauth enable --providers google,github%b\n" "${COLOR_CYAN}" "${COLOR_RESET}"
  printf "  2. Set credentials: %bnself oauth config google --client-id=xxx --client-secret=xxx%b\n" "${COLOR_CYAN}" "${COLOR_RESET}"
  printf "  3. Rebuild services: %bnself build%b\n" "${COLOR_CYAN}" "${COLOR_RESET}"
  printf "  4. Start services: %bnself start%b\n" "${COLOR_CYAN}" "${COLOR_RESET}"
}

# ============================================================================
# Enable Command
# ============================================================================

cmd_oauth_enable() {
  local providers=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --providers=*)
        providers="${1#*=}"
        shift
        ;;
      --help|-h)
        oauth_usage
        exit 0
        ;;
      *)
        log_error "Unknown option: $1"
        exit 1
        ;;
    esac
  done

  if [[ -z "$providers" ]]; then
    log_error "Please specify providers with --providers=google,github,slack"
    exit 1
  fi

  # Check if service is installed
  if ! is_oauth_service_installed; then
    log_error "OAuth handlers service not installed. Run: nself oauth install"
    exit 1
  fi

  # Get env file
  local env_file
  env_file=$(get_env_file)

  # Enable each provider
  IFS=',' read -ra PROVIDER_LIST <<< "$providers"
  for provider in "${PROVIDER_LIST[@]}"; do
    # Trim whitespace
    provider=$(echo "$provider" | tr -d '[:space:]')

    # Convert to uppercase for env var
    local provider_upper
    provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

    # Check if already enabled
    if grep -q "^OAUTH_${provider_upper}_ENABLED=true" "$env_file" 2>/dev/null; then
      log_warning "$provider OAuth already enabled"
    else
      # Add or update enabled flag
      if grep -q "^OAUTH_${provider_upper}_ENABLED=" "$env_file" 2>/dev/null; then
        safe_sed_inline "$env_file" "s/^OAUTH_${provider_upper}_ENABLED=.*/OAUTH_${provider_upper}_ENABLED=true/"
      else
        printf "\n# %s OAuth\nOAUTH_%s_ENABLED=true\n" "$provider" "$provider_upper" >> "$env_file"
      fi
      log_success "Enabled $provider OAuth"
    fi
  done

  log_info "Next step: Configure credentials with 'nself oauth config <provider>'"
}

# ============================================================================
# Disable Command
# ============================================================================

cmd_oauth_disable() {
  local providers=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --providers=*)
        providers="${1#*=}"
        shift
        ;;
      --help|-h)
        oauth_usage
        exit 0
        ;;
      *)
        log_error "Unknown option: $1"
        exit 1
        ;;
    esac
  done

  if [[ -z "$providers" ]]; then
    log_error "Please specify providers with --providers=google,github"
    exit 1
  fi

  # Get env file
  local env_file
  env_file=$(get_env_file)

  # Disable each provider
  IFS=',' read -ra PROVIDER_LIST <<< "$providers"
  for provider in "${PROVIDER_LIST[@]}"; do
    provider=$(echo "$provider" | tr -d '[:space:]')
    local provider_upper
    provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

    if grep -q "^OAUTH_${provider_upper}_ENABLED=" "$env_file" 2>/dev/null; then
      safe_sed_inline "$env_file" "s/^OAUTH_${provider_upper}_ENABLED=.*/OAUTH_${provider_upper}_ENABLED=false/"
      log_success "Disabled $provider OAuth"
    else
      log_warning "$provider OAuth not configured"
    fi
  done
}

# ============================================================================
# Config Command
# ============================================================================

cmd_oauth_config() {
  local provider="${1:-}"
  shift || true

  if [[ -z "$provider" ]]; then
    log_error "Provider name required. Usage: nself oauth config <provider>"
    exit 1
  fi

  local client_id=""
  local client_secret=""
  local tenant_id=""
  local callback_url=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --client-id=*)
        client_id="${1#*=}"
        shift
        ;;
      --client-secret=*)
        client_secret="${1#*=}"
        shift
        ;;
      --tenant-id=*)
        tenant_id="${1#*=}"
        shift
        ;;
      --callback-url=*)
        callback_url="${1#*=}"
        shift
        ;;
      --help|-h)
        oauth_usage
        exit 0
        ;;
      *)
        log_error "Unknown option: $1"
        exit 1
        ;;
    esac
  done

  if [[ -z "$client_id" ]] || [[ -z "$client_secret" ]]; then
    log_error "Client ID and client secret required"
    printf "Usage: nself oauth config %s --client-id=xxx --client-secret=xxx\n" "$provider"
    exit 1
  fi

  # Get env file
  local env_file
  env_file=$(get_env_file)

  # Convert provider to uppercase
  local provider_upper
  provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

  # Set default callback URL if not provided
  if [[ -z "$callback_url" ]]; then
    local base_domain
    base_domain=$(grep -E "^BASE_DOMAIN=" "$env_file" 2>/dev/null | cut -d'=' -f2 || echo "localhost")
    callback_url="http://${base_domain}/oauth/${provider}/callback"
  fi

  # Update or add configuration
  log_info "Configuring $provider OAuth..."

  # Helper function to update or add env var
  update_env_var() {
    local key="$1"
    local value="$2"

    if grep -q "^${key}=" "$env_file" 2>/dev/null; then
      safe_sed_inline "$env_file" "s|^${key}=.*|${key}=${value}|"
    else
      printf "%s=%s\n" "$key" "$value" >> "$env_file"
    fi
  }

  update_env_var "OAUTH_${provider_upper}_CLIENT_ID" "$client_id"
  update_env_var "OAUTH_${provider_upper}_CLIENT_SECRET" "$client_secret"
  update_env_var "OAUTH_${provider_upper}_CALLBACK_URL" "$callback_url"

  if [[ -n "$tenant_id" ]]; then
    update_env_var "OAUTH_${provider_upper}_TENANT_ID" "$tenant_id"
  fi

  log_success "$provider OAuth configured successfully"
  log_info "Callback URL: $callback_url"
  log_info "Remember to add this callback URL to your OAuth app settings"
}

# ============================================================================
# Test Command
# ============================================================================

cmd_oauth_test() {
  local provider="${1:-}"

  if [[ -z "$provider" ]]; then
    log_error "Provider name required. Usage: nself oauth test <provider>"
    exit 1
  fi

  log_info "Testing $provider OAuth configuration..."

  # Get env file
  local env_file
  env_file=$(get_env_file)

  # Source env file
  set -a
  source "$env_file"
  set +a

  # Convert provider to uppercase
  local provider_upper
  provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

  # Check if enabled
  local enabled_var="OAUTH_${provider_upper}_ENABLED"
  if [[ "${!enabled_var:-false}" != "true" ]]; then
    log_error "$provider OAuth is not enabled"
    printf "Run: %bnself oauth enable --providers %s%b\n" "${COLOR_CYAN}" "$provider" "${COLOR_RESET}"
    exit 1
  fi

  # Check credentials
  local client_id_var="OAUTH_${provider_upper}_CLIENT_ID"
  local client_secret_var="OAUTH_${provider_upper}_CLIENT_SECRET"
  local callback_url_var="OAUTH_${provider_upper}_CALLBACK_URL"

  local all_good=true

  if [[ -z "${!client_id_var:-}" ]]; then
    log_error "Client ID not set"
    all_good=false
  else
    log_success "Client ID configured"
  fi

  if [[ -z "${!client_secret_var:-}" ]]; then
    log_error "Client secret not set"
    all_good=false
  else
    log_success "Client secret configured"
  fi

  if [[ -z "${!callback_url_var:-}" ]]; then
    log_warning "Callback URL not set (using default)"
  else
    log_success "Callback URL: ${!callback_url_var}"
  fi

  if $all_good; then
    log_success "$provider OAuth configuration is valid"
    log_info "Test the OAuth flow by visiting: http://localhost:3100/oauth/$provider"
  else
    log_error "$provider OAuth configuration is incomplete"
    exit 1
  fi
}

# ============================================================================
# List Command
# ============================================================================

cmd_oauth_list() {
  printf "\n%bAvailable OAuth Providers:%b\n\n" "${COLOR_BOLD}" "${COLOR_RESET}"

  # Get env file
  local env_file
  env_file=$(get_env_file)

  # Define providers
  local providers=("google" "github" "microsoft" "slack")

  for provider in "${providers[@]}"; do
    local provider_upper
    provider_upper=$(echo "$provider" | tr '[:lower:]' '[:upper:]')

    local enabled_var="OAUTH_${provider_upper}_ENABLED"
    local enabled="false"

    if grep -q "^${enabled_var}=true" "$env_file" 2>/dev/null; then
      enabled="true"
    fi

    if [[ "$enabled" == "true" ]]; then
      printf "  %b✓%b %s (enabled)\n" "${COLOR_GREEN}" "${COLOR_RESET}" "$provider"
    else
      printf "  %b○%b %s\n" "${COLOR_DIM}" "${COLOR_RESET}" "$provider"
    fi
  done

  printf "\n"
}

# ============================================================================
# Status Command
# ============================================================================

cmd_oauth_status() {
  log_info "OAuth Service Status"
  printf "\n"

  # Check if installed
  if is_oauth_service_installed; then
    local service_dir
    service_dir=$(get_oauth_service_dir)
    log_success "OAuth handlers service installed at: $service_dir"
  else
    log_warning "OAuth handlers service not installed"
    printf "Run: %bnself oauth install%b\n\n" "${COLOR_CYAN}" "${COLOR_RESET}"
    return 0
  fi

  # List enabled providers
  cmd_oauth_list

  # Check if service is running
  if docker ps 2>/dev/null | grep -q "oauth-handlers"; then
    log_success "OAuth handlers service is running"
  else
    log_warning "OAuth handlers service is not running"
    printf "Run: %bnself start%b\n" "${COLOR_CYAN}" "${COLOR_RESET}"
  fi

  printf "\n"
}

# ============================================================================
# Accounts Command
# ============================================================================

cmd_oauth_accounts() {
  local user_id="${1:-}"

  if [[ -z "$user_id" ]]; then
    log_error "User ID required. Usage: nself oauth accounts <user_id>"
    exit 1
  fi

  oauth_list_providers "$user_id"
}

# ============================================================================
# Refresh Command
# ============================================================================

cmd_oauth_refresh() {
  local subcommand="${1:-status}"

  local refresh_script="$NSELF_ROOT/src/lib/auth/oauth-token-refresh.sh"

  if [[ ! -f "$refresh_script" ]]; then
    log_error "OAuth token refresh service not found"
    exit 1
  fi

  case "$subcommand" in
    status)
      bash "$refresh_script" status
      ;;
    start)
      log_info "Starting OAuth token refresh service..."
      nohup bash "$refresh_script" daemon > /var/log/nself/oauth-refresh.log 2>&1 &
      log_success "OAuth token refresh service started"
      ;;
    stop)
      log_info "Stopping OAuth token refresh service..."
      pkill -f "oauth-token-refresh.sh daemon" || true
      log_success "OAuth token refresh service stopped"
      ;;
    once)
      bash "$refresh_script" once
      ;;
    *)
      log_error "Unknown refresh subcommand: $subcommand"
      printf "Usage: nself oauth refresh [status|start|stop|once]\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# Link Provider Command
# ============================================================================

cmd_oauth_link_provider() {
  local user_id="${1:-}"
  local provider="${2:-}"

  if [[ -z "$user_id" ]] || [[ -z "$provider" ]]; then
    log_error "Usage: nself oauth link <user_id> <provider>"
    exit 1
  fi

  log_warning "This command initiates OAuth flow for linking"
  log_info "User must complete OAuth flow in browser"
  log_info "Linking $provider to user $user_id"

  # In a real implementation, this would generate a special OAuth URL
  # that includes the user_id in the state parameter
  log_info "Visit: http://localhost:3100/oauth/$provider?link_to=$user_id"
}

# ============================================================================
# Unlink Provider Command
# ============================================================================

cmd_oauth_unlink_provider() {
  local user_id="${1:-}"
  local provider="${2:-}"

  if [[ -z "$user_id" ]] || [[ -z "$provider" ]]; then
    log_error "Usage: nself oauth unlink <user_id> <provider>"
    exit 1
  fi

  oauth_unlink_provider "$user_id" "$provider"
}

# ============================================================================
# Main Command Router
# ============================================================================

cmd_oauth() {
  local subcommand="${1:-}"

  if [[ -z "$subcommand" ]]; then
    oauth_usage
    exit 0
  fi

  shift

  case "$subcommand" in
    install)
      cmd_oauth_install "$@"
      ;;
    enable)
      cmd_oauth_enable "$@"
      ;;
    disable)
      cmd_oauth_disable "$@"
      ;;
    config)
      cmd_oauth_config "$@"
      ;;
    test)
      cmd_oauth_test "$@"
      ;;
    list)
      cmd_oauth_list "$@"
      ;;
    status)
      cmd_oauth_status "$@"
      ;;
    accounts)
      cmd_oauth_accounts "$@"
      ;;
    refresh)
      cmd_oauth_refresh "$@"
      ;;
    link)
      cmd_oauth_link_provider "$@"
      ;;
    unlink)
      cmd_oauth_unlink_provider "$@"
      ;;
    help|--help|-h)
      oauth_usage
      exit 0
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      printf "\n"
      oauth_usage
      exit 1
      ;;
  esac
}

# ============================================================================
# Export command for main CLI dispatcher
# ============================================================================

# If executed directly (for testing)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_oauth "$@"
fi
