# macOS Apple Silicon (arm64) Install Guide

Install the ɳSelf CLI natively on Apple M-series Macs (M1, M2, M3, M4). No Rosetta required.

## Contents

- [Why arm64-native matters](#why-arm64-native-matters)
- [Prerequisites](#prerequisites)
- [Step 1: Add the Homebrew tap](#step-1-add-the-homebrew-tap)
- [Step 2: Install nself](#step-2-install-nself)
- [Step 3: Verify architecture](#step-3-verify-architecture)
- [Step 4: Verify version](#step-4-verify-version)
- [Step 5: Run your first project](#step-5-run-your-first-project)
- [Troubleshooting](#troubleshooting)

---

## Why arm64-native matters

The ɳSelf CLI ships a dedicated `darwin-arm64` binary in every release. Running the arm64 build on Apple Silicon gives you:

- Full use of the M-series performance and efficiency cores with no translation overhead
- Correct `file` output: `Mach-O 64-bit executable arm64` (not `x86_64`)
- Compatibility with Docker Desktop for Apple Silicon containers
- Homebrew's `brew install` selects the arm64 bottle automatically on M-series hardware

If you previously installed an `x86_64` build under Rosetta, uninstall it first (`brew uninstall nself`) and follow this guide to get the native binary.

---

## Prerequisites

- **macOS 11 Big Sur or later** (macOS 12 Monterey or later recommended)
- **Apple M-series chip** (M1, M2, M3, or M4, any variant)
- **Homebrew** installed for Apple Silicon (prefix `/opt/homebrew`). If you are unsure, run:

  ```bash
  brew --prefix
  ```

 The output must be `/opt/homebrew`. If it is `/usr/local`, your Homebrew is running under Rosetta. Reinstall Homebrew natively before continuing.

- **Docker Desktop for Apple Silicon** 4.x or later, with the Apple Silicon build selected at installation. Download from [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop/).

 After installing Docker Desktop, confirm it is running and that the engine is reachable:

  ```bash
  docker info --format '{{.Architecture}}'
  ```

 Expected output: `aarch64`

---

## Step 1: Add the Homebrew tap

```bash
brew tap nself-org/nself
```

This registers the official ɳSelf tap. You only need to do this once per machine.

---

## Step 2: Install nself

```bash
brew install nself
```

Homebrew downloads the `darwin-arm64` bottle, verifies the sha256 checksum, and places the binary on your `PATH`. No manual download or architecture selection is needed.

---

## Step 3: Verify architecture

Confirm the installed binary is the native arm64 build:

```bash
file $(which nself)
```

Expected output:

```
/opt/homebrew/bin/nself: Mach-O 64-bit executable arm64
```

If the output shows `x86_64` instead of `arm64`, your Homebrew is running under Rosetta. Follow the Homebrew Apple Silicon reinstall guide, then repeat Steps 1 and 2.

---

## Step 4: Verify version

```bash
nself version
```

Expected output:

```
nself version X.Y.Z
```

---

## Step 5: Run your first project

```bash
nself init my-project
cd my-project
nself start
```

`nself init` creates the project directory and `.env.dev`. `nself start` boots the full stack (Postgres, Hasura, Auth, Nginx) using Docker Desktop. The first run pulls container images and may take a few minutes depending on your connection.

Once started, `nself urls` shows the local endpoints for your project.

---

## Troubleshooting

### Docker Desktop memory and CPU for M-series

Docker Desktop on Apple Silicon shares the macOS unified memory pool. The default allocation may be too low for a full ɳSelf stack.

Recommended settings (Docker Desktop Preferences > Resources):

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU cores | 2 | 4 |
| Memory | 4 GB | 8 GB |
| Disk image size | 20 GB | 60 GB |

After changing these settings, click "Apply and restart" in Docker Desktop.

### sha256 validation error from Homebrew

If `brew install nself` fails with a checksum mismatch, your local Homebrew formula cache may be stale. Refresh it:

```bash
brew update
brew install nself
```

If the error persists, check [github.com/nself-org/homebrew-nself](https://github.com/nself-org/homebrew-nself) for any open issues on the current release.

### Gatekeeper quarantine on first run

macOS Gatekeeper may block the binary the first time you run it if Homebrew's bottle was not notarized. To clear the quarantine attribute:

```bash
xattr -d com.apple.quarantine $(which nself)
```

Then re-run `nself version` to confirm the binary executes.

### `nself start` hangs or times out

If containers fail to start, check Docker Desktop is running and the engine is responsive:

```bash
docker ps
```

If Docker is running but `nself start` still hangs, increase the memory allocation (see above) and retry. Use `nself logs` to inspect service output for a specific error.

---

## Related

- [[Installation]], all platforms overview
- [[windows-wsl2]], Windows WSL2 install guide
- [[Quick-Start]], first project walkthrough
- [[FAQ]], common setup questions
- [[Home]]
