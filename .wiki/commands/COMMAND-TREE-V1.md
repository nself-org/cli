# nself Command Tree v1.0

**Complete command hierarchy for nself v1.0**

This is the authoritative command structure after consolidation from 79 → 30 top-level commands.

> **Note on backward-compatibility stubs**: ~38 deprecated command files exist alongside these 30 commands (e.g. `nself email`, `nself ssl`, `nself staging`, `nself helm`, `nself destroy`, `nself perf`, `nself backup`, `nself hasura`). They show a deprecation warning and delegate to the consolidated command. They are NOT part of the v1.0 command surface and will be removed in v1.0.0. If a command you use isn't listed here, check its wiki page for the replacement command.

**New in v0.9.6:**

- `infra destroy` - Safe infrastructure destruction with selective targeting (was standalone `nself destroy`)
- `deploy server` - 10 new subcommands for complete VPS lifecycle management
- `deploy sync` - 4 subcommands for environment synchronization
- `infra provider k8s-*` - Unified Kubernetes management across 8 cloud providers

---

## Core (5 commands)

### init
```
nself init [--demo|--simple|--full]
```
Initialize a new nself project with configuration wizard.

### build
```
nself build [--force|--clean]
```
Generate Docker Compose configs, Nginx configs, and service files.

### start
```
nself start [service...]
```
Start all services or specific services.

### stop
```
nself stop [service...]
```
Stop all services or specific services.

### restart
```
nself restart [service...]
```
Restart all services or specific services.

---

## Utilities (16 commands)

### status
```
nself status [service...]
```
Show health status of all services or specific services.

### logs
```
nself logs <service> [-f|--follow] [--tail N]
```
View logs for a specific service.

### help
```
nself help [command]
```
Show general help or help for a specific command.

### admin
```
nself admin [--dev]
```
Open the nself Admin UI.

### urls
```
nself urls [--json]
```
Display all service URLs and routes.

### exec
```
nself exec <service> <command> [args...]
```
Execute a command inside a service container.

### doctor
```
nself doctor [--fix] [--verbose]
```
Run diagnostics and check for common issues.

### monitor
```
nself monitor [dashboard]
```
Access monitoring dashboards (Grafana, Prometheus, etc.).

### health
```
nself health [service...] [--deep]
```
Perform health checks on services.

### version
```
nself version [--check]
```
Show nself version and check for updates.

### update
```
nself update [--preview] [--force]
```
Update nself CLI to the latest version.

### completion
```
nself completion <bash|zsh|fish>
```
Generate shell completion scripts.

### metrics
```
nself metrics [service] [--profile]
```
View metrics and performance data.

### history
```
nself history [--limit N] [--filter TYPE]
```
View command history and audit trail.

### audit
```
nself audit [--export] [--format json|csv]
```
View audit logs and security events.

### harden
```
nself harden [--all] [--check]
```
Security hardening for nself infrastructure.

---

## Complex (9 commands)

### 1. db - Database Operations
```
nself db <subcommand>

Subcommands:
  migrate <up|down|status|create|rollback>   # Migration management
  checklist                                  # Database migration checklist
  schema <dump|load|diff|validate>           # Schema operations
  seed [dataset]                             # Seed data
  mock <table> [--count N]                   # Generate mock data
  backup [--output FILE]                     # Backup database
  backup list                                # List available backups
  restore <file>                             # Restore from backup
  shell                                      # Interactive psql shell
  query <sql>                                # Execute SQL query
  types <language>                           # Generate type definitions
  inspect [table]                            # Database inspection
  data <import|export> [options]             # Data operations
  hasura console                             # Open Hasura Console
  hasura metadata apply                      # Apply Hasura metadata
  hasura metadata export                     # Export Hasura metadata
  hasura metadata reload                     # Reload metadata cache
```

**Total subcommands:** 17

---

### 2. tenant - Multi-Tenancy & Billing
```
nself tenant <subcommand>

Core Tenant Management:
  init                                       # Initialize multi-tenancy
  create <name> [--plan PLAN]                # Create new tenant
  list [--status STATUS]                     # List tenants
  show <tenant-id>                           # Show tenant details
  update <tenant-id> [options]               # Update tenant
  suspend <tenant-id>                        # Suspend tenant
  activate <tenant-id>                       # Activate tenant
  delete <tenant-id>                         # Delete tenant

Member Management:
  member add <tenant-id> <email> <role>      # Add member
  member remove <tenant-id> <user-id>        # Remove member
  member list <tenant-id>                    # List members
  member update <tenant-id> <user-id>        # Update member
  member role <tenant-id> <user-id> <role>   # Change role
  member invite <tenant-id> <email>          # Invite member
  member accept <token>                      # Accept invitation

Settings Management:
  setting get <tenant-id> <key>              # Get setting
  setting set <tenant-id> <key> <value>      # Set setting
  setting list <tenant-id>                   # List settings
  setting delete <tenant-id> <key>           # Delete setting
  setting reset <tenant-id>                  # Reset to defaults

Billing Management (consolidated from 'billing' command):
  billing plans                              # Manage plans
  billing subscribe <tenant-id> <plan>       # Subscribe to plan
  billing cancel <tenant-id>                 # Cancel subscription
  billing usage <tenant-id>                  # View usage
  billing invoice <tenant-id>                # View invoices
  billing payment <tenant-id>                # Payment methods
  billing stripe                             # Stripe integration
  billing test                               # Test billing

Organization Management (consolidated from 'org' command):
  org create <name>                          # Create organization
  org list                                   # List organizations
  org show <org-id>                          # Show details
  org members <org-id>                       # Manage members
  org delete <org-id>                        # Delete organization

Branding:
  branding logo <tenant-id> <file>           # Upload logo
  branding colors <tenant-id> [options]      # Set brand colors
  branding preview <tenant-id>               # Preview branding
  branding reset <tenant-id>                 # Reset to defaults

Custom Domains:
  domains add <tenant-id> <domain>           # Add custom domain
  domains remove <tenant-id> <domain>        # Remove domain
  domains verify <tenant-id> <domain>        # Verify DNS
  domains list <tenant-id>                   # List domains
  domains ssl <tenant-id> <domain>           # SSL certificate
  domains primary <tenant-id> <domain>       # Set primary domain

Email Templates:
  email list <tenant-id>                     # List templates
  email edit <tenant-id> <template>          # Edit template
  email preview <tenant-id> <template>       # Preview template
  email reset <tenant-id> <template>         # Reset to default

Themes:
  themes list                                # List themes
  themes apply <tenant-id> <theme>           # Apply theme
  themes customize <tenant-id>               # Customize theme
  themes preview <tenant-id>                 # Preview theme
  themes reset <tenant-id>                   # Reset theme
```

**Total subcommands:** 50+

---

### 3. deploy - Deployment & Remote Environments
```
nself deploy <subcommand>

Deployment:
  staging [--auto-migrate]                   # Deploy to staging
  production [--auto-migrate]                # Deploy to production
  preview <branch>                           # Create preview environment
  canary <percentage>                        # Canary deployment
  blue-green                                 # Zero-downtime deployment
  rollback [--version N]                     # Rollback deployment
  upgrade [--zero-downtime]                  # Upgrade deployment (was: separate command)
  status                                     # Deployment status
  config <env>                               # Deployment config
  logs <deployment>                          # Deployment logs
  history                                    # Deployment history
  promote <from> <to>                        # Promote environment

Remote Server Management (consolidated from 'server', 'servers', 'provision'):
  provision <provider> [options]             # Provision remote server (was: separate command)

  server init <host> [--domain DOMAIN]       # Initialize VPS for nself (NEW v0.9.6)
  server check <host>                        # Verify server readiness (NEW v0.9.6)
  server status [server-id]                  # Quick status of all/specific servers (NEW v0.9.6)
  server diagnose <env>                      # Comprehensive diagnostics (NEW v0.9.6)
  server list                                # List all configured servers (NEW v0.9.6)
  server add <name> --host <host> [options]  # Add server configuration (NEW v0.9.6)
  server remove <name> [--force]             # Remove server configuration (NEW v0.9.6)
  server ssh <name> [command]                # Quick SSH connection (NEW v0.9.6)
  server info <name>                         # Display comprehensive server info (NEW v0.9.6)
  server create <name> [options]             # Create server (legacy)
  server destroy <server-id>                 # Destroy server (legacy)

Synchronization (consolidated from 'sync' command):
  sync pull <env> [--dry-run] [--force]      # Pull configuration from remote (NEW v0.9.6)
  sync push <env> [--dry-run] [--force]      # Push configuration to remote (NEW v0.9.6)
  sync status                                # Show synchronization status (NEW v0.9.6)
  sync full <env> [--no-rebuild]             # Complete synchronization (NEW v0.9.6)
```

**Total subcommands:** 33 (10 new server subcommands + 4 sync subcommands)

**New in v0.9.6 - Server Management:**
- Complete VPS lifecycle management
- Automated server initialization with security hardening
- Comprehensive health checks and diagnostics
- SSH connection management
- Environment file synchronization

**Related Documentation:** [Server Management Guide](../deployment/SERVER-MANAGEMENT.md)

---

### 4. infra - Infrastructure Management
```
nself infra <subcommand>

Cloud Providers (consolidated from 'provider', 'cloud'):
  provider list [--filter TYPE]              # List 26+ providers
  provider init <provider>                   # Configure credentials
  provider validate <provider>               # Validate configuration
  provider info <provider>                   # Provider details
  provider install <provider>                # Install provider CLI (NEW v0.9.6)
  provider test <provider>                   # Test provider connection (NEW v0.9.6)
  provider server create <provider> [opts]   # Provision server
  provider server destroy <id>               # Destroy server
  provider server list                       # List servers
  provider server status <id>                # Server status
  provider server ssh <id>                   # SSH to server
  provider server add <host>                 # Add existing server
  provider server remove <id>                # Remove server
  provider cost estimate <provider>          # Estimate costs
  provider cost compare                      # Compare providers
  provider deploy quick <provider>           # Quick deploy
  provider deploy full <provider>            # Full production setup

  # Kubernetes Abstraction (NEW v0.9.6)
  provider k8s-create <provider> <name> <region> <nodes> <size>  # Create managed K8s cluster
  provider k8s-delete <provider> <name> [region]                 # Delete managed K8s cluster
  provider k8s-kubeconfig <provider> <name> [region]             # Get kubeconfig credentials

  # Supported K8s Providers (8 total):
  # - aws (EKS)           - $73/month control plane
  # - gcp (GKE)           - Free control plane
  # - azure (AKS)         - Free control plane
  # - digitalocean (DOKS) - $12/month
  # - linode (LKE)        - Free control plane
  # - vultr (VKE)         - Free control plane
  # - hetzner             - Free control plane (manual setup via console)
  # - scaleway (Kapsule)  - Free control plane

Kubernetes (consolidated from 'k8s'):
  k8s init [--provider PROVIDER]             # Initialize K8s config
  k8s convert                                # Convert Compose to K8s
  k8s apply                                  # Apply manifests
  k8s deploy                                 # Full deployment
  k8s status                                 # Deployment status
  k8s logs <pod>                             # Pod logs
  k8s scale <deployment> <replicas>          # Scale deployment
  k8s rollback                               # Rollback deployment
  k8s delete                                 # Delete deployment
  k8s cluster <action>                       # Cluster management
  k8s namespace <action>                     # Namespace management

Helm (consolidated from 'helm'):
  helm init                                  # Initialize Helm chart
  helm generate                              # Generate/update chart
  helm install <release>                     # Install to cluster
  helm upgrade <release>                     # Upgrade release
  helm rollback <release>                    # Rollback release
  helm uninstall <release>                   # Remove release
  helm list                                  # List releases
  helm status <release>                      # Release status
  helm values <release>                      # Show/edit values
  helm template                              # Render locally
  helm package                               # Package chart
  helm repo <action>                         # Repository management

Infrastructure Reset (consolidated from 'destroy', 'backup reset/clean'):
  destroy [OPTIONS]                          # Safe infrastructure destruction (see DESTROY.md for options)
  reset [--confirm]                          # Reset to clean state
  clean [--age DAYS]                         # Clean old Docker resources
```

**Total subcommands:** 51 (added destroy, reset, clean; added 10 K8s abstraction commands)

**New in v0.9.6 - Kubernetes Abstraction:**
- Unified CLI across 8 cloud providers
- Intelligent node size mapping (small/medium/large/xlarge)
- Automatic kubeconfig configuration
- Multi-cloud deployment support
- Cost-optimized provider selection

**K8s Node Sizes:**
- `small` - Development (~2 vCPU, 4GB RAM)
- `medium` - Production (~2-4 vCPU, 8-16GB RAM)
- `large` - High-performance (~4-8 vCPU, 16-32GB RAM)
- `xlarge` - Enterprise (~8-16 vCPU, 32-64GB RAM)

**Related Documentation:** [Kubernetes Implementation Guide](../infrastructure/K8S-IMPLEMENTATION-GUIDE.md)

---

### 5. service - Service Management
```
nself service <subcommand>

Core Service Operations:
  list [--status STATUS]                     # List services
  enable <service>                           # Enable service
  disable <service>                          # Disable service
  status <service>                           # Service status
  restart <service>                          # Restart service
  logs <service> [-f]                        # Service logs
  init <service> [--template TYPE]           # Initialize from template
  scaffold <name> <type>                     # Scaffold new service
  wizard                                     # Service creation wizard
  search <query>                             # Search services

Admin Service (consolidated from 'admin', 'admin-dev'):
  admin [--dev]                              # Start admin UI

Storage Service (consolidated from 'storage'):
  storage init                               # Initialize storage
  storage upload <file>                      # Upload file
  storage list [path]                        # List files
  storage delete <file>                      # Delete file
  storage config                             # Configure pipeline
  storage status                             # Pipeline status
  storage test                               # Test uploads
  storage graphql-setup                      # Generate GraphQL integration

Email Service (consolidated from 'email'):
  email send <to> <subject> [options]        # Send email
  email template <action>                    # Email templates
  email test <provider>                      # Test email
  email config <provider>                    # Configure provider

Search Service (consolidated from 'search'):
  search init <provider>                     # Initialize search
  search index <action>                      # Manage indexes
  search query <text>                        # Test queries
  search config                              # Configure provider

Redis Cache (consolidated from 'redis'):
  redis init                                 # Initialize Redis
  redis flush [pattern]                      # Flush cache
  redis cli                                  # Redis CLI
  redis stats                                # Cache statistics

Functions (consolidated from 'functions'):
  functions init                             # Initialize functions
  functions deploy <function>                # Deploy function
  functions list                             # List functions
  functions logs <function>                  # Function logs
  functions invoke <function>                # Invoke function

MLflow (consolidated from 'mlflow'):
  mlflow init                                # Initialize MLflow
  mlflow ui                                  # Open MLflow UI
  mlflow experiments                         # List experiments
  mlflow models                              # Model registry

Realtime (consolidated from 'realtime'):
  realtime init                              # Initialize realtime
  realtime events                            # Event management
  realtime test                              # Test connections

Performance (consolidated from 'perf'):
  bench [service] [--duration N]             # Benchmark service performance
  scale <service> <replicas>                 # Scale service replicas
  profile [service] [--duration N]           # Profile service resource usage
  optimize [--auto-fix]                      # Get optimization suggestions
```

**Total subcommands:** 47 (added bench, scale, profile, optimize)

---

### 6. config - Configuration Management
```
nself config <subcommand>

Configuration:
  show [key]                                 # Show configuration
  edit [key]                                 # Edit configuration
  validate                                   # Validate configuration
  export <file>                              # Export configuration
  import <file>                              # Import configuration
  sync <action>                              # Sync configuration

Environment Management (consolidated from 'env'):
  env list                                   # List environments
  env switch <env>                           # Switch environment
  env create <name>                          # Create environment
  env delete <name>                          # Delete environment
  env sync <env>                             # Sync with environment

Secrets Management (consolidated from 'secrets'):
  secrets list                               # List secrets
  secrets get <key>                          # Get secret
  secrets set <key> <value>                  # Set secret
  secrets delete <key>                       # Delete secret
  secrets rotate [key]                       # Rotate secrets

Vault Integration (consolidated from 'vault'):
  vault init                                 # Initialize Vault
  vault config                               # Configure Vault
  vault status                               # Vault status
```

**Total subcommands:** 20

---

### 7. auth - Authentication & Security
```
nself auth <subcommand>

Authentication:
  login [--provider PROVIDER]                # User login
  logout                                     # User logout
  status                                     # Auth status

MFA (consolidated from 'mfa'):
  mfa enable                                 # Enable MFA
  mfa disable                                # Disable MFA
  mfa verify <code>                          # Verify MFA
  mfa backup-codes                           # Generate backup codes

Roles (consolidated from 'roles'):
  roles list                                 # List roles
  roles create <name> [permissions]          # Create role
  roles assign <user> <role>                 # Assign role
  roles remove <user> <role>                 # Remove role

Devices (consolidated from 'devices'):
  devices list                               # List devices
  devices register <device>                  # Register device
  devices revoke <device>                    # Revoke device
  devices trust <device>                     # Trust device

OAuth (consolidated from 'oauth'):
  oauth install                              # Install OAuth service
  oauth enable <provider>                    # Enable provider
  oauth disable <provider>                   # Disable provider
  oauth config <provider>                    # Configure credentials
  oauth test <provider>                      # Test provider
  oauth list                                 # List providers
  oauth status                               # Service status

Security (consolidated from 'security'):
  security scan [--deep]                     # Security scan
  security audit                             # Security audit
  security report                            # Generate report

SSL Management (consolidated from 'ssl', 'trust'):
  ssl generate [domain]                      # Generate certificate
  ssl install <cert>                         # Install certificate
  ssl renew [domain]                         # Renew certificate
  ssl info [domain]                          # Certificate info
  ssl trust                                  # Trust local certificates

Rate Limiting (consolidated from 'rate-limit'):
  rate-limit config [options]                # Configure rate limits
  rate-limit status                          # Rate limit status
  rate-limit reset [ip]                      # Reset rate limits

Webhooks (consolidated from 'webhooks'):
  webhooks create <url> [events]             # Create webhook
  webhooks list                              # List webhooks
  webhooks delete <id>                       # Delete webhook
  webhooks test <id>                         # Test webhook
  webhooks logs <id>                         # Webhook logs
```

**Total subcommands:** 38

---

### 8. dev - Developer Tools

```
nself dev <subcommand>

Developer Mode:
  mode [on|off]                              # Enable/disable dev mode

Frontend Management (consolidated from 'frontend'):
  frontend add <name> <port>                 # Add frontend app
  frontend remove <name>                     # Remove frontend app
  frontend list                              # List frontend apps
  frontend config <name>                     # Configure frontend

CI/CD (consolidated from 'ci'):
  ci generate [--provider PROVIDER]          # Generate CI config
  ci update                                  # Update CI config
  ci templates                               # List CI templates

Documentation (consolidated from 'docs'):
  docs generate                              # Generate documentation
  docs serve                                 # Serve documentation
  docs build                                 # Build documentation

White-label (consolidated from 'whitelabel'):
  whitelabel config [options]                # Configure white-label
  whitelabel preview                         # Preview white-label
  whitelabel deploy                          # Deploy white-label
```

**Total subcommands:** 16

---

### 9. plugin - Plugin System

```
nself plugin <subcommand>

Plugin Management:
  list [--filter TYPE]                       # List available plugins
  install <plugin>                           # Install plugin
  remove <plugin>                            # Remove plugin
  update [plugin]                            # Update plugin(s)
  updates                                    # Check for updates
  refresh                                    # Refresh registry
  status [plugin]                            # Plugin status
  create <name>                              # Create new plugin

Plugin License:
  license                                    # Show Pro license status
  license show                               # Show license key and status
  license validate                           # Validate key against API
  license plugins                            # List Pro Plugins covered

Plugin Runtime:
  start [plugin] [--all]                     # Start plugins
  stop [plugin] [--all]                      # Stop plugins
  restart <plugin>                           # Restart a plugin
  logs <plugin> [-f|--follow]                # View plugin logs
  ps                                         # List running plugins
  running                                    # Alias for ps
  health                                     # Health check all running plugins

Plugin Actions:
  <plugin> <action> [args...]                # Run plugin action
```

**Total subcommands:** 19+ (plugin-specific actions)

---

## Command Consolidation Map

**Commands that moved:**

| Old Command | New Location | Notes |
|-------------|--------------|-------|
| `billing` | `tenant billing` | Billing is tenant-specific |
| `org` | `tenant org` | Organizations are tenant containers |
| `upgrade` | `deploy upgrade` | Upgrade is a deployment operation |
| `staging` | `deploy staging` | Quick access to staging deployment |
| `prod` | `deploy production` | Quick access to prod deployment |
| `provision` | `deploy provision` | Provision for deployment |
| `server` | `deploy server` | Server management for deployment |
| `servers` | `deploy server list` | Alias for server listing |
| `sync` | `deploy sync` or `config sync` | Context-dependent |
| `provider` | `infra provider` | Cloud infrastructure |
| `cloud` | `infra provider` | Deprecated, now provider |
| `k8s` | `infra k8s` | Kubernetes infrastructure |
| `helm` | `infra helm` | Helm infrastructure |
| `storage` | `service storage` | Storage is a service |
| `email` | `service email` | Email is a service |
| `search` | `service search` | Search is a service |
| `redis` | `service redis` | Redis is a service |
| `functions` | `service functions` | Functions are a service |
| `mlflow` | `service mlflow` | MLflow is a service |
| `realtime` | `service realtime` | Realtime is a service |
| `admin-dev` | `service admin --dev` | Dev mode flag |
| `env` | `config env` | Environment configuration |
| `secrets` | `config secrets` | Secrets are configuration |
| `vault` | `config vault` | Vault is for secrets/config |
| `validate` | `config validate` | Configuration validation |
| `mfa` | `auth mfa` | MFA is authentication |
| `roles` | `auth roles` | Roles are auth/security |
| `devices` | `auth devices` | Device management is auth |
| `oauth` | `auth oauth` | OAuth is authentication |
| `security` | `auth security` | Security operations |
| `ssl` | `auth ssl` | SSL is security |
| `trust` | `auth ssl trust` | Trust local certificates |
| `rate-limit` | `auth rate-limit` | Rate limiting is security |
| `webhooks` | `auth webhooks` | Webhook security |
| `destroy` | `infra destroy` | Destruction is infrastructure |
| `bench` | `service bench` | Benchmarking is a service operation |
| `scale` | `service scale` | Scaling is a service operation |
| `migrate` | `db migrate` | Migrations are a DB operation |
| `rollback` | `deploy rollback` | Rollback is a deploy operation |
| `reset` | `infra reset` | Reset is infrastructure |
| `clean` | `infra clean` | Cleanup is infrastructure |
| `perf` | `service bench\|scale\|profile\|optimize` or `db migrate` | Distributed to service and db |
| `backup` | `db backup\|restore`, `deploy rollback`, `infra reset\|clean` | Distributed across commands |
| `hasura` | `db hasura` | Hasura management is database-adjacent |
| `frontend` | `dev frontend` | Frontend is dev tooling |
| `ci` | `dev ci` | CI/CD is dev tooling |
| `docs` | `dev docs` | Documentation is dev tooling |
| `whitelabel` | `dev whitelabel` | White-label is dev tooling |

---

## Summary Statistics

- **Total Top-Level Commands:** 30 (was 79)
- **Reduction:** 62.0%
- **Total Subcommands:** 300+
- **Average Subcommands per TLC:** 10.0

**Category Breakdown:**
- Core: 5 commands (17%)
- Utilities: 16 commands (53%)
- Complex: 9 commands (30%)
- **Total: 5 + 16 + 9 = 30** ✓

**Most Complex Commands (by subcommand count):**

1. infra: 51 subcommands (added destroy, reset, clean)
2. tenant: 50+ subcommands
3. service: 47 subcommands (added bench, scale, profile, optimize)
4. auth: 38 subcommands
5. deploy: 33 subcommands

---

**Version:** 1.0.0 (Breaking)
**Date:** January 2026
**Status:** Approved for implementation
