# nself servers - Server Management

> **⚠️ DEPRECATED**: `nself servers` is deprecated and will be removed in v1.0.0.
> Please use `nself deploy server` instead.
> Run `nself deploy server --help` for full usage information.

**Version 0.4.6** | Server and infrastructure management

---

## Overview

The `nself servers` command manages your deployment servers and infrastructure. Add, remove, and monitor servers across all environments.

---

## Usage

```bash
nself servers <subcommand> [options]
```

---

## Subcommands

### `list` (default)

List all configured servers.

```bash
nself servers                   # List all
nself servers list              # Same as above
nself servers list --env prod   # Filter by environment
```

### `add <name>`

Add a new server.

```bash
nself servers add web1 --ip 1.2.3.4
nself servers add staging-db --ip 5.6.7.8 --provider hetzner
```

### `remove <name>`

Remove a server.

```bash
nself servers remove web1
nself servers remove web1 --force
```

### `status [name]`

Check server status.

```bash
nself servers status            # All servers
nself servers status web1       # Specific server
```

### `ssh <name>`

SSH into a server.

```bash
nself servers ssh web1
nself servers ssh web1 -- ls -la  # Run command
```

### `logs <name>`

View server logs.

```bash
nself servers logs web1
nself servers logs web1 --lines 100
```

### `update <name>`

Update server configuration.

```bash
nself servers update web1 --ip 1.2.3.5
nself servers update web1 --user deploy
```

### `reboot <name>`

Reboot a server.

```bash
nself servers reboot web1
nself servers reboot web1 --force
```

### `info <name>`

Show detailed server info.

```bash
nself servers info web1
```

---

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--provider NAME` | Cloud provider | manual |
| `--region REGION` | Server region | - |
| `--type TYPE` | Server type/size | - |
| `--ip IP` | Server IP address | - |
| `--user USER` | SSH user | root |
| `--env NAME` | Environment filter | all |
| `--lines N` | Log lines to show | 100 |
| `--force` | Skip confirmation | false |
| `--json` | Output in JSON format | false |
| `-h, --help` | Show help message | - |

---

## Examples

```bash
# List all servers
nself servers list

# Add a new server
nself servers add prod-web --ip 123.45.67.89 --provider hetzner --env prod

# Check server status
nself servers status

# SSH to server
nself servers ssh prod-web

# View server info
nself servers info prod-web

# View logs
nself servers logs prod-web

# Reboot server
nself servers reboot prod-web --force
```

---

## Output Example

### List Command

```
  ➞ Server List

  Name                 IP              Provider        Environment  Status
  ----                 --              --------        -----------  ------
  prod-web             123.45.67.89    hetzner         prod         active
  staging-web          234.56.78.90    digitalocean    staging      active
  dev-db               345.67.89.01    manual          dev          stopped

  Total: 3 server(s)
```

### Status Command

```
  ➞ Server Status

  Name                 IP              SSH        Ping       Load
  ----                 --              ---        ----       ----
  prod-web             123.45.67.89    ok         ok         0.45
  staging-web          234.56.78.90    ok         ok         0.12
```

### Info Command

```
  ➞ Server Info: prod-web

  ➞ Configuration
  Name: prod-web
  IP: 123.45.67.89
  User: root
  Provider: hetzner
  Region: fsn1
  Type: cx21
  Environment: prod
  Added: 2026-01-15

  ➞ Live Status
  SSH: connected
  Uptime: up 15 days
  Memory: 1.2GB/4GB
  Disk: 15GB/40GB (38% used)
  Load: 0.45 0.38 0.32
```

---

## Server Configuration

Servers are stored in `.nself/servers/servers.json`:

```json
{
  "servers": [
    {
      "name": "prod-web",
      "ip": "123.45.67.89",
      "provider": "hetzner",
      "region": "fsn1",
      "type": "cx21",
      "user": "root",
      "env": "prod",
      "status": "active",
      "added": "2026-01-15T10:30:00Z"
    }
  ]
}
```

---

## Supported Providers

| Provider | Value |
|----------|-------|
| AWS | `aws` |
| Google Cloud | `gcp` |
| Azure | `azure` |
| DigitalOcean | `digitalocean`, `do` |
| Hetzner | `hetzner` |
| Linode | `linode` |
| Vultr | `vultr` |
| Manual/Other | `manual` |

---

## SSH Access

SSH connections use the configured user and IP. Ensure SSH keys are configured:

```bash
# Test SSH access
nself servers status prod-web

# If SSH fails, add your key
ssh-copy-id root@123.45.67.89
```

---

## Related Commands

- [providers](PROVIDERS.md) - Cloud provider configuration
- [provision](PROVISION.md) - Server provisioning
- [deploy](DEPLOY.md) - Deployment

---

*Last Updated: January 24, 2026 | Version: 0.4.8*
