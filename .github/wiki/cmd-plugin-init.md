# nself plugin init

> Scaffold a new plugin project with all required files.

## Synopsis

```
nself plugin init <name> [flags]
nself plugin scaffold <name> [flags]   # alias
nself plugin new <name> [flags]        # deprecated — use init instead
```

## Description

`nself plugin init` creates a new plugin project directory containing a canonical scaffold that mirrors the output of the standalone `new-plugin` binary shipped with `plugin-sdk-go`. Both entry points call the same `scaffold.Run` function so plugin authors get identical output regardless of which tool they use.

The scaffold includes a `plugin.json` manifest, Docker artifacts, a CI workflow, a hot-reload config, and language-specific source files. Run `nself plugin link <dir>` after scaffolding to mount the plugin for local development.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--template` | `go` | Plugin language: `go`, `rust`, `node`, `static` |
| `--tier` | `free` | Plugin tier: `free` or `pro` |
| `--bundle` | `""` | Bundle display name (licensed plugins only, e.g. `nClaw`) |
| `--description` | auto | Short description of the plugin |
| `--author` | `""` | Plugin author name |
| `--category` | `custom` | Plugin category |
| `--min-cli` | `1.0.9` | Minimum nSelf CLI version required |
| `--min-sdk` | `0.1.0` | Minimum plugin-sdk-go version required |
| `--port` | `8080` | Default listen port |
| `--out` | `./<name>` | Output directory |
| `--force` | false | Overwrite files if the output directory already exists |
| `--tenancy` | `""` | Multi-tenancy mode (see below); skips interactive prompt when set |
| `--no-interactive` | false | Skip interactive prompt; defaults to `none` |
| `--help`, `-h` | — | Show help |

## Multi-tenancy (--tenancy)

When `--tenancy` is omitted and stdin is a terminal, `plugin init` runs an interactive prompt to determine which isolation pattern to scaffold. Pass `--tenancy` to skip the prompt. Pass `--no-interactive` to skip the prompt and default to `none`.

| Value | Column emitted | Use when |
|-------|---------------|----------|
| `none` | (none) | This plugin stores no per-user Postgres data |
| `app-isolation` | `source_account_id TEXT NOT NULL DEFAULT 'primary'` | Separating independent consumer apps within one nSelf deploy |
| `cloud-tenant` | `tenant_id UUID` + Hasura row filter | Separating paying customers in nSelf Cloud SaaS |
| `both` | Both columns | Unsure — remove the unused column before going to production |

These conventions follow the Multi-Tenant Convention Wall documented in `docs/architecture/multi-tenant-conventions.md`. Never use `source_account_id` for Cloud customer isolation, and never use `tenant_id` for per-app isolation within one deploy.

## Generated files

| File | Purpose |
|------|---------|
| `plugin.json` | Canonical manifest (`multiApp` section reflects tenancy choice) |
| `migrations/001_init.sql` | Migration stub with correct columns + RLS policy |
| `hasura/metadata.json` | Hasura permission stub (row filter for `cloud-tenant` mode) |
| `Dockerfile` | Multi-stage build for the chosen language |
| `docker-compose.plugin.yml` | Compose overlay merged by `nself build` |
| `.air.toml` | Hot-reload config for `nself plugin dev` |
| `README.md` | Plugin README |
| `.github/workflows/ci.yml` | GitHub Actions CI workflow |
| `cmd/main.go` | Plugin entrypoint (Go only) |
| `internal/config/config.go` | Env-var config loader (Go only) |
| `internal/server/server.go` | HTTP router wired with SDK (Go only) |
| `internal/server/server_test.go` | Starter tests (Go only) |

## Examples

```bash
# Go plugin with interactive tenancy prompt
nself plugin init my-plugin

# Go plugin with explicit tenancy (skip prompt)
nself plugin init my-plugin --tenancy app-isolation

# Pro Go plugin in the nClaw bundle
nself plugin init my-plugin --tier pro --bundle nClaw

# Rust plugin
nself plugin init my-plugin --template rust

# Node.js plugin
nself plugin init my-plugin --template node

# Cloud multi-tenant plugin (tenant_id + Hasura row filter)
nself plugin init my-plugin --tenancy cloud-tenant

# Scaffold alias (identical behavior)
nself plugin scaffold my-plugin --tenancy none

# Non-interactive (CI / scripted)
nself plugin init my-plugin --no-interactive
```

## After scaffolding

```bash
cd my-plugin
go mod tidy && go test ./...
nself plugin link my-plugin    # mount for development
nself plugin dev my-plugin     # start hot-reload watcher
```

## See also

- [[cmd-plugin]], plugin management reference
- [[Plugin-Dev-Guide]], local development workflow
- [[Plugin-Licensing]], license tiers and key format

← [[cmd-plugin]] | [[Home]] →
