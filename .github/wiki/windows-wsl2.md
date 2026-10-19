# Windows: WSL2 Install Guide

ɳSelf CLI requires WSL2 on Windows. The CLI depends on Docker socket activation, POSIX path semantics, and systemd-style service management, none of which are natively available on Win32.

## Why WSL2 is Required

- **Docker socket:** ɳSelf drives Docker via `/var/run/docker.sock`. Docker Desktop exposes this socket inside WSL2 only.
- **POSIX paths:** `nself build` generates `docker-compose.yml` with Linux-style volume mounts. Windows paths break compose volume resolution.
- **Linuxbrew:** the official `brew tap nself-org/nself` distribution requires Linuxbrew (runs inside WSL2 Ubuntu).

## Step 1: Windows Version

Requires Windows 10 22H2 (build 19045) or Windows 11. Check: `winver` in Run dialog.

Enable WSL from an elevated PowerShell:

```powershell
wsl --install
```

Reboot when prompted.

## Step 2: Install Ubuntu 22.04

```powershell
wsl --install -d Ubuntu-22.04
```

Or install **Ubuntu 22.04 long-term support** from the Microsoft Store. Launch it once to complete the initial user setup (username + password).

## Step 3: Install Docker Inside WSL2

**Option A (recommended): Docker Desktop with WSL2 integration**

1. Download [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop/).
2. During install, enable "Use WSL 2 based engine".
3. After install, open Docker Desktop > Settings > Resources > WSL Integration > toggle on **Ubuntu-22.04**.
4. Apply and restart.

**Option B: Docker Engine directly in WSL2**

Inside the Ubuntu 22.04 terminal:

```bash
sudo apt update && sudo apt install -y ca-certificates curl gnupg
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list
sudo apt update && sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo usermod -aG docker $USER
newgrp docker
```

## Step 4: Install ɳSelf via Linuxbrew

Inside the Ubuntu 22.04 terminal:

```bash
# Install Linuxbrew if not present
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> ~/.bashrc
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"

# Tap and install
brew tap nself-org/nself
brew install nself
```

## Step 5: Verify Installation

```bash
nself version
```

Expected output:

```
nself vX.Y.Z (linux/amd64)
```

## Step 6: End-to-End Smoke Test

```bash
nself init my-project
cd my-project
nself start
```

ɳSelf boots Postgres, Hasura, Auth, and Nginx. When all services show `healthy`, the stack is ready.

## Troubleshooting

**Docker not found inside WSL2**

If using Docker Desktop, verify WSL2 integration is toggled on for Ubuntu-22.04 (Settings > Resources > WSL Integration). Then restart your WSL2 terminal.

**Network: cannot reach `localhost` from Windows browser**

WSL2 uses a virtual network adapter. Access ɳSelf services via the WSL2 IP:

```bash
ip addr show eth0 | grep 'inet '
```

Use that IP in your Windows browser, or add a Windows hosts entry for convenience.

**Memory limits: WSL2 consuming too much RAM**

Create `%UserProfile%\.wslconfig` on the Windows side:

```ini
[wsl2]
memory=4GB
processors=2
swap=2GB
```

Restart WSL2 (`wsl --shutdown`) for changes to take effect.

**`brew` command not found after install**

Linuxbrew writes to `~/.bashrc`. If you use `~/.zshrc`, add the `shellenv` eval there too:

```bash
echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> ~/.zshrc
source ~/.zshrc
```

---

← [[Installation]] | [[Quick-Start]] →
