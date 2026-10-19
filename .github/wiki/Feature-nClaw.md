# Feature: ɳClaw (ɳClaw)

ɳClaw is a self-hosted AI personal assistant built on ɳSelf. It combines multi-provider LLM routing, a knowledge graph, email management, calendar integration, browser automation, voice interaction, and 19+ tools into a single system that runs on your own server.

**Status:** Active (shipping since Phase 223)
**Repos:** nself-org/nclaw (client apps), bundles (server plugins)
**Marketing site:** claw.nself.org
**Client platforms:** iOS, Android, macOS, Web
**Admin interface:** nself-admin ɳClaw tab (localhost:3021)
**Web dashboard:** claw-web (Svelte, served by plugin-claw-web)

---

## Core Architecture

ɳClaw is built from multiple ɳSelf plugins working together:

| Plugin | Role |
|--------|------|
| `ai` | Multi-provider LLM routing (Ollama, Gemini, Claude, GPT-4o, Phi-4) |
| `claw` | Core assistant logic, thread management, memory, tools, personas |
| `claw-web` | Svelte web dashboard for chat, settings, and admin |
| `claw-budget` | Token budget tracking, daily/monthly spend limits |
| `mux` | Email pipeline (Gmail integration, rules, auto-reply) |
| `voice` | TTS (Piper, ElevenLabs), STT (whisper.cpp), phone calls (Twilio) |
| `browser` | Web scraping, screenshots, automation via Playwright |
| `google` | Gmail, Calendar, Drive, Sheets integration |
| `notify` | Multi-channel notifications (Telegram, webhook) |
| `cron` | Scheduled tasks (morning briefing, health checks, goal nudges) |
| `post` | Social media publishing (7 platforms) |

---

## Multi-Model Routing

ɳClaw routes every request to the best model based on privacy, complexity, and cost:

### Privacy Classification
The routing engine scans each message for sensitive patterns (dollar amounts, credentials, health data, banking). Messages flagged as private route to local models or privacy-respecting providers.

### 4-Tier Routing

| Tier | Models | When Used |
|------|--------|-----------|
| 0 | Ollama (local) | Private data, simple classification |
| 1 | Gemini Flash Lite | Basic summarization, FAQ |
| 2 | Gemini Flash | Standard chat, moderate complexity |
| 3 | Claude / GPT-4o (OAuth) | Code, legal, medical, complex reasoning |

Each tier escalates to the next on rate limits (429). The `max_tier` parameter caps escalation to control cost. The `@model` override syntax lets users force a specific model:

```
@claude-opus Explain this contract clause...
@local Summarize my grocery list
@gemini What's the weather?
```

### Cost Tracking
Every AI call records model, tokens, and cost in `np_claw.model_usage`. The budget system enforces daily and monthly spend limits. When a budget threshold is hit, the system degrades to a lower tier rather than refusing requests.

---

## ReAct Tool System

ɳClaw uses a ReAct (Reason + Act) loop. The LLM reasons about the user's request, decides which tools to invoke, executes them, observes the results, and continues reasoning until the task is complete.

### 19+ Built-in Tools

**Communication:**
- `send_notification` -- push messages via Telegram or webhook
- `create_post` -- publish to WordPress, Ghost, Twitter, Telegram, Dev.to, Hashnode
- `send_email` / `draft_email` -- compose and send via Gmail

**Information:**
- `search_emails` -- search Gmail with date/account filters
- `mux_history` -- search email pipeline history
- `get_email` -- fetch full email content
- `get_calendar_events` / `find_free_slots` / `create_calendar_event` -- Google Calendar
- `query_transactions` -- search financial transactions by keyword, category, date
- `recall_memory` -- semantic search over the knowledge base (pgvector)

**System:**
- `shell_dispatch_vps` -- execute shell commands on the server (gated by NCLAW_ALLOW_SHELL)
- `git_create_pr` -- create GitHub pull requests
- `get_service_status` -- check ɳSelf service health
- `get_env_vars` -- read environment configuration (secrets masked)
- `install_plugin` / `restart_services` -- manage ɳSelf plugins
- `list_cron_jobs` / `create_cron_job` / `delete_cron_job` / `run_cron_job` -- cron management

**Browser:**
- `browser_navigate` / `browser_click` / `browser_type` -- CDP browser automation
- `browser_extract` / `browser_screenshot` / `browser_evaluate` -- page inspection
- `browser_get_cookies` -- cookie extraction

**Monitoring:**
- `monitoring.silence_alerts` / `monitoring.delete_silence` -- Alertmanager control
- `save_contact` / `find_contact` / `export_contacts` -- contact management

**Home Automation:**
- Integration adapter tools (Home Assistant, Grocy, Jellyfin presets)
- Custom integration tools defined per-integration via CRUD API

### Dynamic Tool Registry

Tools are registered dynamically based on installed plugins. The `plugin_awareness.rs` module checks plugin health at startup and injects tool definitions into the system prompt. If a plugin is not installed or unhealthy, its tools are excluded.

Native tools (google, cron, notify, shell, git, nself) are always available. Plugin-manifest tools are added based on capability reports from each plugin.

---

## Knowledge Graph (3-Layer Memory)

ɳClaw maintains persistent memory across conversations using a three-layer system:

### Layer 1: Recent Context (800 tokens)
The last few messages in the current thread. Provides immediate conversational context.

### Layer 2: Memory Blocks (600 tokens)
Typed memory entries stored in `np_claw_memories` with pgvector embeddings:

| Type | Example |
|------|---------|
| `fact` | "User's birthday is March 15" |
| `preference` | "Prefers dark mode, dislikes Java" |
| `decision` | "Chose PostgreSQL over MySQL for the project" |
| `pattern` | "Usually checks email at 9am" |
| `relationship` | "Alice is the user's manager" |

Memories are auto-extracted from conversations (up to 3 per turn) using an AI classifier. Semantic deduplication prevents redundant entries. A `superseded_by` chain tracks memory evolution. Unused memories age out over time.

### Layer 3: Core Summary (400 tokens)
A compressed summary of the user's identity, preferences, and key facts. Updated periodically from Layer 2 memories.

### Semantic Search
Memory retrieval uses pgvector cosine similarity (0.7 threshold) with HNSW indexing. When vector search returns no results, it falls back to ILIKE text matching.

---

## Persona System

Personas customize ɳClaw's identity, behavior, and available tools:

```
ClawPersona {
  name: "CamClaw"
  system_prompt: "You are CamClaw, a personal AI assistant..."
  tools_allowed: ["search_emails", "send_notification", ...]
  tier_ceiling: 3
  schedule_cron: "0 8 * * *"  // active 8am daily
  onboarding_questions: [...]
}
```

- `tools_allowed` whitelists which tools this persona can use
- `tier_ceiling` caps the maximum model tier (cost control)
- `schedule_cron` activates the persona on a schedule
- Custom `onboarding_questions` per persona

Built-in personas: CamClaw (personal assistant), ɳChat (messaging), ChatIslam (Islamic Q&A).

---

## Prompt Pipeline (6 Layers)

The system prompt is assembled dynamically at runtime by `build_system_prompt()`:

| Layer | Content | Source |
|-------|---------|--------|
| 1 | Identity | Active persona's system_prompt |
| 2 | User context | User profile, preferences, timezone |
| 3 | Semantic memory | RAG results from pgvector (relevant memories) |
| 4 | Table registry | Available Hasura tables and schemas |
| 5 | Tool manifest | All registered tools (native + plugin + integration) |
| 6 | Model formatting | Per-model tuning (Claude: XML, Gemini: lean, Phi-4: tight) |

Prompt caching headers are included for providers that support them (Claude).

---

## Email Pipeline (Mux)

The mux plugin processes Gmail in real-time via Google Pub/Sub:

1. **Watch registration.** Registers a Gmail push watch. Google sends notifications on new emails.
2. **Metadata fetch.** Retrieves email headers and body on notification.
3. **Rule evaluation.** YAML-defined rules match on from/to/subject/body with 16 action types:
 - `TelegramNotify`, `AiSummarize`, `AiClassify`, `AiExtract`
 - `CompanionNotify`, `VoiceCall`, `VoiceTts`
 - `SheetsLog`, `CalendarSync`, `Forward`
 - `MarkRead`, `ContinueEvaluation`, and more
4. **AI pre-screening.** Ollama tier-0 model screens emails before expensive summarization (eliminates ~60% of Gemini calls).
5. **Auto-reply.** Draft replies with AI, subject to guard rails and approval via Telegram.
6. **DLQ.** Failed metadata fetches go to a dead letter queue with 3-attempt retry.

### Priority Classification
Background AI classifies emails into: `action_required`, `time_sensitive`, `fyi`, `junk`. Accessible via `GET /claw/email/priorities`.

---

## Proactive Intelligence

ɳClaw does not just respond to requests. It proactively notifies you:

- **Morning briefing.** Daily cron sends email count + server CPU summary via Telegram.
- **Server health.** Polls Prometheus every 5 minutes. Alerts on CPU >80%, disk >85%, or service down. 1-hour dedup cooldown.
- **Goal nudges.** Daily noon check on your active goals with progress reminders.
- **Quiet hours.** Suppresses notifications between `NCLAW_QUIET_START` and `NCLAW_QUIET_END`.

---

## Companion Apps

### macOS Daemon
A native macOS daemon (port 7432) that gives ɳClaw awareness of your local environment:
- Active file watcher (Accessibility API)
- Terminal buffer capture
- Clipboard monitoring
- Screenshot capture
- Editor context (current file, language, selection)

Device capabilities are reported to the server, enabling context-aware responses.

### Browser Automation (Chrome CDP)
Chrome DevTools Protocol integration for web automation:
- Navigate, click, type, extract content, take screenshots
- Cookie extraction for session management
- URL allowlist + audit log for safety
- Consent flow required (NClaw_BrowserEnabled user default)

### Telegram Integration
Full two-way chat via Telegram bot:
- All message types: text, photos, documents, voice, video, stickers, location
- GraphQL mutation approval via inline buttons
- Draft email approval/discard via Telegram commands
- Webhook auto-registration on startup

---

## Client Apps

| Platform | Technology | Notes |
|----------|-----------|-------|
| Web | Next.js redirect shell | Points to user's self-hosted claw-web instance |
| iOS | SwiftUI + libnclaw FFI | Keychain auth, native UI |
| Android | Kotlin/Compose + libnclaw AAR | Material Design |
| macOS | Companion daemon | Local context provider |

The web app at `claw/apps/web/` is a thin redirect shell. It reads `NEXT_PUBLIC_CLAW_URL` or `nclaw_server_url` from localStorage and redirects to the user's self-hosted claw-web dashboard. No data is stored in the web app itself.

---

## claw-web Dashboard (Svelte)

The primary interface for interacting with ɳClaw. Served by the `claw-web` plugin.

**Pages:**
- Chat (threaded conversations, model badge, code copy, uncertainty chips)
- Memory (Knowledge Browser with type facets, Memory Health tab)
- Personas (selector, CRUD, onboarding questions)
- Budget (spend bar, daily/monthly limits, cache stats, estimated savings)
- Integrations (CRUD, template gallery, SSRF validation, health checks)
- Settings (30+ configuration pages for all plugin features)
- Onboarding (first-visit overlay, step cards, progress)
- Transactions (search, filter, summary charts)

---

## Setup

```bash
# Install nClaw plugins
nself license set nself_pro_xxxxx...
nself plugin install ai claw claw-web claw-budget mux voice browser google notify cron post
nself build && nself start

# Configure (minimum required)
# In .env.secrets:
NCLAW_FAST_MODEL=gemini-2.5-flash
CLAW_TG_BOT_TOKEN=your-bot-token
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret

# Access claw-web
# https://claw.yourdomain.com (or localhost via nself admin)
```

---

## Environment Variables

See [[Config-Env-Vars]] for core variables. ɳClaw-specific variables are documented on each plugin's wiki page. Key variables:

| Variable | Purpose |
|----------|---------|
| `NCLAW_FAST_MODEL` | Default fast model for routing |
| `NCLAW_ALLOW_SHELL` | Enable shell command execution tool |
| `NCLAW_QUIET_START` / `NCLAW_QUIET_END` | Quiet hours for proactive notifications |
| `NCLAW_DRAFT_APPROVAL` | Require Telegram approval for email drafts |
| `CLAW_BLOCKED_RECIPIENTS` | Comma-separated email blocklist |
| `CLAW_WEB_SECRET` | Authentication secret for claw-web |
| `MUX_CLAW_SHARED_SECRET` | Inter-plugin auth between mux and claw |
| `PLUGIN_INTERNAL_SECRET` | Generic inter-plugin authentication |

---

## Related Pages

- [[Plugin-Licensing]] -- tier requirements for ɳClaw plugins
- [[Feature-Plugins]] -- plugin system overview
- [[Security-Architecture]] -- authentication and authorization model
- [[API-Reference]] -- REST endpoints for claw, ai, mux plugins

---

← [[Features]] | [[_Sidebar]]
