#!/usr/bin/env bash
set -euo pipefail
# custom-services-templates.sh - Generate Docker services from template-based CS_ variables

# Generate custom service from template-based CS_ variable
generate_template_based_service() {
  local index="$1"
  local service_name="$2"
  local template_type="$3"
  local service_port="$4"

  # Skip if service directory doesn't exist (template not copied)
  [[ ! -d "services/$service_name" ]] && return 0

  # Check if replicas are specified (can't use container_name with replicas > 1)
  local replicas_var="CS_${index}_REPLICAS"
  local has_replicas=false
  local replica_count="${!replicas_var:-1}"

  # CRITICAL (Bug #11): Block replicas > 1 with port bindings in standalone mode
  # Docker Compose standalone doesn't support port bindings with replicas
  # All replicas try to bind to the same host port, causing conflicts
  if [[ "$replica_count" -gt 1 ]] && [[ -n "$service_port" && "$service_port" != "0" ]]; then
    printf "\n${CLI_RED}✗ ERROR: ${service_name} has replicas=${replica_count} with port binding${CLI_RESET}\n" >&2
    printf "  Port bindings conflict with replicas in standalone Docker Compose\n" >&2
    printf "\n" >&2
    printf "  ${CLI_YELLOW}Fix options:${CLI_RESET}\n" >&2
    printf "    1. Set CS_${index}_REPLICAS=1 (recommended for standalone mode)\n" >&2
    printf "    2. Remove port binding (set CS_${index}_PORT=0)\n" >&2
    printf "    3. Deploy to Docker Swarm mode (supports load-balanced ports)\n" >&2
    printf "\n" >&2
    printf "  ${CLI_DIM}Replicas > 1 require Docker Swarm for proper load balancing${CLI_RESET}\n" >&2
    printf "  ${CLI_DIM}In standalone mode, all replicas compete for the same host port${CLI_RESET}\n" >&2
    exit 1
  fi

  if [[ "$replica_count" -gt 1 ]]; then
    has_replicas=true
  fi

  cat <<EOF

  # Custom Service ${index}: ${service_name}
  ${service_name}:
    build:
      context: ./services/${service_name}
      dockerfile: Dockerfile
EOF

  # Only add container_name if not using replicas (container_name conflicts with replicas > 1)
  if [[ "$has_replicas" != "true" ]]; then
    echo "    container_name: \${PROJECT_NAME}_${service_name}"
  fi

  cat <<EOF
    restart: unless-stopped
    networks:
      - \${DOCKER_NETWORK}
EOF

  # Add ports if specified
  if [[ -n "$service_port" && "$service_port" != "0" ]]; then
    cat <<EOF
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:${service_port}:${service_port}"
EOF
  fi

  # Add environment variables
  cat <<EOF
    environment:
      - ENV=\${ENV:-dev}
      - NODE_ENV=\${ENV:-dev}
      - APP_ENV=\${ENV:-dev}
      - ENVIRONMENT=\${ENV:-dev}
      - PROJECT_NAME=\${PROJECT_NAME}
      - BASE_DOMAIN=\${BASE_DOMAIN:-localhost}
      - DOCKER_NETWORK=\${DOCKER_NETWORK:-\${PROJECT_NAME}_network}
      - SERVICE_NAME=${service_name}
      - SERVICE_PORT=${service_port}
      - PORT=${service_port}
      - POSTGRES_HOST=postgres
      - POSTGRES_PORT=5432
      - POSTGRES_DB=\${POSTGRES_DB}
      - POSTGRES_USER=\${POSTGRES_USER}
      - POSTGRES_PASSWORD=\${POSTGRES_PASSWORD}
      - DATABASE_URL=postgres://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}?sslmode=disable
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - REDIS_PASSWORD=\${REDIS_PASSWORD:-}
      - REDIS_URL=redis://redis:6379
      - HASURA_GRAPHQL_ENDPOINT=http://hasura:8080/v1/graphql
      - HASURA_ADMIN_SECRET=\${HASURA_GRAPHQL_ADMIN_SECRET}
EOF

  # Add volumes ONLY for development (not staging/production)
  # CRITICAL (Bug #23 fix): Skip volume mounts entirely for compiled-language templates.
  # Volume mounts overwrite the compiled binary from the multi-stage Docker build,
  # causing "no such file or directory" errors. Hot-reload doesn't work for compiled
  # languages anyway — developers must rebuild the container.
  local current_env="${ENV:-dev}"
  local is_compiled=false
  case "$template_type" in
    go|grpc|gin|echo|fiber|spring*|quarkus|actix*|rocket|axum|oatpp|zap|aspnet|ktor|vapor)
      is_compiled=true
      ;;
  esac

  if [[ "$current_env" == "dev" || "$current_env" == "development" || "$current_env" == "local" ]] && [[ "$is_compiled" != "true" ]]; then
    cat <<EOF
    volumes:
      - ./services/${service_name}:/app
EOF

    # Add language-specific volume exclusions based on template type
    case "$template_type" in
      *js|*ts|node*|express*|nest*|fastify*|hono*|bullmq*|bun|deno)
        echo "      - /app/node_modules"
        echo "      - /app/dist"
        ;;
      py*|fastapi|django*|flask|celery)
        echo "      - /app/.venv"
        echo "      - /app/__pycache__"
        ;;
      php*|laravel)
        echo "      - /app/vendor"
        ;;
      ruby*|rails|sinatra)
        echo "      - /app/vendor"
        ;;
    esac
  fi

  # Add resource limits if specified (smart defaults)
  local memory_var="CS_${index}_MEMORY"
  local cpu_var="CS_${index}_CPU"
  local replicas_var="CS_${index}_REPLICAS"

  # Only add deploy section if any resource constraints are specified
  if [[ -n "${!memory_var:-}" ]] || [[ -n "${!cpu_var:-}" ]] || [[ -n "${!replicas_var:-}" ]]; then
    cat <<EOF
    deploy:
      resources:
        limits:
          memory: \${${memory_var}:-512M}
          cpus: '\${${cpu_var}:-0.5}'
      replicas: \${${replicas_var}:-1}
EOF
  fi

  # Add dependencies
  cat <<EOF
    depends_on:
      - postgres
      - redis
EOF

  [[ "${HASURA_ENABLED:-false}" == "true" ]] && echo "      - hasura"

  # Add healthcheck if port is exposed
  # Bug #33 fix: Match actual template names (gin, echo, fiber, not just *go*).
  # Use wget (not curl) for Alpine images, 127.0.0.1 (not localhost) to avoid IPv6.
  # Python slim images don't have curl either — use python urllib.
  if [[ -n "$service_port" && "$service_port" != "0" ]]; then
    case "$template_type" in
      go|grpc|gin|echo|fiber|actix*|rocket|axum|oatpp|zap)
        # Go/Rust Alpine images: use wget (always available), 127.0.0.1 (no IPv6)
        cat <<EOF
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "-O", "/dev/null", "http://127.0.0.1:${service_port}/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
EOF
        ;;
      py*|fastapi|django*|flask|celery)
        # Python slim images: no curl/wget, use python stdlib
        cat <<EOF
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:${service_port}/health')"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
EOF
        ;;
      *js|*ts|node*|express*|nest*|fastify*|hono*|bun|deno)
        # Node.js images: wget available, use 127.0.0.1 to avoid IPv6
        cat <<EOF
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "-O", "/dev/null", "http://127.0.0.1:${service_port}/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
EOF
        ;;
      *)
        # Fallback: wget with 127.0.0.1 (most portable)
        cat <<EOF
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "-O", "/dev/null", "http://127.0.0.1:${service_port}/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
EOF
        ;;
    esac
  fi
}

# Generate all template-based custom services
generate_template_custom_services() {
  local services_found=false

  # Check for CS_ variables (format: service_name:template_type:port)
  # CRITICAL (Bug #17 fix): Use seq instead of {1..10} for Bash 3.2 compatibility
  # Note: nself only supports CS_1 through CS_10 (per documentation)
  local i
  for i in $(seq 1 10); do
    local cs_var="CS_${i}"
    local cs_value="${!cs_var:-}"

    [[ -z "$cs_value" ]] && continue

    # Parse CS_ format: service_name:template_type:port
    IFS=':' read -r service_name template_type port <<< "$cs_value"

    # Skip if essential fields are missing
    [[ -z "$service_name" || -z "$template_type" ]] && continue

    if [[ "$services_found" == "false" ]]; then
      echo ""
      echo "  # ============================================"
      echo "  # Custom Services (from templates)"
      echo "  # ============================================"
      services_found=true
    fi

    generate_template_based_service "$i" "$service_name" "$template_type" "${port:-8000}"
  done
}

# Export functions
export -f generate_template_based_service
export -f generate_template_custom_services