# ɳClaw Plugin Bundle

## Contents

- [What Is ɳClaw](#what-is-nclaw)
- [Plugins Included](#plugins-included)
- [How They Work Together](#how-they-work-together)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Related Pages](#related-pages)

## What Is ɳClaw

ɳClaw is the marketing name for the bundle of plugins that powers the [ɳClaw personal AI assistant](https://github.com/nself-org/nclaw). It is not a repo or a single plugin. When the bundle is installed, your ɳSelf backend gains the AI gateway, assistant runtime, voice I/O, browser tool, email and calendar integrations, scheduled jobs, and push notifications that the ɳClaw client app needs to operate.

ɳClaw pairs with the [claw](https://github.com/nself-org/nclaw) repo (Flutter client for iOS, Android, macOS, web).

## Plugins Included

The bundle's licensed plugins are mostly `tier: max` (require Pro tier or higher); a few are `tier: pro` (Basic tier or higher).

| Plugin | Tier | Language | What It Does |
|--------|------|----------|-------------|
| [nself-ai-gateway](nself-ai-gateway) | max | Go | Provider key vault, request routing, quota enforcement (port 3761) |
| [nself-ai-cc](nself-ai-cc) | max | Go | PTY session relay for AI CLI binaries: claude, codex (port 3760) |
| [claw](plugin-claw) | max | Go | AI assistant runtime: conversations, memory, knowledge graph |
| [claw-web](plugin-claw-web) | max | Go | Web client backend for the ɳClaw assistant |
| [claw-budget](plugin-claw-budget) | max | Go | Budget tracking and financial analytics for the assistant |
| [claw-news](plugin-claw-news) | pro | Go | News feed ingestion, summarization, and topic tracking |
| [voice](plugin-voice) | max | Go | Text-to-speech and speech-to-text |
| [browser](plugin-browser) | max | Go | Headless browser tool for web research and automation |
| [google](plugin-google) | pro | Go | Gmail, Calendar, Drive, Contacts integration |
| [mux](plugin-mux) | pro | Go | Email and message classification, summarization, routing |
| [notify](plugin-notify) | pro | Go | Push notifications across iOS, Android, web |
| [cron](plugin-cron) | pro | Go | Scheduled jobs and recurring tasks |

`claw-budget` and `claw-news` membership pending user verification (TRAP 10), see [F06-BUNDLE-INVENTORY](https://github.com/nself-org/nself/blob/main/.claude/docs/sport/F06-BUNDLE-INVENTORY.md).

## How They Work Together

```
Voice / Text input → AI gateway → Claw runtime → Tools (browser, google, mux, notify, cron) → Response
                                       ↓
                                Memory + Knowledge Graph
```

### Input

1. **voice** transcribes speech to text and synthesizes responses back to audio
2. **claw-web** routes web client requests into the assistant runtime

### Reasoning

1. **ai** routes prompts to the configured provider (Anthropic, OpenAI, Groq, local) with caching and streaming
2. **claw** is the assistant runtime: maintains conversation state, queries the knowledge graph, decides which tools to call

### Tools and integrations

1. **browser** runs headless web research, page fetches, structured extraction
2. **google** reads Gmail, schedules Calendar events, fetches Drive files
3. **mux** classifies and summarizes incoming email and messages
4. **notify** delivers push notifications across devices
5. **cron** schedules recurring jobs (daily summary, watch-list, etc.)
6. **claw-news** ingests RSS feeds and topic streams
7. **claw-budget** tracks budgets and produces financial summaries

## Installation

```bash
# Pro tier required for tier: max plugins (ai, claw, claw-web, voice, browser, claw-budget)
nself license set nself_pro_xxxxx...
nself plugin install ai claw claw-web claw-budget claw-news voice browser google mux notify cron

# Rebuild and start
nself build && nself start
```

Pro tier is $1.99/mo or $19.99/yr. See [Plugin-Licensing](Plugin-Licensing) for the tier comparison.

## Getting Started

### Prerequisites

- ɳSelf CLI installed and a project initialized (`nself init`)
- Backend running (`nself start`)
- Pro tier license key (Basic does not include `tier: max` plugins)
- Provider API keys (Anthropic, OpenAI, etc.) for the `ai` plugin
- Google OAuth credentials for the `google` plugin

### Step 1: Install the plugins

Install all ɳClaw plugins with the command above.

### Step 2: Configure the AI provider

Add provider keys via the gateway: `nself gateway keys add --provider anthropic --key "sk-ant-..."`. See [nself-ai-gateway](nself-ai-gateway) for full env var and key pool setup.

### Step 3: Configure integrations

Add Google OAuth credentials, push notification keys, and any other integration env vars. Each plugin documents its env vars on its own wiki page.

### Step 4: Connect a client

Point the [claw](https://github.com/nself-org/nclaw) Flutter app at your ɳSelf backend's API URL. See the claw repo's README for setup.

### Troubleshooting

- **AI calls failing:** verify `AI_API_KEY` is set and matches the provider in `AI_PROVIDER`
- **Google integration silent:** check OAuth consent screen and scopes match the `google` plugin requirements
- **No notifications:** confirm push credentials are loaded for the platforms you target

## Related Pages

- [Plugin Overview](Plugin-Overview), all plugins and tiers
- [Plugin Install](Plugin-Install), how to install plugins
- [Plugin Licensing](Plugin-Licensing), license keys and tiers
- Individual plugin pages: [nself-ai-gateway](nself-ai-gateway), [nself-ai-cc](nself-ai-cc), [claw](plugin-claw), [claw-web](plugin-claw-web), [claw-budget](plugin-claw-budget), [claw-news](plugin-claw-news), [voice](plugin-voice), [browser](plugin-browser), [google](plugin-google), [mux](plugin-mux), [notify](plugin-notify), [cron](plugin-cron)
