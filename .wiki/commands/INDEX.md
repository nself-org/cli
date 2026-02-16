# nself Commands - Complete Reference

**v1.0 Command Structure** | 31 top-level commands | 285+ subcommands

This is the single entry point for all nself command documentation. The v1.0 structure consolidates the original 79 commands into 31 logical top-level commands. Legacy commands still work with deprecation warnings until v2.0.

---

## Choose Your View

Different views for different needs:

| View | Best For | Link |
|------|----------|------|
| **Command Tree** ⭐ | Complete hierarchy, syntax, and descriptions - **Authoritative Reference** | [COMMAND-TREE-V1.md](COMMAND-TREE-V1.md) |
| **Command by Use Case** | Finding the right command for what you want to do | [COMMAND-USE-CASES.md](COMMAND-USE-CASES.md) |
| **SPORT Command Matrix** | Verifying runtime coverage (no gaps) | [SPORT-COMMAND-MATRIX.md](SPORT-COMMAND-MATRIX.md) |
| **Quick Reference** | Printable cheat sheet for daily use | [../reference/COMMAND-REFERENCE.md](../reference/COMMAND-REFERENCE.md) |

---

## Core Commands (5)

The essential lifecycle commands you use every day.

| Command | Description | Details |
|---------|-------------|---------|
| [init](INIT.md) | Initialize a new nself project (interactive wizard, demo mode, or simple setup) | `nself init [--demo\|--simple\|--full]` |
| [build](BUILD.md) | Generate Docker Compose, Nginx configs, SSL certs, and service files | `nself build [--clean\|--no-cache]` |
| [start](START.md) | Start all services with smart defaults, health checks, and progress display | `nself start [--fresh\|--verbose]` |
| [stop](STOP.md) | Stop all services or a specific service | `nself stop [service...]` |
| [restart](RESTART.md) | Restart all services or a specific service | `nself restart [service...]` |

**Typical first-run workflow:**
```bash
nself init --demo && nself build && nself start
nself urls          # See all service URLs
nself status        # Check service health
```

---

## Database - `db` (11 subcommands)

Full database lifecycle management under a single namespace.

| Subcommand | Description | Details |
|------------|-------------|---------|
| [db](DB.md) migrate | Run, rollback, or create migrations | `nself db migrate <up\|down\|create\|status>` |
| [db](DB.md) schema | Scaffold from templates, import DBML, export diagrams | `nself db schema <scaffold\|import\|apply\|diagram>` |
| [db](DB.md) seed | Seed the database with sample data | `nself db seed [dataset]` |
| [db](DB.md) mock | Generate realistic mock data from schema | `nself db mock [auto] [--seed N]` |
| [db](DB.md) backup | Create named or compressed backups | `nself db backup [--name NAME]` |
| [db](DB.md) restore | Restore from a backup file | `nself db restore <file\|latest>` |
| [db](DB.md) shell | Open an interactive psql session | `nself db shell [--readonly]` |
| [db](DB.md) query | Execute SQL directly or from a file | `nself db query "<sql>" [-f file.sql]` |
| [db](DB.md) types | Generate TypeScript, Go, or Python types from schema | `nself db types [typescript\|go\|python]` |
| [db](DB.md) inspect | Analyze table sizes, slow queries, and schema overview | `nself db inspect [size\|slow]` |
| [db](DB.md) data | Import, export, or anonymize table data | `nself db data <export\|anonymize>` |

---

## Multi-Tenant - `tenant` (50+ subcommands)

Complete multi-tenancy system with billing, branding, organizations, domains, email templates, and themes.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [tenant](TENANT.md) core | Create, list, show, suspend, activate, delete tenants | `nself tenant create <name> [--plan PLAN]` |
| [tenant](TENANT.md) member | Add, remove, invite, and manage tenant members | `nself tenant member <add\|remove\|list\|role>` |
| [tenant](TENANT.md) setting | Get, set, list, and reset tenant settings | `nself tenant setting <get\|set\|list>` |
| [tenant](TENANT.md) billing | Plans, subscriptions, invoices, usage, payments | `nself tenant billing <plans\|usage\|invoice>` |
| [tenant](TENANT.md) org | Create and manage organizations | `nself tenant org <create\|list\|show\|members>` |
| [tenant](TENANT.md) branding | Upload logos, set colors, preview branding | `nself tenant branding <logo\|colors\|preview>` |
| [tenant](TENANT.md) domains | Add custom domains, verify DNS, provision SSL | `nself tenant domains <add\|verify\|ssl>` |
| [tenant](TENANT.md) email | List, edit, preview, and test email templates | `nself tenant email <list\|edit\|preview>` |
| [tenant](TENANT.md) themes | Apply, customize, preview, and export themes | `nself tenant themes <apply\|customize\|preview>` |

---

## Deployment - `deploy` (23 subcommands)

Deployment strategies, remote server management, and environment synchronization.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [deploy](DEPLOY.md) staging | Deploy to staging environment | `nself deploy staging [--auto-migrate]` |
| [deploy](DEPLOY.md) production | Deploy to production environment | `nself deploy production [--auto-migrate]` |
| [deploy](DEPLOY.md) preview | Create ephemeral preview environments per branch | `nself deploy preview <branch>` |
| [deploy](DEPLOY.md) canary | Gradual traffic shifting to new version | `nself deploy canary [--percentage N]` |
| [deploy](DEPLOY.md) blue-green | Zero-downtime deployment with instant switch | `nself deploy blue-green` |
| [deploy](DEPLOY.md) rollback | Roll back the last deployment | `nself deploy rollback [--version N]` |
| [deploy](DEPLOY.md) server | Initialize, check, diagnose, and manage remote VPS | `nself deploy server <init\|check\|list\|ssh>` |
| [deploy](DEPLOY.md) sync | Push/pull configuration and environment files | `nself deploy sync <push\|pull\|status\|full>` |

---

## Infrastructure - `infra` (38 subcommands)

Cloud providers, Kubernetes, and Helm chart management.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [infra](INFRA.md) provider | List, configure, and validate 26+ cloud providers | `nself infra provider <list\|init\|validate>` |
| [infra](INFRA.md) provider server | Provision, destroy, SSH to cloud servers | `nself infra provider server <create\|list\|ssh>` |
| [infra](INFRA.md) provider cost | Estimate and compare provider costs | `nself infra provider cost <estimate\|compare>` |
| [infra](INFRA.md) provider k8s-* | Create managed K8s clusters across 8 providers | `nself infra provider k8s-create <provider>` |
| [infra](INFRA.md) k8s | Convert Compose to K8s, deploy, scale, manage clusters | `nself infra k8s <init\|convert\|apply\|deploy>` |
| [infra](INFRA.md) helm | Generate, install, upgrade, and manage Helm charts | `nself infra helm <init\|install\|upgrade\|list>` |

---

## Services - `service` (43 subcommands)

Manage optional services, custom services, and service-specific operations.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [service](SERVICE.md) core | List, enable, disable, status, restart, scaffold | `nself service <list\|enable\|disable\|wizard>` |
| [service](SERVICE.md) storage | S3-compatible file storage (MinIO) with upload pipeline | `nself service storage <upload\|list\|config>` |
| [service](SERVICE.md) email | Email service with MailPit for dev, SMTP for prod | `nself service email <send\|test\|config>` |
| [service](SERVICE.md) search | MeiliSearch, Typesense, or other search providers | `nself service search <init\|index\|query>` |
| [service](SERVICE.md) redis | Redis cache management with CLI access | `nself service redis <init\|flush\|cli\|stats>` |
| [service](SERVICE.md) functions | Serverless functions runtime | `nself service functions <deploy\|list\|invoke>` |
| [service](SERVICE.md) mlflow | ML experiment tracking and model registry | `nself service mlflow <ui\|experiments\|models>` |
| [service](SERVICE.md) realtime | Real-time event subscriptions and WebSockets | `nself service realtime <init\|events\|test>` |

---

## Auth & Security - `auth` (38 subcommands)

Authentication, authorization, SSL, rate limiting, and security scanning.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [auth](AUTH.md) core | Login, logout, and auth status | `nself auth <login\|logout\|status>` |
| [auth](AUTH.md) mfa | Enable, disable, and verify multi-factor authentication | `nself auth mfa <enable\|disable\|verify>` |
| [auth](AUTH.md) roles | Create, assign, and manage user roles | `nself auth roles <list\|create\|assign>` |
| [auth](AUTH.md) devices | Register, revoke, and trust devices | `nself auth devices <list\|register\|revoke>` |
| [auth](AUTH.md) oauth | Install, enable, and configure OAuth providers | `nself auth oauth <enable\|config\|test>` |
| [auth](AUTH.md) security | Run security scans, audits, and generate reports | `nself auth security <scan\|audit\|report>` |
| [auth](AUTH.md) ssl | Generate, install, renew, and trust SSL certificates | `nself auth ssl <generate\|renew\|trust>` |
| [auth](AUTH.md) rate-limit | Configure and monitor API rate limiting | `nself auth rate-limit <config\|status\|reset>` |
| [auth](AUTH.md) webhooks | Create, test, and manage webhook endpoints | `nself auth webhooks <create\|list\|test>` |

---

## Configuration - `config` (20 subcommands)

Environment management, secrets, and vault integration.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [config](CONFIG.md) core | Show, edit, validate, export, and import configuration | `nself config <show\|edit\|validate>` |
| [config](CONFIG.md) env | List, switch, create, and sync environments | `nself config env <list\|switch\|create>` |
| [config](CONFIG.md) secrets | List, get, set, delete, and rotate secrets | `nself config secrets <list\|get\|set\|rotate>` |
| [config](CONFIG.md) vault | Initialize and manage HashiCorp Vault integration | `nself config vault <init\|config\|status>` |

---

## Performance - `perf` (5 subcommands)

Profiling, benchmarking, scaling, and optimization.

| Subcommand | Description | Details |
|------------|-------------|---------|
| [perf](PERF.md) profile | Profile service performance over time | `nself perf profile [service] [--duration N]` |
| [perf](PERF.md) bench | Run load tests and benchmarks | `nself perf bench [service] [--duration N]` |
| [perf](PERF.md) scale | Scale services horizontally or vertically | `nself perf scale <service> <replicas>` |
| [perf](PERF.md) migrate | Cross-environment migration tooling | `nself perf migrate [options]` |
| [perf](PERF.md) optimize | Get optimization suggestions with optional auto-fix | `nself perf optimize [--auto-fix]` |

---

## Backup & Recovery - `backup` (6 subcommands)

Backup creation, restoration, rollback, and cleanup.

| Subcommand | Description | Details |
|------------|-------------|---------|
| [backup](BACKUP.md) create | Create full or incremental backups | `nself backup create [--full\|--incremental]` |
| [backup](BACKUP.md) restore | Restore from a specific backup | `nself backup restore <backup-id>` |
| [backup](BACKUP.md) list | List available backups with optional date filter | `nself backup list [--filter DATE]` |
| [backup](BACKUP.md) rollback | Roll back to a previous version | `nself backup rollback [--version N]` |
| [backup](BACKUP.md) reset | Reset to a clean state (destructive) | `nself backup reset [--confirm]` |
| [backup](BACKUP.md) clean | Remove old backups and resources | `nself backup clean [--age DAYS]` |

---

## Developer Tools - `dev` (16 subcommands)

Frontend management, CI/CD generation, documentation, and white-label tooling.

| Subcommand Group | Description | Details |
|------------------|-------------|---------|
| [dev](DEV.md) mode | Toggle developer mode on or off | `nself dev mode [on\|off]` |
| [dev](DEV.md) frontend | Add, remove, list, and configure frontend apps | `nself dev frontend <add\|remove\|list\|config>` |
| [dev](DEV.md) ci | Generate CI/CD configs for GitHub Actions, GitLab, etc. | `nself dev ci <generate\|update\|templates>` |
| [dev](DEV.md) docs | Generate, serve, and build project documentation | `nself dev docs <generate\|serve\|build>` |
| [dev](DEV.md) whitelabel | Configure, preview, and deploy white-label branding | `nself dev whitelabel <config\|preview\|deploy>` |

---

## Plugins - `plugin` (8+ subcommands)

Third-party integrations via the plugin system.

| Subcommand | Description | Details |
|------------|-------------|---------|
| [plugin](PLUGIN.md) list | List available and installed plugins | `nself plugin list [--installed]` |
| [plugin](PLUGIN.md) install | Install a plugin from the registry | `nself plugin install <plugin>` |
| [plugin](PLUGIN.md) remove | Remove an installed plugin | `nself plugin remove <plugin>` |
| [plugin](PLUGIN.md) update | Update one or all plugins | `nself plugin update [plugin\|--all]` |
| [plugin](PLUGIN.md) updates | Check for available plugin updates | `nself plugin updates` |
| [plugin](PLUGIN.md) refresh | Refresh the plugin registry cache | `nself plugin refresh` |
| [plugin](PLUGIN.md) status | Show plugin health and configuration | `nself plugin status [plugin]` |
| [plugin](PLUGIN.md) create | Scaffold a new plugin for development | `nself plugin create <name>` |

**Available plugins:** [Stripe](../plugins/stripe.md), [GitHub](../plugins/github.md), [Shopify](../plugins/shopify.md)

---

## Utility Commands (15)

Standalone commands for monitoring, diagnostics, and system management.

| Command | Description | Details |
|---------|-------------|---------|
| [status](STATUS.md) | Show health status of all services (supports `--watch`, `--json`) | `nself status [service...]` |
| [logs](LOGS.md) | View and follow service logs | `nself logs <service> [-f] [--tail N]` |
| [urls](URLS.md) | Display all service URLs and routes | `nself urls [--json\|--env ENV]` |
| [doctor](DOCTOR.md) | Run diagnostics and auto-fix common issues | `nself doctor [--fix] [--verbose]` |
| [monitor](MONITOR.md) | Open monitoring dashboards (Grafana, Prometheus) | `nself monitor [dashboard]` |
| [health](HEALTH.md) | Run deep health checks on services and endpoints | `nself health [service...] [--deep]` |
| [help](HELP.md) | Show help for any command | `nself help [command]` |
| [version](VERSION.md) | Show version info and check for updates | `nself version [--check]` |
| [update](UPDATE.md) | Update nself to the latest version | `nself update [--preview\|--force]` |
| [completion](COMPLETION.md) | Generate shell completions for bash, zsh, or fish | `nself completion <shell>` |
| [exec](EXEC.md) | Execute a command inside a service container | `nself exec <service> <command>` |
| [admin](ADMIN.md) | Open the nself Admin UI | `nself admin [--dev]` |
| [metrics](METRICS.md) | View metrics, set monitoring profiles | `nself metrics [service] [--profile]` |
| [history](HISTORY.md) | View command and deployment audit trail | `nself history [--limit N]` |
| [audit](AUDIT.md) | View audit logs and security events | `nself audit [--export] [--format json\|csv]` |

---

## Infrastructure Safety

| Command | Description | Details |
|---------|-------------|---------|
| [destroy](DESTROY.md) | Safely tear down project infrastructure with selective targeting | `nself destroy [--dry-run\|--keep-volumes]` |

---

## Deprecated Commands

These legacy commands still work but print deprecation warnings. They will be removed in v2.0.

| Legacy Command | Use Instead | Migration |
|----------------|-------------|-----------|
| `nself up` | `nself start` | [UP.md](UP.md) |
| `nself down` | `nself stop` | [DOWN.md](DOWN.md) |
| `nself billing` | `nself tenant billing` | [BILLING.md](BILLING.md) |
| `nself org` | `nself tenant org` | [org.md](org.md) |
| `nself staging` | `nself deploy staging` | [STAGING.md](STAGING.md) |
| `nself prod` | `nself deploy production` | [PROD.md](PROD.md) |
| `nself provider` | `nself infra provider` | [PROVIDER.md](PROVIDER.md) |
| `nself k8s` | `nself infra k8s` | [K8S.md](K8S.md) |
| `nself helm` | `nself infra helm` | [HELM.md](HELM.md) |
| `nself storage` | `nself service storage` | [storage.md](storage.md) |
| `nself mfa` | `nself auth mfa` | [MFA.md](MFA.md) |
| `nself oauth` | `nself auth oauth` | [OAUTH.md](OAUTH.md) |
| `nself ssl` | `nself auth ssl` | [SSL.md](SSL.md) |
| `nself secrets` | `nself config secrets` | [secrets.md](secrets.md) |
| `nself env` | `nself config env` | [ENV.md](ENV.md) |
| `nself bench` | `nself perf bench` | [BENCH.md](BENCH.md) |
| `nself rollback` | `nself backup rollback` | [ROLLBACK.md](ROLLBACK.md) |
| `nself frontend` | `nself dev frontend` | [FRONTEND.md](FRONTEND.md) |
| `nself ci` | `nself dev ci` | [CI.md](CI.md) |
| `nself whitelabel` | `nself dev whitelabel` | [WHITELABEL.md](WHITELABEL.md) |

For the full consolidation map, see [COMMAND-TREE-V1.md](COMMAND-TREE-V1.md#command-consolidation-map).

---

## Getting Help

```bash
nself help                  # General help
nself help <command>        # Command-specific help
nself <command> --help      # Alternative syntax
nself doctor                # System diagnostics
nself doctor --fix          # Auto-fix common issues
```

---

**[Back to Documentation Home](../README.md)** | **[Getting Started](../getting-started/Quick-Start.md)** | **[Architecture](../architecture/ARCHITECTURE.md)**
