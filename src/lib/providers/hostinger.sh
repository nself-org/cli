#!/usr/bin/env bash
# hostinger.sh - Hostinger VPS provider module


PROVIDER_NAME="hostinger"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Hostinger"

HOSTINGER_REGIONS=("us" "eu" "asia" "sa")

# Bash 3.2 compatible - use function instead of associative array
_hostinger_get_plan() {
  local size="$1"
  case "$size" in
    tiny | small) echo "KVM 1" ;;
    medium) echo "KVM 2" ;;
    large) echo "KVM 4" ;;
    xlarge) echo "KVM 8" ;;
    *) echo "KVM 1" ;;
  esac
}

provider_hostinger_init() {
  log_info "Initializing Hostinger provider..."
  log_info "Get API token from: https://hpanel.hostinger.com/api"

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  read -rsp "API Token: " api_token
  echo

  cat >"$config_dir/hostinger.yml" <<EOF
provider: hostinger
api_token: "$api_token"
default_region: us
default_size: small
EOF

  log_success "Hostinger provider initialized"
}

provider_hostinger_validate() {
  local config_file="${HOME}/.nself/providers/hostinger.yml"
  [[ ! -f "$config_file" ]] && {
    log_error "Hostinger not configured"
    return 1
  }
  local token
  token=$(grep "api_token:" "$config_file" | cut -d'"' -f2)
  if curl -s -H "Authorization: Bearer $token" "https://api.hostinger.com/v1/vps" 2>/dev/null | grep -q "data"; then
    log_success "Hostinger credentials valid"
    return 0
  fi
  log_error "Hostinger credentials invalid"
  return 1
}

provider_hostinger_list_regions() {
  printf "%-10s %-25s\n" "REGION" "LOCATION"
  printf "%-10s %-25s\n" "us" "USA" "eu" "Europe" "asia" "Asia" "sa" "South America"
}

provider_hostinger_list_sizes() {
  printf "%-10s %-10s %-6s %-8s %-12s\n" "SIZE" "PLAN" "vCPU" "RAM" "EST. COST"
  printf "%-10s %-10s %-6s %-8s %-12s\n" "small" "KVM 1" "1" "4GB" "~\$5/mo"
  printf "%-10s %-10s %-6s %-8s %-12s\n" "medium" "KVM 2" "2" "8GB" "~\$9/mo"
  printf "%-10s %-10s %-6s %-8s %-12s\n" "large" "KVM 4" "4" "16GB" "~\$16/mo"
  printf "%-10s %-10s %-6s %-8s %-12s\n" "xlarge" "KVM 8" "8" "32GB" "~\$30/mo"
}

provider_hostinger_provision() {
  local name="" size="small" region="us"
  while [[ $# -gt 0 ]]; do
    case "$1" in --name)
      name="$2"
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
    *) shift ;; esac
  done
  [[ -z "$name" ]] && {
    log_error "Server name required"
    return 1
  }

  local config_file="${HOME}/.nself/providers/hostinger.yml"
  local token
  token=$(grep "api_token:" "$config_file" | cut -d'"' -f2)
  local plan
  plan=$(_hostinger_get_plan "$size")

  log_info "Provisioning Hostinger VPS: $name ($plan)"

  local response
  response=$(curl -s -X POST "https://api.hostinger.com/v1/vps" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -d "{\"hostname\": \"$name\", \"plan\": \"$plan\", \"location\": \"$region\"}" 2>/dev/null)

  if echo "$response" | grep -q "id"; then
    local id
    id=$(echo "$response" | grep -o '"id":[0-9]*' | cut -d':' -f2)
    log_success "VPS created: $id"
    echo "$id"
  else
    log_error "Failed to create VPS"
    return 1
  fi
}

provider_hostinger_destroy() {
  local id="$1"
  local config_file="${HOME}/.nself/providers/hostinger.yml"
  local token
  token=$(grep "api_token:" "$config_file" | cut -d'"' -f2)
  curl -s -X DELETE "https://api.hostinger.com/v1/vps/$id" -H "Authorization: Bearer $token" 2>/dev/null
  log_success "VPS deleted"
}

provider_hostinger_status() {
  local config_file="${HOME}/.nself/providers/hostinger.yml"
  local token
  token=$(grep "api_token:" "$config_file" | cut -d'"' -f2)
  if [[ -n "${1:-}" ]]; then
    curl -s "https://api.hostinger.com/v1/vps/$1" -H "Authorization: Bearer $token" 2>/dev/null
  else
    curl -s "https://api.hostinger.com/v1/vps" -H "Authorization: Bearer $token" 2>/dev/null
  fi
}

provider_hostinger_list() { provider_hostinger_status; }
provider_hostinger_ssh() {
  local id="$1"
  shift
  local ip
  ip=$(provider_hostinger_get_ip "$id")
  ssh "root@$ip" "$@"
}
provider_hostinger_get_ip() { provider_hostinger_status "$1" 2>/dev/null | grep -o '"ip":"[0-9.]*"' | head -1 | cut -d'"' -f4; }
provider_hostinger_estimate_cost() {
  case "${1:-small}" in tiny | small) echo "5" ;; medium) echo "9" ;; large) echo "16" ;; xlarge) echo "30" ;; *) echo "5" ;; esac
}

export -f provider_hostinger_init provider_hostinger_validate provider_hostinger_list_regions
export -f provider_hostinger_list_sizes provider_hostinger_provision provider_hostinger_destroy
export -f provider_hostinger_status provider_hostinger_list provider_hostinger_ssh
export -f provider_hostinger_get_ip provider_hostinger_estimate_cost
