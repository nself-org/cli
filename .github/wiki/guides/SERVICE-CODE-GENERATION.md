# Service Code Generation Guide

> **New in nself v0.8+**: Automatically generate production-ready service code from 40+ templates

## Overview

nself includes a powerful service scaffolding system that generates complete, production-ready microservices from templates. No more writing boilerplate—get started coding your business logic immediately.

## Quick Start

### 1. List Available Templates

```bash
nself service list-templates
```

This shows all 40+ available templates organized by category:

- **Web Frameworks**: Express, Fastify, Hono, Gin, Fiber, Rails, etc.
- **Full-Stack Frameworks**: NestJS, Django REST, FastAPI, Spring Boot
- **Real-time & Messaging**: Socket.IO, Temporal
- **Background Jobs**: BullMQ, Celery, Ray
- **AI & ML Agents**: LLM, Vision, Analytics, Training, Time Series
- **And more...**

### 2. Scaffold a Service

```bash
nself service scaffold realtime --template socketio-ts --port 3101
```

This generates:

```
services/realtime/
├── package.json          # Dependencies configured
├── tsconfig.json         # TypeScript config
├── Dockerfile            # Production-ready container
├── README.md             # Complete documentation
└── src/
    └── server.ts         # Working Socket.IO server with:
                          # - Type-safe events
                          # - Redis adapter ready
                          # - Room/namespace support
                          # - Health checks
                          # - Graceful shutdown
```

### 3. Add to Environment

```bash
# Add to .env
CS_2=realtime:socketio-ts:3101:ws
CS_2_REDIS_PREFIX=ws:
CS_2_REPLICAS=2
```

### 4. Build and Start

```bash
nself build && nself start
```

Your service is now running at `https://ws.yourdomain.com` (or `ws.localhost` in dev).

## Commands

### `nself service scaffold`

Generate service code from a template.

**Syntax:**
```bash
nself service scaffold <name> --template <template> [options]
```

**Options:**
- `--template, -t` - Template to use (required)
- `--port, -p` - Port number (default: 3000)
- `--output, -o` - Output directory (default: services)

**Examples:**
```bash
# Socket.IO real-time service
nself service scaffold realtime --template socketio-ts --port 3101

# FastAPI REST API
nself service scaffold api --template fastapi --port 8000

# BullMQ background worker
nself service scaffold worker --template bullmq-ts --port 3102

# NestJS full-stack API
nself service scaffold backend --template nest-ts --port 4000

# Go Fiber high-performance API
nself service scaffold fiber-api --template fiber --port 8080
```

### `nself service list-templates`

List all available templates.

**Options:**
- `--language, -l` - Filter by language (js, ts, python, go, ruby, rust, etc.)
- `--category, -c` - Filter by category

**Examples:**
```bash
# Show all templates
nself service list-templates

# TypeScript templates only
nself service list-templates --language typescript

# Real-time templates only
nself service list-templates --category "Real-time & Messaging"
```

### `nself service template-info`

Show detailed information about a template.

**Syntax:**
```bash
nself service template-info <template>
```

**Example:**
```bash
nself service template-info socketio-ts
```

Shows:
- Full description
- Language and category
- Key features
- Dependencies
- Usage examples

### `nself service wizard`

Interactive service creation wizard.

**Syntax:**
```bash
nself service wizard
```

Guides you through:
1. Service name
2. Language selection
3. Template selection
4. Port configuration
5. Confirmation and generation

## Template Categories

### Web Frameworks

Fast, lightweight HTTP frameworks for building APIs.

**JavaScript/TypeScript:**
- `express-js`, `express-ts` - Minimalist, flexible
- `fastify-js`, `fastify-ts` - High performance, schema validation
- `hono-js`, `hono-ts` - Ultra-fast, edge-ready
- `node-js`, `node-ts` - Bare Node.js HTTP server

**Python:**
- `flask` - Lightweight, flexible
- `fastapi` - Modern, async, auto-docs

**Go:**
- `gin` - Fast, minimalist
- `fiber` - Express-inspired
- `echo` - High performance

**Other:**
- `rails`, `sinatra` (Ruby)
- `actix-web` (Rust)
- `laravel` (PHP)
- `vapor` (Swift)
- `oatpp` (C++)

### Full-Stack Frameworks

Enterprise-grade frameworks with batteries included.

- `nest-js`, `nest-ts` - TypeScript-first, dependency injection
- `django-rest` - Python ORM, admin panel, full auth
- `spring-boot` - Java enterprise applications
- `phoenix` - Elixir real-time apps with LiveView
- `aspnet` - C# cross-platform framework

### Real-time & Messaging

WebSocket and real-time communication services.

- `socketio-js`, `socketio-ts` - **Bidirectional events, rooms, Redis adapter**
- `temporal-js`, `temporal-ts` - Durable workflow orchestration

### Background Jobs & Workers

Asynchronous task processing and job queues.

- `bullmq-js`, `bullmq-ts` - **Redis-backed job queues with TypeScript**
- `celery` - Python distributed task queue
- `ray` - Python distributed computing

### AI & ML Agents

Specialized templates for AI/ML workloads.

- `agent-llm` - LLM provider integration, streaming, RAG
- `agent-vision` - Computer vision, image processing, OCR
- `agent-analytics` - Data analysis, visualization, reporting
- `agent-training` - Model training, hyperparameter tuning
- `agent-timeseries` - Forecasting, anomaly detection

### API Frameworks

Type-safe and high-performance API development.

- `trpc` - End-to-end type safety for TypeScript
- `grpc` - Protocol Buffers, high-performance RPC

### Runtime Servers

Modern JavaScript/TypeScript runtimes.

- `bun` - Fast all-in-one runtime
- `deno` - Secure TypeScript runtime

## Complete Examples

### Example 1: Real-time Chat Backend

**Goal**: Socket.IO service with Redis adapter for horizontal scaling.

```bash
# 1. Generate service
nself service scaffold realtime --template socketio-ts --port 3101

# 2. Review generated code
cd services/realtime
cat src/server.ts  # Type-safe events, rooms, broadcasting

# 3. Add Redis adapter (optional, for multiple instances)
npm install @socket.io/redis-adapter ioredis

# 4. Configure in .env
cat >> .env << 'EOF'
REDIS_ENABLED=true
CS_2=realtime:socketio-ts:3101:ws
CS_2_REDIS_PREFIX=ws:
CS_2_REPLICAS=2
CS_2_MEMORY=512M
EOF

# 5. Build and start
cd ../..
nself build && nself start

# 6. Test
curl http://localhost:3101/health
# {"status":"healthy","service":"realtime","connections":0}
```

**Access:**
- Development: `ws://localhost:3101`
- Production: `wss://ws.yourdomain.com`

### Example 2: FastAPI REST API

**Goal**: Python API with auto-generated docs and Pydantic validation.

```bash
# 1. Generate service
nself service scaffold api --template fastapi --port 8000

# 2. Customize code
cd services/api
# Edit main.py to add your endpoints

# 3. Configure
cat >> ../../.env << 'EOF'
CS_3=api:fastapi:8000
EOF

# 4. Build and start
cd ../..
nself build && nself start

# 5. Access auto-generated docs
open http://localhost:8000/docs  # Swagger UI
open http://localhost:8000/redoc # ReDoc
```

### Example 3: Background Job Worker

**Goal**: BullMQ worker for processing async tasks.

```bash
# 1. Generate worker
nself service scaffold worker --template bullmq-ts --port 3102

# 2. Define job processors
cd services/worker/src
# Edit worker.ts to add job handlers

# 3. Configure
cat >> ../../../.env << 'EOF'
REDIS_ENABLED=true
CS_4=worker:bullmq-ts:3102
CS_4_CONCURRENCY=10
EOF

# 4. Build and start
cd ../../..
nself build && nself start

# 5. Enqueue jobs from other services
# Use BullMQ client to add jobs to queues
```

### Example 4: Multi-Service Architecture

**Complete stack**: Web API + Real-time + Worker + AI Agent

```bash
# Generate all services
nself service scaffold api --template fastapi --port 8000
nself service scaffold realtime --template socketio-ts --port 3101
nself service scaffold worker --template bullmq-ts --port 3102
nself service scaffold ai --template agent-llm --port 8001

# Configure in .env
cat >> .env << 'EOF'
# Enable required services
REDIS_ENABLED=true
MINIO_ENABLED=true
MEILISEARCH_ENABLED=true

# Custom services
CS_1=api:fastapi:8000
CS_2=realtime:socketio-ts:3101:ws
CS_2_REDIS_PREFIX=ws:
CS_2_REPLICAS=2
CS_3=worker:bullmq-ts:3102
CS_4=ai:agent-llm:8001
CS_4_MEMORY=2G
EOF

# Build and start entire stack
nself build && nself start

# Access points:
# - API: http://api.localhost
# - WebSocket: ws://ws.localhost
# - AI: http://ai.localhost
# - Hasura: http://api.localhost (GraphQL)
```

## Template Features

### What's Included in Every Template

All templates include:

✅ **Production-Ready Dockerfile**
- Multi-stage builds
- Security best practices
- Non-root user
- Health checks
- Proper signal handling

✅ **Development Tooling**
- Hot reload / watch mode
- TypeScript (for TS templates)
- Linting configuration
- Debug support

✅ **Health Checks**
- `/health` endpoint
- Ready for k8s/Docker health checks

✅ **Environment Configuration**
- `.env` variable support
- Sensible defaults
- nself integration variables

✅ **Documentation**
- Comprehensive README
- Usage examples
- API documentation
- Deployment guide

### Socket.IO Template (`socketio-ts`)

**Special Features:**

```typescript
// Type-safe events
interface ServerToClientEvents {
  welcome: (data: WelcomeData) => void;
  message: (data: MessageData) => void;
}

interface ClientToServerEvents {
  message: (text: string) => void;
  join_room: (room: string) => void;
}

// Redis adapter ready
import { createAdapter } from '@socket.io/redis-adapter';
io.adapter(createAdapter(pubClient, subClient));
```

**Includes:**
- Room/namespace management
- Broadcasting
- Authentication hooks
- Rate limiting ready
- Presence tracking examples

### FastAPI Template (`fastapi`)

**Special Features:**

```python
# Auto-generated OpenAPI docs
# Pydantic validation
# Async/await support

@app.post("/items/")
async def create_item(item: Item):
    # Validated automatically
    return {"item": item}
```

**Includes:**
- Swagger UI at `/docs`
- ReDoc at `/redoc`
- CORS configuration
- Database integration examples
- JWT auth ready

### BullMQ Template (`bullmq-ts`)

**Special Features:**

```typescript
// Type-safe job processing
interface EmailJob {
  to: string;
  subject: string;
  body: string;
}

worker.process('email', async (job: Job<EmailJob>) => {
  const { to, subject, body } = job.data;
  await sendEmail(to, subject, body);
});
```

**Includes:**
- Job retries
- Scheduling
- Rate limiting
- Priority queues
- Job events

### NestJS Template (`nest-ts`)

**Special Features:**

```typescript
// Dependency injection
@Injectable()
export class UsersService {
  constructor(
    @InjectRepository(User)
    private usersRepository: Repository<User>,
  ) {}
}

// Decorators
@Controller('users')
export class UsersController {
  @Get()
  findAll(): Promise<User[]> {
    return this.usersService.findAll();
  }
}
```

**Includes:**
- Modular architecture
- Guards and interceptors
- Pipes for validation
- Database integration
- Swagger integration

## Best Practices

### 1. Service Organization

```
services/
├── api/           # Main REST API (FastAPI/NestJS)
├── realtime/      # Socket.IO for real-time
├── worker/        # Background jobs (BullMQ)
├── ai/            # AI/ML services
└── grpc/          # gRPC microservices
```

### 2. Port Convention

- `3000-3099`: Frontend applications
- `3100-3199`: Real-time services (WebSocket)
- `4000-4999`: Backend APIs
- `8000-8999`: Python services
- `9000-9999`: Go services

### 3. Naming Convention

Use descriptive names that indicate purpose:
- `api` - Main REST API
- `realtime` - WebSocket/Socket.IO
- `worker` - Background job processor
- `ml-training` - ML model training
- `analytics` - Analytics processing

### 4. Environment Configuration

```bash
# In .env
CS_1=api:fastapi:8000
CS_1_MEMORY=1G
CS_1_REPLICAS=2

CS_2=realtime:socketio-ts:3101:ws
CS_2_REDIS_PREFIX=ws:
CS_2_REPLICAS=3

CS_3=worker:bullmq-ts:3102
CS_3_CONCURRENCY=10
```

### 5. Development Workflow

```bash
# 1. Scaffold service
nself service scaffold myservice --template <template>

# 2. Develop locally
cd services/myservice
npm run dev  # or python main.py, go run main.go

# 3. Test in isolation
curl http://localhost:PORT/health

# 4. Add to nself stack
# Edit .env to add CS_N=myservice:template:port

# 5. Build and deploy
nself build && nself start
```

## Customization

### Modifying Templates

After scaffolding, the generated code is yours to customize:

```bash
nself service scaffold api --template fastapi --port 8000
cd services/api

# Customize freely:
# - Add new endpoints
# - Change dependencies
# - Modify Dockerfile
# - Add middleware
# - Configure logging
```

Files are never overwritten by `nself build` - your changes persist.

### Adding nself Integration

Services auto-include environment variables for nself services:

```typescript
// TypeScript example
const hasuraEndpoint = process.env.HASURA_GRAPHQL_ENDPOINT;
const hasuraSecret = process.env.HASURA_GRAPHQL_ADMIN_SECRET;
const redisUrl = process.env.REDIS_URL;
const minioEndpoint = process.env.MINIO_ENDPOINT;

// Use in your service
import fetch from 'node-fetch';

async function queryHasura(query: string) {
  const response = await fetch(hasuraEndpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-hasura-admin-secret': hasuraSecret
    },
    body: JSON.stringify({ query })
  });
  return response.json();
}
```

```python
# Python example
import os
import httpx

HASURA_ENDPOINT = os.getenv('HASURA_GRAPHQL_ENDPOINT')
HASURA_SECRET = os.getenv('HASURA_GRAPHQL_ADMIN_SECRET')

async def query_hasura(query: str):
    async with httpx.AsyncClient() as client:
        response = await client.post(
            HASURA_ENDPOINT,
            json={'query': query},
            headers={'x-hasura-admin-secret': HASURA_SECRET}
        )
        return response.json()
```

## Troubleshooting

### Template Not Found

```bash
$ nself service scaffold test --template invalid
Error: Template not found: invalid
Run 'nself service list-templates' to see available templates
```

**Solution**: Use `nself service list-templates` to find the correct template name.

### Service Directory Exists

```bash
$ nself service scaffold api --template fastapi
Error: Service directory already exists: services/api
Use a different name or remove the existing directory
```

**Solution**: Choose a different name or remove the existing directory.

### Missing Dependencies

After scaffolding, install dependencies:

```bash
# Node.js/TypeScript
cd services/myservice
npm install

# Python
cd services/myservice
pip install -r requirements.txt

# Go
cd services/myservice
go mod download
```

## Advanced Usage

### Custom Output Directory

```bash
nself service scaffold api --template fastapi --output backend
# Generates: backend/api/ instead of services/api/
```

### Scripted Service Generation

```bash
#!/bin/bash
# generate-microservices.sh

services=(
  "api:fastapi:8000"
  "realtime:socketio-ts:3101"
  "worker:bullmq-ts:3102"
  "analytics:agent-analytics:8001"
)

for service in "${services[@]}"; do
  IFS=':' read -r name template port <<< "$service"
  nself service scaffold "$name" --template "$template" --port "$port"
done
```

### Multi-Language Microservices

```bash
# Generate polyglot architecture
nself service scaffold api-gateway --template nest-ts --port 4000
nself service scaffold ml-service --template fastapi --port 8000
nself service scaffold cache-service --template go-fiber --port 9000
nself service scaffold worker --template bullmq-ts --port 3102
```

## See Also

- [Custom Services Guide](../services/SERVICES_CUSTOM.md)
- [Template Reference](../reference/SERVICE-SCAFFOLDING-CHEATSHEET.md)
- [Service Configuration](../configuration/README.md)
- [Architecture Patterns](../architecture/ARCHITECTURE.md)
