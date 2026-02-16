#!/usr/bin/env bash
# alibaba.sh - Alibaba Cloud provider (ACK Kubernetes, Asia-focused)

PROVIDER_NAME="alibaba"

set -euo pipefail

ALIBABA_API_URL="https://ecs.aliyuncs.com"

provider_alibaba_init() {
  log_info "Initializing Alibaba Cloud provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Alibaba Cloud Access Key ID: "
  read -r access_key_id
  read -rsp "Access Key Secret: " access_key_secret
  echo
  printf "Default Region [cn-hangzhou]: "
  read -r region
  region="${region:-cn-hangzhou}"

  cat >"$config_dir/alibaba.yml" <<EOF
provider: alibaba
access_key_id: "$access_key_id"
access_key_secret: "$access_key_secret"
default_region: "$region"
default_size: small
EOF

  log_success "Alibaba Cloud provider initialized"
}

provider_alibaba_validate() {
  local config_file="${HOME}/.nself/providers/alibaba.yml"
  if [[ -f "$config_file" ]]; then
    log_success "Alibaba Cloud configured"
    return 0
  fi
  log_error "Alibaba Cloud not configured. Run: nself provider init alibaba"
  return 1
}

provider_alibaba_list_regions() {
  printf "%-15s %-25s %-15s\n" "REGION" "LOCATION" "NOTES"
  echo "--- China Regions ---"
  printf "%-15s %-25s %-15s\n" "cn-hangzhou" "Hangzhou, China" "Primary"
  printf "%-15s %-25s %-15s\n" "cn-shanghai" "Shanghai, China" "East China"
  printf "%-15s %-25s %-15s\n" "cn-beijing" "Beijing, China" "North China"
  printf "%-15s %-25s %-15s\n" "cn-shenzhen" "Shenzhen, China" "South China"
  printf "%-15s %-25s %-15s\n" "cn-hongkong" "Hong Kong" "Special Admin"
  echo "--- International ---"
  printf "%-15s %-25s %-15s\n" "ap-southeast-1" "Singapore" "Southeast Asia"
  printf "%-15s %-25s %-15s\n" "ap-southeast-3" "Kuala Lumpur" "Malaysia"
  printf "%-15s %-25s %-15s\n" "ap-northeast-1" "Tokyo, Japan" "Northeast Asia"
  printf "%-15s %-25s %-15s\n" "eu-central-1" "Frankfurt, Germany" "Europe"
  printf "%-15s %-25s %-15s\n" "us-west-1" "Silicon Valley, USA" "West US"
  printf "%-15s %-25s %-15s\n" "me-east-1" "Dubai, UAE" "Middle East"
}

provider_alibaba_list_sizes() {
  printf "%-15s %-6s %-8s %-12s\n" "INSTANCE TYPE" "vCPU" "RAM" "EST. COST"
  printf "%-15s %-6s %-8s %-12s\n" "ecs.t6-c1m1" "1" "1GB" "~\$4/mo"
  printf "%-15s %-6s %-8s %-12s\n" "ecs.t6-c1m2" "1" "2GB" "~\$8/mo"
  printf "%-15s %-6s %-8s %-12s\n" "ecs.g6.large" "2" "8GB" "~\$40/mo"
  printf "%-15s %-6s %-8s %-12s\n" "ecs.g6.xlarge" "4" "16GB" "~\$80/mo"
  printf "%-15s %-6s %-8s %-12s\n" "ecs.g6.2xlarge" "8" "32GB" "~\$160/mo"
  echo ""
  echo "Note: Prices vary by region. China regions are often cheaper."
  echo "ACK (Container Service for Kubernetes) available in most regions."
}

provider_alibaba_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local region="${3:-cn-hangzhou}"

  local config_file="${HOME}/.nself/providers/alibaba.yml"
  if [[ ! -f "$config_file" ]]; then
    log_error "Alibaba Cloud not configured"
    return 1
  fi

  # Map size to instance type
  local instance_type
  case "$size" in
    tiny) instance_type="ecs.t6-c1m1.large" ;;
    small) instance_type="ecs.t6-c1m2.large" ;;
    medium) instance_type="ecs.g6.large" ;;
    large) instance_type="ecs.g6.xlarge" ;;
    xlarge) instance_type="ecs.g6.2xlarge" ;;
    *) instance_type="ecs.t6-c1m2.large" ;;
  esac

  log_info "Provisioning Alibaba ECS: $name ($instance_type) in $region..."
  log_warning "Alibaba Cloud API requires SDK. Use aliyun CLI or console:"
  log_info "  aliyun ecs CreateInstance --InstanceName $name --InstanceType $instance_type"
  log_info "  Or visit: https://ecs.console.aliyun.com/"

  return 1
}

provider_alibaba_destroy() {
  local instance_id="$1"
  log_info "To destroy Alibaba ECS instance:"
  log_info "  aliyun ecs DeleteInstance --InstanceId $instance_id --Force true"
  log_info "  Or visit: https://ecs.console.aliyun.com/"
  return 1
}

provider_alibaba_status() {
  local instance_id="$1"
  log_info "To check Alibaba ECS status:"
  log_info "  aliyun ecs DescribeInstanceStatus --InstanceId.1 $instance_id"
}

provider_alibaba_list() {
  log_info "To list Alibaba ECS instances:"
  log_info "  aliyun ecs DescribeInstances"
  log_info "  Or visit: https://ecs.console.aliyun.com/"
}

provider_alibaba_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "root@$ip" "$@"
}

provider_alibaba_get_ip() {
  echo "$1"
}

provider_alibaba_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "4" ;;
    small) echo "8" ;;
    medium) echo "40" ;;
    large) echo "80" ;;
    xlarge) echo "160" ;;
    *) echo "8" ;;
  esac
}

# Kubernetes support via ACK
provider_alibaba_k8s_supported() {
  echo "true"
}

provider_alibaba_k8s_create() {
  local name="${1:-nself-cluster}"
  local region="${2:-cn-hangzhou}"
  local node_count="${3:-3}"

  log_info "Creating Alibaba ACK cluster: $name in $region..."
  log_warning "ACK cluster creation requires aliyun CLI or console:"
  log_info "  aliyun cs CreateCluster --name $name --region-id $region"
  log_info "  Or visit: https://cs.console.aliyun.com/"
  return 1
}

provider_alibaba_k8s_delete() {
  local cluster_id="$1"
  log_info "To delete ACK cluster:"
  log_info "  aliyun cs DeleteCluster --ClusterId $cluster_id"
}

provider_alibaba_k8s_kubeconfig() {
  local cluster_id="$1"
  log_info "To get ACK kubeconfig:"
  log_info "  aliyun cs DescribeClusterUserKubeconfig --ClusterId $cluster_id"
}

# Export functions
export -f provider_alibaba_init provider_alibaba_validate provider_alibaba_list_regions
export -f provider_alibaba_list_sizes provider_alibaba_provision provider_alibaba_destroy
export -f provider_alibaba_status provider_alibaba_list provider_alibaba_ssh
export -f provider_alibaba_get_ip provider_alibaba_estimate_cost
export -f provider_alibaba_k8s_supported provider_alibaba_k8s_create
export -f provider_alibaba_k8s_delete provider_alibaba_k8s_kubeconfig
