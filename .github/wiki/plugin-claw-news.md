# Claw News Plugin

> News aggregation, AI summarization, and breaking-news alerts for ɳClaw. **Pro plugin. Requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| ɳClaw Bundle | $0.99/mo | $9.99/yr | Yes |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes (all bundles + all apps) |

**Minimum tier:** any bundle that includes this plugin (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

This plugin is included in the following bundles:

- **ɳClaw Bundle** ($0.99/mo), see [[bundle-nclaw]]

Or get all bundles + all apps via **ɳSelf+** ($3.99/mo or $39.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install claw-news
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

The Claw News plugin polls RSS and Atom feeds, classifies incoming articles by topic, summarizes them with the `ai` plugin, generates daily digests, and fires breaking-news alerts via the `notify` plugin. It exposes article search, source management, and topic configuration through a REST API and through ɳClaw tools.

Sentiment analysis runs alongside summarization so the assistant can answer questions like "what changed in the AI space this week" without re-reading every article. Multi-app isolation is supported via the `source_account_id` column, so multiple ɳClaw users on one ɳSelf instance see only their own subscriptions.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | — | Postgres connection string (required) |
| `PORT` | `3718` | News service port |
| `PLUGIN_AI_INTERNAL_URL` | — | Internal URL for the `ai` plugin (summarization) |
| `PLUGIN_NOTIFY_URL` | — | Internal URL for the `notify` plugin (alerts) |

## Ports

| Port | Purpose |
|------|---------|
| 3718 | News service REST API |

## Database Schema

4 tables added to your Postgres database (prefix: `np_news_`):

- `np_news_sources`, RSS/Atom feed source registrations
- `np_news_articles`, ingested articles with summaries and sentiment
- `np_news_topics`, topic definitions and keyword filters
- `np_news_alerts`, breaking-news alert rules and history

## REST API

```
GET  /health                     — Health check
GET  /articles                   — List and search news articles
POST /sources                    — Register an RSS feed source
GET  /sources                    — List registered sources
POST /topics                     — Create a topic with keyword filters
GET  /topics                     — List topics
POST /alerts                     — Configure a breaking-news alert
POST /digest                     — Generate an AI digest on demand
```

## Examples

### Subscribe to a news feed

```bash
curl -X POST http://localhost:3718/sources \
  -H "Content-Type: application/json" \
  -d '{"url":"https://hnrss.org/frontpage","title":"Hacker News"}'
```

### Generate a daily digest

```bash
curl -X POST http://localhost:3718/digest \
  -H "Content-Type: application/json" \
  -d '{"topics":["AI","Open Source"],"window_hours":24}'
```

## Source

Source-available (license required to run): [`bundles/paid/claw-news/`](https://github.com/nself-org/bundles/tree/main/paid/claw-news)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [[plugin-claw]], ɳClaw assistant runtime that consumes news
- [[plugin-nself-ai-gateway]], AI gateway providing summarization for article ingestion
- [[plugin-notify]], delivers breaking-news alerts
- [[bundle-nclaw]], bundle that includes this plugin
- [[Plugin-Licensing]], tier comparison
- [[Plugin-Overview]], full plugin index

← [[Plugin-Overview]] | [[Home]] →
