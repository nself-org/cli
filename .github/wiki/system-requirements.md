# System Requirements

This page lists the minimum requirements to install and run the ɳSelf CLI correctly across all supported platforms.

---

## Platform Minimums

| Platform | Minimum Version | Architectures | Notes |
|---|---|---|---|
| **macOS** | 11 (Big Sur) | arm64, x86_64 | Apple Silicon (M1/M2/M3) natively supported , see [macos-apple-silicon.md](macos-apple-silicon.md) |
| **Ubuntu** | 20.04 long-term support | x86_64, arm64 | 22.04 long-term support and 24.04 long-term support also fully supported |
| **Debian** | 11 (Bullseye) | x86_64, arm64 | Bookworm (12) also supported |
| **Fedora** | 36 | x86_64, arm64 | Tested on current stable releases |
| **Arch Linux** | Rolling | x86_64, arm64 | Latest rolling release; no minimum pinned |
| **Windows** | 10 22H2 / 11 | x86_64 | **WSL2 required** , see [windows-wsl2.md](windows-wsl2.md) |

> **Windows note:** Native Windows execution is not supported. All CLI commands must run inside a WSL2 environment. Docker Desktop for Windows with WSL2 backend satisfies the Docker requirement automatically.

---

## Required Dependencies

### Docker

| Dependency | Minimum Version | Notes |
|---|---|---|
| Docker Engine | 24.x | Linux native install |
| Docker Desktop | 4.x | macOS and Windows (WSL2 backend) |
| Docker Compose | v2.20 or newer | Bundled with Docker Desktop; install the standalone `docker compose` plugin on Linux headless installs |

The CLI uses `docker compose` (v2 plugin syntax). The legacy `docker-compose` (v1 standalone binary) is **not** supported.

Compose 2.20 is the floor because older releases cannot read a double-quoted `compose.env` value that ends in a backslash. They stop with "unterminated quoted value" and the stack does not start. `nself doctor` warns when the installed version is below 2.20, and `nself start` refuses to run. Upgrade Docker Desktop, or the `docker-compose-plugin` package on Linux.

### Hardware

| Resource | Minimum | Recommended |
|---|---|---|
| **Disk** | 5 GB free | 10 GB free (for a typical dev stack with images + data) |
| **RAM** | 4 GB | 8 GB (multi-service stack with monitoring) |
| **CPU** | 2 cores | 4+ cores (production or concurrent service loads) |

> Docker image pulls and Postgres data volumes are the primary disk consumers. The 5 GB minimum covers a single-product install with no monitoring bundle. Enable the monitoring bundle only when the 10 GB threshold is met.

---

## Network Requirements

### Outbound (required)

| Destination | Protocol | Purpose |
|---|---|---|
| `ghcr.io` | HTTPS (443) | GitHub Container Registry , CLI and service images |
| `docker.io` | HTTPS (443) | Docker Hub , base images |
| `plugins.nself.org` | HTTPS (443) | Plugin registry API |
| `ping.nself.org` | HTTPS (443) | License validation and telemetry |

All outbound traffic is HTTPS only. No inbound firewall rules are required for local development.

### Inbound (production deployments only)

| Port | Protocol | Purpose |
|---|---|---|
| 80 | TCP | HTTP → Nginx (redirects to HTTPS) |
| 443 | TCP | HTTPS → Nginx reverse proxy |

Local dev stacks bind to `127.0.0.1` and require no inbound firewall exceptions.

---

## Optional Tools

These tools are not required to run the CLI but are useful for advanced workflows.

| Tool | Purpose |
|---|---|
| `psql` (PostgreSQL client) | Direct database access for debugging, migrations, and ad-hoc queries |
| `pgbouncer` | Connection pooling for high-concurrency production deployments |
| `jq` | JSON output parsing for scripting and automation |
| `curl` | Manual health-check calls to API endpoints |

---

## Cross-References

- **Apple Silicon (M1/M2/M3):** [macos-apple-silicon.md](macos-apple-silicon.md), Rosetta considerations, native arm64 images, known issues
- **Windows / WSL2 setup:** [windows-wsl2.md](windows-wsl2.md), WSL2 installation, Docker Desktop WSL2 backend, file-system path notes
- **Installation:** [Installation.md](Installation.md), step-by-step install for each platform
- **Quick Start:** [Quick-Start.md](Quick-Start.md), first project after install
