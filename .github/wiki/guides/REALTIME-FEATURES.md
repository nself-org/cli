# Real-Time Features Guide

**nself v0.8.0** | Complete guide to real-time collaboration and messaging

---

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Getting Started](#getting-started)
4. [Channel Types](#channel-types)
5. [Client-Side Integration](#client-side-integration)
6. [Messaging](#messaging)
7. [Presence Tracking](#presence-tracking)
8. [Broadcasting Events](#broadcasting-events)
9. [Database Change Streaming](#database-change-streaming)
10. [Security](#security)
11. [Scaling Real-Time](#scaling-real-time)
12. [Monitoring & Debugging](#monitoring--debugging)
13. [Advanced Topics](#advanced-topics)

---

## Overview

### What is the nself Real-Time System?

The nself real-time system provides WebSocket-based communication infrastructure for building collaborative applications, live chat, notifications, and real-time data synchronization. It's a complete real-time solution that rivals commercial offerings like Supabase Realtime, Pusher, and Ably.

### Architecture

The system consists of three main components:

1. **WebSocket Server** - Socket.IO-based server handling connections and message routing
2. **PostgreSQL Backend** - Persistent storage for messages, channels, and presence data
3. **PostgreSQL NOTIFY/LISTEN** - Database-level pub/sub for real-time events

```
┌─────────────┐      WebSocket       ┌──────────────────┐
│   Clients   │ ◄─────────────────► │  WebSocket       │
│ (Browser/   │                      │  Server          │
│  Mobile)    │                      │  (Socket.IO)     │
└─────────────┘                      └────────┬─────────┘
                                              │
                                              │ pg-notify
                                              │ SQL queries
                                              ▼
                                     ┌──────────────────┐
                                     │   PostgreSQL     │
                                     │  + NOTIFY/LISTEN │
                                     │  + RLS Policies  │
                                     └──────────────────┘
```

### Use Cases

- **Chat Applications** - Real-time messaging with message history
- **Collaborative Editing** - Google Docs-style multi-user editing with cursor tracking
- **Notifications** - Live notification delivery
- **Presence Tracking** - See who's online and where
- **Live Dashboards** - Real-time data updates from database changes
- **Typing Indicators** - Show when users are typing
- **File Collaboration** - Coordinate file sharing and editing

### Comparison to Other Solutions

| Feature | nself Realtime | Supabase Realtime | Pusher | Ably |
|---------|---------------|-------------------|--------|------|
| **WebSocket Server** | ✅ Socket.IO | ✅ Phoenix | ✅ Proprietary | ✅ Proprietary |
| **Message Persistence** | ✅ PostgreSQL | ❌ Ephemeral | ❌ Ephemeral | ✅ Optional |
| **Database Streaming** | ✅ NOTIFY/LISTEN | ✅ Replication | ❌ No | ❌ No |
| **Presence Tracking** | ✅ Built-in | ✅ Built-in | ✅ Built-in | ✅ Built-in |
| **Self-Hosted** | ✅ Yes | ✅ Yes | ❌ No | ❌ No |
| **Authentication** | ✅ JWT | ✅ JWT | ✅ Various | ✅ Various |
| **Horizontal Scaling** | ✅ Redis | ✅ Built-in | ✅ Built-in | ✅ Built-in |
| **Cost** | Free (self-host) | Free tier + paid | Paid only | Paid only |

---

## Core Concepts

### 1. Connections

A **connection** represents an active WebSocket link between a client and the server.

**Lifecycle:**
1. Client initiates WebSocket connection
2. Server authenticates via JWT token
3. Connection record created in `realtime.connections` table
4. Client subscribes to channels
5. Client can send/receive messages
6. Connection terminates (disconnect or timeout)
7. Connection record marked as disconnected

**Connection Metadata:**
- User ID (authenticated user)
- Tenant ID (multi-tenancy support)
- Connection ID (unique identifier)
- Client IP and User Agent
- Connected/Last Seen timestamps
- Status (connected, disconnected, idle)

### 2. Channels

**Channels** are communication rooms where users can send messages and receive updates. Think of them as Slack channels or Discord servers.

**Channel Properties:**
- Name and slug (URL-friendly identifier)
- Type (public, private, presence, direct)
- Max members limit
- Message persistence setting
- Custom metadata (JSON)

**Channel Types:** See [Channel Types](#channel-types) section below.

### 3. Messages

**Messages** are persistent chat messages stored in the database.

**Message Types:**
- `text` - Regular text messages
- `system` - System-generated messages (e.g., "User joined")
- `file` - File attachments with metadata
- `event` - Custom application events

**Message Properties:**
- Content (text or JSON)
- Message type
- Metadata (custom JSON data)
- Sent/Edited/Deleted timestamps
- User and channel references

### 4. Broadcasts

**Broadcasts** are ephemeral events that don't persist in the database. Perfect for transient updates like typing indicators and cursor movements.

**Broadcast Properties:**
- Event type (e.g., `typing`, `cursor_move`)
- Payload (JSON data)
- Expiry (default: 5 minutes)
- Channel and user references

**Use Cases:**
- Typing indicators (`typing_start`, `typing_stop`)
- Cursor position updates
- Selection changes in collaborative editors
- Temporary UI state synchronization

### 5. Presence

**Presence** tracks which users are online, their status, and their location in the application.

**Presence Data:**
- User status (online, away, busy, offline)
- Current page/resource
- Cursor position and selection (for collaborative editing)
- Custom metadata (user preferences, device info, etc.)

**Status Types:**
- `online` - User is active
- `away` - User is idle (no activity for X minutes)
- `busy` - User is in focus mode (custom status)
- `offline` - User has disconnected

### 6. Subscriptions

**Subscriptions** represent a user's active subscriptions to channels via a specific connection.

**Features:**
- Per-connection channel subscriptions
- Optional filters for selective message delivery
- Automatic cleanup on disconnect

---

## Getting Started

### Initialize Real-Time System

Before using real-time features, initialize the database schema and WebSocket server:

```bash
# Initialize real-time database schema
nself realtime init

# This creates:
# - realtime schema in PostgreSQL
# - Tables: connections, channels, messages, presence, broadcasts, subscriptions
# - Functions: connect(), disconnect(), send_message(), broadcast(), etc.
# - Triggers and RLS policies
# - WebSocket server service (if not exists)
```

### Start WebSocket Server

```bash
# Start the WebSocket server
nself realtime start

# Check status
nself realtime status
# Output:
# ✓ WebSocket server is running
#   Active connections: 3
```

### View Server Logs

```bash
# Stream WebSocket server logs
nself realtime logs

# Output:
# Connected to PostgreSQL
# WebSocket server running on port 3001
# User connected: 123e4567-e89b-12d3-a456-426614174000 (abc123)
```

### Create Your First Channel

```bash
# Create a public channel
nself realtime channel create "general"
# Output: ✓ Channel created: general (ID: 456e7890-...)

# Create a private channel
nself realtime channel create "team-chat" private

# List all channels
nself realtime channel list
# Output:
#  slug        | name       | type    | members | online | created_at
# -------------|------------|---------|---------|--------|------------------
#  general     | general    | public  | 15      | 3      | 2026-01-29 10:00
#  team-chat   | team-chat  | private | 5       | 2      | 2026-01-29 10:05
```

---

## Channel Types

### 1. Public Channels

Anyone can join and see messages. No permission required.

**Use Cases:**
- General chat rooms
- Announcements
- Public discussions

**Example:**
```bash
nself realtime channel create "announcements" public
```

**Client Usage:**
```javascript
// Anyone can subscribe
await client.subscribe('announcements');
```

### 2. Private Channels

Invite-only channels. Users must be members to join and see messages.

**Use Cases:**
- Team-specific discussions
- Project channels
- Customer support rooms

**Example:**
```bash
nself realtime channel create "project-alpha" private
```

**Client Usage:**
```javascript
// Only members can subscribe
await client.subscribe('project-alpha');
// Will fail if user is not a member
```

**Adding Members:**
```sql
-- Add user to private channel
INSERT INTO realtime.channel_members (channel_id, user_id, role)
SELECT
  (SELECT id FROM realtime.channels WHERE slug = 'project-alpha'),
  '123e4567-e89b-12d3-a456-426614174000'::uuid,
  'member';
```

### 3. Presence Channels

Public or private channels with enhanced presence tracking. Automatically tracks who's online.

**Use Cases:**
- Collaborative editing
- Multi-user dashboards
- Gaming lobbies

**Example:**
```bash
nself realtime channel create "document-123" presence
```

**Client Usage:**
```javascript
await client.subscribe('document-123');

// Get online users
const users = await client.getPresence('document-123');
// Returns: [{ userId: "...", status: "online", cursor_position: {...} }]
```

### 4. Direct Channels

One-on-one private messaging between two users.

**Use Cases:**
- Direct messages
- Customer support 1-on-1
- User-to-user communication

**Example:**
```bash
nself realtime channel create "dm-alice-bob" direct
```

**Client Usage:**
```javascript
// Create direct channel between two users
const channel = `dm-${userId1}-${userId2}`;
await client.subscribe(channel);
```

---

## Client-Side Integration

### JavaScript/TypeScript (WebSocket API)

#### Installation

```bash
npm install socket.io-client
```

#### Basic Connection

```javascript
const { io } = require('socket.io-client');

// Connect to WebSocket server
const socket = io('wss://realtime.yourdomain.com', {
  auth: {
    token: 'your-jwt-token' // From nself auth login
  },
  transports: ['websocket', 'polling']
});

socket.on('connect', () => {
  console.log('Connected:', socket.id);
});

socket.on('disconnect', (reason) => {
  console.log('Disconnected:', reason);
});

socket.on('connect_error', (error) => {
  console.error('Connection error:', error);
});
```

#### Using the nself Client Library

```javascript
const RealtimeClient = require('./realtime-client');

// Initialize client
const client = new RealtimeClient({
  url: 'wss://realtime.yourdomain.com',
  token: 'your-jwt-token',
  maxReconnectAttempts: 5,
  reconnectDelay: 1000
});

// Connect
await client.connect();

// Subscribe to channel
await client.subscribe('general');

// Listen for messages
client.on('message', (data) => {
  console.log('New message:', data);
});

// Send message
await client.send('general', 'Hello, world!');

// Disconnect
client.disconnect();
```

### React Integration

#### Custom Hook

```javascript
import { useEffect, useState } from 'react';
import RealtimeClient from './realtime-client';

export function useRealtime(channel, token) {
  const [client, setClient] = useState(null);
  const [messages, setMessages] = useState([]);
  const [onlineUsers, setOnlineUsers] = useState([]);

  useEffect(() => {
    const rtClient = new RealtimeClient({ token });

    rtClient.connect().then(() => {
      rtClient.subscribe(channel);
      setClient(rtClient);
    });

    // Listen for messages
    rtClient.on('message', (msg) => {
      setMessages(prev => [...prev, msg]);
    });

    // Listen for presence updates
    rtClient.on('presenceUpdate', (data) => {
      rtClient.getPresence(channel).then(setOnlineUsers);
    });

    return () => {
      rtClient.disconnect();
    };
  }, [channel, token]);

  const sendMessage = async (content) => {
    if (client) {
      await client.send(channel, content);
    }
  };

  return { messages, onlineUsers, sendMessage };
}
```

#### Usage in Component

```javascript
function ChatRoom({ channelName, userToken }) {
  const { messages, onlineUsers, sendMessage } = useRealtime(channelName, userToken);
  const [input, setInput] = useState('');

  const handleSend = () => {
    sendMessage(input);
    setInput('');
  };

  return (
    <div>
      <div className="online-users">
        Online: {onlineUsers.map(u => u.userId).join(', ')}
      </div>

      <div className="messages">
        {messages.map(msg => (
          <div key={msg.id}>
            <strong>{msg.userId}:</strong> {msg.content}
          </div>
        ))}
      </div>

      <input
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyPress={(e) => e.key === 'Enter' && handleSend()}
      />
      <button onClick={handleSend}>Send</button>
    </div>
  );
}
```

### Vue Integration

```javascript
import { ref, onMounted, onUnmounted } from 'vue';
import RealtimeClient from './realtime-client';

export function useRealtime(channel, token) {
  const messages = ref([]);
  const onlineUsers = ref([]);
  let client = null;

  onMounted(async () => {
    client = new RealtimeClient({ token });
    await client.connect();
    await client.subscribe(channel);

    client.on('message', (msg) => {
      messages.value.push(msg);
    });

    client.on('presenceUpdate', async () => {
      onlineUsers.value = await client.getPresence(channel);
    });
  });

  onUnmounted(() => {
    if (client) client.disconnect();
  });

  const sendMessage = async (content) => {
    if (client) await client.send(channel, content);
  };

  return { messages, onlineUsers, sendMessage };
}
```

### Python Client

```python
import socketio
import asyncio

class RealtimeClient:
    def __init__(self, url, token):
        self.url = url
        self.token = token
        self.sio = socketio.AsyncClient()
        self.setup_handlers()

    def setup_handlers(self):
        @self.sio.event
        async def connect():
            print('Connected to server')

        @self.sio.event
        async def disconnect():
            print('Disconnected from server')

        @self.sio.on('message:new')
        async def on_message(data):
            print(f"New message: {data}")

    async def connect(self):
        await self.sio.connect(
            self.url,
            auth={'token': self.token},
            transports=['websocket']
        )

    async def subscribe(self, channel):
        await self.sio.emit('subscribe', {'channel': channel})
        await self.sio.wait()

    async def send(self, channel, content):
        await self.sio.emit('message:send', {
            'channel': channel,
            'content': content,
            'messageType': 'text'
        })

    async def disconnect(self):
        await self.sio.disconnect()

# Usage
async def main():
    client = RealtimeClient('wss://realtime.yourdomain.com', 'jwt-token')
    await client.connect()
    await client.subscribe('general')
    await client.send('general', 'Hello from Python!')
    await asyncio.sleep(10)
    await client.disconnect()

asyncio.run(main())
```

---

## Messaging

### Sending Messages

#### Basic Text Message

```javascript
await client.send('general', 'Hello, world!');
```

#### Message with Metadata

```javascript
await client.send('general', 'Check out this feature!', {
  messageType: 'text',
  metadata: {
    priority: 'high',
    tags: ['announcement', 'feature']
  }
});
```

#### File Message

```javascript
await client.send('general', 'file-url-or-id', {
  messageType: 'file',
  metadata: {
    fileName: 'document.pdf',
    fileSize: 1024000,
    mimeType: 'application/pdf'
  }
});
```

### Receiving Messages

```javascript
client.on('message', (data) => {
  console.log('Message ID:', data.id);
  console.log('From:', data.userId);
  console.log('Channel:', data.channelId);
  console.log('Content:', data.content);
  console.log('Type:', data.messageType);
  console.log('Metadata:', data.metadata);
  console.log('Sent at:', data.sentAt);
});
```

### Message History

Use Hasura GraphQL to fetch message history:

```graphql
query GetChannelMessages($channelSlug: String!, $limit: Int = 50) {
  realtime_messages(
    where: {
      channel: { slug: { _eq: $channelSlug } }
      deleted_at: { _is_null: true }
    }
    order_by: { sent_at: desc }
    limit: $limit
  ) {
    id
    content
    message_type
    metadata
    sent_at
    user_id
  }
}
```

### Message Pagination

```graphql
query GetMessagesPaginated(
  $channelSlug: String!,
  $limit: Int = 50,
  $offset: Int = 0
) {
  realtime_messages(
    where: {
      channel: { slug: { _eq: $channelSlug } }
      deleted_at: { _is_null: true }
    }
    order_by: { sent_at: desc }
    limit: $limit
    offset: $offset
  ) {
    id
    content
    sent_at
    user_id
  }

  realtime_messages_aggregate(
    where: {
      channel: { slug: { _eq: $channelSlug } }
      deleted_at: { _is_null: true }
    }
  ) {
    aggregate {
      count
    }
  }
}
```

### Message Search

```graphql
query SearchMessages($channelSlug: String!, $query: String!) {
  realtime_messages(
    where: {
      channel: { slug: { _eq: $channelSlug } }
      content: { _ilike: $query }
      deleted_at: { _is_null: true }
    }
    order_by: { sent_at: desc }
    limit: 100
  ) {
    id
    content
    sent_at
    user_id
  }
}
```

---

## Presence Tracking

### Setting User Status

```javascript
// Set status to online
client.updatePresence('general', 'online', {
  displayName: 'Alice',
  avatar: 'https://...'
});

// Set status to away
client.updatePresence('general', 'away');

// Set status to busy
client.updatePresence('general', 'busy', {
  statusMessage: 'In a meeting'
});
```

### Tracking Cursor Position

For collaborative editing (Google Docs-style):

```javascript
// Update cursor position
client.updatePresence('document-123', 'online', {
  cursor: {
    line: 10,
    column: 5,
    position: 245
  }
});

// Broadcast cursor movement (ephemeral)
client.broadcast('document-123', 'cursor_move', {
  line: 10,
  column: 6,
  position: 246
});
```

### Tracking Selection

```javascript
// Update text selection
client.updatePresence('document-123', 'online', {
  selection: {
    start: { line: 10, column: 0 },
    end: { line: 15, column: 20 }
  }
});
```

### Getting Online Users

```javascript
// Get all online users in channel
const users = await client.getPresence('general');

// Example response:
// [
//   {
//     userId: "123e4567-...",
//     status: "online",
//     cursor_position: { line: 10, column: 5 },
//     updated_at: "2026-01-29T10:30:00Z"
//   },
//   {
//     userId: "456e7890-...",
//     status: "away",
//     cursor_position: null,
//     updated_at: "2026-01-29T10:25:00Z"
//   }
// ]
```

### Presence Events

```javascript
// Listen for users joining
client.on('userJoined', (data) => {
  console.log(`User ${data.userId} joined at ${data.timestamp}`);
});

// Listen for users leaving
client.on('userLeft', (data) => {
  console.log(`User ${data.userId} left at ${data.timestamp}`);
});

// Listen for presence updates
client.on('presenceUpdate', (data) => {
  console.log(`User ${data.userId} status: ${data.status}`);
  console.log('Metadata:', data.metadata);
});
```

---

## Broadcasting Events

Broadcasts are ephemeral events that don't persist in the database. Perfect for transient UI updates.

### Typing Indicators

```javascript
// User starts typing
client.broadcast('general', 'typing_start', {
  displayName: 'Alice'
});

// User stops typing
client.broadcast('general', 'typing_stop', {
  displayName: 'Alice'
});

// Listen for typing events
client.on('broadcast:typing_start', (data) => {
  console.log(`${data.payload.displayName} is typing...`);
});

client.on('broadcast:typing_stop', (data) => {
  console.log(`${data.payload.displayName} stopped typing`);
});
```

### Cursor Movement (Collaborative Editing)

```javascript
// Broadcast cursor position change (high frequency)
document.addEventListener('selectionchange', () => {
  const selection = window.getSelection();
  const position = {
    anchorOffset: selection.anchorOffset,
    focusOffset: selection.focusOffset
  };

  client.broadcast('document-123', 'cursor_move', position);
});

// Listen for other users' cursors
client.on('broadcast:cursor_move', (data) => {
  updateCursor(data.userId, data.payload);
});
```

### Custom Application Events

```javascript
// Broadcast custom event
client.broadcast('game-room', 'player_action', {
  action: 'move',
  direction: 'north',
  position: { x: 10, y: 20 }
});

// Listen for custom events
client.on('broadcast:player_action', (data) => {
  console.log(`Player ${data.userId} action:`, data.payload);
});
```

### Event Expiry

Broadcasts expire automatically after 5 minutes (configurable in migration).

To change expiry:

```sql
-- Update default expiry to 10 minutes
ALTER TABLE realtime.broadcasts
ALTER COLUMN expires_at SET DEFAULT NOW() + INTERVAL '10 minutes';
```

---

## Database Change Streaming

Stream real-time updates from database changes using PostgreSQL NOTIFY/LISTEN.

### How It Works

1. Database triggers send NOTIFY events on table changes
2. WebSocket server LISTENs to these notifications
3. Server broadcasts changes to subscribed clients

### Example: Streaming Table Changes

#### 1. Create Database Trigger

```sql
-- Create function to notify on user changes
CREATE OR REPLACE FUNCTION notify_user_changes()
RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify(
    'table_users',
    json_build_object(
      'operation', TG_OP,
      'record', row_to_json(NEW),
      'old_record', row_to_json(OLD)
    )::text
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Attach trigger to users table
CREATE TRIGGER users_notify
AFTER INSERT OR UPDATE OR DELETE ON public.users
FOR EACH ROW EXECUTE FUNCTION notify_user_changes();
```

#### 2. Subscribe to Database Notifications (Server-Side)

The WebSocket server automatically forwards PostgreSQL notifications to clients:

```javascript
// In server.js (already implemented)
pgClient.on('notification', (msg) => {
  const payload = JSON.parse(msg.payload);
  const channel = msg.channel; // e.g., 'table_users'

  io.to(channel).emit('db:notification', payload);
});

// Listen to specific table notifications
pgClient.query('LISTEN table_users');
```

#### 3. Client-Side Subscription

```javascript
// Subscribe to database notifications
await client.subscribe('table_users');

// Listen for database changes
client.on('dbNotification', (data) => {
  console.log('Operation:', data.operation); // INSERT, UPDATE, DELETE
  console.log('New record:', data.record);
  console.log('Old record:', data.old_record);

  // Update UI accordingly
  if (data.operation === 'INSERT') {
    addUserToList(data.record);
  } else if (data.operation === 'UPDATE') {
    updateUserInList(data.record);
  } else if (data.operation === 'DELETE') {
    removeUserFromList(data.old_record);
  }
});
```

### Use Cases

- **Live Dashboards** - Real-time chart updates from data changes
- **Notifications** - Instant notification when new records are created
- **Data Sync** - Keep client-side cache in sync with database
- **Audit Logs** - Live audit log viewer

---

## Security

### Authentication

All WebSocket connections **require JWT authentication**:

```javascript
// Client must provide valid JWT token
const client = new RealtimeClient({
  token: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...'
});

// Token must contain:
// - sub or user_id (user identifier)
// - tenant_id (for multi-tenancy)
```

Get JWT token from nself auth:

```bash
# Login and get token
nself auth login email@example.com password
# Returns JWT token
```

### Channel Authorization

Channels use Row-Level Security (RLS) policies to control access:

#### Public Channels

```sql
-- Anyone can see public channels
CREATE POLICY channels_public ON realtime.channels
  FOR SELECT
  USING (type = 'public');
```

#### Private Channels

```sql
-- Users can only see channels they're members of
CREATE POLICY channels_member ON realtime.channels
  FOR SELECT
  USING (
    id IN (
      SELECT channel_id FROM realtime.channel_members
      WHERE user_id = tenants.current_user_id()
    )
  );
```

#### Message Access

```sql
-- Users can only see messages in their channels
CREATE POLICY messages_channel_member ON realtime.messages
  FOR SELECT
  USING (
    channel_id IN (
      SELECT channel_id FROM realtime.channel_members
      WHERE user_id = tenants.current_user_id()
    )
    OR
    channel_id IN (
      SELECT id FROM realtime.channels WHERE type = 'public'
    )
  );
```

#### Sending Messages

```sql
-- Users can only send messages to channels they have permission in
CREATE POLICY messages_send ON realtime.messages
  FOR INSERT
  WITH CHECK (
    user_id = tenants.current_user_id()
    AND
    channel_id IN (
      SELECT channel_id FROM realtime.channel_members
      WHERE user_id = tenants.current_user_id()
      AND can_send = true
    )
  );
```

### Rate Limiting

Configure rate limiting in `.env`:

```bash
# WebSocket rate limiting
WEBSOCKET_RATE_LIMIT_ENABLED=true
WEBSOCKET_MAX_MESSAGES_PER_MINUTE=60
WEBSOCKET_MAX_BROADCASTS_PER_MINUTE=120
```

Implement in server:

```javascript
const rateLimit = new Map(); // userId -> { count, resetTime }

socket.on('message:send', async (data) => {
  const userId = socket.userId;
  const limit = process.env.WEBSOCKET_MAX_MESSAGES_PER_MINUTE || 60;

  // Check rate limit
  const now = Date.now();
  const userLimit = rateLimit.get(userId) || { count: 0, resetTime: now + 60000 };

  if (now > userLimit.resetTime) {
    userLimit.count = 0;
    userLimit.resetTime = now + 60000;
  }

  if (userLimit.count >= limit) {
    socket.emit('error', { message: 'Rate limit exceeded' });
    return;
  }

  userLimit.count++;
  rateLimit.set(userId, userLimit);

  // Process message...
});
```

### Message Validation

Validate message content before processing:

```javascript
socket.on('message:send', async (data) => {
  const { content, messageType } = data;

  // Validate content length
  if (!content || content.length === 0) {
    socket.emit('error', { message: 'Content cannot be empty' });
    return;
  }

  if (content.length > 10000) {
    socket.emit('error', { message: 'Content too long (max 10,000 chars)' });
    return;
  }

  // Validate message type
  const validTypes = ['text', 'file', 'system', 'event'];
  if (!validTypes.includes(messageType)) {
    socket.emit('error', { message: 'Invalid message type' });
    return;
  }

  // Sanitize HTML (prevent XSS)
  const sanitizedContent = sanitizeHtml(content);

  // Process message...
});
```

### RLS Integration

The real-time system integrates with PostgreSQL Row-Level Security:

```sql
-- Function to get current user ID from JWT
CREATE OR REPLACE FUNCTION tenants.current_user_id()
RETURNS UUID AS $$
  SELECT NULLIF(current_setting('hasura.user.id', true), '')::uuid;
$$ LANGUAGE sql STABLE;

-- All queries use this function for RLS policies
-- JWT claims are automatically set by Hasura
```

---

## Scaling Real-Time

### Horizontal Scaling with Redis

For multi-server deployments, use Redis as a message broker:

#### 1. Enable Redis

```bash
# In .env
REDIS_ENABLED=true
REDIS_HOST=redis
REDIS_PORT=6379
```

#### 2. Configure Socket.IO Adapter

```javascript
// server.js
const { createAdapter } = require('@socket.io/redis-adapter');
const { createClient } = require('redis');

const pubClient = createClient({
  host: process.env.REDIS_HOST,
  port: process.env.REDIS_PORT
});
const subClient = pubClient.duplicate();

Promise.all([pubClient.connect(), subClient.connect()]).then(() => {
  io.adapter(createAdapter(pubClient, subClient));
  console.log('Redis adapter connected');
});
```

#### 3. Deploy Multiple WebSocket Servers

```yaml
# docker-compose.yml
services:
  websocket-server-1:
    image: nself/websocket-server
    environment:
      - REDIS_ENABLED=true
      - REDIS_HOST=redis
    ports:
      - "3100:3100"

  websocket-server-2:
    image: nself/websocket-server
    environment:
      - REDIS_ENABLED=true
      - REDIS_HOST=redis
    ports:
      - "3101:3100"

  websocket-server-3:
    image: nself/websocket-server
    environment:
      - REDIS_ENABLED=true
      - REDIS_HOST=redis
    ports:
      - "3102:3100"
```

#### 4. Load Balance with Nginx

```nginx
upstream websocket {
    ip_hash; # Sticky sessions
    server websocket-server-1:3100;
    server websocket-server-2:3100;
    server websocket-server-3:3100;
}

server {
    listen 443 ssl;
    server_name realtime.yourdomain.com;

    location / {
        proxy_pass http://websocket;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### Connection Pooling

Use connection pooling for PostgreSQL:

```javascript
const { Pool } = require('pg');

const pool = new Pool({
  host: process.env.POSTGRES_HOST,
  port: process.env.POSTGRES_PORT,
  database: process.env.POSTGRES_DB,
  user: process.env.POSTGRES_USER,
  password: process.env.POSTGRES_PASSWORD,
  max: 20, // Max connections
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 2000
});

// Use pool instead of client
socket.on('message:send', async (data) => {
  const result = await pool.query(
    'SELECT realtime.send_message(...)',
    [...]
  );
});
```

### Performance Tuning

#### Server Configuration

```bash
# .env
WEBSOCKET_PORT=3100
WEBSOCKET_MAX_CONNECTIONS=10000
WEBSOCKET_PING_TIMEOUT=60000
WEBSOCKET_PING_INTERVAL=25000
```

#### PostgreSQL Optimization

```sql
-- Indexes for performance
CREATE INDEX CONCURRENTLY idx_messages_channel_sent
ON realtime.messages(channel_id, sent_at DESC);

CREATE INDEX CONCURRENTLY idx_presence_channel_status
ON realtime.presence(channel_id, status)
WHERE status != 'offline';

-- Partition messages by date (for high volume)
CREATE TABLE realtime.messages_2026_01 PARTITION OF realtime.messages
FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
```

#### Cleanup Automation

```bash
# Add cron job for cleanup
0 * * * * docker exec postgres psql -U postgres -d nself -c "SELECT realtime.cleanup_stale_connections()"
0 */6 * * * docker exec postgres psql -U postgres -d nself -c "SELECT realtime.cleanup_expired_broadcasts()"
```

---

## Monitoring & Debugging

### View Active Connections

```bash
# Show active connections
nself realtime connections

# Output:
#  user_id                              | connected_at         | last_seen_at        | subscribed_channels
# --------------------------------------|----------------------|---------------------|--------------------
#  123e4567-e89b-12d3-a456-426614174000 | 2026-01-29 10:00:00 | 2026-01-29 10:30:00 | 3
#  456e7890-e89b-12d3-a456-426614174001 | 2026-01-29 10:15:00 | 2026-01-29 10:29:00 | 1
```

### View Statistics

```bash
# Show real-time statistics
nself realtime stats

# Output:
#  active_connections | total_channels | messages_24h | online_users
# --------------------|----------------|--------------|-------------
#  15                 | 8              | 1247         | 12
```

### Cleanup Stale Data

```bash
# Manually trigger cleanup
nself realtime cleanup

# Output:
#  stale_connections | expired_broadcasts
# -------------------|-------------------
#  5                 | 142
# ✓ Cleanup complete
```

### Monitor Connection Health

Use Prometheus metrics (if monitoring enabled):

```bash
# Add to prometheus.yml
scrape_configs:
  - job_name: 'websocket'
    static_configs:
      - targets: ['websocket-server:3100']
```

Custom metrics to track:

```javascript
// server.js
const promClient = require('prom-client');

const activeConnections = new promClient.Gauge({
  name: 'websocket_active_connections',
  help: 'Number of active WebSocket connections'
});

const messagesPerSecond = new promClient.Counter({
  name: 'websocket_messages_total',
  help: 'Total number of messages sent',
  labelNames: ['channel', 'type']
});

// Update metrics
io.on('connection', () => {
  activeConnections.inc();
});

io.on('disconnect', () => {
  activeConnections.dec();
});

socket.on('message:send', (data) => {
  messagesPerSecond.inc({
    channel: data.channel,
    type: data.messageType
  });
});
```

### Performance Metrics

Track message delivery latency:

```javascript
socket.on('message:send', async (data) => {
  const startTime = Date.now();

  // Send message
  await sendMessage(data);

  const latency = Date.now() - startTime;
  console.log(`Message delivery latency: ${latency}ms`);

  // Alert if latency is high
  if (latency > 1000) {
    console.warn('High latency detected:', latency);
  }
});
```

### Debugging Tips

#### Enable Debug Logging

```bash
# Set environment variable
DEBUG=socket.io:* node server.js

# Or in .env
DEBUG=socket.io:*
LOG_LEVEL=debug
```

#### Monitor PostgreSQL Notifications

```sql
-- Listen to channel notifications manually
LISTEN channel_general;

-- You'll see notifications in psql:
-- Asynchronous notification "channel_general" with payload "{"type":"new_message",...}" received
```

#### Test Connection

```bash
# Test WebSocket connection
wscat -c wss://realtime.yourdomain.com -H "Authorization: Bearer your-jwt-token"

# Send test message
> {"event": "subscribe", "data": {"channel": "general"}}
```

---

## Advanced Topics

### Custom Message Types

Create custom message types for your application:

```javascript
// Register custom message handler
socket.on('message:custom_type', async (data) => {
  const { channel, customData } = data;

  // Validate custom data structure
  if (!validateCustomData(customData)) {
    socket.emit('error', { message: 'Invalid custom data' });
    return;
  }

  // Store with custom metadata
  await pgClient.query(`
    SELECT realtime.send_message(
      (SELECT id FROM realtime.channels WHERE slug = $1),
      $2::uuid,
      $3,
      'custom_type',
      $4
    )
  `, [
    channel,
    socket.userId,
    JSON.stringify(customData),
    JSON.stringify({ customType: data.customType })
  ]);

  // Broadcast to channel
  io.to(channel).emit('message:custom_type', {
    userId: socket.userId,
    customData,
    timestamp: new Date()
  });
});
```

### File Sharing Through Channels

Coordinate file uploads and sharing:

```javascript
// Client uploads file to storage first
const fileUrl = await uploadToStorage(file);

// Then send file message
await client.send('team-chat', fileUrl, {
  messageType: 'file',
  metadata: {
    fileName: file.name,
    fileSize: file.size,
    mimeType: file.type,
    thumbnailUrl: thumbnailUrl // if image
  }
});

// Other clients receive file message
client.on('message', (data) => {
  if (data.messageType === 'file') {
    displayFileMessage(data.content, data.metadata);
  }
});
```

### Video/Voice Call Signaling

Use WebSocket for WebRTC signaling:

```javascript
// Initiate call
socket.emit('call:initiate', {
  channel: 'direct-alice-bob',
  callType: 'video', // or 'audio'
  offer: rtcPeerConnection.localDescription
});

// Receive call
socket.on('call:initiate', async (data) => {
  const answer = await createAnswer(data.offer);
  socket.emit('call:answer', {
    channel: data.channel,
    answer
  });
});

// Exchange ICE candidates
socket.on('call:ice_candidate', (data) => {
  rtcPeerConnection.addIceCandidate(data.candidate);
});

// End call
socket.emit('call:end', { channel: 'direct-alice-bob' });
```

### Screen Sharing Coordination

Signal screen sharing sessions:

```javascript
// Start screen sharing
client.broadcast('meeting-room', 'screen_share_start', {
  userId: currentUserId,
  displayName: 'Alice'
});

// Other participants receive notification
client.on('broadcast:screen_share_start', (data) => {
  showScreenShareNotification(data.payload.displayName);
});

// Stop screen sharing
client.broadcast('meeting-room', 'screen_share_stop', {
  userId: currentUserId
});
```

### Conflict Resolution in Collaborative Editing

Implement Operational Transformation (OT) or CRDT for conflict-free editing:

```javascript
// Client sends operation
client.broadcast('document-123', 'edit_operation', {
  operation: {
    type: 'insert',
    position: 100,
    content: 'Hello',
    version: 42 // Document version
  }
});

// Server validates and broadcasts
socket.on('broadcast', async (data) => {
  if (data.eventType === 'edit_operation') {
    // Validate operation version
    const currentVersion = await getDocumentVersion(data.channel);

    if (data.payload.operation.version !== currentVersion) {
      // Transform operation to current version
      const transformed = await transformOperation(
        data.payload.operation,
        currentVersion
      );
      data.payload.operation = transformed;
    }

    // Apply and broadcast
    await applyOperation(data.channel, data.payload.operation);
    socket.to(data.channel).emit('broadcast', data);
  }
});
```

### Multi-Tenant Isolation

Ensure tenant isolation in channels:

```sql
-- All channels belong to a tenant
ALTER TABLE realtime.channels
ADD CONSTRAINT channels_tenant_required
CHECK (tenant_id IS NOT NULL);

-- RLS policy for tenant isolation
CREATE POLICY channels_tenant_isolation ON realtime.channels
  FOR ALL
  USING (tenant_id = tenants.current_tenant_id());

-- Same for messages, presence, etc.
CREATE POLICY messages_tenant_isolation ON realtime.messages
  FOR ALL
  USING (
    channel_id IN (
      SELECT id FROM realtime.channels
      WHERE tenant_id = tenants.current_tenant_id()
    )
  );
```

### React Native Integration

```javascript
import { io } from 'socket.io-client';
import { useEffect, useState } from 'react';

export function useRealtime(channel, token) {
  const [socket, setSocket] = useState(null);
  const [messages, setMessages] = useState([]);

  useEffect(() => {
    const newSocket = io('wss://realtime.yourdomain.com', {
      auth: { token },
      transports: ['websocket']
    });

    newSocket.on('connect', () => {
      newSocket.emit('subscribe', { channel });
    });

    newSocket.on('message:new', (msg) => {
      setMessages(prev => [...prev, msg]);
    });

    setSocket(newSocket);

    return () => newSocket.disconnect();
  }, [channel, token]);

  const send = (content) => {
    if (socket) {
      socket.emit('message:send', {
        channel,
        content,
        messageType: 'text'
      });
    }
  };

  return { messages, send };
}
```

---

## Next Steps

- **[Deployment Guide](./Deployment.md)** - Deploy real-time infrastructure to production
- **[Security Guide](./SECURITY.md)** - Security best practices for real-time systems
- **[Database Workflow](./DATABASE-WORKFLOW.md)** - Manage real-time schema and migrations
- **[Examples](./EXAMPLES.md)** - Complete real-time application examples

---

## Additional Resources

- **WebSocket Protocol**: [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455)
- **Socket.IO Documentation**: [socket.io/docs](https://socket.io/docs)
- **PostgreSQL NOTIFY/LISTEN**: [PostgreSQL Docs](https://www.postgresql.org/docs/current/sql-notify.html)
- **Supabase Realtime**: [Comparison reference](https://supabase.com/docs/guides/realtime)

---

**Last Updated**: January 29, 2026
**nself Version**: v0.8.0 (Real-Time Collaboration)
**Status**: Production Ready
