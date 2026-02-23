#!/usr/bin/env bash

# status.sh - Detailed service status with resource usage

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/docker.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/services.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/docker-batch.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/runtime.sh" 2>/dev/null || true

# Source display.sh and force colors to be set
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true

# Always ensure colors are defined (they'll work in real terminal)
COLOR_RESET=${COLOR_RESET:-$'\033[0m'}
COLOR_BLUE=${COLOR_BLUE:-$'\033[0;34m'}
COLOR_BOLD=${COLOR_BOLD:-$'\033[1m'}
COLOR_DIM=${COLOR_DIM:-$'\033[2m'}
COLOR_CYAN=${COLOR_CYAN:-$'\033[0;36m'}
COLOR_GREEN=${COLOR_GREEN:-$'\033[0;32m'}
COLOR_RED=${COLOR_RED:-$'\033[0;31m'}
COLOR_YELLOW=${COLOR_YELLOW:-$'\033[0;33m'}

export COLOR_RESET COLOR_BLUE COLOR_BOLD COLOR_DIM COLOR_CYAN COLOR_GREEN COLOR_RED COLOR_YELLOW

# Note: header.sh is sourced by display.sh, no need to source it again
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"
# Color output functions (consistent with main nself.sh)

# Function to format duration
format_duration() {
  local seconds=$1
  local days=$((seconds / 86400))
  local hours=$(((seconds % 86400) / 3600))
  local minutes=$(((seconds % 3600) / 60))
  local secs=$((seconds % 60))

  if [[ $days -gt 0 ]]; then
    echo "${days}d${hours}h${minutes}m"
  elif [[ $hours -gt 0 ]]; then
    echo "${hours}h${minutes}m"
  elif [[ $minutes -gt 0 ]]; then
    echo "${minutes}m${secs}s"
  else
    echo "${secs}s"
  fi
}

# Function to get container stats efficiently
get_container_stats() {
  local container=$1

  local stats=$(docker stats --no-stream --format "{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}" "$container" 2>/dev/null)
  if [[ -n "$stats" ]]; then
    echo "$stats"
  else
    echo "N/A\tN/A\tN/A\tN/A"
  fi
}

# Function to get container info
get_container_info() {
  local service=$1
  local format='{{.Names}}\t{{.Status}}\t{{.State}}\t{{.RunningFor}}\t{{.Ports}}'

  compose ps --format "$format" "$service" 2>/dev/null || echo "$service\tN/A\tN/A\tN/A\tN/A"
}

# Function to check service health efficiently
check_service_health() {
  local service=$1
  # Replace hyphens with underscores in container name (Docker naming convention)
  local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

  # Check if container exists and get state + health in one call
  local container_info=$(docker inspect "$container_name" --format='{{.State.Status}} {{.State.Health.Status}}' 2>/dev/null)

  if [[ -z "$container_info" ]]; then
    echo "stopped"
    return
  fi

  local state=$(echo "$container_info" | awk '{print $1}')
  local health_status=$(echo "$container_info" | awk '{print $2}')

  if [[ "$health_status" == "healthy" ]]; then
    echo "healthy"
  elif [[ "$health_status" == "starting" ]]; then
    echo "starting"
  elif [[ "$health_status" == "unhealthy" ]]; then
    # Container has a health check and it's failing
    echo "unhealthy"
  elif [[ "$health_status" == "<no value>" ]] && [[ "$state" == "running" ]]; then
    # No health check defined, but container is running - treat as healthy
    echo "healthy"
  elif [[ "$state" == "running" ]]; then
    # Running without health check - default to healthy
    echo "healthy"
  else
    echo "$state"
  fi
}

# Function to get database statistics
get_database_stats() {
  if ! compose ps postgres --filter "status=running" >/dev/null 2>&1; then
    echo "Database is not running"
    return
  fi

  local db_name="${POSTGRES_DB:-postgres}"
  local db_user="${POSTGRES_USER:-postgres}"

  # Get database size and connection count with timeout
  local stats=$(safe_timeout 5 compose exec -T postgres psql -U "$db_user" -d "$db_name" -t -c "
        SELECT 
            pg_size_pretty(pg_database_size('$db_name')) as size,
            (SELECT count(*) FROM pg_stat_activity WHERE datname = '$db_name') as connections,
            (SELECT count(*) FROM pg_stat_activity WHERE datname = '$db_name' AND state = 'active') as active_connections
    " 2>/dev/null | sed 's/[\n\r]//g; s/|/ /g' | xargs)

  if [[ -n "$stats" && "$stats" != *"Unable"* ]]; then
    echo "$stats"
  else
    echo "Unable to get database statistics"
  fi
}

# Function to get migration status
get_migration_status() {
  if ! compose ps hasura --filter "status=running" >/dev/null 2>&1; then
    echo "Hasura is not running"
    return
  fi

  # Check if there are pending migrations
  local migration_status=$(compose exec -T hasura hasura-cli migrate status --endpoint "http://localhost:8080" --admin-secret "$HASURA_GRAPHQL_ADMIN_SECRET" 2>/dev/null | grep -E "(Database is up to date|Not Present)" | wc -l || echo "0")

  if [[ $migration_status -gt 0 ]]; then
    echo "Up to date"
  else
    echo "Pending migrations"
  fi
}

# Function to categorize services
categorize_service() {
  local service=$1

  # Infrastructure services (no health checks needed)
  case "$service" in
    nginx | storage | minio | mailhog | mailpit | adminer)
      echo "infrastructure"
      ;;
    postgres | redis)
      echo "database"
      ;;
    hasura | auth)
      echo "core"
      ;;
    *)
      # Check if it's a CS_N custom service
      local n=1
      while [[ -n "$(eval echo "\${CS_${n}:-}")" ]]; do
        local cs_def=$(eval echo "\${CS_${n}:-}")
        IFS=',' read -r cs_name cs_framework <<<"$cs_def"
        cs_name=$(echo "$cs_name" | xargs)
        if [[ "$service" == "$cs_name" ]]; then
          echo "custom"
          return
        fi
        n=$((n + 1))
      done

      # Check legacy CUSTOM_SERVICES
      if [[ -n "${CUSTOM_SERVICES:-}" ]]; then
        IFS=',' read -ra services <<<"${CUSTOM_SERVICES}"
        for svc in "${services[@]}"; do
          IFS=':' read -r svc_name rest <<<"$svc"
          svc_name=$(echo "$svc_name" | xargs)
          if [[ "$service" == "$svc_name" ]]; then
            echo "custom"
            return
          fi
        done
      fi

      echo "application"
      ;;
  esac
}

# Function to get service status description
get_service_status_desc() {
  local service=$1
  local health=$2

  case "$service" in
    postgres)
      [[ "$health" == "healthy" ]] && echo "DB accepting connections" || echo "Database unavailable"
      ;;
    hasura)
      [[ "$health" == "healthy" ]] && echo "GraphQL endpoint responsive" || echo "GraphQL unavailable"
      ;;
    redis)
      [[ "$health" == "healthy" ]] && echo "Cache operational" || echo "Cache unavailable"
      ;;
    auth)
      [[ "$health" == "healthy" ]] && echo "Auth endpoints working" || echo "Auth service down"
      ;;
    nginx)
      echo "Proxy active"
      ;;
    storage | minio)
      echo "S3 compatible storage"
      ;;
    functions)
      [[ "$health" == "healthy" ]] && echo "Functions available" || echo "No health endpoint"
      ;;
    *)
      # Check if it's a CS_N custom service
      local n=1
      while [[ -n "$(eval echo "\${CS_${n}:-}")" ]]; do
        local cs_def=$(eval echo "\${CS_${n}:-}")
        IFS=',' read -r cs_name cs_framework <<<"$cs_def"
        cs_name=$(echo "$cs_name" | xargs)
        cs_framework=$(echo "$cs_framework" | xargs)
        if [[ "$service" == "$cs_name" ]]; then
          if [[ "$health" == "healthy" ]]; then
            echo "Custom ${cs_framework} service"
          elif [[ "$health" == "unhealthy" ]]; then
            echo "${cs_framework} service down"
          else
            echo "${cs_framework} running"
          fi
          return
        fi
        n=$((n + 1))
      done

      # Default behavior for unknown services
      if [[ "$health" == "healthy" ]]; then
        echo "Service healthy"
      elif [[ "$health" == "unhealthy" ]]; then
        echo "Health check failed"
      else
        echo "Running"
      fi
      ;;
  esac
}

# Function to check for configuration drift
check_config_drift() {
  local has_drift=false
  local drift_reasons=()

  # Get modification timestamps
  local compose_mtime=0
  local env_mtime=0
  local last_start_time=0

  # Get docker-compose.yml modification time
  if [[ -f "docker-compose.yml" ]]; then
    compose_mtime=$(safe_stat_mtime "docker-compose.yml" 2>/dev/null || echo "0")
  fi

  # Get .env modification time
  for env_file in .env .env.dev .env.local; do
    if [[ -f "$env_file" ]]; then
      local mtime=$(safe_stat_mtime "$env_file" 2>/dev/null || echo "0")
      if [[ "$mtime" -gt "$env_mtime" ]]; then
        env_mtime="$mtime"
      fi
    fi
  done

  # Get last container start time (use oldest running container)
  local project_name="${PROJECT_NAME:-nself}"
  local container_start=$(docker ps --filter "name=${project_name}_" --format "{{.CreatedAt}}" 2>/dev/null | head -1)
  if [[ -n "$container_start" ]]; then
    # Convert to epoch (simplified check - just compare dates)
    last_start_time=$(date -d "$container_start" +%s 2>/dev/null || echo "0")
    # macOS fallback
    if [[ "$last_start_time" == "0" ]] && [[ "$(uname)" == "Darwin" ]]; then
      last_start_time=$(date -j -f "%Y-%m-%d %H:%M:%S" "$container_start" +%s 2>/dev/null || echo "0")
    fi
  fi

  # Check if compose file changed after containers started
  if [[ "$compose_mtime" -gt "$last_start_time" ]] && [[ "$last_start_time" -gt 0 ]]; then
    has_drift=true
    drift_reasons+=("docker-compose.yml modified since last start")
  fi

  # Check if env changed after containers started
  if [[ "$env_mtime" -gt "$last_start_time" ]] && [[ "$last_start_time" -gt 0 ]]; then
    has_drift=true
    drift_reasons+=(".env file modified since last start")
  fi

  # Check service count mismatch
  local expected_services=$(compose config --services 2>/dev/null | wc -l | tr -d ' ')
  local running_services=$(docker ps --filter "name=${project_name}_" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')

  if [[ "$expected_services" -gt 0 ]] && [[ "$running_services" -gt 0 ]]; then
    if [[ "$expected_services" != "$running_services" ]]; then
      has_drift=true
      drift_reasons+=("Service count mismatch: $running_services running, $expected_services configured")
    fi
  fi

  # Display drift warning if detected
  if [[ "$has_drift" == "true" ]]; then
    echo ""
    printf "${COLOR_YELLOW}⚠ Configuration Drift Detected${COLOR_RESET}\n"
    for reason in "${drift_reasons[@]}"; do
      printf "  ${COLOR_DIM}• %s${COLOR_RESET}\n" "$reason"
    done
    printf "  ${COLOR_DIM}Run: nself build && nself restart${COLOR_RESET}\n"
    echo ""
    return 1
  fi

  return 0
}

# Function to show compact service overview
show_service_overview() {
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found. Run 'nself build' first."
    return
  fi

  # Load environment
  if [[ -f ".env" ]] || [[ -f ".env.local" ]] || [[ -f ".env.dev" ]]; then
    load_env_with_priority
  fi

  # Aggressively remove any lingering init containers on every status check
  cleanup_init_containers 2>/dev/null || true

  # Performance: Use batch Docker calls if in fast mode (v0.9.8)
  if [[ "${NSELF_FAST_MODE:-false}" == "true" ]] && command -v fast_service_overview >/dev/null 2>&1; then
    # Use optimized batch call
    local overview_data=$(fast_service_overview)
    local running=0
    local total=0
    local service_list=()

    while IFS='|' read -r service is_running state; do
      if [[ -n "$service" ]]; then
        total=$((total + 1))
        if [[ "$is_running" == "true" ]]; then
          running=$((running + 1))
          service_list+=("\033[1;32m✓\033[0m $service")
        else
          service_list+=("\033[1;37m○\033[0m $service")
        fi
      fi
    done <<<"$overview_data"

    # Show services header with count
    printf "\033[1;36m→\033[0m Services ($running/$total running)\n"
    echo ""

    # Show all services
    for service_entry in "${service_list[@]}"; do
      printf "$service_entry\n"
    done

    return
  fi

  # Standard mode (original implementation)
  # Get services from compose config, fallback to Docker if config fails
  local services=($(compose config --services 2>/dev/null))

  # If compose config fails, get running containers directly from Docker
  if [[ ${#services[@]} -eq 0 ]]; then
    local project_name="${PROJECT_NAME:-nself}"
    services=($(docker ps -a --filter "name=${project_name}_" --format "{{.Names}}" | sed "s|^${project_name}_||" | sed 's/_[0-9]*$//' | sort -u))
  fi

  # Filter out init containers (anything ending with -init or _init)
  local filtered_services=()
  for service in "${services[@]}"; do
    if [[ ! "$service" =~ ([-_]init)$ ]]; then
      filtered_services+=("$service")
    fi
  done
  services=("${filtered_services[@]}")

  local running=0
  local total=${#services[@]}

  # Sort services in display order
  local sorted_services=($(sort_services "${services[@]}"))

  # Get all container info in one call - fallback to docker ps if compose ps fails
  local all_containers=$(compose ps --format "{{.Service}}\t{{.Status}}\t{{.State}}" 2>/dev/null)

  # If compose ps failed, query docker directly
  local use_docker_fallback=false
  if [[ -z "$all_containers" ]]; then
    use_docker_fallback=true
    local project_name="${PROJECT_NAME:-nself}"
  fi

  # Build service list
  local service_list=()
  local stopped_count=0

  for service in "${sorted_services[@]}"; do
    local health=$(check_service_health "$service")
    local is_running=false

    if [[ "$use_docker_fallback" == "true" ]]; then
      # Query docker directly for container status
      local container_name="${project_name}_${service}"
      local container_status=$(docker ps --filter "name=${container_name}" --format "{{.Status}}" 2>/dev/null)
      if [[ -n "$container_status" ]]; then
        is_running=true
      fi
    else
      # Use compose ps output
      local info=$(echo "$all_containers" | grep "^$service\t" | head -1)
      if [[ -n "$info" ]]; then
        local status=$(echo "$info" | cut -f2)
        if [[ "$status" == *"Up"* ]]; then
          is_running=true
        fi
      fi
    fi

    if [[ "$is_running" == "true" ]]; then
      running=$((running + 1))
      local indicator=""

      # Handle distroless images (Tempo) - they report unhealthy but are actually working
      # Docker healthcheck can't run shell commands in distroless containers
      if [[ "$service" == "tempo" && "$health" == "unhealthy" ]]; then
        health="healthy"
      fi

      # Choose indicator based on health
      if [[ "$health" == "healthy" ]]; then
        indicator="\033[1;32m✓\033[0m" # Green check for healthy
      elif [[ "$health" == "unhealthy" ]]; then
        indicator="\033[1;31m✗\033[0m" # Red X for unhealthy
      elif [[ "$health" == "starting" ]]; then
        indicator="\033[1;33m⟳\033[0m" # Yellow spinner for starting
      else
        indicator="\033[1;36m●\033[0m" # Cyan for no health check
      fi

      service_list+=("$indicator $service")
    else
      stopped_count=$((stopped_count + 1))
      if [[ $stopped_count -le 5 ]]; then
        service_list+=("\033[1;37m○\033[0m $service")
      fi
    fi
  done

  # Show services header with count
  printf "\033[1;36m→\033[0m Services ($running/$total running)\n"
  echo ""

  # Show all services
  for service_entry in "${service_list[@]}"; do
    printf "$service_entry\n"
  done

  if [[ $stopped_count -gt 5 ]]; then
    printf "\033[1;37m...\033[0m +$(($stopped_count - 5)) more stopped\n"
  fi
}

# Function to show compact resource usage
show_resource_usage() {
  if [[ "$SHOW_RESOURCES" != "true" ]]; then
    return
  fi

  local running_services=($(compose ps --services --filter "status=running" 2>/dev/null))
  if [[ ${#running_services[@]} -eq 0 ]]; then
    return
  fi

  show_header "Resource Usage (Top 5)"

  # Get all stats and sort by CPU usage
  local project_name="${PROJECT_NAME:-nself}"
  local container_names=()
  for service in "${running_services[@]}"; do
    container_names+=("${project_name}_${service}")
  done

  if [[ ${#container_names[@]} -gt 0 ]]; then
    local stats_output=$(docker stats --no-stream --format "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" "${container_names[@]}" 2>/dev/null | sort -k2 -nr | head -5)

    if [[ -n "$stats_output" ]]; then
      printf "%-18s %-8s %-20s\n" "SERVICE" "CPU%" "MEMORY"
      echo "──────────────────────────────────────────────"

      echo "$stats_output" | while read -r line; do
        if [[ -n "$line" ]]; then
          local container_name=$(echo "$line" | cut -f1)
          local service=${container_name#${project_name}_}
          local cpu=$(echo "$line" | cut -f2)
          local memory=$(echo "$line" | cut -f3)
          printf "%-18s %-8s %-20s\n" "$service" "$cpu" "$memory"
        fi
      done
    fi
  fi
  echo ""
}

# Function to show compact database info
show_database_info() {
  local postgres_health=$(check_service_health "postgres")
  if [[ "$postgres_health" == "stopped" ]]; then
    return
  fi

  show_header "Database"
  local db_stats=$(get_database_stats)
  if [[ "$db_stats" != *"Unable"* && "$db_stats" != *"not running"* && -n "$db_stats" ]]; then
    local size=$(echo "$db_stats" | awk '{print $1}' 2>/dev/null)
    local connections=$(echo "$db_stats" | awk '{print $2}' 2>/dev/null)

    # Remove pipe characters if present
    size="${size//|/}"
    connections="${connections//|/}"

    if [[ -n "$size" && -n "$connections" ]]; then
      echo "Size: $size • Connections: $connections"
    fi
  fi

  local migration_status=$(safe_timeout 2 bash -c 'get_migration_status' 2>/dev/null || echo "Unknown")
  if [[ "$migration_status" != "Unknown" ]]; then
    echo "Migrations: $migration_status"
  fi
  echo ""
}

# Function to show all available service URLs
show_urls() {
  if [[ ! -f ".env" ]] && [[ ! -f ".env.dev" ]]; then
    return
  fi

  load_env_with_priority
  local base_domain="${BASE_DOMAIN:-local.nself.org}"

  echo ""
  printf "\033[1;36m→\033[0m Service URLs\n"
  echo ""

  # GraphQL API with sub-items
  echo "GraphQL API:    https://api.$base_domain"
  echo " - Console:     https://api.$base_domain/console"

  # Check for remote schemas
  local remote_schema_count=0
  for i in {1..10}; do
    local schema_name_var="REMOTE_SCHEMA_${i}_NAME"
    local schema_name="${!schema_name_var}"
    if [[ -n "$schema_name" ]]; then
      remote_schema_count=$((remote_schema_count + 1))
      local schema_url_var="REMOTE_SCHEMA_${i}_URL"
      local schema_url="${!schema_url_var}"
      echo " - Schema $remote_schema_count:    $schema_url"
    fi
  done

  # Auth service
  echo "Auth:           https://auth.$base_domain"

  # Storage service
  echo "Storage:        https://storage.$base_domain"

  # Functions if enabled
  if [[ "$FUNCTIONS_ENABLED" == "true" ]]; then
    echo "Functions:      https://functions.$base_domain"
  fi

  # Dashboard if enabled
  if [[ "$DASHBOARD_ENABLED" == "true" ]]; then
    echo "Dashboard:      https://dashboard.$base_domain"
  fi

  # Custom APIs
  if [[ "$NESTJS_ENABLED" == "true" ]]; then
    echo "NestJS API:     https://nestjs.$base_domain"
  fi

  if [[ "$GOLANG_ENABLED" == "true" ]]; then
    echo "Golang API:     https://golang.$base_domain"
  fi

  if [[ "$PYTHON_ENABLED" == "true" ]]; then
    echo "Python API:     https://python.$base_domain"
  fi

  # Development tools
  if [[ "$ENV" == "dev" ]] || [[ -z "$ENV" ]]; then
    if [[ "$MAILHOG_ENABLED" != "false" ]] && [[ -n "$(docker ps -q -f name=mailpit)" ]]; then
      echo "Mail UI:        http://localhost:8025"
    fi

    if [[ "$ADMINER_ENABLED" == "true" ]]; then
      echo "Adminer:        https://adminer.$base_domain"
    fi

    echo "MinIO Console:  http://localhost:9001"
  fi
}

# Function for watch mode with improved performance
watch_status() {
  # Trap Ctrl+C for clean exit
  trap 'echo "

Exiting watch mode..."; exit 0' INT

  while true; do

    clear
    echo ""
    printf "\033[1;36mnself Status (Watch Mode)\033[0m • Refresh: ${REFRESH_INTERVAL}s • Ctrl+C to exit\n"
    echo "───────────────────────────────────────────────────────────────────────────"
    echo ""

    show_service_overview
    show_resource_usage
    show_database_info

    log_info "Updated: $(date '+%H:%M:%S') • Next: ${REFRESH_INTERVAL}s"

    sleep "$REFRESH_INTERVAL"
  done
}

# Function to show detailed service info
show_service_detail() {
  local service_name="$1"

  show_header "Detailed Status: $service_name"
  echo ""

  local container_name="${PROJECT_NAME:-nself}_${service_name}"

  # Check if container exists (grep -Fx: fixed-string, exact-line to avoid partial matches)
  if ! docker ps -a --filter "name=$container_name" --format "{{.Names}}" | grep -Fxq "$container_name"; then
    log_error "Service '$service_name' not found"
    log_info "Available services: $(compose config --services 2>/dev/null | xargs)"
    return 1
  fi

  # Basic info
  local info=$(docker inspect "$container_name" --format='{{.State.Status}}\t{{.State.Running}}\t{{.State.StartedAt}}\t{{.State.Health.Status}}' 2>/dev/null)
  local status=$(echo "$info" | cut -f1)
  local running=$(echo "$info" | cut -f2)
  local started=$(echo "$info" | cut -f3)
  local health=$(echo "$info" | cut -f4)

  log_info "Status: $status"
  log_info "Running: $running"
  log_info "Started: $started"
  if [[ "$health" != "<no value>" ]]; then
    log_info "Health: $health"
  fi

  # Resource usage
  echo ""
  show_header "Resource Usage"
  docker stats --no-stream --format "table {{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}" "$container_name" 2>/dev/null || echo "Unable to get resource statistics"

  # Port mappings
  echo ""
  show_header "Port Mappings"
  docker port "$container_name" 2>/dev/null || echo "No port mappings"

  # Recent logs (last 20 lines)
  echo ""
  show_header "Recent Logs (last 20 lines)"
  docker logs --tail 20 "$container_name" 2>&1 | sed 's/^/   /'
}

# Function to show verbose health check details for a service
show_verbose_health() {
  local service=$1
  local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

  # Get health check configuration
  local health_config=$(docker inspect "$container_name" --format='{{json .State.Health}}' 2>/dev/null)

  if [[ -z "$health_config" ]] || [[ "$health_config" == "null" ]]; then
    printf "  ${COLOR_DIM}No health check configured${COLOR_RESET}\n"
    return
  fi

  # Parse health status
  local status=$(echo "$health_config" | grep -o '"Status":"[^"]*"' | cut -d'"' -f4)
  local failing=$(echo "$health_config" | grep -o '"FailingStreak":[0-9]*' | cut -d':' -f2)

  # Get health check test command
  local test_cmd=$(docker inspect "$container_name" --format='{{range .Config.Healthcheck.Test}}{{.}} {{end}}' 2>/dev/null | sed 's/CMD-SHELL //' | sed 's/CMD //')

  # Get health check timing
  local interval=$(docker inspect "$container_name" --format='{{.Config.Healthcheck.Interval}}' 2>/dev/null)
  local timeout=$(docker inspect "$container_name" --format='{{.Config.Healthcheck.Timeout}}' 2>/dev/null)
  local retries=$(docker inspect "$container_name" --format='{{.Config.Healthcheck.Retries}}' 2>/dev/null)

  # Format duration (convert nanoseconds to human-readable)
  format_ns_duration() {
    local ns=$1
    if [[ -n "$ns" ]] && [[ "$ns" != "0" ]]; then
      local secs=$((ns / 1000000000))
      echo "${secs}s"
    else
      echo "default"
    fi
  }

  local interval_fmt=$(format_ns_duration "$interval")
  local timeout_fmt=$(format_ns_duration "$timeout")

  # Display health info
  local status_color="$COLOR_GREEN"
  [[ "$status" == "unhealthy" ]] && status_color="$COLOR_RED"
  [[ "$status" == "starting" ]] && status_color="$COLOR_YELLOW"

  printf "  Health: ${status_color}%s${COLOR_RESET}" "$status"
  [[ -n "$failing" ]] && [[ "$failing" != "0" ]] && printf " (failing streak: %s)" "$failing"
  printf "\n"

  printf "  ${COLOR_DIM}Command: %s${COLOR_RESET}\n" "$test_cmd"
  printf "  ${COLOR_DIM}Interval: %s • Timeout: %s • Retries: %s${COLOR_RESET}\n" "$interval_fmt" "$timeout_fmt" "${retries:-3}"

  # Show last health check log
  local last_log=$(docker inspect "$container_name" --format='{{range $i, $log := .State.Health.Log}}{{if eq $i 0}}{{.Output}}{{end}}{{end}}' 2>/dev/null | head -c 200)
  if [[ -n "$last_log" ]] && [[ "$last_log" != "null" ]]; then
    printf "  ${COLOR_DIM}Last check: %s${COLOR_RESET}\n" "${last_log:0:100}"
  fi
}

# Function to show verbose service overview with health details
show_verbose_service_overview() {
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found. Run 'nself build' first."
    return
  fi

  # Load environment
  if [[ -f ".env" ]] || [[ -f ".env.local" ]] || [[ -f ".env.dev" ]]; then
    load_env_with_priority
  fi

  local services=($(compose config --services 2>/dev/null))

  if [[ ${#services[@]} -eq 0 ]]; then
    local project_name="${PROJECT_NAME:-nself}"
    services=($(docker ps -a --filter "name=${project_name}_" --format "{{.Names}}" | sed "s|^${project_name}_||" | sed 's/_[0-9]*$//' | sort -u))
  fi

  local running=0
  local total=${#services[@]}
  local sorted_services=($(sort_services "${services[@]}"))

  printf "\033[1;36m→\033[0m Services ($total total) - Verbose Health Check Output\n"
  echo ""

  for service in "${sorted_services[@]}"; do
    local health=$(check_service_health "$service")
    local container_name="${PROJECT_NAME:-nself}_${service//-/_}"

    # Check if running
    local is_running=$(docker ps --filter "name=${container_name}" --format "{{.Status}}" 2>/dev/null)

    if [[ -n "$is_running" ]]; then
      running=$((running + 1))
      local indicator=""

      if [[ "$health" == "healthy" ]]; then
        indicator="\033[1;32m✓\033[0m"
      elif [[ "$health" == "unhealthy" ]]; then
        indicator="\033[1;31m✗\033[0m"
      elif [[ "$health" == "starting" ]]; then
        indicator="\033[1;33m⟳\033[0m"
      else
        indicator="\033[1;36m●\033[0m"
      fi

      printf "${indicator} ${COLOR_BOLD}$service${COLOR_RESET}\n"
      show_verbose_health "$service"
      echo ""
    else
      printf "\033[1;37m○\033[0m ${COLOR_DIM}$service (stopped)${COLOR_RESET}\n"
    fi
  done

  echo "Running: $running/$total services"
}

# Function to show help
show_help() {
  echo "nself status - Detailed service status with resource usage"
  echo ""
  echo "Usage: nself status [options] [service]"
  echo ""
  echo "Options:"
  echo "  -d, --detailed        Comprehensive status with HTTP checks and full URLs"
  echo "  -v, --verbose         Show verbose health check details"
  echo "  -w, --watch           Watch mode (refresh every 5s)"
  echo "  -i, --interval N      Set refresh interval for watch mode (default: 5s)"
  echo "  --fast                Fast mode (skip detailed health checks, 3-5x faster)"
  echo "  --all-envs            Show status across all configured environments"
  echo "  --no-resources        Hide resource usage information"
  echo "  --no-health           Hide health check information"
  echo "  --show-ports          Show detailed port information"
  echo "  --json                Output in JSON format"
  echo "  --format FORMAT       Output format: table, json (default: table)"
  echo "  -h, --help            Show this help message"
  echo ""
  echo "Examples:"
  echo "  nself status                    # Show overview of all services"
  echo "  nself status --fast             # Quick status (3-5x faster)"
  echo "  nself status --detailed         # Full status with HTTP checks"
  echo "  nself status -d --json          # Detailed status as JSON"
  echo "  nself status --verbose          # Show detailed health check info"
  echo "  nself status postgres           # Show detailed status of postgres service"
  echo "  nself status --watch            # Watch mode with 5s refresh"
  echo "  nself status -w -i 10           # Watch mode with 10s refresh"
  echo "  nself status --json             # JSON output for tooling"
  echo "  nself status --all-envs         # Check all environments"
  echo ""
  echo "Information shown:"
  echo "  • Service status and health"
  echo "  • Plugin status (installed, running, configured)"
  echo "  • Resource usage (CPU, Memory, Network, Disk I/O)"
  echo "  • Database statistics"
  echo "  • Service URLs"
  echo "  • Migration status"
  echo ""
  echo "Performance:"
  echo "  --fast mode uses batch Docker API calls for 3-5x speed improvement"
}

# Function to output JSON status
output_json_status() {
  load_env_with_priority

  local services=($(compose config --services 2>/dev/null))
  local project_name="${PROJECT_NAME:-nself}"

  printf '{\n  "timestamp": "%s",\n  "environment": "%s",\n  "services": [\n' \
    "$(date -Iseconds)" "${ENV:-local}"

  local first=true
  for service in "${services[@]}"; do
    local health=$(check_service_health "$service")
    local container_name="${project_name}_${service//-/_}"

    # Handle distroless images (Tempo) - they report unhealthy but are actually working
    if [[ "$service" == "tempo" && "$health" == "unhealthy" ]]; then
      health="healthy"
    fi

    # Get container stats
    local stats=$(docker stats --no-stream --format "{{.CPUPerc}}\t{{.MemUsage}}" "$container_name" 2>/dev/null || echo "0%\t0B")
    local cpu=$(echo "$stats" | cut -f1)
    local mem=$(echo "$stats" | cut -f2)

    [[ "$first" != "true" ]] && printf ",\n"
    first=false

    printf '    {"name": "%s", "status": "%s", "cpu": "%s", "memory": "%s"}' \
      "$service" "$health" "$cpu" "$mem"
  done

  printf '\n  ]\n}\n'
}

# HTTP check with timeout
http_check() {
  local url="$1"
  local timeout="${2:-5}"

  # Use curl to check HTTP status
  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time "$timeout" -k "$url" 2>/dev/null || echo "000")

  # Ensure we have a valid HTTP code (empty or invalid -> 000)
  if [[ -z "$http_code" || ! "$http_code" =~ ^[0-9]+$ ]]; then
    http_code="000"
  fi

  # Normalize to 3 digits (in case curl returns something unexpected)
  http_code="${http_code:0:3}"

  echo "$http_code"
}

# Comprehensive detailed status with HTTP checks (FEAT-011)
show_detailed_status() {
  local json_mode="${1:-false}"

  # Load environment
  load_env_with_priority 2>/dev/null || true

  # Aggressively remove any lingering init containers on every status check
  cleanup_init_containers 2>/dev/null || true

  local project_name="${PROJECT_NAME:-nself}"
  local base_domain="${BASE_DOMAIN:-local.nself.org}"
  local env_name="${ENV:-dev}"

  if [[ "$json_mode" == "true" ]]; then
    show_detailed_status_json "$project_name" "$base_domain" "$env_name"
    return
  fi

  # Print header
  printf "\n"
  printf "╔══════════════════════════════════════════════════════════════════════════════╗\n"
  printf "║ %-76s ║\n" "nself status --detailed"
  printf "║ %-76s ║\n" "Project: $project_name | Environment: $env_name | Domain: $base_domain"
  printf "╚══════════════════════════════════════════════════════════════════════════════╝\n"

  local total_healthy=0
  local total_unhealthy=0
  local known_issues=0

  # === CORE SERVICES ===
  printf "\n═══ CORE SERVICES ═══════════════════════════════════════════════════════════\n\n"
  printf "%-16s %-22s %-38s %-6s %s\n" "Service" "Container" "URL" "HTTP" "Status"
  printf "%s\n" "$(printf '%.0s─' {1..95})"

  # PostgreSQL
  local pg_health=$(check_service_health "postgres")
  local pg_status="✓ Healthy"
  [[ "$pg_health" != "healthy" ]] && pg_status="✗ $pg_health" && total_unhealthy=$((total_unhealthy + 1)) || total_healthy=$((total_healthy + 1))
  printf "%-16s %-22s %-38s %-6s %s\n" "PostgreSQL" "${project_name}_postgres" "-" "-" "$pg_status"

  # Hasura
  local hasura_health=$(check_service_health "hasura")
  local hasura_url="https://api.$base_domain"
  local hasura_http=$(http_check "$hasura_url")
  local hasura_status="✓ Healthy"
  [[ "$hasura_health" != "healthy" ]] && hasura_status="✗ $hasura_health" && total_unhealthy=$((total_unhealthy + 1)) || total_healthy=$((total_healthy + 1))
  printf "%-16s %-22s %-38s %-6s %s\n" "Hasura" "${project_name}_hasura" "$hasura_url" "$hasura_http" "$hasura_status"

  local console_http=$(http_check "${hasura_url}/console")
  printf "%-16s %-22s %-38s %-6s %s\n" "  └─ Console" "" "${hasura_url}/console" "$console_http" ""

  # Auth
  local auth_health=$(check_service_health "auth")
  local auth_url="https://auth.$base_domain"
  local auth_http=$(http_check "$auth_url")
  local auth_status="✓ Healthy"
  [[ "$auth_health" != "healthy" ]] && auth_status="✗ $auth_health" && total_unhealthy=$((total_unhealthy + 1)) || total_healthy=$((total_healthy + 1))
  printf "%-16s %-22s %-38s %-6s %s\n" "Auth" "${project_name}_auth" "$auth_url" "$auth_http" "$auth_status"

  # Nginx
  local nginx_health=$(check_service_health "nginx")
  local nginx_status="✓ Healthy"
  [[ "$nginx_health" != "healthy" ]] && nginx_status="✗ $nginx_health" && total_unhealthy=$((total_unhealthy + 1)) || total_healthy=$((total_healthy + 1))
  printf "%-16s %-22s %-38s %-6s %s\n" "Nginx" "${project_name}_nginx" "-" "-" "$nginx_status"

  local core_healthy=$total_healthy
  local core_total=$((total_healthy + total_unhealthy))

  # === OPTIONAL SERVICES ===
  local opt_healthy=0
  local opt_total=0

  printf "\n═══ OPTIONAL SERVICES ═══════════════════════════════════════════════════════\n\n"
  printf "%-16s %-22s %-38s %-6s %s\n" "Service" "Container" "URL" "HTTP" "Status"
  printf "%s\n" "$(printf '%.0s─' {1..95})"

  # Admin
  if [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]]; then
    local admin_health=$(check_service_health "nself-admin")
    local admin_url="https://admin.$base_domain"
    local admin_http=$(http_check "$admin_url")
    local admin_status="✓ Healthy"
    local admin_dev="${NSELF_ADMIN_DEV:-false}"

    if [[ "$admin_dev" == "true" ]]; then
      admin_status="✓ Dev Mode"
      opt_healthy=$((opt_healthy + 1))
      printf "%-16s %-22s %-38s %-6s %s\n" "Admin" "${project_name}_admin" "$admin_url" "$admin_http" "$admin_status"
      printf "%-16s %-22s %-38s %-6s %s\n" "  └─ Mode" "" "DEV (localhost:${NSELF_ADMIN_DEV_PORT:-3025})" "" ""
    else
      [[ "$admin_health" != "healthy" ]] && admin_status="✗ $admin_health" && opt_total=$((opt_total + 1)) || opt_healthy=$((opt_healthy + 1))
      printf "%-16s %-22s %-38s %-6s %s\n" "Admin" "${project_name}_nself-admin" "$admin_url" "$admin_http" "$admin_status"
    fi
    opt_total=$((opt_total + 1))
  fi

  # MinIO/Storage
  if [[ "${MINIO_ENABLED:-false}" == "true" ]]; then
    local minio_health=$(check_service_health "minio")
    local minio_url="https://storage.$base_domain"
    local minio_http=$(http_check "$minio_url")
    local minio_status="✓ Healthy"
    [[ "$minio_health" != "healthy" ]] && minio_status="✗ $minio_health"
    [[ "$minio_health" == "healthy" ]] && opt_healthy=$((opt_healthy + 1))
    opt_total=$((opt_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Storage (MinIO)" "${project_name}_minio" "$minio_url" "$minio_http" "$minio_status"

    local console_http=$(http_check "https://storage-console.$base_domain")
    printf "%-16s %-22s %-38s %-6s %s\n" "  └─ Console" "" "https://storage-console.$base_domain" "$console_http" ""
  fi

  # Redis
  if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
    local redis_health=$(check_service_health "redis")
    local redis_status="✓ Healthy"
    [[ "$redis_health" != "healthy" ]] && redis_status="✗ $redis_health"
    [[ "$redis_health" == "healthy" ]] && opt_healthy=$((opt_healthy + 1))
    opt_total=$((opt_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Redis" "${project_name}_redis" "-" "-" "$redis_status"
  fi

  # MeiliSearch
  if [[ "${MEILISEARCH_ENABLED:-false}" == "true" ]]; then
    local search_health=$(check_service_health "meilisearch")
    local search_url="https://search.$base_domain"
    local search_http=$(http_check "$search_url")
    local search_status="✓ Healthy"
    [[ "$search_health" != "healthy" ]] && search_status="✗ $search_health"
    [[ "$search_health" == "healthy" ]] && opt_healthy=$((opt_healthy + 1))
    opt_total=$((opt_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Search" "${project_name}_meilisearch" "$search_url" "$search_http" "$search_status"
  fi

  # MailPit
  if [[ "${MAILPIT_ENABLED:-false}" == "true" ]]; then
    local mail_health=$(check_service_health "mailpit")
    local mail_url="https://mail.$base_domain"
    local mail_http=$(http_check "$mail_url")
    local mail_status="✓ Healthy"
    [[ "$mail_health" != "healthy" ]] && mail_status="✗ $mail_health"
    [[ "$mail_health" == "healthy" ]] && opt_healthy=$((opt_healthy + 1))
    opt_total=$((opt_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Mail (MailPit)" "${project_name}_mailpit" "$mail_url" "$mail_http" "$mail_status"
  fi

  # MLflow
  if [[ "${MLFLOW_ENABLED:-false}" == "true" ]]; then
    local mlflow_health=$(check_service_health "mlflow")
    local mlflow_url="https://mlflow.$base_domain"
    local mlflow_http=$(http_check "$mlflow_url")
    local mlflow_status="✓ Healthy"
    [[ "$mlflow_health" != "healthy" ]] && mlflow_status="✗ $mlflow_health"
    [[ "$mlflow_health" == "healthy" ]] && opt_healthy=$((opt_healthy + 1))
    opt_total=$((opt_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "MLflow" "${project_name}_mlflow" "$mlflow_url" "$mlflow_http" "$mlflow_status"
  fi

  # Functions
  if [[ "${FUNCTIONS_ENABLED:-false}" == "true" ]]; then
    local fn_health=$(check_service_health "functions")
    local fn_url="https://functions.$base_domain"
    local fn_http=$(http_check "$fn_url")
    local fn_status="✓ Healthy"
    [[ "$fn_health" != "healthy" ]] && fn_status="✗ $fn_health"
    [[ "$fn_health" == "healthy" ]] && opt_healthy=$((opt_healthy + 1))
    opt_total=$((opt_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Functions" "${project_name}_functions" "$fn_url" "$fn_http" "$fn_status"
  fi

  if [[ $opt_total -eq 0 ]]; then
    printf "  ${COLOR_DIM}No optional services enabled${COLOR_RESET}\n"
  fi

  # === MONITORING STACK ===
  local mon_healthy=0
  local mon_total=0

  if [[ "${MONITORING_ENABLED:-false}" == "true" ]]; then
    printf "\n═══ MONITORING STACK ════════════════════════════════════════════════════════\n\n"
    printf "%-16s %-22s %-38s %-6s %s\n" "Service" "Container" "URL" "HTTP" "Status"
    printf "%s\n" "$(printf '%.0s─' {1..95})"

    # Grafana
    local grafana_health=$(check_service_health "grafana")
    local grafana_url="https://grafana.$base_domain"
    local grafana_http=$(http_check "$grafana_url")
    local grafana_status="✓ Healthy"
    [[ "$grafana_health" != "healthy" ]] && grafana_status="✗ $grafana_health"
    [[ "$grafana_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Grafana" "${project_name}_grafana" "$grafana_url" "$grafana_http" "$grafana_status"

    # Prometheus
    local prom_health=$(check_service_health "prometheus")
    local prom_url="https://prometheus.$base_domain"
    local prom_http=$(http_check "$prom_url")
    local prom_status="✓ Healthy"
    [[ "$prom_health" != "healthy" ]] && prom_status="✗ $prom_health"
    [[ "$prom_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Prometheus" "${project_name}_prometheus" "$prom_url" "$prom_http" "$prom_status"

    # Loki
    local loki_health=$(check_service_health "loki")
    local loki_status="✓ Healthy"
    [[ "$loki_health" != "healthy" ]] && loki_status="✗ $loki_health"
    [[ "$loki_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Loki" "${project_name}_loki" "-" "-" "$loki_status"

    # Promtail
    local promtail_health=$(check_service_health "promtail")
    local promtail_status="✓ Healthy"
    [[ "$promtail_health" != "healthy" ]] && promtail_status="✗ $promtail_health"
    [[ "$promtail_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Promtail" "${project_name}_promtail" "-" "-" "$promtail_status"

    # Tempo (distroless image - treat as healthy if running)
    local tempo_health=$(check_service_health "tempo")
    local tempo_status="✓ Healthy"
    local tempo_note=""
    if [[ "$tempo_health" == "unhealthy" ]]; then
      # Tempo uses distroless image, healthcheck fails but service works
      tempo_status="✓ Healthy"
      tempo_note="  └─ Note: Docker healthcheck disabled (distroless image)"
    elif [[ "$tempo_health" != "healthy" ]]; then
      tempo_status="✗ $tempo_health"
    fi
    [[ "$tempo_health" == "healthy" || "$tempo_health" == "unhealthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Tempo" "${project_name}_tempo" "-" "-" "$tempo_status"
    [[ -n "$tempo_note" ]] && printf "%s\n" "$tempo_note"

    # Alertmanager
    local alert_health=$(check_service_health "alertmanager")
    local alert_status="✓ Healthy"
    [[ "$alert_health" != "healthy" ]] && alert_status="✗ $alert_health"
    [[ "$alert_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Alertmanager" "${project_name}_alertmanager" "-" "-" "$alert_status"

    # cAdvisor
    local cadvisor_health=$(check_service_health "cadvisor")
    local cadvisor_status="✓ Healthy"
    [[ "$cadvisor_health" != "healthy" ]] && cadvisor_status="✗ $cadvisor_health"
    [[ "$cadvisor_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "cAdvisor" "${project_name}_cadvisor" "-" "-" "$cadvisor_status"

    # Node Exporter
    local node_health=$(check_service_health "node-exporter")
    local node_status="✓ Healthy"
    [[ "$node_health" != "healthy" ]] && node_status="✗ $node_health"
    [[ "$node_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Node Exporter" "${project_name}_node_exporter" "-" "-" "$node_status"

    # Postgres Exporter
    local pgexp_health=$(check_service_health "postgres-exporter")
    local pgexp_status="✓ Healthy"
    [[ "$pgexp_health" != "healthy" ]] && pgexp_status="✗ $pgexp_health"
    [[ "$pgexp_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
    mon_total=$((mon_total + 1))
    printf "%-16s %-22s %-38s %-6s %s\n" "Postgres Export" "${project_name}_postgres_exp" "-" "-" "$pgexp_status"

    # Redis Exporter
    if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
      local redisexp_health=$(check_service_health "redis-exporter")
      local redisexp_status="✓ Healthy"
      [[ "$redisexp_health" != "healthy" ]] && redisexp_status="✗ $redisexp_health"
      [[ "$redisexp_health" == "healthy" ]] && mon_healthy=$((mon_healthy + 1))
      mon_total=$((mon_total + 1))
      printf "%-16s %-22s %-38s %-6s %s\n" "Redis Exporter" "${project_name}_redis_exp" "-" "-" "$redisexp_status"
    fi

  fi

  # === CUSTOM SERVICES ===
  local cs_healthy=0
  local cs_total=0
  local has_custom=false

  for i in {1..10}; do
    local cs_def
    cs_def=$(eval echo "\${CS_${i}:-}")
    if [[ -n "$cs_def" ]]; then
      if [[ "$has_custom" == "false" ]]; then
        printf "\n═══ CUSTOM SERVICES ═════════════════════════════════════════════════════════\n\n"
        printf "%-16s %-22s %-38s %-6s %s\n" "Service" "Container" "URL" "HTTP" "Status"
        printf "%s\n" "$(printf '%.0s─' {1..95})"
        has_custom=true
      fi

      # Parse CS_N format: name,template or name:template:port
      local cs_name cs_route cs_port
      IFS=':,' read -r cs_name cs_template cs_port <<<"$cs_def"
      cs_name=$(echo "$cs_name" | xargs)

      local cs_health=$(check_service_health "$cs_name")
      local cs_status="✓ Healthy"
      [[ "$cs_health" != "healthy" ]] && cs_status="✗ $cs_health"
      [[ "$cs_health" == "healthy" ]] && cs_healthy=$((cs_healthy + 1))
      cs_total=$((cs_total + 1))

      # Try to find route for custom service
      local cs_route_var="CS_${i}_ROUTE"
      cs_route=$(eval echo "\${$cs_route_var:-}")
      local cs_url="-"
      local cs_http="-"
      if [[ -n "$cs_route" ]]; then
        cs_url="https://${cs_route}.$base_domain"
        cs_http=$(http_check "$cs_url")
      fi

      printf "%-16s %-22s %-38s %-6s %s\n" "$cs_name (CS_$i)" "${project_name}_$cs_name" "$cs_url" "$cs_http" "$cs_status"
    fi
  done

  # === FRONTEND APPS ===
  local fe_total=0
  local fe_responding=0
  local has_frontend=false

  for i in {1..10}; do
    local fe_name fe_port fe_route
    fe_name=$(eval echo "\${FRONTEND_APP_${i}_NAME:-}")
    if [[ -z "$fe_name" ]]; then
      fe_name=$(eval echo "\${APP_${i}_NAME:-}")
    fi

    if [[ -n "$fe_name" ]]; then
      if [[ "$has_frontend" == "false" ]]; then
        printf "\n═══ FRONTEND APPS ═══════════════════════════════════════════════════════════\n\n"
        printf "%-16s %-8s %-38s %-6s %s\n" "App" "Port" "URL" "HTTP" "Status"
        printf "%s\n" "$(printf '%.0s─' {1..95})"
        has_frontend=true
      fi

      fe_port=$(eval echo "\${FRONTEND_APP_${i}_PORT:-}")
      [[ -z "$fe_port" ]] && fe_port=$(eval echo "\${APP_${i}_PORT:-}")
      fe_route=$(eval echo "\${FRONTEND_APP_${i}_ROUTE:-}")
      [[ -z "$fe_route" ]] && fe_route=$(eval echo "\${APP_${i}_ROUTE:-$fe_name}")

      local fe_url="https://${fe_route}.$base_domain"
      local fe_http=$(http_check "$fe_url")
      local fe_status="✓ Running"

      if [[ "$fe_http" == "000" ]]; then
        fe_status="✗ Not responding"
      elif [[ "$fe_http" == "timeout" ]]; then
        fe_status="⚠ Slow"
        fe_responding=$((fe_responding + 1))
      else
        fe_responding=$((fe_responding + 1))
      fi

      fe_total=$((fe_total + 1))
      printf "%-16s %-8s %-38s %-6s %s\n" "$fe_name" "$fe_port" "$fe_url" "$fe_http" "$fe_status"
    fi
  done

  # === SUMMARY ===
  printf "\n═══ SUMMARY ═════════════════════════════════════════════════════════════════\n\n"

  local backend_total=$((core_total + opt_total + mon_total + cs_total))
  local backend_healthy=$((core_healthy + opt_healthy + mon_healthy + cs_healthy))

  printf "Backend:  %d/%d containers" "$backend_healthy" "$backend_total"
  if [[ $known_issues -gt 0 ]]; then
    printf " (%d healthy, %d known issue)" "$((backend_healthy - known_issues))" "$known_issues"
  fi
  printf "\n"

  if [[ $fe_total -gt 0 ]]; then
    printf "Frontend: %d/%d ports configured (%d responding)\n" "$fe_total" "$fe_total" "$fe_responding"
  fi

  printf "URLs:     All services accessible via https://*.%s\n" "$base_domain"
  printf "\nLast check: %s\n\n" "$(date '+%Y-%m-%d %H:%M:%S')"
}

# JSON output for detailed status
show_detailed_status_json() {
  local project_name="$1"
  local base_domain="$2"
  local env_name="$3"

  printf '{\n'
  printf '  "project": "%s",\n' "$project_name"
  printf '  "environment": "%s",\n' "$env_name"
  printf '  "domain": "%s",\n' "$base_domain"
  printf '  "timestamp": "%s",\n' "$(date -Iseconds 2>/dev/null || date +%Y-%m-%dT%H:%M:%S)"
  printf '  "services": {\n'

  # Core services
  printf '    "core": [\n'
  local pg_health=$(check_service_health "postgres")
  local hasura_health=$(check_service_health "hasura")
  local hasura_http=$(http_check "https://api.$base_domain")
  local auth_health=$(check_service_health "auth")
  local auth_http=$(http_check "https://auth.$base_domain")
  local nginx_health=$(check_service_health "nginx")

  printf '      {"name": "postgres", "healthy": %s},\n' "$([[ "$pg_health" == "healthy" ]] && echo "true" || echo "false")"
  printf '      {"name": "hasura", "url": "https://api.%s", "http_code": %s, "healthy": %s},\n' "$base_domain" "$hasura_http" "$([[ "$hasura_health" == "healthy" ]] && echo "true" || echo "false")"
  printf '      {"name": "auth", "url": "https://auth.%s", "http_code": %s, "healthy": %s},\n' "$base_domain" "$auth_http" "$([[ "$auth_health" == "healthy" ]] && echo "true" || echo "false")"
  printf '      {"name": "nginx", "healthy": %s}\n' "$([[ "$nginx_health" == "healthy" ]] && echo "true" || echo "false")"
  printf '    ]\n'

  printf '  }\n'
  printf '}\n'
}

# Function to check status across all environments
check_all_envs_status() {
  local json_mode="${JSON_OUTPUT:-false}"

  # Find all environment files
  local env_files=()
  [[ -f ".env" ]] && env_files+=("local:.env")
  [[ -f ".env.dev" ]] && env_files+=("dev:.env.dev")
  [[ -f ".env.staging" ]] && env_files+=("staging:.env.staging")
  [[ -f ".env.prod" ]] && env_files+=("prod:.env.prod")
  [[ -f ".env.production" ]] && env_files+=("production:.env.production")

  # Check .environments directory
  if [[ -d ".environments" ]]; then
    for env_dir in .environments/*/; do
      local env_name=$(basename "$env_dir")
      if [[ -f "${env_dir}server.json" ]]; then
        env_files+=("${env_name}:${env_dir}server.json")
      fi
    done
  fi

  if [[ "$json_mode" == "true" ]]; then
    printf '{\n  "timestamp": "%s",\n  "environments": [\n' "$(date -Iseconds)"
  else
    show_command_header "nself status" "All Environments Status"
    echo ""
    printf "  %-15s %-15s %-12s %s\n" "Environment" "Type" "Status" "Details"
    printf "  %-15s %-15s %-12s %s\n" "-----------" "----" "------" "-------"
  fi

  local first=true
  for entry in "${env_files[@]}"; do
    local env_name="${entry%%:*}"
    local env_file="${entry#*:}"
    local env_type="local"
    local status="unknown"
    local details=""

    # Determine if local or remote
    if [[ "$env_file" == *"server.json"* ]]; then
      env_type="remote"
      # Try to get server info
      if [[ -f "$env_file" ]]; then
        local host=$(grep '"host"' "$env_file" 2>/dev/null | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
        if [[ -n "$host" ]]; then
          # Quick ping check
          if ping -c 1 -W 2 "$host" >/dev/null 2>&1; then
            status="reachable"
            details="$host"
          else
            status="unreachable"
            details="$host"
          fi
        fi
      fi
    else
      env_type="local"
      # Check local Docker status
      if docker ps >/dev/null 2>&1; then
        local running=$(docker ps -q 2>/dev/null | wc -l | tr -d ' ')
        if [[ "$running" -gt 0 ]]; then
          status="running"
          details="$running containers"
        else
          status="stopped"
          details="no containers"
        fi
      else
        status="docker-unavailable"
      fi
    fi

    if [[ "$json_mode" == "true" ]]; then
      [[ "$first" != "true" ]] && printf ",\n"
      first=false
      printf '    {"name": "%s", "type": "%s", "status": "%s", "details": "%s"}' \
        "$env_name" "$env_type" "$status" "$details"
    else
      local status_color="$COLOR_GREEN"
      [[ "$status" == "unreachable" || "$status" == "stopped" ]] && status_color="$COLOR_RED"
      [[ "$status" == "unknown" ]] && status_color="$COLOR_YELLOW"

      printf "  %-15s %-15s ${status_color}%-12s${COLOR_RESET} %s\n" \
        "$env_name" "$env_type" "$status" "$details"
    fi
  done

  if [[ "$json_mode" == "true" ]]; then
    printf '\n  ]\n}\n'
  fi
}

# Show plugin status
show_plugin_status() {
  local plugin_dir="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"

  # Check if plugin directory exists
  if [[ ! -d "$plugin_dir" ]]; then
    return 0
  fi

  # Find installed plugins
  local plugins=()
  for plugin_path in "$plugin_dir"/*/plugin.json; do
    if [[ -f "$plugin_path" ]]; then
      local plugin_name
      plugin_name=$(dirname "$plugin_path")
      plugin_name=$(basename "$plugin_name")

      # Skip shared utilities
      if [[ "$plugin_name" != "_shared" ]]; then
        plugins+=("$plugin_name")
      fi
    fi
  done

  # No plugins installed
  if [[ ${#plugins[@]} -eq 0 ]]; then
    return 0
  fi

  echo ""
  printf "\033[1;36m→\033[0m Plugins (%d installed)\n" "${#plugins[@]}"
  echo ""

  for plugin_name in "${plugins[@]}"; do
    local plugin_json="$plugin_dir/$plugin_name/plugin.json"
    local version=""
    local indicator=""
    local status_text=""

    # Get version
    if command -v jq >/dev/null 2>&1; then
      version=$(jq -r '.version // "?"' "$plugin_json" 2>/dev/null)
    else
      version=$(grep '"version"' "$plugin_json" 2>/dev/null | head -1 | sed 's/.*: *"\([^"]*\)".*/\1/')
    fi

    # Check if plugin is configured (has required env vars)
    local is_configured=true
    case "$plugin_name" in
      stripe)
        [[ -z "${STRIPE_API_KEY:-}" ]] && is_configured=false
        ;;
      github)
        [[ -z "${GITHUB_TOKEN:-}" ]] && is_configured=false
        ;;
      shopify)
        [[ -z "${SHOPIFY_ACCESS_TOKEN:-}" || -z "${SHOPIFY_STORE:-}" ]] && is_configured=false
        ;;
    esac

    # Check running status (if runtime.sh is loaded)
    local is_running=false
    local plugin_port=""
    if command -v is_plugin_running >/dev/null 2>&1; then
      if is_plugin_running "$plugin_name" 2>/dev/null; then
        is_running=true
        local _env_file="$plugin_dir/$plugin_name/ts/.env"
        if [[ -f "$_env_file" ]]; then
          plugin_port=$(grep "^PORT=" "$_env_file" | cut -d= -f2)
        fi
      fi
    fi

    if [[ "$is_running" == "true" ]]; then
      indicator="\033[1;32m✓\033[0m"
      if [[ -n "$plugin_port" ]]; then
        status_text="running on port $plugin_port"
      else
        status_text="running"
      fi
    elif [[ "$is_configured" == "true" ]]; then
      indicator="\033[1;33m○\033[0m"
      status_text="installed, not running"
    else
      indicator="\033[1;31m✗\033[0m"
      status_text="not configured"
    fi

    printf "%b %s v%s (%s)\n" "$indicator" "$plugin_name" "$version" "$status_text"
  done
}

# Main function
main() {
  local service_name=""
  local json_mode=false
  local all_envs=false
  local verbose_mode=false
  local detailed_mode=false
  local fast_mode=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case $1 in
      -d | --detailed | --full)
        detailed_mode=true
        shift
        ;;
      -v | --verbose)
        verbose_mode=true
        shift
        ;;
      -w | --watch)
        WATCH_MODE=true
        shift
        ;;
      -i | --interval)
        REFRESH_INTERVAL="$2"
        shift 2
        ;;
      --fast)
        fast_mode=true
        shift
        ;;
      --all-envs)
        all_envs=true
        shift
        ;;
      --no-resources)
        SHOW_RESOURCES=false
        shift
        ;;
      --no-health)
        SHOW_HEALTH=false
        shift
        ;;
      --show-ports)
        SHOW_PORTS=true
        shift
        ;;
      --json)
        json_mode=true
        JSON_OUTPUT=true
        shift
        ;;
      --format)
        OUTPUT_FORMAT="$2"
        [[ "$OUTPUT_FORMAT" == "json" ]] && json_mode=true && JSON_OUTPUT=true
        shift 2
        ;;
      -h | --help)
        show_help
        exit 0
        ;;
      -*)
        log_error "Unknown option: $1"
        log_info "Use 'nself status --help' for usage information"
        exit 1
        ;;
      *)
        service_name="$1"
        shift
        ;;
    esac
  done

  # Fast mode configuration (Performance: v0.9.8)
  if [[ "$fast_mode" == "true" ]]; then
    export NSELF_FAST_MODE=true
    export SHOW_RESOURCES=false  # Skip resource checks in fast mode
  fi

  # Handle --all-envs mode
  if [[ "$all_envs" == "true" ]]; then
    check_all_envs_status
    exit 0
  fi

  # Check if docker-compose.yml exists
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "docker-compose.yml not found"
    log_info "Run 'nself build' to generate project structure"
    exit 1
  fi

  # Handle detailed mode (FEAT-011)
  if [[ "$detailed_mode" == "true" ]]; then
    show_detailed_status "$json_mode"
    exit 0
  fi

  # Load environment
  if [[ -f ".env" ]] || [[ -f ".env.local" ]] || [[ -f ".env.dev" ]]; then
    load_env_with_priority
  fi

  # Handle specific service detail view
  if [[ -n "$service_name" ]]; then
    show_service_detail "$service_name"
    exit 0
  fi

  # Handle watch mode
  if [[ "$WATCH_MODE" == "true" ]]; then
    watch_status
    exit 0
  fi

  # Handle JSON output mode
  if [[ "$json_mode" == "true" ]]; then
    output_json_status
    exit 0
  fi

  # Handle verbose mode
  if [[ "$verbose_mode" == "true" ]]; then
    show_command_header "nself status" "Verbose health check output" ""
    echo
    show_verbose_service_overview
    exit 0
  fi

  # Default compact overview mode
  show_command_header "nself status" "Service health and resource monitoring" ""
  echo

  show_service_overview

  # Show plugin status if any plugins are installed
  show_plugin_status

  # Check for configuration drift
  check_config_drift

  # Show legend right after services
  echo ""
  printf "\033[1;32m✓\033[0m Healthy  \033[1;31m✗\033[0m Unhealthy  \033[1;36m●\033[0m Running  \033[1;33m⟳\033[0m Starting  \033[1;37m○\033[0m Stopped\n"

  echo ""
  echo "nself status <service> | nself status --watch"
  echo "nself urls | nself logs <service> | nself doctor"
  echo
}

# Run main function
main "$@"
