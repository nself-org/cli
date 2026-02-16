# ɳSelf Development Roadmap

## Quick Navigation
[Released](#released) | [Planned (v0.4.x)](#planned-v04x-series) | [Plugins (v0.4.8)](#v048---plugin-system) | [v0.5.0 LTS](#v050---production-ready-lts)

## Current Status Summary
- **v0.9.9 (Current)**: QA & Stabilization Release - Bug fixes, security hardening, final testing before v1.0
- **v1.0.0 (Target Q1 2026)**: LTS Release - Long-term support commitment, stable API, production-proven
- **Previous**: v0.9.8 - Production Ready foundation

---

## Vision
Transform ɳSelf from a powerful CLI tool into a complete self-hosted backend platform that rivals commercial BaaS offerings (Supabase, Nhost, Firebase) while maintaining simplicity, control, and the ability to run anywhere.

---

## Released

### v0.9.8 - Production Ready
**Status**: Released | **Release Date**: February 2026

Production-ready release with comprehensive testing, complete documentation, and UX polish:

- **80% Test Coverage** - 700+ tests across unit, integration, and E2E
- **100% Documentation** - Every command and feature fully documented
- **UX Polish** - Standardized errors, progress indicators, input validation
- **Performance Optimized** - Build caching, batched operations, parallel execution
- **Production Features** - Health endpoints, graceful shutdown, automated backups
- **Quality Metrics** - Benchmarks, security audits, cross-platform verification

Full details: [v0.9.8 Release Notes](./v0.9.8.md)

---

### v0.9.7 - Security & CI/CD Complete
**Status**: Released | **Release Date**: January 31, 2026

Enterprise-grade security and complete CI/CD passing:

- **All CI/CD Passing** - 7/7 GitHub Actions workflows green
- **Tenant Isolation Complete** - 100% test coverage with RLS policies
- **Enterprise Security** - Comprehensive scanning, secrets management, rate limiting
- **Compliance Ready** - GDPR (85%), HIPAA (75%), SOC 2 (70%)
- **Structured Logging** - Immutable audit trails with tamper detection
- **100% Backward Compatible** - Zero breaking changes

Full details: [v0.9.7 Release Notes](./v0.9.7.md)

---

### v0.9.6 - Command Consolidation Complete
**Status**: Released | **Release Date**: January 30, 2026

Complete command consolidation from 79 top-level commands to 31 organized commands with logical hierarchies:

- **Command Reduction** - 79 → 31 top-level commands (60.8% reduction)
- **Logical Hierarchies** - Organized by domain: tenant, deploy, infra, service, auth, config
- **285+ Subcommands** - All functionality preserved, better organized
- **100% Backward Compatible** - All old commands work with deprecation warnings
- **Improved Discoverability** - Consistent patterns, logical grouping
- **Clear Migration Path** - Automated migration tools, comprehensive guides

Full details: [v0.9.6 Release Notes](./v0.9.6.md)

---

### v0.9.5 - Feature Parity & Security Hardening
**Status**: Released | **Release Date**: January 30, 2026

Complete feature parity with Supabase and Nhost, comprehensive security hardening, and real-time communication system:

- **Real-Time Communication** - Complete WebSocket system with channels, presence tracking, broadcast messaging
- **Database Subscriptions** - Change Data Capture (CDC) with INSERT/UPDATE/DELETE event streaming
- **Security Hardening** - Fixed ALL SQL injection vulnerabilities, added CSP framework, comprehensive security audit
- **Enhanced OAuth** - PKCE support for mobile apps, state validation, improved token management
- **Migration Tools** - One-command migration from Supabase and Nhost with data preservation
- **Complete Documentation** - 20+ new guides, comprehensive CLI reference, troubleshooting guides

**Competitive Advantages:**
- Multi-tenancy (vs. none in Supabase/Nhost)
- White-label customization (vs. locked branding)
- Built-in billing system (vs. external integration required)
- Complete data ownership (vs. vendor lock-in)
- Deploy anywhere (vs. cloud-first)

Full details: [v0.9.5 Release Notes](./v0.9.5.md)

---

### v0.9.0 - Enterprise Features
**Status**: Released | **Release Date**: January 30, 2026

Complete enterprise monetization and customization platform with billing, white-label, OAuth handlers, file upload pipeline, and code generation:

- **Billing Integration** - Stripe/Paddle subscription management, usage tracking, invoicing
- **White-Label Platform** - Custom branding, domains, email templates, legal documents
- **OAuth Framework** - Google, GitHub, Microsoft, Apple with account linking
- **File Upload Pipeline** - Resumable uploads, image optimization, virus scanning, CDN integration
- **Code Generation** - TypeScript, Python, Go, Java, Swift SDK generation

Full details: [v0.9.0 Release Notes](./v0.9.0.md)

---

### v0.4.5 - Provider Support
**Status**: Released | **Release Date**: January 21, 2026

Deploy anywhere release adding support for 10 cloud providers with one-command provisioning:

- **Provider Management** - `nself providers` for credential configuration
- **One-Command Provisioning** - `nself provision <provider>` creates full infrastructure
- **10 Providers Supported**: AWS, GCP, Azure, DigitalOcean, Hetzner, Linode, Vultr, IONOS, OVH, Scaleway
- **Cost Estimation** - See costs before provisioning with `--estimate`
- **Cost Comparison** - Compare same setup across providers
- **Terraform Export** - Export infrastructure as Terraform/Pulumi
- **Environment Sync** - `nself sync` for database/config/file sync between environments
- **CI/CD Integration** - `nself ci init` generates GitHub Actions or GitLab CI workflows
- **Shell Completion** - `nself completion` for bash/zsh/fish
- **Doctor Auto-Fix** - `nself doctor --fix` automatically resolves common issues

All providers use normalized sizing (small/medium/large) that maps to provider-specific instance types.
Full details: [Provider Documentation](../commands/PROVIDERS.md)

---

### v0.4.4 - Database Tools
**Status**: Released | **Release Date**: January 20, 2026

Comprehensive database management release adding the unified `nself db` command with all database operations:

- **Migrations** - Full migration lifecycle with rollback support (up, down, create, status, fresh, repair)
- **Seeding** - Environment-aware seeding with special user handling (mock users for dev, real admins for prod)
- **Mock Data** - Deterministic, shareable mock data generation with configurable seeds
- **Backup/Restore** - Complete backup management with scheduling and cross-environment restore
- **Schema Tools** - Schema diffing, DBML diagram generation, index advisor
- **Type Generation** - Generate TypeScript, Go, Python types from database schema
- **Inspection** - Database analysis (like Supabase inspect db) for sizes, cache, indexes, bloat, slow queries
- **Data Operations** - Export, import, anonymize data with PII detection

All commands consolidated under `nself db` with smart defaults and environment-aware safety guards.
Full details: [DB.md](../commands/DB.md)

---

### v0.4.3 - Deployment Pipeline
**Status**: Released | **Release Date**: January 19, 2026

Feature release adding comprehensive deployment pipeline commands:

- **Environment Command** - Create, switch, diff, validate environments
- **Deploy Command** - Enhanced SSH deployment with zero-downtime support
- **Prod Command** - Production deployment shortcut
- **Staging Command** - Staging deployment shortcut

**New library modules**: env/, deploy/, security/. **Bug fixes**: nginx variable substitution, 16 Dockerfile templates.
Full details: [v0.4.3 Release Notes](./v0.4.3.md)

---

### v0.4.2 - Service & Monitoring Management
**Status**: Released | **Release Date**: January 18, 2026

Feature release adding 6 new service management commands:

- **Email Command** - Email provider configuration, SMTP pre-flight checks, testing
- **Search Command** - 6-engine search management (PostgreSQL, MeiliSearch, Typesense, etc.)
- **Functions Command** - Serverless deployment with TypeScript support
- **MLflow Command** - ML experiment tracking, runs management
- **Metrics Command** - Monitoring profiles (minimal/standard/full/auto)
- **Monitor Command** - Dashboard access and CLI monitoring views

**Also includes**: 92 unit tests, complete documentation, cross-platform fixes.
Full details: [v0.4.2 Release Notes](./v0.4.2.md)

---

### v0.4.1 - Platform Compatibility Fixes
**Status**: Released | **Release Date**: January 17, 2026

Bug fix release addressing 5 critical platform compatibility issues:

- **Bash 3.2 Compatibility** - Fixed array declaration syntax
- **Cross-Platform sed** - Fixed in-place editing for macOS/Linux
- **Cross-Platform stat** - Fixed file stat commands
- **Portable timeout** - Added guards for timeout command
- **Portable output** - Converted echo -e to printf

**22 files fixed** across the codebase. Full details: [v0.4.1 Release Notes](./v0.4.1.md)

---

### v0.4.0 - Production-Ready Core
**Status**: Released | **Release Date**: October 13, 2025

v0.4.0 represents the **stable, production-ready release** of nself with all core features complete.

- **Complete Backend Stack** - PostgreSQL with 60+ extensions, Hasura GraphQL, Auth, Storage
- **Admin UI** - Web-based monitoring dashboard
- **40+ Service Templates** - Production-ready microservice templates across 10 languages
- **SSL Management** - Automatic certificates with mkcert (Let's Encrypt ready)
- **Environment Management** - Multi-environment configuration with smart defaults
- **Docker Compose** - Battle-tested container orchestration
- **Monitoring Bundle** - Complete 10-service monitoring stack

[View Full Release Notes](./v0.4.0.md)

---

## Released

### v0.4.6 - Scaling & Performance
**Status**: Released | **Release Date**: January 22, 2026
**Focus**: Performance profiling, benchmarking, scaling, and cross-environment migration

#### New Commands (9)

| Command | Purpose |
|---------|---------|
| `nself perf` | Performance profiling and analysis |
| `nself bench` | Benchmarking and load testing |
| `nself scale` | Horizontal/vertical scaling |
| `nself migrate` | Cross-environment migration |
| `nself health` | Health check management |
| `nself frontend` | Frontend application management |
| `nself history` | Deployment audit trail |
| `nself config` | Configuration management |
| `nself servers` | Server infrastructure management |

#### Enhanced Commands

| Command | Enhancement |
|---------|-------------|
| `nself status` | Added `--json`, `--all-envs` flags |
| `nself urls` | Added `--env`, `--diff` flags |
| `nself deploy` | Added `check` subcommand for pre-deployment validation |

#### `nself scale` - Scaling
```bash
# Vertical scaling (resources)
nself scale up <service>     # Increase resources
nself scale up postgres --memory 4G
nself scale up hasura --cpu 2
nself scale down <service>   # Decrease resources

# Horizontal scaling (replicas)
nself scale out <service>    # Add replica
nself scale out api --replicas 3
nself scale in <service>     # Remove replica

# Auto-scaling
nself scale auto <service>   # Enable auto-scaling
nself scale auto api --min 2 --max 10
nself scale auto api --cpu-threshold 70
nself scale auto --disable <service>

# Connection pooling
nself scale pooler enable    # Enable PgBouncer
nself scale pooler config --pool-size 100
nself scale pooler status

# Redis cluster
nself scale redis cluster --nodes 6
nself scale redis add-node
nself scale redis status

# Status
nself scale status           # Show scaling status
nself scale status <service>
```

#### `nself perf` - Performance Profiling
```bash
# Profiling
nself perf profile           # Full system profile
nself perf profile <service> # Service-specific
nself perf profile --duration 60

# Analysis
nself perf analyze           # Analyze current performance
nself perf analyze --slow-queries
nself perf analyze --memory
nself perf analyze --cpu
nself perf slow-queries      # Detailed slow query analysis

# Reports
nself perf report            # Generate performance report
nself perf report --format html
nself perf report --compare last

# Dashboard
nself perf dashboard         # Real-time terminal dashboard

# Recommendations
nself perf suggest           # Get optimization suggestions
```

#### `nself migrate` - Cross-Environment Migration
```bash
# Full migration
nself migrate staging prod   # Migrate staging to prod
nself migrate local staging  # Migrate local to staging
nself migrate --dry-run      # Preview migration

# Selective migration
nself migrate --schema-only
nself migrate --data-only
nself migrate --config-only

# Sync
nself migrate sync staging prod  # Keep environments in sync
nself migrate sync --watch   # Continuous sync

# Drift detection
nself migrate diff staging prod  # Show differences
nself migrate diff --schema
nself migrate diff --config

# Rollback
nself migrate rollback       # Rollback last migration
nself migrate rollback --to checkpoint-2026-01-22
```

#### `nself bench` - Benchmarking
```bash
# API benchmarking
nself bench api              # Benchmark all endpoints
nself bench api /users       # Specific endpoint
nself bench api --concurrent 100
nself bench api --duration 60

# Database benchmarking
nself bench db               # Overall database performance
nself bench db --read
nself bench db --write
nself bench db --mixed

# Load testing
nself bench load             # Simulate production load
nself bench load --users 1000
nself bench load --scenario checkout

# Comparison
nself bench compare          # Compare to baseline
nself bench compare --with last
nself bench baseline save    # Save current as baseline
```

#### Key Features
- **Load Balancer Config**: Automatic nginx upstream for replicas
- **PgBouncer Integration**: Connection pooling for better DB performance
- **Redis Cluster**: Scale Redis horizontally
- **Slow Query Detection**: Identify and suggest fixes for slow queries
- **Configuration Drift**: Detect differences between environments
- **Benchmark Scenarios**: Define custom user flow scenarios

---

### v0.4.7 - Kubernetes Support
**Status**: Released | **Release Date**: January 23, 2026
**Focus**: Full support for Kubernetes and container orchestration

#### New Commands (2)

| Command | Purpose |
|---------|---------|
| `nself k8s` | Kubernetes operations |
| `nself helm` | Helm chart management |

#### `nself k8s` - Kubernetes Operations
```bash
# Generation
nself k8s generate           # Generate K8s manifests from compose
nself k8s generate --output ./k8s
nself k8s generate --namespace myapp

# Deployment
nself k8s apply              # Apply manifests to cluster
nself k8s apply --env staging
nself k8s apply --dry-run

# Status
nself k8s status             # Cluster status
nself k8s status <service>   # Service status
nself k8s pods               # List all pods
nself k8s events             # Recent events

# Operations
nself k8s logs <service>     # Pod logs
nself k8s logs <service> -f  # Follow logs
nself k8s exec <service>     # Exec into pod
nself k8s shell <service>    # Interactive shell

# Rollout
nself k8s rollout <service>  # Rolling update
nself k8s rollout status
nself k8s rollback <service>
nself k8s rollback --to v1.2.3

# Scaling
nself k8s scale <service> 3  # Scale to 3 replicas
nself k8s autoscale <service>  # Enable HPA

# Context
nself k8s context            # Show current context
nself k8s context use <name> # Switch context
nself k8s contexts           # List all contexts
```

#### `nself helm` - Helm Chart Management
```bash
# Chart creation
nself helm init              # Initialize Helm chart
nself helm init --from-compose

# Packaging
nself helm package           # Package chart
nself helm lint              # Lint chart
nself helm template          # Render templates locally

# Installation
nself helm install           # Install to cluster
nself helm install --env staging
nself helm upgrade           # Upgrade release
nself helm rollback          # Rollback release

# Management
nself helm list              # List releases
nself helm status            # Release status
nself helm history           # Release history
nself helm uninstall         # Remove release

# Repository
nself helm repo add <url>    # Add chart repo
nself helm push              # Push to registry
```

#### Supported Platforms

| Platform | Provider | Status |
|----------|----------|--------|
| **EKS** | AWS | Full |
| **GKE** | Google Cloud | Full |
| **AKS** | Azure | Full |
| **DOKS** | DigitalOcean | Full |
| **LKE** | Linode | Full |
| **Self-Hosted** | Any | Full |
| **k3s** | Edge/IoT | Full |
| **minikube** | Local Dev | Full |

#### Key Features
- **Automatic Conversion**: docker-compose.yml → K8s manifests
- **Kustomize Support**: Environment-specific overlays
- **Helm Charts**: Generate, package, and deploy Helm charts
- **Service Mesh**: Istio and Linkerd integration
- **Rolling Deployments**: Zero-downtime updates
- **HPA Integration**: Horizontal Pod Autoscaler support

---

### v0.8.0 - Security Hardening
**Status**: Released | **Release Date**: January 29, 2026
**Focus**: Complete security framework with SSL/TLS, firewall, secrets management

#### Sprint 21: Security Hardening (25pts)
- Automated SSL/TLS certificate management (Let's Encrypt integration)
- Nginx security headers and best practices
- WAF (Web Application Firewall) integration
- Firewall configuration and port management
- Security audit logging
- Intrusion detection system (IDS) integration

#### Sprint 22: Secrets Management (20pts)
- Vault integration for secrets storage
- Environment-specific secret rotation
- API key management
- Database credential encryption
- Secrets audit trail
- Emergency access protocols

#### Key Features
- **Automated SSL**: Let's Encrypt certificates with auto-renewal
- **Firewall Management**: UFW/iptables configuration via nself
- **Secrets Vault**: Encrypted storage for API keys and credentials
- **Security Scanning**: Regular vulnerability scans
- **Audit Logging**: Complete security event tracking
- **Compliance Tools**: SOC2, HIPAA, PCI-DSS helpers

---

### v0.4.8 - Plugin System
**Status**: Released | **Release Date**: January 24, 2026
**Focus**: Extensible plugin architecture for third-party integrations

#### Overview

**Data Sync Plugins** extend nself with deep integrations that keep your PostgreSQL database in sync with external services.

> **Note**: This is different from service commands like `nself email` which configure how nself uses services. Plugins are specifically for **syncing external data** into your database.

Unlike Custom Services (CS_N) which are independent backend apps, **Plugins** provide:

- **Schema Sync**: Mirror external service data in your PostgreSQL database
- **Webhook Handling**: Automatic webhook endpoints for real-time updates
- **Sanity Checks**: Verify your DB matches the external service
- **Historical Downloads**: Backfill historical data from the service
- **CLI Commands**: Plugin-specific management commands

#### New Commands (2)

| Command | Purpose |
|---------|---------|
| `nself plugin` | Plugin management |
| `nself stripe` | Stripe-specific commands (first plugin) |

#### `nself plugin` - Plugin Management
```bash
# Discovery
nself plugin list               # List available plugins
nself plugin search <query>     # Search plugin registry
nself plugin info <name>        # Show plugin details

# Installation
nself plugin install <name>     # Install a plugin
nself plugin install nself-stripe
nself plugin install nself-stripe@1.2.0  # Specific version
nself plugin uninstall <name>   # Remove a plugin

# Management
nself plugin status             # Show installed plugins status
nself plugin update             # Update all plugins
nself plugin update <name>      # Update specific plugin
nself plugin config <name>      # Configure plugin settings

# Development
nself plugin init               # Create new plugin template
nself plugin validate           # Validate plugin structure
nself plugin publish            # Publish to registry
```

#### First Plugin: nself-stripe

**nself-stripe** syncs your Stripe account to your PostgreSQL database and keeps it in sync via webhooks.

```bash
# Installation
nself plugin install nself-stripe

# Configuration
nself stripe init               # Interactive setup wizard
nself stripe config             # Edit configuration
nself stripe config set api_key sk_live_xxx
nself stripe config set webhook_secret whsec_xxx

# Schema & Sync
nself stripe schema             # Show Stripe tables that will be created
nself stripe schema apply       # Create/update Stripe tables in DB
nself stripe sync               # Full sync from Stripe API
nself stripe sync --since 2024-01-01  # Sync from date
nself stripe sync customers     # Sync specific resource
nself stripe sync --incremental # Only sync changes

# Webhook Management
nself stripe webhook status     # Show webhook endpoint status
nself stripe webhook register   # Register webhook with Stripe
nself stripe webhook test       # Send test webhook
nself stripe webhook logs       # View webhook event logs

# Sanity Checks
nself stripe check              # Verify DB matches Stripe
nself stripe check customers    # Check specific resource
nself stripe check --fix        # Auto-fix discrepancies
nself stripe diff               # Show differences

# Historical Data
nself stripe backfill           # Download all historical data
nself stripe backfill --from 2020-01-01
nself stripe backfill invoices  # Backfill specific resource

# Status
nself stripe status             # Overall sync status
nself stripe status customers   # Resource-specific status
```

#### Stripe Resources Synced

| Resource | Table | Webhook Events |
|----------|-------|----------------|
| **Customers** | `stripe_customers` | customer.created, customer.updated, customer.deleted |
| **Subscriptions** | `stripe_subscriptions` | subscription.* |
| **Invoices** | `stripe_invoices` | invoice.* |
| **Payments** | `stripe_payment_intents` | payment_intent.* |
| **Products** | `stripe_products` | product.* |
| **Prices** | `stripe_prices` | price.* |
| **Charges** | `stripe_charges` | charge.* |
| **Refunds** | `stripe_refunds` | refund.* |
| **Disputes** | `stripe_disputes` | dispute.* |
| **Payouts** | `stripe_payouts` | payout.* |
| **Events** | `stripe_events` | All events (audit log) |

#### Webhook Endpoint Configuration

The plugin creates a webhook endpoint at your choice:
- `api.domain.com/webhooks/stripe` (default)
- `api.stripe.domain.com` (dedicated subdomain)
- Custom path configurable

```bash
# Configure webhook endpoint
nself stripe config set webhook_path /webhooks/stripe
nself stripe config set webhook_subdomain stripe  # api.stripe.domain.com
```

#### Plugin Architecture

```
plugins/
├── nself-stripe/
│   ├── plugin.json           # Plugin manifest
│   ├── schema/
│   │   └── stripe.sql        # Database schema
│   ├── sync/
│   │   ├── customers.ts      # Resource sync logic
│   │   ├── subscriptions.ts
│   │   └── ...
│   ├── webhooks/
│   │   └── handler.ts        # Webhook processing
│   ├── commands/
│   │   └── stripe.sh         # CLI commands
│   └── docker/
│       └── Dockerfile        # Plugin service container
```

#### Key Features

- **1:1 Database Sync**: Your PostgreSQL always matches Stripe exactly
- **Real-Time Updates**: Webhooks keep data current within seconds
- **Sanity Checks**: Detect and fix drift between DB and Stripe
- **Historical Backfill**: Download years of data on first setup
- **Audit Log**: All Stripe events stored for compliance
- **Multi-Account**: Support multiple Stripe accounts per nself instance
- **Test Mode**: Separate sync for Stripe test vs live mode

#### Planned Plugins

| Plugin | Purpose | Status |
|--------|---------|--------|
| **nself-stripe** | Stripe billing/payments data sync | **Planned** (first plugin) |

#### Potential Future Plugin Ideas

These are ideas for future data sync plugins - not currently planned:

- **nself-shopify** - Shopify store/order sync
- **nself-github** - GitHub repo/issue/PR sync
- **nself-linear** - Linear issue tracking sync
- **nself-twilio** - Twilio SMS/voice call logs
- **nself-sendgrid** - SendGrid email event tracking

> The plugin architecture will be designed with nself-stripe first, then expanded based on community needs.

---

### v0.9.0 - Enterprise Features
**Target**: Q1 2026
**Focus**: Billing integration and white-label customization for enterprise use

#### Sprint 13: Billing Integration & Usage Tracking (75pts)
- Stripe billing integration
- Usage metering and tracking
- Subscription management
- Invoice generation
- Payment gateway integration
- Billing analytics and reporting
- Quota enforcement
- Cost allocation by tenant/service

#### Sprint 14: White-Label & Customization (60pts)
- Custom branding configuration
- White-label admin UI
- Multi-tenant isolation
- Custom domain management
- Email template customization
- Logo and color scheme overrides
- Custom authentication flows
- Client-specific configurations

#### Key Features
- **Usage Tracking**: Automatic metering of API calls, storage, compute
- **Billing Automation**: Stripe integration with automated invoicing
- **White-Label Support**: Complete brand customization
- **Multi-Tenancy**: Isolated environments per client
- **Cost Management**: Budget alerts and cost optimization
- **Enterprise SSO**: SAML, OAuth, LDAP integration

---

### v0.4.9 - Extensive Polish & Testing
**Target**: Q1 2026
**Focus**: Comprehensive QA, polish, and minor changes to finish the v0.4.x series

#### Focus Areas

##### Bug Fixes & Stability
- Address all reported GitHub issues
- Edge case handling
- Error message improvements (actionable suggestions)
- Recovery mechanisms for common failures

##### nself-admin Integration
- Real-time monitoring dashboard
- Service health visualization
- Log viewer with search and filtering
- Metrics graphs and charts
- Alert configuration UI
- User management UI
- Database management UI
- Backup/restore UI
- Configuration editor
- Web-based CLI terminal

##### CLI Improvements
```bash
# Shell completions
nself completion bash >> ~/.bashrc
nself completion zsh >> ~/.zshrc
nself completion fish >> ~/.config/fish/completions/nself.fish

# Interactive mode
nself interactive
nself -i

# Offline documentation
nself docs
nself docs deploy
nself help deploy  # Works offline
```

##### Testing & Quality
- Unit test coverage > 90%
- Integration test coverage > 80%
- E2E test coverage > 70%
- Cross-platform verification (Ubuntu, macOS, WSL)
- Performance benchmarks

---

## v0.5.0 - Production Ready LTS

### v0.5.0 - Production Ready LTS Release
**Target**: Q1-Q2 2026
**Focus**: Production-ready Long-Term Support release with nself-admin v0.1

This is the **1.0-equivalent LTS release** for nself - the complete, production-ready, long-term support self-hosted backend platform.

#### Includes
- **nself CLI v0.5.0** - All features from v0.4.x series stable and polished
- **nself-admin v0.1.0** - First official release of the web admin UI

#### nself CLI v0.5.0 - Complete Command List (48 commands)

```
Core Commands (8):
  init, build, start, stop, restart, reset, clean, version

Status Commands (6):
  status, logs, exec, urls, doctor, help

Management Commands (4):
  update, ssl, trust, admin

Service Commands (6) - v0.4.2:
  email, search, functions, mlflow, metrics, monitor

Environment Commands (4) - v0.4.3:
  env, deploy, prod, staging

Database Commands (6) - v0.4.4:
  db, backup, restore, seed, mock, data

Cloud Commands (2) - v0.4.5:
  cloud, provision

Scaling Commands (4) - v0.4.6:
  scale, perf, migrate, bench

Kubernetes Commands (2) - v0.4.7:
  k8s, helm

Plugin Commands (2) - v0.4.8:
  plugin, stripe

Utility Commands (4) - v0.4.9:
  completion, interactive, docs, config
```

#### nself-admin v0.1.0 Features

| Feature | Description |
|---------|-------------|
| **Dashboard** | System overview, health, resource usage |
| **Services** | Service management with start/stop/restart |
| **Logs** | Real-time log streaming with search |
| **Metrics** | Grafana-like visualization |
| **Alerts** | Alert configuration and history |
| **Database** | Table browser, query runner |
| **Backups** | Backup/restore interface |
| **Users** | User and role management |
| **Config** | Environment configuration editor |
| **Terminal** | Web-based CLI access |

#### Quality Standards
- 100% documented commands
- Cross-platform tested (macOS, Linux, WSL)
- Security audited
- Performance benchmarked
- Production-proven

---

## Development Principles

1. **Stability First** - Never break existing features
2. **Smart Defaults** - Everything works out of the box
3. **No Lock-in** - Standard Docker/PostgreSQL/GraphQL
4. **Progressive Disclosure** - Advanced features stay hidden until needed
5. **Auto-Fix** - Detect and resolve problems automatically
6. **Offline-First** - Works without internet connection
7. **Security by Default** - Production-ready security out of the box
8. **Cross-Platform** - Works on macOS, Linux, WSL (Bash 3.2+)

---

## Release Timeline

| Version | Status | Focus | Target |
|---------|--------|-------|--------|
| v0.4.0 | Released | Production-Ready Core | Oct 13, 2025 |
| v0.4.1 | Released | Platform Compatibility | Jan 17, 2026 |
| v0.4.2 | Released | Service & Monitoring | Jan 18, 2026 |
| v0.4.3 | Released | Deployment Pipeline | Jan 19, 2026 |
| v0.4.4 | Released | Database Tools | Jan 20, 2026 |
| v0.4.5 | Released | Provider Support | Jan 21, 2026 |
| v0.4.6 | Released | Scaling & Performance | Jan 22, 2026 |
| v0.4.7 | Released | Kubernetes Support | Jan 23, 2026 |
| v0.4.8 | Released | Plugin System | Jan 24, 2026 |
| v0.8.0 | Released | Security Hardening | Jan 29, 2026 |
| v0.9.0 | Released | Enterprise Features | Jan 30, 2026 |
| v0.9.5 | Released | Feature Parity & Security | Jan 30, 2026 |
| v0.9.6 | Released | Command Consolidation | Jan 30, 2026 |
| v0.9.7 | Released | Security & CI/CD Complete | Jan 31, 2026 |
| **v0.9.8** | **Released** | Production Ready | Feb 2026 |
| **v0.9.9** | **Next** | QA & Final Testing | Feb 2026 |
| **v1.0.0** | **Target** | LTS Release | Q1 2026 |

---

## Command Summary by Release

### Currently Available (v0.8.0) - 46 commands
```
Core: init, build, start, stop, restart, reset, clean, version
Status: status, logs, exec, urls, doctor, help
Management: update, ssl, trust, admin
Services: email, search, functions, mlflow, metrics, monitor
Deployment: env, deploy, prod, staging
Database: db (migrate, seed, mock, backup, restore, schema, types, shell, inspect, data)
Provider: providers, provision, sync, ci, completion
Performance: perf, bench, scale, migrate
Operations: health, frontend, history, config, servers
Kubernetes: k8s, helm
Plugins: plugin (+ stripe actions)
Security: security (ssl, firewall, secrets, audit, scan)
```

Note: Database operations are consolidated under `nself db` with subcommands.
Plugin actions are accessed via `nself plugin <name> <action>`.
Security operations are consolidated under `nself security` with subcommands.

### Coming in v0.9.0 - Enterprise Features
```
billing, usage, tenant, whitelabel
```

### Coming in v0.4.9 - +4 commands
```
completion, interactive, docs, config
```

**Total Commands at v0.5.0**: 54

---

## Contributing

### Priority Areas
1. Test v0.4.4 database tools in development and staging environments
2. Report bugs and edge cases
3. Documentation improvements
4. Community feedback on roadmap priorities
5. Performance benchmarks

### How to Contribute
- **GitHub**: [github.com/acamarata/nself](https://github.com/acamarata/nself)
- **Issues**: [Report Bugs](https://github.com/acamarata/nself/issues)
- **Discussions**: [Feature Requests & Ideas](https://github.com/acamarata/nself/discussions)
- **Testing**: Help test new features in development

---

*This roadmap reflects actual implemented features and realistic future plans. Updated regularly based on development progress and community feedback.*

*Last Updated: January 31, 2026*
