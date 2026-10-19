# Microkernel Architecture

nSelf is built as a microkernel. A thin core ships with the CLI plus a minimum service set; everything else is a plugin. The model is borrowed from the Linux kernel and its loadable modules: the kernel guarantees a small set of invariants, and modules extend the surface area without touching kernel internals.

Plugins are the product. The core exists to load, validate, and run them.

## Overview

The kernel is the `nself` CLI binary plus the four required services (Postgres, Hasura, Auth, Nginx). Everything else is a plugin: messaging, AI, media, payments, search, observability, billing. Plugins ship as signed manifests. The CLI resolves, validates, and runs them through a single lifecycle.

This split keeps the core small enough to audit and stable enough to depend on, while letting features evolve at plugin pace. A plugin can ship in days; the kernel ships in months.

## Plugin Manifest Schema

Every plugin declares itself in a 12-field YAML manifest. The schema is defined in ticket G1-T30 and enforced by the loader.

| Field | Type | Notes |
|---|---|---|
| `name` | slug | Lowercase, hyphenated. Must be unique in the registry |
| `version` | semver | `MAJOR.MINOR.PATCH` |
| `tier` | enum | `free`, `basic`, or `pro` |
| `bundle` | enum | `nChat`, `nClaw`, `nFamily`, `nTV`, `ClawDE`, `ɳSelf+`, or `none` |
| `language` | enum | `go`, `rust`, `ts`, `py`, or `flutter` |
| `services` | array | Each entry: `{name, port, healthcheck, image}` |
| `tables` | array | `np_*`-prefixed Postgres table schemas |
| `hasura_metadata` | path or inline | Track-table, permission, and relationship metadata |
| `dependencies` | array | Other plugin slugs this plugin requires |
| `requires_license` | bool | True for any `tier: pro` plugin |
| `signature` | base64 | Ed25519 signature of the manifest body |
| `description` | string | One-line summary used by `nself plugin list` |

The `signature` field covers every other field. Any tampering invalidates the signature and the loader refuses to install.

## Plugin Lifecycle

A plugin moves through eight phases from request to running container.

1. **Discovery.** The CLI queries `plugins.nself.org` for a manifest by slug. The registry returns the manifest plus the signed checksum of any artifacts.
2. **Validation.** The loader verifies the Ed25519 signature against the ping_api public key. For `tier: pro` plugins, the loader checks the local license against the cached entitlement table.
3. **Resolution.** Dependencies are walked and topo-sorted. Version conflicts surface as install errors with the conflicting pair named.
4. **Download.** Artifacts pull from OCI for container images, or from npm, cargo, or pip for SDK code. Checksums are verified after download.
5. **Installation.** `nself plugin install <name>` writes plugin files into the project, updates the local plugin registry, and registers the plugin with the build system.
6. **Configuration.** Plugin env vars merge into the project env cascade. Ports are allocated from the reserved 38xx range when the plugin requires one.
7. **Build.** `nself build` reads installed manifests and emits matching entries in the generated `docker-compose.yml`. Manifests never bypass the build step.
8. **Runtime.** `nself start` brings services up. Healthchecks gate readiness. The loader continues to honor license revocation on a cached TTL during runtime.

## BIOS Invariants

The kernel guarantees five invariants. Plugins that violate any of them fail to load.

- **Table prefix.** Plugin tables must use the `np_*` prefix. The loader rejects any migration that creates a non-prefixed table.
- **Bind address.** Plugin services bind to `127.0.0.1`. External traffic enters through Nginx only.
- **Signature.** Licensed plugins must carry a valid Ed25519 signature. The CLI loader refuses unsigned or tampered licensed plugins, with no override.
- **License revocation.** License changes propagate within 24 hours through cached TTL. The system runs FAIL-OPEN for 7 days when ping.nself.org is unreachable, then fails closed.
- **Convention Wall.** Multi-app isolation uses `source_account_id TEXT`; Cloud multi-tenancy uses `tenant_id UUID`. The two columns are never mixed in the same table. See [Multi-Tenant Conventions](Multi-Tenant-Conventions).

These are non-negotiable. The doctor command (`nself doctor --deep`) checks each invariant on every install and on a daily scan.

## gRPC Deferral

The initial release uses HTTP plus JSON for all plugin-to-plugin and plugin-to-core communication. gRPC is a frequent question and the answer is the same one given for many binary-protocol decisions: not yet. We have no measured perf bottleneck that JSON-over-HTTP cannot meet, and gRPC adds a code-generation step, a protobuf schema registry, and a debugging surface that JSON does not.

gRPC is deferred to v1.2.0 or later, gated on a real perf measurement that justifies the move. Until then, plugins speak HTTP.

## See Also

- [Architecture](Architecture) — top-level system overview
- [Multi-Tenant Conventions](Multi-Tenant-Conventions) — `source_account_id` vs `tenant_id`
- [Plugin Development](Plugin-Development) — building a plugin from scratch
