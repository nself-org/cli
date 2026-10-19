# Linux x86_64 (amd64)

Install the ɳSelf CLI on 64-bit Linux (linux/amd64). Tested on Ubuntu 22.04/24.04,
Debian 12, Fedora 39/40, Rocky Linux 9, and Amazon Linux 2023.

## Prerequisites

| Requirement | Minimum version | Notes |
|---|---|---|
| Linux kernel | 5.4+ | Needed for Docker cgroups v2 |
| Docker Engine | 24.0 | See install steps below |
| Docker Compose | v2.20+ | Bundled with Docker Engine v24+ |
| curl + bash | Any | For the installer script |

### Install Docker (if not already installed)

**Ubuntu / Debian:**

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
```

**Fedora / RHEL / Rocky:**

```bash
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
newgrp docker
```

Verify:

```bash
docker --version        # Docker version 24+
docker compose version  # Docker Compose version v2+
```

## Install ɳSelf CLI

### Option 1: Installer script (recommended)

```bash
curl -sSL https://install.nself.org | bash
```

The script detects `x86_64` / `amd64`, downloads `nself-linux-amd64.tar.gz` from
the latest release, extracts it to `/usr/local/bin/nself`, and verifies the install.

### Option 2: Manual binary download

```bash
VERSION=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/nself-linux-amd64.tar.gz" -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself /usr/local/bin/nself
chmod +x /usr/local/bin/nself
```

Verify the downloaded binary:

```bash
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/checksums.txt" -o checksums.txt
grep "nself-linux-amd64" checksums.txt | sha256sum --check
```

## Verify Installation

```bash
nself version
# nself vX.Y.Z (linux/amd64)

nself doctor
```

Expected:

```
✓ Docker: running (v26.1.0)
✓ Docker Compose: v2.27.0
✓ curl: available
✓ bash: available
✓ Architecture: amd64
```

## Plugin Docker Requirements

All plugin containers run natively as `linux/amd64`. No extra configuration needed.

## Common Pitfalls

**`permission denied` when running docker**

Add your user to the docker group:

```bash
sudo usermod -aG docker $USER
newgrp docker   # or log out and back in
```

**`nself doctor` fails: Docker Compose version < v2**

Uninstall `docker-compose` (v1 standalone) and install the v2 plugin:

```bash
sudo apt-get remove docker-compose       # Debian/Ubuntu
sudo apt-get install docker-compose-plugin
```

**Firewall blocks inter-container networking**

ɳSelf services communicate over Docker bridge networks. If containers cannot reach each
other, check `iptables` or `nftables` rules. Docker typically manages bridge rules
automatically, ensure `iptables-legacy` or `nftables` compatibility is configured.

**SELinux (Fedora / RHEL / Rocky) blocks volume mounts**

```bash
sudo setsebool -P container_manage_cgroup on
```

Or use `:z` volume labels in your docker-compose file for SELinux relabeling.

---
← [[install/macos-intel]] | [[install/linux-arm64]] →
