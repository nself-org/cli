#!/usr/bin/env bash
set -euo pipefail
# plugin-services.sh - Generate Docker services for installed nself plugins

# Constants
PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"

# Generate Docker service for a single plugin
generate_plugin_service() {
  local plugin_name="$1"
  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local manifest="$plugin_dir/plugin.json"

  # Skip if plugin directory or manifest doesn't exist
  [[ ! -d "$plugin_dir" ]] && return 0
  [[ ! -f "$manifest" ]] && return 0

  # Check if plugin has a Dockerfile
  local dockerfile_path=""
  if [[ -f "$plugin_dir/Dockerfile" ]]; then
    dockerfile_path="Dockerfile"
  elif [[ -f "$plugin_dir/docker/Dockerfile" ]]; then
    dockerfile_path="docker/Dockerfile"
  else
    # No Dockerfile - skip (plugin might be external or host-based)
    return 0
  fi

  # Parse plugin metadata
  local port=$(grep -A5 '"service"' "$manifest" | grep '"port"' | head -1 | sed 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/')
  local replicas=$(grep -A5 '"service"' "$manifest" | grep '"replicas"' | head -1 | sed 's/.*"replicas"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/')
  local memory=$(grep -A5 '"service"' "$manifest" | grep '"memory"' | head -1 | sed 's/.*"memory"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  local cpu=$(grep -A5 '"service"' "$manifest" | grep '"cpu"' | head -1 | sed 's/.*"cpu"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

  # Default values
  [[ -z "$port" ]] && port=3000
  [[ -z "$replicas" ]] && replicas=1
  [[ -z "$memory" ]] && memory="512M"
  [[ -z "$cpu" ]] && cpu="0.5"

  # Sanitize plugin name for Docker
  local safe_name=$(echo "$plugin_name" | tr '[:upper:]' '[:lower:]' | tr -cd '[:alnum:]_-')

  # Start service definition
  cat <<YAML

  # Plugin: ${plugin_name}
  plugin-${safe_name}:
    build:
      context: ${plugin_dir}
      dockerfile: ${dockerfile_path}
YAML

  # Only add container_name if not using replicas
  if [[ "$replicas" == "1" ]]; then
    echo "    container_name: \${PROJECT_NAME}_plugin_${safe_name}"
  fi

  cat <<YAML
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
YAML

  # Add ports if specified
  if [[ -n "$port" && "$port" != "0" ]]; then
    cat <<YAML
    ports:
      - "${port}:${port}"
YAML
  fi

  # Add environment variables
  cat <<YAML
    environment:
      - ENV=\${ENV:-dev}
      - NODE_ENV=\${ENV:-dev}
      - APP_ENV=\${ENV:-dev}
      - PROJECT_NAME=\${PROJECT_NAME}
      - BASE_DOMAIN=\${BASE_DOMAIN:-localhost}
      - DOCKER_NETWORK=${DOCKER_NETWORK}
      - SERVICE_NAME=${plugin_name}
      - SERVICE_PORT=${port}
      - PORT=${port}
      - POSTGRES_HOST=postgres
      - POSTGRES_PORT=5432
      - POSTGRES_DB=\${POSTGRES_DB}
      - POSTGRES_USER=\${POSTGRES_USER}
      - POSTGRES_PASSWORD=\${POSTGRES_PASSWORD}
      - DATABASE_URL=postgres://\${POSTGRES_USER}:\${POSTGRES_PASSWORD}@postgres:5432/\${POSTGRES_DB}
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - REDIS_PASSWORD=\${REDIS_PASSWORD:-}
      - HASURA_GRAPHQL_ENDPOINT=http://hasura:8080/v1/graphql
      - HASURA_ADMIN_SECRET=\${HASURA_GRAPHQL_ADMIN_SECRET}
YAML

  # Add plugin-specific environment variables from manifest
  local env_vars=$(grep -A20 '"environment"' "$manifest" | grep -o '"[A-Z_]*"[[:space:]]*:' | tr -d '":' | tr -d ' ')
  if [[ -n "$env_vars" ]]; then
    for var in $env_vars; do
      echo "      - ${var}=\${${var}:-}"
    done
  fi

  # Add resource limits
  cat <<YAML
    deploy:
      resources:
        limits:
          memory: ${memory}
          cpus: '${cpu}'
      replicas: ${replicas}
YAML

  # Add dependencies
  cat <<YAML
    depends_on:
      - postgres
YAML

  # Add optional dependencies if enabled
  [[ "${REDIS_ENABLED:-false}" == "true" ]] && echo "      - redis"
  [[ "${HASURA_ENABLED:-true}" == "true" ]] && echo "      - hasura"

  # Add healthcheck if port is exposed
  if [[ -n "$port" && "$port" != "0" ]]; then
    cat <<YAML
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:${port}/health"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
YAML
  fi
}

# Generate all plugin services
generate_all_plugin_services() {
  local plugins_found=false

  # Check if plugin directory exists
  [[ ! -d "$PLUGIN_DIR" ]] && return 0

  # Iterate through installed plugins
  for plugin_path in "$PLUGIN_DIR"/*/; do
    # Skip if not a directory or no plugin.json
    [[ ! -d "$plugin_path" ]] && continue
    [[ ! -f "$plugin_path/plugin.json" ]] && continue

    local plugin_name=$(basename "$plugin_path")

    # Check if plugin has Dockerfile
    if [[ -f "$plugin_path/Dockerfile" ]] || [[ -f "$plugin_path/docker/Dockerfile" ]]; then
      if [[ "$plugins_found" == "false" ]]; then
        echo ""
        echo "  # ============================================"
        echo "  # Plugin Services (with Docker support)"
        echo "  # ============================================"
        plugins_found=true
      fi

      generate_plugin_service "$plugin_name"
    fi
  done

  # If plugins were found, add note about non-Dockerized plugins
  if [[ "$plugins_found" == "true" ]]; then
    echo ""
    echo "  # Note: Plugins without Dockerfiles run externally (host or remote)"
  fi
}

# Export functions
export -f generate_plugin_service
export -f generate_all_plugin_services
