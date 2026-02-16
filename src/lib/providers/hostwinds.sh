#!/usr/bin/env bash
# hostwinds.sh - Hostwinds provider (managed VPS option)
PROVIDER_NAME="hostwinds"

set -euo pipefail


provider_hostwinds_init() {
  log_info "Initializing Hostwinds provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"
  read -rsp "API Key: " api_key
  echo
  cat >"$config_dir/hostwinds.yml" <<EOF
provider: hostwinds
api_key: "$api_key"
default_region: seattle
default_size: small
EOF
  log_success "Hostwinds provider initialized"
}

provider_hostwinds_validate() {
  local config_file="${HOME}/.nself/providers/hostwinds.yml"
  [[ -f "$config_file" ]] && {
    log_success "Hostwinds configured"
    return 0
  }
  log_error "Hostwinds not configured"
  return 1
}

provider_hostwinds_list_regions() {
  printf "%-12s %-20s\n" "REGION" "LOCATION"
  printf "%-12s %-20s\n" "seattle" "Seattle, WA" "dallas" "Dallas, TX" "amsterdam" "Amsterdam, NL"
}

provider_hostwinds_list_sizes() {
  printf "%-10s %-6s %-8s %-12s\n" "SIZE" "vCPU" "RAM" "EST. COST"
  printf "%-10s %-6s %-8s %-12s\n" "small" "1" "1GB" "~\$5/mo" "medium" "2" "4GB" "~\$15/mo"
  printf "%-10s %-6s %-8s %-12s\n" "large" "4" "8GB" "~\$30/mo" "xlarge" "8" "16GB" "~\$60/mo"
}

provider_hostwinds_provision() {
  log_warning "Hostwinds provisioning via portal: https://www.hostwinds.com"
  return 1
}
provider_hostwinds_destroy() {
  log_warning "Manage via Hostwinds portal"
  return 1
}
provider_hostwinds_status() { log_info "Check status at: https://www.hostwinds.com"; }
provider_hostwinds_list() { provider_hostwinds_status; }
provider_hostwinds_ssh() {
  local ip="$1"
  shift
  ssh "root@$ip" "$@"
}
provider_hostwinds_get_ip() { echo "$1"; }
provider_hostwinds_estimate_cost() { case "${1:-small}" in small) echo "5" ;; medium) echo "15" ;; large) echo "30" ;; *) echo "5" ;; esac }

export -f provider_hostwinds_init provider_hostwinds_validate provider_hostwinds_list_regions
export -f provider_hostwinds_list_sizes provider_hostwinds_provision provider_hostwinds_destroy
export -f provider_hostwinds_status provider_hostwinds_list provider_hostwinds_ssh
export -f provider_hostwinds_get_ip provider_hostwinds_estimate_cost
