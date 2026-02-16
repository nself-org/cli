# Service Templates Reference

Complete reference for all 40+ nself service templates.

## Quick Reference Table

| Template | Language | Category | Use Case |
|----------|----------|----------|----------|
| `socketio-ts` | TypeScript | Real-time | WebSocket, chat, notifications |
| `socketio-js` | JavaScript | Real-time | WebSocket, real-time events |
| `express-ts` | TypeScript | Web Framework | REST APIs, simple backends |
| `express-js` | JavaScript | Web Framework | REST APIs, simple backends |
| `fastify-ts` | TypeScript | Web Framework | High-performance APIs |
| `fastify-js` | JavaScript | Web Framework | High-performance APIs |
| `hono-ts` | TypeScript | Web Framework | Edge functions, serverless |
| `hono-js` | JavaScript | Web Framework | Edge functions, serverless |
| `nest-ts` | TypeScript | Full-Stack | Enterprise applications |
| `nest-js` | JavaScript | Full-Stack | Enterprise applications |
| `bullmq-ts` | TypeScript | Workers | Background jobs, queues |
| `bullmq-js` | JavaScript | Workers | Background jobs, queues |
| `temporal-ts` | TypeScript | Workflows | Durable execution |
| `temporal-js` | JavaScript | Workflows | Durable execution |
| `trpc` | TypeScript | API | Type-safe APIs |
| `bun` | JavaScript | Runtime | Fast Bun runtime |
| `deno` | TypeScript | Runtime | Secure Deno runtime |
| `node-ts` | TypeScript | Runtime | Basic Node.js server |
| `node-js` | JavaScript | Runtime | Basic Node.js server |
| `fastapi` | Python | Full-Stack | Modern Python APIs |
| `flask` | Python | Web Framework | Lightweight Python APIs |
| `django-rest` | Python | Full-Stack | Enterprise Python apps |
| `celery` | Python | Workers | Distributed task queue |
| `ray` | Python | Distributed | ML workloads, parallel |
| `agent-llm` | Python | AI/ML | LLM integration |
| `agent-vision` | Python | AI/ML | Computer vision |
| `agent-analytics` | Python | AI/ML | Data analytics |
| `agent-training` | Python | AI/ML | Model training |
| `agent-timeseries` | Python | AI/ML | Time series forecasting |
| `gin` | Go | Web Framework | Fast Go APIs |
| `fiber` | Go | Web Framework | Express-like Go |
| `echo` | Go | Web Framework | Minimalist Go |
| `grpc` | Go | RPC | High-performance RPC |
| `rails` | Ruby | Full-Stack | Full-stack Ruby |
| `sinatra` | Ruby | Web Framework | Lightweight Ruby |
| `actix-web` | Rust | Web Framework | High-performance Rust |
| `spring-boot` | Java | Full-Stack | Enterprise Java |
| `aspnet` | C# | Full-Stack | .NET Core apps |
| `laravel` | PHP | Full-Stack | Modern PHP |
| `phoenix` | Elixir | Full-Stack | Real-time Elixir |
| `ktor` | Kotlin | Web Framework | Kotlin async |
| `vapor` | Swift | Web Framework | Server-side Swift |
| `oatpp` | C++ | Web Framework | High-performance C++ |
| `lapis` | Lua | Web Framework | OpenResty Lua |
| `zap` | Zig | Web Framework | Blazingly fast Zig |

---

## JavaScript/TypeScript Templates

### Socket.IO Templates

#### `socketio-ts` (Recommended)

**Type**: Real-time WebSocket server with TypeScript

**Features:**
- Type-safe event definitions
- Redis adapter ready for horizontal scaling
- Room and namespace support
- Presence tracking
- Broadcasting
- Authentication middleware
- Health checks
- Graceful shutdown

**Generated Files:**
```
services/<name>/
├── package.json
├── tsconfig.json
├── Dockerfile
├── README.md
└── src/
    └── server.ts    # Complete Socket.IO server
```

**Usage:**
```bash
nself service scaffold realtime --template socketio-ts --port 3101
```

**Perfect For:**
- Chat applications
- Real-time dashboards
- Live notifications
- Collaborative editing
- Online gaming backends
- WebRTC signaling

**Dependencies:**
- socket.io ^4.7.2
- express ^4.18.2
- cors ^2.8.5
- typescript ^5.2.0

#### `socketio-js`

JavaScript version of Socket.IO server. Same features as `socketio-ts` but without TypeScript.

### Express Templates

#### `express-ts`

**Type**: Minimalist web framework with TypeScript

**Features:**
- Lightweight and flexible
- Middleware system
- Routing
- Error handling
- Health checks
- CORS configured

**Usage:**
```bash
nself service scaffold api --template express-ts --port 4000
```

**Perfect For:**
- Simple REST APIs
- Microservices
- Prototypes
- Learning projects

**Dependencies:**
- express ^4.18.2
- cors ^2.8.5
- @types/express ^4.17.17
- typescript ^5.2.0

#### `express-js`

JavaScript version. Same features without TypeScript.

### Fastify Templates

#### `fastify-ts`

**Type**: High-performance web framework with TypeScript

**Features:**
- Ultra-fast (benchmarks show 65k+ req/sec)
- JSON schema validation
- Automatic serialization
- Plugin architecture
- Logging with Pino
- TypeScript support

**Usage:**
```bash
nself service scaffold api --template fastify-ts --port 4000
```

**Perfect For:**
- High-throughput APIs
- Performance-critical services
- Microservices with validation
- JSON APIs

**Dependencies:**
- fastify ^4.24.0
- @fastify/cors ^8.4.0
- @fastify/helmet ^11.1.0
- typescript ^5.2.0

#### `fastify-js`

JavaScript version with same performance benefits.

### Hono Templates

#### `hono-ts`

**Type**: Ultra-fast edge-ready framework

**Features:**
- Extremely lightweight (12KB)
- Edge runtime support (Cloudflare Workers, Deno Deploy)
- Fast routing
- Middleware
- Type-safe

**Usage:**
```bash
nself service scaffold edge --template hono-ts --port 3000
```

**Perfect For:**
- Edge functions
- Serverless APIs
- Cloudflare Workers
- Ultra-fast APIs

**Dependencies:**
- hono ^3.9.0
- typescript ^5.2.0

#### `hono-js`

JavaScript version for edge deployments.

### NestJS Templates

#### `nest-ts` (Recommended for Enterprise)

**Type**: Progressive Node.js framework

**Features:**
- TypeScript-first
- Dependency injection
- Modular architecture
- Decorators
- Guards and interceptors
- Database integration (TypeORM)
- Swagger/OpenAPI
- Testing utilities

**Usage:**
```bash
nself service scaffold backend --template nest-ts --port 4000
```

**Perfect For:**
- Enterprise applications
- Complex business logic
- Team projects
- Scalable architectures
- Microservices
- GraphQL APIs

**Dependencies:**
- @nestjs/core ^10.2.0
- @nestjs/platform-express ^10.2.0
- @nestjs/common ^10.2.0
- reflect-metadata ^0.1.13
- typescript ^5.2.0

#### `nest-js`

JavaScript version (less common, TypeScript recommended).

### BullMQ Templates

#### `bullmq-ts` (Recommended)

**Type**: Redis-backed job queue with TypeScript

**Features:**
- Type-safe job definitions
- Job priorities
- Delayed jobs
- Job retries with backoff
- Rate limiting
- Job events
- Sandboxed processors
- Redis-backed persistence

**Usage:**
```bash
nself service scaffold worker --template bullmq-ts --port 3102
```

**Perfect For:**
- Email sending
- Image processing
- Video transcoding
- Report generation
- Batch processing
- Scheduled tasks

**Dependencies:**
- bullmq ^4.14.0
- ioredis ^5.3.0
- typescript ^5.2.0

#### `bullmq-js`

JavaScript version of BullMQ worker.

### Temporal Templates

#### `temporal-ts`

**Type**: Durable workflow orchestration

**Features:**
- Durable execution
- Workflow versioning
- Activity retries
- Saga pattern support
- Long-running processes
- Fault tolerance

**Usage:**
```bash
nself service scaffold workflows --template temporal-ts --port 3103
```

**Perfect For:**
- Multi-step business processes
- Payment processing
- Order fulfillment
- ETL pipelines
- Saga orchestration

**Dependencies:**
- @temporalio/worker ^1.8.0
- @temporalio/client ^1.8.0
- typescript ^5.2.0

#### `temporal-js`

JavaScript version of Temporal worker.

### Other JavaScript/TypeScript Templates

#### `trpc`

**Type**: End-to-end typesafe APIs

**Features:**
- Full TypeScript inference
- No code generation
- Autocomplete everywhere
- Zod validation
- Minimal bundle size

**Perfect For:**
- Full-stack TypeScript apps
- Type-safe APIs
- Monorepos
- Next.js backends

#### `bun`

**Type**: Bun runtime server

**Features:**
- Extremely fast startup
- Built-in bundler
- Native TypeScript
- npm-compatible

**Perfect For:**
- Development servers
- Fast prototypes
- Modern JavaScript

#### `deno`

**Type**: Deno runtime server

**Features:**
- Secure by default
- TypeScript native
- Web standard APIs
- No node_modules

**Perfect For:**
- Secure services
- Modern TypeScript
- Serverless functions

#### `node-ts` / `node-js`

**Type**: Basic Node.js HTTP server

**Features:**
- Minimal dependencies
- Built-in HTTP module
- Starting point for custom servers

**Perfect For:**
- Learning
- Custom implementations
- Minimal overhead

---

## Python Templates

### FastAPI Template

#### `fastapi` (Recommended)

**Type**: Modern, fast web framework for Python APIs

**Features:**
- Automatic OpenAPI docs (Swagger UI at `/docs`)
- Pydantic validation
- Async/await support
- Type hints
- Dependency injection
- Security utilities (OAuth2, JWT)
- CORS middleware

**Usage:**
```bash
nself service scaffold api --template fastapi --port 8000
```

**Perfect For:**
- Modern REST APIs
- Microservices
- ML model serving
- Real-time APIs
- Data validation-heavy apps

**Dependencies:**
- fastapi ^0.104.0
- uvicorn[standard] ^0.24.0
- pydantic ^2.4.0

**Example Code:**
```python
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()

class Item(BaseModel):
    name: str
    price: float

@app.post("/items/")
async def create_item(item: Item):
    return {"item": item}
```

### Flask Template

#### `flask`

**Type**: Lightweight WSGI web framework

**Features:**
- Minimal and flexible
- Jinja2 templates
- Extension system
- Simple to learn
- RESTful request dispatching

**Usage:**
```bash
nself service scaffold api --template flask --port 5000
```

**Perfect For:**
- Simple APIs
- Prototypes
- Learning Python web development
- Microservices

**Dependencies:**
- flask ^3.0.0
- flask-cors ^4.0.0
- gunicorn ^21.2.0

### Django REST Framework

#### `django-rest`

**Type**: Powerful toolkit for building Web APIs

**Features:**
- Django ORM
- Admin panel
- Authentication/authorization
- Serialization
- Browsable API
- Class-based views

**Usage:**
```bash
nself service scaffold backend --template django-rest --port 8000
```

**Perfect For:**
- Full-featured APIs
- Complex data models
- Admin interfaces
- Enterprise applications

**Dependencies:**
- django ^4.2.0
- djangorestframework ^3.14.0
- psycopg2-binary ^2.9.0

### Celery Worker

#### `celery`

**Type**: Distributed task queue

**Features:**
- Async task execution
- Scheduled tasks (cron-like)
- Task routing
- Result backends
- Monitoring
- Multi-worker support

**Usage:**
```bash
nself service scaffold worker --template celery --port 5555
```

**Perfect For:**
- Background processing
- Scheduled jobs
- Email sending
- Report generation
- Long-running tasks

**Dependencies:**
- celery ^5.3.0
- redis ^5.0.0
- kombu ^5.3.0

### Ray Worker

#### `ray`

**Type**: Distributed computing framework

**Features:**
- Parallel processing
- Distributed data processing
- ML workload distribution
- Actor model
- Task scheduling

**Usage:**
```bash
nself service scaffold compute --template ray --port 8265
```

**Perfect For:**
- ML training
- Data processing
- Parallel computation
- Scientific computing

### AI/ML Agent Templates

#### `agent-llm`

**Type**: Large Language Model integration service

**Features:**
- OpenAI API integration
- LLM provider integration
- Streaming responses
- RAG (Retrieval Augmented Generation)
- Function calling
- Conversation history
- Prompt templates

**Usage:**
```bash
nself service scaffold ai --template agent-llm --port 8001
```

**Perfect For:**
- Chatbots
- Content generation
- Code assistance
- Question answering
- Summarization

**Dependencies:**
- openai ^1.3.0
- llm-providers ^0.7.0
- langchain ^0.0.340
- chromadb ^0.4.0

#### `agent-vision`

**Type**: Computer vision service

**Features:**
- Image classification
- Object detection
- OCR
- Image generation
- Face recognition
- Preprocessing pipelines

**Usage:**
```bash
nself service scaffold vision --template agent-vision --port 8002
```

**Perfect For:**
- Image analysis
- Document processing
- Visual search
- Quality control
- Surveillance

**Dependencies:**
- opencv-python ^4.8.0
- pillow ^10.1.0
- torch ^2.1.0
- transformers ^4.35.0

#### `agent-analytics`

**Type**: Data analytics service

**Features:**
- Data processing with pandas
- Statistical analysis
- Visualization
- Report generation
- Data export

**Usage:**
```bash
nself service scaffold analytics --template agent-analytics --port 8003
```

**Perfect For:**
- Business intelligence
- Data dashboards
- Report automation
- KPI tracking

**Dependencies:**
- pandas ^2.1.0
- numpy ^1.26.0
- matplotlib ^3.8.0
- plotly ^5.17.0

#### `agent-training`

**Type**: ML model training service

**Features:**
- Model training
- Hyperparameter tuning
- Experiment tracking (MLflow)
- Model versioning
- Model serving

**Usage:**
```bash
nself service scaffold training --template agent-training --port 8004
```

**Perfect For:**
- ML model development
- AutoML pipelines
- Model experimentation
- A/B testing models

**Dependencies:**
- scikit-learn ^1.3.0
- tensorflow ^2.14.0
- pytorch ^2.1.0
- mlflow ^2.8.0

#### `agent-timeseries`

**Type**: Time series analysis and forecasting

**Features:**
- Forecasting models
- Anomaly detection
- Trend analysis
- Seasonality decomposition
- Prophet integration

**Usage:**
```bash
nself service scaffold forecasting --template agent-timeseries --port 8005
```

**Perfect For:**
- Sales forecasting
- Demand prediction
- Anomaly detection
- Financial analysis

**Dependencies:**
- prophet ^1.1.0
- statsmodels ^0.14.0
- pandas ^2.1.0
- numpy ^1.26.0

---

## Go Templates

### Gin Template

#### `gin`

**Type**: HTTP web framework

**Features:**
- Fast routing (40x faster than martini)
- Middleware support
- JSON validation
- Error management
- Crash-free
- Group routing

**Usage:**
```bash
nself service scaffold api --template gin --port 8080
```

**Perfect For:**
- High-performance APIs
- Microservices
- RESTful services
- JSON APIs

**Dependencies:**
```go
github.com/gin-gonic/gin v1.9.1
```

### Fiber Template

#### `fiber`

**Type**: Express-inspired framework

**Features:**
- Express-like API
- Fast HTTP engine (fasthttp)
- Low memory footprint
- Middleware
- Routing
- WebSocket support

**Usage:**
```bash
nself service scaffold api --template fiber --port 8080
```

**Perfect For:**
- Developers from Node.js
- High-performance needs
- Microservices

**Dependencies:**
```go
github.com/gofiber/fiber/v2 v2.50.0
```

### Echo Template

#### `echo`

**Type**: High-performance framework

**Features:**
- Optimized router
- Middleware
- Data binding
- HTTP/2 support
- Template rendering

**Usage:**
```bash
nself service scaffold api --template echo --port 8080
```

**Perfect For:**
- RESTful APIs
- Microservices
- High-traffic services

**Dependencies:**
```go
github.com/labstack/echo/v4 v4.11.3
```

### gRPC Template

#### `grpc`

**Type**: High-performance RPC framework

**Features:**
- Protocol Buffers
- Streaming (unary, server, client, bidirectional)
- Language-agnostic
- Service mesh ready
- HTTP/2 based

**Usage:**
```bash
nself service scaffold rpc --template grpc --port 50051
```

**Perfect For:**
- Microservice communication
- Low-latency services
- Polyglot architectures
- Service mesh

**Dependencies:**
```go
google.golang.org/grpc v1.59.0
google.golang.org/protobuf v1.31.0
```

---

## Other Language Templates

### Ruby Templates

#### `rails`
- Full-stack MVC framework
- Active Record ORM
- Asset pipeline
- Scaffolding

#### `sinatra`
- Lightweight DSL
- Minimal and flexible
- Perfect for small APIs

### Rust Templates

#### `actix-web`
- Actor-based framework
- Extremely fast
- Type-safe
- Async/await

### Java Templates

#### `spring-boot`
- Enterprise framework
- Auto-configuration
- Embedded servers
- Production-ready

### C# Templates

#### `aspnet`
- ASP.NET Core
- Cross-platform
- High performance
- Cloud-ready

### PHP Templates

#### `laravel`
- Modern PHP framework
- Eloquent ORM
- Blade templates
- Artisan CLI

### Elixir Templates

#### `phoenix`
- Real-time capabilities
- LiveView
- Channels (WebSocket)
- Fault-tolerant

### Kotlin Templates

#### `ktor`
- Asynchronous
- Coroutines
- Type-safe DSL
- Lightweight

### Swift Templates

#### `vapor`
- Server-side Swift
- Type-safe
- Async/await
- Fluent ORM

### C++ Templates

#### `oatpp`
- Modern C++ web framework
- High performance
- Swagger integration
- Zero-copy streams

### Lua Templates

#### `lapis`
- OpenResty framework
- Fast (nginx-based)
- MoonScript support
- PostgreSQL integration

### Zig Templates

#### `zap`
- Blazingly fast
- Memory-safe
- Low overhead
- HTTP/1.1 support

---

## See Also

- [Service Code Generation Guide](../guides/SERVICE-CODE-GENERATION.md)
- [Custom Services Guide](SERVICES_CUSTOM.md)
- [Architecture Overview](../architecture/ARCHITECTURE.md)
