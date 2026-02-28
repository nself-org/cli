# New Features Guide - v0.9.6

**Version:** nself v0.9.6
**Release Date:** January 2026

Complete usage examples for all new features introduced in v0.9.6.

---

## Table of Contents

- [Overview](#overview)
- [Destroy Command](#destroy-command---safe-infrastructure-teardown)
- [Server Management](#server-management---complete-vps-lifecycle)
- [Kubernetes Abstraction](#kubernetes-abstraction---multi-cloud-k8s)
- [Common Workflows](#common-workflows)
- [Migration Guide](#migration-guide-from-old-commands)

---

## Overview

v0.9.6 introduces three major feature sets:

1. **Safe Destruction** - `nself destroy` with selective targeting
2. **Server Management** - `nself deploy server` with 10 subcommands
3. **K8s Abstraction** - Unified Kubernetes across 8 cloud providers

### Quick Command Reference

```bash
# New Commands
nself destroy                              # Safe infrastructure teardown
nself deploy server <subcommand>           # Server lifecycle management
nself deploy sync <subcommand>             # Environment synchronization
nself infra provider k8s-create <provider> # Create K8s cluster
nself infra provider k8s-delete <provider> # Delete K8s cluster
nself infra provider k8s-kubeconfig <provider> # Get kubeconfig
```

---

## Destroy Command - Safe Infrastructure Teardown

### Basic Usage

```bash
# Interactive destruction (safest - recommended)
nself destroy

# Preview without executing
nself destroy --dry-run

# Preserve data volumes
nself destroy --keep-volumes

# Non-interactive (automation)
nself destroy --force
```

### Selective Destruction

```bash
# Only remove Docker containers
nself destroy --containers-only

# Only remove Docker volumes (DATA LOSS!)
nself destroy --volumes-only

# Only remove Docker networks
nself destroy --networks-only

# Only remove generated files
nself destroy --generated-only
```

### Common Scenarios

#### Clean Rebuild (Preserve Data)

```bash
# Step 1: Destroy infrastructure but keep databases
nself destroy --keep-volumes

# Step 2: Regenerate configurations
nself build

# Step 3: Start with existing data
nself start

# Result: Fresh infrastructure, data intact
```

#### Force Container Restart

```bash
# Remove only containers (no data loss)
nself destroy --containers-only

# Restart services
nself start
```

#### Complete Fresh Start

```bash
# Step 1: Backup first (safety)
nself backup create

# Step 2: Full destruction
nself destroy

# Step 3: Re-initialize
nself init --wizard

# Step 4: Build and start
nself build && nself start
```

#### Automation Script

```bash
#!/bin/bash
# Automated teardown with safety

# Safety backup
nself backup create

# Destroy (non-interactive)
nself destroy --force

# Clean Docker system
docker system prune -af --volumes

# Reinitialize
nself init --simple
nself build
nself start
```

### Safety Features

```bash
# Dry run - see what would happen
nself destroy --dry-run

# Output shows:
# - 25 containers to be removed
# - 8 volumes to be removed (DATA LOSS warning)
# - 2 networks to be removed
# - Generated files to be removed

# Interactive mode requires:
# 1. "yes" confirmation
# 2. Project name confirmation for volume destruction

# Verbose output for debugging
nself destroy --verbose
```

---

## Server Management - Complete VPS Lifecycle

### Quick Start - Initialize Production Server

```bash
# Complete server setup in one command
nself deploy server init root@prod.example.com \
  --domain example.com \
  --yes

# What it does:
# 1. Updates system packages
# 2. Installs Docker and Docker Compose
# 3. Configures UFW firewall (ports 22, 80, 443)
# 4. Sets up fail2ban for SSH protection
# 5. Hardens SSH configuration
# 6. Creates /var/www/nself directory
# 7. Sets up SSL if domain resolves
```

### Server Health Checks

```bash
# Quick status of all configured servers
nself deploy server status
# Output:
#   staging      ● ONLINE   up 2 days
#   production   ● ONLINE   up 15 days

# Verify specific server readiness
nself deploy server check root@prod.example.com
# Checks:
# 1. SSH connectivity
# 2. Docker installation
# 3. Docker service status
# 4. Docker Compose availability
# 5. Disk space
# 6. Memory
# 7. Firewall status
# 8. Required ports (80, 443)

# Full diagnostics
nself deploy server diagnose prod
# Includes:
# - DNS resolution
# - ICMP ping test
# - Port accessibility (22, 80, 443)
# - SSH connection test
# - Server information (OS, kernel, uptime, etc.)
```

### Server Configuration

```bash
# Add server to configuration
nself deploy server add staging \
  --host staging.example.com \
  --user deploy \
  --port 2222 \
  --key ~/.ssh/staging_key \
  --path /opt/myapp

# List all servers
nself deploy server list
# Output:
# NAME         HOST                    USER    PORT   STATUS
# staging      staging.example.com     deploy  2222   online
# production   prod.example.com        root    22     online

# Get comprehensive server info
nself deploy server info prod
# Shows:
# - Connection details
# - System information
# - Deployment status
# - Quick action commands

# Remove server
nself deploy server remove old-staging --force
```

### SSH Operations

```bash
# Interactive SSH session
nself deploy server ssh staging

# Execute remote command
nself deploy server ssh staging "docker ps"

# Check disk space
nself deploy server ssh prod "df -h"

# View logs
nself deploy server ssh staging "tail -f /var/log/nginx/access.log"

# Multiple commands
nself deploy server ssh prod "
  docker ps
  df -h
  free -m
  uptime
"
```

### Environment Synchronization

```bash
# Pull configuration from remote
nself deploy sync pull staging

# Preview sync without changes
nself deploy sync pull staging --dry-run

# Push configuration to remote
nself deploy sync push staging

# Full synchronization (configs + services + restart)
nself deploy sync full staging

# Check sync status
nself deploy sync status
# Output:
# ENVIRONMENT   STATUS      LAST SYNC              FILES
# staging       synced      2026-01-30T14:23:15Z   complete
# production    synced      2026-01-28T09:15:43Z   complete
```

### Complete Deployment Workflow

```bash
# 1. Initialize server
nself deploy server init root@prod.example.com --domain example.com

# 2. Verify server is ready
nself deploy server check prod

# 3. Create production environment locally
nself config env create prod

# 4. Add server configuration
nself deploy server add prod --host prod.example.com

# 5. Generate production secrets
nself config secrets generate --env prod

# 6. Build for production
nself build --env prod

# 7. Push configuration to server
nself deploy sync push prod

# 8. Deploy to production
nself deploy production

# 9. Verify deployment
nself deploy server info prod
```

---

## Kubernetes Abstraction - Multi-Cloud K8s

### Supported Providers

```bash
# AWS EKS - $73/month control plane
nself infra provider k8s-create aws cluster us-east-1 3 medium

# GCP GKE - Free control plane
nself infra provider k8s-create gcp cluster us-central1 3 medium

# Azure AKS - Free control plane
nself infra provider k8s-create azure cluster eastus 3 medium

# DigitalOcean DOKS - $12/month
nself infra provider k8s-create digitalocean cluster nyc3 3 medium

# Linode LKE - Free control plane
nself infra provider k8s-create linode cluster us-east 3 medium

# Vultr VKE - Free control plane
nself infra provider k8s-create vultr cluster ewr 3 medium

# Hetzner - Free control plane (manual setup via console)
# Visit: https://console.hetzner.cloud/

# Scaleway Kapsule - Free control plane
nself infra provider k8s-create scaleway cluster fr-par-1 3 medium
```

### Node Size Options

```bash
# small - Development (~2 vCPU, 4GB RAM)
nself infra provider k8s-create aws dev-cluster us-east-1 2 small

# medium - Production (~2-4 vCPU, 8-16GB RAM)
nself infra provider k8s-create gcp prod-cluster us-central1 3 medium

# large - High-performance (~4-8 vCPU, 16-32GB RAM)
nself infra provider k8s-create azure api-cluster eastus 5 large

# xlarge - Enterprise (~8-16 vCPU, 32-64GB RAM)
nself infra provider k8s-create digitalocean db-cluster nyc3 3 xlarge
```

### Complete K8s Deployment

```bash
# 1. Install provider CLI
brew install awscli eksctl  # AWS
brew install google-cloud-sdk  # GCP
brew install doctl  # DigitalOcean

# 2. Initialize provider
nself infra provider init aws

# 3. Create cluster (10-15 min)
nself infra provider k8s-create aws production us-east-1 3 medium

# 4. Get kubeconfig
nself infra provider k8s-kubeconfig aws production us-east-1

# 5. Verify connection
kubectl get nodes

# 6. Deploy application
nself infra k8s deploy production

# 7. Monitor deployment
kubectl get pods
kubectl get services

# 8. Access application
kubectl port-forward svc/nself-api 8080:80
```

### Multi-Cloud Deployment

```bash
# Deploy to multiple providers for redundancy

# Create clusters
nself infra provider k8s-create aws prod-aws us-east-1 3 medium
nself infra provider k8s-create gcp prod-gcp us-central1 3 medium
nself infra provider k8s-create azure prod-azure eastus 3 medium

# Get all kubeconfigs
nself infra provider k8s-kubeconfig aws prod-aws us-east-1
nself infra provider k8s-kubeconfig gcp prod-gcp us-central1
nself infra provider k8s-kubeconfig azure prod-azure eastus

# Switch between clusters
kubectx prod-aws
kubectx prod-gcp
kubectx prod-azure

# Deploy to all
for cluster in prod-aws prod-gcp prod-azure; do
  kubectx $cluster
  nself infra k8s deploy production
done
```

### Cost Optimization

```bash
# Use providers with free control planes
# GCP GKE - Free
nself infra provider k8s-create gcp prod us-central1 3 medium
# Total: ~$144/month (nodes only)

# Linode LKE - Free
nself infra provider k8s-create linode prod us-east 3 medium
# Total: ~$144/month (nodes only)

# Hetzner - Free, best value
# Manual setup via console
# Total: ~$33/month (cheapest option!)

# Compare to AWS EKS
nself infra provider k8s-create aws prod us-east-1 3 medium
# Total: ~$253/month ($73 control plane + $180 nodes)
```

### Cleanup

```bash
# Delete clusters
nself infra provider k8s-delete aws production us-east-1
nself infra provider k8s-delete gcp production us-central1
nself infra provider k8s-delete azure production eastus

# Verify deletion
kubectl config get-contexts
```

---

## Common Workflows

### Development Environment Reset

```bash
# 1. Backup current state
nself backup create

# 2. Destroy but keep data
nself destroy --keep-volumes

# 3. Rebuild
nself build

# 4. Start fresh
nself start
```

### Production Server Setup

```bash
# 1. Initialize VPS
nself deploy server init root@prod.example.com --domain example.com

# 2. Verify readiness
nself deploy server check prod

# 3. Create local environment
nself config env create prod

# 4. Configure server
nself deploy server add prod --host prod.example.com

# 5. Generate secrets
nself config secrets generate --env prod

# 6. Build production config
nself build --env prod

# 7. Sync to server
nself deploy sync push prod

# 8. Deploy
nself deploy production

# 9. Verify
nself deploy server status
```

### Multi-Cloud Kubernetes

```bash
# 1. Initialize providers
nself infra provider init aws
nself infra provider init gcp

# 2. Create clusters
nself infra provider k8s-create aws prod-aws us-east-1 3 medium
nself infra provider k8s-create gcp prod-gcp us-central1 3 medium

# 3. Get kubeconfigs
nself infra provider k8s-kubeconfig aws prod-aws us-east-1
nself infra provider k8s-kubeconfig gcp prod-gcp us-central1

# 4. Deploy to both
kubectx prod-aws && kubectl apply -f deployment.yaml
kubectx prod-gcp && kubectl apply -f deployment.yaml

# 5. Monitor both
watch -n 2 'echo "AWS:"; kubectx prod-aws; kubectl get pods; echo "GCP:"; kubectx prod-gcp; kubectl get pods'
```

### Automated Testing Pipeline

```bash
#!/bin/bash
# CI/CD test pipeline

# 1. Create test environment
nself init --simple

# 2. Build
nself build

# 3. Start services
nself start

# 4. Run tests
npm test

# 5. Cleanup
nself destroy --force

# 6. Clean Docker
docker system prune -af --volumes
```

---

## Migration Guide from Old Commands

### Command Consolidation Changes

```bash
# Old commands → New commands (v0.9.6)

# Server management
nself servers list          → nself deploy server list
nself servers add           → nself deploy server add
nself servers remove        → nself deploy server remove
nself servers ssh           → nself deploy server ssh

# Sync operations
nself sync pull             → nself deploy sync pull
nself sync push             → nself deploy sync push

# Kubernetes
nself k8s create            → nself infra k8s create
nself k8s delete            → nself infra k8s delete
nself provider k8s-create   → nself infra provider k8s-create
```

### Updating Scripts

**Before (v0.9.5):**
```bash
#!/bin/bash
nself servers add prod --host prod.example.com
nself sync push prod
nself prod
```

**After (v0.9.6):**
```bash
#!/bin/bash
nself deploy server add prod --host prod.example.com
nself deploy sync push prod
nself deploy production
```

---

## Best Practices

### 1. Always Backup Before Destruction

```bash
# Create backup before any destructive operation
nself backup create
nself destroy
```

### 2. Use Dry Run for Preview

```bash
# Always preview before executing
nself destroy --dry-run
nself deploy sync push prod --dry-run
```

### 3. Verify Server Health

```bash
# Check before deployment
nself deploy server check prod
nself deploy server diagnose prod
```

### 4. Use Selective Destruction

```bash
# Prefer targeted destruction over full teardown
nself destroy --containers-only  # Instead of full destroy
nself destroy --keep-volumes     # Preserve data
```

### 5. Cost-Optimize K8s Deployments

```bash
# Use providers with free control planes
nself infra provider k8s-create gcp prod us-central1 3 medium  # Free control plane
nself infra provider k8s-create linode prod us-east 3 medium   # Free control plane
# Instead of:
nself infra provider k8s-create aws prod us-east-1 3 medium    # $73/month control plane
```

---

## Troubleshooting

### Destroy Issues

```bash
# Resources not fully removed
nself destroy --verbose
docker ps -a | grep myapp
docker volume ls | grep myapp

# Permission denied
sudo nself destroy

# Volumes in use
nself stop
nself destroy --containers-only
nself destroy --volumes-only
```

### Server Management Issues

```bash
# Connection failed
nself deploy server diagnose prod
ssh -v root@prod.example.com  # Verbose SSH

# Init failed
nself deploy server init root@prod.example.com --verbose

# Sync failed
nself deploy sync pull prod --dry-run
nself deploy server ssh prod "ls -la /var/www/nself"
```

### Kubernetes Issues

```bash
# Cluster creation failed
nself infra provider validate aws
aws eks describe-cluster --name production

# Kubeconfig not working
nself infra provider k8s-kubeconfig aws production us-east-1
kubectl config get-contexts

# Node pool failed
kubectl get nodes
kubectl describe node <node-name>
```

---

## Additional Resources

### Documentation
- [Destroy Command Reference](../commands/DESTROY.md)
- [Server Management Guide](../deployment/SERVER-MANAGEMENT.md)
- [Kubernetes Implementation Guide](../infrastructure/K8S-IMPLEMENTATION-GUIDE.md)
- [Command Tree v1.0](../commands/COMMAND-TREE-V1.md)

### Related Commands
- `nself backup` - Backup and recovery
- `nself deploy` - Deployment operations
- `nself infra` - Infrastructure management
- `nself config` - Configuration management

---

**Version:** nself v0.9.6
**Updated:** January 2026
**Maintainer:** nself core team
