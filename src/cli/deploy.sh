#!/usr/bin/env bash
# deploy.sh - Unified deployment and remote server management
# Consolidates: deploy, upgrade, sync, server, servers, provision
# POSIX-compliant, Bash 3.2+ compatible

# Determine root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

ROOT_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"
LIB_DIR="$SCRIPT_DIR/../lib"

# Source required utilities
source "$LIB_DIR/utils/display.sh"
source "$LIB_DIR/utils/env.sh"
source "$LIB_DIR/utils/platform-compat.sh"
source "$LIB_DIR/utils/header.sh" 2>/dev/null || true
source "$LIB_DIR/utils/cli-output.sh" 2>/dev/null || true

# Source deployment modules
source "$LIB_DIR/deploy/ssh.sh" 2>/dev/null || true
source "$LIB_DIR/deploy/credentials.sh" 2>/dev/null || true
source "$LIB_DIR/deploy/health-check.sh" 2>/dev/null || true
source "$LIB_DIR/deploy/zero-downtime.sh" 2>/dev/null || true
source "$LIB_DIR/deploy/security-preflight.sh" 2>/dev/null || true

# Source secure-by-default module
source "$LIB_DIR/security/secure-defaults.sh" 2>/dev/null || true

# Source upgrade libraries
source "$LIB_DIR/upgrade/blue-green.sh" 2>/dev/null || true

# Source environment modules
source "$LIB_DIR/env/create.sh" 2>/dev/null || true
source "$LIB_DIR/env/switch.sh" 2>/dev/null || true

# Configuration
SERVERS_DIR="${SERVERS_DIR:-.nself/servers}"
SERVERS_FILE="${SERVERS_DIR}/servers.json"
SYNC_CONFIG_DIR=".nself/sync"
SYNC_PROFILES_FILE="$SYNC_CONFIG_DIR/profiles.yaml"
SYNC_HISTORY_FILE="$SYNC_CONFIG_DIR/history.log"
PROVIDERS_CONFIG_DIR="${HOME}/.nself/providers"
PROVISION_STATE_DIR=".nself/provision"

# =============================================================================
# HELP TEXT
# =============================================================================

show_deploy_help() {
  cat <<EOF
${CLI_BOLD}nself deploy${CLI_RESET} - Unified deployment and server management

${CLI_BOLD}Usage:${CLI_RESET}
  nself deploy [environment] [OPTIONS]
  nself deploy <subcommand> [OPTIONS]

${CLI_BOLD}Environment Deployment:${CLI_RESET}
  nself deploy staging              Deploy to staging environment
  nself deploy production           Deploy to production environment
  nself deploy <env-name>           Deploy to custom environment

${CLI_BOLD}Deployment Subcommands:${CLI_RESET}
  init          Initialize deployment configuration
  check         Pre-deployment validation checks
  status        Show deployment status
  rollback      Rollback deployment
  logs          View deployment logs
  health        Check deployment health

${CLI_BOLD}Upgrade Management:${CLI_RESET}
  upgrade check         Check for available nself updates
  upgrade perform       Perform zero-downtime upgrade (blue-green)
  upgrade rolling       Perform rolling update (gradual)
  upgrade status        Show current deployment status
  upgrade switch <color> Switch traffic to specified deployment
  upgrade rollback      Rollback to previous deployment

${CLI_BOLD}Remote Server Management:${CLI_RESET}
  server init <host>    Initialize a new VPS server
  server check <host>   Check server readiness
  server status [env]   Quick status of all environments
  server diagnose <env> Full connectivity diagnostics
  server list           List all configured servers
  server add <name>     Add a server
  server remove <name>  Remove a server
  server ssh <name>     SSH into a server
  server info <name>    Show detailed server info

${CLI_BOLD}Infrastructure Provisioning:${CLI_RESET}
  provision <provider>  Provision infrastructure on cloud provider
    --name <name>       Server name (default: PROJECT_NAME-server)
    --size <size>       Instance size: tiny, small, medium, large, xlarge
    --region <region>   Deployment region
    --token <token>     API token (or set HCLOUD_TOKEN / provider env var)
    --ssh-key <name>    SSH key name to attach
    --dry-run           Preview without executing
    --estimate          Show cost estimate only
    --sizes             List available sizes for provider
    --regions           List available regions for provider

  Providers: aws, gcp, azure, do, hetzner, linode, vultr, ionos, ovh, scaleway

${CLI_BOLD}Synchronization:${CLI_RESET}
  sync pull <env>       Pull config from remote environment
  sync push <env>       Push config to remote environment
  sync status           Show sync status
  sync full <env>       Full sync (env + files + rebuild)

${CLI_BOLD}Options:${CLI_RESET}
  --dry-run             Preview deployment without executing
  --force               Skip confirmation prompts
  --rolling             Use rolling deployment (zero-downtime)
  --skip-health         Skip health checks after deployment
  --auto-switch         Automatically switch traffic after health checks
  --auto-cleanup        Automatically cleanup old deployment

${CLI_BOLD}Examples:${CLI_RESET}
  # Deploy to environments
  nself deploy staging
  nself deploy production --dry-run

  # Server management
  nself deploy server init root@server.example.com --domain example.com
  nself deploy server status
  nself deploy server list

  # Upgrades
  nself deploy upgrade perform
  nself deploy upgrade check

  # Provisioning
  nself deploy provision hetzner --size medium
  nself deploy provision hetzner --estimate
  nself deploy provision hetzner --sizes
  nself deploy provision hetzner --token \$HETZNER_CLAWDE_TOKEN --name clawde-api

  # Sync
  nself deploy sync pull staging
  nself deploy sync push staging

${CLI_BOLD}Documentation:${CLI_RESET}
  Full deployment docs: .wiki/deployment/
EOF
}

# =============================================================================
# ENVIRONMENT DEPLOYMENT
# =============================================================================

deploy_environment() {
  local env_name="$1"
  shift

  local dry_run=false
  local force=false
  local rolling=false
  local skip_health=false
  local include_frontends=""

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      --force)
        force=true
        shift
        ;;
      --rolling)
        rolling=true
        shift
        ;;
      --skip-health)
        skip_health=true
        shift
        ;;
      --include-frontends)
        include_frontends="true"
        shift
        ;;
      --exclude-frontends | --backend-only)
        include_frontends="false"
        shift
        ;;
      *) shift ;;
    esac
  done

  show_command_header "nself deploy" "Deploy to $env_name"

  # Check if environment exists
  if [[ ! -d ".environments/$env_name" ]]; then
    cli_error "Environment '$env_name' not found"
    printf "\n"
    cli_info "Create it with: nself env create $env_name"
    return 1
  fi

  # Load environment config
  local env_dir=".environments/$env_name"
  local host=""
  local user="root"
  local port="22"
  local deploy_path=""
  local project_subdir=""

  if [[ -f "$env_dir/server.json" ]]; then
    host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
    user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
    port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
    deploy_path=$(grep '"deploy_path"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
    project_subdir=$(grep '"project_subdir"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4 || true)
    user="${user:-root}"
    port="${port:-22}"
    deploy_path="${deploy_path:-/var/www/nself}"
  fi

  if [[ -z "$host" ]]; then
    cli_error "No host configured for $env_name"
    printf "\n"
    cli_info "Add host to $env_dir/server.json"
    return 1
  fi

  # Compute the directory where nself build runs (deploy_path + optional project_subdir)
  local nself_build_dir="$deploy_path"
  if [[ -n "$project_subdir" ]]; then
    nself_build_dir="$deploy_path/$project_subdir"
  fi

  printf "\n"
  cli_section "Deployment Configuration"
  printf "  Environment:  %s\n" "$env_name"
  printf "  Host:         %s\n" "$host"
  printf "  User:         %s\n" "$user"
  printf "  Port:         %s\n" "$port"
  printf "  Deploy path:  %s\n" "$deploy_path"
  if [[ -n "$project_subdir" ]]; then
    printf "  Project dir:  %s\n" "$project_subdir"
  fi
  printf "\n"

  if [[ "$dry_run" == "true" ]]; then
    cli_info "Dry run mode - showing what would be deployed"
    return 0
  fi

  # Load environment-specific vars from .environments/<env>/ so the
  # security pre-flight can read secrets (e.g. POSTGRES_PASSWORD)
  if [[ -f "$env_dir/.env" ]]; then
    safe_source_env "$env_dir/.env" 2>/dev/null || true
  fi
  if [[ -f "$env_dir/.env.secrets" ]]; then
    safe_source_env "$env_dir/.env.secrets" 2>/dev/null || true
  fi

  # ============================================================
  # SECURE BY DEFAULT: Production Security Pre-flight
  # ============================================================
  if [[ "$env_name" == "prod" ]] || [[ "$env_name" == "production" ]]; then
    cli_section "Security Pre-flight"

    # Run production security checks
    if command -v security::production_preflight >/dev/null 2>&1; then
      if ! security::production_preflight "$env_name"; then
        cli_error "Deployment blocked due to security issues"
        return 1
      fi
    fi

    # Auto-configure firewall on remote server
    cli_info "Configuring firewall on remote server..."

    local firewall_script='
      # Configure UFW if available
      if command -v ufw >/dev/null 2>&1; then
        ufw default deny incoming
        ufw default allow outgoing
        ufw allow ssh
        ufw allow 80/tcp
        ufw allow 443/tcp
        ufw --force enable
        echo "firewall_configured"
      elif command -v firewall-cmd >/dev/null 2>&1; then
        firewall-cmd --permanent --add-service=ssh
        firewall-cmd --permanent --add-service=http
        firewall-cmd --permanent --add-service=https
        firewall-cmd --reload
        echo "firewall_configured"
      else
        echo "no_firewall"
      fi
    '

    local fw_result
    fw_result=$(ssh -o ConnectTimeout=10 -p "$port" "${user}@${host}" "$firewall_script" 2>/dev/null || echo "ssh_failed")

    if echo "$fw_result" | grep -q "firewall_configured"; then
      cli_success "Firewall configured (only ports 22, 80, 443 allowed)"
    elif echo "$fw_result" | grep -q "no_firewall"; then
      cli_warning "No firewall available on server - install ufw or firewalld"
    else
      cli_warning "Could not configure firewall automatically"
    fi
    printf "\n"
  fi

  # Perform deployment
  cli_info "Deploying to $env_name..."

  # Build remote deploy script using configured paths from server.json
  local deploy_script
  deploy_script='cd '"'$deploy_path'"' 2>/dev/null || { echo "ERROR: Cannot access deploy path: '"$deploy_path"'" >&2; exit 1; }

    # Pull latest code if git repo
    if [ -d .git ]; then
      git pull origin main 2>/dev/null || git pull 2>/dev/null || true
    fi

    # Run nself build from the project subdir (where .env files live)
    if command -v nself >/dev/null 2>&1; then
      cd '"'$nself_build_dir'"' 2>/dev/null || true
      nself build 2>/dev/null || true
      cd '"'$deploy_path'"' 2>/dev/null || true
    fi

    # Rebuild images and restart containers
    docker compose up -d --force-recreate --build

    echo "deployment_complete"
  '

  local deploy_result
  deploy_result=$(ssh -o ConnectTimeout=30 -p "$port" "${user}@${host}" "$deploy_script" 2>&1 || echo "deploy_failed")

  if echo "$deploy_result" | grep -q "deployment_complete"; then
    # ============================================================
    # SECURE BY DEFAULT: Post-deployment security verification
    # ============================================================

    # Only run strict security check for production environments
    if [[ "$env_name" == "prod" ]] || [[ "$env_name" == "production" ]]; then
      cli_info "Verifying production security on remote server..."

      local verify_script='
        # Wait for containers to fully bind ports before checking
        sleep 8
        errors=0
        # Check each sensitive port is NOT exposed externally
        for svc_port in 6379 5432 7700 9000; do
          if ss -tlnp 2>/dev/null | grep ":$svc_port " | grep -qv "127.0.0.1:$svc_port\|[::1]:$svc_port"; then
            if ss -tlnp 2>/dev/null | grep ":$svc_port " | grep -q "0.0.0.0"; then
              echo "SECURITY_ERROR: Port $svc_port exposed on 0.0.0.0"
              errors=$((errors + 1))
            fi
          fi
        done

        if [[ $errors -eq 0 ]]; then
          echo "security_verified"
        else
          echo "security_failed"
        fi
      '

      local verify_result
      verify_result=$(ssh -o ConnectTimeout=30 -p "$port" "${user}@${host}" "$verify_script" 2>/dev/null || echo "verify_failed")

      if echo "$verify_result" | grep -q "security_verified"; then
        cli_success "Production security verification passed"
      else
        cli_error "SECURITY VIOLATION: Sensitive ports exposed in production"
        echo "$verify_result" | grep "SECURITY_ERROR" || true
        cli_warning "Rolling back deployment..."
        ssh -o ConnectTimeout=10 -p "$port" "${user}@${host}" "cd '$deploy_path' && docker compose down" 2>/dev/null || true
        return 1
      fi
    else
      # Staging/dev - just informational
      cli_info "Skipping strict port security check for $env_name environment"
      printf "  ${CLI_DIM}Note: In production, sensitive ports must NOT be exposed on 0.0.0.0${CLI_RESET}\n"
    fi

    cli_success "Deployment complete"
  else
    cli_error "Deployment failed"
    echo "$deploy_result" | head -20
    return 1
  fi
}

# =============================================================================
# UPGRADE SUBCOMMANDS
# =============================================================================

cmd_upgrade() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    check)
      upgrade_check_updates
      ;;
    perform)
      upgrade_perform "$@"
      ;;
    rolling)
      upgrade_rolling
      ;;
    status)
      upgrade_status
      ;;
    switch)
      upgrade_switch "$@"
      ;;
    rollback)
      upgrade_rollback
      ;;
    help | --help | -h)
      show_upgrade_help
      ;;
    *)
      cli_error "Unknown upgrade subcommand: $subcommand"
      show_upgrade_help
      return 1
      ;;
  esac
}

upgrade_check_updates() {
  show_command_header "nself deploy upgrade" "Check for Updates"
  printf "\n"

  local current_version="0.9.5"
  if [[ -f "$SCRIPT_DIR/../VERSION" ]]; then
    current_version=$(cat "$SCRIPT_DIR/../VERSION" 2>/dev/null || echo "0.9.5")
  fi

  cli_info "Current version: $current_version"

  if command -v curl >/dev/null 2>&1; then
    cli_info "Checking for updates..."

    local latest_version
    latest_version=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest |
      grep '"tag_name"' |
      sed -E 's/.*"v?([0-9.]+)".*/\1/' 2>/dev/null || echo "")

    if [[ -n "$latest_version" ]]; then
      cli_info "Latest version: $latest_version"
      printf "\n"

      if [[ "$latest_version" != "$current_version" ]]; then
        cli_warning "Update available: $current_version → $latest_version"
        printf "\n"
        printf "To update:\n"
        printf "  curl -sSL https://install.nself.org | bash\n"
        printf "\n"
        printf "Or via Homebrew:\n"
        printf "  brew upgrade nself\n"
        printf "\n"
        printf "Then run: nself deploy upgrade perform\n"
      else
        cli_success "You are running the latest version"
      fi
    else
      cli_warning "Could not check for updates (API rate limit or network issue)"
    fi
  else
    cli_warning "curl not available - cannot check for updates"
  fi
}

upgrade_perform() {
  cli_info "Starting zero-downtime upgrade..."
  printf "\n"

  # Use blue-green deployment if available
  if declare -f perform_blue_green_deployment >/dev/null 2>&1; then
    perform_blue_green_deployment "${SKIP_HEALTH:-false}"
  else
    cli_warning "Blue-green deployment not available"
    cli_info "Performing standard deployment..."
  fi
}

upgrade_rolling() {
  if declare -f perform_rolling_update >/dev/null 2>&1; then
    perform_rolling_update
  else
    cli_error "Rolling update not available"
    return 1
  fi
}

upgrade_status() {
  if declare -f show_deployment_status >/dev/null 2>&1; then
    show_deployment_status
  else
    cli_info "No deployment status available"
  fi
}

upgrade_switch() {
  local target_color="${1:-}"

  if [[ -z "$target_color" ]]; then
    cli_error "Deployment color required (blue or green)"
    return 1
  fi

  if [[ "$target_color" != "blue" ]] && [[ "$target_color" != "green" ]]; then
    cli_error "Invalid color: $target_color (must be 'blue' or 'green')"
    return 1
  fi

  show_command_header "nself deploy upgrade" "Switch Traffic"
  printf "\n"

  if declare -f switch_traffic >/dev/null 2>&1; then
    switch_traffic "$target_color"
  else
    cli_error "Traffic switching not available"
    return 1
  fi
}

upgrade_rollback() {
  if declare -f rollback_deployment >/dev/null 2>&1; then
    rollback_deployment
  else
    cli_error "Rollback not available"
    return 1
  fi
}

show_upgrade_help() {
  cat <<'EOF'
nself deploy upgrade - Zero-downtime upgrades and deployments

Usage: nself deploy upgrade <subcommand> [options]

Subcommands:
  check                 Check for available nself updates
  perform               Perform zero-downtime upgrade (blue-green)
  rolling               Perform rolling update (gradual)
  status                Show current deployment status
  switch <color>        Switch traffic to specified deployment
  rollback              Rollback to previous deployment

Options:
  --auto-switch         Automatically switch traffic after health checks
  --auto-cleanup        Automatically cleanup old deployment
  --skip-health         Skip health checks (not recommended)
  --timeout <seconds>   Health check timeout (default: 60)

Examples:
  nself deploy upgrade check
  nself deploy upgrade perform
  nself deploy upgrade rolling
  nself deploy upgrade switch green
  nself deploy upgrade rollback
EOF
}

# =============================================================================
# SERVER INITIALIZATION PHASES
# =============================================================================

server_init_phase1() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"

  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "$key_file")
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new" "-o" "ConnectTimeout=10" "-p" "$port")

  cli_info "Updating system packages..."

  local update_script='
    # Detect OS
    if command -v apt-get >/dev/null 2>&1; then
      # Debian/Ubuntu
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq
      apt-get upgrade -y -qq
      apt-get install -y -qq curl wget git ca-certificates gnupg lsb-release
    elif command -v yum >/dev/null 2>&1; then
      # RHEL/CentOS/Fedora
      yum update -y -q
      yum install -y -q curl wget git ca-certificates
    else
      echo "Unsupported OS"
      exit 1
    fi
    echo "packages_updated"
  '

  if ssh "${ssh_args[@]}" "${user}@${host}" "$update_script" 2>/dev/null | grep -q "packages_updated"; then
    cli_success "System packages updated"
  else
    cli_error "Failed to update system packages"
    return 1
  fi

  cli_info "Installing Docker and Docker Compose..."

  local docker_script='
    # Check if Docker already installed
    if command -v docker >/dev/null 2>&1; then
      echo "Docker already installed: $(docker --version)"
      exit 0
    fi

    # Install Docker
    if command -v apt-get >/dev/null 2>&1; then
      # Debian/Ubuntu
      export DEBIAN_FRONTEND=noninteractive

      # Add Docker official GPG key
      install -m 0755 -d /etc/apt/keyrings
      curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
      chmod a+r /etc/apt/keyrings/docker.asc

      # Add Docker repository
      echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

      # Install Docker
      apt-get update -qq
      apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin

    elif command -v yum >/dev/null 2>&1; then
      # RHEL/CentOS/Fedora
      yum install -y -q yum-utils
      yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
      yum install -y -q docker-ce docker-ce-cli containerd.io docker-compose-plugin
    fi

    # Enable and start Docker
    systemctl enable docker
    systemctl start docker

    # Verify installation
    docker --version
    docker compose version
    echo "docker_installed"
  '

  if ssh "${ssh_args[@]}" "${user}@${host}" "$docker_script" 2>/dev/null | grep -q "docker_installed\|already installed"; then
    cli_success "Docker installed and running"
  else
    cli_error "Failed to install Docker"
    return 1
  fi

  return 0
}

server_init_phase2() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"

  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "$key_file")
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new" "-o" "ConnectTimeout=10" "-p" "$port")

  cli_info "Configuring firewall (UFW)..."

  local firewall_script='
    # Install UFW if not present
    if ! command -v ufw >/dev/null 2>&1; then
      if command -v apt-get >/dev/null 2>&1; then
        apt-get install -y -qq ufw
      elif command -v yum >/dev/null 2>&1; then
        yum install -y -q ufw
      fi
    fi

    # Configure UFW
    ufw --force reset
    ufw default deny incoming
    ufw default allow outgoing

    # Allow SSH
    ufw allow '"$port"'/tcp

    # Allow HTTP/HTTPS
    ufw allow 80/tcp
    ufw allow 443/tcp

    # Enable UFW
    ufw --force enable

    echo "firewall_configured"
  '

  if ssh "${ssh_args[@]}" "${user}@${host}" "$firewall_script" 2>/dev/null | grep -q "firewall_configured"; then
    cli_success "Firewall configured"
  else
    cli_warning "Could not configure firewall (may not be supported)"
  fi

  cli_info "Installing and configuring fail2ban..."

  local fail2ban_script='
    # Install fail2ban
    if ! command -v fail2ban-client >/dev/null 2>&1; then
      if command -v apt-get >/dev/null 2>&1; then
        apt-get install -y -qq fail2ban
      elif command -v yum >/dev/null 2>&1; then
        yum install -y -q fail2ban
      fi
    fi

    # Create custom SSH jail
    cat > /etc/fail2ban/jail.local << "FAIL2BAN_EOF"
[sshd]
enabled = true
port = '"$port"'
logpath = /var/log/auth.log
maxretry = 5
bantime = 3600
findtime = 600
FAIL2BAN_EOF

    # Start and enable fail2ban
    systemctl enable fail2ban 2>/dev/null || true
    systemctl restart fail2ban 2>/dev/null || true

    echo "fail2ban_configured"
  '

  if ssh "${ssh_args[@]}" "${user}@${host}" "$fail2ban_script" 2>/dev/null | grep -q "fail2ban_configured"; then
    cli_success "fail2ban configured"
  else
    cli_warning "Could not configure fail2ban"
  fi

  cli_info "Hardening SSH configuration..."

  local ssh_harden_script='
    # Backup current SSH config
    cp /etc/ssh/sshd_config /etc/ssh/sshd_config.backup

    # Apply security settings (only if not already set)
    grep -q "^PermitRootLogin" /etc/ssh/sshd_config || echo "PermitRootLogin prohibit-password" >> /etc/ssh/sshd_config
    grep -q "^PasswordAuthentication" /etc/ssh/sshd_config || echo "PasswordAuthentication no" >> /etc/ssh/sshd_config
    grep -q "^PubkeyAuthentication" /etc/ssh/sshd_config || echo "PubkeyAuthentication yes" >> /etc/ssh/sshd_config
    grep -q "^PermitEmptyPasswords" /etc/ssh/sshd_config || echo "PermitEmptyPasswords no" >> /etc/ssh/sshd_config
    grep -q "^X11Forwarding" /etc/ssh/sshd_config || echo "X11Forwarding no" >> /etc/ssh/sshd_config

    # Reload SSH (dont restart to avoid disconnection)
    systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || true

    echo "ssh_hardened"
  '

  if ssh "${ssh_args[@]}" "${user}@${host}" "$ssh_harden_script" 2>/dev/null | grep -q "ssh_hardened"; then
    cli_success "SSH hardened"
  else
    cli_warning "Could not harden SSH configuration"
  fi

  return 0
}

server_init_phase3() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"
  local env_name="$5"
  local domain="$6"

  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "$key_file")
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new" "-o" "ConnectTimeout=10" "-p" "$port")

  cli_info "Creating nself directory structure..."

  local dir_script='
    # Create nself deployment directory
    mkdir -p /var/www/nself
    mkdir -p /var/www/nself/backups
    mkdir -p /var/www/nself/logs

    # Set permissions
    chmod 755 /var/www/nself

    echo "directories_created"
  '

  if ssh "${ssh_args[@]}" "${user}@${host}" "$dir_script" 2>/dev/null | grep -q "directories_created"; then
    cli_success "Directory structure created"
  else
    cli_error "Failed to create directory structure"
    return 1
  fi

  cli_info "Configuring DNS fallback..."

  local dns_script='
    # Backup current resolv.conf
    cp /etc/resolv.conf /etc/resolv.conf.backup 2>/dev/null || true

    # Add Cloudflare and Google DNS as fallback
    cat > /etc/resolv.conf.d/nself-dns << "DNS_EOF"
# nself DNS fallback configuration
nameserver 1.1.1.1
nameserver 1.0.0.1
nameserver 8.8.8.8
nameserver 8.8.4.4
DNS_EOF

    # Make immutable to prevent overwriting
    chattr +i /etc/resolv.conf.d/nself-dns 2>/dev/null || true

    echo "dns_configured"
  '

  ssh "${ssh_args[@]}" "${user}@${host}" "$dns_script" 2>/dev/null | grep -q "dns_configured" && \
    cli_success "DNS fallback configured" || \
    cli_warning "Could not configure DNS fallback"

  # Setup SSL if domain provided
  if [[ -n "$domain" ]]; then
    cli_info "Setting up SSL for domain: $domain"

    local ssl_script='
      domain="'"$domain"'"

      # Install certbot
      if ! command -v certbot >/dev/null 2>&1; then
        if command -v apt-get >/dev/null 2>&1; then
          apt-get install -y -qq certbot python3-certbot-nginx
        elif command -v yum >/dev/null 2>&1; then
          yum install -y -q certbot python3-certbot-nginx
        fi
      fi

      # Check if domain resolves to this server
      server_ip=$(curl -s ifconfig.me)
      domain_ip=$(dig +short "$domain" | tail -n1)

      if [ "$server_ip" = "$domain_ip" ]; then
        echo "Domain resolves correctly to $server_ip"
        echo "ssl_ready"
      else
        echo "Domain does not resolve to this server yet"
        echo "Expected: $server_ip, Got: $domain_ip"
        echo "ssl_not_ready"
      fi
    '

    local ssl_result
    ssl_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "$ssl_script" 2>/dev/null)

    if echo "$ssl_result" | grep -q "ssl_ready"; then
      cli_success "SSL ready (domain resolves correctly)"
      cli_info "SSL certificates will be generated on first deployment"
    else
      cli_warning "Domain does not resolve to server yet"
      cli_info "Update your DNS records to point $domain to this server"
      cli_info "SSL certificates will be generated once DNS propagates"
    fi
  fi

  # Create environment marker
  local env_marker_script='
    echo "'"$env_name"'" > /var/www/nself/.environment
    echo "initialized_at=$(date -Iseconds)" >> /var/www/nself/.environment
    echo "initialized_by=nself-cli" >> /var/www/nself/.environment
    echo "environment_created"
  '

  ssh "${ssh_args[@]}" "${user}@${host}" "$env_marker_script" 2>/dev/null | grep -q "environment_created" && \
    cli_success "Environment marker created" || \
    cli_warning "Could not create environment marker"

  return 0
}

# =============================================================================
# SERVER SUBCOMMANDS
# =============================================================================

cmd_server() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    init)
      server_init "$@"
      ;;
    check)
      server_check "$@"
      ;;
    status)
      server_status "$@"
      ;;
    diagnose)
      server_diagnose "$@"
      ;;
    list)
      server_list "$@"
      ;;
    add)
      server_add "$@"
      ;;
    remove | rm)
      server_remove "$@"
      ;;
    ssh)
      server_ssh "$@"
      ;;
    info)
      server_info "$@"
      ;;
    help | --help | -h)
      show_server_help
      ;;
    *)
      cli_error "Unknown server subcommand: $subcommand"
      show_server_help
      return 1
      ;;
  esac
}

server_init() {
  # Implementation from server.sh
  local host=""
  local user="root"
  local port="22"
  local key_file=""
  local env_name="prod"
  local domain=""
  local skip_ssl="false"
  local skip_dns="false"
  local auto_yes="false"

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --host | -h)
        host="$2"
        shift 2
        ;;
      --user | -u)
        user="$2"
        shift 2
        ;;
      --port | -p)
        port="$2"
        shift 2
        ;;
      --key | -k)
        key_file="$2"
        shift 2
        ;;
      --env | -e)
        env_name="$2"
        shift 2
        ;;
      --domain | -d)
        domain="$2"
        shift 2
        ;;
      --skip-ssl)
        skip_ssl="true"
        shift
        ;;
      --skip-dns)
        skip_dns="true"
        shift
        ;;
      --yes | -y)
        auto_yes="true"
        shift
        ;;
      *)
        # First positional arg is host
        if [[ -z "$host" ]]; then
          host="$1"
        fi
        shift
        ;;
    esac
  done

  show_command_header "nself deploy server init" "Initialize VPS for nself deployment"

  if [[ -z "$host" ]]; then
    cli_error "Host is required"
    printf "Usage: nself deploy server init <host> [options]\n"
    printf "\nExample:\n"
    printf "  nself deploy server init root@server.example.com --domain example.com\n"
    return 1
  fi

  # Parse user@host format
  if [[ "$host" == *"@"* ]]; then
    user="${host%%@*}"
    host="${host#*@}"
  fi

  printf "\n"
  cli_section "Server Configuration"
  printf "  Host:     %s\n" "$host"
  printf "  User:     %s\n" "$user"
  printf "  Port:     %s\n" "$port"
  printf "  Domain:   %s\n" "${domain:-<not set>}"
  printf "  Env:      %s\n" "$env_name"
  printf "\n"

  if [[ "$auto_yes" != "true" ]]; then
    printf "This will:\n"
    printf "  1. Update system packages\n"
    printf "  2. Install Docker and Docker Compose\n"
    printf "  3. Configure firewall (UFW)\n"
    printf "  4. Setup fail2ban for SSH protection\n"
    printf "  5. Configure DNS fallback (optional)\n"
    printf "  6. Setup Let's Encrypt SSL (optional)\n"
    printf "\n"
    printf "Continue? [y/N]: "
    local confirm
    read -r confirm
    confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')
    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "yes" ]]; then
      cli_info "Cancelled"
      return 0
    fi
  fi

  printf "\n"

  # Build SSH arguments
  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "$key_file")
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new")
  ssh_args+=("-o" "ConnectTimeout=10")
  ssh_args+=("-p" "$port")

  # Test connection
  cli_info "Testing SSH connection..."
  if ! ssh "${ssh_args[@]}" "${user}@${host}" "echo 'Connection successful'" 2>/dev/null; then
    cli_error "Cannot connect to $host"
    printf "Check that:\n"
    printf "  1. The server is accessible\n"
    printf "  2. SSH is enabled on port %s\n" "$port"
    printf "  3. Your SSH key is authorized\n"
    return 1
  fi
  cli_success "SSH connection verified"

  # Run initialization phases
  cli_info "Initializing server..."
  printf "\n"

  # Phase 1: System Update & Package Installation
  cli_section "Phase 1: System Setup"
  server_init_phase1 "$host" "$user" "$port" "$key_file"

  # Phase 2: Security Hardening
  cli_section "Phase 2: Security Configuration"
  server_init_phase2 "$host" "$user" "$port" "$key_file"

  # Phase 3: Environment Setup
  cli_section "Phase 3: nself Environment"
  server_init_phase3 "$host" "$user" "$port" "$key_file" "$env_name" "$domain"

  printf "\n"
  cli_success "Server initialization complete!"
  printf "\n"
  printf "Next steps:\n"
  printf "  1. Configure your project: ${CLI_CYAN}nself init${CLI_RESET}\n"
  printf "  2. Build for deployment:   ${CLI_CYAN}nself build --env %s${CLI_RESET}\n" "$env_name"
  printf "  3. Deploy to server:       ${CLI_CYAN}nself deploy %s${CLI_RESET}\n" "$env_name"
  printf "\n"
}

server_check() {
  local host="${1:-}"
  local user="root"
  local port="22"
  local key_file=""

  # Parse user@host:port format
  if [[ "$host" == *"@"* ]]; then
    user="${host%%@*}"
    host="${host#*@}"
  fi

  if [[ "$host" == *":"* ]]; then
    port="${host#*:}"
    host="${host%%:*}"
  fi

  if [[ -z "$host" ]]; then
    cli_error "Host is required"
    printf "Usage: nself deploy server check <host>\n"
    printf "       nself deploy server check user@host:port\n"
    return 1
  fi

  show_command_header "nself deploy server check" "Verify server readiness for deployment"
  printf "\n"

  local ssh_args=()
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new" "-o" "ConnectTimeout=10" "-p" "$port")

  local total_checks=0
  local passed_checks=0

  # Check 1: SSH Connectivity
  total_checks=$((total_checks + 1))
  printf "  [1/8] SSH Connectivity... "
  if ssh "${ssh_args[@]}" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
    printf "${CLI_GREEN}PASS${CLI_RESET}\n"
    passed_checks=$((passed_checks + 1))
  else
    printf "${CLI_RED}FAIL${CLI_RESET}\n"
    cli_error "Cannot connect to $host"
    return 1
  fi

  # Check 2: Docker Installation
  total_checks=$((total_checks + 1))
  printf "  [2/8] Docker Installation... "
  local docker_check
  docker_check=$(ssh "${ssh_args[@]}" "${user}@${host}" "command -v docker >/dev/null 2>&1 && echo 'installed' || echo 'missing'" 2>/dev/null)
  if [[ "$docker_check" == "installed" ]]; then
    local docker_version
    docker_version=$(ssh "${ssh_args[@]}" "${user}@${host}" "docker --version 2>/dev/null" | cut -d' ' -f3 | tr -d ',')
    printf "${CLI_GREEN}PASS${CLI_RESET} (v%s)\n" "$docker_version"
    passed_checks=$((passed_checks + 1))
  else
    printf "${CLI_YELLOW}WARN${CLI_RESET} (not installed)\n"
  fi

  # Check 3: Docker Running
  total_checks=$((total_checks + 1))
  printf "  [3/8] Docker Service... "
  local docker_running
  docker_running=$(ssh "${ssh_args[@]}" "${user}@${host}" "docker info >/dev/null 2>&1 && echo 'running' || echo 'not_running'" 2>/dev/null)
  if [[ "$docker_running" == "running" ]]; then
    printf "${CLI_GREEN}PASS${CLI_RESET}\n"
    passed_checks=$((passed_checks + 1))
  else
    printf "${CLI_YELLOW}WARN${CLI_RESET} (not running)\n"
  fi

  # Check 4: Docker Compose
  total_checks=$((total_checks + 1))
  printf "  [4/8] Docker Compose... "
  local compose_check
  compose_check=$(ssh "${ssh_args[@]}" "${user}@${host}" "docker compose version >/dev/null 2>&1 && echo 'installed' || echo 'missing'" 2>/dev/null)
  if [[ "$compose_check" == "installed" ]]; then
    local compose_version
    compose_version=$(ssh "${ssh_args[@]}" "${user}@${host}" "docker compose version 2>/dev/null | cut -d' ' -f4" 2>/dev/null)
    printf "${CLI_GREEN}PASS${CLI_RESET} (v%s)\n" "$compose_version"
    passed_checks=$((passed_checks + 1))
  else
    printf "${CLI_YELLOW}WARN${CLI_RESET} (not installed)\n"
  fi

  # Check 5: Disk Space
  total_checks=$((total_checks + 1))
  printf "  [5/8] Disk Space... "
  local disk_info
  disk_info=$(ssh "${ssh_args[@]}" "${user}@${host}" "df -h / | tail -n1 | awk '{print \$4, \$5}'" 2>/dev/null)
  local disk_avail disk_used
  disk_avail=$(echo "$disk_info" | cut -d' ' -f1)
  disk_used=$(echo "$disk_info" | cut -d' ' -f2)

  local used_percent
  used_percent=$(echo "$disk_used" | tr -d '%')

  if [[ $used_percent -lt 80 ]]; then
    printf "${CLI_GREEN}PASS${CLI_RESET} (%s available, %s used)\n" "$disk_avail" "$disk_used"
    passed_checks=$((passed_checks + 1))
  elif [[ $used_percent -lt 90 ]]; then
    printf "${CLI_YELLOW}WARN${CLI_RESET} (%s available, %s used)\n" "$disk_avail" "$disk_used"
  else
    printf "${CLI_RED}FAIL${CLI_RESET} (%s available, %s used)\n" "$disk_avail" "$disk_used"
  fi

  # Check 6: Memory
  total_checks=$((total_checks + 1))
  printf "  [6/8] Memory... "
  local mem_info
  mem_info=$(ssh "${ssh_args[@]}" "${user}@${host}" "free -h | grep '^Mem:' | awk '{print \$2, \$3, \$7}'" 2>/dev/null)
  local mem_total mem_used mem_avail
  mem_total=$(echo "$mem_info" | cut -d' ' -f1)
  mem_used=$(echo "$mem_info" | cut -d' ' -f2)
  mem_avail=$(echo "$mem_info" | cut -d' ' -f3)

  printf "${CLI_GREEN}PASS${CLI_RESET} (%s total, %s available)\n" "$mem_total" "$mem_avail"
  passed_checks=$((passed_checks + 1))

  # Check 7: Firewall
  total_checks=$((total_checks + 1))
  printf "  [7/8] Firewall... "
  local firewall_check
  firewall_check=$(ssh "${ssh_args[@]}" "${user}@${host}" "command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | head -n1" 2>/dev/null)
  if echo "$firewall_check" | grep -q "Status: active"; then
    printf "${CLI_GREEN}PASS${CLI_RESET} (active)\n"
    passed_checks=$((passed_checks + 1))
  elif echo "$firewall_check" | grep -q "Status: inactive"; then
    printf "${CLI_YELLOW}WARN${CLI_RESET} (inactive)\n"
  else
    printf "${CLI_YELLOW}WARN${CLI_RESET} (not configured)\n"
  fi

  # Check 8: Ports Available
  total_checks=$((total_checks + 1))
  printf "  [8/8] Required Ports (80, 443)... "
  local port_check
  port_check=$(ssh "${ssh_args[@]}" "${user}@${host}" "
    port80=\$(netstat -tuln 2>/dev/null | grep ':80 ' | wc -l || echo 0)
    port443=\$(netstat -tuln 2>/dev/null | grep ':443 ' | wc -l || echo 0)
    echo \"\$port80 \$port443\"
  " 2>/dev/null)

  local port80_used port443_used
  port80_used=$(echo "$port_check" | cut -d' ' -f1)
  port443_used=$(echo "$port_check" | cut -d' ' -f2)

  if [[ "$port80_used" -eq 0 ]] && [[ "$port443_used" -eq 0 ]]; then
    printf "${CLI_GREEN}PASS${CLI_RESET} (available)\n"
    passed_checks=$((passed_checks + 1))
  else
    printf "${CLI_YELLOW}WARN${CLI_RESET} ("
    [[ "$port80_used" -gt 0 ]] && printf "80 in use "
    [[ "$port443_used" -gt 0 ]] && printf "443 in use"
    printf ")\n"
  fi

  # Summary
  printf "\n"
  cli_section "Check Summary"
  printf "  Passed: %d/%d\n" "$passed_checks" "$total_checks"
  printf "\n"

  if [[ $passed_checks -eq $total_checks ]]; then
    cli_success "Server is ready for deployment"
    return 0
  elif [[ $passed_checks -ge 6 ]]; then
    cli_warning "Server is mostly ready (some warnings)"
    printf "\n"
    cli_info "You can proceed, but consider fixing warnings"
    return 0
  else
    cli_error "Server is not ready for deployment"
    printf "\n"
    cli_info "Run 'nself deploy server init %s' to initialize the server" "${user}@${host}"
    return 1
  fi
}

server_status() {
  show_command_header "nself deploy server status" "Check server connectivity"
  printf "\n"

  # Check if environments directory exists
  if [[ ! -d ".environments" ]]; then
    cli_info "No environments configured"
    printf "\n"
    cli_info "Create an environment with: nself env create <name>"
    return 0
  fi

  # Find all environments with server configurations
  local env_count=0
  local online_count=0

  for env_dir in .environments/*/; do
    if [[ -d "$env_dir" ]] && [[ -f "$env_dir/server.json" ]]; then
      local env_name
      env_name=$(basename "$env_dir")

      # Extract server details from server.json
      local host user port
      host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
      user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
      port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')

      user="${user:-root}"
      port="${port:-22}"

      # Skip if no host configured
      if [[ -z "$host" ]] || [[ "$host" == "localhost" ]]; then
        continue
      fi

      env_count=$((env_count + 1))

      # Test connection
      printf "  %-15s " "$env_name"

      if ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=accept-new -p "$port" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
        printf "${CLI_GREEN}●${CLI_RESET} ONLINE   "

        # Get uptime
        local uptime_info
        uptime_info=$(ssh -o BatchMode=yes -o ConnectTimeout=5 -p "$port" "${user}@${host}" "uptime -p 2>/dev/null || uptime | cut -d',' -f1" 2>/dev/null | head -n1)
        printf "%s\n" "$uptime_info"

        online_count=$((online_count + 1))
      else
        printf "${CLI_RED}●${CLI_RESET} OFFLINE  "
        printf "%s@%s:%s\n" "$user" "$host" "$port"
      fi
    fi
  done

  if [[ $env_count -eq 0 ]]; then
    cli_info "No remote servers configured"
    printf "\n"
    cli_info "Add server details to .environments/<name>/server.json"
  else
    printf "\n"
    printf "  Total: %d server(s), %d online, %d offline\n" "$env_count" "$online_count" "$((env_count - online_count))"
  fi
}

server_diagnose() {
  local env_name="${1:-prod}"

  show_command_header "nself deploy server diagnose" "Full server diagnostics"
  printf "\n"

  # Check if environment exists
  local env_dir=".environments/$env_name"
  if [[ ! -d "$env_dir" ]]; then
    cli_error "Environment '$env_name' not found"
    printf "\n"
    cli_info "Available environments:"
    for dir in .environments/*/; do
      [[ -d "$dir" ]] && printf "  - %s\n" "$(basename "$dir")"
    done
    return 1
  fi

  # Load server configuration
  if [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "No server configuration found for $env_name"
    printf "\n"
    cli_info "Configure server details in $env_dir/server.json"
    return 1
  fi

  local host user port
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')

  user="${user:-root}"
  port="${port:-22}"

  if [[ -z "$host" ]]; then
    cli_error "No host configured in $env_dir/server.json"
    return 1
  fi

  cli_section "Environment: $env_name"
  printf "  Host: %s\n" "$host"
  printf "  User: %s\n" "$user"
  printf "  Port: %s\n" "$port"
  printf "\n"

  cli_section "Network Diagnostics"

  # DNS Resolution
  printf "  [1/5] DNS Resolution... "
  if command -v host >/dev/null 2>&1; then
    local dns_result
    dns_result=$(host "$host" 2>&1)
    if echo "$dns_result" | grep -q "has address"; then
      local ip_address
      ip_address=$(echo "$dns_result" | grep "has address" | head -n1 | awk '{print $NF}')
      printf "${CLI_GREEN}OK${CLI_RESET} → %s\n" "$ip_address"
    else
      printf "${CLI_YELLOW}UNRESOLVED${CLI_RESET} (using as-is)\n"
    fi
  else
    printf "${CLI_YELLOW}SKIP${CLI_RESET} (host command not available)\n"
  fi

  # ICMP Ping
  printf "  [2/5] ICMP Ping... "
  if command -v ping >/dev/null 2>&1; then
    if ping -c 1 -W 2 "$host" >/dev/null 2>&1; then
      local ping_time
      ping_time=$(ping -c 1 "$host" 2>/dev/null | grep "time=" | sed 's/.*time=\([0-9.]*\).*/\1/' | head -n1)
      printf "${CLI_GREEN}OK${CLI_RESET} (%s ms)\n" "${ping_time:-unknown}"
    else
      printf "${CLI_RED}FAIL${CLI_RESET} (host not reachable)\n"
    fi
  else
    printf "${CLI_YELLOW}SKIP${CLI_RESET} (ping not available)\n"
  fi

  # Port 22 (SSH)
  printf "  [3/5] Port %s (SSH)... " "$port"
  if command -v nc >/dev/null 2>&1; then
    if nc -z -w 2 "$host" "$port" 2>/dev/null; then
      printf "${CLI_GREEN}OPEN${CLI_RESET}\n"
    else
      printf "${CLI_RED}CLOSED${CLI_RESET}\n"
    fi
  elif command -v telnet >/dev/null 2>&1; then
    if timeout 2 bash -c "echo '' | telnet $host $port" 2>&1 | grep -q "Connected\|Escape"; then
      printf "${CLI_GREEN}OPEN${CLI_RESET}\n"
    else
      printf "${CLI_RED}CLOSED${CLI_RESET}\n"
    fi
  else
    printf "${CLI_YELLOW}SKIP${CLI_RESET} (nc/telnet not available)\n"
  fi

  # Port 80 (HTTP)
  printf "  [4/5] Port 80 (HTTP)... "
  if command -v nc >/dev/null 2>&1; then
    if nc -z -w 2 "$host" 80 2>/dev/null; then
      printf "${CLI_GREEN}OPEN${CLI_RESET}\n"
    else
      printf "${CLI_YELLOW}CLOSED${CLI_RESET}\n"
    fi
  else
    printf "${CLI_YELLOW}SKIP${CLI_RESET}\n"
  fi

  # Port 443 (HTTPS)
  printf "  [5/5] Port 443 (HTTPS)... "
  if command -v nc >/dev/null 2>&1; then
    if nc -z -w 2 "$host" 443 2>/dev/null; then
      printf "${CLI_GREEN}OPEN${CLI_RESET}\n"
    else
      printf "${CLI_YELLOW}CLOSED${CLI_RESET}\n"
    fi
  else
    printf "${CLI_YELLOW}SKIP${CLI_RESET}\n"
  fi

  printf "\n"
  cli_section "SSH Connection Test"

  local ssh_args=()
  ssh_args+=("-o" "BatchMode=yes" "-o" "ConnectTimeout=10" "-o" "StrictHostKeyChecking=accept-new" "-p" "$port")

  printf "  Attempting SSH connection... "
  if ssh "${ssh_args[@]}" "${user}@${host}" "echo 'connection_ok'" 2>/dev/null | grep -q "connection_ok"; then
    printf "${CLI_GREEN}SUCCESS${CLI_RESET}\n"

    # Get server info if connected
    printf "\n"
    cli_section "Server Information"

    local server_info
    server_info=$(ssh "${ssh_args[@]}" "${user}@${host}" "
      echo \"hostname=\$(hostname)\"
      echo \"os=\$(cat /etc/os-release 2>/dev/null | grep '^ID=' | cut -d= -f2 | tr -d '\"')\"
      echo \"kernel=\$(uname -r)\"
      echo \"uptime=\$(uptime -p 2>/dev/null || uptime | cut -d',' -f1)\"
      echo \"load=\$(uptime | awk -F'load average:' '{print \$2}' | xargs)\"
      echo \"memory=\$(free -h 2>/dev/null | grep '^Mem:' | awk '{print \$2}')\"
      echo \"docker=\$(docker --version 2>/dev/null | cut -d' ' -f3 | tr -d ',' || echo 'not installed')\"
    " 2>/dev/null)

    echo "$server_info" | while IFS='=' read -r key value; do
      printf "  %-12s %s\n" "${key}:" "$value"
    done

  else
    printf "${CLI_RED}FAILED${CLI_RESET}\n"
    printf "\n"
    cli_section "Recommendations"
    printf "  1. Verify SSH key is authorized on the server\n"
    printf "  2. Check that SSH is running on port %s\n" "$port"
    printf "  3. Ensure firewall allows SSH connections\n"
    printf "  4. Try manual connection: ssh -p %s %s@%s\n" "$port" "$user" "$host"
    return 1
  fi

  printf "\n"
  cli_success "Diagnostics complete"
}

server_list() {
  init_servers_config

  show_command_header "nself deploy server" "Server List"
  printf "\n"

  local server_count=0

  # Check environments directory
  if [[ ! -d ".environments" ]]; then
    cli_info "No servers configured"
    printf "\n"
    cli_info "Create an environment with server details:"
    printf "  nself env create staging\n"
    printf "  nself deploy server add staging --host server.example.com\n"
    return 0
  fi

  # List all environments with server configurations
  printf "%-15s %-25s %-10s %-10s %s\n" "NAME" "HOST" "USER" "PORT" "STATUS"
  printf "%-15s %-25s %-10s %-10s %s\n" "---------------" "-------------------------" "----------" "----------" "----------"

  for env_dir in .environments/*/; do
    if [[ -d "$env_dir" ]] && [[ -f "$env_dir/server.json" ]]; then
      local env_name host user port env_type status
      env_name=$(basename "$env_dir")

      # Parse server.json
      host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
      user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
      port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
      env_type=$(grep '"type"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)

      user="${user:-root}"
      port="${port:-22}"
      env_type="${env_type:-unknown}"

      # Skip localhost/local
      if [[ -z "$host" ]] || [[ "$host" == "localhost" ]] || [[ "$env_type" == "local" ]]; then
        continue
      fi

      server_count=$((server_count + 1))

      # Quick connectivity check
      if ssh -o BatchMode=yes -o ConnectTimeout=2 -o StrictHostKeyChecking=accept-new -p "$port" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
        status="${CLI_GREEN}online${CLI_RESET}"
      else
        status="${CLI_RED}offline${CLI_RESET}"
      fi

      printf "%-15s %-25s %-10s %-10s %s\n" "$env_name" "$host" "$user" "$port" "$status"
    fi
  done

  if [[ $server_count -eq 0 ]]; then
    printf "\n"
    cli_info "No remote servers configured"
    printf "\n"
    cli_info "Add a server:"
    printf "  nself env create prod\n"
    printf "  nself deploy server add prod --host server.example.com\n"
  else
    printf "\n"
    printf "Total: %d server(s)\n" "$server_count"
  fi
}

server_add() {
  local name="$1"
  shift

  if [[ -z "$name" ]]; then
    cli_error "Server name required"
    printf "Usage: nself deploy server add <name> --host <host> [--user <user>] [--port <port>]\n"
    return 1
  fi

  local host=""
  local user="root"
  local port="22"
  local key_file=""
  local deploy_path="/var/www/nself"
  local project_subdir=""

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --host | -h)
        host="$2"
        shift 2
        ;;
      --user | -u)
        user="$2"
        shift 2
        ;;
      --port | -p)
        port="$2"
        shift 2
        ;;
      --key | -k)
        key_file="$2"
        shift 2
        ;;
      --path)
        deploy_path="$2"
        shift 2
        ;;
      --subdir)
        project_subdir="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ -z "$host" ]]; then
    cli_error "Host is required"
    printf "Usage: nself deploy server add <name> --host <host>\n"
    return 1
  fi

  # Check if environment exists
  local env_dir=".environments/$name"
  if [[ ! -d "$env_dir" ]]; then
    cli_info "Environment '$name' does not exist, creating it..."
    mkdir -p "$env_dir"

    # Create basic .env
    cat >"$env_dir/.env" <<EOF
# $name Environment Configuration
ENV=production
BASE_DOMAIN=$host
EOF
  fi

  # Create or update server.json
  if [[ -n "$project_subdir" ]]; then
    cat >"$env_dir/server.json" <<EOF
{
  "name": "$name",
  "type": "remote",
  "host": "$host",
  "port": $port,
  "user": "$user",
  "key": "$key_file",
  "deploy_path": "$deploy_path",
  "project_subdir": "$project_subdir",
  "description": "Remote server configuration",
  "created_at": "$(date -Iseconds 2>/dev/null || date)"
}
EOF
  else
    cat >"$env_dir/server.json" <<EOF
{
  "name": "$name",
  "type": "remote",
  "host": "$host",
  "port": $port,
  "user": "$user",
  "key": "$key_file",
  "deploy_path": "$deploy_path",
  "description": "Remote server configuration",
  "created_at": "$(date -Iseconds 2>/dev/null || date)"
}
EOF
  fi

  cli_success "Server added: $name"
  printf "\n"
  printf "Server details:\n"
  printf "  Host:        %s\n" "$host"
  printf "  User:        %s\n" "$user"
  printf "  Port:        %s\n" "$port"
  printf "  Deploy path: %s\n" "$deploy_path"
  if [[ -n "$project_subdir" ]]; then
    printf "  Subdir:      %s\n" "$project_subdir"
    printf "  Project dir: %s/%s\n" "$deploy_path" "$project_subdir"
  fi
  printf "\n"
  cli_info "Test connection with: nself deploy server check $name"
}

server_remove() {
  local name="$1"
  shift

  if [[ -z "$name" ]]; then
    cli_error "Server name required"
    printf "Usage: nself deploy server remove <name>\n"
    return 1
  fi

  local force=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force | -f)
        force=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  local env_dir=".environments/$name"

  if [[ ! -d "$env_dir" ]]; then
    cli_error "Server '$name' not found"
    return 1
  fi

  if [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "Not a configured server: $name"
    return 1
  fi

  # Get server details for confirmation
  local host
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)

  printf "This will remove server configuration:\n"
  printf "  Name: %s\n" "$name"
  printf "  Host: %s\n" "$host"
  printf "\n"
  printf "${CLI_YELLOW}WARNING:${CLI_RESET} This will NOT delete the environment or remote data\n"
  printf "         Only the server.json configuration will be removed\n"
  printf "\n"

  if [[ "$force" != "true" ]]; then
    printf "Are you sure? [y/N]: "
    local confirm
    read -r confirm
    confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')

    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "yes" ]]; then
      cli_info "Cancelled"
      return 0
    fi
  fi

  # Remove server.json
  rm -f "$env_dir/server.json"

  cli_success "Server configuration removed: $name"
  printf "\n"
  cli_info "The environment directory still exists at: $env_dir"
  cli_info "To completely remove the environment, use: nself env delete $name"
}

server_ssh() {
  local name="$1"
  shift

  if [[ -z "$name" ]]; then
    cli_error "Server name required"
    printf "Usage: nself deploy server ssh <name> [command]\n"
    return 1
  fi

  local env_dir=".environments/$name"

  if [[ ! -d "$env_dir" ]] || [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "Server '$name' not found"
    printf "\n"
    cli_info "Available servers:"
    for dir in .environments/*/; do
      if [[ -f "$dir/server.json" ]]; then
        printf "  - %s\n" "$(basename "$dir")"
      fi
    done
    return 1
  fi

  # Load server configuration
  local host user port key_file
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
  key_file=$(grep '"key"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)

  user="${user:-root}"
  port="${port:-22}"

  if [[ -z "$host" ]]; then
    cli_error "No host configured for $name"
    return 1
  fi

  # Build SSH command
  local ssh_cmd="ssh"

  # Add key if specified
  if [[ -n "$key_file" ]] && [[ -f "${key_file/#\~/$HOME}" ]]; then
    ssh_cmd="$ssh_cmd -i ${key_file/#\~/$HOME}"
  fi

  # Add port
  ssh_cmd="$ssh_cmd -p $port"

  # Add host
  ssh_cmd="$ssh_cmd ${user}@${host}"

  # If additional arguments provided, treat as remote command
  if [[ $# -gt 0 ]]; then
    cli_info "Executing on ${name}: $*"
    $ssh_cmd "$@"
  else
    cli_info "Connecting to ${name} (${user}@${host}:${port})..."
    printf "\n"
    $ssh_cmd
  fi
}

server_info() {
  local name="$1"

  if [[ -z "$name" ]]; then
    cli_error "Server name required"
    printf "Usage: nself deploy server info <name>\n"
    return 1
  fi

  local env_dir=".environments/$name"

  if [[ ! -d "$env_dir" ]] || [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "Server '$name' not found"
    return 1
  fi

  show_command_header "nself deploy server info" "Server Details: $name"
  printf "\n"

  # Parse server.json
  local host user port key_file deploy_path env_type description
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
  key_file=$(grep '"key"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  deploy_path=$(grep '"deploy_path"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  env_type=$(grep '"type"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  description=$(grep '"description"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)

  user="${user:-root}"
  port="${port:-22}"
  deploy_path="${deploy_path:-/var/www/nself}"
  env_type="${env_type:-remote}"

  cli_section "Connection Details"
  printf "  Name:        %s\n" "$name"
  printf "  Host:        %s\n" "$host"
  printf "  User:        %s\n" "$user"
  printf "  Port:        %s\n" "$port"
  printf "  Type:        %s\n" "$env_type"
  [[ -n "$key_file" ]] && printf "  SSH Key:     %s\n" "$key_file"
  printf "  Deploy Path: %s\n" "$deploy_path"
  [[ -n "$description" ]] && printf "  Description: %s\n" "$description"
  printf "\n"

  # Test connectivity
  cli_section "Connectivity"
  printf "  Testing SSH connection... "

  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "${key_file/#\~/$HOME}")
  ssh_args+=("-o" "BatchMode=yes" "-o" "ConnectTimeout=5" "-o" "StrictHostKeyChecking=accept-new" "-p" "$port")

  if ssh "${ssh_args[@]}" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
    printf "${CLI_GREEN}CONNECTED${CLI_RESET}\n"

    # Get remote system information
    printf "\n"
    cli_section "Remote System Information"

    local remote_info
    remote_info=$(ssh "${ssh_args[@]}" "${user}@${host}" "
      echo \"hostname=\$(hostname 2>/dev/null || echo 'unknown')\"
      echo \"os=\$(cat /etc/os-release 2>/dev/null | grep '^PRETTY_NAME=' | cut -d'\"' -f2 || echo 'unknown')\"
      echo \"kernel=\$(uname -r 2>/dev/null || echo 'unknown')\"
      echo \"arch=\$(uname -m 2>/dev/null || echo 'unknown')\"
      echo \"cpu_cores=\$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'unknown')\"
      echo \"memory=\$(free -h 2>/dev/null | grep '^Mem:' | awk '{print \$2}' || echo 'unknown')\"
      echo \"disk_root=\$(df -h / 2>/dev/null | tail -n1 | awk '{print \$2}' || echo 'unknown')\"
      echo \"disk_avail=\$(df -h / 2>/dev/null | tail -n1 | awk '{print \$4}' || echo 'unknown')\"
      echo \"uptime=\$(uptime -p 2>/dev/null || uptime | cut -d',' -f1 || echo 'unknown')\"
      echo \"docker=\$(docker --version 2>/dev/null | cut -d' ' -f3 | tr -d ',' || echo 'not installed')\"
      echo \"compose=\$(docker compose version 2>/dev/null | cut -d' ' -f4 || echo 'not installed')\"
    " 2>/dev/null)

    if [[ -n "$remote_info" ]]; then
      echo "$remote_info" | while IFS='=' read -r key value; do
        printf "  %-15s %s\n" "${key}:" "$value"
      done
    else
      printf "  Could not retrieve system information\n"
    fi

    # Check if nself is deployed
    printf "\n"
    cli_section "Deployment Status"

    local deploy_status
    deploy_status=$(ssh "${ssh_args[@]}" "${user}@${host}" "
      if [ -d '$deploy_path' ]; then
        if [ -f '$deploy_path/docker-compose.yml' ]; then
          cd '$deploy_path' 2>/dev/null
          running=\$(docker compose ps --format '{{.Name}}' --filter 'status=running' 2>/dev/null | wc -l)
          total=\$(docker compose ps -a --format '{{.Name}}' 2>/dev/null | wc -l)
          echo \"deployed=yes\"
          echo \"containers=\$running/\$total\"
        else
          echo \"deployed=no\"
          echo \"reason=no docker-compose.yml\"
        fi
      else
        echo \"deployed=no\"
        echo \"reason=directory not found\"
      fi
    " 2>/dev/null)

    local is_deployed containers
    is_deployed=$(echo "$deploy_status" | grep "^deployed=" | cut -d'=' -f2)
    containers=$(echo "$deploy_status" | grep "^containers=" | cut -d'=' -f2)

    if [[ "$is_deployed" == "yes" ]]; then
      printf "  Status:      ${CLI_GREEN}Deployed${CLI_RESET}\n"
      printf "  Containers:  %s running\n" "$containers"
    else
      printf "  Status:      ${CLI_YELLOW}Not deployed${CLI_RESET}\n"
    fi

  else
    printf "${CLI_RED}FAILED${CLI_RESET}\n"
    printf "\n"
    cli_warning "Cannot connect to server"
    printf "\n"
    printf "Try:\n"
    printf "  ssh -p %s %s@%s\n" "$port" "$user" "$host"
  fi

  printf "\n"
  cli_section "Quick Actions"
  printf "  Connect:     nself deploy server ssh %s\n" "$name"
  printf "  Diagnose:    nself deploy server diagnose %s\n" "$name"
  printf "  Deploy:      nself deploy %s\n" "$name"
  printf "\n"
}

show_server_help() {
  cat <<EOF
nself deploy server - VPS server management

Usage: nself deploy server <command> [options]

Commands:
  init      Initialize a new VPS server for nself
  check     Check server readiness (detailed)
  status    Quick status of all environments
  diagnose  Full connectivity diagnostics
  list      List all configured servers
  add       Add a server
  remove    Remove a server
  ssh       SSH into a server
  info      Show detailed server info

Examples:
  nself deploy server init root@server.example.com --domain example.com
  nself deploy server check root@server.example.com
  nself deploy server status
  nself deploy server diagnose prod
  nself deploy server list
EOF
}

# =============================================================================
# PROVISION SUBCOMMANDS
# =============================================================================

cmd_provision() {
  local provider="${1:-}"
  shift || true

  if [[ -z "$provider" ]]; then
    cli_error "Provider required"
    printf "\n"
    cli_info "Supported providers:"
    printf "  aws, gcp, azure, do, hetzner, linode, vultr, ionos, ovh, scaleway\n"
    printf "\n"
    cli_info "Usage: nself deploy provision <provider> [options]\n"
    printf "\n"
    cli_info "Options:"
    printf "  --name <name>       Server name (default: PROJECT_NAME-server)\n"
    printf "  --size <size>       Instance size: tiny, small, medium, large, xlarge\n"
    printf "  --region <region>   Deployment region\n"
    printf "  --token <token>     API token (overrides env var / hcloud context)\n"
    printf "  --ssh-key <name>    SSH key name to attach\n"
    printf "  --dry-run           Preview without executing\n"
    printf "  --estimate          Show cost estimate only\n"
    printf "  --sizes             List available sizes for provider\n"
    printf "  --regions           List available regions for provider\n"
    return 1
  fi

  # Source provider interface
  source "$LIB_DIR/providers/provider-interface.sh" 2>/dev/null || {
    cli_error "Provider system not available"
    return 1
  }

  # Validate provider
  if ! provider_is_supported "$provider"; then
    cli_error "Unsupported provider: $provider"
    cli_info "Run: nself deploy provision  (to see supported providers)"
    return 1
  fi

  local server_name=""
  local size="small"
  local region=""
  local api_token=""
  local ssh_key=""
  local dry_run=false
  local estimate_only=false
  local list_sizes=false
  local list_regions=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name)
        server_name="$2"
        shift 2
        ;;
      --size)
        size="$2"
        shift 2
        ;;
      --region)
        region="$2"
        shift 2
        ;;
      --token)
        api_token="$2"
        shift 2
        ;;
      --ssh-key)
        ssh_key="$2"
        shift 2
        ;;
      --dry-run)
        dry_run=true
        shift
        ;;
      --estimate)
        estimate_only=true
        shift
        ;;
      --sizes)
        list_sizes=true
        shift
        ;;
      --regions)
        list_regions=true
        shift
        ;;
      -h | --help)
        cmd_provision
        return 0
        ;;
      *)
        cli_warning "Unknown option: $1"
        shift
        ;;
    esac
  done

  # Load the provider module
  provider_load "$provider" || {
    cli_error "Failed to load provider module: $provider"
    return 1
  }

  # Handle --sizes: list available sizes and exit
  if [[ "$list_sizes" == "true" ]]; then
    show_command_header "nself deploy provision" "Available Sizes: $provider"
    printf "\n"
    provider_list_sizes "$provider"
    return 0
  fi

  # Handle --regions: list available regions and exit
  if [[ "$list_regions" == "true" ]]; then
    show_command_header "nself deploy provision" "Available Regions: $provider"
    printf "\n"
    provider_list_regions "$provider"
    return 0
  fi

  # Validate size
  if ! provider_validate_size "$size"; then
    cli_error "Invalid size: $size"
    cli_info "Valid sizes: tiny, small, medium, large, xlarge"
    cli_info "Run: nself deploy provision $provider --sizes"
    return 1
  fi

  # Resolve server name from PROJECT_NAME if not provided
  if [[ -z "$server_name" ]]; then
    local project_name="${PROJECT_NAME:-nself}"
    server_name="${project_name}-server"
  fi

  # Resolve API token: --token flag > env vars > hcloud context
  _provision_resolve_token "$provider" "$api_token"

  show_command_header "nself deploy provision" "Infrastructure Provisioning"
  printf "\n"

  # Get cost estimate
  local cost_range=""
  cost_range=$(provider_estimate_cost "$provider" "$size" 2>/dev/null || printf "N/A")

  # Show provisioning plan
  cli_section "Provisioning Plan"
  local size_specs
  size_specs=$(provider_normalize_size "$size" "all")
  local vcpu ram disk
  vcpu=$(provider_normalize_size "$size" "vcpu")
  ram=$(provider_normalize_size "$size" "ram")
  disk=$(provider_normalize_size "$size" "disk")

  printf "  Provider:    %s\n" "$provider"
  printf "  Server:      %s\n" "$server_name"
  printf "  Size:        %s (%s vCPU, %s MB RAM, %s GB disk)\n" "$size" "$vcpu" "$ram" "$disk"
  printf "  Region:      %s\n" "${region:-default}"
  if [[ -n "$ssh_key" ]]; then
    printf "  SSH Key:     %s\n" "$ssh_key"
  fi
  printf "  Est. Cost:   ~\$%s/month\n" "$cost_range"
  printf "\n"

  # Handle --estimate: show cost only
  if [[ "$estimate_only" == "true" ]]; then
    cli_section "Cost Details"
    printf "  Size '%s' on %s: ~\$%s/month\n" "$size" "$provider" "$cost_range"
    printf "\n"
    cli_info "Compare all providers: nself deploy provision <provider> --sizes"
    return 0
  fi

  # Handle --dry-run: show plan only
  if [[ "$dry_run" == "true" ]]; then
    cli_info "Dry run mode - no resources will be created"
    printf "\n"
    cli_info "Remove --dry-run to provision this server"
    return 0
  fi

  # Confirm before provisioning (this creates real infrastructure)
  printf "  This will create a real server and incur charges.\n"
  printf "  Continue? [y/N] "
  local confirm=""
  read -r confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    cli_info "Provisioning cancelled"
    return 0
  fi
  printf "\n"

  # Provision the server
  cli_info "Provisioning $provider server: $server_name ($size)..."

  local provision_args="$server_name $size"
  if [[ -n "$region" ]]; then
    provision_args="$server_name $size $region"
  fi

  if provider_provision "$provider" $provision_args; then
    printf "\n"

    # Get the server IP
    local server_ip=""
    server_ip=$(provider_get_ip "$provider" "$server_name" 2>/dev/null || printf "")

    if [[ -n "$server_ip" ]]; then
      cli_success "Server provisioned successfully"
      printf "\n"
      cli_section "Server Details"
      printf "  Name:    %s\n" "$server_name"
      printf "  IP:      %s\n" "$server_ip"
      printf "  Provider: %s\n" "$provider"
      printf "\n"

      # Save server info to local state
      _provision_save_server "$server_name" "$provider" "$server_ip" "$size" "$region"

      cli_section "Next Steps"
      printf "  1. Point your domain DNS to %s\n" "$server_ip"
      printf "  2. Initialize the server:\n"
      printf "     nself deploy server init root@%s --domain YOUR_DOMAIN\n" "$server_ip"
      printf "  3. Deploy your project:\n"
      printf "     nself deploy staging  (or production)\n"
    else
      cli_success "Server provisioned (check provider console for details)"
    fi
  else
    cli_error "Provisioning failed"
    return 1
  fi
}

# Resolve API token for a provider from --token flag, env vars, or existing config
_provision_resolve_token() {
  local provider="$1"
  local explicit_token="$2"

  # Explicit --token flag takes priority
  if [[ -n "$explicit_token" ]]; then
    case "$provider" in
      hetzner)
        export HCLOUD_TOKEN="$explicit_token"
        ;;
      digitalocean | do)
        export DIGITALOCEAN_ACCESS_TOKEN="$explicit_token"
        ;;
      linode)
        export LINODE_TOKEN="$explicit_token"
        ;;
      vultr)
        export VULTR_API_KEY="$explicit_token"
        ;;
      aws)
        # AWS uses key pairs, not single token — explicit token not applicable
        cli_warning "AWS uses key pairs. Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY instead."
        ;;
      *)
        # Generic: set as provider-specific env var
        local upper_provider
        upper_provider=$(printf '%s' "$provider" | tr '[:lower:]' '[:upper:]')
        export "${upper_provider}_API_TOKEN=$explicit_token"
        ;;
    esac
    return 0
  fi

  # Check for project-specific env vars (e.g., HETZNER_CLAWDE_TOKEN, HETZNER_NSELF_TOKEN)
  case "$provider" in
    hetzner)
      # Priority: HCLOUD_TOKEN > HETZNER_*_TOKEN vars from .env or vault
      if [[ -z "${HCLOUD_TOKEN:-}" ]]; then
        # Check for project-specific tokens loaded from env cascade
        local project_name="${PROJECT_NAME:-}"
        if [[ -n "$project_name" ]]; then
          local upper_project
          upper_project=$(printf '%s' "$project_name" | tr '[:lower:]' '[:upper:]')
          local project_token_var="HETZNER_${upper_project}_TOKEN"
          local project_token="${!project_token_var:-}"
          if [[ -n "$project_token" ]]; then
            export HCLOUD_TOKEN="$project_token"
            return 0
          fi
        fi
        # Fallback: check for generic HETZNER_TOKEN
        if [[ -n "${HETZNER_TOKEN:-}" ]]; then
          export HCLOUD_TOKEN="$HETZNER_TOKEN"
        fi
      fi
      ;;
  esac
}

# Save provisioned server info to local state
_provision_save_server() {
  local name="$1"
  local provider="$2"
  local ip="$3"
  local size="$4"
  local region="${5:-default}"

  mkdir -p "$SERVERS_DIR"

  # Append server entry (simple format — one line per server)
  local timestamp
  timestamp=$(date +%Y-%m-%dT%H:%M:%S)
  printf '{"name":"%s","provider":"%s","ip":"%s","size":"%s","region":"%s","created":"%s"}\n' \
    "$name" "$provider" "$ip" "$size" "$region" "$timestamp" >>"$SERVERS_FILE"
}

# =============================================================================
# SYNC SUBCOMMANDS
# =============================================================================

cmd_sync() {
  local action="${1:-help}"
  shift || true

  case "$action" in
    pull)
      sync_pull "$@"
      ;;
    push)
      sync_push "$@"
      ;;
    status)
      sync_status "$@"
      ;;
    full)
      sync_full "$@"
      ;;
    help | --help | -h)
      show_sync_help
      ;;
    *)
      cli_error "Unknown sync action: $action"
      show_sync_help
      return 1
      ;;
  esac
}

sync_pull() {
  local target="${1:-staging}"
  shift || true

  local dry_run=false
  local force=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      --force | -f)
        force=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  show_command_header "nself deploy sync pull" "Pull configuration from $target"
  printf "\n"

  # Check if environment exists
  local env_dir=".environments/$target"
  if [[ ! -d "$env_dir" ]]; then
    cli_error "Environment '$target' not found"
    return 1
  fi

  if [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "No server configured for $target"
    return 1
  fi

  # Load server configuration
  local host user port key_file deploy_path project_subdir
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
  key_file=$(grep '"key"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  deploy_path=$(grep '"deploy_path"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  project_subdir=$(grep '"project_subdir"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4 || true)

  user="${user:-root}"
  port="${port:-22}"
  deploy_path="${deploy_path:-/var/www/nself}"

  # Monorepo support: if project_subdir is set, resolve full project path
  if [[ -n "$project_subdir" ]]; then
    deploy_path="${deploy_path}/${project_subdir}"
  fi

  if [[ -z "$host" ]]; then
    cli_error "No host configured for $target"
    return 1
  fi

  cli_section "Sync Configuration"
  printf "  Source:      %s@%s:%s\n" "$user" "$host" "$deploy_path"
  printf "  Destination: %s\n" "$env_dir"
  printf "\n"

  # Build SSH arguments (lowercase -p for ssh)
  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "${key_file/#\~/$HOME}")
  ssh_args+=("-o" "BatchMode=yes" "-o" "StrictHostKeyChecking=accept-new" "-p" "$port")

  # Build SCP arguments (uppercase -P for scp)
  local scp_args=()
  [[ -n "$key_file" ]] && scp_args+=("-i" "${key_file/#\~/$HOME}")
  scp_args+=("-o" "BatchMode=yes" "-o" "StrictHostKeyChecking=accept-new" "-P" "$port")

  # Test connection
  cli_info "Testing connection..."
  if ! ssh "${ssh_args[@]}" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
    cli_error "Cannot connect to $host"
    return 1
  fi
  cli_success "Connected"

  # Check what files exist remotely
  cli_info "Checking remote files..."

  local remote_files
  remote_files=$(ssh "${ssh_args[@]}" "${user}@${host}" "
    cd '$deploy_path' 2>/dev/null || exit 1
    [ -f .env ] && echo '.env'
    [ -f .env.secrets ] && echo '.env.secrets'
    [ -f docker-compose.yml ] && echo 'docker-compose.yml'
  " 2>/dev/null)

  if [[ -z "$remote_files" ]]; then
    cli_warning "No configuration files found on remote server"
    return 1
  fi

  printf "\n"
  cli_section "Files to Pull"
  echo "$remote_files" | while read -r file; do
    printf "  - %s\n" "$file"
  done
  printf "\n"

  if [[ "$dry_run" == "true" ]]; then
    cli_info "Dry run - no files were synced"
    return 0
  fi

  # Confirm if not forced
  if [[ "$force" != "true" ]]; then
    printf "This will overwrite local files. Continue? [y/N]: "
    local confirm
    read -r confirm
    confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')

    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "yes" ]]; then
      cli_info "Cancelled"
      return 0
    fi
  fi

  # Pull files
  cli_info "Pulling files..."

  local pull_errors=0

  echo "$remote_files" | while read -r file; do
    printf "  Pulling %s... " "$file"

    local remote_path="$deploy_path/$file"
    local local_path="$env_dir/$file"

    if scp "${scp_args[@]}" "${user}@${host}:${remote_path}" "$local_path" 2>/dev/null; then
      printf "${CLI_GREEN}OK${CLI_RESET}\n"
    else
      printf "${CLI_RED}FAILED${CLI_RESET}\n"
      pull_errors=$((pull_errors + 1))
    fi
  done

  if [[ $pull_errors -eq 0 ]]; then
    printf "\n"
    cli_success "Sync complete: $target → local"
    printf "\n"
    cli_info "Files synced to: $env_dir"
  else
    printf "\n"
    cli_warning "Sync completed with $pull_errors error(s)"
    return 1
  fi
}

sync_push() {
  local target="${1:-staging}"
  shift || true

  local dry_run=false
  local force=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      --force | -f)
        force=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  show_command_header "nself deploy sync push" "Push configuration to $target"
  printf "\n"

  # Check if environment exists
  local env_dir=".environments/$target"
  if [[ ! -d "$env_dir" ]]; then
    cli_error "Environment '$target' not found"
    return 1
  fi

  if [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "No server configured for $target"
    return 1
  fi

  # Load server configuration
  local host user port key_file deploy_path project_subdir
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
  key_file=$(grep '"key"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  deploy_path=$(grep '"deploy_path"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  project_subdir=$(grep '"project_subdir"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4 || true)

  user="${user:-root}"
  port="${port:-22}"
  deploy_path="${deploy_path:-/var/www/nself}"

  # Monorepo support: if project_subdir is set, resolve full project path
  if [[ -n "$project_subdir" ]]; then
    deploy_path="${deploy_path}/${project_subdir}"
  fi

  if [[ -z "$host" ]]; then
    cli_error "No host configured for $target"
    return 1
  fi

  cli_section "Sync Configuration"
  printf "  Source:      %s\n" "$env_dir"
  printf "  Destination: %s@%s:%s\n" "$user" "$host" "$deploy_path"
  printf "\n"

  # Build SSH arguments (lowercase -p for ssh)
  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "${key_file/#\~/$HOME}")
  ssh_args+=("-o" "BatchMode=yes" "-o" "StrictHostKeyChecking=accept-new" "-p" "$port")

  # Build SCP arguments (uppercase -P for scp)
  local scp_args=()
  [[ -n "$key_file" ]] && scp_args+=("-i" "${key_file/#\~/$HOME}")
  scp_args+=("-o" "BatchMode=yes" "-o" "StrictHostKeyChecking=accept-new" "-P" "$port")

  # Test connection
  cli_info "Testing connection..."
  if ! ssh "${ssh_args[@]}" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
    cli_error "Cannot connect to $host"
    return 1
  fi
  cli_success "Connected"

  # Find local files to push
  local local_files=()
  [[ -f "$env_dir/.env" ]] && local_files+=(".env")
  [[ -f "$env_dir/.env.secrets" ]] && local_files+=(".env.secrets")

  if [[ ${#local_files[@]} -eq 0 ]]; then
    cli_error "No configuration files found in $env_dir"
    return 1
  fi

  printf "\n"
  cli_section "Files to Push"
  for file in "${local_files[@]}"; do
    printf "  - %s\n" "$file"
  done
  printf "\n"

  if [[ "$dry_run" == "true" ]]; then
    cli_info "Dry run - no files were synced"
    return 0
  fi

  # Warning for production
  if [[ "$target" == "prod" ]] || [[ "$target" == "production" ]]; then
    printf "${CLI_YELLOW}WARNING:${CLI_RESET} You are about to push to PRODUCTION\n"
    printf "\n"
  fi

  # Confirm if not forced
  if [[ "$force" != "true" ]]; then
    printf "This will overwrite remote files. Continue? [y/N]: "
    local confirm
    read -r confirm
    confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')

    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "yes" ]]; then
      cli_info "Cancelled"
      return 0
    fi
  fi

  # Ensure remote directory exists
  cli_info "Ensuring remote directory exists..."
  ssh "${ssh_args[@]}" "${user}@${host}" "mkdir -p '$deploy_path'" 2>/dev/null

  # Push files
  cli_info "Pushing files..."

  local push_errors=0

  for file in "${local_files[@]}"; do
    printf "  Pushing %s... " "$file"

    local local_path="$env_dir/$file"
    local remote_path="$deploy_path/$file"

    if scp "${scp_args[@]}" "$local_path" "${user}@${host}:${remote_path}" 2>/dev/null; then
      printf "${CLI_GREEN}OK${CLI_RESET}\n"

      # Set proper permissions for secrets
      if [[ "$file" == ".env.secrets" ]]; then
        ssh "${ssh_args[@]}" "${user}@${host}" "chmod 600 '$remote_path'" 2>/dev/null
      fi
    else
      printf "${CLI_RED}FAILED${CLI_RESET}\n"
      push_errors=$((push_errors + 1))
    fi
  done

  if [[ $push_errors -eq 0 ]]; then
    # Merge .env.secrets into .env on the remote so docker-compose can read all
    # variables via ${VAR} substitution. Docker Compose only auto-reads .env —
    # .env.secrets is NOT picked up automatically. Since .env was just pushed fresh
    # (config-only), appending secrets here never creates duplicates.
    if [[ -f "$env_dir/.env" ]] && [[ -f "$env_dir/.env.secrets" ]]; then
      cli_info "Merging .env.secrets into .env for docker-compose..."
      local merge_ok=false
      if ssh "${ssh_args[@]}" "${user}@${host}" "
        cd '$deploy_path' 2>/dev/null || exit 1
        printf '\n# Secrets (auto-merged from .env.secrets by nself deploy sync push)\n' >> .env
        grep -E '^[A-Za-z_][A-Za-z0-9_]*=' .env.secrets >> .env 2>/dev/null || true
        echo 'merge_ok'
      " 2>/dev/null | grep -q "merge_ok"; then
        merge_ok=true
        cli_success "Merged .env.secrets into .env"
      else
        cli_warning "Could not auto-merge .env.secrets — run manually: cat .env.secrets >> .env"
      fi
    fi
    printf "\n"
    cli_success "Sync complete: local → $target"
    printf "\n"
    cli_info "Files synced to: ${user}@${host}:${deploy_path}"
    cli_info "Run 'nself deploy $target' to apply changes"
  else
    printf "\n"
    cli_warning "Sync completed with $push_errors error(s)"
    return 1
  fi
}

sync_status() {
  show_command_header "nself deploy sync" "Synchronization Status"
  printf "\n"

  # Check if environments directory exists
  if [[ ! -d ".environments" ]]; then
    cli_info "No environments configured"
    return 0
  fi

  # Check each environment for sync status
  local env_count=0

  printf "%-15s %-10s %-25s %s\n" "ENVIRONMENT" "STATUS" "LAST SYNC" "FILES"
  printf "%-15s %-10s %-25s %s\n" "---------------" "----------" "-------------------------" "----------"

  for env_dir in .environments/*/; do
    if [[ -d "$env_dir" ]] && [[ -f "$env_dir/server.json" ]]; then
      local env_name host status last_sync files_status
      env_name=$(basename "$env_dir")

      host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)

      # Skip localhost/local
      if [[ -z "$host" ]] || [[ "$host" == "localhost" ]]; then
        continue
      fi

      env_count=$((env_count + 1))

      # Check if files exist locally
      local has_env has_secrets
      has_env="no"
      has_secrets="no"

      [[ -f "$env_dir/.env" ]] && has_env="yes"
      [[ -f "$env_dir/.env.secrets" ]] && has_secrets="yes"

      if [[ "$has_env" == "yes" ]] && [[ "$has_secrets" == "yes" ]]; then
        files_status="${CLI_GREEN}complete${CLI_RESET}"
      elif [[ "$has_env" == "yes" ]]; then
        files_status="${CLI_YELLOW}partial${CLI_RESET}"
      else
        files_status="${CLI_RED}missing${CLI_RESET}"
      fi

      # Check if sync history exists
      if [[ -f "$env_dir/.sync-history" ]]; then
        last_sync=$(tail -n1 "$env_dir/.sync-history" 2>/dev/null | cut -d'|' -f1)
        status="${CLI_GREEN}synced${CLI_RESET}"
      else
        last_sync="never"
        status="${CLI_YELLOW}not synced${CLI_RESET}"
      fi

      printf "%-15s %-10s %-25s %s\n" "$env_name" "$status" "$last_sync" "$files_status"
    fi
  done

  if [[ $env_count -eq 0 ]]; then
    printf "\n"
    cli_info "No remote environments configured"
  else
    printf "\n"
    printf "Legend:\n"
    printf "  ${CLI_GREEN}complete${CLI_RESET} - .env and .env.secrets present\n"
    printf "  ${CLI_YELLOW}partial${CLI_RESET}  - only .env present\n"
    printf "  ${CLI_RED}missing${CLI_RESET}  - configuration files missing\n"
  fi

  printf "\n"
  cli_info "Sync files between environments:"
  printf "  Pull: nself deploy sync pull <environment>\n"
  printf "  Push: nself deploy sync push <environment>\n"
}

sync_full() {
  local target="${1:-staging}"
  shift || true

  local dry_run=false
  local force=false
  local rebuild=true

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      --force | -f)
        force=true
        shift
        ;;
      --no-rebuild)
        rebuild=false
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  show_command_header "nself deploy sync full" "Full synchronization to $target"
  printf "\n"

  # Check if environment exists
  local env_dir=".environments/$target"
  if [[ ! -d "$env_dir" ]]; then
    cli_error "Environment '$target' not found"
    return 1
  fi

  if [[ ! -f "$env_dir/server.json" ]]; then
    cli_error "No server configured for $target"
    return 1
  fi

  # Load server configuration
  local host user port key_file deploy_path project_subdir
  host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
  key_file=$(grep '"key"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  deploy_path=$(grep '"deploy_path"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
  project_subdir=$(grep '"project_subdir"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4 || true)

  user="${user:-root}"
  port="${port:-22}"
  deploy_path="${deploy_path:-/var/www/nself}"

  # Monorepo support: if project_subdir is set, resolve full project path
  if [[ -n "$project_subdir" ]]; then
    deploy_path="${deploy_path}/${project_subdir}"
  fi

  if [[ -z "$host" ]]; then
    cli_error "No host configured for $target"
    return 1
  fi

  cli_section "Full Sync Plan"
  printf "  Target: %s@%s:%s\n" "$user" "$host" "$deploy_path"
  if [[ -n "$project_subdir" ]]; then
    printf "  ${CLI_DIM}(monorepo subdir: %s)${CLI_RESET}\n" "$project_subdir"
  fi
  printf "\n"
  printf "  1. Sync environment files (.env, .env.secrets)\n"
  printf "  2. Sync docker-compose.yml and configs\n"
  printf "  3. Sync nginx configuration\n"
  printf "  4. Sync custom services\n"
  if [[ "$rebuild" == "true" ]]; then
    printf "  5. Restart services on remote\n"
  fi
  printf "\n"

  if [[ "$dry_run" == "true" ]]; then
    cli_info "Dry run - no files were synced"
    return 0
  fi

  # Confirm if not forced
  if [[ "$force" != "true" ]]; then
    printf "This will perform a full sync to ${CLI_CYAN}%s${CLI_RESET}. Continue? [y/N]: " "$target"
    local confirm
    read -r confirm
    confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')

    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "yes" ]]; then
      cli_info "Cancelled"
      return 0
    fi
  fi

  # Build SSH arguments (lowercase -p for ssh)
  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "${key_file/#\~/$HOME}")
  ssh_args+=("-o" "BatchMode=yes" "-o" "StrictHostKeyChecking=accept-new" "-p" "$port")

  # Build SCP arguments (capital -P for scp)
  local scp_args=()
  [[ -n "$key_file" ]] && scp_args+=("-i" "${key_file/#\~/$HOME}")
  scp_args+=("-o" "BatchMode=yes" "-o" "StrictHostKeyChecking=accept-new" "-P" "$port")

  # Test connection
  printf "\n"
  cli_info "Testing connection..."
  if ! ssh "${ssh_args[@]}" "${user}@${host}" "echo 'ok'" 2>/dev/null | grep -q "ok"; then
    cli_error "Cannot connect to $host"
    return 1
  fi
  cli_success "Connected"

  # Step 1: Sync environment files
  printf "\n"
  cli_section "Step 1: Environment Files"

  local files_synced=0

  # CRITICAL FIX: Test SSH connection first to avoid hangs
  printf "  ${CLI_DIM}Testing SSH connection...${CLI_RESET}\n"
  if ! ssh "${ssh_args[@]}" -o ConnectTimeout=10 "${user}@${host}" "echo 'connected' >/dev/null 2>&1" 2>/dev/null; then
    printf "  ${CLI_RED}✗${CLI_RESET} SSH connection failed\n"
    cli_error "Cannot connect to ${host}"
    cli_info "Check your SSH keys and server accessibility"
    exit 1
  fi

  # Ensure deploy directory exists on remote
  ssh "${ssh_args[@]}" "${user}@${host}" "mkdir -p '$deploy_path'" 2>/dev/null || true

  if [[ -f "$env_dir/.env" ]]; then
    printf "  Syncing .env... "

    # CRITICAL FIX: Use timeout and proper error handling to prevent hangs
    local scp_result=0
    local env_error_file=$(mktemp)

    # Run scp with timeout (30 seconds max) - use scp_args with -P for port
    if command -v timeout >/dev/null 2>&1; then
      timeout 30 scp "${scp_args[@]}" "$env_dir/.env" "${user}@${host}:${deploy_path}/.env" 2>"$env_error_file" || scp_result=$?
    elif command -v gtimeout >/dev/null 2>&1; then
      gtimeout 30 scp "${scp_args[@]}" "$env_dir/.env" "${user}@${host}:${deploy_path}/.env" 2>"$env_error_file" || scp_result=$?
    else
      # No timeout available - run directly but warn
      scp "${scp_args[@]}" "$env_dir/.env" "${user}@${host}:${deploy_path}/.env" 2>"$env_error_file" || scp_result=$?
    fi

    if [[ $scp_result -eq 0 ]]; then
      printf "${CLI_GREEN}OK${CLI_RESET}\n"
      files_synced=$((files_synced + 1))
    else
      printf "${CLI_RED}FAILED${CLI_RESET}\n"
      if [[ -s "$env_error_file" ]]; then
        printf "  ${CLI_DIM}Error: %s${CLI_RESET}\n" "$(head -1 "$env_error_file")"
      fi
      if [[ $scp_result -eq 124 ]] || [[ $scp_result -eq 137 ]]; then
        printf "  ${CLI_DIM}(timed out after 30 seconds)${CLI_RESET}\n"
      fi
    fi

    rm -f "$env_error_file"
  fi

  if [[ -f "$env_dir/.env.secrets" ]]; then
    printf "  Syncing .env.secrets... "

    # CRITICAL FIX: Use timeout and proper error handling to prevent hangs
    local scp_result=0
    local secrets_error_file=$(mktemp)

    # Run scp with timeout (30 seconds max) - use scp_args with -P for port
    if command -v timeout >/dev/null 2>&1; then
      timeout 30 scp "${scp_args[@]}" "$env_dir/.env.secrets" "${user}@${host}:${deploy_path}/.env.secrets" 2>"$secrets_error_file" || scp_result=$?
    elif command -v gtimeout >/dev/null 2>&1; then
      gtimeout 30 scp "${scp_args[@]}" "$env_dir/.env.secrets" "${user}@${host}:${deploy_path}/.env.secrets" 2>"$secrets_error_file" || scp_result=$?
    else
      # No timeout available - run directly
      scp "${scp_args[@]}" "$env_dir/.env.secrets" "${user}@${host}:${deploy_path}/.env.secrets" 2>"$secrets_error_file" || scp_result=$?
    fi

    if [[ $scp_result -eq 0 ]]; then
      ssh "${ssh_args[@]}" "${user}@${host}" "chmod 600 '$deploy_path/.env.secrets'" 2>/dev/null
      printf "${CLI_GREEN}OK${CLI_RESET}\n"
      files_synced=$((files_synced + 1))
    else
      printf "${CLI_RED}FAILED${CLI_RESET}\n"
      if [[ -s "$secrets_error_file" ]]; then
        printf "  ${CLI_DIM}Error: %s${CLI_RESET}\n" "$(head -1 "$secrets_error_file")"
      fi
      if [[ $scp_result -eq 124 ]] || [[ $scp_result -eq 137 ]]; then
        printf "  ${CLI_DIM}(timed out after 30 seconds)${CLI_RESET}\n"
      fi
    fi

    rm -f "$secrets_error_file"
  fi

  # Step 1.5: Rebuild configs on remote with correct environment
  printf "\n"
  cli_section "Step 1.5: Rebuild Configuration"

  # Check if nself CLI is available on remote
  local nself_available
  nself_available=$(ssh "${ssh_args[@]}" "${user}@${host}" "command -v nself" 2>/dev/null || echo "")

  if [[ -n "$nself_available" ]]; then
    printf "  Rebuilding configs for target environment... "

    local rebuild_result
    rebuild_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "cd '$deploy_path' && nself build --force 2>&1" || echo "rebuild_failed")

    if echo "$rebuild_result" | grep -q "rebuild_failed" || echo "$rebuild_result" | grep -q "syntax error"; then
      printf "${CLI_RED}FAILED${CLI_RESET}\n"
      printf "  ${CLI_DIM}Config rebuild failed on remote server${CLI_RESET}\n"
      if echo "$rebuild_result" | grep -q "syntax error"; then
        printf "  ${CLI_DIM}Error: Bash syntax error detected${CLI_RESET}\n"
        printf "  ${CLI_DIM}This is a bug in nself - please report it${CLI_RESET}\n"
      fi
      printf "  ${CLI_YELLOW}⚠${CLI_RESET} Continuing with existing configs (may be stale)\n"
    else
      printf "${CLI_GREEN}OK${CLI_RESET}\n"
      printf "  ${CLI_DIM}Configs regenerated with remote .env${CLI_RESET}\n"
    fi
  else
    printf "  ${CLI_YELLOW}⚠${CLI_RESET} nself CLI not found on remote\n"
    printf "  ${CLI_DIM}Configs will use local BASE_DOMAIN${CLI_RESET}\n"
    printf "  ${CLI_DIM}Install nself CLI on remote for automatic config rebuild${CLI_RESET}\n"
  fi

  # Step 2: Sync docker-compose and configs
  printf "\n"
  cli_section "Step 2: Docker Configuration"

  if [[ -f "docker-compose.yml" ]]; then
    printf "  Syncing docker-compose.yml... "
    if scp "${scp_args[@]}" "docker-compose.yml" "${user}@${host}:${deploy_path}/docker-compose.yml" 2>/dev/null; then
      printf "${CLI_GREEN}OK${CLI_RESET}\n"
      files_synced=$((files_synced + 1))
    else
      printf "${CLI_RED}FAILED${CLI_RESET}\n"
    fi
  else
    printf "  ${CLI_YELLOW}WARNING:${CLI_RESET} docker-compose.yml not found locally\n"
  fi

  # Step 3: Sync nginx if exists
  printf "\n"
  cli_section "Step 3: Nginx Configuration"

  if [[ -d "nginx" ]]; then
    printf "  Syncing nginx directory... "

    # Use rsync if available, fallback to scp
    if command -v rsync >/dev/null 2>&1; then
      local rsync_ssh="ssh"
      [[ -n "$key_file" ]] && rsync_ssh="$rsync_ssh -i ${key_file/#\~/$HOME}"
      rsync_ssh="$rsync_ssh -p $port"

      if rsync -avz --delete -e "$rsync_ssh" nginx/ "${user}@${host}:${deploy_path}/nginx/" 2>/dev/null; then
        printf "${CLI_GREEN}OK${CLI_RESET}\n"
        files_synced=$((files_synced + 1))
      else
        printf "${CLI_RED}FAILED${CLI_RESET}\n"
      fi
    else
      printf "${CLI_YELLOW}SKIP${CLI_RESET} (rsync not available)\n"
    fi
  else
    printf "  ${CLI_YELLOW}SKIP:${CLI_RESET} nginx directory not found\n"
  fi

  # Step 4: Sync custom services
  printf "\n"
  cli_section "Step 4: Custom Services"

  if [[ -d "services" ]]; then
    printf "  Syncing services directory... "

    if command -v rsync >/dev/null 2>&1; then
      local rsync_ssh="ssh"
      [[ -n "$key_file" ]] && rsync_ssh="$rsync_ssh -i ${key_file/#\~/$HOME}"
      rsync_ssh="$rsync_ssh -p $port"

      if rsync -avz --delete -e "$rsync_ssh" services/ "${user}@${host}:${deploy_path}/services/" 2>/dev/null; then
        printf "${CLI_GREEN}OK${CLI_RESET}\n"
        files_synced=$((files_synced + 1))
      else
        printf "${CLI_RED}FAILED${CLI_RESET}\n"
      fi
    else
      printf "${CLI_YELLOW}SKIP${CLI_RESET} (rsync not available)\n"
    fi
  else
    printf "  ${CLI_YELLOW}SKIP:${CLI_RESET} services directory not found\n"
  fi

  # Step 4.5: Sync Hasura database files (CRITICAL for database deployment)
  printf "\n"
  cli_section "Step 4.5: Database Files (Hasura)"

  local hasura_synced=0

  # Sync migrations
  if [[ -d "hasura/migrations" ]]; then
    printf "  Syncing migrations... "
    if command -v rsync >/dev/null 2>&1; then
      local rsync_ssh="ssh"
      [[ -n "$key_file" ]] && rsync_ssh="$rsync_ssh -i ${key_file/#\~/$HOME}"
      rsync_ssh="$rsync_ssh -p $port"

      # Ensure directory exists on remote
      ssh "${ssh_args[@]}" "${user}@${host}" "mkdir -p '$deploy_path/hasura/migrations'" 2>/dev/null

      if rsync -avz -e "$rsync_ssh" hasura/migrations/ "${user}@${host}:${deploy_path}/hasura/migrations/" 2>/dev/null; then
        printf "${CLI_GREEN}OK${CLI_RESET}\n"
        hasura_synced=$((hasura_synced + 1))
      else
        printf "${CLI_RED}FAILED${CLI_RESET}\n"
      fi
    else
      printf "${CLI_YELLOW}SKIP${CLI_RESET} (rsync not available)\n"
    fi
  else
    printf "  ${CLI_DIM}SKIP:${CLI_RESET} No migrations directory\n"
  fi

  # Sync seeds
  if [[ -d "hasura/seeds" ]]; then
    printf "  Syncing seeds... "
    if command -v rsync >/dev/null 2>&1; then
      local rsync_ssh="ssh"
      [[ -n "$key_file" ]] && rsync_ssh="$rsync_ssh -i ${key_file/#\~/$HOME}"
      rsync_ssh="$rsync_ssh -p $port"

      # Ensure directory exists on remote
      ssh "${ssh_args[@]}" "${user}@${host}" "mkdir -p '$deploy_path/hasura/seeds'" 2>/dev/null

      if rsync -avz -e "$rsync_ssh" hasura/seeds/ "${user}@${host}:${deploy_path}/hasura/seeds/" 2>/dev/null; then
        printf "${CLI_GREEN}OK${CLI_RESET}\n"
        hasura_synced=$((hasura_synced + 1))
      else
        printf "${CLI_RED}FAILED${CLI_RESET}\n"
      fi
    else
      printf "${CLI_YELLOW}SKIP${CLI_RESET} (rsync not available)\n"
    fi
  else
    printf "  ${CLI_DIM}SKIP:${CLI_RESET} No seeds directory\n"
  fi

  # Sync metadata
  if [[ -d "hasura/metadata" ]]; then
    printf "  Syncing metadata... "
    if command -v rsync >/dev/null 2>&1; then
      local rsync_ssh="ssh"
      [[ -n "$key_file" ]] && rsync_ssh="$rsync_ssh -i ${key_file/#\~/$HOME}"
      rsync_ssh="$rsync_ssh -p $port"

      # Ensure directory exists on remote
      ssh "${ssh_args[@]}" "${user}@${host}" "mkdir -p '$deploy_path/hasura/metadata'" 2>/dev/null

      if rsync -avz -e "$rsync_ssh" hasura/metadata/ "${user}@${host}:${deploy_path}/hasura/metadata/" 2>/dev/null; then
        printf "${CLI_GREEN}OK${CLI_RESET}\n"
        hasura_synced=$((hasura_synced + 1))
      else
        printf "${CLI_RED}FAILED${CLI_RESET}\n"
      fi
    else
      printf "${CLI_YELLOW}SKIP${CLI_RESET} (rsync not available)\n"
    fi
  else
    printf "  ${CLI_DIM}SKIP:${CLI_RESET} No metadata directory\n"
  fi

  if [[ $hasura_synced -gt 0 ]]; then
    printf "\n  ${CLI_GREEN}✓${CLI_RESET} Synced %d Hasura directory/directories\n" "$hasura_synced"
  fi

  # Step 5: Restart services if rebuild enabled
  if [[ "$rebuild" == "true" ]]; then
    printf "\n"
    cli_section "Step 5: Restart Services"

    # Use nself stop/start instead of raw docker compose commands
    # This ensures environment variables are computed correctly (Bug #12 fix)
    printf "  Stopping services on remote... "
    local stop_result
    stop_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "cd '$deploy_path' && nself stop 2>&1" || echo "stop_failed")

    if echo "$stop_result" | grep -q "stop_failed"; then
      printf "${CLI_YELLOW}SKIP${CLI_RESET} (services may not be running)\n"
    else
      printf "${CLI_GREEN}OK${CLI_RESET}\n"
    fi

    printf "  Starting services on remote... "
    local restart_result
    restart_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "cd '$deploy_path' && nself start 2>&1" || echo "start_failed")

    if ! echo "$restart_result" | grep -q "start_failed"; then
      printf "${CLI_GREEN}OK${CLI_RESET}\n"

      # Wait for database to be ready before Step 6
      printf "  Waiting for database to be ready"

      # Configurable timeout via env var
      local max_wait=${DEPLOYMENT_DB_WAIT_TIMEOUT:-60}
      local waited=0
      local db_ready=false

      # Get project name from remote .env to match actual container names
      local project_name
      project_name=$(ssh "${ssh_args[@]}" "${user}@${host}" \
        "grep -E '^PROJECT_NAME=' '$deploy_path/.env' 2>/dev/null | cut -d'=' -f2" 2>/dev/null)
      project_name="${project_name:-nself}"

      while [[ $waited -lt $max_wait ]]; do
        # Check if postgres container is accepting connections
        if ssh "${ssh_args[@]}" "${user}@${host}" \
          "docker exec ${project_name}_postgres pg_isready -U postgres" 2>/dev/null | grep -q "accepting connections"; then
          printf " ${CLI_GREEN}OK${CLI_RESET} (${waited}s)\n"
          db_ready=true
          break
        fi
        printf "."
        sleep 2
        waited=$((waited + 2))
      done

      if [[ "$db_ready" != "true" ]]; then
        printf " ${CLI_YELLOW}TIMEOUT${CLI_RESET} (${max_wait}s)\n"
        cli_warning "Database not ready - database automation may fail"
      fi
    else
      printf "${CLI_YELLOW}PARTIAL${CLI_RESET}\n"
      cli_warning "Service restart may have issues - database automation may fail"
    fi
  fi

  # Step 6: Database Automation (CRITICAL - Apply migrations, seeds, metadata)
  # Always run if we synced hasura files OR if hasura exists on remote
  local should_deploy_db=false
  if [[ $hasura_synced -gt 0 ]]; then
    should_deploy_db=true
  else
    # Check if hasura directory exists on REMOTE server
    local remote_hasura_check
    remote_hasura_check=$(ssh "${ssh_args[@]}" "${user}@${host}" "[ -d '$deploy_path/hasura' ] && echo 'exists'" 2>/dev/null || echo "")
    if [[ "$remote_hasura_check" == "exists" ]]; then
      should_deploy_db=true
    fi
  fi

  if [[ "$should_deploy_db" == "true" ]]; then
    printf "\n"
    cli_section "Step 6: Database Deployment"

    printf "\n${CLI_DIM}Running database automation on remote server...${CLI_RESET}\n\n"

    # Check if nself CLI is available on remote
    local nself_available
    nself_available=$(ssh "${ssh_args[@]}" "${user}@${host}" "command -v nself" 2>/dev/null || echo "")

    if [[ -z "$nself_available" ]]; then
      printf "  ${CLI_YELLOW}⚠${CLI_RESET} nself CLI not found on remote server\n"
      printf "  ${CLI_DIM}Running database commands via docker exec...${CLI_RESET}\n\n"

      # Fallback: Run database commands directly via docker exec
      local db_result
      db_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "
        cd '$deploy_path' 2>/dev/null || exit 1

        # Get project name and database info from .env
        PROJECT_NAME=\$(grep -E '^PROJECT_NAME=' .env 2>/dev/null | cut -d'=' -f2)
        POSTGRES_DB=\$(grep -E '^POSTGRES_DB=' .env 2>/dev/null | cut -d'=' -f2)
        POSTGRES_USER=\$(grep -E '^POSTGRES_USER=' .env 2>/dev/null | cut -d'=' -f2)
        ENV=\$(grep -E '^ENV=' .env 2>/dev/null | cut -d'=' -f2)

        PROJECT_NAME=\${PROJECT_NAME:-nself}
        POSTGRES_DB=\${POSTGRES_DB:-\${PROJECT_NAME}_db}
        POSTGRES_USER=\${POSTGRES_USER:-postgres}
        ENV=\${ENV:-production}

        DB_CONTAINER=\"\${PROJECT_NAME}_postgres\"

        echo \"Environment: \$ENV\"
        echo \"Database: \$POSTGRES_DB\"
        echo \"\"

        # Apply migrations
        if [ -d 'hasura/migrations/default' ]; then
          echo 'Applying migrations...'
          migration_count=0
          for migration_dir in hasura/migrations/default/*/; do
            if [ -f \"\${migration_dir}up.sql\" ]; then
              migration_name=\$(basename \"\$migration_dir\")
              echo \"  → \$migration_name\"
              docker exec \"\$DB_CONTAINER\" psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -f /docker-entrypoint-initdb.d/\"\$migration_name\"/up.sql 2>/dev/null || \
              docker exec -i \"\$DB_CONTAINER\" psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" < \"\${migration_dir}up.sql\" 2>/dev/null
              migration_count=\$((migration_count + 1))
            fi
          done
          echo \"✓ Applied \$migration_count migration(s)\"
          echo \"\"
        fi

        # Apply seeds (environment-aware)
        if [ -d 'hasura/seeds/default' ]; then
          echo 'Applying seeds...'
          seed_count=0

          # Determine seed pattern based on environment
          case \"\$ENV\" in
            prod|production)
              # Production: Only 000-001
              seed_pattern='^(000|001)_'
              echo '  Strategy: Production-safe seeds only (000-001)'
              ;;
            staging|stage)
              # Staging: 000-004
              seed_pattern='^(000|001|002|003|004)_'
              echo '  Strategy: System + basic demo data (000-004)'
              ;;
            *)
              # Development: All seeds
              seed_pattern='.*'
              echo '  Strategy: All seeds (full demo)'
              ;;
          esac

          for seed_file in hasura/seeds/default/*.sql; do
            if [ -f \"\$seed_file\" ]; then
              seed_name=\$(basename \"\$seed_file\")

              # Check if seed matches environment pattern
              if echo \"\$seed_name\" | grep -qE \"\$seed_pattern\"; then
                echo \"  → \$seed_name\"
                docker exec -i \"\$DB_CONTAINER\" psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" < \"\$seed_file\" 2>/dev/null
                seed_count=\$((seed_count + 1))
              else
                echo \"  ○ \$seed_name (skipped for \$ENV)\"
              fi
            fi
          done
          echo \"✓ Applied \$seed_count seed(s)\"
          echo \"\"
        fi

        # Check tables created
        table_count=\$(docker exec \"\$DB_CONTAINER\" psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -t -c \"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'\" 2>/dev/null | tr -d ' ')
        echo \"✓ Database has \$table_count table(s)\"
        echo \"\"

        # Apply Hasura metadata (if metadata directory exists)
        if [ -d 'hasura/metadata' ]; then
          echo 'Applying Hasura metadata...'

          # Get Hasura admin secret from .env
          HASURA_ADMIN_SECRET=\$(grep -E '^HASURA_GRAPHQL_ADMIN_SECRET=' .env 2>/dev/null | cut -d'=' -f2)

          if [ -n \"\$HASURA_ADMIN_SECRET\" ]; then
            # Check if hasura CLI is available
            if command -v hasura >/dev/null 2>&1; then
              # Use Hasura CLI
              cd hasura && hasura metadata apply --endpoint http://localhost:8080 --admin-secret \"\$HASURA_ADMIN_SECRET\" 2>&1 || echo '  ⚠ Metadata apply failed (may need manual intervention)'
              cd ..
            else
              # Use direct API call as fallback
              if [ -f 'hasura/metadata/metadata.json' ]; then
                curl -s -X POST http://localhost:8080/v1/metadata \
                  -H \"x-hasura-admin-secret: \$HASURA_ADMIN_SECRET\" \
                  -H \"Content-Type: application/json\" \
                  -d @hasura/metadata/metadata.json >/dev/null 2>&1 && echo '  ✓ Metadata applied via API' || echo '  ⚠ Metadata API call failed'
              else
                echo '  ⚠ metadata.json not found (install hasura CLI for metadata apply)'
              fi
            fi
          else
            echo '  ⚠ HASURA_GRAPHQL_ADMIN_SECRET not set in .env'
          fi
        fi

        echo 'database_deployment_complete'
      " 2>&1)

      # Display results
      echo "$db_result" | while IFS= read -r line; do
        if [[ "$line" == "database_deployment_complete" ]]; then
          continue
        elif [[ "$line" =~ ^"✓" ]]; then
          printf "  ${CLI_GREEN}%s${CLI_RESET}\n" "$line"
        elif [[ "$line" =~ ^"→" ]]; then
          printf "  ${CLI_BLUE}%s${CLI_RESET}\n" "$line"
        elif [[ "$line" =~ ^"○" ]]; then
          printf "  ${CLI_DIM}%s${CLI_RESET}\n" "$line"
        elif [[ -n "$line" ]]; then
          printf "  %s\n" "$line"
        fi
      done

      if echo "$db_result" | grep -q "database_deployment_complete"; then
        printf "\n  ${CLI_GREEN}✓${CLI_RESET} Database deployment successful\n"

        # Health check: Verify database
        printf "\n  ${CLI_DIM}Running health checks...${CLI_RESET}\n"
        local health_result
        health_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "
          cd '$deploy_path' 2>/dev/null || exit 1
          PROJECT_NAME=\$(grep -E '^PROJECT_NAME=' .env 2>/dev/null | cut -d'=' -f2)
          POSTGRES_DB=\$(grep -E '^POSTGRES_DB=' .env 2>/dev/null | cut -d'=' -f2)
          POSTGRES_USER=\$(grep -E '^POSTGRES_USER=' .env 2>/dev/null | cut -d'=' -f2)
          PROJECT_NAME=\${PROJECT_NAME:-nself}
          POSTGRES_DB=\${POSTGRES_DB:-\${PROJECT_NAME}_db}
          POSTGRES_USER=\${POSTGRES_USER:-postgres}
          DB_CONTAINER=\"\${PROJECT_NAME}_postgres\"

          # Count tables
          table_count=\$(docker exec \"\$DB_CONTAINER\" psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -t -c \"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'\" 2>/dev/null | tr -d ' ')
          echo \"tables:\$table_count\"
        " 2>/dev/null)

        local table_count=$(echo "$health_result" | grep "tables:" | cut -d':' -f2)
        if [[ -n "$table_count" ]] && [[ "$table_count" -gt 0 ]]; then
          printf "    ${CLI_GREEN}✓${CLI_RESET} Database health check passed (%d tables)\n" "$table_count"
        else
          printf "    ${CLI_YELLOW}⚠${CLI_RESET} Database may be empty\n"
        fi
      else
        printf "\n  ${CLI_YELLOW}⚠${CLI_RESET} Database deployment may have issues\n"
      fi
    else
      printf "  ${CLI_GREEN}✓${CLI_RESET} nself CLI found on remote: $nself_available\n\n"

      # Use nself CLI commands directly
      local db_cli_result
      db_cli_result=$(ssh "${ssh_args[@]}" "${user}@${host}" "
        cd '$deploy_path' 2>/dev/null || exit 1

        # CRITICAL FIX (Bug #8): Load environment variables before running nself commands
        # SSH non-interactive shells don't auto-source .env files
        if [ -f .env ]; then
          set -a  # Auto-export all variables
          source .env 2>/dev/null || true
          if [ -f .env.secrets ]; then
            source .env.secrets 2>/dev/null || true
          fi
          set +a
        fi

        echo 'Running: nself db migrate up'
        nself db migrate up 2>&1 || echo 'migrate_failed'

        echo ''
        echo 'Running: nself db seed'
        nself db seed 2>&1 || echo 'seed_failed'

        echo ''
        echo 'Applying Hasura metadata...'

        # Check if nself hasura command exists
        if nself hasura >/dev/null 2>&1; then
          echo '  Using: nself hasura metadata apply'
          nself hasura metadata apply 2>&1 || echo 'metadata_failed'
        else
          # Fallback: nself hasura not available in minimal installation
          echo '  ⚠ nself hasura not available, using fallback method'

          # Get admin secret from .env
          ADMIN_SECRET=\$(grep -E '^HASURA_GRAPHQL_ADMIN_SECRET=' .env 2>/dev/null | cut -d'=' -f2)

          if [ -z \"\$ADMIN_SECRET\" ]; then
            echo '  ✗ HASURA_GRAPHQL_ADMIN_SECRET not set in .env'
            echo 'metadata_failed'
          elif command -v hasura >/dev/null 2>&1; then
            # Use hasura CLI directly
            echo '  Using: hasura CLI'
            cd hasura && hasura metadata apply --endpoint http://localhost:8080 --admin-secret \"\$ADMIN_SECRET\" 2>&1 || echo 'metadata_failed'
            cd ..
          elif [ -f 'hasura/metadata/metadata.json' ]; then
            # Use direct API call
            echo '  Using: Direct API call'
            curl -s -X POST http://localhost:8080/v1/metadata \
              -H \"x-hasura-admin-secret: \$ADMIN_SECRET\" \
              -H \"Content-Type: application/json\" \
              -d @hasura/metadata/metadata.json >/dev/null 2>&1 && echo '  ✓ Metadata applied via API' || echo 'metadata_failed'
          else
            echo '  ✗ No method available to apply metadata'
            echo 'metadata_failed'
          fi
        fi

        echo 'cli_complete'
      " 2>&1)

      echo "$db_cli_result"

      if echo "$db_cli_result" | grep -q "cli_complete" && ! echo "$db_cli_result" | grep -q "_failed"; then
        printf "\n  ${CLI_GREEN}✓${CLI_RESET} Database automation completed via nself CLI\n"
      else
        printf "\n  ${CLI_YELLOW}⚠${CLI_RESET} Some database commands may have failed\n"
      fi
    fi
  fi

  # Post-deployment verification (ensures services actually started)
  if [[ "$rebuild" == "true" ]]; then
    printf "\n"
    cli_section "Step 7: Deployment Verification"

    printf "  Checking service status... "
    local running_count
    running_count=$(ssh "${ssh_args[@]}" "${user}@${host}" "cd '$deploy_path' && docker compose ps --services --filter 'status=running' 2>/dev/null | wc -l" 2>/dev/null || echo "0")
    running_count=$(echo "$running_count" | tr -d '[:space:]')

    if [[ "${running_count:-0}" -gt 0 ]]; then
      printf "${CLI_GREEN}OK${CLI_RESET} (${running_count} services running)\n"
    else
      printf "${CLI_RED}FAILED${CLI_RESET} (no services running)\n"
      printf "\n${CLI_RED}✗ Deployment verification failed${CLI_RESET}\n"
      printf "  Services did not start. Check logs with:\n"
      printf "  ${CLI_DIM}ssh ${user}@${host} 'cd $deploy_path && nself logs'${CLI_RESET}\n"
      exit 1
    fi

    # Test API health if Hasura is enabled
    if [[ "${HASURA_ENABLED:-false}" == "true" ]] || [[ $hasura_synced -gt 0 ]]; then
      printf "  Testing API health... "
      local health_url="https://api.${BASE_DOMAIN}/healthz"
      if command -v curl >/dev/null 2>&1; then
        if curl -sf "$health_url" > /dev/null 2>&1; then
          printf "${CLI_GREEN}OK${CLI_RESET}\n"
        else
          printf "${CLI_YELLOW}SKIP${CLI_RESET} (API not responding yet)\n"
        fi
      else
        printf "${CLI_YELLOW}SKIP${CLI_RESET} (curl not available)\n"
      fi
    fi
  fi

  # Record sync history
  echo "$(date -Iseconds 2>/dev/null || date)|full|$files_synced files" >> "$env_dir/.sync-history"

  printf "\n"
  cli_success "Deployment complete and verified"
  printf "  ${CLI_DIM}Synced %d file(s), %d services running${CLI_RESET}\n" "$files_synced" "${running_count:-0}"

  if [[ $hasura_synced -gt 0 ]]; then
    printf "\n"
    cli_info "GraphQL API: https://api.$target.nself.org/v1/graphql"
  fi
}

show_sync_help() {
  cat <<EOF
nself deploy sync - Environment synchronization

Usage: nself deploy sync <action> [options]

Actions:
  pull <env>       Pull config from remote environment
  push <env>       Push config to remote environment
  status           Show sync status
  full <env>       Full sync (env + files + rebuild)

Examples:
  nself deploy sync pull staging
  nself deploy sync push staging
  nself deploy sync full staging
EOF
}

# =============================================================================
# HELPER FUNCTIONS
# =============================================================================

init_servers_config() {
  mkdir -p "$SERVERS_DIR"

  if [[ ! -f "$SERVERS_FILE" ]]; then
    printf '{"servers": []}\n' >"$SERVERS_FILE"
  fi
}

# =============================================================================
# MAIN COMMAND ROUTER
# =============================================================================

cmd_deploy() {
  local subcommand="${1:-help}"

  # Check for help
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]] || [[ "$subcommand" == "help" ]]; then
    show_deploy_help
    return 0
  fi

  # Route to appropriate handler
  case "$subcommand" in
    # Environment deployment
    staging | production | prod | dev | test)
      shift
      deploy_environment "$subcommand" "$@"
      ;;

    # Upgrade subcommands
    upgrade)
      shift
      cmd_upgrade "$@"
      ;;

    # Server subcommands
    server)
      shift
      cmd_server "$@"
      ;;

    # Provision subcommands
    provision)
      shift
      cmd_provision "$@"
      ;;

    # Sync subcommands
    sync)
      shift
      cmd_sync "$@"
      ;;

    # Legacy/compatibility subcommands
    rollback)
      shift
      source "$(dirname "${BASH_SOURCE[0]}")/backup.sh"
      cmd_backup rollback "$@"
      ;;

    init | check | status | logs | health)
      # These can be implemented or redirect to environment-specific versions
      cli_warning "Legacy command - use 'nself deploy <environment>' instead"
      show_deploy_help
      ;;

    *)
      # Try to treat as environment name
      if [[ -d ".environments/$subcommand" ]]; then
        shift
        deploy_environment "$subcommand" "$@"
      else
        cli_error "Unknown subcommand or environment: $subcommand"
        printf "\n"
        show_deploy_help
        return 1
      fi
      ;;
  esac
}

# Export for use as library
export -f cmd_deploy

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_deploy "$@"
  exit $?
fi
