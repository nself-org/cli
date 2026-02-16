# nself provision - Server Provisioning

**Version 0.9.9** | Provision cloud servers

---

## Overview

The `nself provision` command creates and configures cloud servers for nself deployment. It supports multiple cloud providers with normalized sizing.

> **Note**: This is a legacy command. Use `nself cloud server create` for new deployments.

---

## Basic Usage

```bash
# Provision with interactive wizard
nself provision

# Provision on specific provider
nself provision --provider digitalocean

# Use predefined configuration
nself provision --config server.json
```

---

## Supported Providers

| Provider | Command |
|----------|---------|
| DigitalOcean | `--provider digitalocean` |
| AWS | `--provider aws` |
| Linode | `--provider linode` |
| Hetzner | `--provider hetzner` |
| Vultr | `--provider vultr` |

See [providers](../guides/PROVIDERS-COMPLETE.md) for full list.

---

## Server Sizes

| Size | vCPUs | RAM | Storage |
|------|-------|-----|---------|
| `small` | 1 | 1GB | 25GB |
| `medium` | 2 | 4GB | 80GB |
| `large` | 4 | 8GB | 160GB |
| `xlarge` | 8 | 16GB | 320GB |

```bash
nself provision --size medium
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--provider` | Cloud provider |
| `--size` | Server size |
| `--region` | Server region |
| `--name` | Server name |
| `--config` | Configuration file |
| `--dry-run` | Preview only |

---

## See Also

- [cloud](CLOUD.md) - Cloud operations
- [servers](SERVERS.md) - Server management
- [deploy](DEPLOY.md) - Deployment
