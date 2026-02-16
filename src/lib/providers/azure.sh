#!/usr/bin/env bash
# azure.sh - Microsoft Azure provider module
# Supports Virtual Machines, AKS (Azure Kubernetes Service)


PROVIDER_NAME="azure"

set -euo pipefail

PROVIDER_DISPLAY_NAME="Microsoft Azure"

# Azure default settings
AZURE_DEFAULT_LOCATION="${AZURE_DEFAULT_LOCATION:-eastus}"

# ============================================================================
# Provider Functions
# ============================================================================

provider_azure_init() {
  log_info "Initializing Azure provider..."

  # Check for Azure CLI
  if ! command -v az &>/dev/null; then
    log_error "Azure CLI not found"
    log_info "Install: https://docs.microsoft.com/cli/azure/install-azure-cli"
    log_info "  brew install azure-cli  # macOS"
    log_info "  curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash  # Ubuntu/Debian"
    return 1
  fi

  # Check if already logged in
  local current_account
  current_account=$(az account show --query "user.name" -o tsv 2>/dev/null || echo "")

  if [[ -n "$current_account" ]]; then
    log_info "Azure CLI already logged in as: $current_account"
    log_info "Run 'az login' to switch accounts"
  else
    log_info "Running Azure authentication..."
    az login
  fi

  # Save to nself config
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  local subscription_id
  subscription_id=$(az account show --query "id" -o tsv 2>/dev/null || echo "")

  cat >"$config_dir/azure.yml" <<EOF
provider: azure
configured: true
subscription_id: ${subscription_id}
default_location: ${AZURE_DEFAULT_LOCATION}
default_size: small
notes: |
  Azure Free Tier includes:
  - 750 hours/month of B1S Linux VM (12 months)
  - 64GB storage
  - AKS: Free cluster management, pay for nodes only
EOF

  log_success "Azure provider initialized"
}

provider_azure_validate() {
  if ! command -v az &>/dev/null; then
    log_error "Azure CLI not installed"
    return 1
  fi

  log_info "Validating Azure credentials..."

  if az account show &>/dev/null; then
    log_success "Azure credentials valid"
    return 0
  else
    log_error "Azure credentials invalid or expired"
    log_info "Run: az login"
    return 1
  fi
}

provider_azure_list_regions() {
  if command -v az &>/dev/null && az account list-locations --output table 2>/dev/null; then
    return 0
  fi

  # Fallback to static list
  printf "%-20s %-30s\n" "LOCATION" "DISPLAY NAME"
  printf "%-20s %-30s\n" "--------" "------------"
  printf "%-20s %-30s\n" "eastus" "East US"
  printf "%-20s %-30s\n" "eastus2" "East US 2"
  printf "%-20s %-30s\n" "westus" "West US"
  printf "%-20s %-30s\n" "westus2" "West US 2"
  printf "%-20s %-30s\n" "centralus" "Central US"
  printf "%-20s %-30s\n" "northeurope" "North Europe"
  printf "%-20s %-30s\n" "westeurope" "West Europe"
  printf "%-20s %-30s\n" "southeastasia" "Southeast Asia"
  printf "%-20s %-30s\n" "japaneast" "Japan East"
}

provider_azure_list_sizes() {
  printf "%-15s %-6s %-10s %-15s\n" "VM SIZE" "vCPU" "RAM" "EST. COST"
  printf "%-15s %-6s %-10s %-15s\n" "-------" "----" "---" "---------"
  printf "%-15s %-6s %-10s %-15s\n" "B1s" "1" "1GB" "\$7-10/mo"
  printf "%-15s %-6s %-10s %-15s\n" "B2s" "2" "4GB" "\$30-40/mo"
  printf "%-15s %-6s %-10s %-15s\n" "B2ms" "2" "8GB" "\$60-80/mo"
  printf "%-15s %-6s %-10s %-15s\n" "D2s_v3" "2" "8GB" "\$70-90/mo"
  printf "%-15s %-6s %-10s %-15s\n" "D4s_v3" "4" "16GB" "\$140-180/mo"
  echo ""
  echo "Note: Prices vary by region. On-Demand pricing shown."
  echo "AKS: No cluster management fee, pay for nodes only"
}

provider_azure_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local location="${3:-${AZURE_DEFAULT_LOCATION}}"

  # Validate credentials
  if ! provider_azure_validate &>/dev/null; then
    return 1
  fi

  # Map size to VM size
  local vm_size
  case "$size" in
    tiny) vm_size="Standard_B1s" ;;
    small) vm_size="Standard_B2s" ;;
    medium) vm_size="Standard_B2ms" ;;
    large) vm_size="Standard_D2s_v3" ;;
    xlarge) vm_size="Standard_D4s_v3" ;;
    *) vm_size="Standard_B2s" ;;
  esac

  log_info "Provisioning Azure VM: $name ($vm_size) in $location..."
  log_info "Use Azure CLI or Portal for full control"
  log_info "  az vm create --resource-group <rg> --name $name --size $vm_size --location $location"

  return 1
}

provider_azure_destroy() {
  local vm_name="$1"
  local resource_group="${2:-nself-rg}"

  if ! command -v az &>/dev/null; then
    log_error "Azure CLI required"
    return 1
  fi

  log_warning "Deleting Azure VM: $vm_name"
  az vm delete --resource-group "$resource_group" --name "$vm_name" --yes &>/dev/null
  log_success "VM deletion initiated"
}

provider_azure_status() {
  local vm_name="${1:-}"

  if ! command -v az &>/dev/null; then
    log_error "Azure CLI required"
    return 1
  fi

  if [[ -n "$vm_name" ]]; then
    az vm show --name "$vm_name" --output table 2>/dev/null || log_error "VM not found"
  else
    az vm list --output table
  fi
}

provider_azure_list() {
  if ! command -v az &>/dev/null; then
    log_error "Azure CLI required"
    return 1
  fi

  az vm list \
    --query "[].{Name:name, ResourceGroup:resourceGroup, Location:location, PowerState:powerState}" \
    --output table
}

provider_azure_ssh() {
  local vm_name="$1"
  local resource_group="${2:-nself-rg}"
  shift 2

  local public_ip
  public_ip=$(provider_azure_get_ip "$vm_name" "$resource_group")

  if [[ -z "$public_ip" ]]; then
    log_error "Could not get IP for VM"
    return 1
  fi

  ssh -o StrictHostKeyChecking=no "azureuser@${public_ip}" "$@"
}

provider_azure_get_ip() {
  local vm_name="$1"
  local resource_group="${2:-nself-rg}"

  az vm show \
    --resource-group "$resource_group" \
    --name "$vm_name" \
    --show-details \
    --query "publicIps" \
    --output tsv 2>/dev/null
}

provider_azure_estimate_cost() {
  local size="${1:-small}"

  case "$size" in
    tiny) echo "7-10" ;;
    small) echo "30-40" ;;
    medium) echo "60-80" ;;
    large) echo "70-90" ;;
    xlarge) echo "140-180" ;;
    *) echo "30-40" ;;
  esac
}

# ============================================================================
# AKS (Azure Kubernetes Service) Support
# ============================================================================

provider_azure_k8s_create() {
  local cluster_name="${1:-nself-cluster}"
  local location="${2:-${AZURE_DEFAULT_LOCATION}}"
  local node_count="${3:-3}"
  local node_size="${4:-medium}"

  # Validate Azure CLI
  if ! command -v az &>/dev/null; then
    log_error "Azure CLI required for AKS cluster creation"
    log_info "Install: brew install azure-cli"
    return 1
  fi

  # Validate credentials
  if ! provider_azure_validate &>/dev/null; then
    return 1
  fi

  # Map size to VM size
  local vm_size
  case "$node_size" in
    small) vm_size="Standard_D2s_v3" ;;
    medium) vm_size="Standard_D4s_v3" ;;
    large) vm_size="Standard_D8s_v3" ;;
    xlarge) vm_size="Standard_D16s_v3" ;;
    *) vm_size="Standard_D4s_v3" ;;
  esac

  # Resource group
  local resource_group="${cluster_name}-rg"

  log_info "Creating AKS cluster: $cluster_name in $location"
  log_info "  Resource group: $resource_group"
  log_info "  Node count: $node_count"
  log_info "  Node size: $vm_size"
  log_info "This operation can take 5-10 minutes..."

  # Create resource group
  log_info "Creating resource group..."
  az group create --name "$resource_group" --location "$location" &>/dev/null

  # Create AKS cluster
  az aks create \
    --resource-group "$resource_group" \
    --name "$cluster_name" \
    --location "$location" \
    --node-count "$node_count" \
    --node-vm-size "$vm_size" \
    --enable-managed-identity \
    --generate-ssh-keys \
    --enable-cluster-autoscaler \
    --min-count 1 \
    --max-count 5

  if [[ $? -eq 0 ]]; then
    log_success "AKS cluster created successfully"

    # Get credentials automatically
    az aks get-credentials \
      --resource-group "$resource_group" \
      --name "$cluster_name" \
      --overwrite-existing

    log_success "Kubeconfig updated at ~/.kube/config"
    log_info "Test connection: kubectl cluster-info"
    return 0
  else
    log_error "Failed to create AKS cluster"
    return 1
  fi
}

provider_azure_k8s_delete() {
  local cluster_name="$1"
  local resource_group="${2:-${cluster_name}-rg}"

  if ! command -v az &>/dev/null; then
    log_error "Azure CLI required"
    return 1
  fi

  log_warning "Deleting AKS cluster: $cluster_name"

  # Delete AKS cluster
  az aks delete \
    --resource-group "$resource_group" \
    --name "$cluster_name" \
    --yes \
    --no-wait

  log_info "Waiting for cluster deletion..."
  sleep 10

  # Delete resource group
  log_info "Deleting resource group: $resource_group"
  az group delete --name "$resource_group" --yes --no-wait

  log_success "AKS cluster deletion initiated"
  log_info "Deletion is running in background (5-10 minutes)"
}

provider_azure_k8s_kubeconfig() {
  local cluster_name="$1"
  local resource_group="${2:-${cluster_name}-rg}"

  if ! command -v az &>/dev/null; then
    log_error "Azure CLI required"
    return 1
  fi

  log_info "Retrieving kubeconfig for AKS cluster: $cluster_name"

  # Get cluster credentials
  az aks get-credentials \
    --resource-group "$resource_group" \
    --name "$cluster_name" \
    --overwrite-existing

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
export -f provider_azure_init provider_azure_validate
export -f provider_azure_list_regions provider_azure_list_sizes
export -f provider_azure_provision provider_azure_destroy
export -f provider_azure_status provider_azure_list provider_azure_ssh
export -f provider_azure_get_ip provider_azure_estimate_cost
export -f provider_azure_k8s_create provider_azure_k8s_delete provider_azure_k8s_kubeconfig
