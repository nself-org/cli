#!/usr/bin/env bash
# upcloud.sh - UpCloud provider module
# Known for MaxIOPS high-performance storage


PROVIDER_NAME="upcloud"

set -euo pipefail

PROVIDER_DISPLAY_NAME="UpCloud"

# UpCloud zones
UPCLOUD_ZONES=(
  "de-fra1" "fi-hel1" "fi-hel2" "nl-ams1"
  "sg-sin1" "uk-lon1" "us-chi1" "us-nyc1" "us-sjo1"
  "au-syd1" "es-mad1" "pl-waw1" "se-sto1"
)

# Size mappings - Bash 3.2 compatible function
_upcloud_get_plan() {
  local size="$1"
  case "$size" in
    tiny) echo "1xCPU-1GB" ;;
    small) echo "1xCPU-2GB" ;;
    medium) echo "2xCPU-4GB" ;;
    large) echo "4xCPU-8GB" ;;
    xlarge) echo "8xCPU-16GB" ;;
    *) echo "1xCPU-2GB" ;;
  esac
}

provider_upcloud_init() {
  log_info "Initializing UpCloud provider..."
  log_info "Get API credentials from: https://hub.upcloud.com/account"

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  read -rp "UpCloud Username: " username
  read -rsp "UpCloud Password: " password
  echo

  cat >"$config_dir/upcloud.yml" <<EOF
provider: upcloud
username: "$username"
password: "$password"
default_zone: de-fra1
default_size: small
notes: |
  MaxIOPS storage - fastest I/O performance
  \$25 free credit for new accounts
EOF

  log_success "UpCloud provider initialized"
}

provider_upcloud_validate() {
  local config_file="${HOME}/.nself/providers/upcloud.yml"
  [[ ! -f "$config_file" ]] && {
    log_error "UpCloud not configured"
    return 1
  }

  local username password
  username=$(grep "username:" "$config_file" | cut -d'"' -f2)
  password=$(grep "password:" "$config_file" | cut -d'"' -f2)

  if curl -s -u "$username:$password" "https://api.upcloud.com/1.3/account" | grep -q "credits"; then
    log_success "UpCloud credentials valid"
    return 0
  else
    log_error "UpCloud credentials invalid"
    return 1
  fi
}

provider_upcloud_list_regions() {
  printf "%-12s %-25s\n" "ZONE" "LOCATION"
  printf "%-12s %-25s\n" "----" "--------"
  printf "%-12s %-25s\n" "de-fra1" "Germany (Frankfurt)"
  printf "%-12s %-25s\n" "fi-hel1" "Finland (Helsinki)"
  printf "%-12s %-25s\n" "nl-ams1" "Netherlands (Amsterdam)"
  printf "%-12s %-25s\n" "uk-lon1" "UK (London)"
  printf "%-12s %-25s\n" "us-chi1" "USA (Chicago)"
  printf "%-12s %-25s\n" "us-nyc1" "USA (New York)"
  printf "%-12s %-25s\n" "sg-sin1" "Singapore"
  printf "%-12s %-25s\n" "au-syd1" "Australia (Sydney)"
}

provider_upcloud_list_sizes() {
  printf "%-10s %-12s %-6s %-8s %-10s\n" "SIZE" "PLAN" "CPU" "RAM" "EST. COST"
  printf "%-10s %-12s %-6s %-8s %-10s\n" "----" "----" "---" "---" "---------"
  printf "%-10s %-12s %-6s %-8s %-10s\n" "tiny" "1xCPU-1GB" "1" "1GB" "~\$5/mo"
  printf "%-10s %-12s %-6s %-8s %-10s\n" "small" "1xCPU-2GB" "1" "2GB" "~\$10/mo"
  printf "%-10s %-12s %-6s %-8s %-10s\n" "medium" "2xCPU-4GB" "2" "4GB" "~\$20/mo"
  printf "%-10s %-12s %-6s %-8s %-10s\n" "large" "4xCPU-8GB" "4" "8GB" "~\$40/mo"
  printf "%-10s %-12s %-6s %-8s %-10s\n" "xlarge" "8xCPU-16GB" "8" "16GB" "~\$80/mo"
}

provider_upcloud_provision() {
  local name="" size="small" zone="de-fra1"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name)
        name="$2"
        shift 2
        ;;
      --size)
        size="$2"
        shift 2
        ;;
      --region | --zone)
        zone="$2"
        shift 2
        ;;
      *) shift ;;
    esac
  done

  [[ -z "$name" ]] && {
    log_error "Server name required"
    return 1
  }

  local config_file="${HOME}/.nself/providers/upcloud.yml"
  local username password
  username=$(grep "username:" "$config_file" | cut -d'"' -f2)
  password=$(grep "password:" "$config_file" | cut -d'"' -f2)

  local plan
  plan=$(_upcloud_get_plan "$size")

  log_info "Provisioning UpCloud server: $name ($plan in $zone)"

  local response
  response=$(curl -s -u "$username:$password" \
    -X POST "https://api.upcloud.com/1.3/server" \
    -H "Content-Type: application/json" \
    -d "{
      \"server\": {
        \"zone\": \"$zone\",
        \"title\": \"$name\",
        \"hostname\": \"$name\",
        \"plan\": \"$plan\",
        \"storage_devices\": {
          \"storage_device\": [{
            \"action\": \"clone\",
            \"storage\": \"01000000-0000-4000-8000-000030200200\",
            \"title\": \"$name-disk\",
            \"tier\": \"maxiops\"
          }]
        }
      }
    }" 2>/dev/null)

  if echo "$response" | grep -q '"uuid"'; then
    local uuid
    uuid=$(echo "$response" | grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "Server created: $uuid"
    echo "$uuid"
  else
    log_error "Failed to create server"
    echo "$response"
    return 1
  fi
}

provider_upcloud_destroy() {
  local server_id="$1"
  local config_file="${HOME}/.nself/providers/upcloud.yml"
  local username password
  username=$(grep "username:" "$config_file" | cut -d'"' -f2)
  password=$(grep "password:" "$config_file" | cut -d'"' -f2)

  # Stop server first
  curl -s -u "$username:$password" -X POST \
    "https://api.upcloud.com/1.3/server/$server_id/stop" \
    -H "Content-Type: application/json" \
    -d '{"stop_server":{"stop_type":"soft"}}' 2>/dev/null

  sleep 5

  # Delete server
  curl -s -u "$username:$password" -X DELETE \
    "https://api.upcloud.com/1.3/server/$server_id?storages=1" 2>/dev/null

  log_success "Server deleted: $server_id"
}

provider_upcloud_status() {
  local server_id="${1:-}"
  local config_file="${HOME}/.nself/providers/upcloud.yml"
  local username password
  username=$(grep "username:" "$config_file" | cut -d'"' -f2)
  password=$(grep "password:" "$config_file" | cut -d'"' -f2)

  if [[ -n "$server_id" ]]; then
    curl -s -u "$username:$password" "https://api.upcloud.com/1.3/server/$server_id" 2>/dev/null
  else
    curl -s -u "$username:$password" "https://api.upcloud.com/1.3/server" 2>/dev/null
  fi
}

provider_upcloud_list() {
  provider_upcloud_status
}

provider_upcloud_ssh() {
  local server_id="$1"
  shift
  local ip
  ip=$(provider_upcloud_get_ip "$server_id")
  [[ -z "$ip" ]] && {
    log_error "Could not get IP"
    return 1
  }
  ssh -o StrictHostKeyChecking=no "root@${ip}" "$@"
}

provider_upcloud_get_ip() {
  local server_id="$1"
  provider_upcloud_status "$server_id" 2>/dev/null | grep -o '"address":"[0-9.]*"' | head -1 | cut -d'"' -f4
}

provider_upcloud_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "5" ;; small) echo "10" ;; medium) echo "20" ;;
    large) echo "40" ;; xlarge) echo "80" ;; *) echo "10" ;;
  esac
}

export -f provider_upcloud_init provider_upcloud_validate
export -f provider_upcloud_list_regions provider_upcloud_list_sizes
export -f provider_upcloud_provision provider_upcloud_destroy
export -f provider_upcloud_status provider_upcloud_list provider_upcloud_ssh
export -f provider_upcloud_get_ip provider_upcloud_estimate_cost
