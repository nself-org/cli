#!/usr/bin/env bash


fix_postgres_connection() {

set -euo pipefail

  local service_name="$1"
  local service_logs="$2"
  local strategy="${3:-restart_postgres}"

  log_info "Applying strategy: $strategy"

  case "$strategy" in
    restart_postgres)
      # First attempt: Just ensure postgres is running
      log_info "Ensuring Postgres is running"
      docker compose up -d postgres >/dev/null 2>&1

      # Wait for it to be ready
      local max_wait=15
      local waited=0
      while [[ $waited -lt $max_wait ]]; do
        if docker exec ${PROJECT_NAME:-nself}_postgres pg_isready -U postgres >/dev/null 2>&1; then
          log_success "Postgres is ready"
          # Restart the dependent service
          docker compose stop "$service_name" >/dev/null 2>&1
          docker compose rm -f "$service_name" >/dev/null 2>&1
          return 99 # Retry
        fi
        sleep 1
        ((waited++))
      done

      log_warning "Postgres not ready after ${max_wait} seconds"
      return 99 # Try next strategy
      ;;

    check_port_config)
      # Second attempt: Check port configuration mismatch
      log_info "Checking Postgres port configuration"

      # Check if it's a port 5433 vs 5432 issue
      if echo "$service_logs" | grep -q "port 5433"; then
        # Check actual postgres port
        local postgres_port=$(docker port ${PROJECT_NAME:-nself}_postgres 5432 2>/dev/null | cut -d: -f2)

        if [[ "$postgres_port" == "5432" ]]; then
          log_info "Port mismatch detected: Postgres on 5432, app expects 5433"

          # Try to update the configuration
          if grep -q "POSTGRES_PORT=5433" .env.local 2>/dev/null; then
            sed -i '' 's/POSTGRES_PORT=5433/POSTGRES_PORT=5432/' .env.local
            log_info "Updated POSTGRES_PORT to 5432 in .env.local"

            # Rebuild configuration
            nself build --force >/dev/null 2>&1
            sleep 2
            return 99 # Retry
          fi

          # Alternative: restart postgres on correct port
          log_info "Restarting Postgres on port 5433"
          docker compose stop postgres >/dev/null 2>&1
          docker compose rm -f postgres >/dev/null 2>&1

          # Set the correct port and restart
          export POSTGRES_PORT=5433
          docker compose up -d postgres >/dev/null 2>&1
          sleep 5
          return 99 # Retry
        fi
      fi

      # If no port issue, try recreating postgres
      docker compose stop postgres >/dev/null 2>&1
      docker compose rm -f postgres >/dev/null 2>&1
      docker compose up -d postgres >/dev/null 2>&1
      sleep 5
      return 99 # Retry
      ;;

    recreate_network)
      # Third attempt: Network issues
      log_info "Recreating Docker network"

      # Stop all services
      docker compose down >/dev/null 2>&1

      # Remove and recreate the network
      local network_name="${PROJECT_NAME:-nself}_default"
      docker network rm $network_name 2>/dev/null
      docker network create $network_name >/dev/null 2>&1

      # Start postgres first
      docker compose up -d postgres >/dev/null 2>&1
      sleep 5

      # Then the dependent service
      return 99 # Retry
      ;;

    full_database_reset)
      # Fourth attempt: Complete database reset
      log_info "Performing full database reset"

      # Stop everything
      docker compose down -v >/dev/null 2>&1

      # Remove postgres data volume
      docker volume rm ${PROJECT_NAME:-nself}_postgres_data 2>/dev/null

      # Rebuild everything
      nself build --force >/dev/null 2>&1

      # Start postgres fresh
      docker compose up -d postgres >/dev/null 2>&1

      # Wait longer for initial setup
      log_info "Waiting for fresh Postgres initialization"
      sleep 10

      # Check if ready
      if docker exec ${PROJECT_NAME:-nself}_postgres pg_isready -U postgres >/dev/null 2>&1; then
        log_success "Fresh Postgres is ready"
        return 99 # Retry
      fi

      return 1 # Give up if even this doesn't work
      ;;

    recreate_service)
      # Fallback: Just recreate the service
      log_info "Recreating service as last resort"
      docker compose stop "$service_name" >/dev/null 2>&1
      docker compose rm -f "$service_name" >/dev/null 2>&1
      return 99 # Retry
      ;;

    *)
      log_error "Unknown strategy: $strategy"
      return 1
      ;;
  esac
}
