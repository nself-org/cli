#!/usr/bin/env bash

# docker.sh - Fix Docker issues

fix_docker_build() {

set -euo pipefail

  local issue="$1"

  case "$issue" in
    "compose_missing")
      log_info "Generating docker-compose.yml..."
      bash "$SCRIPT_DIR/../../cli/build.sh"
      ;;
    "service_unhealthy")
      log_info "Restarting unhealthy services..."
      docker compose restart
      ;;
    *)
      return 1
      ;;
  esac
}

export -f fix_docker_build
