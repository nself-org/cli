# Public Roadmap — nSelf

This document tracks planned features and upcoming releases across the nSelf ecosystem. It is updated after each sprint cycle.

**Current version:** v1.0.11 LTS  
**Legend:** ✅ Done | 🚧 In Progress | 🔲 Planned | 🚫 Deferred

---

## Upcoming: v1.1.0

Target: Q2 2026

| Feature | Repo | Status | Notes |
|---------|------|--------|-------|
| nFamily social app | nfamily | 🔲 Planned | Family social media, private; bundle $0.99/mo |
| ClawDE bundle launch | clawde | 🔲 Planned | cloud sync, mobile twin, team features at $0.99/mo |
| Admin multi-user support | admin | 🔲 Planned | Multi-operator via `NSELF_ADMIN_MULTIUSER`. Single-operator in v1.x |
| nCloud managed hosting | web/cloud | 🔲 Planned | Dedicated Hetzner VPS, $2/mo margin + at-cost infra |
| Plugin SDK public release | cli/sdk | 🔲 Planned | plugin-sdk-go + plugin-sdk-ts as standalone packages |
| GitHub Discussions on all repos | all | 🔲 Planned | Q&A, Ideas, Show-and-tell, Announcements categories |
| Benchmark harness | .github/benchmarks | 🔲 Planned | Setup-time + RPS + cost comparison vs Supabase/Nhost/PocketBase |

---

## Upcoming: v1.2.0

Target: Q3 2026

| Feature | Repo | Status | Notes |
|---------|------|--------|-------|
| nFamily core features | nfamily | 🔲 Planned | Social feed, family tree, photo albums |
| nCloud Light tier | web/cloud | 🔲 Planned | Shared Docker hosting at $1-2/mo |
| Voice calls (continuous) | nclaw | 🔲 Planned | LiveKit server-side wiring |
| iOS/Android share extension | nclaw | 🔲 Planned | Native share integration |

---

## Recently Shipped: v1.0.11

- Trust install idempotency (ports/dns/ssl) — no more stacked OS admin dialogs
- Admin Prompt Hygiene doctrine enforced
- Compliance tooling: GDPR export/forget/consent, audit log, SOC2 evidence, SBOM
- Benchmark harness + comparison vs competitors

---

## Suggestions and Feedback

Open a [GitHub Discussion](https://github.com/nself-org/cli/discussions) under the **Ideas** category. Roadmap items are reviewed after each sprint cycle. The founder makes final prioritization decisions (see [GOVERNANCE.md](GOVERNANCE.md)).

For security-related requests, use [SECURITY.md](SECURITY.md) — do not open a public issue.

