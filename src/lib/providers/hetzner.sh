#!/usr/bin/env bash
# hetzner.sh - Hetzner Cloud provider module
# Supports Cloud Servers, managed Kubernetes


PROVIDER_NAME="hetzner"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Hetzner Cloud"

HETZNER_DEFAULT_LOCATION="${HETZNER_DEFAULT_LOCATION:-nbg1}"

provider_hetzner_init() {
  log_info "Initializing Hetzner Cloud provider..."

  if ! command -v hcloud &>/dev/null; then
    log_error "hcloud CLI not found"
    log_info "Install: https://github.com/hetznercloud/cli"
    log_info "  brew install hcloud  # macOS"
    return 1
  fi

  printf "Hetzner Cloud API Token: "
  read -rs api_token
  echo

  # Create context
  hcloud context create nself --token "$api_token"
  hcloud context use nself

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  cat >"$config_dir/hetzner.yml" <<EOF
provider: hetzner
configured: true
default_location: ${HETZNER_DEFAULT_LOCATION}
default_size: small
notes: |
  Hetzner Cloud features:
  - Excellent price/performance ratio
  - European datacenter locations
  - Free bandwidth
  - Managed Kubernetes (no control plane fees)
EOF

  log_success "Hetzner Cloud provider initialized"
}

provider_hetzner_validate() {
  if ! command -v hcloud &>/dev/null; then
    log_error "hcloud CLI not installed"
    return 1
  fi

  if hcloud server list &>/dev/null; then
    log_success "Hetzner credentials valid"
    return 0
  else
    log_error "Hetzner credentials invalid"
    log_info "Run: nself infra provider init hetzner"
    return 1
  fi
}

provider_hetzner_list_regions() {
  if command -v hcloud &>/dev/null; then
    hcloud location list 2>/dev/null || log_warning "Unable to fetch locations"
  fi
}

provider_hetzner_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "TYPE" "vCPU" "RAM" "COST"
  printf "%-15s %-6s %-10s %-15s\n" "cx11" "1" "2GB" "~€3.29/mo"
  printf "%-15s %-6s %-10s %-15s\n" "cpx11" "2" "2GB" "~€4.15/mo"
  printf "%-15s %-6s %-10s %-15s\n" "cx21" "2" "4GB" "~€5.83/mo"
  printf "%-15s %-6s %-10s %-15s\n" "cpx21" "3" "4GB" "~€7.50/mo"
  printf "%-15s %-6s %-10s %-15s\n" "cx31" "2" "8GB" "~€11.05/mo"
  echo ""
  echo "Managed Kubernetes: No control plane fees"
}

provider_hetzner_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local location="${3:-${HETZNER_DEFAULT_LOCATION}}"

  if ! provider_hetzner_validate &>/dev/null; then
    return 1
  fi

  local server_type
  case "$size" in
    tiny) server_type="cx11" ;;
    small) server_type="cpx11" ;;
    medium) server_type="cx21" ;;
    large) server_type="cx31" ;;
    xlarge) server_type="cx41" ;;
    *) server_type="cpx11" ;;
  esac

  log_info "Provisioning Hetzner Cloud server: $name ($server_type) in $location..."

  hcloud server create \
    --name "$name" \
    --type "$server_type" \
    --location "$location" \
    --image ubuntu-22.04

  if [[ $? -eq 0 ]]; then
    log_success "Server created successfully"
    return 0
  else
    log_error "Failed to create server"
    return 1
  fi
}

provider_hetzner_destroy() {
  local server_name="$1"

  if ! command -v hcloud &>/dev/null; then
    log_error "hcloud CLI required"
    return 1
  fi

  log_warning "Deleting Hetzner server: $server_name"
  hcloud server delete "$server_name"
  log_success "Server deleted"
}

provider_hetzner_status() {
  local server_name="${1:-}"

  if ! command -v hcloud &>/dev/null; then
    log_error "hcloud CLI required"
    return 1
  fi

  if [[ -n "$server_name" ]]; then
    hcloud server describe "$server_name"
  else
    hcloud server list
  fi
}

provider_hetzner_list() {
  if ! command -v hcloud &>/dev/null; then
    log_error "hcloud CLI required"
    return 1
  fi

  hcloud server list
}

provider_hetzner_ssh() {
  local server_name="$1"
  shift

  local public_ip
  public_ip=$(provider_hetzner_get_ip "$server_name")

  if [[ -z "$public_ip" ]]; then
    log_error "Could not get IP for server"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "root@${public_ip}" "$@"
}

provider_hetzner_get_ip() {
  local server_name="$1"
  hcloud server ip "$server_name" 2>/dev/null
}

provider_hetzner_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "3-4" ;;
    small) echo "4-5" ;;
    medium) echo "6-7" ;;
    large) echo "11-12" ;;
    xlarge) echo "21-22" ;;
    *) echo "4-5" ;;
  esac
}

# Hetzner Managed Kubernetes Support
provider_hetzner_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local location="${2:-${HETZNER_DEFAULT_LOCATION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  if ! command -v hcloud &>/dev/null; then
    log_error "hcloud CLI required for Kubernetes cluster creation"
    return 1
  fi

  local server_type
  case "$node_size" in
    small) server_type="cpx21" ;;
    medium) server_type="cx31" ;;
    large) server_type="cx41" ;;
    xlarge) server_type="cx51" ;;
    *) server_type="cx31" ;;
  esac

  log_info "Creating Hetzner Kubernetes cluster: $cluster_name"
  log_info "  Location: $location"
  log_info "  Node count: $node_count"
  log_info "  Node type: $server_type"
  log_info "Note: Hetzner doesn't charge for control plane"

  # Note: hcloud CLI doesn't support K8s creation directly
  # Users must use Hetzner Cloud Console or API
  log_warning "Hetzner Kubernetes creation requires Cloud Console or API"
  log_info "Visit: https://console.hetzner.cloud/"
  log_info "Or use Terraform: https://registry.terraform.io/providers/hetznercloud/hcloud/"

  return 1
}

provider_hetzner_k8s_delete() {
  local cluster_name="$1"

  log_info "Hetzner Kubernetes deletion requires Cloud Console"
  log_info "Visit: https://console.hetzner.cloud/"

  return 1
}

provider_hetzner_k8s_kubeconfig() {
  local cluster_name="$1"

  log_info "Download kubeconfig from Hetzner Cloud Console"
  log_info "Visit: https://console.hetzner.cloud/"
  log_info "Then save to: ~/.kube/config"

  return 1
}

export -f provider_hetzner_init provider_hetzner_validate
export -f provider_hetzner_list_regions provider_hetzner_list_sizes
export -f provider_hetzner_provision provider_hetzner_destroy
export -f provider_hetzner_status provider_hetzner_list provider_hetzner_ssh
export -f provider_hetzner_get_ip provider_hetzner_estimate_cost
export -f provider_hetzner_k8s_create provider_hetzner_k8s_delete provider_hetzner_k8s_kubeconfig
