# Service-to-Service Communication Guide

Complete guide to internal service communication in nself using Docker DNS, service mesh patterns, and best practices.

## Table of Contents

- [Overview](#overview)
- [Internal DNS Resolution](#internal-dns-resolution)
- [Communication Patterns](#communication-patterns)
- [Authentication & Security](#authentication--security)
- [Load Balancing](#load-balancing)
- [Health Checks](#health-checks)
- [Circuit Breakers](#circuit-breakers)
- [Service Discovery](#service-discovery)
- [Real-World Examples](#real-world-examples)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Overview

In nself, all Docker services share a common network and can communicate using internal DNS names. This enables microservices architecture without complex networking configuration.

### Key Concepts

1. **Internal DNS** - Docker provides automatic DNS resolution for service names
2. **Service Names** - Each service is accessible by its container/service name
3. **Network Isolation** - All services on the same Docker network can communicate
4. **No External Exposure** - Internal communication doesn't require nginx routes

### Basic Communication Flow

```
┌─────────────┐          ┌─────────────┐
│   Service   │          │   Service   │
│     A       │───────▶  │     B       │
│  (Client)   │  HTTP    │  (Server)   │
└─────────────┘          └─────────────┘
     │                         │
     └────── http://service-b:8002 ──────┘
              (Internal DNS)
```

---

## Internal DNS Resolution

Docker provides automatic DNS resolution for all services on the same network.

### Service Name Format

**Container Name:** `${PROJECT_NAME}_<service_name>`
**DNS Name:** `<service_name>` (without project prefix)

### Example

```bash
# .env
PROJECT_NAME=myapp

# Service definition
CS_1=api:express-ts:8001
CS_2=worker:bullmq-ts:8002

# Container names (in Docker):
# - myapp_api
# - myapp_worker

# DNS names (for communication):
# - api
# - worker
```

### Accessing Services

**From Service A to Service B:**

```javascript
// ✅ CORRECT - Use service name
const response = await fetch('http://api:8001/users');
const response = await fetch('http://worker:8002/status');

// ❌ WRONG - Don't use container name
const response = await fetch('http://myapp_api:8001/users');

// ❌ WRONG - Don't use external domain
const response = await fetch('https://api.example.com/users');
```

### Service Communication URLs

```bash
# Core Services (always available)
http://postgres:5432           # PostgreSQL database
http://redis:6379              # Redis cache
http://hasura:8080             # Hasura GraphQL
http://auth:4000               # nHost Auth
http://nginx:80                # Nginx (internal routing)

# Optional Services (when enabled)
http://minio:9000              # MinIO storage
http://meilisearch:7700        # MeiliSearch
http://mailpit:1025            # MailPit SMTP
http://mailpit:8025            # MailPit Web UI

# Custom Services (CS_N)
http://api:8001                # CS_1=api:express-ts:8001
http://worker:8002             # CS_2=worker:bullmq-ts:8002
http://processor:8003          # CS_3=processor:fastapi:8003
```

---

## Communication Patterns

### Pattern 1: Request-Response (HTTP/REST)

**Use Case:** Synchronous API calls between services

**Example:**

```bash
# Service definitions
CS_1=api:express-ts:8001:api           # Public-facing API
CS_2=user-service:express-ts:8002      # Internal user service
CS_3=order-service:express-ts:8003     # Internal order service
```

**API Gateway (CS_1):**

```javascript
// src/routes/users.js
import express from 'express';

const router = express.Router();

router.get('/users/:id', async (req, res) => {
  try {
    // Call internal user service
    const response = await fetch(`http://user-service:8002/users/${req.params.id}`);
    const user = await response.json();

    res.json(user);
  } catch (error) {
    res.status(500).json({ error: 'User service unavailable' });
  }
});

router.get('/users/:id/orders', async (req, res) => {
  try {
    // Call user service
    const userRes = await fetch(`http://user-service:8002/users/${req.params.id}`);
    const user = await userRes.json();

    // Call order service
    const orderRes = await fetch(`http://order-service:8003/orders?userId=${req.params.id}`);
    const orders = await orderRes.json();

    res.json({ user, orders });
  } catch (error) {
    res.status(500).json({ error: 'Service unavailable' });
  }
});

export default router;
```

**User Service (CS_2):**

```javascript
// src/routes/users.js
router.get('/users/:id', async (req, res) => {
  // Direct database access (internal service)
  const user = await db.users.findById(req.params.id);
  res.json(user);
});
```

**Order Service (CS_3):**

```javascript
// src/routes/orders.js
router.get('/orders', async (req, res) => {
  const { userId } = req.query;
  const orders = await db.orders.findByUserId(userId);
  res.json(orders);
});
```

---

### Pattern 2: Event-Driven (Pub/Sub via Redis)

**Use Case:** Asynchronous event processing, decoupled services

**Example:**

```bash
CS_1=api:express-ts:8001:api
CS_2=email-worker:bullmq-ts:8002
CS_3=analytics-worker:bullmq-ts:8003

CS_1_REDIS_PREFIX=events:
CS_2_REDIS_PREFIX=events:
CS_3_REDIS_PREFIX=events:
```

**API Service (CS_1) - Event Publisher:**

```javascript
// src/services/events.js
import { Queue } from 'bullmq';

const userQueue = new Queue('user-events', {
  connection: {
    host: 'redis',
    port: 6379,
  },
  prefix: 'events:',
});

export async function publishUserCreated(user) {
  await userQueue.add('user.created', {
    userId: user.id,
    email: user.email,
    createdAt: new Date(),
  });
}

// src/routes/users.js
router.post('/users', async (req, res) => {
  const user = await db.users.create(req.body);

  // Publish event (fire and forget)
  await publishUserCreated(user);

  res.status(201).json(user);
});
```

**Email Worker (CS_2) - Event Consumer:**

```javascript
// src/worker.js
import { Worker } from 'bullmq';

const worker = new Worker('user-events', async (job) => {
  if (job.name === 'user.created') {
    const { email } = job.data;

    // Send welcome email
    await sendEmail(email, 'Welcome!', welcomeTemplate);

    console.log(`Welcome email sent to ${email}`);
  }
}, {
  connection: {
    host: 'redis',
    port: 6379,
  },
  prefix: 'events:',
});

worker.on('completed', (job) => {
  console.log(`Job ${job.id} completed`);
});
```

**Analytics Worker (CS_3) - Event Consumer:**

```javascript
// src/worker.js
import { Worker } from 'bullmq';

const worker = new Worker('user-events', async (job) => {
  if (job.name === 'user.created') {
    const { userId, createdAt } = job.data;

    // Track user signup in analytics
    await analytics.track({
      event: 'User Signup',
      userId,
      timestamp: createdAt,
    });

    console.log(`User ${userId} tracked in analytics`);
  }
}, {
  connection: {
    host: 'redis',
    port: 6379,
  },
  prefix: 'events:',
});
```

---

### Pattern 3: gRPC Communication

**Use Case:** High-performance RPC between services

**Example:**

```bash
CS_1=api:express-ts:8001:api
CS_2=grpc-service:grpc:50051
CS_2_PORTS=50051:50051
```

**gRPC Service (CS_2):**

```go
// services/grpc-service/main.go
package main

import (
    "context"
    "log"
    "net"

    pb "grpc-service/proto"
    "google.golang.org/grpc"
)

type server struct {
    pb.UnimplementedUserServiceServer
}

func (s *server) GetUser(ctx context.Context, req *pb.UserRequest) (*pb.UserResponse, error) {
    // Fetch user from database
    user := &pb.UserResponse{
        Id:    req.Id,
        Name:  "John Doe",
        Email: "john@example.com",
    }
    return user, nil
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    s := grpc.NewServer()
    pb.RegisterUserServiceServer(s, &server{})

    log.Printf("gRPC server listening on :50051")
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
```

**API Service (CS_1) - gRPC Client:**

```javascript
// src/services/grpc-client.js
import grpc from '@grpc/grpc-js';
import protoLoader from '@grpc/proto-loader';

const packageDefinition = protoLoader.loadSync('user.proto');
const proto = grpc.loadPackageDefinition(packageDefinition);

const client = new proto.UserService(
  'grpc-service:50051',  // Internal DNS
  grpc.credentials.createInsecure()
);

export function getUserById(userId) {
  return new Promise((resolve, reject) => {
    client.GetUser({ id: userId }, (error, response) => {
      if (error) reject(error);
      else resolve(response);
    });
  });
}

// src/routes/users.js
router.get('/users/:id', async (req, res) => {
  try {
    const user = await getUserById(req.params.id);
    res.json(user);
  } catch (error) {
    res.status(500).json({ error: 'gRPC service unavailable' });
  }
});
```

---

### Pattern 4: WebSocket Communication

**Use Case:** Real-time bidirectional communication

**Example:**

```bash
CS_1=api:express-ts:8001:api
CS_2=websocket:socketio-ts:8002:ws
CS_3=worker:bullmq-ts:8003

CS_2_REDIS_PREFIX=ws:
CS_3_ENV=WS_URL=http://websocket:8002
```

**WebSocket Server (CS_2):**

```javascript
// src/server.js
import { Server } from 'socket.io';
import { createServer } from 'http';
import { createAdapter } from '@socket.io/redis-adapter';
import { createClient } from 'redis';

const httpServer = createServer();
const io = new Server(httpServer, {
  cors: { origin: '*' },
});

// Redis adapter for multi-instance support
const pubClient = createClient({ host: 'redis', port: 6379 });
const subClient = pubClient.duplicate();

Promise.all([pubClient.connect(), subClient.connect()]).then(() => {
  io.adapter(createAdapter(pubClient, subClient));
});

io.on('connection', (socket) => {
  console.log(`Client connected: ${socket.id}`);

  socket.on('join-room', (roomId) => {
    socket.join(roomId);
    console.log(`Client ${socket.id} joined room ${roomId}`);
  });

  socket.on('disconnect', () => {
    console.log(`Client disconnected: ${socket.id}`);
  });
});

// Expose HTTP endpoint for internal services to emit events
import express from 'express';
const app = express();
app.use(express.json());

app.post('/emit', (req, res) => {
  const { room, event, data } = req.body;

  if (room) {
    io.to(room).emit(event, data);
  } else {
    io.emit(event, data);
  }

  res.json({ success: true });
});

httpServer.listen(8002, () => {
  console.log('WebSocket server listening on :8002');
});
```

**API Service (CS_1) - Emit Events:**

```javascript
// src/services/websocket.js
import fetch from 'node-fetch';

export async function emitToRoom(room, event, data) {
  await fetch('http://websocket:8002/emit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ room, event, data }),
  });
}

// src/routes/messages.js
router.post('/messages', async (req, res) => {
  const message = await db.messages.create(req.body);

  // Emit to WebSocket clients
  await emitToRoom(message.roomId, 'new-message', message);

  res.status(201).json(message);
});
```

**Worker (CS_3) - Emit from Background Job:**

```javascript
// src/worker.js
import { Worker } from 'bullmq';
import fetch from 'node-fetch';

const worker = new Worker('notifications', async (job) => {
  const { userId, notification } = job.data;

  // Emit notification via WebSocket
  await fetch('http://websocket:8002/emit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      room: `user:${userId}`,
      event: 'notification',
      data: notification,
    }),
  });
}, {
  connection: { host: 'redis', port: 6379 },
});
```

---

## Authentication & Security

### Pattern 1: Shared JWT Validation

All services validate the same JWT token issued by Auth service.

**Configuration:**

```bash
CS_1=api:express-ts:8001:api
CS_2=user-service:express-ts:8002
CS_3=order-service:express-ts:8003

# All services share JWT secret
CS_1_ENV=JWT_SECRET=${HASURA_JWT_KEY}
CS_2_ENV=JWT_SECRET=${HASURA_JWT_KEY}
CS_3_ENV=JWT_SECRET=${HASURA_JWT_KEY}
```

**Middleware (Shared):**

```javascript
// src/middleware/auth.js
import jwt from 'jsonwebtoken';

export function authenticate(req, res, next) {
  const token = req.headers.authorization?.replace('Bearer ', '');

  if (!token) {
    return res.status(401).json({ error: 'No token provided' });
  }

  try {
    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    req.user = decoded;
    next();
  } catch (error) {
    res.status(401).json({ error: 'Invalid token' });
  }
}
```

**Usage:**

```javascript
// API Gateway (CS_1) - Validates and forwards
router.get('/users/:id', authenticate, async (req, res) => {
  // Forward request to user service with original token
  const response = await fetch(`http://user-service:8002/users/${req.params.id}`, {
    headers: {
      'Authorization': req.headers.authorization,
    },
  });

  const user = await response.json();
  res.json(user);
});

// User Service (CS_2) - Re-validates
router.get('/users/:id', authenticate, async (req, res) => {
  // req.user available from middleware
  const user = await db.users.findById(req.params.id);
  res.json(user);
});
```

---

### Pattern 2: API Key for Internal Services

Internal services use API keys instead of user tokens.

**Configuration:**

```bash
CS_1=api:express-ts:8001:api
CS_2=internal-service:express-ts:8002

# Generate random API key
INTERNAL_API_KEY=sk_internal_$(openssl rand -hex 32)

CS_1_ENV=INTERNAL_API_KEY=${INTERNAL_API_KEY}
CS_2_ENV=INTERNAL_API_KEY=${INTERNAL_API_KEY}
```

**Internal Service (CS_2):**

```javascript
// src/middleware/internal-auth.js
export function authenticateInternal(req, res, next) {
  const apiKey = req.headers['x-api-key'];

  if (apiKey !== process.env.INTERNAL_API_KEY) {
    return res.status(403).json({ error: 'Invalid API key' });
  }

  next();
}

// src/routes/internal.js
router.get('/internal/users', authenticateInternal, async (req, res) => {
  const users = await db.users.findAll();
  res.json(users);
});
```

**API Service (CS_1):**

```javascript
// src/services/internal.js
export async function fetchAllUsers() {
  const response = await fetch('http://internal-service:8002/internal/users', {
    headers: {
      'X-API-Key': process.env.INTERNAL_API_KEY,
    },
  });

  return response.json();
}
```

---

### Pattern 3: Service Mesh with mTLS (Advanced)

Mutual TLS for zero-trust service communication.

**Not built-in to nself, but can be added via:**
- **Istio** - Full-featured service mesh
- **Linkerd** - Lightweight service mesh
- **Consul** - Service mesh with service discovery

---

## Load Balancing

### Docker Swarm Mode (Built-in)

When using replicas, Docker automatically load balances across instances.

**Configuration:**

```bash
CS_1=api:express-ts:8001:api
CS_1_REPLICAS=3

# Docker creates 3 containers:
# - api.1
# - api.2
# - api.3

# Requests to http://api:8001 are automatically load balanced
```

**Client Service:**

```javascript
// No changes needed - Docker handles load balancing
const response = await fetch('http://api:8001/users');
// Automatically routed to one of 3 replicas
```

---

### External Load Balancer (Production)

For production, use nginx, HAProxy, or cloud load balancers.

**nginx Load Balancer (CS_N):**

```bash
CS_1=api:express-ts:8001
CS_2=api:express-ts:8002
CS_3=api:express-ts:8003
CS_4=nginx-lb:nginx:8000:api
```

**nginx Config:**

```nginx
upstream api_backend {
    server api-1:8001 weight=1;
    server api-2:8002 weight=1;
    server api-3:8003 weight=1;
}

server {
    listen 8000;

    location / {
        proxy_pass http://api_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Health Checks

### Built-in Docker Health Checks

**Configuration:**

```bash
CS_1=api:express-ts:8001
CS_1_HEALTHCHECK=/health
```

**Implementation:**

```javascript
// src/routes/health.js
import express from 'express';

const router = express.Router();

router.get('/health', async (req, res) => {
  try {
    // Check database
    await db.raw('SELECT 1');

    // Check Redis
    await redis.ping();

    res.json({
      status: 'healthy',
      timestamp: new Date(),
      services: {
        database: 'up',
        redis: 'up',
      },
    });
  } catch (error) {
    res.status(503).json({
      status: 'unhealthy',
      error: error.message,
    });
  }
});

export default router;
```

---

### Health Check Before Communication

**Check service health before calling:**

```javascript
// src/services/service-checker.js
export async function isServiceHealthy(serviceName, port) {
  try {
    const response = await fetch(`http://${serviceName}:${port}/health`, {
      timeout: 2000,
    });
    return response.ok;
  } catch (error) {
    return false;
  }
}

// src/routes/users.js
router.get('/users/:id', async (req, res) => {
  // Check if user-service is healthy
  const healthy = await isServiceHealthy('user-service', 8002);

  if (!healthy) {
    return res.status(503).json({ error: 'User service unavailable' });
  }

  const response = await fetch(`http://user-service:8002/users/${req.params.id}`);
  const user = await response.json();
  res.json(user);
});
```

---

## Circuit Breakers

Prevent cascading failures by short-circuiting calls to failing services.

### Using opossum Library

**Install:**

```bash
npm install opossum
```

**Implementation:**

```javascript
// src/services/circuit-breaker.js
import CircuitBreaker from 'opossum';

// Wrap service call in circuit breaker
function callUserService(userId) {
  return fetch(`http://user-service:8002/users/${userId}`)
    .then(res => res.json());
}

const breaker = new CircuitBreaker(callUserService, {
  timeout: 3000,           // 3 second timeout
  errorThresholdPercentage: 50,  // Open after 50% failures
  resetTimeout: 30000,     // Try again after 30 seconds
});

breaker.fallback(() => {
  return { error: 'User service unavailable', fallback: true };
});

breaker.on('open', () => {
  console.log('Circuit breaker opened - user service failing');
});

breaker.on('halfOpen', () => {
  console.log('Circuit breaker half-open - testing user service');
});

breaker.on('close', () => {
  console.log('Circuit breaker closed - user service recovered');
});

export default breaker;

// src/routes/users.js
import userServiceBreaker from '../services/circuit-breaker.js';

router.get('/users/:id', async (req, res) => {
  try {
    const user = await userServiceBreaker.fire(req.params.id);
    res.json(user);
  } catch (error) {
    res.status(503).json({ error: 'Service temporarily unavailable' });
  }
});
```

---

## Service Discovery

### Static DNS (Default)

Docker provides static DNS for all services on the same network.

```javascript
// Hardcoded service names (simple, works for most cases)
const API_URL = 'http://api:8001';
const USER_SERVICE_URL = 'http://user-service:8002';
```

---

### Dynamic Service Discovery (Advanced)

For complex deployments, use Consul or etcd.

**Using Consul:**

```bash
# Add Consul service
CS_10=consul:consul:8500:consul

CS_1=api:express-ts:8001
CS_1_ENV=CONSUL_URL=http://consul:8500
```

**Register Service:**

```javascript
// src/services/consul.js
import Consul from 'consul';

const consul = new Consul({ host: 'consul', port: 8500 });

export async function registerService(name, port) {
  await consul.agent.service.register({
    name,
    address: name,  // Docker DNS name
    port,
    check: {
      http: `http://${name}:${port}/health`,
      interval: '10s',
    },
  });

  console.log(`Service ${name} registered with Consul`);
}

export async function discoverService(name) {
  const services = await consul.health.service(name);

  if (!services.length) {
    throw new Error(`Service ${name} not found`);
  }

  const service = services[0];
  return `http://${service.Service.Address}:${service.Service.Port}`;
}

// src/index.js
import { registerService } from './services/consul.js';

registerService('api', 8001);
```

**Discover Service:**

```javascript
// src/routes/users.js
import { discoverService } from '../services/consul.js';

router.get('/users/:id', async (req, res) => {
  const userServiceUrl = await discoverService('user-service');
  const response = await fetch(`${userServiceUrl}/users/${req.params.id}`);
  const user = await response.json();
  res.json(user);
});
```

---

## Real-World Examples

### Example 1: E-Commerce Microservices

```bash
# Gateway
CS_1=gateway:express-ts:8001:api
CS_1_RATE_LIMIT=100
CS_1_REPLICAS=2

# Product Service
CS_2=product-service:express-ts:8002
CS_2_TABLE_PREFIX=product_
CS_2_REDIS_PREFIX=product:

# Cart Service
CS_3=cart-service:express-ts:8003
CS_3_TABLE_PREFIX=cart_
CS_3_REDIS_PREFIX=cart:

# Order Service
CS_4=order-service:express-ts:8004
CS_4_TABLE_PREFIX=order_
CS_4_REDIS_PREFIX=order:

# Payment Service
CS_5=payment-service:express-ts:8005
CS_5_TABLE_PREFIX=payment_
CS_5_ENV=STRIPE_API_KEY=${STRIPE_API_KEY}

# Email Worker
CS_6=email-worker:bullmq-ts:8006
CS_6_ENV=SENDGRID_API_KEY=${SENDGRID_API_KEY}

# Inventory Worker
CS_7=inventory-worker:bullmq-ts:8007
```

**Gateway Routes Requests:**

```javascript
// services/gateway/src/routes/index.js
router.use('/products', proxyTo('http://product-service:8002'));
router.use('/cart', proxyTo('http://cart-service:8003'));
router.use('/orders', proxyTo('http://order-service:8004'));

function proxyTo(target) {
  return async (req, res) => {
    const url = `${target}${req.originalUrl}`;
    const response = await fetch(url, {
      method: req.method,
      headers: req.headers,
      body: req.method !== 'GET' ? JSON.stringify(req.body) : undefined,
    });

    const data = await response.json();
    res.status(response.status).json(data);
  };
}
```

**Order Service Creates Order:**

```javascript
// services/order-service/src/routes/orders.js
import { Queue } from 'bullmq';

const emailQueue = new Queue('emails', {
  connection: { host: 'redis', port: 6379 },
});

router.post('/orders', async (req, res) => {
  const { userId, items, paymentMethod } = req.body;

  // 1. Validate cart
  const cartRes = await fetch(`http://cart-service:8003/cart/${userId}`);
  const cart = await cartRes.json();

  // 2. Process payment
  const paymentRes = await fetch('http://payment-service:8005/charge', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ amount: cart.total, paymentMethod }),
  });

  if (!paymentRes.ok) {
    return res.status(400).json({ error: 'Payment failed' });
  }

  const payment = await paymentRes.json();

  // 3. Create order
  const order = await db.orders.create({
    userId,
    items: cart.items,
    total: cart.total,
    paymentId: payment.id,
  });

  // 4. Queue email (async)
  await emailQueue.add('order-confirmation', {
    userId,
    orderId: order.id,
  });

  // 5. Queue inventory update (async)
  await inventoryQueue.add('decrease-stock', {
    items: cart.items,
  });

  res.status(201).json(order);
});
```

---

### Example 2: Real-Time Chat Application

```bash
# REST API
CS_1=api:express-ts:8001:api
CS_1_REPLICAS=2

# WebSocket Server
CS_2=websocket:socketio-ts:8002:ws
CS_2_REPLICAS=3
CS_2_REDIS_PREFIX=ws:

# Message Worker
CS_3=message-worker:bullmq-ts:8003
CS_3_ENV=WS_URL=http://websocket:8002

# Notification Worker
CS_4=notification-worker:bullmq-ts:8004
CS_4_ENV=WS_URL=http://websocket:8002
```

**API Creates Message:**

```javascript
// services/api/src/routes/messages.js
router.post('/messages', async (req, res) => {
  const { roomId, userId, text } = req.body;

  // Save to database
  const message = await db.messages.create({ roomId, userId, text });

  // Emit to WebSocket (real-time)
  await fetch('http://websocket:8002/emit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      room: `room:${roomId}`,
      event: 'new-message',
      data: message,
    }),
  });

  // Queue notification (async)
  await messageQueue.add('notify-participants', {
    roomId,
    messageId: message.id,
  });

  res.status(201).json(message);
});
```

---

### Example 3: ML Pipeline

```bash
# API
CS_1=api:fastapi:8001:api

# Data Processor
CS_2=data-processor:agent-data:8002
CS_2_MEMORY=2G

# Model Trainer
CS_3=model-trainer:fastapi:8003
CS_3_MEMORY=4G
CS_3_CPU=2.0

# Inference Service
CS_4=inference:fastapi:8004:predict
CS_4_REPLICAS=3
CS_4_MEMORY=2G
```

**API Triggers Training:**

```python
# services/api/routes/models.py
@router.post("/models/train")
async def train_model(dataset_id: str):
    # 1. Process data
    response = await http_client.post(
        "http://data-processor:8002/process",
        json={"dataset_id": dataset_id}
    )
    processed_data = response.json()

    # 2. Train model
    response = await http_client.post(
        "http://model-trainer:8003/train",
        json={
            "dataset_id": dataset_id,
            "processed_data_path": processed_data["path"]
        }
    )
    model = response.json()

    # 3. Deploy to inference service
    response = await http_client.post(
        "http://inference:8004/deploy",
        json={"model_id": model["id"]}
    )

    return {"status": "training_started", "model_id": model["id"]}
```

---

## Best Practices

### 1. Use Service Names, Not IPs

```javascript
// ✅ GOOD
const url = 'http://api:8001/users';

// ❌ BAD
const url = 'http://172.18.0.5:8001/users';
```

### 2. Implement Health Checks

```javascript
// Every service should have /health endpoint
router.get('/health', async (req, res) => {
  const healthy = await checkDependencies();
  res.status(healthy ? 200 : 503).json({ status: healthy ? 'ok' : 'error' });
});
```

### 3. Use Circuit Breakers

```javascript
// Prevent cascading failures
const breaker = new CircuitBreaker(callExternalService, {
  timeout: 3000,
  errorThresholdPercentage: 50,
});
```

### 4. Implement Retries with Backoff

```javascript
async function fetchWithRetry(url, retries = 3) {
  for (let i = 0; i < retries; i++) {
    try {
      return await fetch(url);
    } catch (error) {
      if (i === retries - 1) throw error;
      await sleep(Math.pow(2, i) * 1000);  // Exponential backoff
    }
  }
}
```

### 5. Set Timeouts

```javascript
const response = await fetch('http://service:8001/data', {
  timeout: 5000,  // 5 second timeout
});
```

### 6. Log Service Calls

```javascript
async function callService(url) {
  console.log(`Calling ${url}`);
  const start = Date.now();

  try {
    const response = await fetch(url);
    console.log(`${url} responded in ${Date.now() - start}ms`);
    return response;
  } catch (error) {
    console.error(`${url} failed after ${Date.now() - start}ms:`, error);
    throw error;
  }
}
```

### 7. Use Environment Variables

```javascript
// ✅ GOOD - Configurable
const USER_SERVICE_URL = process.env.USER_SERVICE_URL || 'http://user-service:8002';

// ❌ BAD - Hardcoded
const USER_SERVICE_URL = 'http://user-service:8002';
```

### 8. Validate Responses

```javascript
const response = await fetch('http://api:8001/users');

if (!response.ok) {
  throw new Error(`API returned ${response.status}`);
}

const data = await response.json();

if (!data || !Array.isArray(data)) {
  throw new Error('Invalid response format');
}
```

---

## Troubleshooting

### Service Not Reachable

**Problem:** `fetch ENOTFOUND service-name`

**Solutions:**

1. **Check service name:**
   ```bash
   docker ps | grep service-name
   ```

2. **Check network:**
   ```bash
   docker network inspect ${PROJECT_NAME}_network
   ```

3. **Ping from another container:**
   ```bash
   docker exec -it ${PROJECT_NAME}_api ping service-name
   ```

4. **Check service logs:**
   ```bash
   docker logs ${PROJECT_NAME}_service-name
   ```

---

### Connection Refused

**Problem:** `connect ECONNREFUSED`

**Solutions:**

1. **Check service is running:**
   ```bash
   docker ps | grep service-name
   ```

2. **Check service health:**
   ```bash
   curl http://localhost:PORT/health
   ```

3. **Check port is correct:**
   ```bash
   # Verify CS_N_PORT matches actual port
   docker inspect ${PROJECT_NAME}_service-name | grep Port
   ```

4. **Check service is listening:**
   ```bash
   docker exec -it ${PROJECT_NAME}_service-name netstat -tlnp
   ```

---

### Timeout Errors

**Problem:** Requests timing out

**Solutions:**

1. **Increase timeout:**
   ```javascript
   fetch(url, { timeout: 30000 })  // 30 seconds
   ```

2. **Check service performance:**
   ```bash
   docker stats ${PROJECT_NAME}_service-name
   ```

3. **Add resource limits:**
   ```bash
   CS_1_MEMORY=1G
   CS_1_CPU=1.0
   ```

4. **Add dependency wait:**
   ```bash
   CS_1_DEPENDS_ON=postgres,redis,minio
   ```

---

### Authentication Failures

**Problem:** Service calls return 401/403

**Solutions:**

1. **Forward auth headers:**
   ```javascript
   fetch(url, {
     headers: {
       'Authorization': req.headers.authorization,
     },
   })
   ```

2. **Use internal API key:**
   ```javascript
   fetch(url, {
     headers: {
       'X-API-Key': process.env.INTERNAL_API_KEY,
     },
   })
   ```

3. **Share JWT secret:**
   ```bash
   CS_1_ENV=JWT_SECRET=${HASURA_JWT_KEY}
   CS_2_ENV=JWT_SECRET=${HASURA_JWT_KEY}
   ```

---

## Related Documentation

- [Custom Services Environment Variables](../configuration/CUSTOM-SERVICES-ENV-VARS.md)
- [Deployment Architecture](./DEPLOYMENT-ARCHITECTURE.md)
- [Multi-App Setup](./MULTI_APP_SETUP.md)
- [Examples](./EXAMPLES.md)

---

**Last Updated:** January 30, 2026
**nself Version:** 0.4.8+
