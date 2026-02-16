#!/usr/bin/env bash
# auth.sh - Authentication & Security Management CLI
# Part of nself - Consolidated authentication and security commands
#
# Commands (38 subcommands total):
#   Authentication:
#     nself auth login [--provider=<provider>] [--email=<email>] [--password=<password>]
#     nself auth logout [--all]
#     nself auth status
#
#   MFA (Multi-Factor Authentication):
#     nself auth mfa enable [--method=totp|sms|email] [--user=<id>]
#     nself auth mfa disable [--method=<method>] [--user=<id>]
#     nself auth mfa verify [--method=<method>] [--user=<id>] [--code=<code>]
#     nself auth mfa backup-codes [generate|list|status] [--user=<id>]
#
#   Roles & Permissions:
#     nself auth roles list
#     nself auth roles create [--name=<name>] [--description=<desc>]
#     nself auth roles assign [--user=<id>] [--role=<name>]
#     nself auth roles remove [--user=<id>] [--role=<name>]
#
#   Devices:
#     nself auth devices list <user_id>
#     nself auth devices register <device>
#     nself auth devices revoke <device>
#     nself auth devices trust <device>
#
#   OAuth:
#     nself auth oauth install
#     nself auth oauth enable <provider>
#     nself auth oauth disable <provider>
#     nself auth oauth config <provider> [--client-id=<id>] [--client-secret=<secret>]
#     nself auth oauth test <provider>
#     nself auth oauth list
#     nself auth oauth status
#
#   Security:
#     nself auth security scan [--deep]
#     nself auth security audit
#     nself auth security report
#
#   SSL Management:
#     nself auth ssl generate [domain]
#     nself auth ssl install <cert>
#     nself auth ssl renew [domain]
#     nself auth ssl info [domain]
#     nself auth ssl trust
#
#   Rate Limiting:
#     nself auth rate-limit config [options]
#     nself auth rate-limit status
#     nself auth rate-limit reset [ip]
#
#   Webhooks:
#     nself auth webhooks create <url> [events]
#     nself auth webhooks list
#     nself auth webhooks delete <id>
#     nself auth webhooks test <id>
#     nself auth webhooks logs <id>
#
# Usage: nself auth <subcommand> [options]

set -euo pipefail

# Get script directory for sourcing dependencies
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Save CLI directory before sourcing other modules (which may override SCRIPT_DIR)
CLI_DIR="$SCRIPT_DIR"

# Source dependencies
if [[ -z "${CLI_OUTPUT_SOURCED:-}" ]]; then
  source "$NSELF_ROOT/src/lib/utils/cli-output.sh" 2>/dev/null || true
fi

if [[ -z "${EXIT_SUCCESS:-}" ]]; then
  source "$NSELF_ROOT/src/lib/config/constants.sh" 2>/dev/null || true
fi

# Source auth manager
if [[ -f "$NSELF_ROOT/src/lib/auth/auth-manager.sh" ]]; then
  source "$NSELF_ROOT/src/lib/auth/auth-manager.sh"
fi

# ============================================================================
# Command Functions
# ============================================================================

# Show usage information
auth_usage() {
  cat <<EOF
Usage: nself auth <subcommand> [options]

Authentication and Security management for nself

AUTHENTICATION:
  login              Authenticate user
  logout             End session
  status             Show current auth status

USER MANAGEMENT:
  setup              Interactive auth setup wizard
  create-user        Create new auth user
  list-users         List all auth users

MFA (MULTI-FACTOR AUTHENTICATION):
  mfa enable         Enable MFA for a user
  mfa disable        Disable MFA for a user
  mfa verify         Verify MFA code
  mfa backup-codes   Manage backup codes

ROLES & PERMISSIONS:
  roles list         List all roles
  roles create       Create a new role
  roles assign       Assign role to user
  roles remove       Remove role from user

DEVICES:
  devices list       List user's devices
  devices register   Register a new device
  devices revoke     Revoke device access
  devices trust      Trust a device

OAUTH:
  oauth install      Install OAuth handlers service
  oauth enable       Enable OAuth provider
  oauth disable      Disable OAuth provider
  oauth config       Configure OAuth credentials
  oauth test         Test OAuth provider
  oauth list         List OAuth providers
  oauth status       Show OAuth service status

SECURITY:
  security scan      Security vulnerability scan
  security audit     Security audit
  security report    Generate security report

SSL:
  ssl generate       Generate SSL certificate
  ssl install        Install SSL certificate
  ssl renew          Renew SSL certificate
  ssl info           Show certificate info
  ssl trust          Trust local certificates

RATE LIMITING:
  rate-limit set     Set rate limit for zone
  rate-limit list    List rate limit configuration
  rate-limit status  Show rate limit status
  rate-limit reset   Reset rate limits for IP
  rate-limit whitelist  Manage IP whitelist
  rate-limit block   Manage IP blacklist

WEBHOOKS:
  webhooks create    Create webhook
  webhooks list      List webhooks
  webhooks delete    Delete webhook
  webhooks test      Test webhook
  webhooks logs      Webhook logs

EXAMPLES:
  # Email/password login
  nself auth login --email=user@example.com --password=secret

  # Enable MFA
  nself auth mfa enable --method=totp --user=<user_id>

  # List roles
  nself auth roles list

  # Configure OAuth
  nself auth oauth config google --client-id=xxx --client-secret=yyy

  # Security scan
  nself auth security scan

  # Generate SSL certificate
  nself auth ssl generate

  # Trust local certificates
  nself auth ssl trust

For more information, see: .wiki/commands/AUTH.md
EOF
}

# ============================================================================
# Main Auth Command Router
# ============================================================================

cmd_auth() {
  local subcommand="${1:-}"

  if [[ -z "$subcommand" ]]; then
    auth_usage
    exit 0
  fi

  shift

  case "$subcommand" in
    # Authentication
    login)
      cmd_auth_login "$@"
      ;;
    logout)
      cmd_auth_logout "$@"
      ;;
    status)
      cmd_auth_status "$@"
      ;;

    # User Management
    setup)
      cmd_auth_setup "$@"
      ;;
    create-user | user-create)
      cmd_auth_create_user "$@"
      ;;
    list-users | users)
      cmd_auth_list_users "$@"
      ;;

    # MFA
    mfa)
      cmd_auth_mfa "$@"
      ;;

    # Roles
    roles)
      cmd_auth_roles "$@"
      ;;

    # Devices
    devices)
      cmd_auth_devices "$@"
      ;;

    # OAuth
    oauth)
      cmd_auth_oauth "$@"
      ;;

    # Security
    security)
      cmd_auth_security "$@"
      ;;

    # SSL
    ssl)
      cmd_auth_ssl "$@"
      ;;

    # Rate Limiting
    rate-limit)
      cmd_auth_rate_limit "$@"
      ;;

    # Webhooks
    webhooks)
      cmd_auth_webhooks "$@"
      ;;

    help | --help | -h)
      auth_usage
      exit 0
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      auth_usage
      exit 1
      ;;
  esac
}

# ============================================================================
# Login Command
# ============================================================================

cmd_auth_login() {
  local provider=""
  local email=""
  local password=""
  local phone=""
  local anonymous=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider=*)
        provider="${1#*=}"
        shift
        ;;
      --email=*)
        email="${1#*=}"
        shift
        ;;
      --password=*)
        password="${1#*=}"
        shift
        ;;
      --phone=*)
        phone="${1#*=}"
        shift
        ;;
      --anonymous)
        anonymous=true
        shift
        ;;
      --help | -h)
        auth_usage
        exit 0
        ;;
      *)
        cli_error "Unknown option: $1"
        exit 1
        ;;
    esac
  done

  # Route to appropriate auth method
  if [[ -n "$email" ]] && [[ -n "$password" ]]; then
    auth_login_email "$email" "$password"
  elif [[ -n "$provider" ]]; then
    cli_warning "OAuth login not yet implemented (OAUTH-003+)"
    auth_login_oauth "$provider"
  elif [[ -n "$phone" ]]; then
    cli_warning "Phone login not yet implemented (AUTH-006)"
    auth_login_phone "$phone"
  elif $anonymous; then
    cli_warning "Anonymous login not yet implemented (AUTH-007)"
    auth_login_anonymous
  elif [[ -n "$email" ]]; then
    cli_warning "Magic link not yet implemented (AUTH-005)"
    auth_login_magic_link "$email"
  else
    cli_error "Please provide login credentials"
    printf "Options: --email and --password, --provider, --phone, or --anonymous\n"
    exit 1
  fi
}

# ============================================================================
# Logout Command
# ============================================================================

cmd_auth_logout() {
  local logout_all=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --all)
        logout_all=true
        shift
        ;;
      --help | -h)
        auth_usage
        exit 0
        ;;
      *)
        cli_error "Unknown option: $1"
        exit 1
        ;;
    esac
  done

  cli_info "Logout functionality coming in AUTH-003"
  cli_info "Logout all sessions: $logout_all"
  cli_warning "Not yet implemented - Sprint 1 in progress"
}

# ============================================================================
# Status Command
# ============================================================================

cmd_auth_status() {
  cli_info "Auth status functionality coming in AUTH-003"
  cli_warning "Not yet implemented - Sprint 1 in progress"
}

# ============================================================================
# MFA Commands
# ============================================================================

cmd_auth_mfa() {
  local action="${1:-}"

  if [[ -z "$action" ]]; then
    cli_error "MFA action required"
    printf "Actions: enable, disable, verify, backup-codes\n"
    exit 1
  fi

  shift

  # Delegate to original mfa implementation
  if [[ -f "$CLI_DIR/_deprecated/mfa.sh.backup" ]]; then
    bash "$CLI_DIR/_deprecated/mfa.sh.backup" "$action" "$@"
  else
    cli_error "MFA module not found"
    exit 1
  fi
}

# ============================================================================
# Roles Commands
# ============================================================================

cmd_auth_roles() {
  local action="${1:-list}"
  shift || true

  # Delegate to original roles implementation
  if [[ -f "$CLI_DIR/_deprecated/roles.sh.backup" ]]; then
    bash "$CLI_DIR/_deprecated/roles.sh.backup" "$action" "$@"
  else
    cli_error "Roles module not found"
    exit 1
  fi
}

# ============================================================================
# Devices Commands
# ============================================================================

cmd_auth_devices() {
  local action="${1:-list}"
  shift || true

  # Delegate to original devices implementation
  if [[ -f "$CLI_DIR/_deprecated/devices.sh.backup" ]]; then
    bash "$CLI_DIR/_deprecated/devices.sh.backup" "$action" "$@"
  else
    cli_error "Devices module not found"
    exit 1
  fi
}

# ============================================================================
# OAuth Commands
# ============================================================================

cmd_auth_oauth() {
  local action="${1:-}"

  if [[ -z "$action" ]]; then
    cli_error "OAuth action required"
    printf "Actions: install, enable, disable, config, test, list, status\n"
    exit 1
  fi

  shift

  # Delegate to original oauth implementation
  if [[ -f "$CLI_DIR/_deprecated/oauth.sh.backup" ]]; then
    bash "$CLI_DIR/_deprecated/oauth.sh.backup" "$action" "$@"
  else
    cli_error "OAuth module not found"
    exit 1
  fi
}

# ============================================================================
# Security Commands
# ============================================================================

cmd_auth_security() {
  local action="${1:-scan}"
  shift || true

  # Delegate to original security implementation
  if [[ -f "$CLI_DIR/_deprecated/security.sh.backup" ]]; then
    bash "$CLI_DIR/_deprecated/security.sh.backup" "$action" "$@"
  else
    cli_error "Security module not found"
    exit 1
  fi
}

# ============================================================================
# SSL Commands
# ============================================================================

cmd_auth_ssl() {
  local action="${1:-}"

  if [[ -z "$action" ]]; then
    cli_error "SSL action required"
    printf "Actions: generate, install, renew, info, trust\n"
    exit 1
  fi

  shift

  case "$action" in
    generate)
      # Delegate to original ssl implementation
      if [[ -f "$CLI_DIR/_deprecated/ssl.sh.backup" ]]; then
        bash "$CLI_DIR/_deprecated/ssl.sh.backup" bootstrap "$@"
      else
        cli_error "SSL module not found"
        exit 1
      fi
      ;;
    renew)
      if [[ -f "$CLI_DIR/_deprecated/ssl.sh.backup" ]]; then
        bash "$CLI_DIR/_deprecated/ssl.sh.backup" renew "$@"
      else
        cli_error "SSL module not found"
        exit 1
      fi
      ;;
    info | status)
      if [[ -f "$CLI_DIR/_deprecated/ssl.sh.backup" ]]; then
        bash "$CLI_DIR/_deprecated/ssl.sh.backup" status "$@"
      else
        cli_error "SSL module not found"
        exit 1
      fi
      ;;
    trust)
      # Delegate to original trust implementation
      if [[ -f "$CLI_DIR/_deprecated/trust.sh.backup" ]]; then
        bash "$CLI_DIR/_deprecated/trust.sh.backup" install "$@"
      else
        cli_error "Trust module not found"
        exit 1
      fi
      ;;
    install)
      cli_warning "SSL installation is handled automatically by 'nself build'"
      cli_info "To manually trust certificates, use: nself auth ssl trust"
      ;;
    *)
      cli_error "Unknown SSL action: $action"
      printf "Actions: generate, install, renew, info, trust\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# Rate Limit Commands
# ============================================================================

cmd_auth_rate_limit() {
  local action="${1:-status}"
  shift || true

  # Source rate limit libraries
  if [[ -f "$NSELF_ROOT/src/lib/rate-limit/core.sh" ]]; then
    source "$NSELF_ROOT/src/lib/rate-limit/core.sh"
  fi
  if [[ -f "$NSELF_ROOT/src/lib/rate-limit/nginx-manager.sh" ]]; then
    source "$NSELF_ROOT/src/lib/rate-limit/nginx-manager.sh"
  fi

  case "$action" in
    set)
      # Set rate limit for a zone
      local zone="${1:-}"
      local rate="${2:-}"

      if [[ -z "$zone" ]] || [[ -z "$rate" ]]; then
        cli_error "Zone and rate required"
        printf "Usage: nself auth rate-limit set <zone> <rate>\n"
        printf "Zones: general, graphql_api, auth, uploads, static, webhooks, functions\n"
        printf "Rate: 10r/s (per second) or 100r/m (per minute)\n"
        exit 1
      fi

      nginx_rate_limit_set "$zone" "$rate"
      ;;

    list)
      # List current rate limit configuration
      nginx_rate_limit_list
      ;;

    status)
      # Show rate limit status
      nginx_rate_limit_status
      ;;

    reset)
      # Reset rate limits for an IP
      local ip="${1:-}"

      if [[ -z "$ip" ]]; then
        cli_error "IP address required"
        printf "Usage: nself auth rate-limit reset <ip_address>\n"
        exit 1
      fi

      nginx_rate_limit_reset_ip "$ip"
      ;;

    whitelist)
      # Manage whitelist
      local wl_action="${1:-list}"
      shift || true

      case "$wl_action" in
        add)
          local ip="${1:-}"
          local description="${2:-}"

          if [[ -z "$ip" ]]; then
            cli_error "IP address required"
            printf "Usage: nself auth rate-limit whitelist add <ip> [description]\n"
            exit 1
          fi

          nginx_whitelist_add "$ip" "$description"
          ;;
        remove)
          local ip="${1:-}"

          if [[ -z "$ip" ]]; then
            cli_error "IP address required"
            printf "Usage: nself auth rate-limit whitelist remove <ip>\n"
            exit 1
          fi

          nginx_whitelist_remove "$ip"
          ;;
        list)
          local whitelist
          whitelist=$(nginx_whitelist_list)

          if [[ "$whitelist" == "No whitelisted IPs" ]]; then
            printf "No whitelisted IPs\n"
          else
            printf "Whitelisted IPs:\n\n"
            echo "$whitelist" | jq -r '["IP", "DESCRIPTION", "ENABLED", "CREATED"],
              (.[] | [.ip_address, .description, .enabled, .created_at]) | @tsv' | column -t
          fi
          ;;
        *)
          cli_error "Unknown whitelist action: $wl_action"
          printf "Actions: add, remove, list\n"
          exit 1
          ;;
      esac
      ;;

    block)
      # Manage blacklist
      local bl_action="${1:-list}"
      shift || true

      case "$bl_action" in
        add)
          local ip="${1:-}"
          local reason="${2:-Blocked for abuse}"
          local duration="${3:-}"

          if [[ -z "$ip" ]]; then
            cli_error "IP address required"
            printf "Usage: nself auth rate-limit block add <ip> [reason] [duration_seconds]\n"
            exit 1
          fi

          nginx_blacklist_add "$ip" "$reason" "$duration"
          ;;
        remove)
          local ip="${1:-}"

          if [[ -z "$ip" ]]; then
            cli_error "IP address required"
            printf "Usage: nself auth rate-limit block remove <ip>\n"
            exit 1
          fi

          nginx_blacklist_remove "$ip"
          ;;
        *)
          cli_error "Unknown block action: $bl_action"
          printf "Actions: add, remove\n"
          exit 1
          ;;
      esac
      ;;

    init)
      # Initialize rate limiter
      cli_info "Initializing rate limiter..."
      if rate_limit_init; then
        cli_success "Rate limiter initialized"
      else
        cli_error "Failed to initialize rate limiter"
        exit 1
      fi
      ;;

    violations)
      # Show violations
      if [[ -f "$NSELF_ROOT/src/lib/rate-limit/monitoring.sh" ]]; then
        source "$NSELF_ROOT/src/lib/rate-limit/monitoring.sh"
      fi

      local hours="${1:-24}"
      local violations
      violations=$(rate_limit_violations "$hours")

      if [[ "$violations" == "[]" ]]; then
        printf "No violations in last %s hours\n" "$hours"
      else
        printf "Rate Limit Violations (last %s hours):\n\n" "$hours"
        echo "$violations" | jq -r '["KEY", "VIOLATIONS", "FIRST", "LAST"],
          (.[] | [.key, .violation_count, .first_violation, .last_violation]) | @tsv' | column -t
      fi
      ;;

    alerts)
      # Check for alerts
      if [[ -f "$NSELF_ROOT/src/lib/rate-limit/monitoring.sh" ]]; then
        source "$NSELF_ROOT/src/lib/rate-limit/monitoring.sh"
      fi

      rate_limit_check_alerts
      ;;

    analyze)
      # Analyze nginx logs
      if [[ -f "$NSELF_ROOT/src/lib/rate-limit/monitoring.sh" ]]; then
        source "$NSELF_ROOT/src/lib/rate-limit/monitoring.sh"
      fi

      rate_limit_analyze_nginx_logs
      ;;

    help | --help | -h)
      cat <<EOF
Usage: nself auth rate-limit <action> [options]

Rate limiting management for DDoS protection and abuse prevention

ACTIONS:
  set <zone> <rate>           Set rate limit for a zone
  list                        List current configuration
  status                      Show rate limit status
  reset <ip>                  Reset rate limits for IP
  whitelist <action>          Manage IP whitelist
  block <action>              Manage IP blacklist
  init                        Initialize rate limiter
  violations [hours]          Show recent violations
  alerts                      Check for suspicious patterns
  analyze                     Analyze nginx logs

ZONES:
  general                     General API endpoints
  graphql_api                 GraphQL API (Hasura)
  auth                        Authentication endpoints
  uploads                     File upload endpoints
  static                      Static assets (CSS/JS/images)
  webhooks                    Webhook endpoints
  functions                   Serverless functions
  user_api                    Per-user authenticated requests

RATE FORMAT:
  10r/s                       10 requests per second
  100r/m                      100 requests per minute

EXAMPLES:
  # Set GraphQL API limit
  nself auth rate-limit set graphql_api 200r/m

  # List configuration
  nself auth rate-limit list

  # Check status
  nself auth rate-limit status

  # Reset limits for IP
  nself auth rate-limit reset 192.168.1.100

  # Whitelist trusted IP
  nself auth rate-limit whitelist add 10.0.0.1 "Internal server"

  # Block abusive IP for 1 hour
  nself auth rate-limit block add 1.2.3.4 "Brute force attempt" 3600

  # Remove block
  nself auth rate-limit block remove 1.2.3.4

For more information: https://docs.nself.org/security/rate-limiting
EOF
      ;;

    *)
      cli_error "Unknown rate-limit action: $action"
      printf "Run 'nself auth rate-limit help' for usage\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# Webhooks Commands
# ============================================================================

cmd_auth_webhooks() {
  local action="${1:-list}"
  shift || true

  # Delegate to original webhooks implementation
  if [[ -f "$CLI_DIR/_deprecated/webhooks.sh.backup" ]]; then
    bash "$CLI_DIR/_deprecated/webhooks.sh.backup" "$action" "$@"
  else
    cli_error "Webhooks module not found"
    exit 1
  fi
}

# ============================================================================
# User Management Commands (New)
# ============================================================================

# Setup auth - interactive wizard
cmd_auth_setup() {
  local interactive=true
  local create_defaults=false

  # Parse flags
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --default-users)
        create_defaults=true
        interactive=false
        shift
        ;;
      --non-interactive)
        interactive=false
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  cli_header "Auth Setup Wizard"
  echo ""

  # Step 1: Check database running
  if ! docker ps --format "{{.Names}}" | grep -q postgres; then
    cli_error "PostgreSQL not running. Run 'nself start' first."
    exit 1
  fi

  # Step 2: Check Hasura metadata
  cli_info "Checking Hasura metadata..."
  if ! check_hasura_tracks_auth_tables; then
    cli_warning "Hasura doesn't track auth tables"
    if [[ "$interactive" == "true" ]]; then
      printf "Apply Hasura metadata now? (Y/n): "
      read -r response
      response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
      if [[ ! "$response" =~ ^n ]]; then
        apply_hasura_auth_metadata
      fi
    else
      apply_hasura_auth_metadata
    fi
  else
    cli_success "Hasura metadata configured"
  fi

  # Step 3: Create users
  if [[ "$create_defaults" == "true" ]] || [[ "$interactive" == "false" ]]; then
    cli_info "Creating default users..."
    create_default_auth_users
  else
    printf "Create default staff users? (Y/n): "
    read -r response
    response=$(echo "$response" | tr '[:upper:]' '[:lower:]')
    if [[ ! "$response" =~ ^n ]]; then
      create_default_auth_users
    else
      cli_info "Skipping user creation"
    fi
  fi

  # Step 4: Verify auth service
  cli_info "Verifying auth service..."
  if verify_auth_service_health; then
    cli_success "Auth service configured correctly!"
  else
    cli_warning "Auth service verification failed"
    cli_info "Check logs: nself logs auth"
  fi

  echo ""
  cli_success "Auth setup complete!"
  echo ""
  cli_info "Next steps:"
  echo "  - Test login: curl -k https://auth.local.nself.org/signin/email-password"
  echo "  - Create more users: nself auth create-user user@example.com"
  echo "  - List users: nself auth list-users"
}

# Create single auth user
cmd_auth_create_user() {
  local email=""
  local password=""
  local role="user"
  local display_name=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --email=*)
        email="${1#*=}"
        shift
        ;;
      --password=*)
        password="${1#*=}"
        shift
        ;;
      --role=*)
        role="${1#*=}"
        shift
        ;;
      --name=*)
        display_name="${1#*=}"
        shift
        ;;
      *)
        # First positional arg is email
        if [[ -z "$email" ]]; then
          email="$1"
        fi
        shift
        ;;
    esac
  done

  # Interactive mode if email not provided
  if [[ -z "$email" ]]; then
    printf "Email: "
    read -r email
  fi

  if [[ -z "$email" ]]; then
    cli_error "Email is required"
    exit 1
  fi

  if [[ -z "$password" ]]; then
    printf "Password (leave empty for auto-generated): "
    read -rs password
    echo ""
    if [[ -z "$password" ]]; then
      password=$(generate_secure_password 16)
      cli_info "Generated password: $password"
    fi
  fi

  if [[ -z "$display_name" ]]; then
    display_name="$email"
  fi

  # Create user in database
  create_nhost_auth_user "$email" "$password" "$role" "$display_name"
}

# List auth users
cmd_auth_list_users() {
  # Check database
  if ! docker ps --format "{{.Names}}" | grep -q postgres; then
    cli_error "PostgreSQL not running. Run 'nself start' first."
    exit 1
  fi

  cli_info "Auth Users"
  echo ""

  # Load environment
  load_env_with_priority 2>/dev/null || true

  local db="${POSTGRES_DB:-nself}"
  local user="${POSTGRES_USER:-postgres}"
  local project_name="${PROJECT_NAME:-$(basename "$(pwd)")}"

  # Query with JOIN to get email from user_providers
  local sql="
    SELECT
      u.id,
      up.provider_user_id as email,
      u.display_name,
      u.metadata->>'role' as role,
      u.email_verified,
      u.disabled,
      u.created_at
    FROM auth.users u
    LEFT JOIN auth.user_providers up ON u.id = up.user_id AND up.provider_id = 'email'
    ORDER BY u.created_at DESC;
  "

  docker exec -i "${project_name}_postgres" psql -U "$user" -d "$db" -c "$sql" 2>/dev/null || {
    cli_error "Failed to query users"
    exit 1
  }
}

# ============================================================================
# Helper Functions for User Management
# ============================================================================

# Create nHost auth user with proper schema
create_nhost_auth_user() {
  local email="$1"
  local password="$2"
  local role="${3:-user}"
  local display_name="${4:-$email}"

  # Generate UUID (cross-platform compatible)
  local user_id
  if command -v uuidgen >/dev/null 2>&1; then
    user_id=$(uuidgen | tr '[:upper:]' '[:lower:]')
  else
    # Fallback: generate pseudo-UUID
    user_id=$(cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 32 | head -n 1 | sed 's/\(.\{8\}\)\(.\{4\}\)\(.\{4\}\)\(.\{4\}\)\(.\{12\}\)/\1-\2-\3-\4-\5/')
  fi

  cli_info "Creating user: $email (role: $role)"

  # Load environment
  load_env_with_priority 2>/dev/null || true

  local db="${POSTGRES_DB:-nself}"
  local user="${POSTGRES_USER:-postgres}"
  local project_name="${PROJECT_NAME:-$(basename "$(pwd)")}"

  # SQL to create user with proper nHost structure
  local sql="
    -- Ensure email provider exists
    INSERT INTO auth.providers (id) VALUES ('email') ON CONFLICT DO NOTHING;

    -- Create user
    INSERT INTO auth.users (
      id, display_name, password_hash, email_verified,
      locale, default_role, metadata
    ) VALUES (
      '$user_id',
      '$display_name',
      crypt('$password', gen_salt('bf', 10)),
      true,
      'en',
      'user',
      '{\"role\": \"$role\"}'::jsonb
    ) ON CONFLICT (id) DO NOTHING;

    -- Link to email provider
    INSERT INTO auth.user_providers (
      id, user_id, provider_id, provider_user_id, access_token
    ) VALUES (
      gen_random_uuid(),
      '$user_id',
      'email',
      '$email',
      'seed_token_' || gen_random_uuid()::text
    ) ON CONFLICT (provider_id, provider_user_id) DO NOTHING;
  "

  # Execute via psql
  if echo "$sql" | docker exec -i "${project_name}_postgres" psql -U "$user" -d "$db" >/dev/null 2>&1; then
    cli_success "User created: $email"
    cli_info "User ID: $user_id"
    return 0
  else
    cli_error "Failed to create user"
    return 1
  fi
}

# Check if Hasura tracks auth tables
check_hasura_tracks_auth_tables() {
  # Load environment
  load_env_with_priority 2>/dev/null || true

  local hasura_url="http://localhost:${HASURA_GRAPHQL_PORT:-8080}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET}"

  if [[ -z "$admin_secret" ]]; then
    return 1
  fi

  local response=$(curl -s -X POST "$hasura_url/v1/metadata" \
    -H "X-Hasura-Admin-Secret: $admin_secret" \
    -d '{"type":"export_metadata","args":{}}' 2>/dev/null)

  # Check if auth.users is in tracked tables
  echo "$response" | grep -q '"schema":"auth"' && \
  echo "$response" | grep -q '"name":"users"'
}

# Apply Hasura metadata for auth
apply_hasura_auth_metadata() {
  cli_info "Applying Hasura metadata for auth tables..."

  # Source hasura.sh to use its functions
  if [[ -f "$CLI_DIR/hasura.sh" ]]; then
    source "$CLI_DIR/hasura.sh"
    track_default_schemas
  else
    cli_warning "Hasura commands not available"
    return 1
  fi
}

# Generate secure random password
# Returns: 16-character password with mixed case, numbers, and symbols
# Compatible with Bash 3.2 (macOS), Linux, WSL
generate_secure_password() {
  local length="${1:-16}"

  # Try openssl first (most portable)
  if command -v openssl >/dev/null 2>&1; then
    # Generate base64 random bytes, remove special chars openssl adds, take first N chars
    openssl rand -base64 32 | tr -d '/+=' | head -c "$length"
    return 0
  fi

  # Fallback to /dev/urandom (Linux, macOS, WSL)
  if [[ -r /dev/urandom ]]; then
    # Read random bytes, convert to alphanumeric + some symbols
    tr -dc 'A-Za-z0-9!@#$%^&*' < /dev/urandom | head -c "$length"
    return 0
  fi

  # Last resort: use $RANDOM (weak but better than hardcoded)
  local password=""
  local chars="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*"
  for ((i=0; i<length; i++)); do
    password="${password}${chars:RANDOM%${#chars}:1}"
  done
  printf "%s" "$password"
}

# Create default staff users
create_default_auth_users() {
  # Load environment for password
  load_env_with_priority 2>/dev/null || true

  # Generate secure random password or use environment override
  local default_password="${AUTH_DEFAULT_PASSWORD:-$(generate_secure_password 16)}"

  create_nhost_auth_user "owner@nself.org" "$default_password" "owner" "Platform Owner"
  create_nhost_auth_user "admin@nself.org" "$default_password" "admin" "Administrator"
  create_nhost_auth_user "support@nself.org" "$default_password" "support" "Support Staff"

  cli_success "Created 3 default users (password: $default_password)"
  cli_info "Store this password securely - it will not be shown again"
}

# Verify auth service health
verify_auth_service_health() {
  # Load environment
  load_env_with_priority 2>/dev/null || true

  local base_domain="${BASE_DOMAIN:-local.nself.org}"
  local auth_url="https://auth.${base_domain}"

  local response=$(curl -sk "$auth_url/healthz" 2>/dev/null)
  [[ "$response" == *"ok"* ]] || [[ "$response" == *"healthy"* ]]
}

# ============================================================================
# Export command for main CLI dispatcher
# ============================================================================

# If executed directly (for testing)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_auth "$@"
fi
