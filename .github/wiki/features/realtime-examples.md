# Real-Time System Examples

This guide provides practical examples for using nself's real-time features.

## Table of Contents

- [Chat Application](#chat-application)
- [Live Dashboard](#live-dashboard)
- [Collaborative Editing](#collaborative-editing)
- [Multiplayer Game](#multiplayer-game)
- [Notification System](#notification-system)

---

## Chat Application

### Backend Setup

```bash
# 1. Initialize real-time
nself realtime init

# 2. Create channels
nself realtime channel create "general" public
nself realtime channel create "random" public
nself realtime channel create "support" private

# 3. Subscribe to user changes
nself realtime subscribe public.users INSERT,UPDATE,DELETE

# 4. Monitor activity
nself realtime presence online general
```

### Client Code (React)

```typescript
import { useEffect, useState } from 'react';
import { createClient } from '@nself/client';

const client = createClient({
  url: process.env.NEXT_PUBLIC_API_URL,
  realtimeUrl: process.env.NEXT_PUBLIC_REALTIME_URL,
});

function ChatRoom({ roomId }: { roomId: string }) {
  const [messages, setMessages] = useState([]);
  const [onlineUsers, setOnlineUsers] = useState([]);

  useEffect(() => {
    // Subscribe to channel
    const channel = client.channel(roomId);

    // Listen for messages
    channel.on('broadcast', { event: 'message' }, (payload) => {
      setMessages((prev) => [...prev, payload]);
    });

    // Track presence
    channel.on('presence', { event: 'sync' }, () => {
      const state = channel.presenceState();
      setOnlineUsers(Object.keys(state));
    });

    // Join channel
    channel.subscribe(async (status) => {
      if (status === 'SUBSCRIBED') {
        await channel.track({
          user_id: currentUser.id,
          username: currentUser.name,
          online_at: new Date().toISOString(),
        });
      }
    });

    return () => {
      channel.unsubscribe();
    };
  }, [roomId]);

  const sendMessage = async (text: string) => {
    const channel = client.channel(roomId);
    await channel.send({
      type: 'broadcast',
      event: 'message',
      payload: {
        user_id: currentUser.id,
        text,
        created_at: new Date().toISOString(),
      },
    });
  };

  return (
    <div className="chat-room">
      <div className="online-users">
        Online: {onlineUsers.length}
      </div>
      <div className="messages">
        {messages.map((msg, i) => (
          <div key={i}>{msg.text}</div>
        ))}
      </div>
      <ChatInput onSend={sendMessage} />
    </div>
  );
}
```

### CLI Testing

```bash
# Create test channel
nself realtime channel create "test-chat" public

# Track presence
nself realtime presence track user-1 test-chat online
nself realtime presence track user-2 test-chat online

# Send messages
nself realtime broadcast test-chat message.sent '{"user": "user-1", "text": "Hello!"}'
nself realtime broadcast test-chat message.sent '{"user": "user-2", "text": "Hi there!"}'

# View messages
nself realtime messages test-chat 10

# Monitor online users
nself realtime presence online test-chat
```

---

## Live Dashboard

### Setup

```bash
# Subscribe to all relevant tables
nself realtime subscribe public.orders INSERT,UPDATE
nself realtime subscribe public.analytics INSERT,UPDATE
nself realtime subscribe public.metrics INSERT,UPDATE

# Create dashboard channel
nself realtime channel create "dashboard" public
```

### Client Code (React)

```typescript
function LiveDashboard() {
  const [orders, setOrders] = useState([]);
  const [metrics, setMetrics] = useState({});

  useEffect(() => {
    // Subscribe to orders table
    const ordersChannel = client
      .channel('realtime:table:public.orders')
      .on('INSERT', (payload) => {
        setOrders((prev) => [payload.new, ...prev]);
        showNotification('New order received!');
      })
      .on('UPDATE', (payload) => {
        setOrders((prev) =>
          prev.map((order) =>
            order.id === payload.new.id ? payload.new : order
          )
        );
      })
      .subscribe();

    // Subscribe to metrics
    const metricsChannel = client
      .channel('realtime:table:public.metrics')
      .on('INSERT', (payload) => {
        setMetrics(payload.new);
      })
      .subscribe();

    return () => {
      ordersChannel.unsubscribe();
      metricsChannel.unsubscribe();
    };
  }, []);

  return (
    <div className="dashboard">
      <MetricsCards metrics={metrics} />
      <OrdersList orders={orders} />
    </div>
  );
}
```

### CLI Testing

```bash
# Insert test order (triggers notification)
psql $DATABASE_URL -c "
  INSERT INTO public.orders (customer_id, amount, status)
  VALUES ('cust-123', 99.99, 'pending');
"

# Update order status (triggers update)
psql $DATABASE_URL -c "
  UPDATE public.orders
  SET status = 'completed'
  WHERE id = '...';
"

# Listen to changes
nself realtime listen public.orders
```

---

## Collaborative Editing

### Setup

```bash
# Create document channel
nself realtime channel create "doc-123" presence

# Subscribe to document table
nself realtime subscribe public.documents INSERT,UPDATE
```

### Client Code (React + Yjs)

```typescript
import { useEffect } from 'react';
import * as Y from 'yjs';

function CollaborativeEditor({ documentId }: { documentId: string }) {
  const [doc] = useState(() => new Y.Doc());
  const [awareness, setAwareness] = useState({});

  useEffect(() => {
    const channel = client.channel(`doc-${documentId}`);

    // Track cursor position
    channel.on('broadcast', { event: 'cursor' }, (payload) => {
      setAwareness((prev) => ({
        ...prev,
        [payload.user_id]: payload.cursor,
      }));
    });

    // Sync document changes
    channel.on('broadcast', { event: 'delta' }, (payload) => {
      Y.applyUpdate(doc, new Uint8Array(payload.update));
    });

    // Track presence
    channel.subscribe(async (status) => {
      if (status === 'SUBSCRIBED') {
        await channel.track({
          user_id: currentUser.id,
          color: getRandomColor(),
        });
      }
    });

    // Send changes
    doc.on('update', (update) => {
      channel.send({
        type: 'broadcast',
        event: 'delta',
        payload: { update: Array.from(update) },
      });
    });

    return () => {
      channel.unsubscribe();
    };
  }, [documentId]);

  return <Editor doc={doc} awareness={awareness} />;
}
```

### CLI Testing

```bash
# Track multiple users
nself realtime presence track user-alice doc-123 online '{"color": "#ff0000"}'
nself realtime presence track user-bob doc-123 online '{"color": "#00ff00"}'

# View who's editing
nself realtime presence online doc-123

# Monitor document changes
nself realtime messages doc-123 50
```

---

## Multiplayer Game

### Setup

```bash
# Create game lobby
nself realtime channel create "game-lobby" presence

# Create game rooms
nself realtime channel create "game-room-1" presence
nself realtime channel create "game-room-2" presence

# Subscribe to game state
nself realtime subscribe public.game_state INSERT,UPDATE
```

### Client Code (React)

```typescript
function GameLobby() {
  const [players, setPlayers] = useState([]);

  useEffect(() => {
    const channel = client.channel('game-lobby');

    // Track player presence
    channel.on('presence', { event: 'sync' }, () => {
      const state = channel.presenceState();
      setPlayers(
        Object.entries(state).map(([id, presence]) => ({
          id,
          ...presence[0],
        }))
      );
    });

    // Listen for game invites
    channel.on('broadcast', { event: 'game.invite' }, (payload) => {
      showInviteModal(payload);
    });

    // Join lobby
    channel.subscribe(async (status) => {
      if (status === 'SUBSCRIBED') {
        await channel.track({
          player_id: currentPlayer.id,
          name: currentPlayer.name,
          level: currentPlayer.level,
          status: 'waiting',
        });
      }
    });

    return () => {
      channel.unsubscribe();
    };
  }, []);

  return (
    <div className="lobby">
      <PlayerList players={players} />
    </div>
  );
}

function GameRoom({ roomId }: { roomId: string }) {
  const [gameState, setGameState] = useState({});

  useEffect(() => {
    const channel = client.channel(`game-room-${roomId}`);

    // Listen for player moves
    channel.on('broadcast', { event: 'player.move' }, (payload) => {
      updatePlayerPosition(payload);
    });

    // Sync game state
    channel.on('broadcast', { event: 'game.state' }, (payload) => {
      setGameState(payload);
    });

    channel.subscribe();

    return () => {
      channel.unsubscribe();
    };
  }, [roomId]);

  const sendMove = (position) => {
    const channel = client.channel(`game-room-${roomId}`);
    channel.send({
      type: 'broadcast',
      event: 'player.move',
      payload: {
        player_id: currentPlayer.id,
        position,
        timestamp: Date.now(),
      },
    });
  };

  return <GameCanvas gameState={gameState} onMove={sendMove} />;
}
```

### CLI Testing

```bash
# Join lobby
nself realtime presence track player-1 game-lobby online '{"level": 10}'
nself realtime presence track player-2 game-lobby online '{"level": 8}'

# View players
nself realtime presence online game-lobby

# Send game invite
nself realtime broadcast game-lobby game.invite '{
  "from": "player-1",
  "to": "player-2",
  "room": "game-room-1"
}'

# Track game moves
nself realtime broadcast game-room-1 player.move '{
  "player_id": "player-1",
  "position": {"x": 100, "y": 200}
}'

# View game activity
nself realtime messages game-room-1 20
```

---

## Notification System

### Setup

```bash
# Create notification channel per user
nself realtime channel create "notifications-user-123" private

# Subscribe to notifications table
nself realtime subscribe public.notifications INSERT
```

### Client Code (React)

```typescript
function NotificationBell() {
  const [notifications, setNotifications] = useState([]);
  const [unreadCount, setUnreadCount] = useState(0);

  useEffect(() => {
    // Subscribe to user's notification channel
    const channel = client
      .channel('realtime:table:public.notifications')
      .on('INSERT', (payload) => {
        // Only show if notification is for current user
        if (payload.new.user_id === currentUser.id) {
          setNotifications((prev) => [payload.new, ...prev]);
          setUnreadCount((prev) => prev + 1);
          showToast(payload.new.message);
        }
      })
      .subscribe();

    // Subscribe to personal channel for direct notifications
    const personalChannel = client
      .channel(`notifications-${currentUser.id}`)
      .on('broadcast', { event: 'notification' }, (payload) => {
        setNotifications((prev) => [payload, ...prev]);
        setUnreadCount((prev) => prev + 1);
        showToast(payload.message);
      })
      .subscribe();

    return () => {
      channel.unsubscribe();
      personalChannel.unsubscribe();
    };
  }, []);

  const markAsRead = async (notificationId) => {
    await fetch(`/api/notifications/${notificationId}`, {
      method: 'PATCH',
      body: JSON.stringify({ read: true }),
    });
    setUnreadCount((prev) => Math.max(0, prev - 1));
  };

  return (
    <div className="notification-bell">
      {unreadCount > 0 && <Badge count={unreadCount} />}
      <NotificationList
        notifications={notifications}
        onMarkAsRead={markAsRead}
      />
    </div>
  );
}
```

### CLI Testing

```bash
# Send notification via database insert
psql $DATABASE_URL -c "
  INSERT INTO public.notifications (user_id, message, type)
  VALUES ('user-123', 'You have a new message', 'message');
"

# Send direct notification
nself realtime broadcast notifications-user-123 notification '{
  "message": "Your order has shipped!",
  "type": "order_update",
  "link": "/orders/123"
}'

# View user notifications
nself realtime messages notifications-user-123 10
```

---

## Advanced Patterns

### Optimistic Updates

```typescript
function TodoList() {
  const [todos, setTodos] = useState([]);

  const addTodo = async (text) => {
    const optimisticId = `temp-${Date.now()}`;

    // Optimistic update
    setTodos((prev) => [...prev, { id: optimisticId, text, done: false }]);

    try {
      // Server request
      const { data } = await client.from('todos').insert({ text });

      // Replace temp ID with real ID
      setTodos((prev) =>
        prev.map((todo) =>
          todo.id === optimisticId ? data[0] : todo
        )
      );
    } catch (error) {
      // Rollback on error
      setTodos((prev) => prev.filter((todo) => todo.id !== optimisticId));
    }
  };

  useEffect(() => {
    // Subscribe to todos
    const channel = client
      .channel('realtime:table:public.todos')
      .on('INSERT', (payload) => {
        // Skip if already optimistically added
        if (!todos.find((t) => t.id === payload.new.id)) {
          setTodos((prev) => [...prev, payload.new]);
        }
      })
      .subscribe();

    return () => channel.unsubscribe();
  }, []);

  return <div>{/* ... */}</div>;
}
```

### Message Replay (Catch-up)

```typescript
function ChatWithReplay({ channelId }) {
  const [messages, setMessages] = useState([]);
  const lastSeenRef = useRef(Date.now());

  useEffect(() => {
    const channel = client.channel(channelId);

    // On reconnect, replay missed messages
    channel.on('system', { event: 'connected' }, async () => {
      const { data } = await fetch(
        `/api/realtime/replay/${channelId}?since=${lastSeenRef.current}`
      );

      if (data.length > 0) {
        setMessages((prev) => [...data, ...prev]);
        lastSeenRef.current = Date.now();
      }
    });

    channel.on('broadcast', { event: 'message' }, (payload) => {
      setMessages((prev) => [...prev, payload]);
      lastSeenRef.current = Date.now();
    });

    channel.subscribe();

    return () => channel.unsubscribe();
  }, [channelId]);

  return <MessageList messages={messages} />;
}
```

### CLI for Replay

```bash
# Get timestamp when user disconnected
DISCONNECT_TIME=$(date -u +%s -d "5 minutes ago")

# Replay all messages since disconnect
nself realtime replay general $DISCONNECT_TIME
```

---

## Performance Tips

### Batch Updates

```typescript
// Bad: Individual broadcasts
todos.forEach((todo) => {
  channel.send({ type: 'broadcast', event: 'todo.update', payload: todo });
});

// Good: Batch broadcast
channel.send({
  type: 'broadcast',
  event: 'todos.batch_update',
  payload: { todos },
});
```

### Throttle Presence Updates

```typescript
import { throttle } from 'lodash';

const updatePresence = throttle(
  (data) => {
    channel.track(data);
  },
  1000,
  { leading: true, trailing: true }
);

// Use throttled version
updatePresence({ cursor: { x: 100, y: 200 } });
```

### Filter Events Client-Side

```typescript
// Subscribe once, filter locally
channel
  .on('broadcast', {}, (payload) => {
    if (payload.event === 'message' && payload.room_id === currentRoom) {
      handleMessage(payload);
    }
  })
  .subscribe();
```

---

For more examples, see:
- [Real-time Documentation](./REALTIME.md)
- Client SDK Reference
