# Service Scaffolding Quick Reference

## Commands

```bash
# List all templates
nself service list-templates

# Filter by language
nself service list-templates --language typescript

# Get template details
nself service template-info socketio-ts

# Generate service
nself service scaffold <name> --template <template> --port <port>

# Interactive wizard
nself service wizard
```

## Popular Templates

### Real-time & WebSocket

```bash
# Socket.IO with TypeScript
nself service scaffold realtime --template socketio-ts --port 3101

# Socket.IO with JavaScript
nself service scaffold realtime --template socketio-js --port 3101
```

### REST APIs

```bash
# FastAPI (Python) - Auto docs
nself service scaffold api --template fastapi --port 8000

# Express (TypeScript) - Minimal
nself service scaffold api --template express-ts --port 4000

# Fastify (TypeScript) - High performance
nself service scaffold api --template fastify-ts --port 4000

# NestJS (TypeScript) - Enterprise
nself service scaffold backend --template nest-ts --port 4000

# Go Fiber - Fast
nself service scaffold api --template fiber --port 8080
```

### Background Workers

```bash
# BullMQ (TypeScript) - Redis queue
nself service scaffold worker --template bullmq-ts --port 3102

# Celery (Python) - Distributed tasks
nself service scaffold worker --template celery --port 5555
```

### AI/ML Services

```bash
# LLM Agent - OpenAI and others
nself service scaffold ai --template agent-llm --port 8001

# Vision Agent - Computer vision
nself service scaffold vision --template agent-vision --port 8002

# Analytics Agent - Data processing
nself service scaffold analytics --template agent-analytics --port 8003
```

## Quick Workflow

### 1. Generate Service

```bash
nself service scaffold myservice --template socketio-ts --port 3101
```

### 2. Customize Code

```bash
cd services/myservice
# Edit src/server.ts
npm install  # or pip install -r requirements.txt
```

### 3. Add to Environment

```bash
# Edit .env
echo "CS_2=myservice:socketio-ts:3101:ws" >> .env
```

### 4. Build and Start

```bash
nself build && nself start
```

### 5. Test

```bash
curl http://localhost:3101/health
```

## Common Patterns

### Multi-Service Architecture

```bash
# Generate all services
nself service scaffold api --template fastapi --port 8000
nself service scaffold realtime --template socketio-ts --port 3101
nself service scaffold worker --template bullmq-ts --port 3102

# Configure in .env
cat >> .env << 'EOF'
CS_1=api:fastapi:8000
CS_2=realtime:socketio-ts:3101:ws
CS_3=worker:bullmq-ts:3102
EOF

# Build and start
nself build && nself start
```

### With Redis (for scaling)

```bash
# Enable Redis
echo "REDIS_ENABLED=true" >> .env

# Generate Socket.IO with Redis adapter
nself service scaffold realtime --template socketio-ts --port 3101
cd services/realtime
npm install @socket.io/redis-adapter ioredis

# Configure multiple instances
echo "CS_2=realtime:socketio-ts:3101:ws" >> ../../.env
echo "CS_2_REDIS_PREFIX=ws:" >> ../../.env
echo "CS_2_REPLICAS=3" >> ../../.env
```

### With Hasura Integration

```typescript
// In your service
const HASURA_ENDPOINT = process.env.HASURA_GRAPHQL_ENDPOINT;
const HASURA_SECRET = process.env.HASURA_GRAPHQL_ADMIN_SECRET;

async function queryHasura(query: string) {
  const response = await fetch(HASURA_ENDPOINT, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-hasura-admin-secret': HASURA_SECRET
    },
    body: JSON.stringify({ query })
  });
  return response.json();
}
```

## Template Categories

| Category | Templates |
|----------|-----------|
| **Real-time** | socketio-js, socketio-ts, temporal-js, temporal-ts |
| **Web Frameworks** | express, fastify, hono (js/ts), gin, fiber, echo |
| **Full-Stack** | nest (js/ts), fastapi, django-rest, flask |
| **Workers** | bullmq (js/ts), celery, ray |
| **AI/ML** | agent-llm, agent-vision, agent-analytics, agent-training, agent-timeseries |
| **API** | trpc, grpc |
| **Runtime** | bun, deno, node (js/ts) |

## Language Filters

```bash
# JavaScript templates
nself service list-templates --language javascript

# TypeScript templates
nself service list-templates --language typescript

# Python templates
nself service list-templates --language python

# Go templates
nself service list-templates --language go
```

## Tips

### Port Conventions

- `3000-3099`: Frontend apps
- `3100-3199`: Real-time services (WebSocket)
- `4000-4999`: Backend APIs
- `8000-8999`: Python services
- `9000-9999`: Go services

### Naming Conventions

- `api` - Main REST API
- `realtime` - WebSocket/Socket.IO
- `worker` - Background jobs
- `ml-*` - ML services
- `*-api` - Language-specific APIs

### Development

```bash
# Develop locally first
cd services/myservice
npm run dev  # or python main.py

# Then integrate with nself
cd ../..
nself build && nself start
```

### Production

```bash
# Set environment
ENV=production

# Enable SSL
SSL_ENABLED=true

# Configure replicas
CS_N_REPLICAS=3

# Set memory limits
CS_N_MEMORY=1G
```

## Troubleshooting

### Service not generating

```bash
# Check template name
nself service list-templates

# Get correct template name
nself service template-info <template>
```

### Directory exists

```bash
# Remove existing directory
rm -rf services/myservice

# Or use different name
nself service scaffold myservice2 --template socketio-ts
```

### Dependencies

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

## See Also

- [Complete Guide](../guides/SERVICE-CODE-GENERATION.md)
- [Template Reference](SERVICE_TEMPLATES.md)
- [Examples](../examples/REALTIME-CHAT-SERVICE.md)
