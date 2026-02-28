# Deprecated Command Migration Guide

**Applies to:** nself v1.0.0 and later
**Last Updated:** February 2026

---

## Overview

nself v0.9.6 consolidated the CLI from **79 top-level commands down to 31**, grouping related functionality under logical parent commands (`service`, `deploy`, `config`, `auth`, `tenant`, and others). The goal was to make the CLI more discoverable, consistent, and maintainable.

All deprecated commands continue to work throughout the v1.0.x series — they display a deprecation warning and then execute the equivalent new command automatically. **They will be removed in v1.1.0.** This guide gives you everything you need to update your scripts before that breaking change arrives.

### Why Was This Done?

Before consolidation, the top-level namespace was cluttered: `nself email`, `nself redis`, `nself mlflow`, `nself staging`, `nself org`, and 74 other commands all lived at the same level. Discoverability suffered — there was no way to know what commands existed without reading the full reference. The new structure groups commands by domain so that `nself service --help` reveals all optional service management in one place.

### Deprecation Timeline

| Version | Behavior |
|---------|----------|
| v0.9.x | Old commands work silently (no warnings) |
| **v1.0.0** | Old commands work **with deprecation warnings** (current) |
| **v1.1.0** | Old commands are **removed** — scripts will break |

---

## Quick Reference — Complete Migration Table

All deprecated commands and their replacements at a glance.

| Old Command | New Command | Category | Deprecated Since | Removal |
|-------------|-------------|----------|-----------------|---------|
| `nself admin` | `nself service admin` | Service | v0.9.6 | v1.1.0 |
| `nself admin-dev` | `nself service admin dev` | Service | v0.9.6 | v1.1.0 |
| `nself email` | `nself service email` | Service | v0.9.6 | v1.1.0 |
| `nself functions` | `nself service functions` | Service | v0.9.6 | v1.1.0 |
| `nself mlflow` | `nself service mlflow` | Service | v0.9.6 | v1.1.0 |
| `nself realtime` | `nself service realtime` | Service | v0.9.6 | v1.1.0 |
| `nself redis` | `nself service redis` | Service | v0.9.6 | v1.1.0 |
| `nself search` | `nself service search` | Service | v0.9.6 | v1.1.0 |
| `nself storage` | `nself service storage` | Service | v0.9.6 | v1.1.0 |
| `nself webhooks` | `nself service webhooks` | Service | v0.9.6 | v1.1.0 |
| `nself provision` | `nself deploy server provision` | Deploy | v0.9.6 | v1.1.0 |
| `nself server` | `nself deploy server` | Deploy | v0.9.6 | v1.1.0 |
| `nself servers` | `nself servers` / `nself deploy server list` | Deploy | v0.9.6 | v1.1.0 |
| `nself staging` | `nself deploy staging` | Deploy | v0.9.6 | v1.1.0 |
| `nself sync` | `nself deploy sync` | Deploy | v0.9.6 | v1.1.0 |
| `nself cloud` | `nself deploy cloud` | Deploy | v0.9.6 | v1.1.0 |
| `nself upgrade` | `nself update` | Update | v0.9.6 | v1.1.0 |
| `nself secrets` | `nself config secrets` | Config | v0.9.6 | v1.1.0 |
| `nself vault` | `nself config vault` | Config | v0.9.6 | v1.1.0 |
| `nself rate-limit` | `nself config rate-limit` | Config | v0.9.6 | v1.1.0 |
| `nself validate` | `nself doctor` | Diagnostics | v0.9.6 | v1.1.0 |
| `nself security` | `nself harden` | Security | v0.9.6 | v1.1.0 |
| `nself roles` | `nself auth roles` | Auth | v0.9.6 | v1.1.0 |
| `nself org` | `nself tenant` | Tenant | v0.9.6 | v1.1.0 |
| `nself up` | `nself start` | Alias | Legacy | v1.1.0 |
| `nself down` | `nself stop` | Alias | Legacy | v1.1.0 |

---

## Service Commands

Optional services previously had their own top-level commands. They are now all subcommands of `nself service`.

### Why the Change?

Running `nself --help` listed email, redis, mlflow, and other services as peers of `nself start` and `nself db`. Grouping them under `nself service` makes it clear which commands manage optional add-ons, and `nself service --help` becomes the single place to discover all available service management.

### Migration Map

| Old Command | New Command |
|-------------|-------------|
| `nself admin` | `nself service admin` |
| `nself admin-dev` | `nself service admin dev` |
| `nself email` | `nself service email` |
| `nself functions` | `nself service functions` |
| `nself mlflow` | `nself service mlflow` |
| `nself realtime` | `nself service realtime` |
| `nself redis` | `nself service redis` |
| `nself search` | `nself service search` |
| `nself storage` | `nself service storage` |
| `nself webhooks` | `nself service webhooks` |

All subcommands and flags are identical — only the prefix changes.

### Before and After Examples

**Email service:**

```bash
# Before (deprecated)
nself email test admin@example.com
nself email configure --provider sendgrid
nself email templates list

# After (v1.0+)
nself service email test admin@example.com
nself service email configure --provider sendgrid
nself service email templates list
```

**Redis:**

```bash
# Before (deprecated)
nself redis enable
nself redis status
nself redis cli

# After (v1.0+)
nself service redis enable
nself service redis status
nself service redis cli
```

**Object storage:**

```bash
# Before (deprecated)
nself storage enable
nself storage buckets list
nself storage buckets create uploads --public

# After (v1.0+)
nself service storage enable
nself service storage buckets list
nself service storage buckets create uploads --public
```

**Full-text search:**

```bash
# Before (deprecated)
nself search enable
nself search index --collection products

# After (v1.0+)
nself service search enable
nself service search index --collection products
```

**Serverless functions:**

```bash
# Before (deprecated)
nself functions deploy my-function
nself functions logs my-function --tail

# After (v1.0+)
nself service functions deploy my-function
nself service functions logs my-function --tail
```

**MLflow (ML experiment tracking):**

```bash
# Before (deprecated)
nself mlflow enable
nself mlflow status

# After (v1.0+)
nself service mlflow enable
nself service mlflow status
```

**Real-time (WebSocket/live queries):**

```bash
# Before (deprecated)
nself realtime enable
nself realtime status

# After (v1.0+)
nself service realtime enable
nself service realtime status
```

**Admin UI:**

```bash
# Before (deprecated)
nself admin enable
nself admin password --reset

# After (v1.0+)
nself service admin enable
nself service admin password --reset
```

**Admin UI dev mode:**

```bash
# Before (deprecated)
nself admin-dev

# After (v1.0+)
nself service admin dev
```

**Webhooks:**

```bash
# Before (deprecated)
nself webhooks list
nself webhooks create --event user.created --url https://api.example.com/hook

# After (v1.0+)
nself service webhooks list
nself service webhooks create --event user.created --url https://api.example.com/hook
```

---

## Deploy Commands

Deployment-related commands are now consolidated under `nself deploy`.

### Why the Change?

`nself staging`, `nself provision`, `nself server`, and `nself sync` were isolated commands with no clear relationship to one another in the command tree. Grouping them under `nself deploy` makes the full deployment surface visible in one place.

### Migration Map

| Old Command | New Command |
|-------------|-------------|
| `nself staging` | `nself deploy staging` |
| `nself provision` | `nself deploy server provision` |
| `nself server` | `nself deploy server` |
| `nself servers` | `nself deploy server list` |
| `nself sync` | `nself deploy sync` |
| `nself cloud` | `nself deploy cloud` |

### Before and After Examples

**Deploy to staging:**

```bash
# Before (deprecated — removed as of v0.9.6)
nself staging

# After (v1.0+)
nself deploy staging
```

**Server provisioning:**

```bash
# Before (deprecated)
nself provision --provider hetzner --size cx31

# After (v1.0+)
nself deploy server provision --provider hetzner --size cx31
```

**Server management:**

```bash
# Before (deprecated)
nself server status
nself server ssh myserver
nself server list

# After (v1.0+)
nself deploy server status
nself deploy server ssh myserver
nself deploy server list

# Listing servers — both of these also work:
nself servers                    # still valid (nself servers is a retained top-level command)
nself deploy server list         # canonical new form
```

**Sync configuration to remote:**

```bash
# Before (deprecated)
nself sync --env staging

# After (v1.0+)
nself deploy sync --env staging
```

**Cloud deployment:**

```bash
# Before (deprecated)
nself cloud deploy --provider hetzner

# After (v1.0+)
nself deploy cloud --provider hetzner
```

---

## Config Commands

Configuration management commands are now grouped under `nself config`.

### Why the Change?

`nself secrets`, `nself vault`, and `nself rate-limit` were top-level commands that logically belong to configuration management. Consolidating them under `nself config` means `nself config --help` shows all configuration-related operations in one place.

### Migration Map

| Old Command | New Command |
|-------------|-------------|
| `nself secrets` | `nself config secrets` |
| `nself vault` | `nself config vault` |
| `nself rate-limit` | `nself config rate-limit` |

### Before and After Examples

**Secrets management:**

```bash
# Before (deprecated)
nself secrets list
nself secrets set DATABASE_URL "postgres://..."
nself secrets delete OLD_KEY

# After (v1.0+)
nself config secrets list
nself config secrets set DATABASE_URL "postgres://..."
nself config secrets delete OLD_KEY
```

**Vault management:**

```bash
# Before (deprecated)
nself vault unlock
nself vault status

# After (v1.0+)
nself config vault unlock
nself config vault status
```

**Rate limiting configuration:**

```bash
# Before (deprecated)
nself rate-limit status
nself rate-limit configure --requests 100 --window 60s
nself rate-limit disable

# After (v1.0+)
nself config rate-limit status
nself config rate-limit configure --requests 100 --window 60s
nself config rate-limit disable
```

---

## Auth Commands

Authentication-adjacent commands that previously lived at the top level are now under `nself auth`.

### Why the Change?

`nself roles` managed role-based access control but had no obvious connection to `nself auth` for OAuth or MFA. Moving it under `nself auth` creates a single namespace for all authentication and authorization concerns.

### Migration Map

| Old Command | New Command |
|-------------|-------------|
| `nself roles` | `nself auth roles` |

### Before and After Examples

**Role management:**

```bash
# Before (deprecated)
nself roles list
nself roles create editor --permissions read,write
nself roles assign editor --user user@example.com
nself roles delete editor

# After (v1.0+)
nself auth roles list
nself auth roles create editor --permissions read,write
nself auth roles assign editor --user user@example.com
nself auth roles delete editor
```

---

## Tenant Commands

The `nself org` command was renamed to `nself tenant` to better reflect multi-tenancy as a first-class concept.

### Migration Map

| Old Command | New Command |
|-------------|-------------|
| `nself org` | `nself tenant` |

### Before and After Examples

```bash
# Before (deprecated)
nself org create acme --plan pro
nself org list
nself org switch acme
nself org billing status
nself org delete acme

# After (v1.0+)
nself tenant create acme --plan pro
nself tenant list
nself tenant switch acme
nself tenant billing status
nself tenant delete acme
```

---

## Diagnostics and Security Commands

Two commands were renamed to better reflect their purpose.

### Migration Map

| Old Command | New Command | Reason |
|-------------|-------------|--------|
| `nself validate` | `nself doctor` | Consolidated health checks and validation into one diagnostic command |
| `nself security` | `nself harden` | Renamed to clarify the command actively hardens the server, not just audits it |

### Before and After Examples

**Environment validation:**

```bash
# Before (deprecated)
nself validate
nself validate --config
nself validate --network

# After (v1.0+)
nself doctor
nself doctor --config
nself doctor --network
```

**Security hardening:**

```bash
# Before (deprecated)
nself security audit
nself security harden --level strict
nself security status

# After (v1.0+)
nself harden audit
nself harden --level strict
nself harden status
```

---

## Legacy Aliases

These aliases have existed since early versions of nself. They mirror Unix service management conventions but were never the canonical command names.

### Migration Map

| Old Command | New Command | Notes |
|-------------|-------------|-------|
| `nself up` | `nself start` | `nself up` has been an alias since v0.1.0 |
| `nself down` | `nself stop` | `nself down` has been an alias since v0.1.0 |

### Before and After Examples

```bash
# Before (legacy aliases — still work but will be removed in v1.1.0)
nself up
nself down

# After (canonical commands)
nself start
nself stop
```

---

## Update Command Rename

`nself upgrade` was renamed to `nself update` for naming consistency with the broader ecosystem.

### Migration Map

| Old Command | New Command |
|-------------|-------------|
| `nself upgrade` | `nself update` |

### Before and After Examples

```bash
# Before (deprecated)
nself upgrade
nself upgrade --check
nself upgrade --version 0.9.9

# After (v1.0+)
nself update
nself update --check
nself update --version 0.9.9
```

---

## CI/CD Update Guidance

If your CI/CD pipelines use deprecated commands, update them before v1.1.0 ships. Below are patterns for the most common pipeline tools.

### Scanning Your Scripts

Before updating, find all deprecated command usages across your project:

```bash
# Find all deprecated commands in shell scripts and CI configs
grep -rn \
  -e 'nself email\b' \
  -e 'nself redis\b' \
  -e 'nself storage\b' \
  -e 'nself search\b' \
  -e 'nself functions\b' \
  -e 'nself mlflow\b' \
  -e 'nself realtime\b' \
  -e 'nself admin\b' \
  -e 'nself admin-dev\b' \
  -e 'nself webhooks\b' \
  -e 'nself provision\b' \
  -e 'nself server\b' \
  -e 'nself servers\b' \
  -e 'nself staging\b' \
  -e 'nself sync\b' \
  -e 'nself cloud\b' \
  -e 'nself upgrade\b' \
  -e 'nself secrets\b' \
  -e 'nself vault\b' \
  -e 'nself rate-limit\b' \
  -e 'nself validate\b' \
  -e 'nself security\b' \
  -e 'nself roles\b' \
  -e 'nself org\b' \
  -e '\bnself up\b' \
  -e '\bnself down\b' \
  .github/ scripts/ deploy/ Makefile *.sh 2>/dev/null
```

### GitHub Actions

```yaml
# Before
- name: Deploy to staging
  run: |
    nself staging
    nself email configure --provider sendgrid
    nself secrets set APP_KEY "${{ secrets.APP_KEY }}"

# After
- name: Deploy to staging
  run: |
    nself deploy staging
    nself service email configure --provider sendgrid
    nself config secrets set APP_KEY "${{ secrets.APP_KEY }}"
```

### Shell Deploy Scripts

```bash
#!/usr/bin/env bash

# Before (deprecated)
nself validate
nself sync --env production
nself upgrade --check

# After (v1.0+)
nself doctor
nself deploy sync --env production
nself update --check
```

### Makefile

```makefile
# Before (deprecated)
.PHONY: deploy
deploy:
	nself staging
	nself email configure --provider sendgrid

# After (v1.0+)
.PHONY: deploy
deploy:
	nself deploy staging
	nself service email configure --provider sendgrid
```

### Docker Entrypoint Scripts

```bash
#!/usr/bin/env bash

# Before (deprecated)
nself up
nself storage enable
nself redis enable

# After (v1.0+)
nself start
nself service storage enable
nself service redis enable
```

### nself-chat / nself-demo `.backend/` Scripts

If your project uses nself as a backend (following the CLI-first pattern), update any wrapper scripts in your `.backend/` directory:

```bash
# Before (deprecated pattern)
cd .backend
nself up
nself secrets set NEXT_PUBLIC_NHOST_SUBDOMAIN "$SUBDOMAIN"

# After (v1.0+)
cd .backend
nself start
nself config secrets set NEXT_PUBLIC_NHOST_SUBDOMAIN "$SUBDOMAIN"
```

---

## FAQ

**Q: Will my scripts break immediately when I upgrade to nself v1.0.0?**

No. All deprecated commands continue to work in v1.0.0 — they emit a deprecation warning to stderr and then execute normally. Your pipelines and scripts will not break until v1.1.0.

**Q: What does the deprecation warning look like?**

```
⚠  DEPRECATED: 'nself email' is deprecated.
   Use: nself service email instead

[command output follows normally...]
```

The warning goes to stderr, so scripts that parse stdout output are unaffected.

**Q: Do the deprecated commands accept the same flags and arguments?**

Yes. Deprecated command wrappers forward all arguments verbatim to the new command. `nself email test admin@example.com` and `nself service email test admin@example.com` are functionally identical.

**Q: When exactly is v1.1.0 releasing?**

No date has been announced. Monitor the [Releases page](../releases/INDEX.md) and [Roadmap](../releases/ROADMAP.md) for announcements. Because v1.1.0 is a breaking change for anyone using deprecated commands, it will have a clear migration window.

**Q: I'm using `nself up` and `nself down` in dozens of scripts. Is there a quick way to update them all?**

```bash
# macOS / BSD sed
find . -name "*.sh" -exec sed -i '' 's/\bnself up\b/nself start/g; s/\bnself down\b/nself stop/g' {} +

# GNU sed (Linux)
find . -name "*.sh" -exec sed -i 's/\bnself up\b/nself start/g; s/\bnself down\b/nself stop/g' {} +
```

Always review the diff before committing — context matters, and some matches may require manual adjustment.

**Q: Can I use the new command structure on nself v0.9.x?**

Yes. The new command structure (`nself service email`, `nself deploy staging`, etc.) was introduced in v0.9.6. If you're on v0.9.6 or later, you can migrate your scripts now — both old and new forms will work until v1.1.0.

**Q: What happened to `nself provision`? The new command is `nself deploy server provision` — that's longer.**

The longer path reflects the command's place in the hierarchy: you are provisioning a server as part of a deployment operation. The additional words are the path through the command tree, and `nself deploy server --help` shows all server-related operations together. For frequent use, shell aliases are an option: `alias ns-provision='nself deploy server provision'`.

**Q: Is `nself servers` (plural) gone?**

`nself servers` remains available as a convenience alias pointing to `nself deploy server list`. It will also be removed in v1.1.0. The canonical form is `nself deploy server list`.

**Q: The deprecation warning is cluttering my CI logs. Can I suppress it?**

Use `--quiet` or `-q` to suppress all non-essential output including deprecation warnings:

```bash
nself email configure --provider sendgrid --quiet
```

Or silence stderr entirely (not recommended for production pipelines where warnings signal real issues):

```bash
nself email configure --provider sendgrid 2>/dev/null
```

**Q: Where can I see all available subcommands for a parent command?**

```bash
nself service --help        # lists all optional service subcommands
nself deploy --help         # lists all deployment subcommands
nself config --help         # lists all configuration subcommands
nself auth --help           # lists all auth and security subcommands
nself tenant --help         # lists all multi-tenancy subcommands
```

---

## Related Documentation

- [Command Tree v1.0](../commands/COMMAND-TREE-V1.md) — authoritative reference for all 31 top-level commands and 295+ subcommands
- [Command Consolidation Architecture](../architecture/COMMAND-CONSOLIDATION-MAP.md) — full mapping from 79 → 31 commands
- [Infrastructure Consolidation](INFRA-CONSOLIDATION.md) — migration guide for `infra` commands (`provider`, `k8s`, `helm`)
- [v1.0 Migration Status](V1-MIGRATION-STATUS.md) — documentation migration tracker
- [Changelog](../releases/CHANGELOG.md) — version history and breaking change log
- [Roadmap](../releases/ROADMAP.md) — upcoming releases and timeline

---

**[← Migration Index](INDEX.md)** | **[Command Tree →](../commands/COMMAND-TREE-V1.md)**
