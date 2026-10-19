# PostgreSQL Configuration

PostgreSQL is the foundation of every ɳSelf stack. It stores all application data and is the backing store for Hasura GraphQL, Auth, and any custom services you add. This page covers every `POSTGRES_*` environment variable, how the connection string is computed, and how to connect external database tools during development.

---

## Environment Variables

Every Postgres variable is optional. When `POSTGRES_PASSWORD` is unset, `nself build` generates a random password once and writes it to `.env.secrets` (mode 0600), so later builds read the same value back. A password you set yourself always wins; see [Where the password comes from](#where-the-password-comes-from).

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `POSTGRES_VERSION` | `16-alpine` | No | Docker image tag for the Postgres container. Change this to pin a specific version, e.g. `15-alpine` or `16.2-alpine`. |
| `POSTGRES_IMAGE` | *(unset)* | No | Explicit image override for the Postgres container (e.g. `pgvector/pgvector:pg16`). When set, wins over both `POSTGRES_VERSION` and the `POSTGRES_EXTENSIONS` pgvector inference below — use it to pin a running image across `nself build` regens. |
| `POSTGRES_DATA_VOLUME` | *(unset)* | No | Name of the docker volume Postgres keeps its data in (compose `volumes.postgres_data.name`). Written by `nself db image switch --to pgvector`; leave it unset to keep the project's own `<project>_postgres_data` volume. See [[pgvector]]. |
| `POSTGRES_HOST` | `postgres` | No | Internal hostname used by other containers (Hasura, Auth, Functions) to reach Postgres. This is a Docker network name and should almost never be changed. |
| `POSTGRES_INTERNAL_PORT` | `5432` | No | Port Postgres listens on inside the container. Always `5432`. Do not change this , it is the standard Postgres port expected by all dependent services. |
| `POSTGRES_PORT` | `5432` | No | Port exposed to the host machine. Change this if you have a port conflict with another local Postgres instance (e.g., set to `5433`). |
| `POSTGRES_DB` | `nself` | No | Name of the default database created on first start. |
| `POSTGRES_USER` | `postgres` | No | Superuser account name for the database. |
| `POSTGRES_PASSWORD` | *(generated on first build)* | No | Superuser password. Must be at least 16 characters. ɳSelf enforces a minimum length and, in staging and production, rejects common insecure patterns (e.g., `postgres`, `password`, `changeme`). When unset, `nself build` generates one and persists it to `.env.secrets`. |
| `POSTGRES_EXTENSIONS` | `uuid-ossp` | No | Comma-separated list of extensions to install automatically at startup. The extensions `uuid-ossp`, `pgcrypto`, and `pg_trgm` are always installed regardless of this value. Add additional extensions here as needed. When this list contains `pgvector` and `POSTGRES_IMAGE` is unset, `nself build` selects the `pgvector/pgvector:pg<major>` image instead of plain `postgres:<POSTGRES_VERSION>`. |
| `POSTGRES_EXPOSE_PORT` | `auto` | No | Controls whether `POSTGRES_PORT` is bound on the host. `auto` exposes the port in dev and hides it in prod. Set to `true` to always expose, or `false` to always hide. |
| `POSTGRES_MEM_LIMIT` | `2g` | No | Docker memory limit for the Postgres container. Uses Docker format: `512m`, `1g`, `4g`, etc. |
| `POSTGRES_CPU_LIMIT` | `2.0` | No | CPU core limit for the Postgres container. A value of `2.0` means the container can use up to two full cores. |

### POSTGRES_EXPOSE_PORT behaviour

| Value | Dev (`ENV=dev`) | Prod (`ENV=prod`) |
|-------|-----------------|-------------------|
| `auto` *(default)* | Port exposed to host | Port hidden from host |
| `true` | Port exposed to host | Port exposed to host |
| `false` | Port hidden from host | Port hidden from host |

In development you almost always want `auto` (the default) so you can connect TablePlus, DBeaver, or `psql` directly. In production the default `auto` hides the port, which is the safe behaviour, external access goes through Hasura GraphQL over Nginx instead.

---

## Computed Variable: DATABASE_URL

ɳSelf computes `DATABASE_URL` automatically from the Postgres variables above. You do not set this manually.

```
DATABASE_URL=postgresql://{POSTGRES_USER}:{POSTGRES_PASSWORD}@postgres:5432/{POSTGRES_DB}
```

Note the hostname is always `postgres` (the internal Docker network name), not `localhost`. This URL is used by Hasura, Auth, Functions, and any custom service that needs a direct database connection. The password component is URL-encoded automatically, you do not need to encode special characters yourself.

With default values and a password of `my-secure-password-here`, the computed URL would be:

```
DATABASE_URL=postgresql://postgres:my-secure-password-here@postgres:5432/nself
```

---

## Connection String Format

When connecting from an external tool (outside the Docker network), use `localhost` as the host instead of the internal `postgres` hostname:

```
postgresql://{POSTGRES_USER}:{POSTGRES_PASSWORD}@localhost:{POSTGRES_PORT}/{POSTGRES_DB}
```

With all defaults:

```
postgresql://postgres:your-password@localhost:5432/nself
```

---

## Connecting External Tools

During development, Postgres is exposed on your host machine at `localhost:{POSTGRES_PORT}`. You can connect any standard Postgres client, TablePlus, DBeaver, DataGrip, pgAdmin, or the `psql` CLI.

### Connection parameters

| Field | Value |
|-------|-------|
| Host | `localhost` (or `127.0.0.1`) |
| Port | Value of `POSTGRES_PORT` (default `5432`) |
| Database | Value of `POSTGRES_DB` (default `nself`) |
| Username | Value of `POSTGRES_USER` (default `postgres`) |
| Password | Value of `POSTGRES_PASSWORD` |
| SSL | Not required for local connections |

### psql (command line)

```bash
psql -h localhost -p 5432 -U postgres -d nself
```

You will be prompted for the password, or you can pass it via the `PGPASSWORD` environment variable:

```bash
PGPASSWORD=your-password psql -h localhost -p 5432 -U postgres -d nself
```

### TablePlus

1. Create a new connection, choose **PostgreSQL**
2. Fill in the fields from the table above
3. Click **Test**, you should see a green success indicator
4. Click **Connect**

### DBeaver

1. **Database > New Database Connection > PostgreSQL**
2. Fill in Host (`localhost`), Port, Database, Username, and Password
3. Click **Test Connection**, then **Finish**

### Port conflicts

If you already have a local Postgres instance running on port 5432, set `POSTGRES_PORT` to a different value in your `.env.local`:

```bash
# .env.local
POSTGRES_PORT=5433
```

Then connect your external tool to port `5433` instead.

---

## Default Extensions

The following extensions are installed automatically on every ɳSelf Postgres instance regardless of the `POSTGRES_EXTENSIONS` setting:

| Extension | Purpose |
|-----------|---------|
| `uuid-ossp` | UUID generation functions (`uuid_generate_v4()` etc.) |
| `pgcrypto` | Cryptographic functions used by Auth |
| `pg_trgm` | Trigram matching for full-text search and fuzzy queries |

To install additional extensions, add them to the `POSTGRES_EXTENSIONS` variable:

```bash
# .env.dev
POSTGRES_EXTENSIONS=uuid-ossp,postgis,hstore
```

Extensions are installed via `CREATE EXTENSION IF NOT EXISTS` at container startup, so adding an extension to an existing project is safe, it will be created on the next restart.

---

## Security Notes

**Postgres is not directly reachable from the internet.** The container binds to `127.0.0.1` inside Docker and is only accessible within the Docker network (by other containers) or from the host machine via the exposed port. There is no path from the public internet to Postgres directly, all external database access in production goes through Hasura GraphQL over Nginx.

Additional hardening applied to every Postgres container:

- `cap_drop: ALL`, all Linux capabilities are dropped
- `cap_add: IPC_LOCK`, only the minimum capability required for shared memory is re-added
- `POSTGRES_EXPOSE_PORT=auto` defaults to hidden in production, so the host port is not bound unless you explicitly opt in

**Password requirements:**

- Minimum 16 characters
- Common weak patterns are rejected in staging and production (`postgres`, `password`, `secret`, `changeme`, etc.)
- To pick your own, set `POSTGRES_PASSWORD` in `.env.secrets` (or `.env.local` for personal development); `openssl rand -base64 24` makes a strong one

### Where the password comes from

`nself build` resolves `POSTGRES_PASSWORD` in this order, first match wins:

1. A value in any env file of the active cascade, or exported in the process environment. Nothing is generated or written.
2. A project built by an earlier version keeps its running database password only in `.nself/compose.env`. The build reuses that value for the render and logs a warning naming the key. It does not write it, so a later rotated export is not shadowed. Pin it by adding `POSTGRES_PASSWORD` to `.env.secrets`.
3. Otherwise the build generates a random password and appends it to `.env.secrets`.

`HASURA_GRAPHQL_ADMIN_SECRET` follows the same rule. `.env.secrets` loads after `.env.<env>`, so a value there overrides the same key in an earlier file; the build warns (key name only) when that happens.

**Upgrading a project that was built before the password was persisted.** Earlier versions generated a new password on every load and never saved it, so a rebuild could render a password that no longer matched the existing database volume. After upgrading, run `nself build`: the build adopts the live password from `.nself/compose.env`. Then add `POSTGRES_PASSWORD` (and `HASURA_GRAPHQL_ADMIN_SECRET`) to `.env.secrets` so the value is pinned.

**Never commit `POSTGRES_PASSWORD` to git.** `.env.secrets` and `.env.local` are gitignored by default.

---

← [[Config-Env-Vars]] | [[Configuration]] | [[Config-Hasura]] →
