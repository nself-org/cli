# Kubernetes Implementation Guide for nself

**Commands:** `nself infra k8s` and `nself infra provider k8s-*`

Complete guide to nself's Kubernetes abstraction layer - deploy managed Kubernetes clusters across 8 cloud providers with a unified CLI.

---

## Quick Navigation

- [Overview](#overview)
- [Supported Providers](#supported-providers)
- [Quick Start](#quick-start-deploy-in-5-minutes)
- [CLI Installation](#cli-installation-requirements)
- [Usage Examples](#usage-examples)
- [Provider Details](#provider-specific-details)
- [Cost Comparison](#cost-comparison)
- [Troubleshooting](#troubleshooting)

---

This guide documents the Kubernetes abstraction layer implementation across all supported cloud providers in nself.

## Table of Contents

- [Overview](#overview)
- [Supported Providers](#supported-providers)
- [CLI Installation Requirements](#cli-installation-requirements)
- [Usage Examples](#usage-examples)
- [Provider-Specific Details](#provider-specific-details)
- [Cost Comparison](#cost-comparison)
- [Troubleshooting](#troubleshooting)

---

## Overview

nself provides a unified abstraction layer for managed Kubernetes clusters across 8 major cloud providers. Each provider implements three core functions:

1. **`provider_k8s_create`** - Create a managed Kubernetes cluster
2. **`provider_k8s_delete`** - Delete a managed Kubernetes cluster
3. **`provider_k8s_kubeconfig`** - Retrieve kubeconfig credentials

The abstraction layer allows consistent commands across all providers while handling provider-specific CLI tools and authentication automatically.

---

## Supported Providers

| Provider | Service | Control Plane Fee | CLI Tool | Status |
|----------|---------|------------------|----------|--------|
| **AWS** | EKS (Elastic Kubernetes Service) | $0.10/hour (~$73/mo) | `aws` / `eksctl` | ✅ Full |
| **GCP** | GKE (Google Kubernetes Engine) | Free | `gcloud` | ✅ Full |
| **Azure** | AKS (Azure Kubernetes Service) | Free | `az` | ✅ Full |
| **DigitalOcean** | DOKS | $12/month | `doctl` | ✅ Full |
| **Linode** | LKE (Linode Kubernetes Engine) | Free | `linode-cli` | ✅ Full |
| **Vultr** | VKE (Vultr Kubernetes Engine) | Free | `vultr-cli` | ✅ Full |
| **Hetzner** | Managed Kubernetes | Free | `hcloud` | ⚠️ Manual (Console only) |
| **Scaleway** | Kapsule | Free | `scw` | ✅ Full |

**Legend:**
- ✅ Full: Complete CLI automation
- ⚠️ Manual: Requires web console or Terraform

---

## CLI Installation Requirements

### AWS (EKS)

```bash
# macOS
brew install awscli eksctl

# Ubuntu/Debian
apt install awscli
curl --silent --location "https://github.com/weaveworks/eksctl/releases/latest/download/eksctl_$(uname -s)_amd64.tar.gz" | tar xz -C /tmp
sudo mv /tmp/eksctl /usr/local/bin
```

**Initialize:**
```bash
nself infra provider init aws
# Or: aws configure
```

### GCP (GKE)

```bash
# macOS
brew install google-cloud-sdk

# Ubuntu/Debian
echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | sudo tee /etc/apt/sources.list.d/google-cloud-sdk.list
curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
apt update && apt install google-cloud-sdk
```

**Initialize:**
```bash
nself infra provider init gcp
# Or: gcloud auth login && gcloud init
```

### Azure (AKS)

```bash
# macOS
brew install azure-cli

# Ubuntu/Debian
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
```

**Initialize:**
```bash
nself infra provider init azure
# Or: az login
```

### DigitalOcean (DOKS)

```bash
# macOS
brew install doctl

# Ubuntu/Linux
snap install doctl
```

**Initialize:**
```bash
nself infra provider init digitalocean
# Or: doctl auth init
```

### Linode (LKE)

```bash
# All platforms
pip3 install linode-cli
```

**Initialize:**
```bash
nself infra provider init linode
# Or: linode-cli configure
```

### Vultr (VKE)

```bash
# macOS
brew install vultr/vultr-cli/vultr-cli

# Linux
wget https://github.com/vultr/vultr-cli/releases/latest/download/vultr-cli_linux_amd64.tar.gz
tar xf vultr-cli_linux_amd64.tar.gz
sudo mv vultr-cli /usr/local/bin/
```

**Initialize:**
```bash
nself infra provider init vultr
# Or: export VULTR_API_KEY="your-api-key"
```

### Hetzner

```bash
# macOS
brew install hcloud

# Linux
wget https://github.com/hetznercloud/cli/releases/latest/download/hcloud-linux-amd64.tar.gz
tar xf hcloud-linux-amd64.tar.gz
sudo mv hcloud /usr/local/bin/
```

**Initialize:**
```bash
nself infra provider init hetzner
# Or: hcloud context create nself --token <token>
```

### Scaleway (Kapsule)

```bash
# macOS
brew install scw

# Linux
curl -s https://raw.githubusercontent.com/scaleway/scaleway-cli/master/scripts/get.sh | sh
```

**Initialize:**
```bash
nself infra provider init scaleway
# Or: scw init
```

---

## Quick Start: Deploy in 5 Minutes

### 1. Install Provider CLI

```bash
# Choose your provider and install CLI
# AWS
brew install awscli eksctl

# GCP
brew install google-cloud-sdk

# DigitalOcean
brew install doctl

# Or use nself to help
nself infra provider install aws
```

### 2. Initialize Provider

```bash
# Interactive setup
nself infra provider init aws

# Or manual
aws configure
```

### 3. Create Cluster

```bash
# One command to create managed K8s cluster
nself infra provider k8s-create aws production-cluster us-east-1 3 medium

# Wait 10-15 minutes for cluster provisioning
```

### 4. Get kubeconfig

```bash
# Automatically configures kubectl
nself infra provider k8s-kubeconfig aws production-cluster us-east-1

# Verify connection
kubectl get nodes
```

### 5. Deploy nself

```bash
# Deploy your application to K8s
nself infra k8s deploy production
```

---

## Usage Examples

### Basic Cluster Creation

```bash
# Using nself abstraction (recommended)
nself infra provider k8s-create aws nself-production us-east-1 3 medium

# Provider-specific parameters:
# AWS:          <cluster-name> <region> <node-count> <node-size>
# GCP:          <cluster-name> <region> <node-count> <node-size>
# Azure:        <cluster-name> <location> <node-count> <node-size>
# DigitalOcean: <cluster-name> <region> <node-count> <node-size>
# Linode:       <cluster-name> <region> <node-count> <node-size>
# Vultr:        <cluster-name> <region> <node-count> <node-size>
# Scaleway:     <cluster-name> <zone> <node-count> <node-size>
```

### Provider-Specific Examples

#### AWS EKS

```bash
# Create cluster (automatic kubeconfig setup)
nself infra provider k8s-create aws production-cluster us-west-2 3 medium

# Manual creation with eksctl (if preferred)
eksctl create cluster \
  --name production-cluster \
  --region us-west-2 \
  --nodes 3 \
  --node-type t3.large

# Get kubeconfig
nself infra provider k8s-kubeconfig aws production-cluster us-west-2

# Delete cluster
nself infra provider k8s-delete aws production-cluster us-west-2
```

**Cost:** ~$73/month (control plane) + node costs (~$60/node for t3.large)

#### GCP GKE

```bash
# Create cluster
nself infra provider k8s-create gcp production-cluster us-central1 3 medium

# Manual with gcloud
gcloud container clusters create production-cluster \
  --region us-central1 \
  --num-nodes 3 \
  --machine-type e2-standard-2 \
  --enable-autoscaling --min-nodes 1 --max-nodes 5

# Get kubeconfig
nself infra provider k8s-kubeconfig gcp production-cluster us-central1

# Delete cluster
nself infra provider k8s-delete gcp production-cluster us-central1
```

**Cost:** Free control plane + node costs (~$48/node for e2-standard-2)

#### Azure AKS

```bash
# Create cluster (creates resource group automatically)
nself infra provider k8s-create azure production-cluster eastus 3 medium

# Manual with az CLI
az aks create \
  --resource-group production-cluster-rg \
  --name production-cluster \
  --location eastus \
  --node-count 3 \
  --node-vm-size Standard_D4s_v3

# Get kubeconfig
nself infra provider k8s-kubeconfig azure production-cluster

# Delete cluster (deletes resource group too)
nself infra provider k8s-delete azure production-cluster
```

**Cost:** Free control plane + node costs (~$140/node for D4s_v3)

#### DigitalOcean DOKS

```bash
# Create cluster
nself infra provider k8s-create digitalocean production-cluster nyc3 3 medium

# Manual with doctl
doctl kubernetes cluster create production-cluster \
  --region nyc3 \
  --size s-4vcpu-8gb \
  --count 3 \
  --auto-upgrade

# Get kubeconfig
nself infra provider k8s-kubeconfig digitalocean production-cluster

# Delete cluster
nself infra provider k8s-delete digitalocean production-cluster
```

**Cost:** $12/month (cluster) + node costs ($48/node for s-4vcpu-8gb)

#### Linode LKE

```bash
# Create cluster
nself infra provider k8s-create linode production-cluster us-east 3 medium

# Manual with linode-cli
linode-cli lke cluster-create \
  --label production-cluster \
  --region us-east \
  --k8s_version "1.28" \
  --node_pools.type g6-standard-4 \
  --node_pools.count 3

# Get kubeconfig
nself infra provider k8s-kubeconfig linode production-cluster

# Delete cluster
nself infra provider k8s-delete linode production-cluster
```

**Cost:** Free control plane + node costs ($48/node for g6-standard-4)

#### Vultr VKE

```bash
# Create cluster
nself infra provider k8s-create vultr production-cluster ewr 3 medium

# Manual with vultr-cli
vultr-cli kubernetes create \
  --label production-cluster \
  --region ewr \
  --version "v1.28.2+1" \
  --node-pool "quantity=3,plan=vc2-4c-8gb,label=nodepool"

# Get kubeconfig
nself infra provider k8s-kubeconfig vultr <cluster-id>

# Delete cluster
nself infra provider k8s-delete vultr <cluster-id>
```

**Cost:** Free control plane + node costs ($48/node for vc2-4c-8gb)

#### Hetzner Kubernetes

**Note:** Hetzner Kubernetes requires manual creation via web console or Terraform.

```bash
# Manual creation
# 1. Visit: https://console.hetzner.cloud/
# 2. Create Kubernetes cluster
# 3. Download kubeconfig
# 4. Save to ~/.kube/config

# Alternative: Use Terraform
terraform init
terraform apply
```

**Cost:** Free control plane + node costs (~$11/node for cx31)

#### Scaleway Kapsule

```bash
# Create cluster
nself infra provider k8s-create scaleway production-cluster fr-par-1 3 medium

# Manual with scw
scw k8s cluster create name=production-cluster region=fr-par version=1.28.0
scw k8s pool create cluster-id=<id> name=default-pool node-type=DEV1-L size=3

# Get kubeconfig
nself infra provider k8s-kubeconfig scaleway production-cluster fr-par-1

# Delete cluster
nself infra provider k8s-delete scaleway production-cluster fr-par-1
```

**Cost:** Free control plane + node costs (~$24/node for DEV1-L)

---

## Provider-Specific Details

### AWS EKS

**Features:**
- Highly integrated with AWS ecosystem
- Best for existing AWS infrastructure
- Supports Fargate (serverless nodes)
- Advanced IAM integration

**Considerations:**
- Control plane costs $73/month (highest among providers)
- Requires VPC, subnets, security groups setup
- eksctl recommended for easier cluster management

**Node Size Mapping:**
- `small`: t3.medium (2 vCPU, 4GB RAM)
- `medium`: t3.large (2 vCPU, 8GB RAM)
- `large`: t3.xlarge (4 vCPU, 16GB RAM)
- `xlarge`: t3.2xlarge (8 vCPU, 32GB RAM)

### GCP GKE

**Features:**
- No control plane fees
- Excellent auto-scaling
- Good integration with Google services
- Autopilot mode available (fully managed)

**Considerations:**
- Requires active GCP project
- Regional clusters recommended for HA
- Strong monitoring with Google Cloud Operations

**Node Size Mapping:**
- `small`: e2-medium (2 vCPU, 4GB RAM)
- `medium`: e2-standard-2 (2 vCPU, 8GB RAM)
- `large`: e2-standard-4 (4 vCPU, 16GB RAM)
- `xlarge`: e2-standard-8 (8 vCPU, 32GB RAM)

### Azure AKS

**Features:**
- No control plane fees
- Strong enterprise integration
- Azure AD authentication
- Virtual nodes (serverless) support

**Considerations:**
- Creates separate resource group
- Deleting cluster can leave orphaned resources
- Good Windows container support

**Node Size Mapping:**
- `small`: Standard_D2s_v3 (2 vCPU, 8GB RAM)
- `medium`: Standard_D4s_v3 (4 vCPU, 16GB RAM)
- `large`: Standard_D8s_v3 (8 vCPU, 32GB RAM)
- `xlarge`: Standard_D16s_v3 (16 vCPU, 64GB RAM)

### DigitalOcean DOKS

**Features:**
- Simplest setup among all providers
- Predictable pricing ($12/month cluster fee)
- Auto-upgrade and surge-upgrade available
- 1TB free bandwidth per droplet

**Considerations:**
- Limited to DigitalOcean regions
- Basic feature set compared to major clouds
- Good for small-medium workloads

**Node Size Mapping:**
- `small`: s-2vcpu-4gb
- `medium`: s-4vcpu-8gb
- `large`: s-8vcpu-16gb
- `xlarge`: s-16vcpu-32gb

### Linode LKE

**Features:**
- No control plane fees
- Simple pricing model
- Good performance/price ratio
- 40Gbps network connectivity

**Considerations:**
- Kubeconfig requires base64 decoding
- Fewer global regions than major clouds
- Solid choice for cost-conscious deployments

**Node Size Mapping:**
- `small`: g6-standard-2
- `medium`: g6-standard-4
- `large`: g6-standard-8
- `xlarge`: g6-standard-16

### Vultr VKE

**Features:**
- No control plane fees
- 32 global locations
- High-performance infrastructure
- Simple API

**Considerations:**
- Requires cluster ID (not name) for operations
- Newer service (less mature than competitors)
- Good global coverage

**Node Size Mapping:**
- `small`: vc2-2c-4gb
- `medium`: vc2-4c-8gb
- `large`: vc2-8c-16gb
- `xlarge`: vc2-16c-32gb

### Hetzner Kubernetes

**Features:**
- No control plane fees
- Best price/performance ratio
- European data centers
- Excellent network performance

**Considerations:**
- No CLI support for cluster creation (console only)
- Limited to European locations
- Requires manual setup or Terraform
- Best for EU-based workloads

**Node Size Mapping:**
- `small`: cpx21 (3 vCPU, 4GB RAM)
- `medium`: cx31 (2 vCPU, 8GB RAM)
- `large`: cx41 (4 vCPU, 16GB RAM)
- `xlarge`: cx51 (8 vCPU, 32GB RAM)

### Scaleway Kapsule

**Features:**
- No control plane fees
- European cloud provider
- Good compliance with EU regulations
- Free tier available

**Considerations:**
- Requires separate cluster and pool creation
- Zone-based (not region-based) like others
- CLI uses region for cluster, zone for nodes

**Node Size Mapping:**
- `small`: DEV1-M (3 vCPU, 4GB RAM)
- `medium`: DEV1-L (4 vCPU, 8GB RAM)
- `large`: GP1-XS (4 vCPU, 16GB RAM)
- `xlarge`: GP1-S (8 vCPU, 32GB RAM)

---

## Cost Comparison

### Control Plane Costs

| Provider | Control Plane Cost | Notes |
|----------|-------------------|-------|
| AWS EKS | **$73/month** | $0.10/hour |
| GCP GKE | **Free** | No cluster management fee |
| Azure AKS | **Free** | No cluster management fee |
| DigitalOcean DOKS | **$12/month** | Fixed monthly fee |
| Linode LKE | **Free** | No cluster management fee |
| Vultr VKE | **Free** | No cluster management fee |
| Hetzner | **Free** | No cluster management fee |
| Scaleway Kapsule | **Free** | No cluster management fee |

### 3-Node Cluster Cost Estimate (Medium Size)

| Provider | Control Plane | Nodes (3x) | Total/Month |
|----------|--------------|-----------|-------------|
| **AWS EKS** | $73 | $180 (t3.large) | **$253** |
| **GCP GKE** | $0 | $144 (e2-standard-2) | **$144** |
| **Azure AKS** | $0 | $420 (D4s_v3) | **$420** |
| **DigitalOcean DOKS** | $12 | $144 (s-4vcpu-8gb) | **$156** |
| **Linode LKE** | $0 | $144 (g6-standard-4) | **$144** |
| **Vultr VKE** | $0 | $144 (vc2-4c-8gb) | **$144** |
| **Hetzner** | $0 | $33 (cx31) | **$33** ⭐ Best Value |
| **Scaleway Kapsule** | $0 | $72 (DEV1-L) | **$72** |

**Winner for Cost:** Hetzner ($33/month for 3-node medium cluster)
**Winner for Features:** AWS EKS (most mature, best AWS integration)
**Winner for Simplicity:** DigitalOcean DOKS (easiest setup)
**Winner for Balance:** GCP GKE or Linode LKE (free control plane, good features)

---

## Troubleshooting

### Common Issues

#### 1. CLI Tool Not Found

**Error:** `bash: aws: command not found`

**Solution:**
```bash
# Install the required CLI tool for your provider
# See "CLI Installation Requirements" section above

# Verify installation
aws --version        # AWS
gcloud --version     # GCP
az --version         # Azure
doctl version        # DigitalOcean
linode-cli --version # Linode
vultr-cli version    # Vultr
hcloud version       # Hetzner
scw version          # Scaleway
```

#### 2. Authentication Failed

**Error:** `Error: credentials invalid or expired`

**Solution:**
```bash
# Re-initialize provider
nself infra provider init <provider>

# Or use provider-specific auth
aws configure              # AWS
gcloud auth login          # GCP
az login                   # Azure
doctl auth init            # DigitalOcean
linode-cli configure       # Linode
export VULTR_API_KEY="..." # Vultr
hcloud context create ...  # Hetzner
scw init                   # Scaleway
```

#### 3. Cluster Creation Timeout

**Error:** Operation times out after 10-15 minutes

**Possible causes:**
- Provider API rate limiting
- Resource quota exceeded
- Network connectivity issues
- Invalid region/zone

**Solution:**
```bash
# Check provider status
nself infra provider validate <provider>

# Verify quotas (provider-specific)
aws service-quotas list-service-quotas --service-code eks
gcloud compute project-info describe --project=<project>
az vm list-usage --location <location>

# Use different region/zone
nself infra provider k8s-create <provider> cluster <different-region> 3 medium
```

#### 4. Kubeconfig Not Updated

**Error:** `kubectl` still points to old cluster

**Solution:**
```bash
# Manually retrieve kubeconfig
nself infra provider k8s-kubeconfig <provider> <cluster-name>

# Verify current context
kubectl config current-context

# Switch context if needed
kubectl config use-context <context-name>

# View all contexts
kubectl config get-contexts
```

#### 5. Node Pool Creation Failed

**Error:** Cluster created but nodes not ready

**Solution:**
```bash
# Check node status
kubectl get nodes

# Describe node issues
kubectl describe node <node-name>

# Provider-specific node checks
aws eks describe-nodegroup --cluster-name <name> --nodegroup-name <ng>
gcloud container node-pools list --cluster <name>
az aks nodepool list --cluster-name <name> --resource-group <rg>
doctl kubernetes cluster node-pool list <cluster-id>
```

#### 6. Delete Operation Stuck

**Error:** Cluster deletion hangs indefinitely

**Solution:**
```bash
# AWS EKS: Delete node groups first
eksctl delete nodegroup --cluster=<name> --name=<nodegroup>
eksctl delete cluster --name=<name>

# GCP: Force delete with --quiet
gcloud container clusters delete <name> --region=<region> --quiet

# Azure: Delete resource group (nuclear option)
az group delete --name <cluster-name>-rg --yes --no-wait

# DigitalOcean: Force delete
doctl kubernetes cluster delete <cluster-id> --force

# Check for orphaned resources and clean up manually
```

---

## Advanced Usage

### Multi-Provider Deployments

```bash
# Create clusters in multiple providers for redundancy
nself infra provider k8s-create aws prod-aws us-east-1 3 medium
nself infra provider k8s-create gcp prod-gcp us-central1 3 medium
nself infra provider k8s-create azure prod-azure eastus 3 medium

# Get all kubeconfigs
nself infra provider k8s-kubeconfig aws prod-aws us-east-1
nself infra provider k8s-kubeconfig gcp prod-gcp us-central1
nself infra provider k8s-kubeconfig azure prod-azure eastus

# Use kubectx to switch between clusters
kubectx                          # List contexts
kubectx prod-aws                 # Switch to AWS cluster
kubectx prod-gcp                 # Switch to GCP cluster
```

### Cost Optimization

```bash
# Use spot/preemptible instances (provider-specific)

# AWS: Add spot node group after cluster creation
eksctl create nodegroup \
  --cluster=<name> \
  --spot \
  --instance-types=t3.medium,t3.large

# GCP: Use preemptible nodes
gcloud container clusters create <name> \
  --preemptible \
  --enable-autoscaling

# Azure: Use spot VMs
az aks nodepool add \
  --cluster-name <name> \
  --name spot-pool \
  --priority Spot \
  --eviction-policy Delete
```

### Backup and Disaster Recovery

```bash
# Install Velero for cluster backup
kubectl apply -f https://github.com/vmware-tanzu/velero/releases/download/v1.12.0/velero-v1.12.0-linux-amd64.tar.gz

# Backup cluster resources
velero backup create my-backup

# Restore to different cluster
velero restore create --from-backup my-backup

# Schedule regular backups
velero schedule create daily-backup --schedule="0 2 * * *"
```

---

## Additional Resources

### Official Documentation

- **AWS EKS:** https://docs.aws.amazon.com/eks/
- **GCP GKE:** https://cloud.google.com/kubernetes-engine/docs
- **Azure AKS:** https://docs.microsoft.com/en-us/azure/aks/
- **DigitalOcean DOKS:** https://docs.digitalocean.com/products/kubernetes/
- **Linode LKE:** https://www.linode.com/docs/kubernetes/
- **Vultr VKE:** https://www.vultr.com/docs/vultr-kubernetes-engine/
- **Hetzner:** https://docs.hetzner.com/cloud/kubernetes/
- **Scaleway Kapsule:** https://www.scaleway.com/en/docs/compute/kubernetes/

### Kubernetes Tools

- **kubectl:** https://kubernetes.io/docs/tasks/tools/
- **kubectx:** https://github.com/ahmetb/kubectx
- **k9s:** https://k9scli.io/
- **Lens:** https://k8slens.dev/
- **Helm:** https://helm.sh/

### nself Documentation

- **Provider Interface:** `/src/lib/providers/provider-interface.sh`
- **CLI Commands:** `nself infra provider --help`
- **Configuration:** `~/.nself/providers/<provider>.yml`

---

## nself K8s Abstraction Benefits

### Unified CLI Across Providers

**Without nself:**
```bash
# AWS (3 different tools)
eksctl create cluster --name prod --region us-east-1
aws eks update-kubeconfig --name prod --region us-east-1
eksctl delete cluster --name prod

# GCP (different syntax)
gcloud container clusters create prod --region us-central1
gcloud container clusters get-credentials prod --region us-central1
gcloud container clusters delete prod --region us-central1

# Azure (completely different)
az aks create --resource-group rg --name prod --location eastus
az aks get-credentials --resource-group rg --name prod
az aks delete --resource-group rg --name prod
```

**With nself:**
```bash
# Same commands for all providers!
nself infra provider k8s-create <provider> prod <region> 3 medium
nself infra provider k8s-kubeconfig <provider> prod <region>
nself infra provider k8s-delete <provider> prod <region>
```

### Intelligent Node Size Mapping

nself maps human-readable sizes to provider-specific instance types:

```bash
# Same command, provider chooses appropriate instance
nself infra provider k8s-create aws prod us-east-1 3 medium
# AWS: Uses t3.large (2 vCPU, 8GB)

nself infra provider k8s-create gcp prod us-central1 3 medium
# GCP: Uses e2-standard-2 (2 vCPU, 8GB)

nself infra provider k8s-create azure prod eastus 3 medium
# Azure: Uses Standard_D4s_v3 (4 vCPU, 16GB)
```

**Size Options:**
- `small` - Development workloads (~2 vCPU, 4GB RAM)
- `medium` - Production workloads (~2-4 vCPU, 8-16GB RAM)
- `large` - High-performance apps (~4-8 vCPU, 16-32GB RAM)
- `xlarge` - Enterprise workloads (~8-16 vCPU, 32-64GB RAM)

### Provider Detection and Validation

```bash
# Check provider setup
nself infra provider validate aws

# Test connectivity
nself infra provider test aws

# View provider info
nself infra provider info aws
```

### Multi-Cloud Management

```bash
# Deploy to multiple clouds simultaneously
nself infra provider k8s-create aws prod-aws us-east-1 3 medium
nself infra provider k8s-create gcp prod-gcp us-central1 3 medium
nself infra provider k8s-create azure prod-azure eastus 3 medium

# Switch between clusters easily
kubectx prod-aws
kubectx prod-gcp
kubectx prod-azure
```

---

## Contributing

To add a new provider or improve existing implementations:

1. Create provider module: `/src/lib/providers/<provider>.sh`
2. Implement required functions:
   - `provider_<name>_k8s_create`
   - `provider_<name>_k8s_delete`
   - `provider_<name>_k8s_kubeconfig`
3. Add K8s support to `provider-interface.sh`:
   ```bash
   _get_k8s_support() {
     case "$provider" in
       newprovider) echo "service-name" ;;
     esac
   }
   ```
4. Test all three functions
5. Update this guide with provider details
6. Submit pull request

---

**Last Updated:** January 2026
**nself Version:** 0.9.6+
**Maintainer:** nself core team

For issues or questions, visit: https://github.com/nself-org/cli/issues
