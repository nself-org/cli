# nself k8s - Kubernetes Management

> **⚠️ DEPRECATED in v0.9.6**: This command has been consolidated.
> Please use `nself infra k8s` instead.
> See [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md) and [v0.9.6 Release Notes](../releases/v0.9.6.md) for details.

**Version**: 0.4.7+ | **Status**: Available

---

## Overview

The `nself k8s` command provides complete Kubernetes management, including converting your Docker Compose setup to K8s manifests, deploying to clusters, and managing workloads.

---

## Subcommands

### Initialization

```bash
nself k8s init                         # Initialize K8s configuration
nself k8s init --context <context>     # Use specific kubectl context
```

### Conversion

```bash
nself k8s convert                      # Convert compose to K8s manifests
nself k8s convert --output ./k8s       # Custom output directory
nself k8s convert --namespace myapp    # Custom namespace
```

### Deployment

```bash
nself k8s apply                        # Apply manifests to cluster
nself k8s apply --dry-run              # Preview what would be applied
nself k8s deploy                       # Full deployment workflow
nself k8s deploy --env staging         # Deploy to specific environment
```

### Operations

```bash
nself k8s status                       # Show deployment status
nself k8s logs <service>               # View pod logs
nself k8s logs <service> -f            # Follow logs
nself k8s scale <service> <replicas>   # Scale deployment
nself k8s rollback <service>           # Rollback to previous version
nself k8s delete                       # Delete all resources
```

### Cluster Management

```bash
nself k8s cluster list                 # List available clusters
nself k8s cluster connect <name>       # Connect to cluster
nself k8s cluster info                 # Show cluster information
```

### Namespace Management

```bash
nself k8s namespace list               # List namespaces
nself k8s namespace create <name>      # Create namespace
nself k8s namespace delete <name>      # Delete namespace
nself k8s namespace switch <name>      # Switch active namespace
```

---

## Manifest Generation

### Convert Docker Compose

```bash
$ nself k8s convert
Converting docker-compose.yml to Kubernetes manifests...
  Converting service: postgres
  Converting service: hasura
  Converting service: auth
  Converting service: nginx

Generated manifests in .nself/k8s/manifests/:
  00-namespace.yaml
  01-postgres-deployment.yaml
  01-postgres-service.yaml
  01-postgres-pvc.yaml
  01-postgres-configmap.yaml
  02-hasura-deployment.yaml
  02-hasura-service.yaml
  ...
  99-ingress.yaml

Conversion complete: 4 services processed
```

### Generated Resources

For each service, nself generates:

| Resource | Description |
|----------|-------------|
| **Deployment** | Pod template with rolling updates |
| **Service** | ClusterIP for internal communication |
| **ConfigMap** | Environment variables |
| **PVC** | Persistent storage (if volumes defined) |
| **Ingress** | External access (for web services) |

### Default Configuration

```yaml
# Deployment defaults
replicas: 2
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1
    maxUnavailable: 0

# Resource limits
resources:
  requests:
    cpu: "100m"
    memory: "128Mi"
  limits:
    cpu: "500m"
    memory: "512Mi"
```

---

## Deployment Workflow

### Full Deployment

```bash
$ nself k8s deploy
Pre-deployment checks...
  ✓ kubectl configured
  ✓ Cluster accessible
  ✓ Namespace available

Converting compose to manifests... done
Applying manifests...
  ✓ Namespace created
  ✓ ConfigMaps applied
  ✓ PVCs created
  ✓ Deployments created
  ✓ Services created
  ✓ Ingress configured

Waiting for pods to be ready...
  postgres: 2/2 ready
  hasura: 2/2 ready
  auth: 2/2 ready
  nginx: 2/2 ready

Deployment complete!
```

### Dry Run

Preview changes before applying:

```bash
$ nself k8s apply --dry-run
Would apply the following changes:
  CREATE: Namespace/myapp
  CREATE: Deployment/postgres
  CREATE: Service/postgres
  CREATE: PersistentVolumeClaim/postgres-data
  ...
```

---

## Operations

### Check Status

```bash
$ nself k8s status
Kubernetes Deployment Status
============================
Namespace: myapp

DEPLOYMENT          READY   UP-TO-DATE   AVAILABLE
postgres            2/2     2            2
hasura              2/2     2            2
auth                2/2     2            2
nginx               2/2     2            2

PODS                              STATUS    RESTARTS   AGE
postgres-7b9d4f6c-abc12           Running   0          2d
postgres-7b9d4f6c-def34           Running   0          2d
hasura-8c5e7g8h-ghi56             Running   0          2d
...
```

### View Logs

```bash
# View logs for a service
nself k8s logs hasura

# Follow logs in real-time
nself k8s logs hasura -f

# Logs from specific pod
nself k8s logs hasura --pod hasura-8c5e7g8h-ghi56

# Last 100 lines
nself k8s logs hasura --tail 100
```

### Scale Deployments

```bash
# Scale to 5 replicas
nself k8s scale hasura 5

# Scale with HPA
nself k8s scale hasura --auto --min 2 --max 10 --cpu 70
```

### Rollback

```bash
# Rollback to previous version
nself k8s rollback hasura

# Rollback to specific revision
nself k8s rollback hasura --revision 3

# Check rollout history
nself k8s rollout history hasura
```

---

## Cluster Management

### List Clusters

```bash
$ nself k8s cluster list
Available Clusters:
  NAME              PROVIDER       REGION      STATUS
  prod-cluster      digitalocean   nyc1        Connected (current)
  staging-cluster   digitalocean   sfo3        Available
  local-minikube    local          -           Available
```

### Connect to Cluster

```bash
# Connect using kubeconfig
nself k8s cluster connect prod-cluster

# Connect to managed K8s
nself k8s cluster connect --provider digitalocean --cluster my-doks
```

---

## Supported Platforms

| Platform | Provider | Notes |
|----------|----------|-------|
| **EKS** | AWS | Full support |
| **GKE** | Google Cloud | Full support |
| **AKS** | Azure | Full support |
| **DOKS** | DigitalOcean | Full support |
| **LKE** | Linode | Full support |
| **VKE** | Vultr | Full support |
| **SKS** | Exoscale | Full support |
| **ACK** | Alibaba | Full support |
| **TKE** | Tencent | Full support |
| **MKS** | Yandex | Full support |
| **OKE** | Oracle | Full support |
| **IKS** | IBM | Full support |
| **k3s** | Self-hosted | Full support |
| **minikube** | Local | Development |
| **kind** | Local | Testing |

---

## Environment-Specific Values

### Using Kustomize Overlays

```bash
# Generate with environment overlay
nself k8s convert --env staging
nself k8s convert --env production

# Apply specific environment
nself k8s apply --env production
```

Generated structure:

```
.nself/k8s/
├── base/
│   ├── kustomization.yaml
│   └── manifests/
│       ├── deployment.yaml
│       ├── service.yaml
│       └── ...
└── overlays/
    ├── staging/
    │   ├── kustomization.yaml
    │   └── patches/
    └── production/
        ├── kustomization.yaml
        └── patches/
```

---

## Configuration

### K8s Configuration File

`.nself/k8s/config.yaml`:

```yaml
kubernetes:
  namespace: myapp

  defaults:
    replicas: 2
    imagePullPolicy: IfNotPresent

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"

  ingress:
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
```

---

## Examples

### Development to Production

```bash
# Local development
nself start

# Convert to K8s
nself k8s convert

# Deploy to staging
nself k8s deploy --env staging

# Verify staging
nself k8s status

# Deploy to production
nself k8s deploy --env production
```

### Troubleshooting

```bash
# Check pod issues
nself k8s status
kubectl describe pod <pod-name>

# Check events
kubectl get events --sort-by='.lastTimestamp'

# Debug specific pod
kubectl exec -it <pod-name> -- /bin/bash
```

---

*See also: [HELM](HELM.md) | [CLOUD](CLOUD.md) | [DEPLOY](DEPLOY.md)*
