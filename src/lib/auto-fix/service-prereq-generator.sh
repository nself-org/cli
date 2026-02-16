#!/usr/bin/env bash


# service-prereq-generator.sh - Pre-generate all services defined in docker-compose.yml

# Source utilities
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"

set -euo pipefail

source "$SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true
source "$SCRIPT_DIR/dockerfile-generator.sh" 2>/dev/null || true
source "$SCRIPT_DIR/service-generator.sh" 2>/dev/null || true

# Check and generate all required services from docker-compose.yml
ensure_all_services_exist() {
  local compose_file="${1:-docker-compose.yml}"

  if [[ ! -f "$compose_file" ]]; then
    return 0 # No compose file, nothing to check
  fi

  log_info "Checking for required services..."

  # Extract all services that have build contexts
  local services_with_build=$(docker compose config 2>/dev/null |
    grep -B1 "context:" | grep -E "^  [a-z-]+:" | sed 's/://; s/^  //')

  local generated_count=0

  for service in $services_with_build; do
    # Get the build context for this service
    local context=$(docker compose config 2>/dev/null |
      grep -A5 "^  $service:" | grep "context:" | awk '{print $2}')

    if [[ -n "$context" ]]; then
      # Check if the context directory exists
      if [[ ! -d "$context" ]]; then
        log_warning "Missing build context for service '$service': $context"

        # Determine what type of service this is and generate it
        if [[ "$context" =~ ^services/ ]]; then
          # It's a microservice, use the service generator
          if check_missing_service "$context"; then
            ((generated_count++))
          fi
        elif [[ "$context" == "functions" ]] || [[ "$context" == "dashboard" ]]; then
          # It's a system service, use the dockerfile generator
          if generate_dockerfile_for_service "$(basename "$context")" "$context"; then
            ((generated_count++))
          fi
        fi
      elif [[ ! -f "$context/Dockerfile" ]]; then
        # Directory exists but no Dockerfile
        log_warning "Missing Dockerfile for service '$service' in: $context"

        # Generate just the Dockerfile
        if generate_dockerfile_for_service "$(basename "$context")" "$context"; then
          ((generated_count++))
        fi
      fi
    fi
  done

  if [[ $generated_count -gt 0 ]]; then
    log_success "Generated $generated_count missing service(s)"
  fi

  return 0
}

# Export function
export -f ensure_all_services_exist
