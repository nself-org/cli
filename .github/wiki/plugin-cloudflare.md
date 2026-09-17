# Cloudflare Plugin

> Cloudflare zone, DNS, R2, cache, and analytics management through one endpoint. **Pro plugin — requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Basic | $0.99/mo | $9.99/yr | Yes |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

This plugin is not currently part of a named bundle. Purchase any per-license subscription (Basic and up) for access.

Or get all bundles + all apps via **ɳSelf+** ($3.99/mo or $39.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install cloudflare
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error with a link to upgrade.

## Description

The Cloudflare plugin connects your nSelf instance to a Cloudflare account, exposing full zone management, DNS record CRUD, R2 object storage operations, cache purging, and analytics retrieval through a single REST API on port 3024. Read-heavy responses are cached locally; writes proxy directly to Cloudflare's API.

Every zone and record change is logged to Postgres for auditability. The `CF_API_TOKEN` is held only on the server side and never exposed to clients. R2 bucket operations require an account ID and R2 access keys; DNS and cache operations need only a zone-scoped token.

Analytics snapshots are stored on retrieval so you can query historical data without re-hitting the Cloudflare API on every request.

## How this differs from the cdn plugin

nSelf ships two CDN-adjacent pro plugins. They do not overlap, and you can run either or both.

| | **cloudflare** (port 3024) | **cdn** (port 3036) |
|---|---|---|
| Scope | Full Cloudflare account: zone management, DNS record CRUD, R2 buckets, WAF rules, analytics | Edge cache operations only: purge + signed URL generation |
| Providers | Cloudflare only | Cloudflare **and** BunnyCDN (multi-provider) |
| Primary use | Manage your Cloudflare zones and storage as infrastructure | Purge caches and mint HMAC-SHA256 signed URLs at request time |
| State tables | `np_cf_zones`, `np_cf_dns_records`, `np_cf_r2_buckets`, … | `np_cdn_purge_requests`, `np_cdn_signed_urls` |

Use **cloudflare** when you want nSelf to administer your Cloudflare zone, DNS, and R2 storage. Use **cdn** when you only need provider-agnostic cache purging and signed URLs and may swap providers. The two share no tables or routes.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `CF_API_TOKEN` | Yes | — | Cloudflare API token with zone and account permissions |
| `DATABASE_URL` | Yes | — | Postgres connection string for plugin state |
| `CF_PLUGIN_PORT` | No | `3024` | Port the plugin service listens on |
| `CF_PLUGIN_HOST` | No | `127.0.0.1` | Bind host |
| `CF_LOG_LEVEL` | No | `info` | Log verbosity |
| `CF_APP_IDS` | No | — | Multi-app isolation scope |
| `CF_ZONE_IDS` | No | — | Default zone ID list |
| `CF_R2_ACCESS_KEY` | No | — | R2 access key (required for R2 operations) |
| `CF_R2_SECRET_KEY` | No | — | R2 secret key (required for R2 operations) |
| `CF_ACCOUNT_ID` | No | — | Cloudflare account ID (required for R2) |
| `CF_SYNC_INTERVAL` | No | — | Analytics sync interval |

Reference vault credentials. Never hardcode secrets.

## Ports

| Port | Purpose |
|------|---------|
| `3024` | Cloudflare plugin HTTP service |

Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

6 tables added to your Postgres database (prefix `np_cf_`):

- `np_cf_zones`, synced zone records
- `np_cf_dns_records`, DNS A/AAAA/CNAME/MX/TXT records
- `np_cf_r2_buckets`, R2 bucket metadata
- `np_cf_cache_purge_log`, cache purge history
- `np_cf_analytics`, retrieved analytics snapshots
- `np_cf_webhook_events`, incoming Cloudflare webhook events

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

Public endpoints exposed by the plugin. Internal admin endpoints exist but are not part of the public surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/` | Plugin index / capability list |

Refer to the plugin's OpenAPI spec for the full route list.

## Examples

List zones:

```bash
curl -H 'Authorization: Bearer $TOKEN' https://api.example.com/cloudflare/zones
```

Add a DNS record:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/cloudflare/zones/zone_xxx/records \
  -d '{"type":"A","name":"app","content":"1.2.3.4"}'
```

Purge cache for a path:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/cloudflare/zones/zone_xxx/cache/purge \
  -d '{"files":["https://example.com/style.css"]}'
```

## Source code

Source-available (license required to run): [`bundles/paid/cloudflare/`](https://github.com/nself-org/bundles/tree/main/paid/cloudflare). The `plugins-pro` repository is private; source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [[plugin-cdn]], cache purge and signed URLs (multi-provider)
- [[plugin-ddns]], dynamic DNS updates
- [[plugin-object-storage]], S3-compatible storage
- [[Plugin-Overview]]

---

← [[Plugin-Overview]] | [[Home]] →
