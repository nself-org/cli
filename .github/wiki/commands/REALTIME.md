# nself service realtime - Real-Time Communication

> **DEPRECATED COMMAND NAME**: This command was formerly `nself realtime` in v0.x. It has been consolidated to `nself service realtime` in v1.0. The old command name may still work as an alias.

Real-time communication system for WebSocket channels, presence tracking, broadcast messaging, and database subscriptions (Change Data Capture).

**Version:** v0.9.5+
**Status:** Production Ready

---

## Synopsis

```bash
nself service realtime <command> [options]
```

---

## Description

The `nself service realtime` command provides a complete real-time communication system compatible with Supabase and Nhost. It includes:

- **Database Subscriptions (CDC)** - Subscribe to table changes (INSERT, UPDATE, DELETE)
- **Channel Management** - Public, private, and presence channels
- **Broadcast Messages** - Send messages to channel subscribers
- **Presence Tracking** - Track user online/away/offline status
- **WebSocket Server** - Automatic reconnection and connection pooling

---

## System Management

### init

Initialize the real-time system and database schema.

```bash
nself service realtime init
```

**What it does:**
- Creates real-time database schema and tables
- Sets up WebSocket server configuration
- Initializes presence tracking system
- Creates default channels

**Example:**
```bash
nself service realtime init
```

---

### status

Show real-time system status.

```bash
nself service realtime status
```

**Shows:**
- WebSocket server status
- Active connections count
- Enabled channels count
- Subscription count
- System configuration

**Example:**
```bash
nself service realtime status
```

**Output:**
```
Real-Time System Status

✓ WebSocket server running
✓ 145 active connections
✓ 23 channels (12 public, 8 private, 3 presence)
✓ 47 active subscriptions
✓ Presence tracking enabled
```

---

### logs

View real-time system logs.

```bash
nself service realtimelogs [--follow]
```

**Options:**
- `--follow` - Stream logs in real-time

**Examples:**
```bash
# View recent logs
nself service realtimelogs

# Follow logs
nself service realtimelogs --follow
```

---

### cleanup

Clean up stale connections and old messages.

```bash
nself service realtimecleanup
```

**What it cleans:**
- Stale WebSocket connections (timeout: 5 minutes)
- Old messages (default retention: 30 days)
- Expired presence records
- Orphaned subscriptions

**Example:**
```bash
nself service realtimecleanup
```

---

## Database Subscriptions (CDC)

Subscribe to database table changes using PostgreSQL Change Data Capture (CDC).

### subscribe

Subscribe to table changes.

```bash
nself service realtimesubscribe <table> [events]
```

**Arguments:**
- `<table>` - Table name (schema.table format)
- `[events]` - Comma-separated events: INSERT,UPDATE,DELETE (default: all)

**Examples:**
```bash
# Subscribe to all events
nself service realtimesubscribe public.users

# Subscribe to specific events
nself service realtimesubscribe public.posts INSERT,UPDATE

# Subscribe to schema
nself service realtimesubscribe public.*
```

---

### unsubscribe

Remove subscription from table.

```bash
nself service realtimeunsubscribe <table>
```

**Example:**
```bash
nself service realtimeunsubscribe public.users
```

---

### listen

Listen to table changes in real-time (blocking).

```bash
nself service realtimelisten <table> [seconds]
```

**Arguments:**
- `<table>` - Table to listen to
- `[seconds]` - Duration to listen (default: 60)

**Example:**
```bash
# Listen for 60 seconds
nself service realtimelisten public.users

# Listen for 5 minutes
nself service realtimelisten public.users 300
```

**Output:**
```
Listening to public.users...
[2026-01-30 10:15:23] INSERT: {"id": 123, "email": "user@example.com"}
[2026-01-30 10:15:45] UPDATE: {"id": 123, "name": "John Doe"}
```

---

### subscriptions

List all active subscriptions.

```bash
nself service realtimesubscriptions
```

**Example:**
```bash
nself service realtimesubscriptions
```

**Output:**
```
Active Database Subscriptions:

  public.users              INSERT, UPDATE, DELETE
  public.posts              INSERT, UPDATE
  public.comments           INSERT, DELETE
```

---

## Channel Management

Create and manage real-time channels for messaging.

### channel create

Create a new channel.

```bash
nself service realtimechannel create <name> [type]
```

**Arguments:**
- `<name>` - Channel name
- `[type]` - Channel type: public|private|presence (default: public)

**Channel Types:**
- **public** - Open to all users
- **private** - Invite-only, requires authorization
- **presence** - Tracks online users

**Examples:**
```bash
# Public channel (anyone can join)
nself service realtimechannel create general public

# Private channel (invite only)
nself service realtimechannel create team-alpha private

# Presence channel (tracks online users)
nself service realtimechannel create lobby presence
```

---

### channel list

List all channels.

```bash
nself service realtimechannel list [type]
```

**Arguments:**
- `[type]` - Filter by type: all|public|private|presence (default: all)

**Examples:**
```bash
# All channels
nself service realtimechannel list

# Only public channels
nself service realtimechannel list public

# Only presence channels
nself service realtimechannel list presence
```

**Output:**
```
Channels:

  general          public      245 members
  announcements    public      1,203 members
  team-alpha       private     12 members
  lobby            presence    89 online
```

---

### channel get

Get channel details.

```bash
nself service realtimechannel get <id>
```

**Example:**
```bash
nself service realtimechannel get general
```

**Output:**
```
Channel: general
Type: public
Members: 245
Created: 2026-01-15 08:30:00
Messages (24h): 1,847
```

---

### channel delete

Delete a channel.

```bash
nself service realtimechannel delete <id>
```

**Warning:** This deletes all channel messages and memberships.

**Example:**
```bash
nself service realtimechannel delete old-channel
```

---

### channel members

List channel members.

```bash
nself service realtimechannel members <id>
```

**Example:**
```bash
nself service realtimechannel members general
```

**Output:**
```
Members of #general (245):

  user-123    John Doe       online
  user-456    Jane Smith     away
  user-789    Bob Johnson    offline
```

---

### channel join

Add user to channel.

```bash
nself service realtimechannel join <channel> <user>
```

**Example:**
```bash
nself service realtimechannel join general user-123
```

---

### channel leave

Remove user from channel.

```bash
nself service realtimechannel leave <channel> <user>
```

**Example:**
```bash
nself service realtimechannel leave general user-123
```

---

## Broadcast Messages

Send messages to channel subscribers.

### broadcast

Send message to channel.

```bash
nself service realtimebroadcast <channel> <event> <payload>
```

**Arguments:**
- `<channel>` - Channel name or ID
- `<event>` - Event type (e.g., user.joined, message.new)
- `<payload>` - JSON payload

**Examples:**
```bash
# User joined event
nself service realtimebroadcast general user.joined '{"user_id": "123", "name": "John"}'

# New message event
nself service realtimebroadcast general message.new '{"text": "Hello!", "from": "user-123"}'

# Custom event
nself service realtimebroadcast general app.notification '{"title": "Update", "body": "New version available"}'
```

---

### messages

Get recent channel messages.

```bash
nself service realtimemessages <channel> [limit]
```

**Arguments:**
- `<channel>` - Channel name
- `[limit]` - Number of messages (default: 50, max: 1000)

**Example:**
```bash
# Last 50 messages
nself service realtimemessages general

# Last 100 messages
nself service realtimemessages general 100
```

---

### replay

Replay messages since timestamp.

```bash
nself service realtimereplay <channel> <timestamp>
```

**Arguments:**
- `<channel>` - Channel name
- `<timestamp>` - ISO 8601 timestamp

**Example:**
```bash
# Replay messages from today
nself service realtimereplay general 2026-01-30T00:00:00Z

# Replay last hour
nself service realtimereplay general 2026-01-30T09:00:00Z
```

---

### events

List event types in channel.

```bash
nself service realtimeevents <channel> [hours]
```

**Arguments:**
- `<channel>` - Channel name
- `[hours]` - Timeframe in hours (default: 24)

**Example:**
```bash
# Event types in last 24 hours
nself service realtimeevents general

# Event types in last week
nself service realtimeevents general 168
```

**Output:**
```
Event Types (last 24h):

  user.joined       47 events
  message.new       1,203 events
  user.left         39 events
  typing.start      567 events
```

---

## Presence Tracking

Track user online/away/offline status.

### presence track

Track user presence.

```bash
nself service realtimepresence track <user> <channel> [status]
```

**Arguments:**
- `<user>` - User ID
- `<channel>` - Channel name
- `[status]` - Status: online|away|offline (default: online)

**Examples:**
```bash
# User comes online
nself service realtimepresence track user-123 general online

# User goes away
nself service realtimepresence track user-123 general away

# User goes offline
nself service realtimepresence track user-123 general offline
```

---

### presence get

Get user presence.

```bash
nself service realtimepresence get <user> [channel]
```

**Arguments:**
- `<user>` - User ID
- `[channel]` - Channel name (optional, shows all if omitted)

**Examples:**
```bash
# All channels
nself service realtimepresence get user-123

# Specific channel
nself service realtimepresence get user-123 general
```

**Output:**
```
Presence: user-123

  general          online     Last seen: now
  team-alpha       away       Last seen: 5m ago
  support          offline    Last seen: 2h ago
```

---

### presence online

List online users.

```bash
nself service realtimepresence online [channel]
```

**Arguments:**
- `[channel]` - Channel name (optional, shows global if omitted)

**Examples:**
```bash
# Global online users
nself service realtimepresence online

# Channel online users
nself service realtimepresence online general
```

**Output:**
```
Online Users (general): 89

  user-123    John Doe       online    5s ago
  user-456    Jane Smith     online    12s ago
  user-789    Bob Johnson    away      2m ago
```

---

### presence count

Count online users.

```bash
nself service realtimepresence count [channel]
```

**Example:**
```bash
# Global count
nself service realtimepresence count

# Channel count
nself service realtimepresence count general
```

**Output:**
```
Online: 89
Away: 12
Offline: 144
Total: 245
```

---

### presence offline

Set user offline.

```bash
nself service realtimepresence offline <user> [channel]
```

**Examples:**
```bash
# Offline in all channels
nself service realtimepresence offline user-123

# Offline in specific channel
nself service realtimepresence offline user-123 general
```

---

### presence stats

Get presence statistics.

```bash
nself service realtimepresence stats
```

**Example:**
```bash
nself service realtimepresence stats
```

**Output:**
```
Presence Statistics:

  Total users tracked:     1,247
  Currently online:        389
  Currently away:          78
  Currently offline:       780

  Channels with presence:  15
  Average presence/channel: 83
```

---

## Connection Management

### connections

Show active WebSocket connections.

```bash
nself service realtimeconnections [--json]
```

**Options:**
- `--json` - Output in JSON format

**Example:**
```bash
nself service realtimeconnections
```

**Output:**
```
Active Connections: 145

  conn-abc123    user-123    general, team-alpha     Connected 5m ago
  conn-def456    user-456    general                 Connected 12m ago
  conn-ghi789    user-789    support                 Connected 1h ago
```

---

### stats

Show detailed real-time statistics.

```bash
nself service realtimestats
```

**Example:**
```bash
nself service realtimestats
```

**Output:**
```
Real-Time Statistics:

WebSocket Connections:
  Active:                  145
  Peak (24h):             287
  Average latency:         15ms

Channels:
  Total:                   23
  Public:                  12
  Private:                 8
  Presence:                3

Messages (24h):
  Sent:                    45,203
  Delivered:               45,187
  Failed:                  16

Database Subscriptions:
  Active:                  47
  Tables monitored:        12
  Events processed (24h):  8,934

Presence:
  Users tracked:           1,247
  Currently online:        389
  Heartbeats (1m):         389
```

---

## Configuration

Real-time system configuration via environment variables:

```bash
# .env
REALTIME_ENABLED=true
REALTIME_PORT=4000
REALTIME_MAX_CONNECTIONS=10000
REALTIME_MESSAGE_TTL=86400          # 1 day
REALTIME_PRESENCE_TIMEOUT=300       # 5 minutes
REALTIME_HEARTBEAT_INTERVAL=30      # 30 seconds
REALTIME_RECONNECT_DELAY=5000       # 5 seconds
```

---

## Database Schema

Real-time system uses these PostgreSQL tables:

```sql
-- Channels
realtime.channels (id, name, type, created_at, metadata)

-- Channel members
realtime.channel_members (channel_id, user_id, joined_at)

-- Messages
realtime.messages (id, channel_id, event, payload, created_at)

-- Presence
realtime.presence (user_id, channel_id, status, last_seen, metadata)

-- Subscriptions
realtime.subscriptions (id, table_name, events, created_at)

-- Connections
realtime.connections (id, user_id, connected_at, last_ping)
```

---

## Examples

### Complete Setup

```bash
# 1. Initialize system
nself service realtimeinit

# 2. Create channels
nself service realtimechannel create general public
nself service realtimechannel create announcements public
nself service realtimechannel create support private

# 3. Subscribe to database changes
nself service realtimesubscribe public.users INSERT,UPDATE,DELETE
nself service realtimesubscribe public.posts INSERT,UPDATE

# 4. Check status
nself service realtimestatus
```

### User Joins Channel

```bash
# Add user to channel
nself service realtimechannel join general user-123

# Track presence
nself service realtimepresence track user-123 general online

# Broadcast join event
nself service realtimebroadcast general user.joined '{"user_id": "user-123", "name": "John Doe"}'
```

### Monitor Activity

```bash
# Watch database changes
nself service realtimelisten public.users 300

# View channel messages
nself service realtimemessages general 50

# Check who's online
nself service realtimepresence online general

# View statistics
nself service realtimestats
```

---

## Performance

- **WebSocket Connections:** 10,000+ concurrent per instance
- **Message Delivery:** <10ms latency
- **Presence Update:** <50ms propagation
- **Database CDC:** <20ms event delivery
- **Channel Broadcast:** <30ms to all subscribers

---

## Security

### Authentication

All real-time operations require authentication via JWT tokens.

### Authorization

- **Public channels:** Anyone can join
- **Private channels:** Requires membership
- **Presence channels:** Membership + presence tracking

### Rate Limiting

- **Connections:** 100 per IP per minute
- **Messages:** 60 per connection per minute
- **Presence updates:** 10 per connection per minute

---

## Troubleshooting

### Connection Issues

```bash
# Check WebSocket server
nself service realtimestatus

# View logs
nself service realtimelogs --follow

# Check connections
nself service realtimeconnections
```

### Message Delivery Issues

```bash
# Verify channel exists
nself service realtimechannel list

# Check recent messages
nself service realtimemessages <channel> 10

# Monitor broadcast
nself service realtimelisten <table>
```

### Presence Not Updating

```bash
# Check presence configuration
nself service realtimepresence stats

# Manually update presence
nself service realtimepresence track <user> <channel> online

# Clean up stale presence
nself service realtimecleanup
```

---

## Migration from Supabase

ɳSelf real-time is compatible with Supabase real-time API:

```bash
# Import Supabase real-time configuration
nself migrate from-supabase realtime

# Channels are automatically created
# Subscriptions are migrated
# Presence tracking continues working
```

---

## See Also

- [Real-Time Features Guide](../guides/REALTIME-FEATURES.md)
- [API Reference](../reference/api/README.md)
- [Database Workflow](../guides/DATABASE-WORKFLOW.md)
- [Real-Time Examples](../guides/REALTIME-FEATURES.md)
- [nself status](./STATUS.md)

---

**Version:** ɳSelf v0.9.5
**Last Updated:** January 30, 2026
