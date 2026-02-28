# Custom Service Templates

nself includes **50 production-ready templates** across 14 languages for building custom services.

## Quick Start

```bash
# Add a custom service to your .env
echo "CS_1=api:express-js:8001" >> .env

# Build generates the service scaffolding
nself build

# Your code is in services/api/
cd services/api/
```

## How Templates Work

When you define `CS_N=name:template:port` in your `.env` file, `nself build`:

1. Copies the template from `src/templates/services/[language]/[template]/`
2. Replaces placeholders (`{{SERVICE_NAME}}`, `{{PORT}}`, etc.)
3. Removes `.template` extensions from filenames
4. Generates Dockerfile, docker-compose entry, and nginx route
5. **Preserves your edits** on subsequent builds (never overwrites existing files)

## Available Templates

### JavaScript / TypeScript (19 templates)

| Template | Type | Description |
|----------|------|-------------|
| `express-js` | HTTP API | Express.js REST API (JavaScript) |
| `express-ts` | HTTP API | Express.js REST API (TypeScript) |
| `fastify-js` | HTTP API | Fastify high-performance API (JavaScript) |
| `fastify-ts` | HTTP API | Fastify high-performance API (TypeScript) |
| `hono-js` | HTTP API | Hono ultrafast web framework (JavaScript) |
| `hono-ts` | HTTP API | Hono ultrafast web framework (TypeScript) |
| `nest-js` | HTTP API | NestJS enterprise framework (JavaScript) |
| `nest-ts` | HTTP API | NestJS enterprise framework (TypeScript) |
| `node-js` | HTTP API | Plain Node.js HTTP server (JavaScript) |
| `node-ts` | HTTP API | Plain Node.js HTTP server (TypeScript) |
| `trpc` | HTTP API | tRPC end-to-end typesafe API |
| `bun` | HTTP API | Bun runtime HTTP server |
| `deno` | HTTP API | Deno runtime HTTP server |
| `socketio-js` | WebSocket | Socket.IO real-time server (JavaScript) |
| `socketio-ts` | WebSocket | Socket.IO real-time server (TypeScript) |
| `bullmq-js` | Worker | BullMQ job queue worker (JavaScript) |
| `bullmq-ts` | Worker | BullMQ job queue worker (TypeScript) |
| `temporal-js` | Worker | Temporal workflow worker (JavaScript) |
| `temporal-ts` | Worker | Temporal workflow worker (TypeScript) |

### Python (10 templates)

| Template | Type | Description |
|----------|------|-------------|
| `fastapi` | HTTP API | FastAPI async web framework |
| `flask` | HTTP API | Flask lightweight web framework |
| `django-rest` | HTTP API | Django REST Framework |
| `celery` | Worker | Celery distributed task queue |
| `ray` | ML | Ray distributed computing framework |
| `agent-llm` | ML Agent | LLM-powered AI agent |
| `agent-analytics` | ML Agent | Analytics processing agent |
| `agent-timeseries` | ML Agent | Time-series analysis agent |
| `agent-training` | ML Agent | Model training agent |
| `agent-vision` | ML Agent | Computer vision agent |

### Go (4 templates)

| Template | Type | Description |
|----------|------|-------------|
| `gin` | HTTP API | Gin web framework |
| `echo` | HTTP API | Echo web framework |
| `fiber` | HTTP API | Fiber Express-inspired framework |
| `grpc` | gRPC | gRPC service with protobuf |

### Ruby (2 templates)

| Template | Type | Description |
|----------|------|-------------|
| `rails` | HTTP API | Ruby on Rails API mode |
| `sinatra` | HTTP API | Sinatra lightweight framework |

### Java (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `spring-boot` | HTTP API | Spring Boot REST API |

### C# (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `aspnet` | HTTP API | ASP.NET Core Web API |

### Rust (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `actix-web` | HTTP API | Actix-web high-performance framework |

### PHP (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `laravel` | HTTP API | Laravel PHP framework |

### Kotlin (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `ktor` | HTTP API | Ktor async Kotlin framework |

### Swift (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `vapor` | HTTP API | Vapor server-side Swift framework |

### Elixir (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `phoenix` | HTTP API | Phoenix web framework |

### Lua (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `lapis` | HTTP API | Lapis web framework (OpenResty) |

### C++ (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `oatpp` | HTTP API | Oat++ high-performance C++ framework |

### Zig (1 template)

| Template | Type | Description |
|----------|------|-------------|
| `zap` | HTTP API | Zap blazingly fast Zig web framework |

### Special Purpose (3 templates)

| Template | Type | Description |
|----------|------|-------------|
| `oauth-handlers` | Auth | OAuth provider handler service (Google, GitHub, Slack, Microsoft) |
| `websocket-server` | WebSocket | Standalone WebSocket server |
| `custom` | Custom | Bare-bones Docker service (bring your own code) |

## Template Types

### HTTP API Templates
Standard REST/GraphQL APIs that expose an HTTP port. Includes health check endpoint, Docker multi-stage build, and nginx route generation.

### Worker Templates
Background job processors (BullMQ, Celery, Temporal). No public HTTP port by default. Connects to Redis or other message brokers.

### WebSocket Templates
Real-time communication servers. Generates nginx WebSocket upgrade configuration.

### gRPC Templates
Protocol buffer-based RPC services. Generates appropriate nginx gRPC proxy configuration.

### ML Agent Templates
Python-based machine learning services. Includes MLflow integration hooks and GPU support in Dockerfile.

## Configuration Examples

### Single API Service
```bash
CS_1=api:express-js:8001
```

### Full-Stack Setup
```bash
CS_1=api:express-ts:8001
CS_2=worker:bullmq-ts:8002
CS_3=ml:fastapi:8003
CS_4=realtime:socketio-ts:8004
```

### Multi-Language Architecture
```bash
CS_1=gateway:gin:8001
CS_2=users:spring-boot:8002
CS_3=analytics:fastapi:8003
CS_4=notifications:express-ts:8004
```

### ML Pipeline
```bash
CS_1=inference:fastapi:8001
CS_2=training:agent-training:8002
CS_3=analytics:agent-analytics:8003
CS_4=vision:agent-vision:8004
```

## Generated File Structure

Each template generates:

```
services/[name]/
├── Dockerfile              # Multi-stage Docker build
├── [config file]           # package.json, go.mod, requirements.txt, etc.
├── [entry point]           # index.js, main.py, main.go, etc.
├── .dockerignore           # Docker build optimization
└── [framework files]       # tsconfig.json, CMakeLists.txt, etc.
```

## Placeholder Reference

Templates use these placeholders, replaced during `nself build`:

| Placeholder | Replaced With | Example |
|-------------|---------------|---------|
| `{{SERVICE_NAME}}` | Service name from CS_N | `api` |
| `{{PORT}}` | Port from CS_N | `8001` |
| `{{PROJECT_NAME}}` | From .env PROJECT_NAME | `myapp` |

## Working With Templates

### First Build
```bash
# Templates are copied and placeholders replaced
nself build
```

### Editing Your Service
```bash
# Edit the generated code freely
cd services/api/
vim index.js
```

### Subsequent Builds
```bash
# Your edits are preserved - nself never overwrites existing service files
nself build
```

### Restarting After Changes
```bash
# Restart just your service
nself restart api

# Follow logs
nself logs api -f
```

### Starting Fresh
```bash
# Delete the service directory to regenerate from template
rm -rf services/api/
nself build
```

## See Also

- [Custom Services Configuration](../configuration/CUSTOM-SERVICES.md)
- [Docker Compose Generation](../architecture/DOCKER-COMPOSE.md)
- [Nginx Routing](../architecture/NGINX.md)
