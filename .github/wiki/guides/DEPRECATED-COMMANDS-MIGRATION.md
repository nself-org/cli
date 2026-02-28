# Deprecated Commands Migration Guide

**Version**: v0.9.9+
**Last Updated**: 2026-02-16

---

## Overview

Starting in **v0.9.9**, nself consolidated **79 top-level commands** into **31 organized command hierarchies**. This consolidation improves:

- **Discoverability** - Related functionality grouped logically
- **Consistency** - Predictable command patterns
- **Maintainability** - Easier to document and extend
- **User Experience** - Less cognitive load, clearer mental model

**What this means for you:**
- All deprecated commands still work in v0.9.9+ with warning messages
- Commands will be removed in v1.0.0 (planned)
- Simple migration: most commands just need a prefix added

**Example:**
```bash
# Old (deprecated)
nself email send user@example.com "Subject" --body "Message"

# New (recommended)
nself service email send user@example.com "Subject" --body "Message"
```

---

## Quick Reference Table

### All Deprecated Commands

| Old Command | New Command | Category | Notes |
|-------------|-------------|----------|-------|
| `billing <cmd>` | `tenant billing <cmd>` | Multi-tenancy | Billing is tenant-specific |
| `org <cmd>` | `tenant org <cmd>` | Multi-tenancy | Organizations are tenant containers |
| `upgrade <cmd>` | `deploy upgrade <cmd>` | Deployment | Upgrade is a deployment operation |
| `staging` | `deploy staging` | Deployment | Quick access to staging deployment |
| `prod` | `deploy production` | Deployment | Quick access to prod deployment |
| `production` | `deploy production` | Deployment | Alias for consistency |
| `provision <cmd>` | `deploy provision <cmd>` | Deployment | Provision for deployment |
| `server <cmd>` | `deploy server <cmd>` | Deployment | Server management for deployment |
| `servers <cmd>` | `deploy server list` | Deployment | Alias for server listing |
| `sync <cmd>` | `deploy sync <cmd>` OR `config sync <cmd>` | Deployment/Config | **Context-dependent** (see below) |
| `provider <cmd>` | `infra provider <cmd>` | Infrastructure | Cloud infrastructure |
| `cloud <cmd>` | `infra provider <cmd>` | Infrastructure | Deprecated, now provider |
| `k8s <cmd>` | `infra k8s <cmd>` | Infrastructure | Kubernetes infrastructure |
| `helm <cmd>` | `infra helm <cmd>` | Infrastructure | Helm infrastructure |
| `storage <cmd>` | `service storage <cmd>` | Services | Storage is a service |
| `email <cmd>` | `service email <cmd>` | Services | Email is a service |
| `search <cmd>` | `service search <cmd>` | Services | Search is a service |
| `redis <cmd>` | `service redis <cmd>` | Services | Redis is a service |
| `functions <cmd>` | `service functions <cmd>` | Services | Functions are a service |
| `mlflow <cmd>` | `service mlflow <cmd>` | Services | MLflow is a service |
| `realtime <cmd>` | `service realtime <cmd>` | Services | Realtime is a service |
| `admin-dev` | `service admin --dev` | Services | Dev mode flag |
| `env <cmd>` | `config env <cmd>` | Configuration | Environment configuration |
| `secrets <cmd>` | `config secrets <cmd>` | Configuration | Secrets are configuration |
| `vault <cmd>` | `config vault <cmd>` | Configuration | Vault is for secrets/config |
| `validate` | `config validate` | Configuration | Configuration validation |
| `mfa <cmd>` | `auth mfa <cmd>` | Security | MFA is authentication |
| `roles <cmd>` | `auth roles <cmd>` | Security | Roles are auth/security |
| `devices <cmd>` | `auth devices <cmd>` | Security | Device management is auth |
| `oauth <cmd>` | `auth oauth <cmd>` | Security | OAuth is authentication |
| `security <cmd>` | `auth security <cmd>` | Security | Security operations |
| `ssl <cmd>` | `auth ssl <cmd>` | Security | SSL is security |
| `trust` | `auth ssl trust` | Security | Trust local certificates |
| `rate-limit <cmd>` | `auth rate-limit <cmd>` | Security | Rate limiting is security |
| `webhooks <cmd>` | `auth webhooks <cmd>` | Security | Webhook security |
| `bench <cmd>` | `perf bench <cmd>` | Performance | Benchmarking is performance |
| `scale <cmd>` | `perf scale <cmd>` | Performance | Scaling is performance |
| `migrate <cmd>` | `perf migrate <cmd>` | Performance | Migration is performance-related |
| `rollback` | `backup rollback` | Backup | Rollback is backup/recovery |
| `reset` | `backup reset` | Backup | Reset is recovery |
| `clean` | `backup clean` | Backup | Cleanup is maintenance |
| `frontend <cmd>` | `dev frontend <cmd>` | Developer Tools | Frontend is dev tooling |
| `ci <cmd>` | `dev ci <cmd>` | Developer Tools | CI/CD is dev tooling |
| `docs <cmd>` | `dev docs <cmd>` | Developer Tools | Documentation is dev tooling |
| `whitelabel <cmd>` | `dev whitelabel <cmd>` | Developer Tools | White-label is dev tooling |

---

## Detailed Migrations by Category

### 1. Service Commands

**Philosophy**: All optional services (storage, email, search, redis, functions, mlflow, realtime) are now under the unified `service` command.

#### email → service email

**Old:**
```bash
nself email send user@example.com "Welcome" --body "Hello!"
nself email template list
nself email test smtp
nself email config sendgrid
```

**New:**
```bash
nself service email send user@example.com "Welcome" --body "Hello!"
nself service email template list
nself service email test smtp
nself service email config sendgrid
```

**Why**: Email is a service like any other. Grouping under `service` makes it consistent with other service management commands.

---

#### functions → service functions

**Old:**
```bash
nself functions init
nself functions deploy my-function
nself functions list
nself functions logs my-function
nself functions invoke my-function --data '{"key":"value"}'
```

**New:**
```bash
nself service functions init
nself service functions deploy my-function
nself service functions list
nself service functions logs my-function
nself service functions invoke my-function --data '{"key":"value"}'
```

---

#### mlflow → service mlflow

**Old:**
```bash
nself mlflow init
nself mlflow ui
nself mlflow experiments
nself mlflow models
```

**New:**
```bash
nself service mlflow init
nself service mlflow ui
nself service mlflow experiments
nself service mlflow models
```

---

#### realtime → service realtime

**Old:**
```bash
nself realtime init
nself realtime events
nself realtime test
```

**New:**
```bash
nself service realtime init
nself service realtime events
nself service realtime test
```

---

#### redis → service redis

**Old:**
```bash
nself redis init
nself redis flush --pattern "session:*"
nself redis cli
nself redis stats
```

**New:**
```bash
nself service redis init
nself service redis flush --pattern "session:*"
nself service redis cli
nself service redis stats
```

---

#### search → service search

**Old:**
```bash
nself search init meilisearch
nself search index create products
nself search query "laptop"
nself search config
```

**New:**
```bash
nself service search init meilisearch
nself service search index create products
nself service search query "laptop"
nself service search config
```

---

#### storage → service storage

**Old:**
```bash
nself storage init
nself storage upload photo.jpg
nself storage list images/
nself storage delete old-file.pdf
nself storage config
nself storage status
nself storage test
nself storage graphql-setup
```

**New:**
```bash
nself service storage init
nself service storage upload photo.jpg
nself service storage list images/
nself service storage delete old-file.pdf
nself service storage config
nself service storage status
nself service storage test
nself service storage graphql-setup
```

---

#### admin-dev → service admin --dev

**Old:**
```bash
nself admin-dev
```

**New:**
```bash
nself service admin --dev
# Or simply:
nself admin --dev
```

**Note**: `nself admin` is still a top-level utility command, but the dev mode is now a flag.

---

### 2. Deployment Commands

**Philosophy**: All deployment-related operations (staging, production, provisioning, servers, upgrades) belong under `deploy`.

#### staging → deploy staging

**Old:**
```bash
nself staging --auto-migrate
```

**New:**
```bash
nself deploy staging --auto-migrate
```

**Why**: Staging is a deployment environment, not a standalone concept.

---

#### prod / production → deploy production

**Old:**
```bash
nself prod --auto-migrate
nself production --auto-migrate
```

**New:**
```bash
nself deploy production --auto-migrate
# Or use the alias:
nself deploy prod --auto-migrate
```

---

#### provision → deploy provision

**Old:**
```bash
nself provision hetzner --size cx21 --region nbg1
nself provision digitalocean --size s-2vcpu-4gb --region nyc3
```

**New:**
```bash
nself deploy provision hetzner --size cx21 --region nbg1
nself deploy provision digitalocean --size s-2vcpu-4gb --region nyc3
```

**Why**: Provisioning is the first step in deployment. It creates the server you'll deploy to.

---

#### server / servers → deploy server

**Old:**
```bash
nself server list
nself server create staging-1 --host 192.168.1.100
nself server status staging-1
nself server ssh staging-1
nself server destroy staging-1

nself servers list  # Alias
```

**New:**
```bash
nself deploy server list
nself deploy server create staging-1 --host 192.168.1.100
nself deploy server status staging-1
nself deploy server ssh staging-1
nself deploy server destroy staging-1
```

**Note**: `servers list` is now `deploy server list` (singular).

**New in v0.9.6 - Enhanced Server Management:**
```bash
nself deploy server init <host> --domain example.com    # Initialize VPS for nself
nself deploy server check <host>                        # Verify server readiness
nself deploy server diagnose staging                    # Comprehensive diagnostics
nself deploy server add staging-2 --host 10.0.0.5       # Add server configuration
nself deploy server remove staging-2 --force            # Remove server configuration
nself deploy server info staging-1                      # Display comprehensive info
```

---

#### upgrade → deploy upgrade

**Old:**
```bash
nself upgrade --zero-downtime
nself upgrade --version 0.9.8
```

**New:**
```bash
nself deploy upgrade --zero-downtime
nself deploy upgrade --version 0.9.8
```

**Why**: Upgrading is a deployment operation that affects running infrastructure.

---

#### sync → deploy sync OR config sync (Special Case)

**This command was SPLIT into two categories based on context:**

##### Deploy Sync (Remote Environment Synchronization)

**Use case**: Syncing code/configurations between local and remote environments.

**Old:**
```bash
nself sync staging pull
nself sync prod push
nself sync status
```

**New:**
```bash
nself deploy sync pull staging --dry-run
nself deploy sync push prod --force
nself deploy sync status
nself deploy sync full staging --no-rebuild
```

**New in v0.9.6:**
- `--dry-run` flag for safe preview
- `--force` flag to skip confirmations
- `full` subcommand for complete synchronization

##### Config Sync (Configuration Management)

**Use case**: Syncing configuration files, environment variables, secrets.

**Old:**
```bash
nself sync env staging
nself sync secrets prod
```

**New:**
```bash
nself config sync env staging
nself config sync secrets prod
nself config sync validate
```

**Decision Guide**:
- **Deploying/pushing code to servers?** → Use `deploy sync`
- **Managing environment variables/secrets?** → Use `config sync`

---

### 3. Infrastructure Commands

**Philosophy**: All cloud, Kubernetes, and infrastructure operations belong under `infra`.

#### provider / cloud → infra provider

**Old:**
```bash
nself provider list
nself provider init hetzner
nself provider validate aws
nself provider server create digitalocean

nself cloud list  # Deprecated alias
```

**New:**
```bash
nself infra provider list
nself infra provider init hetzner
nself infra provider validate aws
nself infra provider server create digitalocean
```

**New in v0.9.6 - Kubernetes Abstraction:**
```bash
# Unified K8s cluster management across 8 cloud providers
nself infra provider k8s-create aws my-cluster us-east-1 3 medium
nself infra provider k8s-delete gcp my-cluster us-central1
nself infra provider k8s-kubeconfig digitalocean my-cluster nyc3
```

**Supported K8s Providers:**
- AWS (EKS) - $73/month control plane
- GCP (GKE) - Free control plane
- Azure (AKS) - Free control plane
- DigitalOcean (DOKS) - $12/month
- Linode (LKE) - Free control plane
- Vultr (VKE) - Free control plane
- Hetzner - Free control plane (manual via console)
- Scaleway (Kapsule) - Free control plane

---

#### k8s → infra k8s

**Old:**
```bash
nself k8s init
nself k8s convert
nself k8s apply
nself k8s deploy
nself k8s status
nself k8s logs my-pod
nself k8s scale my-deployment 5
nself k8s rollback
nself k8s delete
```

**New:**
```bash
nself infra k8s init
nself infra k8s convert
nself infra k8s apply
nself infra k8s deploy
nself infra k8s status
nself infra k8s logs my-pod
nself infra k8s scale my-deployment 5
nself infra k8s rollback
nself infra k8s delete
```

---

#### helm → infra helm

**Old:**
```bash
nself helm init
nself helm generate
nself helm install my-release
nself helm upgrade my-release
nself helm rollback my-release
nself helm list
nself helm status my-release
```

**New:**
```bash
nself infra helm init
nself infra helm generate
nself infra helm install my-release
nself infra helm upgrade my-release
nself infra helm rollback my-release
nself infra helm list
nself infra helm status my-release
```

---

### 4. Configuration Commands

**Philosophy**: All environment variables, secrets, and configuration management belong under `config`.

#### env → config env

**Old:**
```bash
nself env list
nself env switch staging
nself env create testing
nself env delete testing
nself env sync prod
```

**New:**
```bash
nself config env list
nself config env switch staging
nself config env create testing
nself config env delete testing
nself config env sync prod
```

---

#### secrets → config secrets

**Old:**
```bash
nself secrets list
nself secrets get DATABASE_PASSWORD
nself secrets set API_KEY "abc123"
nself secrets delete OLD_KEY
nself secrets rotate
```

**New:**
```bash
nself config secrets list
nself config secrets get DATABASE_PASSWORD
nself config secrets set API_KEY "abc123"
nself config secrets delete OLD_KEY
nself config secrets rotate
```

---

#### vault → config vault

**Old:**
```bash
nself vault init
nself vault config
nself vault status
```

**New:**
```bash
nself config vault init
nself config vault config
nself config vault status
```

---

#### validate → config validate

**Old:**
```bash
nself validate
nself validate --strict
```

**New:**
```bash
nself config validate
nself config validate --strict
```

---

### 5. Authentication & Security Commands

**Philosophy**: All authentication, authorization, SSL, and security operations belong under `auth`.

#### mfa → auth mfa

**Old:**
```bash
nself mfa enable
nself mfa disable
nself mfa verify 123456
nself mfa backup-codes
```

**New:**
```bash
nself auth mfa enable
nself auth mfa disable
nself auth mfa verify 123456
nself auth mfa backup-codes
```

---

#### roles → auth roles

**Old:**
```bash
nself roles list
nself roles create admin "admin,users,billing"
nself roles assign user@example.com admin
nself roles remove user@example.com admin
```

**New:**
```bash
nself auth roles list
nself auth roles create admin "admin,users,billing"
nself auth roles assign user@example.com admin
nself auth roles remove user@example.com admin
```

---

#### devices → auth devices

**Old:**
```bash
nself devices list
nself devices register my-laptop
nself devices revoke old-phone
nself devices trust my-laptop
```

**New:**
```bash
nself auth devices list
nself auth devices register my-laptop
nself auth devices revoke old-phone
nself auth devices trust my-laptop
```

---

#### oauth → auth oauth

**Old:**
```bash
nself oauth install
nself oauth enable google
nself oauth disable github
nself oauth config google --client-id XXX --client-secret YYY
nself oauth test google
nself oauth list
nself oauth status
```

**New:**
```bash
nself auth oauth install
nself auth oauth enable google
nself auth oauth disable github
nself auth oauth config google --client-id XXX --client-secret YYY
nself auth oauth test google
nself auth oauth list
nself auth oauth status
```

---

#### security → auth security

**Old:**
```bash
nself security scan
nself security scan --deep
nself security audit
nself security report
```

**New:**
```bash
nself auth security scan
nself auth security scan --deep
nself auth security audit
nself auth security report
```

---

#### ssl → auth ssl

**Old:**
```bash
nself ssl generate example.com
nself ssl install ./cert.pem
nself ssl renew example.com
nself ssl info example.com
```

**New:**
```bash
nself auth ssl generate example.com
nself auth ssl install ./cert.pem
nself auth ssl renew example.com
nself auth ssl info example.com
```

---

#### trust → auth ssl trust

**Old:**
```bash
nself trust
```

**New:**
```bash
nself auth ssl trust
```

**Why**: "Trust" specifically refers to trusting self-signed SSL certificates for local development.

---

#### rate-limit → auth rate-limit

**Old:**
```bash
nself rate-limit config --requests 100 --window 60
nself rate-limit status
nself rate-limit reset 192.168.1.100
```

**New:**
```bash
nself auth rate-limit config --requests 100 --window 60
nself auth rate-limit status
nself auth rate-limit reset 192.168.1.100
```

---

#### webhooks → auth webhooks

**Old:**
```bash
nself webhooks create https://example.com/hook user.created,user.updated
nself webhooks list
nself webhooks delete webhook-123
nself webhooks test webhook-123
nself webhooks logs webhook-123
```

**New:**
```bash
nself auth webhooks create https://example.com/hook user.created,user.updated
nself auth webhooks list
nself auth webhooks delete webhook-123
nself auth webhooks test webhook-123
nself auth webhooks logs webhook-123
```

**Why**: Webhooks are a security concern (authentication, validation, replay protection).

---

### 6. Performance Commands

**Philosophy**: All performance, benchmarking, and scaling operations belong under `perf`.

#### bench → perf bench

**Old:**
```bash
nself bench
nself bench postgres --duration 60
nself bench api --requests 10000
```

**New:**
```bash
nself perf bench
nself perf bench postgres --duration 60
nself perf bench api --requests 10000
```

---

#### scale → perf scale

**Old:**
```bash
nself scale api 5
nself scale worker 10
```

**New:**
```bash
nself perf scale api 5
nself perf scale worker 10
```

---

#### migrate → perf migrate

**Old:**
```bash
nself migrate --analyze
nself migrate --optimize
```

**New:**
```bash
nself perf migrate --analyze
nself perf migrate --optimize
```

**Note**: This is different from `nself db migrate` (database migrations). `perf migrate` is for performance optimization during migrations.

---

### 7. Backup & Recovery Commands

**Philosophy**: All backup, restore, rollback, and cleanup operations belong under `backup`.

#### rollback → backup rollback

**Old:**
```bash
nself rollback
nself rollback --version 3
```

**New:**
```bash
nself backup rollback
nself backup rollback --version 3
```

---

#### reset → backup reset

**Old:**
```bash
nself reset
nself reset --confirm
```

**New:**
```bash
nself backup reset
nself backup reset --confirm
```

---

#### clean → backup clean

**Old:**
```bash
nself clean
nself clean --age 30
nself clean --dry-run
```

**New:**
```bash
nself backup clean
nself backup clean --age 30
nself backup clean --dry-run
```

---

### 8. Multi-Tenancy & Billing Commands

**Philosophy**: All tenant operations (including billing and organizations) belong under `tenant`.

#### billing → tenant billing

**Old:**
```bash
nself billing plans
nself billing subscribe tenant-123 enterprise
nself billing cancel tenant-123
nself billing usage tenant-123
nself billing invoice tenant-123
nself billing payment tenant-123
nself billing stripe
nself billing test
```

**New:**
```bash
nself tenant billing plans
nself tenant billing subscribe tenant-123 enterprise
nself tenant billing cancel tenant-123
nself tenant billing usage tenant-123
nself tenant billing invoice tenant-123
nself tenant billing payment tenant-123
nself tenant billing stripe
nself tenant billing test
```

**Why**: Billing is always tenant-specific. No billing exists without a tenant context.

---

#### org → tenant org

**Old:**
```bash
nself org create "Acme Corp"
nself org list
nself org show org-123
nself org members org-123
nself org delete org-123
```

**New:**
```bash
nself tenant org create "Acme Corp"
nself tenant org list
nself tenant org show org-123
nself tenant org members org-123
nself tenant org delete org-123
```

**Why**: Organizations are containers for multiple tenants. They're part of the multi-tenancy system.

---

### 9. Developer Tools Commands

**Philosophy**: All developer tooling (frontend, CI, docs, white-label) belongs under `dev`.

#### frontend → dev frontend

**Old:**
```bash
nself frontend add webapp 3000
nself frontend remove webapp
nself frontend list
nself frontend config webapp
```

**New:**
```bash
nself dev frontend add webapp 3000
nself dev frontend remove webapp
nself dev frontend list
nself dev frontend config webapp
```

---

#### ci → dev ci

**Old:**
```bash
nself ci generate
nself ci generate --provider github
nself ci update
nself ci templates
```

**New:**
```bash
nself dev ci generate
nself dev ci generate --provider github
nself dev ci update
nself dev ci templates
```

---

#### docs → dev docs

**Old:**
```bash
nself docs generate
nself docs serve
nself docs build
```

**New:**
```bash
nself dev docs generate
nself dev docs serve
nself dev docs build
```

---

#### whitelabel → dev whitelabel

**Old:**
```bash
nself whitelabel config --brand "MyCompany"
nself whitelabel preview
nself whitelabel deploy
```

**New:**
```bash
nself dev whitelabel config --brand "MyCompany"
nself dev whitelabel preview
nself dev whitelabel deploy
```

---

## Special Cases & Exceptions

### Commands That Stayed Top-Level

Some commands remain at the top level because they're fundamental utilities:

- `nself admin` - Direct access to admin UI
- `nself hasura` - Direct access to Hasura console
- `nself status` - Quick health check
- `nself logs` - Quick log access
- `nself urls` - Quick URL listing
- `nself monitor` - Direct monitoring access
- `nself version` - Version info
- `nself update` - Self-update
- `nself help` - Help system

**Why?** These are high-frequency commands that benefit from short paths.

---

### The sync Command Split Explained

The `sync` command was unique in that it had **two distinct use cases**:

1. **Deployment synchronization** - Syncing code/configs between local and remote servers
2. **Configuration synchronization** - Syncing environment variables and secrets

**Decision:**
- Deployment context → `nself deploy sync`
- Configuration context → `nself config sync`

**Examples:**

```bash
# Deploying to staging? Use deploy sync
nself deploy sync pull staging
nself deploy sync push prod

# Managing environment variables? Use config sync
nself config env sync staging
nself config secrets sync prod
```

---

## Migration Strategies

### Strategy 1: Search & Replace (Simple Projects)

If you have a small number of command invocations:

```bash
# Example: Migrate all email commands
find . -type f -name "*.sh" -exec sed -i '' 's/nself email /nself service email /g' {} \;
find . -type f -name "*.md" -exec sed -i '' 's/nself email /nself service email /g' {} \;
```

### Strategy 2: Gradual Migration (Large Projects)

1. **Phase 1** - Update documentation first
2. **Phase 2** - Update CI/CD pipelines
3. **Phase 3** - Update application scripts
4. **Phase 4** - Update developer guides

### Strategy 3: Alias Wrapper (Temporary Bridge)

Create a shell function/alias for transition:

```bash
# In your ~/.bashrc or ~/.zshrc
nself-old-email() {
  echo "DEPRECATED: Use 'nself service email' instead" >&2
  nself service email "$@"
}
```

### Strategy 4: Pre-commit Hook

Add a pre-commit hook to catch deprecated commands:

```bash
#!/bin/bash
# .git/hooks/pre-commit

deprecated_commands=(
  "nself email"
  "nself storage"
  "nself redis"
  "nself functions"
  "nself mlflow"
  "nself realtime"
  "nself search"
  "nself billing"
  "nself org "
  "nself provision"
  "nself upgrade"
  # ... add more
)

for cmd in "${deprecated_commands[@]}"; do
  if git diff --cached --name-only | xargs grep -l "$cmd" 2>/dev/null; then
    echo "ERROR: Deprecated command found: $cmd"
    echo "Please update to new command structure."
    exit 1
  fi
done
```

---

## Timeline & Breaking Changes

### v0.9.9 (Current)
- **Status**: Deprecated commands show warnings
- **Behavior**: All old commands still work
- **Warning message**: "DEPRECATED: Use 'nself service email' instead of 'nself email'"
- **Action required**: None (but start migrating)

### v0.9.10 - v0.9.99 (Future)
- **Status**: Transition period
- **Behavior**: Warnings continue
- **Action required**: Migrate your scripts

### v1.0.0 (Planned - Breaking Release)
- **Status**: Deprecated commands removed
- **Behavior**: Old commands will error
- **Action required**: All migrations MUST be complete

---

## Testing Your Migration

### 1. Check for Deprecated Usage

```bash
# Search your project for deprecated commands
grep -r "nself email" .
grep -r "nself storage" .
grep -r "nself billing" .
# ... etc
```

### 2. Test New Commands

```bash
# Verify new commands work as expected
nself service email --help
nself tenant billing --help
nself deploy sync --help
```

### 3. Run Your Test Suite

After migration, run your full test suite to ensure nothing broke.

### 4. Update Documentation

Don't forget to update:
- README files
- Developer guides
- CI/CD documentation
- Deployment playbooks
- Training materials

---

## Getting Help

### Command Help

```bash
# View all subcommands
nself service --help
nself deploy --help
nself tenant --help

# View specific subcommand help
nself service email --help
nself tenant billing --help
nself deploy sync --help
```

### Documentation

- **Command Tree**: `.wiki/commands/COMMAND-TREE-V1.md`
- **Service Commands**: `.wiki/commands/service/README.md`
- **Deploy Commands**: `.wiki/commands/deploy/README.md`
- **Full Wiki**: `.wiki/Home.md`

### Community Support

- **GitHub Issues**: https://github.com/nself-org/cli/issues
- **Discussions**: https://github.com/nself-org/cli/discussions
- **Discord**: https://discord.gg/nself

### Reporting Issues

If you find a command that doesn't work as documented:

```bash
nself doctor --verbose > issue-report.txt
```

Then open an issue with the report attached.

---

## FAQ

### Q: Will my old scripts break immediately?

**A:** No. Deprecated commands still work in v0.9.9+ with warnings. They won't be removed until v1.0.0.

### Q: Can I disable the deprecation warnings?

**A:** Yes, set `NSELF_SUPPRESS_WARNINGS=1` in your environment:

```bash
export NSELF_SUPPRESS_WARNINGS=1
nself email send ...  # No warning
```

### Q: How do I know which commands I'm using?

**A:** Run this audit:

```bash
nself history --filter deprecated
```

### Q: Is there an automated migration tool?

**A:** Not yet, but you can use search & replace (see Migration Strategies above).

### Q: Why consolidate now?

**A:** The project grew organically to 79 commands. Before v1.0, we're establishing a sustainable structure.

### Q: Will there be more consolidations?

**A:** No. The 31 top-level commands in v1.0 are the stable structure going forward.

### Q: What if I have custom scripts that use old commands?

**A:** Migrate them before v1.0.0. Use the deprecation period to update at your own pace.

### Q: Can I use both old and new commands during migration?

**A:** Yes! This is encouraged. Migrate incrementally, test thoroughly.

---

## Quick Migration Cheat Sheet

**Print this section for quick reference:**

```
OLD                              NEW
────────────────────────────────────────────────────────
nself email <cmd>          →    nself service email <cmd>
nself storage <cmd>        →    nself service storage <cmd>
nself redis <cmd>          →    nself service redis <cmd>
nself functions <cmd>      →    nself service functions <cmd>
nself mlflow <cmd>         →    nself service mlflow <cmd>
nself realtime <cmd>       →    nself service realtime <cmd>
nself search <cmd>         →    nself service search <cmd>
nself billing <cmd>        →    nself tenant billing <cmd>
nself org <cmd>            →    nself tenant org <cmd>
nself provision <cmd>      →    nself deploy provision <cmd>
nself server <cmd>         →    nself deploy server <cmd>
nself upgrade <cmd>        →    nself deploy upgrade <cmd>
nself staging              →    nself deploy staging
nself prod                 →    nself deploy production
nself sync <cmd>           →    nself deploy sync <cmd> OR nself config sync <cmd>
nself provider <cmd>       →    nself infra provider <cmd>
nself k8s <cmd>            →    nself infra k8s <cmd>
nself helm <cmd>           →    nself infra helm <cmd>
nself env <cmd>            →    nself config env <cmd>
nself secrets <cmd>        →    nself config secrets <cmd>
nself vault <cmd>          →    nself config vault <cmd>
nself validate             →    nself config validate
nself mfa <cmd>            →    nself auth mfa <cmd>
nself roles <cmd>          →    nself auth roles <cmd>
nself devices <cmd>        →    nself auth devices <cmd>
nself oauth <cmd>          →    nself auth oauth <cmd>
nself security <cmd>       →    nself auth security <cmd>
nself ssl <cmd>            →    nself auth ssl <cmd>
nself trust                →    nself auth ssl trust
nself rate-limit <cmd>     →    nself auth rate-limit <cmd>
nself webhooks <cmd>       →    nself auth webhooks <cmd>
nself bench <cmd>          →    nself perf bench <cmd>
nself scale <cmd>          →    nself perf scale <cmd>
nself migrate <cmd>        →    nself perf migrate <cmd>
nself rollback             →    nself backup rollback
nself reset                →    nself backup reset
nself clean                →    nself backup clean
nself frontend <cmd>       →    nself dev frontend <cmd>
nself ci <cmd>             →    nself dev ci <cmd>
nself docs <cmd>           →    nself dev docs <cmd>
nself whitelabel <cmd>     →    nself dev whitelabel <cmd>
```

---

## Conclusion

The command consolidation in v0.9.9 creates a more maintainable, discoverable, and consistent CLI experience. While migration requires some effort, the long-term benefits are significant:

- **Easier to learn** - Logical grouping reduces cognitive load
- **Easier to discover** - Related commands are together
- **Easier to document** - Clear hierarchies and patterns
- **Easier to extend** - New features fit into existing structure

**Migration is straightforward**: most commands just need a prefix added. Take advantage of the transition period to migrate at your own pace, and reach out if you need help!

---

**Happy migrating!** 🚀
