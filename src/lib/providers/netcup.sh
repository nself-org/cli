#!/usr/bin/env bash
# netcup.sh - Netcup provider module (German quality at budget prices)


PROVIDER_NAME="netcup"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Netcup"

NETCUP_REGIONS=("de" "at" "us")

# Bash 3.2 compatible - use function instead of associative array
_netcup_get_size() {
  local size="$1"
  case "$size" in
    tiny) echo "VPS 200 G10s" ;;
    small) echo "VPS 500 G10s" ;;
    medium) echo "VPS 1000 G10s" ;;
    large) echo "VPS 2000 G10s" ;;
    xlarge) echo "VPS 3000 G10s" ;;
    *) echo "VPS 500 G10s" ;;
  esac
}

provider_netcup_init() {
  log_info "Initializing Netcup provider..."
  log_info "Get API credentials from: https://www.customercontrolpanel.de/"

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  read -rp "Customer ID: " customer_id
  read -rsp "API Key: " api_key
  echo
  read -rsp "API Password: " api_password
  echo

  cat >"$config_dir/netcup.yml" <<EOF
provider: netcup
customer_id: "$customer_id"
api_key: "$api_key"
api_password: "$api_password"
default_region: de
default_size: small
EOF

  log_success "Netcup provider initialized"
}

provider_netcup_validate() {
  local config_file="${HOME}/.nself/providers/netcup.yml"
  [[ ! -f "$config_file" ]] && {
    log_error "Netcup not configured"
    return 1
  }

  # Netcup uses SOAP API - validation would require proper SOAP client
  log_info "Netcup credentials saved (SOAP API validation skipped)"
  return 0
}

provider_netcup_list_regions() {
  printf "%-10s %-25s\n" "REGION" "LOCATION"
  printf "%-10s %-25s\n" "------" "--------"
  printf "%-10s %-25s\n" "de" "Germany (Nuremberg)"
  printf "%-10s %-25s\n" "at" "Austria (Vienna)"
  printf "%-10s %-25s\n" "us" "USA"
}

provider_netcup_list_sizes() {
  printf "%-10s %-18s %-6s %-8s %-10s\n" "SIZE" "PLAN" "vCPU" "RAM" "EST. COST"
  printf "%-10s %-18s %-6s %-8s %-10s\n" "----" "----" "----" "---" "---------"
  printf "%-10s %-18s %-6s %-8s %-10s\n" "tiny" "VPS 200 G10s" "2" "2GB" "€2.99/mo"
  printf "%-10s %-18s %-6s %-8s %-10s\n" "small" "VPS 500 G10s" "4" "4GB" "€4.99/mo"
  printf "%-10s %-18s %-6s %-8s %-10s\n" "medium" "VPS 1000 G10s" "6" "8GB" "€8.99/mo"
  printf "%-10s %-18s %-6s %-8s %-10s\n" "large" "VPS 2000 G10s" "8" "16GB" "€14.99/mo"
  printf "%-10s %-18s %-6s %-8s %-10s\n" "xlarge" "VPS 3000 G10s" "10" "24GB" "€22.99/mo"
}

provider_netcup_provision() {
  log_warning "Netcup provisioning requires manual setup via SCP (Server Control Panel)"
  log_info "Visit: https://www.customercontrolpanel.de/"
  log_info "After creating VPS, add server: nself provider servers add <name> --ip <ip>"
  return 1
}

provider_netcup_destroy() {
  log_warning "Netcup VPS must be cancelled via SCP"
  log_info "Visit: https://www.customercontrolpanel.de/"
  return 1
}

provider_netcup_status() { log_info "Check status at: https://www.customercontrolpanel.de/"; }
provider_netcup_list() { provider_netcup_status; }
provider_netcup_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@${ip}" "$@"
}
provider_netcup_get_ip() { echo "$1"; }
provider_netcup_estimate_cost() {
  case "${1:-small}" in
    tiny) echo "3" ;; small) echo "5" ;; medium) echo "9" ;;
    large) echo "15" ;; xlarge) echo "23" ;; *) echo "5" ;;
  esac
}

export -f provider_netcup_init provider_netcup_validate provider_netcup_list_regions
export -f provider_netcup_list_sizes provider_netcup_provision provider_netcup_destroy
export -f provider_netcup_status provider_netcup_list provider_netcup_ssh
export -f provider_netcup_get_ip provider_netcup_estimate_cost
