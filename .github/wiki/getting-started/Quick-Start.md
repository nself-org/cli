# Quick Start Guide

Get a complete backend running in under 5 minutes.

---

## Prerequisites

| Requirement | Minimum | Check |
|-------------|---------|-------|
| **Bash** | 3.2+ | `bash --version` |
| **Docker Desktop** | v4.0+ | `docker --version` |
| **Docker Compose** | v2.0+ | `docker compose version` |
| **RAM** | 4 GB | - |
| **Disk** | 10 GB | - |

**Install Docker:** [docs.docker.com/get-docker](https://docs.docker.com/get-docker/)

**Note:** nself is fully compatible with Bash 3.2+ (the default on macOS). No need to upgrade Bash!

---

## Step 1: Install ɳSelf

```bash
curl -sSL https://install.nself.org | bash
```

Or manual installation:
```bash
git clone https://github.com/nself-org/cli.git ~/.nself
echo 'export PATH="$PATH:$HOME/.nself/bin"' >> ~/.bashrc
source ~/.bashrc
```

Verify installation:
```bash
nself version
```

---

## Step 2: Create a Project

```bash
mkdir myapp && cd myapp
nself init
```

You'll be asked a few questions (or accept defaults):
- Project name
- Domain (default: `local.nself.org`)
- Which services to enable

**What gets created:**
```
myapp/
├── .env           # Your configuration
├── .env.secrets   # Sensitive credentials (auto-generated)
└── nself/         # Project directory
```

---

## Step 3: Build and Start

```bash
nself build    # Generate docker-compose.yml, nginx configs, etc.
nself start    # Launch all services
```

**First start takes 2-5 minutes** (downloading Docker images).

Check status:
```bash
nself status   # Service health
nself urls     # Access URLs
```

---

## Step 4: Design Your Database (Recommended)

nself uses a **database-first** approach. Design your schema, and everything else follows.

### Option A: Use a Template

```bash
nself db schema scaffold basic    # Creates schema.dbml
```

Available templates:
- `basic` - Users, profiles, posts
- `ecommerce` - Products, orders, cart
- `saas` - Organizations, members, projects
- `blog` - Posts, categories, comments

### Option B: Design Visually

1. Go to [dbdiagram.io](https://dbdiagram.io)
2. Design your schema
3. Export as DBML
4. Save as `schema.dbml` in your project

### Apply Your Schema

```bash
nself db schema apply schema.dbml
```

This single command:
1. Creates SQL migration from your DBML
2. Runs the migration
3. Generates mock data for testing
4. Seeds sample users

**Sample users created (local/staging):**
- `admin@example.com` (admin role)
- `user@example.com` (user role)
- `demo@example.com` (viewer role)

---

## Step 5: Explore Your Backend

### Access Points

| Service | URL | Description |
|---------|-----|-------------|
| **GraphQL API** | https://api.local.nself.org | Hasura console + API |
| **Auth** | https://auth.local.nself.org | Authentication service |
| **Admin** | https://admin.local.nself.org | Admin dashboard (if enabled) |
| **Mail** | https://mail.local.nself.org | Email testing UI (if enabled) |

### Trust SSL Certificates

For HTTPS to work without browser warnings:

```bash
nself trust
```

### Try These Commands

```bash
# Check all URLs
nself urls

# View logs
nself logs

# Database shell
nself db shell

# Run a query
nself db query "SELECT * FROM users"

# Generate TypeScript types
nself db types
```

---

## Step 6: To Production (When Ready)

```bash
# Create production environment
nself config env create prod production

# Edit server configuration
# .environments/prod/server.json

# Deploy
nself deploy prod
```

> **v0.9.7+:** Environment commands moved to `nself config env`. Old syntax `nself env` still works.

See [Deployment Guide](../guides/Deployment.md) for complete instructions.

---

## Common Commands

### Everyday Use

```bash
nself start              # Start services
nself stop               # Stop services
nself restart            # Restart services
nself status             # Check health
nself logs               # View all logs
nself logs postgres      # View specific service
```

### Database

```bash
nself db migrate up      # Run migrations
nself db migrate create NAME  # Create migration
nself db seed            # Seed data
nself db backup          # Create backup
nself db restore         # Restore backup
nself db shell           # PostgreSQL shell
nself db types           # Generate types
```

### Project Management

```bash
nself build              # Regenerate configs
nself urls               # Show all URLs
nself doctor             # Diagnose issues
nself reset              # Reset to clean state
```

### Performance & Operations (v0.4.6)

```bash
nself perf               # Performance profiling
nself bench run          # Run benchmarks
nself health check       # Check service health
nself config show        # Show configuration
nself history show       # View operation history
```

---

## Project Structure

After `nself init && nself build`:

```
myapp/
├── .env                  # Main configuration
├── .env.secrets          # Sensitive credentials
├── schema.dbml           # Your database schema
├── docker-compose.yml    # Generated orchestration
├── nginx/                # Reverse proxy configs
├── postgres/             # Database initialization
│   └── migrations/       # SQL migrations
├── ssl/                  # SSL certificates
├── services/             # Custom services (if any)
└── nself/
    ├── migrations/       # Migration files
    ├── seeds/            # Seed data files
    ├── types/            # Generated types
    └── backups/          # Database backups
```

---

## Enable More Services

Edit `.env` and add:

```bash
# Enable Redis
REDIS_ENABLED=true

# Enable MinIO (S3 storage)
MINIO_ENABLED=true

# Enable search
MEILISEARCH_ENABLED=true

# Enable monitoring (10 services)
MONITORING_ENABLED=true

# Enable admin dashboard
NSELF_ADMIN_ENABLED=true
```

Then rebuild:
```bash
nself build && nself restart
```

---

## Troubleshooting

### Docker Not Running?

```bash
nself doctor           # Diagnose issues
nself doctor --fix     # Auto-fix common issues (v0.4.5+)
```

### Port Conflicts?

```bash
# Auto-fix reassigns ports
AUTO_FIX=true nself build
```

### Services Not Starting?

```bash
nself logs [service]     # Check logs
nself restart            # Try restarting
nself doctor             # Diagnose issues
```

### Can't Access URLs?

```bash
nself urls               # Verify URLs
nself trust              # Trust SSL certs
```

---

## Next Steps

| Guide | Description |
|-------|-------------|
| **[Database Workflow](../guides/DATABASE-WORKFLOW.md)** | Deep dive into DBML, migrations, seeding |
| **[Services Overview](../services/SERVICES.md)** | All available services |
| **[Custom Services](../services/SERVICES_CUSTOM.md)** | Build your own microservices |
| **[Deployment](../guides/Deployment.md)** | Go to production |
| **[FAQ](../getting-started/FAQ.md)** | Common questions |

---

## Help

- **[Troubleshooting](../guides/TROUBLESHOOTING.md)** - Common issues
- **[GitHub Issues](https://github.com/nself-org/cli/issues)** - Report bugs
- **[Discussions](https://github.com/nself-org/cli/discussions)** - Ask questions

---

*Next: [Database Workflow](../guides/DATABASE-WORKFLOW.md) - Design your database schema*
