#!/usr/bin/env bash
set -euo pipefail
# utility-services.sh - Generate utility service definitions
# This module handles Mailpit, Adminer, BullMQ Dashboard, and other utility services

# Generate Mailpit email testing service
generate_mailpit_service() {
  local enabled="${MAILPIT_ENABLED:-true}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Mailpit - Email Testing Tool
  mailpit:
    image: axllent/mailpit:${MAILPIT_VERSION:-latest}
    container_name: \${PROJECT_NAME}_mailpit
    restart: unless-stopped
    user: "1000:1000"
    networks:
      - ${DOCKER_NETWORK}
    environment:
      MP_UI_BIND_ADDR: 0.0.0.0:8025
      MP_SMTP_BIND_ADDR: 0.0.0.0:1025
EOF

  # Environment-conditional security settings
  if [[ "${ENV:-dev}" == "dev" ]]; then
    cat <<'EOF'
      MP_SMTP_AUTH_ACCEPT_ANY: 1
      MP_SMTP_AUTH_ALLOW_INSECURE: 1
EOF
  else
    cat <<'EOF'
      MP_SMTP_AUTH_ACCEPT_ANY: ${MAILPIT_ACCEPT_ANY_AUTH:-0}
      MP_SMTP_AUTH_ALLOW_INSECURE: 0
EOF
  fi

  cat <<'EOF'
      MP_MAX_MESSAGES: ${MAILPIT_MAX_MESSAGES:-500}
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:${MAILPIT_SMTP_PORT:-1025}:1025"
      - "127.0.0.1:${MAILPIT_UI_PORT:-8025}:8025"
    healthcheck:
      test: ["CMD", "nc", "-z", "localhost", "8025"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF
}



# Generate nself Admin dashboard
generate_nself_admin_service() {
  local enabled="${NSELF_ADMIN_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  # Admin-dev mode: skip Docker container, use local dev server instead
  # Nginx will route admin.domain to host.docker.internal:NSELF_ADMIN_DEV_PORT
  if [[ "${NSELF_ADMIN_DEV:-false}" == "true" ]]; then
    return 0
  fi

  # Check if using local development paths
  local admin_local_path="${NSELF_ADMIN_LOCAL_PATH:-}"
  local nself_cli_local_path="${NSELF_CLI_LOCAL_PATH:-}"

  cat <<EOF

  # nself Admin - Project Management Dashboard
  nself-admin:
    user: "1000:1000"
EOF

  # Use build context if local path is set, otherwise use Docker image
  if [[ -n "$admin_local_path" ]] && [[ -d "$admin_local_path" ]]; then
    cat <<EOF
    build:
      context: ${admin_local_path}
      dockerfile: Dockerfile
EOF
  else
    cat <<EOF
    image: nself/nself-admin:\${NSELF_ADMIN_VERSION:-latest}
EOF
  fi

  cat <<EOF
    container_name: \${PROJECT_NAME}_admin
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
      hasura:
        condition: service_healthy
    environment:
      NODE_ENV: production
      PROJECT_PATH: /workspace
      NSELF_PROJECT_PATH: /workspace
      NSELF_CLI_PATH: /usr/local/bin/nself
      PROJECT_NAME: \${PROJECT_NAME}
      BASE_DOMAIN: \${BASE_DOMAIN}
      ENV: \${ENV}
      DATABASE_URL: postgres://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}
      HASURA_GRAPHQL_ENDPOINT: http://hasura:8080/v1/graphql
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_GRAPHQL_ADMIN_SECRET}
      ADMIN_SECRET_KEY: \${ADMIN_SECRET_KEY}
      ADMIN_PASSWORD_HASH: \${ADMIN_PASSWORD_HASH}
      DOCKER_HOST: unix:///var/run/docker.sock
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:\${NSELF_ADMIN_PORT:-3021}:3021"
    volumes:
      - ./:/workspace:rw
      - nself_admin_data:/app/data
      - /var/run/docker.sock:/var/run/docker.sock:ro
EOF

  # Add source mount for hot reload in dev with local path
  if [[ -n "$admin_local_path" ]] && [[ -d "$admin_local_path" ]] && [[ "${ENV:-}" == "dev" ]]; then
    cat <<EOF
      - ${admin_local_path}/src:/app/src:ro
EOF
  fi

  # Mount local nself CLI for development (allows using local source instead of installed version)
  if [[ -n "$nself_cli_local_path" ]] && [[ -d "$nself_cli_local_path" ]]; then
    cat <<EOF
      - ${nself_cli_local_path}:/opt/nself:ro
EOF
  fi

  cat <<EOF
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3021/api/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 30s
EOF
}

# Generate Functions service (serverless functions runtime)
generate_functions_service() {
  local enabled="${FUNCTIONS_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  # Check if we should use fallback functions service
  local use_fallback="${FUNCTIONS_USE_FALLBACK:-false}"

  # If fallback is enabled or ENV is demo, use the fallback service
  if [[ "$use_fallback" == "true" ]] || [[ "${ENV:-}" == "demo" ]] || [[ "${DEMO_CONTENT:-false}" == "true" ]]; then
    # Generate fallback functions service
    cat <<EOF

  # Functions - Serverless Functions Runtime (Fallback)
  functions:
    build:
      context: ./fallback-services
      dockerfile: Dockerfile.functions
    container_name: \${PROJECT_NAME}_functions
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
      hasura:
        condition: service_healthy
    environment:
      PORT: 3000
      NODE_ENV: \${ENV:-development}
      DATABASE_URL: postgres://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}
      HASURA_GRAPHQL_ENDPOINT: http://hasura:8080/v1/graphql
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_GRAPHQL_ADMIN_SECRET}
    volumes:
      - ./functions:/opt/project
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:\${FUNCTIONS_PORT:-3008}:3000"
EOF
  else
    # Use original nhost/functions
    cat <<EOF

  # Functions - Serverless Functions Runtime
  functions:
    image: nhost/functions:\${FUNCTIONS_VERSION:-latest}
    container_name: \${PROJECT_NAME}_functions
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
      hasura:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}
      HASURA_GRAPHQL_ENDPOINT: http://hasura:8080/v1/graphql
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_GRAPHQL_ADMIN_SECRET}
      NODE_ENV: \${ENV:-development}
      PORT: 3008
    volumes:
      - ./functions:/opt/project
    ports:
      # SECURITY: Bind to localhost only - access via nginx reverse proxy
      - "127.0.0.1:\${FUNCTIONS_PORT:-3008}:3008"
    healthcheck:
      test: ["CMD-SHELL", "node -e 'require(\"http\").get(\"http://localhost:3000/healthz\", (r) => process.exit(r.statusCode === 200 ? 0 : 1)).on(\"error\", () => process.exit(1))' || curl -f http://localhost:3000/healthz || wget -q --spider http://localhost:3000/healthz"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 40s
EOF
  fi
}




# Generate backup service
generate_backup_service() {
  local enabled="${BACKUP_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  cat <<EOF

  # Backup Service - Automated Database Backups
  backup:
    image: postgres:${POSTGRES_VERSION:-16-alpine}
    container_name: \${PROJECT_NAME}_backup
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      PGHOST: postgres
      PGUSER: \${POSTGRES_USER:-postgres}
      PGPASSWORD: \${POSTGRES_PASSWORD}
      PGDATABASE: \${POSTGRES_DB:-\${PROJECT_NAME}}
      BACKUP_SCHEDULE: \${BACKUP_SCHEDULE:-0 2 * * *}
      BACKUP_RETENTION_DAYS: \${BACKUP_RETENTION_DAYS:-7}
    volumes:
      - ./backups:/backups
      - ./src/scripts/backup.sh:/usr/local/bin/backup.sh:ro
    entrypoint: >
      sh -c "
        apk add --no-cache dcron &&
        echo '\${BACKUP_SCHEDULE} /usr/local/bin/backup.sh' | crontab - &&
        crond -f -l 2
      "
EOF
}

# Generate MLflow service
# SECURITY: MLflow binds to 127.0.0.1 only - access via nginx proxy
generate_mlflow_service() {
  local enabled="${MLFLOW_ENABLED:-false}"
  [[ "$enabled" != "true" ]] && return 0

  # Ensure MLflow directory and Dockerfile exist
  mkdir -p mlflow
  cat > mlflow/Dockerfile <<'DOCKERFILE'
FROM ghcr.io/mlflow/mlflow:latest

# Install PostgreSQL adapter
RUN pip install --no-cache-dir psycopg2-binary

# Ensure artifacts directory exists
RUN mkdir -p /mlflow/artifacts

# Set working directory
WORKDIR /mlflow
DOCKERFILE

  cat <<EOF

  # MLflow - Machine Learning Lifecycle Platform
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  mlflow:
    build:
      context: ./mlflow
      dockerfile: Dockerfile
    container_name: \${PROJECT_NAME}_mlflow
    restart: unless-stopped
    user: "1000:1000"
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      MLFLOW_BACKEND_STORE_URI: postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD}@postgres:5432/mlflow
      MLFLOW_DEFAULT_ARTIFACT_ROOT: /mlflow/artifacts
    command: [
      "mlflow", "server",
      "--backend-store-uri", "postgresql://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD}@postgres:5432/mlflow",
      "--default-artifact-root", "/mlflow/artifacts",
      "--host", "0.0.0.0",
      "--port", "\${MLFLOW_PORT:-5005}",
      "--serve-artifacts"
    ]
    volumes:
      - mlflow_data:/mlflow/artifacts
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${MLFLOW_PORT:-5005}:\${MLFLOW_PORT:-5005}"
    healthcheck:
      test: ["CMD-SHELL", "python -c 'import urllib.request; urllib.request.urlopen(\"http://localhost:\${MLFLOW_PORT:-5005}/health\")' || wget --spider -q http://localhost:\${MLFLOW_PORT:-5005}/health || curl -f http://localhost:\${MLFLOW_PORT:-5005}/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
EOF
}

# Generate search services (MeiliSearch or Typesense based on SEARCH_PROVIDER)
generate_search_services() {
  local search_enabled="${SEARCH_ENABLED:-false}"
  local search_provider="${SEARCH_PROVIDER:-meilisearch}"

  # Legacy: Support old MEILISEARCH_ENABLED for backward compatibility
  if [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]]; then
    search_enabled="true"
    search_provider="meilisearch"
  fi

  # Legacy: Support old TYPESENSE_ENABLED for backward compatibility
  if [[ "${TYPESENSE_ENABLED:-false}" == "true" ]]; then
    search_enabled="true"
    search_provider="typesense"
  fi

  # Return if search is not enabled
  [[ "$search_enabled" != "true" ]] && return 0

  # Generate service based on provider
  case "$search_provider" in
    meilisearch)
      generate_meilisearch_service
      ;;
    typesense)
      generate_typesense_service
      ;;
    *)
      # Default to meilisearch if unknown provider
      generate_meilisearch_service
      ;;
  esac
}

# Generate MeiliSearch service
# SECURITY: MeiliSearch binds to 127.0.0.1 only - access via nginx proxy
generate_meilisearch_service() {
  cat <<EOF

  # MeiliSearch Volume Permission Fix - Init Container
  # Fixes permissions on MeiliSearch data volume for non-root user (1000:1000)
  # This runs once before MeiliSearch starts to ensure the container can write to the volume
  meilisearch-init:
    image: busybox:latest
    container_name: \${PROJECT_NAME}_meilisearch_init
    restart: "no"
    user: root
    networks:
      - ${DOCKER_NETWORK}
    volumes:
      - meilisearch_data:/meili_data
    command: >
      sh -c "
        echo '→ Fixing MeiliSearch volume permissions...';
        chown -R 1000:1000 /meili_data;
        chmod -R 755 /meili_data;
        echo '✓ MeiliSearch volume permissions fixed';
      "
    labels:
      - "nself.type=init-container"
      - "nself.service=meilisearch"
      - "nself.auto-remove=true"

  # MeiliSearch - Lightning Fast Search
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  meilisearch:
    image: getmeili/meilisearch:\${MEILISEARCH_VERSION:-v1.5}
    container_name: \${PROJECT_NAME}_meilisearch
    restart: unless-stopped
    user: "1000:1000"
    networks:
      - ${DOCKER_NETWORK}
    depends_on:
      meilisearch-init:
        condition: service_completed_successfully
    environment:
      MEILI_MASTER_KEY: \${SEARCH_API_KEY:-\${MEILISEARCH_MASTER_KEY}}
      MEILI_ENV: \${MEILI_ENV:-development}
      MEILI_HTTP_ADDR: 0.0.0.0:7700
      MEILI_NO_ANALYTICS: true
    volumes:
      - meilisearch_data:/meili_data
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${SEARCH_PORT:-\${MEILISEARCH_PORT:-7700}}:7700"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:7700/health"]
      interval: 30s
      timeout: 10s
      retries: 5
EOF
}

# Generate Typesense service
# SECURITY: Typesense binds to 127.0.0.1 only - access via nginx proxy
generate_typesense_service() {
  cat <<EOF

  # Typesense - Fast, Typo-Tolerant Search Engine
  # SECURITY: Bound to localhost only - access via nginx reverse proxy
  typesense:
    image: typesense/typesense:\${TYPESENSE_VERSION:-27.1}
    container_name: \${PROJECT_NAME}_typesense
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    environment:
      TYPESENSE_API_KEY: \${SEARCH_API_KEY:-\${TYPESENSE_API_KEY}}
      TYPESENSE_DATA_DIR: /data
      TYPESENSE_ENABLE_CORS: \${TYPESENSE_ENABLE_CORS:-true}
      TYPESENSE_LOG_LEVEL: \${TYPESENSE_LOG_LEVEL:-info}
    command: '--data-dir /data --api-key=\${SEARCH_API_KEY:-\${TYPESENSE_API_KEY}} --enable-cors'
    volumes:
      - typesense_data:/data
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${SEARCH_PORT:-\${TYPESENSE_PORT:-8108}}:8108"
    healthcheck:
      test: ["CMD", "curl", "-f", "-H", "X-TYPESENSE-API-KEY: \${SEARCH_API_KEY:-\${TYPESENSE_API_KEY}}", "http://localhost:8108/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 10s
EOF
}

# Main function to generate all utility services in display order
generate_utility_services() {
  generate_nself_admin_service
  generate_minio_service
  generate_redis_service
  generate_functions_service
  generate_mailpit_service
  generate_search_services
  generate_mlflow_service
}

# Export functions
export -f generate_mailpit_service
export -f generate_nself_admin_service
export -f generate_functions_service
export -f generate_mlflow_service
export -f generate_search_services
export -f generate_meilisearch_service
export -f generate_typesense_service
export -f generate_utility_services