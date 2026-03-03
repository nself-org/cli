#!/usr/bin/env bash
# start.sh - Professional start command with clean progress indicators
# Matches the style of nself build command

set -euo pipefail

# Trap unexpected exits and print context so failures are never silent
trap '_rc=$?; if [[ $_rc -ne 0 ]]; then printf "\n[nself start] unexpected exit (code %s) at line %s\n" "$_rc" "${LINENO:-?}" >&2; fi' EXIT


# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$SCRIPT_DIR/../lib"

# Source only essential utilities
source "$LIB_DIR/utils/display.sh"
source "$LIB_DIR/utils/env.sh"
source "$LIB_DIR/utils/docker.sh"
source "$LIB_DIR/utils/error-messages.sh" 2>/dev/null || true

# Source security module for secure-by-default enforcement
source "$LIB_DIR/security/secure-defaults.sh" 2>/dev/null || true

# Source plugin runtime for auto-start support
source "$LIB_DIR/plugin/runtime.sh" 2>/dev/null || true

# Source pre-checks (port conflict detection, environment checks)
source "$LIB_DIR/start/pre-checks.sh" 2>/dev/null || true

# Smart defaults from environment variables
HEALTH_CHECK_TIMEOUT="${NSELF_HEALTH_CHECK_TIMEOUT:-120}"
HEALTH_CHECK_INTERVAL="${NSELF_HEALTH_CHECK_INTERVAL:-2}"
HEALTH_CHECK_REQUIRED="${NSELF_HEALTH_CHECK_REQUIRED:-80}"
SKIP_HEALTH_CHECKS="${NSELF_SKIP_HEALTH_CHECKS:-false}"
START_MODE="${NSELF_START_MODE:-smart}"
CLEANUP_ON_START="${NSELF_CLEANUP_ON_START:-auto}"

# Validate ranges for health check settings
if [[ $HEALTH_CHECK_TIMEOUT -lt 30 ]] || [[ $HEALTH_CHECK_TIMEOUT -gt 600 ]]; then
  HEALTH_CHECK_TIMEOUT=120
fi
if [[ $HEALTH_CHECK_INTERVAL -le 0 ]] || [[ $HEALTH_CHECK_INTERVAL -gt 10 ]]; then
  HEALTH_CHECK_INTERVAL=2
fi
if [[ $HEALTH_CHECK_REQUIRED -lt 0 ]] || [[ $HEALTH_CHECK_REQUIRED -gt 100 ]]; then
  HEALTH_CHECK_REQUIRED=80
fi

# Parse arguments first
VERBOSE=false
DEBUG=false
SHOW_HELP=false
SKIP_HEALTH=false
FORCE_RECREATE=false
SKIP_PORT_CHECK=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v | --verbose)
      VERBOSE=true
      shift
      ;;
    -d | --debug)
      DEBUG=true
      VERBOSE=true # Debug implies verbose
      shift
      ;;
    -h | --help)
      SHOW_HELP=true
      shift
      ;;
    --skip-health-checks)
      SKIP_HEALTH_CHECKS=true
      shift
      ;;
    --timeout)
      if [[ -z "$2" ]] || ! [[ "$2" =~ ^[0-9]+$ ]]; then
        echo "Error: --timeout requires a numeric value" >&2
        exit 1
      fi
      HEALTH_CHECK_TIMEOUT="$2"
      shift 2
      ;;
    --fresh | --force-recreate)
      START_MODE="fresh"
      shift
      ;;
    --clean-start)
      CLEANUP_ON_START="always"
      shift
      ;;
    --skip-port-check)
      SKIP_PORT_CHECK=true
      shift
      ;;
    --quick)
      HEALTH_CHECK_TIMEOUT=30
      HEALTH_CHECK_REQUIRED=60
      shift
      ;;
    *)
      shift
      ;;
  esac
done

# Show help if requested
if [[ "$SHOW_HELP" == "true" ]]; then
  echo "Usage: nself start [OPTIONS]"
  echo ""
  echo "Start all services defined in docker-compose.yml"
  echo ""
  echo "Options:"
  echo "  -v, --verbose           Show detailed Docker output"
  echo "  -d, --debug            Show debug information and detailed output"
  echo "  -h, --help             Show this help message"
  echo "  --skip-health-checks   Skip health check validation"
  echo "  --timeout N            Set health check timeout in seconds (default: $HEALTH_CHECK_TIMEOUT)"
  echo "  --fresh                Force recreate all containers"
  echo "  --clean-start          Remove all containers before starting"
  echo "  --quick                Quick start with relaxed health checks"
  echo "  --skip-port-check      Skip pre-flight port availability check"
  echo ""
  echo "Environment Variables (optional):"
  echo "  NSELF_START_MODE              Start mode: smart, fresh, force (default: smart)"
  echo "  NSELF_HEALTH_CHECK_TIMEOUT    Health check timeout seconds (default: 120)"
  echo "  NSELF_HEALTH_CHECK_REQUIRED   Percent services required healthy (default: 80)"
  echo "  NSELF_SKIP_HEALTH_CHECKS      Skip health validation (default: false)"
  echo ""
  echo "Examples:"
  echo "  nself start                  # Start with smart defaults"
  echo "  nself start -v               # Start with verbose output"
  echo "  nself start --quick          # Quick start for development"
  echo "  nself start --fresh          # Force recreate all containers"
  echo "  nself start --timeout 180    # Wait up to 3 minutes for health"
  exit 0
fi

# ============================================================
# AUTOMATIC DATABASE READINESS (runs EVERY start)
# ============================================================
# The Golden Rule: Users should NEVER SSH into servers.
# This function ensures database is ready automatically.
# ============================================================

ensure_database_ready() {
  local project_name="$1"
  local max_wait="${2:-60}"
  local db_user="${POSTGRES_USER:-postgres}"
  local db_name="${POSTGRES_DB:-nhost}"
  local db_password="${POSTGRES_PASSWORD:-}"

  local container_name="${project_name}_postgres"

  # Check if postgres container exists
  if ! docker ps --format "{{.Names}}" | grep -q "^${container_name}"; then
    # Try alternate naming pattern
    container_name="${project_name}-postgres-1"
    if ! docker ps --format "{{.Names}}" | grep -q "^${container_name}"; then
      return 0 # No postgres container, skip
    fi
  fi

  printf "  ${COLOR_BLUE}⠋${COLOR_RESET} Ensuring database is ready..."

  # Step 1: Wait for PostgreSQL to accept connections
  local waited=0
  while [[ $waited -lt $max_wait ]]; do
    if docker exec "$container_name" pg_isready -U "$db_user" >/dev/null 2>&1; then
      break
    fi
    sleep 1
    waited=$((waited + 1))
    # Update spinner
    local spinners=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
    printf "\r  ${COLOR_BLUE}%s${COLOR_RESET} Waiting for PostgreSQL... (%ds)" "${spinners[$((waited % 10))]}" "$waited"
  done

  if [[ $waited -ge $max_wait ]]; then
    printf "\r  ${COLOR_YELLOW}!${COLOR_RESET} PostgreSQL not ready after ${max_wait}s (will retry)\n"
    return 0 # Don't fail, let health checks handle it
  fi

  # Step 2: Ensure database exists
  printf "\r  ${COLOR_BLUE}⠋${COLOR_RESET} Checking database '%s'...          " "$db_name"

  local db_exists
  db_exists=$(docker exec "$container_name" psql -U "$db_user" -tAc \
    "SELECT 1 FROM pg_database WHERE datname='$db_name'" 2>/dev/null || echo "")

  if [[ "$db_exists" != "1" ]]; then
    printf "\r  ${COLOR_BLUE}⠋${COLOR_RESET} Creating database '%s'...          " "$db_name"
    docker exec "$container_name" psql -U "$db_user" -c \
      "CREATE DATABASE \"$db_name\";" >/dev/null 2>&1 || true
  fi

  # Step 3: Ensure required schemas exist
  printf "\r  ${COLOR_BLUE}⠋${COLOR_RESET} Ensuring required schemas...          "

  # Create auth schema (required by nhost-auth)
  docker exec "$container_name" psql -U "$db_user" -d "$db_name" -c \
    "CREATE SCHEMA IF NOT EXISTS auth;" >/dev/null 2>&1 || true

  # Create storage schema (required by nhost-storage)
  docker exec "$container_name" psql -U "$db_user" -d "$db_name" -c \
    "CREATE SCHEMA IF NOT EXISTS storage;" >/dev/null 2>&1 || true

  # Create public schema if missing
  docker exec "$container_name" psql -U "$db_user" -d "$db_name" -c \
    "CREATE SCHEMA IF NOT EXISTS public;" >/dev/null 2>&1 || true

  # Step 4: Grant permissions
  printf "\r  ${COLOR_BLUE}⠋${COLOR_RESET} Setting schema permissions...          "

  docker exec "$container_name" psql -U "$db_user" -d "$db_name" -c \
    "GRANT ALL ON SCHEMA auth TO \"$db_user\";
     GRANT ALL ON SCHEMA storage TO \"$db_user\";
     GRANT ALL ON SCHEMA public TO \"$db_user\";" >/dev/null 2>&1 || true

  # Step 5: Ensure pgcrypto extension (required for auth)
  docker exec "$container_name" psql -U "$db_user" -d "$db_name" -c \
    "CREATE EXTENSION IF NOT EXISTS pgcrypto;" >/dev/null 2>&1 || true

  # Step 6: Ensure citext extension (often needed)
  docker exec "$container_name" psql -U "$db_user" -d "$db_name" -c \
    "CREATE EXTENSION IF NOT EXISTS citext;" >/dev/null 2>&1 || true

  printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} Database ready: %s (schemas: auth, storage, public)\n" "$db_name"
  return 0
}

# ============================================================
# AUTOMATIC REDIS READINESS
# ============================================================

ensure_redis_ready() {
  local project_name="$1"
  local max_wait="${2:-30}"

  local container_name="${project_name}_redis"

  # Check if redis container exists
  if ! docker ps --format "{{.Names}}" | grep -q "^${container_name}"; then
    container_name="${project_name}-redis-1"
    if ! docker ps --format "{{.Names}}" | grep -q "^${container_name}"; then
      return 0 # No redis container, skip
    fi
  fi

  printf "  ${COLOR_BLUE}⠋${COLOR_RESET} Ensuring Redis is ready..."

  local waited=0
  local redis_password="${REDIS_PASSWORD:-}"
  local auth_arg=""
  [[ -n "$redis_password" ]] && auth_arg="-a $redis_password"

  while [[ $waited -lt $max_wait ]]; do
    if docker exec "$container_name" redis-cli $auth_arg ping 2>/dev/null | grep -q "PONG"; then
      printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} Redis ready                              \n"
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done

  printf "\r  ${COLOR_YELLOW}!${COLOR_RESET} Redis not responding (will retry)      \n"
  return 0
}

# Progress tracking functions
PROGRESS_STEPS=()
PROGRESS_STATUS=()
CURRENT_STEP=0

add_progress() {
  PROGRESS_STEPS+=("$1")
  PROGRESS_STATUS+=("pending")
}

update_progress() {
  local step=$1
  local status=$2
  PROGRESS_STATUS[$step]=$status

  if [[ "$VERBOSE" == "false" ]]; then
    # Clear line and show updated status
    local message="${PROGRESS_STEPS[$step]}"
    if [[ "$status" == "running" ]]; then
      printf "\r${COLOR_BLUE}⠋${COLOR_RESET} %s..." "$message"
    elif [[ "$status" == "done" ]]; then
      printf "\r${COLOR_GREEN}✓${COLOR_RESET} %-40s\n" "$message"
    elif [[ "$status" == "error" ]]; then
      printf "\r${COLOR_RED}✗${COLOR_RESET} %-40s\n" "$message"
    fi
  fi
}

# Start services function
start_services() {
  # 1. Detect environment and project
  local env="${ENV:-dev}"
  if [[ -f ".env" ]]; then
    env=$(grep "^ENV=" .env 2>/dev/null | cut -d= -f2- || echo "dev")
  fi

  # Project name priority: COMPOSE_PROJECT_NAME > PROJECT_NAME > .env > dirname
  local project_name="${COMPOSE_PROJECT_NAME:-${PROJECT_NAME:-}}"
  if [[ -z "$project_name" ]] && [[ -f ".env" ]]; then
    project_name=$(grep "^COMPOSE_PROJECT_NAME=" .env 2>/dev/null | cut -d= -f2- || true)
    [[ -z "$project_name" ]] && project_name=$(grep "^PROJECT_NAME=" .env 2>/dev/null | cut -d= -f2- || true)
  fi
  if [[ -z "$project_name" ]]; then
    project_name=$(basename "$PWD")
  fi

  # 2. Show header (like build command)
  show_command_header "nself start" "Start all project services"

  # 3. Setup progress steps
  add_progress "Validating prerequisites"
  add_progress "Cleaning previous state"
  add_progress "Creating network"
  add_progress "Creating volumes"
  add_progress "Creating containers"
  add_progress "Starting core services"
  add_progress "Starting optional services"
  add_progress "Starting monitoring"
  add_progress "Starting custom services"
  add_progress "Verifying health checks"

  # 4. Validate prerequisites
  update_progress 0 "running"

  if [[ ! -f "docker-compose.yml" ]]; then
    # Monorepo fallback: check if backend/ has docker-compose.yml
    if [[ -d "backend" ]] && [[ -f "backend/docker-compose.yml" ]]; then
      printf "${COLOR_CYAN}Monorepo detected${COLOR_RESET} — switching to backend/ directory\n"
      cd backend
      # Reload environment from backend directory
      if [[ -f ".env" ]] || [[ -f ".env.local" ]]; then
        load_env_with_priority >/dev/null 2>&1 || true
        project_name="${PROJECT_NAME:-$(basename "$PWD")}"
      fi
    else
      update_progress 0 "error"
      printf "\n${COLOR_RED}Error: docker-compose.yml not found${COLOR_RESET}\n"
      printf "Run '${COLOR_BLUE}nself build${COLOR_RESET}' first to generate configuration\n\n"
      return 1
    fi
  fi

  if ! command -v docker >/dev/null 2>&1; then
    update_progress 0 "error"
    printf "\n${COLOR_RED}Error: Docker is not installed${COLOR_RESET}\n\n"
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    update_progress 0 "error"
    printf "\n${COLOR_RED}Error: Docker daemon is not running${COLOR_RESET}\n"
    printf "Start Docker Desktop or run: ${COLOR_BLUE}sudo systemctl start docker${COLOR_RESET}\n\n"
    return 1
  fi

  update_progress 0 "done"

  # 4b. Pre-flight port conflict check
  # Load .env first so custom port vars (e.g. NGINX_SSL_PORT=8443) are resolved
  if command -v load_env_with_priority >/dev/null 2>&1; then
    load_env_with_priority "true" >/dev/null 2>&1 || true
  fi
  # Skip when --fresh (containers will be stopped before docker compose up, so
  # currently-held ports aren't a real blocker) or when --skip-port-check is set
  if [[ "$SKIP_PORT_CHECK" != "true" ]] && [[ "$START_MODE" != "fresh" ]]; then
    if command -v preflight_port_check >/dev/null 2>&1; then
      if ! preflight_port_check; then
        printf "\n${COLOR_RED}Cannot start: one or more required ports are in use.${COLOR_RESET}\n"
        printf "Update the port variables shown above in your .env file, then run 'nself build && nself start'\n\n"
        return 1
      fi
    fi
  fi

  # 5. Clean up containers based on CLEANUP_ON_START setting
  update_progress 1 "running"

  # Determine cleanup behavior — use Docker Compose labels to scope cleanup
  # to ONLY this project's containers (prevents cross-project destruction)
  local should_cleanup=false
  if [[ "$CLEANUP_ON_START" == "always" ]]; then
    should_cleanup=true
  elif [[ "$CLEANUP_ON_START" == "auto" ]]; then
    # Check if any containers for THIS project are in error/stale state (label-scoped)
    local error_containers
    error_containers=$(docker ps -a --filter "label=com.docker.compose.project=$project_name" --filter "status=exited" --format "{{.Names}}" 2>/dev/null)
    if [[ -n "$error_containers" ]]; then
      should_cleanup=true
    fi
    # Also check for containers in 'created' state — these indicate stale Docker Compose
    # tracking state (phantom containers) left by a previously interrupted start or reset.
    # The 'created' container cannot be started by ID and blocks compose from re-creating
    # the service. Running compose down clears this internal tracking state.
    local created_containers
    created_containers=$(docker ps -a --filter "label=com.docker.compose.project=$project_name" --filter "status=created" --format "{{.ID}}" 2>/dev/null)
    if [[ -n "$created_containers" ]]; then
      should_cleanup=true
    fi
  fi

  if [[ "$should_cleanup" == "true" ]]; then
    # Run compose down first to clear any stale container-tracking state (phantom containers).
    # This is necessary when Docker Compose holds references to containers that were created
    # but never started and may no longer be findable by docker rm/inspect.
    docker compose --project-name "$project_name" down --remove-orphans >/dev/null 2>&1 || true
    # Clean up ONLY containers belonging to this project (label-scoped)
    local existing_containers=$(docker ps -aq --filter "label=com.docker.compose.project=$project_name" 2>/dev/null)
    if [[ -n "$existing_containers" ]]; then
      echo "$existing_containers" | xargs -r docker rm -f >/dev/null 2>&1 || true
    fi

    # Clean up project network
    docker network rm "${project_name}_network" >/dev/null 2>&1 || true
    docker network rm "${project_name}_default" >/dev/null 2>&1 || true
  fi

  # Remove orphan containers: named like ${project_name}_* but NOT managed by compose
  # These block docker compose from taking ownership of services (e.g., a manually-run nginx
  # left over from a previous session prevents compose from creating unity_nginx with its labels)
  local orphan_containers
  orphan_containers=$(docker ps -a --format "{{.Names}}" 2>/dev/null \
    | grep -E "^${project_name}[-_]" || true)
  if [[ -n "$orphan_containers" ]]; then
    while IFS= read -r cname; do
      [[ -z "$cname" ]] && continue
      local cproject
      cproject=$(docker inspect "$cname" \
        --format '{{index .Config.Labels "com.docker.compose.project"}}' 2>/dev/null || true)
      if [[ -z "$cproject" ]]; then
        # No compose label — orphan. Remove so compose can take ownership.
        docker rm -f "$cname" >/dev/null 2>&1 || true
      fi
    done <<< "$orphan_containers"
  fi

  update_progress 1 "done"

  # 6. Source env-merger if available
  if [[ -f "$LIB_DIR/utils/env-merger.sh" ]]; then
    source "$LIB_DIR/utils/env-merger.sh"
  fi

  # 7. Generate merged runtime environment
  local target_env="${ENV:-dev}"
  if command -v merge_environments >/dev/null 2>&1; then
    if [[ "$VERBOSE" == "false" ]]; then
      merge_environments "$target_env" ".env.runtime" >/dev/null 2>&1
    else
      printf "Merging environment configuration...\n"
      merge_environments "$target_env" ".env.runtime"
    fi
  fi

  # 8. Determine env file and update project name from runtime
  local env_file=".env"
  if [[ -f ".env.runtime" ]]; then
    # Ensure restrictive permissions (contains secrets)
    chmod 600 ".env.runtime" 2>/dev/null || true
    env_file=".env.runtime"
    # Update project_name from runtime file
    project_name=$(grep "^PROJECT_NAME=" .env.runtime 2>/dev/null | cut -d= -f2- || echo "$project_name")
  fi

  # 9. Start services with progress tracking
  local compose_cmd="docker compose"
  local start_output=$(mktemp)
  local error_output=$(mktemp)

  # ============================================================
  # SECURE BY DEFAULT: Force recreate security-sensitive containers
  # This ensures Redis/PostgreSQL pick up latest secure config
  # ============================================================
  if command -v security::force_recreate_sensitive_containers >/dev/null 2>&1; then
    security::force_recreate_sensitive_containers "$project_name"
  fi

  # Warning for Mailpit in production
  if [[ "${ENV:-dev}" == "production" ]] && [[ "${MAILPIT_ENABLED:-false}" == "true" ]]; then
    printf "\n${COLOR_YELLOW}⚠ Warning:${COLOR_RESET} Mailpit is for development only and insecure\n"
    printf "  Configure production email with: ${COLOR_BLUE}nself service email configure${COLOR_RESET}\n\n"
    sleep 2
  fi

  # Build the docker compose command based on start mode
  local compose_args=(
    "--project-name" "$project_name"
    "--env-file" "$env_file"
    "up" "-d"
    "--remove-orphans"
  )

  # Add mode-specific flags
  # SECURITY: Always force-recreate for fresh/force modes to ensure config is applied
  if [[ "$START_MODE" == "fresh" ]]; then
    compose_args+=("--force-recreate")
  elif [[ "$START_MODE" == "force" ]]; then
    compose_args+=("--force-recreate" "--renew-anon-volumes")
  fi

  if [[ "$DEBUG" == "true" ]]; then
    echo ""
    echo "DEBUG: Project name: $project_name"
    echo "DEBUG: Environment: $env"
    echo "DEBUG: Env file: $env_file"
    echo "DEBUG: Command: $compose_cmd ${compose_args[*]}"
    echo ""
  fi

  # Show initial preparing message
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Analyzing Docker configuration..."

  # Execute docker compose
  if [[ "$VERBOSE" == "true" ]]; then
    # Verbose mode - show Docker output directly
    printf "\r%-60s\r" " " # Clear the preparing message
    $compose_cmd "${compose_args[@]}" 2>&1 | tee "$start_output"
    local exit_code=${PIPESTATUS[0]}
  else
    # Clean mode - capture output and show progress
    $compose_cmd "${compose_args[@]}" >"$start_output" 2>"$error_output" &
    local compose_pid=$!

    # Spinner characters for animation
    local spinner=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
    local spin_index=0

    # Track progress based on docker output
    local network_done=false
    local volumes_done=false
    local containers_created=false
    local services_starting=false
    local monitoring_started=false
    local custom_started=false

    # Count total expected services from docker-compose.yml
    local total_services=$(grep -c "^  [a-z].*:" docker-compose.yml 2>/dev/null || echo "25")
    local images_to_pull=0
    local images_pulled=0
    local containers_started=0
    local current_action="Analyzing Docker configuration"
    local last_line=""
    local last_update=$(date +%s)

    # Initial delay to let docker compose start
    sleep 0.2

    while ps -p $compose_pid >/dev/null 2>&1; do
      # Update spinner
      spin_index=$(((spin_index + 1) % 10))

      # Get the last non-empty line from output to see what's happening
      last_line=$(tail -n 10 "$start_output" 2>/dev/null | grep -v "^$" | tail -n 1 || echo "")

      # Check what's happening based on output patterns with more detail
      if echo "$last_line" | grep -q "Building\|Step\|RUN\|COPY\|FROM"; then
        # Building custom images - count steps
        local build_steps
        build_steps=$(grep -c "Step [0-9]" "$start_output" 2>/dev/null) || build_steps=0
        local image_name=$(echo "$last_line" | grep -oE "Building [a-z_-]+" | sed 's/Building //' || echo "image")
        current_action="Building custom Docker images"
        if [[ -n "$image_name" ]] && [[ "$image_name" != "image" ]]; then
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (building %s)" "${spinner[$spin_index]}" "$current_action" "$image_name"
        else
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (step %d)" "${spinner[$spin_index]}" "$current_action" "$build_steps"
        fi

      elif echo "$last_line" | grep -q "Pulling\|Pull complete\|Already exists\|Downloading\|Extracting\|Waiting"; then
        # Count unique images being pulled - better tracking
        local pulling_count
        pulling_count=$(grep -c "Pulling from" "$start_output" 2>/dev/null) || pulling_count=0
        local pulled_count
        pulled_count=$(grep -c "Pull complete\|Already exists" "$start_output" 2>/dev/null) || pulled_count=0

        # Try to estimate total images needed
        if [[ $images_to_pull -eq 0 ]]; then
          # Rough estimate based on service count
          images_to_pull=$((total_services * 2 / 3)) # Not all services have unique images
        fi

        # Get the current image being pulled
        local current_image=$(echo "$last_line" | grep -oE "[a-z0-9-]+/[a-z0-9-]+" | tail -1 || echo "")
        current_action="Downloading Docker images"

        if [[ -n "$current_image" ]]; then
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d/%d) %s" "${spinner[$spin_index]}" "$current_action" "$pulling_count" "$images_to_pull" "$current_image"
        else
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d/%d)" "${spinner[$spin_index]}" "$current_action" "$pulling_count" "$images_to_pull"
        fi

      elif grep -q "Network.*Creating\|Network.*Created" "$start_output" 2>/dev/null; then
        # Network creation
        local network_count
        network_count=$(grep -c "Network.*Created" "$start_output" 2>/dev/null) || network_count=0
        current_action="Creating Docker network"
        printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s..." "${spinner[$spin_index]}" "$current_action"

        if [[ "$network_done" == "false" ]] && [[ "$network_count" -gt 0 ]]; then
          update_progress 2 "done"
          network_done=true
        fi

      elif grep -q "Volume.*Creating\|Volume.*Created" "$start_output" 2>/dev/null; then
        # Volume creation
        local volume_count
        volume_count=$(grep -c "Volume.*Created" "$start_output" 2>/dev/null) || volume_count=0
        current_action="Creating Docker volumes"
        if [[ "$volume_count" -gt 0 ]]; then
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d created)" "${spinner[$spin_index]}" "$current_action" "$volume_count"
        else
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s..." "${spinner[$spin_index]}" "$current_action"
        fi

        if [[ "$volumes_done" == "false" ]] && [[ "$volume_count" -gt 0 ]]; then
          update_progress 3 "done"
          volumes_done=true
        fi

      elif echo "$last_line" | grep -q "Container.*Creating\|Container.*Created"; then
        # Count containers being created with more detail
        local created_count
        created_count=$(grep -c "Container.*Created" "$start_output" 2>/dev/null) || created_count=0
        local creating_count
        creating_count=$(grep -c "Container.*Creating" "$start_output" 2>/dev/null) || creating_count=0
        local total_creating=$((created_count + creating_count))

        # Get the name of container being created
        local container_name=$(echo "$last_line" | grep -oE "Container ${project_name}_[a-z0-9_-]+" | sed "s/Container ${project_name}_//" || echo "")
        current_action="Creating Docker containers"

        if [[ -n "$container_name" ]]; then
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d/%d) %s" "${spinner[$spin_index]}" "$current_action" "$created_count" "$total_services" "$container_name"
        else
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d/%d)" "${spinner[$spin_index]}" "$current_action" "$created_count" "$total_services"
        fi

        if [[ "$created_count" -ge "$total_services" ]] && [[ "$containers_created" == "false" ]]; then
          update_progress 4 "done"
          containers_created=true
        fi

      elif echo "$last_line" | grep -q "Container.*Starting\|Container.*Started\|Container.*Running"; then
        # Count containers being started with more detail
        containers_started=$(grep -c "Container.*Started" "$start_output" 2>/dev/null) || containers_started=0
        local starting_count
        starting_count=$(grep -c "Container.*Starting" "$start_output" 2>/dev/null) || starting_count=0

        # Get the name of container being started
        local container_name=$(echo "$last_line" | grep -oE "Container ${project_name}_[a-z0-9_-]+" | sed "s/Container ${project_name}_//" || echo "")
        current_action="Starting Docker containers"

        if [[ -n "$container_name" ]]; then
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d/%d) %s" "${spinner[$spin_index]}" "$current_action" "$containers_started" "$total_services" "$container_name"
        else
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d/%d)" "${spinner[$spin_index]}" "$current_action" "$containers_started" "$total_services"
        fi

        # Update specific service categories as they start
        if [[ "$services_starting" == "false" ]] && grep -q "Container ${project_name}_postgres.*Started" "$start_output" 2>/dev/null; then
          update_progress 5 "done"
          services_starting=true
        fi

        if [[ "$services_starting" == "true" ]] && grep -q "Container ${project_name}_minio.*Started" "$start_output" 2>/dev/null; then
          update_progress 6 "done"
        fi

        if [[ "$monitoring_started" == "false" ]] && grep -q "Container ${project_name}_prometheus.*Started" "$start_output" 2>/dev/null; then
          update_progress 7 "done"
          monitoring_started=true
        fi

        if [[ "$custom_started" == "false" ]] && grep -q "Container ${project_name}_express_api.*Started" "$start_output" 2>/dev/null; then
          update_progress 8 "done"
          custom_started=true
        fi
      else
        # Default spinner while waiting - show more detail
        local current_time=$(date +%s)
        local elapsed=$((current_time - last_update))

        # Change message based on elapsed time and what we're likely doing
        if [[ "$elapsed" -lt 5 ]]; then
          current_action="Preparing Docker environment"
        elif [[ "$elapsed" -lt 15 ]]; then
          current_action="Checking Docker images"
        elif [[ "$elapsed" -lt 30 ]]; then
          current_action="Processing service dependencies"
        elif [[ "$elapsed" -lt 60 ]]; then
          current_action="Configuring network and volumes"
        else
          current_action="Initializing services"
        fi

        # Show basic progress even when we don't have specific info
        local any_containers=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
        if [[ "$any_containers" -gt 0 ]]; then
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s... (%d containers active)" "${spinner[$spin_index]}" "$current_action" "$any_containers"
        else
          printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s..." "${spinner[$spin_index]}" "$current_action"
        fi
      fi

      sleep 0.1 # Faster updates for smoother animation
    done

    wait $compose_pid
    local exit_code=$?

    # Clear the spinner line
    printf "\r%-60s\r" " "
  fi

  # 10. Check results
  if [[ $exit_code -eq 0 ]]; then
    # Mark any remaining steps as done
    for i in 2 3 4 5 6 7 8; do
      if [[ "${PROGRESS_STATUS[$i]}" == "pending" ]]; then
        update_progress $i "done"
      fi
    done

    # Init containers (minio-init, meilisearch-init, etc.) finish within seconds of compose up.
    # Clean them up immediately — do NOT wait until after health checks.
    cleanup_init_containers 2>/dev/null || true

    # ========================================================
    # AUTOMATIC SERVICE READINESS (runs EVERY start)
    # The Golden Rule: No manual intervention required
    # ========================================================
    printf "\n${COLOR_CYAN}Ensuring services are ready...${COLOR_RESET}\n"

    # Load env vars for database configuration
    if [[ -f "$env_file" ]]; then
      set -a
      source "$env_file" 2>/dev/null || true
      set +a
    fi

    # Ensure database is ready (creates DB and schemas if needed)
    ensure_database_ready "$project_name" 60

    # Ensure Redis is ready (if enabled)
    if [[ "${REDIS_ENABLED:-false}" == "true" ]]; then
      ensure_redis_ready "$project_name" 30
    fi

    # ========================================================
    # SECURE BY DEFAULT: Verify no sensitive ports exposed
    # This catches config drift or misconfigured containers
    # ========================================================
    if command -v security::verify_no_exposed_ports >/dev/null 2>&1; then
      if ! security::verify_no_exposed_ports; then
        # Critical security violation - stop everything
        printf "\n${COLOR_RED}CRITICAL SECURITY VIOLATION DETECTED${COLOR_RESET}\n"
        printf "Sensitive ports exposed to public internet.\n"
        printf "Stopping all services to prevent security breach...\n\n"
        docker compose --project-name "$project_name" down >/dev/null 2>&1 || true
        printf "Fix the port bindings in docker-compose.yml and run 'nself build' again.\n"
        printf "All internal services (Redis, PostgreSQL, etc.) must bind to 127.0.0.1 only.\n\n"
        rm -f "$start_output" "$error_output"
        return 1
      fi
    fi

    printf "\n"
    # ========================================================

    # Verify health checks (unless skipped)
    if [[ "$SKIP_HEALTH_CHECKS" != "true" ]]; then
      update_progress 9 "running"

      # Progressive health check with configurable timeout and threshold
      local start_time=$(date +%s)
      local health_check_passed=false

      while true; do
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))

        if [[ $elapsed -ge $HEALTH_CHECK_TIMEOUT ]]; then
          break
        fi

        # Count health status
        local running_count=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
        local healthy_count
        healthy_count=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Status}}" 2>/dev/null | grep -c "healthy") || healthy_count=0
        local total_with_health
        total_with_health=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Status}}" 2>/dev/null | grep -cE "(healthy|unhealthy|starting)") || total_with_health=0

        # Calculate percentage
        if [[ $total_with_health -gt 0 ]]; then
          local health_percent=$((healthy_count * 100 / total_with_health))
          if [[ $health_percent -ge $HEALTH_CHECK_REQUIRED ]]; then
            health_check_passed=true
            break
          fi
        elif [[ $running_count -gt 0 ]]; then
          # If no health checks defined, consider it passing if containers are running
          health_check_passed=true
          break
        fi

        sleep "$HEALTH_CHECK_INTERVAL"
      done

      update_progress 9 "done"
    else
      # Skip health checks
      update_progress 9 "done"
      local running_count=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
      local healthy_count=0
      local total_with_health=0
    fi

    # Get final counts for summary
    local running_count=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
    local healthy_count
    healthy_count=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Status}}" 2>/dev/null | grep -c "healthy") || healthy_count=0
    local total_with_health
    total_with_health=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Status}}" 2>/dev/null | grep -cE "(healthy|unhealthy|starting)") || total_with_health=0

    # Count service types
    local core_count=4
    local optional_count
    optional_count=$(grep -c "_ENABLED=true" "$env_file" 2>/dev/null) || optional_count=0
    local monitoring_count=0
    if grep -q "MONITORING_ENABLED=true" "$env_file" 2>/dev/null; then
      monitoring_count=10
    fi
    local custom_count
    custom_count=$(grep -c "^CS_[0-9]=" "$env_file" 2>/dev/null) || custom_count=0

    # Final summary (like build command)
    printf "\n"
    printf "${COLOR_GREEN}✓${COLOR_RESET} ${COLOR_BOLD}All services started successfully${COLOR_RESET}\n"

    # Aggressively clean up init containers (minio-init, meilisearch-init, etc.)
    # Wait for them to finish, then force-remove so they never linger in Docker Desktop
    wait_and_cleanup_init_containers 30 2>/dev/null || true

    printf "${COLOR_GREEN}✓${COLOR_RESET} Project: ${COLOR_BOLD}%s${COLOR_RESET} (%s) / BD: %s\n" "$project_name" "$env" "${BASE_DOMAIN:-localhost}"
    printf "${COLOR_GREEN}✓${COLOR_RESET} Services (%s): %s core, %s optional, %s monitoring, %s custom\n" \
      "${running_count:-0}" "${core_count:-4}" "${optional_count:-0}" "${monitoring_count:-0}" "${custom_count:-0}"

    if [[ $total_with_health -gt 0 ]]; then
      printf "${COLOR_GREEN}✓${COLOR_RESET} Health: %s/%s checks passing\n" "${healthy_count:-0}" "${total_with_health:-0}"
    fi

    # ========================================================
    # AUTO-START PLUGINS (per-project scoped)
    #
    # If .nself/plugins exists in the project directory:
    #   - Non-empty file → start only listed plugins (per-project mode)
    #   - Empty file     → no plugins for this project (silent)
    # If .nself/plugins does NOT exist:
    #   - Backward-compat: start all globally installed plugins
    # ========================================================
    local plugin_dir="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
    local project_plugin_list=".nself/plugins"
    if command -v start_all_plugins >/dev/null 2>&1; then
      if [[ -f "$project_plugin_list" ]]; then
        # Per-project mode
        if [[ -s "$project_plugin_list" ]]; then
          printf "\n${COLOR_CYAN}Starting project plugins...${COLOR_RESET}\n"
          start_all_plugins "$project_plugin_list"
        fi
        # Empty file → no plugins for this project; output nothing
      elif [[ -d "$plugin_dir" ]]; then
        # Global fallback: start all globally installed plugins
        local has_plugins=false
        for _pdir in "$plugin_dir"/*/plugin.json; do
          if [[ -f "$_pdir" ]]; then
            local _pname
            _pname=$(dirname "$_pdir")
            _pname=$(basename "$_pname")
            if [[ "$_pname" != "_shared" ]]; then
              has_plugins=true
              break
            fi
          fi
        done
        if [[ "$has_plugins" == "true" ]]; then
          printf "\n${COLOR_CYAN}Starting installed plugins...${COLOR_RESET}\n"
          start_all_plugins
        fi
      fi
    fi

    printf "\n\n${COLOR_BOLD}Next steps:${COLOR_RESET}\n\n"
    printf "1. ${COLOR_BLUE}nself status${COLOR_RESET} - Check service health\n"
    printf "   View detailed status of all running services\n\n"
    printf "2. ${COLOR_BLUE}nself urls${COLOR_RESET} - View service URLs\n"
    printf "   Access your application and service dashboards\n\n"
    printf "3. ${COLOR_BLUE}nself logs${COLOR_RESET} - View service logs\n"
    printf "   Monitor real-time logs from all services\n\n"
    printf "For more help, use: ${COLOR_DIM}nself help${COLOR_RESET} or ${COLOR_DIM}nself help start${COLOR_RESET}\n\n"

  else
    # Error occurred - mark remaining steps as error
    for i in "${!PROGRESS_STATUS[@]}"; do
      if [[ "${PROGRESS_STATUS[$i]}" == "pending" || "${PROGRESS_STATUS[$i]}" == "running" ]]; then
        update_progress $i "error"
      fi
    done

    printf "\n${COLOR_RED}✗ Failed to start services${COLOR_RESET}\n\n"

    # Show which services DID start (helps diagnose partial success)
    local started_services
    started_services=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "  ✓ {{.Names}}" 2>/dev/null)
    if [[ -n "$started_services" ]]; then
      printf "${COLOR_GREEN}Services that started:${COLOR_RESET}\n%s\n\n" "$started_services"
    fi

    # Show error details — display all stderr output, not just grep-filtered lines,
    # so errors like "Error response from daemon: No such container" are always visible.
    if [[ -s "$error_output" ]]; then
      printf "${COLOR_RED}Error details:${COLOR_RESET}\n"
      cat "$error_output" | head -20

      # Check for known issues and give targeted hints
      if grep -q "No such container" "$error_output" 2>/dev/null; then
        printf "\n${COLOR_YELLOW}Tip:${COLOR_RESET} Stale container state detected. Run:\n"
        printf "  ${COLOR_BLUE}nself start --clean-start${COLOR_RESET}\n"
      elif grep -q "port is already allocated\|address already in use" "$error_output" 2>/dev/null; then
        printf "\n${COLOR_YELLOW}Tip:${COLOR_RESET} A required port is already in use.\n"
        printf "  Check with: ${COLOR_BLUE}nself doctor${COLOR_RESET}\n"
      fi
    elif [[ -s "$start_output" ]]; then
      # Some compose versions send errors to stdout
      printf "${COLOR_RED}Error details:${COLOR_RESET}\n"
      grep -E "(Error|error|failed|Failed|unhealthy)" "$start_output" 2>/dev/null | head -10 || true
    fi

    # In verbose mode, show full output
    if [[ "$VERBOSE" == "true" ]] && [[ -s "$start_output" ]]; then
      printf "\n${COLOR_DIM}Full output:${COLOR_RESET}\n"
      cat "$start_output"
    fi

    printf "\n${COLOR_DIM}Tip: Run with --verbose for detailed output${COLOR_RESET}\n\n"

    rm -f "$start_output" "$error_output"
    return 1
  fi

  # Clean up temp files
  rm -f "$start_output" "$error_output"
  return 0
}

# Run start
start_services
