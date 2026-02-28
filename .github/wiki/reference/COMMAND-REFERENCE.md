# nself CLI Command Reference

> Quick reference guide for all nself commands - Optimized for printing

**Version:** v1.0 (consolidated in v0.9.6) | **Total Commands:** 31 top-level commands with 285+ subcommands

> **Important:** v0.9.6 introduced the consolidated v1.0 command structure. Old commands like `nself billing`, `nself org`, `nself staging` are now subcommands under `nself tenant`, `nself deploy`, etc. See [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md) for full details.

---

## Core Commands (7)

```bash
nself init [--demo|--simple]           # Initialize project
nself build [--clean|--no-cache]       # Generate configs
nself start [--fresh|--verbose]        # Start services
nself stop [service]                   # Stop services
nself restart [service]                # Restart services
nself reset [--force]                  # Reset to clean state
nself clean [--all]                    # Clean Docker resources
```

**Key flags:**
- `--verbose` - Detailed output
- `--skip-health-checks` - Skip validation
- `--timeout <seconds>` - Health check timeout

---

## Database Commands (1 with 11 subcommands)

```bash
# Migrations
nself db migrate [up|down|create <name>|status|fresh]

# Schema
nself db schema [scaffold <template>|import <file>|apply <file>|diagram]

# Data
nself db seed [dataset]                # Seed data
nself db mock [auto|--seed N]          # Generate mock data
nself db backup [--name <name>]        # Create backup
nself db restore <file>                # Restore backup

# Operations
nself db shell [--readonly]            # Interactive psql
nself db query <sql>                   # Execute SQL
nself db types [typescript|go|python]  # Generate types
nself db inspect [size|slow]           # Database inspection
nself db data [export <table>|anonymize] # Data operations
```

**Schema templates:** `saas`, `ecommerce`, `blog`, `analytics`

---

## Multi-Tenant Commands (1 with 32+ subcommands) - v0.9.0

> **Consolidated in v0.9.6:** The old `nself billing` and `nself org` commands are now `nself tenant billing` and `nself tenant org`.

```bash
# Core
nself tenant init                      # Initialize multi-tenancy
nself tenant create <name> [--plan]    # Create tenant
nself tenant list                      # List all tenants
nself tenant show <id>                 # Show details
nself tenant suspend|activate <id>     # Lifecycle
nself tenant delete <id>               # Delete tenant
nself tenant stats                     # Statistics

# Members
nself tenant member add <tenant> <user> [role]
nself tenant member remove <tenant> <user>
nself tenant member list <tenant>

# Settings
nself tenant setting set <tenant> <key> <value>
nself tenant setting get <tenant> <key>
nself tenant setting list <tenant>

# Billing (was: nself billing)
nself tenant billing usage             # Usage statistics
nself tenant billing invoice [list|show|download|pay]
nself tenant billing subscription [show|upgrade|downgrade]
nself tenant billing payment [list|add|remove]
nself tenant billing quota             # Check quota
nself tenant billing plan [list|show|compare]
nself tenant billing export --format csv

# Organization (was: nself org)
nself tenant org list                  # List organizations
nself tenant org create <name>         # Create organization
nself tenant org show <id>             # Show details
nself tenant org update <id>           # Update organization
nself tenant org delete <id>           # Delete organization

# Branding
nself tenant branding create <name>
nself tenant branding set-colors --primary #hex
nself tenant branding set-fonts --heading "Font"
nself tenant branding upload-logo <file>
nself tenant branding set-css <file>
nself tenant branding preview

# Domains
nself tenant domains add <domain>
nself tenant domains verify <domain>
nself tenant domains ssl <domain>
nself tenant domains health <domain>
nself tenant domains remove <domain>

# Email Templates
nself tenant email list
nself tenant email edit <template>
nself tenant email preview <template>
nself tenant email test <template> <email>

# Themes
nself tenant themes create <name>
nself tenant themes edit <name>
nself tenant themes activate <name>
nself tenant themes preview <name>
nself tenant themes export <name>
nself tenant themes import <path>
```

---

## OAuth Commands (1 with 6 subcommands) - v0.9.0

> **Consolidated in v0.9.6:** OAuth commands are now under `nself auth oauth`.

```bash
nself auth oauth install               # Install OAuth service
nself auth oauth enable --providers <list>  # Enable providers
nself auth oauth disable --providers <list> # Disable providers
nself auth oauth config <provider>     # Configure credentials
  --client-id=<id>
  --client-secret=<secret>
  --tenant-id=<id>                     # Microsoft only
nself auth oauth test <provider>       # Test configuration
nself auth oauth list                  # List all providers
nself auth oauth status                # Service status
```

**Providers:** `google`, `github`, `slack`, `microsoft`

---

## Storage Commands (1 with 7 subcommands) - v0.9.0

> **Consolidated in v0.9.6:** Storage commands are now under `nself service storage`.

```bash
nself service storage init             # Initialize storage
nself service storage upload <file>    # Upload file
  --dest <path>                        # Destination path
  --thumbnails                         # Generate thumbnails
  --virus-scan                         # Scan for viruses
  --compression                        # Compress large files
  --all-features                       # Enable all features
nself service storage list [prefix]    # List files
nself service storage delete <path>    # Delete file
nself service storage config           # Configure pipeline
nself service storage status           # Pipeline status
nself service storage test             # Test uploads
nself service storage graphql-setup    # Generate GraphQL integration
```

---

## Service Management (1 with 15+ subcommands)

```bash
# Core
nself service list                     # List optional services
nself service enable <service>         # Enable service
nself service disable <service>        # Disable service
nself service status [service]         # Service status
nself service restart <service>        # Restart service
nself service logs <service> [-f]      # Service logs

# Templates
nself service init                     # Initialize from template
nself service scaffold                 # Scaffold new service
nself service wizard                   # Creation wizard
nself service search                   # Search services

# Admin UI
nself service admin status|open|users|config|dev

# Email
nself service email test|inbox|config

# Search
nself service search index|query <term>|stats

# Functions
nself service functions deploy|invoke <fn>|logs|list

# MLflow
nself service mlflow ui|experiments|runs|artifacts

# Storage (MinIO)
nself service storage buckets|upload|download|presign

# Cache (Redis)
nself service cache stats|flush|keys
```

---

## Deployment Commands (1 with 12 subcommands)

> **Consolidated in v0.9.6:** Old top-level commands like `nself staging`, `nself prod`, `nself upgrade` are now under `nself deploy`. Server management moved from `nself servers` to `nself deploy server`.

```bash
# Basic Deployment
nself deploy staging                   # Deploy to staging (was: nself staging)
nself deploy production                # Deploy to production (was: nself prod)
nself deploy upgrade                   # Upgrade deployment (was: nself upgrade)

# Server Management (was: nself servers)
nself deploy server list               # List servers
nself deploy server create <name>      # Create server
nself deploy server remove <name>      # Remove server
nself deploy server ssh <name>         # SSH into server

# Environment Sync (was: nself sync)
nself deploy sync <source> <target>    # Sync environments

# Preview Environments
nself deploy preview                   # Create preview
nself deploy preview list              # List previews
nself deploy preview destroy <id>      # Destroy preview

# Canary Deployment
nself deploy canary [--percentage N]   # Start canary
nself deploy canary promote            # Promote to 100%
nself deploy canary rollback           # Rollback canary
nself deploy canary status             # Canary status

# Blue-Green Deployment
nself deploy blue-green                # Deploy to inactive
nself deploy blue-green switch         # Switch traffic
nself deploy blue-green rollback       # Rollback switch
nself deploy blue-green status         # Show active

# Utilities
nself deploy rollback                  # Rollback deployment
nself deploy check [--fix]             # Pre-deploy validation
nself deploy status                    # Deployment status
```

---

## Cloud Infrastructure (1 with 9+ subcommands) - v0.4.7

> **Consolidated in v0.9.6:** Cloud provider commands moved from `nself cloud`/`nself provider` to `nself infra provider`. Server provisioning moved to `nself deploy server`.

```bash
# Providers (was: nself cloud / nself provider)
nself infra provider list              # List 26+ providers
nself infra provider init <provider>   # Configure credentials
nself infra provider validate          # Validate config
nself infra provider info <provider>   # Provider details

# Server Management (now under: nself deploy server)
nself deploy server create <provider> [--size]
nself deploy server destroy <server>
nself deploy server list
nself deploy server status [server]
nself deploy server ssh <server>
nself deploy server add <ip>
nself deploy server remove <server>

# Cost Management
nself infra provider cost estimate <provider>
nself infra provider cost compare

# Quick Deploy
nself infra provider deploy quick <provider>
nself infra provider deploy full <provider>
```

**Providers:** AWS, GCP, Azure, DigitalOcean, Linode, Vultr, Hetzner, OVH, and 18+ more

---

## Kubernetes & Helm (2 commands) - v0.4.7

> **Consolidated in v0.9.6:** K8s and Helm commands are now under `nself infra k8s` and `nself infra helm`.

```bash
# Kubernetes (was: nself k8s)
nself infra k8s init                   # Initialize K8s config
nself infra k8s convert [--output|--namespace]
nself infra k8s apply [--dry-run]
nself infra k8s deploy [--env <env>]
nself infra k8s status
nself infra k8s logs <service> [-f]
nself infra k8s scale <service> <replicas>
nself infra k8s rollback <service>
nself infra k8s delete
nself infra k8s cluster [list|connect|info]
nself infra k8s namespace [list|create|delete|switch]

# Helm (was: nself helm)
nself infra helm init [--from-compose]
nself infra helm generate
nself infra helm install [--env <env>]
nself infra helm upgrade
nself infra helm rollback
nself infra helm uninstall
nself infra helm list
nself infra helm status
nself infra helm values
nself infra helm template
nself infra helm package
nself infra helm repo [add|remove|update|list]
```

---

## Observability & Monitoring (10 commands)

```bash
# Status & Health
nself status [service] [--json|--watch|--all-envs]
nself health [check|service <name>|watch|history]
nself doctor [--fix|--check <category>]

# Logs & Execution
nself logs [service] [-f|--tail N|--since <time>]
nself exec <service> <command>

# URLs & Monitoring
nself urls [--env|--diff|--json]
nself monitor [grafana|prometheus|alertmanager]
nself metrics [profile <type>|view]

# History & Audit
nself history [show|deployments|migrations|search]
nself audit [logs|events <user>]
```

---

## Security Commands (10 commands)

> **Consolidated in v0.9.6:** Security commands like `mfa`, `roles`, `devices`, `oauth`, `ssl`, `rate-limit`, `webhooks` are now under `nself auth`. Secrets and vault moved to `nself config`.

```bash
# Security Scanning (was: nself security)
nself auth security scan [passwords|mfa|suspicious]
nself auth security devices|incidents|events <user>|webauthn

# Authentication
nself auth users|roles|providers

# MFA & Devices (was: nself mfa, nself devices)
nself auth mfa enable|disable|status
nself auth devices list|approve <id>|revoke <id>

# Roles & Permissions (was: nself roles)
nself auth roles list|create <name>|assign <user> <role>

# Secrets & Vault (was: nself secrets, nself vault)
nself config secrets list|add <key> <value>|rotate
nself config vault init|status|unseal

# SSL & Trust (was: nself ssl, nself trust)
nself auth ssl generate|renew|info
nself auth ssl trust [--system]

# Rate Limiting & Webhooks (was: nself rate-limit, nself webhooks)
nself auth rate-limit config|status
nself auth webhooks list|test <url>
```

---

## Performance & Optimization (4 commands) - v0.4.6

> **Consolidated in v0.9.6:** Performance commands like `bench`, `scale`, `migrate` are now under `nself perf`.

```bash
# Performance Profiling
nself perf profile [service]
nself perf analyze
nself perf slow-queries
nself perf report
nself perf dashboard
nself perf suggest

# Benchmarking (was: nself bench)
nself perf bench run [target]
nself perf bench baseline
nself perf bench compare [file]
nself perf bench stress [target] --users N
nself perf bench report

# Scaling (was: nself scale)
nself perf scale <service> [--cpu N|--memory N|--replicas N]
nself perf scale <service> --auto --min N --max N
nself perf scale status

# Migration (was: nself migrate)
nself perf migrate <source> <target> [--dry-run]
nself perf migrate diff <source> <target>
nself perf migrate sync <source> <target>
nself perf migrate rollback
```

---

## Developer Tools (6 commands)

> **Consolidated in v0.9.6:** Developer tools like `frontend`, `ci`, `docs`, `whitelabel` are now under `nself dev`.

```bash
# Dev Tools - v0.8.0
nself dev sdk generate [typescript|python]
nself dev docs [generate|openapi]
nself dev test [init|fixtures|factory|snapshot|run]
nself dev mock <entity> <count>

# Frontend Management (was: nself frontend)
nself dev frontend [status|list|add|remove|deploy|logs|env]

# CI/CD Generation (was: nself ci)
nself dev ci init [github|gitlab|circleci]
nself dev ci validate
nself dev ci status

# Documentation (was: nself docs)
nself dev docs generate|openapi

# White-label (was: nself whitelabel)
nself dev whitelabel [init|config|brand|theme]

# Shell Completion
nself completion [bash|zsh|fish]
nself completion install <shell>
```

---

## Plugin System (1 command) - v0.4.8

```bash
# Plugin Management
nself plugin list [--installed|--category]
nself plugin install <name>
nself plugin remove <name> [--keep-data]
nself plugin update [name|--all]
nself plugin updates
nself plugin refresh
nself plugin status [name]
nself plugin init                      # Create plugin template

# Stripe Plugin
nself plugin stripe init|sync|check
nself plugin stripe customers [list|show <id>]
nself plugin stripe subscriptions [list|show <id>]
nself plugin stripe invoices [list|show <id>]
nself plugin stripe webhook status|test

# GitHub Plugin
nself plugin github init|sync
nself plugin github repos list
nself plugin github issues [list|show <id>]
nself plugin github prs [list|show <id>]
nself plugin github workflows list
nself plugin github webhook status

# Shopify Plugin
nself plugin shopify init|sync
nself plugin shopify products [list|show <id>]
nself plugin shopify orders [list|show <id>]
nself plugin shopify customers list
nself plugin shopify webhook status
```

---

## Configuration (4 commands)

> **Consolidated in v0.9.6:** Configuration commands like `env`, `secrets`, `vault`, `validate` are now under `nself config`. Sync moved to `nself deploy sync`.

```bash
# Configuration
nself config show|get <key>|set <key> <value>
nself config list|edit|validate
nself config diff <env1> <env2>
nself config export|import <file>
nself config reset

# Environment (was: nself env)
nself config env [list]
nself config env create <name> <type>
nself config env switch <env>
nself config env diff <env1> <env2>
nself config env validate
nself config env access [--check <env>]

# Secrets (was: nself secrets)
nself config secrets list|add <key> <value>|rotate
nself config secrets get <key>
nself config secrets delete <key>

# Vault (was: nself vault)
nself config vault init|status|unseal
nself config vault seal
nself config vault rotate

# Validation (was: nself validate)
nself config validate [--fix|--strict]

# Sync (was: nself sync - now under deploy)
nself deploy sync db|files|config|full <source> <target>
nself deploy sync pull <env>
nself deploy sync auto [--setup|--stop]
nself deploy sync watch [--path|--interval]
nself deploy sync status|history
```

---

## Utilities (5 commands)

> **Consolidated in v0.9.6:** The `upgrade` command is now `nself deploy upgrade`.

```bash
# Help & Version
nself help [command]
nself version [--short|--json|--check]

# Updates
nself update [--check|--version <ver>|--force]

# Zero-Downtime Upgrades - v0.8.0 (was: nself upgrade)
nself deploy upgrade perform|rolling|rollback|status

# Admin UI
nself admin [start|stop]
```

**Aliases:**
```bash
nself up                               # = nself start
nself down                             # = nself stop
nself -v                               # = nself version --short
```

---

## Global Flags

Available on most commands:

```bash
-h, --help                             # Show help
--version                              # Show version
--json                                 # JSON output
--quiet                                # Minimal output
--verbose                              # Detailed output
--debug                                # Debug mode
--env <env>                            # Target environment
--format <format>                      # Output format (table, json, csv)
```

---

## Environment Variables

### Core Configuration

```bash
PROJECT_NAME=myapp
ENV=dev|staging|prod
BASE_DOMAIN=localhost

POSTGRES_DB=myapp_db
POSTGRES_PASSWORD=secure
HASURA_GRAPHQL_ADMIN_SECRET=secret
```

### Optional Services

```bash
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
MONITORING_ENABLED=true                # Enables all 10 services
```

### Multi-Tenancy (v0.9.0)

```bash
MULTI_TENANCY_ENABLED=true
REALTIME_ENABLED=true
```

### Custom Services

```bash
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
CS_3=grpc:grpc:8003
```

### nself Behavior

```bash
NSELF_START_MODE=smart|fresh|force
NSELF_HEALTH_CHECK_TIMEOUT=120
NSELF_HEALTH_CHECK_REQUIRED=80
NSELF_SKIP_HEALTH_CHECKS=false
NSELF_LOG_LEVEL=info|debug|warn|error
```

---

## Exit Codes

```bash
0   # Success
1   # General error
2   # Invalid arguments
3   # Configuration error
4   # Docker error
5   # Database error
126 # Permission denied
127 # Command not found
```

---

## Quick Workflows

### New Project Setup

```bash
nself init --demo
nself build
nself start
nself urls
```

### Daily Development

```bash
nself start
nself status
nself logs -f
nself db shell
nself stop
```

### Database Development

```bash
nself db schema scaffold saas
# Edit schema.dbml
nself db schema apply schema.dbml
nself db types
```

### Deployment

```bash
nself config env switch staging        # was: nself env switch staging
nself deploy check
nself deploy staging                   # was: nself staging
nself deploy production --blue-green   # was: nself prod --blue-green
```

### Multi-Tenant SaaS

```bash
nself tenant init
nself tenant create "Acme Corp" --plan pro
nself tenant billing usage              # was: nself billing usage
nself tenant org list                   # was: nself org list
nself tenant domains add app.example.com
nself tenant branding upload-logo logo.png
```

### Monitoring

```bash
nself status --watch
nself monitor
nself perf dashboard
nself security scan
```

---

## Service Count Summary

### Docker Containers (Demo Config)

- **Required Services:** 4 (Postgres, Hasura, Auth, Nginx)
- **Optional Services:** 7 (Admin, MinIO, Redis, Functions, MLflow, Email, Search)
- **Monitoring Bundle:** 10 (Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager, cAdvisor, Node Exporter, Postgres Exporter, Redis Exporter)
- **Custom Services:** 4 (CS_1 through CS_4)

**Total Docker Containers:** 25
**Frontend Apps:** 2 (external, not in Docker)
**Total Routes:** 21

---

## Common Service Routes

### Required
```
/                                      # Application root
api.local.nself.org                    # Hasura GraphQL
auth.local.nself.org                   # Authentication
```

### Optional Services
```
admin.local.nself.org                  # nself Admin
minio.local.nself.org                  # MinIO Console
functions.local.nself.org              # Functions runtime
mail.local.nself.org                   # MailPit UI
search.local.nself.org                 # MeiliSearch
mlflow.local.nself.org                 # MLflow
```

### Monitoring
```
grafana.local.nself.org                # Grafana
prometheus.local.nself.org             # Prometheus
alertmanager.local.nself.org           # Alertmanager
```

### Custom Services
```
express-api.local.nself.org            # Express API (CS_1)
grpc-api.local.nself.org               # gRPC service (CS_3)
ml-api.local.nself.org                 # Python API (CS_4)
```

### Frontend Apps
```
app1.local.nself.org                   # Frontend App 1
app2.local.nself.org                   # Frontend App 2
```

---

## Version History

| Version | Commands Added |
|---------|----------------|
| **1.0 (v0.9.6)** | Consolidated command structure (79→31 commands) |
| **0.9.5** | Feature parity & security hardening |
| **0.9.0** | `tenant` (32+), `oauth` (7), `storage` (8) |
| **0.8.0** | `dev`, `realtime`, `org`, `security`, `upgrade` |
| **0.4.8** | `plugin` with Stripe/GitHub/Shopify |
| **0.4.7** | `provider`, `service`, `k8s`, `helm` |
| **0.4.6** | `perf`, `bench`, `scale`, `migrate`, `health` |
| **0.4.5** | `providers`, `provision`, `sync`, `ci` |
| **0.4.4** | `db schema`, `db mock`, `db types` |

---

## Getting Help

```bash
nself help                             # General help
nself help <command>                   # Command help
nself <command> --help                 # Alternative
nself doctor                           # System check
nself doctor --fix                     # Auto-fix
nself version --check                  # Check updates
```

**Documentation:**
- GitHub: https://github.com/nself-org/cli
- Wiki: https://github.com/nself-org/cli/wiki
- Complete Reference: [docs/commands/COMMANDS.md](../commands/COMMANDS.md)

---

## Command Consolidation Reference

**Quick lookup for old → new commands:**

| Old Command | New Command (v1.0) |
|-------------|-------------------|
| `nself billing` | `nself tenant billing` |
| `nself org` | `nself tenant org` |
| `nself staging` | `nself deploy staging` |
| `nself prod` | `nself deploy production` |
| `nself upgrade` | `nself deploy upgrade` |
| `nself servers` | `nself deploy server` |
| `nself sync` | `nself deploy sync` |
| `nself cloud` | `nself infra provider` |
| `nself provider` | `nself infra provider` |
| `nself k8s` | `nself infra k8s` |
| `nself helm` | `nself infra helm` |
| `nself storage` | `nself service storage` |
| `nself email` | `nself service email` |
| `nself search` | `nself service search` |
| `nself redis` | `nself service redis` |
| `nself functions` | `nself service functions` |
| `nself mlflow` | `nself service mlflow` |
| `nself realtime` | `nself service realtime` |
| `nself env` | `nself config env` |
| `nself secrets` | `nself config secrets` |
| `nself vault` | `nself config vault` |
| `nself validate` | `nself config validate` |
| `nself mfa` | `nself auth mfa` |
| `nself roles` | `nself auth roles` |
| `nself devices` | `nself auth devices` |
| `nself oauth` | `nself auth oauth` |
| `nself security` | `nself auth security` |
| `nself ssl` | `nself auth ssl` |
| `nself trust` | `nself auth ssl trust` |
| `nself rate-limit` | `nself auth rate-limit` |
| `nself webhooks` | `nself auth webhooks` |
| `nself bench` | `nself perf bench` |
| `nself scale` | `nself perf scale` |
| `nself migrate` | `nself perf migrate` |
| `nself rollback` | `nself backup rollback` |
| `nself reset` | `nself backup reset` |
| `nself clean` | `nself backup clean` |
| `nself frontend` | `nself dev frontend` |
| `nself ci` | `nself dev ci` |
| `nself docs` | `nself dev docs` |
| `nself whitelabel` | `nself dev whitelabel` |

**Full details:** [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md)

---

*Print this page for your desk!*

*Last Updated: January 30, 2026 | Version: 1.0 (v0.9.6 consolidated)*
*nself - Self-Hosted Infrastructure Manager*
