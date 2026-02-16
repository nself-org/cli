# Custom Services

Custom services are your own backend microservices that integrate seamlessly with the nself infrastructure. You can create up to 10 custom services using templates or your own code.

## Quick Start

Define custom services in your `.env` file:

```bash
# Format: CS_N=service_name:template_type:port
CS_1=api:express-js:8001
CS_2=worker:bullmq-js:8002
CS_3=grpc:grpc:50051
CS_4=ml_api:fastapi:8004
```

Run `nself build` to generate services from templates, then customize the code.

## Available Templates

### JavaScript/TypeScript
- **express-js** - Express.js REST API
- **express-ts** - Express with TypeScript
- **fastify-js** - High-performance REST API
- **fastify-ts** - Fastify with TypeScript
- **nest-js** - NestJS framework
- **nest-ts** - NestJS with TypeScript
- **bullmq-js** - BullMQ job worker
- **bullmq-ts** - BullMQ with TypeScript
- **socketio-js** - WebSocket server
- **socketio-ts** - Socket.IO with TypeScript
- **temporal-js** - Temporal workflow
- **temporal-ts** - Temporal with TypeScript
- **trpc** - tRPC API server
- **hono-js** - Ultrafast web framework
- **hono-ts** - Hono with TypeScript
- **bun** - Bun runtime service
- **deno** - Deno runtime service

### Python
- **fastapi** - FastAPI REST service
- **flask** - Flask web framework
- **django-rest** - Django REST Framework
- **celery** - Celery worker
- **ray** - Ray distributed computing
- **agent-analytics** - Analytics agent
- **agent-llm** - LLM agent service
- **agent-vision** - Computer vision agent
- **agent-timeseries** - Time series analysis
- **agent-training** - ML training agent

### Go
- **gin** - Gin web framework
- **fiber** - Fiber web framework
- **echo** - Echo web framework
- **grpc** - gRPC service

### Other Languages
- **rust/actix-web** - Rust Actix framework
- **ruby/rails** - Ruby on Rails
- **ruby/sinatra** - Sinatra framework
- **php/laravel** - Laravel framework
- **java/spring-boot** - Spring Boot
- **kotlin/ktor** - Ktor framework
- **csharp/aspnet** - ASP.NET Core
- **elixir/phoenix** - Phoenix framework
- **swift/vapor** - Vapor framework
- **zig/zap** - Zap framework
- **lua/lapis** - Lapis framework
- **cpp/oatpp** - Oat++ framework

## Template System

### How It Works

1. **Define Service** in `.env`:
   ```bash
   CS_1=my_api:express-js:8001
   ```

2. **Run Build** to generate from template:
   ```bash
   nself build
   ```

3. **Service Created** at `services/my_api/`:
   ```
   services/my_api/
   ├── Dockerfile
   ├── index.js
   └── package.json
   ```

4. **Customize** your code - files won't be overwritten on rebuild

### Template Placeholders

Templates use placeholders that are automatically replaced:

- `{{SERVICE_NAME}}` - Your service name
- `{{SERVICE_PORT}}` - Service port number
- `{{PROJECT_NAME}}` - Project name
- `{{BASE_DOMAIN}}` - Base domain
- `{{POSTGRES_HOST}}` - Database host
- `{{REDIS_HOST}}` - Redis host

### File Structure

Each template provides:
- **Dockerfile** - Container configuration
- **Source files** - Language-specific code
- **Dependencies** - package.json, requirements.txt, go.mod, etc.
- **Health check** - `/health` endpoint
- **Basic routes** - Example endpoints

## Custom Service Examples

### REST API Service
```bash
CS_1=api:express-js:8001
```
Creates an Express.js API with:
- RESTful endpoints
- PostgreSQL connection
- JWT authentication ready
- Health checks

### Background Worker
```bash
CS_2=worker:bullmq-js:8002
```
Creates a BullMQ worker with:
- Job queue processing
- Redis connection
- Retry logic
- Dead letter queue

### gRPC Service
```bash
CS_3=grpc:grpc:50051
```
Creates a Go gRPC service with:
- Protocol buffer definitions
- Service implementation
- Health checking
- Reflection enabled

### ML API
```bash
CS_4=ml_api:fastapi:8004
```
Creates a FastAPI service with:
- Async endpoints
- Pydantic models
- OpenAPI documentation
- ML model serving ready

## Service Configuration

### Environment Variables

All custom services receive these environment variables:

```bash
# Database
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_DB=${POSTGRES_DB}
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

# Redis
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=${REDIS_PASSWORD}

# Hasura
HASURA_GRAPHQL_ENDPOINT=http://hasura:8080/v1/graphql
HASURA_ADMIN_SECRET=${HASURA_GRAPHQL_ADMIN_SECRET}

# Service Info
SERVICE_NAME=${service_name}
SERVICE_PORT=${port}
PROJECT_NAME=${PROJECT_NAME}
BASE_DOMAIN=${BASE_DOMAIN}
NODE_ENV=${ENV}
```

### Networking

Custom services are automatically:
- Added to Docker network
- Configured in nginx routing
- Given health check endpoints
- Exposed on specified ports

### Volumes

Default volumes for development:
```yaml
volumes:
  - ./services/${service_name}:/app
  - /app/node_modules      # Node.js
  - /app/.venv            # Python
  - /app/vendor           # PHP/Ruby
```

## Building Custom Services

### From Scratch

If you don't use a template, create your service structure:

```
services/my_service/
├── Dockerfile
├── src/
│   └── main.js
├── package.json
└── .env
```

**Dockerfile example:**
```dockerfile
FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
EXPOSE 8080
CMD ["node", "src/main.js"]
```

### Using Docker Compose

Custom services are added to docker-compose.yml:

```yaml
my_service:
  build:
    context: ./services/my_service
    dockerfile: Dockerfile
  container_name: ${PROJECT_NAME}_my_service
  ports:
    - "8080:8080"
  environment:
    - NODE_ENV=${ENV}
    - DATABASE_URL=${DATABASE_URL}
  depends_on:
    - postgres
    - redis
```

## Service Communication

### Internal Communication

Services can communicate internally using service names:

```javascript
// From one service to another
const response = await fetch('http://other_service:8002/api/data');

// To Hasura
const graphql = await fetch('http://hasura:8080/v1/graphql', {
  headers: {
    'x-hasura-admin-secret': process.env.HASURA_ADMIN_SECRET
  }
});

// To PostgreSQL
const pg = new Pool({
  host: 'postgres',
  port: 5432,
  database: process.env.POSTGRES_DB
});
```

### External Access

Services are exposed via nginx:
- `https://api.<domain>` → Your API service
- `https://worker.<domain>` → Worker dashboard
- `https://grpc.<domain>` → gRPC service

## Best Practices

### 1. Use Templates
Start with templates - they include best practices and proper configuration.

### 2. Health Checks
Always implement `/health` endpoint:
```javascript
app.get('/health', (req, res) => {
  res.json({ status: 'healthy', service: 'my_api' });
});
```

### 3. Environment Variables
Never hardcode configuration:
```javascript
const port = process.env.SERVICE_PORT || 8080;
const dbUrl = process.env.DATABASE_URL;
```

### 4. Logging
Use structured logging:
```javascript
console.log(JSON.stringify({
  level: 'info',
  service: process.env.SERVICE_NAME,
  message: 'Request processed',
  requestId: req.id
}));
```

### 5. Error Handling
Implement proper error handling:
```javascript
app.use((err, req, res, next) => {
  console.error(err);
  res.status(500).json({
    error: 'Internal Server Error',
    requestId: req.id
  });
});
```

## Monitoring Custom Services

### Metrics
Expose Prometheus metrics at `/metrics`:
```javascript
const prometheus = require('prom-client');
const register = prometheus.register;

app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});
```

### Logs
Logs are automatically collected by Promtail and sent to Loki.

### Traces
Use OpenTelemetry for distributed tracing:
```javascript
const { trace } = require('@opentelemetry/api');
const tracer = trace.getTracer('my-service');
```

## Scaling Custom Services

### Horizontal Scaling
```bash
# In docker-compose.yml
deploy:
  replicas: 3
```

### Resource Limits
```yaml
deploy:
  resources:
    limits:
      cpus: '0.5'
      memory: 512M
    reservations:
      cpus: '0.25'
      memory: 256M
```

## Troubleshooting

### Service Won't Start
- Check logs: `nself logs my_service`
- Verify port isn't in use
- Check Dockerfile syntax
- Ensure dependencies installed

### Can't Connect to Database
- Verify PostgreSQL is running
- Check connection string
- Ensure network connectivity
- Verify credentials

### Template Not Found
- Check template name spelling
- Verify template exists
- Use `nself templates list`

## Related Documentation

- [Services Overview](SERVICES.md)
- [Service Templates](SERVICE-TEMPLATES.md)
- [Docker Compose Structure](../architecture/ARCHITECTURE.md)
- [Monitoring Bundle](MONITORING-BUNDLE.md)