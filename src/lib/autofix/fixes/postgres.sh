#!/usr/bin/env bash


# Track what fix was applied
LAST_FIX_DESCRIPTION=""

set -euo pipefail


get_last_fix_description() {
  echo "$LAST_FIX_DESCRIPTION"
}

fix_postgres_port_5433() {
  local service_name="${1:-${PROJECT_NAME:-nself}_hasura}"
  local attempt="${2:-0}"

  # Different strategy for each attempt
  case $attempt in
    0)
      # First attempt: Start Postgres if not running
      if ! docker ps -q -f name=${PROJECT_NAME:-nself}_postgres | grep -q .; then
        docker compose up -d postgres >/dev/null 2>&1
        sleep 3
        LAST_FIX_DESCRIPTION="Started Postgres on port 5433"
        return 0
      fi

      # If running, restart it
      docker compose restart postgres >/dev/null 2>&1
      sleep 3
      LAST_FIX_DESCRIPTION="Restarted Postgres service"
      return 0
      ;;

    1)
      # Second attempt: Change port configuration
      # Check if we need to switch from 5433 to 5432
      local current_port=$(grep "POSTGRES_PORT=" .env.local 2>/dev/null | cut -d= -f2)

      if [[ "$current_port" == "5433" ]]; then
        # Try switching to 5432
        sed -i '' 's/POSTGRES_PORT=5433/POSTGRES_PORT=5432/' .env.local 2>/dev/null
        LAST_FIX_DESCRIPTION="Changed Postgres port from 5433 to 5432"
      else
        # Ensure it's set to 5432
        if ! grep -q "POSTGRES_PORT=" .env.local 2>/dev/null; then
          echo "POSTGRES_PORT=5432" >>.env.local
        fi
        LAST_FIX_DESCRIPTION="Set Postgres port to 5432"
      fi

      # Rebuild and restart
      nself build --force >/dev/null 2>&1
      docker compose stop postgres hasura >/dev/null 2>&1
      docker compose rm -f postgres hasura >/dev/null 2>&1
      docker compose up -d postgres >/dev/null 2>&1
      sleep 5
      docker compose up -d hasura >/dev/null 2>&1
      return 0
      ;;

    2)
      # Third attempt: Full reset with port 5432
      docker compose down >/dev/null 2>&1

      # Force port 5432
      sed -i '' '/POSTGRES_PORT=/d' .env.local 2>/dev/null
      echo "POSTGRES_PORT=5432" >>.env.local

      # Clear any cached volumes
      docker volume rm ${PROJECT_NAME:-nself}_postgres_data 2>/dev/null

      # Rebuild everything
      nself build --force >/dev/null 2>&1

      # Start fresh
      docker compose up -d postgres >/dev/null 2>&1
      sleep 5

      LAST_FIX_DESCRIPTION="Reset Postgres with port 5432 and fresh data"
      return 0
      ;;

    *)
      return 1
      ;;
  esac
}

fix_postgres_not_running() {
  docker compose up -d postgres >/dev/null 2>&1

  # Wait for it to be ready
  local max_wait=15
  local waited=0
  while [[ $waited -lt $max_wait ]]; do
    if docker exec ${PROJECT_NAME:-nself}_postgres pg_isready -U postgres >/dev/null 2>&1; then
      LAST_FIX_DESCRIPTION="Started Postgres database"
      return 0
    fi
    sleep 1
    ((waited++))
  done

  return 1
}

fix_postgres_connection() {
  # Full restart with fresh state
  docker compose stop postgres >/dev/null 2>&1
  docker compose rm -f postgres >/dev/null 2>&1
  docker volume rm ${PROJECT_NAME:-nself}_postgres_data 2>/dev/null
  docker compose up -d postgres >/dev/null 2>&1
  sleep 5

  LAST_FIX_DESCRIPTION="Reset Postgres with fresh state"
  return 0
}
