#!/usr/bin/env bash
# server.sh - VPS server initialization and management
# Supports all major VPS providers (Hetzner, DigitalOcean, Vultr, Linode, AWS, etc.)

set -euo pipefail

# Get script directory
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"

# Source utilities
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/deploy/ssh.sh" 2>/dev/null || true

# ============================================================
# Server Command - VPS Server Management
# ============================================================

cmd_server() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    init)
      server_init "$@"
      ;;
    setup)
      server_setup "$@"
      ;;
    ssl)
      server_ssl "$@"
      ;;
    dns)
      server_dns "$@"
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
    secure)
      server_secure "$@"
      ;;
    help | --help | -h)
      show_server_help
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_server_help
      return 1
      ;;
  esac
}

# ============================================================
# Server Init - Initialize a new VPS server
# ============================================================

server_init() {
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

  show_command_header "nself server init" "Initialize VPS for nself deployment"

  if [[ -z "$host" ]]; then
    log_error "Host is required"
    printf "Usage: nself server init <host> [options]\n"
    printf "\nExample:\n"
    printf "  nself server init root@server.example.com --domain example.com\n"
    return 1
  fi

  # Parse user@host format
  if [[ "$host" == *"@"* ]]; then
    user="${host%%@*}"
    host="${host#*@}"
  fi

  printf "\n${COLOR_CYAN}Server Configuration${COLOR_RESET}\n"
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
    read -r confirm
    confirm=$(printf "%s" "$confirm" | tr '[:upper:]' '[:lower:]')
    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "yes" ]]; then
      log_info "Cancelled"
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
  log_info "Testing SSH connection..."
  if ! ssh "${ssh_args[@]}" "${user}@${host}" "echo 'Connection successful'" 2>/dev/null; then
    log_error "Cannot connect to $host"
    printf "Check that:\n"
    printf "  1. The server is accessible\n"
    printf "  2. SSH is enabled on port %s\n" "$port"
    printf "  3. Your SSH key is authorized\n"
    return 1
  fi
  log_success "SSH connection verified"

  # Run initialization phases
  log_info "Phase 1: System update and Docker installation"
  server_init_phase1 "$host" "$user" "$port" "$key_file"

  log_info "Phase 2: Security hardening"
  server_init_phase2 "$host" "$user" "$port" "$key_file"

  log_info "Phase 3: nself environment setup"
  server_init_phase3 "$host" "$user" "$port" "$key_file" "$env_name"

  if [[ "$skip_dns" != "true" ]]; then
    log_info "Phase 4: DNS fallback configuration"
    server_init_phase4_dns "$host" "$user" "$port" "$key_file"
  fi

  if [[ "$skip_ssl" != "true" ]] && [[ -n "$domain" ]]; then
    log_info "Phase 5: SSL certificate setup"
    server_init_phase5_ssl "$host" "$user" "$port" "$key_file" "$domain"
  fi

  printf "\n"
  printf "${COLOR_GREEN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "${COLOR_GREEN}     Server Initialization Complete!${COLOR_RESET}\n"
  printf "${COLOR_GREEN}═══════════════════════════════════════════════════${COLOR_RESET}\n"
  printf "\n"
  printf "Next steps:\n"
  printf "  1. Configure your project: ${COLOR_CYAN}nself init${COLOR_RESET}\n"
  printf "  2. Generate secrets:       ${COLOR_CYAN}nself secrets generate --env %s${COLOR_RESET}\n" "$env_name"
  printf "  3. Build for deployment:   ${COLOR_CYAN}nself build --env %s${COLOR_RESET}\n" "$env_name"
  printf "  4. Deploy to server:       ${COLOR_CYAN}nself deploy %s${COLOR_RESET}\n" "$env_name"
  printf "\n"
}

# Phase 1: System update and Docker
server_init_phase1() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"

  local init_script='#!/bin/bash
set -e

# Detect package manager
if command -v apt-get >/dev/null 2>&1; then
  PKG_MGR="apt"
elif command -v yum >/dev/null 2>&1; then
  PKG_MGR="yum"
elif command -v dnf >/dev/null 2>&1; then
  PKG_MGR="dnf"
else
  echo "Unsupported package manager"
  exit 1
fi

echo "Using package manager: $PKG_MGR"

# Update system
echo "Updating system packages..."
case $PKG_MGR in
  apt)
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get upgrade -y -qq
    apt-get install -y -qq curl wget git ca-certificates gnupg lsb-release
    ;;
  yum|dnf)
    $PKG_MGR update -y -q
    $PKG_MGR install -y -q curl wget git ca-certificates
    ;;
esac

# Install Docker if not present
if ! command -v docker >/dev/null 2>&1; then
  echo "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
fi

# Ensure Docker is running
systemctl enable docker 2>/dev/null || true
systemctl start docker 2>/dev/null || true

# Install Docker Compose v2 plugin if not present
if ! docker compose version >/dev/null 2>&1; then
  echo "Installing Docker Compose..."
  mkdir -p /usr/local/lib/docker/cli-plugins
  COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep tag_name | cut -d "\"" -f 4)
  curl -fsSL "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/lib/docker/cli-plugins/docker-compose
  chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
fi

# Verify installations
docker --version
docker compose version

echo "Phase 1 complete"
'

  ssh_exec_script "$host" "$user" "$port" "$key_file" "$init_script"
  log_success "Docker and system packages installed"
}

# Phase 2: Security hardening
server_init_phase2() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"

  local security_script='#!/bin/bash
set -e

# Detect package manager
if command -v apt-get >/dev/null 2>&1; then
  PKG_MGR="apt"
elif command -v yum >/dev/null 2>&1; then
  PKG_MGR="yum"
elif command -v dnf >/dev/null 2>&1; then
  PKG_MGR="dnf"
fi

# Install security packages
echo "Installing security packages..."
case $PKG_MGR in
  apt)
    export DEBIAN_FRONTEND=noninteractive
    apt-get install -y -qq ufw fail2ban
    ;;
  yum|dnf)
    $PKG_MGR install -y -q firewalld fail2ban
    ;;
esac

# Configure firewall
echo "Configuring firewall..."
if command -v ufw >/dev/null 2>&1; then
  # UFW (Ubuntu/Debian)
  ufw default deny incoming 2>/dev/null || true
  ufw default allow outgoing 2>/dev/null || true
  ufw allow ssh 2>/dev/null || true
  ufw allow 22/tcp 2>/dev/null || true
  ufw allow 80/tcp 2>/dev/null || true
  ufw allow 443/tcp 2>/dev/null || true
  ufw --force enable 2>/dev/null || true
elif command -v firewall-cmd >/dev/null 2>&1; then
  # firewalld (RHEL/CentOS)
  systemctl enable firewalld 2>/dev/null || true
  systemctl start firewalld 2>/dev/null || true
  firewall-cmd --permanent --add-service=ssh 2>/dev/null || true
  firewall-cmd --permanent --add-service=http 2>/dev/null || true
  firewall-cmd --permanent --add-service=https 2>/dev/null || true
  firewall-cmd --reload 2>/dev/null || true
fi

# Configure fail2ban
echo "Configuring fail2ban..."
cat > /etc/fail2ban/jail.local << "JAILEOF"
[DEFAULT]
bantime = 1h
findtime = 10m
maxretry = 5

[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
bantime = 1h
JAILEOF

# Enable fail2ban
systemctl enable fail2ban 2>/dev/null || true
systemctl restart fail2ban 2>/dev/null || true

# Harden SSH config (only if not already hardened)
if ! grep -q "^PermitRootLogin prohibit-password" /etc/ssh/sshd_config 2>/dev/null; then
  echo "Hardening SSH configuration..."
  sed -i "s/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/" /etc/ssh/sshd_config 2>/dev/null || true
  sed -i "s/^#*PasswordAuthentication.*/PasswordAuthentication no/" /etc/ssh/sshd_config 2>/dev/null || true
  systemctl reload sshd 2>/dev/null || systemctl reload ssh 2>/dev/null || true
fi

echo "Phase 2 complete"
'

  ssh_exec_script "$host" "$user" "$port" "$key_file" "$security_script"
  log_success "Security hardening applied"
}

# Phase 3: nself environment setup
server_init_phase3() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"
  local env_name="$5"

  local setup_script="#!/bin/bash
set -e

# Create nself directories
echo 'Creating nself directories...'
mkdir -p /var/www/nself
mkdir -p /var/www/nself/backups
mkdir -p /var/www/nself/ssl
mkdir -p /var/www/nself/.secrets

# Set permissions
chmod 700 /var/www/nself/.secrets

# Create placeholder .env for environment
cat > /var/www/nself/.env << 'ENVEOF'
# nself Production Configuration
# Generated by nself server init

ENV=$env_name
PROJECT_NAME=nself

# Domain configuration - set your domain
# BASE_DOMAIN=example.com

# Secrets will be in .secrets/
# Use 'nself secrets generate' to create secure secrets
ENVEOF

echo 'Phase 3 complete'
"

  ssh_exec_script "$host" "$user" "$port" "$key_file" "$setup_script"
  log_success "nself environment created at /var/www/nself"
}

# Phase 4: DNS fallback configuration
server_init_phase4_dns() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"

  local dns_script='#!/bin/bash
set -e

echo "Configuring DNS fallback..."

# Check if systemd-resolved is used
if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
  echo "Configuring systemd-resolved with fallback DNS..."

  # Create resolved.conf.d directory
  mkdir -p /etc/systemd/resolved.conf.d/

  # Configure fallback DNS
  cat > /etc/systemd/resolved.conf.d/nself-dns.conf << "DNSEOF"
# nself DNS configuration with reliable fallbacks
[Resolve]
# Primary: Use provider DNS (auto-detected)
# Fallback to Cloudflare and Google
FallbackDNS=1.1.1.1 8.8.8.8 1.0.0.1 8.8.4.4

# Enable DNS over TLS for privacy
DNSOverTLS=opportunistic

# Cache settings
Cache=yes
CacheFromLocalhost=yes
DNSEOF

  systemctl restart systemd-resolved

elif [ -f /etc/resolv.conf ]; then
  echo "Configuring /etc/resolv.conf with fallback DNS..."

  # Backup existing resolv.conf
  cp /etc/resolv.conf /etc/resolv.conf.backup

  # Add fallback DNS servers if not present
  if ! grep -q "8.8.8.8" /etc/resolv.conf 2>/dev/null; then
    echo "nameserver 1.1.1.1" >> /etc/resolv.conf
    echo "nameserver 8.8.8.8" >> /etc/resolv.conf
  fi
fi

# Verify DNS is working
echo "Testing DNS resolution..."
if host google.com >/dev/null 2>&1 || nslookup google.com >/dev/null 2>&1 || dig google.com +short >/dev/null 2>&1; then
  echo "DNS resolution working"
else
  echo "Warning: DNS resolution test failed"
fi

echo "Phase 4 complete"
'

  ssh_exec_script "$host" "$user" "$port" "$key_file" "$dns_script"
  log_success "DNS fallback configured"
}

# Phase 5: SSL certificate setup
server_init_phase5_ssl() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"
  local domain="$5"

  local ssl_script="#!/bin/bash
set -e

DOMAIN='$domain'

echo \"Setting up SSL for \$DOMAIN...\"

# Install certbot if not present
if ! command -v certbot >/dev/null 2>&1; then
  echo 'Installing certbot...'
  if command -v apt-get >/dev/null 2>&1; then
    apt-get install -y -qq certbot
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q certbot
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q certbot
  fi
fi

# Create certificate directory
mkdir -p /var/www/nself/ssl

# Check if we can get a certificate (DNS must be pointing to this server)
echo 'Checking if domain points to this server...'
SERVER_IP=\$(curl -s ifconfig.me 2>/dev/null || curl -s icanhazip.com 2>/dev/null)
DOMAIN_IP=\$(dig +short \$DOMAIN 2>/dev/null | head -1)

if [ \"\$SERVER_IP\" = \"\$DOMAIN_IP\" ]; then
  echo 'Domain points to this server, attempting to get Let'\''s Encrypt certificate...'

  # Stop nginx/apache if running (to free port 80)
  systemctl stop nginx 2>/dev/null || true
  docker stop \$(docker ps -q --filter 'expose=80') 2>/dev/null || true

  # Get certificate
  certbot certonly --standalone \\
    -d \$DOMAIN \\
    -d *.\$DOMAIN \\
    --non-interactive \\
    --agree-tos \\
    --email admin@\$DOMAIN \\
    --cert-name nself || {
    echo 'Certbot failed (this is normal if wildcard without DNS challenge)'
    echo 'Trying single domain certificate...'
    certbot certonly --standalone \\
      -d \$DOMAIN \\
      --non-interactive \\
      --agree-tos \\
      --email admin@\$DOMAIN \\
      --cert-name nself || true
  }

  # Copy certificates to nself directory
  if [ -d /etc/letsencrypt/live/nself ]; then
    cp /etc/letsencrypt/live/nself/fullchain.pem /var/www/nself/ssl/
    cp /etc/letsencrypt/live/nself/privkey.pem /var/www/nself/ssl/
    chmod 600 /var/www/nself/ssl/privkey.pem
    echo 'Let'\''s Encrypt certificate installed'
  fi

  # Setup auto-renewal cron
  echo '0 3 * * * certbot renew --quiet --deploy-hook \"cp /etc/letsencrypt/live/nself/*.pem /var/www/nself/ssl/ && docker compose -f /var/www/nself/docker-compose.yml restart nginx 2>/dev/null || true\"' | crontab -

else
  echo 'Domain does not point to this server yet'
  echo 'Server IP: '\$SERVER_IP
  echo 'Domain IP: '\$DOMAIN_IP
  echo ''
  echo 'To enable Let'\''s Encrypt:'
  echo '  1. Point your domain to '\$SERVER_IP
  echo '  2. Run: nself ssl setup --domain '\$DOMAIN

  # Generate self-signed certificate as fallback
  echo ''
  echo 'Generating self-signed certificate as fallback...'
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 \\
    -keyout /var/www/nself/ssl/privkey.pem \\
    -out /var/www/nself/ssl/fullchain.pem \\
    -subj '/CN='\$DOMAIN'/O=nself/C=US'
  chmod 600 /var/www/nself/ssl/privkey.pem
fi

echo 'Phase 5 complete'
"

  ssh_exec_script "$host" "$user" "$port" "$key_file" "$ssl_script"
  log_success "SSL configuration complete"
}

# Helper: Execute script on remote server
ssh_exec_script() {
  local host="$1"
  local user="$2"
  local port="$3"
  local key_file="$4"
  local script="$5"

  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "$key_file")
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new")
  ssh_args+=("-p" "$port")

  ssh "${ssh_args[@]}" "${user}@${host}" "bash -s" <<<"$script"
}

# ============================================================
# Server Setup - Configure individual components
# ============================================================

server_setup() {
  local component="${1:-}"
  shift || true

  case "$component" in
    docker)
      server_setup_docker "$@"
      ;;
    firewall)
      server_setup_firewall "$@"
      ;;
    security)
      server_setup_security "$@"
      ;;
    "")
      log_error "Component required"
      printf "Available components: docker, firewall, security\n"
      return 1
      ;;
    *)
      log_error "Unknown component: $component"
      return 1
      ;;
  esac
}

# ============================================================
# Server Check - Verify server readiness
# ============================================================

server_check() {
  local host=""
  local user="root"
  local port="22"
  local key_file=""

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
      *)
        if [[ -z "$host" ]]; then
          host="$1"
        fi
        shift
        ;;
    esac
  done

  show_command_header "nself server check" "Verify server readiness for deployment"

  if [[ -z "$host" ]]; then
    log_error "Host is required"
    printf "Usage: nself server check <host>\n"
    return 1
  fi

  # Parse user@host format
  if [[ "$host" == *"@"* ]]; then
    user="${host%%@*}"
    host="${host#*@}"
  fi

  printf "\nChecking server: %s@%s\n\n" "$user" "$host"

  local checks_passed=0
  local checks_failed=0

  # Build SSH args
  local ssh_args=()
  [[ -n "$key_file" ]] && ssh_args+=("-i" "$key_file")
  ssh_args+=("-o" "StrictHostKeyChecking=accept-new")
  ssh_args+=("-o" "ConnectTimeout=10")
  ssh_args+=("-p" "$port")

  # Check SSH connection
  printf "  SSH Connection:    "
  if ssh "${ssh_args[@]}" "${user}@${host}" "true" 2>/dev/null; then
    printf "${COLOR_GREEN}OK${COLOR_RESET}\n"
    checks_passed=$((checks_passed + 1))
  else
    printf "${COLOR_RED}FAILED${COLOR_RESET}\n"
    checks_failed=$((checks_failed + 1))
    log_error "Cannot connect to server"
    return 1
  fi

  # Check Docker
  printf "  Docker:            "
  local docker_version=$(ssh "${ssh_args[@]}" "${user}@${host}" "docker --version 2>/dev/null | head -1" 2>/dev/null)
  if [[ -n "$docker_version" ]]; then
    printf "${COLOR_GREEN}OK${COLOR_RESET} (%s)\n" "$docker_version"
    checks_passed=$((checks_passed + 1))
  else
    printf "${COLOR_RED}NOT INSTALLED${COLOR_RESET}\n"
    checks_failed=$((checks_failed + 1))
  fi

  # Check Docker Compose
  printf "  Docker Compose:    "
  local compose_version=$(ssh "${ssh_args[@]}" "${user}@${host}" "docker compose version --short 2>/dev/null" 2>/dev/null)
  if [[ -n "$compose_version" ]]; then
    printf "${COLOR_GREEN}OK${COLOR_RESET} (v%s)\n" "$compose_version"
    checks_passed=$((checks_passed + 1))
  else
    printf "${COLOR_RED}NOT INSTALLED${COLOR_RESET}\n"
    checks_failed=$((checks_failed + 1))
  fi

  # Check firewall
  printf "  Firewall:          "
  local firewall_status=$(ssh "${ssh_args[@]}" "${user}@${host}" "
    if command -v ufw >/dev/null 2>&1; then
      ufw status 2>/dev/null | head -1
    elif command -v firewall-cmd >/dev/null 2>&1; then
      firewall-cmd --state 2>/dev/null
    else
      echo 'not installed'
    fi
  " 2>/dev/null)
  if [[ "$firewall_status" == *"active"* ]] || [[ "$firewall_status" == *"running"* ]]; then
    printf "${COLOR_GREEN}ACTIVE${COLOR_RESET}\n"
    checks_passed=$((checks_passed + 1))
  elif [[ "$firewall_status" == *"inactive"* ]] || [[ "$firewall_status" == *"not running"* ]]; then
    printf "${COLOR_YELLOW}INACTIVE${COLOR_RESET}\n"
    checks_failed=$((checks_failed + 1))
  else
    printf "${COLOR_YELLOW}NOT INSTALLED${COLOR_RESET}\n"
  fi

  # Check fail2ban
  printf "  Fail2ban:          "
  local fail2ban_status=$(ssh "${ssh_args[@]}" "${user}@${host}" "systemctl is-active fail2ban 2>/dev/null || echo 'not installed'" 2>/dev/null)
  if [[ "$fail2ban_status" == "active" ]]; then
    printf "${COLOR_GREEN}ACTIVE${COLOR_RESET}\n"
    checks_passed=$((checks_passed + 1))
  else
    printf "${COLOR_YELLOW}%s${COLOR_RESET}\n" "$fail2ban_status"
  fi

  # Check SSL certificates
  printf "  SSL Certificates:  "
  local ssl_status=$(ssh "${ssh_args[@]}" "${user}@${host}" "
    if [ -f /var/www/nself/ssl/fullchain.pem ]; then
      openssl x509 -in /var/www/nself/ssl/fullchain.pem -noout -dates 2>/dev/null | grep notAfter | cut -d= -f2
    else
      echo 'not found'
    fi
  " 2>/dev/null)
  if [[ "$ssl_status" != "not found" ]] && [[ -n "$ssl_status" ]]; then
    printf "${COLOR_GREEN}OK${COLOR_RESET} (expires: %s)\n" "$ssl_status"
    checks_passed=$((checks_passed + 1))
  else
    printf "${COLOR_YELLOW}NOT CONFIGURED${COLOR_RESET}\n"
  fi

  # Check disk space
  printf "  Disk Space:        "
  local disk_space=$(ssh "${ssh_args[@]}" "${user}@${host}" "df -h / | tail -1 | awk '{print \$4}'" 2>/dev/null)
  if [[ -n "$disk_space" ]]; then
    local space_gb=$(echo "$disk_space" | sed 's/G.*//')
    if [[ "$space_gb" -gt 5 ]]; then
      printf "${COLOR_GREEN}%s${COLOR_RESET} available\n" "$disk_space"
      checks_passed=$((checks_passed + 1))
    else
      printf "${COLOR_YELLOW}%s${COLOR_RESET} (low)\n" "$disk_space"
    fi
  fi

  # Check memory
  printf "  Memory:            "
  local memory=$(ssh "${ssh_args[@]}" "${user}@${host}" "free -h | grep Mem | awk '{print \$2}'" 2>/dev/null)
  if [[ -n "$memory" ]]; then
    printf "%s total\n" "$memory"
    checks_passed=$((checks_passed + 1))
  fi

  # Summary
  printf "\n"
  printf "Checks passed: %d\n" "$checks_passed"
  printf "Checks failed: %d\n" "$checks_failed"

  if [[ $checks_failed -gt 0 ]]; then
    printf "\n"
    log_warning "Some checks failed. Run 'nself server init' to fix issues."
    return 1
  else
    printf "\n"
    log_success "Server is ready for deployment"
    return 0
  fi
}

# ============================================================
# Server Status - Quick status check for all environments
# ============================================================

server_status() {
  local env_name="${1:-}"

  show_command_header "nself server status" "Check server connectivity"

  # If specific environment requested
  if [[ -n "$env_name" ]]; then
    check_single_server "$env_name"
    return $?
  fi

  # Check all configured environments
  printf "\n${COLOR_CYAN}Environment Status${COLOR_RESET}\n"
  printf "════════════════════════════════════════════════════════════════\n"
  printf "%-12s %-20s %-12s %s\n" "Environment" "Server" "Status" "Last Deploy"
  printf "────────────────────────────────────────────────────────────────\n"

  local found_envs=0
  for env_dir in .environments/*/; do
    [[ ! -d "$env_dir" ]] && continue
    found_envs=$((found_envs + 1))

    local env=$(basename "$env_dir")
    local host=""
    local status="${COLOR_YELLOW}Unknown${COLOR_RESET}"
    local last_deploy="-"

    # Read server.json
    if [[ -f "$env_dir/server.json" ]]; then
      host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
    fi

    if [[ -z "$host" ]]; then
      host="not configured"
      status="${COLOR_DIM}Not configured${COLOR_RESET}"
    else
      # Quick connectivity check (2 second timeout)
      if nc -z -w 2 "$host" 22 2>/dev/null; then
        status="${COLOR_GREEN}✓ Reachable${COLOR_RESET}"
      else
        status="${COLOR_RED}✗ Unreachable${COLOR_RESET}"
      fi
    fi

    printf "%-12s %-20s %b %s\n" "$env" "$host" "$status" "$last_deploy"
  done

  if [[ $found_envs -eq 0 ]]; then
    printf "  ${COLOR_DIM}No environments configured${COLOR_RESET}\n"
    printf "\n"
    printf "Create an environment with:\n"
    printf "  ${COLOR_CYAN}nself env create prod prod${COLOR_RESET}\n"
  fi

  printf "\n"
}

check_single_server() {
  local env_name="$1"
  local env_dir=".environments/$env_name"

  if [[ ! -d "$env_dir" ]]; then
    log_error "Environment '$env_name' not found"
    return 1
  fi

  local host=""
  local port="22"
  local user="root"

  if [[ -f "$env_dir/server.json" ]]; then
    host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
    port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
    user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
    port="${port:-22}"
    user="${user:-root}"
  fi

  if [[ -z "$host" ]]; then
    log_error "No host configured for '$env_name'"
    return 1
  fi

  printf "Environment: %s\n" "$env_name"
  printf "Server:      %s@%s:%s\n\n" "$user" "$host" "$port"

  # Run connectivity checks
  printf "Connectivity:\n"

  printf "  Port 22 (SSH):    "
  if nc -z -w 3 "$host" 22 2>/dev/null; then
    printf "${COLOR_GREEN}Open${COLOR_RESET}\n"
  else
    printf "${COLOR_RED}Closed/Timeout${COLOR_RESET}\n"
  fi

  printf "  Port 80 (HTTP):   "
  if nc -z -w 3 "$host" 80 2>/dev/null; then
    printf "${COLOR_GREEN}Open${COLOR_RESET}\n"
  else
    printf "${COLOR_RED}Closed/Timeout${COLOR_RESET}\n"
  fi

  printf "  Port 443 (HTTPS): "
  if nc -z -w 3 "$host" 443 2>/dev/null; then
    printf "${COLOR_GREEN}Open${COLOR_RESET}\n"
  else
    printf "${COLOR_RED}Closed/Timeout${COLOR_RESET}\n"
  fi

  printf "\n"
}

# ============================================================
# Server Diagnose - Full connectivity diagnostics
# ============================================================

server_diagnose() {
  local env_name="${1:-prod}"
  local env_dir=".environments/$env_name"

  show_command_header "nself server diagnose" "Full server diagnostics"

  # Check if environment exists
  if [[ ! -d "$env_dir" ]] && [[ "$env_name" != *"@"* ]]; then
    # Maybe they passed host directly
    if [[ "$env_name" == *"."* ]] || [[ "$env_name" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      local host="$env_name"
      local port="22"
    else
      log_error "Environment '$env_name' not found"
      printf "\nUsage:\n"
      printf "  nself server diagnose prod              # Diagnose prod environment\n"
      printf "  nself server diagnose 5.75.235.42       # Diagnose by IP directly\n"
      return 1
    fi
  else
    # Read from server.json
    local host=""
    local port="22"
    local user="root"

    if [[ -f "$env_dir/server.json" ]]; then
      host=$(grep '"host"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
      port=$(grep '"port"' "$env_dir/server.json" 2>/dev/null | sed 's/[^0-9]//g')
      user=$(grep '"user"' "$env_dir/server.json" 2>/dev/null | cut -d'"' -f4)
      port="${port:-22}"
      user="${user:-root}"
    fi
  fi

  if [[ -z "$host" ]]; then
    log_error "No host to diagnose"
    return 1
  fi

  printf "Diagnosing: ${COLOR_CYAN}%s${COLOR_RESET}\n\n" "$host"

  # 1. DNS Resolution
  printf "${COLOR_BOLD}1. DNS Resolution${COLOR_RESET}\n"
  if [[ "$host" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf "   ${COLOR_DIM}Skipped (IP address provided)${COLOR_RESET}\n"
  else
    local resolved_ip
    resolved_ip=$(dig +short "$host" 2>/dev/null | head -1)
    if [[ -n "$resolved_ip" ]]; then
      printf "   ${COLOR_GREEN}✓${COLOR_RESET} Resolves to: %s\n" "$resolved_ip"
      host="$resolved_ip" # Use IP for remaining tests
    else
      printf "   ${COLOR_RED}✗${COLOR_RESET} DNS resolution failed\n"
      printf "   ${COLOR_DIM}Check your domain's DNS settings${COLOR_RESET}\n"
    fi
  fi
  printf "\n"

  # 2. ICMP Ping
  printf "${COLOR_BOLD}2. ICMP Ping${COLOR_RESET}\n"
  if ping -c 1 -W 3 "$host" >/dev/null 2>&1; then
    printf "   ${COLOR_GREEN}✓${COLOR_RESET} Server responds to ping\n"
  else
    printf "   ${COLOR_YELLOW}!${COLOR_RESET} No ping response (may be blocked by firewall)\n"
  fi
  printf "\n"

  # 3. Port Scanning
  printf "${COLOR_BOLD}3. Port Connectivity${COLOR_RESET}\n"
  local ports=(22 80 443 5432 6379 8080 9000)
  local port_names=("SSH" "HTTP" "HTTPS" "PostgreSQL" "Redis" "Hasura" "MinIO")

  for i in "${!ports[@]}"; do
    local p="${ports[$i]}"
    local name="${port_names[$i]}"
    printf "   Port %-5s (%-10s): " "$p" "$name"

    if nc -z -w 3 "$host" "$p" 2>/dev/null; then
      printf "${COLOR_GREEN}Open${COLOR_RESET}\n"
    else
      printf "${COLOR_RED}Closed/Timeout${COLOR_RESET}\n"
    fi
  done
  printf "\n"

  # 4. SSH Connection Test
  printf "${COLOR_BOLD}4. SSH Connection${COLOR_RESET}\n"
  if nc -z -w 3 "$host" "${port:-22}" 2>/dev/null; then
    printf "   ${COLOR_GREEN}✓${COLOR_RESET} SSH port is open\n"

    # Try SSH banner grab
    local ssh_banner
    ssh_banner=$(echo "" | nc -w 3 "$host" "${port:-22}" 2>/dev/null | head -1)
    if [[ -n "$ssh_banner" ]]; then
      printf "   ${COLOR_DIM}Banner: %s${COLOR_RESET}\n" "$ssh_banner"
    fi
  else
    printf "   ${COLOR_RED}✗${COLOR_RESET} SSH port not reachable\n"
  fi
  printf "\n"

  # 5. Recommendations
  printf "${COLOR_BOLD}5. Recommendations${COLOR_RESET}\n"
  local all_ports_closed=true
  for p in "${ports[@]}"; do
    if nc -z -w 1 "$host" "$p" 2>/dev/null; then
      all_ports_closed=false
      break
    fi
  done

  if [[ "$all_ports_closed" == "true" ]]; then
    printf "   ${COLOR_RED}All ports appear closed${COLOR_RESET}\n\n"
    printf "   Possible causes:\n"
    printf "   • Server is powered off\n"
    printf "   • Server crashed or is unresponsive\n"
    printf "   • Firewall is blocking all incoming traffic\n"
    printf "   • Wrong IP address\n"
    printf "\n"
    printf "   ${COLOR_YELLOW}Recommended actions:${COLOR_RESET}\n"
    printf "   1. Log into your VPS provider console (Hetzner, DigitalOcean, etc.)\n"
    printf "   2. Check if the server is running\n"
    printf "   3. Check firewall rules - ensure SSH (22), HTTP (80), HTTPS (443) are allowed\n"
    printf "   4. Try accessing via console/VNC to diagnose\n"
    printf "   5. Verify the IP address is correct: %s\n" "$host"
  else
    printf "   ${COLOR_GREEN}Server is partially reachable${COLOR_RESET}\n"
    printf "   Run ${COLOR_CYAN}nself server check %s${COLOR_RESET} for detailed status\n" "$env_name"
  fi

  printf "\n"
}

# ============================================================
# Server Secure - Additional security hardening
# ============================================================

server_secure() {
  log_info "Additional security hardening options:"
  printf "\n"
  printf "  1. Change SSH port:      nself server setup security --ssh-port 2222\n"
  printf "  2. Disable root login:   nself server setup security --no-root\n"
  printf "  3. Setup SSH keys:       nself server setup security --add-key <pubkey>\n"
  printf "\n"
}

# ============================================================
# Help
# ============================================================

show_server_help() {
  printf "\033[0;33m⚠ DEPRECATED:\033[0m 'nself server' has moved to 'nself deploy server'\n\n" >&2
  printf "Usage: nself server <command> [options]\n"
  printf "\n"
  printf "VPS server management for nself deployments\n"
  printf "(DEPRECATED: use 'nself deploy server' instead)\n"
  printf "\n"
  printf "Commands:\n"
  printf "  init      Initialize a new VPS server for nself\n"
  printf "  check     Check server readiness (detailed)\n"
  printf "  status    Quick status of all environments\n"
  printf "  diagnose  Full connectivity diagnostics\n"
  printf "  setup     Configure individual server components\n"
  printf "  ssl       Manage SSL certificates on server\n"
  printf "  dns       Configure DNS settings\n"
  printf "  secure    Additional security hardening\n"
  printf "\n"
  printf "Examples:\n"
  printf "  # Initialize a new server\n"
  printf "  nself server init root@server.example.com --domain example.com\n"
  printf "\n"
  printf "  # Check server readiness\n"
  printf "  nself server check root@server.example.com\n"
  printf "\n"
  printf "  # Quick status of all environments\n"
  printf "  nself server status\n"
  printf "\n"
  printf "  # Diagnose unreachable server\n"
  printf "  nself server diagnose prod\n"
  printf "  nself server diagnose 5.75.235.42\n"
  printf "\n"
  printf "  # Initialize with custom port and key\n"
  printf "  nself server init root@server.example.com --port 2222 --key ~/.ssh/deploy_key\n"
  printf "\n"
  printf "Supported Providers:\n"
  printf "  • Hetzner Cloud\n"
  printf "  • DigitalOcean\n"
  printf "  • Vultr\n"
  printf "  • Linode\n"
  printf "  • AWS EC2\n"
  printf "  • Google Cloud\n"
  printf "  • Azure\n"
  printf "  • Any VPS with SSH access\n"
  printf "\n"
  printf "Options:\n"
  printf "  --host, -h     Server hostname or IP\n"
  printf "  --user, -u     SSH user (default: root)\n"
  printf "  --port, -p     SSH port (default: 22)\n"
  printf "  --key, -k      SSH private key file\n"
  printf "  --domain, -d   Domain name for SSL setup\n"
  printf "  --env, -e      Environment name (default: prod)\n"
  printf "  --skip-ssl     Skip SSL certificate setup\n"
  printf "  --skip-dns     Skip DNS fallback configuration\n"
  printf "  --yes, -y      Skip confirmation prompts\n"
}

# Export function
export -f cmd_server

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_server "$@"
  exit $?
fi
