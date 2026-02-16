#!/usr/bin/env bash
# vultr.sh - Vultr provider module
# Supports Instances, VKE (Vultr Kubernetes Engine)


PROVIDER_NAME="vultr"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Vultr"

VULTR_DEFAULT_REGION="${VULTR_DEFAULT_REGION:-ewr}"

provider_vultr_init() {
  log_info "Initializing Vultr provider..."

  if ! command -v vultr-cli &>/dev/null; then
    log_error "vultr-cli not found"
    log_info "Install: https://github.com/vultr/vultr-cli"
    log_info "  brew install vultr/vultr-cli/vultr-cli  # macOS"
    return 1
  fi

  printf "Vultr API Key: "
  read -rs api_key
  echo

  export VULTR_API_KEY="$api_key"

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  cat >"$config_dir/vultr.yml" <<EOF
provider: vultr
configured: true
api_key: $api_key
default_region: ${VULTR_DEFAULT_REGION}
default_size: small
notes: |
  Vultr features:
  - High-performance cloud
  - 32 global locations
  - VKE: Free cluster management
EOF

  log_success "Vultr provider initialized"
}

provider_vultr_validate() {
  if ! command -v vultr-cli &>/dev/null; then
    log_error "vultr-cli not installed"
    return 1
  fi

  if [[ -z "${VULTR_API_KEY:-}" ]]; then
    log_error "VULTR_API_KEY not set"
    return 1
  fi

  if vultr-cli account get &>/dev/null; then
    log_success "Vultr credentials valid"
    return 0
  else
    log_error "Vultr credentials invalid"
    return 1
  fi
}

provider_vultr_list_regions() {
  if command -v vultr-cli &>/dev/null; then
    vultr-cli regions list 2>/dev/null || log_warning "Unable to fetch regions"
  fi
}

provider_vultr_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "PLAN" "vCPU" "RAM" "COST"
  printf "%-15s %-6s %-10s %-15s\n" "vc2-1c-1gb" "1" "1GB" "\$6/mo"
  printf "%-15s %-6s %-10s %-15s\n" "vc2-1c-2gb" "1" "2GB" "\$12/mo"
  printf "%-15s %-6s %-10s %-15s\n" "vc2-2c-4gb" "2" "4GB" "\$24/mo"
  printf "%-15s %-6s %-10s %-15s\n" "vc2-4c-8gb" "4" "8GB" "\$48/mo"
  echo ""
  echo "VKE: Free cluster management"
}

provider_vultr_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local region="${3:-${VULTR_DEFAULT_REGION}}"

  if ! provider_vultr_validate &>/dev/null; then
    return 1
  fi

  local plan
  case "$size" in
    tiny) plan="vc2-1c-1gb" ;;
    small) plan="vc2-1c-2gb" ;;
    medium) plan="vc2-2c-4gb" ;;
    large) plan="vc2-4c-8gb" ;;
    xlarge) plan="vc2-8c-16gb" ;;
    *) plan="vc2-1c-2gb" ;;
  esac

  log_info "Provisioning Vultr instance: $name ($plan) in $region..."
  log_info "Use vultr-cli for full control"

  return 1
}

provider_vultr_destroy() {
  local instance_id="$1"
  vultr-cli instance delete "$instance_id"
  log_success "Instance deleted"
}

provider_vultr_status() {
  vultr-cli instance list
}

provider_vultr_list() {
  vultr-cli instance list
}

provider_vultr_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_vultr_get_ip() {
  local instance_id="$1"
  vultr-cli instance get "$instance_id" | grep "Main IP" | awk '{print $3}' 2>/dev/null
}

provider_vultr_estimate_cost() {
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

# VKE Support
provider_vultr_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local region="${2:-${VULTR_DEFAULT_REGION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  if ! command -v vultr-cli &>/dev/null; then
    log_error "vultr-cli required for VKE cluster creation"
    return 1
  fi

  local plan
  case "$node_size" in
    small) plan="vc2-2c-4gb" ;;
    medium) plan="vc2-4c-8gb" ;;
    large) plan="vc2-8c-16gb" ;;
    xlarge) plan="vc2-16c-32gb" ;;
    *) plan="vc2-4c-8gb" ;;
  esac

  log_info "Creating VKE cluster: $cluster_name in $region"
  log_info "  Node count: $node_count"
  log_info "  Node plan: $plan"

  vultr-cli kubernetes create \
    --label "$cluster_name" \
    --region "$region" \
    --version "v1.28.2+1" \
    --node-pool "quantity=${node_count},plan=${plan},label=nodepool"

  if [[ $? -eq 0 ]]; then
    log_success "VKE cluster created successfully"
    return 0
  else
    log_error "Failed to create VKE cluster"
    return 1
  fi
}

provider_vultr_k8s_delete() {
  local cluster_id="$1"

  if ! command -v vultr-cli &>/dev/null; then
    log_error "vultr-cli required"
    return 1
  fi

  log_warning "Deleting VKE cluster: $cluster_id"
  vultr-cli kubernetes delete "$cluster_id"
  log_success "VKE cluster deleted"
}

provider_vultr_k8s_kubeconfig() {
  local cluster_id="$1"

  if ! command -v vultr-cli &>/dev/null; then
    log_error "vultr-cli required"
    return 1
  fi

  log_info "Retrieving kubeconfig for VKE cluster: $cluster_id"

  vultr-cli kubernetes config "$cluster_id" > "${HOME}/.kube/config"

  log_success "Kubeconfig saved to ~/.kube/config"
  log_info "Test connection: kubectl cluster-info"
}

export -f provider_vultr_init provider_vultr_validate
export -f provider_vultr_list_regions provider_vultr_list_sizes
export -f provider_vultr_provision provider_vultr_destroy
export -f provider_vultr_status provider_vultr_list provider_vultr_ssh
export -f provider_vultr_get_ip provider_vultr_estimate_cost
export -f provider_vultr_k8s_create provider_vultr_k8s_delete provider_vultr_k8s_kubeconfig
