#!/usr/bin/env bash


# Service Template Configuration System
# Maps service definitions to actual template directories

# Get template path for a service definition
# Format: language:framework or just framework
get_service_template() {

set -euo pipefail

  local service_def="$1"
  # Get template base dynamically
  local template_base=""
  if [[ -n "${NSELF_ROOT:-}" ]]; then
    template_base="$NSELF_ROOT/src/templates/services"
  elif [[ -d "$(dirname "${BASH_SOURCE[0]}")/../../../src/templates/services" ]]; then
    template_base="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../src/templates/services" && pwd)"
  else
    template_base="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/templates/services"
  fi

  case "$service_def" in
    # JavaScript/TypeScript frameworks
    express | express-js) echo "$template_base/js/express-js" ;;
    express-ts) echo "$template_base/js/express-ts" ;;
    fastify | fastify-js) echo "$template_base/js/fastify-js" ;;
    fastify-ts) echo "$template_base/js/fastify-ts" ;;
    hono | hono-js) echo "$template_base/js/hono-js" ;;
    hono-ts) echo "$template_base/js/hono-ts" ;;
    nest | nestjs | nest-js) echo "$template_base/js/nest-js" ;;
    nest-ts) echo "$template_base/js/nest-ts" ;;
    socketio | socketio-js) echo "$template_base/js/socketio-js" ;;
    socketio-ts) echo "$template_base/js/socketio-ts" ;;
    temporal | temporal-js) echo "$template_base/js/temporal-js" ;;
    temporal-ts) echo "$template_base/js/temporal-ts" ;;
    trpc) echo "$template_base/js/trpc" ;;
    bullmq | bullmq-js) echo "$template_base/js/bullmq-js" ;;
    bullmq-ts) echo "$template_base/js/bullmq-ts" ;;
    bun) echo "$template_base/js/bun" ;;
    deno) echo "$template_base/js/deno" ;;
    node | node-js) echo "$template_base/js/node-js" ;;
    node-ts) echo "$template_base/js/node-ts" ;;

    # Python frameworks
    fastapi | python:fastapi | py:fastapi) echo "$template_base/py/fastapi" ;;
    flask | python:flask | py:flask) echo "$template_base/py/flask" ;;
    django | django-rest | python:django | py:django) echo "$template_base/py/django-rest" ;;
    celery | python:celery | py:celery) echo "$template_base/py/celery" ;;
    ray | python:ray | py:ray) echo "$template_base/py/ray" ;;
    agent-llm | python:agent-llm | py:agent-llm) echo "$template_base/py/agent-llm" ;;
    agent-vision | python:agent-vision | py:agent-vision) echo "$template_base/py/agent-vision" ;;
    agent-analytics | python:agent-analytics | py:agent-analytics) echo "$template_base/py/agent-analytics" ;;
    agent-training | python:agent-training | py:agent-training) echo "$template_base/py/agent-training" ;;
    agent-timeseries | python:agent-timeseries | py:agent-timeseries) echo "$template_base/py/agent-timeseries" ;;

    # Go frameworks
    gin | go:gin | golang:gin) echo "$template_base/go/gin" ;;
    echo | go:echo | golang:echo) echo "$template_base/go/echo" ;;
    fiber | go:fiber | golang:fiber) echo "$template_base/go/fiber" ;;
    grpc | go:grpc | golang:grpc) echo "$template_base/go/grpc" ;;

    # Ruby frameworks
    rails | ruby:rails | rb:rails) echo "$template_base/ruby/rails" ;;
    sinatra | ruby:sinatra | rb:sinatra) echo "$template_base/ruby/sinatra" ;;

    # Rust frameworks
    actix | actix-web | rust:actix | rs:actix) echo "$template_base/rust/actix-web" ;;

    # Java frameworks
    spring | spring-boot | java:spring) echo "$template_base/java/spring-boot" ;;

    # C# frameworks
    aspnet | csharp:aspnet | cs:aspnet) echo "$template_base/csharp/aspnet" ;;

    # PHP frameworks
    laravel | php:laravel) echo "$template_base/php/laravel" ;;

    # Elixir frameworks
    phoenix | elixir:phoenix | ex:phoenix) echo "$template_base/elixir/phoenix" ;;

    # Kotlin frameworks
    ktor | kotlin:ktor | kt:ktor) echo "$template_base/kotlin/ktor" ;;

    # Swift frameworks
    vapor | swift:vapor) echo "$template_base/swift/vapor" ;;

    # C++ frameworks
    oatpp | cpp:oatpp) echo "$template_base/cpp/oatpp" ;;

    # Lua frameworks
    lapis | lua:lapis) echo "$template_base/lua/lapis" ;;

    # Zig frameworks
    zap | zig:zap) echo "$template_base/zig/zap" ;;

    # Default fallback
    *) echo "" ;;
  esac
}

# Parse service definition
# Format: name:framework:port or name:framework or just name
parse_service_definition() {
  local service_def="$1"
  local -n name_ref=$2
  local -n framework_ref=$3
  local -n port_ref=$4

  IFS=':' read -ra PARTS <<<"$service_def"

  name_ref="${PARTS[0]}"
  framework_ref="${PARTS[1]:-nest-ts}" # Default to nest-ts
  port_ref="${PARTS[2]:-}"             # Port is optional
}

# Get language from framework
get_language_from_framework() {
  local framework="$1"

  case "$framework" in
    express* | fastify* | hono* | nest* | socketio* | temporal* | trpc | bullmq* | bun | deno | node*)
      echo "javascript"
      ;;
    fastapi | flask | django* | celery | ray | agent-*)
      echo "python"
      ;;
    gin | echo | fiber | grpc)
      echo "go"
      ;;
    rails | sinatra)
      echo "ruby"
      ;;
    actix*)
      echo "rust"
      ;;
    spring*)
      echo "java"
      ;;
    aspnet)
      echo "csharp"
      ;;
    laravel)
      echo "php"
      ;;
    phoenix)
      echo "elixir"
      ;;
    ktor)
      echo "kotlin"
      ;;
    vapor)
      echo "swift"
      ;;
    oatpp)
      echo "cpp"
      ;;
    lapis)
      echo "lua"
      ;;
    zap)
      echo "zig"
      ;;
    *)
      echo "unknown"
      ;;
  esac
}

# Export functions
export -f get_service_template
export -f parse_service_definition
export -f get_language_from_framework
