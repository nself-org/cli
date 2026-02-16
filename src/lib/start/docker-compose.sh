#!/usr/bin/env bash

# docker-compose.sh - Docker Compose operations for nself start
# Bash 3.2 compatible, cross-platform

# Source error messages library (namespaced to avoid clobbering caller's SCRIPT_DIR)
_START_COMPOSE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${_START_COMPOSE_DIR}/../utils/error-messages.sh" 2>/dev/null || true

# Get the appropriate docker compose command
get_compose_command() {
  # Prefer docker compose v2
  if docker compose version >/dev/null 2>&1; then
    echo "docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    echo "docker-compose"
  else
    echo "docker compose" # Assume v2
  fi
}

# Build docker compose command with all flags
build_compose_command() {
  local env_file="${1:-.env}"
  local project="${2:-nself}"
  local detached="${3:-true}"
  local build="${4:-true}"

  local compose_cmd=$(get_compose_command)
  local cmd="$compose_cmd --project-name \"$project\" --env-file \"$env_file\""

  if [ "$build" = "true" ]; then
    cmd="$cmd up --build"
  else
    # Use --no-build to prevent hanging on build errors
    cmd="$cmd up --no-build"
  fi

  if [ "$detached" = "true" ]; then
    cmd="$cmd -d"
  fi

  echo "$cmd"
}

# Execute docker compose with progress monitoring
execute_compose_with_progress() {
  local compose_cmd="$1"
  local project="${2:-nself}"
  local timeout="${3:-300}" # 5 minutes default for image downloads
  local verbose="${4:-false}"

  local output_file=$(mktemp)
  local services_to_start=0
  local compose_bin=$(get_compose_command)

  # Get expected service count
  services_to_start=$($compose_bin --project-name "$project" --env-file ".env.runtime" config --services 2>/dev/null | wc -l | tr -d ' ')

  if [ "$verbose" = "true" ]; then
    # Show full output with timeout
    printf "\n"
    if command -v timeout >/dev/null 2>&1; then
      timeout "$timeout" sh -c "$compose_cmd" 2>&1 | tee "$output_file"
    else
      # SECURITY: Use bash -c instead of raw eval for compose commands
      bash -c "$compose_cmd" 2>&1 | tee "$output_file" &
      local cmd_pid=$!
      sleep "$timeout"
      kill -TERM $cmd_pid 2>/dev/null
    fi
    local result=${PIPESTATUS[0]}
  else
    # Run with absolute timeout - don't let it hang
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Starting Docker services...\n"

    # Start compose in background - use bash -c to preserve quoting
    bash -c "$compose_cmd" >"$output_file" 2>&1 &
    local compose_pid=$!

    # Simple progress indicator with hard timeout
    local elapsed=0
    local spin_chars="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
    local i=0

    while [ $elapsed -lt $timeout ]; do
      # Check if process is still running
      if ! kill -0 $compose_pid 2>/dev/null; then
        # Process finished
        wait $compose_pid
        local result=$?
        break
      fi

      # Show progress
      local char="${spin_chars:$((i % ${#spin_chars})):1}"
      local running_count=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
      local total_count=$(docker ps -a --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')

      # Check if images are being pulled (show different message)
      if [ "$total_count" -eq 0 ] && [ $elapsed -lt 30 ]; then
        printf "\r${COLOR_BLUE}%s${COLOR_RESET} Pulling Docker images..." "$char"
      elif [ "$running_count" -lt "$total_count" ]; then
        printf "\r${COLOR_BLUE}%s${COLOR_RESET} Starting containers: %d/%d running..." "$char" "$running_count" "$services_to_start"
      else
        printf "\r${COLOR_BLUE}%s${COLOR_RESET} Progress: %d/%d services running..." "$char" "$running_count" "$services_to_start"
      fi

      i=$((i + 1))
      elapsed=$((elapsed + 1))
      sleep 1
    done

    # Kill if still running after timeout
    if kill -0 $compose_pid 2>/dev/null; then
      printf "\n${COLOR_YELLOW}⚠${COLOR_RESET}  Compose taking longer than expected (may be downloading images)...\n"
      # Give it more time if services are still being created
      local extra_wait=60
      while [ $extra_wait -gt 0 ] && kill -0 $compose_pid 2>/dev/null; do
        local current_count=$(docker ps -a --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
        printf "\r${COLOR_BLUE}⠋${COLOR_RESET} Still starting... %d services created" "$current_count"
        sleep 5
        extra_wait=$((extra_wait - 5))
      done

      if kill -0 $compose_pid 2>/dev/null; then
        printf "\n${COLOR_YELLOW}⚠${COLOR_RESET}  Forcing completion...\n"
        kill -TERM $compose_pid 2>/dev/null
        sleep 2
        kill -KILL $compose_pid 2>/dev/null
      fi
      local result=124 # Timeout exit code
    fi
  fi

  # Always ensure created containers are started
  printf "\n${COLOR_BLUE}⠋${COLOR_RESET} Ensuring all containers are started...\n"
  start_created_containers "$project"

  # Use docker compose start as fallback - this ensures all defined services are started
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Starting all services via compose...\n"
  $compose_bin --project-name "$project" --env-file ".env.runtime" start 2>/dev/null || true

  # One more attempt to start created containers after compose start
  local still_created=$(docker ps -a --filter "label=com.docker.compose.project=$project" --filter "status=created" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
  if [ "$still_created" -gt 0 ]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Final attempt to start %d created containers...\n" "$still_created"
    start_created_containers "$project"
  fi

  rm -f "$output_file"

  # Return success if we have running containers
  local running_count=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
  if [ "$running_count" -gt 0 ]; then
    return 0
  fi

  return ${result:-1}
}

# Try to start services without building (fallback for build failures)
execute_compose_without_build() {
  local env_file="${1:-.env}"
  local project="${2:-nself}"
  local timeout="${3:-300}"
  local verbose="${4:-false}"

  printf "${COLOR_BLUE}⠋${COLOR_RESET} Attempting to start pre-built services...\n"

  # Build command without --build flag
  local compose_cmd=$(get_compose_command)
  local cmd="$compose_cmd --project-name \"$project\" --env-file \"$env_file\" up -d"

  local output_file=$(mktemp)

  if [ "$verbose" = "true" ]; then
    printf "\n"
    # SECURITY: Use bash -c instead of raw eval for compose commands
    bash -c "$cmd" 2>&1 | tee "$output_file"
    local result=${PIPESTATUS[0]}
  else
    (bash -c "$cmd" 2>&1) >"$output_file" &
    local compose_pid=$!

    # Monitor with shorter timeout for fallback
    monitor_compose_progress "$compose_pid" "$output_file" "$project" "0" "$timeout"
    local result=$?
  fi

  rm -f "$output_file"
  return $result
}

# Monitor docker compose progress
monitor_compose_progress() {
  local compose_pid="$1"
  local output_file="$2"
  local project="$3"
  local expected_services="${4:-0}"
  local timeout="${5:-600}"

  local spin_chars="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
  local i=0
  local elapsed=0
  local last_message=""

  while kill -0 $compose_pid 2>/dev/null; do
    if [ $elapsed -ge $timeout ]; then
      printf "\r${COLOR_RED}✗${COLOR_RESET} Timeout after ${timeout}s                  \n"
      kill $compose_pid 2>/dev/null
      return 1
    fi

    # Get current running count
    local current_count=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')

    # Parse output for status
    local status_msg=""
    if [ -f "$output_file" ] && [ $elapsed -gt 1 ]; then
      # Check for buildkit build progress (new format)
      if tail -20 "$output_file" 2>/dev/null | grep -q "internal.*load\|transferring dockerfile\|load metadata"; then
        status_msg=" - Building images"
      # Check for pulling
      elif tail -20 "$output_file" 2>/dev/null | grep -q "Pulling"; then
        local image=$(tail -20 "$output_file" | grep "Pulling" | tail -1 | awk '{print $1}')
        status_msg=" - Pulling $image"
      # Check for building (classic format)
      elif tail -20 "$output_file" 2>/dev/null | grep -q "Building"; then
        local service=$(tail -20 "$output_file" | grep "Building" | tail -1 | awk '{print $2}')
        status_msg=" - Building $service"
      # Check for creating
      elif tail -20 "$output_file" 2>/dev/null | grep -q "Creating"; then
        local container=$(tail -20 "$output_file" | grep "Creating" | tail -1 | awk '{print $2}')
        status_msg=" - Creating $container"
      # Check for starting
      elif tail -20 "$output_file" 2>/dev/null | grep -q "Starting"; then
        status_msg=" - Starting containers"
      fi
    fi

    # Display progress
    local char="${spin_chars:$((i % ${#spin_chars})):1}"

    if [ -n "$status_msg" ]; then
      printf "\r${COLOR_BLUE}%s${COLOR_RESET} Starting services...%s          " "$char" "$status_msg"
    elif [ $expected_services -gt 0 ] && [ $current_count -gt 0 ]; then
      printf "\r${COLOR_BLUE}%s${COLOR_RESET} Starting services... (%d/%d)          " "$char" "$current_count" "$expected_services"
    else
      printf "\r${COLOR_BLUE}%s${COLOR_RESET} Initializing services...          " "$char"
    fi

    i=$((i + 1))
    elapsed=$((elapsed + 1))
    sleep 1

    # Check for stuck state - no change in container count for 15 seconds
    if [ $elapsed -gt 10 ]; then
      if [ "$current_count" -eq "${last_count:-0}" ]; then
        stuck_time=$((${stuck_time:-0} + 1))
        if [ $stuck_time -gt 15 ]; then
          printf "\r${COLOR_YELLOW}⚠${COLOR_RESET}  Process appears stuck, attempting recovery...       \n"
          kill -TERM $compose_pid 2>/dev/null
          sleep 2
          kill -KILL $compose_pid 2>/dev/null
          start_created_containers "$project"
          return 0
        fi
      else
        stuck_time=0
        last_count=$current_count
      fi
    else
      last_count=$current_count
    fi

    # Check for build failures in output (common error patterns)
    if [ -f "$output_file" ] && [ $elapsed -gt 10 ]; then
      if tail -50 "$output_file" 2>/dev/null | grep -q "ERROR\|failed.*build\|build.*failed\|Error response from daemon\|port is already allocated"; then
        printf "\r${COLOR_YELLOW}⚠${COLOR_RESET}  Errors detected - attempting recovery...              \n"
        kill $compose_pid 2>/dev/null
        return 2 # Special code for build failures
      fi
    fi
  done

  wait $compose_pid
  local compose_exit_code=$?

  # Ensure all containers in "Created" state are started
  start_created_containers "$project"

  # Check if essential services are running regardless of compose exit code
  local running_count=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')

  # If we have running services, consider it successful even if compose had warnings/errors
  if [ "$running_count" -gt 0 ]; then
    return 0
  else
    return $compose_exit_code
  fi
}

# Start containers that are in "Created" state but not running
start_created_containers() {
  local project="${1:-nself}"

  # Get containers in Created state
  local created_containers=$(docker ps -a --filter "label=com.docker.compose.project=$project" --filter "status=created" --format "{{.Names}}" 2>/dev/null)

  if [ -n "$created_containers" ]; then
    # Try to start all created containers, handling port conflicts
    for container in $created_containers; do
      # Try to start the container
      if ! docker start "$container" >/dev/null 2>&1; then
        # If failed, check if it's a port conflict
        local error_msg=$(docker start "$container" 2>&1)
        if echo "$error_msg" | grep -q "port is already allocated"; then
          # Extract the port number
          local port=$(echo "$error_msg" | grep -oE ":[0-9]+" | tail -1 | tr -d ':')
          if [ -n "$port" ]; then
            # Try to find what's using the port
            local conflicting=$(docker ps --format "{{.Names}}" --filter "publish=$port" 2>/dev/null | head -1)
            local process=""
            if command -v lsof >/dev/null 2>&1; then
              process=$(lsof -iTCP:$port -sTCP:LISTEN 2>/dev/null | tail -1 | awk '{print $1}')
            fi

            # Show helpful error message
            if [ -n "$conflicting" ] && [ "$conflicting" != "$container" ]; then
              # It's another Docker container
              printf "\n" >&2
              printf "${COLOR_YELLOW}⚠${COLOR_RESET}  Port conflict: Container '%s' cannot start\n" "$container" >&2
              printf "${COLOR_DIM}   Port %s is used by container '%s'${COLOR_RESET}\n" "$port" "$conflicting" >&2
              printf "${COLOR_DIM}   Stopping conflicting container...${COLOR_RESET}\n" >&2
              docker stop "$conflicting" >/dev/null 2>&1
              sleep 1
              docker start "$container" >/dev/null 2>&1 || true
            else
              # It's an external process or we can't start it
              printf "\n" >&2
              show_port_conflict_error "$port" "$container" "$process" >&2
              printf "${COLOR_DIM}   Removing container - please fix port conflict and restart${COLOR_RESET}\n" >&2
              docker rm -f "$container" >/dev/null 2>&1
            fi
          fi
        fi
      fi
    done

    # Give them a moment to start
    sleep 2
  fi
}

# Check if all services are healthy
check_services_health() {
  local project="${1:-nself}"
  local max_wait="${2:-60}"
  local elapsed=0
  local spin_chars="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
  local i=0

  while [ $elapsed -lt $max_wait ]; do
    # Get service status counts
    local total=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" | wc -l | tr -d ' ')
    local healthy=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Status}}" | grep -c "(healthy)" 2>/dev/null || echo "0")
    local unhealthy=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Status}}" | grep -c "(unhealthy)" 2>/dev/null || echo "0")
    local starting=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Status}}" | grep -c "starting" 2>/dev/null || echo "0")
    local restarting=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Status}}" | grep -c "Restarting" 2>/dev/null || echo "0")

    # Clean up counts
    healthy=$(echo "$healthy" | tr -d ' \n\r')
    unhealthy=$(echo "$unhealthy" | tr -d ' \n\r')
    starting=$(echo "$starting" | tr -d ' \n\r')
    restarting=$(echo "$restarting" | tr -d ' \n\r')

    # Show progress with colored status
    local char="${spin_chars:$((i % ${#spin_chars})):1}"
    printf "\r${COLOR_BLUE}%s${COLOR_RESET} Health check: " "$char"

    # Show colored counts
    if [ "$healthy" -gt 0 ]; then
      printf "${COLOR_GREEN}%d healthy${COLOR_RESET}" "$healthy"
    fi

    if [ "$starting" -gt 0 ]; then
      [ "$healthy" -gt 0 ] && printf ", "
      printf "${COLOR_YELLOW}%d starting${COLOR_RESET}" "$starting"
    fi

    if [ "$unhealthy" -gt 0 ]; then
      [ "$healthy" -gt 0 ] || [ "$starting" -gt 0 ] && printf ", "
      printf "${COLOR_RED}%d unhealthy${COLOR_RESET}" "$unhealthy"
    fi

    if [ "$restarting" -gt 0 ]; then
      [ "$healthy" -gt 0 ] || [ "$starting" -gt 0 ] || [ "$unhealthy" -gt 0 ] && printf ", "
      printf "${COLOR_RED}%d restarting${COLOR_RESET}" "$restarting"
    fi

    printf " (total: %d)      " "$total"

    # If all are healthy or none are unhealthy/restarting and none are starting, we're good
    if [ "$unhealthy" -eq 0 ] && [ "$starting" -eq 0 ] && [ "$restarting" -eq 0 ]; then
      printf "\r${COLOR_GREEN}✓${COLOR_RESET} All %d services healthy                                     \n" "$total"
      return 0
    fi

    sleep 2
    elapsed=$((elapsed + 2))
    i=$((i + 1))
  done

  # Final status if timeout
  printf "\r${COLOR_YELLOW}⚠${COLOR_RESET}  Timeout - "
  printf "${COLOR_GREEN}%d healthy${COLOR_RESET}" "$healthy"
  [ "$unhealthy" -gt 0 ] && printf ", ${COLOR_RED}%d unhealthy${COLOR_RESET}" "$unhealthy"
  [ "$restarting" -gt 0 ] && printf ", ${COLOR_RED}%d restarting${COLOR_RESET}" "$restarting"
  printf " (total: %d)    \n" "$total"

  return 1
}

# Show detailed service status with colors
show_service_details() {
  local project="${1:-nself}"
  local show_all="${2:-false}"

  # Get problematic services
  local unhealthy=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}:{{.Status}}" 2>/dev/null | grep -E "unhealthy|Restarting")

  if [ -n "$unhealthy" ] || [ "$show_all" = "true" ]; then
    echo ""
    echo "${COLOR_CYAN}Service Details:${COLOR_RESET}"
    echo ""

    # Show unhealthy/restarting services first
    if [ -n "$unhealthy" ]; then
      echo "${COLOR_RED}Issues detected:${COLOR_RESET}"
      echo "$unhealthy" | while IFS=':' read -r name status; do
        printf "  ${COLOR_RED}✗${COLOR_RESET} %-30s %s\n" "$name" "$status"

        # Show last log line for failed services
        if echo "$status" | grep -q "Restarting"; then
          local last_log=$(docker logs "$name" 2>&1 | tail -1)
          if [ -n "$last_log" ]; then
            printf "    ${COLOR_GRAY}└─ %s${COLOR_RESET}\n" "${last_log:0:60}"
          fi
        fi
      done
      echo ""
    fi

    # Show healthy services if requested
    if [ "$show_all" = "true" ]; then
      echo "${COLOR_GREEN}Healthy services:${COLOR_RESET}"
      docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}:{{.Status}}" 2>/dev/null |
        grep "healthy" | while IFS=':' read -r name status; do
        printf "  ${COLOR_GREEN}✓${COLOR_RESET} %-30s %s\n" "$name" "${status:0:40}"
      done
    fi
  fi
}

# Stop services
stop_services() {
  local project="${1:-nself}"
  local compose_cmd=$(get_compose_command)

  $compose_cmd --project-name "$project" down
}

# Restart specific service
restart_service() {
  local service="$1"
  local project="${2:-nself}"
  local compose_cmd=$(get_compose_command)

  $compose_cmd --project-name "$project" restart "$service"
}
