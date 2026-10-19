# ClawDE Plugin Bundle

## Contents

- [What Is ClawDE](#what-is-clawde)
- [Plugins Included](#plugins-included)
- [How They Work Together](#how-they-work-together)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Related Pages](#related-pages)

## What Is ClawDE

ClawDE is the marketing name for the bundle of plugins that adds cloud sync, mobile twin, premium models, and team features to [ClawDE](https://github.com/nself-org/clawde), the AI dev environment (Flutter desktop + mobile).

ClawDE itself runs free locally with limited functionality. The ClawDE bundle unlocks server-side sync between desktop and mobile, premium model access, and team workspaces.

**Status note:** Bundle membership is provisional pending user verification (see [F06-BUNDLE-INVENTORY](https://github.com/nself-org/nself/blob/main/.claude/docs/sport/F06-BUNDLE-INVENTORY.md)). The plugin list below is the candidate set per F06's open question.

## Plugins Included

Candidate set (licensed plugins, all `tier: pro`).

| Plugin | Tier | Language | What It Does |
|--------|------|----------|-------------|
| [realtime](plugin-realtime) | pro | Go | WebSocket sync between desktop and mobile clients |
| [auth](plugin-auth) | pro | Go | Account management, MFA, session sharing across devices |
| [cms](plugin-cms) | pro | Go | Project / workspace storage and team-shared configs |
| [notify](plugin-notify) | pro | Go | Push notifications for build status and team events |

Final membership pending user verification, TRAP 10 in [F06-BUNDLE-INVENTORY](https://github.com/nself-org/nself/blob/main/.claude/docs/sport/F06-BUNDLE-INVENTORY.md).

## How They Work Together

```
ClawDE Desktop ↔ realtime ↔ ClawDE Mobile
       ↓
     auth (account + MFA + session)
       ↓
     cms (project sync, team workspace)
       ↓
     notify (build complete, team event)
```

### Sync

1. **realtime** keeps desktop and mobile clients in sync via WebSocket: open files, recent commands, terminal output
2. **auth** ties devices to a single account with MFA and shared session state

### Storage

1. **cms** stores per-project settings, snippets, and team-shared workspace configs

### Notifications

1. **notify** sends push notifications when builds complete, team members @-mention, or remote agent tasks finish

## Installation

```bash
# Basic tier or higher (all ClawDE plugins are tier: pro)
nself license set nself_pro_xxxxx...
nself plugin install realtime auth cms notify

# Rebuild and start
nself build && nself start
```

ClawDE subscription is $0.99/mo or $9.99/yr (separately bundled, see [Plugin-Licensing](Plugin-Licensing)).

## Getting Started

### Prerequisites

- ɳSelf CLI installed and a project initialized (`nself init`)
- Backend running (`nself start`)
- ClawDE subscription
- ClawDE desktop installed locally; ClawDE mobile installed on at least one device

### Step 1: Install the plugins

Install the candidate plugin set with the command above.

### Step 2: Configure auth

Set MFA and session sharing in your `.env`. See [plugin-auth](plugin-auth).

### Step 3: Connect your devices

Sign in to the same account on ClawDE desktop and ClawDE mobile. The clients pair automatically once both are signed in and the backend is reachable.

### Step 4: Enable push notifications

Set push credentials for each platform. See [plugin-notify](plugin-notify).

### Troubleshooting

- **Devices not pairing:** verify both clients reach the same backend URL and the JWTs come from the same auth realm
- **Sync lag:** check `realtime` WebSocket connection and TURN credentials if behind NAT
- **Push silent:** confirm push credentials match the platform (APNs, FCM)

## Related Pages

- [Plugin Overview](Plugin-Overview), all plugins and tiers
- [Plugin Install](Plugin-Install), how to install plugins
- [Plugin Licensing](Plugin-Licensing), license keys and tiers
- [ClawDE repo](https://github.com/nself-org/clawde), the AI dev environment client
- Individual plugin pages: [realtime](plugin-realtime), [auth](plugin-auth), [cms](plugin-cms), [notify](plugin-notify)
