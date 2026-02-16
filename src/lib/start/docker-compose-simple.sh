#!/usr/bin/env bash

# docker-compose-simple.sh - Simplified docker compose startup

# Simple docker compose up that doesn't hang
simple_compose_up() {

set -euo pipefail

  local project="${1:-nself}"
  local env_file="${2:-.env.runtime}"
  local verbose="${3:-false}"

  local compose_cmd=$(get_compose_command 2>/dev/null || echo "docker compose")

  printf "${COLOR_BLUE}⠋${COLOR_RESET} Starting Docker services (this may take a few minutes if images need downloading)...\n"

  # Start services without building - use pre-built images
  if [ "$verbose" = "true" ]; then
    $compose_cmd --project-name "$project" --env-file "$env_file" up -d --no-build
  else
    $compose_cmd --project-name "$project" --env-file "$env_file" up -d --no-build 2>&1 |
      grep -E "(Pulling|Creating|Starting|Created|Started|Container|Network|Volume)" || true
  fi

  local result=$?

  # Give services a moment to stabilize
  sleep 3

  # Count running services
  local running_count=$(docker ps --filter "label=com.docker.compose.project=$project" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')

  if [ "$running_count" -gt 0 ]; then
    printf "${COLOR_GREEN}✓${COLOR_RESET} Started %d services\n" "$running_count"

    # Run init containers (like minio_client) if they exist
    run_init_containers "$project" "$env_file" "$compose_cmd" "$verbose"

    return 0
  else
    printf "${COLOR_YELLOW}⚠${COLOR_RESET}  No services started - check docker-compose.yml and .env.runtime\n"
    return 1
  fi
}

# Run one-time initialization containers
run_init_containers() {
  local project="$1"
  local env_file="$2"
  local compose_cmd="$3"
  local verbose="${4:-false}"

  # Check if init-containers profile exists in docker-compose.yml
  if ! grep -q "profiles:" docker-compose.yml 2>/dev/null; then
    return 0
  fi

  if ! grep -q "init-containers" docker-compose.yml 2>/dev/null; then
    return 0
  fi

  # Run init containers
  if [ "$verbose" = "true" ]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Running initialization containers...\n"
  fi

  # Start init-containers profile, wait for completion, then remove
  $compose_cmd --project-name "$project" --env-file "$env_file" --profile init-containers up --no-build 2>&1 |
    grep -v "Attaching to" || true

  # Remove exited init containers to keep Docker clean
  cleanup_init_containers "$project" "$verbose"
}

# Cleanup exited init containers
cleanup_init_containers() {
  local project="$1"
  local verbose="${2:-false}"

  # Find and remove containers with init-container label that have exited
  local init_containers=$(docker ps -a \
    --filter "label=nself.type=init-container" \
    --filter "label=com.docker.compose.project=$project" \
    --filter "status=exited" \
    --format "{{.Names}}" 2>/dev/null)

  if [ -n "$init_containers" ]; then
    if [ "$verbose" = "true" ]; then
      printf "${COLOR_BLUE}⠋${COLOR_RESET} Cleaning up init containers...\n"
    fi

    echo "$init_containers" | while read -r container; do
      docker rm "$container" >/dev/null 2>&1 || true
    done

    if [ "$verbose" = "true" ]; then
      printf "${COLOR_GREEN}✓${COLOR_RESET} Init containers cleaned up\n"
    fi
  fi
}

export -f simple_compose_up
export -f run_init_containers
export -f cleanup_init_containers
