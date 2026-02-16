#!/usr/bin/env bash


# Error pattern analyzer - identifies root causes from error logs
# Returns error codes that map to specific fix strategies

analyze_error() {

set -euo pipefail

  local service_logs="$1"

  # Postgres port 5433 connection refused
  if echo "$service_logs" | grep -q "port 5433 failed: Connection refused"; then
    echo "POSTGRES_PORT_5433"
    return
  fi

  # Postgres port 5432 connection refused
  if echo "$service_logs" | grep -q "port 5432 failed: Connection refused"; then
    echo "POSTGRES_NOT_RUNNING"
    return
  fi

  # Postgres authentication failed
  if echo "$service_logs" | grep -q "password authentication failed\|FATAL.*authentication"; then
    echo "POSTGRES_AUTH_FAILED"
    return
  fi

  # Generic postgres connection error
  if echo "$service_logs" | grep -q "postgres.*connection\|connection.*postgres"; then
    echo "POSTGRES_CONNECTION"
    return
  fi

  # Database does not exist
  if echo "$service_logs" | grep -q "database.*does not exist\|FATAL.*database"; then
    echo "DATABASE_NOT_FOUND"
    return
  fi

  # Schema does not exist (auth, storage, etc)
  if echo "$service_logs" | grep -q "schema.*does not exist\|pq: schema"; then
    echo "SCHEMA_NOT_FOUND"
    return
  fi

  # Missing node modules (especially BullMQ)
  if echo "$service_logs" | grep -q "Cannot find module\|MODULE_NOT_FOUND"; then
    if echo "$service_logs" | grep -q "bullmq\|bull"; then
      echo "BULLMQ_MISSING_MODULES"
    else
      echo "MISSING_NODE_MODULES"
    fi
    return
  fi

  # BullMQ Redis connection issues
  if echo "$service_logs" | grep -q "Redis.*ECONNREFUSED\|Redis.*Connection refused"; then
    echo "BULLMQ_REDIS_CONNECTION"
    return
  fi

  # Nginx configuration errors
  if echo "$service_logs" | grep -q "nginx.*emerg\|nginx.*error"; then
    # Rate limiting directive in wrong context
    if echo "$service_logs" | grep -q "limit_req_zone.*directive is not allowed here"; then
      echo "NGINX_RATE_LIMIT_ERROR"
      return
    fi

    # Upstream host not found
    if echo "$service_logs" | grep -q "host not found in upstream"; then
      echo "NGINX_UPSTREAM_NOT_FOUND"
      return
    fi

    # SSL certificate missing
    if echo "$service_logs" | grep -q "cannot load certificate.*No such file"; then
      echo "NGINX_SSL_MISSING"
      return
    fi

    # Deprecated directives
    if echo "$service_logs" | grep -q 'the "listen ... http2" directive is deprecated'; then
      echo "NGINX_DEPRECATED_SYNTAX"
      return
    fi

    # Generic nginx config error
    echo "NGINX_CONFIG_ERROR"
    return
  fi

  # Nginx upstream host not found
  if echo "$service_logs" | grep -q "host not found in upstream"; then
    echo "NGINX_UPSTREAM_NOT_FOUND"
    return
  fi

  # OCI runtime exec failed - container has no shell
  if echo "$service_logs" | grep -q "OCI runtime exec failed\|exec: 'sh': executable file not found"; then
    echo "NO_SHELL_IN_CONTAINER"
    return
  fi

  # Missing healthcheck tools (curl, wget)
  if echo "$service_logs" | grep -q "curl.*not found\|wget.*not found\|executable file not found"; then
    echo "MISSING_HEALTHCHECK_TOOLS"
    return
  fi

  # Redis connection issues
  if echo "$service_logs" | grep -q "redis.*connection\|connection.*redis\|ECONNREFUSED.*6379"; then
    echo "REDIS_CONNECTION"
    return
  fi

  # Elasticsearch/OpenSearch connection issues
  if echo "$service_logs" | grep -q "elasticsearch.*connection\|opensearch.*connection\|port 9200"; then
    echo "ELASTICSEARCH_CONNECTION"
    return
  fi

  # Network/DNS issues
  if echo "$service_logs" | grep -q "Unknown or invalid host\|cannot resolve\|getaddrinfo ENOTFOUND"; then
    echo "NETWORK_DNS"
    return
  fi

  # Port already in use
  if echo "$service_logs" | grep -q "bind.*address already in use\|port is already allocated"; then
    echo "PORT_IN_USE"
    return
  fi

  # Out of memory
  if echo "$service_logs" | grep -q "Cannot allocate memory\|OOMKilled\|out of memory"; then
    echo "OUT_OF_MEMORY"
    return
  fi

  # Permission denied
  if echo "$service_logs" | grep -q "[Pp]ermission denied\|EACCES"; then
    echo "PERMISSION_DENIED"
    return
  fi

  # Missing environment variables
  if echo "$service_logs" | grep -q "environment variable.*not set\|missing required.*env"; then
    echo "MISSING_ENV_VARS"
    return
  fi

  # Missing files
  if echo "$service_logs" | grep -q "No such file or directory\|not found\|ENOENT"; then
    echo "MISSING_FILES"
    return
  fi

  # SSL/TLS certificate issues
  if echo "$service_logs" | grep -q "certificate.*expired\|SSL.*error\|TLS.*handshake"; then
    echo "SSL_CERT_ERROR"
    return
  fi

  # Unknown error
  echo "UNKNOWN"
}

# Get a concise error message for the user
get_error_message() {
  local error_code="$1"

  case "$error_code" in
    POSTGRES_PORT_5433)
      echo "Postgres port mismatch (expects 5433)"
      ;;
    POSTGRES_NOT_RUNNING)
      echo "Postgres not running"
      ;;
    POSTGRES_AUTH_FAILED)
      echo "Postgres authentication failed"
      ;;
    POSTGRES_CONNECTION)
      echo "Cannot connect to Postgres"
      ;;
    DATABASE_NOT_FOUND)
      echo "Database does not exist"
      ;;
    REDIS_CONNECTION)
      echo "Cannot connect to Redis"
      ;;
    ELASTICSEARCH_CONNECTION)
      echo "Cannot connect to Elasticsearch/OpenSearch"
      ;;
    NETWORK_DNS)
      echo "Network/DNS resolution issue"
      ;;
    PORT_IN_USE)
      echo "Port already in use"
      ;;
    OUT_OF_MEMORY)
      echo "Out of memory"
      ;;
    PERMISSION_DENIED)
      echo "Permission denied"
      ;;
    MISSING_ENV_VARS)
      echo "Missing required environment variables"
      ;;
    MISSING_FILES)
      echo "Required files missing"
      ;;
    SSL_CERT_ERROR)
      echo "SSL/TLS certificate error"
      ;;
    SCHEMA_NOT_FOUND)
      echo "Database schema missing"
      ;;
    MISSING_NODE_MODULES)
      echo "Node modules not installed"
      ;;
    BULLMQ_MISSING_MODULES)
      echo "BullMQ dependencies missing"
      ;;
    BULLMQ_REDIS_CONNECTION)
      echo "BullMQ cannot connect to Redis"
      ;;
    NGINX_UPSTREAM_NOT_FOUND)
      echo "Nginx cannot find upstream service"
      ;;
    MISSING_HEALTHCHECK_TOOLS)
      echo "Healthcheck tools missing in container"
      ;;
    NO_SHELL_IN_CONTAINER)
      echo "Container has no shell (minimal image)"
      ;;
    *)
      echo "Service startup failed"
      ;;
  esac
}
