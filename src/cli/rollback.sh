#!/usr/bin/env bash
# rollback.sh - Rollback to previous version or backup

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source utilities
source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/utils/env.sh"
source "$SCRIPT_DIR/../lib/deployment/rollback.sh"

# Command function
cmd_rollback() {
  local target="${1:-}"

  # Check for help
  if [[ "$target" == "--help" ]] || [[ "$target" == "-h" ]] || [[ -z "$target" ]]; then
    show_rollback_help
    return 0
  fi

  show_command_header "nself rollback" "Rollback to previous version"

  case "$target" in
    latest)
      # Rollback to latest backup
      rollback_to_latest_backup
      ;;
    backup)
      # List backups and let user choose
      local backup_id="${2:-}"
      if [[ -z "$backup_id" ]]; then
        list_available_backups
        echo
        read -p "Enter backup ID to rollback to: " backup_id
      fi
      rollback_to_backup "$backup_id"
      ;;
    migration)
      # Rollback database migrations
      local steps="${2:-1}"
      rollback_migrations "$steps"
      ;;
    deployment)
      # Rollback to previous deployment
      rollback_deployment
      ;;
    config)
      # Rollback configuration changes
      rollback_config
      ;;
    *)
      # Try to interpret as backup ID or timestamp
      if [[ "$target" =~ ^[0-9]{8}_[0-9]{6}$ ]]; then
        rollback_to_backup "$target"
      else
        log_error "Unknown rollback target: $target"
        show_rollback_help
        return 1
      fi
      ;;
  esac
}

# Show help
show_rollback_help() {
  echo "nself rollback - Rollback to previous version or backup"
  echo ""
  echo "Usage: nself rollback <target> [options]"
  echo ""
  echo "Targets:"
  echo "  latest              Rollback to latest backup"
  echo "  backup [id]         Rollback to specific backup"
  echo "  migration [steps]   Rollback database migrations"
  echo "  deployment          Rollback to previous deployment"
  echo "  config              Rollback configuration changes"
  echo "  <backup-id>         Direct rollback to backup ID"
  echo ""
  echo "Options:"
  echo "  --dry-run           Show what would be rolled back"
  echo "  --force             Skip confirmation prompts"
  echo "  -h, --help          Show this help message"
  echo ""
  echo "Examples:"
  echo "  nself rollback latest"
  echo "  nself rollback backup 20240117_143022"
  echo "  nself rollback migration 2"
  echo "  nself rollback deployment"
  echo ""
  echo "Note: Rollback creates a backup of current state before reverting"
}

# List available backups
list_available_backups() {
  echo ""
  log_info "Available backups:"
  echo ""

  local backup_dir="${BACKUP_DIR:-./backups}"
  if [[ -d "$backup_dir" ]]; then
    ls -1 "$backup_dir" | grep -E '^[0-9]{8}_[0-9]{6}$' | sort -r | head -10 | while read -r backup; do
      local size=$(du -sh "$backup_dir/$backup" 2>/dev/null | cut -f1)
      local date=$(echo "$backup" | sed 's/\([0-9]\{4\}\)\([0-9]\{2\}\)\([0-9]\{2\}\)_\([0-9]\{2\}\)\([0-9]\{2\}\)\([0-9]\{2\}\)/\1-\2-\3 \4:\5:\6/')
      echo "  $backup  ($date, $size)"
    done
  fi

  # Check S3 backups if configured
  if [[ -n "${S3_BACKUP_BUCKET:-}" ]]; then
    echo ""
    log_info "S3 backups:"
    aws s3 ls "s3://$S3_BACKUP_BUCKET/backups/" 2>/dev/null | grep PRE | awk '{print $2}' | sed 's/\///' | sort -r | head -10
  fi
}

# Rollback to latest backup
rollback_to_latest_backup() {
  log_info "Finding latest backup..."

  local backup_dir="${BACKUP_DIR:-./backups}"
  local latest_backup=""

  if [[ -d "$backup_dir" ]]; then
    latest_backup=$(ls -1 "$backup_dir" | grep -E '^[0-9]{8}_[0-9]{6}$' | sort -r | head -1)
  fi

  if [[ -z "$latest_backup" ]]; then
    log_error "No backups found"
    log_info "Create a backup first with: nself backup create"
    return 1
  fi

  log_info "Latest backup: $latest_backup"
  rollback_to_backup "$latest_backup"
}

# Rollback to specific backup
rollback_to_backup() {
  local backup_id="$1"

  # Confirm rollback
  echo ""
  log_warning "This will rollback to backup: $backup_id"
  log_warning "Current state will be backed up first"
  echo ""
  read -p "Continue? (y/N) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Rollback cancelled"
    return 1
  fi

  # Create backup of current state
  log_info "Creating backup of current state..."
  nself backup create --label "before-rollback"

  # Stop services
  log_info "Stopping services..."
  nself stop

  # Restore backup
  log_info "Restoring from backup: $backup_id"
  nself backup restore "$backup_id"

  # Start services
  log_info "Starting services..."
  nself start

  log_success "Rollback completed successfully"
}

# Rollback database migrations
rollback_migrations() {
  local steps="${1:-1}"

  log_info "Rolling back $steps migration(s)..."

  # Verify admin secret is set
  if [[ -z "${HASURA_GRAPHQL_ADMIN_SECRET:-}" ]]; then
    log_error "HASURA_GRAPHQL_ADMIN_SECRET is not set"
    log_info "Set this in your .env file or environment"
    return 1
  fi

  # Use Hasura CLI to rollback migrations
  if command -v hasura &>/dev/null; then
    cd hasura
    hasura migrate apply --down "$steps" --admin-secret "${HASURA_GRAPHQL_ADMIN_SECRET}"
    cd ..
    log_success "Rolled back $steps migration(s)"
  else
    log_error "Hasura CLI not found"
    log_info "Install with: curl -L https://github.com/hasura/graphql-engine/raw/stable/cli/get.sh | bash"
    return 1
  fi
}

# Rollback deployment
rollback_deployment() {
  log_info "Rolling back to previous deployment..."

  # Check for deployment history
  local deploy_history=".nself/deployments"
  if [[ ! -d "$deploy_history" ]]; then
    log_error "No deployment history found"
    return 1
  fi

  # Get previous deployment
  local previous=$(ls -1 "$deploy_history" | sort -r | head -2 | tail -1)
  if [[ -z "$previous" ]]; then
    log_error "No previous deployment found"
    return 1
  fi

  log_info "Previous deployment: $previous"

  # Restore previous deployment
  cp "$deploy_history/$previous/docker-compose.yml" docker-compose.yml
  cp "$deploy_history/$previous/.env.local" .env.local 2>/dev/null || true

  # Restart services
  nself restart

  log_success "Rolled back to deployment: $previous"
}

# Rollback configuration
rollback_config() {
  log_info "Rolling back configuration..."

  # Check for config backups
  if [[ -f ".env.local.backup" ]]; then
    log_info "Found configuration backup"
    cp .env.local.backup .env.local
    log_success "Configuration rolled back"

    # Rebuild and restart
    log_info "Rebuilding with previous configuration..."
    nself build
    nself restart
  else
    log_error "No configuration backup found"
    log_info "Backups are created automatically when using 'nself prod' or 'nself init'"
    return 1
  fi
}

# Export for use as library
export -f cmd_rollback

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_rollback "$@"
  exit $?
fi
