# nself infra

**Category**: Infrastructure Commands

Manage cloud infrastructure, Kubernetes deployments, and Helm charts.

## Overview

All infrastructure operations use `nself infra <subcommand>` for managing cloud providers, Kubernetes clusters, and infrastructure as code.

**Features**:
- ✅ Multi-cloud provider support
- ✅ Kubernetes cluster management
- ✅ Helm chart deployment
- ✅ Infrastructure as code
- ✅ Resource provisioning
- ✅ Cost optimization

## Command Categories

### Cloud Providers (15 subcommands)
- provider list, connect, disconnect, configure, regions, pricing, resources

### Kubernetes (15 subcommands)
- k8s init, deploy, scale, rollback, logs, exec, port-forward, config

### Helm Charts (8 subcommands)
- helm install, upgrade, rollback, list, values, template, package

## Subcommands Reference

### Cloud Provider Management

| Subcommand | Description |
|------------|-------------|
| `provider list` | List supported providers |
| `provider connect` | Connect cloud provider |
| `provider disconnect` | Disconnect provider |
| `provider configure` | Configure provider |
| `provider regions` | List regions |
| `provider pricing` | Show pricing |
| `provider resources` | List resources |
| `provider create-server` | Create server/VM |
| `provider create-cluster` | Create K8s cluster |
| `provider create-database` | Create managed database |
| `provider create-storage` | Create object storage |
| `provider create-network` | Create VPC/network |
| `provider create-loadbalancer` | Create load balancer |
| `provider list-resources` | List all resources |
| `provider estimate-cost` | Estimate costs |

### Kubernetes Management

| Subcommand | Description |
|------------|-------------|
| `k8s init` | Initialize K8s deployment |
| `k8s deploy` | Deploy to Kubernetes |
| `k8s scale` | Scale deployment |
| `k8s rollback` | Rollback deployment |
| `k8s logs` | View pod logs |
| `k8s exec` | Execute in pod |
| `k8s port-forward` | Port forwarding |
| `k8s config` | Manage kubeconfig |
| `k8s apply` | Apply manifest |
| `k8s delete` | Delete resources |
| `k8s get` | Get resources |
| `k8s describe` | Describe resource |
| `k8s events` | View events |
| `k8s top` | Resource usage |
| `k8s context` | Manage contexts |

### Helm Chart Management

| Subcommand | Description |
|------------|-------------|
| `helm install` | Install chart |
| `helm upgrade` | Upgrade release |
| `helm rollback` | Rollback release |
| `helm list` | List releases |
| `helm values` | Show values |
| `helm template` | Render template |
| `helm package` | Package chart |
| `helm test` | Test release |

## Supported Cloud Providers

### Hetzner Cloud

```bash
# Connect provider
nself infra provider connect hetzner --token your-api-token

# Create server
nself infra provider create-server \
  --provider hetzner \
  --type cx21 \
  --region fsn1 \
  --name staging-server

# List resources
nself infra provider list-resources hetzner
```

**Server Types**:
- cx11 (1 vCPU, 2GB RAM) - €3.79/month
- cx21 (2 vCPU, 4GB RAM) - €5.83/month
- cx31 (2 vCPU, 8GB RAM) - €10.23/month
- cx41 (4 vCPU, 16GB RAM) - €18.03/month
- cx51 (8 vCPU, 32GB RAM) - €33.63/month

### AWS (Amazon Web Services)

```bash
# Connect provider
nself infra provider connect aws \
  --access-key-id YOUR_ACCESS_KEY \
  --secret-access-key YOUR_SECRET_KEY \
  --region us-east-1

# Create EC2 instance
nself infra provider create-server \
  --provider aws \
  --type t3.medium \
  --region us-east-1 \
  --name production-server

# Create RDS database
nself infra provider create-database \
  --provider aws \
  --engine postgres \
  --instance-class db.t3.medium
```

### DigitalOcean

```bash
# Connect provider
nself infra provider connect digitalocean --token your-api-token

# Create droplet
nself infra provider create-server \
  --provider digitalocean \
  --size s-2vcpu-4gb \
  --region nyc3 \
  --name app-server
```

### Google Cloud Platform

```bash
# Connect provider
nself infra provider connect gcp \
  --project-id your-project \
  --credentials-file service-account.json

# Create Compute Engine instance
nself infra provider create-server \
  --provider gcp \
  --machine-type n1-standard-2 \
  --zone us-central1-a
```

## Kubernetes Deployment

### Initialize Kubernetes

```bash
nself infra k8s init [OPTIONS]
```

**Options**:
- `--provider PROVIDER` - Cloud provider
- `--cluster-name NAME` - Cluster name
- `--nodes N` - Number of nodes
- `--node-type TYPE` - Node instance type

**Examples**:
```bash
# Initialize with Hetzner
nself infra k8s init \
  --provider hetzner \
  --cluster-name production \
  --nodes 3 \
  --node-type cx21

# Initialize with AWS EKS
nself infra k8s init \
  --provider aws \
  --cluster-name production \
  --nodes 3 \
  --node-type t3.medium \
  --region us-east-1
```

### Deploy to Kubernetes

```bash
nself infra k8s deploy [OPTIONS]
```

**Deploys**:
- PostgreSQL (StatefulSet)
- Hasura (Deployment)
- Auth service (Deployment)
- Nginx (Ingress Controller)
- Optional services (based on .env)

**Examples**:
```bash
# Deploy to K8s
nself infra k8s deploy

# Deploy to specific namespace
nself infra k8s deploy --namespace production

# Dry run
nself infra k8s deploy --dry-run
```

**Generated Resources**:
```yaml
# deployment.yaml (excerpt)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hasura
spec:
  replicas: 3
  selector:
    matchLabels:
      app: hasura
  template:
    metadata:
      labels:
        app: hasura
    spec:
      containers:
      - name: hasura
        image: hasura/graphql-engine:v2.35.0
        ports:
        - containerPort: 8080
        env:
        - name: HASURA_GRAPHQL_DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: hasura-secrets
              key: database-url
```

### Scale Deployment

```bash
nself infra k8s scale <deployment> --replicas <n>
```

**Examples**:
```bash
# Scale Hasura to 5 replicas
nself infra k8s scale hasura --replicas 5

# Scale down
nself infra k8s scale hasura --replicas 2

# Auto-scaling
nself infra k8s autoscale hasura \
  --min 2 \
  --max 10 \
  --cpu-percent 70
```

## Helm Chart Deployment

### Install nself Helm Chart

```bash
nself infra helm install nself-production ./charts/nself \
  --values production-values.yaml \
  --namespace production \
  --create-namespace
```

**values.yaml**:
```yaml
# production-values.yaml
global:
  domain: app.example.com

postgres:
  enabled: true
  persistence:
    size: 100Gi
  resources:
    requests:
      memory: "4Gi"
      cpu: "2"

hasura:
  enabled: true
  replicas: 3
  resources:
    requests:
      memory: "1Gi"
      cpu: "1"

redis:
  enabled: true
  persistence:
    size: 10Gi

minio:
  enabled: true
  persistence:
    size: 500Gi
```

### Upgrade Helm Release

```bash
nself infra helm upgrade nself-production ./charts/nself \
  --values production-values.yaml \
  --namespace production
```

### List Helm Releases

```bash
nself infra helm list --namespace production
```

**Output**:
```
NAME                NAMESPACE    REVISION  STATUS    CHART
nself-production    production   5         deployed  nself-0.9.9
```

## Infrastructure as Code

### Generate Terraform Configuration

```bash
nself infra generate terraform \
  --provider hetzner \
  --output ./terraform
```

**Generated Files**:
```
terraform/
├── main.tf           # Main configuration
├── variables.tf      # Variables
├── outputs.tf        # Outputs
├── providers.tf      # Provider config
└── terraform.tfvars  # Variable values
```

**main.tf** (excerpt):
```hcl
resource "hcloud_server" "app_server" {
  name        = "app-server"
  server_type = "cx21"
  image       = "ubuntu-22.04"
  location    = "fsn1"

  ssh_keys = [hcloud_ssh_key.default.id]

  labels = {
    environment = "production"
    managed_by  = "nself"
  }
}

resource "hcloud_firewall" "app_firewall" {
  name = "app-firewall"

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "80"
    source_ips = ["0.0.0.0/0"]
  }

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "443"
    source_ips = ["0.0.0.0/0"]
  }
}
```

### Apply Infrastructure

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

## Cost Management

### Estimate Costs

```bash
nself infra provider estimate-cost \
  --provider hetzner \
  --resources "cx21:3,cx11:2" \
  --duration monthly
```

**Output**:
```
Cost Estimate - Hetzner Cloud

Resources:
  3x cx21 servers (2 vCPU, 4GB)     €17.49
  2x cx11 servers (1 vCPU, 2GB)     €7.58
  200GB volume storage              €10.00
  1TB bandwidth (included)          €0.00

Total Monthly: €35.07
Total Annual:  €420.84

Savings: -15% with annual commitment
Annual with commitment: €357.71
```

### Optimize Costs

```bash
nself infra optimize \
  --provider hetzner \
  --max-cost 50 \
  --recommendations
```

**Output**:
```
Cost Optimization Recommendations

Current monthly cost: €45.23

1. Reduce server types (€10.50 savings)
   • Downgrade 2x cx31 → cx21 for dev/staging

2. Use volume snapshots instead of backups (€5.00 savings)
   • Current backup cost: €8.00/month
   • Snapshot cost: €3.00/month

3. Enable auto-shutdown for dev servers (€7.20 savings)
   • Shutdown dev servers nights/weekends

Potential monthly savings: €22.70 (50%)
New monthly cost: €22.53
```

## Monitoring & Alerts

### Infrastructure Monitoring

```bash
nself infra monitor [OPTIONS]
```

**Monitors**:
- Server CPU/Memory usage
- Disk I/O
- Network traffic
- K8s cluster health
- Pod resource usage

### Infrastructure Alerts

```bash
nself infra alerts configure [OPTIONS]
```

**Alert Types**:
- High CPU usage (>80%)
- Low disk space (<10%)
- Pod crashes
- Node failures
- Cost thresholds

## Best Practices

### 1. Use Infrastructure as Code

```bash
# Generate Terraform
nself infra generate terraform --provider hetzner

# Version control
git add terraform/
git commit -m "feat: infrastructure as code"
```

### 2. Enable Auto-Scaling (K8s)

```bash
# Horizontal Pod Autoscaler
nself infra k8s autoscale hasura \
  --min 2 \
  --max 10 \
  --cpu-percent 70

# Cluster Autoscaler
nself infra k8s enable-autoscaler \
  --min-nodes 2 \
  --max-nodes 10
```

### 3. Regular Cost Reviews

```bash
# Monthly cost review
nself infra provider estimate-cost --actual
nself infra optimize --recommendations
```

### 4. Backup Critical Resources

```bash
# Backup K8s configs
nself infra k8s backup --all

# Backup persistent volumes
nself infra k8s backup-volumes
```

## Related Commands

- `nself deploy` - Deploy application
- `nself config` - Manage configuration
- `nself monitor` - Monitoring dashboards

## See Also

- [Infrastructure Guide](../../guides/INFRASTRUCTURE.md)
- [Kubernetes Deployment](../../guides/KUBERNETES.md)
- [Helm Charts](../../guides/HELM.md)
- [Cloud Providers](../../guides/CLOUD-PROVIDERS.md)
