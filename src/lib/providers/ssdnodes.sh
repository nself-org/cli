#!/usr/bin/env bash
# ssdnodes.sh - SSDNodes provider (extreme budget, high specs)

PROVIDER_NAME="ssdnodes"

set -euo pipefail


# Note: SSDNodes uses a portal-based system, API access limited
# This provider focuses on manual provisioning with SSH automation

provider_ssdnodes_init() {
  log_info "Initializing SSDNodes provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Default SSH Key Path [~/.ssh/id_rsa.pub]: "
  read -r ssh_key_path
  ssh_key_path="${ssh_key_path:-$HOME/.ssh/id_rsa.pub}"

  if [[ ! -f "$ssh_key_path" ]]; then
    log_warning "SSH key not found at $ssh_key_path"
  fi

  cat >"$config_dir/ssdnodes.yml" <<EOF
provider: ssdnodes
ssh_key_path: "$ssh_key_path"
default_location: dallas
portal_url: https://www.ssdnodes.com/manage/
notes: |
  SSDNodes requires manual provisioning via portal.
  nself handles deployment once server IP is added.
EOF

  log_success "SSDNodes provider initialized"
  log_info "Provision servers at: https://www.ssdnodes.com"
}

provider_ssdnodes_validate() {
  local config_file="${HOME}/.nself/providers/ssdnodes.yml"
  if [[ -f "$config_file" ]]; then
    log_success "SSDNodes configured"
    return 0
  fi
  log_error "SSDNodes not configured. Run: nself provider init ssdnodes"
  return 1
}

provider_ssdnodes_list_regions() {
  printf "%-12s %-25s %-20s\n" "LOCATION" "CITY" "NOTES"
  printf "%-12s %-25s %-20s\n" "dallas" "Dallas, Texas" "Primary US location"
  printf "%-12s %-25s %-20s\n" "seattle" "Seattle, Washington" "West Coast US"
  printf "%-12s %-25s %-20s\n" "amsterdam" "Amsterdam, NL" "EU location"
}

provider_ssdnodes_list_sizes() {
  printf "%-12s %-6s %-8s %-10s %-12s\n" "PLAN" "vCPU" "RAM" "STORAGE" "PRICE"
  echo ""
  echo "KVM VPS Plans (Excellent value):"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Performance" "4" "16GB" "80GB NVMe" "~\$9.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Pro" "8" "32GB" "160GB NVMe" "~\$14.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Elite" "16" "64GB" "320GB NVMe" "~\$24.99/mo"
  echo ""
  echo "Note: SSDNodes offers some of the best specs-per-dollar in the industry"
  echo "Annual billing offers significant discounts (up to 75% off)"
}

provider_ssdnodes_provision() {
  log_warning "SSDNodes requires manual provisioning via portal"
  log_info "1. Visit: https://www.ssdnodes.com/pricing/"
  log_info "2. Select your plan and complete checkout"
  log_info "3. Add your server IP to nself with: nself provider server add <ip>"
  echo ""
  log_info "Recommended plans for nself:"
  echo "  - Performance VPS: 4 vCPU, 16GB RAM, 80GB NVMe - \$9.99/mo"
  echo "  - Pro VPS: 8 vCPU, 32GB RAM, 160GB NVMe - \$14.99/mo"
  return 1
}

provider_ssdnodes_destroy() {
  log_warning "Manage your SSDNodes servers at: https://www.ssdnodes.com/manage/"
  log_info "To remove from nself: nself provider server remove <ip>"
  return 1
}

provider_ssdnodes_status() {
  local ip="$1"
  log_info "Checking connectivity to $ip..."

  if ping -c 1 -W 2 "$ip" >/dev/null 2>&1; then
    log_success "Server $ip is reachable"

    # Try to get uptime via SSH
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

provider_ssdnodes_list() {
  local servers_file="${HOME}/.nself/servers.yml"

  if [[ -f "$servers_file" ]]; then
    printf "%-15s %-20s %-10s\n" "IP" "NAME" "STATUS"
    grep "provider: ssdnodes" -A 2 "$servers_file" 2>/dev/null | while read -r line; do
      if echo "$line" | grep -q "ip:"; then
        local ip
        ip=$(echo "$line" | cut -d':' -f2 | tr -d ' "')
        printf "%-15s %-20s %-10s\n" "$ip" "-" "manual"
      fi
    done
  else
    log_info "No SSDNodes servers registered"
    log_info "Add servers with: nself provider server add <ip> --provider ssdnodes"
  fi
}

provider_ssdnodes_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_ssdnodes_get_ip() {
  # For manual providers, the IP is passed directly
  echo "$1"
}

provider_ssdnodes_estimate_cost() {
  local size="${1:-small}"
  # SSDNodes has exceptional value
  case "$size" in
    tiny) echo "5" ;;    # Smallest available
    small) echo "10" ;;  # Performance plan
    medium) echo "15" ;; # Pro plan
    large) echo "25" ;;  # Elite plan
    xlarge) echo "40" ;; # Custom/larger
    *) echo "10" ;;
  esac
}

# Export functions
export -f provider_ssdnodes_init provider_ssdnodes_validate provider_ssdnodes_list_regions
export -f provider_ssdnodes_list_sizes provider_ssdnodes_provision provider_ssdnodes_destroy
export -f provider_ssdnodes_status provider_ssdnodes_list provider_ssdnodes_ssh
export -f provider_ssdnodes_get_ip provider_ssdnodes_estimate_cost
