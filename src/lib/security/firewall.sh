#!/usr/bin/env bash

# firewall.sh - Firewall configuration and management
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
SECURITY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_ROOT="$(dirname "$SECURITY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true

# Firewall type detection
FIREWALL_TYPE=""

# Detect available firewall
firewall::detect() {
  if command -v ufw >/dev/null 2>&1; then
    FIREWALL_TYPE="ufw"
  elif command -v firewall-cmd >/dev/null 2>&1; then
    FIREWALL_TYPE="firewalld"
  elif command -v iptables >/dev/null 2>&1; then
    FIREWALL_TYPE="iptables"
  else
    FIREWALL_TYPE="none"
  fi

  printf "%s" "$FIREWALL_TYPE"
}

# Check firewall status
firewall::status() {
  local fw_type
  fw_type=$(firewall::detect)

  printf "Firewall Status\n"
  printf "═══════════════════════════════════════\n\n"
  printf "  Type: %s\n\n" "$fw_type"

  case "$fw_type" in
    ufw)
      firewall::ufw_status
      ;;
    firewalld)
      firewall::firewalld_status
      ;;
    iptables)
      firewall::iptables_status
      ;;
    none)
      log_warning "No firewall detected"
      printf "\n  Install a firewall with:\n"
      printf "    Ubuntu/Debian: sudo apt install ufw\n"
      printf "    RHEL/CentOS:   sudo yum install firewalld\n"
      ;;
  esac
}

# UFW status
firewall::ufw_status() {
  local status
  status=$(sudo ufw status 2>/dev/null || echo "inactive")

  if echo "$status" | grep -q "Status: active"; then
    printf "  ${COLOR_GREEN}✓ UFW is active${COLOR_RESET}\n\n"
    printf "  Current rules:\n"
    sudo ufw status numbered 2>/dev/null | tail -n +4 | while IFS= read -r line; do
      printf "    %s\n" "$line"
    done
  else
    printf "  ${COLOR_YELLOW}⚠ UFW is inactive${COLOR_RESET}\n"
    printf "  Enable with: sudo ufw enable\n"
  fi
}

# Firewalld status
firewall::firewalld_status() {
  if systemctl is-active firewalld >/dev/null 2>&1; then
    printf "  ${COLOR_GREEN}✓ Firewalld is active${COLOR_RESET}\n\n"
    printf "  Default zone: %s\n" "$(firewall-cmd --get-default-zone 2>/dev/null)"
    printf "\n  Open ports:\n"
    firewall-cmd --list-ports 2>/dev/null | tr ' ' '\n' | while IFS= read -r port; do
      [[ -n "$port" ]] && printf "    %s\n" "$port"
    done
  else
    printf "  ${COLOR_YELLOW}⚠ Firewalld is inactive${COLOR_RESET}\n"
    printf "  Enable with: sudo systemctl enable --now firewalld\n"
  fi
}

# Iptables status
firewall::iptables_status() {
  printf "  iptables rules summary:\n"
  printf "    INPUT:   %s rules\n" "$(sudo iptables -L INPUT -n 2>/dev/null | tail -n +3 | wc -l)"
  printf "    OUTPUT:  %s rules\n" "$(sudo iptables -L OUTPUT -n 2>/dev/null | tail -n +3 | wc -l)"
  printf "    FORWARD: %s rules\n" "$(sudo iptables -L FORWARD -n 2>/dev/null | tail -n +3 | wc -l)"
}

# Configure firewall for nself (production recommended rules)
firewall::configure() {
  local dry_run="${1:-false}"
  local ssh_port="${2:-22}"
  local http_port="${3:-80}"
  local https_port="${4:-443}"

  local fw_type
  fw_type=$(firewall::detect)

  printf "Configuring firewall for production...\n\n"

  if [[ "$dry_run" == "true" ]]; then
    printf "${COLOR_YELLOW}DRY RUN - Commands that would be executed:${COLOR_RESET}\n\n"
  fi

  case "$fw_type" in
    ufw)
      firewall::configure_ufw "$dry_run" "$ssh_port" "$http_port" "$https_port"
      ;;
    firewalld)
      firewall::configure_firewalld "$dry_run" "$ssh_port" "$http_port" "$https_port"
      ;;
    iptables)
      firewall::configure_iptables "$dry_run" "$ssh_port" "$http_port" "$https_port"
      ;;
    none)
      log_error "No firewall available to configure"
      return 1
      ;;
  esac
}

# Configure UFW
firewall::configure_ufw() {
  local dry_run="${1:-false}"
  local ssh_port="${2:-22}"
  local http_port="${3:-80}"
  local https_port="${4:-443}"

  local commands=""

  # Default deny incoming, allow outgoing
  commands="$commands
ufw default deny incoming
ufw default allow outgoing"

  # Allow SSH (with rate limiting)
  commands="$commands
ufw limit ${ssh_port}/tcp comment 'SSH with rate limiting'"

  # Allow HTTP and HTTPS
  commands="$commands
ufw allow ${http_port}/tcp comment 'HTTP'
ufw allow ${https_port}/tcp comment 'HTTPS'"

  # Enable firewall
  commands="$commands
ufw --force enable"

  if [[ "$dry_run" == "true" ]]; then
    printf "%s\n" "$commands" | while IFS= read -r cmd; do
      [[ -n "$cmd" ]] && printf "  sudo %s\n" "$cmd"
    done
  else
    printf "%s\n" "$commands" | while IFS= read -r cmd; do
      if [[ -n "$cmd" ]]; then
        printf "  Running: %s\n" "$cmd"
        sudo $cmd
      fi
    done
    log_success "UFW configured successfully"
  fi
}

# Configure firewalld
firewall::configure_firewalld() {
  local dry_run="${1:-false}"
  local ssh_port="${2:-22}"
  local http_port="${3:-80}"
  local https_port="${4:-443}"

  local commands=""

  commands="$commands
firewall-cmd --permanent --add-port=${ssh_port}/tcp
firewall-cmd --permanent --add-service=http
firewall-cmd --permanent --add-service=https
firewall-cmd --reload"

  if [[ "$dry_run" == "true" ]]; then
    printf "%s\n" "$commands" | while IFS= read -r cmd; do
      [[ -n "$cmd" ]] && printf "  sudo %s\n" "$cmd"
    done
  else
    printf "%s\n" "$commands" | while IFS= read -r cmd; do
      if [[ -n "$cmd" ]]; then
        printf "  Running: %s\n" "$cmd"
        sudo $cmd
      fi
    done
    log_success "Firewalld configured successfully"
  fi
}

# Configure iptables (basic rules)
firewall::configure_iptables() {
  local dry_run="${1:-false}"
  local ssh_port="${2:-22}"
  local http_port="${3:-80}"
  local https_port="${4:-443}"

  printf "  ${COLOR_YELLOW}Note: iptables rules are not persistent by default${COLOR_RESET}\n"
  printf "  Consider using ufw or firewalld for easier management.\n\n"

  local commands=""

  # Flush existing rules
  commands="$commands
iptables -F
iptables -X"

  # Default policies
  commands="$commands
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT ACCEPT"

  # Allow loopback
  commands="$commands
iptables -A INPUT -i lo -j ACCEPT"

  # Allow established connections
  commands="$commands
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT"

  # Allow SSH with rate limiting
  commands="$commands
iptables -A INPUT -p tcp --dport ${ssh_port} -m state --state NEW -m recent --set
iptables -A INPUT -p tcp --dport ${ssh_port} -m state --state NEW -m recent --update --seconds 60 --hitcount 4 -j DROP
iptables -A INPUT -p tcp --dport ${ssh_port} -j ACCEPT"

  # Allow HTTP and HTTPS
  commands="$commands
iptables -A INPUT -p tcp --dport ${http_port} -j ACCEPT
iptables -A INPUT -p tcp --dport ${https_port} -j ACCEPT"

  if [[ "$dry_run" == "true" ]]; then
    printf "%s\n" "$commands" | while IFS= read -r cmd; do
      [[ -n "$cmd" ]] && printf "  sudo %s\n" "$cmd"
    done
    printf "\n  ${COLOR_YELLOW}To persist rules, install iptables-persistent:${COLOR_RESET}\n"
    printf "    sudo apt install iptables-persistent\n"
    printf "    sudo netfilter-persistent save\n"
  else
    printf "%s\n" "$commands" | while IFS= read -r cmd; do
      if [[ -n "$cmd" ]]; then
        printf "  Running: %s\n" "$cmd"
        sudo $cmd
      fi
    done
    log_success "iptables configured (rules are not persistent)"
  fi
}

# Allow a specific port
firewall::allow_port() {
  local port="$1"
  local protocol="${2:-tcp}"
  local comment="${3:-}"

  if [[ -z "$port" ]]; then
    log_error "Port is required"
    return 1
  fi

  local fw_type
  fw_type=$(firewall::detect)

  case "$fw_type" in
    ufw)
      if [[ -n "$comment" ]]; then
        sudo ufw allow "${port}/${protocol}" comment "$comment"
      else
        sudo ufw allow "${port}/${protocol}"
      fi
      ;;
    firewalld)
      sudo firewall-cmd --permanent --add-port="${port}/${protocol}"
      sudo firewall-cmd --reload
      ;;
    iptables)
      sudo iptables -A INPUT -p "$protocol" --dport "$port" -j ACCEPT
      ;;
    *)
      log_error "Cannot configure firewall: $fw_type"
      return 1
      ;;
  esac

  log_success "Allowed port ${port}/${protocol}"
}

# Block a specific port
firewall::block_port() {
  local port="$1"
  local protocol="${2:-tcp}"

  if [[ -z "$port" ]]; then
    log_error "Port is required"
    return 1
  fi

  local fw_type
  fw_type=$(firewall::detect)

  case "$fw_type" in
    ufw)
      sudo ufw deny "${port}/${protocol}"
      ;;
    firewalld)
      sudo firewall-cmd --permanent --remove-port="${port}/${protocol}"
      sudo firewall-cmd --reload
      ;;
    iptables)
      sudo iptables -A INPUT -p "$protocol" --dport "$port" -j DROP
      ;;
    *)
      log_error "Cannot configure firewall: $fw_type"
      return 1
      ;;
  esac

  log_success "Blocked port ${port}/${protocol}"
}

# Allow IP address
firewall::allow_ip() {
  local ip="$1"
  local port="${2:-}"

  if [[ -z "$ip" ]]; then
    log_error "IP address is required"
    return 1
  fi

  local fw_type
  fw_type=$(firewall::detect)

  case "$fw_type" in
    ufw)
      if [[ -n "$port" ]]; then
        sudo ufw allow from "$ip" to any port "$port"
      else
        sudo ufw allow from "$ip"
      fi
      ;;
    firewalld)
      if [[ -n "$port" ]]; then
        sudo firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=$ip port port=$port protocol=tcp accept"
      else
        sudo firewall-cmd --permanent --add-source="$ip"
      fi
      sudo firewall-cmd --reload
      ;;
    iptables)
      if [[ -n "$port" ]]; then
        sudo iptables -A INPUT -p tcp -s "$ip" --dport "$port" -j ACCEPT
      else
        sudo iptables -A INPUT -s "$ip" -j ACCEPT
      fi
      ;;
    *)
      log_error "Cannot configure firewall: $fw_type"
      return 1
      ;;
  esac

  if [[ -n "$port" ]]; then
    log_success "Allowed $ip to port $port"
  else
    log_success "Allowed all traffic from $ip"
  fi
}

# Block IP address
firewall::block_ip() {
  local ip="$1"

  if [[ -z "$ip" ]]; then
    log_error "IP address is required"
    return 1
  fi

  local fw_type
  fw_type=$(firewall::detect)

  case "$fw_type" in
    ufw)
      sudo ufw deny from "$ip"
      ;;
    firewalld)
      sudo firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=$ip reject"
      sudo firewall-cmd --reload
      ;;
    iptables)
      sudo iptables -A INPUT -s "$ip" -j DROP
      ;;
    *)
      log_error "Cannot configure firewall: $fw_type"
      return 1
      ;;
  esac

  log_success "Blocked all traffic from $ip"
}

# Generate recommended rules for nself
firewall::generate_rules() {
  local output_file="${1:-firewall-rules.sh}"

  cat >"$output_file" <<'EOF'
#!/usr/bin/env bash
# nself recommended firewall rules
# Generated by nself firewall generate

# Exit on error
set -euo pipefail

# Detect firewall type
if command -v ufw >/dev/null 2>&1; then
  echo "Configuring UFW..."

  # Reset to defaults
  sudo ufw --force reset

  # Default policies
  sudo ufw default deny incoming
  sudo ufw default allow outgoing

  # SSH (with rate limiting)
  sudo ufw limit 22/tcp comment 'SSH'

  # HTTP and HTTPS
  sudo ufw allow 80/tcp comment 'HTTP'
  sudo ufw allow 443/tcp comment 'HTTPS'

  # Enable
  sudo ufw --force enable

  echo "UFW configured successfully"

elif command -v firewall-cmd >/dev/null 2>&1; then
  echo "Configuring firewalld..."

  sudo firewall-cmd --permanent --add-service=ssh
  sudo firewall-cmd --permanent --add-service=http
  sudo firewall-cmd --permanent --add-service=https
  sudo firewall-cmd --reload

  echo "Firewalld configured successfully"

else
  echo "No supported firewall found"
  exit 1
fi

echo ""
echo "Firewall configured for nself production deployment"
echo ""
echo "Allowed ports:"
echo "  22/tcp   - SSH"
echo "  80/tcp   - HTTP"
echo "  443/tcp  - HTTPS"
EOF

  chmod +x "$output_file"
  log_success "Generated firewall rules: $output_file"
  printf "  Review and run: ./%s\n" "$output_file"
}

# Quick security hardening recommendations
firewall::recommendations() {
  printf "Firewall Security Recommendations\n"
  printf "═══════════════════════════════════════\n\n"

  printf "1. Enable firewall if not already enabled\n"
  printf "   UFW:       sudo ufw enable\n"
  printf "   Firewalld: sudo systemctl enable --now firewalld\n\n"

  printf "2. Default deny incoming traffic\n"
  printf "   UFW:       sudo ufw default deny incoming\n\n"

  printf "3. Only allow required ports\n"
  printf "   - 22/tcp (SSH)\n"
  printf "   - 80/tcp (HTTP - for ACME challenges)\n"
  printf "   - 443/tcp (HTTPS)\n\n"

  printf "4. Use rate limiting for SSH\n"
  printf "   UFW:       sudo ufw limit 22/tcp\n\n"

  printf "5. Consider fail2ban for brute-force protection\n"
  printf "   Install:   sudo apt install fail2ban\n\n"

  printf "6. Use SSH keys instead of passwords\n"
  printf "   Disable password auth in /etc/ssh/sshd_config:\n"
  printf "   PasswordAuthentication no\n\n"

  printf "7. Change default SSH port (optional)\n"
  printf "   Edit /etc/ssh/sshd_config: Port 2222\n"
  printf "   Update firewall: sudo ufw allow 2222/tcp\n\n"

  printf "8. Keep system updated\n"
  printf "   Ubuntu: sudo apt update && sudo apt upgrade\n"
}

# Export functions
export -f firewall::detect
export -f firewall::status
export -f firewall::configure
export -f firewall::configure_ufw
export -f firewall::configure_firewalld
export -f firewall::configure_iptables
export -f firewall::allow_port
export -f firewall::block_port
export -f firewall::allow_ip
export -f firewall::block_ip
export -f firewall::generate_rules
export -f firewall::recommendations
