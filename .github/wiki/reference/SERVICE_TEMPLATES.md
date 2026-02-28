# Service Templates - Complete Reference

> **Comprehensive guide to all 40 built-in service templates for microservices and custom backends**

nself provides 40+ production-ready service templates spanning 10 programming languages. Each template includes Docker configuration, health checks, CORS support, graceful shutdown, and follows language-specific best practices.

## Quick Start

```bash
# Enable custom services
SERVICES_ENABLED=true

# Add a service using a template
CS_1=api:fastapi:3001:/api

# Build and start
nself build && nself start
```

## Template Categories

- **[JavaScript/TypeScript](#javascripttypescript-19-templates)**: 19 templates (Node, Express, Fastify, NestJS, Hono, Socket.IO, BullMQ, Temporal, Bun, Deno, tRPC)
- **[Python](#python-7-templates)**: 7 templates (Flask, FastAPI, Django REST, Celery, Ray, AI Agents)
- **[Go](#go-4-templates)**: 4 templates (Gin, Echo, Fiber, gRPC)
- **[Other Languages](#other-languages-10-templates)**: 10 templates (Rust, Java, C#, C++, Ruby, Elixir, PHP, Kotlin, Swift)

## Usage Format

```bash
CS_N=name:template:port:route
```

- **name**: Service identifier (alphanumeric + hyphens)
- **template**: Template from the list below
- **port**: Container port (optional, auto-assigns)
- **route**: URL path prefix or domain (optional)

## JavaScript/TypeScript (19 Templates)

### Node.js Core
- **`node-js`** - Plain Node.js HTTP server (JavaScript)
- **`node-ts`** - Plain Node.js HTTP server (TypeScript)

### Web Frameworks
- **`express-js`** - Express.js web framework (JavaScript)
- **`express-ts`** - Express.js web framework (TypeScript)
- **`fastify-js`** - High-performance Fastify framework (JavaScript)
- **`fastify-ts`** - High-performance Fastify framework (TypeScript)
- **`hono-js`** - Ultra-light edge-optimized framework (JavaScript)
- **`hono-ts`** - Ultra-light edge-optimized framework (TypeScript)

### Enterprise Frameworks
- **`nest-js`** - NestJS enterprise framework (JavaScript)
- **`nest-ts`** - NestJS enterprise framework (TypeScript)

### Real-time & Communication
- **`socketio-js`** - Socket.IO real-time bidirectional communication (JavaScript)
- **`socketio-ts`** - Socket.IO real-time bidirectional communication (TypeScript)

### Background Processing
- **`bullmq-js`** - Redis-backed job queues with BullMQ (JavaScript)
- **`bullmq-ts`** - Redis-backed job queues with BullMQ (TypeScript)

### Workflow Orchestration
- **`temporal-js`** - Temporal workflow orchestration (JavaScript)
- **`temporal-ts`** - Temporal workflow orchestration (TypeScript)

### Alternative Runtimes
- **`bun`** - Bun runtime server (JavaScript)
- **`deno`** - Deno runtime server (TypeScript)

### Type-safe APIs
- **`trpc`** - Type-safe RPC framework (TypeScript only)

#### Example Configurations
```bash
# Express API with TypeScript
CS_1=api:express-ts:3001:/api

# Real-time WebSocket service
CS_2=chat:socketio-ts:3002

# Background job processor
CS_3=jobs:bullmq-ts:3003

# High-performance API
CS_4=fast:fastify-ts:3004:/v1

# Enterprise microservice
CS_5=service:nest-ts:3005
```

## Python (7 Templates)

### Web Frameworks
- **`flask`** - Lightweight Flask microframework
- **`fastapi`** - Async type-hinted FastAPI framework
- **`django-rest`** - Django REST Framework APIs

### Distributed Processing
- **`celery`** - Distributed task queue with Redis backend
- **`ray`** - Distributed ML compute and model serving

### AI & Data
- **`agent-llm`** - LLM agent orchestration starter with OpenAI integration
- **`agent-data`** - Data-centric agent with pandas, scikit-learn, and DuckDB

#### Example Configurations
```bash
# FastAPI microservice
CS_1=api:fastapi:8001:/api

# Background task processor
CS_2=tasks:celery:8002

# ML model serving
CS_3=ml:ray:8003:/models

# AI agent service
CS_4=agent:agent-llm:8004:/chat

# Data processing service
CS_5=data:agent-data:8005:/process
```

## Go (4 Templates)

- **`gin`** - High-performance Gin web framework
- **`echo`** - Minimal Echo API framework
- **`fiber`** - Express-inspired, speed-focused Fiber framework
- **`grpc`** - Official Go gRPC implementation with Protocol Buffers

#### Example Configurations
```bash
# High-performance API
CS_1=api:gin:8001:/api

# Minimal microservice
CS_2=service:echo:8002

# Fast web service
CS_3=web:fiber:8003

# gRPC service
CS_4=grpc:grpc:50051
```

## Other Languages (10 Templates)

### Rust
- **`rust/axum`** - Modern async Rust web framework with Tokio

### Java
- **`java/spring-boot`** - Enterprise Java Spring Boot framework

### C#/.NET
- **`csharp/aspnet`** - ASP.NET Core Web API

### C++
- **`cpp/oatpp`** - Modern C++ web framework with high performance

### Ruby
- **`ruby/rails`** - Ruby on Rails in API mode

### Elixir
- **`elixir/phoenix`** - Productive Elixir Phoenix framework

### PHP
- **`php/laravel`** - Laravel PHP framework

### Kotlin
- **`kotlin/ktor`** - Asynchronous Kotlin Ktor framework

### Swift
- **`swift/vapor`** - Swift Vapor server-side framework

#### Example Configurations
```bash
# Rust high-performance API
CS_1=api:rust/axum:8001

# Java enterprise service
CS_2=enterprise:java/spring-boot:8002

# C# .NET API
CS_3=dotnet:csharp/aspnet:8003

# Ruby on Rails API
CS_4=rails:ruby/rails:8004

# Elixir Phoenix service
CS_5=phoenix:elixir/phoenix:8005
```

## Template Features

Every template includes:

### 🐳 Production Docker
- Multi-stage builds for smaller images
- Non-root container users
- Optimized layer caching
- Health check endpoints

### 🛡️ Security
- Security headers (X-Frame-Options, X-Content-Type-Options, etc.)
- CORS configuration
- Input validation patterns
- No secrets in container images

### 📊 Observability
- Health check endpoints at `/health`
- Structured logging support
- Environment-based configuration
- Version information in responses

### ⚡ Performance
- Graceful shutdown handling
- Connection pooling where applicable
- Production-optimized dependencies
- Efficient resource utilization

### 🔧 Developer Experience
- Template variable placeholders (`{{SERVICE_NAME}}`, `{{SERVICE_PORT}}`, etc.)
- Consistent file structure across languages
- Clear documentation comments
- Ready-to-customize code

## Custom Templates

Create your own templates in `src/templates/services/custom/`:

```bash
# Use custom template
CS_1=myservice:custom:8001

# nself will look for:
# src/templates/services/custom/Dockerfile.template
# src/templates/services/custom/[source files]
```

## Advanced Configuration

### Environment Variables
```bash
# Per-service environment variables
CS_1_ENV=NODE_ENV=production,LOG_LEVEL=info,API_KEY=${API_KEY}

# Health check customization
CS_1_HEALTH_PATH=/api/health
CS_1_HEALTH_TIMEOUT=30s
```

### Networking
```bash
# Internal service (no external access)
CS_1=internal:fastapi:8001

# Public service with domain
CS_2=api:fastapi:8002:api.example.com

# Service with custom route
CS_3=v1:fastapi:8003:/v1/api
```

### Scaling
```bash
# Multiple instances of the same service
CS_1=api-1:fastapi:8001:/api
CS_2=api-2:fastapi:8002:/api
CS_3=api-3:fastapi:8003:/api
```

## Migration & Updates

### From v0.3.8 to v0.3.9
- All 40 templates are now fully validated and production-ready
- Template names are standardized (use hyphens, not underscores)
- TypeScript versions available for most JavaScript frameworks

### Breaking Changes
- `nodejs-raw` → `node-js`
- `nodejs-raw-ts` → `node-ts`
- Removed incomplete templates from previous versions

## Best Practices

### 1. Choose the Right Template
- **High Performance**: `fastify-ts`, `gin`, `rust/axum`
- **Enterprise**: `nest-ts`, `java/spring-boot`
- **Rapid Development**: `fastapi`, `express-ts`
- **Real-time**: `socketio-ts`, `elixir/phoenix`
- **Background Jobs**: `bullmq-ts`, `celery`

### 2. Environment Configuration
```bash
# Use environment-specific settings
CS_1_ENV=NODE_ENV=${ENV},LOG_LEVEL=debug,DATABASE_URL=${DATABASE_URL}
```

### 3. Health Checks
```bash
# Configure appropriate timeouts
CS_1_HEALTH_TIMEOUT=30s
CS_1_HEALTH_INTERVAL=30s
```

### 4. Resource Limits
```bash
# Set appropriate memory limits
CS_1_MEMORY=512m
CS_1_CPUS=0.5
```

## Troubleshooting

### Template Not Found
```bash
# List available templates
find src/templates/services/ -name "Dockerfile.template"

# Verify template structure
ls src/templates/services/js/express-ts/
```

### Build Failures
```bash
# Check template validation
nself validate

# Review build logs
nself logs CS_1
```

### Service Not Starting
```bash
# Check service logs
nself logs [service-name]

# Verify port availability
nself status
```

## Related Documentation

- **[Configuration Reference](../configuration/CUSTOM-SERVICES-ENV-VARS.md)** - Detailed service configuration
- **[Examples](../examples/README.md)** - Real-world usage examples
- **[Architecture](../architecture/ARCHITECTURE.md)** - How services fit into nself
- **[Template Validation](../qa/QA-SUMMARY.md)** - Quality assurance report

---

> **Need a new template?** [Submit a feature request](https://github.com/nself-org/cli/issues) or contribute your own template following the existing patterns.
