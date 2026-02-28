# Infrastructure Documentation

Infrastructure deployment and orchestration documentation for nself.

## Overview

This directory contains documentation for deploying nself to various infrastructure platforms, including Kubernetes, Helm, and cloud-native environments.

## Kubernetes Deployment

### Implementation Guides
- **[Kubernetes Implementation Guide](K8S-IMPLEMENTATION-GUIDE.md)** - Complete guide for deploying nself to Kubernetes clusters

## Coming Soon

The following infrastructure documentation is under development:

- **Helm Charts** - Kubernetes deployment via Helm
- **Terraform Modules** - Infrastructure as Code
- **Cloud Provider Specifics** - AWS EKS, GKE, AKS deployment guides
- **Service Mesh Integration** - Istio/Linkerd integration
- **GitOps Workflows** - ArgoCD/Flux deployment patterns

## Related Documentation

- **[Deployment Guide](../deployment/README.md)** - Production deployment overview
- **[Cloud Providers](../deployment/CLOUD-PROVIDERS.md)** - 26+ supported cloud providers
- **[Server Management](../deployment/SERVER-MANAGEMENT.md)** - Server configuration
- **[Commands: infra](../commands/INFRA.md)** - Infrastructure CLI commands

## Quick Start

For Kubernetes deployment:

```bash
# View Kubernetes commands
nself infra k8s --help

# Generate Kubernetes manifests
nself infra k8s manifests

# Deploy to cluster
nself infra k8s deploy

# Monitor deployment
nself infra k8s status
```

For cloud provider deployment:

```bash
# List supported providers
nself infra provider list

# Configure provider
nself infra provider setup aws

# Deploy infrastructure
nself deploy prod --provider aws
```

---

**Last Updated**: January 31, 2026
**Version**: v0.9.6
