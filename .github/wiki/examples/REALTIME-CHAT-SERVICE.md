# Example: Building a Real-time Chat Service

This guide shows how to build a production-ready real-time chat backend using nself's Socket.IO template.

## What We'll Build

A complete chat backend with:
- ✅ WebSocket-based real-time messaging
- ✅ Multiple chat rooms
- ✅ User presence tracking
- ✅ Typing indicators
- ✅ Redis adapter for horizontal scaling
- ✅ Integration with Hasura for message persistence
- ✅ Health checks and monitoring

## Prerequisites

```bash
# Initialize nself project
nself init --demo
```

## Step 1: Generate the Socket.IO Service

```bash
nself service scaffold realtime --template socketio-ts --port 3101
```

This creates:
```
services/realtime/
├── package.json
├── tsconfig.json
├── Dockerfile
├── README.md
└── src/
    └── server.ts
```

## Step 2: Review Generated Code

The generated `src/server.ts` includes:

```typescript
import express from 'express';
import { createServer } from 'http';
import { Server, Socket } from 'socket.io';

const app = express();
const server = createServer(app);

// Socket.IO with CORS
const io = new Server(server, {
  cors: {
    origin: process.env.CORS_ORIGIN || '*',
    credentials: true
  }
});

// Type-safe events
interface MessageData {
  text: string;
  user?: string;
  room?: string;
}

// Connection handling
io.on('connection', (socket: Socket) => {
  console.log(`User connected: ${socket.id}`);

  socket.on('message', (data: MessageData) => {
    // Handle message
    socket.broadcast.emit('broadcast_message', {
      message: data,
      from: socket.id,
      timestamp: new Date().toISOString()
    });
  });

  // Room support
  socket.on('join_room', (room: string) => {
    socket.join(room);
    socket.to(room).emit('user_joined', {
      socketId: socket.id,
      room
    });
  });
});

const PORT = process.env.PORT || 3101;
server.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});
```

## Step 3: Add Redis Adapter for Scaling

Install the Redis adapter:

```bash
cd services/realtime
npm install @socket.io/redis-adapter ioredis
```

Update `src/server.ts`:

```typescript
import { createAdapter } from '@socket.io/redis-adapter';
import { createClient } from 'redis';

// Add after io initialization
const redisUrl = process.env.REDIS_URL || 'redis://localhost:6379';
const pubClient = createClient({ url: redisUrl });
const subClient = pubClient.duplicate();

Promise.all([pubClient.connect(), subClient.connect()]).then(() => {
  io.adapter(createAdapter(pubClient, subClient));
  console.log('✓ Redis adapter connected');
});
```

## Step 4: Add Message Persistence with Hasura

Add GraphQL mutations for saving messages:

```typescript
import fetch from 'node-fetch';

const HASURA_ENDPOINT = process.env.HASURA_GRAPHQL_ENDPOINT;
const HASURA_SECRET = process.env.HASURA_GRAPHQL_ADMIN_SECRET;

async function saveMessage(data: {
  room_id: string;
  user_id: string;
  text: string;
}) {
  const mutation = `
    mutation InsertMessage($room_id: uuid!, $user_id: uuid!, $text: String!) {
      insert_messages_one(object: {
        room_id: $room_id
        user_id: $user_id
        text: $text
      }) {
        id
        created_at
      }
    }
  `;

  const response = await fetch(HASURA_ENDPOINT, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-hasura-admin-secret': HASURA_SECRET
    },
    body: JSON.stringify({
      query: mutation,
      variables: data
    })
  });

  return response.json();
}

// Use in message handler
socket.on('message', async (data: MessageData) => {
  // Save to database
  await saveMessage({
    room_id: data.room,
    user_id: socket.data.userId,
    text: data.text
  });

  // Broadcast to room
  socket.to(data.room).emit('new_message', {
    text: data.text,
    user_id: socket.data.userId,
    timestamp: new Date().toISOString()
  });
});
```

## Step 5: Add User Authentication

Add authentication middleware:

```typescript
import jwt from 'jsonwebtoken';

io.use((socket, next) => {
  const token = socket.handshake.auth.token;

  if (!token) {
    return next(new Error('Authentication required'));
  }

  try {
    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    socket.data.userId = decoded.userId;
    socket.data.username = decoded.username;
    next();
  } catch (err) {
    next(new Error('Invalid token'));
  }
});
```

## Step 6: Add Typing Indicators

```typescript
socket.on('typing_start', (room: string) => {
  socket.to(room).emit('user_typing', {
    userId: socket.data.userId,
    username: socket.data.username,
    room
  });
});

socket.on('typing_stop', (room: string) => {
  socket.to(room).emit('user_stopped_typing', {
    userId: socket.data.userId,
    room
  });
});
```

## Step 7: Add Presence Tracking

```typescript
// Track online users per room
const roomUsers = new Map<string, Set<string>>();

socket.on('join_room', async (room: string) => {
  socket.join(room);

  // Track user in room
  if (!roomUsers.has(room)) {
    roomUsers.set(room, new Set());
  }
  roomUsers.get(room).add(socket.data.userId);

  // Notify others
  socket.to(room).emit('user_joined', {
    userId: socket.data.userId,
    username: socket.data.username,
    room
  });

  // Send current users to new joiner
  const users = Array.from(roomUsers.get(room));
  socket.emit('room_users', { room, users });
});

socket.on('disconnect', () => {
  // Remove from all rooms
  for (const [room, users] of roomUsers.entries()) {
    if (users.has(socket.data.userId)) {
      users.delete(socket.data.userId);

      io.to(room).emit('user_left', {
        userId: socket.data.userId,
        username: socket.data.username,
        room
      });
    }
  }
});
```

## Step 8: Configure nself Environment

Edit `.env`:

```bash
# Enable Redis for Socket.IO adapter
REDIS_ENABLED=true

# Add the realtime service
CS_2=realtime:socketio-ts:3101:ws
CS_2_REDIS_PREFIX=chat:
CS_2_REPLICAS=2
CS_2_MEMORY=512M
CS_2_ENV=ENABLE_PRESENCE=true,ENABLE_TYPING=true

# CORS for frontend
CORS_ORIGIN=https://yourdomain.com
```

## Step 9: Create Hasura Schema

Create `hasura/migrations/001_chat_schema.sql`:

```sql
-- Rooms table
CREATE TABLE rooms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Messages table
CREATE TABLE messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  text TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_messages_room ON messages(room_id, created_at DESC);
CREATE INDEX idx_messages_user ON messages(user_id, created_at DESC);

-- Set up Hasura permissions
-- (Allow authenticated users to read messages in their rooms)
```

## Step 10: Build and Start

```bash
# Build all services
nself build

# Start the stack
nself start

# Check health
curl http://localhost:3101/health
```

## Step 11: Create Frontend Client

Example React client:

```typescript
// src/lib/socket-client.ts
import { io, Socket } from 'socket.io-client';

interface ServerToClientEvents {
  welcome: (data: any) => void;
  new_message: (data: { text: string; user_id: string; timestamp: string }) => void;
  user_joined: (data: { userId: string; username: string; room: string }) => void;
  user_left: (data: { userId: string; username: string; room: string }) => void;
  user_typing: (data: { userId: string; username: string; room: string }) => void;
  user_stopped_typing: (data: { userId: string; room: string }) => void;
  room_users: (data: { room: string; users: string[] }) => void;
}

interface ClientToServerEvents {
  message: (data: { text: string; room: string }) => void;
  join_room: (room: string) => void;
  leave_room: (room: string) => void;
  typing_start: (room: string) => void;
  typing_stop: (room: string) => void;
}

export const createSocketClient = (token: string): Socket<ServerToClientEvents, ClientToServerEvents> => {
  return io('http://localhost:3101', {
    auth: { token }
  });
};
```

```typescript
// src/components/ChatRoom.tsx
import { useEffect, useState } from 'react';
import { createSocketClient } from '../lib/socket-client';

export function ChatRoom({ roomId, token }: { roomId: string; token: string }) {
  const [socket, setSocket] = useState(null);
  const [messages, setMessages] = useState([]);
  const [inputText, setInputText] = useState('');

  useEffect(() => {
    const s = createSocketClient(token);
    setSocket(s);

    // Join room
    s.emit('join_room', roomId);

    // Listen for messages
    s.on('new_message', (data) => {
      setMessages((prev) => [...prev, data]);
    });

    // Listen for typing
    s.on('user_typing', (data) => {
      console.log(`${data.username} is typing...`);
    });

    return () => {
      s.emit('leave_room', roomId);
      s.disconnect();
    };
  }, [roomId, token]);

  const sendMessage = () => {
    socket?.emit('message', { text: inputText, room: roomId });
    setInputText('');
  };

  const handleTyping = () => {
    socket?.emit('typing_start', roomId);

    // Stop typing after 2 seconds
    setTimeout(() => {
      socket?.emit('typing_stop', roomId);
    }, 2000);
  };

  return (
    <div className="chat-room">
      <div className="messages">
        {messages.map((msg, i) => (
          <div key={i} className="message">
            <strong>{msg.user_id}:</strong> {msg.text}
            <span className="timestamp">{msg.timestamp}</span>
          </div>
        ))}
      </div>

      <input
        type="text"
        value={inputText}
        onChange={(e) => {
          setInputText(e.target.value);
          handleTyping();
        }}
        onKeyPress={(e) => e.key === 'Enter' && sendMessage()}
      />
      <button onClick={sendMessage}>Send</button>
    </div>
  );
}
```

## Step 12: Test the Service

### Test Health Check

```bash
curl http://localhost:3101/health

# Response:
{
  "status": "healthy",
  "service": "realtime",
  "timestamp": "2024-01-30T12:00:00.000Z",
  "connections": 0
}
```

### Test WebSocket Connection

```javascript
// Node.js test client
const io = require('socket.io-client');

const socket = io('http://localhost:3101', {
  auth: { token: 'your-jwt-token' }
});

socket.on('welcome', (data) => {
  console.log('Welcome:', data);

  // Join room
  socket.emit('join_room', 'general');

  // Send message
  socket.emit('message', {
    text: 'Hello, world!',
    room: 'general'
  });
});

socket.on('new_message', (data) => {
  console.log('Message received:', data);
});
```

## Step 13: Monitor and Scale

### View Logs

```bash
nself service logs realtime --follow
```

### Check Metrics

```bash
curl http://localhost:3101/api/info

# Response includes:
# - Uptime
# - Memory usage
# - Connection count
```

### Scale Horizontally

The Redis adapter allows multiple instances:

```bash
# In .env
CS_2_REPLICAS=3  # Run 3 instances

# Rebuild and restart
nself build && nself restart
```

Nginx load balancer distributes WebSocket connections across all instances.

## Production Deployment

### 1. Environment Variables

```bash
# Production .env
ENV=production
BASE_DOMAIN=yourdomain.com

# Enable SSL
SSL_ENABLED=true

# Redis for production
REDIS_URL=redis://production-redis:6379

# JWT secret
JWT_SECRET=your-production-secret

# CORS
CORS_ORIGIN=https://yourdomain.com
```

### 2. Deploy

```bash
# On production server
nself build && nself start
```

### 3. Access

- WebSocket: `wss://ws.yourdomain.com`
- Health: `https://ws.yourdomain.com/health`
- API Info: `https://ws.yourdomain.com/api/info`

## Performance Tips

### 1. Enable Redis Adapter

Always use Redis adapter for multiple instances:

```typescript
io.adapter(createAdapter(pubClient, subClient));
```

### 2. Use Rooms Efficiently

Group users by rooms to reduce broadcast overhead:

```typescript
// Instead of broadcasting to all
io.emit('message', data);  // ❌ Broadcasts to everyone

// Use rooms
io.to(roomId).emit('message', data);  // ✅ Only to room members
```

### 3. Rate Limiting

Prevent spam with rate limiting:

```typescript
import rateLimit from 'express-rate-limit';

const limiter = rateLimit({
  windowMs: 1000, // 1 second
  max: 10 // 10 messages per second
});

app.use('/api', limiter);
```

### 4. Message Compression

Enable compression for Socket.IO:

```typescript
const io = new Server(server, {
  perMessageDeflate: true,
  httpCompression: true
});
```

## Troubleshooting

### Connection Issues

```bash
# Check if service is running
nself service status realtime

# Check logs
nself service logs realtime

# Test health endpoint
curl http://localhost:3101/health
```

### Redis Connection Issues

```bash
# Check Redis is running
docker ps | grep redis

# Test Redis connection
redis-cli ping

# Check Redis URL
echo $REDIS_URL
```

### CORS Issues

Update CORS configuration in `src/server.ts`:

```typescript
const io = new Server(server, {
  cors: {
    origin: process.env.CORS_ORIGIN || 'https://yourdomain.com',
    methods: ['GET', 'POST'],
    credentials: true
  }
});
```

## Summary

You now have a production-ready real-time chat service with:

✅ WebSocket communication
✅ Multi-room support
✅ Presence tracking
✅ Typing indicators
✅ Message persistence
✅ Horizontal scaling
✅ Authentication
✅ Health monitoring

## Next Steps

- Add file upload support
- Implement message reactions
- Add read receipts
- Create message search
- Add voice/video calls (WebRTC)
- Implement message encryption

## See Also

- [Socket.IO Documentation](https://socket.io/docs/)
- [Service Templates Reference](../reference/SERVICE-SCAFFOLDING-CHEATSHEET.md)
- [Custom Services Guide](../services/SERVICES_CUSTOM.md)
