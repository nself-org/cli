# nself build

<!-- BEGIN PROSE:summary -->
> Generate `docker-compose.yml`, nginx configs, and SSL certificates from `.env`.
<!-- END PROSE:summary -->

## Synopsis

```
nself build [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself build` reads your `.env` cascade and generates all infrastructure configuration files: a `docker-compose.yml` with every enabled service, nginx reverse-proxy configs, and SSL certificates. It must be run after `nself init` and after any configuration change before restarting services.

The build pipeline loads configuration from `.env.dev` → `.env.{ENV}` → `.env.secrets` → `.env.local` → `.env`, merges plugin configurations from `~/.nself/plugins/`, and applies security validation (password strength, no wildcard CORS in production, port binding checks). The result is a single `docker-compose.yml` that includes core services, optional services, monitoring, custom services (CS_1–CS_10), and any installed plugins.

By default, `nself build` is smart-cached: it compares `.env` modification time against `docker-compose.yml` and skips regeneration when nothing has changed. Use `--force` to override the cache.

## Declared plugins (nself.yaml)

Since v1.2.2, `nself build` reads the project manifest (`nself.yaml` or `nself.yml`) and guarantees every declared plugin is wired into the generated stack. Two declaration shapes are accepted:

```yaml
# flat list
plugins:
  - cron
  - notify

# tiered map, plus bundle expansion
bundle: nsentry
plugins:
  free: [cron, notify]
  pro: [ai-gateway]
```

`bundle:` and `bundles:` expand through the canonical bundle catalog. For each declared plugin the build checks the plugin install directory, then core-service aliases (`auth`, `storage`), then attempts a best-effort auto-install from the registry (60 second timeout per plugin). Any plugin that still cannot be wired is reported with a per-plugin warning and a printed `nself plugin install ...` fix command. A declared plugin is never silently dropped.

| Env var | Default | Effect |
|---------|---------|--------|
| `NSELF_AUTO_INSTALL_PLUGINS` | `true` | Set `false` to disable auto-install (offline builds, hermetic CI) |
| `NSELF_PLUGIN_DIR` | `~/.nself/plugins` | Override the plugin install directory (per-project plugin sets, CI) |

Projects without a manifest keep the install-then-discover flow unchanged.

## Secret templating

Since v1.2.2, generated `docker-compose.yml` files contain no literal secret values. Secrets (Postgres password, Hasura admin secret, JWT configuration, MinIO credentials, SMTP password) are emitted as `${VAR}` references. The interpolation values are written to `.nself/compose.env` with mode 0600, and `nself start`/`stop`/`restart` pass it to docker compose via `--env-file`. Editing a secret in `.env` and re-running `nself build` updates the running stack on next start. Projects built before v1.2.2 keep default `.env` discovery until rebuilt.

## Unknown env var warnings

`nself build` warns on unrecognized variables in the `.env` cascade. Common app-owned variables (`NODE_ENV`, `JWT_SECRET`, `LOG_LEVEL`, `COOKIE_SECRET`, `SSL_AUTO_TRUST`, `ENABLE_DEBUG`, `NSELF_PROJECT_NAME`) are recognized and never warn. For project-specific variables, set `ENV_ALLOWLIST` to a comma-separated list of exact names or prefix patterns ending in `*`:

```bash
ENV_ALLOWLIST=MY_APP_TOKEN,FEATURE_*
```

## Redis auto-enable

`nself build` automatically includes a Redis service in `docker-compose.yml` when any installed plugin requires it, even if `REDIS_ENABLED` is not set in `.env`.

Plugins that trigger Redis auto-enable: `ai`, `claw`, `mux`, `cron`, `notify`.

When auto-enable fires, the build prints:

```
Note: Redis auto-enabled because a BullMQ-backed plugin (cron, notify, ai, claw, or mux) was detected.
```

To disable auto-enable, uninstall the plugin or explicitly set `REDIS_ENABLED=false` and confirm you do not need the plugin's queue features. To opt in explicitly and suppress the note, set `REDIS_ENABLED=true` in `.env`.

## v0.9 project detection

`nself build` scans for v0.9 project artifacts before generating any files. Two or more detected artifacts trigger a hard error pointing to the migration guide. A single artifact produces a non-blocking warning. Use `--no-migration-check` in automation (CI) where you are certain no v0.9 projects exist. Use `--allow-legacy` only as a temporary workaround while running `nself migrate`. See [[Upgrade-From-v0.9]].

## Docker Compose v5 compatibility

`nself build` generates compose files that are compatible with both Docker Compose v4 and v5.

Docker Compose v5 introduced a stricter parser: it treats the top-level `pids_limit` service field as a canonical alias for `deploy.resources.limits.pids`. When both are present, v5 rejects the compose file with:

```
services.<name>: can't set distinct values on 'pids_limit' and 'deploy.resources.limits.pids': invalid compose project
```

The generator uses the `deploy.resources.limits.pids` form exclusively. The `pids_limit` top-level field is never emitted. This form is also valid on Compose v3.4+ and v4, so generated files work across all current Docker Compose versions.

Each long-running service gets a default pids limit of 100 to prevent fork-bomb attacks. Services that need more (such as postgres under high concurrency) override this in their configuration.
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--allow-insecure` | `false` | Allow insecure config (dev only) |
| `--allow-legacy` | `false` | Bypass v0.9 artifact check and proceed with WARNING (not recommended) |
| `--check` | `false` | Validate only, don't build |
| `--debug` | `false` | Enable debug mode |
| `--force`, `-f` | `false` | Force rebuild all components |
| `--no-auto-redis` | `false` | Disable automatic Redis enablement when a BullMQ-backed plugin is detected |
| `--no-cache` | `false` | Disable build cache |
| `--no-migration-check` | `false` | Skip v1 artifact detection (for automation/CI) |
| `--no-monorepo` | `false` | Disable automatic monorepo backend detection |
| `--profile` | `""` | Service profile: curated subset of services to include in docker-compose.yml.   app (default) — full service set, identical to pre-profile behaviour.   ops           — observability + CI server: postgres, hasura, auth, nginx,                   monitoring stack; excludes minio, mailpit, admin, functions, search. Overrides NSELF_PROFILE env var. Valid values: app, ops. |
| `--quiet`, `-q` | `false` | Suppress non-error output (for CI use) |
| `--remove-orphans` | `false` | Remove containers with no matching service in the freshly generated compose (G-014). Detection always runs; removal is opt-in. |
| `--security-report` | `false` | Generate security analysis |
| `--verbose`, `-v` | `false` | Show environment cascade |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Standard build
nself build

# Validate config only, don't write files
nself build --check

# Force rebuild everything, ignoring cache
nself build --force

# CI mode — quiet output
nself build -q

# Show the environment cascade as it loads
nself build --verbose

# Generate a security analysis report
nself build --security-report

# Rebuild for a specific environment
nself build --force --verbose
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
