# nself provider - Provider Infrastructure Management

**Version**: 0.4.7+ | **Status**: Available

---

## Overview

The `nself provider` command provides unified management for provider infrastructure across 26 cloud providers. It consolidates provider configuration, server provisioning, and cost management into a single coherent interface.

---

## Subcommands

### Provider Management

```bash
nself provider list              # List all 26 supported providers
nself provider init <provider>   # Configure provider credentials
nself provider validate          # Validate current configuration
nself provider info <provider>   # Show provider details
```

### Server Management

```bash
nself provider server create <provider>   # Provision new server
nself provider server destroy <server>    # Destroy server
nself provider server list                # List all managed servers
nself provider server status [server]     # Server health status
nself provider server ssh <server>        # SSH to server
nself provider server add <ip>            # Add existing server
nself provider server remove <server>     # Remove from registry
```

### Cost Management

```bash
nself provider cost estimate <provider>   # Estimate monthly costs
nself provider cost compare               # Compare costs across providers
```

### Quick Deployment

```bash
nself provider deploy quick               # Provision and deploy in one step
nself provider deploy full                # Full production setup
```

---

## Provider Configuration

### Initializing a Provider

```bash
$ nself provider provider init digitalocean
Initializing DigitalOcean provider...
API Token: ****************************
Default Region [nyc1]:
Default Size [small]:
Configuration saved to ~/.nself/providers/digitalocean.yml
```

### Configuration Files

Provider credentials are stored in `~/.nself/providers/<provider>.yml`:

```yaml
provider: digitalocean
api_token: "dop_v1_xxx"
default_region: "nyc1"
default_size: "small"
```

---

## Size Mapping

nself uses normalized sizes that map to provider-specific instance types:

| Size | vCPU | RAM | Description |
|------|------|-----|-------------|
| `tiny` | 1 | 1GB | Testing only |
| `small` | 1-2 | 2GB | Development |
| `medium` | 2 | 4-8GB | Staging |
| `large` | 4 | 8-16GB | Production |
| `xlarge` | 8+ | 32GB+ | High traffic |

Example mappings:

| Provider | small | medium | large |
|----------|-------|--------|-------|
| DigitalOcean | s-1vcpu-2gb | s-2vcpu-4gb | s-4vcpu-8gb |
| Hetzner | cx21 | cx31 | cx41 |
| Vultr | vc2-1c-2gb | vc2-2c-4gb | vc2-4c-8gb |
| AWS | t3.small | t3.medium | t3.large |

---

## Server Operations

### Creating a Server

```bash
$ nself provider server create digitalocean --name myapp-prod --size medium
Provisioning DigitalOcean Droplet...
Name: myapp-prod
Size: s-2vcpu-4gb (medium)
Region: nyc1
Image: Ubuntu 22.04

Creating droplet... done
Waiting for IP... 164.92.123.45
Installing nself prerequisites... done

Server ready!
IP: 164.92.123.45
SSH: ssh root@164.92.123.45
```

### Adding Existing Servers

For servers provisioned outside nself:

```bash
$ nself provider server add 164.92.123.45 --name prod-server --provider manual
Testing SSH connection... success
Adding to server registry... done

Server 'prod-server' added successfully
```

### Server Registry

Managed servers are tracked in `~/.nself/servers.yml`:

```yaml
servers:
  myapp-prod:
    ip: 164.92.123.45
    provider: digitalocean
    region: nyc1
    size: medium
    created: 2026-01-23T10:30:00Z

  prod-server:
    ip: 164.92.200.100
    provider: manual
    created: 2026-01-23T11:00:00Z
```

---

## Cost Comparison

### Estimate for Single Provider

```bash
$ nself provider cost estimate hetzner --size medium
Hetzner Cost Estimate (medium)
==============================
Instance: CX31 (2 vCPU, 8GB RAM)
Monthly: ~$6.90
Hourly: ~$0.01

Includes:
- 80GB SSD
- 20TB Traffic
- IPv4 + IPv6
```

### Compare All Providers

```bash
$ nself provider cost compare --size medium
Provider Cost Comparison (medium)
=================================
Provider        Monthly    Specs
--------------- ---------- ----------------
Hetzner         $6.90      2 vCPU / 8GB
Contabo         $8.49      4 vCPU / 8GB
RackNerd        $10.00     2 vCPU / 4GB
Vultr           $12.00     2 vCPU / 4GB
DigitalOcean    $24.00     2 vCPU / 4GB
Linode          $24.00     2 vCPU / 4GB
AWS             $35.00     2 vCPU / 4GB (t3.medium)
```

---

## Supported Providers

### Enterprise Cloud
- AWS (EKS support)
- Google Cloud (GKE support)
- Microsoft Azure (AKS support)
- Oracle Cloud (OKE support)
- IBM Cloud (IKS support)

### Developer VPS
- DigitalOcean (DOKS support)
- Linode (LKE support)
- Vultr (VKE support)
- Hetzner
- OVH
- Scaleway (SKS support)
- UpCloud

### Budget VPS
- Contabo
- Hostinger
- Kamatera
- SSDNodes
- RackNerd
- BuyVM
- Time4VPS

### Regional
- Alibaba Cloud (ACK - Asia)
- Tencent Cloud (TKE - Asia)
- Yandex Cloud (MKS - Russia/CIS)
- Exoscale (SKS - Swiss/EU)

### Edge/Custom
- Raspberry Pi
- Custom SSH

---

## Quick Deploy

### One-Command Deployment

```bash
$ nself provider deploy quick --provider digitalocean
This will:
1. Provision a new server on DigitalOcean (medium)
2. Install Docker and nself prerequisites
3. Deploy your application

Estimated cost: $24/month
Continue? [Y/n]: y

Provisioning server... done (164.92.123.45)
Installing prerequisites... done
Deploying application... done

Deployment complete!
URL: https://myapp.example.com
SSH: ssh root@164.92.123.45
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NSELF_DEFAULT_PROVIDER` | Default cloud provider | - |
| `NSELF_DEFAULT_REGION` | Default region | Provider default |
| `NSELF_DEFAULT_SIZE` | Default instance size | `small` |

---

## Examples

### Development Workflow

```bash
# Setup provider once
nself provider provider init hetzner

# Create development server
nself provider server create hetzner --name dev --size small

# Deploy to it
nself deploy --target dev
```

### Production Setup

```bash
# Compare costs first
nself provider cost compare --size large

# Provision production server
nself provider server create digitalocean \
  --name prod \
  --size large \
  --region nyc1

# Full deployment with monitoring
nself provider deploy full --target prod
```

---

## Troubleshooting

### Connection Issues

```bash
# Test SSH access
nself provider server status myserver

# Manual SSH test
ssh -v root@<ip>
```

### Provider Validation

```bash
# Validate all configured providers
nself provider provider validate

# Check specific provider
nself provider provider info digitalocean
```

---

*See also: [K8S](K8S.md) | [HELM](HELM.md) | [DEPLOY](DEPLOY.md)*
