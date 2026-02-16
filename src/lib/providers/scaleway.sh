#!/usr/bin/env bash
# scaleway.sh - Scaleway provider module
# Supports Instances, Kapsule (Scaleway Kubernetes)


PROVIDER_NAME="scaleway"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Scaleway"

SCALEWAY_DEFAULT_ZONE="${SCALEWAY_DEFAULT_ZONE:-fr-par-1}"

provider_scaleway_init() {
  log_info "Initializing Scaleway provider..."

  if ! command -v scw &>/dev/null; then
    log_error "scw CLI not found"
    log_info "Install: https://github.com/scaleway/scaleway-cli"
    log_info "  brew install scw  # macOS"
    log_info "  curl -s https://raw.githubusercontent.com/scaleway/scaleway-cli/master/scripts/get.sh | sh"
    return 1
  fi

  if scw config get access-key &>/dev/null; then
    log_info "scw CLI already configured"
  else
    log_info "Running Scaleway configuration..."
    scw init
  fi

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  cat >"$config_dir/scaleway.yml" <<EOF
provider: scaleway
configured: true
default_zone: ${SCALEWAY_DEFAULT_ZONE}
default_size: small
notes: |
  Scaleway features:
  - European cloud provider
  - Competitive pricing
  - Free tier available
  - Kapsule: Managed Kubernetes, free control plane
EOF

  log_success "Scaleway provider initialized"
}

provider_scaleway_validate() {
  if ! command -v scw &>/dev/null; then
    log_error "scw CLI not installed"
    return 1
  fi

  if scw account project list &>/dev/null; then
    log_success "Scaleway credentials valid"
    return 0
  else
    log_error "Scaleway credentials invalid"
    log_info "Run: scw init"
    return 1
  fi
}

provider_scaleway_list_regions() {
  if command -v scw &>/dev/null; then
    scw instance zone list 2>/dev/null || log_warning "Unable to fetch zones"
  fi
}

provider_scaleway_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "TYPE" "vCPU" "RAM" "COST"
  printf "%-15s %-6s %-10s %-15s\n" "DEV1-S" "2" "2GB" "~€8/mo"
  printf "%-15s %-6s %-10s %-15s\n" "DEV1-M" "3" "4GB" "~€15/mo"
  printf "%-15s %-6s %-10s %-15s\n" "DEV1-L" "4" "8GB" "~€24/mo"
  printf "%-15s %-6s %-10s %-15s\n" "GP1-XS" "4" "16GB" "~€45/mo"
  echo ""
  echo "Kapsule: Free control plane, pay for nodes only"
}

provider_scaleway_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local zone="${3:-${SCALEWAY_DEFAULT_ZONE}}"

  if ! provider_scaleway_validate &>/dev/null; then
    return 1
  fi

  local instance_type
  case "$size" in
    tiny) instance_type="DEV1-S" ;;
    small) instance_type="DEV1-M" ;;
    medium) instance_type="DEV1-L" ;;
    large) instance_type="GP1-XS" ;;
    xlarge) instance_type="GP1-S" ;;
    *) instance_type="DEV1-M" ;;
  esac

  log_info "Provisioning Scaleway instance: $name ($instance_type) in $zone..."

  scw instance server create \
    name="$name" \
    type="$instance_type" \
    zone="$zone" \
    image=ubuntu_jammy

  if [[ $? -eq 0 ]]; then
    log_success "Instance created successfully"
    return 0
  else
    log_error "Failed to create instance"
    return 1
  fi
}

provider_scaleway_destroy() {
  local server_id="$1"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"

  if ! command -v scw &>/dev/null; then
    log_error "scw CLI required"
    return 1
  fi

  log_warning "Deleting Scaleway instance: $server_id"
  scw instance server delete "$server_id" zone="$zone" with-ip=true with-block=true
  log_success "Instance deleted"
}

provider_scaleway_status() {
  local server_id="${1:-}"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"

  if ! command -v scw &>/dev/null; then
    log_error "scw CLI required"
    return 1
  fi

  if [[ -n "$server_id" ]]; then
    scw instance server get "$server_id" zone="$zone"
  else
    scw instance server list zone="$zone"
  fi
}

provider_scaleway_list() {
  if ! command -v scw &>/dev/null; then
    log_error "scw CLI required"
    return 1
  fi

  scw instance server list
}

provider_scaleway_ssh() {
  local server_id="$1"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"
  shift 2

  local public_ip
  public_ip=$(provider_scaleway_get_ip "$server_id" "$zone")

  if [[ -z "$public_ip" ]]; then
    log_error "Could not get IP for instance"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "root@${public_ip}" "$@"
}

provider_scaleway_get_ip() {
  local server_id="$1"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"

  scw instance server get "$server_id" zone="$zone" -o json | grep -oP '"address":\s*"\K[^"]+' | head -1 2>/dev/null
}

provider_scaleway_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "8-10" ;;
    small) echo "15-18" ;;
    medium) echo "24-28" ;;
    large) echo "45-50" ;;
    xlarge) echo "90-100" ;;
    *) echo "15-18" ;;
  esac
}

# Kapsule (Scaleway Kubernetes) Support
provider_scaleway_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  if ! command -v scw &>/dev/null; then
    log_error "scw CLI required for Kapsule cluster creation"
    return 1
  fi

  local node_type
  case "$node_size" in
    small) node_type="DEV1-M" ;;
    medium) node_type="DEV1-L" ;;
    large) node_type="GP1-XS" ;;
    xlarge) node_type="GP1-S" ;;
    *) node_type="DEV1-L" ;;
  esac

  # Extract region from zone (e.g., fr-par-1 -> fr-par)
  local region="${zone%-*}"

  log_info "Creating Kapsule cluster: $cluster_name in $region"
  log_info "  Node count: $node_count"
  log_info "  Node type: $node_type"
  log_info "This may take 5-10 minutes..."

  # Create cluster
  local cluster_id
  cluster_id=$(scw k8s cluster create \
    name="$cluster_name" \
    region="$region" \
    version="1.28.0" \
    cni=cilium \
    -o json | grep -oP '"id":\s*"\K[^"]+' | head -1)

  if [[ -z "$cluster_id" ]]; then
    log_error "Failed to create cluster"
    return 1
  fi

  log_info "Cluster created: $cluster_id"
  log_info "Creating node pool..."

  # Create node pool
  scw k8s pool create \
    cluster-id="$cluster_id" \
    name="default-pool" \
    node-type="$node_type" \
    size="$node_count" \
    min-size=1 \
    max-size=5 \
    autoscaling=true \
    region="$region"

  if [[ $? -eq 0 ]]; then
    log_success "Kapsule cluster created successfully"

    # Get kubeconfig
    scw k8s kubeconfig install "$cluster_id" region="$region"

    log_success "Kubeconfig installed"
    log_info "Test connection: kubectl cluster-info"
    return 0
  else
    log_error "Failed to create node pool"
    return 1
  fi
}

provider_scaleway_k8s_delete() {
  local cluster_name="$1"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"

  if ! command -v scw &>/dev/null; then
    log_error "scw CLI required"
    return 1
  fi

  # Extract region from zone
  local region="${zone%-*}"

  # Find cluster ID by name
  local cluster_id
  cluster_id=$(scw k8s cluster list region="$region" -o json | grep -B2 "\"name\": \"$cluster_name\"" | grep "\"id\"" | grep -oP '"\K[^"]+' | head -1)

  if [[ -z "$cluster_id" ]]; then
    log_error "Cluster not found: $cluster_name"
    return 1
  fi

  log_warning "Deleting Kapsule cluster: $cluster_name (ID: $cluster_id)"

  scw k8s cluster delete "$cluster_id" region="$region" with-additional-resources=true

  log_success "Kapsule cluster deleted"
}

provider_scaleway_k8s_kubeconfig() {
  local cluster_name="$1"
  local zone="${2:-${SCALEWAY_DEFAULT_ZONE}}"

  if ! command -v scw &>/dev/null; then
    log_error "scw CLI required"
    return 1
  fi

  # Extract region from zone
  local region="${zone%-*}"

  # Find cluster ID by name
  local cluster_id
  cluster_id=$(scw k8s cluster list region="$region" -o json | grep -B2 "\"name\": \"$cluster_name\"" | grep "\"id\"" | grep -oP '"\K[^"]+' | head -1)

  if [[ -z "$cluster_id" ]]; then
    log_error "Cluster not found: $cluster_name"
    return 1
  fi

  log_info "Retrieving kubeconfig for Kapsule cluster: $cluster_name"

  scw k8s kubeconfig install "$cluster_id" region="$region"

  log_success "Kubeconfig installed to ~/.kube/config"
  log_info "Test connection: kubectl cluster-info"
}

export -f provider_scaleway_init provider_scaleway_validate
export -f provider_scaleway_list_regions provider_scaleway_list_sizes
export -f provider_scaleway_provision provider_scaleway_destroy
export -f provider_scaleway_status provider_scaleway_list provider_scaleway_ssh
export -f provider_scaleway_get_ip provider_scaleway_estimate_cost
export -f provider_scaleway_k8s_create provider_scaleway_k8s_delete provider_scaleway_k8s_kubeconfig
