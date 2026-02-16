#!/usr/bin/env bash
set -euo pipefail


# Service-specific fixes

LAST_FIX_DESCRIPTION=""

get_last_fix_description() {
  echo "$LAST_FIX_DESCRIPTION"
}

fix_redis_connection() {
  # Start Redis if not running
  if ! docker ps -q -f name=${PROJECT_NAME:-nself}_redis | grep -q .; then
    docker compose up -d redis >/dev/null 2>&1
    sleep 3
    LAST_FIX_DESCRIPTION="Started Redis service"
    return 0
  fi

  # Restart Redis
  docker compose restart redis >/dev/null 2>&1
  sleep 3
  LAST_FIX_DESCRIPTION="Restarted Redis service"
  return 0
}

fix_elasticsearch_connection() {
  # Start Elasticsearch/OpenSearch if not running
  if ! docker ps -q -f name=${PROJECT_NAME:-nself}_elasticsearch | grep -q .; then
    if ! docker ps -q -f name=${PROJECT_NAME:-nself}_opensearch | grep -q .; then
      docker compose up -d elasticsearch opensearch 2>/dev/null
      sleep 10 # ES takes longer to start
      LAST_FIX_DESCRIPTION="Started Elasticsearch/OpenSearch"
      return 0
    fi
  fi

  # Restart the service
  docker compose restart elasticsearch opensearch 2>/dev/null
  sleep 10
  LAST_FIX_DESCRIPTION="Restarted Elasticsearch/OpenSearch"
  return 0
}

fix_port_in_use() {
  local service_name="${1:-}"

  # Extract port from error
  local port=$(docker compose logs "$service_name" 2>&1 | grep -oE "bind.*:([0-9]+)" | grep -oE "[0-9]+" | head -1)

  if [[ -n "$port" ]]; then
    # Find next available port
    local new_port=$((port + 1))
    while lsof -i ":$new_port" >/dev/null 2>&1; do
      new_port=$((new_port + 1))
    done

    # Update port in .env.local
    local service_upper=$(echo "$service_name" | tr '[:lower:]' '[:upper:]' | tr '-' '_')
    sed -i '' "s/${service_upper}_PORT=.*/${service_upper}_PORT=$new_port/" .env.local 2>/dev/null

    # Rebuild and restart
    nself build --force >/dev/null 2>&1
    docker compose up -d "$service_name" >/dev/null 2>&1

    LAST_FIX_DESCRIPTION="Changed port from $port to $new_port"
    return 0
  fi

  # Fallback: stop conflicting service
  docker compose stop "$service_name" >/dev/null 2>&1
  docker compose rm -f "$service_name" >/dev/null 2>&1
  docker compose up -d "$service_name" >/dev/null 2>&1

  LAST_FIX_DESCRIPTION="Recreated service to resolve port conflict"
  return 0
}
