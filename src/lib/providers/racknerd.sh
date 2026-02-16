#!/usr/bin/env bash
# racknerd.sh - RackNerd provider (extreme budget VPS)

PROVIDER_NAME="racknerd"

set -euo pipefail


# Note: RackNerd uses portal-based provisioning
# This provider handles SSH deployment to existing servers

provider_racknerd_init() {
  log_info "Initializing RackNerd provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Default SSH Key Path [~/.ssh/id_rsa.pub]: "
  read -r ssh_key_path
  ssh_key_path="${ssh_key_path:-$HOME/.ssh/id_rsa.pub}"

  cat >"$config_dir/racknerd.yml" <<EOF
provider: racknerd
ssh_key_path: "$ssh_key_path"
portal_url: https://my.racknerd.com
notes: |
  RackNerd offers extreme budget VPS with excellent flash sales.
  Provision via portal, then add server IP to nself.
  Flash sales often 75%+ off regular prices.
EOF

  log_success "RackNerd provider initialized"
  log_info "Provision servers at: https://www.racknerd.com"
  log_info "Check for flash sales at: https://lowendbox.com (search RackNerd)"
}

provider_racknerd_validate() {
  local config_file="${HOME}/.nself/providers/racknerd.yml"
  if [[ -f "$config_file" ]]; then
    log_success "RackNerd configured"
    return 0
  fi
  log_error "RackNerd not configured. Run: nself provider init racknerd"
  return 1
}

provider_racknerd_list_regions() {
  printf "%-15s %-25s\n" "LOCATION" "CITY"
  printf "%-15s %-25s\n" "los-angeles" "Los Angeles, CA"
  printf "%-15s %-25s\n" "san-jose" "San Jose, CA"
  printf "%-15s %-25s\n" "seattle" "Seattle, WA"
  printf "%-15s %-25s\n" "dallas" "Dallas, TX"
  printf "%-15s %-25s\n" "chicago" "Chicago, IL"
  printf "%-15s %-25s\n" "new-york" "New York, NY"
  printf "%-15s %-25s\n" "atlanta" "Atlanta, GA"
  printf "%-15s %-25s\n" "ashburn" "Ashburn, VA"
  printf "%-15s %-25s\n" "amsterdam" "Amsterdam, NL"
  printf "%-15s %-25s\n" "strasbourg" "Strasbourg, FR"
}

provider_racknerd_list_sizes() {
  printf "%-12s %-6s %-8s %-10s %-15s\n" "PLAN" "vCPU" "RAM" "STORAGE" "PRICE"
  echo ""
  echo "Regular KVM VPS Plans:"
  printf "%-12s %-6s %-8s %-10s %-15s\n" "KVM512" "1" "512MB" "10GB SSD" "~\$10.28/yr"
  printf "%-12s %-6s %-8s %-10s %-15s\n" "KVM1G" "1" "1GB" "20GB SSD" "~\$11.49/yr"
  printf "%-12s %-6s %-8s %-10s %-15s\n" "KVM2G" "1" "2GB" "35GB SSD" "~\$18.88/yr"
  printf "%-12s %-6s %-8s %-10s %-15s\n" "KVM3G" "2" "3GB" "55GB SSD" "~\$27.98/yr"
  printf "%-12s %-6s %-8s %-10s %-15s\n" "KVM4G" "2" "4GB" "80GB SSD" "~\$37.38/yr"
  echo ""
  echo "Flash Sale Plans (when available):"
  printf "%-12s %-6s %-8s %-10s %-15s\n" "Special" "1-2" "1-3GB" "20-45GB" "~\$10-15/yr"
  echo ""
  echo "Note: Flash sales offer EXTREME value. Check lowendbox.com regularly."
  echo "Annual billing is typical. Monthly ~3x more expensive."
}

provider_racknerd_provision() {
  log_warning "RackNerd requires portal provisioning"
  log_info ""
  log_info "Steps to provision:"
  log_info "1. Visit: https://www.racknerd.com/NewYear/"
  log_info "2. Or check: https://lowendbox.com (search 'RackNerd')"
  log_info "3. Select plan and complete checkout"
  log_info "4. Add your server: nself provider server add <ip>"
  echo ""
  log_info "Best value recommendations:"
  echo "  - New Year Deal: 1GB/1vCPU/21GB SSD - \$11.49/year"
  echo "  - Black Friday: Often 2GB+ for similar price"
  return 1
}

provider_racknerd_destroy() {
  log_warning "Manage RackNerd servers at: https://my.racknerd.com"
  log_info "To remove from nself: nself provider server remove <ip>"
  return 1
}

provider_racknerd_status() {
  local ip="$1"
  log_info "Checking connectivity to $ip..."

  if ping -c 1 -W 2 "$ip" >/dev/null 2>&1; then
    log_success "Server $ip is reachable"
    local uptime
    uptime=$(ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "root@$ip" uptime 2>/dev/null) || true
    if [[ -n "$uptime" ]]; then
      echo "Uptime: $uptime"
    fi
    return 0
  else
    log_error "Server $ip is not reachable"
    return 1
  fi
}

provider_racknerd_list() {
  local servers_file="${HOME}/.nself/servers.yml"

  if [[ -f "$servers_file" ]]; then
    printf "%-15s %-20s %-10s\n" "IP" "NAME" "STATUS"
    grep "provider: racknerd" -A 2 "$servers_file" 2>/dev/null | while read -r line; do
      if echo "$line" | grep -q "ip:"; then
        local ip
        ip=$(echo "$line" | cut -d':' -f2 | tr -d ' "')
        printf "%-15s %-20s %-10s\n" "$ip" "-" "manual"
      fi
    done
  else
    log_info "No RackNerd servers registered"
    log_info "Add servers: nself provider server add <ip> --provider racknerd"
  fi
}

provider_racknerd_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_racknerd_get_ip() {
  echo "$1"
}

provider_racknerd_estimate_cost() {
  local size="${1:-small}"
  # RackNerd yearly prices converted to monthly
  case "$size" in
    tiny) echo "1" ;;   # ~$10/year
    small) echo "1" ;;  # ~$12/year
    medium) echo "2" ;; # ~$19/year
    large) echo "3" ;;  # ~$28/year
    xlarge) echo "4" ;; # ~$37/year
    *) echo "1" ;;
  esac
}

# Export functions
export -f provider_racknerd_init provider_racknerd_validate provider_racknerd_list_regions
export -f provider_racknerd_list_sizes provider_racknerd_provision provider_racknerd_destroy
export -f provider_racknerd_status provider_racknerd_list provider_racknerd_ssh
export -f provider_racknerd_get_ip provider_racknerd_estimate_cost
