#!/usr/bin/env bash
# ibm.sh - IBM Cloud provider module


PROVIDER_NAME="ibm"

set -euo pipefail

PROVIDER_DISPLAY_NAME="IBM Cloud"

IBM_REGIONS=("us-south" "us-east" "eu-gb" "eu-de" "jp-tok" "au-syd" "jp-osa" "br-sao" "ca-tor")

# Bash 3.2 compatible - use function instead of associative array
_ibm_get_profile() {
  local size="$1"
  case "$size" in
    tiny | small) echo "bx2-2x8" ;;
    medium) echo "bx2-4x16" ;;
    large) echo "bx2-8x32" ;;
    xlarge) echo "bx2-16x64" ;;
    *) echo "bx2-2x8" ;;
  esac
}

provider_ibm_init() {
  log_info "Initializing IBM Cloud provider..."

  if ! command -v ibmcloud &>/dev/null; then
    log_error "IBM Cloud CLI not found"
    log_info "Install: curl -fsSL https://clis.cloud.ibm.com/install/linux | sh"
    return 1
  fi

  log_info "Running IBM Cloud login..."
  ibmcloud login

  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"
  cat >"$config_dir/ibm.yml" <<'EOF'
provider: ibm
configured: true
default_region: us-south
default_size: small
EOF

  log_success "IBM Cloud provider initialized"
}

provider_ibm_validate() {
  command -v ibmcloud &>/dev/null || {
    log_error "IBM Cloud CLI not installed"
    return 1
  }
  ibmcloud account show &>/dev/null && {
    log_success "IBM Cloud credentials valid"
    return 0
  }
  log_error "IBM Cloud credentials invalid"
  return 1
}

provider_ibm_list_regions() {
  if command -v ibmcloud &>/dev/null; then
    ibmcloud regions 2>/dev/null || true
  fi
  printf "%-12s %-25s\n" "REGION" "LOCATION"
  printf "%-12s %-25s\n" "us-south" "Dallas" "us-east" "Washington DC"
  printf "%-12s %-25s\n" "eu-gb" "London" "eu-de" "Frankfurt"
  printf "%-12s %-25s\n" "jp-tok" "Tokyo" "au-syd" "Sydney"
}

provider_ibm_list_sizes() {
  printf "%-10s %-12s %-6s %-8s %-12s\n" "SIZE" "PROFILE" "vCPU" "RAM" "EST. COST"
  printf "%-10s %-12s %-6s %-8s %-12s\n" "----" "-------" "----" "---" "---------"
  printf "%-10s %-12s %-6s %-8s %-12s\n" "small" "bx2-2x8" "2" "8GB" "~\$40/mo"
  printf "%-10s %-12s %-6s %-8s %-12s\n" "medium" "bx2-4x16" "4" "16GB" "~\$80/mo"
  printf "%-10s %-12s %-6s %-8s %-12s\n" "large" "bx2-8x32" "8" "32GB" "~\$160/mo"
  printf "%-10s %-12s %-6s %-8s %-12s\n" "xlarge" "bx2-16x64" "16" "64GB" "~\$320/mo"
}

provider_ibm_provision() {
  local name="" size="small" region="us-south"
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
      *) shift ;;
    esac
  done
  [[ -z "$name" ]] && {
    log_error "Server name required"
    return 1
  }

  local profile
  profile=$(_ibm_get_profile "$size")
  log_info "Provisioning IBM Cloud instance: $name ($profile in $region)"

  ibmcloud is instance-create "$name" \
    --profile "$profile" \
    --zone "${region}-1" \
    --image ibm-ubuntu-22-04-minimal-amd64-1 \
    --vpc default 2>/dev/null || {
    log_error "Failed to create instance"
    return 1
  }

  log_success "Instance creation initiated"
}

provider_ibm_destroy() {
  local instance_id="$1"
  ibmcloud is instance-delete "$instance_id" -f 2>/dev/null
  log_success "Instance deleted"
}

provider_ibm_status() {
  if [[ -n "${1:-}" ]]; then
    ibmcloud is instance "$1" 2>/dev/null
  else
    ibmcloud is instances 2>/dev/null
  fi
}

provider_ibm_list() { ibmcloud is instances 2>/dev/null || echo "No instances"; }
provider_ibm_ssh() {
  local id="$1"
  shift
  local ip
  ip=$(provider_ibm_get_ip "$id")
  ssh "root@$ip" "$@"
}
provider_ibm_get_ip() { ibmcloud is instance "$1" --output json 2>/dev/null | grep -o '"address":"[0-9.]*"' | head -1 | cut -d'"' -f4; }
provider_ibm_estimate_cost() {
  case "${1:-small}" in
    tiny | small) echo "40" ;; medium) echo "80" ;; large) echo "160" ;; xlarge) echo "320" ;; *) echo "40" ;;
  esac
}

# IKS (IBM Kubernetes Service)
provider_ibm_k8s_create() {
  local name="" nodes=3
  while [[ $# -gt 0 ]]; do
    case "$1" in --name)
      name="$2"
      shift 2
      ;;
    --nodes)
      nodes="$2"
      shift 2
      ;;
    *) shift ;; esac
  done
  [[ -z "$name" ]] && {
    log_error "Cluster name required"
    return 1
  }
  log_info "Creating IKS cluster: $name"
  ibmcloud ks cluster create vpc-gen2 --name "$name" --workers "$nodes" 2>/dev/null
}

provider_ibm_k8s_kubeconfig() {
  local cluster_id="$1"
  ibmcloud ks cluster config --cluster "$cluster_id" 2>/dev/null
  log_success "Kubeconfig configured"
}

export -f provider_ibm_init provider_ibm_validate provider_ibm_list_regions provider_ibm_list_sizes
export -f provider_ibm_provision provider_ibm_destroy provider_ibm_status provider_ibm_list
export -f provider_ibm_ssh provider_ibm_get_ip provider_ibm_estimate_cost
export -f provider_ibm_k8s_create provider_ibm_k8s_kubeconfig
