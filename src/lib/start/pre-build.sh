#!/usr/bin/env bash

# pre-build.sh - Pre-build required images before starting

# Pre-build services that have build contexts
pre_build_services() {

set -euo pipefail

  local project="${1:-nself}"
  local env_file="${2:-.env.runtime}"
  local verbose="${3:-false}"

  # Get list of services with build contexts
  local services_to_build=""

  # Check if auth needs fallback
  if [[ -d "fallback-services" ]]; then
    services_to_build="$services_to_build auth functions"
  fi

  # Check if mlflow needs custom build
  if [[ -d "mlflow" ]] && [[ -f "mlflow/Dockerfile" ]]; then
    services_to_build="$services_to_build mlflow"
  fi

  # Check for custom services
  for i in {1..10}; do
    local cs_var="CS_$i"
    local cs_value="${!cs_var:-}"
    if [[ -n "$cs_value" ]]; then
      local service_name=$(echo "$cs_value" | cut -d: -f1)
      if [[ -d "services/$service_name" ]] && [[ -f "services/$service_name/Dockerfile" ]]; then
        services_to_build="$services_to_build $service_name"
      fi
    fi
  done

  if [[ -z "$services_to_build" ]]; then
    return 0
  fi

  printf "${COLOR_BLUE}⠋${COLOR_RESET} Pre-building required services...\n"

  local compose_cmd=$(get_compose_command)
  local failed_builds=""

  for service in $services_to_build; do
    if [ "$verbose" = "true" ]; then
      printf "  Building %s...\n" "$service"
    fi

    if ! $compose_cmd --project-name "$project" --env-file "$env_file" build "$service" >/dev/null 2>&1; then
      failed_builds="$failed_builds $service"
    fi
  done

  if [[ -n "$failed_builds" ]]; then
    printf "${COLOR_YELLOW}⚠${COLOR_RESET}  Some services failed to build:%s\n" "$failed_builds"
    printf "  They will use existing images if available\n"
    return 1
  fi

  printf "${COLOR_GREEN}✓${COLOR_RESET} Services pre-built successfully\n"
  return 0
}

export -f pre_build_services
