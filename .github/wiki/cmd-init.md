# nself init

<!-- BEGIN PROSE:summary -->
> Initialize a new ɳSelf project. It asks no questions: `nself init && nself start` works with zero prompts.
<!-- END PROSE:summary -->

## Synopsis

```
nself init [flags]
```

## Description

<!-- BEGIN PROSE:description -->
`nself init` launches an interactive setup wizard that creates a pristine `.env` configuration for a new ɳSelf project. It uses the project directory name and the default base domain `local.nself.org` (override with `--name` and `--domain`), then auto-generates cryptographically secure secrets (Postgres password, Hasura admin secret, JWT key).

You can choose which optional services to enable during init (Redis, MinIO, MeiliSearch, Mailpit, Monitoring). Each selection updates the generated `.env` accordingly. All values are validated before being written, domain format, password strength, and required fields are all checked.

After `nself init` completes, run `nself build` to generate `docker-compose.yml` and nginx configs, then `nself start` to boot the stack.

## Derived project and database names

`init` takes the project name from the current directory name and writes it to
`PROJECT_NAME`. It also derives `POSTGRES_DB` from that name, because a
database name has to be a valid SQL identifier and a directory name frequently
is not.

The derivation lowercases the name, folds `-`, `.` and space to `_`, drops
anything else, prefixes a leading digit with `_`, and truncates to Postgres's
63-byte identifier limit. So `my-app` yields database `my_app`, while
`PROJECT_NAME` keeps its hyphens for Docker compatibility. The two values are
allowed to differ and normally do.

Set `POSTGRES_DB` yourself in `.env` if you want a specific name. A directory
name that reduces to nothing usable (`---`, for example) is rejected at init
rather than at first start.

## Clone Templates

Clone templates are full-stack app starters embedded in the CLI binary. They include a Postgres schema with RLS policies, Hasura metadata, seed data, and a Flutter UI starter. No network access is needed to scaffold them.

### Available clone templates

| Template | Tables | Required plugins |
|---|---|---|
| `airbnb-clone` | `np_listings`, `np_bookings`, `np_reviews` | `auth`, `notify`, `photos` |
| `discord-clone` | `np_servers`, `np_channels`, `np_messages`, `np_members`, `np_roles` | `chat`, `realtime`, `auth`, `moderation` |
| `notion-clone` | `np_workspaces`, `np_pages`, `np_blocks`, `np_page_members` | `cms`, `auth`, `realtime` |
| `slack-clone` | `np_workspaces`, `np_channels`, `np_messages`, `np_threads`, `np_reactions` | `chat`, `livekit`, `realtime`, `auth`, `notify` |
| `substack-clone` | `np_newsletters`, `np_posts`, `np_subscribers`, `np_tiers` | `cms`, `notify`, `auth` |
| `zoom-clone` | `np_meetings`, `np_participants`, `np_recordings` | `livekit`, `recording`, `auth`, `notify` |

### Usage

```bash
# Scaffold a clone template into the current directory
nself init --template airbnb-clone

# Scaffold into a named directory
nself init --template discord-clone ./my-discord

# Skip seed data (schema only)
nself init --template notion-clone --no-seed

# Preview files without writing
nself init --template slack-clone --dry-run

# Scaffold and immediately install required plugins
nself init --template substack-clone ./my-newsletter
cd my-newsletter
nself plugin install cms notify auth
nself start
nself db migrate
nself hasura metadata apply --file hasura/metadata.json
```

### What gets scaffolded

Each clone template writes:

```
<dest>/
  migrations/
    001_schema.sql   — CREATE TABLE np_* statements with RLS policies
    002_seed.sql     — demo data (omitted with --no-seed)
  hasura/
    metadata.json    — Hasura permissions and relationships
  flutter/
    pubspec.yaml
    lib/main.dart    — starter app with GraphQL client and skeleton screens
  nself.yml          — project config with required plugins list
  .env.example       — environment variable template
  README.md          — getting started guide
```

Use `nself template list` to see all bundled and marketplace templates together.

## Telemetry

`nself init` emits a single anonymous `init_complete` event when `NSELF_TELEMETRY_OPT_IN=1` is set. The event is sent in a background goroutine with a 1-second timeout and is silently dropped on failure.

**Opt-in only.** No data is collected unless you explicitly set the environment variable. Absence of the variable means zero network calls.

Fields collected:

| Field | Type | Example | Notes |
|---|---|---|---|
| `wizard_mode` | string | `fast`, `wizard`, `demo`, `non-interactive`, `default` | Which init mode was used |
| `duration_ms` | integer | `312` | Time from command start to completion |
| `success` | bool | `true` | Whether init completed without error |
| `err_category` | string | `docker-not-found` | Error bucket; one of `timeout`, `permission-denied`, `docker-not-found`, `other` |
| `install_source` | string | `hn` | Source tag from `?ref=` in install.sh (nullable) |
| `install_method` | string | `brew` | How nself was installed: `brew`, `curl`, `docker`, `other` (nullable) |
| `app_context` | string | `nclaw` | Which Type-C app initiated this install (nullable) |

**No PII is collected.** Email addresses, file paths, project names, and license keys are never included. All values are categorical or numeric.

To opt in:

```bash
export NSELF_TELEMETRY_OPT_IN=1
nself init --fast
```
<!-- END PROSE:description -->

## Flags

<!-- BEGIN GENERATED:flags -->
| Flag | Default | Description |
|------|---------|-------------|
| `--cs-template` | `""` | Scaffold a custom service at init time: specify language (go, node, python, rust, other) |
| `--demo` | `false` | Auto-configure with all services enabled |
| `--domain` | `""` | Base domain (default `local.nself.org`; `init` never shows a domain menu. Use e.g. `myapp.dev`, `localhost` or `127.0.0.1.nip.io`) |
| `--dry-run` | `false` | Print files that would be written without writing them (clone templates only) |
| `--fast` | `false` | Skip advanced options, use smart defaults |
| `--force` | `false` | Overwrite existing configuration |
| `--full` | `false` | Create all environment files (.env.dev, .env.staging, .env.prod, .env.secrets) |
| `--interactive` | `false` | Explicitly enable interactive wizard |
| `--list-presets` | `false` | List all available project presets and exit |
| `--name` | `""` | Project name (sets PROJECT_NAME in generated .env) |
| `--no-pgvector` | `false` | Skip pgvector extension and RAG scaffold tables (sets PGVECTOR_ENABLED=false) |
| `--no-seed` | `false` | Skip seed data when scaffolding a clone template |
| `--non-interactive` | `false` | Use all defaults without prompts |
| `--preset` | `""` | Use a project-type preset: b2b-saas, mobile-backend, ai-assistant, community-forum, media-hosting, dev, sentry, nclaw-app |
| `--profile` | `""` | Resource profile: 'tiny' for small VPS (Postgres+nginx only) |
| `--quiet` | `false` | Suppress output messages |
| `--skip-validation` | `false` | Skip configuration validation |
| `--template` | `""` | Use a built-in, clone, or marketplace template (e.g. airbnb-clone, go, rust) |
| `--wizard` | `false` | Run the full 10-step interactive wizard |
| `--help`, `-h` | — | Show help |
<!-- END GENERATED:flags -->

## Examples

<!-- BEGIN PROSE:examples -->
```bash
# Minimal interactive setup
nself init

# Full 10-step wizard
nself init --wizard

# All services enabled (demo/evaluation)
nself init --demo

# Create all env files at once
nself init --full

# Smart defaults, no prompts
nself init --fast

# Non-interactive — all defaults, safe for CI
nself init --non-interactive

# Small VPS (512 MB–1 GB RAM): start with Postgres + nginx only
nself init --profile=tiny

# Skip prompts by supplying name and domain directly
nself init --name myapp --domain myapp.dev

# Start from a Go project template
nself init --template go --name myapi --domain myapi.local
```
<!-- END PROSE:examples -->

## See Also

<!-- BEGIN PROSE:see-also -->
- [[Commands]] — full command index
- [[Core-Services]] — what a stack is made of
<!-- END PROSE:see-also -->

← [[Commands]] | [[Home]] →
