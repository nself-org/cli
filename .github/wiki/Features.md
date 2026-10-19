# Features

## Contents

- [Core Stack](#core-stack)
- [Optional Services](#optional-services)
- [CLI](#cli)
- [Plugin System](#plugin-system)
- [Security](#security)
- [Feature Details](#feature-details)

ɳSelf ships a complete self-hosted backend stack with a full CLI, core services, optional services, and a plugin ecosystem of free and licensed plugins.

## Core Stack

| Feature | Status | Description |
|---------|--------|-------------|
| PostgreSQL | ✅ | Primary database, version-pinned |
| Hasura GraphQL | ✅ | Auto-generated GraphQL API + subscriptions |
| Auth (nHost) | ✅ | JWT, OAuth, magic links, MFA |
| Nginx | ✅ | Reverse proxy, TLS termination, security headers |

## Optional Services

| Feature | Status | Description |
|---------|--------|-------------|
| Redis | ✅ | Caching, sessions, pub/sub |
| MinIO | ✅ | S3-compatible object storage |
| Email | ✅ | SMTP + 16 provider integrations |
| Search | ✅ | MeiliSearch full-text search |
| Functions | ✅ | Serverless runtime |
| Admin UI | ✅ | Local GUI at localhost:3021 |

## CLI

| Feature | Status | Description |
|---------|--------|-------------|
| Top-level commands | ✅ | Full lifecycle management |
| Smart auto-build | ✅ | `nself start` builds if no compose file |
| Config management | ✅ | show / get / set / list / validate / export / import |
| Service toggles | ✅ | `nself service enable/disable` |
| Plugin proxy | ✅ | Unknown commands routed to plugin binaries |
| Migration (v1→v2) | ✅ | Detect, migrate, rollback |
| Self-update | ✅ | `nself update` with binary download |
| Doctor | ✅ | `nself doctor --fix` for auto-repair |

## Plugin System

| Feature | Status | Description |
|---------|--------|-------------|
| Free plugins | ✅ | MIT licensed, no key required |
| Licensed plugins | ✅ | Tier-gated, license key required |
| Compose overlay | ✅ | Plugins inject Docker services |
| Nginx injection | ✅ | Plugins add location blocks |
| Plugin config templating | ✅ | Env vars declared in manifest |

## Security

| Feature | Status | Description |
|---------|--------|-------------|
| Container hardening | ✅ | cap_drop ALL + no-new-privileges |
| Auth rate limiting | ✅ | 30r/m default, configurable |
| Input sanitisation | ✅ | All user inputs validated |
| Secret redaction | ✅ | Secrets never appear in output |
| Plugin integrity | ✅ | Manifest validation + checksum |
| File permissions | ✅ | .env 0600, backups 0700 |
| Security audit CLI | ✅ | `nself security audit/setup/status` (v1.0.3) |

## Feature Details

- [[Feature-Auth]], authentication and authorisation
- [[Feature-Storage]], object storage with MinIO
- [[Feature-Search]], full-text search with MeiliSearch
- [[Feature-Functions]], serverless runtime
- [[Feature-Email]], email delivery integrations
- [[Feature-Monitoring]], metrics, logs, and alerting
- [[Feature-Plugins]], plugin system overview

---
← [[Home]] | [[_Sidebar]]
