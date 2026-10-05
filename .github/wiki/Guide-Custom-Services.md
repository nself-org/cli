# Guide: Custom Services

Add up to 10 custom Docker services (CS_1 through CS_10) to your ɳSelf stack. Use this for app-specific services that aren't covered by the plugin ecosystem.

## How Custom Services Work

Custom services are defined via environment variables. ɳSelf reads `CS_1_*` through `CS_10_*` variables and generates the corresponding `docker-compose` service block automatically on `nself build`.

## Adding a Custom Service

### Step 1: Define the service in `.env`

```env
# CS_1: Node.js API server
CS_1_ENABLED=true
CS_1_NAME=my-api
CS_1_IMAGE=node:20-alpine
CS_1_PORT=3100
CS_1_COMMAND=node server.js
CS_1_WORKDIR=/app

# Pass environment variables to the service
CS_1_ENV_NODE_ENV=production
CS_1_ENV_DATABASE_URL=${DATABASE_URL}
```

### Step 2: Rebuild and restart

```bash
nself build    # generates updated docker-compose.yml
nself restart  # applies changes
```

### Step 3: Verify

```bash
nself status          # should show my-api as running
nself urls            # shows URL if routed through nginx
```

## Available Language Templates

See 40+ starter templates to get going quickly:

```bash
nself service templates
```

Templates include: Node.js, Python (FastAPI, Django), Go, Ruby on Rails, PHP, Java Spring Boot, Rust, and more.

## Example: Python Worker (CS_2)

```env
CS_2_ENABLED=true
CS_2_NAME=worker
CS_2_IMAGE=python:3.12-slim
CS_2_COMMAND=python -m celery worker
CS_2_ENV_CELERY_BROKER_URL=redis://redis:6379/0
```

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

Because the build context is the repository root, the builder receives everything under it. Put a `.dockerignore` at that root that excludes `.env*` and `.secrets/`. `nself build` refuses an ancestor context without one (E528), and refuses a context above the directory that holds `.git`.

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

A third-party stack that has several containers (a message broker and a UI, say) maps to one `CS_N_IMAGE` service per container, or to a plugin when one exists. `CS_N_IMAGE` services can share a project network with `CS_N_NETWORKS=<PROJECT_NAME>_shared` once that network exists. See [[Custom-Services-Migration]] for what `CS_N` does not cover yet.

## Custom Service Variables Reference

| Variable | Description |
|----------|-------------|
| `CS_N_ENABLED` | `true` / `false` |
| `CS_N_NAME` | Service name (lowercase, alphanumeric + hyphens) |
| `CS_N_IMAGE` | Docker image reference |
| `CS_N_PORT` | Internal port the service listens on |
| `CS_N_COMMAND` | Override container command (exec form, split on whitespace, no shell) |
| `CS_N_PATH` | Build context: a path inside the project, or an ancestor such as `../..` (see [[Config-Custom-Services]]) |
| `CS_N_DOCKERFILE` | Dockerfile path relative to the build context |
| `CS_N_BUILD_TARGET` | Multi-stage build target |
| `CS_N_ENV_FILE` | One or more dotenv files, comma-separated, later wins |
| `CS_N_DEPENDS_ON` | `name[:started\|healthy\|completed]`, comma-separated |
| `CS_N_NETWORKS` | Extra `<PROJECT_NAME>_*` networks (must already exist) |
| `CS_N_WORKDIR` | Working directory inside container |
| `CS_N_ENV_*` | Environment variables passed to the service |

## See Also

- [[Config-Custom-Services]], full env var reference
- [[Custom-Services-Migration]], moving hand-written compose fragments to `CS_N`
- [[cmd-service]], service command reference
- [[Architecture]], how custom services fit the stack

---
← [[Home]] | [[_Sidebar]]
