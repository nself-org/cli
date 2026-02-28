# Production Deployment Documentation

Complete guides for deploying nself and custom services to production environments.

---

## Overview

nself provides comprehensive production deployment capabilities for all services, including custom backend services (CS_N). This documentation covers everything from single-server VPS deployments to multi-cloud Kubernetes orchestration.

---

## 📚 Documentation Index

### Core Deployment Guides

| Guide | Description | Target Audience |
|-------|-------------|-----------------|
| **[Production Deployment](PRODUCTION-DEPLOYMENT.md)** | Complete production deployment workflow | All users |
| **[Custom Services Production](CUSTOM-SERVICES-PRODUCTION.md)** | Deploy custom services (CS_N) to production | Custom service developers |
| **[Cloud Providers](CLOUD-PROVIDERS.md)** | Multi-cloud deployment guide (26+ providers) | DevOps, Cloud architects |

### Examples & Templates

| Resource | Description | Format |
|----------|-------------|--------|
| **[Examples Directory](examples/README.md)** | Production-ready configurations | Complete workflows |
| [GitHub Actions](examples/github-actions-complete.yml) | Full CI/CD pipeline for GitHub | YAML workflow |
| [GitLab CI](examples/gitlab-ci-complete.yml) | Complete GitLab CI/CD pipeline | YAML config |
| [Kubernetes Manifests](examples/kubernetes-manifests.yml) | Production K8s manifests | K8s YAML |
| [Blue-Green Deployment](examples/blue-green-deployment.md) | Zero-downtime deployment pattern | Guide + scripts |

---

## 🚀 Quick Start by Scenario

### Single Server (VPS) Deployment

**Best for**: Startups, small teams, cost-conscious production

```bash
# 1. Provision server
nself provision digitalocean --size medium --region nyc1

# 2. Setup environment
nself config env create prod prod
nself config secrets generate --env prod

# 3. Configure custom services
cat > .environments/prod/.env <<EOF
CS_1=api:express-ts:8001
CS_2=worker:bullmq-js:8002
CS_3=analytics:fastapi:8003
EOF

# 4. Deploy
nself deploy prod

# Custom services deploy automatically via Docker Compose
```

**Read**: [Production Deployment Guide](PRODUCTION-DEPLOYMENT.md)

---

### Kubernetes Deployment

**Best for**: High availability, auto-scaling, enterprise

```bash
# 1. Convert to Kubernetes
nself infra k8s convert --env production

# 2. Deploy to cluster
nself infra k8s deploy --env production

# Custom services deploy as K8s Deployments with:
# - Auto-scaling (HPA)
# - Load balancing
# - Health checks
# - Rolling updates
```

**Read**: [Kubernetes Manifests Example](examples/kubernetes-manifests.yml)

---

### Multi-Cloud Deployment

**Best for**: Global reach, redundancy, disaster recovery

```bash
# Deploy to multiple providers
nself provision aws --region us-east-1
nself deploy prod-aws

nself provision hetzner --region fsn1
nself deploy prod-eu

nself provision digitalocean --region sgp1
nself deploy prod-asia

# Custom services deploy to all regions
```

**Read**: [Cloud Providers Guide](CLOUD-PROVIDERS.md)

---

### CI/CD Automated Deployment

**Best for**: Teams, frequent releases, DevOps best practices

```bash
# 1. Copy workflow template
cp docs/deployment/examples/github-actions-complete.yml .github/workflows/deploy.yml

# 2. Add secrets to GitHub
# - PROD_SSH_KEY
# - PROD_HOST
# - PROD_ENV
# - PROD_SECRETS

# 3. Push to main
git push origin main

# Automatic deployment:
# - Build custom services
# - Run tests
# - Security scan
# - Deploy to staging (auto)
# - Deploy to production (manual approval)
# - Health checks
# - Rollback on failure
```

**Read**: [CI/CD Examples](examples/README.md)

---

## 🎯 Deployment by Custom Service Type

### REST API Service

```bash
# Define service
CS_1=payment-api:express-ts:8001

# Deploy (any method)
nself deploy prod              # Docker Compose
nself k8s deploy              # Kubernetes

# Access
https://payment-api.example.com
```

**Configuration**:
- Exposed via HTTPS (Nginx/Ingress)
- Auto-scaling available (K8s)
- Health checks enabled
- Metrics endpoint `/metrics`

---

### Background Worker

```bash
# Define service
CS_2=notification-worker:bullmq-js:8002

# Deploy
nself deploy prod

# No external access (internal only)
```

**Configuration**:
- Not exposed externally
- Connects to Redis for job queue
- Health checks via process monitoring
- Scalable (multiple replicas)

---

### gRPC Service

```bash
# Define service
CS_3=grpc-api:grpc:50051

# Deploy
nself deploy prod

# Access via gRPC protocol
grpc-api.example.com:50051
```

**Configuration**:
- Custom protocol handling in Nginx/Ingress
- HTTP/2 required
- Health checks via gRPC health probe

---

### ML Inference API

```bash
# Define service
CS_4=ml-inference:fastapi:8004

# Deploy
nself deploy prod

# Access
https://ml-api.example.com
```

**Configuration**:
- Higher resource limits (CPU/memory)
- GPU support (if needed)
- Model versioning
- A/B testing capability

---

## 🔐 Security Best Practices

### Secrets Management

```bash
# Generate production secrets
nself config secrets generate --env prod

# Never commit secrets
echo ".env.secrets" >> .gitignore

# Rotate regularly
nself config secrets rotate --all --env prod
```

### Container Security

```dockerfile
# Run as non-root user
USER 1001

# Read-only filesystem
securityContext:
  readOnlyRootFilesystem: true

# Drop all capabilities
capabilities:
  drop:
  - ALL
```

### Container Security

```dockerfile
# Run as non-root user
USER 1001

# Read-only filesystem (Kubernetes)
securityContext:
  readOnlyRootFilesystem: true

# Drop all capabilities
capabilities:
  drop:
  - ALL
```

---

## 📊 Monitoring & Health Checks

### Health Endpoints

All custom services must implement `/health`:

```javascript
app.get('/health', async (req, res) => {
  const health = {
    status: 'healthy',
    service: process.env.SERVICE_NAME,
    timestamp: new Date().toISOString(),
    checks: {
      database: await checkDatabase(),
      redis: await checkRedis()
    }
  };

  res.status(health.status === 'healthy' ? 200 : 503).json(health);
});
```

### Prometheus Metrics

```javascript
// Expose /metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});
```

### Grafana Dashboards

```bash
# Access monitoring
https://grafana.example.com

# Pre-configured dashboards for:
# - Custom service performance
# - Request rates and latency
# - Error rates
# - Resource usage
```

---

## 🔄 Deployment Strategies

### Rolling Update (Default)

- Zero-downtime
- Gradual rollout
- Automatic rollback on failure

```bash
nself deploy prod --rolling
```

### Blue-Green Deployment

- Instant cutover
- Instant rollback
- Database-safe

```bash
./src/scripts/deploy-blue-green.sh green
```

### Canary Deployment

- Gradual traffic shift (10% → 100%)
- Early issue detection
- A/B testing capability

```nginx
upstream payment_api {
    server payment-api-v1:8001 weight=90;
    server payment-api-v2:8001 weight=10;  # Canary
}
```

---

## 💰 Cost Optimization

### Provider Comparison

| Provider | 4GB RAM | Bandwidth | Cost/mo |
|----------|---------|-----------|---------|
| **Oracle** (free) | 24GB | 10TB | **$0** |
| **Hetzner** | 4GB | 20TB | **$6** |
| **Vultr** | 4GB | 3TB | $12 |
| **DigitalOcean** | 4GB | 4TB | $24 |
| **AWS** | 4GB | 100GB | $30+ |

### Optimization Strategies

1. **Right-sizing**: Start small, scale up as needed
2. **ARM instances**: 20% cheaper (AWS Graviton, Oracle ARM)
3. **Spot instances**: 60-80% savings for fault-tolerant workloads
4. **Reserved instances**: 30-40% savings with commitment
5. **Multi-cloud**: Use cheap providers for non-critical regions

---

## 🌍 Multi-Region Deployment

### Active-Active

```bash
# US East (AWS)
nself provision aws --region us-east-1
nself deploy prod-us

# Europe (Hetzner)
nself provision hetzner --region fsn1
nself deploy prod-eu

# Asia (DigitalOcean)
nself provision digitalocean --region sgp1
nself deploy prod-asia

# Geo-routing with CloudFlare
```

### Active-Passive (DR)

```bash
# Primary: DigitalOcean NYC
nself deploy prod

# DR: Vultr LA (standby)
nself deploy prod-dr

# Automatic failover on primary failure
```

---

## 🛠 Troubleshooting

### Service Won't Start

```bash
# Check logs
nself logs payment-api

# Check health
curl http://localhost:8001/health

# Check resources
docker stats myapp_payment_api
```

### Database Connection Issues

```bash
# Test connectivity
docker exec myapp_payment_api nc -zv postgres 5432

# Check credentials
docker exec myapp_payment_api env | grep POSTGRES

# View PostgreSQL logs
nself logs postgres
```

### Deployment Fails

```bash
# Check validation
nself validate prod --strict

# Check SSH access
nself deploy check-access

# Check server resources
ssh user@server "df -h && free -m"

# View deployment logs
nself deploy logs --env prod
```

---

## 📖 Learning Path

### 1. Beginners (First Production Deployment)

1. Read: [Production Deployment Guide](PRODUCTION-DEPLOYMENT.md)
2. Choose: Single VPS provider (recommend Hetzner or Oracle free tier)
3. Follow: Quick start guide
4. Deploy: Docker Compose deployment
5. Monitor: Setup monitoring bundle

### 2. Intermediate (CI/CD & Automation)

1. Read: [CI/CD Examples](examples/README.md)
2. Setup: GitHub Actions or GitLab CI
3. Configure: Staging + Production environments
4. Implement: Automated testing and deployment
5. Practice: Rollback procedures

### 3. Advanced (Multi-Cloud & K8s)

1. Read: [Cloud Providers Guide](CLOUD-PROVIDERS.md)
2. Read: [Kubernetes Manifests](examples/kubernetes-manifests.yml)
3. Deploy: Multi-region setup
4. Implement: Blue-green or canary deployments
5. Optimize: Cost and performance tuning

---

## 🆘 Support & Resources

### Documentation

- [Custom Services Overview](../services/SERVICES_CUSTOM.md)
- [Service Templates](../services/SERVICE-TEMPLATES.md)
- [Environment Management](../commands/CONFIG.md)
- [Kubernetes Commands](../commands/INFRA.md)

### Command Reference

```bash
nself deploy --help          # Deployment help
nself infra k8s --help      # Kubernetes help
nself config secrets --help # Secrets management
nself config validate --help # Pre-deployment validation
```

### Community

- GitHub Issues: Bug reports and feature requests
- Discussions: Questions and community support
- Examples: Share your deployment configs

---

## ✅ Pre-Deployment Checklist

Before deploying to production:

- [ ] All tests passing locally
- [ ] Environment configuration reviewed
- [ ] Secrets generated and secured
- [ ] Backup strategy in place
- [ ] Monitoring configured
- [ ] SSL certificates ready
- [ ] DNS records configured
- [ ] Health checks implemented
- [ ] Rollback procedure tested
- [ ] Team notified of deployment

---

**Last Updated**: January 30, 2026
**nself Version**: 0.4.8+

**Next Steps**: Choose your deployment method above and follow the corresponding guide.
