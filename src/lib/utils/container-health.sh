#!/bin/bash


# Container health checking utilities

# Check if a container is in a restart loop
is_container_restarting() {

set -euo pipefail

  local container_name="$1"

  # Get restart count and status
  local restart_count=$(docker inspect --format='{{.RestartCount}}' "$container_name" 2>/dev/null)
  local container_status=$(docker inspect --format='{{.State.Status}}' "$container_name" 2>/dev/null)

  # If status is literally "restarting", it's restarting
  if [[ "$container_status" == "restarting" ]]; then
    return 0
  fi

  # Check if it has restarted recently (high restart count)
  if [[ $restart_count -gt 0 ]]; then
    # Get the last started time
    local last_started=$(docker inspect --format='{{.State.StartedAt}}' "$container_name" 2>/dev/null)
    local current_time=$(date -u +%s)
    local started_timestamp=$(date -d "$last_started" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "${last_started%%.*}" +%s 2>/dev/null)

    if [[ -n "$started_timestamp" ]]; then
      local time_since_start=$((current_time - started_timestamp))

      # If restarted in last 30 seconds with restart count > 1, likely in a loop
      if [[ $time_since_start -lt 30 ]] && [[ $restart_count -gt 1 ]]; then
        return 0
      fi
    fi
  fi

  return 1
}

# Get comprehensive container health status
get_container_health() {
  local container_name="$1"

  # Check if container exists
  if ! docker inspect "$container_name" >/dev/null 2>&1; then
    echo "not_found"
    return
  fi

  # Get basic status
  local container_status=$(docker inspect --format='{{.State.Status}}' "$container_name" 2>/dev/null)

  # Check for restart loop
  if is_container_restarting "$container_name"; then
    echo "restarting"
    return
  fi

  # If running, check health status
  if [[ "$container_status" == "running" ]]; then
    local health_status=$(docker inspect --format='{{.State.Health.Status}}' "$container_name" 2>/dev/null)

    if [[ -n "$health_status" ]] && [[ "$health_status" != "<no value>" ]]; then
      echo "$health_status"
    else
      echo "running"
    fi
  else
    echo "$container_status"
  fi
}

# Check all services in a project
check_project_health() {
  local project_name="${1:-nself}"
  local verbose="${2:-false}"

  local all_healthy=true
  local unhealthy_list=""
  local restarting_list=""
  local stopped_list=""

  # Get all containers for this project
  local containers=$(docker ps -a --filter "name=^${project_name}_" --format "{{.Names}}" 2>/dev/null)

  for container in $containers; do
    local service_name="${container#${project_name}_}"
    local health=$(get_container_health "$container")

    case "$health" in
      healthy | running)
        [[ "$verbose" == "true" ]] && echo "✓ $service_name: $health"
        ;;
      restarting)
        all_healthy=false
        restarting_list="$restarting_list $service_name"
        [[ "$verbose" == "true" ]] && echo "↻ $service_name: restarting"
        ;;
      unhealthy | starting)
        all_healthy=false
        unhealthy_list="$unhealthy_list $service_name"
        [[ "$verbose" == "true" ]] && echo "✱ $service_name: $health"
        ;;
      exited | stopped | dead)
        all_healthy=false
        stopped_list="$stopped_list $service_name"
        [[ "$verbose" == "true" ]] && echo "✗ $service_name: $health"
        ;;
      *)
        all_healthy=false
        unhealthy_list="$unhealthy_list $service_name"
        [[ "$verbose" == "true" ]] && echo "? $service_name: $health"
        ;;
    esac
  done

  # Return results
  if [[ "$all_healthy" == "true" ]]; then
    return 0
  else
    # Export problem lists for caller
    export RESTARTING_SERVICES="$restarting_list"
    export UNHEALTHY_SERVICES="$unhealthy_list"
    export STOPPED_SERVICES="$stopped_list"
    return 1
  fi
}

# Export functions
export -f is_container_restarting
export -f get_container_health
export -f check_project_health
