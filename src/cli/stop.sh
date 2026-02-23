#!/usr/bin/env bash
set -euo pipefail

# stop.sh - Stop all services with enhanced feedback

# Source shared utilities
CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/docker.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/ux-standards.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"
source "$CLI_SCRIPT_DIR/../lib/plugin/runtime.sh" 2>/dev/null || true

# Load environment with smart defaults
if [[ -f "$CLI_SCRIPT_DIR/../lib/config/smart-defaults.sh" ]]; then
  source "$CLI_SCRIPT_DIR/../lib/config/smart-defaults.sh"
  load_env_with_defaults >/dev/null 2>&1 || true
fi

# Command function
cmd_stop() {
  local remove_volumes=false
  local remove_images=false
  local remove_orphans=false
  local verbose=false
  local services_to_stop=""
  local graceful_timeout="${NSELF_STOP_TIMEOUT:-30}"

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --volumes | -v)
        remove_volumes=true
        shift
        ;;
      --rmi | --remove-images)
        remove_images=true
        shift
        ;;
      --remove-orphans)
        remove_orphans=true
        shift
        ;;
      --graceful)
        if [[ -n "$2" ]] && [[ "$2" =~ ^[0-9]+$ ]]; then
          graceful_timeout="$2"
          shift 2
        else
          graceful_timeout=30
          shift
        fi
        ;;
      --verbose)
        verbose=true
        shift
        ;;
      --help | -h)
        show_down_help
        return 0
        ;;
      -*)
        ux_error_invalid_input \
          "$1" \
          "valid option (see --help)" \
          "--volumes, --verbose, --help"
        return 1
        ;;
      *)
        # Assume it's a service name
        services_to_stop="$services_to_stop $1"
        shift
        ;;
    esac
  done

  # Load service health utilities if available
  if [[ -f "$SCRIPT_DIR/../lib/utils/service-health.sh" ]]; then
    source "$SCRIPT_DIR/../lib/utils/service-health.sh"
  fi

  # Check if docker-compose.yml exists
  if [[ ! -f "docker-compose.yml" ]]; then
    ux_error \
      "docker-compose.yml not found in current directory" \
      "Run 'nself build' to generate configuration" \
      "Current directory: $(pwd)"
    return 1
  fi

  # Load environment
  if [[ -f ".env" ]] || [[ -f ".env.dev" ]]; then
    set -a
    load_env_with_priority
    set +a
  fi

  # Load runtime-computed values (PROJECT_NAME, DOCKER_NETWORK, etc.)
  if [[ -f ".env.runtime" ]]; then
    set -a
    source ".env.runtime"
    set +a
  fi

  # Get project name
  local project_name="${PROJECT_NAME:-nself}"

  # If specific services requested, stop only those
  if [[ -n "$services_to_stop" ]]; then
    show_command_header "nself stop" "Stop services and containers"
    printf "${COLOR_CYAN}→${COLOR_RESET} Stopping specific services\n"
    echo

    for service in $services_to_stop; do
      printf "${COLOR_BLUE}⠋${COLOR_RESET} Stopping $service..."
      if compose stop "$service" >/dev/null 2>&1; then
        printf "\r${COLOR_GREEN}✓${COLOR_RESET} Stopped $service                       \n"

        # Remove the container if requested
        if [[ "$remove_volumes" == true ]] || [[ "$remove_images" == true ]]; then
          compose rm -f "$service" >/dev/null 2>&1
        fi
      else
        printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to stop $service                \n"
      fi
    done

    echo
    log_success "Service shutdown completed"
    return 0
  fi

  # Show header first (no extra echo before it)
  show_command_header "nself stop" "Stop services and containers"

  # Stop health monitoring daemon if running
  if [[ -f "$SCRIPT_DIR/../lib/auto-fix/health-check-daemon.sh" ]]; then
    source "$SCRIPT_DIR/../lib/auto-fix/health-check-daemon.sh"
    if is_daemon_running 2>/dev/null; then
      stop_health_daemon >/dev/null 2>&1
      log_info "Stopped health monitoring daemon"
    fi
  fi

  # Check what's currently running using docker ps directly
  local running_containers=$(docker ps --filter "label=com.docker.compose.project=${project_name}" --format "{{.Names}}" 2>/dev/null)
  local running_count=0

  if [[ -n "$running_containers" ]]; then
    running_count=$(echo "$running_containers" | wc -l | tr -d ' ')
  fi

  if [[ $running_count -eq 0 ]]; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} No services are currently running\n"
    echo

    # Check for stopped containers
    local stopped_containers=$(docker ps -a --filter "label=com.docker.compose.project=${project_name}" --format "{{.Names}}" 2>/dev/null)

    if [[ -n "$stopped_containers" ]]; then
      local stopped_count=$(echo "$stopped_containers" | wc -l | tr -d ' ')
      printf "${COLOR_YELLOW}⚠${COLOR_RESET}  Found %s stopped containers\n" "$stopped_count"
      echo

      if [[ "$remove_volumes" == true ]] || [[ "$remove_images" == true ]]; then
        printf "${COLOR_BLUE}⠋${COLOR_RESET} Cleaning up..."
        local cleanup_args=("down")
        [[ "$remove_volumes" == true ]] && cleanup_args+=("-v")
        [[ "$remove_images" == true ]] && cleanup_args+=("--rmi" "all")
        compose "${cleanup_args[@]}" >/dev/null 2>&1
        printf "\r${COLOR_GREEN}✓${COLOR_RESET} Cleanup completed                      \n"
      else
        printf "   Run ${COLOR_BLUE}nself stop --volumes${COLOR_RESET} to remove all data\n"
        printf "   Run ${COLOR_BLUE}docker system prune${COLOR_RESET} to clean up Docker\n"
      fi
    fi

    echo
    return 0
  fi

  # Show operation type
  if [[ "$remove_volumes" == true ]]; then
    printf "${COLOR_CYAN}→${COLOR_RESET} Stopping services and removing data (%s running)\n" "$running_count"
  elif [[ "$remove_images" == true ]]; then
    printf "${COLOR_CYAN}→${COLOR_RESET} Stopping services and removing images (%s running)\n" "$running_count"
  else
    printf "${COLOR_CYAN}→${COLOR_RESET} Stopping all services (%s running)\n" "$running_count"
  fi

  # Ensure Docker is running
  if ! ux_validate_docker; then
    return 1
  fi

  # Build the compose down command arguments
  local compose_args=("down")

  if [[ "$remove_volumes" == true ]]; then
    compose_args+=("-v")
  fi

  if [[ "$remove_images" == true ]]; then
    compose_args+=("--rmi" "all")
  fi

  if [[ "$remove_orphans" == true ]]; then
    compose_args+=("--remove-orphans")
  fi

  # Execute the shutdown with optional graceful stop first
  local output_file=$(mktemp)

  # If not forcing immediate removal, do graceful stop first
  if [[ "$remove_volumes" != true ]] && [[ "$remove_images" != true ]]; then
    if [[ "$verbose" == true ]]; then
      echo "Gracefully stopping services (timeout: ${graceful_timeout}s)..."
      compose stop --timeout "$graceful_timeout" 2>&1 | tee -a "$output_file"
    else
      compose stop --timeout "$graceful_timeout" >/dev/null 2>&1
    fi
  fi

  if [[ "$verbose" == true ]]; then
    # Show full output in verbose mode
    printf "\n"
    compose "${compose_args[@]}" 2>&1 | tee -a "$output_file"
    result=${PIPESTATUS[0]}
  else
    # Run silently with spinner and progress
    (compose "${compose_args[@]}" 2>&1) >>"$output_file" &
    local compose_pid=$!

    # Show spinner with progress tracking
    local spin_chars="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
    local i=0
    local stopped_count=0

    while kill -0 $compose_pid 2>/dev/null; do
      # Count how many have stopped so far
      local still_running=$(docker ps --filter "label=com.docker.compose.project=${project_name}" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
      stopped_count=$((running_count - still_running))

      local char="${spin_chars:$((i % ${#spin_chars})):1}"
      printf "\r${COLOR_BLUE}%s${COLOR_RESET} Shutting down services... (%d/%d)                    " "$char" "$stopped_count" "$running_count"
      i=$((i + 1))
      sleep 0.1
    done
    wait $compose_pid
    result=$?
  fi

  if [[ $result -eq 0 ]]; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} All services stopped                              \n"

    # Clean up any exited init containers
    cleanup_init_containers 2>/dev/null || true

    # Clean up runtime environment file (so changes to .env files take effect on restart)
    if [[ -f ".env.runtime" ]]; then
      rm -f .env.runtime
      if [[ "$verbose" == true ]]; then
        printf "${COLOR_GREEN}✓${COLOR_RESET} Removed runtime environment file\n"
      fi
    fi

    # Show additional cleanup info
    if [[ "$remove_volumes" == true ]]; then
      echo
      # Count what was removed
      local removed_volumes=$(grep -c "Volume.*removed" "$output_file" 2>/dev/null | tr -d '\n' || echo "0")
      local removed_networks=$(grep -c "Network.*removed" "$output_file" 2>/dev/null | tr -d '\n' || echo "0")

      if [[ $removed_volumes -gt 0 ]]; then
        printf "${COLOR_GREEN}✓${COLOR_RESET} Removed %s volumes\n" "$removed_volumes"
      fi

      if [[ $removed_networks -gt 0 ]]; then
        printf "${COLOR_GREEN}✓${COLOR_RESET} Removed %s networks\n" "$removed_networks"
      fi

      printf "${COLOR_YELLOW}⚠${COLOR_RESET}  All persistent data has been removed\n"
    fi

    if [[ "$remove_images" == true ]]; then
      local removed_images=$(grep -c "Image.*deleted" "$output_file" 2>/dev/null || echo "0")
      if [[ $removed_images -gt 0 ]]; then
        echo
        printf "${COLOR_GREEN}✓${COLOR_RESET} Removed %s images\n" "$removed_images"
      fi
    fi

    # Clean up orphaned containers if any
    local orphaned=$(docker ps -a --filter "label=com.docker.compose.project=${project_name}" --format "{{.Names}}" 2>/dev/null)
    if [[ -n "$orphaned" ]]; then
      echo
      printf "${COLOR_BLUE}⠋${COLOR_RESET} Cleaning up orphaned containers..."
      echo "$orphaned" | xargs -r docker rm -f >/dev/null 2>&1
      printf "\r${COLOR_GREEN}✓${COLOR_RESET} Orphaned containers removed                    \n"
    fi

    # Stop plugins if available
    if type stop_all_plugins >/dev/null 2>&1; then
      echo
      stop_all_plugins 2>/dev/null || true
    fi

    # Show next steps (consistent with other commands)
    echo
    echo "Next steps:"
    echo
    printf "  ${COLOR_BLUE}nself start${COLOR_RESET}   - Start services again\n"
    printf "  ${COLOR_BLUE}nself status${COLOR_RESET}  - Check service status\n"
    printf "  ${COLOR_BLUE}nself clean${COLOR_RESET}   - Remove all project files\n"
    echo
    echo "For more help, use: nself help or nself help stop"
  else
    printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to stop services                           \n"

    # Show error details
    if [[ "$verbose" != true ]]; then
      echo
      local error_lines=$(grep -E "error|ERROR|failed|Failed" "$output_file" | head -5)
      if [[ -n "$error_lines" ]]; then
        echo "$error_lines"
        echo
      fi
      log_info "Run with --verbose for more details"
    fi

    rm -f "$output_file"
    return 1
  fi

  rm -f "$output_file"
  return 0
}

# Show help
show_down_help() {
  echo "Usage: nself stop [OPTIONS] [SERVICES...]"
  echo ""
  echo "Stop services and optionally remove containers, volumes, and images"
  echo ""
  echo "Options:"
  echo "  -v, --volumes        Remove volumes (WARNING: deletes all data)"
  echo "  --rmi                Remove images"
  echo "  --remove-orphans     Remove containers for services not in compose file"
  echo "  --graceful [N]       Graceful shutdown timeout in seconds (default: 30)"
  echo "  --verbose            Show detailed output"
  echo "  -h, --help           Show this help message"
  echo ""
  echo "Examples:"
  echo "  nself stop                    # Stop all services, keep data"
  echo "  nself stop postgres           # Stop only postgres"
  echo "  nself stop --volumes          # Stop and remove all data"
  echo "  nself stop --rmi              # Stop and remove images"
  echo "  nself stop --volumes --rmi    # Full cleanup"
  echo ""
  echo "Note: Data in volumes is preserved unless --volumes is specified"
}

# Export for use as library
export -f cmd_stop

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_down_help
      exit 0
    fi
  done
  pre_command "stop" || exit $?
  cmd_stop "$@"
  exit_code=$?
  post_command "stop" $exit_code
  exit $exit_code
fi
