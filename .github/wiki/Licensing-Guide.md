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

| Option | Monthly | Annual | What You Get |
|--------|---------|--------|-------------|
| **Free** | $0 | $0 | Core CLI, free plugins and the Task Bundle |
| **Any other Bundle** | $0.99/mo | $9.99/yr | The licensed plugins of that Bundle (Chat, Claw, Family, Sentry or ClawDE) |
| **ɳSelf+** | $3.99/mo | $39.99/yr | Every bundle + all apps + support |

Annual gives roughly 16% savings versus monthly (about 2 months free).

---

## Per-Product Plugin Bundles

Each ɳSelf product has a Bundle: the plugins that power it, plus its apps. "<Name> Bundle" is the self-hosted package; "ɳ<Name>" is our hosted service. Every Bundle except Task is a separate $0.99/mo or $9.99/yr purchase, and ɳSelf+ includes all of them. `nself bundle list` shows the current Bundles and `nself bundle info <bundle>` the plugins each holds.

### Task Bundle
Task management plugins and the ɳTask apps. Free, no license key. Our own deployment runs at task.nself.org.

### Chat Bundle
Messaging.

| Plugin | Purpose |
|--------|---------|
| chat | Channels, DMs, threads, reactions |
| livekit | Video and audio calls |
| recording | Call recording |
| moderation | Content filtering |
| bots | Automated responses |
| realtime | Enhanced WebSocket presence |
| auth | SSO, SAML, LDAP |

### Claw Bundle
AI personal assistant.

| Plugin | Purpose |
|--------|---------|
| ai | Multi-provider LLM routing |
| claw | Core assistant, threads, memory, tools |
| claw-web | Svelte web dashboard |
| claw-budget | Token spend limits and tracking |
| mux | Email pipeline (Gmail, rules, auto-reply) |
| voice | TTS, STT, phone calls |
| browser | Web scraping and automation |
| google | Gmail, Calendar, Drive, Sheets |
| notify | Telegram and webhook notifications |
| cron | Scheduled tasks |
| post | Social media publishing |

### Family Bundle
Private family social media.

| Plugin | Purpose |
|--------|---------|
| social | Activity feed, posts, reactions |
| photos | Photo upload, organization |
| activity-feed | Timeline aggregation |
| moderation | Parental controls |
| realtime | Live updates |
| cms | Family pages |
| chat | Family messaging |

### Sentry Bundle
Error tracking for your own apps. The free Monitoring stack (metrics, logs, traces) is separate and stays free.

### ClawDE Bundle
AI dev environment cloud sync.

Remote and mobile sync via API for the ClawDE desktop and mobile apps.

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

Shows your current plan, expiry date, and which plugin categories you can install.

### Removing Your Key

```bash
nself license remove
```

### Key Format

New keys are per-Bundle (`nself_claw_`, `nself_chat_` and so on) and are linked to your nSelf account. Older keys use these prefixes, which still work (`nself license migrate` links one to your account):

| Prefix | Meaning |
|--------|---------|
| `nself_pro_` | Legacy key |
| `nself_max_` | Legacy key |
| `nself_ent_` | Legacy key |
| `nself_owner_` | Owner/internal |

Keys must be at least 32 characters total.

---

## License Validation

Entitlement is checked server-side. The CLI never makes tier decisions locally.

```
POST https://ping.nself.org/license/validate
{"license_key": "...", "product": "plugins-pro"}
```

| Response | Meaning |
|----------|---------|
| 200 | Valid key, plugin allowed |
| 401/403/404 | Invalid key or not entitled to the plugin |

Validation results are cached at `~/.nself/license/cache`. How long a result counts depends on the plan: free keys are not cached (every command revalidates), single-Bundle keys 7 days, ɳSelf+ and owner keys 30 days. In v1.5 (`NSELF_V15=1`) the cache is checked against ping's signature before it is trusted; see [[Plugin-Security]].

### Runtime Revalidation

Installed plugins revalidate when their cached result runs out (the plan TTL above). If the licence server cannot be reached, the last good result keeps working: silently for up to 72 hours, with a warning after that, and access fails closed once the cached result is 7 days old. A licence that has expired keeps paid plugins working for a 30-day grace window before they go dormant. A warning starts 2 days before the cached result expires. Force revalidation:

```bash
nself license revalidate
```

### Offline Mode

For air-gapped environments:

```bash
NSELF_LICENSE_SKIP_VERIFY=1 nself plugin install {name}
```

---

## Upgrading and Downgrading

### Upgrade
1. Add a Bundle or move to ɳSelf+ at nself.org/pricing.
2. Run `nself license set {new-key}`.
3. Install newly available plugins: `nself plugin install ai claw mux ...`
4. Rebuild: `nself build && nself start`.

Stripe handles proration automatically. You pay the difference for the remainder of your billing cycle.

### Downgrade
1. Change your plan in the Stripe customer portal (cloud.nself.org/license).
2. Plugins your new plan no longer covers stop working at the next revalidation, when the cached result runs out (7 or 30 days, by plan).
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
