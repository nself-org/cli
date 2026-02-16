#!/usr/bin/env bash
# time4vps.sh - Time4VPS provider (Lithuanian budget VPS)

PROVIDER_NAME="time4vps"

set -euo pipefail

TIME4VPS_API_URL="https://billing.time4vps.com/api"

provider_time4vps_init() {
  log_info "Initializing Time4VPS provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Time4VPS API Username (email): "
  read -r username
  read -rsp "Time4VPS API Password: " password
  echo

  cat >"$config_dir/time4vps.yml" <<EOF
provider: time4vps
username: "$username"
password: "$password"
default_location: vilnius
notes: |
  Time4VPS is a Lithuanian provider with excellent EU prices.
  Great for GDPR-compliant EU hosting at budget prices.
EOF

  log_success "Time4VPS provider initialized"
}

provider_time4vps_validate() {
  local config_file="${HOME}/.nself/providers/time4vps.yml"
  if [[ -f "$config_file" ]]; then
    log_success "Time4VPS configured"
    return 0
  fi
  log_error "Time4VPS not configured. Run: nself provider init time4vps"
  return 1
}

provider_time4vps_list_regions() {
  printf "%-15s %-25s %-20s\n" "LOCATION" "CITY" "NOTES"
  printf "%-15s %-25s %-20s\n" "vilnius" "Vilnius, Lithuania" "Primary datacenter"
  echo ""
  echo "Note: Time4VPS operates from Lithuania (EU)."
  echo "Good latency to EU, decent to US East Coast."
}

provider_time4vps_list_sizes() {
  printf "%-12s %-6s %-8s %-10s %-12s\n" "PLAN" "vCPU" "RAM" "STORAGE" "PRICE"
  echo ""
  echo "Linux VPS Plans:"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Linux1" "1" "1GB" "20GB" "€2.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Linux2" "1" "2GB" "40GB" "€3.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Linux4" "2" "4GB" "60GB" "€5.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Linux8" "4" "8GB" "80GB" "€9.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Linux16" "6" "16GB" "160GB" "€17.99/mo"
  echo ""
  echo "Container VPS (OpenVZ):"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Container1" "1" "1GB" "20GB" "€1.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Container2" "1" "2GB" "40GB" "€2.99/mo"
  echo ""
  echo "Storage VPS:"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Storage256" "1" "512MB" "256GB" "€4.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Storage512" "1" "1GB" "512GB" "€7.99/mo"
  printf "%-12s %-6s %-8s %-10s %-12s\n" "Storage1024" "2" "2GB" "1TB" "€12.99/mo"
  echo ""
  echo "Note: Annual billing offers 15-20% discount."
}

provider_time4vps_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"

  local config_file="${HOME}/.nself/providers/time4vps.yml"
  if [[ ! -f "$config_file" ]]; then
    log_error "Time4VPS not configured"
    return 1
  fi

  # Map size to plan
  local plan
  case "$size" in
    tiny) plan="Linux1" ;;
    small) plan="Linux2" ;;
    medium) plan="Linux4" ;;
    large) plan="Linux8" ;;
    xlarge) plan="Linux16" ;;
    *) plan="Linux2" ;;
  esac

  log_info "Provisioning Time4VPS: $name ($plan)..."
  log_warning "Time4VPS API requires order through portal:"
  log_info "  Visit: https://www.time4vps.com/linux-vps/"
  log_info "  Select: $plan"
  log_info "  After provisioning: nself provider server add <ip>"
  return 1
}

provider_time4vps_destroy() {
  log_warning "Manage Time4VPS servers at: https://billing.time4vps.com"
  log_info "To remove from nself: nself provider server remove <ip>"
  return 1
}

provider_time4vps_status() {
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

provider_time4vps_list() {
  local config_file="${HOME}/.nself/providers/time4vps.yml"

  local username password
  username=$(grep "username:" "$config_file" | cut -d'"' -f2)
  password=$(grep "password:" "$config_file" | cut -d'"' -f2)

  # Try API to list services
  local response
  response=$(curl -s -X GET "${TIME4VPS_API_URL}/service" \
    -u "${username}:${password}" 2>/dev/null) || true

  if [[ -n "$response" ]] && echo "$response" | grep -q "domain"; then
    printf "%-20s %-15s %-15s %-10s\n" "NAME" "IP" "PLAN" "STATUS"
    echo "$response" | grep -o '"domain"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4 | while read -r domain; do
      printf "%-20s %-15s %-15s %-10s\n" "$domain" "-" "-" "active"
    done
  else
    # Fall back to local server list
    local servers_file="${HOME}/.nself/servers.yml"
    if [[ -f "$servers_file" ]]; then
      printf "%-15s %-20s %-10s\n" "IP" "NAME" "STATUS"
      grep "provider: time4vps" -A 2 "$servers_file" 2>/dev/null | while read -r line; do
        if echo "$line" | grep -q "ip:"; then
          local ip
          ip=$(echo "$line" | cut -d':' -f2 | tr -d ' "')
          printf "%-15s %-20s %-10s\n" "$ip" "-" "manual"
        fi
      done
    else
      log_info "No Time4VPS servers found"
      log_info "Add servers: nself provider server add <ip> --provider time4vps"
    fi
  fi
}

provider_time4vps_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_time4vps_get_ip() {
  echo "$1"
}

provider_time4vps_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "3" ;;    # Linux1
    small) echo "4" ;;   # Linux2
    medium) echo "6" ;;  # Linux4
    large) echo "10" ;;  # Linux8
    xlarge) echo "18" ;; # Linux16
    *) echo "4" ;;
  esac
}

# Export functions
export -f provider_time4vps_init provider_time4vps_validate provider_time4vps_list_regions
export -f provider_time4vps_list_sizes provider_time4vps_provision provider_time4vps_destroy
export -f provider_time4vps_status provider_time4vps_list provider_time4vps_ssh
export -f provider_time4vps_get_ip provider_time4vps_estimate_cost
