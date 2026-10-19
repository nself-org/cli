# Linux arm64 (aarch64)

Install the ɳSelf CLI on 64-bit ARM Linux (linux/arm64). Tested on AWS Graviton2/3,
Ampere Altra (Hetzner CAX), Raspberry Pi 4/5 (64-bit OS), Oracle Ampere, and
Scaleway EM-A210R-HDD.

> The ɳSelf CI suite runs build + unit tests on `ubuntu-24.04-arm` runners for
> every release, verifying the arm64 binary before it ships.

## Prerequisites

| Requirement | Minimum version | Notes |
|---|---|---|
| Linux kernel | 5.4+ | Required for Docker cgroups v2 |
| Docker Engine | 24.0 | See install steps |
| Docker Compose | v2.20+ | Bundled with Docker Engine v24+ |
| CPU | 64-bit ARM (aarch64) | 32-bit ARM (armv7) is NOT supported |
| RAM | 2 GB minimum | 4 GB+ recommended for a full ɳSelf stack |

### Verify your CPU supports 64-bit

```bash
uname -m
# aarch64   ← supported
# armv7l    ← NOT supported — use Raspberry Pi OS 64-bit instead
```

### Install Docker

**Ubuntu / Debian (arm64):**

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

**Amazon Linux 2023 (Graviton):**

```bash
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

Verify:

```bash
docker --version        # Docker version 24+
docker compose version  # Docker Compose version v2+
```

## Install ɳSelf CLI

### Option 1: Installer script

```bash
curl -sSL https://install.nself.org | bash
```

The script detects `aarch64` / `arm64` and downloads `nself-linux-arm64.tar.gz`.

### Option 2: Manual binary download

```bash
VERSION=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/nself-linux-arm64.tar.gz" -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself /usr/local/bin/nself
chmod +x /usr/local/bin/nself
```

Verify the checksum:

```bash
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/checksums.txt" -o checksums.txt
grep "nself-linux-arm64" checksums.txt | sha256sum --check
```

## Verify Installation

```bash
nself version
# nself vX.Y.Z (linux/arm64)

nself doctor
```

## Plugin Docker Requirements

All official ɳSelf plugin images ship with `linux/amd64 + linux/arm64` manifests.
Docker automatically pulls the `linux/arm64` variant on arm64 hosts, no flags needed.

If any plugin image produces a platform mismatch warning:

```
WARNING: The requested image's platform (linux/amd64) does not match the detected
host platform (linux/arm64) and no specific platform was requested
```

That plugin does not yet have an arm64 layer. File a bug at `github.com/nself-org/plugins`.

## Common Pitfalls

**`nself doctor` fails: Docker not running**

```bash
sudo systemctl start docker
sudo systemctl enable docker
```

**`nself start` is slow on first pull**

First-time image pulls on arm64 servers are typically faster than on developer laptops
because cloud servers have better upstream bandwidth. Budget 1–3 minutes on a typical
VPS on first run.

**Hetzner CAX / Ampere Altra: cgroups v1 warning**

Newer Docker versions default to cgroups v2. If you see warnings about cgroup version:

```bash
# Check cgroup version
stat -fc %T /sys/fs/cgroup
# tmpfs = cgroups v1, cgroup2fs = cgroups v2
```

cgroups v2 is fully supported and preferred. No action needed on Ubuntu 22.04+.

**AWS Graviton: IMDSv2 required**

If your instance uses IMDSv2, Docker Hub pulls work normally. ɳSelf does not call
the EC2 metadata service directly.

---
← [[install/linux-x86_64]] | [[install/windows-wsl2]] →
