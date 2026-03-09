# Setting Up Pro Plugins on a Self-Hosted nself Backend

This guide covers installing and configuring nself Pro plugins on a Hetzner VPS running staging or production. Follow every step in order. Skipping the database migration or seed steps causes confusing errors later.

---

## Prerequisites

- **nself v0.9.9+** installed on the server
- A valid **Pro, Max, or Owner license key** (format: `nself_pro_` + 32 chars)
- **ping_api** (CS_1) running and reachable at `http://127.0.0.1:8001`
- SSH access to the VPS

Confirm ping_api is up before continuing:

```bash
curl -s http://127.0.0.1:8001/health
# Expected: {"status":"ok","service":"ping_api"}
```

---

## 1. Apply the Plugin Registry Migration

The plugin registry tables must exist before any license validation works. Without them, ping_api returns `HTTP 500 "Validation service error"` even for a valid license key.

Migration: `1770895290000_plugin_registry`

This creates four tables:

| Table | Purpose |
| --- | --- |
| `plugins` | Plugin registry (49 plugins, 15 categories) |
| `plugin_versions` | Available versions per plugin |
| `plugin_access` | Which license tiers can install which plugins |
| `plugin_downloads` | Audit log of all download requests |

Apply via the nself CLI (preferred):

```bash
nself db migrate up
```

Or apply directly via psql if the CLI isn't available:

```bash
cat hasura/migrations/default/1770895290000_plugin_registry/up.sql | \
  docker exec -i nself-web_postgres psql -U postgres -d <db_name>
```

Replace `<db_name>` with the value of `POSTGRES_DB` in your `.env` file.

Confirm it applied:

```bash
docker exec -i nself-web_postgres psql -U postgres -d <db_name> \
  -c "SELECT COUNT(*) FROM plugins;"
# Expected: count = 49
```

---

## 2. Apply the Owner License Seed

If you are setting up the nself.org infrastructure (or your own owner-tier deployment), apply the owner license seed. This registers a permanent enterprise license that grants access to all 49 pro plugins.

The seed:
- Inserts the owner license at reserved UUID `00000000-0000-0000-0000-00000000cafe`
- Pre-registers all nself.org domains
- Grants access to all pro plugins for the `enterprise` tier

This seed requires `000_production_owner.sql` to have run first (user `00000000-0000-0000-0000-000000000001` must exist).

```bash
nself db hasura seed apply --file 008_owner_license.sql
```

If you are not running nself.org infrastructure, skip this step. Standard Pro/Max keys work without it.

---

## 3. Set Required Environment Variables

The following variables must be in your `.env` file at the project root. They must be in `.env`, not `.env.prod` or `.env.staging`, because `docker compose` reads the root `.env` file directly.

```bash
# Signing key for plugin download URLs
DOWNLOAD_SIGNING_KEY=<signing_key>

# How long download tokens remain valid (seconds)
DOWNLOAD_TOKEN_TTL_SECONDS=900

# Owner license key (for nself.org infra and testing)
NSELF_PLUGIN_LICENSE_KEY=<owner_key>

# GitHub PAT with read access to private plugin repos
GITHUB_PAT_PLUGINS=<github_pat>
```

Do not set `API_BASE_URL` unless you have a specific reason. ping_api defaults to `https://ping.nself.org` and does not need this set for normal operation.

After editing `.env`, rebuild configs and restart ping_api:

```bash
nself build
docker compose up -d --force-recreate ping_api
```

Verify the variables loaded:

```bash
docker exec nself-web_ping_api env | grep -E 'DOWNLOAD_SIGNING_KEY|GITHUB_PAT'
```

---

## 4. Store the License Key

Run this once on the server. It stores the key in `~/.nself/license/key` so the CLI reads it automatically:

```bash
nself plugin license set <license_key>
```

Confirm it saved:

```bash
nself plugin license status
# Expected: license key loaded, tier: pro (or max / enterprise)
```

---

## 5. Install Plugins

```bash
# Install a single plugin
nself plugin install ai

# Install multiple plugins at once
nself plugin install ai mux claw
```

The CLI validates the license against ping_api, checks your tier, downloads the plugin, and places it in `~/.nself/plugins/<name>/`.

---

## 6. Add a Plugin as a Custom Service

After installing, register the plugin as a custom service in `.env`. nself supports up to 10 custom services (`CS_1` through `CS_10`). CS_1 is reserved for ping_api.

Example: adding the `ai` plugin as CS_2:

```bash
# In .env
CS_2=ai:express-ts:3101
CS_2_ROUTE=ai
CS_2_PUBLIC=false
CS_2_HEALTHCHECK=/health
CS_2_REPLICAS=1
CS_2_MEMORY=1G
CS_2_CPU=1.0
```

Copy the plugin service files into the project:

```bash
cp -r ~/.nself/plugins/ai/ts/ services/ai/
```

Rebuild and start:

```bash
nself build
docker compose up -d ai
```

---

## 7. Verification

Check that license validation works:

```bash
curl -s -X POST http://127.0.0.1:8001/license/validate \
  -H 'Content-Type: application/json' \
  -d '{"license_key":"<key>","plugin_name":"ai"}'
```

Expected response:

```json
{
  "valid": true,
  "tier": "enterprise",
  "can_install_plugin": true
}
```

Check the plugin service is running:

```bash
curl -s http://127.0.0.1:3101/health
# Expected: {"status":"ok","plugin":"ai"}
```

---

## Common Issues

### "Plugin 'X' not found in repository"

The CLI could not find the license key. This happens when `NSELF_PLUGIN_LICENSE_KEY` is set in the environment but the key file is missing.

Fix: run `nself plugin license set <key>` to write the key to `~/.nself/license/key`.

---

### "Failed to reach plugin distribution service"

ping_api is not loading `DOWNLOAD_SIGNING_KEY`. The variable was probably added to `.env.prod` or `.env.staging` instead of the root `.env` file.

Fix: move plugin variables to `.env`, then run:

```bash
docker compose up -d --force-recreate ping_api
```

---

### "Validation service error" (HTTP 500)

The plugin registry tables do not exist. Apply the migration first:

```bash
nself db migrate up
```

See [Step 1](#1-apply-the-plugin-registry-migration) for details.

---

### "This plugin requires Max tier" for an owner or enterprise license

The license tier check is too strict. Owner licenses store tier as `enterprise` in the database, but some older versions checked only for `max` by name.

Fix: update to nself v0.9.9+. The updated ping_api checks the `all_plugins` feature flag on the license record, which bypasses the Max-tier name check.

---

### `nself build` exits silently and CS_N service is missing from docker-compose.yml

Two possible causes:

**Cause 1:** `CS_N_REPLICAS` is greater than 1 with a port binding. Standalone Docker Compose does not support replicas with bound ports.

Fix: set `CS_N_REPLICAS=1`.

**Cause 2:** A variable in the source env file has an unbound reference (e.g., `${SOME_VAR}` that is not defined). With `set -u`, this causes the build script to exit silently.

Fix: update to nself v0.9.9+, which handles unbound references without aborting the build.

---

### "Port 80/443 already in use" on startup

Existing containers were started with a different `COMPOSE_PROJECT_NAME`. Docker Compose uses the project name in container naming, so the old containers are still holding the ports.

Fix: find the old project name and stop those containers, or use `-p` to target them:

```bash
docker compose -p <old_project_name> down
nself start
```

---

## Related

- [Plugin Architecture](../commands/plugin.md)
- [Custom Services Reference](../configuration/custom-services.md)
- [Environment Variables Reference](../configuration/env-vars.md)
- [ping_api Setup](../guides/ping-api-setup.md)
