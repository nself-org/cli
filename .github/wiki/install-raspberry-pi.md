# Raspberry Pi Install Guide

Install the ɳSelf CLI on Raspberry Pi (armv7 32-bit or arm64 64-bit). Tested on Raspberry Pi 4B and Pi 5.

## Prerequisites

- Raspberry Pi OS 64-bit (recommended) or Raspberry Pi OS 32-bit
- Docker Engine 20.10+ installed ([official Docker install guide for Raspberry Pi](https://docs.docker.com/engine/install/debian/))
- Docker Compose v2.0+
- At least 2 GB RAM (Pi 4B 4GB or 8GB recommended for a full stack)
- 8 GB+ storage on the SD card or SSD

## Step 1: Install Docker

```bash
curl -sSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

Verify:

```bash
docker --version
docker compose version
```

## Step 2: Download the ɳSelf CLI Binary

**64-bit (arm64, Raspberry Pi OS 64-bit, Pi 4/5):**

```bash
curl -L https://github.com/nself-org/cli/releases/download/v1.1.1/nself-linux-arm64.tar.gz -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself-linux-arm64 /usr/local/bin/nself
sudo chmod +x /usr/local/bin/nself
```

**32-bit (armv7, Raspberry Pi OS 32-bit):**

```bash
curl -L https://github.com/nself-org/cli/releases/download/v1.1.1/nself-linux-arm.tar.gz -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself-linux-arm /usr/local/bin/nself
sudo chmod +x /usr/local/bin/nself
```

## Step 3: Verify Installation

```bash
nself version
# nself vX.Y.Z

nself doctor
```

## Step 4: Start a Project

```bash
mkdir my-project && cd my-project
nself init
nself build
nself start
```

## Raspberry Pi-Specific Notes

**Memory limits:** A full ɳSelf stack (Postgres + Hasura + Auth + Nginx) uses approximately 600-900 MB RAM at idle. A Pi 4B 2GB can run the stack but leaves little headroom. Recommended: 4GB or 8GB model.

**SD card speed:** SD cards are slow for database I/O. For production-style workloads, boot from USB SSD for better Postgres performance.

**Avoid swap on SD:** Heavy swap on an SD card degrades quickly. Add `vm.swappiness=10` to `/etc/sysctl.conf` to reduce swap usage.

**Temperature:** Under sustained Docker load, Pi can throttle at high temperatures. Use a heatsink and fan for server use.

**Not supported:** Monitoring bundle (10 services) is not recommended on Pi due to memory constraints. Install only what your project needs.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `exec format error` | Wrong binary architecture , use arm64 for 64-bit OS, armv7 for 32-bit OS |
| Docker pull fails | Ensure Docker has arm64/armv7 multi-arch support; run `docker buildx ls` |
| Stack OOM killed | Reduce memory: `nself init --profile tiny` (Postgres + nginx only) and add only the services your project needs |
| Slow startup | Normal on first run , images need pulling. Subsequent starts are fast. |

## Community Verified

This guide is community-verified for Raspberry Pi 4B and Pi 5 running Raspberry Pi OS 64-bit (Debian Bookworm base). Report issues at [github.com/nself-org/cli/issues](https://github.com/nself-org/cli/issues).

---

**Related:** [[Installation]] | [[install-aws-graviton|AWS Graviton]] | [[macos-apple-silicon|macOS Apple Silicon]] | [[Home]]
