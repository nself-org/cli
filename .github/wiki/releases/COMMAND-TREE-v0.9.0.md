# nself v0.9.0 Complete Command Tree

**Version:** 0.9.0
**Date:** January 30, 2026
**Total Commands:** 150+

---

## Full Command Hierarchy

```
nself
│
├─── CORE LIFECYCLE (7 commands)
│    ├── init                           Initialize new nself project
│    ├── build                          Generate Docker configs and service files
│    ├── start                          Start all services
│    ├── stop                           Stop all services
│    ├── restart                        Restart services
│    ├── reset                          Reset to clean state
│    └── clean                          Clean Docker resources
│
├─── STATUS & MONITORING (6 commands)
│    ├── status                         Service health and status
│    │    ├── --all-envs               Show status for all environments
│    │    ├── --json                   Output in JSON format
│    │    └── --watch                  Continuous monitoring mode
│    ├── logs [service]                 View service logs
│    │    └── --tail N                 Show last N lines
│    ├── exec <service> <cmd>           Execute command in container
│    ├── urls                           Show all service URLs
│    │    ├── --env <env>              URLs for specific environment
│    │    ├── --diff                   Compare environments
│    │    └── --json                   JSON output
│    ├── doctor                         System diagnostics
│    │    └── --fix                    Auto-repair issues
│    └── health                         Health checks
│         ├── check                    Run all health checks
│         ├── service <name>           Check specific service
│         ├── endpoint <url>           Check custom endpoint
│         ├── watch                    Continuous health monitoring
│         ├── history                  Health check history
│         └── config                   Health configuration
│
├─── DATABASE (10 command groups)
│    ├── db migrate                     Database migrations
│    │    ├── up [N]                   Run N migrations (default: all)
│    │    ├── down [N]                 Rollback N migrations
│    │    ├── create <name>            Create new migration
│    │    └── status                   Migration status
│    ├── db schema                      Schema management
│    │    ├── scaffold <template>      Create from template (basic, saas, ecommerce, blog)
│    │    ├── import <file>            Import DBML schema
│    │    ├── apply <file>             Full workflow: import → migrate → seed
│    │    └── diagram                  Export to DBML
│    ├── db seed                        Seed data management
│    │    ├── [default]                Run all seed files
│    │    ├── users                    Seed users only
│    │    └── create <name>            Create new seed file
│    ├── db mock                        Mock data generation
│    │    ├── [default]                Generate mock data
│    │    ├── auto                     Auto-generate from schema
│    │    └── --seed N                 Reproducible with seed
│    ├── db backup                      Database backups
│    │    ├── [default]                Create backup
│    │    └── list                     List available backups
│    ├── db restore <file>              Restore from backup
│    ├── db shell                       Interactive psql shell
│    │    └── --readonly               Read-only mode
│    ├── db query <sql>                 Execute SQL query
│    ├── db types                       Type generation
│    │    ├── [default]                TypeScript types
│    │    ├── go                       Go structs
│    │    └── python                   Python classes
│    ├── db inspect                     Database inspection
│    │    ├── [default]                Overview
│    │    ├── size                     Table sizes
│    │    └── slow                     Slow queries
│    └── db data                        Data operations
│         ├── export <table>           Export table to CSV/JSON
│         └── anonymize                Anonymize PII data
│
├─── MULTI-TENANT PLATFORM (32+ commands) ★ NEW v0.9.0 ★
│    ├── tenant init                    Initialize multi-tenancy system
│    ├── tenant create <name>           Create new tenant
│    │    ├── --slug <slug>            Custom URL slug
│    │    ├── --plan <plan>            Subscription plan (free, pro, enterprise)
│    │    └── --owner <user_id>        Owner user ID
│    ├── tenant list                    List all tenants
│    │    └── --json                   JSON output
│    ├── tenant show <id>               Show tenant details
│    ├── tenant suspend <id>            Suspend a tenant
│    ├── tenant activate <id>           Activate suspended tenant
│    ├── tenant delete <id>             Delete tenant (with confirmation)
│    ├── tenant stats                   Tenant statistics
│    │
│    ├── tenant member                  Member management
│    │    ├── add <tenant> <user> [role]      Add user to tenant
│    │    ├── remove <tenant> <user>          Remove user from tenant
│    │    └── list <tenant>                   List tenant members
│    │
│    ├── tenant setting                 Settings management
│    │    ├── set <tenant> <key> <val>        Set tenant setting
│    │    ├── get <tenant> <key>              Get tenant setting
│    │    └── list <tenant>                   List all settings
│    │
│    ├── tenant billing                 Billing & subscriptions
│    │    ├── usage                    Show usage statistics
│    │    ├── invoice                  Invoice management
│    │    │    ├── list                List all invoices
│    │    │    ├── show <id>           Show invoice details
│    │    │    ├── download <id>       Download invoice PDF
│    │    │    └── pay <id>            Pay outstanding invoice
│    │    ├── subscription             Subscription management
│    │    │    ├── show                Show current subscription
│    │    │    ├── upgrade <plan>      Upgrade to plan
│    │    │    ├── downgrade <plan>    Downgrade to plan
│    │    │    └── cancel              Cancel subscription
│    │    ├── payment                  Payment methods
│    │    │    ├── list                List payment methods
│    │    │    ├── add                 Add payment method
│    │    │    ├── remove <id>         Remove payment method
│    │    │    └── set-default <id>    Set default method
│    │    ├── quota                    Quota management
│    │    │    ├── check               Check current usage vs limits
│    │    │    └── increase <resource> Request quota increase
│    │    ├── plan                     Plan management
│    │    │    ├── list                List available plans
│    │    │    ├── show <plan>         Show plan details
│    │    │    ├── compare             Compare all plans
│    │    │    └── current             Show current plan
│    │    ├── export                   Export billing data (CSV/JSON)
│    │    └── customer                 Customer information
│    │         ├── show                Show customer details
│    │         ├── update              Update customer info
│    │         └── portal              Open customer portal
│    │
│    ├── tenant branding                Brand customization
│    │    ├── create <name>            Create new brand
│    │    ├── set-colors               Set brand colors
│    │    │    ├── --primary           Primary color
│    │    │    ├── --secondary         Secondary color
│    │    │    └── --accent            Accent color
│    │    ├── set-fonts                Set brand fonts
│    │    │    ├── --heading           Heading font
│    │    │    └── --body              Body font
│    │    ├── upload-logo <path>       Upload brand logo
│    │    ├── set-css <path>           Set custom CSS
│    │    └── preview                  Preview branding
│    │
│    ├── tenant domains                 Custom domains & SSL
│    │    ├── add <domain>             Add custom domain
│    │    ├── verify <domain>          Verify domain ownership
│    │    ├── ssl <domain>             Provision SSL certificate
│    │    │    ├── --auto              Auto-renew with Let's Encrypt
│    │    │    └── --upload            Upload custom certificate
│    │    ├── health <domain>          Check domain health
│    │    └── remove <domain>          Remove custom domain
│    │
│    ├── tenant email                   Email template customization
│    │    ├── list                     List all email templates
│    │    ├── edit <template>          Edit email template
│    │    │    Available templates:
│    │    │    - welcome               Welcome email
│    │    │    - password-reset        Password reset
│    │    │    - email-verification    Email verification
│    │    │    - invoice               Invoice email
│    │    │    - subscription-change   Subscription change
│    │    ├── preview <template>       Preview email template
│    │    ├── test <template> <email>  Send test email
│    │    └── set-language <code>      Set email language (en, es, fr, de, etc.)
│    │
│    └── tenant themes                  Theme management
│         ├── create <name>            Create new theme
│         ├── edit <name>              Edit theme variables
│         ├── activate <name>          Activate theme
│         ├── preview <name>           Preview theme
│         ├── export <name>            Export theme to JSON
│         └── import <path>            Import theme from JSON
│
├─── OAUTH MANAGEMENT (7 commands) ★ NEW v0.9.0 ★
│    ├── oauth install                  Install OAuth handlers service
│    ├── oauth enable                   Enable OAuth providers
│    │    └── --providers=<list>       Comma-separated: google,github,slack,microsoft
│    ├── oauth disable                  Disable OAuth providers
│    │    └── --providers=<list>       Providers to disable
│    ├── oauth config <provider>        Configure provider credentials
│    │    ├── --client-id=<id>         OAuth client ID
│    │    ├── --client-secret=<secret> OAuth client secret
│    │    ├── --tenant-id=<id>         Tenant ID (Microsoft only)
│    │    └── --callback-url=<url>     Custom callback URL
│    ├── oauth test <provider>          Test provider configuration
│    ├── oauth list                     List all OAuth providers
│    └── oauth status                   Show OAuth service status
│
├─── FILE STORAGE (8 commands) ★ NEW v0.9.0 ★
│    ├── storage init                   Initialize storage system
│    ├── storage upload <file>          Upload file to storage
│    │    ├── --dest <path>            Destination path in storage
│    │    ├── --thumbnails             Generate image thumbnails
│    │    ├── --virus-scan             Scan file for viruses
│    │    ├── --compression            Compress large files
│    │    └── --all-features           Enable all features
│    ├── storage list [prefix]          List uploaded files
│    ├── storage delete <path>          Delete uploaded file
│    ├── storage config                 Show storage configuration
│    ├── storage status                 Show pipeline status
│    ├── storage test                   Test upload functionality
│    └── storage graphql-setup          Generate GraphQL integration package
│
├─── DEPLOYMENT (12 commands)
│    ├── deploy <environment>           Deploy to environment
│    │    ├── staging                  Deploy to staging
│    │    └── production               Deploy to production
│    ├── deploy preview                 Preview environments
│    │    ├── [default]                Create preview environment
│    │    ├── list                     List preview environments
│    │    └── destroy <id>             Destroy preview environment
│    ├── deploy canary                  Canary deployment
│    │    ├── [default]                Start canary deployment
│    │    ├── promote                  Promote to 100%
│    │    ├── rollback                 Rollback canary
│    │    └── status                   Canary status
│    ├── deploy blue-green              Blue-green deployment
│    │    ├── [default]                Deploy to inactive
│    │    ├── switch                   Switch traffic
│    │    ├── rollback                 Rollback switch
│    │    └── status                   Show active environment
│    ├── deploy rollback                Rollback deployment
│    ├── deploy check                   Pre-deploy validation
│    │    └── --fix                    Auto-fix issues
│    └── deploy status                  Deployment status
│
├─── ENVIRONMENT (4 commands)
│    ├── env [default]                  List environments
│    ├── env create <name>              Create environment
│    │    └── <type>                   Type: local, staging, production
│    ├── env switch <name>              Switch active environment
│    └── env diff <env1> <env2>         Compare environments
│
├─── CLOUD INFRASTRUCTURE (13 commands)
│    ├── cloud provider                 Provider management
│    │    ├── list                     List 26 cloud providers
│    │    ├── init <provider>          Configure credentials
│    │    ├── validate                 Validate configuration
│    │    └── info <provider>          Provider details
│    ├── cloud server                   Server management
│    │    ├── create <provider>        Provision new server
│    │    ├── destroy <server>         Destroy server
│    │    ├── list                     List all servers
│    │    ├── status [server]          Server status
│    │    ├── ssh <server>             SSH to server
│    │    ├── add <ip>                 Add existing server
│    │    └── remove <server>          Remove from registry
│    ├── cloud cost                     Cost management
│    │    ├── estimate <provider>      Estimate costs
│    │    └── compare                  Compare all providers
│    └── cloud deploy                   Quick deployment
│         ├── quick                    Provision + deploy
│         └── full                     Full production setup
│
├─── SERVICE MANAGEMENT (15+ command groups)
│    ├── service list                   List optional services
│    ├── service enable <service>       Enable service
│    ├── service disable <service>      Disable service
│    ├── service status [service]       Service status
│    ├── service restart <service>      Restart service
│    ├── service logs <service>         Service logs
│    ├── service init                   Initialize service from template
│    ├── service scaffold               Scaffold new service
│    ├── service wizard                 Service creation wizard
│    ├── service search                 Search services
│    │
│    ├── service admin                  Admin UI management
│    │    ├── status                   Admin UI status
│    │    ├── open                     Open admin UI in browser
│    │    ├── users                    User management
│    │    ├── config                   Admin configuration
│    │    └── dev                      Development mode
│    │
│    ├── service email                  Email service
│    │    ├── test                     Send test email
│    │    ├── inbox                    Open MailPit inbox
│    │    └── config                   Email configuration
│    │
│    ├── service search                 Search service
│    │    ├── index                    Reindex data
│    │    ├── query <term>             Run search query
│    │    └── stats                    Index statistics
│    │
│    ├── service functions              Serverless functions
│    │    ├── deploy                   Deploy all functions
│    │    ├── invoke <fn>              Invoke function
│    │    ├── logs [fn]                View function logs
│    │    └── list                     List all functions
│    │
│    ├── service mlflow                 ML experiment tracking
│    │    ├── ui                       Open MLflow UI
│    │    ├── experiments              List experiments
│    │    ├── runs                     List runs
│    │    └── artifacts                Browse artifacts
│    │
│    ├── service storage                Object storage (MinIO)
│    │    ├── buckets                  List buckets
│    │    ├── upload                   Upload file
│    │    ├── download                 Download file
│    │    └── presign                  Generate presigned URL
│    │
│    └── service cache                  Redis cache
│         ├── stats                    Cache statistics
│         ├── flush                    Flush all cache
│         └── keys                     List cache keys
│
├─── KUBERNETES (14 commands)
│    ├── k8s init                       Initialize K8s config
│    ├── k8s convert                    Convert Docker Compose to K8s
│    │    ├── --output <dir>           Custom output directory
│    │    └── --namespace <ns>         Custom namespace
│    ├── k8s apply                      Apply manifests
│    │    └── --dry-run                Preview changes only
│    ├── k8s deploy                     Full deployment
│    │    └── --env <env>              With environment config
│    ├── k8s status                     Deployment status
│    ├── k8s logs <service>             Pod logs
│    │    └── -f                       Follow logs
│    ├── k8s scale <svc> <n>            Scale deployment
│    ├── k8s rollback <service>         Rollback deployment
│    ├── k8s delete                     Delete deployment
│    ├── k8s cluster                    Cluster management
│    │    ├── list                     List clusters
│    │    ├── connect <name>           Connect to cluster
│    │    └── info                     Cluster info
│    └── k8s namespace                  Namespace management
│         ├── list                     List namespaces
│         ├── create <name>            Create namespace
│         ├── delete <name>            Delete namespace
│         └── switch <name>            Switch namespace
│
├─── HELM CHARTS (13 commands)
│    ├── helm init                      Initialize Helm chart
│    │    └── --from-compose           From docker-compose.yml
│    ├── helm generate                  Generate/update chart
│    ├── helm install                   Install to cluster
│    │    └── --env <env>              With environment values
│    ├── helm upgrade                   Upgrade release
│    ├── helm rollback                  Rollback release
│    ├── helm uninstall                 Remove release
│    ├── helm list                      List releases
│    ├── helm status                    Release status
│    ├── helm values                    Show/edit values
│    ├── helm template                  Render locally
│    ├── helm package                   Package chart
│    └── helm repo                      Repository management
│         ├── add <name> <url>         Add repository
│         ├── remove <name>            Remove repository
│         ├── update                   Update repos
│         └── list                     List repos
│
├─── SYNC (8 commands)
│    ├── sync db <env>                  Sync database
│    ├── sync files <env>               Sync files
│    ├── sync config <env>              Sync configuration
│    ├── sync full <env>                Full sync
│    ├── sync auto                      Auto-sync service
│    │    ├── --setup                  Configure auto-sync
│    │    └── --stop                   Stop auto-sync
│    ├── sync watch                     Watch mode
│    │    ├── --path <dir>             Watch specific path
│    │    └── --interval <s>           Polling interval
│    ├── sync status                    Sync status
│    └── sync history                   Sync history
│
├─── PERFORMANCE (13 commands)
│    ├── perf                           Performance profiling
│    │    ├── profile [service]        System/service profile
│    │    ├── analyze                  Analyze performance
│    │    ├── slow-queries             Slow query analysis
│    │    ├── report                   Generate report
│    │    ├── dashboard                Real-time dashboard
│    │    └── suggest                  Optimization tips
│    ├── bench                          Benchmarking
│    │    ├── run [target]             Run benchmark
│    │    ├── baseline                 Establish baseline
│    │    ├── compare [file]           Compare to baseline
│    │    ├── stress [target]          Stress test
│    │    └── report                   Benchmark report
│    ├── scale                          Service scaling
│    │    ├── <service>                Scale service
│    │    ├── status                   Scale status
│    │    └── --auto                   Enable autoscaling
│    └── migrate                        Cross-env migration
│         ├── <src> <target>           Migrate environments
│         ├── diff <s> <t>             Show differences
│         ├── sync <s> <t>             Continuous sync
│         └── rollback                 Undo migration
│
├─── OPERATIONS (20 commands)
│    ├── frontend                       Frontend management
│    │    ├── status                   Frontend status
│    │    ├── list                     List frontends
│    │    ├── add <name>               Add frontend
│    │    ├── remove <name>            Remove frontend
│    │    ├── deploy <name>            Deploy frontend
│    │    ├── logs <name>              Deploy logs
│    │    └── env <name>               Environment vars
│    ├── history                        Audit trail
│    │    ├── show                     Recent history
│    │    ├── deployments              Deploy history
│    │    ├── migrations               Migration history
│    │    ├── rollbacks                Rollback history
│    │    ├── commands                 Command history
│    │    ├── search <query>           Search history
│    │    ├── export                   Export history
│    │    └── clear                    Clear history
│    └── config                         Configuration
│         ├── show                     Show config
│         ├── get <key>                Get value
│         ├── set <key> <val>          Set value
│         ├── list                     List keys
│         ├── edit                     Open in editor
│         ├── validate                 Validate config
│         ├── diff <e1> <e2>           Compare envs
│         ├── export                   Export config
│         ├── import <file>            Import config
│         └── reset                    Reset to defaults
│
├─── PLUGINS (10+ commands)
│    ├── plugin list                    List available plugins
│    │    ├── --installed              Show installed only
│    │    └── --category               Filter by category
│    ├── plugin install <name>          Install plugin
│    ├── plugin remove <name>           Remove plugin
│    │    └── --keep-data              Keep database tables
│    ├── plugin update [name]           Update plugin(s)
│    │    └── --all                    Update all plugins
│    ├── plugin updates                 Check for updates
│    ├── plugin refresh                 Refresh registry cache
│    ├── plugin status [name]           Plugin status
│    └── plugin <name> <action>         Run plugin action
│         ├── stripe                   Payment processing
│         │    ├── sync                Sync data from Stripe
│         │    ├── customers           Customer management
│         │    ├── subscriptions       Subscription management
│         │    ├── invoices            Invoice management
│         │    └── webhook             Webhook events
│         ├── github                   DevOps integration
│         │    ├── sync                Sync repositories
│         │    ├── repos               Repository list
│         │    ├── issues              Issue tracking
│         │    ├── prs                 Pull requests
│         │    ├── workflows           GitHub Actions
│         │    └── webhook             Webhook events
│         └── shopify                  E-commerce
│              ├── sync                Sync store data
│              ├── products            Product catalog
│              ├── orders              Order management
│              ├── customers           Customer data
│              └── webhook             Webhook events
│
└─── UTILITY (10 commands)
     ├── ssl                            SSL certificate management
     ├── trust                          Trust local certificates
     ├── admin                          Admin UI launcher
     ├── ci                             CI/CD generation
     │    ├── init <platform>          Generate workflow
     │    ├── validate                 Validate config
     │    └── status                   CI status
     ├── completion                     Shell completions
     │    ├── bash                     Bash completions
     │    ├── zsh                      Zsh completions
     │    ├── fish                     Fish completions
     │    └── install <shell>          Auto-install
     ├── update                         Update nself
     │    └── --check                  Check only
     ├── version                        Version info
     │    └── --json                   JSON output
     └── help [command]                 Show help
```

---

## Command Count Summary

| Category | Commands | New in v0.9.0 |
|----------|----------|---------------|
| Core Lifecycle | 7 | - |
| Status & Monitoring | 6 | - |
| Database | 10 groups | - |
| **Multi-Tenant** | **32+** | **✓** |
| **OAuth** | **7** | **✓** |
| **Storage** | **8** | **✓** |
| Deployment | 12 | - |
| Environment | 4 | - |
| Cloud Infrastructure | 13 | - |
| Service Management | 15+ groups | Enhanced |
| Kubernetes | 14 | - |
| Helm Charts | 13 | - |
| Sync | 8 | - |
| Performance | 13 | - |
| Operations | 20 | - |
| Plugins | 10+ | - |
| Utility | 10 | - |
| **TOTAL** | **150+** | **47 new** |

---

## Major v0.9.0 Additions

### 1. Multi-Tenant Platform (32+ commands)
Complete platform for building SaaS applications with:
- Tenant lifecycle management (create, suspend, delete)
- Billing integration (usage tracking, invoicing, subscriptions)
- White-labeling (custom domains, branding, themes)
- Member management (role-based access control)

### 2. OAuth Management (7 commands)
Comprehensive OAuth provider integration:
- Support for Google, GitHub, Microsoft, Slack
- Easy configuration and testing
- Automatic OAuth handlers service

### 3. File Storage (8 commands)
Advanced upload pipeline with:
- Thumbnail generation
- Virus scanning
- File compression
- GraphQL integration generation

---

## Backward Compatibility

Legacy commands still work and redirect to new structure:

| Legacy Command | New Command |
|----------------|-------------|
| `nself billing` | `nself tenant billing` |
| `nself whitelabel` | `nself tenant branding/domains/email/themes` |
| `nself providers` | `nself provider` |
| `nself provision` | `nself provider server create` |
| `nself cloud` | `nself provider` (legacy alias) |
| `nself staging` | `nself deploy staging` |
| `nself prod` | `nself deploy production` |

---

**Last Updated:** January 30, 2026
**Version:** 0.9.0
**Total Commands:** 150+
