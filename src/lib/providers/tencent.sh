#!/usr/bin/env bash
# tencent.sh - Tencent Cloud provider (TKE Kubernetes, Asia coverage)

PROVIDER_NAME="tencent"

set -euo pipefail


provider_tencent_init() {
  log_info "Initializing Tencent Cloud provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Tencent Cloud Secret ID: "
  read -r secret_id
  read -rsp "Tencent Cloud Secret Key: " secret_key
  echo
  printf "Default Region [ap-guangzhou]: "
  read -r region
  region="${region:-ap-guangzhou}"

  cat >"$config_dir/tencent.yml" <<EOF
provider: tencent
secret_id: "$secret_id"
secret_key: "$secret_key"
default_region: "$region"
default_size: small
EOF

  log_success "Tencent Cloud provider initialized"
}

provider_tencent_validate() {
  local config_file="${HOME}/.nself/providers/tencent.yml"
  if [[ -f "$config_file" ]]; then
    log_success "Tencent Cloud configured"
    return 0
  fi
  log_error "Tencent Cloud not configured. Run: nself provider init tencent"
  return 1
}

provider_tencent_list_regions() {
  printf "%-15s %-25s\n" "REGION" "LOCATION"
  echo "--- China Regions ---"
  printf "%-15s %-25s\n" "ap-guangzhou" "Guangzhou, China"
  printf "%-15s %-25s\n" "ap-shanghai" "Shanghai, China"
  printf "%-15s %-25s\n" "ap-beijing" "Beijing, China"
  printf "%-15s %-25s\n" "ap-chengdu" "Chengdu, China"
  printf "%-15s %-25s\n" "ap-hongkong" "Hong Kong"
  echo "--- International ---"
  printf "%-15s %-25s\n" "ap-singapore" "Singapore"
  printf "%-15s %-25s\n" "ap-bangkok" "Bangkok, Thailand"
  printf "%-15s %-25s\n" "ap-tokyo" "Tokyo, Japan"
  printf "%-15s %-25s\n" "ap-mumbai" "Mumbai, India"
  printf "%-15s %-25s\n" "eu-frankfurt" "Frankfurt, Germany"
  printf "%-15s %-25s\n" "na-siliconvalley" "Silicon Valley, USA"
}

provider_tencent_list_sizes() {
  printf "%-15s %-6s %-8s %-12s\n" "INSTANCE" "vCPU" "RAM" "EST. COST"
  printf "%-15s %-6s %-8s %-12s\n" "S5.SMALL1" "1" "1GB" "~\$5/mo"
  printf "%-15s %-6s %-8s %-12s\n" "S5.SMALL2" "1" "2GB" "~\$10/mo"
  printf "%-15s %-6s %-8s %-12s\n" "S5.MEDIUM4" "2" "4GB" "~\$25/mo"
  printf "%-15s %-6s %-8s %-12s\n" "S5.LARGE8" "4" "8GB" "~\$50/mo"
  printf "%-15s %-6s %-8s %-12s\n" "S5.2XLARGE16" "8" "16GB" "~\$100/mo"
  echo ""
  echo "Note: TKE (Tencent Kubernetes Engine) available in all regions."
}

provider_tencent_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local region="${3:-ap-guangzhou}"

  # Map size to instance type
  local instance_type
  case "$size" in
    tiny) instance_type="S5.SMALL1" ;;
    small) instance_type="S5.SMALL2" ;;
    medium) instance_type="S5.MEDIUM4" ;;
    large) instance_type="S5.LARGE8" ;;
    xlarge) instance_type="S5.2XLARGE16" ;;
    *) instance_type="S5.SMALL2" ;;
  esac

  log_info "Provisioning Tencent CVM: $name ($instance_type) in $region..."
  log_warning "Tencent Cloud requires tccli. Use CLI or console:"
  log_info "  tccli cvm RunInstances --InstanceName $name --InstanceType $instance_type"
  log_info "  Or visit: https://console.cloud.tencent.com/cvm"
  return 1
}

provider_tencent_destroy() {
  local instance_id="$1"
  log_info "To destroy Tencent CVM:"
  log_info "  tccli cvm TerminateInstances --InstanceIds.0 $instance_id"
  return 1
}

provider_tencent_status() {
  local instance_id="$1"
  log_info "To check Tencent CVM status:"
  log_info "  tccli cvm DescribeInstancesStatus --InstanceIds.0 $instance_id"
}

provider_tencent_list() {
  log_info "To list Tencent CVM instances:"
  log_info "  tccli cvm DescribeInstances"
}

provider_tencent_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_tencent_get_ip() {
  echo "$1"
}

provider_tencent_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "5" ;;
    small) echo "10" ;;
    medium) echo "25" ;;
    large) echo "50" ;;
    xlarge) echo "100" ;;
    *) echo "10" ;;
  esac
}

# Kubernetes support via TKE
provider_tencent_k8s_supported() {
  echo "true"
}

provider_tencent_k8s_create() {
  local name="${1:-nself-cluster}"
  local region="${2:-ap-guangzhou}"

  log_info "Creating Tencent TKE cluster: $name in $region..."
  log_warning "TKE cluster creation requires tccli or console:"
  log_info "  tccli tke CreateCluster --ClusterName $name"
  log_info "  Or visit: https://console.cloud.tencent.com/tke"
  return 1
}

provider_tencent_k8s_delete() {
  local cluster_id="$1"
  log_info "To delete TKE cluster:"
  log_info "  tccli tke DeleteCluster --ClusterId $cluster_id"
}

provider_tencent_k8s_kubeconfig() {
  local cluster_id="$1"
  log_info "To get TKE kubeconfig:"
  log_info "  tccli tke DescribeClusterKubeconfig --ClusterId $cluster_id"
}

# Export functions
export -f provider_tencent_init provider_tencent_validate provider_tencent_list_regions
export -f provider_tencent_list_sizes provider_tencent_provision provider_tencent_destroy
export -f provider_tencent_status provider_tencent_list provider_tencent_ssh
export -f provider_tencent_get_ip provider_tencent_estimate_cost
export -f provider_tencent_k8s_supported provider_tencent_k8s_create
export -f provider_tencent_k8s_delete provider_tencent_k8s_kubeconfig
