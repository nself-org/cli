#!/usr/bin/env bash
# aws.sh - Amazon Web Services (AWS) provider module
# Supports EC2, EKS (Elastic Kubernetes Service)


PROVIDER_NAME="aws"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Amazon Web Services"

# AWS default regions
AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"

# ============================================================================
# Provider Functions
# ============================================================================

provider_aws_init() {
  log_info "Initializing AWS provider..."

  # Check for AWS CLI
  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI not found"
    log_info "Install: https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html"
    log_info "  brew install awscli  # macOS"
    log_info "  apt install awscli   # Ubuntu/Debian"
    return 1
  fi

  # Check if already configured
  if [[ -f "$HOME/.aws/credentials" ]] && [[ -f "$HOME/.aws/config" ]]; then
    log_info "AWS CLI already configured"
    log_info "Run 'aws configure' to reconfigure"
  else
    log_info "Running AWS configuration..."
    aws configure
  fi

  # Save to nself config
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  # Get current region from AWS config
  local current_region
  current_region=$(aws configure get region 2>/dev/null || echo "us-east-1")

  cat >"$config_dir/aws.yml" <<EOF
provider: aws
configured: true
default_region: ${current_region}
default_size: small
notes: |
  AWS Free Tier includes:
  - 750 hours/month of t2.micro or t3.micro
  - 30GB EBS storage
  - 5GB S3 storage
  - EKS control plane costs \$0.10/hour (~\$73/month)
EOF

  log_success "AWS provider initialized"
}

provider_aws_validate() {
  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI not installed"
    return 1
  fi

  log_info "Validating AWS credentials..."

  if aws sts get-caller-identity &>/dev/null; then
    log_success "AWS credentials valid"
    return 0
  else
    log_error "AWS credentials invalid or expired"
    log_info "Run: aws configure"
    return 1
  fi
}

provider_aws_list_regions() {
  if command -v aws &>/dev/null && aws ec2 describe-regions --output table 2>/dev/null; then
    return 0
  fi

  # Fallback to static list
  printf "%-20s %-30s\n" "REGION" "LOCATION"
  printf "%-20s %-30s\n" "------" "--------"
  printf "%-20s %-30s\n" "us-east-1" "US East (N. Virginia)"
  printf "%-20s %-30s\n" "us-east-2" "US East (Ohio)"
  printf "%-20s %-30s\n" "us-west-1" "US West (N. California)"
  printf "%-20s %-30s\n" "us-west-2" "US West (Oregon)"
  printf "%-20s %-30s\n" "eu-west-1" "Europe (Ireland)"
  printf "%-20s %-30s\n" "eu-central-1" "Europe (Frankfurt)"
  printf "%-20s %-30s\n" "ap-northeast-1" "Asia Pacific (Tokyo)"
  printf "%-20s %-30s\n" "ap-southeast-1" "Asia Pacific (Singapore)"
  printf "%-20s %-30s\n" "ap-southeast-2" "Asia Pacific (Sydney)"
}

provider_aws_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "INSTANCE TYPE" "vCPU" "RAM" "EST. COST"
  printf "%-15s %-6s %-10s %-15s\n" "-------------" "----" "---" "---------"
  printf "%-15s %-6s %-10s %-15s\n" "t3.micro" "2" "1GB" "\$7-9/mo"
  printf "%-15s %-6s %-10s %-15s\n" "t3.small" "2" "2GB" "\$15-18/mo"
  printf "%-15s %-6s %-10s %-15s\n" "t3.medium" "2" "4GB" "\$30-36/mo"
  printf "%-15s %-6s %-10s %-15s\n" "t3.large" "2" "8GB" "\$60-72/mo"
  printf "%-15s %-6s %-10s %-15s\n" "t3.xlarge" "4" "16GB" "\$120-144/mo"
  echo ""
  echo "Note: Prices vary by region. On-Demand pricing shown."
  echo "EKS control plane: \$0.10/hour (~\$73/month)"
}

provider_aws_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local region="${3:-${AWS_DEFAULT_REGION}}"

  # Validate credentials
  if ! provider_aws_validate &>/dev/null; then
    return 1
  fi

  # Map size to instance type
  local instance_type
  case "$size" in
    tiny) instance_type="t3.micro" ;;
    small) instance_type="t3.small" ;;
    medium) instance_type="t3.medium" ;;
    large) instance_type="t3.large" ;;
    xlarge) instance_type="t3.xlarge" ;;
    *) instance_type="t3.small" ;;
  esac

  log_info "Provisioning AWS EC2: $name ($instance_type) in $region..."
  log_info "Use AWS Console or full CLI for complete setup"
  log_info "  aws ec2 run-instances --instance-type $instance_type --tag-specifications \"ResourceType=instance,Tags=[{Key=Name,Value=$name}]\""

  return 1
}

provider_aws_destroy() {
  local instance_id="$1"

  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI required"
    return 1
  fi

  log_warning "Terminating AWS EC2 instance: $instance_id"
  aws ec2 terminate-instances --instance-ids "$instance_id" &>/dev/null
  log_success "Instance termination initiated"
}

provider_aws_status() {
  local instance_id="${1:-}"

  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI required"
    return 1
  fi

  if [[ -n "$instance_id" ]]; then
    aws ec2 describe-instances --instance-ids "$instance_id" --output table
  else
    aws ec2 describe-instances --output table
  fi
}

provider_aws_list() {
  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI required"
    return 1
  fi

  aws ec2 describe-instances \
    --filters "Name=instance-state-name,Values=running" \
    --query "Reservations[].Instances[].[InstanceId,InstanceType,State.Name,PublicIpAddress,Tags[?Key=='Name'].Value|[0]]" \
    --output table
}

provider_aws_ssh() {
  local instance_id="$1"
  shift

  local public_ip
  public_ip=$(provider_aws_get_ip "$instance_id")

  if [[ -z "$public_ip" ]]; then
    log_error "Could not get IP for instance"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "ec2-user@${public_ip}" "$@"
}

provider_aws_get_ip() {
  local instance_id="$1"

  aws ec2 describe-instances \
    --instance-ids "$instance_id" \
    --query "Reservations[0].Instances[0].PublicIpAddress" \
    --output text 2>/dev/null
}

provider_aws_estimate_cost() {
  local size="${1:-small}"

  case "$size" in
    tiny) echo "7-9" ;;
    small) echo "15-18" ;;
    medium) echo "30-36" ;;
    large) echo "60-72" ;;
    xlarge) echo "120-144" ;;
    *) echo "15-18" ;;
  esac
}

# ============================================================================
# EKS (Elastic Kubernetes Service) Support
# ============================================================================

provider_aws_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local region="${2:-${AWS_DEFAULT_REGION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  # Validate AWS CLI
  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI required for EKS cluster creation"
    log_info "Install: brew install awscli"
    return 1
  fi

  # Check for eksctl (recommended for EKS cluster creation)
  if ! command -v eksctl &>/dev/null; then
    log_warning "eksctl not found (recommended for EKS)"
    log_info "Install: brew install eksctl"
    log_info "Or use AWS CLI directly (more complex)"
  fi

  # Validate credentials
  if ! provider_aws_validate &>/dev/null; then
    return 1
  fi

  # Map size to instance type
  local instance_type
  case "$node_size" in
    small) instance_type="t3.medium" ;;
    medium) instance_type="t3.large" ;;
    large) instance_type="t3.xlarge" ;;
    xlarge) instance_type="t3.2xlarge" ;;
    *) instance_type="t3.large" ;;
  esac

  log_info "Creating EKS cluster: $cluster_name in $region"
  log_info "  Node count: $node_count"
  log_info "  Node type: $instance_type"
  log_info "This operation can take 15-20 minutes..."

  if command -v eksctl &>/dev/null; then
    # Use eksctl (easier)
    eksctl create cluster \
      --name "$cluster_name" \
      --region "$region" \
      --nodes "$node_count" \
      --node-type "$instance_type" \
      --managed

    if [[ $? -eq 0 ]]; then
      log_success "EKS cluster created successfully"
      log_info "Kubeconfig automatically updated at ~/.kube/config"
      return 0
    else
      log_error "Failed to create EKS cluster"
      return 1
    fi
  else
    # Provide AWS CLI instructions
    log_error "eksctl not available"
    log_info "Manual EKS creation requires multiple steps:"
    log_info "1. Create IAM role for EKS cluster"
    log_info "2. Create VPC and subnets"
    log_info "3. Create EKS cluster: aws eks create-cluster --name $cluster_name --region $region ..."
    log_info "4. Create node group: aws eks create-nodegroup ..."
    log_info ""
    log_info "Recommend installing eksctl: brew install eksctl"
    return 1
  fi
}

provider_aws_k8s_delete() {
  local cluster_name="$1"
  local region="${2:-${AWS_DEFAULT_REGION}}"

  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI required"
    return 1
  fi

  log_warning "Deleting EKS cluster: $cluster_name"

  if command -v eksctl &>/dev/null; then
    # Use eksctl (handles cleanup properly)
    eksctl delete cluster --name "$cluster_name" --region "$region"
  else
    # Use AWS CLI
    log_info "Deleting node groups..."
    local nodegroups
    nodegroups=$(aws eks list-nodegroups --cluster-name "$cluster_name" --region "$region" --query "nodegroups[]" --output text 2>/dev/null || echo "")

    for ng in $nodegroups; do
      log_info "  Deleting nodegroup: $ng"
      aws eks delete-nodegroup --cluster-name "$cluster_name" --nodegroup-name "$ng" --region "$region" &>/dev/null || true
    done

    # Wait for nodegroups to delete
    log_info "Waiting for nodegroups to delete..."
    sleep 30

    # Delete cluster
    log_info "Deleting cluster..."
    aws eks delete-cluster --name "$cluster_name" --region "$region" &>/dev/null

    log_success "EKS cluster deletion initiated"
  fi
}

provider_aws_k8s_kubeconfig() {
  local cluster_name="$1"
  local region="${2:-${AWS_DEFAULT_REGION}}"

  if ! command -v aws &>/dev/null; then
    log_error "AWS CLI required"
    return 1
  fi

  log_info "Retrieving kubeconfig for EKS cluster: $cluster_name"

  # Update kubeconfig
  aws eks update-kubeconfig \
    --name "$cluster_name" \
    --region "$region" \
    --kubeconfig "${HOME}/.kube/config"

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
export -f provider_aws_init provider_aws_validate
export -f provider_aws_list_regions provider_aws_list_sizes
export -f provider_aws_provision provider_aws_destroy
export -f provider_aws_status provider_aws_list provider_aws_ssh
export -f provider_aws_get_ip provider_aws_estimate_cost
export -f provider_aws_k8s_create provider_aws_k8s_delete provider_aws_k8s_kubeconfig
