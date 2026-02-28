# nself CLI Command Reorganization Proposal

**Status**: Proposal
**Version**: 1.0
**Date**: 2026-01-30
**Authors**: Automated Analysis

---

## Executive Summary

nself currently has **77 top-level commands**, which creates:
- **Discoverability issues** - Users struggle to find the right command
- **Inconsistent patterns** - Some features are top-level, others are subcommands
- **Cognitive overload** - Too many options at the root level
- **Help text bloat** - `nself help` is overwhelming

This proposal reorganizes the CLI into **13 logical categories**, reducing top-level commands by ~83% while improving clarity and maintaining backward compatibility.

---

## Current State Analysis

### Current Top-Level Commands (77)

```
admin          admin-dev      audit          auth           backup
bench          billing        build          ci             clean
cloud          completion     config         db             deploy
dev            devices        doctor         down           email
env            exec           frontend       functions      health
helm           help           history        init           k8s
logs           metrics        mfa            migrate        mlflow
monitor        nself          oauth          org            perf
plugin         prod           providers      provision      rate-limit
realtime       redis          reset          restart        restore
roles          rollback       scale          search         secrets
security       server         servers        service        ssl
staging        start          status         stop           storage
sync           tenant         trust          up             update
upgrade        urls           validate       vault          version
webhooks       whitelabel
```

### Issues Identified

1. **Scattered Features**
   - Authentication split across: `auth`, `oauth`, `mfa`, `devices`, `security`, `roles`, `webhooks`
   - Database split across: `db`, `migrate`, `backup`, `restore`
   - Deployment split across: `deploy`, `staging`, `prod`, `rollback`
   - Cloud split across: `cloud`, `providers`, `provision`, `server`, `servers`

2. **Inconsistent Depth**
   - `tenant` is top-level with 32+ subcommands ✅ GOOD
   - `service` is top-level with 15+ subcommands ✅ GOOD
   - `email`, `search`, `functions`, `mlflow`, `redis` are all top-level ❌ BAD (should be under `service`)

3. **Redundancy**
   - `server` vs `servers`
   - `staging` vs `deploy staging`
   - `prod` vs `deploy production`
   - `whitelabel` vs `tenant branding/domains/email/themes`

4. **Missing Grouping**
   - Monitoring/observability scattered: `logs`, `metrics`, `monitor`, `health`, `status`, `doctor`
   - Security fragmented: `security`, `mfa`, `audit`, `secrets`, `vault`, `ssl`, `trust`

---

## Proposed Reorganization

### 13 Top-Level Categories

```
nself
├── init              Core: Initialize project
├── build             Core: Build configs
├── start             Core: Start services
├── stop              Core: Stop services
├── restart           Core: Restart services
├── status            Core: Service status
│
├── db                Database operations (KEEP AS-IS)
├── auth              Authentication & authorization (EXPANDED)
├── tenant            Multi-tenancy (KEEP AS-IS)
├── service           Service management (KEEP AS-IS)
│
├── deploy            Deployment & environments (EXPANDED)
├── cloud             Cloud infrastructure (EXPANDED)
├── observe           Monitoring & observability (NEW)
├── secure            Security & compliance (NEW)
│
├── plugin            Plugins (KEEP AS-IS)
├── dev               Developer tools (EXPANDED)
├── config            Configuration (EXPANDED)
│
└── help              Help & utilities
```

---

## Detailed Reorganization

### 1. Core Lifecycle (Keep Top-Level)

**NO CHANGES - These are essential daily commands**

```bash
nself init         # Initialize project
nself build        # Build configs
nself start        # Start services
nself stop         # Stop services
nself restart      # Restart services
nself status       # Service status overview
```

---

### 2. Database (`nself db`) - KEEP AS-IS

**Already well-organized, no changes needed**

```bash
nself db migrate        # Migrations
nself db schema         # Schema operations
nself db seed           # Seed data
nself db mock           # Generate mock data
nself db backup         # Create backup
nself db restore        # Restore from backup
nself db shell          # Interactive psql
nself db query          # Execute SQL
nself db types          # Generate types
nself db inspect        # Database inspection
nself db data           # Data operations
```

**Migration Notes:**
- Move `backup` → `db backup` (legacy alias supported)
- Move `restore` → `db restore` (legacy alias supported)
- Move `migrate` → `db migrate` (legacy alias supported)

---

### 3. Authentication (`nself auth`) - EXPANDED

**CURRENT**: `auth`, `oauth`, `mfa`, `devices`, `roles`, `webhooks`
**PROPOSED**: Consolidate under `nself auth`

```bash
# Basic auth (from auth.sh)
nself auth login                    # User login
nself auth signup                   # User signup
nself auth logout                   # Logout
nself auth status                   # Auth status

# OAuth (from oauth.sh)
nself auth oauth install            # Install OAuth service
nself auth oauth enable             # Enable providers
nself auth oauth config <provider>  # Configure provider
nself auth oauth test <provider>    # Test provider
nself auth oauth list               # List providers
nself auth oauth status             # OAuth status

# MFA (from mfa.sh)
nself auth mfa enable               # Enable MFA
nself auth mfa disable              # Disable MFA
nself auth mfa verify               # Verify MFA code
nself auth mfa backup-codes         # Generate backup codes
nself auth mfa qr                   # Show QR code

# Devices (from devices.sh)
nself auth devices list             # List user devices
nself auth devices revoke <id>      # Revoke device
nself auth devices trust <id>       # Trust device

# Roles (from roles.sh)
nself auth roles list               # List roles
nself auth roles create <name>      # Create role
nself auth roles assign <user>      # Assign role
nself auth roles permissions <role> # Manage permissions

# Webhooks (from webhooks.sh)
nself auth webhooks list            # List auth webhooks
nself auth webhooks create          # Create webhook
nself auth webhooks test <id>       # Test webhook
```

**Migration:**
- `nself oauth` → `nself auth oauth` (legacy alias)
- `nself mfa` → `nself auth mfa` (legacy alias)
- `nself devices` → `nself auth devices` (legacy alias)
- `nself roles` → `nself auth roles` (legacy alias)
- `nself webhooks` → `nself auth webhooks` (legacy alias)

---

### 4. Tenant (`nself tenant`) - KEEP AS-IS

**Already well-organized, no changes needed**

```bash
nself tenant init                   # Initialize multi-tenancy
nself tenant create <name>          # Create tenant
nself tenant list                   # List tenants
nself tenant show <id>              # Show details
nself tenant suspend <id>           # Suspend tenant
nself tenant activate <id>          # Activate tenant
nself tenant delete <id>            # Delete tenant
nself tenant stats                  # Statistics

# Member management
nself tenant member add             # Add member
nself tenant member remove          # Remove member
nself tenant member list            # List members

# Billing
nself tenant billing usage          # Usage stats
nself tenant billing invoice        # Invoices
nself tenant billing subscription   # Subscriptions
nself tenant billing payment        # Payment methods
nself tenant billing quota          # Quota limits
nself tenant billing plan           # Plans
nself tenant billing export         # Export data
nself tenant billing customer       # Customer info

# Branding & white-label
nself tenant branding               # Brand customization
nself tenant domains                # Custom domains
nself tenant email                  # Email templates
nself tenant themes                 # Theme management

# Settings
nself tenant setting set            # Set setting
nself tenant setting get            # Get setting
nself tenant setting list           # List settings
```

**Migration:**
- `nself billing` → `nself tenant billing` (legacy alias)
- `nself whitelabel` → `nself tenant branding` (legacy alias)
- `nself org` → `nself tenant` (legacy alias)

---

### 5. Service (`nself service`) - EXPAND

**CURRENT**: Some services under `service`, others top-level
**PROPOSED**: ALL optional services under `nself service`

```bash
# Core service management (existing)
nself service list                  # List services
nself service enable <service>      # Enable service
nself service disable <service>     # Disable service
nself service status [service]      # Service status
nself service restart <service>     # Restart service
nself service logs <service>        # Service logs

# Admin UI (from admin.sh, admin-dev.sh)
nself service admin status          # Admin status
nself service admin open            # Open admin UI
nself service admin users           # User management
nself service admin config          # Admin config
nself service admin dev             # Development mode

# Email (from email.sh)
nself service email test            # Send test email
nself service email inbox           # Open MailPit
nself service email config          # Email config

# Search (from search.sh)
nself service search index          # Reindex data
nself service search query <term>   # Run query
nself service search stats          # Index stats

# Functions (from functions.sh)
nself service functions deploy      # Deploy functions
nself service functions invoke <fn> # Invoke function
nself service functions logs [fn]   # View logs
nself service functions list        # List functions

# MLflow (from mlflow.sh)
nself service mlflow ui             # Open UI
nself service mlflow experiments    # List experiments
nself service mlflow runs           # List runs
nself service mlflow artifacts      # Browse artifacts

# Storage (from storage.sh)
nself service storage init          # Initialize storage
nself service storage upload        # Upload file
nself service storage list          # List files
nself service storage delete        # Delete file
nself service storage config        # Configure
nself service storage status        # Status
nself service storage test          # Test

# Cache/Redis (from redis.sh)
nself service cache stats           # Statistics
nself service cache flush           # Flush cache
nself service cache keys            # List keys

# Realtime (from realtime.sh)
nself service realtime status       # Realtime status
nself service realtime channels     # List channels
nself service realtime broadcast    # Broadcast message

# Rate limiting (from rate-limit.sh)
nself service rate-limit config     # Configure limits
nself service rate-limit status     # Check status
nself service rate-limit reset      # Reset limits
```

**Migration:**
- `nself admin` → `nself service admin` (legacy alias)
- `nself admin-dev` → `nself service admin dev` (legacy alias)
- `nself email` → `nself service email` (legacy alias)
- `nself search` → `nself service search` (legacy alias)
- `nself functions` → `nself service functions` (legacy alias)
- `nself mlflow` → `nself service mlflow` (legacy alias)
- `nself storage` → `nself service storage` (legacy alias)
- `nself redis` → `nself service cache` (legacy alias)
- `nself realtime` → `nself service realtime` (legacy alias)
- `nself rate-limit` → `nself service rate-limit` (legacy alias)

---

### 6. Deploy (`nself deploy`) - EXPANDED

**CURRENT**: `deploy`, `staging`, `prod`, `rollback`, `env`, `sync`, `validate`
**PROPOSED**: Consolidate deployment operations

```bash
# Environment management (from env.sh)
nself deploy env list               # List environments
nself deploy env create <name>      # Create environment
nself deploy env switch <name>      # Switch environment
nself deploy env diff <e1> <e2>     # Compare environments

# Basic deployment (from deploy.sh)
nself deploy staging                # Deploy to staging
nself deploy production             # Deploy to production
nself deploy preview                # Preview environments
nself deploy canary                 # Canary deployment
nself deploy blue-green             # Blue-green deployment
nself deploy rollback               # Rollback deployment
nself deploy check                  # Pre-deploy validation
nself deploy status                 # Deployment status

# Data sync (from sync.sh)
nself deploy sync db <env>          # Sync database
nself deploy sync files <env>       # Sync files
nself deploy sync config <env>      # Sync config
nself deploy sync full <env>        # Full sync
nself deploy sync auto              # Auto-sync
nself deploy sync watch             # Watch mode
nself deploy sync status            # Sync status
nself deploy sync history           # Sync history

# Validation (from validate.sh)
nself deploy validate               # Validate deployment
nself deploy validate config        # Validate config
nself deploy validate env           # Validate environment
```

**Migration:**
- `nself staging` → `nself deploy staging` (legacy alias)
- `nself prod` → `nself deploy production` (legacy alias)
- `nself rollback` → `nself deploy rollback` (legacy alias)
- `nself env` → `nself deploy env` (legacy alias)
- `nself sync` → `nself deploy sync` (legacy alias)
- `nself validate` → `nself deploy validate` (legacy alias)

---

### 7. Cloud (`nself cloud`) - KEEP & EXPAND

**CURRENT**: `cloud`, `providers`, `provision`, `server`, `servers`
**PROPOSED**: Consolidate infrastructure

```bash
# Provider management (already in cloud.sh)
nself cloud provider list           # List providers
nself cloud provider init <prov>    # Configure credentials
nself cloud provider validate       # Validate config
nself cloud provider info <prov>    # Provider details

# Server management (from server.sh, servers.sh)
nself cloud server create <prov>    # Provision server
nself cloud server destroy <srv>    # Destroy server
nself cloud server list             # List servers
nself cloud server status [srv]     # Server status
nself cloud server ssh <srv>        # SSH to server
nself cloud server add <ip>         # Add existing server
nself cloud server remove <srv>     # Remove from registry

# Cost management (already in cloud.sh)
nself cloud cost estimate <prov>    # Estimate costs
nself cloud cost compare            # Compare providers

# Quick deployment (already in cloud.sh)
nself cloud deploy quick            # Provision + deploy
nself cloud deploy full             # Full production setup

# Kubernetes (from k8s.sh)
nself cloud k8s init                # Initialize K8s
nself cloud k8s convert             # Compose to manifests
nself cloud k8s apply               # Apply manifests
nself cloud k8s deploy              # Full deployment
nself cloud k8s status              # Deployment status
nself cloud k8s logs <service>      # Pod logs
nself cloud k8s scale <svc> <n>     # Scale deployment
nself cloud k8s rollback <service>  # Rollback deployment
nself cloud k8s delete              # Delete deployment
nself cloud k8s cluster             # Cluster management
nself cloud k8s namespace           # Namespace management

# Helm (from helm.sh)
nself cloud helm init               # Initialize chart
nself cloud helm generate           # Generate chart
nself cloud helm install            # Install to cluster
nself cloud helm upgrade            # Upgrade release
nself cloud helm rollback           # Rollback release
nself cloud helm uninstall          # Remove release
nself cloud helm list               # List releases
nself cloud helm status             # Release status
nself cloud helm values             # Show/edit values
nself cloud helm template           # Render locally
nself cloud helm package            # Package chart
nself cloud helm repo               # Repository mgmt
```

**Migration:**
- `nself providers` → `nself cloud provider` (legacy alias)
- `nself provision` → `nself cloud server create` (legacy alias)
- `nself server` → `nself cloud server` (legacy alias)
- `nself servers` → `nself cloud server list` (legacy alias)
- `nself k8s` → `nself cloud k8s` (legacy alias)
- `nself helm` → `nself cloud helm` (legacy alias)

---

### 8. Observe (`nself observe`) - NEW CATEGORY

**CURRENT**: `logs`, `metrics`, `monitor`, `health`, `status`, `doctor`, `history`, `urls`, `exec`
**PROPOSED**: Unified observability

```bash
# Logging (from logs.sh)
nself observe logs [service]        # View logs
nself observe logs --follow         # Follow logs
nself observe logs --tail 100       # Last N lines
nself observe logs --since 1h       # Time filter

# Metrics (from metrics.sh)
nself observe metrics [service]     # View metrics
nself observe metrics export        # Export metrics
nself observe metrics dashboard     # Open dashboard

# Monitoring (from monitor.sh)
nself observe monitor status        # Monitor status
nself observe monitor enable        # Enable monitoring
nself observe monitor alerts        # View alerts
nself observe monitor grafana       # Open Grafana

# Health (from health.sh)
nself observe health check          # Run health checks
nself observe health service <name> # Check service
nself observe health endpoint <url> # Check endpoint
nself observe health watch          # Continuous monitoring
nself observe health history        # Check history
nself observe health config         # Health config

# Diagnostics (from doctor.sh)
nself observe doctor                # Run diagnostics
nself observe doctor --fix          # Auto-repair issues

# History/Audit (from history.sh, audit.sh)
nself observe history show          # Recent history
nself observe history deployments   # Deploy history
nself observe history migrations    # Migration history
nself observe history rollbacks     # Rollback history
nself observe history commands      # Command history
nself observe history search <q>    # Search history
nself observe history export        # Export history
nself observe history clear         # Clear history

# Audit trail (from audit.sh)
nself observe audit events          # Audit events
nself observe audit users           # User actions
nself observe audit export          # Export audit log

# URLs (from urls.sh)
nself observe urls                  # Show service URLs
nself observe urls --env <env>      # Specific environment
nself observe urls --diff           # Compare environments
nself observe urls --json           # JSON output

# Shell access (from exec.sh)
nself observe exec <service> <cmd>  # Execute in container
nself observe exec --shell          # Interactive shell
```

**Migration:**
- `nself logs` → `nself observe logs` (legacy alias)
- `nself metrics` → `nself observe metrics` (legacy alias)
- `nself monitor` → `nself observe monitor` (legacy alias)
- `nself health` → `nself observe health` (legacy alias)
- `nself doctor` → `nself observe doctor` (legacy alias)
- `nself history` → `nself observe history` (legacy alias)
- `nself audit` → `nself observe audit` (legacy alias)
- `nself urls` → `nself observe urls` (legacy alias)
- `nself exec` → `nself observe exec` (legacy alias)

---

### 9. Secure (`nself secure`) - NEW CATEGORY

**CURRENT**: `security`, `mfa`, `secrets`, `vault`, `ssl`, `trust`
**PROPOSED**: Unified security

```bash
# Security scanning (from security.sh)
nself secure scan                   # Scan vulnerabilities
nself secure scan passwords         # Weak passwords
nself secure scan mfa               # Missing MFA
nself secure scan sessions          # Expired sessions
nself secure scan suspicious        # Suspicious activity

# Security incidents (from security.sh)
nself secure incidents list         # List incidents
nself secure incidents show <id>    # Show incident
nself secure incidents resolve <id> # Resolve incident

# Security events (from security.sh)
nself secure events list            # List events
nself secure events show <id>       # Show event

# WebAuthn/FIDO2 (from security.sh)
nself secure webauthn list          # List keys
nself secure webauthn add           # Add key
nself secure webauthn remove <id>   # Remove key

# Security headers (from security.sh)
nself secure headers show           # Show headers
nself secure headers config         # Configure headers
nself secure headers test           # Test headers

# Secrets (from secrets.sh)
nself secure secrets list           # List secrets
nself secure secrets set <key>      # Set secret
nself secure secrets get <key>      # Get secret
nself secure secrets delete <key>   # Delete secret
nself secure secrets rotate         # Rotate secrets

# Vault (from vault.sh)
nself secure vault init             # Initialize vault
nself secure vault status           # Vault status
nself secure vault seal             # Seal vault
nself secure vault unseal           # Unseal vault
nself secure vault backup           # Backup vault
nself secure vault restore          # Restore vault

# SSL/TLS (from ssl.sh, trust.sh)
nself secure ssl generate           # Generate cert
nself secure ssl install            # Install cert
nself secure ssl renew              # Renew cert
nself secure ssl trust              # Trust local cert
nself secure ssl verify             # Verify cert
```

**Migration:**
- `nself security` → `nself secure` (legacy alias)
- `nself secrets` → `nself secure secrets` (legacy alias)
- `nself vault` → `nself secure vault` (legacy alias)
- `nself ssl` → `nself secure ssl` (legacy alias)
- `nself trust` → `nself secure ssl trust` (legacy alias)

---

### 10. Plugin (`nself plugin`) - KEEP AS-IS

**Already well-organized, no changes needed**

```bash
nself plugin list                   # List plugins
nself plugin install <name>         # Install plugin
nself plugin remove <name>          # Remove plugin
nself plugin update [name]          # Update plugin
nself plugin updates                # Check updates
nself plugin refresh                # Refresh registry
nself plugin status [name]          # Plugin status

# Plugin-specific actions
nself plugin stripe <action>        # Stripe commands
nself plugin github <action>        # GitHub commands
nself plugin shopify <action>       # Shopify commands
```

---

### 11. Dev (`nself dev`) - EXPANDED

**CURRENT**: `dev`, `bench`, `perf`, `scale`, `frontend`, `ci`, `completion`
**PROPOSED**: Developer tools

```bash
# Development mode (from dev.sh)
nself dev start                     # Start dev mode
nself dev stop                      # Stop dev mode
nself dev status                    # Dev mode status
nself dev watch                     # Watch for changes

# Performance (from perf.sh)
nself dev perf profile [service]    # Performance profile
nself dev perf analyze              # Analyze performance
nself dev perf slow-queries         # Slow queries
nself dev perf report               # Generate report
nself dev perf dashboard            # Real-time dashboard
nself dev perf suggest              # Optimization tips

# Benchmarking (from bench.sh)
nself dev bench run [target]        # Run benchmark
nself dev bench baseline            # Establish baseline
nself dev bench compare [file]      # Compare to baseline
nself dev bench stress [target]     # Stress test
nself dev bench report              # Benchmark report

# Scaling (from scale.sh)
nself dev scale <service> <n>       # Scale service
nself dev scale status              # Scale status
nself dev scale --auto              # Autoscaling

# Frontend (from frontend.sh)
nself dev frontend status           # Frontend status
nself dev frontend list             # List frontends
nself dev frontend add <name>       # Add frontend
nself dev frontend remove <name>    # Remove frontend
nself dev frontend deploy <name>    # Deploy frontend
nself dev frontend logs <name>      # Deploy logs
nself dev frontend env <name>       # Environment vars

# CI/CD (from ci.sh)
nself dev ci init <platform>        # Generate workflow
nself dev ci validate               # Validate config
nself dev ci status                 # CI status

# Shell completion (from completion.sh)
nself dev completion bash           # Bash completions
nself dev completion zsh            # Zsh completions
nself dev completion fish           # Fish completions
nself dev completion install <sh>   # Auto-install
```

**Migration:**
- `nself dev` → `nself dev start` (legacy alias for backward compat)
- `nself perf` → `nself dev perf` (legacy alias)
- `nself bench` → `nself dev bench` (legacy alias)
- `nself scale` → `nself dev scale` (legacy alias)
- `nself frontend` → `nself dev frontend` (legacy alias)
- `nself ci` → `nself dev ci` (legacy alias)
- `nself completion` → `nself dev completion` (legacy alias)

---

### 12. Config (`nself config`) - EXPANDED

**CURRENT**: `config`, `reset`, `clean`
**PROPOSED**: Configuration management

```bash
# Configuration (from config.sh)
nself config show                   # Show config
nself config get <key>              # Get value
nself config set <key> <val>        # Set value
nself config list                   # List keys
nself config edit                   # Open in editor
nself config validate               # Validate config
nself config diff <e1> <e2>         # Compare envs
nself config export                 # Export config
nself config import <file>          # Import config
nself config reset                  # Reset to defaults

# Cleanup (from clean.sh, reset.sh)
nself config clean                  # Clean Docker resources
nself config clean --volumes        # Clean volumes
nself config clean --networks       # Clean networks
nself config clean --images         # Clean images
nself config reset                  # Reset to clean state
nself config reset --hard           # Hard reset (data loss)
```

**Migration:**
- `nself clean` → `nself config clean` (legacy alias)
- `nself reset` → `nself config reset` (legacy alias)

---

### 13. Help & Utilities

**KEEP**: `help`, `version`, `update`, `upgrade`

```bash
nself help [command]                # Show help
nself version                       # Version info
nself update                        # Update nself
nself upgrade                       # Upgrade to new version
```

**Additional utilities kept:**
- `nself up` → alias for `nself start`
- `nself down` → alias for `nself stop`

---

## Summary of Changes

### Top-Level Commands: Before & After

| Category | Before | After | Change |
|----------|--------|-------|--------|
| **Core Lifecycle** | 6 | 6 | No change |
| **Database** | 1 (+3 aliases) | 1 | Consolidated |
| **Auth & Security** | 8 | 2 | 75% reduction |
| **Multi-Tenant** | 1 (+3 aliases) | 1 | Consolidated |
| **Services** | 11 | 1 | 91% reduction |
| **Deployment** | 8 | 1 | 87% reduction |
| **Cloud & Infra** | 8 | 1 | 87% reduction |
| **Observability** | 9 | 1 | 89% reduction |
| **Security** | 6 | 1 | 83% reduction |
| **Plugins** | 1 | 1 | No change |
| **Developer Tools** | 7 | 1 | 86% reduction |
| **Config** | 3 | 1 | 67% reduction |
| **Utilities** | 8 | 4 | 50% reduction |
| **TOTAL** | **77** | **22** | **71% reduction** |

### New Top-Level Commands (13 categories)

```
Core (6):     init, build, start, stop, restart, status
Categories:   db, auth, tenant, service, deploy, cloud, observe, secure
Tools:        plugin, dev, config
Utilities:    help, version, update, upgrade
Aliases:      up, down
```

---

## Migration Strategy

### Phase 1: Add New Commands (Non-Breaking)

1. Create new command structure alongside existing
2. New commands are fully functional
3. Old commands remain unchanged
4. No deprecation warnings yet

**Timeline**: 1-2 weeks
**Risk**: Low (additive only)

### Phase 2: Add Deprecation Warnings

1. Old commands show deprecation notice
2. Suggest new command in output
3. Continue working normally
4. Update documentation with migration guide

**Example:**
```bash
$ nself logs postgres
⚠️  DEPRECATED: 'nself logs' will be removed in v1.0
    Use: nself observe logs postgres

[logs continue as normal...]
```

**Timeline**: 2-4 weeks
**Risk**: Low (warnings only)

### Phase 3: Legacy Alias System

1. Create smart aliases that redirect
2. Track usage with telemetry (opt-in)
3. Maintain for 2+ major versions
4. Document migration path

**Timeline**: Ongoing
**Risk**: Low (backward compatible)

### Phase 4: Remove Old Commands (v1.0+)

1. Remove old command files
2. Show helpful error messages
3. Point to new commands
4. Update all documentation

**Example:**
```bash
$ nself logs
Error: 'nself logs' has been removed
Use:   nself observe logs

For migration guide: nself help migrate
```

**Timeline**: 6-12 months after Phase 1
**Risk**: Medium (breaking change)

---

## Backward Compatibility

### Legacy Alias Map

All existing commands will have aliases for at least 2 major versions:

```bash
# Authentication
nself oauth → nself auth oauth
nself mfa → nself auth mfa
nself devices → nself auth devices
nself roles → nself auth roles
nself webhooks → nself auth webhooks

# Services
nself admin → nself service admin
nself email → nself service email
nself search → nself service search
nself functions → nself service functions
nself mlflow → nself service mlflow
nself storage → nself service storage
nself redis → nself service cache
nself realtime → nself service realtime
nself rate-limit → nself service rate-limit

# Deployment
nself staging → nself deploy staging
nself prod → nself deploy production
nself rollback → nself deploy rollback
nself env → nself deploy env
nself sync → nself deploy sync
nself validate → nself deploy validate

# Cloud
nself providers → nself cloud provider
nself provision → nself cloud server create
nself server → nself cloud server
nself servers → nself cloud server list
nself k8s → nself cloud k8s
nself helm → nself cloud helm

# Observability
nself logs → nself observe logs
nself metrics → nself observe metrics
nself monitor → nself observe monitor
nself health → nself observe health
nself doctor → nself observe doctor
nself history → nself observe history
nself audit → nself observe audit
nself urls → nself observe urls
nself exec → nself observe exec

# Security
nself security → nself secure
nself secrets → nself secure secrets
nself vault → nself secure vault
nself ssl → nself secure ssl
nself trust → nself secure ssl trust

# Developer tools
nself perf → nself dev perf
nself bench → nself dev bench
nself scale → nself dev scale
nself frontend → nself dev frontend
nself ci → nself dev ci
nself completion → nself dev completion

# Config & maintenance
nself clean → nself config clean
nself reset → nself config reset

# Tenant/org
nself billing → nself tenant billing
nself whitelabel → nself tenant branding
nself org → nself tenant

# Database
nself backup → nself db backup
nself restore → nself db restore
nself migrate → nself db migrate
```

---

## Implementation Guide

### File Structure Changes

```
src/cli/
├── init.sh                     # Core (keep)
├── build.sh                    # Core (keep)
├── start.sh                    # Core (keep)
├── stop.sh                     # Core (keep)
├── restart.sh                  # Core (keep)
├── status.sh                   # Core (keep)
│
├── db.sh                       # Database (keep, enhance)
├── auth.sh                     # Auth (expand to absorb oauth, mfa, devices, roles, webhooks)
├── tenant.sh                   # Tenant (keep as-is)
├── service.sh                  # Service (expand to absorb email, search, etc.)
│
├── deploy.sh                   # Deploy (expand to absorb env, sync, validate)
├── cloud.sh                    # Cloud (expand to absorb k8s, helm)
├── observe.sh                  # NEW (consolidate logs, metrics, monitor, health, doctor, etc.)
├── secure.sh                   # NEW (consolidate security, secrets, vault, ssl)
│
├── plugin.sh                   # Plugin (keep as-is)
├── dev.sh                      # Dev (expand to absorb perf, bench, scale, frontend, ci)
├── config.sh                   # Config (expand to absorb clean, reset)
│
├── help.sh                     # Utilities (keep)
├── version.sh                  # Utilities (keep)
├── update.sh                   # Utilities (keep)
├── upgrade.sh                  # Utilities (keep)
│
└── legacy/                     # NEW - Legacy alias handlers
    ├── oauth.sh → auth.sh
    ├── mfa.sh → auth.sh
    ├── logs.sh → observe.sh
    ├── metrics.sh → observe.sh
    └── ... (all other aliases)
```

### Code Changes Required

1. **Create 2 new command files:**
   - `src/cli/observe.sh` (consolidate observability)
   - `src/cli/secure.sh` (consolidate security)

2. **Expand 6 existing files:**
   - `auth.sh` - absorb oauth, mfa, devices, roles, webhooks
   - `service.sh` - absorb email, search, functions, mlflow, storage, redis, realtime, rate-limit
   - `deploy.sh` - absorb env, sync, validate, staging, prod, rollback
   - `cloud.sh` - absorb k8s, helm (partially done)
   - `dev.sh` - absorb perf, bench, scale, frontend, ci, completion
   - `config.sh` - absorb clean, reset

3. **Create legacy alias system:**
   - New directory: `src/cli/legacy/`
   - Smart redirects with deprecation warnings
   - Telemetry tracking (opt-in)

4. **Update main dispatcher:**
   - `src/cli/nself.sh` - route to new structure
   - Handle legacy aliases
   - Show deprecation warnings

5. **Update help system:**
   - `src/cli/help.sh` - new command tree
   - Category-based help
   - Search functionality

6. **Update shell completion:**
   - `src/cli/completion.sh` - new structure
   - Support for both new and legacy

---

## Benefits

### For Users

1. **Easier Discovery**: 13 categories vs 77 commands
2. **Logical Grouping**: Related features together
3. **Less Cognitive Load**: Clear mental model
4. **Better Help**: Organized by category
5. **Consistent Patterns**: Predictable command structure
6. **Backward Compatible**: Legacy commands still work

### For Developers

1. **Easier Maintenance**: Related code together
2. **Clear Ownership**: Each category has a purpose
3. **Better Testing**: Test by category
4. **Simpler Docs**: Category-based documentation
5. **Less Code Duplication**: Shared logic within categories

### For Documentation

1. **Organized Guides**: One guide per category
2. **Better Examples**: Category-specific examples
3. **Easier Navigation**: Clear hierarchy
4. **Reduced Redundancy**: Consolidated documentation

---

## Risks & Mitigation

### Risk 1: Breaking Existing Scripts

**Mitigation:**
- Maintain legacy aliases for 2+ versions
- Provide migration guide
- Show deprecation warnings
- Track usage to guide sunset

### Risk 2: User Confusion During Transition

**Mitigation:**
- Clear migration documentation
- Both systems work simultaneously
- Helpful error messages
- Migration checklist

### Risk 3: Increased Command Length

**Example:** `nself logs` → `nself observe logs`

**Mitigation:**
- Shell aliases: `alias nslog='nself observe logs'`
- Tab completion reduces typing
- Keep common commands short
- Legacy aliases still work

### Risk 4: Implementation Complexity

**Mitigation:**
- Phased rollout (4 phases)
- Start with additive changes
- Extensive testing
- Rollback plan for each phase

---

## Success Metrics

### Quantitative

- **Command Count**: 77 → 22 (71% reduction)
- **Help Text Length**: ~300 lines → ~150 lines (50% reduction)
- **Tab Completions**: Faster with less noise
- **User Support Tickets**: Should decrease

### Qualitative

- Users can find commands without docs
- Logical grouping makes sense
- Help text is scannable
- New users onboard faster

---

## Next Steps

### Immediate (Week 1-2)

1. **Review & Approve**: Stakeholder review of this proposal
2. **Finalize Categories**: Confirm 13 categories
3. **Design Alias System**: How legacy commands redirect
4. **Create Task Breakdown**: Detailed implementation tasks

### Short-term (Week 3-6)

1. **Implement Phase 1**: New commands alongside old
2. **Create observe.sh**: New observability category
3. **Create secure.sh**: New security category
4. **Expand existing**: auth, service, deploy, cloud, dev, config
5. **Build alias system**: Legacy command redirects
6. **Update tests**: New command structure

### Medium-term (Week 7-12)

1. **Implement Phase 2**: Add deprecation warnings
2. **Update documentation**: Migration guide
3. **Update shell completion**: Both old and new
4. **User testing**: Get feedback
5. **Iterate**: Refine based on feedback

### Long-term (3-12 months)

1. **Monitor usage**: Track old vs new commands
2. **Gather feedback**: Community input
3. **Plan Phase 4**: Removal of old commands (v1.0)
4. **Update examples**: Use new commands everywhere
5. **Sunset legacy**: Eventually remove (v1.0+)

---

## Alternatives Considered

### Alternative 1: Keep Current Structure

**Pros:** No breaking changes, no migration effort
**Cons:** Continues to scale poorly, user confusion persists
**Decision:** Rejected - technical debt compounds

### Alternative 2: Radical Restructure (Git-like)

Structure like `git`: `nself <noun> <verb>` (e.g., `nself service start`, `nself database migrate`)

**Pros:** Very consistent pattern
**Cons:** Incompatible with current structure, massive breaking change
**Decision:** Rejected - too disruptive

### Alternative 3: Keep All Top-Level, Add Categories

Add categories as alternate paths but keep all top-level commands.

**Pros:** Zero breaking changes
**Cons:** Doesn't solve the core problem, doubles the API surface
**Decision:** Rejected - makes problem worse

### Alternative 4: This Proposal (Chosen)

Consolidate into 13 categories with legacy aliases.

**Pros:** Reduces complexity, logical grouping, backward compatible
**Cons:** Requires migration effort, some commands get longer
**Decision:** **ACCEPTED** - Best balance of improvement and compatibility

---

## Conclusion

This reorganization reduces top-level commands by 71% (77 → 22) while improving discoverability, consistency, and maintainability. The phased migration approach ensures backward compatibility and minimizes user disruption.

The new structure groups related commands logically:
- **Auth**: All authentication & authorization
- **Observe**: All monitoring & observability
- **Secure**: All security operations
- **Service**: All optional services
- **Deploy**: All deployment operations
- **Cloud**: All infrastructure
- **Dev**: All developer tools
- **Config**: All configuration

This creates a clear mental model for users and makes nself easier to learn, use, and maintain.

---

**Recommendation**: Proceed with Phase 1 implementation (new commands alongside old) and gather user feedback before committing to deprecation timeline.
