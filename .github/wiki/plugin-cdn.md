# CDN Plugin

> Cache purging, HMAC-SHA256 signed URL generation, and multi-provider CDN management. **Pro plugin — requires license.**

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
nself plugin install cdn
nself build
```

The license is validated against `ping.nself.org/license/validate`. Insufficient tier returns an error with a link to upgrade.

## Description

The CDN plugin connects your nSelf instance to Cloudflare and BunnyCDN, exposing cache purging, signed URL generation, and zone management through a single REST API on port 3036.

Cache purge requests are submitted by URL, wildcard pattern, or full-zone sweep. Every purge is logged to the `np_cdn_purge_requests` table so you can audit what was cleared, when, and by whom. Signed URLs use HMAC-SHA256 with a configurable expiry and are written to `np_cdn_signed_urls` for revocation tracking.

Analytics sync is scaffolded but not yet active — the `analytics` command returns stored snapshots from prior manual calls; automatic periodic sync from the CDN provider is a planned feature.

## Cloudflare setup

1. Log in to [dash.cloudflare.com](https://dash.cloudflare.com) and go to **My Profile > API Tokens**.
2. Create a token with the **Zone > Cache Purge** permission (and optionally **Zone > Analytics > Read** for future analytics sync).
3. Copy your **Zone ID** from the zone's Overview sidebar.
4. Set the following env vars:

```bash
CDN_CLOUDFLARE_API_TOKEN=your_cloudflare_api_token
CDN_CLOUDFLARE_ZONE_IDS=zone1id,zone2id   # comma-separated for multiple zones
CDN_PROVIDER=cloudflare
```

## BunnyCDN setup

1. Log in to [bunny.net](https://bunny.net) and go to **Account > API**.
2. Copy your **API Key** from the account settings page.
3. Copy the **Pull Zone ID(s)** from each Pull Zone's configuration panel.
4. Set the following env vars:

```bash
CDN_BUNNYCDN_API_KEY=your_bunny_api_key
CDN_BUNNYCDN_PULL_ZONE_IDS=zone1id,zone2id   # comma-separated
CDN_PROVIDER=bunnycdn
```

## Signed URL configuration

Signed URLs require a secret signing key and an optional default TTL:

```bash
CDN_SIGNING_KEY=your_secret_signing_key          # min 32 random bytes, base64-safe
CDN_SIGNED_URL_TTL=3600                          # default expiry in seconds (1 hour)
```

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | — | Postgres connection string (auto-provided by nSelf) |
| `CDN_API_KEY` | — | API key for inter-plugin auth (auto-provided by nSelf) |
| `CDN_PROVIDER` | — | Active provider: `cloudflare` or `bunnycdn` |
| `CDN_CLOUDFLARE_API_TOKEN` | — | Cloudflare API token with Cache Purge permission |
| `CDN_CLOUDFLARE_ZONE_IDS` | — | Comma-separated Cloudflare zone IDs |
| `CDN_BUNNYCDN_API_KEY` | — | BunnyCDN account API key |
| `CDN_BUNNYCDN_PULL_ZONE_IDS` | — | Comma-separated BunnyCDN pull zone IDs |
| `CDN_SIGNING_KEY` | — | Secret key for HMAC-SHA256 signed URL generation |
| `CDN_SIGNED_URL_TTL` | `3600` | Default signed URL expiry in seconds |
| `CDN_PURGE_BATCH_SIZE` | `30` | Max URLs per purge API call |
| `CDN_ANALYTICS_SYNC_INTERVAL` | `86400` | Analytics poll interval in seconds (future use) |
| `CDN_RATE_LIMIT_MAX` | `100` | Max requests per rate-limit window |
| `CDN_RATE_LIMIT_WINDOW_MS` | `60000` | Rate-limit window duration in ms |
| `CDN_PORT` | `3036` | Port the CDN plugin service listens on |

## Ports

| Port | Purpose |
|------|---------|
| `3036` | CDN plugin HTTP service |

## Database tables

4 tables are added to your Postgres database under the `np_cdn_*` prefix.

| Table | Description |
|-------|-------------|
| `np_cdn_zones` | CDN zone registry (provider, zone ID, label) |
| `np_cdn_purge_requests` | Audit log of all cache purge operations |
| `np_cdn_signed_urls` | Issued signed URL records with expiry and status |
| `np_cdn_analytics` | Cached analytics snapshots from CDN providers |

All tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'` for multi-app isolation per the Multi-Tenant Convention Wall.

## Cache purge API

```
POST /api/v1/purge
Authorization: Bearer <CDN_API_KEY>

{
  "zone_id": "zone1id",
  "urls": ["https://cdn.example.com/assets/app.js"],
  "wildcard": "*.css",
  "purge_all": false
}
```

Retrieve purge history:

```
GET /api/v1/purge-requests
Authorization: Bearer <CDN_API_KEY>
```

## Nginx routes

| Route | Description |
|-------|-------------|
| `/cdn/` | Proxied to CDN plugin service on port 3036 |

## Examples

```bash
# Initialize database schema
nself plugin cdn init

# Add a Cloudflare zone
nself plugin cdn zones add my-zone cloudflare abc123zoneID

# Purge specific URLs
nself plugin cdn purge abc123zoneID --urls https://cdn.example.com/app.js,https://cdn.example.com/app.css

# Purge all CSS files (wildcard)
nself plugin cdn purge abc123zoneID --wildcard "*.css"

# Full-zone cache flush
nself plugin cdn purge abc123zoneID --all

# Generate a signed URL (expires in 1 hour)
nself plugin cdn sign https://cdn.example.com/private/file.mp4 --ttl 3600

# View plugin statistics
nself plugin cdn status
```

## Source

Source-available (license required to run): [`bundles/paid/cdn/`](https://github.com/nself-org/bundles/tree/main/paid/cdn)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [[plugin-cloudflare]] — Cloudflare DNS, Workers, and Pages management
- [[plugin-object-storage]] — S3-compatible object storage for CDN origin content
- [[Plugin-Licensing]] — tier comparison
- [[Plugin-Overview]] — full plugin index

← [[Plugin-Overview]] | [[Home]] →
