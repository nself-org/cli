#!/usr/bin/env bash


# Comprehensive auto-fix module for nself
# Handles all common Docker, PostgreSQL, and service issues

# Source dependencies
AUTOFIX_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$AUTOFIX_DIR/../utils/display.sh" 2>/dev/null || true
source "$AUTOFIX_DIR/../utils/docker.sh" 2>/dev/null || true

# Fix all known issues for a service
fix_service_comprehensive() {
  local service_name="$1"
  local project_name="${PROJECT_NAME:-nself}"
  local container_name="${project_name}_${service_name}"

  # Get service logs for analysis
  local service_logs=$(docker logs "$container_name" 2>&1 | tail -100)

  # Check for BullMQ issues
  if [[ "$service_name" =~ bull ]] && echo "$service_logs" | grep -q "Cannot find module.*bull"; then
    log_info "Fixing BullMQ dependencies for $service_name..."
    if [[ -f "$AUTOFIX_DIR/fixes/bullmq.sh" ]]; then
      source "$AUTOFIX_DIR/fixes/bullmq.sh"
      fix_bullmq_worker "$service_name"
      return $?
    fi
  fi

  # Check for health check issues
  local health_status=$(docker inspect "$container_name" --format='{{.State.Health.Status}}' 2>/dev/null)
  if [[ "$health_status" == "unhealthy" ]]; then
    # Check if it's a false positive (service is actually running)
    local container_state=$(docker inspect "$container_name" --format='{{.State.Status}}' 2>/dev/null)
    if [[ "$container_state" == "running" ]]; then
      # Special handling for nginx and auth services
      if [[ "$service_name" == "nginx" ]] || [[ "$service_name" == "auth" ]]; then
        log_info "Fixing $service_name service..."
        # For nginx, just recreate the container
        if [[ "$service_name" == "nginx" ]]; then
          docker compose stop nginx >/dev/null 2>&1
          docker compose rm -f nginx >/dev/null 2>&1
          docker compose up -d nginx >/dev/null 2>&1
          log_info "Recreated nginx container"
        elif [[ "$service_name" == "auth" ]]; then
          # Auth service might need environment variables
          docker compose stop auth >/dev/null 2>&1
          docker compose rm -f auth >/dev/null 2>&1
          docker compose up -d auth >/dev/null 2>&1
          log_info "Recreated auth container"
        fi
      else
        # For other containers, try to check for shell first
        # Test if container has sh available before trying to exec into it
        if docker exec "$container_name" /bin/sh -c "exit 0" 2>/dev/null ||
          docker exec "$container_name" /bin/bash -c "exit 0" 2>/dev/null; then
          # Container has a shell, check for curl/wget
          if docker exec "$container_name" sh -c "command -v curl || test -x /usr/bin/curl" 2>/dev/null; then
            log_debug "$service_name has curl, health check should work"
          elif docker exec "$container_name" sh -c "command -v wget || test -x /usr/bin/wget" 2>/dev/null; then
            log_info "Fixing health check for $service_name (using wget)..."
            if [[ -f "$AUTOFIX_DIR/fixes/healthcheck.sh" ]]; then
              source "$AUTOFIX_DIR/fixes/healthcheck.sh"
              fix_service_healthcheck "$service_name"
            fi
          else
            log_warning "$service_name missing health check tools, installing..."
            # Try to install wget
            docker exec "$container_name" sh -c "apk add --no-cache wget 2>/dev/null || apt-get update && apt-get install -y wget 2>/dev/null || yum install -y wget 2>/dev/null" >/dev/null 2>&1
          fi
        else
          # No shell available, just recreate the container
          log_warning "$service_name container has no shell, recreating..."
          docker compose stop "$service_name" >/dev/null 2>&1
          docker compose rm -f "$service_name" >/dev/null 2>&1
          docker compose up -d "$service_name" >/dev/null 2>&1
        fi
      fi
    fi
  fi

  # Check for database schema issues
  if echo "$service_logs" | grep -q "schema.*does not exist"; then
    log_info "Fixing database schemas..."
    if [[ -f "$AUTOFIX_DIR/fixes/schema.sh" ]]; then
      source "$AUTOFIX_DIR/fixes/schema.sh"
      fix_missing_schemas "$service_name"
    fi
  fi

  # Check for Node.js module issues
  if echo "$service_logs" | grep -q "Cannot find module" && [[ ! "$service_name" =~ bull ]]; then
    log_info "Installing missing Node.js modules for $service_name..."
    docker exec "$container_name" npm install >/dev/null 2>&1 || {
      # If exec fails, rebuild the container
      docker compose build "$service_name" >/dev/null 2>&1
      docker compose up -d "$service_name" >/dev/null 2>&1
    }
  fi

  # Check for port conflicts
  if echo "$service_logs" | grep -q "address already in use"; then
    log_info "Resolving port conflict for $service_name..."
    # Get the port from the error
    local port=$(echo "$service_logs" | grep -oP 'bind.*:(\d+)' | grep -oP '\d+' | head -1)
    if [[ -n "$port" ]]; then
      # Check if it's our own container using the port
      local using_port=$(lsof -Pi :$port -sTCP:LISTEN -t 2>/dev/null | head -1)
      if [[ -n "$using_port" ]]; then
        # Kill the process if it's not critical
        local process_name=$(ps -p $using_port -o comm= 2>/dev/null)
        if [[ "$process_name" != "docker" ]] && [[ "$process_name" != "systemd" ]]; then
          log_warning "Killing process $process_name using port $port"
          kill -9 $using_port 2>/dev/null || true
          sleep 2
          docker compose up -d "$service_name" >/dev/null 2>&1
        fi
      fi
    fi
  fi

  return 0
}

# Pre-flight checks and fixes before starting services
pre_flight_fixes() {
  local project_name="${PROJECT_NAME:-nself}"

  # Ensure required directories exist
  local required_dirs=(
    "hasura/migrations"
    "hasura/metadata"
    "functions/src"
    "services"
    "nginx/conf.d"
  )

  for dir in "${required_dirs[@]}"; do
    if [[ ! -d "$dir" ]]; then
      log_debug "Creating missing directory: $dir"
      mkdir -p "$dir"
    fi
  done

  # Ensure PostgreSQL is ready
  if docker ps -a | grep -q "${project_name}_postgres"; then
    local postgres_status=$(docker inspect "${project_name}_postgres" --format='{{.State.Status}}' 2>/dev/null)
    if [[ "$postgres_status" != "running" ]]; then
      log_info "Starting PostgreSQL..."
      docker compose up -d postgres >/dev/null 2>&1

      # Wait for PostgreSQL to be ready
      local retries=30
      while [[ $retries -gt 0 ]]; do
        if docker exec "${project_name}_postgres" pg_isready -U postgres >/dev/null 2>&1; then
          break
        fi
        retries=$((retries - 1))
        sleep 1
      done
    fi

    # Ensure database and schemas exist
    local db_name="${POSTGRES_DB:-nhost}"
    docker exec "${project_name}_postgres" psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$db_name'" | grep -q 1 || {
      log_info "Creating database $db_name..."
      docker exec "${project_name}_postgres" psql -U postgres -c "CREATE DATABASE $db_name;" >/dev/null 2>&1
    }

    # Create schemas
    docker exec "${project_name}_postgres" psql -U postgres -d "$db_name" -c "
            CREATE SCHEMA IF NOT EXISTS auth;
            CREATE SCHEMA IF NOT EXISTS storage;
            CREATE SCHEMA IF NOT EXISTS public;
            GRANT ALL ON SCHEMA auth TO postgres;
            GRANT ALL ON SCHEMA storage TO postgres;
            GRANT ALL ON SCHEMA public TO postgres;
        " >/dev/null 2>&1
  fi

  # Ensure Redis is ready for BullMQ
  if [[ "${REDIS_ENABLED:-true}" == "true" ]]; then
    local redis_status=$(docker inspect "${project_name}_redis" --format='{{.State.Status}}' 2>/dev/null)
    if [[ "$redis_status" != "running" ]]; then
      log_info "Starting Redis..."
      docker compose up -d redis >/dev/null 2>&1
      sleep 2
    fi
  fi

  # Fix any nginx configuration issues
  if [[ -d "nginx/conf.d" ]]; then
    for conf in nginx/conf.d/*.conf; do
      if [[ -f "$conf" ]]; then
        # Remove empty timeout directives
        sed -i.bak 's/proxy_connect_timeout[[:space:]]*;/proxy_connect_timeout 60s;/g' "$conf" 2>/dev/null || true
        sed -i.bak 's/proxy_send_timeout[[:space:]]*;/proxy_send_timeout 60s;/g' "$conf" 2>/dev/null || true
        sed -i.bak 's/proxy_read_timeout[[:space:]]*;/proxy_read_timeout 60s;/g' "$conf" 2>/dev/null || true
        rm -f "${conf}.bak"
      fi
    done
  fi

  return 0
}

# Fix all unhealthy services
fix_unhealthy_services() {
  local project_name="${PROJECT_NAME:-nself}"
  local fixed_count=0

  # Get all unhealthy or restarting services
  local unhealthy_services=$(docker ps --filter "name=${project_name}" --format "{{.Names}}\t{{.Status}}" | grep -E "(unhealthy|Restarting)" | cut -f1)

  if [[ -z "$unhealthy_services" ]]; then
    return 0
  fi

  log_info "Found unhealthy services, applying fixes..."

  for container in $unhealthy_services; do
    local service_name="${container#${project_name}_}"
    log_debug "Fixing $service_name..."

    if fix_service_comprehensive "$service_name"; then
      fixed_count=$((fixed_count + 1))
    fi
  done

  if [[ $fixed_count -gt 0 ]]; then
    log_success "Fixed $fixed_count service(s)"
    return 0
  else
    return 1
  fi
}

# Main comprehensive fix function
run_comprehensive_fixes() {
  local max_attempts="${1:-3}"
  local attempt=1

  log_info "Running comprehensive fixes..."

  # Run pre-flight fixes first
  pre_flight_fixes

  # Try to fix unhealthy services
  while [[ $attempt -le $max_attempts ]]; do
    # Wait a moment for services to stabilize
    sleep 3

    # Check for unhealthy services
    local unhealthy_count=$(docker ps --filter "name=${PROJECT_NAME}" --format "{{.Status}}" | grep -cE "(unhealthy|Restarting)" || echo "0")
    # Ensure it's a single number (remove any whitespace/newlines)
    unhealthy_count=$(echo "$unhealthy_count" | tr -d '\n' | awk '{print $1}')

    if [[ $unhealthy_count -eq 0 ]]; then
      log_success "All services healthy!"
      return 0
    fi

    log_info "Attempt $attempt/$max_attempts: Found $unhealthy_count unhealthy service(s)"

    if fix_unhealthy_services; then
      # Give services time to start after fixes
      sleep 5
    else
      log_warning "Some fixes may have failed, retrying..."
    fi

    attempt=$((attempt + 1))
  done

  # Final check
  local final_unhealthy=$(docker ps --filter "name=${PROJECT_NAME}" --format "{{.Status}}" | grep -cE "(unhealthy|Restarting)" || echo "0")
  # Ensure it's a single number (remove any whitespace/newlines)
  final_unhealthy=$(echo "$final_unhealthy" | tr -d '\n' | awk '{print $1}')
  if [[ $final_unhealthy -gt 0 ]]; then
    log_warning "$final_unhealthy service(s) still unhealthy after fixes"
    log_info "You can check logs with: nself logs <service>"
    return 1
  fi

  return 0
}

# Export functions
export -f fix_service_comprehensive
export -f pre_flight_fixes
export -f fix_unhealthy_services
export -f run_comprehensive_fixes
