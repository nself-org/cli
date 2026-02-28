# Production Deployment Examples

Complete, production-ready examples for deploying nself custom services to various environments.

---

## Overview

This directory contains battle-tested deployment configurations and workflows for custom services (CS_N) in production environments.

---

## Available Examples

### CI/CD Pipelines

| File | Description | Use Case |
|------|-------------|----------|
| [github-actions-complete.yml](github-actions-complete.yml) | Complete GitHub Actions workflow | GitHub-hosted projects |
| [gitlab-ci-complete.yml](gitlab-ci-complete.yml) | Complete GitLab CI/CD pipeline | GitLab-hosted projects |

**Features**:
- Multi-stage builds
- Parallel testing
- Security scanning (Trivy, secrets detection)
- Integration tests
- Staging deployment (automatic)
- Production deployment (manual approval)
- Health checks
- Automated rollback on failure
- Slack notifications

### Container Orchestration

| File | Description | Use Case |
|------|-------------|----------|
| [kubernetes-manifests.yml](kubernetes-manifests.yml) | Production K8s manifests | Kubernetes clusters |
| [blue-green-deployment.md](blue-green-deployment.md) | Zero-downtime deployments | High-availability production |

**Features**:
- Deployments, Services, ConfigMaps, Secrets
- HorizontalPodAutoscaler
- PodDisruptionBudget
- Resource limits and requests
- Liveness, readiness, startup probes
- Network policies
- RBAC configuration
- Blue-green deployment patterns

---

## Quick Start

### GitHub Actions

1. **Copy workflow file**:
```bash
mkdir -p .github/workflows
cp docs/deployment/examples/github-actions-complete.yml .github/workflows/deploy.yml
```

2. **Add GitHub Secrets**:
   - `STAGING_SSH_KEY` - SSH private key for staging server
   - `STAGING_HOST` - Staging server hostname
   - `STAGING_ENV` - Staging `.env` file contents
   - `STAGING_SECRETS` - Staging `.env.secrets` file contents
   - `PROD_SSH_KEY` - Production SSH private key
   - `PROD_HOST` - Production server hostname
   - `PROD_ENV` - Production `.env` file contents
   - `PROD_SECRETS` - Production `.env.secrets` file contents
   - `SLACK_WEBHOOK` - Slack webhook URL (optional)

3. **Push to main branch**:
```bash
git add .github/workflows/deploy.yml
git commit -m "Add deployment workflow"
git push origin main
```

4. **Workflow runs automatically** on push to main or changes to `services/` directory.

---

### GitLab CI

1. **Copy pipeline file**:
```bash
cp docs/deployment/examples/gitlab-ci-complete.yml .gitlab-ci.yml
```

2. **Add GitLab CI/CD Variables** (Settings → CI/CD → Variables):
   - `STAGING_SSH_KEY` - SSH private key for staging
   - `STAGING_HOST` - Staging hostname
   - `STAGING_ENV` - Staging environment file
   - `STAGING_SECRETS` - Staging secrets file
   - `PROD_SSH_KEY` - Production SSH key
   - `PROD_HOST` - Production hostname
   - `PROD_ENV` - Production environment file
   - `PROD_SECRETS` - Production secrets file
   - `SLACK_WEBHOOK_URL` - Slack webhook (optional)

3. **Push to GitLab**:
```bash
git add .gitlab-ci.yml
git commit -m "Add CI/CD pipeline"
git push origin main
```

4. **Pipeline runs automatically** on push.

---

### Kubernetes

1. **Generate manifests from your nself project**:
```bash
nself infra k8s convert --env production
```

2. **Or copy and customize example manifests**:
```bash
mkdir -p k8s/
cp docs/deployment/examples/kubernetes-manifests.yml k8s/manifests.yml

# Edit k8s/manifests.yml:
# - Replace image registry URLs
# - Update domain names
# - Set resource limits
# - Configure secrets
```

3. **Apply to cluster**:
```bash
# Create namespace
kubectl create namespace myapp

# Create secrets (do this FIRST)
kubectl create secret generic custom-services-secrets \
  --from-env-file=.environments/prod/.env.secrets \
  --namespace=myapp

# Apply manifests
kubectl apply -f k8s/manifests.yml

# Check status
kubectl get pods -n myapp
```

4. **Verify deployment**:
```bash
nself infra k8s status
```

---

### Blue-Green Deployment

1. **Read the guide**:
```bash
cat docs/deployment/examples/blue-green-deployment.md
```

2. **Setup Docker Compose files**:
```bash
# Create blue/green compose files
cp docker-compose.yml docker-compose.blue.yml
cp docker-compose.yml docker-compose.green.yml

# Edit to differentiate environments (ports, labels, etc.)
```

3. **Copy deployment scripts**:
```bash
mkdir -p src/scripts/
# Copy scripts from blue-green-deployment.md
vim src/scripts/deploy-blue-green.sh
vim src/scripts/rollback-blue-green.sh
chmod +x src/scripts/*.sh
```

4. **Run deployment**:
```bash
./src/scripts/deploy-blue-green.sh green
```

5. **Rollback if needed**:
```bash
./src/scripts/rollback-blue-green.sh
```

---

## Customization Guide

### Adapting for Your Project

All examples are templates. Customize for your specific needs:

#### 1. Service Names

Replace example service names with yours:
- `payment-api` → your service
- `notification-worker` → your service
- `analytics-api` → your service
- `ml-inference` → your service

#### 2. Ports

Update port numbers to match your `.env`:
```bash
CS_1=api:express-ts:8001       # port 8001
CS_2=worker:bullmq-js:8002     # port 8002
CS_3=grpc:grpc:50051           # port 50051
```

#### 3. Environment Variables

Add your custom environment variables:
```yaml
# In CI/CD files
env:
  CUSTOM_API_KEY: ${{ secrets.CUSTOM_API_KEY }}
  FEATURE_FLAG_ENABLED: true
```

#### 4. Resource Limits

Adjust Kubernetes resource requests/limits:
```yaml
resources:
  requests:
    cpu: "250m"      # Increase for CPU-heavy services
    memory: "256Mi"  # Increase for memory-heavy services
  limits:
    cpu: "1000m"
    memory: "512Mi"
```

#### 5. Health Check Paths

If your services use different health endpoints:
```yaml
# Change from /health to your endpoint
livenessProbe:
  httpGet:
    path: /api/health  # or /healthz, /_health, etc.
```

#### 6. Secrets

Add your own secrets:
```yaml
# In Kubernetes
stringData:
  MY_API_KEY: xxx
  MY_SECRET_TOKEN: yyy

# In CI/CD
- name: MY_API_KEY
  valueFrom:
    secretKeyRef:
      name: custom-services-secrets
      key: MY_API_KEY
```

---

## Common Workflows

### Staging → Production Flow

```bash
# 1. Merge feature to main
git checkout main
git merge feature/new-payment-provider

# 2. Automatically deploys to staging (GitHub Actions/GitLab CI)
# Wait for CI to complete

# 3. Verify staging
curl https://payment-api.staging.example.com/health

# 4. Run staging tests
npm run test:staging

# 5. Manually trigger production deploy (CI/CD)
# In GitHub: Go to Actions → Run workflow → deploy:production
# In GitLab: Go to CI/CD → Pipelines → Manual job → deploy:production:main

# 6. Monitor production
watch -n 5 'curl -s https://payment-api.example.com/health | jq'
```

### Hotfix Flow

```bash
# 1. Create hotfix branch from main
git checkout -b hotfix/critical-bug main

# 2. Fix issue
vim services/payment-api/src/bug.ts
git commit -am "Fix critical bug"

# 3. Push and create PR
git push origin hotfix/critical-bug

# PR automatically deploys to staging for verification

# 4. After approval, merge to main
# Production deployment runs automatically or manually trigger

# 5. Verify fix
curl https://payment-api.example.com/health
```

### Rollback Flow

```bash
# If deployment fails or issues detected:

# GitHub Actions:
# - Automatic rollback triggers on health check failure
# - Or manually trigger rollback job

# GitLab CI:
# - Manual job: deploy:production:rollback

# Blue-Green:
./src/scripts/rollback-blue-green.sh

# Kubernetes:
nself infra k8s rollback payment-api
```

---

## Testing Before Production

### Local Testing

```bash
# Test Docker builds locally
docker compose build

# Test health endpoints
docker compose up -d
curl http://localhost:8001/health
curl http://localhost:8003/health

# Test integration
npm run test:integration
```

### Staging Testing

```bash
# After staging deployment
nself deploy health --env staging

# Run smoke tests
./src/scripts/smoke-tests.sh staging

# Load testing
k6 run loadtest.js --env staging
```

---

## Troubleshooting

### CI/CD Pipeline Fails

**Build stage fails**:
```bash
# Check Dockerfile syntax
docker build -t test services/payment-api/

# Check dependencies
cd services/payment-api && npm install
```

**Test stage fails**:
```bash
# Run tests locally
cd services/payment-api && npm test

# Check test database connection
docker compose -f docker-compose.test.yml up -d
```

**Security scan fails**:
```bash
# Fix vulnerabilities
npm audit fix

# Update base images
# Change FROM node:16 to FROM node:18
```

**Deploy stage fails**:
```bash
# Check SSH connectivity
ssh -i ~/.ssh/deploy_key user@server

# Check secrets
echo "$PROD_SECRETS" | grep POSTGRES_PASSWORD

# Check server resources
ssh user@server "df -h && free -m"
```

### Kubernetes Deployment Issues

**Pods not starting**:
```bash
# Check pod status
kubectl get pods -n myapp

# View pod logs
kubectl logs -n myapp payment-api-xxxxx

# Describe pod for events
kubectl describe pod -n myapp payment-api-xxxxx
```

**Health checks failing**:
```bash
# Check health endpoint
kubectl exec -n myapp payment-api-xxxxx -- curl http://localhost:8001/health

# Check environment variables
kubectl exec -n myapp payment-api-xxxxx -- env | grep POSTGRES
```

**Service not accessible**:
```bash
# Check service endpoints
kubectl get endpoints -n myapp payment-api

# Check ingress
kubectl get ingress -n myapp
kubectl describe ingress -n myapp custom-services-ingress
```

### Blue-Green Deployment Issues

**Green environment won't start**:
```bash
# Check logs
docker logs myapp_payment_api_green

# Check port conflicts
docker ps | grep 8101

# Check health
curl http://localhost:8101/health
```

**Traffic switch doesn't work**:
```bash
# Verify nginx config
docker exec myapp_nginx cat /etc/nginx/sites/active-deployment.conf

# Test nginx config
docker exec myapp_nginx nginx -t

# Reload nginx
docker exec myapp_nginx nginx -s reload
```

---

## Best Practices

### Security

✅ **Never commit secrets** - Use CI/CD secret management
✅ **Scan images** - Use Trivy or similar tools
✅ **Run as non-root** - All containers should use non-root users
✅ **Use secret rotation** - Rotate production secrets regularly

### Performance

✅ **Use multi-stage builds** - Smaller images, faster deployments
✅ **Enable caching** - Docker layer caching, npm/pip cache
✅ **Parallel testing** - Run tests in parallel when possible
✅ **Health check tuning** - Adjust intervals based on startup time

### Reliability

✅ **Always backup before deploy** - Database backups are critical
✅ **Test in staging first** - Never skip staging verification
✅ **Monitor after deploy** - Watch metrics for at least 15 minutes
✅ **Have rollback plan** - Test rollback procedures regularly

### Cost Optimization

✅ **Use ARM instances** - 20% cheaper (AWS Graviton, Oracle ARM)
✅ **Right-size resources** - Don't over-provision
✅ **Spot/preemptible instances** - For non-critical workloads
✅ **Clean up old images** - Remove unused Docker images

---

## Additional Resources

- [Custom Services Production Deployment Guide](../CUSTOM-SERVICES-PRODUCTION.md)
- [Multi-Cloud Deployment Guide](../CLOUD-PROVIDERS.md)
- [Production Deployment Best Practices](../PRODUCTION-DEPLOYMENT.md)
- [Kubernetes Command Reference](../../commands/INFRA.md)

---

## Contributing

Found an issue or have an improvement? These examples are templates - feel free to adapt them to your needs. If you have a general improvement that would benefit others, consider contributing back to nself.

---

**Last Updated**: January 30, 2026
**nself Version**: 0.4.8+
