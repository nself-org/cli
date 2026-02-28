# nself helm - Helm Chart Management

> **⚠️ DEPRECATED**: `nself helm` is deprecated and will be removed in v1.0.0.
> Please use `nself infra helm` instead.
> Run `nself infra helm --help` for full usage information.

**Version**: 0.4.7+ | **Status**: Available

---

## Overview

The `nself helm` command manages Helm charts for deploying nself to Kubernetes. It can generate charts from your Docker Compose configuration, package them, and deploy to any Helm-compatible cluster.

---

## Subcommands

### Initialization

```bash
nself helm init                        # Initialize Helm chart
nself helm init --from-compose         # Generate from docker-compose.yml
```

### Chart Generation

```bash
nself helm generate                    # Generate/update chart
nself helm generate --output ./charts  # Custom output directory
```

### Installation

```bash
nself helm install                     # Install to cluster
nself helm install --env staging       # Install with environment values
nself helm upgrade                     # Upgrade existing release
nself helm rollback                    # Rollback to previous version
nself helm uninstall                   # Remove release
```

### Management

```bash
nself helm list                        # List installed releases
nself helm status                      # Show release status
nself helm values                      # Show/edit values
nself helm template                    # Render templates locally
nself helm package                     # Package chart for distribution
```

### Repository

```bash
nself helm repo add <name> <url>       # Add chart repository
nself helm repo remove <name>          # Remove repository
nself helm repo update                 # Update repository cache
nself helm repo list                   # List repositories
```

---

## Chart Generation

### From Docker Compose

```bash
$ nself helm init --from-compose
Generating Helm chart from docker-compose.yml...

Created chart structure:
  charts/nself/
  ├── Chart.yaml
  ├── values.yaml
  ├── templates/
  │   ├── _helpers.tpl
  │   ├── namespace.yaml
  │   ├── postgres-deployment.yaml
  │   ├── postgres-service.yaml
  │   ├── hasura-deployment.yaml
  │   └── ...
  └── values/
      ├── staging.yaml
      └── production.yaml

Chart generated successfully!
```

### Chart Structure

```
charts/nself/
├── Chart.yaml           # Chart metadata
├── values.yaml          # Default values
├── .helmignore          # Ignore patterns
├── templates/
│   ├── _helpers.tpl     # Template helpers
│   ├── NOTES.txt        # Post-install notes
│   ├── namespace.yaml   # Namespace
│   ├── configmap.yaml   # ConfigMaps
│   ├── secret.yaml      # Secrets
│   ├── deployment.yaml  # Deployments
│   ├── service.yaml     # Services
│   ├── pvc.yaml         # Persistent Volume Claims
│   ├── ingress.yaml     # Ingress
│   └── hpa.yaml         # Horizontal Pod Autoscaler
└── values/
    ├── local.yaml       # Local development
    ├── staging.yaml     # Staging overrides
    └── production.yaml  # Production overrides
```

---

## Values Configuration

### Default Values (values.yaml)

```yaml
# Global settings
global:
  environment: production
  namespace: nself

# Image settings
image:
  pullPolicy: IfNotPresent

# PostgreSQL
postgres:
  enabled: true
  replicas: 1
  image: postgres:16-alpine
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
  persistence:
    enabled: true
    size: 10Gi
    storageClass: standard

# Hasura
hasura:
  enabled: true
  replicas: 2
  image: hasura/graphql-engine:v2.43.0
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"

# Auth
auth:
  enabled: true
  replicas: 2

# Ingress
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  hosts:
    - host: api.example.com
      paths:
        - path: /
          service: hasura
          port: 8080
```

### Environment Overrides

`values/production.yaml`:

```yaml
# Production-specific values
global:
  environment: production

postgres:
  replicas: 1
  resources:
    limits:
      cpu: "2000m"
      memory: "2Gi"
  persistence:
    size: 100Gi
    storageClass: premium-ssd

hasura:
  replicas: 3
  resources:
    limits:
      cpu: "2000m"
      memory: "2Gi"

ingress:
  hosts:
    - host: api.myapp.com
      paths:
        - path: /
          service: hasura
          port: 8080
```

---

## Installation

### Basic Install

```bash
$ nself helm install
Installing nself chart...
NAME: nself
NAMESPACE: nself
STATUS: deployed
REVISION: 1

NOTES:
Thank you for installing nself!

Your application is available at:
  https://api.example.com

To check the status:
  nself helm status
```

### With Environment Values

```bash
# Install with staging values
nself helm install --env staging

# Install with production values
nself helm install --env production

# Install with custom values file
nself helm install -f my-values.yaml
```

### Install Options

```bash
nself helm install \
  --name myapp \                    # Release name
  --namespace myapp \               # K8s namespace
  --env production \                # Values environment
  --set replicas=3 \                # Override specific value
  --wait \                          # Wait for pods ready
  --timeout 10m                     # Timeout
```

---

## Upgrade & Rollback

### Upgrade Release

```bash
# Upgrade with latest chart
nself helm upgrade

# Upgrade with new values
nself helm upgrade --set hasura.replicas=5

# Upgrade to new environment
nself helm upgrade --env production
```

### Rollback

```bash
# Rollback to previous version
nself helm rollback

# Rollback to specific revision
nself helm rollback --revision 3

# View release history
nself helm history
```

### Release History

```bash
$ nself helm history
REVISION  UPDATED                   STATUS      DESCRIPTION
1         2026-01-20 10:30:00       superseded  Install complete
2         2026-01-21 14:15:00       superseded  Upgrade replicas
3         2026-01-22 09:00:00       deployed    Upgrade to v0.4.7
```

---

## Status & Debugging

### Release Status

```bash
$ nself helm status
NAME: nself
NAMESPACE: nself
STATUS: deployed
REVISION: 3

RESOURCES:
==> v1/Deployment
NAME       READY   UP-TO-DATE   AVAILABLE
postgres   1/1     1            1
hasura     3/3     3            3
auth       2/2     2            2

==> v1/Service
NAME       TYPE        CLUSTER-IP      PORT(S)
postgres   ClusterIP   10.96.100.1     5432/TCP
hasura     ClusterIP   10.96.100.2     8080/TCP

==> v1/Ingress
NAME    HOSTS              ADDRESS
nself   api.example.com    203.0.113.1
```

### Template Preview

Render templates without installing:

```bash
# Preview all templates
nself helm template

# Preview specific template
nself helm template --show-only templates/deployment.yaml

# Preview with values
nself helm template --env production
```

---

## Repository Management

### Add Repositories

```bash
# Add official Helm repos
nself helm repo add bitnami https://charts.bitnami.com/bitnami
nself helm repo add grafana https://grafana.github.io/helm-charts

# Update repo index
nself helm repo update
```

### Package and Publish

```bash
# Package chart
nself helm package
# Creates: nself-0.4.7.tgz

# Push to OCI registry
helm push nself-0.4.7.tgz oci://registry.example.com/charts

# Push to ChartMuseum
curl --data-binary "@nself-0.4.7.tgz" https://chartmuseum.example.com/api/charts
```

---

## Integration with CI/CD

### GitHub Actions Example

```yaml
- name: Deploy to Kubernetes
  run: |
    # Configure kubectl
    echo "${{ secrets.KUBECONFIG }}" | base64 -d > kubeconfig
    export KUBECONFIG=kubeconfig

    # Deploy with Helm
    nself helm upgrade \
      --env production \
      --set image.tag=${{ github.sha }} \
      --wait
```

### GitLab CI Example

```yaml
deploy:
  stage: deploy
  script:
    - nself helm upgrade --env $CI_ENVIRONMENT_NAME
  environment:
    name: production
```

---

## Best Practices

### Secrets Management

Don't store secrets in values.yaml. Use:

```bash
# External secrets operator
nself helm install --set externalSecrets.enabled=true

# Sealed secrets
kubeseal < secret.yaml > sealed-secret.yaml

# Helm secrets plugin
helm secrets upgrade nself ./charts/nself -f secrets.yaml
```

### Resource Limits

Always set resource limits:

```yaml
resources:
  requests:
    cpu: "100m"
    memory: "128Mi"
  limits:
    cpu: "500m"
    memory: "512Mi"
```

### Health Checks

Configure probes:

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

---

## Examples

### Development Workflow

```bash
# Initialize chart
nself helm init --from-compose

# Test locally
nself helm template

# Install to dev cluster
nself helm install --env local

# Make changes, upgrade
nself helm upgrade
```

### Production Deployment

```bash
# Package for distribution
nself helm package

# Deploy to production
nself helm install \
  --env production \
  --wait \
  --timeout 10m

# Monitor rollout
nself helm status
```

---

*See also: [K8S](K8S.md) | [CLOUD](CLOUD.md) | [DEPLOY](DEPLOY.md)*
