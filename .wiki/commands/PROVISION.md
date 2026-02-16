# nself provision (DEPRECATED)

**Version**: Deprecated in v0.9.9
**Status**: ⚠️ DEPRECATED - Use `nself deploy provision` instead

---

## Migration Guide

This command is deprecated. Use the new consolidated command:

### Old Command
```bash
nself provision <provider> [options]
```

### New Command
```bash
nself deploy provision <provider> [options]
```

## Overview

The `nself provision` command creates and configures cloud servers for nself deployment. It supports multiple cloud providers with normalized sizing.

**This command has been moved to `nself deploy provision` as part of the v1.0 command consolidation.**

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

## Why Deprecated

Part of v1.0 command consolidation (79 → 31 commands). All deployment and infrastructure provisioning commands moved under `nself deploy`.

## Automatic Migration

The old command still works temporarily and automatically redirects:

```bash
$ nself provision --provider hetzner
⚠  The 'nself provision' command is deprecated.
   Please use: nself deploy provision

# Automatically redirects to: nself deploy provision --provider hetzner
```

## Timeline

- **v0.9.9**: Deprecation warning added
- **v1.0.0**: Command will be removed

## See Also

- [deploy](../commands/DEPLOY.md) - Consolidated deployment management
- [COMMAND-TREE-V1](../commands/COMMAND-TREE-V1.md) - Full command structure

---

**Removal Date**: v1.0.0
**Category**: Deprecated Commands
