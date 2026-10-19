# Raspberry Pi

Install the ɳSelf CLI on Raspberry Pi. Tested on Raspberry Pi 4B and Pi 5 running
Raspberry Pi OS 64-bit (Debian Bookworm).

## Prerequisites

| Requirement | Minimum | Notes |
|---|---|---|
| Raspberry Pi | Pi 4B or Pi 5 | Pi 3B+ is too slow for the full stack |
| OS | Raspberry Pi OS 64-bit | 32-bit OS is NOT recommended (limited RAM + architecture) |
| Docker Engine | 24.0+ | See install steps below |
| Docker Compose | v2.20+ | Bundled with Docker Engine v24+ |
| RAM | 2 GB minimum | 4 GB recommended; 8 GB for full stack with monitoring |
| Storage | 8 GB+ free | SSD strongly recommended over SD card for database I/O |

### RAM budgeting

Full ɳSelf stack at idle:

| Service set | RAM usage |
|---|---|
| Minimal (Postgres + Hasura + Nginx) | ~350–500 MB |
| Standard (+ Auth + Redis) | ~550–750 MB |
| Full (+ Mail + Search + MinIO) | ~900 MB–1.2 GB |
| Full + Monitoring bundle (10 services) | ~1.8–2.5 GB |

Use the **minimal services preset** on Pi 4B 2GB or 4GB:

```bash
nself init --preset minimal
```

Do not install the monitoring bundle on Pi, it adds 10 services and exceeds the 2 GB model's capacity.

## Step 1: Install Docker

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

Verify:

```bash
docker --version        # 24+
docker compose version  # v2+
```

## Step 2: Install ɳSelf CLI

```bash
curl -sSL https://install.nself.org | bash
```

The installer detects `aarch64` automatically and downloads `nself-linux-arm64.tar.gz`.

Manual download (64-bit):

```bash
VERSION=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/nself-linux-arm64.tar.gz" -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself /usr/local/bin/nself
chmod +x /usr/local/bin/nself
```

## Step 3: Verify Installation

```bash
nself version
# nself vX.Y.Z (linux/arm64)

nself doctor
```

Expected (all green):

```
✓ Docker: running (v24.x.x)
✓ Docker Compose: v2.x.x
✓ curl: available
✓ bash: available
✓ Architecture: arm64
```

## Step 4: Start Your First Project

```bash
mkdir my-project && cd my-project
nself init --preset minimal   # minimal preset for Pi
nself build
nself start
```

## Raspberry Pi-Specific Notes

### Storage: Use SSD, not SD Card

Postgres writes are I/O intensive. SD cards degrade quickly under database write load
and are 3–10x slower than SATA SSD. Boot from USB SSD for any persistent project.

```bash
# Check storage type
lsblk -d -o NAME,ROTA   # ROTA=1 = rotational/slow; ROTA=0 = SSD
```

### Reduce swap on SD card

If you must use an SD card, minimize swap writes:

```bash
echo 'vm.swappiness=10' | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

### Temperature throttling

Under sustained Docker load, Pi can thermal-throttle. Use a heatsink + fan for
server deployments. Check throttling:

```bash
vcgencmd get_throttled
# 0x0 = no throttling; non-zero = thermal or voltage issue
```

### Plugin Docker requirements

All ɳSelf plugin images ship with `linux/arm64` manifests. Docker pulls the arm64
variant automatically, no flags needed.

### Not supported on Pi

- Monitoring bundle (10 extra services), exceeds 2 GB and 4 GB models' comfortable RAM
- armv7 (32-bit) OS, use Raspberry Pi OS 64-bit (Pi 4/5 only)

## Common Pitfalls

| Problem | Fix |
|---|---|
| `exec format error` | Downloaded the amd64 binary , re-download `nself-linux-arm64.tar.gz` |
| Stack OOM | Use `nself init --preset minimal`; skip monitoring bundle |
| Slow first `nself start` | Normal , first pull caches images. Subsequent starts: 10–30 seconds |
| SD card I/O slow | Move the project and Docker data root to an SSD |
| Docker pull fails | Ensure Pi is connected to internet; try `docker pull hello-world` as a test |

---
← [[install/windows-wsl2]] | [[Installation]] →
