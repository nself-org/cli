# nself deploy provision

Provision cloud infrastructure for your nself project directly from the CLI.

## Usage

```bash
nself deploy provision <provider> [options]
```

## Description

Creates a new cloud server with the specified provider. Supports all major cloud providers with a unified interface. Server details are saved locally for use with `nself deploy` commands.

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--name <name>` | Server name | `PROJECT_NAME-server` |
| `--size <size>` | Instance size: `tiny`, `small`, `medium`, `large`, `xlarge` | `small` |
| `--region <region>` | Deployment region | Provider default |
| `--token <token>` | API token (overrides env var / provider context) | From env |
| `--ssh-key <name>` | SSH key name to attach | None |
| `--dry-run` | Preview provisioning plan without executing | |
| `--estimate` | Show cost estimate only | |
| `--sizes` | List available sizes for provider | |
| `--regions` | List available regions for provider | |

## Supported Providers

| Provider | Category | CLI Tool |
|----------|----------|----------|
| `aws` | Major Cloud | `aws` |
| `gcp` | Major Cloud | `gcloud` |
| `azure` | Major Cloud | `az` |
| `digitalocean` | Developer Cloud | `doctl` |
| `hetzner` | Budget EU | `hcloud` |
| `linode` | Developer Cloud | `linode-cli` |
| `vultr` | Developer Cloud | `vultr-cli` |
| `ionos` | Budget EU | `ionosctl` |
| `ovh` | Budget EU | `ovh` |
| `scaleway` | Budget EU | `scw` |

Run `nself deploy provision` (no provider) to see the full list.

## Size Tiers

| Size | vCPU | RAM | Disk |
|------|------|-----|------|
| `tiny` | 1 | 512 MB | 10 GB |
| `small` | 1 | 1 GB | 25 GB |
| `medium` | 2 | 4 GB | 50 GB |
| `large` | 4 | 8 GB | 100 GB |
| `xlarge` | 8 | 16 GB | 200 GB |

Sizes are normalized across providers — each provider maps these to their closest equivalent.

## Token Resolution

The `--token` flag takes priority. Without it, tokens are resolved in this order:

1. **Explicit flag**: `--token <value>`
2. **Project-specific env var**: `HETZNER_<PROJECT_NAME>_TOKEN` (e.g., `HETZNER_CLAWDE_TOKEN` for `PROJECT_NAME=clawde`)
3. **Provider env var**: `HCLOUD_TOKEN`, `DIGITALOCEAN_ACCESS_TOKEN`, etc.
4. **Provider CLI context**: Previously configured via `nself infra provider init <provider>`

This allows multiple projects to use the same provider with different accounts.

## Examples

```bash
# Provision a Hetzner server with defaults
nself deploy provision hetzner

# Provision with a specific size and region
nself deploy provision hetzner --size medium --region fsn1

# Use a project-specific token
nself deploy provision hetzner --token $HETZNER_CLAWDE_TOKEN --name clawde-api

# Preview what would be created
nself deploy provision hetzner --size large --dry-run

# Check cost estimate
nself deploy provision hetzner --size medium --estimate

# List available sizes
nself deploy provision hetzner --sizes

# List available regions
nself deploy provision hetzner --regions
```

## Post-Provisioning Workflow

After provisioning, follow these steps:

```bash
# 1. Point your domain DNS to the server IP
#    (shown in provisioning output)

# 2. Initialize the server
nself deploy server init root@<server-ip> --domain your-domain.com

# 3. Deploy your project
nself deploy staging
# or
nself deploy production
```

## Related

- [Deploy Overview](/commands/deploy/README.md)
- [Server Management](/commands/deploy/server.md)
- [Cloud Providers Guide](/deployment/CLOUD-PROVIDERS.md)
- [Deployment Guide](/guides/DEPLOYMENT-ARCHITECTURE.md)
