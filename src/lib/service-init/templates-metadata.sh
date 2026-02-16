#!/usr/bin/env bash

# templates-metadata.sh - Service template metadata and descriptions

# Get template metadata
# Returns: name|language|description|features|dependencies
get_template_metadata() {

set -euo pipefail

  local template_name="$1"

  case "$template_name" in
    # === JavaScript/TypeScript Frameworks ===

    socketio-js)
      printf "Socket.IO (JavaScript)|javascript|Real-time bidirectional event-based communication|"
      printf "WebSocket support, Event emitters, Room support, Broadcasting|"
      printf "socket.io, express, cors"
      ;;

    socketio-ts)
      printf "Socket.IO (TypeScript)|typescript|Real-time bidirectional event-based communication with TypeScript|"
      printf "WebSocket support, Type-safe events, Redis adapter ready, Room/namespace support, Broadcasting, Authentication hooks|"
      printf "socket.io, express, cors, @types/node, typescript, tsx"
      ;;

    express-js)
      printf "Express.js (JavaScript)|javascript|Fast, minimalist web framework for Node.js|"
      printf "Middleware support, Routing, Template engines, Static files|"
      printf "express, cors, body-parser"
      ;;

    express-ts)
      printf "Express.js (TypeScript)|typescript|Fast, minimalist web framework with TypeScript|"
      printf "Type safety, Middleware support, Routing, Error handling, Health checks|"
      printf "express, cors, @types/express, typescript, tsx"
      ;;

    fastify-js)
      printf "Fastify (JavaScript)|javascript|Fast and low overhead web framework|"
      printf "Schema validation, Logging, Plugins, Async/await|"
      printf "fastify, @fastify/cors, @fastify/helmet"
      ;;

    fastify-ts)
      printf "Fastify (TypeScript)|typescript|Fast and low overhead web framework with TypeScript|"
      printf "JSON schema validation, Type safety, Plugins, Logging, Performance|"
      printf "fastify, @fastify/cors, @fastify/helmet, typescript, tsx"
      ;;

    hono-js)
      printf "Hono (JavaScript)|javascript|Ultrafast web framework for the edge|"
      printf "Small, Fast, Multi-runtime support, Middleware|"
      printf "hono"
      ;;

    hono-ts)
      printf "Hono (TypeScript)|typescript|Ultrafast web framework for the edge with TypeScript|"
      printf "Ultra-fast, Edge ready, Type-safe routing, Middleware, Lightweight|"
      printf "hono, typescript, tsx"
      ;;

    nest-js)
      printf "NestJS (JavaScript)|javascript|Progressive Node.js framework for scalable applications|"
      printf "TypeScript-first, Modular architecture, Dependency injection, Decorators|"
      printf "@nestjs/core, @nestjs/platform-express, reflect-metadata"
      ;;

    nest-ts)
      printf "NestJS (TypeScript)|typescript|Progressive Node.js framework with full TypeScript support|"
      printf "Enterprise architecture, Dependency injection, Decorators, Modules, Interceptors, Guards|"
      printf "@nestjs/core, @nestjs/platform-express, @nestjs/common, typescript"
      ;;

    bullmq-js)
      printf "BullMQ Worker (JavaScript)|javascript|Background job processing with Redis|"
      printf "Job queues, Delayed jobs, Retries, Job priorities|"
      printf "bullmq, ioredis"
      ;;

    bullmq-ts)
      printf "BullMQ Worker (TypeScript)|typescript|Background job processing with Redis and TypeScript|"
      printf "Type-safe jobs, Job queues, Retries, Priorities, Scheduling, Rate limiting|"
      printf "bullmq, ioredis, typescript, tsx"
      ;;

    temporal-js)
      printf "Temporal Worker (JavaScript)|javascript|Durable workflow execution engine|"
      printf "Workflows, Activities, Long-running processes, Retry logic|"
      printf "@temporalio/worker, @temporalio/client"
      ;;

    temporal-ts)
      printf "Temporal Worker (TypeScript)|typescript|Durable workflow execution engine with TypeScript|"
      printf "Type-safe workflows, Durable execution, Retry logic, Long-running processes|"
      printf "@temporalio/worker, @temporalio/client, typescript"
      ;;

    trpc)
      printf "tRPC Server|typescript|End-to-end typesafe APIs made easy|"
      printf "Full-stack TypeScript, Type inference, No code generation, Autocomplete|"
      printf "@trpc/server, zod, typescript"
      ;;

    bun)
      printf "Bun Server|javascript|Fast all-in-one JavaScript runtime|"
      printf "Built-in bundler, Native TypeScript, Fast startup, HTTP server|"
      printf "None (uses Bun runtime)"
      ;;

    deno)
      printf "Deno Server|typescript|Secure TypeScript runtime|"
      printf "Secure by default, TypeScript native, Web standard APIs|"
      printf "None (uses Deno runtime)"
      ;;

    node-js)
      printf "Node.js HTTP Server|javascript|Basic Node.js HTTP server|"
      printf "Minimal, No framework, HTTP module only|"
      printf "None (Node.js built-in)"
      ;;

    node-ts)
      printf "Node.js HTTP Server (TypeScript)|typescript|Basic Node.js HTTP server with TypeScript|"
      printf "Minimal, Type-safe, No framework, HTTP module only|"
      printf "@types/node, typescript, tsx"
      ;;

    # === Python Frameworks ===

    fastapi)
      printf "FastAPI|python|Modern, fast web framework for building APIs with Python|"
      printf "Auto docs (Swagger/ReDoc), Type hints, Async support, Pydantic validation|"
      printf "fastapi, uvicorn, pydantic"
      ;;

    flask)
      printf "Flask|python|Lightweight WSGI web application framework|"
      printf "Simple, Flexible, Extensions, Jinja2 templates|"
      printf "flask, flask-cors, gunicorn"
      ;;

    django-rest)
      printf "Django REST Framework|python|Powerful toolkit for building Web APIs|"
      printf "ORM, Admin panel, Authentication, Serialization, Browsable API|"
      printf "django, djangorestframework, psycopg2"
      ;;

    celery)
      printf "Celery Worker|python|Distributed task queue|"
      printf "Async tasks, Scheduled jobs, Result backend, Retries|"
      printf "celery, redis, kombu"
      ;;

    ray)
      printf "Ray Worker|python|Distributed computing framework|"
      printf "Parallel processing, ML workloads, Distributed training|"
      printf "ray, numpy"
      ;;

    agent-llm)
      printf "LLM Agent|python|Large Language Model agent service|"
      printf "Multiple LLM provider integration, Streaming, RAG, Function calling|"
      printf "openai, llm-providers, langchain, chromadb"
      ;;

    agent-vision)
      printf "Vision Agent|python|Computer vision and image processing agent|"
      printf "Image recognition, Object detection, OCR, Image generation|"
      printf "opencv-python, pillow, torch, transformers"
      ;;

    agent-analytics)
      printf "Analytics Agent|python|Data analytics and visualization agent|"
      printf "Data processing, Statistical analysis, Visualization, Reporting|"
      printf "pandas, numpy, matplotlib, plotly"
      ;;

    agent-training)
      printf "Model Training Agent|python|ML model training and evaluation|"
      printf "Model training, Hyperparameter tuning, Experiment tracking, Model serving|"
      printf "scikit-learn, tensorflow, pytorch, mlflow"
      ;;

    agent-timeseries)
      printf "Time Series Agent|python|Time series analysis and forecasting|"
      printf "Forecasting, Anomaly detection, Trend analysis, Seasonality|"
      printf "prophet, statsmodels, pandas, numpy"
      ;;

    # === Go Frameworks ===

    gin)
      printf "Gin|go|HTTP web framework with performance and productivity|"
      printf "Fast routing, Middleware, JSON validation, Error management|"
      printf "github.com/gin-gonic/gin"
      ;;

    fiber)
      printf "Fiber|go|Express-inspired web framework built on Fasthttp|"
      printf "Express-like API, Fast, Low memory, Middleware, Routing|"
      printf "github.com/gofiber/fiber/v2"
      ;;

    echo)
      printf "Echo|go|High performance, minimalist web framework|"
      printf "Optimized router, Middleware, Data binding, HTTP/2 support|"
      printf "github.com/labstack/echo/v4"
      ;;

    grpc)
      printf "gRPC|go|High-performance RPC framework|"
      printf "Protocol Buffers, Streaming, Language-agnostic, Service mesh ready|"
      printf "google.golang.org/grpc, google.golang.org/protobuf"
      ;;

    # === Ruby Frameworks ===

    rails)
      printf "Ruby on Rails|ruby|Full-stack web framework|"
      printf "MVC, Active Record, Scaffolding, Convention over configuration|"
      printf "rails, puma, pg"
      ;;

    sinatra)
      printf "Sinatra|ruby|DSL for quickly creating web applications|"
      printf "Minimal, Flexible, Routing, Templates|"
      printf "sinatra, puma, rack"
      ;;

    # === Rust Frameworks ===

    actix-web)
      printf "Actix Web|rust|Powerful, pragmatic, and fast web framework|"
      printf "Actor-based, Async, Type-safe, High performance|"
      printf "actix-web, tokio, serde"
      ;;

    # === Java Frameworks ===

    spring-boot)
      printf "Spring Boot|java|Java framework for production-ready applications|"
      printf "Auto-configuration, Embedded server, Production metrics, Security|"
      printf "spring-boot-starter-web, spring-boot-starter-data-jpa"
      ;;

    # === Other Frameworks ===

    aspnet)
      printf "ASP.NET Core|csharp|Cross-platform .NET framework|"
      printf "High performance, Cloud-ready, Dependency injection, Middleware|"
      printf "Microsoft.AspNetCore.App"
      ;;

    laravel)
      printf "Laravel|php|PHP web framework with elegant syntax|"
      printf "Eloquent ORM, Blade templates, Artisan CLI, Queue system|"
      printf "laravel/framework, guzzlehttp/guzzle"
      ;;

    phoenix)
      printf "Phoenix|elixir|Productive web framework for Elixir|"
      printf "LiveView, Channels, PubSub, Real-time, Fault-tolerant|"
      printf "phoenix, ecto, postgrex"
      ;;

    ktor)
      printf "Ktor|kotlin|Asynchronous framework for Kotlin|"
      printf "Coroutines, Lightweight, Flexible, Type-safe DSL|"
      printf "io.ktor:ktor-server-core, io.ktor:ktor-server-netty"
      ;;

    vapor)
      printf "Vapor|swift|Web framework for Swift|"
      printf "Type-safe, Async/await, Fluent ORM, WebSocket support|"
      printf "vapor/vapor, fluent"
      ;;

    oatpp)
      printf "Oat++|cpp|Modern C++ web framework|"
      printf "High performance, API-first, Swagger integration, Async|"
      printf "oatpp"
      ;;

    lapis)
      printf "Lapis|lua|Web framework for Lua and OpenResty|"
      printf "Fast, MVC, PostgreSQL support, Template engine|"
      printf "lapis"
      ;;

    zap)
      printf "Zap|zig|Blazingly fast web framework for Zig|"
      printf "Extremely fast, Low memory, HTTP/1.1 support|"
      printf "None (Zig standard library)"
      ;;

    *)
      printf "Unknown|unknown|No description available||"
      ;;
  esac
}

# Get all available templates
list_all_templates() {
  printf "socketio-js\n"
  printf "socketio-ts\n"
  printf "express-js\n"
  printf "express-ts\n"
  printf "fastify-js\n"
  printf "fastify-ts\n"
  printf "hono-js\n"
  printf "hono-ts\n"
  printf "nest-js\n"
  printf "nest-ts\n"
  printf "bullmq-js\n"
  printf "bullmq-ts\n"
  printf "temporal-js\n"
  printf "temporal-ts\n"
  printf "trpc\n"
  printf "bun\n"
  printf "deno\n"
  printf "node-js\n"
  printf "node-ts\n"
  printf "fastapi\n"
  printf "flask\n"
  printf "django-rest\n"
  printf "celery\n"
  printf "ray\n"
  printf "agent-llm\n"
  printf "agent-vision\n"
  printf "agent-analytics\n"
  printf "agent-training\n"
  printf "agent-timeseries\n"
  printf "gin\n"
  printf "fiber\n"
  printf "echo\n"
  printf "grpc\n"
  printf "rails\n"
  printf "sinatra\n"
  printf "actix-web\n"
  printf "spring-boot\n"
  printf "aspnet\n"
  printf "laravel\n"
  printf "phoenix\n"
  printf "ktor\n"
  printf "vapor\n"
  printf "oatpp\n"
  printf "lapis\n"
  printf "zap\n"
}

# Get templates by language
get_templates_by_language() {
  local lang="$1"

  case "$lang" in
    javascript | js)
      printf "express-js fastify-js hono-js nest-js socketio-js bullmq-js temporal-js bun deno node-js\n"
      ;;
    typescript | ts)
      printf "express-ts fastify-ts hono-ts nest-ts socketio-ts bullmq-ts temporal-ts trpc node-ts\n"
      ;;
    python | py)
      printf "fastapi flask django-rest celery ray agent-llm agent-vision agent-analytics agent-training agent-timeseries\n"
      ;;
    go | golang)
      printf "gin fiber echo grpc\n"
      ;;
    ruby | rb)
      printf "rails sinatra\n"
      ;;
    rust | rs)
      printf "actix-web\n"
      ;;
    java)
      printf "spring-boot\n"
      ;;
    csharp | cs)
      printf "aspnet\n"
      ;;
    php)
      printf "laravel\n"
      ;;
    elixir | ex)
      printf "phoenix\n"
      ;;
    kotlin | kt)
      printf "ktor\n"
      ;;
    swift)
      printf "vapor\n"
      ;;
    cpp | c++)
      printf "oatpp\n"
      ;;
    lua)
      printf "lapis\n"
      ;;
    zig)
      printf "zap\n"
      ;;
    *)
      printf ""
      ;;
  esac
}

# Get template category
get_template_category() {
  local template="$1"

  case "$template" in
    socketio-* | temporal-*)
      printf "Real-time & Messaging"
      ;;
    express-* | fastify-* | hono-* | gin | fiber | echo | rails | sinatra | actix-web | spring-boot | aspnet | laravel | phoenix | ktor | vapor | oatpp | lapis | zap)
      printf "Web Frameworks"
      ;;
    nest-* | fastapi | django-rest | flask)
      printf "Full-Stack Frameworks"
      ;;
    bullmq-* | celery | ray)
      printf "Background Jobs & Workers"
      ;;
    agent-*)
      printf "AI & ML Agents"
      ;;
    trpc)
      printf "API Frameworks"
      ;;
    grpc)
      printf "RPC Frameworks"
      ;;
    bun | deno | node-*)
      printf "Runtime Servers"
      ;;
    *)
      printf "Other"
      ;;
  esac
}

# Export functions
export -f get_template_metadata
export -f list_all_templates
export -f get_templates_by_language
export -f get_template_category
