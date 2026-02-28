# Deployment Documentation

Guide to deploying ɳSelf to production servers and cloud providers.

## Overview

- **[Deployment Guide](README.md)** - Complete deployment overview
- **[Production Deployment](PRODUCTION-DEPLOYMENT.md)** - Production best practices
- **[Server Management](SERVER-MANAGEMENT.md)** - Managing deployment servers

## Deployment Strategies

### Cloud Deployment
- **[Cloud Providers](CLOUD-PROVIDERS.md)** - 26+ supported cloud providers
  - AWS, Google Cloud, Azure, DigitalOcean, Linode, Vultr, Hetzner, and more

### Custom Services in Production
- **[Custom Services Production](CUSTOM-SERVICES-PRODUCTION.md)** - Deploying custom services

## Deployment Examples

- **[Examples Index](examples/README.md)** - Real-world deployment examples
- **[Blue-Green Deployment](examples/blue-green-deployment.md)** - Zero-downtime deployments

## Deployment Commands

### Quick Deployment
```bash
# Create production environment
nself env create prod production

# Edit server configuration
# Edit .environments/prod/server.json

# Deploy to production
nself deploy prod
```

### Environment Management
```bash
# Switch environments
nself env switch local
nself env switch staging
nself env switch prod

# Sync configurations
nself deploy sync pull staging
nself deploy sync pull prod
nself deploy sync pull secrets  # Lead dev only
```

### Server Operations
```bash
# Provision new server
nself deploy provision staging

# Manage servers
nself deploy server list
nself deploy server add staging user@server.com
nself deploy server remove staging
```

## Configuration Files

### Server Configuration
`.environments/prod/server.json`:
```json
{
  "host": "your-server.com",
  "user": "deploy",
  "port": 22,
  "path": "/var/www/myapp"
}
```

### Environment-Specific Config
- `.environments/local/.env` - Local development
- `.environments/staging/.env` - Staging server
- `.environments/prod/.env` - Production server

## Deployment Checklist

### Pre-Deployment
- [ ] Test locally: `nself start`
- [ ] Run tests: `nself dev ci`
- [ ] Security audit: `nself doctor --security`
- [ ] Backup database: `nself db backup`

### Production Deployment
- [ ] Set up server with SSH access
- [ ] Configure `.environments/prod/server.json`
- [ ] Generate production secrets: `nself deploy provision prod`
- [ ] Deploy: `nself deploy prod`
- [ ] Verify: `nself status --env prod`
- [ ] Monitor: `nself monitor --env prod`

### Post-Deployment
- [ ] Verify all services running
- [ ] Check logs: `nself logs --env prod`
- [ ] Test endpoints
- [ ] Set up monitoring alerts
- [ ] Configure backups

## Supported Environments

### Local Development
- Docker Compose
- Local services
- Mock data and users

### Staging
- Production-like environment
- Real services, test data
- QA and testing

### Production
- Full production setup
- Real users and data
- Enhanced security and monitoring

## Access Control

### Developer Roles

| Role | Local | Staging | Production | Secrets |
|------|-------|---------|------------|---------|
| Developer | ✅ | ❌ | ❌ | ❌ |
| Senior Dev | ✅ | ✅ | ❌ | ❌ |
| Lead Dev | ✅ | ✅ | ✅ | ✅ |

Access controlled via SSH keys:
```bash
# Check your access level
nself env access

# Test specific environment access
nself env access --check staging
nself env access --check prod
```

## Deployment Architecture

### Single Server
- All services on one server
- Simplest deployment
- Good for small/medium apps

### Multi-Server
- Services distributed across servers
- Better scalability
- Load balancing

### Kubernetes
- Container orchestration
- Auto-scaling
- High availability
```bash
nself infra k8s init
nself infra k8s deploy
```

### Docker Swarm
- Native Docker clustering
- Simpler than Kubernetes
- Good for medium-scale

---

**[← Back to Documentation Home](../README.md)**
