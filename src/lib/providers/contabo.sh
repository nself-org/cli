#!/usr/bin/env bash
# contabo.sh - Contabo VPS provider module
# Best specs-per-dollar provider


PROVIDER_NAME="contabo"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Contabo"

# Contabo regions
CONTABO_REGIONS=(
  "EU" "US-central" "US-east" "US-west"
  "SIN" "JPN" "AUS"
)

# Size mappings - Contabo offers incredible specs for the price
# Bash 3.2 compatible - use function instead of associative array
_contabo_get_size() {
  local size="$1"
  case "$size" in
    tiny | small) echo "vps-s-ssd" ;; # 4 vCPU, 8GB, 50GB NVMe - €4.50/mo
    medium) echo "vps-m-ssd" ;;       # 6 vCPU, 16GB, 100GB NVMe - €8.99/mo
    large) echo "vps-l-ssd" ;;        # 8 vCPU, 30GB, 200GB NVMe - €14.99/mo
    xlarge) echo "vps-xl-ssd" ;;      # 10 vCPU, 60GB, 400GB NVMe - €26.99/mo
    *) echo "vps-s-ssd" ;;            # default
  esac
}

# ============================================================================
# Provider Functions
# ============================================================================

provider_contabo_init() {
  log_info "Initializing Contabo provider..."

  # Contabo API info
  log_info "Contabo API requires:"
  log_info "  - Client ID"
  log_info "  - Client Secret"
  log_info "  - API User"
  log_info "  - API Password"
  log_info ""
  log_info "Get credentials from: https://my.contabo.com/api/details"

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  read -rp "Client ID: " client_id
  read -rp "Client Secret: " client_secret
  read -rp "API User: " api_user
  read -rsp "API Password: " api_password
  echo

  cat >"$config_dir/contabo.yml" <<EOF
provider: contabo
client_id: "$client_id"
client_secret: "$client_secret"
api_user: "$api_user"
api_password: "$api_password"
default_region: EU
default_size: small
notes: |
  Best value provider - incredible specs for price
  Small plan: 4 vCPU, 8GB RAM, 50GB NVMe for ~\$5/mo
  Note: Setup fees may apply, longer provisioning times
EOF

  log_success "Contabo provider initialized"
}

provider_contabo_validate() {
  local config_file="${HOME}/.nself/providers/contabo.yml"

  if [[ ! -f "$config_file" ]]; then
    log_error "Contabo not configured. Run: nself provider init contabo"
    return 1
  fi

  log_info "Validating Contabo credentials..."

  # Get OAuth token
  local client_id client_secret api_user api_password
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  client_secret=$(grep "client_secret:" "$config_file" | cut -d'"' -f2)
  api_user=$(grep "api_user:" "$config_file" | cut -d'"' -f2)
  api_password=$(grep "api_password:" "$config_file" | cut -d'"' -f2)

  local token_response
  token_response=$(curl -s -X POST "https://auth.contabo.com/auth/realms/contabo/protocol/openid-connect/token" \
    -d "client_id=$client_id" \
    -d "client_secret=$client_secret" \
    -d "username=$api_user" \
    -d "password=$api_password" \
    -d "grant_type=password" 2>/dev/null)

  if echo "$token_response" | grep -q "access_token"; then
    log_success "Contabo credentials valid"
    return 0
  else
    log_error "Contabo credentials invalid"
    return 1
  fi
}

provider_contabo_list_regions() {
  printf "%-15s %-30s\n" "REGION" "LOCATION"
  printf "%-15s %-30s\n" "------" "--------"
  printf "%-15s %-30s\n" "EU" "Germany (Nuremberg)"
  printf "%-15s %-30s\n" "US-central" "USA (St. Louis)"
  printf "%-15s %-30s\n" "US-east" "USA (New York)"
  printf "%-15s %-30s\n" "US-west" "USA (Seattle)"
  printf "%-15s %-30s\n" "SIN" "Singapore"
  printf "%-15s %-30s\n" "JPN" "Japan (Tokyo)"
  printf "%-15s %-30s\n" "AUS" "Australia (Sydney)"
}

provider_contabo_list_sizes() {
  printf "%-10s %-15s %-8s %-10s %-12s %-15s\n" "SIZE" "PLAN" "vCPU" "RAM (GB)" "DISK (GB)" "EST. COST"
  printf "%-10s %-15s %-8s %-10s %-12s %-15s\n" "----" "----" "----" "--------" "---------" "---------"
  printf "%-10s %-15s %-8s %-10s %-12s %-15s\n" "small" "VPS S SSD" "4" "8" "50" "€4.50/mo"
  printf "%-10s %-15s %-8s %-10s %-12s %-15s\n" "medium" "VPS M SSD" "6" "16" "100" "€8.99/mo"
  printf "%-10s %-15s %-8s %-10s %-12s %-15s\n" "large" "VPS L SSD" "8" "30" "200" "€14.99/mo"
  printf "%-10s %-15s %-8s %-10s %-12s %-15s\n" "xlarge" "VPS XL SSD" "10" "60" "400" "€26.99/mo"
  echo ""
  log_info "Note: Contabo offers exceptional specs at budget prices"
  log_info "Setup fees may apply for new accounts"
}

provider_contabo_provision() {
  local name=""
  local size="small"
  local region="EU"
  local ssh_key=""

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
      --region)
        region="$2"
        shift 2
        ;;
      --ssh-key)
        ssh_key="$2"
        shift 2
        ;;
      *) shift ;;
    esac
  done

  if [[ -z "$name" ]]; then
    log_error "Server name required: --name <name>"
    return 1
  fi

  local config_file="${HOME}/.nself/providers/contabo.yml"
  if [[ ! -f "$config_file" ]]; then
    log_error "Contabo not configured"
    return 1
  fi

  log_info "Provisioning Contabo VPS..."
  log_info "  Name: $name"
  log_info "  Size: $size"
  log_info "  Region: $region"
  log_warning "Note: Contabo provisioning can take 1-24 hours"

  # Get OAuth token
  local client_id client_secret api_user api_password
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  client_secret=$(grep "client_secret:" "$config_file" | cut -d'"' -f2)
  api_user=$(grep "api_user:" "$config_file" | cut -d'"' -f2)
  api_password=$(grep "api_password:" "$config_file" | cut -d'"' -f2)

  local token
  token=$(curl -s -X POST "https://auth.contabo.com/auth/realms/contabo/protocol/openid-connect/token" \
    -d "client_id=$client_id" \
    -d "client_secret=$client_secret" \
    -d "username=$api_user" \
    -d "password=$api_password" \
    -d "grant_type=password" 2>/dev/null | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)

  if [[ -z "$token" ]]; then
    log_error "Failed to authenticate with Contabo"
    return 1
  fi

  # Create instance via API
  local response
  response=$(curl -s -X POST "https://api.contabo.com/v1/compute/instances" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -H "x-request-id: $(uuidgen 2>/dev/null || echo $RANDOM)" \
    -d "{
      \"displayName\": \"$name\",
      \"productId\": \"$(_contabo_get_size "$size")\",
      \"region\": \"$region\"
    }" 2>/dev/null)

  if echo "$response" | grep -q "instanceId"; then
    local instance_id
    instance_id=$(echo "$response" | grep -o '"instanceId":[0-9]*' | cut -d':' -f2)
    log_success "VPS ordered: $instance_id"
    log_info "Check status: nself provider status contabo $instance_id"
    log_warning "Provisioning may take 1-24 hours"
  else
    log_error "Failed to create VPS"
    echo "$response"
    return 1
  fi
}

provider_contabo_destroy() {
  local instance_id="$1"

  log_warning "Destroying Contabo instance: $instance_id"
  log_warning "This will cancel your VPS subscription"

  local config_file="${HOME}/.nself/providers/contabo.yml"
  local client_id client_secret api_user api_password
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  client_secret=$(grep "client_secret:" "$config_file" | cut -d'"' -f2)
  api_user=$(grep "api_user:" "$config_file" | cut -d'"' -f2)
  api_password=$(grep "api_password:" "$config_file" | cut -d'"' -f2)

  local token
  token=$(curl -s -X POST "https://auth.contabo.com/auth/realms/contabo/protocol/openid-connect/token" \
    -d "client_id=$client_id" \
    -d "client_secret=$client_secret" \
    -d "username=$api_user" \
    -d "password=$api_password" \
    -d "grant_type=password" 2>/dev/null | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)

  curl -s -X POST "https://api.contabo.com/v1/compute/instances/$instance_id/actions/cancel" \
    -H "Authorization: Bearer $token" \
    -H "x-request-id: $(uuidgen 2>/dev/null || echo $RANDOM)" 2>/dev/null

  log_success "Cancellation requested for instance $instance_id"
}

provider_contabo_status() {
  local instance_id="${1:-}"

  local config_file="${HOME}/.nself/providers/contabo.yml"
  local client_id client_secret api_user api_password
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  client_secret=$(grep "client_secret:" "$config_file" | cut -d'"' -f2)
  api_user=$(grep "api_user:" "$config_file" | cut -d'"' -f2)
  api_password=$(grep "api_password:" "$config_file" | cut -d'"' -f2)

  local token
  token=$(curl -s -X POST "https://auth.contabo.com/auth/realms/contabo/protocol/openid-connect/token" \
    -d "client_id=$client_id" \
    -d "client_secret=$client_secret" \
    -d "username=$api_user" \
    -d "password=$api_password" \
    -d "grant_type=password" 2>/dev/null | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)

  if [[ -n "$instance_id" ]]; then
    curl -s "https://api.contabo.com/v1/compute/instances/$instance_id" \
      -H "Authorization: Bearer $token" \
      -H "x-request-id: $(uuidgen 2>/dev/null || echo $RANDOM)" 2>/dev/null
  else
    curl -s "https://api.contabo.com/v1/compute/instances" \
      -H "Authorization: Bearer $token" \
      -H "x-request-id: $(uuidgen 2>/dev/null || echo $RANDOM)" 2>/dev/null
  fi
}

provider_contabo_list() {
  provider_contabo_status
}

provider_contabo_ssh() {
  local instance_id="$1"
  shift

  local ip
  ip=$(provider_contabo_get_ip "$instance_id")

  if [[ -z "$ip" ]]; then
    log_error "Could not get IP for instance"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "root@${ip}" "$@"
}

provider_contabo_get_ip() {
  local instance_id="$1"

  local status
  status=$(provider_contabo_status "$instance_id" 2>/dev/null)

  echo "$status" | grep -o '"ipv4":"[^"]*"' | head -1 | cut -d'"' -f4
}

provider_contabo_estimate_cost() {
  local size="${1:-small}"

  case "$size" in
    tiny | small) echo "5" ;;
    medium) echo "9" ;;
    large) echo "15" ;;
    xlarge) echo "27" ;;
    *) echo "5" ;;
  esac
}

# Export functions
export -f provider_contabo_init provider_contabo_validate
export -f provider_contabo_list_regions provider_contabo_list_sizes
export -f provider_contabo_provision provider_contabo_destroy
export -f provider_contabo_status provider_contabo_list provider_contabo_ssh
export -f provider_contabo_get_ip provider_contabo_estimate_cost
