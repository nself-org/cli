#!/usr/bin/env bash

# docker.sh - Centralized Docker utilities

# Source display utilities
UTILS_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$UTILS_DIR/display.sh" 2>/dev/null || true

# Ensure Docker is running
ensure_docker_running() {
  if ! docker info >/dev/null 2>&1; then
    log_error "Docker is not running"
    log_info "Please start Docker Desktop or run: sudo systemctl start docker"
    return 1
  fi
  return 0
}

# Docker Compose wrapper - enforces v2 and consistent options
compose() {
  # Load environment if not already loaded
  if [[ -z "${PROJECT_NAME:-}" ]]; then
    if [[ -f ".env.dev" ]]; then
      set -a
      source ".env.dev" 2>/dev/null || true
      set +a
    fi
    if [[ -f ".env" ]]; then
      set -a
      source ".env" 2>/dev/null || true
      set +a
    fi
    if [[ -f ".env.local" ]]; then
      set -a
      source ".env.local" 2>/dev/null || true
      set +a
    fi
  fi

  # Try .env.runtime first (merged runtime config), then .env.local, then .env, then .env.dev
  local env_file="${COMPOSE_ENV_FILE:-}"
  if [[ -z "$env_file" ]] && [[ -f ".env.runtime" ]]; then
    env_file=".env.runtime"
  fi
  if [[ -z "$env_file" ]] && [[ -f ".env.local" ]]; then
    env_file=".env.local"
  fi
  if [[ -z "$env_file" ]] && [[ -f ".env" ]]; then
    env_file=".env"
  fi
  if [[ -z "$env_file" ]] && [[ -f ".env.dev" ]]; then
    env_file=".env.dev"
  fi

  local project="${PROJECT_NAME:-nself}"
  local compose_files=""

  # Always use main docker-compose.yml
  compose_files="-f docker-compose.yml"

  # Add custom services if exists
  if [[ -f "docker-compose.custom.yml" ]]; then
    compose_files="$compose_files -f docker-compose.custom.yml"
  fi

  # Add override if exists
  if [[ -f "docker-compose.override.yml" ]]; then
    compose_files="$compose_files -f docker-compose.override.yml"
  fi

  # Debug mode - show command being executed
  if [[ "${NSELF_DEBUG:-false}" == "true" ]]; then
    if [[ -f "$env_file" ]]; then
      printf "${COLOR_DIM}[DEBUG] docker compose %s --project-name \"%s\" --env-file \"%s\" %s${COLOR_RESET}\n" \
        "$compose_files" "$project" "$env_file" "$*" >&2
    else
      printf "${COLOR_DIM}[DEBUG] docker compose %s --project-name \"%s\" %s${COLOR_RESET}\n" \
        "$compose_files" "$project" "$*" >&2
    fi
  fi

  if [[ -f "$env_file" ]]; then
    docker compose $compose_files --project-name "$project" --env-file "$env_file" "$@"
  else
    docker compose $compose_files --project-name "$project" "$@"
  fi
}

# Get project name
project_name() {
  echo "${PROJECT_NAME:-nself}"
}

# Sort services in display order
# Priority: Core → Optional → Monitoring → Custom
sort_services() {
  local services=("$@")

  # Define priority order
  local -a core=(postgres hasura auth nginx)
  local -a optional=(nself-admin minio storage redis functions mailpit mailhog meilisearch typesense sonic mlflow)
  local -a monitoring_priority=(prometheus grafana loki promtail tempo alertmanager)
  local -a monitoring_exporters=(cadvisor node-exporter postgres-exporter redis-exporter)

  local -a sorted=()

  # 1. Core services (in order)
  for svc in "${core[@]}"; do
    if [[ " ${services[*]} " =~ " ${svc} " ]]; then
      sorted+=("$svc")
    fi
  done

  # 2. Optional services (in order)
  for svc in "${optional[@]}"; do
    if [[ " ${services[*]} " =~ " ${svc} " ]]; then
      sorted+=("$svc")
    fi
  done

  # 3. Monitoring services (by priority)
  for svc in "${monitoring_priority[@]}"; do
    if [[ " ${services[*]} " =~ " ${svc} " ]]; then
      sorted+=("$svc")
    fi
  done

  # 4. Monitoring exporters (alphabetically)
  for svc in "${monitoring_exporters[@]}"; do
    if [[ " ${services[*]} " =~ " ${svc} " ]]; then
      sorted+=("$svc")
    fi
  done

  # 5. Custom services (CS_*) - alphabetically
  local -a custom=()
  for svc in "${services[@]}"; do
    # Check if it's a custom service (not in any predefined list)
    if [[ ! " ${sorted[*]} " =~ " ${svc} " ]]; then
      custom+=("$svc")
    fi
  done

  # Sort custom services alphabetically
  if [[ ${#custom[@]} -gt 0 ]]; then
    IFS=$'\n' custom=($(sort <<<"${custom[*]}"))
    unset IFS
    sorted+=("${custom[@]}")
  fi

  # Output sorted services
  printf "%s\n" "${sorted[@]}"
}

# Get container name for a service
container_name() {
  local service="$1"
  echo "$(project_name)_${service}_1"
}

# Check if service is running
is_service_running() {
  local service="$1"
  compose ps --services --filter "status=running" 2>/dev/null | grep -qx "$service"
}

# Get service health status
service_health() {
  local service="$1"
  local container=$(container_name "$service")

  local status=$(docker inspect "$container" --format='{{.State.Health.Status}}' 2>/dev/null)

  if [[ -z "$status" ]]; then
    # No health check defined, check if running
    if docker inspect "$container" --format='{{.State.Status}}' 2>/dev/null | grep -q "running"; then
      echo "running"
    else
      echo "stopped"
    fi
  else
    echo "$status"
  fi
}

# Wait for service to be healthy
wait_service_healthy() {
  local service="$1"
  local timeout="${2:-60}"
  local start_time=$(date +%s)

  log_info "Waiting for $service to be healthy..."

  while [[ $(($(date +%s) - start_time)) -lt $timeout ]]; do
    local health=$(service_health "$service")

    case "$health" in
      healthy | running)
        log_success "$service is healthy"
        return 0
        ;;
      starting)
        sleep 2
        ;;
      unhealthy | stopped)
        log_error "$service is $health"
        return 1
        ;;
    esac
  done

  log_warning "Timeout waiting for $service to be healthy"
  return 1
}

# Kill containers using a port (only our project)
kill_port_if_ours() {
  local port="$1"
  local project=$(project_name)

  # Find containers from our project using this port
  local containers=$(docker ps --filter "publish=$port" --format '{{.Names}}' | grep -E "^${project}_" || true)

  if [[ -n "$containers" ]]; then
    log_info "Stopping our containers using port $port"
    if [ -n "$containers" ]; then echo "$containers" | xargs docker stop >/dev/null 2>&1 || true; fi
    if [ -n "$containers" ]; then echo "$containers" | xargs docker rm >/dev/null 2>&1 || true; fi
    return 0
  fi

  return 1
}

# Check if port is available
is_port_available() {
  local port="$1"

  # Check with multiple methods for compatibility
  if command -v lsof >/dev/null 2>&1; then
    ! lsof -i ":$port" >/dev/null 2>&1
  elif command -v netstat >/dev/null 2>&1; then
    ! netstat -tln 2>/dev/null | grep -q ":$port "
  elif command -v ss >/dev/null 2>&1; then
    ! ss -tln 2>/dev/null | grep -q ":$port "
  else
    # Fallback: try to bind to the port
    ! nc -z localhost "$port" 2>/dev/null
  fi
}

# Get all ports used by our project
ports_in_use_for_project() {
  local project=$(project_name)
  docker ps --filter "name=${project}_" --format '{{.Ports}}' |
    grep -oE '[0-9]+' | sort -u
}

# Clean up stopped containers from our project
cleanup_stopped_containers() {
  local project=$(project_name)
  local containers=$(docker ps -a --filter "name=${project}_" --filter "status=exited" --format '{{.Names}}')

  if [[ -n "$containers" ]]; then
    log_info "Cleaning up stopped containers"
    if [ -n "$containers" ]; then echo "$containers" | xargs docker rm >/dev/null 2>&1 || true; fi
  fi
}

# Get Docker Compose config
get_compose_config() {
  compose config 2>/dev/null
}

# Validate Docker Compose file
validate_compose_file() {
  if compose config -q 2>/dev/null; then
    return 0
  else
    log_error "Docker Compose configuration is invalid"
    compose config 2>&1 | head -20
    return 1
  fi
}

# Cleanup exited init containers
# Init containers (minio-init, meilisearch-init, etc.) are meant to run once and exit
# They clutter Docker Desktop and should be aggressively removed after they complete
cleanup_init_containers() {
  local project="${PROJECT_NAME:-$(project_name)}"

  # Primary: label-based cleanup — catches all nself init containers regardless of naming
  # Do NOT filter by status=exited; also catch created/dead containers and force-remove
  local label_containers
  label_containers=$(docker ps -a \
    --filter "label=nself.type=init-container" \
    --format "{{.Names}}" 2>/dev/null || true)

  # Secondary: name-pattern fallback — catches containers without the label
  # Match both underscore and hyphen separators, and Docker Compose v2 naming
  # e.g. project-minio-init-1, project_minio-init_1, project-meilisearch-init-1
  local name_containers
  name_containers=$(docker ps -a --format "{{.Names}}" 2>/dev/null \
    | grep -E "^${project}[-_].*([-_]init[-_]|-init-)[0-9]*$|^${project}[-_].*[-_]init$" || true)

  # Combine, deduplicate, remove empty lines
  local all_containers
  all_containers=$(printf '%s\n%s\n' "$label_containers" "$name_containers" \
    | sort -u | grep -v '^$' || true)

  if [[ -n "$all_containers" ]]; then
    # Force-remove so they never linger in Docker Desktop
    echo "$all_containers" | xargs docker rm -f >/dev/null 2>&1 || true
  fi
}

# Wait for init containers to finish, then aggressively remove them
# Called after docker compose up to ensure init containers don't linger
wait_and_cleanup_init_containers() {
  local project="${PROJECT_NAME:-$(project_name)}"
  local max_wait="${1:-30}"

  # Find any init containers that are still running
  local running_init
  running_init=$(docker ps \
    --filter "label=nself.type=init-container" \
    --filter "status=running" \
    --format "{{.Names}}" 2>/dev/null || true)

  if [[ -z "$running_init" ]]; then
    # Also check by name pattern for containers without the label
    running_init=$(docker ps --format "{{.Names}}" 2>/dev/null \
      | grep -E "^${project}[-_].*([-_]init[-_]|-init-)[0-9]*$|^${project}[-_].*[-_]init$" || true)
  fi

  if [[ -n "$running_init" ]]; then
    # Wait for running init containers to exit (they should be quick)
    local waited=0
    while [[ $waited -lt $max_wait ]]; do
      local still_running
      still_running=$(docker ps \
        --filter "label=nself.type=init-container" \
        --filter "status=running" \
        --format "{{.Names}}" 2>/dev/null || true)
      if [[ -z "$still_running" ]]; then
        break
      fi
      sleep 1
      waited=$((waited + 1))
    done
  fi

  # Now aggressively remove all init containers regardless of state
  cleanup_init_containers
}

# Export all functions
export -f ensure_docker_running compose project_name container_name
export -f is_service_running service_health wait_service_healthy
export -f kill_port_if_ours is_port_available ports_in_use_for_project
export -f cleanup_stopped_containers cleanup_init_containers wait_and_cleanup_init_containers get_compose_config validate_compose_file
