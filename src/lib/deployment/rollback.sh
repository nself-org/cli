#!/usr/bin/env bash
# rollback.sh - Rollback functionality library


# Create deployment snapshot
create_deployment_snapshot() {

set -euo pipefail

  local label="${1:-$(date +%Y%m%d_%H%M%S)}"
  local deploy_dir=".nself/deployments/$label"

  # Create deployment directory
  mkdir -p "$deploy_dir"

  # Copy current configuration
  cp docker-compose.yml "$deploy_dir/" 2>/dev/null || true
  cp .env.local "$deploy_dir/" 2>/dev/null || true
  cp .env "$deploy_dir/" 2>/dev/null || true

  # Save service versions
  docker compose ps --format json >"$deploy_dir/services.json" 2>/dev/null || true

  # Save git commit if in git repo
  if git rev-parse --git-dir &>/dev/null; then
    git rev-parse HEAD >"$deploy_dir/git-commit.txt"
    git status --short >"$deploy_dir/git-status.txt"
  fi

  # Save timestamp
  date -u +"%Y-%m-%dT%H:%M:%SZ" >"$deploy_dir/timestamp.txt"

  log_debug "Created deployment snapshot: $label"
}

# Get deployment history
get_deployment_history() {
  local deploy_dir=".nself/deployments"

  if [[ ! -d "$deploy_dir" ]]; then
    echo "[]"
    return
  fi

  ls -1 "$deploy_dir" | sort -r | while read -r deployment; do
    local timestamp=$(cat "$deploy_dir/$deployment/timestamp.txt" 2>/dev/null || echo "unknown")
    local commit=$(cat "$deploy_dir/$deployment/git-commit.txt" 2>/dev/null || echo "none")
    echo "{\"id\":\"$deployment\",\"timestamp\":\"$timestamp\",\"commit\":\"$commit\"}"
  done
}

# Verify rollback target
verify_rollback_target() {
  local target="$1"
  local backup_dir="${BACKUP_DIR:-./backups}"

  # Check local backup
  if [[ -d "$backup_dir/$target" ]]; then
    return 0
  fi

  # Check S3 backup
  if [[ -n "${S3_BACKUP_BUCKET:-}" ]]; then
    if aws s3 ls "s3://$S3_BACKUP_BUCKET/backups/$target/" &>/dev/null; then
      return 0
    fi
  fi

  # Check deployment history
  if [[ -d ".nself/deployments/$target" ]]; then
    return 0
  fi

  return 1
}

# Perform rollback with validation
safe_rollback() {
  local target="$1"
  local type="${2:-backup}"

  # Verify target exists
  if ! verify_rollback_target "$target"; then
    log_error "Rollback target not found: $target"
    return 1
  fi

  # Create pre-rollback snapshot
  log_info "Creating pre-rollback snapshot..."
  create_deployment_snapshot "pre-rollback-$(date +%Y%m%d_%H%M%S)"

  # Perform rollback based on type
  case "$type" in
    backup)
      # Stop services
      docker compose down --remove-orphans

      # Restore from backup
      if [[ -d "./backups/$target" ]]; then
        cp -r "./backups/$target/"* .
      elif [[ -n "${S3_BACKUP_BUCKET:-}" ]]; then
        aws s3 sync "s3://$S3_BACKUP_BUCKET/backups/$target/" .
      fi

      # Start services
      docker compose up -d
      ;;

    deployment)
      # Restore deployment configuration
      cp ".nself/deployments/$target/docker-compose.yml" docker-compose.yml
      cp ".nself/deployments/$target/.env.local" .env.local 2>/dev/null || true

      # Restart services
      docker compose down --remove-orphans
      docker compose up -d
      ;;

    config)
      # Restore configuration files from latest backup
      latest_backup=$(ls -d _backup/*/ 2>/dev/null | sort -r | head -1)
      if [[ -n "$latest_backup" ]]; then
        if [[ -f "$latest_backup/.env.local" ]]; then
          cp "$latest_backup/.env.local" .env.local
        fi
        if [[ -f "$latest_backup/docker-compose.yml" ]]; then
          cp "$latest_backup/docker-compose.yml" docker-compose.yml
          log_info "Restored docker-compose.yml from $latest_backup"
        fi
      else
        log_warning "No backup found in _backup/ directory"
      fi
      ;;
  esac

  # Verify rollback success
  sleep 5
  if docker compose ps | grep -q "Up"; then
    log_success "Rollback completed successfully"
    return 0
  else
    log_error "Rollback may have failed - services not running"
    log_info "Check logs with: nself logs"
    return 1
  fi
}

# Rollback with health check
rollback_with_health_check() {
  local target="$1"

  # Perform rollback
  if ! safe_rollback "$target"; then
    return 1
  fi

  # Run health check
  log_info "Running health check..."
  nself doctor --quick

  # Check critical services
  local critical_services="postgres hasura nginx"
  for service in $critical_services; do
    if ! docker compose ps "$service" | grep -q "Up"; then
      log_error "Critical service not running: $service"
      log_info "Attempting to recover..."
      docker compose up -d "$service"
    fi
  done

  log_success "Rollback and health check completed"
}

# Export functions
export -f create_deployment_snapshot
export -f get_deployment_history
export -f verify_rollback_target
export -f safe_rollback
export -f rollback_with_health_check
