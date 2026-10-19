# macOS Intel (x86_64)

Install the ɳSelf CLI on macOS with an Intel processor (darwin/amd64).
Tested on macOS 12 Monterey through macOS 14 Sonoma.

## Prerequisites

| Requirement | Minimum version | Notes |
|---|---|---|
| macOS | 12 Monterey | macOS 13/14 recommended |
| Docker | 24.0 | Docker Desktop for Mac (Intel) |
| Homebrew | 4.0+ | For the recommended install path |

### Docker Desktop on Intel Mac

Use the Intel/AMD installer from `docs.docker.com/desktop/install/mac-install/`.
Do NOT install the Apple Silicon build, it requires Rosetta and is slower.

Enable VirtioFS in Docker Desktop settings for better I/O performance:
`Docker Desktop → Settings → General → Use VirtioFS`.

## Install via Homebrew (recommended)

```bash
brew install nself-org/nself/nself
```

Homebrew selects the `darwin/amd64` bottle automatically.

Verify:

```bash
file $(which nself)
# nself: Mach-O 64-bit executable x86_64

nself version
# nself vX.Y.Z (darwin/amd64)
```

## Install via curl

```bash
curl -sSL https://install.nself.org | bash
```

Manual download:

```bash
VERSION=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/nself-darwin-amd64.tar.gz" -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself /usr/local/bin/nself
chmod +x /usr/local/bin/nself
```

## Verify Installation

```bash
nself version
# nself vX.Y.Z (darwin/amd64)

nself doctor
```

Expected output:

```
✓ Docker: running (v26.x.x)
✓ Docker Compose: v2.x.x
✓ curl: available
✓ bash: available
✓ Architecture: amd64
```

## Plugin Docker Requirements

Plugin containers run as `linux/amd64`. Docker Desktop handles the Linux VM
automatically, you do not need to configure anything.

All official ɳSelf plugin images include `linux/amd64` manifests. If `docker pull`
complains about architecture, the plugin has a bug, file a report at
`github.com/nself-org/plugins`.

## Common Pitfalls

**`nself doctor` says Docker is not running**

Click the Docker Desktop whale icon in the menu bar and wait for it to fully start
(the icon stops animating when ready).

**Slow first `nself start`**

First run pulls all plugin images (~2–4 GB total for a standard stack). Subsequent
starts use cached layers. Typical cold start: 3–8 minutes. Warm start: 15–30 seconds.

**macOS security dialog blocks nself binary (Gatekeeper)**

If macOS says the binary cannot be opened because the developer cannot be verified:

```bash
xattr -d com.apple.quarantine $(which nself)
```

Or right-click the binary in Finder → Open → Open.

---
← [[install/macos-apple-silicon]] | [[install/linux-x86_64]] →
