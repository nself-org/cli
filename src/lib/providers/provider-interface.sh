#!/usr/bin/env bash
# provider-interface.sh - Unified provider abstraction layer
# Provides consistent interface across all cloud providers


# Get script directory
PROVIDER_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source utilities
[[ -z "${DISPLAY_SOURCED:-}" ]] && source "$PROVIDER_LIB_DIR/../utils/display.sh"

# Provider registry
declare -a SUPPORTED_PROVIDERS=(
  # Major Cloud
  "aws" "gcp" "azure" "oracle" "ibm"
  # Developer Cloud
  "digitalocean" "linode" "vultr" "scaleway" "upcloud"
  # Budget EU
  "hetzner" "ovh" "ionos" "contabo" "netcup"
  # Budget Global
  "hostinger" "hostwinds" "kamatera" "ssdnodes"
  # Regional
  "exoscale" "alibaba" "tencent" "yandex"
  # Extreme Budget
  "racknerd" "buyvm" "time4vps"
)

# ============================================================================
# Bash 3.2 Compatible Lookup Functions (replacing associative arrays)
# ============================================================================

# Get provider category
_get_provider_category() {
  local provider="$1"
  case "$provider" in
    aws | gcp | azure | oracle | ibm) echo "major" ;;
    digitalocean | linode | vultr | scaleway | upcloud) echo "developer" ;;
    hetzner | ovh | ionos | contabo | netcup) echo "budget-eu" ;;
    hostinger | hostwinds | kamatera | ssdnodes) echo "budget-global" ;;
    exoscale | alibaba | tencent | yandex) echo "regional" ;;
    racknerd | buyvm | time4vps) echo "extreme-budget" ;;
    *) echo "unknown" ;;
  esac
}

# Get size vCPU count
_get_size_vcpu() {
  local size="$1"
  case "$size" in
    tiny) echo "1" ;;
    small) echo "1" ;;
    medium) echo "2" ;;
    large) echo "4" ;;
    xlarge) echo "8" ;;
    *) echo "2" ;; # default
  esac
}

# Get size RAM in MB
_get_size_ram() {
  local size="$1"
  case "$size" in
    tiny) echo "512" ;;
    small) echo "1024" ;;
    medium) echo "4096" ;;
    large) echo "8192" ;;
    xlarge) echo "16384" ;;
    *) echo "2048" ;; # default
  esac
}

# Get size disk in GB
_get_size_disk() {
  local size="$1"
  case "$size" in
    tiny) echo "10" ;;
    small) echo "25" ;;
    medium) echo "50" ;;
    large) echo "100" ;;
    xlarge) echo "200" ;;
    *) echo "40" ;; # default
  esac
}

# Get managed K8s service name for provider
_get_k8s_support() {
  local provider="$1"
  case "$provider" in
    aws) echo "eks" ;;
    gcp) echo "gke" ;;
    azure) echo "aks" ;;
    digitalocean) echo "doks" ;;
    linode) echo "lke" ;;
    vultr) echo "vke" ;;
    scaleway) echo "kapsule" ;;
    ovh) echo "ovhk8s" ;;
    oracle) echo "oke" ;;
    ionos) echo "ionosk8s" ;;
    exoscale) echo "sks" ;;
    alibaba) echo "ack" ;;
    *) echo "" ;; # no support
  esac
}

# ============================================================================
# Provider Loading
# ============================================================================

# Load a provider module
provider_load() {
  local provider="$1"
  local provider_file="$PROVIDER_LIB_DIR/${provider}.sh"

  if [[ ! -f "$provider_file" ]]; then
    log_error "Provider '$provider' not found"
    return 1
  fi

  source "$provider_file"
  return 0
}

# Check if provider is supported
provider_is_supported() {
  local provider="$1"
  local p
  for p in "${SUPPORTED_PROVIDERS[@]}"; do
    [[ "$p" == "$provider" ]] && return 0
  done
  return 1
}

# List all supported providers
provider_list_all() {
  local format="${1:-table}"

  if [[ "$format" == "json" ]]; then
    printf '{\n  "providers": [\n'
    local first=true
    for provider in "${SUPPORTED_PROVIDERS[@]}"; do
      if [[ "$first" == "true" ]]; then
        first=false
      else
        printf ',\n'
      fi
      local category
      category=$(_get_provider_category "$provider")
      local k8s
      k8s=$(_get_k8s_support "$provider")
      [[ -z "$k8s" ]] && k8s="none"
      printf '    {"name": "%s", "category": "%s", "managed_k8s": "%s"}' "$provider" "$category" "$k8s"
    done
    printf '\n  ]\n}\n'
  else
    printf "%-15s %-15s %-12s\n" "PROVIDER" "CATEGORY" "MANAGED K8S"
    printf "%-15s %-15s %-12s\n" "--------" "--------" "-----------"
    for provider in "${SUPPORTED_PROVIDERS[@]}"; do
      local category
      category=$(_get_provider_category "$provider")
      local k8s
      k8s=$(_get_k8s_support "$provider")
      [[ -z "$k8s" ]] && k8s="none"
      printf "%-15s %-15s %-12s\n" "$provider" "$category" "$k8s"
    done
  fi
}

# List providers by category
provider_list_by_category() {
  local category="$1"

  for provider in "${SUPPORTED_PROVIDERS[@]}"; do
    local pcat
    pcat=$(_get_provider_category "$provider")
    if [[ "$pcat" == "$category" ]]; then
      echo "$provider"
    fi
  done
}

# ============================================================================
# Size Normalization
# ============================================================================

# Get normalized size specs
provider_normalize_size() {
  local size="$1"
  local field="${2:-all}"

  case "$field" in
    vcpu) _get_size_vcpu "$size" ;;
    ram) _get_size_ram "$size" ;;
    disk) _get_size_disk "$size" ;;
    all)
      printf '{"vcpu": %s, "ram_mb": %s, "disk_gb": %s}' \
        "$(_get_size_vcpu "$size")" \
        "$(_get_size_ram "$size")" \
        "$(_get_size_disk "$size")"
      ;;
  esac
}

# Validate size name
provider_validate_size() {
  local size="$1"
  case "$size" in
    tiny | small | medium | large | xlarge) return 0 ;;
    *) return 1 ;;
  esac
}

# ============================================================================
# Provider Operations (Generic Wrappers)
# ============================================================================

# Initialize provider credentials
provider_init() {
  local provider="$1"
  shift

  provider_load "$provider" || return 1

  if type "provider_${provider}_init" &>/dev/null; then
    "provider_${provider}_init" "$@"
  else
    log_error "Provider '$provider' does not support init"
    return 1
  fi
}

# Validate provider credentials
provider_validate() {
  local provider="$1"

  provider_load "$provider" || return 1

  if type "provider_${provider}_validate" &>/dev/null; then
    "provider_${provider}_validate"
  else
    log_warning "Provider '$provider' does not have validation"
    return 0
  fi
}

# List available regions
provider_list_regions() {
  local provider="$1"

  provider_load "$provider" || return 1

  if type "provider_${provider}_list_regions" &>/dev/null; then
    "provider_${provider}_list_regions"
  else
    log_error "Provider '$provider' does not support listing regions"
    return 1
  fi
}

# List available instance sizes
provider_list_sizes() {
  local provider="$1"

  provider_load "$provider" || return 1

  if type "provider_${provider}_list_sizes" &>/dev/null; then
    "provider_${provider}_list_sizes"
  else
    log_error "Provider '$provider' does not support listing sizes"
    return 1
  fi
}

# Provision a new server
provider_provision() {
  local provider="$1"
  shift

  provider_load "$provider" || return 1

  if type "provider_${provider}_provision" &>/dev/null; then
    "provider_${provider}_provision" "$@"
  else
    log_error "Provider '$provider' does not support provisioning"
    return 1
  fi
}

# Destroy a server
provider_destroy() {
  local provider="$1"
  local server_id="$2"

  provider_load "$provider" || return 1

  if type "provider_${provider}_destroy" &>/dev/null; then
    "provider_${provider}_destroy" "$server_id"
  else
    log_error "Provider '$provider' does not support destroy"
    return 1
  fi
}

# Get server status
provider_status() {
  local provider="$1"
  local server_id="${2:-}"

  provider_load "$provider" || return 1

  if type "provider_${provider}_status" &>/dev/null; then
    "provider_${provider}_status" "$server_id"
  else
    log_error "Provider '$provider' does not support status"
    return 1
  fi
}

# List servers
provider_list_servers() {
  local provider="$1"

  provider_load "$provider" || return 1

  if type "provider_${provider}_list" &>/dev/null; then
    "provider_${provider}_list"
  else
    log_error "Provider '$provider' does not support listing servers"
    return 1
  fi
}

# SSH to server
provider_ssh() {
  local provider="$1"
  local server_id="$2"
  shift 2

  provider_load "$provider" || return 1

  if type "provider_${provider}_ssh" &>/dev/null; then
    "provider_${provider}_ssh" "$server_id" "$@"
  else
    log_error "Provider '$provider' does not support SSH"
    return 1
  fi
}

# ============================================================================
# Kubernetes Operations
# ============================================================================

# Check if provider supports managed Kubernetes
provider_supports_k8s() {
  local provider="$1"
  local k8s_service
  k8s_service=$(_get_k8s_support "$provider")
  [[ -n "$k8s_service" ]]
}

# Get managed K8s service name
provider_k8s_service() {
  local provider="$1"
  _get_k8s_support "$provider"
}

# Create Kubernetes cluster
provider_k8s_create() {
  local provider="$1"
  shift

  if ! provider_supports_k8s "$provider"; then
    log_error "Provider '$provider' does not support managed Kubernetes"
    log_info "Consider using k3s instead: nself k8s k3s install"
    return 1
  fi

  provider_load "$provider" || return 1

  if type "provider_${provider}_k8s_create" &>/dev/null; then
    "provider_${provider}_k8s_create" "$@"
  else
    log_error "Managed K8s creation not implemented for '$provider'"
    return 1
  fi
}

# Delete Kubernetes cluster
provider_k8s_delete() {
  local provider="$1"
  local cluster_id="$2"

  provider_load "$provider" || return 1

  if type "provider_${provider}_k8s_delete" &>/dev/null; then
    "provider_${provider}_k8s_delete" "$cluster_id"
  else
    log_error "Managed K8s deletion not implemented for '$provider'"
    return 1
  fi
}

# Get kubeconfig
provider_k8s_kubeconfig() {
  local provider="$1"
  local cluster_id="$2"

  provider_load "$provider" || return 1

  if type "provider_${provider}_k8s_kubeconfig" &>/dev/null; then
    "provider_${provider}_k8s_kubeconfig" "$cluster_id"
  else
    log_error "Kubeconfig retrieval not implemented for '$provider'"
    return 1
  fi
}

# ============================================================================
# Cost Estimation
# ============================================================================

# Estimate monthly cost
provider_estimate_cost() {
  local provider="$1"
  local size="${2:-small}"

  provider_load "$provider" || return 1

  if type "provider_${provider}_estimate_cost" &>/dev/null; then
    "provider_${provider}_estimate_cost" "$size"
  else
    # Return generic estimate based on category
    local category
    category=$(_get_provider_category "$provider")
    case "$category" in
      major) echo "15-50" ;;
      developer) echo "5-20" ;;
      budget-eu) echo "3-15" ;;
      budget-global) echo "5-15" ;;
      regional) echo "5-20" ;;
      extreme-budget) echo "2-10" ;;
      *) echo "unknown" ;;
    esac
  fi
}

# Compare costs across providers
provider_compare_costs() {
  local size="${1:-small}"

  printf "%-15s %-15s %-15s\n" "PROVIDER" "CATEGORY" "EST. COST/MO"
  printf "%-15s %-15s %-15s\n" "--------" "--------" "------------"

  for provider in "${SUPPORTED_PROVIDERS[@]}"; do
    local category
    category=$(_get_provider_category "$provider")
    local cost
    cost=$(provider_estimate_cost "$provider" "$size" 2>/dev/null || echo "N/A")
    printf "%-15s %-15s \$%-14s\n" "$provider" "$category" "$cost"
  done
}

# ============================================================================
# Configuration Management
# ============================================================================

# Get provider config directory
provider_config_dir() {
  echo "${HOME}/.nself/providers"
}

# Get provider config file
provider_config_file() {
  local provider="$1"
  echo "$(provider_config_dir)/${provider}.yml"
}

# Check if provider is configured
provider_is_configured() {
  local provider="$1"
  local config_file
  config_file=$(provider_config_file "$provider")
  [[ -f "$config_file" ]]
}

# List configured providers
provider_list_configured() {
  local config_dir
  config_dir=$(provider_config_dir)

  if [[ ! -d "$config_dir" ]]; then
    return 0
  fi

  for file in "$config_dir"/*.yml; do
    [[ -f "$file" ]] || continue
    basename "$file" .yml
  done
}

# ============================================================================
# Utility Functions
# ============================================================================

# Wait for server to be ready
provider_wait_ready() {
  local provider="$1"
  local server_id="$2"
  local timeout="${3:-300}"

  local elapsed=0
  local interval=5

  log_info "Waiting for server to be ready..."

  while [[ $elapsed -lt $timeout ]]; do
    local status
    status=$(provider_status "$provider" "$server_id" 2>/dev/null | grep -i "status" | head -1 || echo "")

    if echo "$status" | grep -qiE "(running|active|ready)"; then
      log_success "Server is ready"
      return 0
    fi

    sleep $interval
    elapsed=$((elapsed + interval))
    printf "."
  done

  echo
  log_error "Timeout waiting for server to be ready"
  return 1
}

# Get server IP address
provider_get_ip() {
  local provider="$1"
  local server_id="$2"

  provider_load "$provider" || return 1

  if type "provider_${provider}_get_ip" &>/dev/null; then
    "provider_${provider}_get_ip" "$server_id"
  else
    # Try to parse from status
    provider_status "$provider" "$server_id" 2>/dev/null | grep -oE '\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b' | head -1
  fi
}

# Export functions
export -f _get_provider_category _get_size_vcpu _get_size_ram _get_size_disk _get_k8s_support
export -f provider_load provider_is_supported provider_list_all
export -f provider_normalize_size provider_validate_size
export -f provider_init provider_validate provider_list_regions provider_list_sizes
export -f provider_provision provider_destroy provider_status provider_list_servers provider_ssh
export -f provider_supports_k8s provider_k8s_service provider_k8s_create provider_k8s_delete provider_k8s_kubeconfig
export -f provider_estimate_cost provider_compare_costs
export -f provider_config_dir provider_config_file provider_is_configured provider_list_configured
export -f provider_wait_ready provider_get_ip
