# Plugin Overview

## Contents

- [Plugin Tiers](#plugin-tiers)
- [Plugin Categories](#plugin-categories)
- [How Plugins Work](#how-plugins-work)
- [Related Pages](#related-pages)

ɳSelf plugins extend your backend stack with additional services and capabilities. Install them with a single command. No manual Docker configuration required.

## Plugin Tiers

| Tier | Price | Includes |
|------|-------|---------|
| **Free** | $0 | Core CLI + free plugins (MIT licensed) |
| **Basic** | $0.99/mo or $9.99/yr | The standard licensed plugins (excludes Pro-tier AI suite) |
| **Pro** | $1.99/mo or $19.99/yr | Basic + AI suite (ai, browser, claw, claw-budget, claw-web, post, voice) |
| **Elite** | $4.99/mo or $49.99/yr | Pro + email support |
| **Business** | $9.99/mo or $99.99/yr | Elite + 24h email support + priority feature requests |
| **Business+** | $49.99/mo or $499.99/yr | Business + dedicated support channel |
| **Enterprise** | $99.99/mo or $999.99/yr | Business+ + managed DevOps |

Annual plans save ~17% compared to monthly. Existing $9.99/yr keys are grandfathered to the Basic tier.

## Plugin Categories

### Free Plugins, MIT Licensed

No license key required. Install with `nself plugin install {name}`.

| Category | Plugins |
|----------|---------|
| Background Jobs | jobs, cron |
| Search | search |
| Feature Management | feature-flags, webhooks |
| Identity | invitations, tokens |
| Integrations | github, github-runner, donorbox, paypal, shopify, stripe |
| Content | content-acquisition, content-progress, torrent-manager, subtitle-manager |
| Network | vpn, mdns, link-preview |
| Notifications | notifications, notify |
| Infrastructure | backup, monitoring, mlflow |

### Licensed Plugins, License Key Required

Require a valid ɳSelf license key. See [[Plugin-Licensing]].

| Category | Count | Plugins |
|----------|-------|---------|
| AI & Automation | 8 | ai, claw, claw-budget, claw-news, claw-web, mux, workflows, bots |
| Communication | 6 | chat, livekit, streaming, voice, podcast, realtime |
| Media & Processing | 6 | media-processing, file-processing, epg, photos, recording, stream-gateway |
| Content & Social | 8 | cms, social, activity-feed, moderation, knowledge-base, support, documents, calendar |
| Commerce & Payments | 6 | donorbox-pro, paypal-pro, shopify-pro, stripe-pro, entitlements, analytics |
| Authentication & Security | 5 | auth, access-controls, idme, compliance, admin-api |
| Infrastructure | 8 | browser, google, cloudflare, object-storage, cdn, observability, ddns, devices |
| Productivity | 3 | cron-pro, notify-pro, backup-pro |
| Gaming & Specialized | 9 | tmdb, game-metadata, retro-gaming, rom-discovery, sports, geocoding, geolocation, home, web3 |
| Social Automation | 3 | post, meetings, linkedin |

## How Plugins Work

Plugins inject into your ɳSelf stack via Docker Compose overlays. When you run `nself build`, all installed plugins are merged into a single `docker-compose.yml`. Each plugin:

- Runs as its own container(s) on a dedicated port
- Gets its own Postgres schema (`np_{name}`) with isolated tables
- Injects nginx routes automatically
- Declares environment variables in `plugin.json`

After installing any plugin, run `nself build` to regenerate your compose file.

## Plugin Bundles

Bundles group plugins that work together to deliver a complete feature set. Per F06-BUNDLE-INVENTORY, the canonical bundles are:

| Bundle | Price | Plugins | Use Case |
|--------|-------|---------|----------|
| [[bundle-nclaw]] | $0.99/mo | ai, claw, claw-web, mux, voice, browser, google, notify, cron, claw-budget, claw-news | AI personal assistant: email, calendar, news, budget, tools |
| [[bundle-clawde]] | $1.99/mo / $19.99/yr | TBD (cloud sync, premium models, team features) | ClawDE+ remote/mobile sync via API |
| [[bundle-nfamily]] | $0.99/mo | social, photos, activity-feed, moderation, realtime, cms, chat | Private family social, photo sharing, activity feeds |
| [[bundle-nchat]] | $0.99/mo | chat, livekit, recording, moderation, bots, realtime, auth | Messaging with video calls, bots, and moderation |

Or get all bundles + all apps via **ɳSelf+** ($49.99/yr).

### Monitoring Stack (10 services)

The free `monitoring` plugin bundles 10 services in one install: alertmanager, glitchtip, grafana, loki, otel-collector, prometheus, promtail, status, synthetics, web-vitals.

## Related Pages

- [[Plugin-Install]], How to install, remove, and update plugins
- [[Plugin-Licensing]], License keys, tiers, and validation
- [[Plugin-Dev-Guide]], Build your own plugin
- [[Plugin-Architecture]], Technical internals

---
← [[Home]] | [[_Sidebar]]
