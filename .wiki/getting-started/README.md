# Getting Started with nself

Welcome to nself! This guide will help you get up and running in minutes.

## What is nself?

nself is a complete self-hosted Backend-as-a-Service (BaaS) platform that provides:

- **GraphQL API** powered by Hasura
- **Authentication & Authorization** with JWT and OAuth
- **PostgreSQL Database** with automatic migrations
- **File Storage** with MinIO (S3-compatible)
- **Real-time Subscriptions** via GraphQL
- **Serverless Functions** runtime
- **Email Service** with templates
- **Search** with MeiliSearch or Typesense
- **Monitoring Stack** with Prometheus, Grafana, Loki
- **40+ Service Templates** for custom services

All running in Docker containers on your own infrastructure.

## Quick Navigation

| Document | Description |
|----------|-------------|
| **[Installation](../getting-started/Installation.md)** | Detailed installation instructions |
| **[Quick Start](../getting-started/Quick-Start.md)** | Get running in 5 minutes |
| **[FAQ](../getting-started/FAQ.md)** | Frequently asked questions |

## 5-Minute Quick Start

### 1. Install nself

```bash
# Install via curl
curl -sSL https://install.nself.org | bash

# Or via Homebrew (macOS)
brew tap acamarata/nself
brew install nself

# Verify installation
nself version
```

### 2. Initialize Your Project

```bash
# Create project directory
mkdir myapp && cd myapp

# Initialize with wizard
nself init

# Or use demo mode with all features
nself init --demo
```

The wizard will ask you:
- Project name
- Domain (e.g., local.nself.org for local dev)
- Which optional services to enable
- Database credentials

### 3. Build Configuration

```bash
nself build
```

This generates:
- `docker-compose.yml` with all your services
- `nginx/` configuration for routing
- `postgres/init/` database initialization
- `ssl/` self-signed certificates
- Custom service scaffolding (if any)

### 4. Start Your Stack

```bash
nself start
```

Wait for services to become healthy (usually 30-60 seconds).

### 5. Access Your Services

```bash
# See all service URLs
nself urls
```

Default services:
- **Application**: http://local.nself.org
- **GraphQL API**: http://api.local.nself.org
- **Authentication**: http://auth.local.nself.org
- **Admin Dashboard**: http://admin.local.nself.org (if enabled)

## What's Next?

### Design Your Database

nself uses a database-first workflow with DBML:

```bash
# Create a schema from template
nself db schema scaffold saas

# Opens schema.dbml - edit at dbdiagram.io for visual design
# Then apply the schema
nself db schema apply schema.dbml
```

This single command:
1. Generates migration SQL
2. Runs the migration
3. Seeds the database with realistic mock data
4. Creates test users

**[Learn More: Database Workflow](../guides/DATABASE-WORKFLOW.md)**

### Add Custom Services

```bash
# Add an Express API
nself service add api express-js 8001

# Add a Python API
nself service add ml-api fastapi 8002

# Add a BullMQ worker
nself service add worker bullmq-js 8003

# Rebuild and restart
nself build && nself restart
```

**[Learn More: Service Templates](../reference/SERVICE_TEMPLATES.md)**

### Deploy to Production

```bash
# Create production environment
nself env create prod production

# Configure server (edit .environments/prod/server.json)
nself env config prod

# Deploy
nself deploy prod
```

**[Learn More: Deployment](../deployment/README.md)**

## Common Commands

```bash
# View service status
nself status

# View logs
nself logs postgres
nself logs hasura --follow

# Access database
nself db connect

# Run migrations
nself db migrate up

# Create backup
nself db backup

# Stop services
nself stop

# Restart services
nself restart

# View all URLs
nself urls

# Get help
nself help
nself help db
```

## Architecture Overview

```
┌─────────────────────────────────────────────────┐
│                 YOUR APPLICATION                 │
├─────────────────────────────────────────────────┤
│   Frontend (React, Vue, Next.js, etc.)          │
│   ↓ GraphQL queries                             │
├─────────────────────────────────────────────────┤
│                     nself                        │
│                                                  │
│   ┌───────────────────────────────────────┐     │
│   │   Required Services (4)               │     │
│   │   PostgreSQL, Hasura, Auth, Nginx     │     │
│   └───────────────────────────────────────┘     │
│                                                  │
│   ┌───────────────────────────────────────┐     │
│   │   Optional Services (7)               │     │
│   │   Redis, MinIO, Search, Mail, etc.    │     │
│   └───────────────────────────────────────┘     │
│                                                  │
│   ┌───────────────────────────────────────┐     │
│   │   Your Custom Services                │     │
│   │   From 40+ templates                  │     │
│   └───────────────────────────────────────┘     │
└─────────────────────────────────────────────────┘
```

**[Learn More: Architecture](../architecture/ARCHITECTURE.md)**

## Tutorials

**By Use Case:**
- **[SaaS Application](../tutorials/QUICK-START-SAAS.md)** - Multi-tenant SaaS
- **[B2B Platform](../tutorials/QUICK-START-B2B.md)** - Enterprise features
- **[Marketplace](../tutorials/QUICK-START-MARKETPLACE.md)** - Two-sided marketplace
- **[Agency](../tutorials/QUICK-START-AGENCY.md)** - White-label platform

**By Feature:**
- **[OAuth Setup](../guides/OAUTH-SETUP.md)** - Social login (Google, GitHub, etc.)
- **[File Uploads](../tutorials/file-uploads-quickstart.md)** - Image uploads with thumbnails
- **[Real-time Features](../guides/REALTIME-FEATURES.md)** - Live updates and chat
- **[Custom Domains](../tutorials/CUSTOM-DOMAINS.md)** - Production domains with SSL
- **[Stripe Integration](../tutorials/STRIPE-INTEGRATION.md)** - Payment processing

## Guides

- **[Multi-App Setup](../guides/MULTI_APP_SETUP.md)** - Multiple apps in one stack
- **[Environments](../guides/ENVIRONMENTS.md)** - Local, staging, production
- **[Service Communication](../guides/SERVICE-TO-SERVICE-COMMUNICATION.md)** - Microservices
- **[Organization Management](../guides/ORGANIZATION-MANAGEMENT.md)** - Multi-tenancy
- **[Billing & Usage](../guides/BILLING-AND-USAGE.md)** - Usage tracking
- **[Security](../guides/SECURITY.md)** - Production security best practices
- **[Backup & Recovery](../guides/BACKUP-RECOVERY.md)** - Database backups

## Need Help?

- **[FAQ](../getting-started/FAQ.md)** - Common questions
- **[Troubleshooting](../guides/TROUBLESHOOTING.md)** - Common issues
- **[Doctor Command](../commands/DOCTOR.md)** - Automated diagnostics
- **[GitHub Issues](https://github.com/acamarata/nself/issues)** - Report bugs
- **[GitHub Discussions](https://github.com/acamarata/nself/discussions)** - Ask questions

## Platform Requirements

- **Docker** 20.10+ and Docker Compose 2.0+
- **Bash** 3.2+ (macOS default, all Linux distributions)
- **Git** (for deployments)
- **Operating System**: macOS, Linux, or WSL (Windows)

**[View Full Installation Guide](../getting-started/Installation.md)**

---

**Next Steps:**
1. **[Complete Installation](../getting-started/Installation.md)** - If you haven't already
2. **[Follow Quick Start](../getting-started/Quick-Start.md)** - Get your first project running
3. **[Design Your Database](../guides/DATABASE-WORKFLOW.md)** - Create your schema
4. **[Explore Examples](../examples/README.md)** - See real-world patterns

---

**Documentation Version**: v0.9.6
**Last Updated**: January 30, 2026
