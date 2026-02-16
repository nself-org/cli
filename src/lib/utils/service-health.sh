#!/bin/bash


# Service health checking utilities

# Check if all expected services are running and healthy
check_all_services_healthy() {

set -euo pipefail

  local verbose="${1:-false}"

  # Check if docker-compose.yml exists
  if [[ ! -f "docker-compose.yml" ]]; then
    return 1 # Can't check if no compose file
  fi

  # Get list of expected services from docker-compose.yml
  local expected_services=$(docker compose config --services 2>/dev/null | sort)
  if [[ -z "$expected_services" ]]; then
    return 1 # No services defined
  fi

  # Count expected services
  local expected_count=$(echo "$expected_services" | wc -l | tr -d ' ')

  # Get list of running services (remove project prefix)
  local project_name="${PROJECT_NAME:-nself}"
  local running_services=$(docker ps --format "{{.Names}}" 2>/dev/null | grep "^${project_name}_" | sed "s/^${project_name}_//" | sort)

  # Count running services
  local running_count=0
  if [[ -n "$running_services" ]]; then
    running_count=$(echo "$running_services" | wc -l | tr -d ' ')
  fi

  # Quick check - if counts don't match, not all services are running
  if [[ $running_count -ne $expected_count ]]; then
    if [[ "$verbose" == "true" ]]; then
      echo "Expected $expected_count services, found $running_count running"
    fi
    return 1
  fi

  # Check each expected service is actually running
  local all_healthy=true
  for service in $expected_services; do
    if ! echo "$running_services" | grep -q "^${service}$"; then
      if [[ "$verbose" == "true" ]]; then
        echo "Service '$service' is not running"
      fi
      all_healthy=false
      break
    fi

    # Check if service is healthy (if it has health check)
    local container_name="${project_name}_${service}"
    local health_status=$(docker inspect --format='{{.State.Health.Status}}' "$container_name" 2>/dev/null)

    if [[ "$health_status" == "unhealthy" ]]; then
      if [[ "$verbose" == "true" ]]; then
        echo "Service '$service' is unhealthy"
        # Try to get more info about why it's unhealthy
        local last_health_output=$(docker inspect --format='{{range .State.Health.Log}}{{.Output}}{{end}}' "$container_name" 2>/dev/null | tail -1)
        if [[ -n "$last_health_output" ]]; then
          echo "  Health check output: $last_health_output"
        fi
      fi
      all_healthy=false
      break
    elif [[ "$health_status" == "starting" ]]; then
      # Service is still starting, not fully healthy yet
      if [[ "$verbose" == "true" ]]; then
        echo "Service '$service' is still starting"
      fi
      all_healthy=false
      break
    fi
  done

  if [[ "$all_healthy" == "true" ]]; then
    return 0 # All services healthy
  else
    return 1 # Some services not healthy
  fi
}

# Display running services in a concise format
display_running_services() {
  local project_name="${PROJECT_NAME:-nself}"

  # Get list of expected services from docker-compose.yml
  local expected_services=$(docker compose config --services 2>/dev/null | sort)
  if [[ -z "$expected_services" ]]; then
    return
  fi

  # Get list of running services
  local running_services=$(docker ps --format "{{.Names}}" 2>/dev/null | grep "^${project_name}_")

  # Display each service with status
  for service in $expected_services; do
    local container_name="${project_name}_${service}"
    local display_name=$(echo "$service" | sed 's/-/ /g; s/_/ /g' | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) tolower(substr($i,2))}1')

    # Check if running
    if echo "$running_services" | grep -q "^${container_name}$"; then
      # Check health status if available
      local health_status=$(docker inspect --format='{{.State.Health.Status}}' "$container_name" 2>/dev/null)

      if [[ "$health_status" == "unhealthy" ]]; then
        printf "  ${COLOR_YELLOW}●${COLOR_RESET} %-20s ${COLOR_YELLOW}(unhealthy)${COLOR_RESET}\n" "$display_name"
      elif [[ "$health_status" == "starting" ]]; then
        printf "  ${COLOR_BLUE}●${COLOR_RESET} %-20s ${COLOR_BLUE}(starting)${COLOR_RESET}\n" "$display_name"
      else
        # Get exposed port if available
        local port_info=""

        # Service-specific port detection
        case "$service" in
          postgres)
            local port=$(docker port "$container_name" 5432 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          redis)
            local port=$(docker port "$container_name" 6379 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          hasura)
            local port=$(docker port "$container_name" 8080 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          auth)
            local port=$(docker port "$container_name" 4000 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          minio | storage)
            local port=$(docker port "$container_name" 9000 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          mailpit | mail)
            local port=$(docker port "$container_name" 8025 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          functions)
            local port=$(docker port "$container_name" 3000 2>/dev/null | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
          *)
            # Try to find any exposed port
            local port=$(docker port "$container_name" 2>/dev/null | head -1 | cut -d: -f2)
            [[ -n "$port" ]] && port_info=":$port"
            ;;
        esac

        printf "  ${COLOR_GREEN}●${COLOR_RESET} %-20s${COLOR_DIM}%s${COLOR_RESET}\n" "$display_name" "$port_info"
      fi
    else
      printf "  ${COLOR_RED}●${COLOR_RESET} %-20s ${COLOR_DIM}(stopped)${COLOR_RESET}\n" "$display_name"
    fi
  done
}

# Check if configuration has changed since services started
check_config_changed() {
  # Check if docker-compose.yml is newer than running containers
  if [[ ! -f "docker-compose.yml" ]]; then
    return 1 # No compose file
  fi

  local compose_modified=$(stat -f %m "docker-compose.yml" 2>/dev/null || stat -c %Y "docker-compose.yml" 2>/dev/null)
  local env_modified=0

  if [[ -f ".env.local" ]]; then
    env_modified=$(stat -f %m ".env.local" 2>/dev/null || stat -c %Y ".env.local" 2>/dev/null)
  fi

  # Get the oldest running container's start time
  local project_name="${PROJECT_NAME:-nself}"
  local oldest_container_start=$(docker ps --format "{{.Names}} {{.CreatedAt}}" 2>/dev/null | grep "^${project_name}_" | while read name created; do
    docker inspect --format='{{.State.StartedAt}}' "$name" 2>/dev/null
  done | sort | head -1)

  if [[ -z "$oldest_container_start" ]]; then
    return 1 # No running containers
  fi

  # Convert container start time to timestamp
  local container_timestamp=$(date -d "$oldest_container_start" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "${oldest_container_start%%.*}" +%s 2>/dev/null)

  # Check if compose or env is newer than containers
  if [[ $compose_modified -gt $container_timestamp ]] || [[ $env_modified -gt $container_timestamp ]]; then
    return 0 # Config has changed
  else
    return 1 # Config unchanged
  fi
}

# Get list of services that need restart due to config changes
get_changed_services() {
  # For now, return all services if config changed
  # Future: implement smart detection of which services actually need restart
  if check_config_changed; then
    docker compose config --services 2>/dev/null
  fi
}

# Export functions
export -f check_all_services_healthy
export -f display_running_services
export -f check_config_changed
export -f get_changed_services
