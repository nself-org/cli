#!/usr/bin/env bash

# scale.sh - Resource scaling management

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Show help
show_scale_help() {
  echo "nself scale - Resource scaling management"
  echo ""
  echo "Usage: nself scale [options] <service> [replicas]"
  echo ""
  echo "Options:"
  echo "  --cpu <limit>      Set CPU limit (e.g., 2, 1.5, 0.5)"
  echo "  --memory <limit>   Set memory limit (e.g., 2G, 512M)"
  echo "  --replicas <n>     Set number of replicas"
  echo "  --auto             Enable auto-scaling"
  echo "  --min <n>          Minimum replicas for auto-scaling"
  echo "  --max <n>          Maximum replicas for auto-scaling"
  echo "  --cpu-target <n>   CPU target for auto-scaling (percentage)"
  echo "  --list             List current resource allocations"
  echo "  -h, --help         Show this help message"
  echo ""
  echo "Services:"
  echo "  postgres           PostgreSQL database"
  echo "  hasura             Hasura GraphQL engine"
  echo "  hasura-auth        Authentication service"
  echo "  hasura-storage     Storage service"
  echo "  nginx              Web server"
  echo "  redis              Redis cache"
  echo "  functions          Serverless functions"
  echo ""
  echo "Examples:"
  echo "  nself scale postgres --memory 4G --cpu 2"
  echo "  nself scale hasura --replicas 3"
  echo "  nself scale nginx --auto --min 2 --max 10"
  echo "  nself scale --list"
}

# Get current resource usage
get_resource_usage() {
  local service="$1"

  # Load environment for PROJECT_NAME
  [[ -f ".env" ]] && source ".env" 2>/dev/null || true
  [[ -f ".env.dev" ]] && source ".env.dev" 2>/dev/null || true

  local project_name="${PROJECT_NAME:-nself}"

  # Try to find container by project name pattern (more reliable than docker compose ps --format json)
  local container_name
  container_name=$(docker ps --filter "name=${project_name}_${service}" --format "{{.Names}}" 2>/dev/null | head -1)

  # Also try with hyphen instead of underscore
  if [[ -z "$container_name" ]]; then
    container_name=$(docker ps --filter "name=${project_name}-${service}" --format "{{.Names}}" 2>/dev/null | head -1)
  fi

  if [[ -n "$container_name" ]]; then
    # Get CPU and memory stats
    local stats
    stats=$(docker stats --no-stream --format "{{.CPUPerc}}\t{{.MemUsage}}" "$container_name" 2>/dev/null || echo "")

    if [[ -n "$stats" ]]; then
      local cpu_percent=$(echo "$stats" | cut -f1)
      local mem_usage=$(echo "$stats" | cut -f2)

      echo "CPU: ${cpu_percent}, Memory: ${mem_usage}"
    else
      echo "No stats available"
    fi
  else
    echo "Service not running"
  fi
}

# List current allocations
list_allocations() {
  show_command_header "nself scale" "Current resource allocations"
  echo ""

  printf "${COLOR_CYAN}%-20s %-15s %-15s %-30s${COLOR_RESET}\n" "Service" "CPU Limit" "Memory Limit" "Current Usage"
  printf "%-20s %-15s %-15s %-30s\n" "-------------------" "--------------" "--------------" "-----------------------------"

  # Read docker-compose.yml for configured limits
  local services=(postgres hasura hasura-auth hasura-storage nginx redis functions)

  for service in "${services[@]}"; do
    if grep -q "^  ${service}:" docker-compose.yml 2>/dev/null; then
      # Extract limits from docker-compose.yml
      local cpu_limit="-"
      local mem_limit="-"

      # Try to find resource limits in compose file - escape service name for regex
      local escaped_service=$(printf '%s\n' "$service" | sed 's/[[\.*^$()+?{|]/\\&/g')
      local service_block=$(awk "/^  ${escaped_service}:/,/^  [a-z]/" docker-compose.yml 2>/dev/null)

      if echo "$service_block" | grep -q "cpus:"; then
        cpu_limit=$(echo "$service_block" | grep "cpus:" | head -1 | sed 's/.*cpus:[ "]*\([^"]*\).*/\1/')
      fi

      if echo "$service_block" | grep -q "memory:"; then
        mem_limit=$(echo "$service_block" | grep "memory:" | head -1 | sed 's/.*memory:[ "]*\([^"]*\).*/\1/')
      fi

      # Get current usage
      local usage=$(get_resource_usage "$service")

      printf "%-20s %-15s %-15s %-30s\n" "$service" "$cpu_limit" "$mem_limit" "$usage"
    fi
  done

  echo ""
  log_info "Use 'nself scale <service> --cpu <limit> --memory <limit>' to adjust resources"
}

# Apply scaling configuration
apply_scaling() {
  local service="$1"
  local cpu_limit="$2"
  local mem_limit="$3"
  local replicas="$4"

  # Check if service exists
  if ! grep -q "^  ${service}:" docker-compose.yml 2>/dev/null; then
    log_error "Service '$service' not found in docker-compose.yml"
    return 1
  fi

  # Create override file
  local override_file="docker-compose.override.yml"

  # Initialize override if it doesn't exist
  if [[ ! -f "$override_file" ]]; then
    cat >"$override_file" <<EOF
version: '3.8'
services:
EOF
  fi

  # Add service scaling configuration
  local temp_file=$(mktemp)

  # Copy existing override but remove the service block if it exists - escape service name
  local escaped_service=$(printf '%s\n' "$service" | sed 's/[[\.*^$()+?{|]/\\&/g')
  awk "/^  ${escaped_service}:/,/^  [a-z]/ { if (/^  [a-z]/ && !/^  ${escaped_service}:/) print; next } { print }" "$override_file" >"$temp_file"

  # Add new service configuration
  cat >>"$temp_file" <<EOF
  ${service}:
    deploy:
      resources:
        limits:
EOF

  if [[ -n "$cpu_limit" ]]; then
    echo "          cpus: '$cpu_limit'" >>"$temp_file"
  fi

  if [[ -n "$mem_limit" ]]; then
    echo "          memory: $mem_limit" >>"$temp_file"
  fi

  if [[ -n "$replicas" ]] && [[ "$replicas" -gt 1 ]]; then
    cat >>"$temp_file" <<EOF
      replicas: $replicas
EOF
  fi

  # Replace override file atomically
  if ! mv "$temp_file" "$override_file"; then
    log_error "Failed to update override file"
    rm -f "$temp_file"
    return 1
  fi

  log_success "Scaling configuration applied for $service"

  # Restart service to apply changes
  log_info "Restarting $service to apply changes..."
  docker compose up -d "$service" 2>/dev/null

  # Wait for service to be healthy
  sleep 2

  # Show new status
  local usage=$(get_resource_usage "$service")
  log_success "Service restarted. Current usage: $usage"
}

# Main scale command
cmd_scale() {
  local service=""
  local cpu_limit=""
  local mem_limit=""
  local replicas=""
  local auto_scale=false
  local min_replicas=""
  local max_replicas=""
  local cpu_target=""
  local list_mode=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h | --help)
        show_scale_help
        return 0
        ;;
      --list)
        list_mode=true
        shift
        ;;
      --cpu)
        cpu_limit="$2"
        shift 2
        ;;
      --memory)
        mem_limit="$2"
        shift 2
        ;;
      --replicas)
        replicas="$2"
        shift 2
        ;;
      --auto)
        auto_scale=true
        shift
        ;;
      --min)
        min_replicas="$2"
        shift 2
        ;;
      --max)
        max_replicas="$2"
        shift 2
        ;;
      --cpu-target)
        cpu_target="$2"
        shift 2
        ;;
      *)
        if [[ -z "$service" ]]; then
          service="$1"
        elif [[ -z "$replicas" ]] && [[ "$1" =~ ^[0-9]+$ ]]; then
          replicas="$1"
        fi
        shift
        ;;
    esac
  done

  # Check if docker-compose.yml exists
  if [[ ! -f "docker-compose.yml" ]]; then
    log_error "No docker-compose.yml found. Run 'nself build' first."
    return 1
  fi

  # List mode
  if [[ "$list_mode" == "true" ]]; then
    list_allocations
    return 0
  fi

  # Check if service specified
  if [[ -z "$service" ]]; then
    log_error "No service specified"
    echo ""
    echo "Usage: nself scale <service> [options]"
    echo "       nself scale --list"
    echo ""
    echo "Run 'nself scale --help' for more information"
    return 1
  fi

  show_command_header "nself scale" "Scaling service: $service"
  echo ""

  # Auto-scaling setup
  if [[ "$auto_scale" == "true" ]]; then
    log_info "Setting up auto-scaling for $service"

    if [[ -z "$min_replicas" ]]; then
      min_replicas=1
    fi

    if [[ -z "$max_replicas" ]]; then
      max_replicas=10
    fi

    if [[ -z "$cpu_target" ]]; then
      cpu_target=70
    fi

    echo "  Min replicas: $min_replicas"
    echo "  Max replicas: $max_replicas"
    echo "  CPU target: ${cpu_target}%"
    echo ""

    # Store auto-scaling config
    mkdir -p .nself
    cat >".nself/autoscale-${service}.conf" <<EOF
MIN_REPLICAS=$min_replicas
MAX_REPLICAS=$max_replicas
CPU_TARGET=$cpu_target
ENABLED=true
EOF

    log_success "Auto-scaling configured for $service"
    log_info "Auto-scaling will be managed by the monitoring system"

    return 0
  fi

  # Manual scaling
  if [[ -n "$cpu_limit" ]] || [[ -n "$mem_limit" ]] || [[ -n "$replicas" ]]; then
    printf "${COLOR_CYAN}➞ Applying scaling configuration${COLOR_RESET}\n"

    if [[ -n "$cpu_limit" ]]; then
      echo "  CPU limit: $cpu_limit"
    fi

    if [[ -n "$mem_limit" ]]; then
      echo "  Memory limit: $mem_limit"
    fi

    if [[ -n "$replicas" ]]; then
      echo "  Replicas: $replicas"
    fi

    echo ""

    apply_scaling "$service" "$cpu_limit" "$mem_limit" "$replicas"
  else
    # Just show current status
    printf "${COLOR_CYAN}➞ Current status for $service${COLOR_RESET}\n"
    echo ""

    local usage=$(get_resource_usage "$service")
    echo "  $usage"
    echo ""

    log_info "Use 'nself scale $service --cpu <limit> --memory <limit>' to adjust resources"
  fi

  return 0
}

# Export for use as library
export -f cmd_scale

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_scale_help
      exit 0
    fi
  done
  pre_command "scale" || exit $?
  cmd_scale "$@"
  exit_code=$?
  post_command "scale" $exit_code
  exit $exit_code
fi
