#!/usr/bin/env bash


# docker.sh - Docker-specific error detection and handling

# Docker error codes
readonly DOCKER_NOT_RUNNING="DOCKER_NOT_RUNNING"

set -euo pipefail

readonly DOCKER_PERMISSION_DENIED="DOCKER_PERMISSION_DENIED"
readonly DOCKER_DISK_FULL="DOCKER_DISK_FULL"
readonly DOCKER_NETWORK_EXISTS="DOCKER_NETWORK_EXISTS"
readonly DOCKER_IMAGE_MISSING="DOCKER_IMAGE_MISSING"
readonly DOCKER_COMPOSE_INVALID="DOCKER_COMPOSE_INVALID"
readonly DOCKER_CONTAINER_UNHEALTHY="DOCKER_CONTAINER_UNHEALTHY"

# Check Docker daemon status
check_docker_daemon() {
  log_info "Checking Docker daemon..."

  if ! command -v docker >/dev/null 2>&1; then
    register_error "$DOCKER_NOT_RUNNING" \
      "Docker is not installed" \
      $ERROR_CRITICAL \
      "false"

    log_error "Docker is not installed"
    log_info "Please install Docker Desktop from https://docker.com"
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    local error_msg=$(docker info 2>&1 | head -1)

    if echo "$error_msg" | grep -q "permission denied"; then
      register_error "$DOCKER_PERMISSION_DENIED" \
        "Docker requires elevated permissions" \
        $ERROR_MAJOR \
        "true" \
        "fix_docker_permissions"
    elif echo "$error_msg" | grep -q "Cannot connect"; then
      register_error "$DOCKER_NOT_RUNNING" \
        "Docker daemon is not running" \
        $ERROR_CRITICAL \
        "true" \
        "fix_docker_not_running"
    else
      register_error "$DOCKER_NOT_RUNNING" \
        "Docker error: $error_msg" \
        $ERROR_CRITICAL \
        "false"
    fi

    return 1
  fi

  log_success "Docker daemon is running"
  return 0
}

# Fix Docker not running
fix_docker_not_running() {
  log_info "Attempting to start Docker..."

  # macOS - try to start Docker Desktop
  if [[ "$OSTYPE" == "darwin"* ]]; then
    if [[ -d "/Applications/Docker.app" ]]; then
      log_info "Starting Docker Desktop..."
      open -a Docker

      # Wait for Docker to start (max 30 seconds)
      local attempts=0
      while [[ $attempts -lt 30 ]]; do
        if docker info >/dev/null 2>&1; then
          log_success "Docker Desktop started successfully"
          return 0
        fi
        sleep 1
        attempts=$((attempts + 1))
      done

      log_error "Docker Desktop failed to start within 30 seconds"
      return 1
    else
      log_error "Docker Desktop not found in /Applications"
      log_info "Please install from: https://docker.com"
      return 1
    fi
  fi

  # Linux - try systemctl
  if command -v systemctl >/dev/null 2>&1; then
    log_info "Starting Docker service..."
    if sudo systemctl start docker; then
      log_success "Docker service started"
      return 0
    else
      log_error "Failed to start Docker service"
      return 1
    fi
  fi

  log_error "Could not start Docker automatically"
  log_info "Please start Docker manually"
  return 1
}

# Fix Docker permissions
fix_docker_permissions() {
  log_info "Fixing Docker permissions..."

  # Linux - add user to docker group
  if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    if ! groups | grep -q docker; then
      log_info "Adding user to docker group..."
      sudo usermod -aG docker "$USER"
      log_warning "You need to log out and back in for changes to take effect"
      log_info "Or run: newgrp docker"
      return 0
    fi
  fi

  # Try sudo docker
  if sudo docker info >/dev/null 2>&1; then
    log_warning "Docker requires sudo on this system"
    export DOCKER_SUDO="sudo"
    return 0
  fi

  log_error "Could not fix Docker permissions"
  return 1
}

# Check Docker disk space
check_docker_disk_space() {
  log_info "Checking Docker disk usage..."

  local disk_usage=$(docker system df --format "{{.Size}}" 2>/dev/null | tail -1)

  if [[ -z "$disk_usage" ]]; then
    log_warning "Could not determine Docker disk usage"
    return 0
  fi

  # Check if disk is nearly full
  local available=$(df /var/lib/docker 2>/dev/null | tail -1 | awk '{print $4}')

  if [[ -n "$available" ]] && [[ $available -lt 1048576 ]]; then # Less than 1GB
    register_error "$DOCKER_DISK_FULL" \
      "Docker is running low on disk space" \
      $ERROR_MAJOR \
      "true" \
      "fix_docker_disk_space"

    log_warning "Docker has less than 1GB free space"
    return 1
  fi

  log_success "Docker has sufficient disk space"
  return 0
}

# Fix Docker disk space
fix_docker_disk_space() {
  log_info "Cleaning up Docker disk space..."

  # Show current usage
  docker system df
  echo ""

  read -p "Remove unused Docker resources? [y/N]: " -n 1 -r
  echo ""

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Skipping cleanup"
    return 1
  fi

  # Clean up in stages
  log_info "Removing stopped containers..."
  docker container prune -f

  log_info "Removing unused images..."
  docker image prune -f

  log_info "Removing unused volumes..."
  docker volume prune -f

  log_info "Removing unused networks..."
  docker network prune -f

  # Show new usage
  echo ""
  docker system df

  log_success "Docker cleanup completed"
  return 0
}

# Check for missing images
check_docker_images() {
  log_info "Checking Docker images..."

  if [[ ! -f "docker-compose.yml" ]]; then
    log_debug "No docker-compose.yml to check"
    return 0
  fi

  # Extract required images
  local required_images=$(grep -E '^\s*image:' docker-compose.yml | sed 's/.*image:\s*//' | sort -u)

  local missing_images=()
  for image in $required_images; do
    if ! docker image inspect "$image" >/dev/null 2>&1; then
      missing_images+=("$image")
    fi
  done

  if [[ ${#missing_images[@]} -gt 0 ]]; then
    register_error "$DOCKER_IMAGE_MISSING" \
      "${#missing_images[@]} Docker image(s) not found locally" \
      $ERROR_MINOR \
      "true" \
      "fix_docker_images"

    log_info "Missing images will be pulled when starting services"
    return 0 # Not critical
  fi

  log_success "All required Docker images are available"
  return 0
}

# Fix missing Docker images
fix_docker_images() {
  log_info "Pulling missing Docker images..."

  if docker compose pull; then
    log_success "Docker images pulled successfully"
    return 0
  else
    log_error "Failed to pull some Docker images"
    log_info "This might be due to network issues or registry problems"
    return 1
  fi
}

# Check Docker Compose configuration
check_docker_compose() {
  log_info "Validating Docker Compose configuration..."

  if [[ ! -f "docker-compose.yml" ]]; then
    register_error "$DOCKER_COMPOSE_INVALID" \
      "docker-compose.yml not found" \
      $ERROR_CRITICAL \
      "true" \
      "fix_docker_compose_missing"
    return 1
  fi

  # Validate syntax
  local validation_output
  validation_output=$(docker compose config 2>&1)

  if [[ $? -ne 0 ]]; then
    register_error "$DOCKER_COMPOSE_INVALID" \
      "Docker Compose configuration is invalid" \
      $ERROR_CRITICAL \
      "false"

    log_error "Docker Compose validation failed:"
    echo "$validation_output" | grep -E "ERROR|error" | head -5
    return 1
  fi

  log_success "Docker Compose configuration is valid"
  return 0
}

# Fix missing docker-compose.yml
fix_docker_compose_missing() {
  log_info "Generating docker-compose.yml..."

  if [[ ! -f ".env.local" ]]; then
    log_error "Cannot generate docker-compose.yml without .env.local"
    log_info "Run: nself init"
    return 1
  fi

  # Run build command
  if bash "$SCRIPT_DIR/../cli/build.sh"; then
    log_success "Generated docker-compose.yml"
    return 0
  else
    log_error "Failed to generate docker-compose.yml"
    return 1
  fi
}

# Check container health
check_container_health() {
  log_info "Checking container health..."

  local unhealthy_containers=()
  local containers=$(docker compose ps --format json 2>/dev/null)

  if [[ -z "$containers" ]]; then
    log_debug "No containers running"
    return 0
  fi

  while IFS= read -r container; do
    local name=$(echo "$container" | jq -r '.Name')
    local status=$(echo "$container" | jq -r '.Status')
    local health=$(echo "$container" | jq -r '.Health // "none"')

    if [[ "$health" == "unhealthy" ]]; then
      unhealthy_containers+=("$name")
    elif [[ "$status" == "restarting" ]]; then
      unhealthy_containers+=("$name (restarting)")
    fi
  done <<<"$containers"

  if [[ ${#unhealthy_containers[@]} -gt 0 ]]; then
    register_error "$DOCKER_CONTAINER_UNHEALTHY" \
      "${#unhealthy_containers[@]} container(s) are unhealthy" \
      $ERROR_MAJOR \
      "true" \
      "fix_unhealthy_containers"

    for container in "${unhealthy_containers[@]}"; do
      log_warning "Unhealthy: $container"
    done

    return 1
  fi

  log_success "All containers are healthy"
  return 0
}

# Fix unhealthy containers
fix_unhealthy_containers() {
  log_info "Attempting to fix unhealthy containers..."

  # Try restarting unhealthy containers
  local unhealthy=$(docker compose ps --format json | jq -r 'select(.Health == "unhealthy") | .Service')

  for service in $unhealthy; do
    log_info "Restarting $service..."
    docker compose restart "$service"
  done

  # Wait for health
  sleep 5

  # Re-check
  if check_container_health; then
    return 0
  else
    log_error "Some containers remain unhealthy"
    log_info "Check logs: nself logs [service]"
    return 1
  fi
}

# Export all functions
export -f check_docker_daemon
export -f fix_docker_not_running
export -f fix_docker_permissions
export -f check_docker_disk_space
export -f fix_docker_disk_space
export -f check_docker_images
export -f fix_docker_images
export -f check_docker_compose
export -f fix_docker_compose_missing
export -f check_container_health
export -f fix_unhealthy_containers
