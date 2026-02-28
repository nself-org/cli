# Infrastructure Documentation

Complete infrastructure deployment documentation for nself.

## What's in This Directory

This directory contains guides for deploying nself to various infrastructure platforms:

- **Kubernetes** - Container orchestration
- **Helm** - Kubernetes package management
- **Terraform** - Infrastructure as Code
- **Cloud Providers** - AWS, GCP, Azure, and more

## Available Documentation

### Kubernetes
- **[K8S Implementation Guide](K8S-IMPLEMENTATION-GUIDE.md)** - Deploy nself to Kubernetes

### Coming Soon
- Helm chart documentation
- Terraform module documentation
- Cloud provider-specific guides
- Service mesh integration guides

## CLI Commands

nself provides infrastructure management commands under `nself infra`:

```bash
# Kubernetes commands
nself infra k8s init              # Initialize K8s config
nself infra k8s manifests         # Generate manifests
nself infra k8s deploy            # Deploy to cluster
nself infra k8s status            # Check deployment status

# Helm commands
nself infra helm install          # Install via Helm
nself infra helm upgrade          # Upgrade release
nself infra helm uninstall        # Remove release

# Provider commands
nself infra provider list         # List supported providers
nself infra provider setup <name> # Configure provider
nself infra provider deploy       # Deploy infrastructure
```

See **[infra command documentation](../commands/INFRA.md)** for complete reference.

## Related Documentation

- **[Deployment](../deployment/README.md)** - Production deployment guides
- **[Cloud Providers](../deployment/CLOUD-PROVIDERS.md)** - Supported cloud platforms
- **[Architecture](../architecture/ARCHITECTURE.md)** - System architecture
- **[Configuration](../configuration/README.md)** - Configuration reference

## Quick Links

- [Command Reference](../commands/INFRA.md)
- [Production Deployment](../deployment/PRODUCTION-DEPLOYMENT.md)
- [Cloud Providers Guide](../deployment/CLOUD-PROVIDERS.md)

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
