# Real-Time Communication System

**Status**: ✅ Complete (Sprint 20)
**Compatibility**: Supabase Realtime, Nhost Realtime

The nself real-time system provides comprehensive real-time communication capabilities including database subscriptions (Change Data Capture), channel broadcasting, presence tracking, and WebSocket connections.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Database Subscriptions (CDC)](#database-subscriptions-cdc)
- [Channel Management](#channel-management)
- [Broadcasting Messages](#broadcasting-messages)
- [Presence Tracking](#presence-tracking)
- [Connection Management](#connection-management)
- [CLI Reference](#cli-reference)
- [Client SDK Examples](#client-sdk-examples)
- [Architecture](#architecture)
- [Best Practices](#best-practices)

---

## Overview

The nself real-time system is built on PostgreSQL LISTEN/NOTIFY and provides:

- **Database Subscriptions (CDC)**: Real-time change data capture from any table
- **Channels**: Public, private, and presence-enabled channels
- **Broadcasting**: Send messages to channels with event types
- **Presence**: Track user online/away/offline status
- **Message Replay**: Replay historical messages since a timestamp
- **PostgreSQL Native**: Uses LISTEN/NOTIFY for low-latency updates
- **Supabase Compatible**: Drop-in replacement for Supabase Realtime

### Key Features

| Feature | nself | Supabase | Nhost |
|---------|-------|----------|-------|
| **Database CDC** | ✅ | ✅ | ✅ |
| **Channels** | ✅ | ✅ | ✅ |
| **Presence** | ✅ | ✅ | ✅ |
| **Broadcast** | ✅ | ✅ | ✅ |
| **Message Replay** | ✅ | ❌ | ❌ |
| **CLI Management** | ✅ | ⚠️ Limited | ⚠️ Limited |
| **Self-hosted** | ✅ | ⚠️ Optional | ⚠️ Optional |

---

## Quick Start

### 1. Initialize Real-Time System

```bash
# Initialize the real-time database schema
nself realtime init
```

This creates:
- `realtime` schema
- Channels, messages, presence, and subscriptions tables
- Helper functions for cleanup and management

### 2. Subscribe to Table Changes

```bash
# Subscribe to all changes on a table
nself realtime subscribe public.users

# Subscribe to specific events
nself realtime subscribe public.posts INSERT,UPDATE

# Listen to changes in real-time
nself realtime listen public.users
```

### 3. Create a Channel

```bash
# Create a public channel
nself realtime channel create "general" public

# Create a private channel
nself realtime channel create "support" private

# Create a presence-enabled channel
nself realtime channel create "lobby" presence
```

### 4. Broadcast a Message

```bash
# Send a message to a channel
nself realtime broadcast general user.joined '{"user_id": "123", "name": "John Doe"}'

# Get recent messages
nself realtime messages general 50

# Replay messages since timestamp
nself realtime replay general 1643723400
```

### 5. Track Presence

```bash
# Track user online
nself realtime presence track user-123 general online

# List online users
nself realtime presence online general

# Count online users
nself realtime presence count general

# Set user offline
nself realtime presence offline user-123 general
```

---

## Database Subscriptions (CDC)

Change Data Capture allows you to subscribe to table changes in real-time.

### Subscribe to Table Changes

```bash
# Subscribe to all events (INSERT, UPDATE, DELETE)
nself realtime subscribe public.users

# Subscribe to specific events
nself realtime subscribe public.posts INSERT,UPDATE

# Subscribe with filter (future feature)
nself realtime subscribe public.comments INSERT,UPDATE "post_id = '123'"
```

### Listen to Changes

```bash
# Listen indefinitely (Ctrl+C to stop)
nself realtime listen public.users

# Listen for 30 seconds
nself realtime listen public.users 30
```

**Example Output:**
```
Asynchronous notification "realtime:table:public.users" received from server process with PID 12345.
Payload: {"timestamp":1643723456,"operation":"INSERT","schema":"public","table":"users","new":{"id":"user-123","email":"john@example.com","created_at":"2026-01-30T10:00:00Z"}}
```

### List Subscriptions

```bash
# Show all active subscriptions
nself realtime subscriptions

# Get subscription stats
nself realtime stats
```

### Unsubscribe

```bash
# Remove subscription by table name
nself realtime unsubscribe public.users

# Remove by subscription ID
nself realtime unsubscribe <uuid>
```

### How It Works

When you subscribe to a table:

1. **Trigger Function Created**: A PostgreSQL function is created to capture changes
2. **Triggers Attached**: INSERT/UPDATE/DELETE triggers are added to the table
3. **NOTIFY Sent**: On each change, PostgreSQL sends a notification
4. **Clients Listen**: WebSocket clients listen via `LISTEN realtime:table:schema.table`

**Database Structure:**

```sql
-- Subscription record
INSERT INTO realtime.subscriptions (table_name, events)
VALUES ('public.users', ARRAY['INSERT', 'UPDATE', 'DELETE']);

-- Trigger function
CREATE FUNCTION realtime.notify_public_users() ...

-- Triggers
CREATE TRIGGER realtime_INSERT_users AFTER INSERT ON public.users ...
CREATE TRIGGER realtime_UPDATE_users AFTER UPDATE ON public.users ...
CREATE TRIGGER realtime_DELETE_users AFTER DELETE ON public.users ...
```

---

## Channel Management

Channels organize real-time communication.

### Channel Types

| Type | Description | Use Case |
|------|-------------|----------|
| **public** | Anyone can join | General chat, announcements |
| **private** | Requires membership | Team channels, DMs |
| **presence** | Tracks who's online | Multiplayer games, collaborative editing |

### Create Channel

```bash
# Public channel
nself realtime channel create "general" public

# Private channel
nself realtime channel create "team-alpha" private

# Presence channel
nself realtime channel create "game-lobby" presence

# With metadata (JSON)
nself realtime channel create "support" private '{"max_members": 10}'
```

### List Channels

```bash
# List all channels
nself realtime channel list

# Filter by type
nself realtime channel list public
nself realtime channel list private
nself realtime channel list presence

# JSON output
nself realtime channel list --format json
```

**Example Output:**
```
 id  | slug        | name        | type    | members | online | created_at
-----+-------------+-------------+---------+---------+--------+------------
 abc | general     | General     | public  | 45      | 12     | 2026-01-30
 def | support     | Support     | private | 8       | 3      | 2026-01-30
```

### Get Channel Details

```bash
# Get by slug
nself realtime channel get general

# Get by ID
nself realtime channel get <uuid>
```

### Delete Channel

```bash
# Delete with confirmation
nself realtime channel delete general

# Force delete (no confirmation)
nself realtime channel delete general true
```

### Manage Members

```bash
# Add member to channel
nself realtime channel join general user-123

# Add moderator
nself realtime channel join support user-456 moderator

# Add admin
nself realtime channel join team-alpha user-789 admin

# Remove member
nself realtime channel leave general user-123

# List members
nself realtime channel members general
```

**Member Roles:**
- `member`: Can read and send messages
- `moderator`: Can moderate messages and members
- `admin`: Full control over channel

---

## Broadcasting Messages

Send real-time messages to channels.

### Send Message

```bash
# Basic broadcast
nself realtime broadcast general user.joined '{"user_id": "123"}'

# With sender
nself realtime broadcast general message.sent '{"text": "Hello"}' user-123

# Complex payload
nself realtime broadcast game-lobby player.moved '{
  "player_id": "player-1",
  "position": {"x": 100, "y": 200},
  "timestamp": 1643723456
}'
```

### Get Messages

```bash
# Get last 50 messages (default)
nself realtime messages general

# Get last 100 messages
nself realtime messages general 100

# JSON format
nself realtime messages general 50 json
```

### Message Replay

Replay messages since a specific timestamp (unique to nself):

```bash
# Replay from UNIX timestamp
nself realtime replay general 1643723400

# Replay from ISO 8601
nself realtime replay general "2026-01-30T10:00:00Z"

# Get JSON output
nself realtime replay general 1643723400 json
```

**Use Cases:**
- Reconnecting clients can get missed messages
- Debugging message flow
- Auditing communication history

### Event Types

```bash
# List event types in last 24 hours
nself realtime events general

# List events in last 48 hours
nself realtime events general 48
```

**Example Output:**
```
 event_type      | count | last_sent
-----------------+-------+---------------------
 user.joined     | 234   | 2026-01-30 10:30:00
 message.sent    | 1843  | 2026-01-30 10:29:45
 user.left       | 189   | 2026-01-30 10:28:12
```

### Broadcast Statistics

```bash
# Global stats
nself realtime stats

# Channel-specific stats
nself realtime broadcast general
```

### Cleanup Old Messages

```bash
# Delete messages older than 24 hours (default)
nself realtime cleanup

# Custom retention (48 hours)
nself realtime broadcast cleanup 48
```

---

## Presence Tracking

Track user online/away/offline status.

### Track Presence

```bash
# Set user online in channel
nself realtime presence track user-123 general online

# Set user away
nself realtime presence track user-123 general away

# Set user offline
nself realtime presence track user-123 general offline

# Track with metadata
nself realtime presence track user-123 game-lobby online '{"level": 5, "score": 1000}'
```

### Status Values

| Status | Description |
|--------|-------------|
| `online` | User is actively present |
| `away` | User is idle/inactive |
| `offline` | User has disconnected |

### Get Presence

```bash
# Get user presence in specific channel
nself realtime presence get user-123 general

# Get global user presence
nself realtime presence get user-123

# JSON format
nself realtime presence get user-123 general json
```

### List Online Users

```bash
# List online users in channel
nself realtime presence online general

# List all online users (global)
nself realtime presence online

# JSON format
nself realtime presence online general json
```

**Example Output:**
```
 user_id   | channel | status | metadata              | last_seen_at        | seconds_ago
-----------+---------+--------+-----------------------+---------------------+-------------
 user-123  | general | online | {"device": "mobile"}  | 2026-01-30 10:30:00 | 5
 user-456  | general | away   | {"device": "desktop"} | 2026-01-30 10:25:00 | 305
```

### Count Online Users

```bash
# Count in channel
nself realtime presence count general

# Count global
nself realtime presence count
```

### Set User Offline

```bash
# Set offline in specific channel
nself realtime presence offline user-123 general

# Set offline in all channels
nself realtime presence offline user-123
```

### Presence Cleanup

Automatically mark stale users as offline:

```bash
# Cleanup users inactive for 5 minutes (default: 300s)
nself realtime presence cleanup

# Custom timeout (10 minutes)
nself realtime presence cleanup 600
```

### Presence Statistics

```bash
nself realtime presence stats
```

**Example Output:**
```
 online | away | offline | total_users | total_channels
--------+------+---------+-------------+----------------
 45     | 12   | 234     | 291         | 8
```

### Update Metadata

```bash
# Update presence metadata
nself realtime presence track user-123 game-lobby online '{"score": 1500, "level": 6}'
```

---

## Connection Management

Monitor active WebSocket connections.

### View Connections

```bash
# Show all active connections
nself realtime connections

# JSON output
nself realtime connections --json
```

**Example Output:**
```
 user_id   | connection_id        | status     | connected_at        | seconds_since_seen
-----------+----------------------+------------+---------------------+-------------------
 user-123  | conn-abc-123         | connected  | 2026-01-30 10:00:00 | 45
 user-456  | conn-def-456         | connected  | 2026-01-30 09:55:00 | 345
```

### Cleanup Stale Connections

```bash
# Clean connections inactive for 5 minutes
nself realtime cleanup
```

---

## CLI Reference

### System Commands

```bash
nself realtime init                    # Initialize real-time system
nself realtime status                  # Show system status
nself realtime logs [--follow]         # Show logs
nself realtime cleanup                 # Clean up stale data
nself realtime stats                   # Show detailed statistics
```

### Database Subscriptions

```bash
nself realtime subscribe <table> [events]     # Subscribe to table changes
nself realtime unsubscribe <table>            # Unsubscribe
nself realtime listen <table> [seconds]       # Listen to changes
nself realtime subscriptions                  # List subscriptions
```

### Channel Management

```bash
nself realtime channel create <name> [type]   # Create channel
nself realtime channel list [type]            # List channels
nself realtime channel get <id>               # Get channel details
nself realtime channel delete <id>            # Delete channel
nself realtime channel members <id>           # List members
nself realtime channel join <ch> <user>       # Add member
nself realtime channel leave <ch> <user>      # Remove member
```

### Broadcasting

```bash
nself realtime broadcast <ch> <event> <payload>  # Send message
nself realtime messages <ch> [limit]             # Get messages
nself realtime replay <ch> <timestamp>           # Replay messages
nself realtime events <ch> [hours]               # List event types
```

### Presence

```bash
nself realtime presence track <user> <ch> [status]  # Track presence
nself realtime presence get <user> [ch]             # Get presence
nself realtime presence online [ch]                 # List online users
nself realtime presence count [ch]                  # Count online
nself realtime presence offline <user> [ch]         # Set offline
nself realtime presence stats                       # Get stats
nself realtime presence cleanup [timeout]           # Cleanup stale
```

---

## Client SDK Examples

### JavaScript/TypeScript

```typescript
import { RealtimeClient } from '@nself/client';

const client = new RealtimeClient({
  url: 'ws://localhost:3100',
  apiKey: 'your-api-key'
});

// Subscribe to table changes
const subscription = client
  .channel('realtime:table:public.users')
  .on('INSERT', (payload) => {
    console.log('New user:', payload.new);
  })
  .on('UPDATE', (payload) => {
    console.log('Updated user:', payload.new);
  })
  .on('DELETE', (payload) => {
    console.log('Deleted user:', payload.old);
  })
  .subscribe();

// Broadcast message
const channel = client.channel('general');
channel.send({
  type: 'broadcast',
  event: 'message',
  payload: { text: 'Hello World!' }
});

// Track presence
const presence = client.channel('lobby');
presence.track({ user_id: 'user-123', status: 'online' });
presence.on('presence', { event: 'join' }, ({ key, currentPresence }) => {
  console.log('User joined:', key, currentPresence);
});
```

### React Hook

```typescript
import { useChannel } from '@nself/react';

function ChatRoom({ roomId }) {
  const { messages, send, online } = useChannel(roomId, {
    events: ['message.sent', 'user.joined', 'user.left'],
    presence: true
  });

  const sendMessage = (text: string) => {
    send('message.sent', { text, user_id: currentUser.id });
  };

  return (
    <div>
      <div>Online: {online.length}</div>
      {messages.map(msg => (
        <div key={msg.id}>{msg.payload.text}</div>
      ))}
    </div>
  );
}
```

### PostgreSQL LISTEN (Direct)

```sql
-- Listen to channel
LISTEN "realtime:table:public.users";

-- In another session, trigger change
INSERT INTO public.users (email) VALUES ('new@example.com');

-- First session receives:
-- Asynchronous notification "realtime:table:public.users" received
-- Payload: {"operation":"INSERT","new":{...}}
```

---

## Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                       Client Applications                    │
│  (Browser, Mobile, Server-side)                             │
└─────────────────────┬───────────────────────────────────────┘
                      │ WebSocket / HTTP
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                   WebSocket Server (Optional)                │
│  - Connection handling                                       │
│  - Authentication                                            │
│  - Message routing                                           │
└─────────────────────┬───────────────────────────────────────┘
                      │ PostgreSQL LISTEN/NOTIFY
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                  PostgreSQL (Core Engine)                    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ realtime.channels      - Channel definitions         │   │
│  │ realtime.messages      - Broadcast messages          │   │
│  │ realtime.presence      - User presence               │   │
│  │ realtime.subscriptions - Table CDC config            │   │
│  │ realtime.connections   - Active connections          │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Triggers on user tables (when subscribed)            │   │
│  │ - notify_<schema>_<table>()                          │   │
│  │ - Sends NOTIFY events on INSERT/UPDATE/DELETE        │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### Database Schema

```sql
realtime.channels          -- Channel definitions
realtime.channel_members   -- Channel membership
realtime.messages          -- Broadcast message history
realtime.presence          -- User presence tracking
realtime.subscriptions     -- Table CDC subscriptions
realtime.connections       -- Active WebSocket connections
```

### PostgreSQL NOTIFY Flow

1. **Table Change**: `INSERT INTO public.users (...)`
2. **Trigger Fires**: `realtime_INSERT_users` trigger executes
3. **Function Runs**: `realtime.notify_public_users()` function
4. **NOTIFY Sent**: `pg_notify('realtime:table:public.users', payload)`
5. **Clients Receive**: All listeners get notification instantly

### Message Format

```json
{
  "timestamp": 1643723456,
  "operation": "INSERT",
  "schema": "public",
  "table": "users",
  "new": {
    "id": "user-123",
    "email": "john@example.com",
    "created_at": "2026-01-30T10:00:00Z"
  }
}
```

---

## Best Practices

### Performance

1. **Cleanup Old Messages**: Run regular cleanups to prevent table bloat
   ```bash
   # Add to cron: cleanup messages older than 24h
   0 */6 * * * nself realtime cleanup
   ```

2. **Index Critical Columns**: Already indexed in migration
   - `messages.sent_at` for time-based queries
   - `presence.user_id` and `channel_id` for lookups

3. **Limit Message Payload Size**: Keep payloads under 1MB
   ```bash
   # Good
   nself realtime broadcast general event '{"id": "123"}'

   # Bad (large payload)
   nself realtime broadcast general event '{"data": "...10MB..."}'
   ```

### Security

1. **Channel Access Control**: Use private channels for sensitive data
   ```bash
   nself realtime channel create "billing" private
   ```

2. **Row Level Security**: Add RLS policies to realtime tables
   ```sql
   ALTER TABLE realtime.channels ENABLE ROW LEVEL SECURITY;

   CREATE POLICY "Users can only see channels they're members of"
   ON realtime.channels FOR SELECT
   USING (
     id IN (
       SELECT channel_id FROM realtime.channel_members
       WHERE user_id = auth.uid()
     )
   );
   ```

3. **Validate Payloads**: Always validate message payloads on the server

### Scalability

1. **Message Retention**: Set appropriate retention periods
   ```bash
   # Shorter retention for high-volume channels
   # Delete messages older than 1 hour
   */15 * * * * psql -c "DELETE FROM realtime.messages WHERE sent_at < NOW() - INTERVAL '1 hour'"
   ```

2. **Connection Limits**: Monitor active connections
   ```bash
   nself realtime connections | wc -l
   ```

3. **Presence Cleanup**: Auto-cleanup stale presence
   ```bash
   # Every 5 minutes, mark users offline if inactive > 5min
   */5 * * * * nself realtime presence cleanup 300
   ```

### Monitoring

1. **Check System Status**:
   ```bash
   nself realtime status
   nself realtime stats
   ```

2. **Monitor Subscription Health**:
   ```bash
   nself realtime subscriptions
   ```

3. **Track Message Volume**:
   ```bash
   nself realtime events general 24
   ```

---

## Comparison: nself vs Supabase vs Nhost

| Feature | nself | Supabase | Nhost |
|---------|-------|----------|-------|
| **Database CDC** | ✅ PostgreSQL LISTEN/NOTIFY | ✅ Custom Realtime server | ✅ Hasura subscriptions |
| **Channels** | ✅ Full support | ✅ Full support | ✅ Full support |
| **Presence** | ✅ Full support | ✅ Full support | ✅ Full support |
| **Broadcast** | ✅ Full support | ✅ Full support | ✅ Full support |
| **Message Replay** | ✅ Built-in | ❌ Not available | ❌ Not available |
| **CLI Management** | ✅ Comprehensive | ⚠️ Limited | ⚠️ Limited |
| **Self-hosted** | ✅ Primary | ⚠️ Optional | ⚠️ Optional |
| **Cost** | ✅ Free | 💰 Usage-based | 💰 Usage-based |

**nself Advantages:**
- Message replay capability
- Comprehensive CLI tools
- Direct PostgreSQL integration
- No external dependencies
- Full control and customization

---

## Troubleshooting

### Subscriptions Not Working

```bash
# Check if subscription exists
nself realtime subscriptions

# Verify triggers exist
psql -c "\d+ public.users"  # Should show triggers

# Test manually
nself realtime listen public.users &
psql -c "INSERT INTO public.users (email) VALUES ('test@example.com');"
```

### Messages Not Arriving

```bash
# Check channel exists
nself realtime channel list

# Verify messages are being stored
nself realtime messages <channel> 10

# Check broadcast stats
nself realtime stats
```

### Presence Not Updating

```bash
# Check presence records
nself realtime presence online

# Cleanup stale presence
nself realtime presence cleanup

# Track presence manually
nself realtime presence track <user> <channel> online
```

---

## Next Steps

- **WebSocket Server**: Implement production WebSocket server (optional)
- **Client SDKs**: Use generated SDKs for your language
- **Integrate with Auth**: Connect presence to user authentication
- **Add RLS Policies**: Secure channels with Row Level Security
- **Monitor Performance**: Set up alerts for connection limits

For more information:
- [Database Subscriptions Guide](../guides/REALTIME-FEATURES.md)
- [Client SDK Documentation](../architecture/API.md)
- [Real-time Performance Guide](realtime-examples.md)

---

**Last Updated**: January 30, 2026
**Version**: 0.9.0
**Status**: Production Ready ✅
