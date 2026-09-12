**[[Home]]**

---

**Getting Started**
- [[Getting-Started]]
- [[Installation]]
- [[Quick-Start]]
- [[First-Project]]
- [[Upgrading-from-v1]]
- [[FAQ]]

---

**Commands**
- [[Commands]], Overview
- _Lifecycle:_ [[cmd-init]] · [[cmd-build]] · [[cmd-start]] · [[cmd-stop]] · [[cmd-restart]] · [[cmd-dev]]
- _Monitoring:_ [[cmd-status]] · [[cmd-logs]] · [[cmd-health]] · [[cmd-urls]] · [[cmd-doctor]] · [[cmd-monitor]] · [[cmd-alerts]] · [[cmd-sentry]] · [[cmd-watchdog]]
- _Data:_ [[cmd-db]] · [[cmd-backup]] · [[cmd-dr]] · [[cmd-queue]] · [[cmd-webhooks]]
- _Config:_ [[cmd-config]] · [[cmd-service]] · [[cmd-env]] · [[cmd-promote]]
- _Networking:_ [[cmd-ssl]] · [[cmd-trust]] · [[cmd-dns-setup]]
- _Security:_ [[cmd-access]] · [[cmd-security]] · [[cmd-secrets]]
- _Tenancy:_ [[cmd-tenant]] · [[cmd-billing]]
- _Plugins:_ [[cmd-plugin]] · [[cmd-license]] · [[cmd-dogfood]] (extracted, CLI-R11) · [[cmd-k8s]] (extracted, CLI-R11) · [[cmd-encryption]] (extracted, CLI-R11) · [[cmd-waf]] (extracted, CLI-R11) · [[cmd-federation]] (extracted, CLI-R11) · [[cmd-mail]] (extracted, CLI-R11) · [[cmd-dlq]] (extracted, CLI-R11)
- _AI:_ [[cmd-ai]] · [[cmd-claw]] · [[cmd-model]]
- _Templates:_ [[cmd-template]]
- _Utilities:_ [[cmd-exec]] · [[cmd-clean]] · [[cmd-reset]] · [[cmd-update]] · [[cmd-upgrade]] · [[cmd-version]] · [[cmd-admin]] · [[cmd-migrate]] · [[cmd-migrate-firebase]] · [[cmd-migrate-supabase]] · [[cmd-completion]]

---

**Features**
- [[Features]], Overview
- [[Feature-Auth]]
- [[Feature-Storage]]
- [[Feature-Search]]
- [[Feature-Functions]]
- [[Feature-Email]]
- [[Feature-Monitoring]]
- [[Feature-Plugins]]
- [[Feature-nClaw]], AI Assistant
- [[Feature-nChat]], Messaging
- [[Feature-nTV]], Media Player
- [[Feature-nFamily]], Family Social
- [[Feature-nCloud]], Managed Hosting
- [[Feature-Memory-Rooms]], Knowledge Organization
- [[Feature-Agent-Dashboard]], Agent Metrics
- [[Feature-Image-Generation]], AI Image Generation

---

**Configuration**
- [[Configuration]], Overview
- [[Config-Env-Vars]]
- [[Config-Postgres]]
- [[Config-Hasura]]
- [[Config-Auth]]
- [[Config-Nginx]]
- [[Config-Optional-Services]]
- [[Config-Custom-Services]]
- [[Config-System]]

---

**Plugins (87 + 10 monitoring)**
- [[Plugin-Overview]]
- [[Plugin-Catalog]]
- [[Plugin-Install]]
- [[Plugin-Licensing]]
- [[Plugin-Architecture]]
- [[Plugin-Dev-Guide]]

<details><summary>Free (25)</summary>

 - [[plugin-backup]]
 - [[plugin-content-acquisition]]
 - [[plugin-content-progress]]
 - [[plugin-cron]]
 - [[plugin-donorbox]]
 - [[plugin-feature-flags]]
 - [[plugin-github]]
 - [[plugin-github-runner]]
 - [[plugin-invitations]]
 - [[plugin-jobs]]
 - [[plugin-link-preview]]
 - [[plugin-mdns]]
 - [[plugin-mlflow]]
 - [[plugin-monitoring]]
 - [[plugin-notifications]]
 - [[plugin-notify]]
 - [[plugin-paypal]]
 - [[plugin-search]]
 - [[plugin-shopify]]
 - [[plugin-stripe]]
 - [[plugin-subtitle-manager]]
 - [[plugin-tokens]]
 - [[plugin-torrent-manager]]
 - [[plugin-vpn]]
 - [[plugin-webhooks]]

</details>

<details><summary>Pro (62)</summary>

 - [[plugin-access-controls]]
 - [[plugin-activity-feed]]
 - [[plugin-admin-api]]
 - [[plugin-nself-ai-gateway]]
 - [[plugin-nself-ai-mcp]]
 - [[plugin-nself-ai-mcp]]
 - [[plugin-analytics]]
 - [[plugin-auth]]
 - [[plugin-backup-pro]]
 - [[plugin-bots]]
 - [[plugin-browser]]
 - [[plugin-calendar]]
 - [[plugin-cdn]]
 - [[plugin-chat]]
 - [[plugin-claw]]
 - [[plugin-claw-budget]]
 - [[plugin-claw-news]]
 - [[plugin-claw-web]]
 - [[plugin-cloudflare]]
 - [[plugin-cms]]
 - [[plugin-compliance]]
 - [[plugin-cron-pro]]
 - [[plugin-ddns]]
 - [[plugin-devices]]
 - [[plugin-documents]]
 - [[plugin-donorbox-pro]]
 - [[plugin-entitlements]]
 - [[plugin-epg]]
 - [[plugin-file-processing]]
 - [[plugin-game-metadata]]
 - [[plugin-geocoding]]
 - [[plugin-geolocation]]
 - [[plugin-google]]
 - [[plugin-home]]
 - [[plugin-idme]]
 - [[plugin-knowledge-base]]
 - [[plugin-linkedin]]
 - [[plugin-livekit]]
 - [[plugin-media-processing]]
 - [[plugin-meetings]]
 - [[plugin-moderation]]
 - [[plugin-mux]]
 - [[plugin-notify-pro]]
 - [[plugin-object-storage]]
 - [[plugin-observability]]
 - [[plugin-paypal-pro]]
 - [[plugin-photos]]
 - [[plugin-podcast]]
 - [[plugin-post]]
 - [[plugin-realtime]]
 - [[plugin-recording]]
 - [[plugin-retro-gaming]]
 - [[plugin-rom-discovery]]
 - [[plugin-shopify-pro]]
 - [[plugin-social]]
 - [[plugin-sports]]
 - [[plugin-stream-gateway]]
 - [[plugin-streaming]]
 - [[plugin-stripe-pro]]
 - [[plugin-support]]
 - [[plugin-tmdb]]
 - [[plugin-voice]]
 - [[plugin-web3]]
 - [[plugin-workflows]]

</details>

<details><summary>Planned (26)</summary>

 - `plugin-audit`
 - `plugin-blog`
 - `plugin-checkout`
 - `plugin-commerce`
 - `plugin-drm`
 - `plugin-export`
 - `plugin-flow`
 - `plugin-import`
 - `plugin-ldap`
 - `plugin-mailgun`
 - `plugin-media`
 - `plugin-oauth-providers`
 - `plugin-pages`
 - `plugin-postmark`
 - `plugin-rate-limit`
 - `plugin-reports`
 - `plugin-saml`
 - `plugin-scheduler`
 - `plugin-sendgrid`
 - `plugin-sso`
 - `plugin-subscription`
 - `plugin-thumb`
 - `plugin-transcoder`
 - `plugin-twilio`
 - `plugin-waf`
 - `plugin-watermark`

</details>

---

**Guides**
- [[Guide-Production-Deployment]]
- [[Guide-SSL-Setup]]
- [[Guide-Multi-Tenancy]]
- [[Guide-Security-Hardening]]
- [[Guide-Monitoring-Setup]]
- [[Guide-Backup-Restore]]
- [[Guide-Custom-Services]]
- [[Guide-Migration-from-v1]]

---

**Architecture**
- [[Architecture]]
- [[Service-Graph]]
- [[Compose-Generation]]
- [[Nginx-Generation]]
- [[Plugin-Architecture]]

---

**Reference**
- [[API-Reference]]
- [[error-codes]], Error Codes

---

**Licensing**
- [[Licensing-Guide]]
- [[Plugin-Licensing]]

---

**Security**
- [[Security-Policy]]
- [[Security-Architecture]]
- [[Security-Hardening]]

---

**Brand**
- [[Branding]]
- [[Brand-Assets]]
- [[Logo-Usage]]
- [[Color-Palette]]

---

**Operations**
- [[operations/release-cascade]], Release Cascade
- [[operations/self-healing]], Self-Healing Schema
- [[operations/redis-tuning]], Redis Pool Tuning
- [[operations/meilisearch-warmup]], MeiliSearch Warm-Up
- [[operations/jwt-rotation]], JWT Key Rotation
- [[operations/windows-wsl2-setup]], Windows / WSL2 Setup
- [[operations/gemini-oauth-reauth]], Gemini OAuth Reauth

---

**Contributing**
- [[Contributing]]
- [[Dev-Setup]]
- [[Plugin-Dev-Guide]]
- [[Release-Process]]

---

**Admin**
- [[USER-ACTION-QUEUE]], Pending Admin Actions

---

[[Changelog]]

---

<!-- BEGIN GENERATED:command-list -->

**All commands (51)**

- _A:_ [[cmd-access]] · [[cmd-account]] · [[cmd-admin]]
- _B:_ [[cmd-backup]] · [[cmd-build]] · [[cmd-bundle]]
- _C:_ [[cmd-ci]] · [[cmd-clean]] · [[cmd-completion]] · [[cmd-config]]
- _D:_ [[cmd-db]] · [[cmd-deploy]] · [[cmd-dev]] · [[cmd-doctor]]
- _E:_ [[cmd-env]] · [[cmd-exec]]
- _F:_ [[cmd-functions]]
- _G:_ [[cmd-generate]]
- _H:_ [[cmd-health]] · [[cmd-help-topics]]
- _I:_ [[cmd-init]] · [[cmd-install]]
- _L:_ [[cmd-license]] · [[cmd-login]] · [[cmd-logout]] · [[cmd-logs]]
- _M:_ [[cmd-man]] · [[cmd-mcp]] · [[cmd-migrate]]
- _O:_ [[cmd-oauth]] · [[cmd-ops]]
- _P:_ [[cmd-plugin]] · [[cmd-promote]]
- _R:_ [[cmd-remove]] · [[cmd-reset]] · [[cmd-restart]]
- _S:_ [[cmd-secrets]] · [[cmd-security]] · [[cmd-self-heal]] · [[cmd-server]] · [[cmd-service]] · [[cmd-start]] · [[cmd-status]] · [[cmd-stop]]
- _T:_ [[cmd-telemetry]] · [[cmd-template]] · [[cmd-trust]]
- _U:_ [[cmd-update]] · [[cmd-urls]]
- _V:_ [[cmd-verify-sbom]] · [[cmd-version]]

<!-- END GENERATED:command-list -->
