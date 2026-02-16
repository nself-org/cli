#!/usr/bin/env bash
# linode.sh - Linode (Akamai) provider module
# Supports Linodes, LKE (Linode Kubernetes Engine)


PROVIDER_NAME="linode"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Linode"

LINODE_DEFAULT_REGION="${LINODE_DEFAULT_REGION:-us-east}"

provider_linode_init() {
  log_info "Initializing Linode provider..."

  if ! command -v linode-cli &>/dev/null; then
    log_error "linode-cli not found"
    log_info "Install: pip3 install linode-cli"
    return 1
  fi

  if linode-cli account view &>/dev/null; then
    log_info "linode-cli already configured"
  else
    log_info "Running Linode configuration..."
    linode-cli configure
  fi

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  cat >"$config_dir/linode.yml" <<EOF
provider: linode
configured: true
default_region: ${LINODE_DEFAULT_REGION}
default_size: small
notes: |
  Linode features:
  - Competitive pricing
  - 40Gbps network
  - LKE: Free cluster management, pay for nodes only
EOF

  log_success "Linode provider initialized"
}

provider_linode_validate() {
  if ! command -v linode-cli &>/dev/null; then
    log_error "linode-cli not installed"
    return 1
  fi

  if linode-cli account view &>/dev/null; then
    log_success "Linode credentials valid"
    return 0
  else
    log_error "Linode credentials invalid"
    log_info "Run: linode-cli configure"
    return 1
  fi
}

provider_linode_list_regions() {
  if command -v linode-cli &>/dev/null; then
    linode-cli regions list 2>/dev/null || log_warning "Unable to fetch regions"
  fi
}

provider_linode_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "PLAN" "vCPU" "RAM" "COST"
  printf "%-15s %-6s %-10s %-15s\n" "nanode-1" "1" "1GB" "\$5/mo"
  printf "%-15s %-6s %-10s %-15s\n" "g6-standard-1" "1" "2GB" "\$12/mo"
  printf "%-15s %-6s %-10s %-15s\n" "g6-standard-2" "2" "4GB" "\$24/mo"
  printf "%-15s %-6s %-10s %-15s\n" "g6-standard-4" "4" "8GB" "\$48/mo"
  echo ""
  echo "LKE: Free cluster management, pay for nodes only"
}

provider_linode_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local region="${3:-${LINODE_DEFAULT_REGION}}"

  if ! provider_linode_validate &>/dev/null; then
    return 1
  fi

  local plan_type
  case "$size" in
    tiny) plan_type="g6-nanode-1" ;;
    small) plan_type="g6-standard-1" ;;
    medium) plan_type="g6-standard-2" ;;
    large) plan_type="g6-standard-4" ;;
    xlarge) plan_type="g6-standard-8" ;;
    *) plan_type="g6-standard-1" ;;
  esac

  log_info "Provisioning Linode: $name ($plan_type) in $region..."
  log_info "Use linode-cli or Cloud Manager for full control"
  log_info "  linode-cli linodes create --label $name --type $plan_type --region $region"

  return 1
}

provider_linode_destroy() {
  local instance_id="$1"
  linode-cli linodes delete "$instance_id"
  log_success "Linode deleted"
}

provider_linode_status() {
  linode-cli linodes list
}

provider_linode_list() {
  linode-cli linodes list --format "id,label,region,type,status,ipv4"
}

provider_linode_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_linode_get_ip() {
  local instance_id="$1"
  linode-cli linodes view "$instance_id" --format ipv4 --no-headers 2>/dev/null
}

provider_linode_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "5" ;;
    small) echo "12" ;;
    medium) echo "24" ;;
    large) echo "48" ;;
    xlarge) echo "96" ;;
    *) echo "12" ;;
  esac
}

# LKE Support
provider_linode_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local region="${2:-${LINODE_DEFAULT_REGION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  if ! command -v linode-cli &>/dev/null; then
    log_error "linode-cli required for LKE cluster creation"
    return 1
  fi

  local node_type
  case "$node_size" in
    small) node_type="g6-standard-2" ;;
    medium) node_type="g6-standard-4" ;;
    large) node_type="g6-standard-8" ;;
    xlarge) node_type="g6-standard-16" ;;
    *) node_type="g6-standard-4" ;;
  esac

  log_info "Creating LKE cluster: $cluster_name in $region"
  log_info "  Node count: $node_count"
  log_info "  Node type: $node_type"
  log_info "This may take 5-10 minutes..."

  linode-cli lke cluster-create \
    --label "$cluster_name" \
    --region "$region" \
    --k8s_version "1.28" \
    --node_pools.type "$node_type" \
    --node_pools.count "$node_count"

  if [[ $? -eq 0 ]]; then
    log_success "LKE cluster created successfully"

    # Get cluster ID
    local cluster_id
    cluster_id=$(linode-cli lke clusters-list --label "$cluster_name" --format id --no-headers 2>/dev/null | head -1)

    # Download kubeconfig
    if [[ -n "$cluster_id" ]]; then
      linode-cli lke kubeconfig-view "$cluster_id" --no-headers --format kubeconfig | base64 -d > "${HOME}/.kube/config-linode-${cluster_name}"
      log_success "Kubeconfig saved to ~/.kube/config-linode-${cluster_name}"
      log_info "Merge with: export KUBECONFIG=~/.kube/config:~/.kube/config-linode-${cluster_name}"
    fi

    return 0
  else
    log_error "Failed to create LKE cluster"
    return 1
  fi
}

provider_linode_k8s_delete() {
  local cluster_name="$1"

  if ! command -v linode-cli &>/dev/null; then
    log_error "linode-cli required"
    return 1
  fi

  # Get cluster ID
  local cluster_id
  cluster_id=$(linode-cli lke clusters-list --label "$cluster_name" --format id --no-headers 2>/dev/null | head -1)

  if [[ -z "$cluster_id" ]]; then
    log_error "Cluster not found: $cluster_name"
    return 1
  fi

  log_warning "Deleting LKE cluster: $cluster_name (ID: $cluster_id)"
  linode-cli lke cluster-delete "$cluster_id"

  log_success "LKE cluster deleted"
}

provider_linode_k8s_kubeconfig() {
  local cluster_name="$1"

  if ! command -v linode-cli &>/dev/null; then
    log_error "linode-cli required"
    return 1
  fi

  # Get cluster ID
  local cluster_id
  cluster_id=$(linode-cli lke clusters-list --label "$cluster_name" --format id --no-headers 2>/dev/null | head -1)

  if [[ -z "$cluster_id" ]]; then
    log_error "Cluster not found: $cluster_name"
    return 1
  fi

  log_info "Retrieving kubeconfig for LKE cluster: $cluster_name"

  linode-cli lke kubeconfig-view "$cluster_id" --no-headers --format kubeconfig | base64 -d > "${HOME}/.kube/config"

  log_success "Kubeconfig saved to ~/.kube/config"
  log_info "Test connection: kubectl cluster-info"
}

export -f provider_linode_init provider_linode_validate
export -f provider_linode_list_regions provider_linode_list_sizes
export -f provider_linode_provision provider_linode_destroy
export -f provider_linode_status provider_linode_list provider_linode_ssh
export -f provider_linode_get_ip provider_linode_estimate_cost
export -f provider_linode_k8s_create provider_linode_k8s_delete provider_linode_k8s_kubeconfig
