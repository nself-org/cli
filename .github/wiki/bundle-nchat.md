# ɳChat Plugin Bundle

## Contents

- [What Is ɳChat](#what-is-nchat)
- [Plugins Included](#plugins-included)
- [How They Work Together](#how-they-work-together)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Related Pages](#related-pages)

## What Is ɳChat

ɳChat is the marketing name for the bundle of plugins that powers the [chat](https://github.com/nself-org/nchat) reference app. It is not a repo or a single plugin. When the bundle is installed, your ɳSelf backend gains real-time messaging, video and audio calls, call recording, moderation, bots, and authentication, every feature of a modern team chat product.

ɳChat pairs with the [chat](https://github.com/nself-org/nchat) repo, an open-source self-hosted messaging app (Next.js + Capacitor + Electron + Tauri). Without the bundle the chat app still runs core messaging; pro features (video, recording, advanced moderation, bots) hide gracefully via runtime feature detection.

## Plugins Included

The bundle's licensed plugins are all `tier: pro` (Basic tier or higher).

| Plugin | Tier | Language | What It Does |
|--------|------|----------|-------------|
| [chat](plugin-chat) | pro | Go | Channels, DMs, threads, reactions, pins, search |
| [livekit](plugin-livekit) | pro | Go | Audio and video call rooms (WebRTC SFU) |
| [recording](plugin-recording) | pro | Go | Call recording with playback |
| [moderation](plugin-moderation) | pro | Go | Auto-moderation, word filters, spam detection, user reports |
| [bots](plugin-bots) | pro | Go | Bot framework with slash commands and event subscriptions |
| [realtime](plugin-realtime) | pro | Go | WebSocket presence, typing indicators, read receipts |
| [auth](plugin-auth) | pro | Go | Advanced auth: SSO, MFA, session management |

## How They Work Together

```
Client (chat/) → realtime + chat → channels, DMs, threads
                       ↓
         livekit (calls) + recording (capture) + moderation (filter) + bots (automation)
                       ↓
                       auth (SSO, MFA)
```

### Messaging

1. **chat** stores channels, DMs, threads, messages, reactions, pins, attachments
2. **realtime** maintains WebSocket connections for typing indicators, presence, read receipts, live updates

### Voice and video

1. **livekit** runs the WebRTC SFU for audio and video call rooms
2. **recording** captures audio and video streams to MP4 with playback support

### Safety

1. **moderation** scans messages for banned content, spam patterns, and rate-limit abuse
2. **bots** lets administrators install custom bots that respond to slash commands or react to events

### Identity

1. **auth** provides SSO connectors (SAML, OIDC), MFA enforcement, and session management beyond the default `auth` service

## Installation

```bash
# Basic tier or higher (all nChat plugins are tier: pro)
nself license set nself_pro_xxxxx...
nself plugin install chat livekit recording moderation bots realtime auth

# Rebuild and start
nself build && nself start
```

Basic tier is $0.99/mo or $9.99/yr. See [Plugin-Licensing](Plugin-Licensing) for the tier comparison.

## Getting Started

### Prerequisites

- ɳSelf CLI installed and a project initialized (`nself init`)
- Backend running (`nself start`)
- Basic tier license key or higher
- TURN/STUN credentials for `livekit` if running across NAT
- Object storage configured for `recording` (MinIO comes free with the ɳSelf stack)

### Step 1: Install the plugins

Install all 7 ɳChat plugins with the command above.

### Step 2: Configure LiveKit

Edit your `.env` to set LiveKit API keys and TURN credentials. See [plugin-livekit](plugin-livekit).

### Step 3: Configure moderation

Set word lists and rate limits for `moderation`. See [plugin-moderation](plugin-moderation).

### Step 4: Connect the chat client

Clone [nself-org/chat](https://github.com/nself-org/nchat), copy `.backend.example/` to `.backend/`, set the license key, and run `nself build && nself start` from `.backend/`. Then `pnpm dev` in the frontend.

### Troubleshooting

- **Calls fail to connect:** verify LiveKit ports are reachable and TURN credentials are valid
- **Recordings missing:** check object storage credentials and that the recording container has write access
- **Moderation too aggressive:** tune word lists and rate limits per channel

## Related Pages

- [Plugin Overview](Plugin-Overview), all plugins and tiers
- [Plugin Install](Plugin-Install), how to install plugins
- [Plugin Licensing](Plugin-Licensing), license keys and tiers
- [Feature-ɳChat](Feature-ɳChat), feature overview
- Individual plugin pages: [chat](plugin-chat), [livekit](plugin-livekit), [recording](plugin-recording), [moderation](plugin-moderation), [bots](plugin-bots), [realtime](plugin-realtime), [auth](plugin-auth)
