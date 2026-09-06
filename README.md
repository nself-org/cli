# ɳSelf CLI

[![Version](https://img.shields.io/github/v/release/nself-org/cli?label=version)](https://github.com/nself-org/cli/releases/latest)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20WSL-lightgrey.svg)](#prerequisites)
[![Docker](https://img.shields.io/badge/docker-required-blue.svg)](https://www.docker.com/get-started)
[![CI Status](https://github.com/nself-org/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/nself-org/cli/actions)
[![Security Scan](https://github.com/nself-org/cli/actions/workflows/security-scan.yml/badge.svg)](https://github.com/nself-org/cli/actions/workflows/security-scan.yml)
[![Test Coverage](https://codecov.io/gh/nself-org/cli/branch/main/graph/badge.svg)](https://codecov.io/gh/nself-org/cli)

<div align="center">

**Self-hosted backend in 5 minutes**

Complete, production-ready backend stack — PostgreSQL, GraphQL API, Authentication, and Nginx — launched from a single command. Extend with 256 plugins (129 free, 127 paid). MIT licensed core, forever.

**v1.3.5 is out.** See the [changelog](.github/wiki/Changelog.md) for what shipped. [Release notes](https://github.com/nself-org/cli/releases/tag/v1.3.5)

```bash
brew install nself-org/nself/nself
```

[Quick Start](#quick-start) · [The Stack](#the-stack) · [Plugins](#plugins) · [Pricing](#pricing) · [Docs](https://nself.org/docs)

</div>

---

## What is ɳSelf?

ɳSelf is a self-hosted backend platform that gives you the equivalent of a commercial Backend-as-a-Service on your own infrastructure. Run it locally, on a VPS, or on any cloud provider (Kubernetes: planned — see the [k8s plugin](https://nself.org/plugins)).

You get Postgres, Hasura GraphQL, nHost Auth, and Nginx as the always-on core. Add optional services like Redis, MinIO, and Search when you need them. Extend everything with plugins — 129 free MIT plugins and 127 paid plugins covering AI, messaging, media processing, billing, and more.

The nself CLI is a single Go binary. No runtime dependencies beyond Docker.

---

## Quick Start

```bash
# Install (macOS)
brew install nself-org/nself/nself

# Install (Linux / WSL2)
curl -fsSL https://install.nself.org | bash

# Create a project and start the stack
mkdir my-backend && cd my-backend
nself init
nself build
nself start
```

Your backend is running:

- GraphQL API: `https://api.local.nself.org`
- Auth: `https://auth.local.nself.org`
- Storage: `https://storage.local.nself.org`
- Admin UI: `http://localhost:3021`

All `*.local.nself.org` domains resolve to `127.0.0.1`. Automatic SSL — no browser warnings.

---

## The Stack

ɳSelf organizes services into three tiers. This ordering matters everywhere: Core first, Optional second, Plugins last.

### Core Services (always on — 4 services)

These four services start with every `nself start` and cannot be disabled. They are the permanent foundation.

| Service | Role |
|---------|------|
| **PostgreSQL** | Primary database (pgvector, PostGIS, TimescaleDB) |
| **Hasura** | Instant GraphQL API with permissions, subscriptions, and metadata |
| **Auth** (nHost) | JWT-based authentication — 13 OAuth providers (Google, GitHub, Microsoft, etc.) |
| **Nginx** | Reverse proxy and SSL terminator — the only external-facing process |

All internal services bind to `127.0.0.1`. Nginx is the only external-facing process.

### Optional Services (enable per project — 6 services)

These services are first-class and CLI-managed. Toggle them in `.env` and rebuild.

| Service | Toggle | Role |
|---------|--------|------|
| **Redis** | `REDIS_ENABLED=true` | Caching, sessions, queues |
| **MinIO** | `MINIO_ENABLED=true` | S3-compatible object storage |
| **Search** | `MEILISEARCH_ENABLED=true` | Full-text search (MeiliSearch) |
| **Email** | `MAIL_ENABLED=true` | Email service — 16+ providers (SendGrid, SES, Mailgun, Postfix, and more) |
| **Functions** | `FUNCTIONS_ENABLED=true` | Serverless runtime |
| **Admin** | `NSELF_ADMIN_ENABLED=true` | Local GUI companion at `localhost:3021` |

```bash
nself service list          # see all available services and their status
nself service enable redis  # enable a service
nself build && nself restart
```

### Plugins (drop-in expansions)

Plugins are the extension layer — install what you need, remove what you don't.

- **129 free MIT plugins** — no license key required
- **127 paid plugins** — require a membership key (starting at $0.99/mo)

```bash
nself plugin install monitoring    # free: Prometheus + Grafana + Loki
nself plugin install search        # free: MeiliSearch full-text search
nself plugin install cron          # free: scheduled job execution

nself license set nself_pro_xxxxx
nself plugin install ai            # paid: multi-provider LLM gateway
nself plugin install livekit       # paid: live video and audio
nself plugin install claw          # paid: AI assistant backend
```

See [Plugins](#plugins) for the full inventory.

---

## Key Features

- Complete backend in under 5 minutes on any machine with Docker
- 50 CLI commands — full control from the terminal over every service and operation
- Built-in CI gate (`nself ci`) — replaces external CI for merge enforcement
- Core services always-on; optional services enable with one line in `.env`
- 256 plugins extend the stack without touching core config
- Multi-tenancy, row-level security, and org management built in
- Stripe/Paddle billing integration included
- White-label support — custom domains, branding, email templates
- Monitoring bundle (Prometheus, Grafana, Loki, Tempo, and 6 more) via the `monitoring` plugin
- Automated SSL via Let's Encrypt in production; trusted local certs in dev
- Security hardening runs automatically on install, update, deploy, and daily — always free
- Multi-environment support: local, staging, and production from one install
- 40+ custom service templates (Express, FastAPI, NestJS, Gin, Rust, gRPC, and more)
- One-command migration from Supabase, Nhost, or Firebase

---

## Installation

### macOS (Homebrew)

```bash
brew install nself-org/nself/nself
```

### Linux

```bash
curl -fsSL https://install.nself.org | bash
```

### Windows

WSL2 is required. Once WSL2 is set up, use the Linux installer above inside your WSL2 terminal.

The installer auto-detects existing installations, checks for Docker, downloads the binary to `~/.nself/bin`, and adds nself to your PATH.

### Docker

```bash
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.nself:/root/.nself nself/nself version
```

### Build from source

```bash
git clone https://github.com/nself-org/cli
cd cli
make build
make install  # copies nself to /usr/local/bin
```

Requires Go 1.22+ and GNU make.

### Secure install (recommended for CI and production)

The installer always fetches `checksums.txt` from the same GitHub release and verifies the SHA-256 of the downloaded tarball before extracting. Installation aborts loudly on any mismatch.

For the most paranoid setups, pin the expected SHA-256 in your shell or CI config:

```bash
# Obtain the expected checksum for a specific release:
curl -fsSL https://github.com/nself-org/cli/releases/download/v1.3.5/checksums.txt

# Install with a pinned version and pinned SHA-256:
NSELF_VERSION=v1.3.5 \
NSELF_INSTALL_PIN_SHA256=<sha256-from-checksums.txt> \
curl -fsSL https://install.nself.org | bash
```

This defends against a compromised release page: even if the tarball at the release URL is replaced, the pinned SHA will not match and the install fails.

### Update

```bash
nself update    # detects your install method and updates in place
```

---

## 5-Minute Tutorial

```bash
# 1. Install
# macOS:  brew install nself-org/nself/nself
# Linux:  curl -fsSL https://install.nself.org | bash

# 2. Create a project
mkdir my-backend && cd my-backend

# 3. Initialize with defaults (or use --wizard for interactive setup)
nself init

# 4. Generate Docker configs from your .env
nself build

# 5. Start the stack
nself start

# 6. Check everything is healthy
nself status
nself urls
```

Your core backend is running. Add optional services:

```bash
# Enable Redis and full-text search
nself service enable redis
nself service enable search
nself build && nself restart

# Install a free plugin
nself plugin install monitoring  # Prometheus + Grafana + Loki

# Set a pro license and install an AI plugin
nself license set nself_pro_xxxxxx
nself plugin install ai
nself build && nself restart
```

---

## CLI Command Reference

ɳSelf provides **47 top-level commands** organized by domain. Full reference: [nself.org/docs/cli](https://nself.org/docs/cli).

### Stack lifecycle

```
nself init          Initialize a new nSelf project
nself build         Generate Docker configs from .env
nself start         Boot the stack (alias: nself up)
nself stop          Stop the stack (alias: nself down)
nself restart       Smart restart with change detection
nself reset         Stop, remove volumes, clean generated files
```

### Services and config

```
nself service       Manage optional services (list, enable, disable)
nself config        Manage project configuration (show, get, set, validate)
nself env           Multi-environment management (use, list, diff, copy)
nself promote       Promote environment to another (staging to prod)
```

### Plugins and licensing

```
nself plugin        Plugin management (install, remove, update, list, status, marketplace)
nself license       License key management (set, show, validate, clear)
```

### Database

```
nself db            Database operations:
                      migrate (up/down/status/create)
                      seed, backup, restore, shell, drop, reset
                      rls, pitr, pgbouncer, audit, soft-delete, fk-index, fixtures
```

### Security and operations

```
nself security      Security audit, setup, scan, and status
nself secrets       Encrypted secret management (age encryption)
nself backup        Backup operations (create, list, restore, verify, prune)
nself ssl           SSL certificate management (status, renew, setup, add)
nself doctor        System diagnostics with auto-repair (--deep for full hardening checks)
nself health        Health checks (check, watch, history)
```

### Monitoring and observability

```
nself status        Service health status (--deep for extended probes)
nself logs          View and filter service logs
nself urls          List all service URLs
nself monitor       Monitoring stack management
nself alerts        Manage Prometheus alert rules and silences
nself watchdog      Self-healing container watchdog with circuit breaker
```

### Infrastructure

```
nself deploy        Deploy to environment
nself tenant        Tenant management (create, upgrade, suspend, destroy, audit)
nself admin         Open the Admin UI (start, connect)
nself dr            Disaster recovery (drill, promote-standby, rollback, fence)
nself waf           Web Application Firewall (enable, mode, report)
```

Run `nself help <command>` for subcommand details.

---

## Comparison

| | ɳSelf | Supabase | Nhost | Coolify | Firebase |
|---|:---:|:---:|:---:|:---:|:---:|
| Fully self-hosted | Yes | Limited | Limited | Yes | No |
| MIT licensed core | Yes | Partial | Partial | Apache 2.0 | No |
| Multi-tenancy built-in | Yes | No | No | No | No |
| Built-in billing integration | Yes | No | No | No | No |
| White-label support | Yes | No | No | No | No |
| Setup time | ~5 min | 30+ min | 30+ min | 15+ min | N/A |
| Plugin ecosystem | 256 total (129 free + 127 pro) | Limited | Limited | App templates | No |
| Deploy anywhere | Yes | Cloud-first | Cloud-first | Yes | No |
| Data ownership | Full | Shared | Shared | Full | No |

---

## Pricing

The nself CLI and 129 free MIT plugins are fully FOSS. Free forever, including commercial use. Pro plugins require a membership key.

| Tier | Monthly | Annual | What's included |
|------|---------|--------|-----------------|
| Free | $0 | $0 | Core CLI + 129 free MIT plugins |
| Bundle | $0.99/mo | $9.99/yr | All 127 pro plugins (ɳChat, ɳClaw, ɳSentry, ɳTV, ɳFamily, or ClawDE) |
| ɳSelf+ | $3.99/mo | $39.99/yr | All 6 bundles + all apps + priority support |
| ɳCloud | custom | custom | Managed hosting + ɳSelf+ license + cloud-exclusive features |

Annual pricing saves ~17% vs monthly. See [nself.org/pricing](https://nself.org/pricing) for current offers and migration from prior tiers.

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install ai
```

---

## Plugins

### Free plugins (29 — MIT, no key required)

Install any free plugin with `nself plugin install <name>`:

| Plugin | Category | What it does |
|--------|----------|-------------|
| `backup` | Infrastructure | Automated PostgreSQL backup with pruning and cloud storage |
| `cron` | Automation | Scheduled job execution with HTTP callbacks |
| `feature-flags` | Infrastructure | Feature flag management with targeting rules |
| `github` | Development | GitHub repository, issue, and workflow integration |
| `jobs` | Infrastructure | PostgreSQL-backed background job queue |
| `monitoring` | Infrastructure | Full observability stack: Prometheus, Grafana, Loki, and 7 more services |
| `notify` | Communication | Email (SMTP) and webhook notifications |
| `search` | Infrastructure | Full-text search (PostgreSQL FTS + MeiliSearch) |
| `stripe` | Commerce | Stripe payment data sync and webhooks |
| `webhooks` | Communication | Outbound webhook delivery with retry and HMAC signing |
| + 15 more | Various | content-acquisition, content-progress, donorbox, github-runner, invitations, link-preview, mdns, notifications, paypal, shopify, subtitle-manager, tokens, torrent-manager, vpn, webhooks |

Full inventory: [nself.org/plugins](https://nself.org/plugins)

### Pro plugins (127 — license-gated, starting at $0.99/mo)

```bash
nself license set nself_pro_xxxxxx
nself plugin install <name>
nself build
```

Categories include AI and intelligence, real-time communication, media processing, billing, compliance, content management, multi-tenancy, and more. Notable plugins:

- `ai` — multi-provider LLM gateway (OpenAI, Anthropic, Cohere, local models)
- `claw` — AI personal assistant backend with infinite memory (pgvector + ltree)
- `livekit` — live video and audio rooms
- `chat` — real-time messaging
- `media-processing` — encoding, transcoding, thumbnail generation
- `streaming` — HLS/DASH adaptive streaming
- `compliance` — GDPR, CCPA, HIPAA, SOC 2, PCI-DSS coverage
- `auth` — WebAuthn/passkeys, TOTP 2FA, magic links, device-code flow

Pro plugins are [source-available](https://nself.org/legal/bundle-license). License validated server-side via `ping.nself.org`. Revoked keys render plugins inert on next build.

---

## Companion Products

ɳSelf powers six open-source apps. Each is an MIT Flutter app you self-host:

| App | Bundle | Price | What it does |
|-----|--------|-------|-------------|
| [ɳTasks](https://github.com/nself-org/ntask) | Free | $0 | Task management — free, no paid plugins ever |
| [nChat](https://github.com/nself-org/nchat) | nChat | $0.99/mo | Messaging with live video, bots, and moderation |
| [ɳClaw](https://github.com/nself-org/nclaw) | ɳClaw | $0.99/mo | AI personal assistant with self-organizing memory |
| [nTV](https://github.com/nself-org/ntv) | nTV | $0.99/mo | Media server and player (6 platforms) |
| [nFamily](https://github.com/nself-org/nfamily) | nFamily | $0.99/mo | Private family social hub (planned) |
| [ClawDE](https://github.com/nself-org/clawde) | ClawDE | $0.99/mo | AI development environment (desktop + mobile) |

Each app's backend is powered by the nself CLI + the matching plugin bundle.

---

## Configuration

Config lives in `.env.dev` (team defaults), `.env.staging`, and `.env.prod`. `nself build` reads the active env file and generates `docker-compose.yml` and Nginx config.

Never hand-edit `docker-compose.yml` — it is generated and will be overwritten on the next build.

```bash
nself config show        # show current config
nself config set KEY VALUE
nself config validate    # validate before building
```

### Env file priority (low to high)

1. `.env.dev` — team defaults (checked in)
2. `.env.staging` — staging shared config (checked in)
3. `.env.prod` — production shared config (checked in)
4. `.env.secrets` — production secrets (not checked in)
5. `.env` — local overrides (not checked in, highest priority)

### Custom services (40+ templates)

Add your own services via `CS_N` slots:

```bash
CS_1=api:fastapi:3001     # Python FastAPI
CS_2=worker:bullmq-ts:3002  # TypeScript BullMQ
CS_3=grpc:grpc:3003       # gRPC service
```

Templates: JavaScript/TypeScript (19), Python (7), Go (4), Rust, Java, C#, Elixir, and more.

---

## Production Deployment

```bash
# Copy project to server
rsync -az --exclude .volumes/ ./ user@server:/opt/myapp/

# On the server
nself start --env prod
nself status
nself health check
```

Security hardening runs automatically on every deploy. No license required.

See [Self-hosting: what is and is not included](https://nself.org/docs/self-hosting/boundary) for
what nSelf provides vs. what you own as the operator (backups, monitoring, on-call, capacity).

---

## Requirements

| | Minimum | Recommended |
|---|---------|-------------|
| Docker | 24+ | latest |
| macOS | 12 (Monterey) | 14+ |
| Linux | Ubuntu 20.04 / Debian 11 | Ubuntu 22.04+ |
| RAM | 2 GB | 4 GB |
| Disk | 5 GB | 10 GB |

---

## Docs and Community

- [nself.org/docs](https://nself.org/docs) — complete documentation
- [nself.org](https://nself.org) — project home, pricing, plugins catalog
- [GitHub Discussions](https://github.com/nself-org/cli/discussions) — community Q&A
- [GitHub Issues](https://github.com/nself-org/cli/issues) — bug reports

---

## Contributing

The CLI is written in Go (1.22+). See [Contributing](.github/wiki/Contributing.md) for the full guide.

Quick setup:

```bash
git clone https://github.com/nself-org/cli
cd cli
make build   # produces ./nself
make test    # go test -mod=vendor ./...
```

Tests live in `internal/` alongside the packages they test. Coverage target: 70% on `cmd/commands/` and `internal/`. Integration tests gate on `INTEGRATION=1` and require Docker.

---

## License

MIT, free for personal and commercial use. See [LICENSE](LICENSE).

Pro plugins (count generated from the plugins-pro registry, not hand-typed here) are [source-available](https://nself.org/legal/bundle-license) under the nSelf Bundle License. Compiled binaries are distributed through `ping.nself.org` after license validation.

---

<div align="center">

**ɳSelf CLI v1.3.5** · [nself.org](https://nself.org) · [GitHub](https://github.com/nself-org/cli)

[Get Started](#quick-start) · [Documentation](https://nself.org/docs) · [Pricing](#pricing)

</div>
