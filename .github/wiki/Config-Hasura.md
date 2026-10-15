# Config-Hasura

Hasura GraphQL Engine is the primary API layer in every ɳSelf stack. It sits in front of PostgreSQL and exposes an auto-generated GraphQL API, handles JWT-based authorization, and supports remote schemas for federating external GraphQL services. This page covers every `HASURA_*` environment variable, explains how JWT secrets are constructed, and describes how to access the Hasura Console.

---

## Environment Variables

All Hasura configuration is driven by `HASURA_*` variables in your `.env` files. Required variables must be set before `nself start`, the CLI will refuse to start if they are missing.

| Variable | Type | Default | Required | Description |
|----------|------|---------|----------|-------------|
| `HASURA_VERSION` | string | blank | No | Blank keeps the project lock pin, or uses v2.44.0 in v1.4 and v2.51.0 in v1.5. Any nonblank tag is explicit. |
| `HASURA_GRAPHQL_ADMIN_SECRET` | string | *(none)* | **Yes** | Admin API secret (minimum 32 characters). Required to access the Hasura Console and the admin API endpoint. |
| `HASURA_JWT_KEY` | string | *(none)* | **Yes** | JWT signing key (minimum 32 characters). Used to verify tokens issued by the Auth service. |
| `HASURA_JWT_TYPE` | string | `HS256` | No | JWT algorithm. Supported values: `HS256`, `RS256`, and other algorithms supported by Hasura. |
| `HASURA_GRAPHQL_JWT_SECRET` | JSON | *auto-computed* | No | Full JWT secret JSON object passed to Hasura. Auto-built from `HASURA_JWT_KEY` and `HASURA_JWT_TYPE`. Do not set this manually , see [JWT Configuration](#jwt-configuration) below. |
| `HASURA_GRAPHQL_ENABLE_CONSOLE` | bool | `true` (dev) / `false` (prod) | No | Enable the Hasura web console. Should be disabled in production. |
| `HASURA_GRAPHQL_DEV_MODE` | bool | `true` (dev) / `false` (prod) | No | Developer mode. When enabled, Hasura returns detailed error messages including internal query details. Disable in production. |
| `HASURA_DEV_MODE` | bool | *(alias)* | No | Backward-compatible alias for `HASURA_GRAPHQL_DEV_MODE`. Both variables are recognised; `HASURA_GRAPHQL_DEV_MODE` takes precedence if both are set. |
| `HASURA_GRAPHQL_ENABLE_TELEMETRY` | bool | `false` | No | Hasura telemetry reporting. Always off by default in ɳSelf projects. |
| `HASURA_GRAPHQL_CORS_DOMAIN` | string | `http://localhost:*` (dev) | No | Allowed CORS origins. Use a comma-separated list of exact origins in production , wildcards are not safe in production. |
| `HASURA_GRAPHQL_UNAUTHORIZED_ROLE` | string | `public` | No | The Hasura role assigned to unauthenticated requests. |
| `HASURA_GRAPHQL_LOG_LEVEL` | string | `warn` | No | Hasura log verbosity. Valid values: `debug`, `info`, `warn`, `error`. |
| `HASURA_PORT` | int | `8080` | No | Internal container port for the Hasura HTTP service. |
| `HASURA_CONSOLE_PORT` | int | `9695` | No | Port used by the `hasura-cli` console proxy (dev mode only). |
| `HASURA_ROUTE` | string | `api` | No | Nginx subdomain prefix. With `BASE_DOMAIN=example.com`, Hasura is exposed at `api.example.com`. |
| `HASURA_MEM_LIMIT` | string | `1g` | No | Docker memory limit for the Hasura container. |
| `HASURA_CPU_LIMIT` | string | `1.0` | No | Docker CPU quota for the Hasura container. |

---

## Admin Secret

`HASURA_GRAPHQL_ADMIN_SECRET` is the master credential for your Hasura instance. It grants unrestricted access to all data, metadata, and the Hasura Console. Treat it like a database root password.

**Requirements:**

- Minimum 32 characters
- Must be set before the stack starts, ɳSelf will not start without it
- Use a randomly generated value; do not reuse the same secret across projects or environments

**Where to set it:**

```bash
# .env.secrets (production — gitignored)
HASURA_GRAPHQL_ADMIN_SECRET=your-randomly-generated-secret-here

# .env.local (personal dev override — gitignored)
HASURA_GRAPHQL_ADMIN_SECRET=your-dev-secret-here
```

**Never put the admin secret in `.env.dev`**, that file is committed to version control and shared with your team.

---

## JWT Configuration

Hasura verifies JWT tokens using a JSON configuration object passed as `HASURA_GRAPHQL_JWT_SECRET`. Rather than making you write this JSON yourself, ɳSelf builds it automatically from two simpler variables:

| Input variable | Example value |
|---------------|--------------|
| `HASURA_JWT_KEY` | `my-super-secret-signing-key-32chars` |
| `HASURA_JWT_TYPE` | `HS256` |

ɳSelf computes and injects the following into Hasura at startup:

```json
{
  "type": "HS256",
  "key": "my-super-secret-signing-key-32chars"
}
```

This value is assigned to `HASURA_GRAPHQL_JWT_SECRET` automatically. You do not need to set `HASURA_GRAPHQL_JWT_SECRET` yourself, setting `HASURA_JWT_KEY` and `HASURA_JWT_TYPE` is sufficient.

**The JWT key must match the Auth service.** ɳSelf configures the Auth service to sign tokens with the same `HASURA_JWT_KEY`. If you ever rotate this key, you must restart both services and all existing sessions will be invalidated.

**Algorithm selection:**

| Algorithm | Use case |
|-----------|---------|
| `HS256` | Default. Symmetric HMAC , the same key signs and verifies. Simplest to configure. |
| `RS256` | Asymmetric RSA. Use this when the signing service (Auth) and the verifying service (Hasura) are owned by different parties, or when you need to publish the public key. |

For most self-hosted ɳSelf projects, `HS256` with a strong random key is the right choice.

---

## Remote Schemas

Remote schemas let you federate external GraphQL services into your Hasura API. ɳSelf supports up to 20 remote schema slots, numbered 1 through 20.

### Variables

Each slot uses three variables:

| Variable | Description |
|----------|-------------|
| `REMOTE_SCHEMA_N_NAME` | Schema name as it appears in Hasura (e.g., `payments`) |
| `REMOTE_SCHEMA_N_URL` | GraphQL endpoint URL (e.g., `https://payments.example.com/graphql`) |
| `REMOTE_SCHEMA_N_HEADERS` | Request headers in `key:value,key:value` format (e.g., `Authorization:Bearer secret`) |

Replace `N` with a slot number from 1 to 20.

### Example

```bash
# .env.dev
REMOTE_SCHEMA_1_NAME=payments
REMOTE_SCHEMA_1_URL=https://payments.example.com/graphql
REMOTE_SCHEMA_1_HEADERS=Authorization:Bearer my-payments-token

REMOTE_SCHEMA_2_NAME=search
REMOTE_SCHEMA_2_URL=https://search.example.com/graphql
REMOTE_SCHEMA_2_HEADERS=
```

### Automatic injection

ɳSelf reads all `REMOTE_SCHEMA_N_*` variables at startup and registers them with Hasura via the metadata API. You do not need to configure remote schemas inside the Hasura Console manually, they are declared in your env files and applied automatically on every `nself start`.

Slots with no `REMOTE_SCHEMA_N_NAME` set are silently skipped.

---

## Forwarding Any HASURA_GRAPHQL_* Engine Tuning Var

The table above lists the `HASURA_GRAPHQL_*` variables ɳSelf builds into a curated compose value (admin secret, JWT secret, console/dev-mode toggles, CORS, log level). Beyond that curated set, **every other `HASURA_GRAPHQL_*` variable you set in any `.env` file is forwarded verbatim into the Hasura container** at `nself build` time — you do not need to add support for it first.

This covers real Hasura engine config surface that ɳSelf does not wrap in its own typed setting, for example:

```bash
# .env.dev — allow-list mode and engine limits
HASURA_GRAPHQL_ENABLE_ALLOWLIST=true
HASURA_GRAPHQL_NODE_LIMIT=5000
HASURA_GRAPHQL_DEPTH_LIMIT=10
HASURA_GRAPHQL_BATCH_SIZE=10
HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_BATCH_SIZE=100
HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_REFETCH_INTERVAL=1000
```

**Rules:**

- The forward is additive only. If a variable is already curated by ɳSelf (`HASURA_GRAPHQL_ADMIN_SECRET`, `_JWT_SECRET`, `_ENABLE_CONSOLE`, `_DEV_MODE`, `_ENABLE_TELEMETRY`, `_CORS_DOMAIN`, `_LOG_LEVEL`, `_UNAUTHORIZED_ROLE`, `_DATABASE_URL`), your `.env` value is **ignored** for that key — use the dedicated variable (or the `HASURA_JWT_KEY`/`HASURA_JWT_TYPE` pair for JWT) instead, so it goes through validation.
- Everything else in the `HASURA_GRAPHQL_*` namespace reaches the container untouched — no allow-list of names to keep in sync in the CLI itself.
- Run `nself build` (or `nself restart`) after adding or changing one of these vars so the regenerated compose picks it up.
- The same additive-forwarding rule applies to hasura-auth's own `AUTH_*`/`HASURA_AUTH_*` namespace — see [[Config-Auth]].

---

## Accessing the Hasura Console

The Hasura Console is a web UI for browsing your schema, running GraphQL queries, managing metadata, and configuring permissions.

### Get the URL

```bash
nself urls
```

This prints all service URLs for your current stack, including the Hasura Console URL. The console is typically available at:

```
https://api.<BASE_DOMAIN>/console
```

For local development this is usually `http://localhost:8080/console` (direct) or via your Nginx proxy if configured.

### Log in

When the console loads it will prompt for the admin secret. Enter the value from `HASURA_GRAPHQL_ADMIN_SECRET`.

### Console availability

| Environment | Default state | Variable |
|-------------|--------------|---------|
| Development | Enabled | `HASURA_GRAPHQL_ENABLE_CONSOLE=true` |
| Production | Disabled | `HASURA_GRAPHQL_ENABLE_CONSOLE=false` |

Always disable the console in production. Leaving it enabled exposes your schema and metadata to anyone who can reach the URL.

---

## Production Checklist

Before going live, verify the following Hasura settings:

- [ ] `HASURA_GRAPHQL_ADMIN_SECRET` is at least 32 characters and randomly generated
- [ ] `HASURA_JWT_KEY` is at least 32 characters and randomly generated
- [ ] `HASURA_GRAPHQL_ENABLE_CONSOLE=false`
- [ ] `HASURA_GRAPHQL_DEV_MODE=false`
- [ ] `HASURA_GRAPHQL_ENABLE_TELEMETRY=false`
- [ ] `HASURA_GRAPHQL_CORS_DOMAIN` lists only exact origins, no wildcards
- [ ] Neither secret appears in any committed file (`.env.dev`, `.env.prod`, etc.)

Run `nself config validate --env prod` to catch common mistakes before deploying.

---

← [[Config-Postgres]] | [[Configuration]] | [[Config-Auth]] →
