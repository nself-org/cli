# Infrastructure Commands (infra)

**Command Group:** `nself infra`
**Subcommands:** 51
**Categories:** Cloud Providers, Kubernetes, Helm, Infrastructure Reset

---

## Overview

The `nself infra` command group manages infrastructure deployment, configuration, and orchestration across multiple cloud providers and Kubernetes environments.

**Key Features:**
- Multi-cloud provider support (10 providers)
- Kubernetes cluster management
- Helm chart deployment
- Infrastructure as code
- Automated provisioning
- Cross-provider migrations

---

## Command Structure

```
nself infra
├── provider (10 subcommands) - Cloud provider management
├── k8s (15 subcommands)      - Kubernetes operations
├── helm (13 subcommands)     - Helm chart management
└── reset ops (3 subcommands) - Infrastructure destruction and reset
    ├── destroy               - Safe infrastructure destruction
    ├── reset                 - Reset to clean state
    └── clean                 - Clean old Docker resources
```

---

## Cloud Provider Commands

### `nself infra provider`

Manage cloud provider configurations and deployments.

**Supported Providers:**
1. AWS (Amazon Web Services)
2. GCP (Google Cloud Platform)
3. Azure (Microsoft Azure)
4. DigitalOcean
5. Linode
6. Vultr
7. Hetzner
8. OVH
9. Scaleway
10. Generic (any provider)

---

### Provider Subcommands

#### `nself infra provider list`

List all configured cloud providers.

**Usage:**
```bash
nself infra provider list [options]
```

**Options:**
- `--format <json|table|yaml>` - Output format (default: table)
- `--active-only` - Show only active providers
- `--verify` - Verify credentials for each provider

**Example:**
```bash
nself infra provider list

Configured Providers:
┌─────────────┬────────────┬────────────┬──────────┐
│ Provider    │ Status     │ Regions    │ Projects │
├─────────────┼────────────┼────────────┼──────────┤
│ AWS         │ Active     │ us-east-1  │ 3        │
│ GCP         │ Active     │ us-central │ 1        │
│ DigitalOcean│ Inactive   │ nyc3       │ 0        │
└─────────────┴────────────┴────────────┴──────────┘
```

---

#### `nself infra provider add <name>`

Add a new cloud provider configuration.

**Usage:**
```bash
nself infra provider add <provider-name> [options]
```

**Arguments:**
- `<provider-name>` - Provider: aws, gcp, azure, do, linode, vultr, hetzner, ovh, scaleway, generic

**Options:**
- `--credentials <file>` - Credentials file path
- `--region <region>` - Default region
- `--project <id>` - Project/account ID
- `--interactive` - Interactive configuration wizard

**Example - AWS:**
```bash
# Interactive mode
nself infra provider add aws --interactive

# With credentials file
nself infra provider add aws \
  --credentials ~/.aws/credentials \
  --region us-east-1 \
  --project myapp-prod
```

**Example - GCP:**
```bash
# Service account key
nself infra provider add gcp \
  --credentials ~/myapp-gcp-key.json \
  --region us-central1 \
  --project myapp-123456
```

**Example - DigitalOcean:**
```bash
# API token via environment
export DIGITALOCEAN_TOKEN="dop_v1_..."
nself infra provider add do \
  --region nyc3

# Or with explicit token
nself infra provider add do \
  --credentials token.txt \
  --region sfo3
```

---

#### `nself infra provider configure <name>`

Configure an existing provider.

**Usage:**
```bash
nself infra provider configure <provider-name> [options]
```

**Options:**
- `--region <region>` - Change default region
- `--credentials <file>` - Update credentials
- `--set <key=value>` - Set configuration value
- `--interactive` - Interactive editor

**Example:**
```bash
# Update region
nself infra provider configure aws --region eu-west-1

# Update multiple settings
nself infra provider configure gcp \
  --set project=new-project \
  --set zone=us-east1-b
```

---

#### `nself infra provider remove <name>`

Remove a cloud provider configuration.

**Usage:**
```bash
nself infra provider remove <provider-name> [options]
```

**Options:**
- `--force` - Skip confirmation
- `--delete-resources` - Also delete cloud resources (dangerous!)

**Example:**
```bash
# With confirmation
nself infra provider remove aws

# Force remove
nself infra provider remove azure --force
```

**Warning:** This only removes the configuration from nself. Cloud resources are NOT deleted unless `--delete-resources` is specified.

---

#### `nself infra provider verify <name>`

Verify provider credentials and connectivity.

**Usage:**
```bash
nself infra provider verify <provider-name>
```

**Example:**
```bash
nself infra provider verify aws

Verifying AWS provider...
  ✓ Credentials valid
  ✓ IAM permissions sufficient
  ✓ Region accessible (us-east-1)
  ✓ API connectivity
  ✓ Quota limits: OK

All checks passed - provider is ready
```

---

#### `nself infra provider deploy <name>`

Deploy nself to a cloud provider.

**Usage:**
```bash
nself infra provider deploy <provider-name> [options]
```

**Options:**
- `--instance-type <type>` - VM instance type
- `--disk-size <gb>` - Root disk size (GB)
- `--ssh-key <path>` - SSH public key for access
- `--network <id>` - VPC/network ID
- `--firewall <rules>` - Firewall rules file
- `--auto-dns` - Automatically configure DNS
- `--monitoring` - Enable cloud provider monitoring

**Example - AWS:**
```bash
nself infra provider deploy aws \
  --instance-type t3.large \
  --disk-size 100 \
  --ssh-key ~/.ssh/id_rsa.pub \
  --auto-dns
```

**Example - DigitalOcean:**
```bash
nself infra provider deploy do \
  --instance-type s-2vcpu-4gb \
  --disk-size 50 \
  --ssh-key ~/.ssh/id_rsa.pub \
  --monitoring
```

---

#### `nself infra provider status <name>`

Check deployment status on a provider.

**Usage:**
```bash
nself infra provider status <provider-name>
```

**Example:**
```bash
nself infra provider status aws

Provider: AWS
Status: Deployed
Region: us-east-1

Resources:
  ✓ VPC: vpc-abc123
  ✓ Subnet: subnet-def456
  ✓ Instance: i-789xyz (running)
  ✓ Load Balancer: alb-nself (active)
  ✓ RDS: nself-db (available)

Services: 24/24 healthy
Uptime: 45 days 3 hours
```

---

#### `nself infra provider migrate`

Migrate from one provider to another.

**Usage:**
```bash
nself infra provider migrate --from <source> --to <target> [options]
```

**Options:**
- `--from <provider>` - Source provider
- `--to <provider>` - Target provider
- `--strategy <blue-green|rolling>` - Migration strategy
- `--data-transfer` - Copy all data
- `--dns-update` - Update DNS automatically
- `--verify-first` - Dry-run verification
- `--rollback-on-error` - Auto-rollback if migration fails

**Example:**
```bash
# Migrate from AWS to GCP with blue-green strategy
nself infra provider migrate \
  --from aws \
  --to gcp \
  --strategy blue-green \
  --data-transfer \
  --dns-update \
  --verify-first
```

**Migration Process:**
1. Provision new infrastructure on target provider
2. Replicate database to target
3. Sync files to target storage
4. Deploy application to target
5. Run health checks
6. Switch DNS to target
7. Monitor for issues
8. Decommission source (manual step)

---

#### `nself infra provider costs <name>`

Show estimated or actual costs for a provider.

**Usage:**
```bash
nself infra provider costs <provider-name> [options]
```

**Options:**
- `--period <month|week|day>` - Time period
- `--breakdown` - Show per-service breakdown
- `--forecast` - Show cost forecast

**Example:**
```bash
nself infra provider costs aws --period month --breakdown

AWS Costs - January 2026
Total: $245.67

Breakdown:
  EC2 Instances:      $120.00 (49%)
  RDS Database:       $80.00  (33%)
  Load Balancer:      $25.00  (10%)
  Data Transfer:      $15.67  (6%)
  S3 Storage:         $5.00   (2%)

Forecast (Feb 2026): $252.00 (+2.6%)
```

---

#### `nself infra provider scale <name>`

Scale infrastructure on a provider.

**Usage:**
```bash
nself infra provider scale <provider-name> [options]
```

**Options:**
- `--instances <count>` - Number of instances
- `--instance-type <type>` - Change instance type
- `--auto-scale` - Enable auto-scaling
- `--min <count>` - Min instances (auto-scale)
- `--max <count>` - Max instances (auto-scale)
- `--target-cpu <percent>` - CPU threshold for scaling

**Example:**
```bash
# Manual scale to 3 instances
nself infra provider scale aws --instances 3

# Enable auto-scaling
nself infra provider scale aws \
  --auto-scale \
  --min 2 \
  --max 10 \
  --target-cpu 70
```

---

## Kubernetes Commands

### `nself infra k8s`

Manage Kubernetes cluster deployments.

**Supported Kubernetes:**
- Self-managed Kubernetes (kubeadm, k3s, etc.)
- AWS EKS (Elastic Kubernetes Service)
- GCP GKE (Google Kubernetes Engine)
- Azure AKS (Azure Kubernetes Service)
- DigitalOcean DOKS
- Linode LKE
- Kind (local development)
- Minikube (local development)

---

### Kubernetes Subcommands

#### `nself infra k8s deploy`

Deploy nself to a Kubernetes cluster.

**Usage:**
```bash
nself infra k8s deploy [options]
```

**Options:**
- `--context <name>` - Kubernetes context to use
- `--namespace <name>` - Target namespace (default: nself)
- `--create-namespace` - Create namespace if missing
- `--ingress <class>` - Ingress controller (nginx, traefik, etc.)
- `--storage-class <class>` - Storage class for PVCs
- `--replicas <count>` - Number of replicas per service
- `--values <file>` - Custom values file

**Example:**
```bash
# Deploy to current context
nself infra k8s deploy

# Deploy to specific context and namespace
nself infra k8s deploy \
  --context prod-cluster \
  --namespace nself-prod \
  --create-namespace \
  --ingress nginx \
  --replicas 3
```

**What Gets Deployed:**
- PostgreSQL StatefulSet with persistent storage
- Hasura Deployment
- Auth Deployment
- Optional services (Redis, MinIO, etc.)
- Nginx Ingress
- Services and ConfigMaps
- Secrets

---

#### `nself infra k8s status`

Check Kubernetes deployment status.

**Usage:**
```bash
nself infra k8s status [options]
```

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--watch` - Watch status updates

**Example:**
```bash
nself infra k8s status

Namespace: nself
Context: prod-cluster

Deployments:
  ✓ postgres (1/1 ready)
  ✓ hasura (3/3 ready)
  ✓ auth (3/3 ready)
  ✓ nginx (2/2 ready)

Services:
  ✓ postgres (ClusterIP)
  ✓ hasura (ClusterIP)
  ✓ auth (ClusterIP)
  ✓ nginx (LoadBalancer) - 203.0.113.45

Ingress:
  ✓ nself-ingress - myapp.com

Storage:
  ✓ postgres-data (50Gi used / 100Gi)

All services healthy
```

---

#### `nself infra k8s logs <service>`

View logs from Kubernetes pods.

**Usage:**
```bash
nself infra k8s logs <service> [options]
```

**Arguments:**
- `<service>` - Service name (postgres, hasura, auth, etc.)

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--follow` - Follow log output
- `--tail <lines>` - Number of lines to show
- `--all-pods` - Show logs from all pods
- `--since <time>` - Show logs since time

**Example:**
```bash
# View hasura logs
nself infra k8s logs hasura --tail 100

# Follow all auth pods
nself infra k8s logs auth --follow --all-pods

# Logs from last hour
nself infra k8s logs postgres --since 1h
```

---

#### `nself infra k8s scale <service>`

Scale a Kubernetes deployment.

**Usage:**
```bash
nself infra k8s scale <service> --replicas <count> [options]
```

**Arguments:**
- `<service>` - Service to scale

**Options:**
- `--replicas <count>` - Number of replicas
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--wait` - Wait for scaling to complete

**Example:**
```bash
# Scale hasura to 5 replicas
nself infra k8s scale hasura --replicas 5

# Scale with wait
nself infra k8s scale auth --replicas 3 --wait
```

---

#### `nself infra k8s exec <service>`

Execute command in a Kubernetes pod.

**Usage:**
```bash
nself infra k8s exec <service> -- <command> [options]
```

**Arguments:**
- `<service>` - Service name
- `<command>` - Command to execute

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--pod <name>` - Specific pod name
- `--container <name>` - Specific container

**Example:**
```bash
# Database shell
nself infra k8s exec postgres -- psql -U postgres

# Check auth version
nself infra k8s exec auth -- nhost-auth --version

# Shell access
nself infra k8s exec hasura -- /bin/sh
```

---

#### `nself infra k8s update`

Update Kubernetes deployment to latest version.

**Usage:**
```bash
nself infra k8s update [options]
```

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--strategy <rolling|recreate>` - Update strategy
- `--image-tag <tag>` - Specific image tag
- `--wait` - Wait for rollout to complete

**Example:**
```bash
# Rolling update
nself infra k8s update --strategy rolling --wait

# Update to specific version
nself infra k8s update --image-tag v0.9.8
```

---

#### `nself infra k8s rollback`

Rollback to previous Kubernetes deployment.

**Usage:**
```bash
nself infra k8s rollback <service> [options]
```

**Arguments:**
- `<service>` - Service to rollback

**Options:**
- `--to-revision <number>` - Specific revision number
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--wait` - Wait for rollback to complete

**Example:**
```bash
# Rollback hasura to previous version
nself infra k8s rollback hasura --wait

# Rollback to specific revision
nself infra k8s rollback auth --to-revision 3
```

---

#### `nself infra k8s config`

Configure Kubernetes deployment settings.

**Usage:**
```bash
nself infra k8s config [options]
```

**Options:**
- `--set <key=value>` - Set configuration value
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--apply` - Apply changes immediately

**Example:**
```bash
# Set environment variable
nself infra k8s config \
  --set HASURA_GRAPHQL_ENABLE_CONSOLE=true \
  --apply

# Set resource limits
nself infra k8s config \
  --set postgres.resources.limits.memory=4Gi \
  --set postgres.resources.limits.cpu=2 \
  --apply
```

---

#### `nself infra k8s backup`

Backup Kubernetes-deployed nself.

**Usage:**
```bash
nself infra k8s backup [options]
```

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--output <path>` - Backup output path
- `--include-pvcs` - Backup persistent volumes
- `--compress` - Compress backup

**Example:**
```bash
# Full backup with PVCs
nself infra k8s backup \
  --include-pvcs \
  --compress \
  --output backups/k8s-backup-$(date +%Y%m%d).tar.gz
```

---

#### `nself infra k8s restore`

Restore Kubernetes deployment from backup.

**Usage:**
```bash
nself infra k8s restore <backup-file> [options]
```

**Arguments:**
- `<backup-file>` - Path to backup file

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--force` - Overwrite existing deployment

**Example:**
```bash
nself infra k8s restore backups/k8s-backup-20260201.tar.gz --force
```

---

#### `nself infra k8s delete`

Delete nself from Kubernetes cluster.

**Usage:**
```bash
nself infra k8s delete [options]
```

**Options:**
- `--context <name>` - Kubernetes context
- `--namespace <name>` - Namespace
- `--delete-namespace` - Also delete namespace
- `--delete-pvcs` - Also delete persistent volumes
- `--force` - Skip confirmation

**Example:**
```bash
# Delete with confirmation
nself infra k8s delete

# Delete everything including PVCs
nself infra k8s delete \
  --delete-namespace \
  --delete-pvcs \
  --force
```

---

#### Additional Kubernetes Commands

**`nself infra k8s context`** - Manage kubectl contexts

**`nself infra k8s ingress`** - Configure ingress settings

**`nself infra k8s secrets`** - Manage Kubernetes secrets

**`nself infra k8s autoscale`** - Configure HPA (Horizontal Pod Autoscaler)

---

## Helm Commands

### `nself infra helm`

Manage nself deployment via Helm charts.

**Features:**
- Official nself Helm chart
- Customizable values
- Easy upgrades and rollbacks
- Multi-environment support

---

### Helm Subcommands

#### `nself infra helm install`

Install nself via Helm chart.

**Usage:**
```bash
nself infra helm install <release-name> [options]
```

**Arguments:**
- `<release-name>` - Helm release name

**Options:**
- `--namespace <name>` - Kubernetes namespace
- `--create-namespace` - Create namespace if missing
- `--values <file>` - Custom values file
- `--set <key=value>` - Set individual values
- `--version <version>` - Chart version
- `--wait` - Wait for resources to be ready

**Example:**
```bash
# Basic installation
nself infra helm install nself-prod \
  --namespace production \
  --create-namespace

# With custom values
nself infra helm install nself-prod \
  --namespace production \
  --values custom-values.yaml \
  --wait

# Set specific values
nself infra helm install nself-prod \
  --set postgres.persistence.size=100Gi \
  --set hasura.replicaCount=3
```

**Example custom-values.yaml:**
```yaml
# custom-values.yaml
postgres:
  persistence:
    enabled: true
    size: 100Gi
    storageClass: fast-ssd
  resources:
    limits:
      memory: 4Gi
      cpu: 2

hasura:
  replicaCount: 3
  resources:
    limits:
      memory: 2Gi
      cpu: 1

auth:
  replicaCount: 2

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: api.myapp.com
      paths:
        - path: /
          pathType: Prefix

monitoring:
  enabled: true
  prometheus:
    enabled: true
  grafana:
    enabled: true
```

---

#### `nself infra helm upgrade`

Upgrade existing Helm release.

**Usage:**
```bash
nself infra helm upgrade <release-name> [options]
```

**Arguments:**
- `<release-name>` - Helm release name

**Options:**
- `--namespace <name>` - Namespace
- `--values <file>` - Updated values file
- `--set <key=value>` - Update values
- `--version <version>` - Chart version
- `--wait` - Wait for upgrade
- `--reuse-values` - Reuse existing values
- `--reset-values` - Reset to chart defaults

**Example:**
```bash
# Upgrade with new values
nself infra helm upgrade nself-prod \
  --values updated-values.yaml \
  --wait

# Upgrade to specific version
nself infra helm upgrade nself-prod \
  --version 0.9.8 \
  --reuse-values
```

---

#### `nself infra helm rollback`

Rollback Helm release to previous version.

**Usage:**
```bash
nself infra helm rollback <release-name> [revision] [options]
```

**Arguments:**
- `<release-name>` - Helm release name
- `[revision]` - Specific revision (optional)

**Options:**
- `--namespace <name>` - Namespace
- `--wait` - Wait for rollback

**Example:**
```bash
# Rollback to previous revision
nself infra helm rollback nself-prod --wait

# Rollback to specific revision
nself infra helm rollback nself-prod 3
```

---

#### `nself infra helm status`

Check Helm release status.

**Usage:**
```bash
nself infra helm status <release-name> [options]
```

**Arguments:**
- `<release-name>` - Helm release name

**Options:**
- `--namespace <name>` - Namespace

**Example:**
```bash
nself infra helm status nself-prod

NAME: nself-prod
LAST DEPLOYED: 2026-02-01 12:00:00
NAMESPACE: production
STATUS: deployed
REVISION: 5

RESOURCES:
  StatefulSet/postgres
  Deployment/hasura
  Deployment/auth
  Service/postgres
  Service/hasura
  Service/auth
  Ingress/nself-ingress

NOTES:
nself has been deployed successfully!

Access your services:
  GraphQL API: https://api.myapp.com
  Auth:        https://auth.myapp.com
```

---

#### `nself infra helm list`

List all Helm releases.

**Usage:**
```bash
nself infra helm list [options]
```

**Options:**
- `--all-namespaces` - List from all namespaces
- `--namespace <name>` - Specific namespace
- `--deployed` - Show only deployed releases
- `--failed` - Show only failed releases

**Example:**
```bash
nself infra helm list --all-namespaces

NAME        NAMESPACE   STATUS      REVISION  CHART VERSION
nself-prod  production  deployed    5         0.9.8
nself-dev   development deployed    2         0.9.7
```

---

#### `nself infra helm delete`

Delete Helm release.

**Usage:**
```bash
nself infra helm delete <release-name> [options]
```

**Arguments:**
- `<release-name>` - Helm release name

**Options:**
- `--namespace <name>` - Namespace
- `--purge` - Also remove history
- `--keep-history` - Keep release history

**Example:**
```bash
# Delete release
nself infra helm delete nself-dev

# Delete and purge history
nself infra helm delete nself-dev --purge
```

---

#### `nself infra helm template`

Render Helm templates locally.

**Usage:**
```bash
nself infra helm template <release-name> [options]
```

**Arguments:**
- `<release-name>` - Release name (for template)

**Options:**
- `--values <file>` - Values file
- `--set <key=value>` - Set values
- `--output-dir <dir>` - Output directory

**Example:**
```bash
# Render templates to stdout
nself infra helm template nself-prod \
  --values values.yaml

# Save to directory
nself infra helm template nself-prod \
  --values values.yaml \
  --output-dir rendered/
```

---

#### `nself infra helm values`

Show values for Helm release.

**Usage:**
```bash
nself infra helm values <release-name> [options]
```

**Arguments:**
- `<release-name>` - Helm release name

**Options:**
- `--namespace <name>` - Namespace
- `--all` - Show all values (including defaults)

**Example:**
```bash
# Show user values
nself infra helm values nself-prod

# Show all values
nself infra helm values nself-prod --all
```

---

#### Additional Helm Commands

**`nself infra helm history`** - View release history

**`nself infra helm get`** - Get release information

**`nself infra helm test`** - Run Helm tests

**`nself infra helm lint`** - Lint chart

**`nself infra helm package`** - Package chart

---

## Common Workflows

### Workflow 1: Deploy to AWS

```bash
# 1. Add AWS provider
nself infra provider add aws --interactive

# 2. Deploy infrastructure
nself infra provider deploy aws \
  --instance-type t3.xlarge \
  --disk-size 100 \
  --ssh-key ~/.ssh/id_rsa.pub

# 3. Check status
nself infra provider status aws

# 4. View costs
nself infra provider costs aws --breakdown
```

---

### Workflow 2: Deploy to Kubernetes

```bash
# 1. Deploy to K8s
nself infra k8s deploy \
  --context prod \
  --namespace nself \
  --create-namespace \
  --replicas 3

# 2. Check status
nself infra k8s status

# 3. View logs
nself infra k8s logs hasura --follow

# 4. Scale if needed
nself infra k8s scale hasura --replicas 5
```

---

### Workflow 3: Deploy via Helm

```bash
# 1. Install with Helm
nself infra helm install nself-prod \
  --namespace production \
  --create-namespace \
  --values production-values.yaml \
  --wait

# 2. Check status
nself infra helm status nself-prod

# 3. Upgrade when needed
nself infra helm upgrade nself-prod \
  --values updated-values.yaml \
  --wait

# 4. Rollback if issues
nself infra helm rollback nself-prod
```

---

### Workflow 4: Multi-Cloud Migration

```bash
# 1. Set up target provider
nself infra provider add gcp --interactive

# 2. Verify migration feasibility
nself infra provider migrate \
  --from aws \
  --to gcp \
  --verify-first

# 3. Execute migration
nself infra provider migrate \
  --from aws \
  --to gcp \
  --strategy blue-green \
  --data-transfer \
  --dns-update

# 4. Monitor new deployment
nself infra provider status gcp

# 5. Verify services
nself health

# 6. Decommission old provider (manual)
nself infra provider remove aws
```

---

## Best Practices

### Cloud Provider Management

1. **Always verify credentials** before deployment
2. **Use separate providers** for staging and production
3. **Monitor costs** regularly
4. **Implement auto-scaling** for production
5. **Set up alerts** for resource limits

### Kubernetes Deployments

1. **Use namespaces** to separate environments
2. **Set resource limits** for all pods
3. **Enable auto-scaling** (HPA) for stateless services
4. **Use persistent volumes** for stateful services
5. **Configure ingress** with SSL/TLS
6. **Implement network policies** for security

### Helm Charts

1. **Use version pinning** for production
2. **Maintain separate values files** per environment
3. **Test upgrades** in staging first
4. **Keep release history** for rollbacks
5. **Document custom values** in version control

---

## Troubleshooting

### Provider Issues

**Problem:** Credentials not working
```bash
# Verify credentials
nself infra provider verify aws

# Re-configure if needed
nself infra provider configure aws --interactive
```

**Problem:** Deployment stuck
```bash
# Check provider status
nself infra provider status aws

# View deployment logs
nself logs deploy
```

### Kubernetes Issues

**Problem:** Pods not starting
```bash
# Check pod status
nself infra k8s status

# View pod logs
nself infra k8s logs <service>

# Describe pod for events
kubectl describe pod -n nself <pod-name>
```

**Problem:** Storage issues
```bash
# Check PVC status
kubectl get pvc -n nself

# Verify storage class
kubectl get storageclass
```

### Helm Issues

**Problem:** Release failed
```bash
# View release status
nself infra helm status nself-prod

# View history
nself infra helm history nself-prod

# Rollback
nself infra helm rollback nself-prod
```

---

## Command Consolidation

The `infra` command consolidates previously separate commands:

- `nself provider` → `nself infra provider`
- `nself k8s` → `nself infra k8s`
- `nself helm` → `nself infra helm`
- `nself cloud` → `nself infra provider`

See [Command Tree](./COMMAND-TREE-V1.md) for full command structure.

---

## See Also

- [Deploy Commands](DEPLOY.md) - Deployment workflows
- [Config Commands](CONFIG.md) - Configuration management
- [Service Commands](SERVICE.md) - Service management
- [Kubernetes Guide](../infrastructure/K8S-IMPLEMENTATION-GUIDE.md)
- [Helm Guide](HELM.md)
- [Multi-Cloud Guide](../deployment/CLOUD-PROVIDERS.md)

---

**Last Updated:** February 1, 2026
**Version:** 0.9.8
