# nself v1.0 Documentation Migration Status

**Date:** January 30, 2026
**Task:** Update all documentation to reflect v1.0 command structure
**Scope:** 150+ markdown files with command references

---

## Migration Overview

### Command Consolidation (v1.0)
- **Before:** 79 top-level commands
- **After:** 31 top-level commands with 285+ subcommands
- **Reduction:** 60.8%
- **Breaking Changes:** Yes (with backward-compatible aliases until v2.0)

---

## Completed Updates

### 1. README.md ✅
**Status:** Updated
**Changes:**
- Updated command tree from 56 → 31 commands with subcommand structure
- Updated email commands: `nself email` → `nself service email`
- Updated SSL commands: `nself ssl` / `nself trust` → `nself auth ssl`
- Updated backup commands: `nself backup prune` → `nself backup clean`
- Updated deployment commands: `nself prod` → `nself deploy production`
- Updated search commands: `nself search` → `nself service search`
- Updated admin commands: `nself admin enable/password` → `nself admin` / `nself admin --dev`
- Fixed all documentation links to point to correct v1.0 files

### 2. docs/commands/COMMANDS.md ✅
**Status:** Already v1.0 compliant
**Notes:** This file is the authoritative v1.0 command reference and is already correct

### 3. docs/commands/COMMAND-TREE-V1.md ✅
**Status:** Already v1.0 compliant
**Notes:** This is the source of truth for v1.0 command structure

---

## Files Requiring Updates

### Priority 1: High-Traffic Documentation (Critical)

#### Quick Start & Guides
- [ ] `docs/guides/Quick-Start.md` - **HIGH PRIORITY**
- [ ] `docs/guides/EXAMPLES.md`
- [ ] `docs/guides/Installation.md`
- [ ] `docs/guides/TROUBLESHOOTING.md`
- [ ] `docs/Home.md` - GitHub wiki home page
- [ ] `docs/README.md` - Docs index

#### Command-Specific Documentation
- [ ] `docs/commands/EMAIL.md` → Update to `service email`
- [ ] `docs/commands/STORAGE.md` → Update to `service storage`
- [ ] `docs/commands/SEARCH.md` → Update to `service search`
- [ ] `docs/commands/FUNCTIONS.md` → Update to `service functions`
- [ ] `docs/commands/MLFLOW.md` → Update to `service mlflow`
- [ ] `docs/commands/REDIS.md` → Update to `service redis`
- [ ] `docs/commands/OAUTH.md` → Update to `auth oauth`
- [ ] `docs/commands/MFA.md` → Update to `auth mfa`
- [ ] `docs/commands/DEVICES.md` → Update to `auth devices`
- [ ] `docs/commands/SSL.md` → Update to `auth ssl`
- [ ] `docs/commands/TRUST.md` → Update to `auth ssl trust`
- [ ] `docs/commands/ENV.md` → Update to `config env`
- [ ] `docs/commands/SECRETS.md` → Update to `config secrets`
- [ ] `docs/commands/VAULT.md` → Update to `config vault`
- [ ] `docs/commands/DEPLOY.md` → Add subcommands
- [ ] `docs/commands/STAGING.md` → Mark as alias
- [ ] `docs/commands/PROD.md` → Mark as alias
- [ ] `docs/commands/PROVISION.md` → Update to `deploy provision`
- [ ] `docs/commands/SERVERS.md` → Update to `deploy server`
- [ ] `docs/commands/SYNC.md` → Update to `deploy sync` / `config sync`
- [ ] `docs/commands/PROVIDER.md` → Update to `infra provider`
- [ ] `docs/commands/K8S.md` → Update to `infra k8s`
- [ ] `docs/commands/HELM.md` → Update to `infra helm`
- [ ] `docs/commands/BENCH.md` → Update to `perf bench`
- [ ] `docs/commands/SCALE.md` → Update to `perf scale`
- [ ] `docs/commands/MIGRATE.md` → Update to `perf migrate`
- [ ] `docs/commands/ROLLBACK.md` → Update to `backup rollback`
- [ ] `docs/commands/RESET.md` → Update to `backup reset`
- [ ] `docs/commands/CLEAN.md` → Update to `backup clean`
- [ ] `docs/commands/FRONTEND.md` → Update to `dev frontend`
- [ ] `docs/commands/CI.md` → Update to `dev ci`
- [ ] `docs/commands/WHITELABEL.md` → Update to `dev whitelabel`

### Priority 2: Feature Documentation (Important)

#### Multi-Tenancy & Billing
- [ ] `docs/guides/BILLING-AND-USAGE.md`
- [ ] `docs/billing/QUOTAS.md`
- [ ] `docs/billing/USAGE-QUICK-REFERENCE.md`
- [ ] `docs/api/BILLING-API.md`
- [ ] `docs/troubleshooting/BILLING-TROUBLESHOOTING.md`
- [ ] `docs/architecture/BILLING-ARCHITECTURE.md`

#### White-Label System
- [ ] `docs/features/WHITELABEL-SYSTEM.md`
- [ ] `docs/guides/WHITELABEL-Quick-Start.md`
- [ ] `docs/guides/WHITE-LABEL-CUSTOMIZATION.md`
- [ ] `docs/troubleshooting/WHITE-LABEL-TROUBLESHOOTING.md`
- [ ] `docs/architecture/WHITE-LABEL-ARCHITECTURE.md`
- [ ] `docs/whitelabel/BRANDING-SYSTEM.md`
- [ ] `docs/whitelabel/EMAIL-TEMPLATES.md`
- [ ] `docs/whitelabel/THEMES.md`
- [ ] `docs/whitelabel/BRANDING-QUICK-START.md`

#### OAuth & Authentication
- [ ] `docs/guides/OAUTH-SETUP.md`
- [ ] `docs/guides/OAUTH-QUICK-START.md`
- [ ] `docs/guides/OAUTH-COMPLETE-FLOWS.md`
- [ ] `docs/cli/oauth.md`
- [ ] `docs/api/OAUTH-API.md`

#### Storage & File Uploads
- [ ] `docs/guides/file-upload-pipeline.md`
- [ ] `docs/features/file-upload-pipeline.md`
- [ ] `docs/tutorials/file-uploads-quickstart.md`

#### Real-Time Features
- [ ] `docs/guides/REALTIME-FEATURES.md`
- [ ] `docs/features/REALTIME.md`
- [ ] `docs/features/realtime-examples.md`
- [ ] `docs/commands/REALTIME.md`

#### Search
- [ ] `docs/services/SEARCH.md`
- [ ] `docs/services/TYPESENSE.md`
- [ ] `src/templates/search/typesense/README.md`

### Priority 3: Architecture & Deployment (Medium)

#### Architecture Documentation
- [ ] `docs/architecture/API.md`
- [ ] `docs/architecture/MULTI-TENANCY.md`
- [ ] `docs/architecture/PROJECT_STRUCTURE.md`
- [ ] `docs/architecture/BUILD_ARCHITECTURE.md`
- [ ] `docs/architecture/DIRECTORY_STRUCTURE.md`
- [ ] `docs/architecture/README.md`

#### Deployment & Cloud
- [ ] `docs/deployment/README.md`
- [ ] `docs/deployment/PRODUCTION-DEPLOYMENT.md`
- [ ] `docs/deployment/CUSTOM-SERVICES-PRODUCTION.md`
- [ ] `docs/deployment/CLOUD-PROVIDERS.md`
- [ ] `docs/guides/Deployment.md`
- [ ] `docs/guides/DEPLOYMENT-ARCHITECTURE.md`
- [ ] `docs/guides/ENVIRONMENTS.md`

#### Providers
- [ ] `docs/providers/PROVIDERS-COMPLETE.md`
- [ ] `docs/commands/PROVIDERS.md`

### Priority 4: Tutorials & Examples (Lower)

#### Quick Start Tutorials
- [ ] `docs/tutorials/QUICK-START-SAAS.md`
- [ ] `docs/tutorials/QUICK-START-B2B.md`
- [ ] `docs/tutorials/QUICK-START-AGENCY.md`
- [ ] `docs/tutorials/QUICK-START-MARKETPLACE.md`
- [ ] `docs/tutorials/QUICK-REFERENCE.md`
- [ ] `docs/tutorials/CUSTOM-DOMAINS.md`
- [ ] `docs/tutorials/STRIPE-INTEGRATION.md`

#### Service Templates
- [ ] `docs/services/SERVICE_TEMPLATES.md`

### Priority 5: Configuration & Reference (Lower)

#### Configuration
- [ ] `docs/configuration/ENVIRONMENT-VARIABLES.md`
- [ ] `docs/configuration/ENV-COMPLETE-REFERENCE.md`
- [ ] `docs/configuration/Admin-UI.md`
- [ ] `docs/configuration/SSL.md`
- [ ] `docs/configuration/CUSTOM-SERVICES-ENV-VARS.md`

#### Quick Reference
- [ ] `docs/quick-reference/COMMAND-REFERENCE.md`
- [ ] `docs/quick-reference/QUICK-NAVIGATION.md`
- [ ] `docs/quick-reference/SERVICE-SCAFFOLDING-CHEATSHEET.md`

#### Security
- [ ] `docs/security/README.md`
- [ ] `docs/security/SECURITY-SYSTEM.md`

### Priority 6: Release Notes (Archive)

All release notes in `docs/releases/` should be preserved as-is since they document historical command structure:
- `v0.1.0.md` through `v0.9.5.md` - Keep unchanged (historical)
- `CHANGELOG.md` - Update to include v1.0 breaking changes
- `ROADMAP.md` - Update for v1.0+

---

## Command Migration Reference

### Most Common Changes

```bash
# OLD → NEW

# Services
nself email        → nself service email
nself storage      → nself service storage
nself search       → nself service search
nself functions    → nself service functions
nself mlflow       → nself service mlflow
nself redis        → nself service redis
nself realtime     → nself service realtime

# Security & Auth
nself oauth        → nself auth oauth
nself mfa          → nself auth mfa
nself roles        → nself auth roles
nself devices      → nself auth devices
nself security     → nself auth security
nself ssl          → nself auth ssl
nself trust        → nself auth ssl trust
nself rate-limit   → nself auth rate-limit
nself webhooks     → nself auth webhooks

# Configuration
nself env          → nself config env
nself secrets      → nself config secrets
nself vault        → nself config vault
nself validate     → nself config validate

# Deployment
nself staging      → nself deploy staging
nself prod         → nself deploy production
nself upgrade      → nself deploy upgrade
nself provision    → nself deploy provision
nself server       → nself deploy server
nself sync         → nself deploy sync (or config sync)

# Infrastructure
nself provider     → nself infra provider
nself cloud        → nself infra provider
nself k8s          → nself infra k8s
nself helm         → nself infra helm

# Performance
nself bench        → nself perf bench
nself scale        → nself perf scale
nself migrate      → nself perf migrate

# Backup & Recovery
nself rollback     → nself backup rollback
nself reset        → nself backup reset
nself clean        → nself backup clean

# Developer Tools
nself frontend     → nself dev frontend
nself ci           → nself dev ci
nself docs         → nself dev docs
nself whitelabel   → nself dev whitelabel

# Multi-Tenancy
nself billing      → nself tenant billing
nself org          → nself tenant org
```

---

## Search & Replace Patterns

### For Documentation Updates

Use these patterns to find and update command references:

```bash
# Find old command patterns
grep -r "nself email " docs/
grep -r "nself storage " docs/
grep -r "nself oauth " docs/
grep -r "nself ssl " docs/
grep -r "nself trust" docs/
grep -r "nself staging" docs/
grep -r "nself prod " docs/
grep -r "nself provider " docs/
grep -r "nself billing " docs/
```

### Bulk Update Commands (Use with caution)

```bash
# Update service commands
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself email /nself service email /g' {} +
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself storage /nself service storage /g' {} +
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself search /nself service search /g' {} +

# Update auth commands
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself oauth /nself auth oauth /g' {} +
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself ssl /nself auth ssl /g' {} +
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself trust/nself auth ssl trust/g' {} +

# Update deployment commands
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself staging/nself deploy staging/g' {} +
find docs/ -type f -name "*.md" -exec sed -i '' 's/nself prod /nself deploy production /g' {} +
```

**⚠️ WARNING:** Always review changes before committing! Some contexts may need manual adjustment.

---

## Backward Compatibility Notes

### Aliases (v1.0 - v2.0)

All old commands work as aliases with deprecation warnings:

```bash
$ nself email test
⚠ DEPRECATED: 'nself email' → use 'nself service email'
This alias will be removed in v2.0.0.

[command continues normally...]
```

### Timeline

- **v1.0** (Current): Old commands work with warnings
- **v1.5**: Warnings become more prominent
- **v2.0**: Old commands may be removed (TBD based on user feedback)

---

## Testing Strategy

### 1. Documentation Validation

```bash
# Check for broken links
npm install -g markdown-link-check
find docs/ -name "*.md" -exec markdown-link-check {} \;

# Check for old command patterns (should return none after update)
grep -r "nself email " docs/ | grep -v "nself service email"
grep -r "nself oauth " docs/ | grep -v "nself auth oauth"
```

### 2. Command Reference Validation

```bash
# Verify all commands in COMMAND-TREE-V1.md are documented
# Verify all command files exist in docs/commands/
# Verify cross-references are correct
```

### 3. User Journey Testing

Test documentation for these user journeys:
1. New user: Quick start → first project
2. Email setup: Development → Production
3. Deployment: Local → Staging → Production
4. Multi-tenant setup: Tenant creation → Billing → Branding

---

## Next Steps

### Immediate (This Session)
1. ✅ Update README.md
2. ✅ Create this migration status document
3. ⏭️ Update high-priority guides (Quick Start, EXAMPLES, TROUBLESHOOTING)
4. ⏭️ Update command-specific documentation
5. ⏭️ Update architecture documentation

### Short-term (Next Session)
1. Update feature documentation (multi-tenancy, OAuth, storage)
2. Update deployment & cloud documentation
3. Update tutorials & examples
4. Update configuration reference

### Medium-term
1. Create automated validation scripts
2. Create migration guide for users
3. Update external documentation (wiki, website)
4. Create video tutorials for new command structure

---

## Resources

### Reference Documents
- `/docs/commands/COMMAND-TREE-V1.md` - Authoritative v1.0 command tree
- `/docs/architecture/COMMAND-CONSOLIDATION-MAP.md` - Old → New mapping
- `/docs/commands/COMMANDS.md` - Complete command reference
- Project documentation - Development notes (updated with v1.0 structure)

### Templates
Use these as templates for updating command documentation:
- `docs/commands/COMMANDS.md` - Comprehensive format
- `docs/commands/COMMAND-TREE-V1.md` - Tree structure format

---

**Status:** In Progress
**Last Updated:** January 30, 2026
**Next Update:** After completing Priority 1 files
