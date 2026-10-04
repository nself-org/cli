# Installation

> **Upgrading from v0.9?** See the [[Upgrade-From-v0.9]] guide for step-by-step migration instructions.

## OS Support

| Platform | Architecture | Status | Guide |
|---|---|---|---|
| macOS | Apple Silicon (arm64) | Supported | [macOS Apple Silicon](install/macos-apple-silicon) |
| macOS | Intel (amd64) | Supported | [macOS Intel](install/macos-intel) |
| Linux | x86_64 (amd64) | Supported | [Linux x86_64](install/linux-x86_64) |
| Linux | arm64 (aarch64) | Supported | [Linux arm64](install/linux-arm64) |
| Windows | WSL2 | Supported | [Windows WSL2](install/windows-wsl2) |
| Windows | Native amd64 | Supported v1.1.0+ | [Windows Native](install/windows-native) |
| Windows | Native arm64 | Supported v1.1.0+ | [Windows Native](install/windows-native) |
| Raspberry Pi | arm64 | Supported | [Raspberry Pi](install/raspberry-pi) |

**Windows native binaries** (`windows-amd64.zip`, `windows-arm64.zip`) ship from v1.1.0 onward.
Download from [GitHub Releases](https://github.com/nself-org/cli/releases/latest) or use the
PowerShell installer. WSL2 remains the simplest path if Docker Desktop is already running.
See [[windows-wsl2]] for native install and [[install/windows-wsl2]] for WSL2.

## Platform Guides

| Platform | Architecture | Description |
|---|---|---|
| [macOS Apple Silicon](install/macos-apple-silicon) | darwin/arm64 | M1/M2/M3/M4 , OrbStack vs Docker Desktop, VirtioFS |
| [macOS Intel](install/macos-intel) | darwin/amd64 | Intel Mac , Homebrew, binary download |
| [Linux x86_64](install/linux-x86_64) | linux/amd64 | Ubuntu, Debian, Fedora, Rocky, Amazon Linux |
| [Linux arm64](install/linux-arm64) | linux/arm64 | Graviton, Ampere Altra, Hetzner CAX, Pi |
| [Windows WSL2](install/windows-wsl2) | linux/amd64 | WSL2 + Docker Desktop |
| [Windows Native](install/windows-native) | windows/amd64, windows/arm64 | Native `.exe` via PowerShell installer or manual download |
| [Raspberry Pi](install/raspberry-pi) | linux/arm64 | Pi 4/5; RAM budgeting; minimal preset |

## Prerequisites

- **Docker** 24 or later
- **Docker Compose** v2 (included with Docker Desktop)
- **curl** and **bash** (for Linux/macOS installer)

## Quick Install

### macOS

```bash
brew install nself-org/nself/nself
```

See [[install/macos-apple-silicon]] or [[install/macos-intel]] for platform-specific notes.

### Linux

```bash
curl -sSL https://install.nself.org | bash
```

The script detects your architecture (amd64 or arm64) and downloads the correct binary.
See [[install/linux-x86_64]] or [[install/linux-arm64]] for manual install and verification.

### Windows (Native, v1.1.0+)

Download `nself-<version>-windows-amd64.zip` from [GitHub Releases](https://github.com/nself-org/cli/releases/latest),
extract, and add the directory to your `PATH`. Or use the PowerShell installer:

```powershell
irm https://install.nself.org/install.ps1 | iex
```

See [[windows-wsl2]] for the full guide including SHA-256 verification and PATH setup.

### Windows (WSL2)

WSL2 + Docker Desktop is the simplest Windows path. See [[install/windows-wsl2]] for the
full step-by-step guide covering WSL2 setup, Docker integration, and troubleshooting.

### Raspberry Pi

See [[install/raspberry-pi]] for RAM budgeting, SSD recommendations, and the minimal
services preset.

## Manual Install

1. Download the binary for your platform from [GitHub Releases](https://github.com/nself-org/cli/releases/latest).
2. Make it executable and place it on your `PATH`:

```bash
chmod +x nself-linux-amd64
sudo mv nself-linux-amd64 /usr/local/bin/nself
```

## Docker image

nSelf also ships as a multi-arch (linux/amd64, linux/arm64) image on Docker Hub:

```bash
docker pull nself/nself:<version>   # a specific release, e.g. the version shown by `nself version`
docker pull nself/nself:latest      # GitHub's latest non-prerelease release
```

`.github/workflows/docker-publish.yml` builds the image after each successful Release run, so
an image exists only for a tag whose release succeeded. `:latest` never points at a prerelease.

## Verify Your Install

```bash
nself version
```

Expected output:

```
nself v1.0.9 (linux/amd64)
```

```bash
nself doctor
```

Expected output (all green):

```
✓ Docker: running (v26.1.0)
✓ Docker Compose: v2.27.0
✓ curl: available
✓ bash: available
✓ Architecture: amd64
```

If `nself doctor` reports a problem, see [[Errors]] for remediation steps.

## Uninstall

See [[install/uninstall]] for full per-OS instructions including Docker cleanup steps.

**Quick uninstall:**

```bash
# macOS (Homebrew)
brew uninstall nself

# Linux / manual
sudo rm /usr/local/bin/nself
rm -rf ~/.nself
```

This removes the CLI binary and local state. Your project `.env` files and Docker volumes
are preserved.

---
← [[Home]] | [[Quick-Start]] →
