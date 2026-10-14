# Guide: Custom Services

Add up to 10 custom Docker services (CS_1 through CS_10) to your ɳSelf stack. Use this for app-specific services that aren't covered by the plugin ecosystem.

## How Custom Services Work

Each service needs a nonempty `CS_N=name:template[:port][:route]` definition. ɳSelf reads its supported `CS_N_*` overrides and generates the corresponding Docker Compose service block on `nself build`. Overrides alone do not register a service.

## Adding a Custom Service

### Step 1: Define the service in `.env`

```env
# CS_1: an existing application image containing /app/server.js
CS_1=my-api:node:3100:api
CS_1_IMAGE=registry.example.com/my-api:1.0.0
CS_1_COMMAND=node server.js

# Pass environment variables to the service
CS_1_ENV=NODE_ENV=production
# DATABASE_URL is injected automatically.
```

The image must set its own working directory (for example `WORKDIR /app` in its Dockerfile) and provide a `/health` endpoint plus `wget`, or set `CS_1_HEALTHCHECK` to a suitable probe. `CS_N_ENABLED`, `CS_N_NAME`, `CS_N_WORKDIR`, and arbitrary `CS_N_ENV_*` keys are not read. Remove the `CS_N` definition to stop generating that slot.

### Step 2: Rebuild and start

```bash
nself build    # generates updated docker-compose.yml
nself start    # creates/recreates containers from the generated config
```

### Step 3: Verify

```bash
nself status          # should show my-api as running
nself logs -f -S my-api # follows logs by the Compose service name
```

The route `api` exposes the service at `api.<BASE_DOMAIN>` by default. Omit the route for a service without an Nginx route.

## Available Language Templates

The custom-service scaffolder supports `go`, `node`, `python`, `static`, `rust`, and `other`:

```bash
nself service add --help # doc-check: skip registered in cmd/commands/service.go; no command-signature fixture yet
```

Use `nself service add <name> --template <language>` to create `services/<name>/` and register the next free slot in `.env.dev`. See [[Config-Custom-Services]] for healthcheck setup.

## Example: Python Worker (CS_2)

```env
CS_2=worker:python:8002
# An existing image containing Celery and the application's worker code
CS_2_IMAGE=registry.example.com/worker:1.0.0
CS_2_COMMAND=python -m celery worker
CS_2_ENV=CELERY_BROKER_URL=redis://redis:6379/0
CS_2_HEALTHCHECK=disabled
```

This example requires Redis to be enabled and reachable at the configured broker URL; include its credentials if required. Use a worker-specific probe instead of `disabled` when one is available.

## Example: Service Built From a Monorepo Root

A service in a monorepo often needs the whole workspace as its build context (shared packages, a lockfile at the root) while its Dockerfile lives deep inside. Four keys express that, with no hand-written compose file:

```env
# Project at <repo>/apps/api. The repository root is two levels up.
CS_1=worker:node:9500
CS_1_PATH=../..
CS_1_DOCKERFILE=apps/api/services/worker/Dockerfile
CS_1_BUILD_TARGET=runtime
CS_1_ENV_FILE=.env.dev,.env.secrets
CS_1_DEPENDS_ON=hasura:healthy
CS_1_COMMAND=node dist/server.js
```

What `nself build` generates for it: build context `../..` with that Dockerfile and target, `depends_on` for `postgres` and `hasura` (both `service_healthy`), the command as a list, and the environment from both files with `.env.secrets` winning on a repeated key.

Because the build context is the repository root, the builder receives everything under it. Put a `.dockerignore` at that root with the lines `**/.env*` and `**/.secrets` (a bare `.env*` only matches the root, so nested `.env` files would still be sent). `nself build` refuses an ancestor context without one (E528), and refuses a context above the directory that holds `.git`.

```bash
nself build
```

## Example: Depend on a Plugin Service

`CS_N_DEPENDS_ON` accepts the name of any service an installed plugin adds, so a service can wait for it:

```env
CS_2=reports:node:9501
CS_2_DEPENDS_ON=claw-api:started
```

The name is resolved after the plugin step of `nself build`. A name that does not exist fails the build with E500.

## Example: A Third-Party Stack

A third-party stack that has several containers (a message broker and a UI, say) maps to one `CS_N` definition with `CS_N_IMAGE` per container, or to a plugin when one exists. Services already share the default project network. To attach an extra network, use `CS_N_NETWORKS=<PROJECT_NAME>_shared` and create it before starting the stack. For `PROJECT_NAME=myapp`:

```bash
docker network create myapp_shared
```

Set `CS_N_NETWORKS=myapp_shared` on each service that needs it. The generator declares these extra networks external; it does not create them. See [[Custom-Services-Migration]] for what `CS_N` does not cover yet.

## Custom Service Variables Reference

| Variable | Description |
|----------|-------------|
| `CS_N` | Required definition: `name:template[:port][:route]` |
| `CS_N_IMAGE` | Docker image reference |
| `CS_N_PORT` | Internal port the service listens on |
| `CS_N_COMMAND` | Override container command (exec form, split on whitespace, no shell) |
| `CS_N_PATH` | Build context: a path inside the project, or an ancestor such as `../..` (see [[Config-Custom-Services]]) |
| `CS_N_DOCKERFILE` | Dockerfile path relative to the build context |
| `CS_N_BUILD_TARGET` | Multi-stage build target |
| `CS_N_ENV_FILE` | One or more dotenv files, comma-separated, later wins |
| `CS_N_DEPENDS_ON` | `name[:started\|healthy\|completed]`, comma-separated |
| `CS_N_NETWORKS` | Extra `<PROJECT_NAME>_*` networks (must already exist) |
| `CS_N_ENV` | Comma-separated `KEY=VALUE` pairs passed to the service |
| `CS_N_ENV_PASSTHROUGH` | Comma-separated allowlist of project environment variables to forward |
| `CS_N_HEALTHCHECK` | Probe path, `CMD`/`CMD-SHELL` command, or `disabled` |

## See Also

- [[Config-Custom-Services]], full env var reference
- [[Custom-Services-Migration]], moving hand-written compose fragments to `CS_N`
- [[cmd-service]], service command reference
- [[Architecture]], how custom services fit the stack

---
← [[Home]] | [[_Sidebar]]
