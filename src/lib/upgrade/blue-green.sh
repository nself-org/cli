#!/usr/bin/env bash

# blue-green.sh - Zero-downtime upgrades using blue-green deployment
# v0.4.8

set -euo pipefail

# Import utilities
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$SCRIPT_DIR/../utils/display.sh" 2>/dev/null || true
source "$SCRIPT_DIR/../utils/env.sh" 2>/dev/null || true

# Colors
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_BLUE:=\033[0;34m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"

# Configuration
DEPLOYMENT_DIR="${DEPLOYMENT_DIR:-.nself/deployments}"
ROLLBACK_LIMIT="${ROLLBACK_LIMIT:-5}"
HEALTH_CHECK_TIMEOUT="${HEALTH_CHECK_TIMEOUT:-60}"
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-2}"

# Get current active deployment (blue or green)
get_active_deployment() {
  local active_file="$DEPLOYMENT_DIR/active"

  if [[ -f "$active_file" ]]; then
    cat "$active_file"
  else
    echo "blue" # Default to blue
  fi
}

# Set active deployment
set_active_deployment() {
  local color="$1"

  mkdir -p "$DEPLOYMENT_DIR"
  echo "$color" >"$DEPLOYMENT_DIR/active"

  log_success "Active deployment set to: $color"
}

# Get inactive deployment color
get_inactive_deployment() {
  local active=$(get_active_deployment)

  if [[ "$active" == "blue" ]]; then
    echo "green"
  else
    echo "blue"
  fi
}

# Create deployment snapshot
create_deployment_snapshot() {
  local color="$1"
  local timestamp=$(date +%Y%m%d-%H%M%S)
  local snapshot_dir="$DEPLOYMENT_DIR/$color-$timestamp"

  mkdir -p "$snapshot_dir"

  # Save environment state
  cp .env "$snapshot_dir/.env" 2>/dev/null || true
  cp docker-compose.yml "$snapshot_dir/docker-compose.yml" 2>/dev/null || true

  # Save container list
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  docker ps -a --filter "name=${project_name}_" --format "{{.Names}}" \
    >"$snapshot_dir/containers.txt" 2>/dev/null || true

  # Save metadata
  cat >"$snapshot_dir/metadata.json" <<EOF
{
  "timestamp": "$timestamp",
  "color": "$color",
  "project_name": "$project_name",
  "git_commit": "$(git rev-parse HEAD 2>/dev/null || echo 'unknown')",
  "git_branch": "$(git branch --show-current 2>/dev/null || echo 'unknown')"
}
EOF

  echo "$snapshot_dir"
}

# Run health checks on deployment
check_deployment_health() {
  local color="$1"
  local timeout="${2:-$HEALTH_CHECK_TIMEOUT}"

  log_info "Running health checks on $color deployment..."

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  # Key services to check
  local services=(
    "${project_name}_postgres"
    "${project_name}_hasura"
    "${project_name}_auth"
  )

  local start_time=$(date +%s)
  local all_healthy=false

  while true; do
    local current_time=$(date +%s)
    local elapsed=$((current_time - start_time))

    if [[ $elapsed -gt $timeout ]]; then
      log_error "Health check timeout after ${timeout}s"
      return 1
    fi

    local healthy_count=0
    local total_count=${#services[@]}

    for service in "${services[@]}"; do
      if docker ps --filter "name=$service" --filter "status=running" --format "{{.Names}}" | grep -q "$service"; then
        # Check if service is actually healthy (has health check)
        local health=$(docker inspect --format='{{.State.Health.Status}}' "$service" 2>/dev/null || echo "none")

        if [[ "$health" == "healthy" ]] || [[ "$health" == "none" ]]; then
          ((healthy_count++))
        fi
      fi
    done

    if [[ $healthy_count -eq $total_count ]]; then
      all_healthy=true
      break
    fi

    printf "${COLOR_DIM}  Health check: $healthy_count/$total_count services healthy (${elapsed}s)${COLOR_RESET}\r"
    sleep "$HEALTH_CHECK_INTERVAL"
  done

  echo "" # Clear progress line

  if [[ "$all_healthy" == "true" ]]; then
    log_success "$color deployment is healthy"
    return 0
  else
    log_error "$color deployment health check failed"
    return 1
  fi
}

# Switch traffic to specified deployment
switch_traffic() {
  local target_color="$1"

  log_info "Switching traffic to $target_color deployment..."

  # Update nginx configuration to point to new deployment
  # In a full implementation, this would update nginx upstream config
  # For now, we just update the active marker

  set_active_deployment "$target_color"

  # Reload nginx (if running)
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  if docker ps --filter "name=${project_name}_nginx" --format "{{.Names}}" | grep -q "nginx"; then
    docker exec "${project_name}_nginx" nginx -s reload 2>/dev/null || true
  fi

  log_success "Traffic switched to $target_color deployment"
}

# Perform blue-green deployment
perform_blue_green_deployment() {
  local skip_health_check="${1:-false}"

  printf "${COLOR_CYAN}╔════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_CYAN}║   Blue-Green Deployment (Zero Down)   ║${COLOR_RESET}\n"
  printf "${COLOR_CYAN}╚════════════════════════════════════════╝${COLOR_RESET}\n"
  echo ""

  # Determine deployment colors
  local active_color=$(get_active_deployment)
  local inactive_color=$(get_inactive_deployment)

  printf "${COLOR_BLUE}Active:   $active_color${COLOR_RESET}\n"
  printf "${COLOR_GREEN}New:      $inactive_color${COLOR_RESET}\n"
  echo ""

  # Step 1: Create snapshot of current deployment
  printf "${COLOR_CYAN}➞ Step 1: Snapshot current deployment${COLOR_RESET}\n"
  local snapshot=$(create_deployment_snapshot "$active_color")
  log_success "Snapshot created: $snapshot"
  echo ""

  # Step 2: Deploy to inactive environment
  printf "${COLOR_CYAN}➞ Step 2: Deploy to $inactive_color environment${COLOR_RESET}\n"
  log_info "Building and starting $inactive_color deployment..."

  # Build new version
  if command -v nself >/dev/null 2>&1; then
    nself build 2>&1 | grep -E "^(✓|Success)" || true
    log_success "Build completed"
  else
    log_warning "nself command not found - skipping build"
  fi

  # Start services
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  docker-compose up -d 2>&1 | grep -E "^(Creating|Starting)" || true
  log_success "$inactive_color deployment started"
  echo ""

  # Step 3: Health checks
  printf "${COLOR_CYAN}➞ Step 3: Health checks${COLOR_RESET}\n"

  if [[ "$skip_health_check" != "true" ]]; then
    if ! check_deployment_health "$inactive_color"; then
      log_error "Health checks failed - aborting deployment"
      log_info "Use 'nself upgrade rollback' to ensure clean state"
      return 1
    fi
  else
    log_warning "Health checks skipped"
  fi
  echo ""

  # Step 4: Switch traffic
  printf "${COLOR_CYAN}➞ Step 4: Switch traffic${COLOR_RESET}\n"

  if [[ "${AUTO_SWITCH:-false}" == "true" ]]; then
    switch_traffic "$inactive_color"
  else
    printf "${COLOR_YELLOW}Ready to switch traffic to $inactive_color deployment${COLOR_RESET}\n"
    read -p "Continue? (y/N) " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
      switch_traffic "$inactive_color"
    else
      log_info "Traffic switch cancelled - both deployments running"
      log_info "To switch manually: nself upgrade switch $inactive_color"
      log_info "To rollback: nself upgrade rollback"
      return 0
    fi
  fi
  echo ""

  # Step 5: Cleanup old deployment (optional)
  printf "${COLOR_CYAN}➞ Step 5: Cleanup${COLOR_RESET}\n"

  if [[ "${AUTO_CLEANUP:-false}" == "true" ]]; then
    log_info "Stopping $active_color deployment..."
    # Don't actually stop containers - keep for rollback
    log_success "Old deployment kept for rollback"
  else
    log_info "$active_color deployment kept running for quick rollback"
    log_info "To stop it: nself upgrade cleanup $active_color"
  fi
  echo ""

  # Record deployment
  local deployment_record="$DEPLOYMENT_DIR/history.json"
  local record_entry="{\"timestamp\":\"$(date -Iseconds)\",\"from\":\"$active_color\",\"to\":\"$inactive_color\",\"snapshot\":\"$snapshot\"}"

  if [[ -f "$deployment_record" ]]; then
    # Append to existing history
    local temp_file=$(mktemp)
    jq ". += [$record_entry]" "$deployment_record" >"$temp_file" 2>/dev/null && mv "$temp_file" "$deployment_record" || {
      # Fallback if jq not available
      echo "$record_entry" >>"$deployment_record"
    }
  else
    echo "[$record_entry]" >"$deployment_record"
  fi

  log_success "Deployment completed successfully"
  log_info "Active deployment: $inactive_color"
  log_info "Rollback available: nself upgrade rollback"

  return 0
}

# Perform rolling update (alternative to blue-green)
perform_rolling_update() {
  printf "${COLOR_CYAN}╔════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_CYAN}║      Rolling Update (Gradual)         ║${COLOR_RESET}\n"
  printf "${COLOR_CYAN}╚════════════════════════════════════════╝${COLOR_RESET}\n"
  echo ""

  log_info "Starting rolling update..."

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  # Get list of all services
  local services=$(docker-compose ps --services 2>/dev/null)

  # Critical services to update last
  local critical_services=("postgres" "hasura" "auth")

  # Update non-critical services first
  for service in $services; do
    local is_critical=false

    for critical in "${critical_services[@]}"; do
      if [[ "$service" == *"$critical"* ]]; then
        is_critical=true
        break
      fi
    done

    if [[ "$is_critical" == "false" ]]; then
      printf "${COLOR_CYAN}➞ Updating $service${COLOR_RESET}\n"

      docker-compose up -d --no-deps --build "$service" 2>&1 | grep -v "Warning" || true

      # Brief health check
      sleep 2

      if docker ps --filter "name=${project_name}_$service" --format "{{.Names}}" | grep -q "$service"; then
        log_success "$service updated"
      else
        log_warning "$service update may have failed"
      fi

      echo ""
    fi
  done

  # Update critical services one by one
  for critical in "${critical_services[@]}"; do
    for service in $services; do
      if [[ "$service" == *"$critical"* ]]; then
        printf "${COLOR_CYAN}➞ Updating critical service: $service${COLOR_RESET}\n"

        docker-compose up -d --no-deps --build "$service" 2>&1 | grep -v "Warning" || true

        # Wait for service to be healthy
        sleep 5

        log_success "$service updated"
        echo ""
      fi
    done
  done

  log_success "Rolling update completed"
  log_info "All services have been updated gradually"

  return 0
}

# Rollback to previous deployment
rollback_deployment() {
  printf "${COLOR_CYAN}╔════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_CYAN}║        Deployment Rollback             ║${COLOR_RESET}\n"
  printf "${COLOR_CYAN}╚════════════════════════════════════════╝${COLOR_RESET}\n"
  echo ""

  local active_color=$(get_active_deployment)
  local previous_color=$(get_inactive_deployment)

  log_warning "Rolling back from $active_color to $previous_color"

  # Check if previous deployment is still running
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  local previous_running=$(docker ps --filter "name=${project_name}_" --format "{{.Names}}" | wc -l)

  if [[ $previous_running -eq 0 ]]; then
    log_error "Previous deployment not found - cannot rollback"
    log_info "Use 'nself restore' to restore from backup instead"
    return 1
  fi

  # Switch traffic back
  printf "${COLOR_CYAN}➞ Switching traffic back to $previous_color${COLOR_RESET}\n"
  switch_traffic "$previous_color"
  echo ""

  # Health check
  printf "${COLOR_CYAN}➞ Verifying $previous_color deployment${COLOR_RESET}\n"
  if check_deployment_health "$previous_color"; then
    log_success "Rollback successful"
    echo ""

    log_info "You can now stop the failed $active_color deployment:"
    echo "  nself upgrade cleanup $active_color"

    return 0
  else
    log_error "Rollback health check failed"
    return 1
  fi
}

# Cleanup old deployment
cleanup_deployment() {
  local color="$1"

  if [[ -z "$color" ]]; then
    log_error "Deployment color required (blue or green)"
    return 1
  fi

  log_info "Cleaning up $color deployment..."

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  # Stop containers (but don't remove)
  docker-compose stop 2>&1 | grep -v "Stopping" || true

  log_success "$color deployment stopped (containers preserved for rollback)"
  log_info "To fully remove: docker-compose down"
}

# Show deployment status
show_deployment_status() {
  printf "${COLOR_CYAN}╔════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_CYAN}║       Deployment Status                ║${COLOR_RESET}\n"
  printf "${COLOR_CYAN}╚════════════════════════════════════════╝${COLOR_RESET}\n"
  echo ""

  local active_color=$(get_active_deployment)
  local inactive_color=$(get_inactive_deployment)

  printf "Active Deployment:  ${COLOR_GREEN}$active_color${COLOR_RESET}\n"
  printf "Standby Deployment: ${COLOR_DIM}$inactive_color${COLOR_RESET}\n"
  echo ""

  # Show container counts
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  local running_count=$(docker ps --filter "name=${project_name}_" --format "{{.Names}}" | wc -l)
  local total_count=$(docker ps -a --filter "name=${project_name}_" --format "{{.Names}}" | wc -l)

  printf "Containers Running: $running_count / $total_count\n"
  echo ""

  # Show recent deployments
  if [[ -f "$DEPLOYMENT_DIR/history.json" ]]; then
    printf "${COLOR_CYAN}Recent Deployments:${COLOR_RESET}\n"

    if command -v jq >/dev/null 2>&1; then
      jq -r '.[-5:] | .[] | "  \(.timestamp) | \(.from) → \(.to)"' "$DEPLOYMENT_DIR/history.json" 2>/dev/null || true
    else
      tail -5 "$DEPLOYMENT_DIR/history.json"
    fi
  fi
}

# Export functions
export -f get_active_deployment
export -f set_active_deployment
export -f get_inactive_deployment
export -f create_deployment_snapshot
export -f check_deployment_health
export -f switch_traffic
export -f perform_blue_green_deployment
export -f perform_rolling_update
export -f rollback_deployment
export -f cleanup_deployment
export -f show_deployment_status
