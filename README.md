# ɳSelf - Complete Self-Hosted Backend Platform

[![Version](https://img.shields.io/badge/version-0.9.9-blue.svg)](https://github.com/nself-org/cli/releases)
[![Status](https://img.shields.io/badge/status-production--ready-brightgreen.svg)](https://github.com/nself-org/cli/releases/tag/v0.9.9)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20WSL-lightgrey.svg)](#-prerequisites)
[![Docker](https://img.shields.io/badge/docker-required-blue.svg)](https://www.docker.com/get-started)
[![CI Status](https://github.com/nself-org/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/nself-org/cli/actions)
[![Security Scan](https://github.com/nself-org/cli/actions/workflows/security-scan.yml/badge.svg)](https://github.com/nself-org/cli/actions/workflows/security-scan.yml)
[![Test Coverage](https://codecov.io/gh/nself-org/cli/branch/main/graph/badge.svg?token=CODECOV_TOKEN)](https://codecov.io/gh/nself-org/cli)
[![License](https://img.shields.io/badge/license-Personal%20Free%20%7C%20Commercial-green.svg)](LICENSE)


<div align="center">

**Deploy a production-ready backend in 5 minutes**

Complete self-hosted Backend-as-a-Service with PostgreSQL, GraphQL API, Authentication, Storage, Real-time features, and unlimited custom services. Seamless local → staging → production workflow with automated SSL, intelligent defaults, and enterprise monitoring.

```bash
curl -sSL https://install.nself.org | bash
```

**One command. Complete backend. Your infrastructure.**

[Quick Start](#-quick-start---3-commands-to-backend-bliss) • [Features](#-why-nself) • [Documentation](.wiki/Home.md) • [Roadmap](.wiki/releases/ROADMAP.md)

</div>

---

## What is ɳSelf?

ɳSelf is a **complete self-hosted Backend-as-a-Service platform** that gives you the same powerful features as commercial services like Supabase and Nhost, but runs entirely on your own infrastructure.

**Get the power of commercial BaaS, plus:**
- **True Data Ownership** - Your data never leaves your infrastructure
- **Multi-Tenancy Built-In** - Enterprise SaaS features out of the box
- **Integrated Billing** - Stripe/Paddle integration included
- **Deploy Anywhere** - Your laptop, VPS, cloud, or Kubernetes
- **No Vendor Lock-In** - Standard open-source components
- **Complete Control** - Customize everything

From zero to production-ready backend in under 5 minutes. Really.

## 🚀 Why ɳSelf?

### ⚡ Lightning Fast Setup
- **Under 5 minutes** from zero to running backend
- One command installation, initialization, and deployment
- Smart defaults that just work™
- Interactive wizard or quick setup mode

### 🎯 Complete Feature Set

**Core Backend Stack:**
- **PostgreSQL** - Production-ready database with pgvector, PostGIS, TimescaleDB
- **Hasura GraphQL** - Instant GraphQL API with permissions and subscriptions
- **Authentication** - JWT-based auth with 13 OAuth providers (Google, GitHub, Microsoft, etc.)
- **Storage (MinIO)** - S3-compatible object storage with CDN integration
- **Real-Time** - WebSocket channels, database subscriptions (CDC), presence tracking

**Enterprise Features (Unique to ɳSelf):**
- **Multi-Tenancy** - Complete tenant isolation, row-level security, org management
- **Billing Integration** - Stripe/Paddle subscriptions, usage tracking, invoicing
- **White-Label** - Custom domains, branding, email templates, legal documents
- **Migration Tools** - One-command migration from Supabase, Nhost, Firebase

**Production Infrastructure:**
- **Monitoring Bundle** - Prometheus, Grafana, Loki, Tempo (10 services)
- **Security Hardening** - SQL injection prevention, CSP framework, rate limiting
- **Automated Backups** - Intelligent pruning, cloud storage, 3-2-1 rule verification
- **SSL Automation** - Trusted certificates with zero configuration

**Developer Experience:**
- **40+ Service Templates** - Express, FastAPI, Flask, Gin, Rust, NestJS, gRPC, and more
- **295+ CLI Commands** - Complete control from the terminal
- **Admin Dashboard** - Web-based management UI
- **Email Management** - 16+ providers with zero-config development mode

### 💪 ɳSelf vs Others

| Feature | ɳSelf | Supabase | Nhost | DIY |
|---------|-------|----------|-------|-----|
| **Full Backend Stack** | ✅ | ✅ | ✅ | ⚠️ Manual |
| **Self-Hosted** | ✅ | ✅ (limited) | ✅ (limited) | ✅ |
| **Multi-Tenancy** | ✅ Built-in | ❌ | ❌ | 🛠️ DIY |
| **Built-in Billing** | ✅ Stripe/Paddle | ❌ External | ❌ External | 🛠️ DIY |
| **White-Label** | ✅ Complete | ❌ | ❌ | 🛠️ DIY |
| **Deploy Anywhere** | ✅ Any infra | ⚠️ Cloud-first | ⚠️ Cloud-first | ✅ |
| **Setup Time** | 5 minutes | 30+ minutes | 30+ minutes | Hours/Days |
| **One Command Deploy** | ✅ | ❌ | ❌ | ❌ |
| **Data Ownership** | ✅ Complete | ⚠️ Shared | ⚠️ Shared | ✅ |
| **Pricing** | Free (personal) | Paid | Paid | Free |

## 📋 Prerequisites

- **Bash 3.2+** (default on macOS and most Linux distributions)
- **Linux, macOS, or Windows with WSL**
- **Docker and Docker Compose** (installer helps install if needed)
- **curl** (for installation)

**Note:** ɳSelf is fully compatible with Bash 3.2+, the default shell on macOS. No Bash upgrade needed!

## 🔧 Installation

### Quick Install (Recommended)

```bash
curl -sSL https://install.nself.org | bash
```

The installer will:
- ✅ Auto-detect existing installations and offer updates
- 📊 Show visual progress with loading spinners
- 🔍 Check and help install Docker/Docker Compose if needed
- 📦 Download nself CLI to `~/.nself/bin`
- 🔗 Add nself to your PATH automatically
- 🚀 Create a global `nself` command

### Alternative Methods

**macOS/Linux (Homebrew)**
```bash
brew tap nself-org/nself
brew install nself
```

**Direct from GitHub**
```bash
curl -fsSL https://raw.githubusercontent.com/nself-org/cli/main/install.sh | bash
```

**Docker**
```bash
docker pull nself-org/cli:latest
docker run -it nself-org/cli:latest version
```

### Updating ɳSelf

```bash
nself update
```

The updater will:
- Check for new versions automatically
- Show version comparison (current → latest)
- Download and install updates seamlessly
- Preserve your existing configurations

## 🏁 Quick Start - 3 Commands to Backend Bliss

```bash
# 1. Create and enter project directory
mkdir my-backend && cd my-backend

# 2. Initialize with interactive wizard (or quick mode)
nself init --wizard  # Interactive setup
# or: nself init     # Quick setup with defaults

# 3. Build and launch everything
nself build && nself start
```

**That's it!** Your complete backend is now running at:
- 🚀 GraphQL API: https://api.local.nself.org
- 🔐 Auth Service: https://auth.local.nself.org
- 📦 Storage: https://storage.local.nself.org
- 📧 Email UI (dev): https://mail.local.nself.org
- 📊 Admin Dashboard: http://localhost:3021

All URLs work with automatic SSL (no browser warnings!).

## 🌐 Default Service URLs

When using the default `local.nself.org` domain (resolves to 127.0.0.1):

**Core Services:**
- **GraphQL API**: https://api.local.nself.org
- **Authentication**: https://auth.local.nself.org
- **Storage**: https://storage.local.nself.org
- **Storage Console**: https://storage-console.local.nself.org

**Optional Services (when enabled):**
- **Functions**: https://functions.local.nself.org
- **Email (MailPit)**: https://mail.local.nself.org
- **Search (MeiliSearch)**: https://search.local.nself.org
- **MLflow**: https://mlflow.local.nself.org

**Monitoring (when enabled):**
- **Grafana**: https://grafana.local.nself.org
- **Prometheus**: https://prometheus.local.nself.org

**Management:**
- **Admin UI**: http://localhost:3021

All `*.local.nself.org` domains automatically resolve to `127.0.0.1` for zero-config local development.

## 📧 Email Configuration

### Development (Zero Config)
Email works out of the box with MailPit - all emails are captured locally:
- 📧 View emails: https://mail.local.nself.org
- 🔧 No setup required
- 📨 Perfect for testing auth flows

### Production (2-Minute Setup)
```bash
nself service email config
```

Choose from 16+ providers:
- **SendGrid** - 100 emails/day free
- **AWS SES** - $0.10 per 1000 emails
- **Mailgun** - First 1000 emails free
- **Postmark** - Transactional email specialist
- **Gmail** - Use your personal/workspace account
- **Postfix** - Full control self-hosted server
- And 10+ more!

The wizard guides you through everything. Example for SendGrid:
```bash
nself service email config sendgrid
# Add your API key to .env
nself build && nself restart
```

## 🎯 Customize Your Stack

Edit `.env` to enable additional services:

```bash
# Core settings
ENV=dev                         # or 'prod' for production
PROJECT_NAME=myapp
BASE_DOMAIN=local.nself.org

# Optional Services
REDIS_ENABLED=true              # Redis caching and queues
MINIO_ENABLED=true              # S3-compatible storage
FUNCTIONS_ENABLED=true          # Serverless functions
MLFLOW_ENABLED=true             # ML experiment tracking
MEILISEARCH_ENABLED=true        # Search engine

# Monitoring Bundle (10 services)
MONITORING_ENABLED=true         # Prometheus, Grafana, Loki, etc.

# Database Extensions
POSTGRES_EXTENSIONS=timescaledb,postgis,pgvector

# Custom Services (40+ templates)
CS_1=api:fastapi:3001           # Python FastAPI
CS_2=worker:bullmq-ts:3002      # Background jobs
CS_3=grpc:grpc:3003             # gRPC service
```

Then rebuild and restart:
```bash
nself build && nself restart
```

## 🚀 Service Templates - 40+ Ready-to-Use Microservices

Add custom backend services with one line in your `.env`:

```bash
# Enable custom services
SERVICES_ENABLED=true

# Add microservices
CS_1=api:fastapi:3001           # Python FastAPI
CS_2=auth:nest-ts:3002          # TypeScript NestJS
CS_3=jobs:bullmq-ts:3003        # Background jobs (BullMQ)
CS_4=ml:ray:3004                # ML model serving (Ray)
CS_5=chat:socketio-ts:3005      # Real-time WebSocket
```

### Available Templates by Language

- **JavaScript/TypeScript (19)**: Express, Fastify, NestJS, Hono, Socket.IO, BullMQ, Temporal, Bun, Deno, tRPC
- **Python (7)**: Flask, FastAPI, Django REST, Celery, Ray, AI Agents (LLM & Data)
- **Go (4)**: Gin, Echo, Fiber, gRPC
- **Other (10)**: Rust, Java, C#, C++, Ruby, Elixir, PHP, Kotlin, Swift

📖 **[View Complete Service Templates Documentation](.wiki/reference/SERVICE_TEMPLATES.md)**

Every template includes:
- 🐳 Production Docker setup with multi-stage builds
- 🛡️ Security headers and CORS configuration
- 📊 Health checks and graceful shutdown
- ⚡ Language-specific optimizations
- 🔧 Template variables for customization

## 📚 Commands Overview

ɳSelf provides a **32-command canonical runtime surface** (**31 grouped domains + `destroy`**) with **295+ subcommands** organized by domain:

### Core Commands (5)
```bash
nself init          # Initialize project with wizard
nself build         # Generate Docker configs
nself start         # Start services
nself stop          # Stop services
nself restart       # Restart services
```

### Utilities (15)
```bash
nself status        # Service health status
nself logs          # View service logs
nself admin         # Open admin UI
nself urls          # List all service URLs
nself doctor        # System diagnostics
nself monitor       # Monitoring dashboards
nself health        # Health checks
nself version       # Version info
nself update        # Update nself
```

### Advanced Commands (11)
```bash
nself db            # Database operations (11 subcommands)
nself tenant        # Multi-tenancy (50+ subcommands)
nself deploy        # Deployment (33 subcommands)
nself infra         # Infrastructure (48 subcommands: K8s, cloud)
nself service       # Service management (43 subcommands)
nself config        # Configuration (20 subcommands)
nself auth          # Security (38 subcommands: OAuth, SSL, MFA)
nself backup        # Backup & recovery (6 subcommands)
nself dev           # Developer tools (16 subcommands)
nself plugin        # Plugin system (8+ subcommands)
nself destroy       # Safe infrastructure destruction
```

📖 **[Complete Command Reference](.wiki/commands/COMMAND-TREE-V1.md)**

## 🎯 Admin Dashboard

Web-based monitoring and management interface:

```bash
nself admin         # Open admin UI in browser
nself admin --dev   # Open in development mode
```

**Features:**
- **Service Health Monitoring** - Real-time status of all containers
- **Docker Management** - Start, stop, restart containers from UI
- **Database Query Interface** - Execute SQL queries directly
- **Log Viewer** - Filter and search through service logs
- **Backup Management** - Create and restore backups via UI
- **Configuration Editor** - Modify settings without SSH

Access at: http://localhost:3021

## 🔐 SSL/TLS Configuration

ɳSelf provides bulletproof SSL with green locks in browsers - no warnings!

### Automatic Certificate Generation

```bash
nself build              # Automatically generates SSL certificates
nself auth ssl trust     # Install root CA for green locks (one-time)
```

That's it! Your browser will show green locks for:
- https://localhost, https://api.localhost
- https://local.nself.org, https://api.local.nself.org

### Two Domain Options (Both Work Perfectly)

1. **`*.localhost`** - Works offline, no DNS needed
2. **`*.local.nself.org`** - Our loopback domain (resolves to 127.0.0.1)

### Advanced: Public Wildcard Certificates

For teams or CI/CD, get globally-trusted certificates:

```bash
# Add to .env
DNS_PROVIDER=cloudflare        # or route53, digitalocean
DNS_API_TOKEN=your_api_token

# Generate public wildcard
nself auth ssl generate
```

Supported DNS providers: Cloudflare, AWS Route53, DigitalOcean, and more via acme.sh

## 💾 Backup & Restore

### Comprehensive Backup System

```bash
# Create backups
nself backup create              # Full backup (database, config, volumes)
nself backup create database     # Database only
nself backup create config       # Configuration only

# Restore from backup
nself backup restore backup_20260201_143022.tar.gz

# List all backups
nself backup list
```

### Cloud Storage Support

Configure automatic cloud uploads:

```bash
# Interactive cloud setup wizard
nself backup cloud setup

# Supported providers:
# - Amazon S3 / MinIO
# - Dropbox, Google Drive, OneDrive
# - 40+ providers via rclone
```

### Automated Backups

```bash
# Schedule backups
nself backup schedule --daily --time "02:00"

# Clean old backups
nself backup clean --age 30      # Remove backups older than 30 days
```

### What Gets Backed Up

**Full Backup includes:**
- PostgreSQL databases (complete dump)
- All environment files (.env.dev, .env.staging, .env.prod, .env.secrets)
- Docker-compose configurations
- Docker volumes (all project data)
- SSL certificates
- Hasura metadata
- Nginx configurations

## 🚀 Production Deployment

### Deploy to Production

```bash
# 1. Configure production environment
nself config env create production

# 2. Deploy to production
nself deploy production

# 3. Check deployment status
nself deploy status

# 4. Monitor deployment
nself monitor
```

### Production Checklist
1. ✅ Set `ENV=prod` (automatically configures security settings)
2. ✅ Use strong passwords (12+ characters, auto-generated)
3. ✅ Configure your custom domain
4. ✅ Enable Let's Encrypt SSL
5. ✅ Set up automated backups
6. ✅ Configure monitoring alerts

### Environment File Priority

Files loaded in order (later files override earlier):
1. `.env.dev` - Team defaults (always loaded)
2. `.env.staging` - Staging environment (if ENV=staging)
3. `.env.prod` - Production environment (if ENV=prod)
4. `.env.secrets` - Production secrets (if ENV=prod)
5. `.env` - Local overrides (highest priority)

## 📁 Project Structure

After running `nself build`:

```
my-backend/
├── .env.dev               # Team defaults
├── .env.staging           # Staging environment (optional)
├── .env.prod              # Production environment (optional)
├── .env.secrets           # Production secrets (optional)
├── .env                   # Local configuration (highest priority)
├── docker-compose.yml     # Generated Docker Compose file
├── nginx/                 # Nginx configuration
│   ├── nginx.conf
│   ├── conf.d/           # Service routing
│   └── ssl/              # SSL certificates
├── postgres/             # Database initialization
│   └── init/
├── hasura/               # GraphQL configuration
│   ├── metadata/
│   └── migrations/
├── functions/            # Optional serverless functions
└── services/             # Custom services (if enabled)
    ├── api/              # CS_1 service
    ├── worker/           # CS_2 service
    └── grpc/             # CS_3 service
```

## 🗄️ Database Management

Comprehensive database tools for schema management and migrations:

### For Lead Developers
```bash
# Design your schema
nano schema.dbml

# Generate migrations from schema
nself db run

# Test migrations locally
nself db migrate:up

# Commit to Git
git add schema.dbml hasura/migrations/
git commit -m "Add new tables"
git push
```

### For All Developers
```bash
# Pull latest code
git pull

# Start services
nself start

# If you see "DATABASE MIGRATIONS PENDING" warning:
nself db update  # Safely apply migrations with confirmation
```

### Database Commands
```bash
nself db                # Show all database commands
nself db run            # Generate migrations from schema.dbml
nself db update         # Apply pending migrations and seeds
nself db seed           # Apply seed data (dev or prod based on ENV)
nself db status         # Check database state
nself db revert         # Restore from backup
nself db sync           # Pull schema from dbdiagram.io
```

## 🐛 Troubleshooting

### Common Issues

**Services not starting?**
```bash
nself doctor            # Run diagnostics
nself logs [service]    # Check service logs
nself status            # Check service status
```

**Port conflicts?**
Edit port numbers in `.env` and rebuild:
```bash
nself build && nself restart
```

**SSL certificate warnings?**
```bash
nself auth ssl trust    # Install root CA (one-time)
```

**Email test not working?**
```bash
nself service email test recipient@example.com
```

**Build command hangs?**
```bash
nself build --force     # Force rebuild
```

📖 **[Complete Troubleshooting Guide](.wiki/guides/TROUBLESHOOTING.md)**

## 📚 Documentation

### Getting Started
- **[Quick Start Tutorial](.wiki/getting-started/Quick-Start.md)** - 5-minute tutorial
- **[Installation Guide](.wiki/getting-started/Installation.md)** - Detailed installation
- **[Configuration Reference](.wiki/configuration/README.md)** - Complete .env settings
- **[Command Reference](.wiki/commands/COMMAND-TREE-V1.md)** - All 295+ commands

### Features & Services
- **[Service Templates](.wiki/reference/SERVICE_TEMPLATES.md)** - 40+ microservice templates
- **[Database Workflow](.wiki/guides/DATABASE-WORKFLOW.md)** - DBML to production
- **[Multi-Tenancy](.wiki/architecture/MULTI-TENANCY.md)** - Enterprise SaaS features
- **[Email Setup](.wiki/commands/EMAIL.md)** - 16+ provider configuration

### Deployment & Operations
- **[Production Deployment](.wiki/deployment/README.md)** - Production guide
- **[Backup Guide](.wiki/guides/BACKUP_GUIDE.md)** - Comprehensive backup system
- **[Monitoring Setup](.wiki/guides/MONITORING-COMPLETE.md)** - Grafana dashboards
- **[Security Hardening](.wiki/security/README.md)** - Security best practices

### Migration & Compatibility
- **[Migration from Supabase](.wiki/migrations/FROM-SUPABASE.md)** - Step-by-step migration
- **[Migration from Nhost](.wiki/migrations/FROM-NHOST.md)** - One-command migration
- **[Migration from Firebase](.wiki/migrations/FROM-FIREBASE.md)** - Auth & Firestore

### Release Information
- **[Release Notes](.wiki/releases/v0.9.9.md)** - What's new in v0.9.9
- **[Roadmap](.wiki/releases/ROADMAP.md)** - Development roadmap
- **[Changelog](.wiki/releases/CHANGELOG.md)** - Version history
- **[All Releases](.wiki/releases/INDEX.md)** - Complete release history

## 🧪 Quality Assurance

### Test Coverage

- **700+ Tests** - 80% code coverage
- **Unit Tests** - 400+ tests for core functionality
- **Integration Tests** - 200+ tests for multi-service workflows
- **End-to-End Tests** - 100+ tests for complete user journeys

### CI/CD Status

All GitHub Actions workflows passing:

| Workflow | Status |
|----------|--------|
| CI | ✅ Passing (50+ checks) |
| Security Scan | ✅ Passing (30+ checks) |
| Tenant Isolation | ✅ Passing (20+ tests) |
| Build Validation | ✅ Passing (40+ checks) |
| Init Testing | ✅ Passing (25+ scenarios) |

### Security

Thoroughly audited for security vulnerabilities:

| Category | Status |
|----------|--------|
| Hardcoded Credentials | ✅ PASS |
| API Keys & Tokens | ✅ PASS |
| Command Injection | ✅ PASS |
| SQL Injection | ✅ PASS |
| Docker Security | ✅ PASS |
| Git History | ✅ PASS |

📖 **[View Security Audit](.wiki/security/SECURITY-AUDIT.md)**

## 🤝 Contributing

Contributions are welcome! Whether it's bug reports, feature requests, documentation improvements, or code contributions.

- **[Contributing Guide](.wiki/contributing/CONTRIBUTING.md)** - How to contribute
- **[Development Setup](.wiki/contributing/README.md)** - Dev environment
- **[Cross-Platform](.wiki/contributing/CROSS-PLATFORM-COMPATIBILITY.md)** - Compatibility requirements

## 📄 License

**Free for personal use. Commercial use requires a license.**

- ✅ **Personal Projects** - Free forever
- ✅ **Learning & Education** - Free forever
- ✅ **Open Source Projects** - Free forever
- 💼 **Commercial Use** - [Contact us for licensing](https://nself.org/commercial)

See [LICENSE](LICENSE) for full terms.

## 🎯 Perfect For

- **Startups** - Get your backend up fast, scale when you need to
- **Agencies** - Standardized backend setup for all client projects
- **Enterprises** - Self-hosted solution with full control
- **Side Projects** - Production-grade infrastructure without the complexity
- **Learning** - See how modern backends work under the hood

## 🔗 Links

- **[Official Website](https://nself.org)** - Project homepage
- **[Documentation](https://github.com/nself-org/cli/wiki)** - Complete nself documentation
- **[GitHub Repository](https://github.com/nself-org/cli)** - Source code
- **[Report Issues](https://github.com/nself-org/cli/issues)** - We'd love your feedback!
- **[Discussions](https://github.com/nself-org/cli/discussions)** - Community discussions
- **[Commercial Licensing](https://nself.org/commercial)** - For business use

## 🔄 Version History

| Version | Date | Focus |
|---------|------|-------|
| **v0.9.9** | Feb 2026 | Production Ready (current) |
| v0.9.7 | Jan 31, 2026 | Security & CI/CD Complete |
| v0.9.6 | Jan 30, 2026 | Command Consolidation |
| v0.9.5 | Jan 30, 2026 | Feature Parity & Security |
| v0.9.0 | Jan 30, 2026 | Multi-Tenant Platform |
| v0.4.8 | Jan 24, 2026 | Plugin System & Registry |

**[Full Roadmap](.wiki/releases/ROADMAP.md)** | **[Changelog](.wiki/releases/CHANGELOG.md)** | **[v1.0 Plan](.wiki/releases/v1.0.0-PLAN.md)**

---

<div align="center">

**ɳSelf v0.9.9** — Built by [nself](https://nself.org) · [GitHub](https://github.com/nself-org/cli)

**Ready for v1.0 LTS** 🚀

[Get Started](#-quick-start---3-commands-to-backend-bliss) • [Documentation](.wiki/Home.md) • [Roadmap](.wiki/releases/ROADMAP.md)

</div>
