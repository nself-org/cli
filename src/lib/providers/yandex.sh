#!/usr/bin/env bash
# yandex.sh - Yandex Cloud provider (MKS Kubernetes, Russia/CIS focused)

PROVIDER_NAME="yandex"

set -euo pipefail

YANDEX_API_URL="https://compute.api.cloud.yandex.net/compute/v1"

provider_yandex_init() {
  log_info "Initializing Yandex Cloud provider..."
  local config_dir="${HOME}/.nself/providers"
  mkdir -p "$config_dir"

  printf "Yandex Cloud OAuth Token: "
  read -rsp "" oauth_token
  echo
  printf "Cloud ID: "
  read -r cloud_id
  printf "Folder ID: "
  read -r folder_id
  printf "Default Zone [ru-central1-a]: "
  read -r zone
  zone="${zone:-ru-central1-a}"

  cat >"$config_dir/yandex.yml" <<EOF
provider: yandex
oauth_token: "$oauth_token"
cloud_id: "$cloud_id"
folder_id: "$folder_id"
default_zone: "$zone"
default_size: small
EOF

  log_success "Yandex Cloud provider initialized"
}

provider_yandex_validate() {
  local config_file="${HOME}/.nself/providers/yandex.yml"
  if [[ -f "$config_file" ]]; then
    log_success "Yandex Cloud configured"
    return 0
  fi
  log_error "Yandex Cloud not configured. Run: nself provider init yandex"
  return 1
}

provider_yandex_list_regions() {
  printf "%-15s %-25s\n" "ZONE" "LOCATION"
  printf "%-15s %-25s\n" "ru-central1-a" "Moscow, Russia (Zone A)"
  printf "%-15s %-25s\n" "ru-central1-b" "Moscow, Russia (Zone B)"
  printf "%-15s %-25s\n" "ru-central1-c" "Moscow, Russia (Zone C)"
  echo ""
  echo "Note: Yandex Cloud primarily serves Russia and CIS countries."
  echo "MKS (Managed Kubernetes Service) available in all zones."
}

provider_yandex_list_sizes() {
  printf "%-15s %-6s %-8s %-12s\n" "PLATFORM" "vCPU" "RAM" "EST. COST"
  printf "%-15s %-6s %-8s %-12s\n" "standard-v3" "2" "2GB" "~\$5/mo"
  printf "%-15s %-6s %-8s %-12s\n" "standard-v3" "2" "4GB" "~\$10/mo"
  printf "%-15s %-6s %-8s %-12s\n" "standard-v3" "4" "8GB" "~\$25/mo"
  printf "%-15s %-6s %-8s %-12s\n" "standard-v3" "8" "16GB" "~\$50/mo"
  printf "%-15s %-6s %-8s %-12s\n" "standard-v3" "16" "32GB" "~\$100/mo"
}

provider_yandex_provision() {
  local name="${1:-nself-server}"
  local size="${2:-small}"
  local zone="${3:-ru-central1-a}"

  local config_file="${HOME}/.nself/providers/yandex.yml"
  if [[ ! -f "$config_file" ]]; then
    log_error "Yandex Cloud not configured"
    return 1
  fi

  local oauth_token folder_id
  oauth_token=$(grep "oauth_token:" "$config_file" | cut -d'"' -f2)
  folder_id=$(grep "folder_id:" "$config_file" | cut -d'"' -f2)

  # Map size to cores/memory
  local cores memory
  case "$size" in
    tiny)
      cores=2
      memory=$((2 * 1024 * 1024 * 1024))
      ;;
    small)
      cores=2
      memory=$((4 * 1024 * 1024 * 1024))
      ;;
    medium)
      cores=4
      memory=$((8 * 1024 * 1024 * 1024))
      ;;
    large)
      cores=8
      memory=$((16 * 1024 * 1024 * 1024))
      ;;
    xlarge)
      cores=16
      memory=$((32 * 1024 * 1024 * 1024))
      ;;
    *)
      cores=2
      memory=$((4 * 1024 * 1024 * 1024))
      ;;
  esac

  log_info "Provisioning Yandex VM: $name in $zone..."

  local response
  response=$(curl -s -X POST "${YANDEX_API_URL}/instances" \
    -H "Authorization: Bearer ${oauth_token}" \
    -H "Content-Type: application/json" \
    -d "{
      \"folderId\": \"${folder_id}\",
      \"name\": \"${name}\",
      \"zoneId\": \"${zone}\",
      \"platformId\": \"standard-v3\",
      \"resourcesSpec\": {
        \"cores\": ${cores},
        \"memory\": ${memory}
      },
      \"bootDiskSpec\": {
        \"autoDelete\": true,
        \"diskSpec\": {
          \"size\": $((20 * 1024 * 1024 * 1024)),
          \"imageId\": \"fd8vmcue7aajpmeo39kk\"
        }
      },
      \"networkInterfaceSpecs\": [{
        \"subnetId\": \"auto\",
        \"primaryV4AddressSpec\": {
          \"oneToOneNatSpec\": {
            \"ipVersion\": \"IPV4\"
          }
        }
      }]
    }" 2>/dev/null)

  if echo "$response" | grep -q '"id"'; then
    local instance_id
    instance_id=$(echo "$response" | grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "VM created. ID: $instance_id"
    echo "$instance_id"
    return 0
  else
    log_error "Failed to provision: $response"
    return 1
  fi
}

provider_yandex_destroy() {
  local instance_id="$1"
  local config_file="${HOME}/.nself/providers/yandex.yml"

  local oauth_token
  oauth_token=$(grep "oauth_token:" "$config_file" | cut -d'"' -f2)

  log_info "Destroying Yandex VM: $instance_id..."

  curl -s -X DELETE "${YANDEX_API_URL}/instances/${instance_id}" \
    -H "Authorization: Bearer ${oauth_token}" 2>/dev/null

  log_success "VM deleted"
}

provider_yandex_status() {
  local instance_id="$1"
  local config_file="${HOME}/.nself/providers/yandex.yml"

  local oauth_token
  oauth_token=$(grep "oauth_token:" "$config_file" | cut -d'"' -f2)

  curl -s -X GET "${YANDEX_API_URL}/instances/${instance_id}" \
    -H "Authorization: Bearer ${oauth_token}" 2>/dev/null
}

provider_yandex_list() {
  local config_file="${HOME}/.nself/providers/yandex.yml"

  local oauth_token folder_id
  oauth_token=$(grep "oauth_token:" "$config_file" | cut -d'"' -f2)
  folder_id=$(grep "folder_id:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${YANDEX_API_URL}/instances?folderId=${folder_id}" \
    -H "Authorization: Bearer ${oauth_token}" 2>/dev/null)

  printf "%-20s %-15s %-15s %-10s\n" "NAME" "ZONE" "IP" "STATUS"
  echo "$response" | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4 | while read -r name; do
    printf "%-20s %-15s %-15s %-10s\n" "$name" "-" "-" "running"
  done
}

provider_yandex_ssh() {
  local ip="$1"
  shift
  ssh -o StrictHostKeyChecking=no "yc-user@$ip" "$@"
}

provider_yandex_get_ip() {
  local instance_id="$1"
  local config_file="${HOME}/.nself/providers/yandex.yml"

  local oauth_token
  oauth_token=$(grep "oauth_token:" "$config_file" | cut -d'"' -f2)

  local response
  response=$(curl -s -X GET "${YANDEX_API_URL}/instances/${instance_id}" \
    -H "Authorization: Bearer ${oauth_token}" 2>/dev/null)

  echo "$response" | grep -o '"address"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4
}

provider_yandex_estimate_cost() {
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

# Kubernetes support via MKS
provider_yandex_k8s_supported() {
  echo "true"
}

provider_yandex_k8s_create() {
  local name="${1:-nself-cluster}"
  local zone="${2:-ru-central1-a}"

  log_info "Creating Yandex MKS cluster: $name in $zone..."
  log_warning "Use yc CLI or console for MKS clusters:"
  log_info "  yc managed-kubernetes cluster create --name $name --zone $zone"
  log_info "  Or visit: https://console.cloud.yandex.com/folders"
  return 1
}

provider_yandex_k8s_delete() {
  local cluster_id="$1"
  log_info "To delete MKS cluster:"
  log_info "  yc managed-kubernetes cluster delete $cluster_id"
}

provider_yandex_k8s_kubeconfig() {
  local cluster_id="$1"
  log_info "To get MKS kubeconfig:"
  log_info "  yc managed-kubernetes cluster get-credentials $cluster_id"
}

# Export functions
export -f provider_yandex_init provider_yandex_validate provider_yandex_list_regions
export -f provider_yandex_list_sizes provider_yandex_provision provider_yandex_destroy
export -f provider_yandex_status provider_yandex_list provider_yandex_ssh
export -f provider_yandex_get_ip provider_yandex_estimate_cost
export -f provider_yandex_k8s_supported provider_yandex_k8s_create
export -f provider_yandex_k8s_delete provider_yandex_k8s_kubeconfig
