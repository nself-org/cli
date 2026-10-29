# Licensing Guide

This page covers the full ɳSelf licensing model: the free core, per-product plugin bundles, the ɳSelf+ all-access subscription, key management, and upgrade/downgrade behavior.

---

## The Free Core

The ɳSelf CLI and the free plugins are MIT licensed. Free forever, including commercial use. No license key needed.

Free plugins include: backup, content-acquisition, content-progress, cron, donorbox, feature-flags, github, github-runner, invitations, jobs, link-preview, mdns, mlflow, monitoring, notifications, notify, paypal, search, shopify, stripe, subtitle-manager, tokens, torrent-manager, vpn, webhooks.

You can run a full production stack (Postgres, Hasura, Auth, Nginx, monitoring, backups, search) without paying anything.

---

## Licensed Plugins

Licensed plugins are source-available Rust binaries distributed through the ɳSelf plugin system. They require a valid license key.

### Per-Bundle Pricing

The authoritative bundle catalog, pricing, and plugin membership live at [nself.org/pricing](https://nself.org/pricing) and are fetched by the CLI from `https://plugins.nself.org/bundles.json`. The wiki pages below summarize each bundle; always treat the live catalog as the source of truth for what is sold.

Current bundles (as of v1.4.13):

| Bundle | Tier | Monthly | Annual | Notes |
|--------|------|---------|--------|-------|
| Task | free | $0 | $0 | Core productivity plugins; no license key required |
| Chat | paid | $0.99 | $9.99 | Messaging and voice |
| Claw | paid | $0.99 | $9.99 | AI personal assistant |
| Family | paid | $0.99 | $9.99 | Private family social platform |
| Sentry | paid | $0.99 | $9.99 | Observability and incident management |
| ClawDE | paid | $0.99 | $9.99 | Dev environment cloud sync |
| ɳSelf+ | paid | $3.99 | $39.99 | Every bundle + all apps + support |

There is no "Basic" or "Pro" tier. Paid bundles are independent purchases; ɳSelf+ includes every paid bundle. Annual pricing saves roughly two months versus monthly.

---

## Per-Product Plugin Bundles

Each bundle page lists its plugin set and purpose. Plugin lists change with releases, and the ClawDE set is provisional; the live catalog at [nself.org/pricing](https://nself.org/pricing) is authoritative.

- [[bundle-nclaw]] — AI personal assistant
- [[bundle-nchat]] — Messaging and voice
- [[bundle-nfamily]] — Private family social platform
- [[cmd-sentry]] — Observability and incident management
- [[bundle-clawde]] — Dev environment cloud sync
- [[Bundles]] — Free core productivity plugins (the Task Bundle)

---

## ɳSelf+ ($3.99/mo or $39.99/yr)

The all-access subscription. Includes every plugin bundle, every product, and support via chat.nself.org or the ɳChat app. One key unlocks the entire ecosystem.

ɳSelf+ costs less than buying individual bundles if you use three or more products.

---

## Key Management

### Setting Your Key

```bash
nself license set nself_pro_xxxxx...
```

The key is stored at `~/.nself/license/key` with chmod 600. It is also read from the `NSELF_PLUGIN_LICENSE_KEY` environment variable.

### Checking Status

```bash
nself license status
```

Shows your current tier, expiry date, and which plugin categories you can install.

### Removing Your Key

```bash
nself license remove
```

### Key Format

Keys use a `nself_<product>_` prefix, for example `nself_plus_` for ɳSelf+. The prefix only names the key's product; the signed license response decides which plugins it unlocks. [[Plugin-Licensing]] lists every accepted prefix.

Keys must be at least 32 characters total.

---

## License Validation

Tier entitlement is checked server-side. The CLI never makes tier decisions locally.

```
POST https://ping.nself.org/license/validate
{"license_key": "...", "product": "plugins-pro"}
```

| Response | Meaning |
|----------|---------|
| 200 | Valid key, plugin allowed |
| 401/403/404 | Invalid key or insufficient tier |

Validation results are cached at `~/.nself/license/cache`. How long a result counts depends on the tier: free keys are not cached (every command revalidates), Pro tiers 7 days, ɳSelf+, Max, Enterprise and owner keys 30 days. In v1.5 (`NSELF_V15=1`) the cache is checked against ping's signature before it is trusted; see [[Plugin-Security]].

### Runtime Revalidation

Installed plugins revalidate when their cached result runs out (the tier TTL above). If the licence server cannot be reached, the last good result keeps working: silently for up to 72 hours, with a warning after that, and access fails closed once the cached result is 7 days old. A licence that has expired keeps paid plugins working for a 30-day grace window before they go dormant. A warning starts 2 days before the cached result expires. Force revalidation:

```bash
nself license revalidate
```

### Offline Mode

For air-gapped environments:

```bash
NSELF_LICENSE_SKIP_VERIFY=1 nself plugin install {name} --force
```

This skips the licence check only. The plugin's signature and checksum are still
verified. Set the variable in your shell: a project `.env` cannot set it.

---

## Upgrading and Downgrading

### Upgrade
1. Purchase a higher tier at nself.org/pricing.
2. Run `nself license set {new-key}`.
3. Install newly available plugins: `nself plugin install ai claw mux ...`
4. Rebuild: `nself build && nself start`.

Stripe handles proration automatically. You pay the difference for the remainder of your billing cycle.

### Downgrade
1. Change your tier in the Stripe customer portal (cloud.nself.org/license).
2. Plugins above your new tier stop working at the next revalidation, when the cached result runs out (7 or 30 days, by tier).
3. The CLI does not forcibly remove plugins, but they will fail health checks and be excluded from the tool registry.

### Cancellation
1. Cancel via the Stripe portal.
2. Free plugins continue working indefinitely.
3. Licensed plugins stop working at next revalidation.
4. Your data is never deleted. You still have full access to your database and files.

---

## Stripe Integration

All payment processing goes through Stripe:

- Checkout at nself.org/pricing creates a Stripe subscription.
- Webhook at ping.nself.org handles subscription lifecycle events.
- Customer portal at cloud.nself.org/license for self-service billing management.
- Invoices accessible in the portal.

---

## Related Pages

- [[Plugin-Licensing]] -- technical details of the plugin license system
- [[Plugin-Overview]] -- full plugin catalog
- [[Plugin-Install]] -- how to install plugins
- [[Feature-nClaw]] -- ɳClaw plugin bundle details
- [[Feature-nCloud]] -- managed hosting (separate from plugin licenses)

---

← [[Home]] | [[_Sidebar]]
