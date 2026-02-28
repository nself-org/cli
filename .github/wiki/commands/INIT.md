# nself init - Project Initialization

**Version 0.9.9** | Create and configure new nself projects

---

## Overview

The `nself init` command creates a new nself project in the current directory. It generates all necessary configuration files, sets up directory structure, and optionally runs an interactive wizard to customize your setup.

---

## Table of Contents

- [Basic Usage](#basic-usage)
- [Initialization Modes](#initialization-modes)
- [Options Reference](#options-reference)
- [Generated Files](#generated-files)
- [Directory Structure](#directory-structure)
- [Environment Configuration](#environment-configuration)
- [Demo Mode](#demo-mode)
- [Templates](#templates)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Basic Usage

```bash
# Interactive initialization (recommended)
nself init

# Quick initialization with defaults
nself init --quick

# Demo mode with all services enabled
nself init --demo

# Minimal setup (just required services)
nself init --minimal
```

---

## Initialization Modes

### Interactive Mode (Default)

The interactive wizard guides you through configuration:

```bash
nself init
```

**Wizard Steps:**

1. **Project Name** - Used for Docker containers and database
2. **Domain Configuration** - Local domain or custom domain
3. **Environment Type** - Development, staging, or production
4. **Optional Services** - Select which services to enable
5. **Custom Services** - Add your own microservices
6. **Monitoring** - Enable observability stack
7. **Database Setup** - Initial schema and seed data

**Example Session:**

```
Welcome to nself!

Project name [myapp]:
Domain [local.nself.org]:
Environment [dev]:

Optional Services:
  [x] Redis - Cache and sessions
  [ ] MinIO - S3-compatible storage
  [x] Mail - Email service (MailPit)
  [ ] Search - MeiliSearch
  [ ] Functions - Serverless runtime
  [ ] MLflow - ML experiment tracking
  [ ] Admin - Web dashboard

Enable monitoring bundle? [y/N]: y

Custom services (CS_1 through CS_10):
  Add custom service? [y/N]: n

✓ Project initialized successfully!

Next steps:
  nself build    # Generate configuration
  nself start    # Start all services
```

### Quick Mode

Skip the wizard and use smart defaults:

```bash
nself init --quick
```

**Default Configuration:**
- Project name: Current directory name
- Domain: `local.nself.org`
- Environment: `dev`
- Optional services: None enabled
- Monitoring: Disabled

### Demo Mode

Create a fully-featured demo with all services:

```bash
nself init --demo
```

**Demo Configuration:**
- All 7 optional services enabled
- Full monitoring bundle (10 services)
- 4 sample custom services from templates
- 2 sample frontend app routes
- Sample database schema

**Total Services in Demo:** 25 containers

### Minimal Mode

Bare minimum for API development:

```bash
nself init --minimal
```

**Minimal Configuration:**
- Only required services (4)
- No optional services
- No monitoring
- No custom services

---

## Options Reference

| Option | Short | Description |
|--------|-------|-------------|
| `--quick` | `-q` | Skip wizard, use defaults |
| `--demo` | `-d` | Full demo configuration |
| `--minimal` | `-m` | Bare minimum setup |
| `--force` | `-f` | Overwrite existing files |
| `--no-git` | | Skip git initialization |
| `--template <name>` | `-t` | Use project template |
| `--name <name>` | `-n` | Set project name |
| `--domain <domain>` | | Set base domain |
| `--env <environment>` | `-e` | Set environment (dev/staging/prod) |

### Template Options

```bash
# Use a predefined template
nself init --template saas      # SaaS application
nself init --template api       # API-only backend
nself init --template ecommerce # E-commerce platform
nself init --template cms       # Content management
```

---

## Generated Files

### Root Directory

| File | Description |
|------|-------------|
| `.env` | Environment configuration |
| `.env.example` | Template for environment variables |
| `.gitignore` | Git ignore patterns |
| `docker-compose.yml` | Generated after `nself build` |

### Configuration Directories

```
project/
├── .env                      # Main configuration
├── .env.example              # Configuration template
├── .gitignore                # Git ignore rules
├── .nself/                   # nself metadata
│   └── project.json          # Project configuration
├── nginx/                    # Generated Nginx configs
├── postgres/                 # Database initialization
├── ssl/                      # SSL certificates
├── services/                 # Custom service code
│   ├── express_api/          # Example custom service
│   └── ...
├── monitoring/               # If monitoring enabled
│   ├── prometheus/
│   ├── grafana/
│   └── ...
└── schema.dbml               # If using schema workflow
```

---

## Directory Structure

### After `nself init`

```
myapp/
├── .env                      # Environment configuration
├── .env.example              # Configuration template
└── .gitignore                # Git ignore patterns
```

### After `nself build`

```
myapp/
├── .env
├── .env.example
├── .gitignore
├── docker-compose.yml        # Generated Docker Compose
├── nginx/
│   ├── nginx.conf            # Main Nginx config
│   ├── includes/             # Security headers, gzip
│   │   ├── security-headers.conf
│   │   ├── gzip.conf
│   │   └── proxy-params.conf
│   └── sites/                # Per-service configs
│       ├── hasura.conf
│       ├── auth.conf
│       └── ...
├── postgres/
│   └── init/
│       └── 00-init.sql       # Database initialization
├── ssl/
│   └── certificates/
│       └── localhost/
│           ├── fullchain.pem
│           └── privkey.pem
└── services/                 # Custom services (if any)
    └── ...
```

---

## Environment Configuration

The `.env` file is the primary configuration source. Key sections:

### Basic Configuration

```bash
# Project Identity
PROJECT_NAME=myapp
ENV=dev
BASE_DOMAIN=local.nself.org

# PostgreSQL
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=secure-password-here
POSTGRES_PORT=5432

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret-here
HASURA_GRAPHQL_JWT_SECRET={"type":"HS256","key":"your-jwt-secret-min-32-chars"}

# Auth
AUTH_SERVER_URL=https://auth.local.nself.org
```

### Optional Services

```bash
# Redis
REDIS_ENABLED=true
REDIS_PORT=6379

# MinIO (S3-compatible storage)
MINIO_ENABLED=true
MINIO_ROOT_USER=minio
MINIO_ROOT_PASSWORD=minio-password

# Mail (MailPit for development)
MAILPIT_ENABLED=true

# Search (MeiliSearch)
MEILISEARCH_ENABLED=true
MEILISEARCH_MASTER_KEY=search-key

# Functions runtime
FUNCTIONS_ENABLED=true

# MLflow
MLFLOW_ENABLED=true

# Admin dashboard
NSELF_ADMIN_ENABLED=true
```

### Monitoring Bundle

```bash
# Enable all 10 monitoring services
MONITORING_ENABLED=true

# Optional overrides
GRAFANA_ADMIN_PASSWORD=admin
PROMETHEUS_RETENTION=15d
```

### Custom Services

```bash
# Format: CS_N=name:template:port
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
CS_3=grpc:go-grpc:8003
CS_4=ml:python-fastapi:8004
```

---

## Demo Mode

Demo mode creates a showcase environment with everything enabled:

```bash
nself init --demo
```

### What's Included

| Category | Services |
|----------|----------|
| **Required (4)** | PostgreSQL, Hasura, Auth, Nginx |
| **Optional (7)** | Redis, MinIO, Mail, Search, Functions, MLflow, Admin |
| **Monitoring (10)** | Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager, cAdvisor, Node Exporter, Postgres Exporter, Redis Exporter |
| **Custom (4)** | Express API, BullMQ Worker, gRPC Service, FastAPI ML |

### Demo URLs

After `nself build && nself start`:

| Service | URL |
|---------|-----|
| GraphQL API | https://api.local.nself.org |
| Auth | https://auth.local.nself.org |
| Admin | https://admin.local.nself.org |
| Grafana | https://grafana.local.nself.org |
| Mail | https://mail.local.nself.org |
| Search | https://search.local.nself.org |

---

## Templates

### Available Templates

| Template | Description | Services |
|----------|-------------|----------|
| `default` | Standard development setup | 4 required |
| `api` | API-focused backend | Required + Redis |
| `saas` | SaaS application | Required + Redis + Mail + Admin |
| `ecommerce` | E-commerce platform | Required + Redis + Search + Mail |
| `cms` | Content management | Required + MinIO + Search |
| `ml` | Machine learning | Required + Redis + MLflow + Functions |
| `demo` | Everything enabled | All 25 services |

### Using Templates

```bash
# SaaS template
nself init --template saas

# With custom name
nself init --template saas --name my-saas-app

# Template + modifications
nself init --template api
# Then edit .env to enable additional services
```

---

## Examples

### API Development Project

```bash
mkdir my-api && cd my-api
nself init --quick --name my-api

# Edit .env to add Redis
echo "REDIS_ENABLED=true" >> .env

nself build && nself start
```

### Full SaaS Platform

```bash
mkdir my-saas && cd my-saas
nself init --template saas

# Add custom API service
echo "CS_1=api:nestjs-ts:3001" >> .env

nself build && nself start
```

### Microservices Architecture

```bash
mkdir platform && cd platform
nself init

# Configure multiple custom services
cat >> .env << 'EOF'
CS_1=users:express-js:3001
CS_2=orders:express-js:3002
CS_3=payments:express-js:3003
CS_4=notifications:bullmq-js:3004
CS_5=analytics:python-fastapi:3005
EOF

nself build && nself start
```

### Production-Ready Setup

```bash
mkdir production-app && cd production-app

# Start with minimal
nself init --minimal --env prod

# Add only what you need
cat >> .env << 'EOF'
REDIS_ENABLED=true
MONITORING_ENABLED=true
BASE_DOMAIN=app.mycompany.com
EOF

nself build
```

---

## Troubleshooting

### "Directory not empty"

```bash
# Use --force to overwrite
nself init --force

# Or clean first
rm -rf .env .env.example .gitignore .nself
nself init
```

### "Port already in use"

Edit `.env` to change default ports:

```bash
POSTGRES_PORT=5433     # Default: 5432
HASURA_PORT=8081       # Default: 8080
NGINX_HTTP_PORT=8080   # Default: 80
NGINX_HTTPS_PORT=8443  # Default: 443
```

### "Permission denied"

```bash
# Ensure write permissions
chmod 755 .
nself init

# Or run from writable directory
cd ~/projects
mkdir newapp && cd newapp
nself init
```

### Wizard Not Showing

```bash
# Check if running in interactive terminal
tty

# Force interactive mode
nself init --no-quick

# Or use quick mode
nself init --quick
```

---

## Environment Variable Reference

See the complete [Environment Variables](../configuration/ENVIRONMENT-VARIABLES.md) documentation for all available options.

---

## What's Next

After initialization:

```bash
# 1. Generate configuration
nself build

# 2. Start services
nself start

# 3. Check status
nself status

# 4. View URLs
nself urls

# 5. Design database (optional)
nself db schema scaffold basic
```

---

## Related Commands

- [build](BUILD.md) - Generate Docker and Nginx configuration
- [start](START.md) - Start all services
- [status](STATUS.md) - Check service status
- [urls](URLS.md) - View service URLs
- [db schema](DB.md#schema) - Database schema management

---

*Last Updated: January 2026 | Version 0.9.9*
