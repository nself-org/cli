#!/usr/bin/env bash

# ports.sh - Fix port conflicts

fix_port_conflict() {

set -euo pipefail

  local port="$1"
  local service="$2"

  # Only fix if it's our own container
  local container=$(docker ps --filter "publish=$port" --format "{{.Names}}" | head -1)
  if [[ "$container" == *"${PROJECT_NAME}"* ]]; then
    log_info "Stopping conflicting container: $container"
    docker stop "$container"
    return 0
  fi

  log_warning "Port $port is used by non-nself service"
  return 1
}

export -f fix_port_conflict
