#!/usr/bin/env bash
# buyvm.sh - BuyVM/Frantech provider (storage VPS, DDoS protection)

PROVIDER_NAME="buyvm"

set -euo pipefail


# Note: BuyVM uses Stallion portal-based provisioning
# Known for excellent storage slabs and DDoS protection

provider_buyvm_init() {
  log_info "Initializing BuyVM provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Default SSH Key Path [~/.ssh/id_rsa.pub]: "
  read -r ssh_key_path
  ssh_key_path="${ssh_key_path:-$HOME/.ssh/id_rsa.pub}"

  cat >"$config_dir/buyvm.yml" <<EOF
provider: buyvm
ssh_key_path: "$ssh_key_path"
portal_url: https://my.frantech.ca
notes: |
  BuyVM (Frantech) offers excellent storage VPS and DDoS protection.
  Known for reliability and responsive support.
  Storage slabs: 256GB-1TB for \$1.25-\$5/mo additional.
EOF

  log_success "BuyVM provider initialized"
  log_info "Provision servers at: https://buyvm.net"
}

provider_buyvm_validate() {
  local config_file="${HOME}/.nself/providers/buyvm.yml"
  if [[ -f "$config_file" ]]; then
    log_success "BuyVM configured"
    return 0
  fi
  log_error "BuyVM not configured. Run: nself provider init buyvm"
  return 1
}

provider_buyvm_list_regions() {
  printf "%-15s %-25s %-20s\n" "LOCATION" "CITY" "NOTES"
  printf "%-15s %-25s %-20s\n" "las-vegas" "Las Vegas, NV" "Primary US"
  printf "%-15s %-25s %-20s\n" "new-york" "New York, NY" "East US"
  printf "%-15s %-25s %-20s\n" "luxembourg" "Luxembourg" "EU location"
  printf "%-15s %-25s %-20s\n" "miami" "Miami, FL" "Southeast US"
  echo ""
  echo "All locations include free DDoS protection."
}

provider_buyvm_list_sizes() {
  printf "%-12s %-6s %-8s %-10s %-12s\n" "PLAN" "vCPU" "RAM" "SSD" "PRICE"
  echo ""
  echo "KVM Slice Plans:"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "SLICE512" "1" "512MB" "10GB" "\$2/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "SLICE1024" "1" "1GB" "20GB" "\$3.50/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "SLICE2048" "1" "2GB" "40GB" "\$7/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "SLICE4096" "2" "4GB" "80GB" "\$15/mo"
  echo ""
  echo "Storage Slabs (add-on):"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "BLOCK256" "-" "-" "256GB" "\$1.25/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "BLOCK512" "-" "-" "512GB" "\$2.50/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "BLOCK1024" "-" "-" "1TB" "\$5.00/mo"
  echo ""
  echo "Note: Storage slabs attach via iSCSI. Great for backups/media."
}

provider_buyvm_provision() {
  log_warning "BuyVM requires portal provisioning (Stallion)"
  log_info ""
  log_info "Steps to provision:"
  log_info "1. Visit: https://buyvm.net"
  log_info "2. Select location and plan"
  log_info "3. Complete checkout"
  log_info "4. Add server: nself provider server add <ip>"
  echo ""
  log_info "Recommended for nself:"
  echo "  - SLICE1024: 1GB RAM, 20GB SSD - \$3.50/mo"
  echo "  - SLICE2048: 2GB RAM, 40GB SSD - \$7/mo"
  echo "  - Add BLOCK256 for backup storage - +\$1.25/mo"
  return 1
}

provider_buyvm_destroy() {
  log_warning "Manage BuyVM servers at: https://my.frantech.ca"
  log_info "To remove from nself: nself provider server remove <ip>"
  return 1
}

provider_buyvm_status() {
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

provider_buyvm_list() {
  local servers_file="${HOME}/.nself/servers.yml"

  if [[ -f "$servers_file" ]]; then
    printf "%-15s %-20s %-10s\n" "IP" "NAME" "STATUS"
    grep "provider: buyvm" -A 2 "$servers_file" 2>/dev/null | while read -r line; do
      if echo "$line" | grep -q "ip:"; then
        local ip
        ip=$(echo "$line" | cut -d':' -f2 | tr -d ' "')
        printf "%-15s %-20s %-10s\n" "$ip" "-" "manual"
      fi
    done
  else
    log_info "No BuyVM servers registered"
    log_info "Add servers: nself provider server add <ip> --provider buyvm"
  fi
}

provider_buyvm_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_buyvm_get_ip() {
  echo "$1"
}

provider_buyvm_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "2" ;;    # SLICE512
    small) echo "4" ;;   # SLICE1024
    medium) echo "7" ;;  # SLICE2048
    large) echo "15" ;;  # SLICE4096
    xlarge) echo "30" ;; # Multiple slices
    *) echo "4" ;;
  esac
}

# Export functions
export -f provider_buyvm_init provider_buyvm_validate provider_buyvm_list_regions
export -f provider_buyvm_list_sizes provider_buyvm_provision provider_buyvm_destroy
export -f provider_buyvm_status provider_buyvm_list provider_buyvm_ssh
export -f provider_buyvm_get_ip provider_buyvm_estimate_cost
