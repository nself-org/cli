#!/usr/bin/env bash
# kamatera.sh - Kamatera provider (excellent hourly billing)

PROVIDER_NAME="kamatera"

set -euo pipefail

KAMATERA_API_URL="https://console.kamatera.com/service/server"

provider_kamatera_init() {
  log_info "Initializing Kamatera provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Kamatera API Client ID: "
  read -r client_id
  read -rsp "Kamatera API Secret: " api_secret
  echo

  # Validate credentials
  local response
  response=$(curl -s -X GET "${KAMATERA_API_URL}/info" \
    -H "AuthClientId: ${client_id}" \
    -H "AuthSecret: ${api_secret}" 2>/dev/null) || true

  if [[ -z "$response" ]] || echo "$response" | grep -q "error"; then
    log_warning "Could not validate credentials (may still work)"
  fi

  cat >"$config_dir/kamatera.yml" <<EOF
provider: kamatera
client_id: "$client_id"
api_secret: "$api_secret"
default_datacenter: US-NY2
default_cpu: 1B
default_ram: 1024
default_disk: 20
EOF

  log_success "Kamatera provider initialized"
}

provider_kamatera_validate() {
  local config_file="${HOME}/.nself/providers/kamatera.yml"
  if [[ -f "$config_file" ]]; then
    log_success "Kamatera configured"
    return 0
  fi
  log_error "Kamatera not configured. Run: nself provider init kamatera"
  return 1
}

provider_kamatera_list_regions() {
  printf "%-12s %-25s %-15s\n" "DATACENTER" "LOCATION" "REGION"
  printf "%-12s %-25s %-15s\n" "US-NY2" "New York, USA" "North America"
  printf "%-12s %-25s %-15s\n" "US-TX" "Dallas, USA" "North America"
  printf "%-12s %-25s %-15s\n" "US-SC" "Santa Clara, USA" "North America"
  printf "%-12s %-25s %-15s\n" "EU-LO" "London, UK" "Europe"
  printf "%-12s %-25s %-15s\n" "EU-AM" "Amsterdam, NL" "Europe"
  printf "%-12s %-25s %-15s\n" "EU-FR" "Frankfurt, DE" "Europe"
  printf "%-12s %-25s %-15s\n" "AS-IL" "Tel Aviv, Israel" "Middle East"
  printf "%-12s %-25s %-15s\n" "AS-HK" "Hong Kong" "Asia"
  printf "%-12s %-25s %-15s\n" "AS-TK" "Tokyo, Japan" "Asia"
}

provider_kamatera_list_sizes() {
  printf "%-10s %-8s %-8s %-12s %-15s\n" "SIZE" "CPU" "RAM" "DISK" "EST. HOURLY"
  printf "%-10s %-8s %-8s %-12s %-15s\n" "tiny" "1B" "1GB" "20GB" "~\$0.007/hr"
  printf "%-10s %-8s %-8s %-12s %-15s\n" "small" "1B" "2GB" "30GB" "~\$0.015/hr"
  printf "%-10s %-8s %-8s %-12s %-15s\n" "medium" "2B" "4GB" "40GB" "~\$0.030/hr"
  printf "%-10s %-8s %-8s %-12s %-15s\n" "large" "4B" "8GB" "80GB" "~\$0.060/hr"
  printf "%-10s %-8s %-8s %-12s %-15s\n" "xlarge" "8B" "16GB" "160GB" "~\$0.120/hr"
  echo ""
  echo "Note: Kamatera uses hourly billing - pay only for what you use!"
  echo "CPU types: A=AMD EPYC, B=Intel Xeon, T=Burstable"
}

provider_kamatera_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local datacenter="${3:-US-NY2}"

  local config_file="${HOME}/.nself/providers/kamatera.yml"
  if [[ ! -f "$config_file" ]]; then
    log_error "Kamatera not configured"
    return 1
  fi

  local client_id api_secret
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  # Map size to Kamatera specs
  local cpu ram disk
  case "$size" in
    tiny)
      cpu="1B"
      ram="1024"
      disk="20"
      ;;
    small)
      cpu="1B"
      ram="2048"
      disk="30"
      ;;
    medium)
      cpu="2B"
      ram="4096"
      disk="40"
      ;;
    large)
      cpu="4B"
      ram="8192"
      disk="80"
      ;;
    xlarge)
      cpu="8B"
      ram="16384"
      disk="160"
      ;;
    *)
      cpu="1B"
      ram="2048"
      disk="30"
      ;;
  esac

  log_info "Provisioning Kamatera server: $name ($size) in $datacenter..."

  local response
  response=$(curl -s -X POST "${KAMATERA_API_URL}/create" \
    -H "AuthClientId: ${client_id}" \
    -H "AuthSecret: ${api_secret}" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"${name}\",
      \"datacenter\": \"${datacenter}\",
      \"cpu\": \"${cpu}\",
      \"ram\": ${ram},
      \"disk\": [{\"size\": ${disk}}],
      \"image\": \"ubuntu_server_22.04_64-bit\",
      \"network\": [{\"name\": \"wan\", \"ip\": \"auto\"}],
      \"daily_backup\": false,
      \"managed\": false,
      \"power_on\": true
    }" 2>/dev/null)

  if echo "$response" | grep -q '"id"'; then
    local server_id
    server_id=$(echo "$response" | grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4)
    log_success "Server provisioning started. ID: $server_id"
    echo "$server_id"
    return 0
  else
    log_error "Failed to provision server: $response"
    return 1
  fi
}

provider_kamatera_destroy() {
  local server_id="$1"
  local config_file="${HOME}/.nself/providers/kamatera.yml"

  local client_id api_secret
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  log_info "Destroying Kamatera server: $server_id..."

  local response
  response=$(curl -s -X DELETE "${KAMATERA_API_URL}/${server_id}/terminate" \
    -H "AuthClientId: ${client_id}" \
    -H "AuthSecret: ${api_secret}" 2>/dev/null)

  if echo "$response" | grep -q "success\|terminated"; then
    log_success "Server $server_id destroyed"
    return 0
  else
    log_error "Failed to destroy server: $response"
    return 1
  fi
}

provider_kamatera_status() {
  local server_id="$1"
  local config_file="${HOME}/.nself/providers/kamatera.yml"

  local client_id api_secret
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${KAMATERA_API_URL}/${server_id}/info" \
    -H "AuthClientId: ${client_id}" \
    -H "AuthSecret: ${api_secret}" 2>/dev/null)

  echo "$response"
}

provider_kamatera_list() {
  local config_file="${HOME}/.nself/providers/kamatera.yml"

  local client_id api_secret
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${KAMATERA_API_URL}" \
    -H "AuthClientId: ${client_id}" \
    -H "AuthSecret: ${api_secret}" 2>/dev/null)

  if [[ -n "$response" ]]; then
    printf "%-20s %-15s %-15s %-10s\n" "NAME" "DATACENTER" "IP" "STATUS"
    echo "$response" | grep -o '"name"[^}]*' | while read -r line; do
      local name dc ip status
      name=$(echo "$line" | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4)
      printf "%-20s %-15s %-15s %-10s\n" "$name" "-" "-" "running"
    done
  fi
}

provider_kamatera_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_kamatera_get_ip() {
  local server_id="$1"
  local config_file="${HOME}/.nself/providers/kamatera.yml"

  local client_id api_secret
  client_id=$(grep "client_id:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${KAMATERA_API_URL}/${server_id}/info" \
    -H "AuthClientId: ${client_id}" \
    -H "AuthSecret: ${api_secret}" 2>/dev/null)

  echo "$response" | grep -o '"ip"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4
}

provider_kamatera_estimate_cost() {
  local size="${1:-small}"
  # Hourly rates converted to monthly (730 hours)
  case "$size" in
    tiny) echo "5" ;;
    small) echo "11" ;;
    medium) echo "22" ;;
    large) echo "44" ;;
    xlarge) echo "88" ;;
    *) echo "11" ;;
  esac
}

# Export functions
export -f provider_kamatera_init provider_kamatera_validate provider_kamatera_list_regions
export -f provider_kamatera_list_sizes provider_kamatera_provision provider_kamatera_destroy
export -f provider_kamatera_status provider_kamatera_list provider_kamatera_ssh
export -f provider_kamatera_get_ip provider_kamatera_estimate_cost
