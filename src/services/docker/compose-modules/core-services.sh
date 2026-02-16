#!/usr/bin/env bash
set -uo pipefail
# core-services.sh - Generate core service definitions for docker-compose
# This module handles PostgreSQL, Hasura, Auth, MinIO, and Redis services

# Sanitize database name (replace hyphens with underscores for PostgreSQL compatibility)
sanitize_db_name() {
  echo "$1" | tr '-' '_'
}

# Generate PostgreSQL service configuration
generate_postgres_service() {
  local enabled="${POSTGRES_ENABLED:-true}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # PostgreSQL Database
  # SECURITY: Bound to localhost only - never expose database to public internet
  postgres:
    image: postgres:${POSTGRES_VERSION:-16-alpine}
    container_name: \${PROJECT_NAME}_postgres
    restart: unless-stopped
    networks:
      - \${DOCKER_NETWORK}
    environment:
      POSTGRES_USER: \${POSTGRES_USER:-postgres}
      POSTGRES_PASSWORD: \${POSTGRES_PASSWORD}
      POSTGRES_DB: \${POSTGRES_DB:-\${PROJECT_NAME}}
      POSTGRES_HOST_AUTH_METHOD: scram-sha-256
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./postgres/init:/docker-entrypoint-initdb.d:ro
EOF

  # Conditional port exposure based on environment
  if [[ "${POSTGRES_EXPOSE_PORT:-auto}" == "true" ]] || \
     [[ "${POSTGRES_EXPOSE_PORT:-auto}" == "auto" && "${ENV:-dev}" == "dev" ]]; then
    cat <<'EOF'
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:${POSTGRES_PORT:-5432}:5432"
EOF
  else
    cat <<'EOF'
    expose:
      - "5432"  # Internal Docker network only
EOF
  fi

  cat <<EOF
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \${POSTGRES_USER:-postgres}"]
      interval: 10s
      timeout: 5s
      retries: 5
EOF

  # Add resource limits if specified
  if [[ -n "${POSTGRES_MEMORY:-}" ]] || [[ -n "${POSTGRES_CPU:-}" ]]; then
    cat <<EOF
    deploy:
      resources:
        limits:
          memory: \${POSTGRES_MEMORY:-2G}
          cpus: '\${POSTGRES_CPU:-1.0}'
EOF
  fi

  echo ""  # Close the service block
}

# Generate Hasura GraphQL Engine service
generate_hasura_service() {
  local enabled="${HASURA_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Hasura GraphQL Engine
  hasura:
    image: hasura/graphql-engine:${HASURA_VERSION:-v2.36.0}
    container_name: \${PROJECT_NAME}_hasura
    restart: unless-stopped
    user: "1001:1001"
    networks:
      - \${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      HASURA_GRAPHQL_DATABASE_URL: postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_GRAPHQL_ADMIN_SECRET}
      HASURA_GRAPHQL_ENABLE_CONSOLE: "true"
      HASURA_GRAPHQL_DEV_MODE: \${HASURA_DEV_MODE:-false}
      HASURA_GRAPHQL_ENABLE_TELEMETRY: "false"
      HASURA_GRAPHQL_CORS_DOMAIN: "\${HASURA_GRAPHQL_CORS_DOMAIN}"
      HASURA_GRAPHQL_LOG_LEVEL: \${HASURA_LOG_LEVEL:-info}
EOF

  # Add auth configuration based on auth mode
  if [[ "${AUTH_ENABLED:-false}" == "true" ]]; then
    # Use JWT mode (nHost auth's native mode)
    # IMPORTANT: nHost auth does NOT have a /webhook endpoint
    # It uses JWT tokens, so Hasura must validate JWTs directly
    cat <<EOF
      HASURA_GRAPHQL_JWT_SECRET: \${HASURA_GRAPHQL_JWT_SECRET}
      HASURA_GRAPHQL_UNAUTHORIZED_ROLE: public
EOF
  else
    # No auth - just use admin secret with public access
    cat <<EOF
      HASURA_GRAPHQL_UNAUTHORIZED_ROLE: public
EOF
  fi

  cat <<EOF
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:\${HASURA_PORT:-8080}:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF

  # Add resource limits if specified
  if [[ -n "${HASURA_MEMORY:-}" ]] || [[ -n "${HASURA_CPU:-}" ]]; then
    cat <<EOF
    deploy:
      resources:
        limits:
          memory: \${HASURA_MEMORY:-1G}
          cpus: '\${HASURA_CPU:-0.5}'
EOF
  fi
}

# Generate Auth service configuration
generate_auth_service() {
  local enabled="${AUTH_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  # Check if we should use fallback auth service
  local use_fallback="${AUTH_USE_FALLBACK:-false}"
  local auth_image="${AUTH_IMAGE:-nhost/hasura-auth:latest}"

  # If fallback is enabled or ENV is demo, use the fallback service
  if [[ "$use_fallback" == "true" ]] || [[ "${ENV:-}" == "demo" ]] || [[ "${DEMO_CONTENT:-false}" == "true" ]]; then
    # Generate fallback auth service
    cat <<EOF
  # Hasura Auth Service (Fallback)
  auth:
    build:
      context: ./fallback-services
      dockerfile: Dockerfile.auth
    container_name: \${PROJECT_NAME}_auth
    restart: unless-stopped
    networks:
      - \${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      AUTH_PORT: 4000
      NODE_ENV: \${ENV:-development}
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:\${AUTH_PORT:-4000}:4000"
EOF
  else
    # Use original nhost/hasura-auth with fixes
    cat <<EOF

  # Hasura Auth Service
  auth:
    image: ${auth_image}
    container_name: \${PROJECT_NAME}_auth
    restart: unless-stopped
    networks:
      - \${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
EOF

  # Note: Don't add hasura dependency to avoid circular dependency
  # Hasura depends on auth webhook, auth doesn't need hasura

  cat <<EOF
    environment:
      AUTH_HOST: "0.0.0.0"
      AUTH_PORT: "4000"
      AUTH_LOG_LEVEL: \${AUTH_LOG_LEVEL:-info}
      DATABASE_URL: postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      AUTH_DATABASE_URL: postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      HASURA_GRAPHQL_DATABASE_URL: postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      POSTGRES_HOST: postgres
      POSTGRES_PORT: 5432
      PGHOST: postgres
      PGPORT: 5432
      PGUSER: \${POSTGRES_USER:-postgres}
      PGPASSWORD: \${POSTGRES_PASSWORD:-postgres}
      PGDATABASE: \${POSTGRES_DB:-\${PROJECT_NAME}}
      POSTGRES_USER: \${POSTGRES_USER:-postgres}
      POSTGRES_PASSWORD: \${POSTGRES_PASSWORD:-postgres}
      POSTGRES_DATABASE: \${POSTGRES_DB:-\${PROJECT_NAME}}
      AUTH_SERVER_URL: \${AUTH_SERVER_URL:-http://localhost:4000}
      AUTH_CLIENT_URL: \${AUTH_CLIENT_URL:-http://localhost:3000}
      AUTH_JWT_SECRET: \${AUTH_JWT_SECRET}
      AUTH_JWT_TYPE: \${AUTH_JWT_TYPE:-HS256}
      AUTH_JWT_KEY: \${AUTH_JWT_KEY:-\${AUTH_JWT_SECRET}}
      HASURA_GRAPHQL_JWT_SECRET: \${HASURA_GRAPHQL_JWT_SECRET}
      HASURA_GRAPHQL_GRAPHQL_URL: \${HASURA_GRAPHQL_GRAPHQL_URL:-http://hasura:8080/v1/graphql}
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_GRAPHQL_ADMIN_SECRET}
      AUTH_ACCESS_TOKEN_EXPIRES_IN: \${AUTH_ACCESS_TOKEN_EXPIRES_IN:-900}
      AUTH_REFRESH_TOKEN_EXPIRES_IN: \${AUTH_REFRESH_TOKEN_EXPIRES_IN:-2592000}
      AUTH_SMTP_HOST: \${AUTH_SMTP_HOST:-mailpit}
      AUTH_SMTP_PORT: \${AUTH_SMTP_PORT:-1025}
      AUTH_SMTP_USER: \${AUTH_SMTP_USER}
      AUTH_SMTP_PASS: \${AUTH_SMTP_PASS}
      AUTH_SMTP_SECURE: \${AUTH_SMTP_SECURE:-false}
      AUTH_SMTP_SENDER: \${AUTH_SMTP_SENDER:-noreply@\${BASE_DOMAIN:-localhost}}
      AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED: \${AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED:-false}
EOF

  # Add OAuth providers if configured
  if [[ -n "${AUTH_PROVIDER_GOOGLE_CLIENT_ID:-}" ]]; then
    cat <<EOF
      AUTH_PROVIDER_GOOGLE_ENABLED: "true"
      AUTH_PROVIDER_GOOGLE_CLIENT_ID: \${AUTH_PROVIDER_GOOGLE_CLIENT_ID}
      AUTH_PROVIDER_GOOGLE_CLIENT_SECRET: \${AUTH_PROVIDER_GOOGLE_CLIENT_SECRET}
EOF
  fi

  if [[ -n "${AUTH_PROVIDER_GITHUB_CLIENT_ID:-}" ]]; then
    cat <<EOF
      AUTH_PROVIDER_GITHUB_ENABLED: "true"
      AUTH_PROVIDER_GITHUB_CLIENT_ID: \${AUTH_PROVIDER_GITHUB_CLIENT_ID}
      AUTH_PROVIDER_GITHUB_CLIENT_SECRET: \${AUTH_PROVIDER_GITHUB_CLIENT_SECRET}
EOF
  fi

  cat <<EOF
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:\${AUTH_PORT:-4000}:4000"
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:4000/healthz"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF

  # Add resource limits if specified
  if [[ -n "${AUTH_MEMORY:-}" ]] || [[ -n "${AUTH_CPU:-}" ]]; then
    cat <<EOF
    deploy:
      resources:
        limits:
          memory: \${AUTH_MEMORY:-512M}
          cpus: '\${AUTH_CPU:-0.5}'
EOF
  fi
  fi  # Close the use_fallback if statement
}

# Generate MinIO service configuration
# SECURITY: MinIO binds to 127.0.0.1 only - access via nginx proxy
generate_minio_service() {
  local enabled="${MINIO_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # MinIO Volume Permission Fix - Init Container
  # Fixes permissions on MinIO data volume for non-root user (1000:1000)
  # This runs once before MinIO starts to ensure the container can write to the volume
  minio-init:
    image: busybox:latest
    container_name: \${PROJECT_NAME}_minio_init
    user: root
    networks:
      - \${DOCKER_NETWORK}
    volumes:
      - minio_data:/data
    command: >
      sh -c "
        echo '→ Fixing MinIO volume permissions...';
        chown -R 1000:1000 /data;
        chmod -R 755 /data;
        echo '✓ MinIO volume permissions fixed';
      "
    labels:
      - "nself.type=init-container"
      - "nself.service=minio"

  # MinIO Object Storage
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  minio:
    image: minio/minio:${MINIO_VERSION:-latest}
    container_name: \${PROJECT_NAME}_minio
    restart: unless-stopped
    user: "1000:1000"
    networks:
      - \${DOCKER_NETWORK}
    depends_on:
      minio-init:
        condition: service_completed_successfully
    environment:
      MINIO_ROOT_USER: \${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: \${MINIO_ROOT_PASSWORD}
      MINIO_DEFAULT_BUCKETS: \${MINIO_DEFAULT_BUCKETS:-uploads}
      MINIO_REGION: \${MINIO_REGION:-us-east-1}
    command: server /data --console-address ":9001"
    volumes:
      - minio_data:/data
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${MINIO_PORT:-9000}:9000"
      - "127.0.0.1:\${MINIO_CONSOLE_PORT:-9001}:9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s
      timeout: 20s
      retries: 3
EOF

  # Add resource limits if specified
  if [[ -n "${MINIO_MEMORY:-}" ]] || [[ -n "${MINIO_CPU:-}" ]]; then
    cat <<EOF
    deploy:
      resources:
        limits:
          memory: \${MINIO_MEMORY:-1G}
          cpus: '\${MINIO_CPU:-0.5}'
EOF
  fi

  # MinIO client runs once to initialize buckets, then removes itself
  # Uses Docker profiles to exclude from normal operations
  cat <<'MINIO_CLIENT_EOF'

  # MinIO Client - One-time bucket initialization (run with --profile init-containers)
  minio-client:
    image: minio/mc:latest
    container_name: ${PROJECT_NAME}_minio_client
    restart: "no"
    profiles:
      - init-containers
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      minio:
        condition: service_healthy
    labels:
      - "nself.type=init-container"
      - "nself.auto-remove=true"
    entrypoint: >
      /bin/sh -c "
      set -e;
      echo '→ Initializing MinIO buckets...';
      /usr/bin/mc config host add myminio http://minio:9000 ${MINIO_ROOT_USER} ${MINIO_ROOT_PASSWORD};
      for bucket in $(echo ${MINIO_DEFAULT_BUCKETS:-uploads} | tr ',' ' '); do
        /usr/bin/mc mb -p myminio/$$bucket 2>/dev/null || echo \"  Bucket $$bucket exists\";
        /usr/bin/mc anonymous set download myminio/$$bucket 2>/dev/null || true;
      done;
      echo '✓ MinIO initialization complete';
      "
MINIO_CLIENT_EOF
}

# Generate Redis service configuration
# SECURITY: Redis is configured with localhost-only binding and password auth
generate_redis_service() {
  local enabled="${REDIS_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  # Build Redis command with security options
  # --appendonly yes: Persist data to disk
  # Bug #30 fix: When no password is set, use --protected-mode no so Docker containers
  # on the bridge network (172.18.x.x) can connect. Protected-mode blocks all non-localhost
  # connections when no password is configured, which breaks all Docker services.
  local redis_cmd="redis-server --appendonly yes"

  # Add password authentication if configured (CRITICAL for security)
  if [[ -n "${REDIS_PASSWORD:-}" ]]; then
    redis_cmd="${redis_cmd} --protected-mode yes --requirepass \${REDIS_PASSWORD}"
  else
    redis_cmd="${redis_cmd} --protected-mode no"
  fi

  cat <<EOF

  # Redis Cache
  # SECURITY: Bound to 127.0.0.1 only - never expose Redis to public internet
  # Password authentication enabled when REDIS_PASSWORD is set
  redis:
    image: redis:${REDIS_VERSION:-7-alpine}
    container_name: \${PROJECT_NAME}_redis
    restart: unless-stopped
    user: "999:999"
    networks:
      - \${DOCKER_NETWORK}
    command: ${redis_cmd}
    volumes:
      - redis_data:/data
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${REDIS_PORT:-6379}:6379"
EOF

  # Use appropriate healthcheck based on password configuration
  if [[ -n "${REDIS_PASSWORD:-}" ]]; then
    cat <<EOF
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "\${REDIS_PASSWORD}", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
EOF
  else
    cat <<EOF
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
EOF
  fi

  # Add resource limits if specified
  if [[ -n "${REDIS_MEMORY:-}" ]] || [[ -n "${REDIS_CPU:-}" ]]; then
    cat <<EOF
    deploy:
      resources:
        limits:
          memory: \${REDIS_MEMORY:-512M}
          cpus: '\${REDIS_CPU:-0.5}'
EOF
  fi
}

# Export functions
export -f generate_postgres_service
export -f generate_hasura_service
export -f generate_auth_service
export -f generate_minio_service
export -f generate_redis_service