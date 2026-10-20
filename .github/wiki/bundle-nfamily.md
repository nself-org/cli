# ɳFamily Plugin Bundle

## Contents

- [What Is ɳFamily](#what-is-nfamily)
- [Plugins Included](#plugins-included)
- [How They Work Together](#how-they-work-together)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Related Pages](#related-pages)

## What Is ɳFamily

ɳFamily is the marketing name for the bundle of plugins that powers a private family social platform: shared photo timelines, activity feeds, group chat, and a moderated content layer. It is not a repo or a single plugin. The frontend client is planned (TBD).

When the bundle is installed, your ɳSelf backend gains social posts, photo storage and timelines, an activity feed engine, real-time updates, a CMS layer for shared family content, group chat, and moderation tools.

## Plugins Included

All of the bundle's plugins are licensed. A Bundle key or ɳSelf+ unlocks them.

| Plugin | Tier | Language | What It Does |
|--------|------|----------|-------------|
| [social](plugin-social) | licensed | Go | Posts, comments, reactions, follows |
| [photos](plugin-photos) | licensed | Go | Photo upload, albums, sharing, EXIF metadata |
| [activity-feed](plugin-activity-feed) | licensed | Go | Personalized activity timeline with fan-out |
| [moderation](plugin-moderation) | licensed | Go | Auto-moderation, word filters, user reports |
| [realtime](plugin-realtime) | licensed | Go | WebSocket presence, live updates |
| [cms](plugin-cms) | licensed | Go | Shared content (announcements, events, recipes) |
| [chat](plugin-chat) | licensed | Go | Channels, DMs, threads (shared with ɳChat bundle) |

`moderation`, `realtime`, and `chat` are also part of the [ɳChat bundle](bundle-nchat), install once, used by both.

## How They Work Together

```
Family member posts → social → activity-feed → realtime → other family members
                          ↓
                       photos (media)
                          ↓
                     moderation (safety)
                          ↓
                       chat (group conversations)
                          ↓
                       cms (shared content: events, recipes, news)
```

### Sharing

1. **social** stores posts, comments, reactions, and the follow graph
2. **photos** handles uploads, generates thumbnails, organizes albums, indexes EXIF metadata
3. **cms** stores shared family content (event calendar, recipes, family news)

### Distribution

1. **activity-feed** maintains a per-user timeline by fanning out post events to followers' feeds
2. **realtime** delivers live updates over WebSocket so feeds refresh without reload

### Conversation

1. **chat** runs channels, DMs, and threads for family group chat
2. **moderation** scans content for banned words and rate-abuse, supports manual reports

## Installation

```bash
# A Family Bundle key or ɳSelf+
nself license set nself_family_xxxxx...
nself plugin install social photos activity-feed moderation realtime cms chat

# Rebuild and start
nself build && nself start
```

The Family Bundle costs $0.99/mo or $9.99/yr; ɳSelf+ ($3.99/mo or $39.99/yr) includes it. See [Plugin-Licensing](Plugin-Licensing) for how keys and plans work.

## Getting Started

### Prerequisites

- ɳSelf CLI installed and a project initialized (`nself init`)
- Backend running (`nself start`)
- A Family Bundle license key or ɳSelf+
- Object storage (MinIO ships with the ɳSelf stack) for `photos`

### Step 1: Install the plugins

Install all 7 ɳFamily plugins with the command above.

### Step 2: Configure storage and limits

Edit your `.env` to set photo upload size limits and album quota. See [plugin-photos](plugin-photos).

### Step 3: Configure moderation

Set word lists, rate limits, and report routing. See [plugin-moderation](plugin-moderation).

### Step 4: Build a client

The official ɳFamily frontend is planned. Until then, the plugin APIs (REST and GraphQL via Hasura) are documented on each plugin page and can drive a custom Next.js or Flutter client.

### Troubleshooting

- **Photo uploads fail:** check MinIO credentials and bucket policy
- **Feed not updating live:** verify `realtime` WebSocket is reachable and JWT auth is configured
- **Moderation false positives:** tune `moderation` word lists per group

## Related Pages

- [Plugin Overview](Plugin-Overview), the offering and all plugins
- [Plugin Install](Plugin-Install), how to install plugins
- [Plugin Licensing](Plugin-Licensing), license keys and validation
- [bundle-nchat](bundle-nchat), shares moderation, realtime, chat
- [Feature-ɳFamily](Feature-ɳFamily), feature overview
- Individual plugin pages: [social](plugin-social), [photos](plugin-photos), [activity-feed](plugin-activity-feed), [moderation](plugin-moderation), [realtime](plugin-realtime), [cms](plugin-cms), [chat](plugin-chat)
