#!/usr/bin/env bash


# disaster-recovery.sh - Automated disaster recovery procedures

# Recovery configuration
RECOVERY_DIR="${RECOVERY_DIR:-./recovery}"

set -euo pipefail

RECOVERY_LOG="${RECOVERY_LOG:-$RECOVERY_DIR/recovery.log}"
MAX_RECOVERY_ATTEMPTS="${MAX_RECOVERY_ATTEMPTS:-3}"

# Initialize recovery system
init_recovery() {
  mkdir -p "$RECOVERY_DIR"
  touch "$RECOVERY_LOG"
}

# Log recovery action
log_recovery() {
  local message="$1"
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $message" >>"$RECOVERY_LOG"
}

# Create recovery checkpoint
create_recovery_checkpoint() {
  local checkpoint_name="${1:-checkpoint-$(date +%Y%m%d-%H%M%S)}"
  local checkpoint_dir="$RECOVERY_DIR/$checkpoint_name"

  mkdir -p "$checkpoint_dir"

  # Save current state
  docker ps -a >"$checkpoint_dir/containers.txt"
  docker volume ls >"$checkpoint_dir/volumes.txt"
  docker network ls >"$checkpoint_dir/networks.txt"

  # Save configuration
  cp .env.local "$checkpoint_dir/" 2>/dev/null || true
  cp docker-compose.yml "$checkpoint_dir/" 2>/dev/null || true

  # Save service health
  nself status --json >"$checkpoint_dir/status.json" 2>/dev/null || true

  log_recovery "Created recovery checkpoint: $checkpoint_name"
  echo "$checkpoint_name"
}

# Perform automatic recovery
auto_recover() {
  local service="${1:-all}"
  local attempt=0

  log_recovery "Starting automatic recovery for: $service"

  while [[ $attempt -lt $MAX_RECOVERY_ATTEMPTS ]]; do
    attempt=$((attempt + 1))
    log_recovery "Recovery attempt $attempt of $MAX_RECOVERY_ATTEMPTS"

    # Create checkpoint before recovery
    local checkpoint=$(create_recovery_checkpoint "pre-recovery-$attempt")

    # Try recovery strategies in order
    if recover_service "$service"; then
      log_recovery "Recovery successful on attempt $attempt"

      # Verify recovery
      if verify_recovery "$service"; then
        log_recovery "Recovery verified successfully"
        return 0
      else
        log_recovery "Recovery verification failed"
      fi
    else
      log_recovery "Recovery attempt $attempt failed"
    fi

    # Wait before next attempt
    sleep $((attempt * 10))
  done

  log_recovery "All recovery attempts failed for: $service"

  # Trigger failover if configured
  if [[ "${FAILOVER_ENABLED:-false}" == "true" ]]; then
    trigger_failover "$service"
  fi

  return 1
}

# Recover specific service
recover_service() {
  local service="$1"

  case "$service" in
    postgres | database)
      recover_database
      ;;
    all)
      recover_all_services
      ;;
    *)
      recover_single_service "$service"
      ;;
  esac
}

# Recover database
recover_database() {
  log_recovery "Recovering database..."

  # Stop unhealthy database
  docker stop postgres 2>/dev/null || true
  docker rm postgres 2>/dev/null || true

  # Check for data corruption
  if check_data_corruption; then
    log_recovery "Data corruption detected, restoring from backup"

    # Find latest backup
    local latest_backup=$(nself backup list | grep "backup-" | head -1 | awk '{print $1}')

    if [[ -n "$latest_backup" ]]; then
      nself backup restore "$latest_backup" database
      return $?
    else
      log_recovery "No backup available for restoration"
      return 1
    fi
  else
    # Try to restart with recovery mode
    docker compose up -d postgres
    sleep 10

    # Check if recovered
    if docker exec postgres pg_isready -U postgres 2>/dev/null; then
      log_recovery "Database recovered successfully"
      return 0
    else
      log_recovery "Database recovery failed"
      return 1
    fi
  fi
}

# Recover all services
recover_all_services() {
  log_recovery "Recovering all services..."

  # Stop all services
  nself stop

  # Clean up dead containers
  docker container prune -f

  # Clean up orphaned volumes
  docker volume prune -f

  # Rebuild and restart
  nself build --force
  nself start

  # Wait for services to stabilize
  sleep 30

  # Check overall health
  local unhealthy_count=$(docker ps --filter "health=unhealthy" -q | wc -l)

  if [[ $unhealthy_count -eq 0 ]]; then
    log_recovery "All services recovered successfully"
    return 0
  else
    log_recovery "$unhealthy_count services still unhealthy"
    return 1
  fi
}

# Recover single service
recover_single_service() {
  local service="$1"

  log_recovery "Recovering service: $service"

  # Try restart first
  docker restart "$service" 2>/dev/null
  sleep 10

  # Check if healthy
  local health=$(docker inspect "$service" --format='{{.State.Health.Status}}' 2>/dev/null || echo "none")

  if [[ "$health" == "healthy" ]] || [[ "$health" == "none" ]]; then
    # Check if running
    if docker ps --filter "name=$service" --filter "status=running" -q | grep -q .; then
      log_recovery "Service $service recovered via restart"
      return 0
    fi
  fi

  # Try recreate
  docker compose up -d --force-recreate "$service"
  sleep 10

  # Final check
  if docker ps --filter "name=$service" --filter "status=running" -q | grep -q .; then
    log_recovery "Service $service recovered via recreate"
    return 0
  else
    log_recovery "Failed to recover service: $service"
    return 1
  fi
}

# Check for data corruption
check_data_corruption() {
  # Check PostgreSQL data directory
  local pg_data=".volumes/postgres"

  if [[ -d "$pg_data" ]]; then
    # Look for corruption indicators
    if find "$pg_data" -name "*.broken" -o -name "lost+found" | grep -q .; then
      return 0 # Corruption detected
    fi
  fi

  return 1 # No corruption detected
}

# Verify recovery
verify_recovery() {
  local service="$1"

  log_recovery "Verifying recovery for: $service"

  if [[ "$service" == "all" ]]; then
    # Run comprehensive health check
    nself doctor >"$RECOVERY_DIR/doctor-report.txt" 2>&1

    if grep -q "System is healthy" "$RECOVERY_DIR/doctor-report.txt"; then
      return 0
    else
      return 1
    fi
  else
    # Check specific service
    local health=$(docker inspect "$service" --format='{{.State.Health.Status}}' 2>/dev/null || echo "none")

    if [[ "$health" == "healthy" ]] || [[ "$health" == "none" ]]; then
      if docker ps --filter "name=$service" --filter "status=running" -q | grep -q .; then
        return 0
      fi
    fi

    return 1
  fi
}

# Trigger failover
trigger_failover() {
  local service="$1"

  log_recovery "Triggering failover for: $service"

  # Check if failover configuration exists
  if [[ -f "$RECOVERY_DIR/failover.conf" ]]; then
    source "$RECOVERY_DIR/failover.conf"

    if [[ -n "${FAILOVER_HOST:-}" ]]; then
      log_recovery "Failing over to: $FAILOVER_HOST"

      # Update configuration to point to failover
      sed -i.bak "s/HOST=.*/HOST=$FAILOVER_HOST/" .env.local

      # Restart affected services
      nself restart

      # Send notification
      send_failover_notification "$service" "$FAILOVER_HOST"
    fi
  else
    log_recovery "No failover configuration available"
  fi
}

# Send failover notification
send_failover_notification() {
  local service="$1"
  local failover_host="$2"

  # Send email notification if configured
  if [[ -n "${ALERT_EMAIL:-}" ]]; then
    echo "Service $service failed over to $failover_host" |
      mail -s "nself Failover Alert" "$ALERT_EMAIL" 2>/dev/null || true
  fi

  # Send webhook notification if configured
  if [[ -n "${ALERT_WEBHOOK:-}" ]]; then
    curl -X POST "$ALERT_WEBHOOK" \
      -H "Content-Type: application/json" \
      -d "{\"service\":\"$service\",\"event\":\"failover\",\"host\":\"$failover_host\"}" \
      2>/dev/null || true
  fi
}

# Rollback to checkpoint
rollback_to_checkpoint() {
  local checkpoint="$1"
  local checkpoint_dir="$RECOVERY_DIR/$checkpoint"

  if [[ ! -d "$checkpoint_dir" ]]; then
    echo "Checkpoint not found: $checkpoint"
    return 1
  fi

  log_recovery "Rolling back to checkpoint: $checkpoint"

  # Stop current services
  nself stop

  # Restore configuration
  cp "$checkpoint_dir/.env.local" . 2>/dev/null || true
  cp "$checkpoint_dir/docker-compose.yml" . 2>/dev/null || true

  # Rebuild and start
  nself build
  nself start

  log_recovery "Rollback completed"
}

# Export functions
export -f init_recovery
export -f create_recovery_checkpoint
export -f auto_recover
export -f recover_service
export -f verify_recovery
export -f rollback_to_checkpoint
