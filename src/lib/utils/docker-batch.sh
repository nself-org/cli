#!/usr/bin/env bash

# docker-batch.sh - Batch Docker API calls for performance
# Part of nself v0.9.8 - Performance Optimization
# POSIX-compliant, no Bash 4+ features

# Batch get container info for multiple containers
# Returns: JSON array with container info
batch_get_container_info() {

set -euo pipefail

  local project_name="${PROJECT_NAME:-nself}"

  # Get all containers in one call
  docker ps -a \
    --filter "name=${project_name}_" \
    --format '{{.Names}}|{{.Status}}|{{.State}}|{{.RunningFor}}|{{.Ports}}' \
    2>/dev/null
}

# Batch get container health for all services
# Returns: name|health status pairs
batch_get_container_health() {
  local project_name="${PROJECT_NAME:-nself}"

  # Get health status for all containers in one inspect call
  docker ps -a \
    --filter "name=${project_name}_" \
    --format "{{.Names}}" \
    2>/dev/null | while read -r container; do
      if [[ -n "$container" ]]; then
        # Get state and health in single inspect
        local info=$(docker inspect "$container" \
          --format='{{.State.Status}}|{{.State.Health.Status}}' 2>/dev/null)

        if [[ -n "$info" ]]; then
          local state=$(echo "$info" | cut -d'|' -f1)
          local health=$(echo "$info" | cut -d'|' -f2)

          # Determine final health
          if [[ "$health" == "healthy" ]]; then
            echo "$container|healthy"
          elif [[ "$health" == "starting" ]]; then
            echo "$container|starting"
          elif [[ "$health" == "unhealthy" ]]; then
            echo "$container|unhealthy"
          elif [[ "$health" == "<no value>" ]] && [[ "$state" == "running" ]]; then
            echo "$container|healthy"  # No healthcheck, but running
          elif [[ "$state" == "running" ]]; then
            echo "$container|healthy"
          else
            echo "$container|$state"
          fi
        fi
      fi
    done
}

# Batch get stats for all running containers
# Returns: name|cpu|memory pairs
batch_get_container_stats() {
  local project_name="${PROJECT_NAME:-nself}"

  # Get all running containers
  local containers=$(docker ps \
    --filter "name=${project_name}_" \
    --format "{{.Names}}" \
    2>/dev/null | tr '\n' ' ')

  if [[ -z "$containers" ]]; then
    return 0
  fi

  # Get stats in single call (no-stream for speed)
  docker stats --no-stream \
    --format "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}" \
    $containers 2>/dev/null
}

# Fast container count by status
fast_count_containers() {
  local status="${1:-running}"  # running, stopped, all
  local project_name="${PROJECT_NAME:-nself}"

  case "$status" in
    running)
      docker ps --filter "name=${project_name}_" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' '
      ;;
    stopped)
      docker ps -a --filter "name=${project_name}_" --filter "status=exited" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' '
      ;;
    all)
      docker ps -a --filter "name=${project_name}_" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' '
      ;;
    *)
      echo "0"
      ;;
  esac
}

# Get all service names from running containers (fast)
fast_get_services() {
  local project_name="${PROJECT_NAME:-nself}"

  docker ps -a \
    --filter "name=${project_name}_" \
    --format "{{.Names}}" \
    2>/dev/null | sed "s|^${project_name}_||" | sort -u
}

# Batch check if services are running
# Args: service names as arguments
# Returns: service|status pairs
batch_check_services_running() {
  local project_name="${PROJECT_NAME:-nself}"

  # Build filter for multiple services
  for service in "$@"; do
    local container_name="${project_name}_${service//-/_}"
    docker ps --filter "name=${container_name}" --format "{{.Names}}|running" 2>/dev/null
  done
}

# Get service uptime in seconds (batch mode compatible)
get_service_uptime() {
  local container="$1"

  local started_at=$(docker inspect "$container" \
    --format='{{.State.StartedAt}}' 2>/dev/null)

  if [[ -n "$started_at" ]]; then
    # Convert to epoch
    local start_epoch=$(date -d "$started_at" +%s 2>/dev/null || \
      date -j -f "%Y-%m-%dT%H:%M:%S" "$started_at" +%s 2>/dev/null || echo "0")
    local now_epoch=$(date +%s)

    if [[ "$start_epoch" -gt 0 ]]; then
      echo $((now_epoch - start_epoch))
    else
      echo "0"
    fi
  else
    echo "0"
  fi
}

# Optimized service overview (single docker call)
fast_service_overview() {
  local project_name="${PROJECT_NAME:-nself}"

  # Get all info in parallel
  local all_info=$(docker ps -a \
    --filter "name=${project_name}_" \
    --format "{{.Names}}|{{.Status}}|{{.State}}" \
    2>/dev/null)

  # Parse and categorize
  echo "$all_info" | while IFS='|' read -r name status state; do
    if [[ -n "$name" ]]; then
      local service=${name#${project_name}_}
      local running="false"

      if [[ "$status" == *"Up"* ]]; then
        running="true"
      fi

      printf "%s|%s|%s\n" "$service" "$running" "$state"
    fi
  done
}

# Export functions
export -f batch_get_container_info
export -f batch_get_container_health
export -f batch_get_container_stats
export -f fast_count_containers
export -f fast_get_services
export -f batch_check_services_running
export -f get_service_uptime
export -f fast_service_overview
