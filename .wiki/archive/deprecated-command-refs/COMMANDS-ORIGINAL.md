# nself Commands Reference

**Complete CLI Reference** | Version 1.0.0

The definitive guide to all nself commands, organized by logical categories with examples, cross-references, and migration guidance.

**BREAKING CHANGES:** v1.0 consolidates legacy command sprawl into a 32-command canonical runtime surface (31 grouped domains + `destroy`). See [Migration Guide](#migration-from-v09x) below.

---

## Table of Contents

- [Command Tree Overview](#command-tree-overview)
- [SPORT Command Matrix](SPORT-COMMAND-MATRIX.md)
- [Quick Start](#quick-start)
- [Core Commands](#core-commands)
- [Database Commands](#database-commands)
- [Multi-Tenant Commands](#multi-tenant-commands)
- [OAuth Commands](#oauth-commands)
- [Storage Commands](#storage-commands)
- [Service Management](#service-management)
- [Deployment Commands](#deployment-commands)
- [Cloud Infrastructure](#cloud-infrastructure)
- [Kubernetes & Helm](#kubernetes--helm)
- [Observability & Monitoring](#observability--monitoring)
- [Security Commands](#security-commands)
- [Performance & Optimization](#performance--optimization)
- [Developer Tools](#developer-tools)
- [Plugin System](#plugin-system)
- [Configuration](#configuration)
- [Utilities](#utilities)
- [Legacy Commands](#legacy-commands)
- [Global Options](#global-options)
- [Environment Variables](#environment-variables)
- [Exit Codes](#exit-codes)
- [Version History](#version-history)

---

## Command Tree Overview (v1.0)

**BREAKING CHANGES:** Commands consolidated into 31 primary domains + `destroy` (32 runtime canonical commands).

```
nself (32 runtime canonical commands)
│
├── Core (5)
│   ├── init                    # Initialize project
│   ├── build                   # Generate configs
│   ├── start                   # Start services
│   ├── stop                    # Stop services
│   └── restart                 # Restart services
│
├── Utilities (15)
│   ├── status                  # Service health
│   ├── logs                    # View logs
│   ├── help                    # Help system
│   ├── admin                   # Admin UI
│   ├── urls                    # Service URLs
│   ├── exec                    # Execute in container
│   ├── doctor                  # Diagnostics
│   ├── monitor                 # Monitoring dashboards
│   ├── health                  # Health checks
│   ├── version                 # Version info
│   ├── update                  # Update nself
│   ├── completion              # Shell completions
│   ├── metrics                 # Metrics & profiling
│   ├── history                 # Audit trail
│   └── audit                   # Audit logging
│
├── Infrastructure Safety (1)
│   └── destroy                 # Safe infra teardown
│
└── Other (11)
    ├── db                      # Database (11 subcommands)
    │   ├── migrate/schema/seed/mock
    │   └── backup/restore/shell/query/types/inspect/data
    │
    ├── tenant                  # Multi-tenancy (50+ subcommands)
    │   ├── init/create/list/show/update/suspend/activate/delete
    │   ├── member (add/remove/list/update/role/invite/accept)
    │   ├── setting (get/set/list/delete/reset)
    │   ├── billing (plans/subscribe/cancel/usage/invoice/payment/stripe/test)
    │   ├── org (create/list/show/members/delete)
    │   ├── branding (logo/colors/preview/reset)
    │   ├── domains (add/remove/verify/list/ssl/primary)
    │   ├── email (list/edit/preview/reset)
    │   └── themes (list/apply/customize/preview/reset)
    │
    ├── deploy                  # Deployment (23 subcommands)
    │   ├── staging/production/preview/canary/blue-green
    │   ├── rollback/upgrade/status/config/logs/history/promote
    │   ├── provision
    │   ├── server (create/destroy/list/status/ssh/add/remove)
    │   └── sync (push/pull/status)
    │
    ├── infra                   # Infrastructure (38 subcommands)
    │   ├── provider (list/init/validate/info/server/cost/deploy)
    │   ├── k8s (init/convert/apply/deploy/status/logs/scale/rollback/delete/cluster/namespace)
    │   └── helm (init/generate/install/upgrade/rollback/uninstall/list/status/values/template/package/repo)
    │
    ├── service                 # Services (43 subcommands)
    │   ├── list/enable/disable/status/restart/logs
    │   ├── init/scaffold/wizard/search
    │   ├── admin
    │   ├── storage (init/upload/list/delete/config/status/test/graphql-setup)
    │   ├── email (send/template/test/config)
    │   ├── search (init/index/query/config)
    │   ├── redis (init/flush/cli/stats)
    │   ├── functions (init/deploy/list/logs/invoke)
    │   ├── mlflow (init/ui/experiments/models)
    │   └── realtime (init/events/test)
    │
    ├── config                  # Configuration (20 subcommands)
    │   ├── show/edit/validate/export/import/sync
    │   ├── env (list/switch/create/delete/sync)
    │   ├── secrets (list/get/set/delete/rotate)
    │   └── vault (init/config/status)
    │
    ├── auth                    # Authentication & Security (38 subcommands)
    │   ├── login/logout/status
    │   ├── mfa (enable/disable/verify/backup-codes)
    │   ├── roles (list/create/assign/remove)
    │   ├── devices (list/register/revoke/trust)
    │   ├── oauth (install/enable/disable/config/test/list/status)
    │   ├── security (scan/audit/report)
    │   ├── ssl (generate/install/renew/info/trust)
    │   ├── rate-limit (config/status/reset)
    │   └── webhooks (create/list/delete/test/logs)
    │
    ├── perf                    # Performance (5 subcommands)
    │   ├── profile/bench/scale/migrate/optimize
    │
    ├── backup                  # Backup & Recovery (6 subcommands)
    │   ├── create/restore/list/rollback/reset/clean
    │
    ├── dev                     # Developer Tools (16 subcommands)
    │   ├── mode
    │   ├── frontend (add/remove/list/config)
    │   ├── ci (generate/update/templates)
    │   ├── docs (generate/serve/build)
    │   └── whitelabel (config/preview/deploy)
    │
    └── plugin                  # Plugin System (8+ subcommands)
        └── list/install/remove/update/updates/refresh/status/create/<plugin>
```

**Total: 32 runtime canonical commands with 285+ subcommands**

**For complete details:** See [COMMAND-TREE-V1.md](./COMMAND-TREE-V1.md)
**Runtime no-gap matrix:** See [SPORT-COMMAND-MATRIX.md](SPORT-COMMAND-MATRIX.md)

---

## Quick Start

### New Project Setup

```bash
# Initialize new project
nself init                      # Interactive wizard
nself init --demo               # Demo configuration
nself init --simple             # Simple wizard

# Build and start
nself build                     # Generate configs
nself start                     # Start all services

# Check status
nself status                    # Service health
nself urls                      # Service URLs
nself doctor                    # System diagnostics
```

### Daily Development Workflow

```bash
# Start working
nself start                     # Start services
nself status                    # Check health
nself logs -f                   # Follow logs

# Database operations
nself db shell                  # PostgreSQL shell
nself db migrate up             # Run migrations
nself db seed                   # Seed data

# Stop when done
nself stop                      # Stop all services
```

### Common Tasks

```bash
# View service URLs
nself urls

# View specific service logs
nself logs postgres
nself logs hasura -f

# Execute commands in containers
nself exec postgres psql -U postgres

# Restart a service
nself restart hasura

# System health check
nself doctor
nself doctor --fix              # Auto-fix issues
```

---

## Migration from v0.9.x

**v1.0 Breaking Changes:** Commands have been reorganized for better discoverability.

### Quick Migration Reference

| v0.9.x Command | v1.0 Command | Notes |
|----------------|--------------|-------|
| `nself billing` | `nself tenant billing` | Billing is tenant-specific |
| `nself org` | `nself tenant org` | Organizations under tenant |
| `nself upgrade` | `nself deploy upgrade` | Deployment operation |
| `nself staging` | `nself deploy staging` | Quick staging deployment |
| `nself prod` | `nself deploy production` | Quick prod deployment |
| `nself provision` | `nself deploy provision` | Server provisioning |
| `nself server` | `nself deploy server` | Server management |
| `nself provider` | `nself infra provider` | Cloud infrastructure |
| `nself cloud` | `nself infra provider` | Deprecated, use provider |
| `nself k8s` | `nself infra k8s` | Kubernetes operations |
| `nself helm` | `nself infra helm` | Helm charts |
| `nself storage` | `nself service storage` | Storage service |
| `nself email` | `nself service email` | Email service |
| `nself search` | `nself service search` | Search service |
| `nself redis` | `nself service redis` | Redis cache |
| `nself functions` | `nself service functions` | Functions service |
| `nself mlflow` | `nself service mlflow` | MLflow service |
| `nself env` | `nself config env` | Environment config |
| `nself secrets` | `nself config secrets` | Secrets management |
| `nself vault` | `nself config vault` | Vault integration |
| `nself validate` | `nself config validate` | Config validation |
| `nself mfa` | `nself auth mfa` | Multi-factor auth |
| `nself roles` | `nself auth roles` | Role management |
| `nself devices` | `nself auth devices` | Device management |
| `nself oauth` | `nself auth oauth` | OAuth providers |
| `nself security` | `nself auth security` | Security scanning |
| `nself ssl` | `nself auth ssl` | SSL certificates |
| `nself trust` | `nself auth ssl trust` | Trust certificates |
| `nself rate-limit` | `nself auth rate-limit` | Rate limiting |
| `nself webhooks` | `nself auth webhooks` | Webhook management |
| `nself bench` | `nself perf bench` | Benchmarking |
| `nself scale` | `nself perf scale` | Service scaling |
| `nself migrate` | `nself perf migrate` | Migration tools |
| `nself rollback` | `nself backup rollback` | Rollback changes |
| `nself reset` | `nself backup reset` | Reset environment |
| `nself clean` | `nself backup clean` | Clean resources |
| `nself frontend` | `nself dev frontend` | Frontend management |
| `nself ci` | `nself dev ci` | CI/CD generation |
| `nself docs` | `nself dev docs` | Documentation |
| `nself whitelabel` | `nself dev whitelabel` | White-label branding |

### Unchanged Commands

These commands remain at the top level:
- Core: `init`, `build`, `start`, `stop`, `restart`
- Utilities: `status`, `logs`, `help`, `admin`, `urls`, `exec`, `doctor`, `monitor`, `health`, `version`, `update`, `completion`, `metrics`, `history`, `audit`
- Platform: `db`, `tenant`, `deploy`, `infra`, `service`, `config`, `auth`, `perf`, `backup`, `dev`, `plugin`

### Automatic Aliases (Temporary)

For backward compatibility, all old commands are aliased with deprecation warnings until v2.0.

```bash
$ nself billing plans
⚠ DEPRECATED: 'nself billing' → use 'nself tenant billing'
This alias will be removed in v2.0.0.

[command continues normally...]
```

---

## Core Commands

Essential lifecycle commands for daily use.

### init

Initialize a new nself project with interactive wizard.

**Usage:**
```bash
nself init [OPTIONS]
```

**Options:**
- `--demo` - Create demo project with all features enabled
- `--simple` - Use simple wizard (fewer options)
- `--template <name>` - Use specific template
- `--name <name>` - Set project name
- `--domain <domain>` - Set base domain

**Examples:**
```bash
# Interactive wizard
nself init

# Demo project (all features)
nself init --demo

# Simple wizard
nself init --simple

# Specify project details
nself init --name myapp --domain myapp.local
```

**See:** [INIT.md](INIT.md)

---

### build

Generate configuration files and build Docker images.

**Usage:**
```bash
nself build [OPTIONS]
```

**Options:**
- `--clean` - Clean build (remove existing configs)
- `--no-cache` - Build without Docker cache
- `--service <name>` - Build specific service only
- `--verbose` - Show detailed build output

**Examples:**
```bash
# Build all services
nself build

# Clean build
nself build --clean

# Build without cache
nself build --no-cache

# Build specific service
nself build --service postgres
```

**See:** [BUILD.md](BUILD.md)

---

### start

Start all configured services with smart defaults.

**Usage:**
```bash
nself start [OPTIONS]
```

**Options:**
- `--fresh` - Force recreate all containers
- `--verbose` - Show detailed output
- `--debug` - Debug mode
- `--skip-health-checks` - Skip health validation
- `--timeout <seconds>` - Health check timeout (default: 120)
- `--clean-start` - Remove everything and start fresh

**Examples:**
```bash
# Smart start (default)
nself start

# Force recreate containers
nself start --fresh

# Quick start (skip health checks)
nself start --skip-health-checks

# Debugging startup issues
nself start --verbose --debug
```

**Environment Variables:**
```bash
NSELF_START_MODE=smart|fresh|force
NSELF_HEALTH_CHECK_TIMEOUT=120
NSELF_HEALTH_CHECK_REQUIRED=80
NSELF_SKIP_HEALTH_CHECKS=false
```

**See:** [START.md](START.md), [docs/configuration/START-COMMAND-OPTIONS.md](../configuration/START-COMMAND-OPTIONS.md)

---

### stop

Stop all running services.

**Usage:**
```bash
nself stop [SERVICE]
```

**Examples:**
```bash
# Stop all services
nself stop

# Stop specific service
nself stop postgres
nself stop hasura
```

**See:** [STOP.md](STOP.md)

---

### restart

Restart services.

**Usage:**
```bash
nself restart [SERVICE]
```

**Examples:**
```bash
# Restart all services
nself restart

# Restart specific service
nself restart hasura
nself restart nginx
```

**See:** [RESTART.md](RESTART.md)

---

### reset

Reset project to clean state (removes all data).

**Usage:**
```bash
nself reset [OPTIONS]
```

**Options:**
- `--force` - Skip confirmation
- `--keep-volumes` - Keep Docker volumes

**Examples:**
```bash
# Interactive reset (with confirmation)
nself reset

# Force reset (no confirmation)
nself reset --force
```

**Warning:** This command removes all Docker containers, volumes, and generated files. Use with caution.

**See:** [RESET.md](RESET.md)

---

### clean

Clean up Docker resources.

**Usage:**
```bash
nself clean [OPTIONS]
```

**Options:**
- `--all` - Remove everything (containers, images, volumes)
- `--images` - Clean images only
- `--volumes` - Clean volumes only
- `--force` - Skip confirmation

**Examples:**
```bash
# Clean unused resources
nself clean

# Clean everything
nself clean --all

# Clean images only
nself clean --images
```

**See:** [CLEAN.md](CLEAN.md)

---

## Database Commands

Comprehensive database management under the `db` namespace.

### db migrate

Migration management.

**Usage:**
```bash
nself db migrate [COMMAND] [OPTIONS]
```

**Commands:**
- `up` - Run pending migrations
- `down` - Rollback last migration
- `create <name>` - Create new migration
- `status` - Migration status
- `fresh` - Drop and recreate (dev only)

**Examples:**
```bash
# Run all pending migrations
nself db migrate up
nself db migrate

# Rollback last migration
nself db migrate down

# Create new migration
nself db migrate create add_users_table

# Check migration status
nself db migrate status

# Fresh migration (dev only)
nself db migrate fresh
```

---

### db schema

Schema operations with DBML support.

**Usage:**
```bash
nself db schema [COMMAND] [OPTIONS]
```

**Commands:**
- `scaffold <template>` - Create from template
- `import <file>` - Import DBML file
- `apply <file>` - Full workflow (import + migrate)
- `diagram` - Export to DBML

**Templates:**
- `saas` - Multi-tenant SaaS
- `ecommerce` - E-commerce platform
- `blog` - Blog/CMS
- `analytics` - Analytics platform

**Examples:**
```bash
# Create from template
nself db schema scaffold saas

# Import DBML file
nself db schema import schema.dbml

# Full workflow (import + migrate)
nself db schema apply schema.dbml

# Export current schema
nself db schema diagram > schema.dbml
```

---

### db seed

Seed database with sample data.

**Usage:**
```bash
nself db seed [DATASET] [OPTIONS]
```

**Datasets:**
- `(default)` - Run all seeds
- `users` - Seed users
- `create <name>` - Create seed file

**Examples:**
```bash
# Run all seeds
nself db seed

# Seed users only
nself db seed users

# Create new seed file
nself db seed create products
```

---

### db mock

Generate realistic mock data.

**Usage:**
```bash
nself db mock [COMMAND] [OPTIONS]
```

**Commands:**
- `(default)` - Generate mocks
- `auto` - Auto-generate from schema
- `--seed <n>` - Reproducible data

**Examples:**
```bash
# Generate mock data
nself db mock

# Auto-generate from schema
nself db mock auto

# Reproducible mock data
nself db mock --seed 12345
```

---

### db backup

Create database backups.

**Usage:**
```bash
nself db backup [OPTIONS]
```

**Options:**
- `--name <name>` - Custom backup name
- `--compress` - Compress backup

**Examples:**
```bash
# Create backup
nself db backup

# Named backup
nself db backup --name pre-migration

# List backups
nself db backup list
```

**See:** [BACKUP.md](BACKUP.md)

---

### db restore

Restore database from backup.

**Usage:**
```bash
nself db restore <backup-file>
```

**Examples:**
```bash
# Restore from backup
nself db restore backups/2026-01-30.sql

# Restore latest backup
nself db restore latest
```

**See:** [RESTORE.md](RESTORE.md)

---

### db shell

Interactive PostgreSQL shell.

**Usage:**
```bash
nself db shell [OPTIONS]
```

**Options:**
- `--readonly` - Read-only mode
- `--database <db>` - Specific database

**Examples:**
```bash
# Open psql shell
nself db shell

# Read-only shell
nself db shell --readonly
```

---

### db query

Execute SQL queries.

**Usage:**
```bash
nself db query <sql>
```

**Examples:**
```bash
# Execute query
nself db query "SELECT * FROM users LIMIT 10;"

# From file
nself db query -f query.sql
```

---

### db types

Generate TypeScript types from schema.

**Usage:**
```bash
nself db types [LANGUAGE] [OPTIONS]
```

**Languages:**
- `typescript` (default) - TypeScript interfaces
- `go` - Go structs
- `python` - Python classes

**Examples:**
```bash
# Generate TypeScript types
nself db types
nself db types typescript

# Generate Go structs
nself db types go

# Generate Python classes
nself db types python
```

---

### db inspect

Database inspection and analysis.

**Usage:**
```bash
nself db inspect [COMMAND]
```

**Commands:**
- `(default)` - Overview
- `size` - Table sizes
- `slow` - Slow queries

**Examples:**
```bash
# Database overview
nself db inspect

# Table sizes
nself db inspect size

# Slow query analysis
nself db inspect slow
```

---

### db data

Data operations.

**Usage:**
```bash
nself db data [COMMAND] [OPTIONS]
```

**Commands:**
- `export <table>` - Export table data
- `anonymize` - Anonymize PII

**Examples:**
```bash
# Export table
nself db data export users > users.csv

# Anonymize PII
nself db data anonymize
```

**See:** [DB.md](DB.md)

---

## Multi-Tenant Commands

Comprehensive multi-tenancy system (v0.9.0).

### tenant

Multi-tenant management with billing, branding, and domains.

**Usage:**
```bash
nself tenant [COMMAND] [OPTIONS]
```

**Core Commands:**
```bash
# Initialize multi-tenancy
nself tenant init

# Create tenant
nself tenant create "Acme Corp" --slug acme --plan pro

# List all tenants
nself tenant list

# Show tenant details
nself tenant show <tenant-id>

# Lifecycle management
nself tenant suspend <tenant-id>
nself tenant activate <tenant-id>
nself tenant delete <tenant-id>

# Statistics
nself tenant stats
```

---

### tenant member

Member management.

**Usage:**
```bash
nself tenant member [COMMAND]
```

**Examples:**
```bash
# Add user to tenant
nself tenant member add acme user@example.com admin

# Remove user from tenant
nself tenant member remove acme user@example.com

# List tenant members
nself tenant member list acme
```

---

### tenant setting

Settings management.

**Usage:**
```bash
nself tenant setting [COMMAND]
```

**Examples:**
```bash
# Set tenant setting
nself tenant setting set acme max_users 100

# Get tenant setting
nself tenant setting get acme max_users

# List all settings
nself tenant setting list acme
```

---

### tenant billing

Billing and subscription management.

**Usage:**
```bash
nself tenant billing [COMMAND]
```

**Commands:**
- `usage` - Usage statistics
- `invoice` - Invoice management
- `subscription` - Subscription management
- `payment` - Payment methods
- `quota` - Quota limits
- `plan` - Billing plans
- `export` - Export billing data
- `customer` - Customer management

**Examples:**
```bash
# View usage
nself tenant billing usage

# List invoices
nself tenant billing invoice list
nself tenant billing invoice show INV-001
nself tenant billing invoice download INV-001
nself tenant billing invoice pay INV-001

# Manage subscriptions
nself tenant billing subscription show
nself tenant billing subscription upgrade pro
nself tenant billing subscription downgrade basic

# Payment methods
nself tenant billing payment list
nself tenant billing payment add
nself tenant billing payment remove pm_xxx

# Check quotas
nself tenant billing quota

# View plans
nself tenant billing plan list
nself tenant billing plan show pro
nself tenant billing plan compare

# Export data
nself tenant billing export --format csv
```

**See:** [BILLING.md](BILLING.md)

---

### tenant branding

Brand customization.

**Usage:**
```bash
nself tenant branding [COMMAND]
```

**Examples:**
```bash
# Create brand
nself tenant branding create "Acme Brand"

# Set colors
nself tenant branding set-colors --primary #0066cc

# Set fonts
nself tenant branding set-fonts --heading "Montserrat"

# Upload logo
nself tenant branding upload-logo logo.png

# Custom CSS
nself tenant branding set-css custom.css

# Preview
nself tenant branding preview
```

---

### tenant domains

Custom domains and SSL certificates.

**Usage:**
```bash
nself tenant domains [COMMAND]
```

**Examples:**
```bash
# Add custom domain
nself tenant domains add app.example.com

# Verify domain ownership
nself tenant domains verify app.example.com

# Provision SSL certificate
nself tenant domains ssl app.example.com

# Check domain health
nself tenant domains health app.example.com

# Remove domain
nself tenant domains remove app.example.com
```

---

### tenant email

Email template management.

**Usage:**
```bash
nself tenant email [COMMAND]
```

**Examples:**
```bash
# List templates
nself tenant email list

# Edit template
nself tenant email edit welcome

# Preview template
nself tenant email preview welcome

# Send test email
nself tenant email test welcome user@example.com

# Set language
nself tenant email set-language en
```

---

### tenant themes

Theme management.

**Usage:**
```bash
nself tenant themes [COMMAND]
```

**Examples:**
```bash
# Create theme
nself tenant themes create "Dark Mode"

# Edit theme
nself tenant themes edit dark-mode

# Activate theme
nself tenant themes activate dark-mode

# Preview theme
nself tenant themes preview dark-mode

# Export theme
nself tenant themes export dark-mode > theme.json

# Import theme
nself tenant themes import theme.json
```

**See:** [TENANT.md](TENANT.md)

---

## OAuth Commands

OAuth provider management (v0.9.0).

### oauth

Manage OAuth authentication providers.

**Usage:**
```bash
nself oauth [COMMAND] [OPTIONS]
```

**Providers:**
- Google
- GitHub
- Slack
- Microsoft

**Commands:**

```bash
# Install OAuth service
nself oauth install

# Enable providers
nself oauth enable --providers google,github,slack

# Disable providers
nself oauth disable --providers slack

# Configure provider
nself oauth config google \
  --client-id=xxx.apps.googleusercontent.com \
  --client-secret=GOCSPX-xxx

# Microsoft (requires tenant ID)
nself oauth config microsoft \
  --client-id=xxx \
  --client-secret=xxx \
  --tenant-id=xxx

# Test provider configuration
nself oauth test google

# List all providers
nself oauth list

# Service status
nself oauth status
```

**See:** [OAUTH.md](OAUTH.md)

---

## Storage Commands

File storage and upload pipeline (v0.9.0).

### storage

Manage file uploads with advanced features.

**Usage:**
```bash
nself storage [COMMAND] [OPTIONS]
```

**Features:**
- Multipart upload for large files
- Automatic thumbnail generation
- Virus scanning
- Image compression
- GraphQL integration

**Commands:**

```bash
# Initialize storage system
nself storage init

# Upload files
nself storage upload photo.jpg
nself storage upload avatar.png --thumbnails
nself storage upload large-file.mp4 --dest videos/
nself storage upload doc.pdf --virus-scan
nself storage upload image.jpg --compression
nself storage upload file.txt --all-features

# List files
nself storage list
nself storage list users/123/

# Delete file
nself storage delete users/123/file.txt

# Configuration
nself storage config

# Pipeline status
nself storage status

# Test uploads
nself storage test

# Generate GraphQL integration
nself storage graphql-setup
```

**See:** [storage.md](storage.md), [File Upload Guide](../guides/file-upload-pipeline.md)

---

## Service Management

Manage optional services and custom services.

### service

Unified service management.

**Usage:**
```bash
nself service [COMMAND] [SERVICE]
```

**Core Commands:**

```bash
# List optional services
nself service list

# Enable/disable services
nself service enable redis
nself service disable minio

# Service status
nself service status
nself service status redis

# Restart service
nself service restart hasura

# View logs
nself service logs postgres -f

# Initialize from template
nself service init

# Scaffold new service
nself service scaffold

# Service creation wizard
nself service wizard

# Search services
nself service search
```

---

### service admin

Admin UI management.

**Usage:**
```bash
nself service admin [COMMAND]
```

**Examples:**
```bash
# Admin UI status
nself service admin status

# Open admin UI
nself service admin open

# User management
nself service admin users

# Admin configuration
nself service admin config

# Development mode
nself service admin dev
```

**See:** [ADMIN.md](ADMIN.md), [ADMIN-DEV.md](ADMIN-DEV.md)

---

### service email

Email service management.

**Usage:**
```bash
nself service email [COMMAND]
```

**Examples:**
```bash
# Send test email
nself service email test

# Open MailPit inbox
nself service email inbox

# Email configuration
nself service email config
```

**See:** [EMAIL.md](EMAIL.md)

---

### service search

Search service management.

**Usage:**
```bash
nself service search [COMMAND]
```

**Examples:**
```bash
# Reindex data
nself service search index

# Run query
nself service search query "search term"

# Index statistics
nself service search stats
```

**Supported Engines:**
- PostgreSQL (built-in)
- MeiliSearch
- Typesense
- Sonic
- ElasticSearch
- Algolia

**See:** [SEARCH.md](SEARCH.md)

---

### service functions

Serverless functions runtime.

**Usage:**
```bash
nself service functions [COMMAND]
```

**Examples:**
```bash
# Deploy all functions
nself service functions deploy

# Invoke function
nself service functions invoke my-function

# View logs
nself service functions logs
nself service functions logs my-function

# List functions
nself service functions list
```

**See:** [FUNCTIONS.md](FUNCTIONS.md)

---

### service mlflow

ML experiment tracking.

**Usage:**
```bash
nself service mlflow [COMMAND]
```

**Examples:**
```bash
# Open MLflow UI
nself service mlflow ui

# List experiments
nself service mlflow experiments

# List runs
nself service mlflow runs

# Browse artifacts
nself service mlflow artifacts
```

**See:** [MLFLOW.md](MLFLOW.md)

---

### service storage

Object storage (MinIO).

**Usage:**
```bash
nself service storage [COMMAND]
```

**Examples:**
```bash
# List buckets
nself service storage buckets

# Upload file
nself service storage upload file.txt my-bucket

# Download file
nself service storage download my-bucket/file.txt

# Generate presigned URL
nself service storage presign my-bucket/file.txt
```

---

### service cache

Redis cache management.

**Usage:**
```bash
nself service cache [COMMAND]
```

**Examples:**
```bash
# Cache statistics
nself service cache stats

# Flush cache
nself service cache flush

# List keys
nself service cache keys
```

**See:** [SERVICE.md](SERVICE.md)

---

## Deployment Commands

Environment and deployment management with advanced strategies.

### deploy

Deploy to environments with multiple strategies.

**Usage:**
```bash
nself deploy [ENVIRONMENT] [OPTIONS]
```

**Environments:**
```bash
# Deploy to staging
nself deploy staging

# Deploy to production
nself deploy production
```

**Advanced Strategies:**

```bash
# Preview environments
nself deploy preview                    # Create preview
nself deploy preview list               # List previews
nself deploy preview destroy PR-123     # Destroy preview

# Canary deployment
nself deploy canary                     # Start canary (20%)
nself deploy canary --percentage 50     # Increase to 50%
nself deploy canary promote             # Promote to 100%
nself deploy canary rollback            # Rollback canary
nself deploy canary status              # Canary status

# Blue-green deployment
nself deploy blue-green                 # Deploy to inactive
nself deploy blue-green switch          # Switch traffic
nself deploy blue-green rollback        # Rollback switch
nself deploy blue-green status          # Show active

# Rollback
nself deploy rollback                   # Rollback last deployment

# Pre-deploy validation
nself deploy check                      # Validate before deploy
nself deploy check --fix                # Auto-fix issues

# Deployment status
nself deploy status
```

**See:** [DEPLOY.md](DEPLOY.md)

---

### env

Environment management.

**Usage:**
```bash
nself env [COMMAND]
```

**Commands:**
```bash
# List environments
nself env
nself env list

# Create environment
nself env create prod production

# Switch environment
nself env switch staging
nself env switch prod

# Compare environments
nself env diff staging prod

# Validate configuration
nself env validate

# Check access level
nself env access
nself env access --check staging
nself env access --check prod
```

**Access Levels:**
- **Dev** - Local only
- **Sr Dev** - Local + Staging
- **Lead Dev** - Local + Staging + Production

**See:** [ENV.md](ENV.md)

---

### sync

Data synchronization between environments.

**Usage:**
```bash
nself sync [TYPE] [SOURCE] [TARGET]
```

**Types:**
- `db` - Database synchronization
- `files` - File synchronization
- `config` - Configuration sync
- `full` - Full sync (all of above)

**Commands:**
```bash
# Sync database
nself sync db staging prod

# Sync files
nself sync files staging prod

# Sync configuration
nself sync config staging prod

# Full sync
nself sync full staging prod

# Pull from staging
nself sync pull staging

# Pull from production (Lead Dev only)
nself sync pull prod
nself sync pull secrets

# Auto-sync
nself sync auto --setup           # Configure service
nself sync auto --stop            # Stop auto-sync

# Watch mode
nself sync watch                  # Watch for changes
nself sync watch --path /data     # Watch specific path
nself sync watch --interval 5     # Polling interval

# Sync status
nself sync status

# Sync history
nself sync history
```

**See:** [SYNC.md](SYNC.md)

---

### Legacy Deployment Shortcuts

**staging** - Deploy to staging (legacy alias)
```bash
nself staging          # Same as: nself deploy staging
nself staging status
```

**prod** - Deploy to production (legacy alias)
```bash
nself prod             # Same as: nself deploy production
nself prod status
```

**See:** [STAGING.md](STAGING.md), [PROD.md](PROD.md)

---

## Cloud Infrastructure

Manage cloud providers and infrastructure (v0.4.7).

### provider

Cloud provider operations.

**Usage:**
```bash
nself provider [COMMAND] [OPTIONS]
```

**Supported Providers (26+):**
- AWS, GCP, Azure, DigitalOcean
- Linode, Vultr, Hetzner, OVH
- Scaleway, UpCloud, and 16+ more

**Commands:**

```bash
# List all providers
nself provider list

# Configure provider credentials
nself provider init aws
nself provider init digitalocean

# Validate configuration
nself provider validate

# Provider information
nself provider info aws
```

---

### provider server

Server provisioning and management.

**Usage:**
```bash
nself provider server [COMMAND] [OPTIONS]
```

**Examples:**
```bash
# Provision server
nself provider server create digitalocean
nself provider server create aws --size medium

# Destroy server
nself provider server destroy myserver

# List servers
nself provider server list

# Server status
nself provider server status
nself provider server status myserver

# SSH to server
nself provider server ssh myserver

# Add existing server
nself provider server add 192.168.1.100

# Remove from registry
nself provider server remove myserver
```

---

### provider cost

Cost estimation and comparison.

**Usage:**
```bash
nself provider cost [COMMAND]
```

**Examples:**
```bash
# Estimate costs
nself provider cost estimate digitalocean

# Compare all providers
nself provider cost compare
```

---

### provider deploy

Quick deployment workflows.

**Usage:**
```bash
nself provider deploy [COMMAND]
```

**Examples:**
```bash
# Quick deploy (provision + deploy)
nself provider deploy quick digitalocean

# Full production setup
nself provider deploy full aws
```

**Legacy Aliases:**
- `nself providers` → `nself provider`
- `nself provision` → `nself provider server create`
- `nself servers` → `nself provider server`
- `nself cloud` → `nself provider` (legacy alias)

**See:** [PROVIDER.md](PROVIDER.md), [PROVIDERS.md](PROVIDERS.md), [PROVISION.md](PROVISION.md), [SERVERS.md](SERVERS.md)

---

## Kubernetes & Helm

Kubernetes and Helm chart management (v0.4.7).

### k8s

Kubernetes operations.

**Usage:**
```bash
nself k8s [COMMAND] [OPTIONS]
```

**Commands:**

```bash
# Initialize K8s config
nself k8s init

# Convert docker-compose to K8s manifests
nself k8s convert
nself k8s convert --output ./k8s
nself k8s convert --namespace myapp

# Apply manifests
nself k8s apply
nself k8s apply --dry-run

# Full deployment
nself k8s deploy
nself k8s deploy --env staging

# Deployment status
nself k8s status

# Pod logs
nself k8s logs postgres
nself k8s logs hasura -f

# Scale deployment
nself k8s scale postgres 3

# Rollback deployment
nself k8s rollback hasura

# Delete deployment
nself k8s delete

# Cluster management
nself k8s cluster list
nself k8s cluster connect mycluster
nself k8s cluster info

# Namespace management
nself k8s namespace list
nself k8s namespace create myapp
nself k8s namespace delete myapp
nself k8s namespace switch myapp
```

**See:** [K8S.md](K8S.md)

---

### helm

Helm chart management.

**Usage:**
```bash
nself helm [COMMAND] [OPTIONS]
```

**Commands:**

```bash
# Initialize Helm chart
nself helm init
nself helm init --from-compose

# Generate/update chart
nself helm generate

# Install to cluster
nself helm install
nself helm install --env staging

# Upgrade release
nself helm upgrade

# Rollback release
nself helm rollback

# Uninstall release
nself helm uninstall

# List releases
nself helm list

# Release status
nself helm status

# Show/edit values
nself helm values

# Render locally
nself helm template

# Package chart
nself helm package

# Repository management
nself helm repo add myrepo https://charts.example.com
nself helm repo remove myrepo
nself helm repo update
nself helm repo list
```

**See:** [HELM.md](HELM.md)

---

## Observability & Monitoring

Service monitoring, logging, and diagnostics.

### status

Show service health and status.

**Usage:**
```bash
nself status [SERVICE] [OPTIONS]
```

**Options:**
- `--json` - JSON output
- `--watch` - Continuous monitoring
- `--all-envs` - All environments

**Examples:**
```bash
# All services
nself status

# Specific service
nself status postgres

# JSON output
nself status --json

# Watch mode
nself status --watch

# All environments
nself status --all-envs
```

**See:** [STATUS.md](STATUS.md)

---

### logs

View service logs.

**Usage:**
```bash
nself logs [SERVICE] [OPTIONS]
```

**Options:**
- `-f, --follow` - Follow logs
- `--tail <n>` - Last N lines
- `--since <time>` - Since timestamp

**Examples:**
```bash
# All services
nself logs

# Specific service
nself logs postgres

# Follow logs
nself logs -f
nself logs hasura -f

# Last 100 lines
nself logs --tail 100

# Since timestamp
nself logs --since 1h
```

**See:** [LOGS.md](LOGS.md)

---

### exec

Execute commands in containers.

**Usage:**
```bash
nself exec <service> <command>
```

**Examples:**
```bash
# PostgreSQL shell
nself exec postgres psql -U postgres

# Hasura console
nself exec hasura hasura-cli console

# Bash shell
nself exec postgres bash

# Redis CLI
nself exec redis redis-cli
```

**See:** [EXEC.md](EXEC.md)

---

### urls

Show service URLs.

**Usage:**
```bash
nself urls [OPTIONS]
```

**Options:**
- `--env <env>` - Environment-specific
- `--diff` - Compare environments
- `--json` - JSON output

**Examples:**
```bash
# All URLs
nself urls

# Environment-specific
nself urls --env staging

# Compare environments
nself urls --diff staging prod

# JSON output
nself urls --json
```

**See:** [URLS.md](URLS.md)

---

### doctor

Run system diagnostics.

**Usage:**
```bash
nself doctor [OPTIONS]
```

**Options:**
- `--fix` - Auto-fix issues
- `--check <category>` - Check specific category

**Categories:**
- `deps` - Dependencies
- `docker` - Docker setup
- `network` - Network configuration
- `services` - Service health

**Examples:**
```bash
# Full diagnostic
nself doctor

# Auto-fix issues
nself doctor --fix

# Check dependencies
nself doctor --check deps
```

**See:** [DOCTOR.md](DOCTOR.md)

---

### health

Health check management (v0.4.6).

**Usage:**
```bash
nself health [COMMAND] [OPTIONS]
```

**Commands:**
```bash
# Run all health checks
nself health check

# Check specific service
nself health service postgres

# Check custom endpoint
nself health endpoint https://api.example.com

# Continuous monitoring
nself health watch

# Health history
nself health history

# Health configuration
nself health config
```

**See:** [HEALTH.md](HEALTH.md)

---

### monitor

Dashboard access.

**Usage:**
```bash
nself monitor [SERVICE]
```

**Examples:**
```bash
# Open Grafana
nself monitor
nself monitor grafana

# Open Prometheus
nself monitor prometheus

# Open Alertmanager
nself monitor alertmanager
```

**See:** [MONITOR.md](MONITOR.md)

---

### metrics

Monitoring profiles.

**Usage:**
```bash
nself metrics [COMMAND]
```

**Profiles:**
- `minimal` - Basic monitoring
- `standard` - Standard monitoring
- `full` - Full observability stack
- `auto` - Auto-configure based on usage

**Examples:**
```bash
# Set monitoring profile
nself metrics profile minimal
nself metrics profile standard
nself metrics profile full
nself metrics profile auto

# View metrics
nself metrics view
```

**See:** [METRICS.md](METRICS.md)

---

### history

Audit trail and deployment history.

**Usage:**
```bash
nself history [COMMAND] [OPTIONS]
```

**Commands:**
```bash
# Show recent history
nself history
nself history show

# Deployment history
nself history deployments

# Migration history
nself history migrations

# Rollback history
nself history rollbacks

# Command history
nself history commands

# Search history
nself history search "deploy production"

# Export history
nself history export

# Clear history
nself history clear
```

**See:** [HISTORY.md](HISTORY.md)

---

### audit

Audit logging for security and compliance.

**Usage:**
```bash
nself audit [COMMAND]
```

**Examples:**
```bash
# Audit logs
nself audit logs

# User events
nself audit events user@example.com
```

**See:** [AUDIT.md](AUDIT.md)

---

## Security Commands

Security scanning, secrets management, and access control.

### security

Security scanning and management.

**Usage:**
```bash
nself security [COMMAND]
```

**Commands:**
```bash
# Full security scan
nself security scan

# Password strength check
nself security scan passwords

# MFA coverage check
nself security scan mfa

# Suspicious activity detection
nself security scan suspicious

# Device management
nself security devices

# Security incidents
nself security incidents

# User security events
nself security events user@example.com

# WebAuthn key management
nself security webauthn
```

**See:** [docs/commands/security.md](security.md)

---

### auth

Authentication management.

**Usage:**
```bash
nself auth [COMMAND]
```

**Examples:**
```bash
# List users
nself auth users

# List roles
nself auth roles

# Auth providers
nself auth providers
```

**See:** [AUTH.md](AUTH.md)

---

### mfa

Multi-factor authentication.

**Usage:**
```bash
nself mfa [COMMAND]
```

**Examples:**
```bash
# Enable MFA
nself mfa enable

# Disable MFA
nself mfa disable

# MFA status
nself mfa status
```

**See:** [MFA.md](MFA.md)

---

### roles

Role management.

**Usage:**
```bash
nself roles [COMMAND]
```

**Examples:**
```bash
# List roles
nself roles list

# Create role
nself roles create editor

# Assign role
nself roles assign user@example.com editor
```

**See:** [docs/commands/roles.md](roles.md)

---

### devices

Device management and approval.

**Usage:**
```bash
nself devices [COMMAND]
```

**Examples:**
```bash
# List devices
nself devices list

# Approve device
nself devices approve device-id-123

# Revoke device
nself devices revoke device-id-123
```

**See:** [DEVICES.md](DEVICES.md)

---

### secrets

Secrets management.

**Usage:**
```bash
nself secrets [COMMAND]
```

**Examples:**
```bash
# List secrets
nself secrets list

# Add secret
nself secrets add API_KEY value

# Rotate secrets
nself secrets rotate
```

**See:** [docs/commands/secrets.md](secrets.md)

---

### vault

HashiCorp Vault integration.

**Usage:**
```bash
nself vault [COMMAND]
```

**Examples:**
```bash
# Initialize Vault
nself vault init

# Vault status
nself vault status

# Unseal Vault
nself vault unseal
```

**See:** [docs/commands/vault.md](vault.md)

---

### ssl

SSL certificate management.

**Usage:**
```bash
nself ssl [COMMAND]
```

**Examples:**
```bash
# Generate self-signed certificate
nself ssl generate

# Renew Let's Encrypt certificate
nself ssl renew

# Certificate information
nself ssl info
```

**See:** [SSL.md](SSL.md)

---

### trust

Trust local SSL certificates.

**Usage:**
```bash
nself trust [OPTIONS]
```

**Options:**
- `--system` - Add to system keychain

**Examples:**
```bash
# Trust local certificates
nself trust

# Add to system keychain (requires sudo)
nself trust --system
```

**See:** [TRUST.md](TRUST.md)

---

### rate-limit

Rate limiting configuration.

**Usage:**
```bash
nself rate-limit [COMMAND]
```

**Examples:**
```bash
# Configure rate limits
nself rate-limit config

# Show current limits
nself rate-limit status
```

**See:** [docs/commands/rate-limit.md](rate-limit.md)

---

### webhooks

Webhook management.

**Usage:**
```bash
nself webhooks [COMMAND]
```

**Examples:**
```bash
# List webhooks
nself webhooks list

# Test webhook
nself webhooks test https://example.com/webhook
```

**See:** [docs/commands/webhooks.md](webhooks.md)

---

## Performance & Optimization

Performance profiling, benchmarking, and scaling (v0.4.6).

### perf

Performance profiling and analysis.

**Usage:**
```bash
nself perf [COMMAND]
```

**Commands:**
```bash
# System profile
nself perf profile
nself perf profile postgres

# Analyze performance
nself perf analyze

# Slow query analysis
nself perf slow-queries

# Generate report
nself perf report

# Real-time dashboard
nself perf dashboard

# Optimization suggestions
nself perf suggest
```

**See:** [PERF.md](PERF.md)

---

### bench

Benchmarking and load testing.

**Usage:**
```bash
nself bench [COMMAND] [TARGET]
```

**Commands:**
```bash
# Run benchmark
nself bench run
nself bench run api
nself bench run db

# Establish baseline
nself bench baseline

# Compare to baseline
nself bench compare results.json

# Stress test
nself bench stress api --users 1000

# Benchmark report
nself bench report
```

**See:** [BENCH.md](BENCH.md)

---

### scale

Service scaling and autoscaling.

**Usage:**
```bash
nself scale [SERVICE] [OPTIONS]
```

**Options:**
- `--cpu <n>` - CPU scaling
- `--memory <size>` - Memory scaling
- `--replicas <n>` - Horizontal scaling
- `--auto` - Enable autoscaling
- `--min <n>` - Min replicas
- `--max <n>` - Max replicas

**Examples:**
```bash
# Vertical scaling
nself scale postgres --cpu 2
nself scale postgres --memory 4G

# Horizontal scaling
nself scale hasura --replicas 3

# Autoscaling
nself scale hasura --auto --min 2 --max 10

# Scale status
nself scale status
```

**See:** [SCALE.md](SCALE.md)

---

### migrate

Cross-environment migration tool.

**Usage:**
```bash
nself migrate [SOURCE] [TARGET] [OPTIONS]
```

**Commands:**
```bash
# Migrate environments
nself migrate staging prod
nself migrate --dry-run staging prod

# Show differences
nself migrate diff staging prod

# Continuous sync
nself migrate sync staging prod

# Rollback migration
nself migrate rollback
```

**Platform Migrations (v0.8.0):**
```bash
# Migrate from Firebase
nself migrate from firebase

# Migrate from Supabase
nself migrate from supabase
```

**See:** [MIGRATE.md](MIGRATE.md)

---

## Developer Tools

Tools for developers: testing, SDK generation, documentation.

### dev

Developer experience tools (v0.8.0).

**Usage:**
```bash
nself dev [COMMAND]
```

**SDK Generation:**
```bash
# Generate TypeScript SDK
nself dev sdk generate typescript

# Generate Python SDK
nself dev sdk generate python ./my-sdk
```

**Documentation:**
```bash
# Generate API documentation
nself dev docs generate

# Generate OpenAPI spec
nself dev docs openapi
```

**Testing:**
```bash
# Initialize test environment
nself dev test init

# Generate test fixtures
nself dev test fixtures users 50

# Mock data factory
nself dev test factory users

# Test snapshots
nself dev test snapshot create baseline

# Run integration tests
nself dev test run
```

**Mock Data:**
```bash
# Generate mock users
nself dev mock users 100
```

**See:** [DEV.md](DEV.md)

---

### frontend

Frontend application management.

**Usage:**
```bash
nself frontend [COMMAND]
```

**Commands:**
```bash
# Frontend status
nself frontend status

# List frontends
nself frontend list

# Add frontend
nself frontend add app1

# Remove frontend
nself frontend remove app1

# Deploy frontend
nself frontend deploy app1

# Deploy logs
nself frontend logs app1

# Environment variables
nself frontend env app1
```

**See:** [FRONTEND.md](FRONTEND.md)

---

### ci

CI/CD workflow generation.

**Usage:**
```bash
nself ci [COMMAND] [PLATFORM]
```

**Supported Platforms:**
- GitHub Actions
- GitLab CI
- CircleCI
- Jenkins

**Examples:**
```bash
# Initialize CI/CD
nself ci init

# GitHub Actions
nself ci init github

# GitLab CI
nself ci init gitlab

# Validate configuration
nself ci validate

# CI status
nself ci status
```

**See:** [CI.md](CI.md)

---

### completion

Shell completion scripts.

**Usage:**
```bash
nself completion [SHELL]
```

**Supported Shells:**
- bash
- zsh
- fish

**Examples:**
```bash
# Bash completion
nself completion bash

# Zsh completion
nself completion zsh

# Fish completion
nself completion fish

# Auto-install
nself completion install bash
```

**See:** [COMPLETION.md](COMPLETION.md)

---

### docs

Documentation generation.

**Usage:**
```bash
nself docs [COMMAND]
```

**Examples:**
```bash
# Generate documentation
nself docs generate

# OpenAPI specification
nself docs openapi
```

**See:** [docs/commands/docs.md](docs.md)

---

## Plugin System

Third-party integrations via plugins (v0.4.8).

### plugin

Plugin management and execution.

**Usage:**
```bash
nself plugin [COMMAND]
```

**Core Commands:**

```bash
# List available plugins
nself plugin list
nself plugin list --installed
nself plugin list --category payments

# Install plugin
nself plugin install stripe
nself plugin install github

# Remove plugin
nself plugin remove stripe
nself plugin remove stripe --keep-data

# Update plugins
nself plugin update stripe
nself plugin update --all

# Check for updates
nself plugin updates

# Refresh registry cache
nself plugin refresh

# Plugin status
nself plugin status
nself plugin status stripe

# Initialize plugin template (for developers)
nself plugin init
```

---

### Available Plugins

**Stripe Plugin** (Payment Processing)
```bash
# Install and configure
nself plugin install stripe
nself plugin stripe init

# Sync data
nself plugin stripe sync

# Customer management
nself plugin stripe customers list
nself plugin stripe customers show cus_xxx

# Subscription management
nself plugin stripe subscriptions list
nself plugin stripe subscriptions show sub_xxx

# Invoice management
nself plugin stripe invoices list
nself plugin stripe invoices show inv_xxx

# Webhook management
nself plugin stripe webhook status
nself plugin stripe webhook test

# Verify sync
nself plugin stripe check
```

**GitHub Plugin** (DevOps Integration)
```bash
# Install and configure
nself plugin install github
nself plugin github init

# Sync repositories
nself plugin github sync

# Repository management
nself plugin github repos list

# Issue tracking
nself plugin github issues list
nself plugin github issues show 123

# Pull requests
nself plugin github prs list
nself plugin github prs show 456

# GitHub Actions workflows
nself plugin github workflows list

# Webhook events
nself plugin github webhook status
```

**Shopify Plugin** (E-commerce)
```bash
# Install and configure
nself plugin install shopify
nself plugin shopify init

# Sync store data
nself plugin shopify sync

# Product catalog
nself plugin shopify products list
nself plugin shopify products show prod_xxx

# Order management
nself plugin shopify orders list
nself plugin shopify orders show order_xxx

# Customer data
nself plugin shopify customers list

# Webhook events
nself plugin shopify webhook status
```

**See:** [PLUGIN.md](PLUGIN.md), [Plugin Directory](../plugins/index.md)

---

## Configuration

Configuration and environment management.

### config

Configuration management.

**Usage:**
```bash
nself config [COMMAND]
```

**Commands:**
```bash
# Show configuration
nself config show

# Get specific value
nself config get PROJECT_NAME

# Set value
nself config set PROJECT_NAME myapp

# List all config keys
nself config list

# Open in editor
nself config edit

# Validate configuration
nself config validate

# Compare environments
nself config diff staging prod

# Export configuration
nself config export > config.json

# Import configuration
nself config import config.json

# Reset to defaults
nself config reset
```

**See:** [CONFIG.md](CONFIG.md)

---

### validate

Validate project configuration.

**Usage:**
```bash
nself validate [OPTIONS]
```

**Options:**
- `--fix` - Auto-fix issues
- `--strict` - Strict validation

**Examples:**
```bash
# Validate configuration
nself validate

# Auto-fix issues
nself validate --fix
```

**See:** [docs/commands/validate.md](validate.md)

---

## Utilities

Essential utilities and helpers.

### help

Show help information.

**Usage:**
```bash
nself help [COMMAND]
```

**Examples:**
```bash
# General help
nself help

# Command-specific help
nself help db
nself help deploy

# Alternative syntax
nself db --help
nself deploy --help
```

**See:** [HELP.md](HELP.md)

---

### version

Show version information.

**Usage:**
```bash
nself version [OPTIONS]
```

**Options:**
- `--short` - Version number only
- `--json` - JSON output
- `--check` - Check for updates

**Examples:**
```bash
# Full version info
nself version

# Short version
nself version --short
nself -v

# JSON output
nself version --json

# Check for updates
nself version --check
```

**See:** [VERSION.md](VERSION.md)

---

### update

Update nself to latest version.

**Usage:**
```bash
nself update [OPTIONS]
```

**Options:**
- `--check` - Check for updates only
- `--version <version>` - Install specific version
- `--force` - Force reinstall

**Examples:**
```bash
# Update to latest
nself update

# Check for updates
nself update --check

# Install specific version
nself update --version 0.9.5

# Force reinstall
nself update --force
```

**See:** [UPDATE.md](UPDATE.md)

---

### upgrade

Zero-downtime upgrades (v0.8.0).

**Usage:**
```bash
nself upgrade [COMMAND]
```

**Commands:**
```bash
# Blue-green deployment upgrade
nself upgrade perform

# Rolling update
nself upgrade rolling

# Instant rollback
nself upgrade rollback

# Upgrade status
nself upgrade status
```

**See:** [docs/commands/upgrade.md](upgrade.md)

---

### admin

Admin UI access (legacy shortcut).

**Usage:**
```bash
nself admin [COMMAND]
```

**Examples:**
```bash
# Open admin UI
nself admin

# Start admin UI
nself admin start

# Stop admin UI
nself admin stop
```

**Note:** Use `nself service admin` for full admin management.

**See:** [ADMIN.md](ADMIN.md)

---

### Aliases

**up** - Alias for `nself start`
```bash
nself up              # Same as: nself start
```

**down** - Alias for `nself stop`
```bash
nself down            # Same as: nself stop
```

**See:** [UP.md](UP.md), [DOWN.md](DOWN.md)

---

## Legacy Commands

These commands are maintained for backward compatibility but have been reorganized.

### Migration Guide

| Legacy Command | New Command | Status |
|----------------|-------------|--------|
| `nself backup` | `nself db backup` | Deprecated v0.9.0 |
| `nself restore` | `nself db restore` | Deprecated v0.9.0 |
| `nself admin` | `nself service admin` | Alias maintained |
| `nself email` | `nself service email` | Alias maintained |
| `nself search` | `nself service search` | Alias maintained |
| `nself functions` | `nself service functions` | Alias maintained |
| `nself mlflow` | `nself service mlflow` | Alias maintained |
| `nself redis` | `nself service cache` | Alias maintained |
| `nself staging` | `nself deploy staging` | Alias maintained |
| `nself prod` | `nself deploy production` | Alias maintained |
| `nself providers` | `nself provider` | Alias maintained |
| `nself provision` | `nself provider server create` | Alias maintained |
| `nself servers` | `nself provider server` | Alias maintained |
| `nself cloud` | `nself provider` | Legacy alias |
| `nself billing` | `nself tenant billing` | Moved v0.9.0 |
| `nself whitelabel` | `nself tenant branding/domains/email/themes` | Moved v0.9.0 |

**Deprecation Timeline:**
- **v0.9.0** - Legacy commands still work, deprecation warnings shown
- **v0.10.0** - Warnings become more prominent
- **v1.0.0** - Legacy commands may be removed (TBD)

**Migration Examples:**

```bash
# Old way (still works)
nself backup
nself restore latest
nself email test
nself staging

# New way (recommended)
nself db backup
nself db restore latest
nself service email test
nself deploy staging
```

---

## Global Options

Available on most commands:

| Option | Description |
|--------|-------------|
| `-h, --help` | Show help for command |
| `--version` | Show version |
| `--json` | JSON output (where supported) |
| `--quiet` | Minimal output |
| `--verbose` | Detailed output |
| `--debug` | Debug mode with maximum verbosity |
| `--env <env>` | Target specific environment |
| `--format <format>` | Output format (table, json, csv) |

**Examples:**
```bash
nself status --json
nself deploy --verbose
nself logs --quiet
nself urls --env staging
nself config list --format csv
```

---

## Environment Variables

Key configuration via `.env` files:

### Core Configuration

```bash
# Project basics
PROJECT_NAME=myapp
ENV=dev|staging|prod
BASE_DOMAIN=localhost

# Database
POSTGRES_DB=myapp_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=secure-password

# Hasura
HASURA_GRAPHQL_ADMIN_SECRET=admin-secret
```

### Optional Services

```bash
# Enable optional services
REDIS_ENABLED=true
MINIO_ENABLED=true
NSELF_ADMIN_ENABLED=true
MAILPIT_ENABLED=true
MEILISEARCH_ENABLED=true
MLFLOW_ENABLED=true
FUNCTIONS_ENABLED=true
```

### Monitoring

```bash
# Enable full monitoring stack (10 services)
MONITORING_ENABLED=true

# Optionally configure individual services
GRAFANA_ADMIN_PASSWORD=custom-password
PROMETHEUS_RETENTION=30d
```

### Multi-Tenancy (v0.9.0)

```bash
# Enable multi-tenancy features
MULTI_TENANCY_ENABLED=true
REALTIME_ENABLED=true
```

### Custom Services

```bash
# Define custom services (CS_1 through CS_10)
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
CS_3=grpc:grpc:8003
CS_4=ml-api:python-api:8004
```

### Frontend Applications

```bash
# External frontend apps (routing only)
FRONTEND_APP_1_NAME=app1
FRONTEND_APP_1_PORT=3000
FRONTEND_APP_1_ROUTE=app1

FRONTEND_APP_2_NAME=app2
FRONTEND_APP_2_PORT=3001
FRONTEND_APP_2_ROUTE=app2
```

### nself Behavior

```bash
# Start command options
NSELF_START_MODE=smart|fresh|force
NSELF_HEALTH_CHECK_TIMEOUT=120
NSELF_HEALTH_CHECK_REQUIRED=80
NSELF_SKIP_HEALTH_CHECKS=false
NSELF_LOG_LEVEL=info|debug|warn|error
NSELF_AUTO_FIX=false
NSELF_SKIP_HOOKS=false
NSELF_DEFAULT_PROVIDER=digitalocean
```

**See:** [Environment Configuration](../configuration/ENVIRONMENT-VARIABLES.md)

---

## Exit Codes

Standard exit codes for all commands:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Invalid arguments |
| `3` | Configuration error |
| `4` | Docker error |
| `5` | Database error |
| `6` | Network error |
| `7` | Service unavailable |
| `126` | Permission denied |
| `127` | Command not found |
| `130` | Script terminated by Ctrl+C |

**Usage in scripts:**
```bash
nself deploy production
if [ $? -eq 0 ]; then
  echo "Deployment successful"
else
  echo "Deployment failed with exit code $?"
fi
```

---

## Version History

| Version | Commands Added | Notable Changes |
|---------|----------------|-----------------|
| **0.9.5** | - | Feature parity & security hardening |
| **0.9.0** | `tenant` (32+ subcommands), `oauth` (7 commands), `storage` (8 commands) | Multi-tenancy, OAuth, File storage |
| **0.8.0** | `dev`, `realtime`, `org`, `security`, `upgrade`, platform migrations | Developer tools, real-time features |
| **0.4.8** | `plugin` (list, install, remove, update, status), Stripe/GitHub/Shopify actions | Plugin system |
| **0.4.7** | `provider`, `service`, `k8s`, `helm`, deploy preview/canary/blue-green, sync auto/watch | Cloud infrastructure |
| **0.4.6** | `perf`, `bench`, `scale`, `migrate`, `health`, `frontend`, `history`, `config` | Performance & operations |
| **0.4.5** | `providers`, `provision`, `sync`, `ci`, `completion` | Deployment tools |
| **0.4.4** | `db schema`, `db mock`, `db types` | Database tooling |
| **0.4.3** | `env`, `deploy`, `staging`, `prod` | Environment management |
| **0.4.0** | Core commands: `init`, `build`, `start`, `stop`, `status`, `logs`, `db` | Initial release |

---

## Future: Command Reorganization

A comprehensive reorganization proposal is in progress to improve discoverability and reduce cognitive load:

**Proposed Changes:**
- Reduce from 80+ top-level commands to 13 logical categories
- New categories: `observe` (monitoring), `secure` (security)
- Expanded categories: `auth`, `service`, `deploy`, `cloud`, `dev`, `config`
- Full backward compatibility with legacy aliases for 2+ versions
- 4-phase rollout over 6-12 months

**See:**
- [Command Reorganization Proposal](../architecture/COMMAND-REORGANIZATION-PROPOSAL.md)
- [Visual Command Guide](../architecture/COMMAND-REORGANIZATION-VISUAL.md)
- [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md)
- [Implementation Checklist](../architecture/COMMAND-REORGANIZATION-CHECKLIST.md)

---

## Getting Help

### Built-in Help

```bash
# General help
nself help

# Command-specific help
nself help <command>
nself <command> --help

# Examples
nself help db
nself db --help
nself deploy --help
```

### Diagnostics

```bash
# System check
nself doctor

# Auto-fix issues
nself doctor --fix

# Check for updates
nself version --check
```

### Documentation

- **GitHub**: https://github.com/acamarata/nself
- **Wiki**: https://github.com/acamarata/nself/wiki
- **Issues**: https://github.com/acamarata/nself/issues
- **Discussions**: https://github.com/acamarata/nself/discussions

---

## Quick Reference

### Common Workflows

**New Project:**
```bash
nself init --demo
nself build
nself start
nself urls
```

**Database Development:**
```bash
nself db schema scaffold saas
# Edit schema.dbml
nself db schema apply schema.dbml
nself db types
```

**Deployment:**
```bash
nself env switch staging
nself deploy staging
nself deploy production --blue-green
```

**Monitoring:**
```bash
nself status --watch
nself logs -f
nself monitor
nself perf dashboard
```

**Multi-Tenant SaaS:**
```bash
nself tenant init
nself tenant create "Acme Corp" --plan pro
nself tenant billing usage
nself tenant domains add app.example.com
nself tenant branding upload-logo logo.png
```

---

*Last Updated: January 30, 2026 | Version: 0.9.6*
*nself - Self-Hosted Infrastructure Manager*
