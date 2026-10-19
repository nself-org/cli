# macOS Apple Silicon (M1/M2/M3/M4)

Install the ɳSelf CLI on macOS with an Apple Silicon processor (darwin/arm64).
Tested on M1, M2, M3, and M4 chips.

> The ɳSelf CI suite runs a smoke test on `macos-14` runners for every release,
> verifying `nself version`, `nself doctor`, `nself init`, and `nself build` pass
> natively on Apple Silicon before any binary ships.

## Prerequisites

| Requirement | Minimum version | Notes |
|---|---|---|
| macOS | 13 Ventura | macOS 14 Sonoma recommended |
| Docker | 24.0 | Use OrbStack or Docker Desktop (see below) |
| Homebrew | 4.0+ | For the recommended install path |
| Go | 1.24+ | Only if building from source |

### OrbStack vs Docker Desktop

Both work. OrbStack is recommended on Apple Silicon:

| | OrbStack | Docker Desktop |
|---|---|---|
| Performance | Faster (VirtioFS, native) | Acceptable |
| Memory | Lower (~200 MB idle) | Higher (~600 MB idle) |
| VirtioFS | Default | Requires enabling in settings |
| Price | Free for personal use | Free with limits |

**Enable VirtioFS in Docker Desktop** (if you use Docker Desktop):
`Docker Desktop → Settings → General → VirtioFS`, this halves bind-mount latency on Apple Silicon.

Do NOT use `osxfs`, it causes significant I/O slowdowns for Postgres data volumes.

## Install via Homebrew (recommended)

```bash
brew install nself-org/nself/nself
```

Homebrew automatically selects the `darwin/arm64` bottle. No Rosetta translation needed.

Verify the binary is native:

```bash
file $(which nself)
# nself: Mach-O 64-bit executable arm64
```

## Install via curl (binary download)

```bash
curl -sSL https://install.nself.org | bash
```

The installer detects `arm64` automatically and downloads `nself-darwin-arm64.tar.gz`.

Manual download:

```bash
VERSION=$(curl -s https://api.github.com/repos/nself-org/cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
curl -L "https://github.com/nself-org/cli/releases/download/${VERSION}/nself-darwin-arm64.tar.gz" -o nself.tar.gz
tar -xzf nself.tar.gz
sudo mv nself /usr/local/bin/nself
chmod +x /usr/local/bin/nself
```

## Verify Installation

```bash
nself version
# nself vX.Y.Z (darwin/arm64)

nself doctor
# All checks should show green
```

## Plugin Docker Requirements

Plugins run as Docker containers. On Apple Silicon:

- All ɳSelf plugin images are multi-arch (`linux/amd64 + linux/arm64`).
 No Rosetta translation is used for plugin containers.
- OrbStack and Docker Desktop both use QEMU or native arm64 layer, either works.
- Plugin images use `linux/arm64` natively when available.

If you see `WARNING: The requested image's platform (linux/amd64) does not match
your machine's platform (linux/arm64)`, that plugin does not yet have an arm64 image.
File a bug at `github.com/nself-org/plugins`.

## Common Pitfalls

**`nself doctor` reports Docker not running**

Start OrbStack from Applications, or click the Docker Desktop whale in your menu bar.

**Homebrew installs the amd64 binary under Rosetta**

This happens if Homebrew itself is running under Rosetta (x86 terminal). Open Terminal
natively (not via Rosetta) or install the ARM Homebrew:

```bash
arch -arm64 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

**`nself start` is slow on first run**

Docker pulls are slower on the first run. Subsequent starts use cached layers and
are significantly faster.

**Port conflicts on localhost**

`nself doctor` will detect port conflicts. Common culprits: Postgres on 5432, Redis on 6379.
Stop local services before `nself start`.

---
← [[Installation]] | [[install/macos-intel]] →
