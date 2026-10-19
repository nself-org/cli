# AWS Graviton (arm64) Install Guide

Install the ɳSelf CLI on AWS Graviton instances (Graviton2 `m6g`, Graviton3 `m7g`, `c7g`, etc.). The ɳSelf CLI ships a native `linux-arm64` binary, no emulation or cross-compilation needed.

## Supported Instances

Any Graviton2/3/4 instance family works: `m6g`, `m6gd`, `c6g`, `r6g`, `t4g`, `m7g`, `c7g`, `r7g`, etc.

Recommended for a full ɳSelf stack:
- **Development:** `t4g.medium` (2 vCPU, 4 GB RAM)
- **Production:** `m6g.large` or higher (2 vCPU, 8 GB RAM)

## Prerequisites

- Amazon Linux 2023 (AL2023), Ubuntu 22.04/24.04 arm64, or Debian 12 arm64
- Docker Engine 20.10+ (see below for install)
- Docker Compose v2.0+
- At least 4 GB RAM for a comfortable full-stack run

## Step 1: Install Docker (if not already installed)

**Amazon Linux 2023:**

```bash
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
newgrp docker
```

**Ubuntu 22.04/24.04:**

```bash
curl -sSL https://get.docker.com | sh
sudo usermod -aG docker ubuntu
newgrp docker
```

Verify:

```bash
docker --version
docker compose version
```

## Step 2: Download the ɳSelf CLI

```bash
curl -L https://github.com/nself-org/cli/releases/download/v1.1.1/nself-linux-arm64.tar.gz -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself-linux-arm64 /usr/local/bin/nself
sudo chmod +x /usr/local/bin/nself
rm nself.tar.gz
```

## Step 3: Verify Installation

```bash
nself version
# nself vX.Y.Z

file $(which nself)
# /usr/local/bin/nself: ELF 64-bit LSB executable, ARM aarch64

nself doctor
```

## Step 4: Start a Project

```bash
mkdir my-app && cd my-app
nself init
nself build
nself start
```

## Graviton-Specific Notes

**glibc vs musl:** The ɳSelf release binary is dynamically linked against glibc (standard for AL2023/Ubuntu/Debian). It does not run on Alpine Linux (musl). If you need Alpine compatibility, build from source with `CGO_ENABLED=0`.

**Docker image compatibility:** ɳSelf uses official multi-arch Docker images for Postgres, Hasura, Redis, etc. All have arm64 variants, no custom image builds needed.

**EBS IOPS:** For production Postgres on Graviton, use `gp3` EBS with at least 3000 IOPS. Default `gp2` can bottleneck on write-heavy workloads.

**Security groups:** ɳSelf binds all services to `127.0.0.1` internally. Expose ports via Nginx (80/443). Open port 22 (SSH) and 80/443 only to public; leave internal ports (5432, 8080, 4000, 3021) closed externally.

**ARM spot instances:** `t4g.medium` spot instances are very cost-effective for development/staging. Use on-demand for production (`m6g.large` or `m6g.xlarge`).

## PATH Setup

Add to `~/.bashrc` or `~/.profile`:

```bash
export PATH="/usr/local/bin:$PATH"
```

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `exec format error` | Verify the binary: `file /usr/local/bin/nself` must show `ARM aarch64` |
| Docker daemon not running | `sudo systemctl start docker` |
| Permission denied on Docker socket | `sudo usermod -aG docker $USER && newgrp docker` |
| `nself doctor` SSL warning | Open ports 80 + 443 in your EC2 security group; run `nself ssl setup` |

---

**Related:** [[Installation]] | [[install-raspberry-pi|Raspberry Pi]] | [[macos-apple-silicon|macOS Apple Silicon]] | [[Home]]
