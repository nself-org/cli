# nself Command Reorganization - Visual Guide

**Quick Reference** | See [COMMAND-REORGANIZATION-PROPOSAL.md](./COMMAND-REORGANIZATION-PROPOSAL.md) for full details

---

## The Problem (Current State)

```
77 top-level commands creating confusion and cognitive overload

nself [one of 77 options] → Too many choices!
```

### Current Command Chaos

```
Authentication scattered:     auth, oauth, mfa, devices, security, roles, webhooks
Database scattered:           db, migrate, backup, restore
Deployment scattered:         deploy, staging, prod, rollback, env, sync, validate
Cloud scattered:              cloud, providers, provision, server, servers, k8s, helm
Observability scattered:      logs, metrics, monitor, health, status, doctor, history, audit, urls, exec
Services scattered:           service, admin, email, search, functions, mlflow, storage, redis, realtime
Security scattered:           security, secrets, vault, ssl, trust, mfa
Developer tools scattered:    dev, perf, bench, scale, frontend, ci, completion
```

**Result:** Users struggle to find commands, help text is overwhelming

---

## The Solution (Proposed State)

```
13 logical categories with clear boundaries

nself <category> <action> → Clear mental model!
```

### New Command Structure

```
Core (6 commands)
├── init                    Initialize project
├── build                   Build configs
├── start                   Start services
├── stop                    Stop services
├── restart                 Restart services
└── status                  Service status

Data & Business Logic (4 categories)
├── db                      Database operations (10+ subcommands)
├── auth                    Authentication & authorization (20+ subcommands)
├── tenant                  Multi-tenancy (32+ subcommands)
└── service                 Service management (25+ subcommands)

Infrastructure (3 categories)
├── deploy                  Deployment & environments (15+ subcommands)
├── cloud                   Cloud infrastructure (30+ subcommands)
└── observe                 Monitoring & observability (25+ subcommands)

Security & Tools (3 categories)
├── secure                  Security & compliance (20+ subcommands)
├── plugin                  Plugin management (10+ subcommands)
└── dev                     Developer tools (20+ subcommands)

Configuration (2 categories)
├── config                  Configuration management (10+ subcommands)
└── help                    Help & utilities (4+ subcommands)
```

**Result:** 71% reduction in top-level commands, logical grouping, easy discovery

---

## Before & After Comparison

### Authentication Commands

**BEFORE (8 top-level commands):**
```bash
nself auth login
nself oauth enable
nself mfa enable
nself devices list
nself roles list
nself webhooks list
nself security scan
# Users must remember 8 separate top-level commands!
```

**AFTER (1 top-level category):**
```bash
nself auth login
nself auth oauth enable
nself auth mfa enable
nself auth devices list
nself auth roles list
nself auth webhooks list
# Everything auth-related is under 'nself auth'
```

### Service Management

**BEFORE (11 top-level commands):**
```bash
nself service list
nself admin open
nself email test
nself search index
nself functions deploy
nself mlflow ui
nself storage upload
nself redis flush
nself realtime status
nself rate-limit config
# Mixed pattern - some under 'service', others top-level
```

**AFTER (1 unified category):**
```bash
nself service list
nself service admin open
nself service email test
nself service search index
nself service functions deploy
nself service mlflow ui
nself service storage upload
nself service cache flush
nself service realtime status
nself service rate-limit config
# ALL optional services under 'nself service'
```

### Observability

**BEFORE (9 top-level commands):**
```bash
nself logs postgres
nself metrics
nself monitor
nself health check
nself doctor
nself history
nself audit
nself urls
nself exec postgres sh
# Scattered across multiple top-level commands
```

**AFTER (1 unified category):**
```bash
nself observe logs postgres
nself observe metrics
nself observe monitor
nself observe health check
nself observe doctor
nself observe history
nself observe audit
nself observe urls
nself observe exec postgres sh
# ALL observability under 'nself observe'
```

---

## Command Migration Map

### Quick Reference

| Old Command | New Command | Category |
|-------------|-------------|----------|
| `nself oauth` | `nself auth oauth` | Auth |
| `nself mfa` | `nself auth mfa` | Auth |
| `nself devices` | `nself auth devices` | Auth |
| `nself roles` | `nself auth roles` | Auth |
| `nself webhooks` | `nself auth webhooks` | Auth |
| | | |
| `nself admin` | `nself service admin` | Service |
| `nself email` | `nself service email` | Service |
| `nself search` | `nself service search` | Service |
| `nself functions` | `nself service functions` | Service |
| `nself mlflow` | `nself service mlflow` | Service |
| `nself storage` | `nself service storage` | Service |
| `nself redis` | `nself service cache` | Service |
| `nself realtime` | `nself service realtime` | Service |
| | | |
| `nself staging` | `nself deploy staging` | Deploy |
| `nself prod` | `nself deploy production` | Deploy |
| `nself env` | `nself deploy env` | Deploy |
| `nself sync` | `nself deploy sync` | Deploy |
| `nself rollback` | `nself deploy rollback` | Deploy |
| | | |
| `nself providers` | `nself cloud provider` | Cloud |
| `nself provision` | `nself cloud server create` | Cloud |
| `nself server` | `nself cloud server` | Cloud |
| `nself servers` | `nself cloud server list` | Cloud |
| `nself k8s` | `nself cloud k8s` | Cloud |
| `nself helm` | `nself cloud helm` | Cloud |
| | | |
| `nself logs` | `nself observe logs` | Observe |
| `nself metrics` | `nself observe metrics` | Observe |
| `nself monitor` | `nself observe monitor` | Observe |
| `nself health` | `nself observe health` | Observe |
| `nself doctor` | `nself observe doctor` | Observe |
| `nself history` | `nself observe history` | Observe |
| `nself audit` | `nself observe audit` | Observe |
| `nself urls` | `nself observe urls` | Observe |
| `nself exec` | `nself observe exec` | Observe |
| | | |
| `nself security` | `nself secure` | Secure |
| `nself secrets` | `nself secure secrets` | Secure |
| `nself vault` | `nself secure vault` | Secure |
| `nself ssl` | `nself secure ssl` | Secure |
| `nself trust` | `nself secure ssl trust` | Secure |
| | | |
| `nself perf` | `nself dev perf` | Dev |
| `nself bench` | `nself dev bench` | Dev |
| `nself scale` | `nself dev scale` | Dev |
| `nself frontend` | `nself dev frontend` | Dev |
| `nself ci` | `nself dev ci` | Dev |
| `nself completion` | `nself dev completion` | Dev |
| | | |
| `nself clean` | `nself config clean` | Config |
| `nself reset` | `nself config reset` | Config |
| | | |
| `nself billing` | `nself tenant billing` | Tenant |
| `nself whitelabel` | `nself tenant branding` | Tenant |
| `nself org` | `nself tenant` | Tenant |

---

## The 13 Categories Explained

### 1. Core Lifecycle (6 commands)

**Daily essentials** - Always top-level, no changes
```
init, build, start, stop, restart, status
```

### 2. Database (`db`)

**ALL database operations** - Already well-organized
```
nself db migrate       nself db backup       nself db types
nself db schema        nself db restore      nself db inspect
nself db seed          nself db shell        nself db data
nself db mock          nself db query
```

### 3. Authentication (`auth`)

**ALL auth & authorization** - Consolidates 8 top-level commands
```
nself auth login              nself auth oauth enable
nself auth signup             nself auth mfa enable
nself auth logout             nself auth devices list
nself auth status             nself auth roles create
```

### 4. Multi-Tenant (`tenant`)

**ALL tenant operations** - Already well-organized
```
nself tenant create           nself tenant billing usage
nself tenant list             nself tenant branding
nself tenant member add       nself tenant domains add
nself tenant setting set      nself tenant email edit
```

### 5. Service Management (`service`)

**ALL optional services** - Consolidates 11 top-level commands
```
nself service enable          nself service email test
nself service disable         nself service search index
nself service admin open      nself service functions deploy
nself service storage upload  nself service cache flush
```

### 6. Deployment (`deploy`)

**ALL deployment operations** - Consolidates 8 top-level commands
```
nself deploy staging          nself deploy env switch
nself deploy production       nself deploy sync db
nself deploy canary           nself deploy rollback
nself deploy blue-green       nself deploy validate
```

### 7. Cloud Infrastructure (`cloud`)

**ALL cloud & infrastructure** - Consolidates 8 top-level commands
```
nself cloud provider list     nself cloud k8s deploy
nself cloud server create     nself cloud helm install
nself cloud server ssh        nself cloud cost compare
nself cloud server list       nself cloud deploy full
```

### 8. Observability (`observe`) - NEW

**ALL monitoring & diagnostics** - Consolidates 9 top-level commands
```
nself observe logs            nself observe health check
nself observe metrics         nself observe doctor
nself observe monitor         nself observe history
nself observe urls            nself observe audit
nself observe exec
```

### 9. Security (`secure`) - NEW

**ALL security operations** - Consolidates 6 top-level commands
```
nself secure scan             nself secure vault init
nself secure incidents        nself secure ssl generate
nself secure secrets set      nself secure headers config
nself secure webauthn add     nself secure ssl trust
```

### 10. Plugins (`plugin`)

**Plugin management** - No changes
```
nself plugin list             nself plugin status
nself plugin install          nself plugin stripe sync
nself plugin remove           nself plugin github repos
nself plugin update           nself plugin shopify orders
```

### 11. Developer Tools (`dev`)

**ALL dev utilities** - Consolidates 7 top-level commands
```
nself dev perf profile        nself dev frontend add
nself dev bench run           nself dev ci init
nself dev scale postgres      nself dev completion bash
nself dev watch
```

### 12. Configuration (`config`)

**ALL config operations** - Consolidates 3 top-level commands
```
nself config show             nself config validate
nself config set              nself config clean
nself config edit             nself config reset
nself config diff
```

### 13. Help & Utilities (`help`)

**Core utilities** - Minimal changes
```
nself help                    nself version
nself update                  nself upgrade
```

---

## Visual Command Tree

```
nself
├── Core (Always available)
│   ├── init
│   ├── build
│   ├── start
│   ├── stop
│   ├── restart
│   └── status
│
├── Data Layer
│   ├── db
│   │   ├── migrate (up, down, create, status)
│   │   ├── schema (scaffold, import, apply, diagram)
│   │   ├── seed (users, create)
│   │   ├── mock (auto)
│   │   ├── backup / restore
│   │   ├── shell / query
│   │   ├── types (typescript, go, python)
│   │   ├── inspect (size, slow)
│   │   └── data (export, anonymize)
│   │
│   ├── auth
│   │   ├── login / signup / logout / status
│   │   ├── oauth (install, enable, config, test, list)
│   │   ├── mfa (enable, disable, verify, backup-codes, qr)
│   │   ├── devices (list, revoke, trust)
│   │   ├── roles (list, create, assign, permissions)
│   │   └── webhooks (list, create, test)
│   │
│   └── tenant
│       ├── init / create / list / show / suspend / activate / delete / stats
│       ├── member (add, remove, list)
│       ├── billing (usage, invoice, subscription, payment, quota, plan, export, customer)
│       ├── branding / domains / email / themes
│       └── setting (set, get, list)
│
├── Infrastructure Layer
│   ├── service
│   │   ├── list / enable / disable / status / restart / logs
│   │   ├── admin (status, open, users, config, dev)
│   │   ├── email (test, inbox, config)
│   │   ├── search (index, query, stats)
│   │   ├── functions (deploy, invoke, logs, list)
│   │   ├── mlflow (ui, experiments, runs, artifacts)
│   │   ├── storage (init, upload, list, delete, config, status, test)
│   │   ├── cache (stats, flush, keys)
│   │   ├── realtime (status, channels, broadcast)
│   │   └── rate-limit (config, status, reset)
│   │
│   ├── deploy
│   │   ├── staging / production / rollback
│   │   ├── preview / canary / blue-green
│   │   ├── check / status
│   │   ├── env (list, create, switch, diff)
│   │   ├── sync (db, files, config, full, auto, watch, status, history)
│   │   └── validate (config, env)
│   │
│   ├── cloud
│   │   ├── provider (list, init, validate, info)
│   │   ├── server (create, destroy, list, status, ssh, add, remove)
│   │   ├── cost (estimate, compare)
│   │   ├── deploy (quick, full)
│   │   ├── k8s (init, convert, apply, deploy, status, logs, scale, rollback, delete, cluster, namespace)
│   │   └── helm (init, generate, install, upgrade, rollback, uninstall, list, status, values, template, package, repo)
│   │
│   └── observe
│       ├── logs [service]
│       ├── metrics [service]
│       ├── monitor (status, enable, alerts, grafana)
│       ├── health (check, service, endpoint, watch, history, config)
│       ├── doctor [--fix]
│       ├── history (show, deployments, migrations, rollbacks, commands, search, export, clear)
│       ├── audit (events, users, export)
│       ├── urls [--env, --diff]
│       └── exec <service> <cmd>
│
├── Security & Tooling
│   ├── secure
│   │   ├── scan (all, passwords, mfa, sessions, suspicious)
│   │   ├── incidents (list, show, resolve)
│   │   ├── events (list, show)
│   │   ├── webauthn (list, add, remove)
│   │   ├── headers (show, config, test)
│   │   ├── secrets (list, set, get, delete, rotate)
│   │   ├── vault (init, status, seal, unseal, backup, restore)
│   │   └── ssl (generate, install, renew, trust, verify)
│   │
│   ├── plugin
│   │   ├── list / install / remove / update / refresh / status
│   │   ├── stripe (sync, customers, subscriptions, invoices, webhook)
│   │   ├── github (sync, repos, issues, prs, workflows, webhook)
│   │   └── shopify (sync, products, orders, customers, webhook)
│   │
│   └── dev
│       ├── start / stop / status / watch
│       ├── perf (profile, analyze, slow-queries, report, dashboard, suggest)
│       ├── bench (run, baseline, compare, stress, report)
│       ├── scale <service> [--auto]
│       ├── frontend (status, list, add, remove, deploy, logs, env)
│       ├── ci (init, validate, status)
│       └── completion (bash, zsh, fish, install)
│
└── Configuration & Help
    ├── config
    │   ├── show / get / set / list / edit
    │   ├── validate / diff / export / import / reset
    │   └── clean / reset [--hard]
    │
    └── help / version / update / upgrade
```

---

## Migration Timeline

### Phase 1: Add New Commands (Weeks 1-2)
```
✅ New commands work alongside old
✅ Zero breaking changes
✅ Users can start trying new syntax
```

### Phase 2: Deprecation Warnings (Weeks 3-6)
```
⚠️  Old commands show deprecation notice
⚠️  Suggest new command
✅ Continue working normally
```

### Phase 3: Legacy Aliases (Ongoing)
```
✅ Smart redirects
✅ Telemetry tracking (opt-in)
✅ Maintained for 2+ versions
```

### Phase 4: Removal (6-12 months)
```
❌ Old commands removed
✅ Helpful error messages
✅ Migration guide available
```

---

## Help Text Comparison

### BEFORE (77 commands)

```
$ nself help
Usage: nself <command>

Commands:
  admin          Admin UI
  admin-dev      Admin dev mode
  audit          Audit trail
  auth           Authentication
  backup         Backup database
  bench          Benchmarking
  billing        Billing management
  build          Build project
  ci             CI/CD generation
  clean          Clean resources
  cloud          Cloud infrastructure
  completion     Shell completion
  config         Configuration
  db             Database operations
  deploy         Deploy application
  dev            Development mode
  devices        User devices
  doctor         Diagnostics
  down           Stop services
  email          Email service
  env            Environment management
  exec           Execute in container
  frontend       Frontend apps
  functions      Serverless functions
  health         Health checks
  helm           Helm charts
  help           Show help
  history        History
  init           Initialize project
  k8s            Kubernetes
  logs           View logs
  metrics        Metrics
  mfa            Multi-factor auth
  migrate        Migrations
  mlflow         ML tracking
  monitor        Monitoring
  nself          nself CLI
  oauth          OAuth providers
  org            Organizations
  perf           Performance
  plugin         Plugins
  prod           Production
  providers      Cloud providers
  provision      Provision servers
  rate-limit     Rate limiting
  realtime       Realtime service
  redis          Redis cache
  reset          Reset project
  restart        Restart services
  restore        Restore database
  roles          User roles
  rollback       Rollback deployment
  scale          Service scaling
  search         Search service
  secrets        Secrets management
  security       Security
  server         Server management
  servers        List servers
  service        Service management
  ssl            SSL certificates
  staging        Staging deployment
  start          Start services
  status         Service status
  stop           Stop services
  storage        Object storage
  sync           Sync data
  tenant         Multi-tenancy
  trust          Trust certificates
  up             Start services
  update         Update nself
  upgrade        Upgrade nself
  urls           Service URLs
  validate       Validate config
  vault          Vault management
  version        Show version
  webhooks       Webhooks
  whitelabel     White-label

(77 commands - overwhelming!)
```

### AFTER (13 categories)

```
$ nself help
Usage: nself <category> <action>

Core Commands:
  init          Initialize project
  build         Build configs
  start         Start services
  stop          Stop services
  restart       Restart services
  status        Service status

Main Categories:
  db            Database operations (migrate, backup, types, etc.)
  auth          Authentication & authorization (login, oauth, mfa, roles, etc.)
  tenant        Multi-tenancy (create, billing, branding, domains, etc.)
  service       Service management (admin, email, search, storage, etc.)

  deploy        Deployment & environments (staging, prod, sync, etc.)
  cloud         Cloud infrastructure (providers, servers, k8s, helm, etc.)
  observe       Monitoring & observability (logs, metrics, health, etc.)
  secure        Security & compliance (scan, secrets, vault, ssl, etc.)

  plugin        Plugin management (install, update, stripe, github, etc.)
  dev           Developer tools (perf, bench, frontend, ci, etc.)
  config        Configuration (show, edit, validate, clean, etc.)

Utilities:
  help          Show help
  version       Show version
  update        Update nself
  upgrade       Upgrade nself

For category help: nself help <category>
For command help:  nself <category> <action> --help

(13 categories - easy to scan!)
```

---

## Common Workflows

### Daily Development

**BEFORE:**
```bash
nself start
nself status
nself logs postgres
nself db migrate up
nself db seed
nself urls
```

**AFTER:**
```bash
nself start
nself status
nself observe logs postgres
nself db migrate up
nself db seed
nself observe urls
```

### Deployment

**BEFORE:**
```bash
nself deploy check
nself staging
nself sync db staging
nself prod
nself rollback
```

**AFTER:**
```bash
nself deploy check
nself deploy staging
nself deploy sync db staging
nself deploy production
nself deploy rollback
```

### Service Management

**BEFORE:**
```bash
nself service enable redis
nself redis flush
nself email test
nself search index
nself functions deploy
```

**AFTER:**
```bash
nself service enable redis
nself service cache flush
nself service email test
nself service search index
nself service functions deploy
```

### Monitoring & Debugging

**BEFORE:**
```bash
nself logs postgres
nself health check
nself doctor
nself metrics
nself urls
```

**AFTER:**
```bash
nself observe logs postgres
nself observe health check
nself observe doctor
nself observe metrics
nself observe urls
```

---

## FAQ

### Q: Will my existing scripts break?

**A:** No! Legacy aliases will be supported for 2+ major versions. Your scripts will continue to work.

### Q: Can I use both old and new commands?

**A:** Yes! During the transition period, both will work. Eventually, old commands will show deprecation warnings before being removed.

### Q: How do I migrate?

**A:** Use the migration map above. Most changes are straightforward:
- `nself logs` → `nself observe logs`
- `nself oauth` → `nself auth oauth`
- `nself email` → `nself service email`

### Q: Why not keep everything top-level?

**A:** 77 commands is too many to remember. Grouping by category makes discovery easier and creates a clear mental model.

### Q: What if I forget the new command?

**A:** The old command will redirect and show you the new one:
```bash
$ nself logs postgres
⚠️  DEPRECATED: Use 'nself observe logs postgres'
[logs continue...]
```

### Q: Will tab completion work?

**A:** Yes! Tab completion will work for both old and new commands during the transition.

---

## Summary

**Problem:** 77 top-level commands creating confusion
**Solution:** 13 logical categories with backward-compatible migration
**Result:** 71% reduction in top-level commands, improved discoverability
**Timeline:** 4-phase rollout over 6-12 months
**Risk:** Low - legacy aliases maintain compatibility

**Next Step:** Review proposal and approve for implementation

---

For full details, see [COMMAND-REORGANIZATION-PROPOSAL.md](./COMMAND-REORGANIZATION-PROPOSAL.md)
