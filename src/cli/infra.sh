#!/usr/bin/env bash
# infra.sh - Unified Infrastructure Management Command
# Part of nself v0.9.6 - Infrastructure Everywhere
# Consolidates: provider (cloud), k8s, helm commands
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_DIR="$SCRIPT_DIR" # Preserve CLI directory

# Source shared utilities (try multiple paths for compatibility)
if [[ -f "${CLI_DIR}/../lib/utils/cli-output.sh" ]]; then
  source "${CLI_DIR}/../lib/utils/cli-output.sh"
fi
if [[ -f "${CLI_DIR}/../lib/utils/display.sh" ]]; then
  source "${CLI_DIR}/../lib/utils/display.sh"
fi
if [[ -f "${CLI_DIR}/../lib/utils/env.sh" ]]; then
  source "${CLI_DIR}/../lib/utils/env.sh"
fi

# Restore SCRIPT_DIR after sourcing (some utilities may change it)
SCRIPT_DIR="$CLI_DIR"

# Fallback logging if display.sh failed to load
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m✓\033[0m %s\n" "$1"; }
fi
if ! declare -f log_warning >/dev/null 2>&1; then
  log_warning() { printf "\033[0;33m!\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m✗\033[0m %s\n" "$1" >&2; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34mℹ\033[0m %s\n" "$1"; }
fi

show_infra_help() {
  cat <<'EOF'
nself infra - Infrastructure Management

USAGE:
  nself infra <category> <subcommand> [options]

CATEGORIES:
  provider          Cloud provider infrastructure (26+ providers)
  k8s               Kubernetes infrastructure management
  helm              Helm chart management
  destroy           Destroy all local project infrastructure (containers, volumes, networks)
  reset             Reset project to clean state (restore from backup)
  clean             Clean old backups and stale resources

PROVIDER SUBCOMMANDS (14):
  provider list [--filter TYPE]              # List 26+ providers
  provider init <provider>                   # Configure credentials
  provider validate <provider>               # Validate configuration
  provider info <provider>                   # Provider details
  provider server create <provider> [opts]   # Provision server
  provider server destroy <id>               # Destroy server
  provider server list                       # List servers
  provider server status <id>                # Server status
  provider server ssh <id>                   # SSH to server
  provider server add <host>                 # Add existing server
  provider server remove <id>                # Remove server
  provider cost estimate <provider>          # Estimate costs
  provider cost compare                      # Compare providers
  provider deploy <quick|full> <provider>    # Deploy to provider

K8S SUBCOMMANDS (11):
  k8s init [--provider PROVIDER]             # Initialize K8s config
  k8s convert                                # Convert Compose to K8s
  k8s apply                                  # Apply manifests
  k8s deploy                                 # Full deployment
  k8s status                                 # Deployment status
  k8s logs <pod>                             # Pod logs
  k8s scale <deployment> <replicas>          # Scale deployment
  k8s rollback                               # Rollback deployment
  k8s delete                                 # Delete deployment
  k8s cluster <action>                       # Cluster management
  k8s namespace <action>                     # Namespace management

HELM SUBCOMMANDS (12):
  helm init                                  # Initialize Helm chart
  helm generate                              # Generate/update chart
  helm install <release>                     # Install to cluster
  helm upgrade <release>                     # Upgrade release
  helm rollback <release>                    # Rollback release
  helm uninstall <release>                   # Remove release
  helm list                                  # List releases
  helm status <release>                      # Release status
  helm values <release>                      # Show/edit values
  helm template                              # Render locally
  helm package                               # Package chart
  helm repo <action>                         # Repository management

TOTAL SUBCOMMANDS: 38

EXAMPLES:
  # Provider Management
  nself infra provider list                  # List all 26 providers
  nself infra provider init digitalocean     # Configure DigitalOcean
  nself infra provider server create --provider hetzner --size medium
  nself infra provider cost compare          # Compare pricing

  # Kubernetes
  nself infra k8s init                       # Initialize K8s config
  nself infra k8s deploy                     # Deploy to Kubernetes
  nself infra k8s status                     # Check deployment status
  nself infra k8s scale api --replicas 3     # Scale API pods

  # Helm
  nself infra helm init                      # Initialize Helm chart
  nself infra helm install myapp             # Install chart
  nself infra helm upgrade myapp             # Upgrade release

OPTIONS:
  --provider        Provider to use
  --namespace       Kubernetes namespace
  --values          Helm values file
  --dry-run         Simulate without making changes
  -h, --help        Show this help message

See 'nself infra <category> --help' for category-specific help.
EOF
}

# === K8S HELP ===
show_k8s_help() {
  cat <<'EOF'
nself infra k8s - Kubernetes Management

USAGE:
  nself infra k8s <subcommand> [options]

SUBCOMMANDS:
  init              Initialize Kubernetes configuration
  convert           Convert Docker Compose to K8s manifests
  apply             Apply manifests to cluster
  deploy            Full deployment to Kubernetes
  status            Show deployment status
  logs <pod>        View pod logs
  scale <name> <n>  Scale deployment to n replicas
  rollback          Rollback deployment
  delete            Delete deployment
  cluster           Cluster management
  namespace         Namespace management

OPTIONS:
  --provider        Kubernetes provider (minikube, kind, etc)
  --namespace       Kubernetes namespace
  --context         Kubectl context
  -h, --help        Show this help message

EXAMPLES:
  nself infra k8s init                    # Initialize K8s config
  nself infra k8s convert                 # Convert compose to manifests
  nself infra k8s deploy                  # Deploy to cluster
  nself infra k8s scale api --replicas 3  # Scale API pods
  nself infra k8s logs api-pod-xyz        # View pod logs

For full documentation, see: .wiki/infrastructure/K8S-IMPLEMENTATION-GUIDE.md
EOF
}

# === HELM HELP ===
show_helm_help() {
  cat <<'EOF'
nself infra helm - Helm Chart Management

USAGE:
  nself infra helm <subcommand> [options]

SUBCOMMANDS:
  init              Initialize Helm chart
  generate          Generate/update chart from compose
  install <name>    Install chart to cluster
  upgrade <name>    Upgrade release
  rollback <name>   Rollback release
  uninstall <name>  Remove release
  list              List releases
  status <name>     Release status
  values <name>     Show/edit values
  template          Render chart locally
  package           Package chart
  repo              Repository management

OPTIONS:
  --namespace       Kubernetes namespace
  --values          Values file
  --set             Set values on command line
  -h, --help        Show this help message

EXAMPLES:
  nself infra helm init                   # Initialize chart
  nself infra helm install myapp          # Install chart
  nself infra helm upgrade myapp          # Upgrade release
  nself infra helm list                   # List all releases

For full documentation, see: .wiki/commands/HELM.md
EOF
}

# === PROVIDER CATEGORY ===
cmd_infra_provider() {
  # Delegate to provider.sh
  if [[ -f "${SCRIPT_DIR}/provider.sh" ]]; then
    bash "${SCRIPT_DIR}/provider.sh" "$@"
  else
    log_error "Provider module not found: ${SCRIPT_DIR}/provider.sh"
    return 1
  fi
}

# === K8S CATEGORY ===
cmd_infra_k8s() {
  local subcommand="${1:-}"
  shift || true

  case "$subcommand" in
    "" | help | -h | --help)
      show_k8s_help
      ;;
    init)
      cmd_k8s_init "$@"
      ;;
    convert)
      cmd_k8s_convert "$@"
      ;;
    apply)
      cmd_k8s_apply "$@"
      ;;
    deploy)
      cmd_k8s_deploy "$@"
      ;;
    status)
      cmd_k8s_status "$@"
      ;;
    logs)
      cmd_k8s_logs "$@"
      ;;
    scale)
      cmd_k8s_scale "$@"
      ;;
    rollback)
      cmd_k8s_rollback "$@"
      ;;
    delete)
      cmd_k8s_delete "$@"
      ;;
    cluster)
      cmd_k8s_cluster "$@"
      ;;
    namespace)
      cmd_k8s_namespace "$@"
      ;;
    *)
      log_error "Unknown k8s subcommand: $subcommand"
      printf "\nRun 'nself infra k8s --help' for available subcommands.\n"
      return 1
      ;;
  esac
}

# === HELM CATEGORY ===
cmd_infra_helm() {
  local subcommand="${1:-}"
  shift || true

  case "$subcommand" in
    "" | help | -h | --help)
      show_helm_help
      ;;
    init)
      cmd_helm_init "$@"
      ;;
    generate)
      cmd_helm_generate "$@"
      ;;
    install)
      cmd_helm_install "$@"
      ;;
    upgrade)
      cmd_helm_upgrade "$@"
      ;;
    rollback)
      cmd_helm_rollback "$@"
      ;;
    uninstall)
      cmd_helm_uninstall "$@"
      ;;
    list)
      cmd_helm_list "$@"
      ;;
    status)
      cmd_helm_status "$@"
      ;;
    values)
      cmd_helm_values "$@"
      ;;
    template)
      cmd_helm_template "$@"
      ;;
    package)
      cmd_helm_package "$@"
      ;;
    repo)
      cmd_helm_repo "$@"
      ;;
    *)
      log_error "Unknown helm subcommand: $subcommand"
      printf "\nRun 'nself infra helm --help' for available subcommands.\n"
      return 1
      ;;
  esac
}

# === K8S HELP AND IMPLEMENTATIONS ===
cmd_k8s_init() {
  log_info "Initializing Kubernetes configuration..."

  local k8s_dir=".nself/k8s"
  mkdir -p "$k8s_dir/manifests"

  log_success "K8s configuration initialized"
  log_info "  Directory: $k8s_dir"
  log_info "  Next: Run 'nself infra k8s convert' to generate manifests"
}

cmd_k8s_convert() {
  log_info "Converting Docker Compose to Kubernetes manifests..."

  # Source the compose-to-k8s converter
  if [[ -f "${SCRIPT_DIR}/../lib/k8s/compose-to-k8s.sh" ]]; then
    source "${SCRIPT_DIR}/../lib/k8s/compose-to-k8s.sh"
    k8s_convert_compose "docker-compose.yml" ".nself/k8s/manifests"
    log_success "Conversion complete"
  else
    log_error "K8s converter not found"
    return 1
  fi
}

cmd_k8s_apply() {
  log_info "Applying Kubernetes manifests..."

  if ! command -v kubectl >/dev/null 2>&1; then
    log_error "kubectl not found. Please install kubectl first."
    return 1
  fi

  local manifests_dir=".nself/k8s/manifests"
  if [[ ! -d "$manifests_dir" ]]; then
    log_error "Manifests directory not found. Run 'nself infra k8s convert' first."
    return 1
  fi

  kubectl apply -f "$manifests_dir/"
  log_success "Manifests applied"
}

cmd_k8s_deploy() {
  log_info "Deploying to Kubernetes..."

  cmd_k8s_convert && cmd_k8s_apply
}

cmd_k8s_status() {
  if ! command -v kubectl >/dev/null 2>&1; then
    log_error "kubectl not found"
    return 1
  fi

  printf "\n=== Kubernetes Deployment Status ===\n\n"
  kubectl get all "$@"
}

cmd_k8s_logs() {
  local pod="${1:-}"
  shift || true

  if [[ -z "$pod" ]]; then
    log_error "Pod name required"
    return 1
  fi

  if ! command -v kubectl >/dev/null 2>&1; then
    log_error "kubectl not found"
    return 1
  fi

  kubectl logs "$pod" "$@"
}

cmd_k8s_scale() {
  local deployment="${1:-}"
  shift || true

  if [[ -z "$deployment" ]]; then
    log_error "Deployment name required"
    log_info "Usage: nself infra k8s scale <deployment> --replicas <n>"
    return 1
  fi

  if ! command -v kubectl >/dev/null 2>&1; then
    log_error "kubectl not found"
    return 1
  fi

  kubectl scale deployment "$deployment" "$@"
}

cmd_k8s_rollback() {
  log_info "Rolling back Kubernetes deployment..."

  if ! command -v kubectl >/dev/null 2>&1; then
    log_error "kubectl not found"
    return 1
  fi

  kubectl rollout undo deployment "$@"
}

cmd_k8s_delete() {
  log_warning "This will delete all resources in the manifests directory"
  printf "Are you sure? [y/N]: "
  read -r confirm
  confirm=$(echo "$confirm" | tr '[:upper:]' '[:lower:]')

  if [[ "$confirm" != "y" && "$confirm" != "yes" ]]; then
    log_info "Aborted"
    return 0
  fi

  if ! command -v kubectl >/dev/null 2>&1; then
    log_error "kubectl not found"
    return 1
  fi

  local manifests_dir=".nself/k8s/manifests"
  if [[ -d "$manifests_dir" ]]; then
    kubectl delete -f "$manifests_dir/"
    log_success "Resources deleted"
  else
    log_error "Manifests directory not found"
    return 1
  fi
}

cmd_k8s_cluster() {
  local action="${1:-}"
  shift || true

  case "$action" in
    info)
      kubectl cluster-info "$@"
      ;;
    nodes)
      kubectl get nodes "$@"
      ;;
    create)
      cmd_k8s_cluster_create "$@"
      ;;
    delete | destroy)
      cmd_k8s_cluster_delete "$@"
      ;;
    kubeconfig | config)
      cmd_k8s_cluster_kubeconfig "$@"
      ;;
    list)
      cmd_k8s_cluster_list "$@"
      ;;
    *)
      log_error "Unknown cluster action: $action"
      log_info "Available: info, nodes, create, delete, kubeconfig, list"
      log_info ""
      log_info "Examples:"
      log_info "  nself infra k8s cluster create --provider aws --name prod-cluster --region us-east-1"
      log_info "  nself infra k8s cluster list --provider gcp"
      log_info "  nself infra k8s cluster kubeconfig --provider azure --name prod-cluster"
      log_info "  nself infra k8s cluster delete --provider digitalocean --name dev-cluster"
      return 1
      ;;
  esac
}

# Create managed Kubernetes cluster
cmd_k8s_cluster_create() {
  local provider="${K8S_PROVIDER:-}"
  local cluster_name="${K8S_CLUSTER_NAME:-nself-cluster}"
  local region="${K8S_REGION:-}"
  local node_count="${K8S_NODE_COUNT:-3}"
  local node_size="${K8S_NODE_SIZE:-medium}"

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider)
        provider="$2"
        shift 2
        ;;
      --name)
        cluster_name="$2"
        shift 2
        ;;
      --region | --location | --zone)
        region="$2"
        shift 2
        ;;
      --nodes | --node-count)
        node_count="$2"
        shift 2
        ;;
      --size | --node-size)
        node_size="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ -z "$provider" ]]; then
    log_error "Provider required: --provider <name>"
    log_info "Supported providers: aws, gcp, azure, digitalocean, linode, vultr, scaleway"
    return 1
  fi

  # Source provider interface
  if [[ -f "${SCRIPT_DIR}/../lib/providers/provider-interface.sh" ]]; then
    source "${SCRIPT_DIR}/../lib/providers/provider-interface.sh"
  else
    log_error "Provider interface not found"
    return 1
  fi

  # Check if provider supports Kubernetes
  if ! provider_supports_k8s "$provider"; then
    log_error "Provider '$provider' does not support managed Kubernetes"
    log_info "Supported providers with managed K8s:"
    log_info "  - aws (EKS)"
    log_info "  - gcp (GKE)"
    log_info "  - azure (AKS)"
    log_info "  - digitalocean (DOKS)"
    log_info "  - linode (LKE)"
    log_info "  - vultr (VKE)"
    log_info "  - scaleway (Kapsule)"
    return 1
  fi

  # Create cluster using provider interface
  log_info "Creating managed Kubernetes cluster via $provider..."
  provider_k8s_create "$provider" "$cluster_name" "$region" "$node_count" "$node_size"
}

# Delete managed Kubernetes cluster
cmd_k8s_cluster_delete() {
  local provider="${K8S_PROVIDER:-}"
  local cluster_name="${K8S_CLUSTER_NAME:-}"

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider)
        provider="$2"
        shift 2
        ;;
      --name)
        cluster_name="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ -z "$provider" ]]; then
    log_error "Provider required: --provider <name>"
    return 1
  fi

  if [[ -z "$cluster_name" ]]; then
    log_error "Cluster name required: --name <cluster-name>"
    return 1
  fi

  # Source provider interface
  if [[ -f "${SCRIPT_DIR}/../lib/providers/provider-interface.sh" ]]; then
    source "${SCRIPT_DIR}/../lib/providers/provider-interface.sh"
  else
    log_error "Provider interface not found"
    return 1
  fi

  # Confirm deletion
  log_warning "This will delete the Kubernetes cluster: $cluster_name"
  printf "Are you sure? [y/N]: "
  read -r confirm
  confirm=$(echo "$confirm" | tr '[:upper:]' '[:lower:]')

  if [[ "$confirm" != "y" && "$confirm" != "yes" ]]; then
    log_info "Aborted"
    return 0
  fi

  # Delete cluster using provider interface
  log_info "Deleting managed Kubernetes cluster from $provider..."
  provider_k8s_delete "$provider" "$cluster_name"
}

# Get kubeconfig for managed Kubernetes cluster
cmd_k8s_cluster_kubeconfig() {
  local provider="${K8S_PROVIDER:-}"
  local cluster_name="${K8S_CLUSTER_NAME:-}"

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider)
        provider="$2"
        shift 2
        ;;
      --name)
        cluster_name="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ -z "$provider" ]]; then
    log_error "Provider required: --provider <name>"
    return 1
  fi

  if [[ -z "$cluster_name" ]]; then
    log_error "Cluster name required: --name <cluster-name>"
    return 1
  fi

  # Source provider interface
  if [[ -f "${SCRIPT_DIR}/../lib/providers/provider-interface.sh" ]]; then
    source "${SCRIPT_DIR}/../lib/providers/provider-interface.sh"
  else
    log_error "Provider interface not found"
    return 1
  fi

  # Get kubeconfig using provider interface
  log_info "Retrieving kubeconfig for cluster: $cluster_name"
  provider_k8s_kubeconfig "$provider" "$cluster_name"
}

# List Kubernetes clusters (placeholder - requires provider-specific implementation)
cmd_k8s_cluster_list() {
  local provider="${K8S_PROVIDER:-}"

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --provider)
        provider="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done

  if [[ -z "$provider" ]]; then
    log_info "Listing clusters requires --provider flag"
    log_info "Example: nself infra k8s cluster list --provider aws"
    return 1
  fi

  # Source provider interface
  if [[ -f "${SCRIPT_DIR}/../lib/providers/provider-interface.sh" ]]; then
    source "${SCRIPT_DIR}/../lib/providers/provider-interface.sh"
  fi

  log_info "Listing Kubernetes clusters for $provider..."

  # Provider-specific listing
  case "$provider" in
    aws)
      if command -v aws &>/dev/null; then
        aws eks list-clusters --output table
      else
        log_error "AWS CLI not installed"
      fi
      ;;
    gcp)
      if command -v gcloud &>/dev/null; then
        gcloud container clusters list
      else
        log_error "gcloud CLI not installed"
      fi
      ;;
    azure)
      if command -v az &>/dev/null; then
        az aks list --output table
      else
        log_error "Azure CLI not installed"
      fi
      ;;
    digitalocean)
      if command -v doctl &>/dev/null; then
        doctl kubernetes cluster list
      else
        log_error "doctl CLI not installed"
      fi
      ;;
    linode)
      if command -v linode-cli &>/dev/null; then
        linode-cli lke clusters-list
      else
        log_error "linode-cli not installed"
      fi
      ;;
    vultr)
      if command -v vultr-cli &>/dev/null; then
        vultr-cli kubernetes list
      else
        log_error "vultr-cli not installed"
      fi
      ;;
    scaleway)
      if command -v scw &>/dev/null; then
        scw k8s cluster list
      else
        log_error "scw CLI not installed"
      fi
      ;;
    *)
      log_error "Unknown provider: $provider"
      ;;
  esac
}

cmd_k8s_namespace() {
  local action="${1:-}"
  shift || true

  case "$action" in
    list | ls)
      kubectl get namespaces "$@"
      ;;
    create)
      kubectl create namespace "$@"
      ;;
    delete)
      kubectl delete namespace "$@"
      ;;
    *)
      log_error "Unknown namespace action: $action"
      log_info "Available: list, create, delete"
      return 1
      ;;
  esac
}

# === HELM HELP AND IMPLEMENTATIONS ===
cmd_helm_init() {
  log_info "Initializing Helm chart..."

  local chart_dir=".nself/helm/chart"
  mkdir -p "$chart_dir/templates"

  # Create Chart.yaml
  cat >"$chart_dir/Chart.yaml" <<EOF
apiVersion: v2
name: $(basename "$(pwd)")
description: A Helm chart for nself project
type: application
version: 0.1.0
appVersion: "1.0"
EOF

  # Create values.yaml
  cat >"$chart_dir/values.yaml" <<EOF
# Default values for nself project
replicaCount: 2

image:
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 80

ingress:
  enabled: false
  className: "nginx"
  hosts:
    - host: chart-example.local
      paths:
        - path: /
          pathType: Prefix

resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi
EOF

  log_success "Helm chart initialized"
  log_info "  Directory: $chart_dir"
  log_info "  Next: Run 'nself infra helm generate' to populate templates"
}

cmd_helm_generate() {
  log_info "Generating Helm chart from Docker Compose..."

  # First convert to K8s manifests
  if [[ -f "${SCRIPT_DIR}/../lib/k8s/compose-to-k8s.sh" ]]; then
    source "${SCRIPT_DIR}/../lib/k8s/compose-to-k8s.sh"

    local temp_dir=".nself/k8s/temp"
    k8s_convert_compose "docker-compose.yml" "$temp_dir"

    # Move manifests to Helm templates
    local chart_templates=".nself/helm/chart/templates"
    mkdir -p "$chart_templates"

    # Copy and templatize manifests
    for manifest in "$temp_dir"/*.yaml; do
      [[ -f "$manifest" ]] || continue
      cp "$manifest" "$chart_templates/$(basename "$manifest")"
    done

    rm -rf "$temp_dir"

    log_success "Helm chart generated"
  else
    log_error "K8s converter not found"
    return 1
  fi
}

cmd_helm_install() {
  local release="${1:-}"
  shift || true

  if [[ -z "$release" ]]; then
    log_error "Release name required"
    return 1
  fi

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found. Please install Helm first."
    return 1
  fi

  local chart_dir=".nself/helm/chart"
  if [[ ! -d "$chart_dir" ]]; then
    log_error "Helm chart not found. Run 'nself infra helm init' first."
    return 1
  fi

  helm install "$release" "$chart_dir" "$@"
}

cmd_helm_upgrade() {
  local release="${1:-}"
  shift || true

  if [[ -z "$release" ]]; then
    log_error "Release name required"
    return 1
  fi

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  helm upgrade "$release" ".nself/helm/chart" "$@"
}

cmd_helm_rollback() {
  local release="${1:-}"
  shift || true

  if [[ -z "$release" ]]; then
    log_error "Release name required"
    return 1
  fi

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  helm rollback "$release" "$@"
}

cmd_helm_uninstall() {
  local release="${1:-}"
  shift || true

  if [[ -z "$release" ]]; then
    log_error "Release name required"
    return 1
  fi

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  helm uninstall "$release" "$@"
}

cmd_helm_list() {
  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  helm list "$@"
}

cmd_helm_status() {
  local release="${1:-}"
  shift || true

  if [[ -z "$release" ]]; then
    log_error "Release name required"
    return 1
  fi

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  helm status "$release" "$@"
}

cmd_helm_values() {
  local release="${1:-}"
  shift || true

  if [[ -z "$release" ]]; then
    log_error "Release name required"
    return 1
  fi

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  helm get values "$release" "$@"
}

cmd_helm_template() {
  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  local chart_dir=".nself/helm/chart"
  if [[ ! -d "$chart_dir" ]]; then
    log_error "Helm chart not found. Run 'nself infra helm init' first."
    return 1
  fi

  helm template "$chart_dir" "$@"
}

cmd_helm_package() {
  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  local chart_dir=".nself/helm/chart"
  if [[ ! -d "$chart_dir" ]]; then
    log_error "Helm chart not found. Run 'nself infra helm init' first."
    return 1
  fi

  helm package "$chart_dir" "$@"
}

cmd_helm_repo() {
  local action="${1:-}"
  shift || true

  if ! command -v helm >/dev/null 2>&1; then
    log_error "helm not found"
    return 1
  fi

  case "$action" in
    add | remove | update | list)
      helm repo "$action" "$@"
      ;;
    *)
      log_error "Unknown repo action: $action"
      log_info "Available: add, remove, update, list"
      return 1
      ;;
  esac
}

# === MAIN ENTRY POINT ===
main() {
  local category="${1:-}"
  shift || true

  case "$category" in
    "" | help | -h | --help)
      show_infra_help
      ;;
    provider | providers)
      cmd_infra_provider "$@"
      ;;
    k8s | kubernetes)
      cmd_infra_k8s "$@"
      ;;
    helm)
      cmd_infra_helm "$@"
      ;;
    destroy)
      source "$(dirname "${BASH_SOURCE[0]}")/destroy.sh"
      cmd_destroy "$@"
      ;;
    reset)
      source "$(dirname "${BASH_SOURCE[0]}")/backup.sh"
      cmd_backup reset "$@"
      ;;
    clean)
      source "$(dirname "${BASH_SOURCE[0]}")/backup.sh"
      cmd_backup clean "$@"
      ;;
    *)
      log_error "Unknown category: $category"
      printf "\nAvailable categories: provider, k8s, helm, destroy, reset, clean\n"
      printf "Run 'nself infra --help' for more information.\n"
      return 1
      ;;
  esac
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
