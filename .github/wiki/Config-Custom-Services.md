# Config, Custom Services

Custom services are user-defined Docker containers that ɳSelf manages alongside the core stack. They live in numbered slots, `CS_1` through `CS_10`, and are treated as first-class citizens: ɳSelf generates their `docker-compose` config, injects core service credentials, routes them through Nginx, and exposes them in `nself status` and `nself logs`.

Use custom services to run any application your project needs, a REST API, a background worker, a webhook processor, an AI inference service, without leaving the ɳSelf config system.

---

## Overview

```
nSelf stack
  ├── postgres, hasura, auth, nginx  (core services — always present)
  ├── redis, minio, search, ...      (optional services — toggled by *_ENABLED)
  └── CS_1 .. CS_10                  (custom services — defined in .env)
        ├── any Docker image or nself template
        ├── auto-receives DATABASE_URL, HASURA_GRAPHQL_ADMIN_SECRET
        ├── optionally exposed via Nginx at {CS_N_ROUTE}.{BASE_DOMAIN}
        └── managed by nself start / stop / logs / status
```

Up to **10 custom service slots** are available per project. Slots are independent, you can use `CS_1` and `CS_3` without defining `CS_2`.

---

## Defining a Custom Service

### Option 1, Shorthand (CS_N)

The `CS_N` variable accepts a single definition string that encodes the most common settings:

```
CS_N=name:template[:port][:route]
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Service name. Used in container labels, logs, and Nginx config. |
| `template` | Yes | Language/framework template. See [Language Templates](#language-templates). |
| `port` | No | Port the service listens on. Auto-assigned if omitted. |
| `route` | No | Nginx subdomain. If omitted, the service is internal-only. |

**Example, a Go service on port 8001 exposed at `ping.{BASE_DOMAIN}`:**

```bash
# .env
CS_1=ping_api:go:8001:ping
```

ɳSelf parses this into the service configuration. The Compose service name is sanitized: `ping_api` becomes `ping-api`. Use that name for logs and internal DNS. The default build context prefers the original directory `services/ping_api/` when it exists.

### Per-Service Overrides

Keep the `CS_N` definition and add supported overrides. `CS_N_PORT` and `CS_N_ROUTE` override the shorthand fields. An empty or absent `CS_N` skips the slot entirely, even if overrides are present. There are no `CS_N_ENABLED`, `CS_N_NAME`, `CS_N_TEMPLATE`, `CS_N_WORKDIR`, or `CS_N_REPLICAS` overrides. Set the working directory in the image's Dockerfile.

```bash
# .env
CS_1=ping_api:go:8001:ping   # base definition
CS_1_MEMORY=512m             # override the default 256m
CS_1_CPU=1.0                 # override the default 0.5
CS_1_PUBLIC=true             # expose via Nginx
```

---

## Environment Variable Reference

`N` is the slot number (1–10). The name and template come from the required `CS_N` definition; the supported overrides are listed below.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CS_N` | string | *(empty)* | Shorthand definition. Format: `name:template[:port][:route]` |
| `CS_N_PORT` | int | parsed from CS_N or auto | Port the service listens on inside the Docker network |
| `CS_N_ROUTE` | string | parsed from CS_N | Nginx subdomain , set this to expose the service externally |
| `CS_N_PUBLIC` | bool | `true` when shorthand includes a route, otherwise `false` | With a nonempty route, enables Nginx routing at `{CS_N_ROUTE}.{BASE_DOMAIN}`. Set explicitly when adding a route only through `CS_N_ROUTE`. |
| `CS_N_MEMORY` | string | `256m` | Docker memory limit |
| `CS_N_CPU` | string | `0.5` | Docker CPU limit (fractional cores) |
| `CS_N_HEALTHCHECK` | string | `/health` | Healthcheck override. A path (e.g. `/auth/health`) probes that path instead of `/health` on the service's own port. A full `CMD ...` / `CMD-SHELL ...` command is passed through verbatim (split on whitespace) for services that need curl, a non-HTTP probe, or a different port. `disabled` / `none` / `false` omits the healthcheck entirely. |
| `CS_N_TABLE_PREFIX` | string | *(empty)* | Injected into the container as `TABLE_PREFIX`; the application decides how to use it |
| `CS_N_ENV_PASSTHROUGH` | string | *(empty)* | Comma-separated allowlist of project `.env` var names to forward into this container in addition to the fixed core set. `CS_N_ENV` still wins on a name conflict. |
| `CS_N_ENV_FILE` | string | *(empty)* | Project-relative path to a dotenv-format file whose `KEY=VALUE` lines are injected into this container, or a comma-separated list of such files (`.env.dev,.env.secrets`). Files load in order and a later file wins on a repeated key. Applied after `CS_N_ENV_PASSTHROUGH`, before `CS_N_ENV`. Use this instead of `CS_N_ENV` when a value itself contains a comma (e.g. some SMTP passwords) or when there are too many vars for one line. A missing file fails `nself build` rather than silently starting the service without those vars. Each entry must be relative and must not contain `..`. |
| `CS_N_ENV` | string | *(empty)* | Additional env vars to inject, in `KEY=VALUE,KEY=VALUE` format. Always applied last — overrides the fixed core set, `CS_N_ENV_PASSTHROUGH`, and `CS_N_ENV_FILE`. |
| `CS_N_IMAGE` | string | *(empty)* | Run a pre-built image instead of building from a Dockerfile — e.g. `docker.io/pgsty/minio:RELEASE.2026-08-04T00-00-00Z@sha256:...` to pin an exact digest. Mutually exclusive with `CS_N_PATH`; when set, no `build:` block is emitted at all. |
| `CS_N_VOLUMES` | string | *(empty)* | Comma-separated extra bind mounts in `host:container[:mode]` form, e.g. `./email-templates:/app/templates:ro`. Appended to the service's generated volume list. See [Host binds](#host-binds-and-the-v15-rule). |
| `CS_N_PATH` | string | `./services/<name>` | Build context. A path inside the project, or an **ancestor** of the project written as `..` segments only (e.g. `../..` for a monorepo root). See [Ancestor build context](#ancestor-build-context). Mutually exclusive with `CS_N_IMAGE`. |
| `CS_N_DOCKERFILE` | string | `Dockerfile` | Dockerfile path relative to the build context (for example `backend/services/api/Dockerfile` when the context is the monorepo root). No `..`, not absolute. Cannot be combined with `CS_N_IMAGE`. |
| `CS_N_BUILD_TARGET` | string | *(empty)* | Multi-stage build target, emitted as `build.target`. Must match `^[A-Za-z0-9_.-]{1,64}$`. Cannot be combined with `CS_N_IMAGE`. |
| `CS_N_COMMAND` | string | image default | Container command in exec form. The value is split on whitespace: no shell, no quoting, no variable expansion. `node dist/server.js` becomes `["node", "dist/server.js"]`. |
| `CS_N_DEPENDS_ON` | string | *(empty)* | Comma-separated `name[:started\|healthy\|completed]`. Adds compose `depends_on` entries next to the built-in `postgres: service_healthy`. `healthy` (the default) is `service_healthy`, `started` is `service_started`, `completed` is `service_completed_successfully`. A name must be a core service, another custom service or a service of an installed plugin; otherwise `nself build` fails with E500. |
| `CS_N_NETWORKS` | string | *(empty)* | Comma-separated extra networks, each named `<PROJECT_NAME>_<name>` (lowercase letters, digits, `-`, `_`). They are emitted as external networks and attached next to the project network, so they must already exist (`docker network create`). Any other name fails with E501: a custom service never joins another project's network. |

All `CS_*` variables are automatically exempt from "unknown env var" warnings.

### Ancestor build context

`CS_N_PATH` may name an ancestor of the project directory so a service can build from a monorepo root:

```bash
# .env, project at <repo>/apps/api, repository root two levels up
CS_1=worker:node:9500
CS_1_PATH=../..
CS_1_DOCKERFILE=apps/api/services/worker/Dockerfile
CS_1_BUILD_TARGET=runtime
```

The whole context directory is sent to the image builder, so it is bounded:

- It may reach at most the nearest ancestor of the project directory that holds `.git`. When no ancestor holds `.git`, the project directory is the limit.
- The ignore file BuildKit will use (a `<Dockerfile>.dockerignore` beside the Dockerfile if one exists, otherwise `.dockerignore` in the context) must exclude `.env*` and `.secrets` at every depth: use the lines `**/.env*` and `**/.secrets`. A bare `.env*` is not enough, because Docker matches it at the context root only and `apps/api/.env` would still be sent. `nself build` checks this by matching probe paths (root, each directory down to the project, and below it) with Docker's pattern rules.
- A `!` line that can re-include a `.env*` or `.secrets` path (for example `!**/.env.production` or `!**/.secrets/pub`) is refused too. Only a name ending `.example`, `.sample`, `.template` or `.dist` may be re-included. A leading UTF-8 byte order mark is ignored, as Docker does.
- Anything else fails `nself build` with **E528**, naming the directory.
- A path that climbs and then descends (`../sibling`) is not an ancestor and is rejected at config load, naming `CS_N_PATH`.

### Where the checks run

The ancestor-context bound, the ignore-file check and the Dockerfile symlink check run in `nself build` (compose generation). Config load checks syntax only, so a tool that loads the config without building does not enforce them. A `CS_N_PATH` inside the project that is itself a symlink leaving the project is not checked.

### Dependencies and networks

`CS_N_DEPENDS_ON` is syntax-checked when the compose file is generated and resolved during build post-validation, after the plugin step has written `.nself/compose-files.txt`. A first `nself build` therefore already sees every plugin service. An unknown name fails with E500 naming `CS_N_DEPENDS_ON` and the name. An unknown condition fails with E500 naming the condition. A dependency cycle fails with E500 naming the path (`api -> db2 -> api`): custom-service cycles when the compose file is generated, cycles through core or plugin services in build post-validation.

The network prefix is a naming rule, not ownership. Another project whose name extends yours (`app` and `app_staging`) also owns names that start with `app_`, so choose network names no other project uses.

Extra networks are external: `nself build` declares them but does not create them. For `PROJECT_NAME=myapp` and `CS_1_NETWORKS=myapp_shared`, create the network before starting the stack:

```bash
docker network create myapp_shared
```

### Host binds and the v1.5 rule

`CS_N_VOLUMES` keeps its v1.4 rules. In v1.5 mode (`NSELF_V15=1`, see [[Compat-V15]]) these entries fail with **E502**:

- a writable absolute host path outside the project directory (add `:ro`, use a named volume or a project-relative path);
- `/var/run/docker.sock` or `/` in any mode suffix.

In v1.4 mode the Docker socket and host root binds still work and print one warning. Named volumes, project-relative binds and read-only outside binds pass in both modes.

### Not covered

These stay outside `CS_N`; see [[Custom-Services-Migration]] for the pattern table and the open gaps. There is no key that includes hand-written compose.


---

## Env Var Injection

When a custom service container starts, ɳSelf automatically injects the following variables:

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `postgresql://user:pass@postgres:5432/db` | Full PostgreSQL connection string |
| `HASURA_GRAPHQL_ADMIN_SECRET` | from project `.env` | For making privileged Hasura API calls |
| `PROJECT_NAME` | from project `.env` | The project namespace |
| `BASE_DOMAIN` | from project `.env` | The root domain |
| `ENV` | from project `.env` | `dev`, `staging`, or `prod` |
| `POSTGRES_HOST`, `POSTGRES_PORT` | `postgres`, `5432` | Internal database address |
| `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` | resolved Postgres config | Database credentials |
| `HASURA_GRAPHQL_ENDPOINT` | `http://hasura:8080/v1/graphql` | Internal GraphQL endpoint |
| `AUTH_SERVER_URL`, `AUTH_CLIENT_URL` | resolved Auth config | Internal Auth server and client URLs |
| `SERVICE_NAME`, `SERVICE_PORT`, `SERVICE_ROUTE` | parsed service configuration | This service's identity |
| `TABLE_PREFIX` | `CS_N_TABLE_PREFIX` | Application table-prefix hint |
| `REDIS_URL` | resolved Redis config | Included when Redis is enabled |
| `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET` | active object-store config | Included when storage is enabled; v1.5 ops profile omits these |
| `CS_N_ENV` values | parsed from `CS_N_ENV` | Any additional vars you define for this slot |

The generated environment consists of that core set plus explicit overrides. Each service's surface
area is explicit and auditable rather than silently coupled to whatever
another part of the stack happens to define. To pull in a specific project
`.env` var, name it in `CS_N_ENV_PASSTHROUGH`; to define a new one outright,
use `CS_N_ENV`.

**Example, reading injected vars in a Go service:**

```go
dbURL := os.Getenv("DATABASE_URL")            // injected by nSelf
adminSecret := os.Getenv("HASURA_GRAPHQL_ADMIN_SECRET") // injected by nSelf
apiKey := os.Getenv("MY_API_KEY")             // forwarded via CS_N_ENV_PASSTHROUGH
```

### Forwarding Project Vars

Use `CS_N_ENV_PASSTHROUGH` to forward specific vars that already exist in the
project's `.env` (a comma-separated allowlist of names, values are not
repeated):

```bash
# .env
MY_API_KEY=sk_live_...
CS_2_ENV_PASSTHROUGH=MY_API_KEY,STRIPE_WEBHOOK_SECRET
```

### Adding Extra Vars

Use `CS_N_ENV` to pass service-specific variables that aren't in the main `.env`,
or to override a value that was forwarded via `CS_N_ENV_PASSTHROUGH`
(`CS_N_ENV` always wins on a name conflict):

```bash
# .env
CS_2_ENV=QUEUE_SIZE=100,WORKER_TIMEOUT=30,LOG_FORMAT=json
```

---

## Language Templates

`nself service add <name> --template <lang>` writes starter source files and a Dockerfile, then registers the next free slot in `.env.dev`.

```bash
nself service add my-api --template go # doc-check: skip registered in cmd/commands/service.go; no command-signature fixture yet
```

**Available templates:**

| Template | Description |
|----------|-----------|
| `go` (default) | Go HTTP service |
| `node` | Node.js service |
| `python` | Python service |
| `static` | Static file server |
| `rust` | Rust service |
| `other` | Minimal scaffold for anything else |

The Go, Node, Python, and Rust scaffolds expose `/healthz`; the default generated healthcheck probes `/health` with `wget`. Set `CS_N_HEALTHCHECK=/healthz` for those apps if the final image supplies `wget`, or supply a suitable `CMD`/`CMD-SHELL` probe. Static and `other` scaffolds need a probe appropriate to their application. Adapt the starter image and probe before relying on health-based dependencies.

---

## Real-World Example, ping_api (web/backend CS_1)

The ɳSelf infrastructure itself (`web/backend`) uses `CS_1` to run **ping_api**, a Go service on port 8001 that handles telemetry and license validation for the CLI. It is accessible at `ping.nself.org`.

```bash
# web/backend/.env
CS_1=ping_api:go:8001:ping
CS_1_PUBLIC=true
CS_1_MEMORY=128m
CS_1_CPU=0.25
CS_1_HEALTHCHECK=/health
```

This configuration produces:

| Derived value | Result |
|---------------|--------|
| Compose service name | `ping-api` |
| Template | `go` |
| Internal port | `8001` |
| Route | `ping` |
| External URL | `https://ping.nself.org` |
| Internal URL | `http://ping-api:8001` (within Docker network) |

The ping_api service receives `DATABASE_URL` and `HASURA_GRAPHQL_ADMIN_SECRET` automatically on startup, so it can validate license keys against the database without any additional configuration.

---

## Quick Start

**1. Create a service using a template:**

```bash
nself service add my-api --template go # doc-check: skip registered in cmd/commands/service.go; no command-signature fixture yet
```

This scaffolds a Go service in `./services/my-api/` with a Dockerfile and `/healthz` endpoint, and assigns it the next available `CS_N` slot and port automatically.

**2. Configure the assigned slot in `.env.dev`:**

The command already registers the service. If it assigned `CS_1`, add a route and a healthcheck suitable for your final image; keep the registered name:

```bash
# .env.dev
CS_1=my-api:go:8001:api
CS_1_MEMORY=256m
# Supply a probe tool in the image, or use an application-specific probe.
CS_1_HEALTHCHECK=/healthz
```

**3. Rebuild and start:**

```bash
nself build     # regenerates docker-compose.yml with CS_1 included
nself start     # brings up the full stack including my-api
```

**4. Verify it's running:**

```bash
nself status    # shows all services including custom ones
nself logs -f -S my-api  # follow logs by Compose service name
```

Once the application is running and routing is configured, its route is `https://api.{BASE_DOMAIN}` and its internal address is `http://my-api:8001`.

---

## Multiple Custom Services

You can define up to 10 slots simultaneously. Slots are independent, gaps are fine.

```bash
# .env
CS_1=ping_api:go:8001:ping           # telemetry service
CS_2=worker:node:8002                 # background job worker (internal-only, no route)
CS_3=cms_api:python:8003:cms          # FastAPI CMS
CS_5=search_proxy:go:8005:search      # search proxy (skipped CS_4 intentionally)

# Override resources on the worker
CS_2_MEMORY=512m
CS_2_CPU=1.0
```

---

## Notes

- Custom service images are built from the scaffolded Dockerfile in `./services/{name}/`. To use a pre-built image instead, set `CS_N_IMAGE` directly — a full image reference, optionally digest-pinned with `@sha256:...`.
- `CS_N_TABLE_PREFIX` supplies the container's `TABLE_PREFIX` environment variable. It does not configure CLI migration or backup filtering.
- If a service's health endpoint isn't `/health` on its own port (e.g. an auth service serving `/auth/health`), set `CS_N_HEALTHCHECK=/auth/health` — otherwise Docker probes the wrong path and reports the service unhealthy forever regardless of its actual state.
- `nself logs` includes all Compose services by default. `--all` increases the tail to 100 lines; select a custom service by its Compose name, not its `CS_N` slot.
- `CS_N_IMAGE`, `CS_N_ENV_FILE`, and `CS_N_VOLUMES` cover the cases that previously forced a hand-authored `docker-compose.override.yml`: a pinned third-party image digest, many/complex injected env vars (e.g. SMTP credentials), and an extra bind mount (e.g. an email-template directory) — see the reference table above.

---

For a step-by-step walkthrough of building and deploying a custom service from scratch, see [[Guide-Custom-Services]].

← [[Config-Optional-Services]] | [[Configuration]] | [[Guide-Custom-Services]] →
