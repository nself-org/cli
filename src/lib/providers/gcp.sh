#!/usr/bin/env bash
# gcp.sh - Google Cloud Platform (GCP) provider module
# Supports Compute Engine, GKE (Google Kubernetes Engine)


PROVIDER_NAME="gcp"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Google Cloud Platform"

# GCP default settings
GCP_DEFAULT_REGION="${GCP_DEFAULT_REGION:-us-central1}"
GCP_DEFAULT_ZONE="${GCP_DEFAULT_ZONE:-us-central1-a}"

# ============================================================================
# Provider Functions
# ============================================================================

provider_gcp_init() {
  log_info "Initializing GCP provider..."

  # Check for gcloud CLI
  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI not found"
    log_info "Install: https://cloud.google.com/sdk/docs/install"
    log_info "  brew install google-cloud-sdk  # macOS"
    return 1
  fi

  # Check if already configured
  local current_account
  current_account=$(gcloud config get-value account 2>/dev/null || echo "")

  if [[ -n "$current_account" ]]; then
    log_info "gcloud already configured for: $current_account"
    log_info "Run 'gcloud auth login' to switch accounts"
  else
    log_info "Running gcloud authentication..."
    gcloud auth login
    gcloud init
  fi

  # Save to nself config
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  local current_project
  current_project=$(gcloud config get-value project 2>/dev/null || echo "")

  cat >"$config_dir/gcp.yml" <<EOF
provider: gcp
configured: true
default_project: ${current_project}
default_region: ${GCP_DEFAULT_REGION}
default_zone: ${GCP_DEFAULT_ZONE}
default_size: small
notes: |
  GCP Free Tier includes:
  - 1 f1-micro instance/month (US regions only)
  - 30GB HDD storage
  - 5GB snapshot storage
  - GKE: Free cluster management, pay for nodes only
EOF

  log_success "GCP provider initialized"
}

provider_gcp_validate() {
  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI not installed"
    return 1
  fi

  log_info "Validating GCP credentials..."

  if gcloud auth list --filter=status:ACTIVE --format="value(account)" 2>/dev/null | grep -q "@"; then
    log_success "GCP credentials valid"
    return 0
  else
    log_error "GCP credentials invalid or expired"
    log_info "Run: gcloud auth login"
    return 1
  fi
}

provider_gcp_list_regions() {
  if command -v gcloud &>/dev/null && gcloud compute regions list --format="table(name,status)" 2>/dev/null; then
    return 0
  fi

  # Fallback to static list
  printf "%-20s %-30s\n" "REGION" "LOCATION"
  printf "%-20s %-30s\n" "------" "--------"
  printf "%-20s %-30s\n" "us-central1" "Iowa, USA"
  printf "%-20s %-30s\n" "us-east1" "South Carolina, USA"
  printf "%-20s %-30s\n" "us-west1" "Oregon, USA"
  printf "%-20s %-30s\n" "europe-west1" "Belgium"
  printf "%-20s %-30s\n" "europe-west4" "Netherlands"
  printf "%-20s %-30s\n" "asia-east1" "Taiwan"
  printf "%-20s %-30s\n" "asia-northeast1" "Tokyo, Japan"
  printf "%-20s %-30s\n" "asia-southeast1" "Singapore"
}

provider_gcp_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "MACHINE TYPE" "vCPU" "RAM" "EST. COST"
  printf "%-15s %-6s %-10s %-15s\n" "------------" "----" "---" "---------"
  printf "%-15s %-6s %-10s %-15s\n" "f1-micro" "1" "0.6GB" "\$4-5/mo (free)"
  printf "%-15s %-6s %-10s %-15s\n" "g1-small" "1" "1.7GB" "\$13-15/mo"
  printf "%-15s %-6s %-10s %-15s\n" "e2-small" "2" "2GB" "\$12-15/mo"
  printf "%-15s %-6s %-10s %-15s\n" "e2-medium" "2" "4GB" "\$24-30/mo"
  printf "%-15s %-6s %-10s %-15s\n" "e2-standard-2" "2" "8GB" "\$48-60/mo"
  printf "%-15s %-6s %-10s %-15s\n" "e2-standard-4" "4" "16GB" "\$96-120/mo"
  echo ""
  echo "Note: Prices vary by region. On-Demand pricing shown."
  echo "GKE: No cluster management fee, pay for nodes only"
}

provider_gcp_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local zone="${3:-${GCP_DEFAULT_ZONE}}"

  # Validate credentials
  if ! provider_gcp_validate &>/dev/null; then
    return 1
  fi

  # Map size to machine type
  local machine_type
  case "$size" in
    tiny) machine_type="f1-micro" ;;
    small) machine_type="e2-small" ;;
    medium) machine_type="e2-medium" ;;
    large) machine_type="e2-standard-2" ;;
    xlarge) machine_type="e2-standard-4" ;;
    *) machine_type="e2-small" ;;
  esac

  log_info "Provisioning GCP Compute Engine: $name ($machine_type) in $zone..."
  log_info "Use gcloud command for full control"
  log_info "  gcloud compute instances create $name --machine-type=$machine_type --zone=$zone"

  return 1
}

provider_gcp_destroy() {
  local instance_name="$1"
  local zone="${2:-${GCP_DEFAULT_ZONE}}"

  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required"
    return 1
  fi

  log_warning "Deleting GCP Compute Engine instance: $instance_name"
  gcloud compute instances delete "$instance_name" --zone="$zone" --quiet &>/dev/null
  log_success "Instance deleted"
}

provider_gcp_status() {
  local instance_name="${1:-}"

  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required"
    return 1
  fi

  if [[ -n "$instance_name" ]]; then
    gcloud compute instances describe "$instance_name" --format="table(name,status,machineType,networkInterfaces[0].accessConfigs[0].natIP)"
  else
    gcloud compute instances list --format="table(name,status,machineType,networkInterfaces[0].accessConfigs[0].natIP)"
  fi
}

provider_gcp_list() {
  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required"
    return 1
  fi

  gcloud compute instances list \
    --filter="status=RUNNING" \
    --format="table(name,zone,machineType,networkInterfaces[0].accessConfigs[0].natIP)"
}

provider_gcp_ssh() {
  local instance_name="$1"
  local zone="${2:-${GCP_DEFAULT_ZONE}}"
  shift 2

  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required"
    return 1
  fi

  gcloud compute ssh "$instance_name" --zone="$zone" -- "$@"
}

provider_gcp_get_ip() {
  local instance_name="$1"
  local zone="${2:-${GCP_DEFAULT_ZONE}}"

  gcloud compute instances describe "$instance_name" \
    --zone="$zone" \
    --format="get(networkInterfaces[0].accessConfigs[0].natIP)" 2>/dev/null
}

provider_gcp_estimate_cost() {
  local size="${1:-small}"

  case "$size" in
    tiny) echo "4-5" ;;
    small) echo "12-15" ;;
    medium) echo "24-30" ;;
    large) echo "48-60" ;;
    xlarge) echo "96-120" ;;
    *) echo "12-15" ;;
  esac
}

# ============================================================================
# GKE (Google Kubernetes Engine) Support
# ============================================================================

provider_gcp_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local region="${2:-${GCP_DEFAULT_REGION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  # Validate gcloud CLI
  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required for GKE cluster creation"
    log_info "Install: brew install google-cloud-sdk"
    return 1
  fi

  # Validate credentials
  if ! provider_gcp_validate &>/dev/null; then
    return 1
  fi

  # Check for current project
  local project
  project=$(gcloud config get-value project 2>/dev/null || echo "")
  if [[ -z "$project" ]]; then
    log_error "No GCP project configured"
    log_info "Run: gcloud config set project PROJECT_ID"
    return 1
  fi

  # Map size to machine type
  local machine_type
  case "$node_size" in
    small) machine_type="e2-medium" ;;
    medium) machine_type="e2-standard-2" ;;
    large) machine_type="e2-standard-4" ;;
    xlarge) machine_type="e2-standard-8" ;;
    *) machine_type="e2-standard-2" ;;
  esac

  log_info "Creating GKE cluster: $cluster_name in $region"
  log_info "  Project: $project"
  log_info "  Node count: $node_count"
  log_info "  Machine type: $machine_type"
  log_info "This operation can take 5-10 minutes..."

  # Create GKE cluster
  gcloud container clusters create "$cluster_name" \
    --region="$region" \
    --num-nodes="$node_count" \
    --machine-type="$machine_type" \
    --enable-autoscaling \
    --min-nodes=1 \
    --max-nodes=5 \
    --enable-autorepair \
    --enable-autoupgrade \
    --disk-size=50 \
    --disk-type=pd-standard

  if [[ $? -eq 0 ]]; then
    log_success "GKE cluster created successfully"

    # Get credentials automatically
    gcloud container clusters get-credentials "$cluster_name" --region="$region"

    log_success "Kubeconfig updated at ~/.kube/config"
    log_info "Test connection: kubectl cluster-info"
    return 0
  else
    log_error "Failed to create GKE cluster"
    return 1
  fi
}

provider_gcp_k8s_delete() {
  local cluster_name="$1"
  local region="${2:-${GCP_DEFAULT_REGION}}"

  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required"
    return 1
  fi

  log_warning "Deleting GKE cluster: $cluster_name"

  gcloud container clusters delete "$cluster_name" \
    --region="$region" \
    --quiet

  if [[ $? -eq 0 ]]; then
    log_success "GKE cluster deleted successfully"
    return 0
  else
    log_error "Failed to delete GKE cluster"
    return 1
  fi
}

provider_gcp_k8s_kubeconfig() {
  local cluster_name="$1"
  local region="${2:-${GCP_DEFAULT_REGION}}"

  if ! command -v gcloud &>/dev/null; then
    log_error "gcloud CLI required"
    return 1
  fi

  log_info "Retrieving kubeconfig for GKE cluster: $cluster_name"

  # Get cluster credentials
  gcloud container clusters get-credentials "$cluster_name" \
    --region="$region"

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
export -f provider_gcp_init provider_gcp_validate
export -f provider_gcp_list_regions provider_gcp_list_sizes
export -f provider_gcp_provision provider_gcp_destroy
export -f provider_gcp_status provider_gcp_list provider_gcp_ssh
export -f provider_gcp_get_ip provider_gcp_estimate_cost
export -f provider_gcp_k8s_create provider_gcp_k8s_delete provider_gcp_k8s_kubeconfig
