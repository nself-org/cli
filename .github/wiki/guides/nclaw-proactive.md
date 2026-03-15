# ɳClaw Proactive Scheduler

ɳClaw runs a background scheduler that executes jobs on a cron schedule —
sending morning digests, checking SSL certs, monitoring disk usage, and
detecting anomalies. Jobs that fire outside business hours are suppressed
by the quiet-hours window (configurable). Critical jobs like `ssl_check`
and `disk_check` always run, regardless of quiet hours.

## Built-in jobs

| Job type | Default schedule | Critical | What it does |
| --- | --- | --- | --- |
| `morning_digest` | `0 7 * * *` | No | Sends a daily AI-enriched summary to Telegram |
| `health_report` | `0 * * * *` | No | Hourly server health snapshot |
| `ssl_check` | `0 6 * * *` | Yes | Checks cert expiry for all configured domains |
| `disk_check` | `30 * * * *` | Yes | Alerts when disk usage exceeds threshold |
| `anomaly_detect` | `*/15 * * * *` | No | Scans recent activity for unusual patterns |

Critical jobs bypass the quiet-hours window. Non-critical jobs are skipped when the
current hour falls within the configured `CLAW_QUIET_START`–`CLAW_QUIET_END` range.

A job is automatically disabled after 5 consecutive failures. Fix the underlying
issue and re-enable it with `nself claw proactive enable <job_type>`.

## Configuration

Set these in your `.env` before running `nself build`:

```bash
# Quiet hours (UTC). Jobs marked non-critical are skipped during this window.
# Default: 22:00-07:00. Set both to 0 to disable quiet hours entirely.
CLAW_QUIET_START=22
CLAW_QUIET_END=7

# Telegram bot for sending digests and alerts
CLAW_TG_BOT_TOKEN=<your-bot-token>
CLAW_TG_CHAT_ID=<your-chat-id>

# AI enrichment for morning_digest (optional — falls back to plain text if unset)
# Uses the tier configured for 'summarize' in routing
```

## Managing jobs with the CLI

```bash
source .env  # PLUGIN_INTERNAL_SECRET must be set
```

### Show all jobs and their state

```bash
nself claw proactive status
```

Displays job type, enabled/disabled state, cron expression, failure count,
and last run timestamp.

### Enable or disable a job

```bash
nself claw proactive enable  morning_digest
nself claw proactive disable health_report
```

Changes take effect immediately — the scheduler picks up the new state on its
next tick (every 60 seconds).

### Preview the next morning digest

```bash
nself claw proactive run
```

Builds and displays the digest that would be sent next, without sending it.
Useful for verifying the format and content before enabling Telegram delivery.

## Managing jobs via Telegram

If your Telegram bot is configured, you can control jobs from the bot chat:

| Command | What it does |
| --- | --- |
| `/digest` | Show the current morning digest preview |
| `/jobs` | List all jobs and their enabled state |
| `/enable <job_type>` | Enable a specific job |
| `/disable <job_type>` | Disable a specific job |

## Morning digest format

The plain-text digest sent each morning includes:

```
ɳClaw morning digest
Yesterday:
• Sessions started: N
• Messages sent: N
• Memories stored: N
• Top models: gpt-4o: 15, claude-3-haiku: 7
• AI cost: $0.0042
```

When AI enrichment is available, a 2-3 sentence AI-generated summary is appended.
If the AI endpoint is unavailable, the plain-text digest is sent without enrichment.

## Failure behavior

When a job fails:

1. The failure count increments in the database.
2. At failure count 5 the job is excluded from the due list (effectively disabled).
3. Check `nself claw proactive status` to see which jobs have elevated failure counts.
4. Fix the root cause, then re-enable: `nself claw proactive enable <job_type>`.

## Related

- [ɳClaw Memories](nclaw-memories.md) — per-user memory store
- [ɳClaw Telegram Setup](../../../.wiki/guides/claw-companion-pairing.md) — configure a bot
