#!/usr/bin/env bash

# zero-downtime.sh - Zero-downtime deployment strategies
# POSIX-compliant, no Bash 4+ features

# Get the directory where this script is located
DEPLOY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

LIB_ROOT="$(dirname "$DEPLOY_LIB_DIR")"

# Source dependencies
source "$LIB_ROOT/utils/display.sh" 2>/dev/null || true
source "$LIB_ROOT/utils/platform-compat.sh" 2>/dev/null || true
source "$DEPLOY_LIB_DIR/ssh.sh" 2>/dev/null || true
source "$DEPLOY_LIB_DIR/health-check.sh" 2>/dev/null || true

# Deployment configuration
DEPLOY_TIMEOUT="${DEPLOY_TIMEOUT:-300}"
DEPLOY_ROLLBACK_ON_FAILURE="${DEPLOY_ROLLBACK_ON_FAILURE:-true}"

# Rolling deployment - update services one at a time
rolling::deploy() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  log_info "Starting rolling deployment..."

  # Create pre-deployment backup
  rolling::create_backup "$host" "$deploy_path" "$user" "$port" "$key_file"

  # Get list of services
  local services
  services=$(ssh::exec "$host" "
    cd '$deploy_path' && docker compose config --services 2>/dev/null
  " "$user" "$port" "$key_file" 2>/dev/null)

  if [[ -z "$services" ]]; then
    log_error "No services found"
    return 1
  fi

  # Define service update order (infrastructure first, then apps)
  local infrastructure_services="postgres redis minio"
  local core_services="hasura auth nginx"
  local remaining_services=""

  # Categorize services
  for service in $services; do
    local is_infra=false
    local is_core=false

    for infra in $infrastructure_services; do
      if [[ "$service" == "$infra" ]]; then
        is_infra=true
        break
      fi
    done

    for core in $core_services; do
      if [[ "$service" == "$core" ]]; then
        is_core=true
        break
      fi
    done

    if [[ "$is_infra" != "true" ]] && [[ "$is_core" != "true" ]]; then
      remaining_services="$remaining_services $service"
    fi
  done

  # Update in order: infrastructure → core → custom services
  local all_services_order="$infrastructure_services $core_services $remaining_services"
  local failed_services=""
  local updated_count=0
  local total_count=0

  for service in $all_services_order; do
    # Check if this service is in our compose file
    if ! echo "$services" | grep -qw "$service"; then
      continue
    fi

    total_count=$((total_count + 1))
    printf "  Updating %s... " "$service"

    if rolling::update_service "$host" "$deploy_path" "$service" "$user" "$port" "$key_file"; then
      printf "${COLOR_GREEN}OK${COLOR_RESET}\n"
      updated_count=$((updated_count + 1))
    else
      printf "${COLOR_RED}FAILED${COLOR_RESET}\n"
      failed_services="$failed_services $service"

      if [[ "$DEPLOY_ROLLBACK_ON_FAILURE" == "true" ]]; then
        log_warning "Rolling back due to failure..."
        rolling::rollback "$host" "$deploy_path" "$user" "$port" "$key_file"
        return 1
      fi
    fi
  done

  # Summary
  printf "\n"
  if [[ -z "$failed_services" ]]; then
    log_success "Rolling deployment complete: $updated_count services updated"
    return 0
  else
    log_error "Deployment completed with errors: $updated_count/$total_count services updated"
    log_error "Failed services:$failed_services"
    return 1
  fi
}

# Update a single service with health check
rolling::update_service() {
  local host="$1"
  local deploy_path="$2"
  local service="$3"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  local update_script="
    cd '$deploy_path' || exit 1

    # Pull latest image (if using images, not build)
    docker compose pull '$service' 2>/dev/null || true

    # Recreate service
    docker compose up -d --no-deps --force-recreate '$service' 2>/dev/null

    # Wait for service to be running
    for i in 1 2 3 4 5 6 7 8 9 10; do
      status=\$(docker compose ps '$service' --format '{{.Status}}' 2>/dev/null)
      if echo \"\$status\" | grep -qi 'running\|healthy'; then
        echo 'service_healthy'
        exit 0
      fi
      sleep 3
    done

    echo 'service_unhealthy'
    exit 1
  "

  local result
  result=$(ssh::exec "$host" "$update_script" "$user" "$port" "$key_file" 2>/dev/null)

  if echo "$result" | grep -q "service_healthy"; then
    return 0
  else
    return 1
  fi
}

# Create pre-deployment backup
rolling::create_backup() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  log_info "Creating pre-deployment backup..."

  local backup_script="
    cd '$deploy_path' || exit 1
    backup_dir=\".backups/\$(date +%Y%m%d-%H%M%S)\"
    mkdir -p \"\$backup_dir\"

    # Backup docker-compose.yml
    cp docker-compose.yml \"\$backup_dir/\" 2>/dev/null || true

    # Backup .env files
    cp .env* \"\$backup_dir/\" 2>/dev/null || true

    # Store current image versions
    docker compose images --format 'table {{.Repository}}:{{.Tag}}' > \"\$backup_dir/images.txt\" 2>/dev/null || true

    # Record current git commit (if applicable)
    git rev-parse HEAD > \"\$backup_dir/git-commit.txt\" 2>/dev/null || true

    # Cleanup old backups (keep last 5)
    ls -dt .backups/*/ 2>/dev/null | tail -n +6 | xargs rm -rf 2>/dev/null || true

    echo \"\$backup_dir\"
  "

  local backup_dir
  backup_dir=$(ssh::exec "$host" "$backup_script" "$user" "$port" "$key_file" 2>/dev/null)

  if [[ -n "$backup_dir" ]]; then
    log_success "Backup created: $backup_dir"
    return 0
  else
    log_warning "Backup creation may have failed"
    return 1
  fi
}

# Rollback to previous deployment
rolling::rollback() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  log_info "Rolling back deployment..."

  local rollback_script="
    cd '$deploy_path' || exit 1

    # Find most recent backup
    backup_dir=\$(ls -dt .backups/*/ 2>/dev/null | head -1)

    if [ -z \"\$backup_dir\" ]; then
      echo 'no_backup'
      exit 1
    fi

    # Restore docker-compose.yml
    if [ -f \"\$backup_dir/docker-compose.yml\" ]; then
      cp \"\$backup_dir/docker-compose.yml\" docker-compose.yml
    fi

    # Restore .env files
    for env_file in \"\$backup_dir\"/.env*; do
      if [ -f \"\$env_file\" ]; then
        cp \"\$env_file\" .
      fi
    done

    # Restart services with old configuration
    docker compose up -d --force-recreate 2>/dev/null

    echo 'rollback_complete'
  "

  local result
  result=$(ssh::exec "$host" "$rollback_script" "$user" "$port" "$key_file" 2>/dev/null)

  if echo "$result" | grep -q "rollback_complete"; then
    log_success "Rollback complete"

    # Verify services are healthy after rollback
    health::wait_for_healthy "$host" "$deploy_path" 60 "$user" "$port" "$key_file"
    return $?
  else
    log_error "Rollback failed - manual intervention required"
    return 1
  fi
}

# Blue-green deployment style update
bluegreen::deploy() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  log_info "Starting blue-green deployment..."

  # This creates a parallel deployment and switches traffic
  local deploy_script="
    cd '$deploy_path' || exit 1

    # Create green deployment directory
    green_dir=\"${deploy_path}-green-\$(date +%s)\"
    cp -r '$deploy_path' \"\$green_dir\"

    cd \"\$green_dir\" || exit 1

    # Pull and build
    docker compose pull 2>/dev/null || true
    docker compose build 2>/dev/null || true

    # Start green deployment on different ports
    # (This requires compose file modification for port mapping)
    GREEN_DEPLOYMENT=true docker compose up -d 2>/dev/null

    # Wait for green to be healthy
    for i in 1 2 3 4 5 6 7 8 9 10 11 12; do
      running=\$(docker compose ps --format '{{.Status}}' 2>/dev/null | grep -c 'running' || echo 0)
      total=\$(docker compose ps -a --format '{{.Name}}' 2>/dev/null | wc -l)
      if [ \"\$running\" -eq \"\$total\" ] && [ \"\$total\" -gt 0 ]; then
        echo 'green_healthy'
        break
      fi
      sleep 5
    done

    echo \"\$green_dir\"
  "

  local green_dir
  green_dir=$(ssh::exec "$host" "$deploy_script" "$user" "$port" "$key_file" 2>/dev/null)

  if echo "$green_dir" | grep -q "green_healthy"; then
    log_success "Green deployment is healthy"

    # Switch traffic (update nginx/load balancer)
    bluegreen::switch_traffic "$host" "$deploy_path" "$green_dir" "$user" "$port" "$key_file"

    return $?
  else
    log_error "Green deployment failed to become healthy"
    return 1
  fi
}

# Switch traffic from blue to green deployment
bluegreen::switch_traffic() {
  local host="$1"
  local blue_path="$2"
  local green_path="$3"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  log_info "Switching traffic to new deployment..."

  # This would typically update nginx upstream or load balancer
  # For simplicity, we'll do an atomic symlink switch

  local switch_script="
    # Create symlink pointing to active deployment
    active_link='${blue_path%/*}/active'

    # Atomic switch using rename
    ln -sfn '$green_path' \"\${active_link}.new\"
    mv \"\${active_link}.new\" \"\$active_link\"

    # Stop old deployment
    cd '$blue_path' && docker compose down 2>/dev/null

    echo 'traffic_switched'
  "

  local result
  result=$(ssh::exec "$host" "$switch_script" "$user" "$port" "$key_file" 2>/dev/null)

  if echo "$result" | grep -q "traffic_switched"; then
    log_success "Traffic switched to new deployment"
    return 0
  else
    log_error "Failed to switch traffic"
    return 1
  fi
}

# Canary deployment - gradual traffic shift
canary::deploy() {
  local host="$1"
  local deploy_path="$2"
  local canary_percent="${3:-10}"
  local user="${4:-root}"
  local port="${5:-22}"
  local key_file="${6:-}"

  log_info "Starting canary deployment (${canary_percent}% traffic)..."

  # This is a simplified canary - real implementation would need
  # load balancer support (nginx upstream weights, etc.)

  log_warning "Canary deployment requires load balancer configuration"
  log_info "Consider using nginx upstream weights or external LB"

  return 1
}

# Check if rollback is available
rollback::is_available() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  local result
  result=$(ssh::exec "$host" "
    cd '$deploy_path' && ls -d .backups/*/ 2>/dev/null | wc -l
  " "$user" "$port" "$key_file" 2>/dev/null)

  [[ "${result:-0}" -gt 0 ]]
}

# List available rollback points
rollback::list() {
  local host="$1"
  local deploy_path="$2"
  local user="${3:-root}"
  local port="${4:-22}"
  local key_file="${5:-}"

  log_info "Available rollback points:"

  ssh::exec "$host" "
    cd '$deploy_path' || exit 1
    for backup in \$(ls -dt .backups/*/ 2>/dev/null); do
      name=\$(basename \"\$backup\")
      commit=\$(cat \"\$backup/git-commit.txt\" 2>/dev/null | head -c 8)
      echo \"  \$name \${commit:+(\$commit)}\"
    done
  " "$user" "$port" "$key_file"
}

# Export functions
export -f rolling::deploy
export -f rolling::update_service
export -f rolling::create_backup
export -f rolling::rollback
export -f bluegreen::deploy
export -f bluegreen::switch_traffic
export -f canary::deploy
export -f rollback::is_available
export -f rollback::list
