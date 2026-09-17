# LinkedIn Plugin

> LinkedIn publishing integration with OAuth 2.0 and post history. **Pro plugin. Requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Basic | $0.99/mo | $9.99/yr | Yes |
| Pro | $1.99/mo | $19.99/yr | Yes |
| Elite | $4.99/mo | $49.99/yr | Yes |
| Business | $9.99/mo | $99.99/yr | Yes |
| Business+ | $49.99/mo | $499.99/yr | Yes |
| Enterprise | $99.99/mo | $999.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

Not currently bundled. Purchase a tier subscription (Basic and up) for access.

Or get all bundles + all apps via **ɳSelf+** ($49.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install linkedin
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

The LinkedIn plugin connects a LinkedIn account via OAuth 2.0, posts to a member's LinkedIn feed with optional image attachments, and tracks post history for later reference. It also exposes a Claw tool descriptor so ɳClaw can publish on the user's behalf.

OAuth tokens are stored per `source_account_id`, so multi-tenant ɳSelf installs keep each user's LinkedIn credentials isolated. The plugin is currently in `beta` status.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | — | Postgres connection string (required) |
| `LINKEDIN_CLIENT_ID` | — | LinkedIn OAuth app client ID (required) |
| `LINKEDIN_CLIENT_SECRET` | — | LinkedIn OAuth app client secret (required) |
| `LINKEDIN_REDIRECT_URI` | — | OAuth callback URL (required) |
| `LINKEDIN_INTERNAL_SECRET` | — | Internal request signing secret (required) |
| `PORT` | `3722` | LinkedIn service port |
| `BIND_ADDRESS` | `127.0.0.1` | Bind address for the service |
| `NSELF_PLUGIN_LICENSE_KEY` | — | License key (read from CLI by default) |

## Ports

| Port | Purpose |
|------|---------|
| 3722 | LinkedIn service REST API |

## Database Schema

2 tables added to your Postgres database (prefix: `np_linkedin_`):

- `np_linkedin_tokens`, OAuth access and refresh tokens per account
- `np_linkedin_posts`, published post history with status

## REST API

```
GET  /health                       — Health check
GET  /oauth/start                  — Begin OAuth connection
GET  /oauth/callback               — Complete OAuth handshake
POST /posts                        — Publish a post to the connected feed
GET  /posts                        — List published posts
```

## Examples

### Publish a text post

```bash
curl -X POST http://localhost:3722/posts \
  -H "Content-Type: application/json" \
  -d '{"text":"New ɳSelf release shipping today.","visibility":"PUBLIC"}'
```

### List recent posts

```bash
curl http://localhost:3722/posts
```

## Source

Source-available (license required to run): [`bundles/paid/linkedin/`](https://github.com/nself-org/bundles/tree/main/paid/linkedin)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [[plugin-post]], multi-platform publisher (includes LinkedIn as a target)
- [[plugin-google]], similar OAuth-driven integration pattern
- [[Plugin-Licensing]], tier comparison
- [[Plugin-Overview]], full plugin index

← [[Plugin-Overview]] | [[Home]] →
