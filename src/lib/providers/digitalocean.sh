#!/usr/bin/env bash
# digitalocean.sh - DigitalOcean provider module
# Supports Droplets, DOKS (DigitalOcean Kubernetes Service)


PROVIDER_NAME="digitalocean"

set -euo pipefail

PROVIDER_DISPLAY_NAME="DigitalOcean"

# DigitalOcean default settings
DO_DEFAULT_REGION="${DO_DEFAULT_REGION:-nyc3}"

# ============================================================================
# Provider Functions
# ============================================================================

provider_digitalocean_init() {
  log_info "Initializing DigitalOcean provider..."

  # Check for doctl CLI
  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI not found"
    log_info "Install: https://docs.digitalocean.com/reference/doctl/how-to/install/"
    log_info "  brew install doctl  # macOS"
    log_info "  snap install doctl  # Ubuntu/Linux"
    return 1
  fi

  # Check if already authenticated
  if doctl account get &>/dev/null; then
    log_info "doctl already authenticated"
    log_info "Run 'doctl auth init' to switch tokens"
  else
    log_info "Running DigitalOcean authentication..."
    doctl auth init
  fi

  # Save to nself config
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  cat >"$config_dir/digitalocean.yml" <<EOF
provider: digitalocean
configured: true
default_region: ${DO_DEFAULT_REGION}
default_size: small
notes: |
  DigitalOcean features:
  - Simple, predictable pricing
  - All droplets include SSD storage
  - Free outbound bandwidth (1TB/droplet)
  - DOKS: \$12/month per cluster + node costs
EOF

  log_success "DigitalOcean provider initialized"
}

provider_digitalocean_validate() {
  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI not installed"
    return 1
  fi

  log_info "Validating DigitalOcean credentials..."

  if doctl account get &>/dev/null; then
    log_success "DigitalOcean credentials valid"
    return 0
  else
    log_error "DigitalOcean credentials invalid or expired"
    log_info "Run: doctl auth init"
    return 1
  fi
}

provider_digitalocean_list_regions() {
  if command -v doctl &>/dev/null && doctl compute region list 2>/dev/null; then
    return 0
  fi

  # Fallback to static list
  printf "%-15s %-30s\n" "SLUG" "NAME"
  printf "%-15s %-30s\n" "----" "----"
  printf "%-15s %-30s\n" "nyc1" "New York 1"
  printf "%-15s %-30s\n" "nyc3" "New York 3"
  printf "%-15s %-30s\n" "sfo3" "San Francisco 3"
  printf "%-15s %-30s\n" "ams3" "Amsterdam 3"
  printf "%-15s %-30s\n" "sgp1" "Singapore 1"
  printf "%-15s %-30s\n" "lon1" "London 1"
  printf "%-15s %-30s\n" "fra1" "Frankfurt 1"
  printf "%-15s %-30s\n" "tor1" "Toronto 1"
  printf "%-15s %-30s\n" "blr1" "Bangalore 1"
}

provider_digitalocean_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "SLUG" "vCPU" "RAM" "COST"
  printf "%-15s %-6s %-10s %-15s\n" "----" "----" "---" "----"
  printf "%-15s %-6s %-10s %-15s\n" "s-1vcpu-1gb" "1" "1GB" "\$6/mo"
  printf "%-15s %-6s %-10s %-15s\n" "s-1vcpu-2gb" "1" "2GB" "\$12/mo"
  printf "%-15s %-6s %-10s %-15s\n" "s-2vcpu-2gb" "2" "2GB" "\$18/mo"
  printf "%-15s %-6s %-10s %-15s\n" "s-2vcpu-4gb" "2" "4GB" "\$24/mo"
  printf "%-15s %-6s %-10s %-15s\n" "s-4vcpu-8gb" "4" "8GB" "\$48/mo"
  echo ""
  echo "DOKS cluster management: \$12/month + node costs"
}

provider_digitalocean_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local region="${3:-${DO_DEFAULT_REGION}}"

  # Validate credentials
  if ! provider_digitalocean_validate &>/dev/null; then
    return 1
  fi

  # Map size to droplet size
  local droplet_size
  case "$size" in
    tiny) droplet_size="s-1vcpu-1gb" ;;
    small) droplet_size="s-1vcpu-2gb" ;;
    medium) droplet_size="s-2vcpu-4gb" ;;
    large) droplet_size="s-4vcpu-8gb" ;;
    xlarge) droplet_size="s-8vcpu-16gb" ;;
    *) droplet_size="s-1vcpu-2gb" ;;
  esac

  log_info "Provisioning DigitalOcean Droplet: $name ($droplet_size) in $region..."

  # Get latest Ubuntu image
  local image
  image=$(doctl compute image list --public --format Slug | grep "ubuntu-22-04" | head -1 || echo "ubuntu-22-04-x64")

  # Create droplet
  doctl compute droplet create "$name" \
    --size "$droplet_size" \
    --image "$image" \
    --region "$region" \
    --wait

  if [[ $? -eq 0 ]]; then
    log_success "Droplet created successfully"
    return 0
  else
    log_error "Failed to create droplet"
    return 1
  fi
}

provider_digitalocean_destroy() {
  local droplet_id="$1"

  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI required"
    return 1
  fi

  log_warning "Destroying DigitalOcean Droplet: $droplet_id"
  doctl compute droplet delete "$droplet_id" --force
  log_success "Droplet deleted"
}

provider_digitalocean_status() {
  local droplet_id="${1:-}"

  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI required"
    return 1
  fi

  if [[ -n "$droplet_id" ]]; then
    doctl compute droplet get "$droplet_id"
  else
    doctl compute droplet list
  fi
}

provider_digitalocean_list() {
  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI required"
    return 1
  fi

  doctl compute droplet list --format "ID,Name,PublicIPv4,Status,Region,Size"
}

provider_digitalocean_ssh() {
  local droplet_id="$1"
  shift

  local public_ip
  public_ip=$(provider_digitalocean_get_ip "$droplet_id")

  if [[ -z "$public_ip" ]]; then
    log_error "Could not get IP for droplet"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "root@${public_ip}" "$@"
}

provider_digitalocean_get_ip() {
  local droplet_id="$1"

  doctl compute droplet get "$droplet_id" --format PublicIPv4 --no-header 2>/dev/null
}

provider_digitalocean_estimate_cost() {
  local size="${1:-small}"

  case "$size" in
    tiny) echo "6" ;;
    small) echo "12" ;;
    medium) echo "24" ;;
    large) echo "48" ;;
    xlarge) echo "96" ;;
    *) echo "12" ;;
  esac
}

# ============================================================================
# DOKS (DigitalOcean Kubernetes Service) Support
# ============================================================================

provider_digitalocean_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local region="${2:-${DO_DEFAULT_REGION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  # Validate doctl CLI
  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI required for DOKS cluster creation"
    log_info "Install: brew install doctl"
    return 1
  fi

  # Validate credentials
  if ! provider_digitalocean_validate &>/dev/null; then
    return 1
  fi

  # Map size to node size
  local droplet_size
  case "$node_size" in
    small) droplet_size="s-2vcpu-4gb" ;;
    medium) droplet_size="s-4vcpu-8gb" ;;
    large) droplet_size="s-8vcpu-16gb" ;;
    xlarge) droplet_size="s-16vcpu-32gb" ;;
    *) droplet_size="s-4vcpu-8gb" ;;
  esac

  log_info "Creating DOKS cluster: $cluster_name in $region"
  log_info "  Node count: $node_count"
  log_info "  Node size: $droplet_size"
  log_info "  Cluster management fee: \$12/month"
  log_info "This operation can take 5-10 minutes..."

  # Create DOKS cluster
  doctl kubernetes cluster create "$cluster_name" \
    --region "$region" \
    --size "$droplet_size" \
    --count "$node_count" \
    --auto-upgrade \
    --surge-upgrade \
    --wait

  if [[ $? -eq 0 ]]; then
    log_success "DOKS cluster created successfully"

    # Get credentials automatically
    doctl kubernetes cluster kubeconfig save "$cluster_name"

    log_success "Kubeconfig updated at ~/.kube/config"
    log_info "Test connection: kubectl cluster-info"
    return 0
  else
    log_error "Failed to create DOKS cluster"
    return 1
  fi
}

provider_digitalocean_k8s_delete() {
  local cluster_name="$1"

  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI required"
    return 1
  fi

  log_warning "Deleting DOKS cluster: $cluster_name"

  # Delete cluster
  doctl kubernetes cluster delete "$cluster_name" --force

  if [[ $? -eq 0 ]]; then
    log_success "DOKS cluster deleted successfully"
    return 0
  else
    log_error "Failed to delete DOKS cluster"
    return 1
  fi
}

provider_digitalocean_k8s_kubeconfig() {
  local cluster_name="$1"

  if ! command -v doctl &>/dev/null; then
    log_error "doctl CLI required"
    return 1
  fi

  log_info "Retrieving kubeconfig for DOKS cluster: $cluster_name"

  # Get cluster credentials
  doctl kubernetes cluster kubeconfig save "$cluster_name"

  if [[ $? -eq 0 ]]; then
    log_success "Kubeconfig updated at ~/.kube/config"
    log_info "Test connection: kubectl cluster-info"
    return 0
  else
    log_error "Failed to retrieve kubeconfig"
    return 1
  fi
}

# Export functions
export -f provider_digitalocean_init provider_digitalocean_validate
export -f provider_digitalocean_list_regions provider_digitalocean_list_sizes
export -f provider_digitalocean_provision provider_digitalocean_destroy
export -f provider_digitalocean_status provider_digitalocean_list provider_digitalocean_ssh
export -f provider_digitalocean_get_ip provider_digitalocean_estimate_cost
export -f provider_digitalocean_k8s_create provider_digitalocean_k8s_delete provider_digitalocean_k8s_kubeconfig
