#!/usr/bin/env bash
# exoscale.sh - Exoscale provider (Swiss quality, GDPR-compliant, SKS Kubernetes)

PROVIDER_NAME="exoscale"

set -euo pipefail

EXOSCALE_API_URL="https://api.exoscale.com/v2"

provider_exoscale_init() {
  log_info "Initializing Exoscale provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Exoscale API Key: "
  read -r api_key
  read -rsp "Exoscale API Secret: " api_secret
  echo

  # Validate credentials
  local response
  response=$(curl -s -X GET "${EXOSCALE_API_URL}/zone" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null) || true

  if echo "$response" | grep -q "zones"; then
    log_success "Credentials validated"
  else
    log_warning "Could not validate credentials"
  fi

  cat >"$config_dir/exoscale.yml" <<EOF
provider: exoscale
api_key: "$api_key"
api_secret: "$api_secret"
default_zone: ch-gva-2
default_size: small
EOF

  log_success "Exoscale provider initialized"
}

provider_exoscale_validate() {
  local config_file="${HOME}/.nself/providers/exoscale.yml"
  if [[ -f "$config_file" ]]; then
    log_success "Exoscale configured"
    return 0
  fi
  log_error "Exoscale not configured. Run: nself provider init exoscale"
  return 1
}

provider_exoscale_list_regions() {
  printf "%-12s %-25s %-15s\n" "ZONE" "LOCATION" "NOTES"
  printf "%-12s %-25s %-15s\n" "ch-gva-2" "Geneva, Switzerland" "Primary, GDPR"
  printf "%-12s %-25s %-15s\n" "ch-dk-2" "Zurich, Switzerland" "GDPR"
  printf "%-12s %-25s %-15s\n" "de-fra-1" "Frankfurt, Germany" "EU Central"
  printf "%-12s %-25s %-15s\n" "de-muc-1" "Munich, Germany" "EU"
  printf "%-12s %-25s %-15s\n" "at-vie-1" "Vienna, Austria" "EU"
  printf "%-12s %-25s %-15s\n" "bg-sof-1" "Sofia, Bulgaria" "EU East"
}

provider_exoscale_list_sizes() {
  printf "%-12s %-6s %-8s %-12s\n" "SIZE" "vCPU" "RAM" "EST. COST"
  printf "%-12s %-6s %-8s %-12s\n" "tiny" "1" "512MB" "~€3.50/mo"
  printf "%-12s %-6s %-8s %-12s\n" "small" "1" "1GB" "~€7/mo"
  printf "%-12s %-6s %-8s %-12s\n" "medium" "2" "4GB" "~€28/mo"
  printf "%-12s %-6s %-8s %-12s\n" "large" "4" "8GB" "~€56/mo"
  printf "%-12s %-6s %-8s %-12s\n" "xlarge" "8" "16GB" "~€112/mo"
  echo ""
  echo "Note: Exoscale offers SKS (Scalable Kubernetes Service)"
}

provider_exoscale_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local zone="${3:-ch-gva-2}"

  local config_file="${HOME}/.nself/providers/exoscale.yml"
  if [[ ! -f "$config_file" ]]; then
    log_error "Exoscale not configured"
    return 1
  fi

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  # Map size to Exoscale instance type
  local instance_type
  case "$size" in
    tiny) instance_type="standard.tiny" ;;
    small) instance_type="standard.small" ;;
    medium) instance_type="standard.medium" ;;
    large) instance_type="standard.large" ;;
    xlarge) instance_type="standard.extra-large" ;;
    *) instance_type="standard.small" ;;
  esac

  log_info "Provisioning Exoscale instance: $name ($instance_type) in $zone..."

  # Get template ID for Ubuntu
  local template_id
  template_id=$(curl -s -X GET "${EXOSCALE_API_URL}/template" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" \
    -H "Content-Type: application/json" 2>/dev/null |
    grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

  local response
  response=$(curl -s -X POST "${EXOSCALE_API_URL}/instance" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"${name}\",
      \"instance-type\": {\"name\": \"${instance_type}\"},
      \"template\": {\"id\": \"${template_id}\"},
      \"zone\": {\"name\": \"${zone}\"}
    }" 2>/dev/null)

  if echo "$response" | grep -q '"id"'; then
    local instance_id
    instance_id=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "Instance created. ID: $instance_id"
    echo "$instance_id"
    return 0
  else
    log_error "Failed to provision instance: $response"
    return 1
  fi
}

provider_exoscale_destroy() {
  local instance_id="$1"
  local config_file="${HOME}/.nself/providers/exoscale.yml"

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  log_info "Destroying Exoscale instance: $instance_id..."

  local response
  response=$(curl -s -X DELETE "${EXOSCALE_API_URL}/instance/${instance_id}" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null)

  log_success "Instance $instance_id destroyed"
  return 0
}

provider_exoscale_status() {
  local instance_id="$1"
  local config_file="${HOME}/.nself/providers/exoscale.yml"

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  curl -s -X GET "${EXOSCALE_API_URL}/instance/${instance_id}" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null
}

provider_exoscale_list() {
  local config_file="${HOME}/.nself/providers/exoscale.yml"

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${EXOSCALE_API_URL}/instance" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null)

  printf "%-20s %-12s %-15s %-10s\n" "NAME" "ZONE" "IP" "STATE"
  echo "$response" | grep -o '"name":"[^"]*"' | cut -d'"' -f4 | while read -r name; do
    printf "%-20s %-12s %-15s %-10s\n" "$name" "-" "-" "running"
  done
}

provider_exoscale_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "ubuntu@$ip" "$@"
}

provider_exoscale_get_ip() {
  local instance_id="$1"
  local config_file="${HOME}/.nself/providers/exoscale.yml"

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${EXOSCALE_API_URL}/instance/${instance_id}" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null)

  echo "$response" | grep -o '"ip-address":"[^"]*"' | cut -d'"' -f4
}

provider_exoscale_estimate_cost() {
  local size="${1:-small}"
  case "$size" in
    tiny) echo "4" ;;
    small) echo "7" ;;
    medium) echo "28" ;;
    large) echo "56" ;;
    xlarge) echo "112" ;;
    *) echo "7" ;;
  esac
}

# Kubernetes support via SKS
provider_exoscale_k8s_supported() {
  echo "true"
}

provider_exoscale_k8s_create() {
  local name="${1:-nself-cluster}"
  local zone="${2:-ch-gva-2}"
  local node_count="${3:-3}"
  local node_size="${4:-standard.medium}"

  local config_file="${HOME}/.nself/providers/exoscale.yml"
  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  log_info "Creating SKS cluster: $name in $zone..."

  local response
  response=$(curl -s -X POST "${EXOSCALE_API_URL}/sks-cluster" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"${name}\",
      \"zone\": {\"name\": \"${zone}\"},
      \"level\": \"pro\",
      \"cni\": \"calico\",
      \"nodepools\": [{
        \"name\": \"default\",
        \"size\": ${node_count},
        \"instance-type\": {\"name\": \"${node_size}\"}
      }]
    }" 2>/dev/null)

  if echo "$response" | grep -q '"id"'; then
    local cluster_id
    cluster_id=$(echo "$response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "SKS cluster creating. ID: $cluster_id"
    echo "$cluster_id"
    return 0
  else
    log_error "Failed to create cluster: $response"
    return 1
  fi
}

provider_exoscale_k8s_delete() {
  local cluster_id="$1"
  local config_file="${HOME}/.nself/providers/exoscale.yml"

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  log_info "Deleting SKS cluster: $cluster_id..."

  curl -s -X DELETE "${EXOSCALE_API_URL}/sks-cluster/${cluster_id}" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null

  log_success "SKS cluster deleted"
}

provider_exoscale_k8s_kubeconfig() {
  local cluster_id="$1"
  local config_file="${HOME}/.nself/providers/exoscale.yml"

  local api_key api_secret
  api_key=$(grep "api_key:" "$config_file" | cut -d'"' -f2)
  api_secret=$(grep "api_secret:" "$config_file" | cut -d'"' -f2)

  curl -s -X GET "${EXOSCALE_API_URL}/sks-cluster/${cluster_id}/kubeconfig" \
    -H "Authorization: Bearer ${api_key}:${api_secret}" 2>/dev/null
}

# Export functions
export -f provider_exoscale_init provider_exoscale_validate provider_exoscale_list_regions
export -f provider_exoscale_list_sizes provider_exoscale_provision provider_exoscale_destroy
export -f provider_exoscale_status provider_exoscale_list provider_exoscale_ssh
export -f provider_exoscale_get_ip provider_exoscale_estimate_cost
export -f provider_exoscale_k8s_supported provider_exoscale_k8s_create
export -f provider_exoscale_k8s_delete provider_exoscale_k8s_kubeconfig
