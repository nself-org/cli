# ɳClaw Memories

ɳClaw builds a memory store for each user over time. During every session it
extracts facts worth remembering — preferences, past decisions, long-term goals —
and injects them as context at the start of future sessions. The result is an
assistant that actually knows who it is talking to.

## How memories are stored

Two paths write to the memory store:

| Path | When it fires | Source |
| --- | --- | --- |
| **Automatic** | At session end, the plugin extracts key facts from the conversation | `nself-claw` plugin |
| **Explicit** | You or the user add a memory directly | CLI, companion app, or API |

Automatic extraction uses the AI tier configured for `summarize` tasks (see
`nself claw routing show`). If no AI is available, extraction is skipped and the
session ends without writing.

## Memory limits

| Tier | Per-user limit |
| --- | --- |
| Default | 500 memories |
| Override | Set `CLAW_MEMORY_LIMIT` in `.env` |

Once the limit is reached, the oldest memories are evicted when new ones are written.

## Managing memories with the CLI

All memory commands require `PLUGIN_INTERNAL_SECRET` in the environment:

```bash
source .env  # or set manually: export PLUGIN_INTERNAL_SECRET=...
```

### List memories for a user

```bash
nself claw memories list --user <user-id>
```

Shows ID, source (automatic or explicit), and the first 80 characters of content.

### Add an explicit memory

```bash
nself claw memories add --user <user-id> --content "Prefers concise answers without bullet points"
```

Useful for pre-seeding preferences or important facts before the user has a session.

### Delete a single memory

```bash
nself claw memories delete --id <uuid>
```

Get the UUID from `memories list`. The deletion is permanent.

### Clear all memories for a user

```bash
nself claw memories clear --user <user-id>
```

Removes every memory for the user. Use this when a user requests data deletion or
when you want to reset the assistant's context for them.

### Show memory stats

```bash
nself claw memories stats --user <user-id>
```

Returns total, explicit, and semantic memory counts for the user.

## Managing memories via Telegram

If you have a Telegram bot configured, users can manage their own memories
through the bot without needing CLI access:

| Command | What it does |
| --- | --- |
| `/memories` | List the 10 most recent memories |
| `/forget <uuid>` | Delete a specific memory |
| `/forget all` | Clear all memories (prompts for confirmation) |

## Privacy

Memories are stored in the PostgreSQL database on your server. They are never
sent to any external service except as context in AI completion requests (to
whichever AI provider handles the user's session).

To export all memories for a user, use:

```bash
nself claw memories list --user <user-id>
```

The output is JSON-compatible when piped through `jq`.

To fulfill a data deletion request, use `memories clear --user <id>`.

## Related

- [ɳClaw Proactive Scheduler](nclaw-proactive.md) — morning digests and automated jobs
- [ɳClaw Setup Guide](../../../.wiki/guides/claw-companion-pairing.md) — initial setup and pairing
