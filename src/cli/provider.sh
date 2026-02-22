#!/usr/bin/env bash
# provider.sh - Unified provider infrastructure management
# Part of nself v0.9.6 - Infrastructure Everywhere
# Consolidates: providers, provision, servers commands
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared utilities (same pattern as other CLI commands)
source "${SCRIPT_DIR}/../lib/utils/display.sh" 2>/dev/null || true
source "${SCRIPT_DIR}/../lib/utils/env.sh" 2>/dev/null || true

# Fallback logging if display.sh failed to load
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m✓\033[0m %s\n" "$1"; }
fi
if ! declare -f log_warning >/dev/null 2>&1; then
  log_warning() { printf "\033[0;33m!\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m✗\033[0m %s\n" "$1" >&2; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34mℹ\033[0m %s\n" "$1"; }
fi

# Source provider interface
source "${SCRIPT_DIR}/../lib/providers/provider-interface.sh" 2>/dev/null || true

# Server registry file
SERVERS_FILE="${HOME}/.nself/servers.yml"

show_provider_help() {
  cat <<'EOF'
nself provider - Unified Provider Infrastructure Management

USAGE:
  nself provider <subcommand> [options]

SUBCOMMANDS:
  info              Provider information and management
    list            List all supported providers
    init            Initialize/configure a provider
    validate        Validate provider configuration
    show            Show provider details and pricing

  server            Server management
    create          Create/provision new server
    destroy         Destroy/terminate server
    list            List all servers
    status          Show server status
    ssh             SSH into server
    add             Register existing server
    remove          Unregister server

  deploy            Deploy nself stack to server
    quick           Quick deploy with defaults
    full            Full deployment with all services

  cost              Cost estimation and comparison
    estimate        Estimate monthly costs
    compare         Compare providers for your needs

PROVIDER CATEGORIES:
  Major Cloud:      aws, gcp, azure, oracle, ibm
  Developer Cloud:  digitalocean, linode, vultr, scaleway, upcloud
  Budget EU:        hetzner, ovh, ionos, contabo, netcup
  Budget Global:    hostinger, hostwinds, kamatera, ssdnodes
  Regional:         exoscale, alibaba, tencent, yandex
  Extreme Budget:   racknerd, buyvm, time4vps

EXAMPLES:
  nself provider info list              # List all 26 providers
  nself provider info init digitalocean # Configure DigitalOcean
  nself provider server create --provider hetzner --size medium
  nself provider server list            # List all your servers
  nself provider server ssh myserver    # SSH into server

  nself provider cost estimate --size medium
  nself provider cost compare --cpu 2 --ram 4

OPTIONS:
  --provider        Provider to use
  --name            Server name
  --size            Server size (tiny/small/medium/large/xlarge)
  --region          Provider region/datacenter
  -h, --help        Show this help message

See 'nself provider <subcommand> --help' for subcommand-specific options.
EOF
}

# Initialize servers file if not exists
init_servers_file() {
  if [[ ! -f "$SERVERS_FILE" ]]; then
    mkdir -p "$(dirname "$SERVERS_FILE")"
    cat >"$SERVERS_FILE" <<EOF
# nself Server Registry
# Managed servers across all providers
servers: []
EOF
  fi
}

# === PROVIDER SUBCOMMANDS ===
cmd_provider_info() {
  local action="${1:-list}"
  shift || true

  case "$action" in
    list | ls)
      cmd_provider_list "$@"
      ;;
    init | setup)
      cmd_provider_init "$@"
      ;;
    validate | check)
      cmd_provider_validate "$@"
      ;;
    show | info)
      cmd_provider_show "$@"
      ;;
    *)
      log_error "Unknown provider action: $action"
      log_info "Available: list, init, validate, show"
      return 1
      ;;
  esac
}

cmd_provider_list() {
  local category="${1:-all}"

  printf "\n=== Supported Providers ===\n\n"

  printf "%-15s %-12s %-12s %-15s\n" "PROVIDER" "CATEGORY" "K8S" "STATUS"
  printf "%s\n" "$(printf '%.0s-' {1..55})"

  # Major Cloud
  if [[ "$category" == "all" || "$category" == "major" ]]; then
    printf "\n%s\n" "Major Cloud Providers:"
    _list_provider_row "aws" "major" "EKS"
    _list_provider_row "gcp" "major" "GKE"
    _list_provider_row "azure" "major" "AKS"
    _list_provider_row "oracle" "major" "OKE"
    _list_provider_row "ibm" "major" "IKS"
  fi

  # Developer Cloud
  if [[ "$category" == "all" || "$category" == "developer" ]]; then
    printf "\n%s\n" "Developer Cloud:"
    _list_provider_row "digitalocean" "developer" "DOKS"
    _list_provider_row "linode" "developer" "LKE"
    _list_provider_row "vultr" "developer" "VKE"
    _list_provider_row "scaleway" "developer" "Kapsule"
    _list_provider_row "upcloud" "developer" "-"
  fi

  # Budget EU
  if [[ "$category" == "all" || "$category" == "budget-eu" ]]; then
    printf "\n%s\n" "Budget EU Providers:"
    _list_provider_row "hetzner" "budget-eu" "Planned"
    _list_provider_row "ovh" "budget-eu" "OVHcloud"
    _list_provider_row "ionos" "budget-eu" "-"
    _list_provider_row "contabo" "budget-eu" "-"
    _list_provider_row "netcup" "budget-eu" "-"
  fi

  # Budget Global
  if [[ "$category" == "all" || "$category" == "budget-global" ]]; then
    printf "\n%s\n" "Budget Global:"
    _list_provider_row "hostinger" "budget-global" "-"
    _list_provider_row "hostwinds" "budget-global" "-"
    _list_provider_row "kamatera" "budget-global" "-"
    _list_provider_row "ssdnodes" "budget-global" "-"
  fi

  # Regional
  if [[ "$category" == "all" || "$category" == "regional" ]]; then
    printf "\n%s\n" "Regional Providers:"
    _list_provider_row "exoscale" "regional" "SKS"
    _list_provider_row "alibaba" "regional" "ACK"
    _list_provider_row "tencent" "regional" "TKE"
    _list_provider_row "yandex" "regional" "MKS"
  fi

  # Extreme Budget
  if [[ "$category" == "all" || "$category" == "extreme-budget" ]]; then
    printf "\n%s\n" "Extreme Budget:"
    _list_provider_row "racknerd" "extreme" "-"
    _list_provider_row "buyvm" "extreme" "-"
    _list_provider_row "time4vps" "extreme" "-"
  fi

  echo ""
  printf "Total: 26 providers supported\n"
  printf "Use 'nself provider info init <provider>' to configure\n\n"
}

_list_provider_row() {
  local provider="$1"
  local category="$2"
  local k8s="$3"

  local status="available"
  local config_file="${HOME}/.nself/providers/${provider}.yml"

  if [[ -f "$config_file" ]]; then
    status="configured"
  fi

  printf "  %-13s %-12s %-12s %-15s\n" "$provider" "$category" "$k8s" "$status"
}

cmd_provider_init() {
  local provider="${1:-}"

  if [[ -z "$provider" ]]; then
    log_error "Provider name required"
    log_info "Usage: nself provider info init <provider>"
    log_info "Use 'nself provider info list' to see available providers"
    return 1
  fi

  # Check if provider exists
  local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"

  if [[ -f "$provider_file" ]]; then
    source "$provider_file"

    if type "provider_${provider}_init" &>/dev/null; then
      "provider_${provider}_init"
    else
      log_error "Provider $provider init function not found"
      return 1
    fi
  else
    log_error "Unknown provider: $provider"
    log_info "Use 'nself provider info list' to see available providers"
    return 1
  fi
}

cmd_provider_validate() {
  local provider="${1:-}"

  if [[ -z "$provider" ]]; then
    # Validate all configured providers
    log_info "Validating all configured providers..."

    local providers_dir="${HOME}/.nself/providers"
    if [[ -d "$providers_dir" ]]; then
      for config_file in "$providers_dir"/*.yml; do
        [[ -f "$config_file" ]] || continue
        local p
        p=$(basename "$config_file" .yml)

        local provider_file="${SCRIPT_DIR}/../lib/providers/${p}.sh"
        if [[ -f "$provider_file" ]]; then
          source "$provider_file"
          if type "provider_${p}_validate" &>/dev/null; then
            "provider_${p}_validate" || true
          fi
        fi
      done
    else
      log_info "No providers configured yet"
    fi
  else
    # Validate specific provider
    local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"

    if [[ -f "$provider_file" ]]; then
      source "$provider_file"

      if type "provider_${provider}_validate" &>/dev/null; then
        "provider_${provider}_validate"
      fi
    else
      log_error "Unknown provider: $provider"
      return 1
    fi
  fi
}

cmd_provider_show() {
  local provider="${1:-}"

  if [[ -z "$provider" ]]; then
    log_error "Provider name required"
    return 1
  fi

  local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"

  if [[ -f "$provider_file" ]]; then
    source "$provider_file"

    printf "\n=== %s Provider Information ===\n\n" "$(echo "$provider" | tr '[:lower:]' '[:upper:]')"

    # Show regions
    printf "Available Regions/Datacenters:\n"
    if type "provider_${provider}_list_regions" &>/dev/null; then
      "provider_${provider}_list_regions"
    fi

    echo ""

    # Show sizes
    printf "Available Sizes/Plans:\n"
    if type "provider_${provider}_list_sizes" &>/dev/null; then
      "provider_${provider}_list_sizes"
    fi

    echo ""
  else
    log_error "Unknown provider: $provider"
    return 1
  fi
}

# === SERVER SUBCOMMANDS ===
cmd_provider_server() {
  local action="${1:-list}"
  shift || true

  init_servers_file

  case "$action" in
    create | provision | new)
      cmd_server_create "$@"
      ;;
    destroy | delete | terminate)
      cmd_server_destroy "$@"
      ;;
    list | ls)
      cmd_server_list "$@"
      ;;
    status | info)
      cmd_server_status "$@"
      ;;
    ssh | connect)
      cmd_server_ssh "$@"
      ;;
    add | register)
      cmd_server_add "$@"
      ;;
    remove | unregister)
      cmd_server_remove "$@"
      ;;
    *)
      log_error "Unknown server action: $action"
      log_info "Available: create, destroy, list, status, ssh, add, remove"
      return 1
      ;;
  esac
}

cmd_server_create() {
  local provider="${PROVIDER:-}"
  local name="${NAME:-nself-server}"
  local size="${SIZE:-small}"
  local region="${REGION:-}"

  if [[ -z "$provider" ]]; then
    log_error "Provider required. Use --provider <name>"
    log_info "Example: nself provider server create --provider digitalocean --size medium"
    return 1
  fi

  local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"

  if [[ ! -f "$provider_file" ]]; then
    log_error "Unknown provider: $provider"
    return 1
  fi

  source "$provider_file"

  # Validate provider is configured
  if type "provider_${provider}_validate" &>/dev/null; then
    if ! "provider_${provider}_validate" 2>/dev/null; then
      log_error "Provider $provider not configured. Run: nself provider info init $provider"
      return 1
    fi
  fi

  log_info "Creating server with $provider..."
  log_info "  Name: $name"
  log_info "  Size: $size"
  [[ -n "$region" ]] && log_info "  Region: $region"

  # Provision server
  if type "provider_${provider}_provision" &>/dev/null; then
    local server_id
    server_id=$("provider_${provider}_provision" "$name" "$size" "$region") || return 1

    if [[ -n "$server_id" ]]; then
      # Wait for IP
      log_info "Waiting for server IP..."
      sleep 5

      local ip=""
      local attempts=0
      while [[ -z "$ip" && $attempts -lt 30 ]]; do
        ip=$("provider_${provider}_get_ip" "$server_id" 2>/dev/null) || true
        [[ -z "$ip" ]] && sleep 5
        attempts=$((attempts + 1))
      done

      if [[ -n "$ip" ]]; then
        # Register server
        _add_server_to_registry "$name" "$provider" "$server_id" "$ip" "$region" "$size"
        log_success "Server created successfully"
        log_info "  ID: $server_id"
        log_info "  IP: $ip"
        log_info "  SSH: ssh root@$ip"
      else
        log_warning "Server created but IP not available yet"
        log_info "Check status: nself provider server status $name"
      fi
    fi
  else
    log_error "Provider $provider does not support API provisioning"
    return 1
  fi
}

cmd_server_destroy() {
  local name_or_id="${1:-}"
  local force="${FORCE:-false}"

  if [[ -z "$name_or_id" ]]; then
    log_error "Server name or ID required"
    return 1
  fi

  # Find server in registry
  local server_info
  server_info=$(_get_server_from_registry "$name_or_id")

  if [[ -z "$server_info" ]]; then
    log_error "Server not found: $name_or_id"
    return 1
  fi

  local provider server_id name
  provider=$(echo "$server_info" | cut -d'|' -f1)
  server_id=$(echo "$server_info" | cut -d'|' -f2)
  name=$(echo "$server_info" | cut -d'|' -f3)

  if [[ "$force" != "true" ]]; then
    log_warning "This will destroy server: $name ($server_id)"
    printf "Are you sure? [y/N]: "
    read -r confirm
    confirm=$(echo "$confirm" | tr '[:upper:]' '[:lower:]')

    if [[ "$confirm" != "y" && "$confirm" != "yes" ]]; then
      log_info "Aborted"
      return 0
    fi
  fi

  local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"
  source "$provider_file"

  if type "provider_${provider}_destroy" &>/dev/null; then
    if "provider_${provider}_destroy" "$server_id"; then
      _remove_server_from_registry "$name"
      log_success "Server $name destroyed"
    else
      log_error "Failed to destroy server"
      return 1
    fi
  else
    log_warning "Provider does not support API destruction. Remove manually."
    _remove_server_from_registry "$name"
  fi
}

cmd_server_list() {
  init_servers_file

  printf "\n=== Registered Servers ===\n\n"
  printf "%-20s %-15s %-15s %-12s %-10s\n" "NAME" "PROVIDER" "IP" "REGION" "SIZE"
  printf "%s\n" "$(printf '%.0s-' {1..75})"

  if [[ -f "$SERVERS_FILE" ]]; then
    local in_servers=0

    while IFS= read -r line; do
      if echo "$line" | grep -q "^servers:"; then
        in_servers=1
        continue
      fi

      if [[ $in_servers -eq 1 ]] && echo "$line" | grep -q "^[[:space:]]*- name:"; then
        local name provider ip region size

        name=$(echo "$line" | grep -o 'name:[[:space:]]*[^,]*' | cut -d':' -f2 | tr -d ' ')

        # Read next few lines for other fields
        read -r next_line
        provider=$(echo "$next_line" | grep -o 'provider:[[:space:]]*[^,]*' | cut -d':' -f2 | tr -d ' ') || provider="-"
        read -r next_line
        ip=$(echo "$next_line" | grep -o 'ip:[[:space:]]*[^,]*' | cut -d':' -f2 | tr -d ' ') || ip="-"
        read -r next_line
        region=$(echo "$next_line" | grep -o 'region:[[:space:]]*[^,]*' | cut -d':' -f2 | tr -d ' ') || region="-"
        read -r next_line
        size=$(echo "$next_line" | grep -o 'size:[[:space:]]*[^,]*' | cut -d':' -f2 | tr -d ' ') || size="-"

        printf "%-20s %-15s %-15s %-12s %-10s\n" "$name" "$provider" "$ip" "$region" "$size"
      fi
    done <"$SERVERS_FILE"
  fi

  echo ""
}

cmd_server_status() {
  local name="${1:-}"

  if [[ -z "$name" ]]; then
    # Show all server statuses
    cmd_server_list
    return
  fi

  local server_info
  server_info=$(_get_server_from_registry "$name")

  if [[ -z "$server_info" ]]; then
    log_error "Server not found: $name"
    return 1
  fi

  local provider server_id ip
  provider=$(echo "$server_info" | cut -d'|' -f1)
  server_id=$(echo "$server_info" | cut -d'|' -f2)
  ip=$(echo "$server_info" | cut -d'|' -f4)

  printf "\n=== Server Status: %s ===\n\n" "$name"

  # Check connectivity
  if ping -c 1 -W 2 "$ip" >/dev/null 2>&1; then
    log_success "Server is reachable"
  else
    log_warning "Server is not reachable via ping"
  fi

  # Get provider status
  local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"
  if [[ -f "$provider_file" ]]; then
    source "$provider_file"

    if type "provider_${provider}_status" &>/dev/null; then
      "provider_${provider}_status" "$server_id"
    fi
  fi
}

cmd_server_ssh() {
  local name="${1:-}"
  shift || true

  if [[ -z "$name" ]]; then
    log_error "Server name required"
    return 1
  fi

  local server_info
  server_info=$(_get_server_from_registry "$name")

  if [[ -z "$server_info" ]]; then
    log_error "Server not found: $name"
    return 1
  fi

  local provider ip
  provider=$(echo "$server_info" | cut -d'|' -f1)
  ip=$(echo "$server_info" | cut -d'|' -f4)

  local provider_file="${SCRIPT_DIR}/../lib/providers/${provider}.sh"
  if [[ -f "$provider_file" ]]; then
    source "$provider_file"

    if type "provider_${provider}_ssh" &>/dev/null; then
      "provider_${provider}_ssh" "$ip" "$@"
    else
      ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
    fi
  else
    ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
  fi
}

cmd_server_add() {
  local ip="${1:-}"
  local name="${NAME:-}"
  local provider="${PROVIDER:-manual}"

  if [[ -z "$ip" ]]; then
    log_error "Server IP required"
    log_info "Usage: nself provider server add <ip> --name <name> --provider <provider>"
    return 1
  fi

  if [[ -z "$name" ]]; then
    name="server-$(echo "$ip" | tr '.' '-')"
  fi

  _add_server_to_registry "$name" "$provider" "manual-$ip" "$ip" "" ""

  log_success "Server registered: $name"
  log_info "  IP: $ip"
  log_info "  Provider: $provider"
}

cmd_server_remove() {
  local name="${1:-}"

  if [[ -z "$name" ]]; then
    log_error "Server name required"
    return 1
  fi

  _remove_server_from_registry "$name"
  log_success "Server unregistered: $name"
}

# === COST SUBCOMMANDS ===
cmd_provider_cost() {
  local action="${1:-estimate}"
  shift || true

  case "$action" in
    estimate)
      cmd_cost_estimate "$@"
      ;;
    compare)
      cmd_cost_compare "$@"
      ;;
    *)
      log_error "Unknown cost action: $action"
      log_info "Available: estimate, compare"
      return 1
      ;;
  esac
}

cmd_cost_estimate() {
  local size="${SIZE:-small}"

  printf "\n=== Monthly Cost Estimates (%s size) ===\n\n" "$size"
  printf "%-20s %-12s %-20s\n" "PROVIDER" "EST. COST" "NOTES"
  printf "%s\n" "$(printf '%.0s-' {1..55})"

  local providers_dir="${SCRIPT_DIR}/../lib/providers"

  for provider_file in "$providers_dir"/*.sh; do
    [[ -f "$provider_file" ]] || continue
    local provider
    provider=$(basename "$provider_file" .sh)

    [[ "$provider" == "provider-interface" ]] && continue

    source "$provider_file" 2>/dev/null || continue

    if type "provider_${provider}_estimate_cost" &>/dev/null; then
      local cost
      cost=$("provider_${provider}_estimate_cost" "$size" 2>/dev/null) || continue

      printf "%-20s \$%-11s %-20s\n" "$provider" "${cost}/mo" ""
    fi
  done | sort -t'$' -k2 -n

  echo ""
  log_info "Costs are estimates and may vary by region and configuration"
}

cmd_cost_compare() {
  local cpu="${CPU:-2}"
  local ram="${RAM:-4}"

  printf "\n=== Provider Comparison (>=%s vCPU, >=%sGB RAM) ===\n\n" "$cpu" "$ram"

  log_info "Finding best matches across all providers..."

  # This would need to query actual provider APIs or use cached data
  # For now, provide general guidance

  cat <<EOF

Recommended by use case:

DEVELOPMENT/TESTING:
  1. Hetzner CX21 (2 vCPU, 4GB) - ~€5.60/mo - Best EU value
  2. Contabo VPS S (4 vCPU, 8GB) - ~€4.50/mo - Best specs/dollar
  3. DigitalOcean Basic (2 vCPU, 4GB) - ~\$24/mo - Great DX

PRODUCTION (US):
  1. Linode Shared (2 vCPU, 4GB) - ~\$24/mo - Reliable
  2. Vultr Cloud Compute (2 vCPU, 4GB) - ~\$24/mo - Fast
  3. AWS t3.medium (2 vCPU, 4GB) - ~\$30/mo - Enterprise

PRODUCTION (EU/GDPR):
  1. Hetzner Cloud - Best value, German quality
  2. OVH - French provider, good peering
  3. Exoscale - Swiss privacy, SKS K8s

EXTREME BUDGET:
  1. RackNerd flash sales - \$10-15/YEAR
  2. BuyVM Slices - \$2-7/mo
  3. Time4VPS - €2-6/mo

HIGH PERFORMANCE:
  1. UpCloud MaxIOPS - Premium storage
  2. Vultr High Frequency - Fast single-thread
  3. Scaleway - ARM64 options

EOF
}

# === DEPLOY SUBCOMMANDS ===
cmd_provider_deploy() {
  local action="${1:-quick}"
  shift || true

  case "$action" in
    quick)
      cmd_deploy_quick "$@"
      ;;
    full)
      cmd_deploy_full "$@"
      ;;
    *)
      log_error "Unknown deploy action: $action"
      log_info "Available: quick, full"
      return 1
      ;;
  esac
}

cmd_deploy_quick() {
  local name="${1:-}"

  if [[ -z "$name" ]]; then
    log_error "Server name required"
    return 1
  fi

  local server_info
  server_info=$(_get_server_from_registry "$name")

  if [[ -z "$server_info" ]]; then
    log_error "Server not found: $name"
    return 1
  fi

  local ip
  ip=$(echo "$server_info" | cut -d'|' -f4)

  log_info "Quick deploying to $name ($ip)..."

  # Use sync command for deployment
  if [[ -f "${SCRIPT_DIR}/sync.sh" ]]; then
    "${SCRIPT_DIR}/sync.sh" "$ip"
  else
    log_info "Syncing project files..."
    rsync -avz --exclude='.git' --exclude='node_modules' \
      ./ "root@$ip:/opt/nself/"

    log_info "Starting services..."
    ssh "root@$ip" "cd /opt/nself && ./nself start"
  fi

  log_success "Deployment complete"
}

cmd_deploy_full() {
  local name="${1:-}"

  if [[ -z "$name" ]]; then
    log_error "Server name required"
    return 1
  fi

  log_info "Full deployment includes:"
  log_info "  - System updates"
  log_info "  - Docker installation"
  log_info "  - SSL certificate setup"
  log_info "  - Project deployment"
  log_info "  - Service startup"

  # Use deploy command for full deployment
  if [[ -f "${SCRIPT_DIR}/deploy.sh" ]]; then
    "${SCRIPT_DIR}/deploy.sh" "$name"
  else
    cmd_deploy_quick "$name"
  fi
}

# === HELPER FUNCTIONS ===
_add_server_to_registry() {
  local name="$1"
  local provider="$2"
  local server_id="$3"
  local ip="$4"
  local region="${5:-}"
  local size="${6:-}"

  init_servers_file

  # Append to servers file
  cat >>"$SERVERS_FILE" <<EOF
  - name: $name
    provider: $provider
    server_id: $server_id
    ip: $ip
    region: $region
    size: $size
    created: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
}

_remove_server_from_registry() {
  local name="$1"

  if [[ ! -f "$SERVERS_FILE" ]]; then
    return
  fi

  # Create temp file without the server
  local temp_file
  temp_file=$(mktemp)

  local skip=0
  while IFS= read -r line; do
    if echo "$line" | grep -q "- name: $name$"; then
      skip=6 # Skip this entry (6 lines)
      continue
    fi

    if [[ $skip -gt 0 ]]; then
      skip=$((skip - 1))
      continue
    fi

    echo "$line" >>"$temp_file"
  done <"$SERVERS_FILE"

  mv "$temp_file" "$SERVERS_FILE"
}

_get_server_from_registry() {
  local name="$1"

  if [[ ! -f "$SERVERS_FILE" ]]; then
    return
  fi

  local found=0
  local provider="" server_id="" ip="" region="" size=""

  while IFS= read -r line; do
    if echo "$line" | grep -q "- name: $name$"; then
      found=1
      continue
    fi

    if [[ $found -eq 1 ]]; then
      if echo "$line" | grep -q "provider:"; then
        provider=$(echo "$line" | cut -d':' -f2 | tr -d ' ')
      elif echo "$line" | grep -q "server_id:"; then
        server_id=$(echo "$line" | cut -d':' -f2 | tr -d ' ')
      elif echo "$line" | grep -q "ip:"; then
        ip=$(echo "$line" | cut -d':' -f2 | tr -d ' ')
      elif echo "$line" | grep -q "region:"; then
        region=$(echo "$line" | cut -d':' -f2 | tr -d ' ')
      elif echo "$line" | grep -q "size:"; then
        size=$(echo "$line" | cut -d':' -f2 | tr -d ' ')
        # Got all fields
        echo "${provider}|${server_id}|${name}|${ip}|${region}|${size}"
        return
      fi
    fi
  done <"$SERVERS_FILE"
}

# === MAIN ENTRY POINT ===
main() {
  local subcommand="${1:-}"
  shift || true

  # Parse global options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider)
        export PROVIDER="$2"
        shift 2
        ;;
      --name)
        export NAME="$2"
        shift 2
        ;;
      --size)
        export SIZE="$2"
        shift 2
        ;;
      --region)
        export REGION="$2"
        shift 2
        ;;
      --cpu)
        export CPU="$2"
        shift 2
        ;;
      --ram)
        export RAM="$2"
        shift 2
        ;;
      --force | -f)
        export FORCE="true"
        shift
        ;;
      -h | --help)
        show_provider_help
        return 0
        ;;
      *)
        break
        ;;
    esac
  done

  case "$subcommand" in
    "" | help | -h | --help)
      show_provider_help
      ;;
    info)
      cmd_provider_info "$@"
      ;;
    server | servers)
      cmd_provider_server "$@"
      ;;
    cost | costs)
      cmd_provider_cost "$@"
      ;;
    deploy)
      cmd_provider_deploy "$@"
      ;;
    # Shortcuts
    list | ls)
      cmd_provider_list "$@"
      ;;
    init)
      cmd_provider_init "$@"
      ;;
    validate)
      cmd_provider_validate "$@"
      ;;
    show)
      cmd_provider_show "$@"
      ;;
    create)
      cmd_provider_server create "$@"
      ;;
    destroy)
      cmd_provider_server destroy "$@"
      ;;
    ssh)
      cmd_provider_server ssh "$@"
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_provider_help
      return 1
      ;;
  esac
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
