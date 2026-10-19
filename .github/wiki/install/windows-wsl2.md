# Windows (WSL2)

> **Native Windows binary is NOT supported in v1.0.9.**
> WSL2 + Docker Desktop is the supported path for Windows users.
> Native Windows binary (`windows-amd64.zip` + `install.ps1`) is planned for v1.1.0.
> See the [OS support matrix](../Installation.md#os-support) for details.

This guide covers installing ɳSelf CLI on Windows using WSL2 (Windows Subsystem for Linux).
Tested on Windows 10 (22H2+) and Windows 11.

## Prerequisites

| Requirement | Minimum | Notes |
|---|---|---|
| Windows | 10 22H2 or Windows 11 | WSL2 requires these builds |
| WSL2 | 2.x | Run `wsl --version` to check |
| Docker Desktop | 4.25+ | Must enable WSL2 backend |
| Virtualization | Enabled in BIOS/UEFI | Required for Hyper-V / WSL2 |
| RAM | 8 GB+ | 4 GB minimum; WSL2 uses ~1–2 GB |

## Step 1: Enable Virtualization in BIOS/UEFI

WSL2 requires CPU virtualization. Before installing, verify it is enabled:

1. Open Task Manager → Performance → CPU
2. Check that "Virtualization: Enabled" appears in the lower right

If disabled, reboot into BIOS/UEFI:
- Intel: enable **VT-x** (Intel Virtualization Technology)
- AMD: enable **AMD-V** or **SVM Mode**

The setting location varies by motherboard vendor, search your model + "enable virtualization BIOS".

## Step 2: Install WSL2

Open **PowerShell as Administrator** and run:

```powershell
wsl --install
```

This installs WSL2 and Ubuntu (default distribution) in one command.
Restart your computer when prompted.

After restart, Ubuntu will finish setup and prompt you to create a Linux username and password.

Verify WSL2 is active:

```powershell
wsl --version
# WSL version: 2.x.x
```

If you already have WSL1, upgrade:

```powershell
wsl --set-default-version 2
```

## Step 3: Fix Line Endings

Configure Git inside WSL to use Unix line endings:

```bash
git config --global core.autocrlf input
```

This prevents Windows CRLF line endings from corrupting shell scripts inside WSL.

## Step 4: Install Docker Desktop with WSL2 Backend

1. Download Docker Desktop for Windows from [docs.docker.com/desktop/install/windows-install/](https://docs.docker.com/desktop/install/windows-install/)
2. During install, select **Use WSL2 instead of Hyper-V** (or both)
3. After install: `Docker Desktop → Settings → Resources → WSL Integration → Enable for your distro (Ubuntu)`
4. Apply and restart Docker Desktop

Verify inside WSL:

```bash
docker --version        # Docker version 24+
docker compose version  # Docker Compose version v2+
```

## Step 5: Install ɳSelf CLI Inside WSL

Open the Ubuntu terminal (Start → Ubuntu) and run:

```bash
curl -sSL https://install.nself.org | bash
```

Or manually:

```bash
VERSION=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/nself-linux-amd64.tar.gz" -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself /usr/local/bin/nself
chmod +x /usr/local/bin/nself
```

## Step 6: Verify

```bash
nself version
# nself vX.Y.Z (linux/amd64)

nself doctor
# All checks should show green
```

## PowerShell Completion (optional)

Generate ɳSelf tab-completion for PowerShell (run ɳSelf commands from PowerShell via WSL):

```powershell
nself completion powershell | Out-String | Invoke-Expression
```

To persist across sessions, add to your PowerShell profile:

```powershell
nself completion powershell >> $PROFILE
```

## Common Pitfalls

**`wsl --install` fails with "feature not supported"**

Your Windows version is too old or virtualization is disabled. Update Windows to 22H2+
and enable virtualization in BIOS (Step 1).

**Docker commands fail inside WSL: "cannot connect to Docker daemon"**

Ensure Docker Desktop is running (check the system tray) and WSL Integration is
enabled for your distro in Docker Desktop settings.

**`nself start` reports port already in use**

A Windows service (e.g., PostgreSQL, Redis) may use the same port.
Stop the conflicting service or change the port in your `.env` file.
Run `netstat -aon | findstr :<port>` in PowerShell to find the process.

**Slow file I/O on Windows-mounted paths**

Keep your ɳSelf project inside the WSL filesystem (e.g., `/home/user/projects/`),
NOT on the Windows drive (`/mnt/c/`). Cross-filesystem I/O is significantly slower.

---

## Native Windows, v1.1.0 Planned

Native Windows binary (`windows-amd64.zip` + `install.ps1`) is planned for v1.1.0.

Known requirements before v1.1.0 ships:
- Path-separator audit (all `filepath.Join` calls, no hardcoded `/`)
- Windows code-signing certificate ($300+/yr EV cert for SmartScreen bypass)
- `nself completion powershell` (already implemented, see above)
- Windows CI runner testing (added to CI in v1.0.9 via S49-T01)

Track progress: [github.com/nself-org/cli/issues](https://github.com/nself-org/cli/issues)

---
← [[install/linux-arm64]] | [[install/raspberry-pi]] →
