#!/usr/bin/env bash
set -euo pipefail


# services.sh - Service-specific error handlers

# Service error codes
readonly POSTGRES_CONNECTION_FAILED="POSTGRES_CONNECTION_FAILED"
readonly POSTGRES_INIT_FAILED="POSTGRES_INIT_FAILED"
readonly HASURA_METADATA_ERROR="HASURA_METADATA_ERROR"
readonly HASURA_MIGRATION_FAILED="HASURA_MIGRATION_FAILED"
readonly AUTH_CONFIG_ERROR="AUTH_CONFIG_ERROR"
readonly MINIO_BUCKET_ERROR="MINIO_BUCKET_ERROR"
readonly REDIS_CONNECTION_FAILED="REDIS_CONNECTION_FAILED"
readonly NGINX_CONFIG_ERROR="NGINX_CONFIG_ERROR"

# Check PostgreSQL
check_postgres() {
  log_info "Checking PostgreSQL..."

  local pg_container="${PROJECT_NAME:-nself}_postgres_1"

  # Check if container is running
  if ! docker ps --format "{{.Names}}" | grep -q "$pg_container"; then
    log_debug "PostgreSQL container not running"
    return 0 # Not an error if not started yet
  fi

  # Try to connect
  local pg_host="${POSTGRES_HOST:-localhost}"
  local pg_port="${POSTGRES_PORT:-5432}"
  local pg_user="${POSTGRES_USER:-postgres}"
  local pg_db="${POSTGRES_DB:-postgres}"

  if ! docker exec "$pg_container" pg_isready -h localhost -p 5432 -U "$pg_user" >/dev/null 2>&1; then
    register_error "$POSTGRES_CONNECTION_FAILED" \
      "Cannot connect to PostgreSQL" \
      $ERROR_MAJOR \
      "true" \
      "fix_postgres_connection"

    # Get logs for debugging
    local pg_logs=$(docker logs "$pg_container" 2>&1 | tail -10)
    log_debug "PostgreSQL logs:\n$pg_logs"

    return 1
  fi

  log_success "PostgreSQL is responding"
  return 0
}

# Fix PostgreSQL connection
fix_postgres_connection() {
  log_info "Attempting to fix PostgreSQL connection..."

  local pg_container="${PROJECT_NAME:-nself}_postgres_1"

  # Check if container exists but is stopped
  if docker ps -a --format "{{.Names}}" | grep -q "$pg_container"; then
    if ! docker ps --format "{{.Names}}" | grep -q "$pg_container"; then
      log_info "Starting PostgreSQL container..."
      docker start "$pg_container"
      sleep 5
    fi
  fi

  # Check if it's a password issue
  local pg_logs=$(docker logs "$pg_container" 2>&1 | tail -20)

  if echo "$pg_logs" | grep -q "password authentication failed"; then
    log_error "PostgreSQL password mismatch"
    log_info "The database password doesn't match .env.local"
    log_info "Solutions:"
    log_info "  1. Reset database: nself reset --volumes"
    log_info "  2. Update password in .env.local to match database"
    return 1
  fi

  if echo "$pg_logs" | grep -q "database system is starting up"; then
    log_info "PostgreSQL is still starting up, waiting..."
    sleep 10

    if check_postgres; then
      return 0
    fi
  fi

  # Try restarting the container
  log_info "Restarting PostgreSQL container..."
  docker restart "$pg_container"
  sleep 5

  if check_postgres; then
    log_success "PostgreSQL connection restored"
    return 0
  else
    log_error "Could not fix PostgreSQL connection"
    return 1
  fi
}

# Check Hasura
check_hasura() {
  log_info "Checking Hasura GraphQL Engine..."

  local hasura_container="${PROJECT_NAME:-nself}_hasura_1"

  if ! docker ps --format "{{.Names}}" | grep -q "$hasura_container"; then
    log_debug "Hasura container not running"
    return 0
  fi

  # Check Hasura health endpoint
  local hasura_port="${HASURA_PORT:-8080}"

  if ! curl -s -f "http://localhost:$hasura_port/healthz" >/dev/null 2>&1; then
    register_error "$HASURA_METADATA_ERROR" \
      "Hasura health check failed" \
      $ERROR_MAJOR \
      "true" \
      "fix_hasura_health"

    # Check logs
    local hasura_logs=$(docker logs "$hasura_container" 2>&1 | grep -i error | tail -5)
    if [[ -n "$hasura_logs" ]]; then
      log_debug "Hasura errors:\n$hasura_logs"
    fi

    return 1
  fi

  log_success "Hasura is healthy"
  return 0
}

# Fix Hasura health
fix_hasura_health() {
  log_info "Attempting to fix Hasura..."

  local hasura_container="${PROJECT_NAME:-nself}_hasura_1"

  # Check logs for specific issues
  local hasura_logs=$(docker logs "$hasura_container" 2>&1 | tail -50)

  if echo "$hasura_logs" | grep -q "connection refused.*5432"; then
    log_info "Hasura cannot connect to PostgreSQL"

    # Fix PostgreSQL first
    if fix_postgres_connection; then
      log_info "Restarting Hasura..."
      docker restart "$hasura_container"
      sleep 5

      if check_hasura; then
        return 0
      fi
    fi
  fi

  if echo "$hasura_logs" | grep -q "admin secret"; then
    log_error "Hasura admin secret mismatch"
    log_info "The HASURA_ADMIN_SECRET in .env.local doesn't match the running container"
    log_info "Solution: nself stop && nself start"
    return 1
  fi

  # Try restarting
  log_info "Restarting Hasura container..."
  docker restart "$hasura_container"
  sleep 10

  if check_hasura; then
    log_success "Hasura health restored"
    return 0
  else
    log_error "Could not fix Hasura"
    return 1
  fi
}

# Check Auth service
check_auth_service() {
  log_info "Checking Auth service..."

  local auth_container="${PROJECT_NAME:-nself}_auth_1"

  if ! docker ps --format "{{.Names}}" | grep -q "$auth_container"; then
    log_debug "Auth container not running"
    return 0
  fi

  local auth_port="${AUTH_PORT:-4000}"

  # Check if auth is responding
  if ! curl -s -f "http://localhost:$auth_port/healthz" >/dev/null 2>&1; then
    register_error "$AUTH_CONFIG_ERROR" \
      "Auth service not responding" \
      $ERROR_MAJOR \
      "true" \
      "fix_auth_service"

    return 1
  fi

  log_success "Auth service is healthy"
  return 0
}

# Fix Auth service
fix_auth_service() {
  log_info "Attempting to fix Auth service..."

  local auth_container="${PROJECT_NAME:-nself}_auth_1"

  # Check logs
  local auth_logs=$(docker logs "$auth_container" 2>&1 | grep -i error | tail -5)

  if echo "$auth_logs" | grep -q "JWT_SECRET"; then
    log_error "Auth service JWT_SECRET issue"
    log_info "The JWT_SECRET is missing or invalid"
    log_info "Check .env.local and ensure JWT_SECRET is set"
    return 1
  fi

  if echo "$auth_logs" | grep -q "HASURA_GRAPHQL_DATABASE_URL"; then
    log_error "Auth service cannot connect to database"

    # Fix PostgreSQL first
    if fix_postgres_connection; then
      log_info "Restarting Auth service..."
      docker restart "$auth_container"
      sleep 5

      if check_auth_service; then
        return 0
      fi
    fi
  fi

  # Generic restart
  log_info "Restarting Auth container..."
  docker restart "$auth_container"
  sleep 5

  if check_auth_service; then
    log_success "Auth service restored"
    return 0
  else
    log_error "Could not fix Auth service"
    return 1
  fi
}

# Check MinIO
check_minio() {
  log_info "Checking MinIO storage..."

  local minio_container="${PROJECT_NAME:-nself}_minio_1"

  if ! docker ps --format "{{.Names}}" | grep -q "$minio_container"; then
    log_debug "MinIO container not running"
    return 0
  fi

  local minio_port="${MINIO_PORT:-9000}"

  # Check MinIO health
  if ! curl -s -f "http://localhost:$minio_port/minio/health/live" >/dev/null 2>&1; then
    register_error "$MINIO_BUCKET_ERROR" \
      "MinIO storage not responding" \
      $ERROR_MINOR \
      "true" \
      "fix_minio"

    return 1
  fi

  log_success "MinIO is healthy"
  return 0
}

# Fix MinIO
fix_minio() {
  log_info "Attempting to fix MinIO..."

  local minio_container="${PROJECT_NAME:-nself}_minio_1"

  # Restart container
  log_info "Restarting MinIO container..."
  docker restart "$minio_container"
  sleep 5

  if check_minio; then
    log_success "MinIO restored"
    return 0
  else
    log_error "Could not fix MinIO"
    log_info "This is not critical - file storage may be unavailable"
    return 1
  fi
}

# Check Redis
check_redis() {
  log_info "Checking Redis cache..."

  local redis_container="${PROJECT_NAME:-nself}_redis_1"

  if ! docker ps --format "{{.Names}}" | grep -q "$redis_container"; then
    log_debug "Redis container not running"
    return 0
  fi

  # Try to ping Redis
  if ! docker exec "$redis_container" redis-cli ping >/dev/null 2>&1; then
    register_error "$REDIS_CONNECTION_FAILED" \
      "Redis not responding" \
      $ERROR_MINOR \
      "true" \
      "fix_redis"

    return 1
  fi

  log_success "Redis is responding"
  return 0
}

# Fix Redis
fix_redis() {
  log_info "Attempting to fix Redis..."

  local redis_container="${PROJECT_NAME:-nself}_redis_1"

  # Restart container
  log_info "Restarting Redis container..."
  docker restart "$redis_container"
  sleep 3

  if check_redis; then
    log_success "Redis restored"
    return 0
  else
    log_error "Could not fix Redis"
    log_info "This is not critical - caching may be disabled"
    return 1
  fi
}

# Check all services
check_all_services() {
  log_info "Checking all services..."

  check_postgres
  check_hasura
  check_auth_service
  check_minio
  check_redis

  local service_errors=$((ERROR_COUNT - FIXED_ERRORS))

  if [[ $service_errors -eq 0 ]]; then
    log_success "All services are healthy"
    return 0
  else
    log_warning "$service_errors service issue(s) detected"
    return 1
  fi
}

# Export functions
export -f check_postgres
export -f fix_postgres_connection
export -f check_hasura
export -f fix_hasura_health
export -f check_auth_service
export -f fix_auth_service
export -f check_minio
export -f fix_minio
export -f check_redis
export -f fix_redis
export -f check_all_services
