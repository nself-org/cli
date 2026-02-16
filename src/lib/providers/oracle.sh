#!/usr/bin/env bash
# oracle.sh - Oracle Cloud Infrastructure (OCI) provider module
# Supports Always Free tier, ARM instances, and OKE


PROVIDER_NAME="oracle"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Oracle Cloud Infrastructure"

# OCI regions
OCI_REGIONS=(
  "us-phoenix-1" "us-ashburn-1" "us-sanjose-1" "us-chicago-1"
  "eu-frankfurt-1" "eu-amsterdam-1" "eu-zurich-1" "eu-marseille-1"
  "uk-london-1" "ap-tokyo-1" "ap-osaka-1" "ap-seoul-1"
  "ap-mumbai-1" "ap-sydney-1" "ap-melbourne-1" "sa-saopaulo-1"
  "ca-toronto-1" "ca-montreal-1" "me-jeddah-1" "me-dubai-1"
)

# Size mappings to OCI shapes - Bash 3.2 compatible functions
_oci_get_shape() {
  local size="$1"
  case "$size" in
    tiny | free) echo "VM.Standard.E2.1.Micro" ;;
    small | medium | large | xlarge) echo "VM.Standard.E4.Flex" ;;
    arm-free) echo "VM.Standard.A1.Flex" ;;
    *) echo "VM.Standard.E4.Flex" ;;
  esac
}

# Flex shape OCPUs
_oci_get_ocpus() {
  local size="$1"
  case "$size" in
    small) echo "1" ;;
    medium) echo "2" ;;
    large) echo "4" ;;
    xlarge) echo "8" ;;
    *) echo "1" ;;
  esac
}

# Flex shape memory (GB)
_oci_get_memory() {
  local size="$1"
  case "$size" in
    small) echo "8" ;;
    medium) echo "16" ;;
    large) echo "32" ;;
    xlarge) echo "64" ;;
    *) echo "8" ;;
  esac
}

# ============================================================================
# Provider Functions
# ============================================================================

provider_oracle_init() {
  log_info "Initializing Oracle Cloud provider..."

  # Check for OCI CLI
  if ! command -v oci &>/dev/null; then
    log_error "OCI CLI not found"
    log_info "Install: https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm"
    log_info "  brew install oci-cli  # macOS"
    log_info "  pip install oci-cli   # pip"
    return 1
  fi

  # Check if already configured
  if [[ -f "$HOME/.oci/config" ]]; then
    log_info "OCI CLI already configured"
    log_info "Run 'oci setup config' to reconfigure"
  else
    log_info "Running OCI setup..."
    oci setup config
  fi

  # Save to nself config
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  cat >"$config_dir/oracle.yml" <<'EOF'
provider: oracle
configured: true
default_region: us-ashburn-1
default_size: small
notes: |
  Always Free tier includes:
  - 2 AMD VMs (VM.Standard.E2.1.Micro)
  - 4 ARM OCPUs + 24GB RAM (VM.Standard.A1.Flex)
  - 200GB block storage
  - 10TB outbound data transfer
EOF

  log_success "Oracle Cloud provider initialized"
}

provider_oracle_validate() {
  if ! command -v oci &>/dev/null; then
    log_error "OCI CLI not installed"
    return 1
  fi

  log_info "Validating Oracle Cloud credentials..."

  if oci iam region list --output table &>/dev/null; then
    log_success "Oracle Cloud credentials valid"
    return 0
  else
    log_error "Oracle Cloud credentials invalid or expired"
    log_info "Run: oci setup config"
    return 1
  fi
}

provider_oracle_list_regions() {
  if command -v oci &>/dev/null && oci iam region list --output table 2>/dev/null; then
    return 0
  fi

  # Fallback to static list
  printf "%-20s %-30s\n" "REGION" "LOCATION"
  printf "%-20s %-30s\n" "------" "--------"
  printf "%-20s %-30s\n" "us-ashburn-1" "US East (Ashburn)"
  printf "%-20s %-30s\n" "us-phoenix-1" "US West (Phoenix)"
  printf "%-20s %-30s\n" "eu-frankfurt-1" "Germany (Frankfurt)"
  printf "%-20s %-30s\n" "eu-amsterdam-1" "Netherlands (Amsterdam)"
  printf "%-20s %-30s\n" "uk-london-1" "UK (London)"
  printf "%-20s %-30s\n" "ap-tokyo-1" "Japan (Tokyo)"
  printf "%-20s %-30s\n" "ap-sydney-1" "Australia (Sydney)"
}

provider_oracle_list_sizes() {
  printf "%-10s %-25s %-8s %-10s %-15s\n" "SIZE" "SHAPE" "OCPU" "RAM (GB)" "EST. COST"
  printf "%-10s %-25s %-8s %-10s %-15s\n" "----" "-----" "----" "--------" "---------"
  printf "%-10s %-25s %-8s %-10s %-15s\n" "free" "VM.Standard.E2.1.Micro" "1" "1" "FREE"
  printf "%-10s %-25s %-8s %-10s %-15s\n" "arm-free" "VM.Standard.A1.Flex" "4" "24" "FREE"
  printf "%-10s %-25s %-8s %-10s %-15s\n" "small" "VM.Standard.E4.Flex" "1" "8" "~\$7/mo"
  printf "%-10s %-25s %-8s %-10s %-15s\n" "medium" "VM.Standard.E4.Flex" "2" "16" "~\$14/mo"
  printf "%-10s %-25s %-8s %-10s %-15s\n" "large" "VM.Standard.E4.Flex" "4" "32" "~\$28/mo"
}

provider_oracle_provision() {
  local name=""
  local size="small"
  local region=""
  local compartment_id=""
  local ssh_key=""
  local use_free_tier=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name)
        name="$2"
        shift 2
        ;;
      --size)
        size="$2"
        shift 2
        ;;
      --region)
        region="$2"
        shift 2
        ;;
      --compartment)
        compartment_id="$2"
        shift 2
        ;;
      --ssh-key)
        ssh_key="$2"
        shift 2
        ;;
      --free)
        use_free_tier=true
        shift
        ;;
      --arm)
        size="arm-free"
        shift
        ;;
      *) shift ;;
    esac
  done

  if [[ -z "$name" ]]; then
    log_error "Server name required: --name <name>"
    return 1
  fi

  # Use free tier shape if requested
  if [[ "$use_free_tier" == "true" ]]; then
    size="free"
  fi

  local shape ocpus memory
  shape=$(_oci_get_shape "$size")
  ocpus=$(_oci_get_ocpus "$size")
  memory=$(_oci_get_memory "$size")

  log_info "Provisioning Oracle Cloud instance..."
  log_info "  Name: $name"
  log_info "  Shape: $shape"
  log_info "  OCPUs: $ocpus"
  log_info "  Memory: ${memory}GB"

  if ! command -v oci &>/dev/null; then
    log_error "OCI CLI required for provisioning"
    return 1
  fi

  # Get compartment if not specified
  if [[ -z "$compartment_id" ]]; then
    compartment_id=$(oci iam compartment list --all --query "data[0].id" --raw-output 2>/dev/null || echo "")
    if [[ -z "$compartment_id" ]]; then
      log_error "Could not determine compartment ID. Use --compartment"
      return 1
    fi
  fi

  # Get availability domain
  local ad
  ad=$(oci iam availability-domain list --compartment-id "$compartment_id" --query "data[0].name" --raw-output 2>/dev/null || echo "")

  # Get latest Oracle Linux image
  local image_id
  image_id=$(oci compute image list --compartment-id "$compartment_id" \
    --operating-system "Oracle Linux" \
    --operating-system-version "8" \
    --shape "$shape" \
    --query "data[0].id" --raw-output 2>/dev/null || echo "")

  if [[ -z "$image_id" ]]; then
    log_error "Could not find suitable image"
    return 1
  fi

  # Create instance
  local instance_id
  if [[ "$shape" == *"Flex"* ]]; then
    instance_id=$(oci compute instance launch \
      --compartment-id "$compartment_id" \
      --availability-domain "$ad" \
      --shape "$shape" \
      --shape-config "{\"ocpus\": $ocpus, \"memoryInGBs\": $memory}" \
      --image-id "$image_id" \
      --display-name "$name" \
      --assign-public-ip true \
      --query "data.id" --raw-output 2>/dev/null)
  else
    instance_id=$(oci compute instance launch \
      --compartment-id "$compartment_id" \
      --availability-domain "$ad" \
      --shape "$shape" \
      --image-id "$image_id" \
      --display-name "$name" \
      --assign-public-ip true \
      --query "data.id" --raw-output 2>/dev/null)
  fi

  if [[ -n "$instance_id" ]]; then
    log_success "Instance created: $instance_id"
    log_info "Waiting for instance to be running..."

    # Wait for running state
    oci compute instance wait-for-state \
      --instance-id "$instance_id" \
      --wait-for-state "RUNNING" \
      --max-wait-seconds 300 2>/dev/null || true

    # Get public IP
    local public_ip
    public_ip=$(oci compute instance list-vnics \
      --instance-id "$instance_id" \
      --query "data[0].\"public-ip\"" --raw-output 2>/dev/null || echo "")

    if [[ -n "$public_ip" ]]; then
      log_success "Instance ready at: $public_ip"
      echo "$public_ip"
    fi
  else
    log_error "Failed to create instance"
    return 1
  fi
}

provider_oracle_destroy() {
  local instance_id="$1"

  log_warning "Destroying Oracle Cloud instance: $instance_id"

  if oci compute instance terminate --instance-id "$instance_id" --force 2>/dev/null; then
    log_success "Instance terminated"
  else
    log_error "Failed to terminate instance"
    return 1
  fi
}

provider_oracle_status() {
  local instance_id="${1:-}"

  if [[ -n "$instance_id" ]]; then
    oci compute instance get --instance-id "$instance_id" --output table 2>/dev/null
  else
    log_info "Listing all instances..."
    oci compute instance list --all --output table 2>/dev/null || log_warning "No instances found"
  fi
}

provider_oracle_list() {
  oci compute instance list --all --lifecycle-state RUNNING --output table 2>/dev/null || echo "No running instances"
}

provider_oracle_ssh() {
  local instance_id="$1"
  shift

  local public_ip
  public_ip=$(oci compute instance list-vnics \
    --instance-id "$instance_id" \
    --query "data[0].\"public-ip\"" --raw-output 2>/dev/null)

  if [[ -z "$public_ip" ]]; then
    log_error "Could not get IP for instance"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "opc@${public_ip}" "$@"
}

provider_oracle_get_ip() {
  local instance_id="$1"

  oci compute instance list-vnics \
    --instance-id "$instance_id" \
    --query "data[0].\"public-ip\"" --raw-output 2>/dev/null
}

provider_oracle_estimate_cost() {
  local size="${1:-small}"

  case "$size" in
    free | arm-free) echo "0" ;;
    tiny) echo "5" ;;
    small) echo "7" ;;
    medium) echo "14" ;;
    large) echo "28" ;;
    xlarge) echo "56" ;;
    *) echo "10" ;;
  esac
}

# OKE (Oracle Kubernetes Engine) support
provider_oracle_k8s_create() {
  local name=""
  local node_count=3
  local node_shape="VM.Standard.E4.Flex"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --name)
        name="$2"
        shift 2
        ;;
      --nodes)
        node_count="$2"
        shift 2
        ;;
      --shape)
        node_shape="$2"
        shift 2
        ;;
      *) shift ;;
    esac
  done

  if [[ -z "$name" ]]; then
    log_error "Cluster name required: --name <name>"
    return 1
  fi

  log_info "Creating OKE cluster: $name"
  log_info "This operation can take 15-20 minutes..."

  # Note: Full OKE creation requires VCN, subnets, etc.
  # This is a simplified version
  log_warning "OKE cluster creation requires additional setup (VCN, subnets)"
  log_info "Recommend using OCI Console for first-time setup"
  log_info "Or use: oci ce cluster create --help"

  return 1
}

provider_oracle_k8s_kubeconfig() {
  local cluster_id="$1"

  oci ce cluster create-kubeconfig \
    --cluster-id "$cluster_id" \
    --file "$HOME/.kube/config" \
    --token-version 2.0.0 2>/dev/null

  log_success "Kubeconfig saved to ~/.kube/config"
}

# Export functions
export -f provider_oracle_init provider_oracle_validate
export -f provider_oracle_list_regions provider_oracle_list_sizes
export -f provider_oracle_provision provider_oracle_destroy
export -f provider_oracle_status provider_oracle_list provider_oracle_ssh
export -f provider_oracle_get_ip provider_oracle_estimate_cost
export -f provider_oracle_k8s_create provider_oracle_k8s_kubeconfig
