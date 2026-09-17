# Plugin Catalog

ɳSelf ships with **138 plugins** total: 25 free (MIT) and 62 pro (license-gated). Each plugin adds a self-contained backend capability to the stack.

Free plugins install with no license. Pro plugins require a valid `nself_pro_*` key set via [[cmd-license]].

- **Install:** `nself plugin install <name>`
- **List installed:** `nself plugin list`
- **Full manager docs:** [[cmd-plugin]]
- **Pricing:** see PPI `F07-PRICING-TIERS.md`; buy bundles on `nself.org`

---

## Free plugins (25)

MIT licensed. No license key required. Source: `plugins/free/` in the `plugins` repo.

| Plugin | Description |
|--------|-------------|
| [[plugin-access-controls]] | Row and column-level access policies in Hasura |
| [[plugin-admin-api]] | Administrative REST API layer |
| [[plugin-analytics]] | Event tracking and simple funnels |
| [[plugin-auth]] | Extended authentication workflows |
| [[plugin-backup]] | Scheduled database backups |
| [[plugin-calendar]] | Calendar event models and CRUD |
| [[plugin-cdn]] | CDN integration helpers |
| [[plugin-content-acquisition]] | Public content ingestion and normalization |
| [[plugin-content-progress]] | Per-user content progress tracking |
| [[plugin-ddns]] | Dynamic DNS updater |
| [[plugin-feature-flags]] | Runtime feature flag service |
| [[plugin-file-processing]] | Server-side file transforms |
| [[plugin-geolocation]] | IP and coordinate lookup |
| [[plugin-github]] | GitHub webhook and API bridge |
| [[plugin-home]] | Home automation primitives |
| [[plugin-invitations]] | Email-based invite flows |
| [[plugin-jobs]] | Scheduled job runner |
| [[plugin-knowledge-base]] | Docs and article CMS |
| [[plugin-link-preview]] | Open Graph unfurl |
| [[plugin-mdns]] | Local network service discovery |
| [[plugin-monitoring]] | Metric aggregation |
| [[plugin-notifications]] | In-app notification queue |
| [[plugin-object-storage]] | S3-compatible object layer |
| [[plugin-observability]] | Tracing and structured logs |
| [[plugin-torrent-manager]] | Torrent client management |

---

## Pro plugins (62)

License-gated. Install with a valid `nself_pro_*` key. Source: `bundles/paid/` in the `plugins-pro` repo.

### ɳClaw bundle ($0.99/mo)

| Plugin | Description |
|--------|-------------|
| [[plugin-nself-ai-gateway]] | Provider key vault, request routing, quota enforcement (port 3761) |
| [[plugin-nself-ai-mcp]] | PTY session relay for AI CLI binaries (port 3760) |
| [[plugin-nself-ai-mcp]] | MCP tool server for AI gateway (port 3762) |
| [[plugin-browser]] | Headless browser as a service |
| [[plugin-claw]] | Core ɳClaw assistant runtime |
| [[plugin-claw-budget]] | Budget, ledger, and financial memory |
| [[plugin-claw-news]] | News aggregation for the assistant |
| [[plugin-claw-web]] | Web dashboard for ɳClaw |
| [[plugin-cron]] | Named cron scheduler |
| [[plugin-google]] | Google API adapter (Gmail, Calendar, Drive) |
| [[plugin-mux]] | Email pipeline and classification |
| [[plugin-notify]] | Push, SMS, email delivery |
| [[plugin-post]] | Post-processing workers |
| [[plugin-voice]] | Speech-to-text and text-to-speech |

### ɳChat bundle ($0.99/mo)

| Plugin | Description |
|--------|-------------|
| [[plugin-bots]] | Chatbot runtime |
| [[plugin-chat]] | Chat service core |
| [[plugin-livekit]] | Real-time video and audio |
| [[plugin-moderation]] | Content moderation tools |
| [[plugin-realtime]] | WebSocket broadcast layer |
| [[plugin-recording]] | Call and stream recording |

### ɳFamily bundle ($0.99/mo)

| Plugin | Description |
|--------|-------------|
| [[plugin-activity-feed]] | Family activity timelines |
| [[plugin-cms]] | Content management |
| [[plugin-photos]] | Photo library |
| [[plugin-social]] | Private social feed |

### ɳTV bundle ($0.99/mo)

| Plugin | Description |
|--------|-------------|
| [[plugin-epg]] | Electronic program guide |
| [[plugin-game-metadata]] | Game library metadata |
| [[plugin-media-processing]] | Media transcoding and thumbnail generation |
| [[plugin-podcast]] | Podcast ingestion and publishing |
| [[plugin-retro-gaming]] | Retro game library integration |
| [[plugin-rom-discovery]] | ROM scanner |
| [[plugin-stream-gateway]] | Streaming gateway |
| [[plugin-streaming]] | Media streaming service |
| [[plugin-subtitle-manager]] | Subtitle download and sync |
| [[plugin-tmdb]] | TheMovieDB metadata |

### Infrastructure and integrations (unbundled pro)

| Plugin | Description |
|--------|-------------|
| [[plugin-analytics]] | Pro-tier analytics (replaces free variant when present) |
| [[plugin-cloudflare]] | Cloudflare DNS, Workers, R2 |
| [[plugin-compliance]] | GDPR, SOC2 audit helpers |
| [[plugin-devices]] | Device registry |
| [[plugin-documents]] | Document storage |
| [[plugin-donorbox]] | Donorbox donation integration |
| [[plugin-entitlements]] | Entitlement and subscription engine |
| [[plugin-geocoding]] | Address to coordinates |
| [[plugin-github-runner]] | Self-hosted GitHub Actions runner |
| [[plugin-idme]] | ID.me identity verification |
| [[plugin-linkedin]] | LinkedIn API adapter |
| [[plugin-meetings]] | Meetings and bookings |
| [[plugin-mlflow]] | MLflow model registry |
| [[plugin-paypal]] | PayPal payments |
| [[plugin-search]] | MeiliSearch pro extension |
| [[plugin-shopify]] | Shopify integration |
| [[plugin-sports]] | Sports data feeds |
| [[plugin-stripe]] | Stripe billing |
| [[plugin-support]] | Support ticketing |
| [[plugin-tokens]] | API token issuance |
| [[plugin-vpn]] | VPN server |
| [[plugin-web3]] | Web3 wallet adapters |
| [[plugin-webhooks]] | Outbound webhooks (pro tier) |
| [[plugin-workflows]] | Workflow engine |

---

## See Also

- [[cmd-plugin]], plugin CLI reference
- [[cmd-license]], license key management
- [[Commands]], full command index

← [[Home]]
