#!/usr/bin/env bash

# docker-compose.sh - Docker Compose generation for build

# Source platform compatibility functions
source "$(dirname "${BASH_SOURCE[0]}")/../utils/platform-compat.sh" 2>/dev/null || true

set -euo pipefail


# Source computed env generator (Bug #12 fix)
source "$(dirname "${BASH_SOURCE[0]}")/generate-computed-env.sh" 2>/dev/null || true

# Generate docker-compose.yml
generate_docker_compose() {
  # Determine the correct path to compose-generate.sh
  local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local compose_script="${script_dir}/../../services/docker/compose-generate.sh"

  # Fallback if first path doesn't work
  if [[ ! -f "$compose_script" ]]; then
    compose_script="${LIB_DIR}/../../services/docker/compose-generate.sh"
  fi

  if [[ -f "$compose_script" ]]; then
    # CRITICAL: Export ALL environment variables so compose-generate.sh can use them
    # compose-generate.sh doesn't load .env files (by design for environment-agnostic builds)
    # so we must export everything it needs from the parent process

    # Core project variables (MUST be set before calling compose-generate.sh)
    # Note: These may already be set by orchestrate_build, but we ensure they're exported
    [[ -z "${PROJECT_NAME:-}" ]] && export PROJECT_NAME="myproject"
    [[ -z "${ENV:-}" ]] && export ENV="dev"
    [[ -z "${BASE_DOMAIN:-}" ]] && export BASE_DOMAIN="localhost"
    export DOCKER_NETWORK="${PROJECT_NAME}_network"

    # Service-enabled flags
    export MINIO_ENABLED="${MINIO_ENABLED:-false}"
    export MEILISEARCH_ENABLED="${MEILISEARCH_ENABLED:-false}"
    export REDIS_ENABLED="${REDIS_ENABLED:-false}"
    export FUNCTIONS_ENABLED="${FUNCTIONS_ENABLED:-false}"
    export MAILPIT_ENABLED="${MAILPIT_ENABLED:-false}"
    export NSELF_ADMIN_ENABLED="${NSELF_ADMIN_ENABLED:-false}"
    export MLFLOW_ENABLED="${MLFLOW_ENABLED:-false}"
    export MONITORING_ENABLED="${MONITORING_ENABLED:-false}"
    export GRAFANA_ENABLED="${GRAFANA_ENABLED:-false}"
    export PROMETHEUS_ENABLED="${PROMETHEUS_ENABLED:-false}"
    export LOKI_ENABLED="${LOKI_ENABLED:-false}"
    export TEMPO_ENABLED="${TEMPO_ENABLED:-false}"
    export PROMTAIL_ENABLED="${PROMTAIL_ENABLED:-false}"
    export ALERTMANAGER_ENABLED="${ALERTMANAGER_ENABLED:-false}"
    export TYPESENSE_ENABLED="${TYPESENSE_ENABLED:-false}"
    export SONIC_ENABLED="${SONIC_ENABLED:-false}"
    export SEARCH_ENABLED="${SEARCH_ENABLED:-false}"

    # CRITICAL (Bug #19 fix): Export custom service variables (CS_1 through CS_10)
    # These are loaded from .env via set -a in orchestrate_build, but must be
    # explicitly exported here for the child bash process to inherit them.
    # Without this, compose-generate.sh cannot see CS_N values and skips
    # generating custom service blocks in docker-compose.yml.
    local _cs_i
    for _cs_i in $(seq 1 10); do
      local _cs_var="CS_${_cs_i}"
      if [[ -n "${!_cs_var:-}" ]]; then
        export "${_cs_var}=${!_cs_var}"
      fi
      # Also export parsed service details if set
      local _cs_name_var="CS_${_cs_i}_NAME"
      local _cs_tmpl_var="CS_${_cs_i}_TEMPLATE"
      local _cs_port_var="CS_${_cs_i}_PORT"
      local _cs_mem_var="CS_${_cs_i}_MEMORY"
      local _cs_cpu_var="CS_${_cs_i}_CPU"
      local _cs_rep_var="CS_${_cs_i}_REPLICAS"
      [[ -n "${!_cs_name_var:-}" ]] && export "${_cs_name_var}=${!_cs_name_var}"
      [[ -n "${!_cs_tmpl_var:-}" ]] && export "${_cs_tmpl_var}=${!_cs_tmpl_var}"
      [[ -n "${!_cs_port_var:-}" ]] && export "${_cs_port_var}=${!_cs_port_var}"
      [[ -n "${!_cs_mem_var:-}" ]] && export "${_cs_mem_var}=${!_cs_mem_var}"
      [[ -n "${!_cs_cpu_var:-}" ]] && export "${_cs_cpu_var}=${!_cs_cpu_var}"
      [[ -n "${!_cs_rep_var:-}" ]] && export "${_cs_rep_var}=${!_cs_rep_var}"
    done

    # Export frontend app variables for compose generation
    local _fa_i
    for _fa_i in $(seq 1 10); do
      local _fa_name="FRONTEND_APP_${_fa_i}_NAME"
      local _fa_port="FRONTEND_APP_${_fa_i}_PORT"
      local _fa_route="FRONTEND_APP_${_fa_i}_ROUTE"
      [[ -n "${!_fa_name:-}" ]] && export "${_fa_name}=${!_fa_name}"
      [[ -n "${!_fa_port:-}" ]] && export "${_fa_port}=${!_fa_port}"
      [[ -n "${!_fa_route:-}" ]] && export "${_fa_route}=${!_fa_route}"
    done

    bash "$compose_script"
    local compose_result=$?

    # CRITICAL (Bug #14 fix): Generate .env.computed AFTER docker-compose.yml generation
    # This ensures we have all environment variables loaded and can compute derived values
    # The .env.computed file contains computed values like DOCKER_NETWORK, DATABASE_URL, etc.
    if type generate_computed_env >/dev/null 2>&1; then
      # Load environment files so we have all variables
      set -a
      [[ -f .env ]] && source .env 2>/dev/null || true
      [[ -f .env.secrets ]] && source .env.secrets 2>/dev/null || true
      set +a

      # Generate the computed environment file
      generate_computed_env ".env.computed" 2>/dev/null || true
    fi

    return $compose_result
  else
    echo "Error: compose-generate.sh not found" >&2
    echo "  Tried: ${script_dir}/../../services/docker/compose-generate.sh" >&2
    echo "  Tried: ${LIB_DIR}/../../services/docker/compose-generate.sh" >&2
    return 1
  fi
}

# Add nginx service
add_nginx_service() {
  local file="$1"

  cat >>"$file" <<EOF

  nginx:
    image: nginx:alpine
    container_name: \${PROJECT_NAME}_nginx
    restart: unless-stopped
    ports:
      - "\${NGINX_PORT:-80}:80"
      - "\${NGINX_SSL_PORT:-443}:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./nginx/sites:/etc/nginx/sites:ro
      - ./ssl/certificates:/etc/nginx/ssl:ro
      - nginx_cache:/var/cache/nginx
    networks:
      - nself_network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF

  # Add dependencies if other services exist
  local deps=()
  [[ "${HASURA_ENABLED:-false}" == "true" ]] && deps+=("hasura")
  [[ "${AUTH_ENABLED:-false}" == "true" ]] && deps+=("auth")
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && deps+=("minio")

  if [[ ${#deps[@]} -gt 0 ]]; then
    echo "    depends_on:" >>"$file"
    for dep in "${deps[@]}"; do
      echo "      - $dep" >>"$file"
    done
  fi
}

# Add PostgreSQL service
add_postgres_service() {
  local file="$1"

  cat >>"$file" <<EOF

  postgres:
    image: postgres:\${POSTGRES_VERSION:-15}-alpine
    container_name: \${PROJECT_NAME}_postgres
    restart: unless-stopped
    ports:
      - "\${POSTGRES_PORT:-5432}:5432"
    environment:
      POSTGRES_DB: \${POSTGRES_DB:-\${PROJECT_NAME}}
      POSTGRES_USER: \${POSTGRES_USER:-postgres}
      POSTGRES_PASSWORD: \${POSTGRES_PASSWORD:-postgres}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/init.sql:ro
    networks:
      - nself_network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \${POSTGRES_USER:-postgres}"]
      interval: 10s
      timeout: 5s
      retries: 5
EOF
}

# Add Redis service
# SECURITY: Redis is configured with localhost-only binding and password auth
add_redis_service() {
  local file="$1"

  # Build Redis command with security options
  local redis_cmd="redis-server --appendonly yes --protected-mode yes"
  if [[ -n "${REDIS_PASSWORD:-}" ]]; then
    redis_cmd="${redis_cmd} --requirepass \${REDIS_PASSWORD}"
  fi

  cat >>"$file" <<EOF

  # Redis Cache - SECURITY: Bound to localhost only
  redis:
    image: redis:\${REDIS_VERSION:-7}-alpine
    container_name: \${PROJECT_NAME}_redis
    restart: unless-stopped
    command: ${redis_cmd}
    ports:
      # SECURITY: Bind to localhost only - prevents external access
      - "127.0.0.1:\${REDIS_PORT:-6379}:6379"
    volumes:
      - redis_data:/data
    networks:
      - nself_network
EOF

  # Use appropriate healthcheck based on password
  if [[ -n "${REDIS_PASSWORD:-}" ]]; then
    cat >>"$file" <<EOF
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "\${REDIS_PASSWORD}", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
EOF
  else
    cat >>"$file" <<EOF
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
EOF
  fi
}

# Add Hasura service
add_hasura_service() {
  local file="$1"

  cat >>"$file" <<EOF

  hasura:
    image: hasura/graphql-engine:\${HASURA_VERSION:-latest}
    container_name: \${PROJECT_NAME}_hasura
    restart: unless-stopped
    ports:
      - "\${HASURA_PORT:-8080}:8080"
    environment:
      HASURA_GRAPHQL_DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      HASURA_GRAPHQL_ENABLE_CONSOLE: "\${HASURA_CONSOLE:-true}"
      HASURA_GRAPHQL_DEV_MODE: "\${HASURA_DEV_MODE:-true}"
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_ADMIN_SECRET:-myadminsecret}
      HASURA_GRAPHQL_JWT_SECRET: \${HASURA_JWT_SECRET:-}
      HASURA_GRAPHQL_UNAUTHORIZED_ROLE: \${HASURA_UNAUTHORIZED_ROLE:-anonymous}
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - nself_network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF
}

# Add Auth service
add_auth_service() {
  local file="$1"

  cat >>"$file" <<EOF

  auth:
    image: nhost/hasura-auth:\${AUTH_VERSION:-latest}
    container_name: \${PROJECT_NAME}_auth
    restart: unless-stopped
    ports:
      - "\${AUTH_PORT:-4000}:4000"
    environment:
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_ADMIN_SECRET:-myadminsecret}
      HASURA_GRAPHQL_URL: http://hasura:8080/v1/graphql
      JWT_SECRET: \${AUTH_JWT_SECRET:-\${HASURA_JWT_SECRET:-}}
      AUTH_HOST: \${AUTH_HOST:-0.0.0.0}
      AUTH_PORT: 4000
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - nself_network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:4000/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF
}

# Add Storage service
add_storage_service() {
  local file="$1"

  cat >>"$file" <<EOF

  storage:
    image: nhost/hasura-storage:\${STORAGE_VERSION:-latest}
    container_name: \${PROJECT_NAME}_storage
    restart: unless-stopped
    ports:
      - "\${STORAGE_PORT:-5000}:5000"
    environment:
      DATABASE_URL: postgres://\${POSTGRES_USER:-postgres}:\${POSTGRES_PASSWORD:-postgres}@postgres:5432/\${POSTGRES_DB:-\${PROJECT_NAME}}
      HASURA_METADATA: "1"
      HASURA_GRAPHQL_URL: http://hasura:8080/v1
      HASURA_GRAPHQL_ADMIN_SECRET: \${HASURA_ADMIN_SECRET:-myadminsecret}
      S3_ENDPOINT: \${S3_ENDPOINT:-}
      S3_ACCESS_KEY: \${S3_ACCESS_KEY:-}
      S3_SECRET_KEY: \${S3_SECRET_KEY:-}
      S3_BUCKET: \${S3_BUCKET:-\${PROJECT_NAME}-storage}
      S3_REGION: \${S3_REGION:-us-east-1}
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - storage_data:/data
    networks:
      - nself_network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:5000/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF
}

# Add custom services
add_custom_services() {
  local file="$1"

  # Check for custom service definitions
  if [[ -d "services" ]]; then
    for service_file in services/*.yml services/*.yaml; do
      if [[ -f "$service_file" ]]; then
        echo "" >>"$file"
        cat "$service_file" >>"$file"
      fi
    done
  fi
}

# Add volumes section
add_volumes_section() {
  local file="$1"

  echo "" >>"$file"
  echo "volumes:" >>"$file"

  [[ "${POSTGRES_ENABLED:-true}" == "true" ]] && echo "  postgres_data:" >>"$file"
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && echo "  redis_data:" >>"$file"
  [[ "${MINIO_ENABLED:-false}" == "true" ]] && echo "  minio_data:" >>"$file"
  [[ "${NGINX_ENABLED:-true}" == "true" ]] && echo "  nginx_cache:" >>"$file"
}

# Add networks section
add_networks_section() {
  local file="$1"

  cat >>"$file" <<EOF

networks:
  nself_network:
    driver: bridge
    name: \${PROJECT_NAME}_network
EOF
}

# Validate docker-compose.yml
validate_docker_compose() {
  local compose_file="${1:-docker-compose.yml}"

  if [[ ! -f "$compose_file" ]]; then
    show_error "docker-compose.yml not found"
    return 1
  fi

  # Try to validate the compose file
  if command_exists docker-compose; then
    if docker-compose -f "$compose_file" config >/dev/null 2>&1; then
      return 0
    else
      show_error "docker-compose.yml validation failed"
      return 1
    fi
  elif docker compose version >/dev/null 2>&1; then
    if docker compose -f "$compose_file" config >/dev/null 2>&1; then
      return 0
    else
      show_error "docker-compose.yml validation failed"
      return 1
    fi
  fi

  return 0
}

# Export functions
export -f generate_docker_compose
export -f add_nginx_service
export -f add_postgres_service
export -f add_redis_service
export -f add_hasura_service
export -f add_auth_service
export -f add_storage_service
export -f add_custom_services
export -f add_volumes_section
export -f add_networks_section
export -f validate_docker_compose
