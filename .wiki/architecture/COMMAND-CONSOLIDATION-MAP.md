# Command Consolidation Map

**Visual reference showing which commands consolidate into which categories**

> 📖 **For detailed migration instructions, see**: [Deprecated Commands Migration Guide](../guides/DEPRECATED-COMMANDS-MIGRATION.md)

---

## 77 Commands → 13 Categories

### Visual Consolidation Flow

```
BEFORE (77 top-level)                    AFTER (13 categories)
════════════════════                     ═══════════════════════

Core Lifecycle (6)                       Core Lifecycle (6)
├─ init                         ────────→ init
├─ build                        ────────→ build
├─ start                        ────────→ start
├─ stop                         ────────→ stop
├─ restart                      ────────→ restart
└─ status                       ────────→ status


Database (4 commands)                    Database (1 category)
├─ db                           ────────→ db (keep as-is)
├─ migrate                      ────┬───→ db migrate
├─ backup                       ────┤
└─ restore                      ────┴───→ db restore


Authentication (8 commands)              Authentication (1 category)
├─ auth                         ────────→ auth (expand)
├─ oauth                        ────┬───→ auth oauth
├─ mfa                          ────┤
├─ devices                      ────┤
├─ roles                        ────┤
├─ webhooks                     ────┴───→ auth webhooks
├─ security (partial)
└─ audit (partial)


Tenant/Org (4 commands)                  Tenant (1 category)
├─ tenant                       ────────→ tenant (keep as-is)
├─ org                          ────┬───→ tenant
├─ billing                      ────┤
└─ whitelabel                   ────┴───→ tenant branding


Services (11 commands)                   Service (1 category)
├─ service                      ────────→ service (expand)
├─ admin                        ────┬───→ service admin
├─ admin-dev                    ────┤
├─ email                        ────┤
├─ search                       ────┤
├─ functions                    ────┤
├─ mlflow                       ────┤
├─ storage                      ────┤
├─ redis                        ────┤   → service cache
├─ realtime                     ────┤
└─ rate-limit                   ────┴───→ service rate-limit


Deployment (8 commands)                  Deploy (1 category)
├─ deploy                       ────────→ deploy (expand)
├─ staging                      ────┬───→ deploy staging
├─ prod                         ────┤   → deploy production
├─ rollback                     ────┤
├─ env                          ────┤
├─ sync                         ────┤
├─ validate                     ────┤
└─ migrate (tool)               ────┴───→ deploy migrate


Cloud/Infra (8 commands)                 Cloud (1 category)
├─ cloud                        ────────→ cloud (expand)
├─ providers                    ────┬───→ cloud provider
├─ provision                    ────┤   → cloud server create
├─ server                       ────┤
├─ servers                      ────┤   → cloud server list
├─ k8s                          ────┤
├─ helm                         ────┤
└─ (cost, deploy in cloud)      ────┴───→ cloud helm


Observability (9 commands)               Observe (1 NEW category)
├─ logs                         ────┬───→ observe (NEW)
├─ metrics                      ────┤
├─ monitor                      ────┤
├─ health                       ────┤
├─ doctor                       ────┤
├─ history                      ────┤
├─ audit                        ────┤
├─ urls                         ────┤
└─ exec                         ────┴───→ observe exec


Security (6 commands)                    Secure (1 NEW category)
├─ security                     ────┬───→ secure (NEW)
├─ secrets                      ────┤
├─ vault                        ────┤
├─ ssl                          ────┤
└─ trust                        ────┴───→ secure ssl trust


Plugins (1 command)                      Plugin (1 category)
└─ plugin                       ────────→ plugin (keep as-is)


Developer Tools (7 commands)             Dev (1 category)
├─ dev                          ────────→ dev (expand)
├─ perf                         ────┬───→ dev perf
├─ bench                        ────┤
├─ scale                        ────┤
├─ frontend                     ────┤
├─ ci                           ────┤
└─ completion                   ────┴───→ dev completion


Configuration (3 commands)               Config (1 category)
├─ config                       ────────→ config (expand)
├─ clean                        ────┬───→ config clean
└─ reset                        ────┴───→ config reset


Utilities (8 commands)                   Utilities (4 commands)
├─ help                         ────────→ help
├─ version                      ────────→ version
├─ update                       ────────→ update
├─ upgrade                      ────────→ upgrade
├─ up                           ────────→ up (alias for start)
├─ down                         ────────→ down (alias for stop)
├─ nself                        ────────→ (internal)
└─ trust                        ────────→ secure ssl trust
```

---

## Category Consolidation Details

### 1. Core (No Change)

```
✅ KEEP AS-IS
─────────────
init
build
start
stop
restart
status
```

**Reason**: Essential daily commands should remain top-level.

---

### 2. Database → `db`

```
BEFORE               AFTER
──────               ─────
db                   db (expand)
migrate          ──→ db migrate
backup           ──→ db backup
restore          ──→ db restore
```

**Commands absorbed**: 3
**New structure**: All database operations under `nself db`

---

### 3. Authentication → `auth`

```
BEFORE               AFTER
──────               ─────
auth                 auth (expand)
oauth            ──→ auth oauth
mfa              ──→ auth mfa
devices          ──→ auth devices
roles            ──→ auth roles
webhooks         ──→ auth webhooks
security*        ──→ auth (user-facing)
audit*           ──→ auth (audit trail)
```

**Commands absorbed**: 7
**New structure**: All authentication under `nself auth`

---

### 4. Tenant → `tenant`

```
BEFORE               AFTER
──────               ─────
tenant               tenant (keep)
org              ──→ tenant
billing          ──→ tenant billing
whitelabel       ──→ tenant branding
```

**Commands absorbed**: 3
**New structure**: All multi-tenancy under `nself tenant`

---

### 5. Service → `service`

```
BEFORE               AFTER
──────               ─────
service              service (expand)
admin            ──→ service admin
admin-dev        ──→ service admin dev
email            ──→ service email
search           ──→ service search
functions        ──→ service functions
mlflow           ──→ service mlflow
storage          ──→ service storage
redis            ──→ service cache
realtime         ──→ service realtime
rate-limit       ──→ service rate-limit
```

**Commands absorbed**: 10
**New structure**: ALL optional services under `nself service`

---

### 6. Deploy → `deploy`

```
BEFORE               AFTER
──────               ─────
deploy               deploy (expand)
staging          ──→ deploy staging
prod             ──→ deploy production
rollback         ──→ deploy rollback
env              ──→ deploy env
sync             ──→ deploy sync
validate         ──→ deploy validate
migrate*         ──→ deploy migrate
```

**Commands absorbed**: 7
**New structure**: All deployment under `nself deploy`

---

### 7. Cloud → `cloud`

```
BEFORE               AFTER
──────               ─────
cloud                cloud (expand)
providers        ──→ cloud provider
provision        ──→ cloud server create
server           ──→ cloud server
servers          ──→ cloud server list
k8s              ──→ cloud k8s
helm             ──→ cloud helm
```

**Commands absorbed**: 7
**New structure**: All infrastructure under `nself cloud`

---

### 8. Observe → `observe` (NEW)

```
BEFORE               AFTER
──────               ─────
logs             ──→ observe logs
metrics          ──→ observe metrics
monitor          ──→ observe monitor
health           ──→ observe health
doctor           ──→ observe doctor
history          ──→ observe history
audit            ──→ observe audit
urls             ──→ observe urls
exec             ──→ observe exec
```

**Commands absorbed**: 9
**New category**: `observe` - All monitoring/observability

---

### 9. Secure → `secure` (NEW)

```
BEFORE               AFTER
──────               ─────
security         ──→ secure
secrets          ──→ secure secrets
vault            ──→ secure vault
ssl              ──→ secure ssl
trust            ──→ secure ssl trust
mfa*             ──→ auth mfa (user-facing)
```

**Commands absorbed**: 6
**New category**: `secure` - All security operations

---

### 10. Plugin → `plugin`

```
✅ KEEP AS-IS
─────────────
plugin
```

**Reason**: Already well-organized, no changes needed.

---

### 11. Dev → `dev`

```
BEFORE               AFTER
──────               ─────
dev                  dev (expand)
perf             ──→ dev perf
bench            ──→ dev bench
scale            ──→ dev scale
frontend         ──→ dev frontend
ci               ──→ dev ci
completion       ──→ dev completion
```

**Commands absorbed**: 6
**New structure**: All developer tools under `nself dev`

---

### 12. Config → `config`

```
BEFORE               AFTER
──────               ─────
config               config (expand)
clean            ──→ config clean
reset            ──→ config reset
```

**Commands absorbed**: 2
**New structure**: All configuration under `nself config`

---

### 13. Utilities → `help`, `version`, `update`, `upgrade`

```
✅ KEEP TOP-LEVEL
─────────────────
help
version
update
upgrade
```

**Reason**: Essential utilities should remain top-level.

---

## Consolidation Statistics

| Category | Commands Before | Commands After | Reduction |
|----------|-----------------|----------------|-----------|
| Core | 6 | 6 | 0% |
| Database | 4 → 1 | 1 | 75% |
| Auth | 8 → 1 | 1 | 87.5% |
| Tenant | 4 → 1 | 1 | 75% |
| Service | 11 → 1 | 1 | 91% |
| Deploy | 8 → 1 | 1 | 87.5% |
| Cloud | 8 → 1 | 1 | 87.5% |
| Observe | 9 → 1 | 1 (NEW) | 89% |
| Secure | 6 → 1 | 1 (NEW) | 83% |
| Plugin | 1 | 1 | 0% |
| Dev | 7 → 1 | 1 | 86% |
| Config | 3 → 1 | 1 | 67% |
| Utilities | 8 | 4 | 50% |
| **TOTAL** | **77** | **22** | **71%** |

---

## Command Flow Examples

### Observability Commands Flow

```
BEFORE                           AFTER
──────                           ─────

nself logs postgres              nself observe logs postgres
nself logs --follow              nself observe logs --follow
nself metrics                    nself observe metrics
nself health check               nself observe health check
nself doctor                     nself observe doctor
nself urls                       nself observe urls
nself exec postgres psql         nself observe exec postgres psql
```

**Pattern**: `nself <action>` → `nself observe <action>`

---

### Service Management Flow

```
BEFORE                           AFTER
──────                           ─────

nself service list               nself service list (same)
nself admin open                 nself service admin open
nself email test                 nself service email test
nself search index               nself service search index
nself functions deploy           nself service functions deploy
nself redis flush                nself service cache flush
nself storage upload file.jpg    nself service storage upload file.jpg
```

**Pattern**: `nself <service>` → `nself service <service>`

---

### Deployment Flow

```
BEFORE                           AFTER
──────                           ─────

nself staging                    nself deploy staging
nself prod                       nself deploy production
nself env switch prod            nself deploy env switch prod
nself sync db staging            nself deploy sync db staging
nself rollback                   nself deploy rollback
```

**Pattern**: `nself <action>` → `nself deploy <action>`

---

### Cloud Infrastructure Flow

```
BEFORE                           AFTER
──────                           ─────

nself providers list             nself cloud provider list
nself provision do               nself cloud server create do
nself servers list               nself cloud server list
nself server ssh myserver        nself cloud server ssh myserver
nself k8s deploy                 nself cloud k8s deploy
nself helm install               nself cloud helm install
```

**Pattern**: `nself <resource>` → `nself cloud <resource>`

---

### Security Flow

```
BEFORE                           AFTER
──────                           ─────

nself security scan              nself secure scan
nself secrets set KEY=value      nself secure secrets set KEY=value
nself vault init                 nself secure vault init
nself ssl generate               nself secure ssl generate
nself trust                      nself secure ssl trust
```

**Pattern**: `nself <action>` → `nself secure <action>`

---

## Impact Analysis

### Most Affected Users

1. **CI/CD Pipelines** - Scripts using deployment commands
   - Old: `nself staging`, `nself prod`
   - New: `nself deploy staging`, `nself deploy production`
   - **Mitigation**: Legacy aliases maintain compatibility

2. **Monitoring Scripts** - Scripts using observability commands
   - Old: `nself logs`, `nself health`, `nself urls`
   - New: `nself observe logs`, `nself observe health`, `nself observe urls`
   - **Mitigation**: Legacy aliases for 2+ versions

3. **Service Management** - Scripts managing optional services
   - Old: `nself email`, `nself search`, `nself redis`
   - New: `nself service email`, `nself service search`, `nself service cache`
   - **Mitigation**: Clear deprecation warnings

### Least Affected Users

1. **Core Operations** - Daily development workflow unchanged
   - `nself init`, `nself build`, `nself start`, `nself stop` remain the same

2. **Database Operations** - Minor changes only
   - `nself db` mostly unchanged, just absorbs `backup`, `restore`, `migrate`

3. **Tenant Management** - Already well-organized
   - `nself tenant` remains largely unchanged

---

## Migration Complexity

### Easy Migrations (1:1 rename)

```
Simple prefix addition:
─────────────────────
nself logs       → nself observe logs
nself metrics    → nself observe metrics
nself urls       → nself observe urls
nself doctor     → nself observe doctor
nself ssl        → nself secure ssl
nself secrets    → nself secure secrets
```

**Effort**: Low - Just add category prefix

---

### Medium Migrations (rename + regroup)

```
Logical regrouping:
──────────────────
nself admin      → nself service admin
nself email      → nself service email
nself redis      → nself service cache
nself staging    → nself deploy staging
nself prod       → nself deploy production
```

**Effort**: Medium - New grouping to learn

---

### Complex Migrations (structural changes)

```
Multi-level changes:
───────────────────
nself server      → nself cloud server
nself servers     → nself cloud server list
nself provision   → nself cloud server create
nself k8s         → nself cloud k8s
nself helm        → nself cloud helm
```

**Effort**: Higher - More significant restructuring

---

## Quick Reference Card

Print this for your desk!

```
╔══════════════════════════════════════════════════════════╗
║            nself Command Quick Reference                 ║
║                  (New Structure)                         ║
╚══════════════════════════════════════════════════════════╝

CORE (no change)
────────────────
  nself init / build / start / stop / restart / status

DATA & BUSINESS
───────────────
  nself db <action>           Database operations
  nself auth <action>         Authentication
  nself tenant <action>       Multi-tenancy
  nself service <action>      Service management

INFRASTRUCTURE
──────────────
  nself deploy <action>       Deployment
  nself cloud <action>        Cloud infrastructure
  nself observe <action>      Monitoring/observability

SECURITY & TOOLS
────────────────
  nself secure <action>       Security operations
  nself plugin <action>       Plugins
  nself dev <action>          Developer tools

CONFIGURATION
─────────────
  nself config <action>       Configuration
  nself help / version / update / upgrade

EXAMPLES
────────
  nself observe logs postgres
  nself service email test
  nself deploy staging
  nself cloud server list
  nself secure scan
  nself dev perf profile

MIGRATION
─────────
  Old command still works (for now)
  New command is preferred
  Help: nself help migrate
```

---

**For full details**: See [COMMAND-REORGANIZATION-PROPOSAL.md](./COMMAND-REORGANIZATION-PROPOSAL.md)
