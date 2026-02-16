#!/usr/bin/env bash


# Service dependency fixes - handles startup order issues

LAST_FIX_DESCRIPTION=""

set -euo pipefail


get_last_fix_description() {
  echo "$LAST_FIX_DESCRIPTION"
}

# Fix service dependency order issues
fix_service_dependencies() {
  local service_name="$1"
  local project_name="${PROJECT_NAME:-nself}"

  case "$service_name" in
    *nginx*)
      # Nginx depends on upstream services
      # Ensure auth, api, and other services are running first
      local deps="auth nest_api nest_webhooks go_gateway py_ml-model"
      local deps_started=0

      for dep in $deps; do
        if ! docker ps -q -f name="${project_name}_${dep}" | grep -q .; then
          docker compose up -d "$dep" >/dev/null 2>&1
          ((deps_started++))
        fi
      done

      if [[ $deps_started -gt 0 ]]; then
        # Wait for services to be ready
        sleep 5
      fi

      # Now restart nginx
      docker compose restart nginx >/dev/null 2>&1
      LAST_FIX_DESCRIPTION="Started nginx dependencies and restarted nginx"
      ;;

    *auth*)
      # Auth depends on postgres and hasura
      if ! docker ps -q -f name="${project_name}_postgres" | grep -q .; then
        docker compose up -d postgres >/dev/null 2>&1
        sleep 5
      fi

      if ! docker ps -q -f name="${project_name}_hasura" | grep -q .; then
        docker compose up -d hasura >/dev/null 2>&1
        sleep 5
      fi

      # Ensure schemas exist
      docker exec "${project_name}_postgres" psql -U postgres -c "CREATE SCHEMA IF NOT EXISTS auth;" >/dev/null 2>&1

      # Restart auth
      docker compose restart auth >/dev/null 2>&1
      LAST_FIX_DESCRIPTION="Ensured auth dependencies and restarted"
      ;;

    *storage*)
      # Storage depends on postgres and minio
      if ! docker ps -q -f name="${project_name}_postgres" | grep -q .; then
        docker compose up -d postgres >/dev/null 2>&1
        sleep 5
      fi

      if ! docker ps -q -f name="${project_name}_minio" | grep -q .; then
        docker compose up -d minio >/dev/null 2>&1
        sleep 5
      fi

      # Ensure storage schema exists
      docker exec "${project_name}_postgres" psql -U postgres -c "CREATE SCHEMA IF NOT EXISTS storage;" >/dev/null 2>&1

      # Restart storage
      docker compose restart storage >/dev/null 2>&1
      LAST_FIX_DESCRIPTION="Ensured storage dependencies and restarted"
      ;;

    *bull* | *worker*)
      # Workers depend on redis
      if ! docker ps -q -f name="${project_name}_redis" | grep -q .; then
        docker compose up -d redis >/dev/null 2>&1
        sleep 3
      fi

      # Restart the worker
      docker compose restart "$service_name" >/dev/null 2>&1
      LAST_FIX_DESCRIPTION="Ensured Redis is running and restarted worker"
      ;;

    *)
      # Generic dependency check - ensure postgres and redis are up
      if ! docker ps -q -f name="${project_name}_postgres" | grep -q .; then
        docker compose up -d postgres >/dev/null 2>&1
        sleep 5
      fi

      if ! docker ps -q -f name="${project_name}_redis" | grep -q .; then
        docker compose up -d redis >/dev/null 2>&1
        sleep 3
      fi

      docker compose restart "$service_name" >/dev/null 2>&1
      LAST_FIX_DESCRIPTION="Ensured core dependencies and restarted service"
      ;;
  esac

  return 0
}

# Start services in correct order
start_services_ordered() {
  local project_name="${PROJECT_NAME:-nself}"

  # Order: databases -> core services -> application services -> gateways
  local order=(
    "postgres"
    "redis"
    "minio"
    "mailpit"
    "hasura"
    "auth"
    "storage"
    "functions"
    "nest_api"
    "nest_webhooks"
    "go_gateway"
    "py_ml-model"
    "bull_email-worker"
    "bull_payment-processor"
    "dashboard"
    "nginx"
  )

  for service in "${order[@]}"; do
    if docker compose config --services 2>/dev/null | grep -q "^${service}$"; then
      if ! docker ps -q -f name="${project_name}_${service}" | grep -q .; then
        docker compose up -d "$service" >/dev/null 2>&1
      fi
    fi
  done

  LAST_FIX_DESCRIPTION="Started all services in dependency order"
  return 0
}
