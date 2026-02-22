#!/usr/bin/env bash
# backup.sh - Backup & Recovery
# Consolidated command including: rollback, reset, clean subcommands

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source utilities (save SCRIPT_DIR before it gets redefined by sourced files)
CLI_DIR="$SCRIPT_DIR"
source "$CLI_DIR/../lib/utils/cli-output.sh"
source "$CLI_DIR/../lib/utils/env.sh"
source "$CLI_DIR/../lib/utils/docker.sh"
source "$CLI_DIR/../lib/utils/header.sh"
source "$CLI_DIR/../lib/hooks/pre-command.sh"
source "$CLI_DIR/../lib/hooks/post-command.sh"
source "$CLI_DIR/../lib/backup/pruning.sh" 2>/dev/null || true

# Backup configuration
BACKUP_ENABLED="${BACKUP_ENABLED:-${DB_BACKUP_ENABLED:-false}}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
BACKUP_RETENTION_COUNT="${BACKUP_RETENTION_COUNT:-10}"
BACKUP_RETENTION_MIN="${BACKUP_RETENTION_MIN:-3}"

# Ensure backup directory exists
mkdir -p "$BACKUP_DIR"

# =============================================================================
# HELP TEXT
# =============================================================================

show_help() {
  cat <<'EOF'
nself backup - Backup & Recovery

USAGE:
  nself backup <subcommand> [options]

SUBCOMMANDS:
  create [type] [name]           Create a backup
  list                           List available backups
  restore <name> [type]          Restore from backup
  verify [name|all]              Verify backup integrity
  prune [policy] [param]         Remove old backups
  clean                          Remove failed/partial backups
  rollback [target]              Rollback to previous version/backup
  reset [options]                Reset project to clean state
  schedule [action]              Manage automated backup scheduling

BACKUP TYPES:
  full                           All components (default)
  database                       Database only
  config                         Configuration only

ROLLBACK TARGETS:
  latest                         Rollback to latest backup
  backup [id]                    Rollback to specific backup
  migration [steps]              Rollback database migrations
  deployment                     Rollback to previous deployment
  config                         Rollback configuration changes

PRUNE POLICIES:
  age <days>                     Remove backups older than N days
  count <n>                      Keep only last N backups
  size <gb>                      Keep total size under N GB
  gfs                            Grandfather-Father-Son retention
  smart                          Apply smart retention policy

RESET OPTIONS:
  --force, -f                    Skip confirmation prompt
  --no-backup                    Don't create backup before reset
  --keep-env                     Preserve environment files

CLEAN OPTIONS:
  --images, -i                   Clean project images (default)
  --containers, -c               Remove stopped containers
  --volumes, -v                  Remove project volumes
  --networks, -n                 Remove project networks
  --builders, -b                 Remove Docker Buildx builders
  --all, -a                      Clean everything

EXAMPLES:
  # Create backups
  nself backup create                      # Full backup
  nself backup create database my-backup   # Database backup with custom name

  # List and restore
  nself backup list
  nself backup restore backup.tar.gz

  # Scheduling
  nself backup schedule create daily       # Daily backups at 2 AM
  nself backup schedule create hourly      # Hourly backups
  nself backup schedule list               # List schedules
  nself backup schedule disable daily      # Disable schedule
  nself backup schedule enable daily       # Enable schedule

  # Pruning
  nself backup prune age 7                 # Remove backups older than 7 days
  nself backup prune count 10              # Keep only last 10 backups
  nself backup prune smart                 # Apply smart retention

  # Rollback
  nself backup rollback latest
  nself backup rollback backup 20240117_143022

  # Reset
  nself backup reset --force
  nself backup reset --keep-env

  # Clean Docker resources
  nself backup clean --images
  nself backup clean --all

For more information: https://docs.nself.org/backup
EOF
}

# =============================================================================
# CREATE SUBCOMMAND
# =============================================================================

cmd_create() {
  local backup_type="${1:-full}"
  local custom_name="${2:-}"

  cli_section "Creating backup: $backup_type"
  printf "\n"

  # Determine backup name
  local timestamp=$(date +%Y%m%d_%H%M%S)
  local backup_name="${custom_name:-nself_backup_${backup_type}_${timestamp}.tar.gz}"
  local backup_path="$BACKUP_DIR/$backup_name"
  local temp_dir=$(mktemp -d)

  # Backup components based on type
  case "$backup_type" in
    full)
      cli_info "Backing up all components..."
      backup_database "$temp_dir"
      backup_config "$temp_dir"
      backup_volumes "$temp_dir"
      ;;
    database)
      cli_info "Backing up database only..."
      backup_database "$temp_dir"
      ;;
    config)
      cli_info "Backing up configuration only..."
      backup_config "$temp_dir"
      ;;
    *)
      cli_error "Unknown backup type: $backup_type"
      cli_info "Valid types: full, database, config"
      rm -rf "$temp_dir"
      return 1
      ;;
  esac

  # Create tarball
  cli_info "Creating archive..."
  tar -czf "$backup_path" -C "$temp_dir" . 2>/dev/null
  rm -rf "$temp_dir"

  # Calculate size
  local size=$(du -h "$backup_path" | cut -f1)

  printf "\n"
  cli_success "Backup created successfully!"
  cli_list_item "Location: $backup_path"
  cli_list_item "Size: $size"
  cli_list_item "Type: $backup_type"

  printf "\n"
  cli_info "To restore this backup, run:"
  cli_indent "nself backup restore $backup_name"
}

# Helper functions for backup
backup_database() {
  local dest_dir="$1"
  local db_backup_dir="$dest_dir/database"
  mkdir -p "$db_backup_dir"

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  if docker ps --format "{{.Names}}" | grep -q "${project_name}_postgres"; then
    cli_list_item "Dumping PostgreSQL database..."
    local db_name="${POSTGRES_DB:-postgres}"
    local db_user="${POSTGRES_USER:-postgres}"

    docker exec "${project_name}_postgres" pg_dumpall -U "$db_user" \
      >"$db_backup_dir/postgres_dump.sql" 2>/dev/null || {
      cli_warning "Failed to dump database"
    }
  else
    cli_warning "PostgreSQL not running, skipping database backup"
  fi
}

backup_config() {
  local dest_dir="$1"
  local config_backup_dir="$dest_dir/config"
  mkdir -p "$config_backup_dir"

  cli_list_item "Backing up configuration files..."

  for env_file in .env .env.dev .env.production .env.local; do
    [[ -f "$env_file" ]] && cp "$env_file" "$config_backup_dir/"
  done

  for compose_file in docker-compose.yml docker-compose.override.yml; do
    [[ -f "$compose_file" ]] && cp "$compose_file" "$config_backup_dir/"
  done

  [[ -d "./nginx" ]] && cp -r ./nginx "$config_backup_dir/"
}

backup_volumes() {
  local dest_dir="$1"
  local volumes_backup_dir="$dest_dir/volumes"
  mkdir -p "$volumes_backup_dir"

  cli_list_item "Backing up Docker volumes..."

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"
  local volumes=$(docker volume ls --format "{{.Name}}" | grep -E "^${project_name}" || true)

  if [[ -n "$volumes" ]]; then
    for volume in $volumes; do
      local volume_name=$(printf '%s\n' "$volume" | sed "s/${project_name}_//")
      docker run --rm -v "$volume:/data" -v "$volumes_backup_dir:/backup" \
        alpine tar -czf "/backup/${volume_name}.tar.gz" -C /data . 2>/dev/null || {
        cli_warning "Failed to backup volume: $volume"
      }
    done
  fi
}

# =============================================================================
# LIST SUBCOMMAND
# =============================================================================

cmd_list() {
  cli_section "Available Backups"
  printf "\n"

  if [[ -d "$BACKUP_DIR" ]] && [[ -n "$(ls -A "$BACKUP_DIR" 2>/dev/null)" ]]; then
    cli_table_header "Name" "Size" "Created"

    for backup in "$BACKUP_DIR"/*.tar.gz; do
      if [[ -f "$backup" ]]; then
        local name=$(basename "$backup")
        local size=$(du -h "$backup" | cut -f1)
        local created=$(stat -f "%Sm" -t "%Y-%m-%d %H:%M" "$backup" 2>/dev/null ||
          stat -c "%y" "$backup" 2>/dev/null | cut -d' ' -f1,2)
        cli_table_row "$name" "$size" "$created"
      fi
    done

    cli_table_footer "Name" "Size" "Created"
  else
    cli_info "No local backups found"
  fi
}

# =============================================================================
# RESTORE SUBCOMMAND
# =============================================================================

cmd_restore() {
  local backup_name="$1"
  local restore_type="${2:-full}"

  if [[ -z "$backup_name" ]]; then
    cli_error "Backup name required"
    printf "Usage: nself backup restore <backup_name> [full|database|config]\n"
    return 1
  fi

  cli_section "Restoring from backup"
  printf "\n"

  # Find backup file
  local backup_path=""
  if [[ -f "$BACKUP_DIR/$backup_name" ]]; then
    backup_path="$BACKUP_DIR/$backup_name"
  elif [[ -f "$backup_name" ]]; then
    backup_path="$backup_name"
  else
    cli_error "Backup not found: $backup_name"
    return 1
  fi

  cli_warning "This will overwrite existing data!"
  read -p "Are you sure? (y/N): " confirm
  if [[ "$confirm" != "y" ]] && [[ "$confirm" != "Y" ]]; then
    cli_info "Restore cancelled"
    return 0
  fi

  local temp_dir=$(mktemp -d)
  cli_info "Extracting backup..."
  tar -xzf "$backup_path" -C "$temp_dir"

  # Restore based on type
  case "$restore_type" in
    full)
      restore_database "$temp_dir"
      restore_config "$temp_dir"
      restore_volumes "$temp_dir"
      ;;
    database)
      restore_database "$temp_dir"
      ;;
    config)
      restore_config "$temp_dir"
      ;;
  esac

  rm -rf "$temp_dir"

  printf "\n"
  cli_success "Restore completed successfully!"
  printf "\n"
  cli_info "Next steps:"
  cli_list_numbered 1 "Run 'nself restart' to apply restored configuration"
  cli_list_numbered 2 "Run 'nself status' to verify services"
}

# Helper functions for restore
restore_database() {
  local source_dir="$1"

  if [[ -f "$source_dir/database/postgres_dump.sql" ]]; then
    cli_list_item "Restoring PostgreSQL database..."

    load_env_with_priority
    local project_name="${PROJECT_NAME:-nself}"
    local db_user="${POSTGRES_USER:-postgres}"

    if ! docker ps --format "{{.Names}}" | grep -q "${project_name}_postgres"; then
      cli_info "Starting PostgreSQL..."
      docker compose up -d postgres
      sleep 5
    fi

    docker exec -i "${project_name}_postgres" psql -U "$db_user" \
      <"$source_dir/database/postgres_dump.sql" 2>/dev/null || {
      cli_error "Failed to restore database"
      return 1
    }
  fi
}

restore_config() {
  local source_dir="$1"

  if [[ -d "$source_dir/config" ]]; then
    cli_list_item "Restoring configuration files..."
    cp -f "$source_dir/config"/.env* . 2>/dev/null || true
    cp -f "$source_dir/config"/docker-compose*.yml . 2>/dev/null || true

    if [[ -d "$source_dir/config/nginx" ]]; then
      rm -rf ./nginx
      cp -r "$source_dir/config/nginx" ./
    fi
  fi
}

restore_volumes() {
  local source_dir="$1"

  if [[ -d "$source_dir/volumes" ]]; then
    cli_list_item "Restoring Docker volumes..."

    for volume_archive in "$source_dir/volumes"/*.tar.gz; do
      if [[ -f "$volume_archive" ]]; then
        local volume_name=$(basename "$volume_archive" .tar.gz)
        load_env_with_priority
        local project_name="${PROJECT_NAME:-nself}"
        local full_volume_name="${project_name}_${volume_name}"

        docker volume create "$full_volume_name" >/dev/null 2>&1 || true
        docker run --rm -v "$full_volume_name:/data" -v "$source_dir/volumes:/backup" \
          alpine tar -xzf "/backup/${volume_name}.tar.gz" -C /data 2>/dev/null || {
          cli_warning "Failed to restore volume: $volume_name"
        }
      fi
    done
  fi
}

# =============================================================================
# PRUNE SUBCOMMAND
# =============================================================================

cmd_prune() {
  local policy="${1:-age}"
  local param="${2:-}"

  cli_section "Pruning old backups"
  printf "\n"

  case "$policy" in
    age)
      local days="${param:-$BACKUP_RETENTION_DAYS}"
      prune_by_age "$days"
      ;;
    count)
      local count="${param:-$BACKUP_RETENTION_COUNT}"
      prune_by_count "$count"
      ;;
    smart)
      prune_smart_policy
      ;;
    *)
      cli_error "Unknown prune policy: $policy"
      cli_info "Valid policies: age, count, smart"
      return 1
      ;;
  esac
}

prune_by_age() {
  local days="$1"

  cli_info "Removing backups older than $days days..."
  printf "\n"

  local count=0
  local total_backups=$(find "$BACKUP_DIR" -name "*.tar.gz" -type f 2>/dev/null | wc -l | tr -d ' ')

  if [[ -d "$BACKUP_DIR" ]]; then
    while IFS= read -r backup; do
      if [[ -f "$backup" ]]; then
        if [[ $((total_backups - count)) -le ${BACKUP_RETENTION_MIN:-3} ]]; then
          cli_info "Keeping (minimum retention): $(basename "$backup")"
        else
          cli_info "Removing: $(basename "$backup")"
          rm -f "$backup"
          count=$((count + 1))
        fi
      fi
    done < <(find "$BACKUP_DIR" -name "*.tar.gz" -type f -mtime +${days} | sort)
  fi

  printf "\n"
  if [[ $count -gt 0 ]]; then
    cli_success "Removed $count backup(s)"
  else
    cli_info "No backups older than $days days found"
  fi
}

prune_by_count() {
  local count="$1"

  cli_info "Keeping only last $count backups..."
  printf "\n"

  local backups=($(ls -1t "$BACKUP_DIR"/*.tar.gz 2>/dev/null))
  local removed=0

  for ((i = count; i < ${#backups[@]}; i++)); do
    cli_info "Removing: $(basename "${backups[$i]}")"
    rm -f "${backups[$i]}"
    removed=$((removed + 1))
  done

  printf "\n"
  if [[ $removed -gt 0 ]]; then
    cli_success "Removed $removed backup(s)"
  else
    cli_info "No backups to remove"
  fi
}

prune_smart_policy() {
  cli_info "Applying smart retention policy..."
  printf "\n"

  # Keep:
  # - All backups from last 24 hours
  # - Daily backups for last week
  # - Weekly backups for last month

  local now=$(date +%s)
  local count=0
  local kept=0

  if [[ -d "$BACKUP_DIR" ]]; then
    for backup in "$BACKUP_DIR"/*.tar.gz; do
      if [[ -f "$backup" ]]; then
        local name=$(basename "$backup")
        local backup_time=$(stat -f %m "$backup" 2>/dev/null || stat -c %Y "$backup")
        local age_days=$(((now - backup_time) / 86400))

        local keep=false
        local reason=""

        if [[ $age_days -le 1 ]]; then
          keep=true
          reason="last 24 hours"
        elif [[ $age_days -le 7 ]]; then
          keep=true
          reason="last week"
        fi

        if [[ "$keep" == true ]]; then
          cli_list_item "Keeping ($reason): $name"
          kept=$((kept + 1))
        else
          cli_info "Removing: $name"
          rm -f "$backup"
          count=$((count + 1))
        fi
      fi
    done
  fi

  printf "\n"
  cli_success "Smart policy applied: Kept $kept, removed $count backup(s)"
}

# =============================================================================
# ROLLBACK SUBCOMMAND (from rollback.sh)
# =============================================================================

cmd_rollback() {
  local target="${1:-latest}"

  cli_section "Rolling back to: $target"
  printf "\n"

  case "$target" in
    latest)
      rollback_to_latest_backup
      ;;
    backup)
      local backup_id="${2:-}"
      if [[ -z "$backup_id" ]]; then
        cmd_list
        read -p "Enter backup ID to rollback to: " backup_id
      fi
      cmd_restore "$backup_id"
      ;;
    *)
      cli_error "Unknown rollback target: $target"
      cli_info "Valid targets: latest, backup [id]"
      return 1
      ;;
  esac
}

rollback_to_latest_backup() {
  local latest_backup=$(ls -1t "$BACKUP_DIR"/*.tar.gz 2>/dev/null | head -1)

  if [[ -z "$latest_backup" ]]; then
    cli_error "No backups found"
    cli_info "Create a backup first with: nself backup create"
    return 1
  fi

  cli_info "Latest backup: $(basename "$latest_backup")"
  cmd_restore "$(basename "$latest_backup")"
}

# =============================================================================
# RESET SUBCOMMAND (from reset.sh)
# =============================================================================

cmd_reset() {
  local force_reset=false
  local create_backup=true
  local keep_env=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force | -f)
        force_reset=true
        shift
        ;;
      --no-backup)
        create_backup=false
        shift
        ;;
      --keep-env)
        keep_env=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  cli_section "Reset project to clean state"
  printf "\n"

  cli_warning "This will:"
  cli_list_item "Stop and remove all containers"
  cli_list_item "Delete all Docker volumes"
  cli_list_item "Remove all generated files"
  printf "\n"

  if [[ "$force_reset" != "true" ]]; then
    read -p "Are you sure you want to reset everything? (y/N): " confirm
    if [[ "$confirm" != "y" ]] && [[ "$confirm" != "Y" ]]; then
      cli_info "Reset cancelled"
      return 1
    fi
  fi

  # Create backup if requested
  if [[ "$create_backup" == "true" ]]; then
    cmd_create "full" "before-reset-$(date +%Y%m%d_%H%M%S)"
  fi

  # Stop and remove containers
  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  if [[ -f "docker-compose.yml" ]]; then
    docker compose down -v >/dev/null 2>&1 || true
  fi

  # Remove project containers
  local containers=$(docker ps -aq --filter "name=${project_name}")
  if [[ -n "$containers" ]]; then
    printf '%s\n' "$containers" | xargs docker rm -f >/dev/null 2>&1 || true
  fi

  # Remove volumes
  local volumes=$(docker volume ls -q | grep "^${project_name}_")
  if [[ -n "$volumes" ]]; then
    printf '%s\n' "$volumes" | xargs docker volume rm -f >/dev/null 2>&1 || true
  fi

  # Remove network
  docker network rm "${project_name}_network" >/dev/null 2>&1 || true

  # Remove generated files
  rm -rf docker-compose.yml nginx postgres monitoring services .nself

  printf "\n"
  cli_success "Project reset complete!"
  printf "\n"
  cli_info "Next steps:"
  cli_list_numbered 1 "nself init    # Create new configuration"
  cli_list_numbered 2 "nself build   # Generate infrastructure"
  cli_list_numbered 3 "nself start   # Start services"
}

# =============================================================================
# SCHEDULE SUBCOMMAND
# =============================================================================

cmd_schedule() {
  local action="${1:-list}"
  shift || true

  case "$action" in
    create)
      schedule_create "$@"
      ;;
    list | ls)
      schedule_list
      ;;
    enable)
      schedule_toggle "$1" "true"
      ;;
    disable)
      schedule_toggle "$1" "false"
      ;;
    remove | delete)
      schedule_remove "$1"
      ;;
    status)
      schedule_status
      ;;
    *)
      cli_error "Unknown schedule action: $action"
      printf "Valid actions: create, list, enable, disable, remove, status\n"
      return 1
      ;;
  esac
}

schedule_create() {
  local frequency="${1:-daily}"
  local backup_type="${2:-full}"

  cli_section "Creating backup schedule"
  printf "\n"

  # Validate frequency
  case "$frequency" in
    hourly | daily | weekly | monthly) ;;
    *)
      cli_error "Invalid frequency: $frequency"
      cli_info "Valid frequencies: hourly, daily, weekly, monthly"
      return 1
      ;;
  esac

  # Create cron job
  local cron_schedule=""
  case "$frequency" in
    hourly)
      cron_schedule="0 * * * *"
      ;;
    daily)
      cron_schedule="0 2 * * *" # 2 AM daily
      ;;
    weekly)
      cron_schedule="0 2 * * 0" # 2 AM on Sundays
      ;;
    monthly)
      cron_schedule="0 2 1 * *" # 2 AM on 1st of month
      ;;
  esac

  local cron_command="cd $(pwd) && nself backup create $backup_type automated_${frequency}_\$(date +\\%Y\\%m\\%d_\\%H\\%M\\%S) >> $(pwd)/backups/backup.log 2>&1"
  local cron_entry="$cron_schedule $cron_command # nself-backup-$frequency"

  # Add to crontab
  (
    crontab -l 2>/dev/null | grep -v "nself-backup-$frequency"
    printf "%s\n" "$cron_entry"
  ) | crontab - 2>/dev/null

  if [[ $? -eq 0 ]]; then
    cli_success "Backup schedule created!"
    cli_list_item "Frequency: $frequency"
    cli_list_item "Type: $backup_type"
    cli_list_item "Next run: $(next_run_time "$frequency")"
    printf "\n"
    cli_info "Backups will be created automatically and logged to backups/backup.log"
  else
    cli_error "Failed to create cron job"
    cli_info "You may need to use 'nself backup schedule create-systemd' instead"
    return 1
  fi
}

schedule_list() {
  cli_section "Backup Schedules"
  printf "\n"

  local schedules=$(crontab -l 2>/dev/null | grep "nself-backup-")

  if [[ -z "$schedules" ]]; then
    cli_info "No backup schedules found"
    printf "\n"
    cli_info "Create one with: nself backup schedule create daily"
    return 0
  fi

  cli_table_header "Frequency" "Type" "Next Run" "Status"

  while IFS= read -r line; do
    local frequency=$(printf '%s\n' "$line" | sed -n 's/.*nself-backup-\([^"]*\).*/\1/p')
    local type=$(printf '%s\n' "$line" | sed -n 's/.*backup create \([^ ]*\).*/\1/p')
    local next_run=$(next_run_time "$frequency")

    # Check if commented (disabled)
    local status="Enabled"
    if printf '%s\n' "$line" | grep -q "^#"; then
      status="Disabled"
    fi

    cli_table_row "$frequency" "$type" "$next_run" "$status"
  done <<<"$schedules"

  cli_table_footer "Frequency" "Type" "Next Run" "Status"
}

schedule_toggle() {
  local frequency="$1"
  local enable="$2"

  if [[ -z "$frequency" ]]; then
    cli_error "Frequency required"
    printf "Usage: nself backup schedule enable|disable <frequency>\n"
    return 1
  fi

  local temp_cron=$(mktemp)
  crontab -l 2>/dev/null >"$temp_cron"

  if [[ "$enable" == "true" ]]; then
    # Remove comment to enable
    sed -i.bak "s/^#\(.*nself-backup-$frequency\)/\1/" "$temp_cron"
    cli_success "Schedule '$frequency' enabled"
  else
    # Add comment to disable
    sed -i.bak "s/^\([^#].*nself-backup-$frequency\)/#\1/" "$temp_cron"
    cli_success "Schedule '$frequency' disabled"
  fi

  crontab "$temp_cron"
  rm -f "$temp_cron" "$temp_cron.bak"
}

schedule_remove() {
  local frequency="$1"

  if [[ -z "$frequency" ]]; then
    cli_error "Frequency required"
    printf "Usage: nself backup schedule remove <frequency>\n"
    return 1
  fi

  crontab -l 2>/dev/null | grep -v "nself-backup-$frequency" | crontab - 2>/dev/null

  cli_success "Schedule '$frequency' removed"
}

schedule_status() {
  cli_section "Backup Schedule Status"
  printf "\n"

  # Check if cron is available
  if ! command -v crontab >/dev/null 2>&1; then
    cli_warning "crontab not available on this system"
    cli_info "Consider using systemd timers instead"
    return 1
  fi

  # Check recent backup activity
  if [[ -f "$BACKUP_DIR/backup.log" ]]; then
    cli_info "Recent backup activity:"
    printf "\n"
    tail -n 20 "$BACKUP_DIR/backup.log" | while read -r line; do
      cli_list_item "$line"
    done
  else
    cli_info "No backup log found yet"
  fi

  # Show next scheduled runs
  printf "\n"
  cli_info "Next scheduled runs:"
  printf "\n"

  crontab -l 2>/dev/null | grep "nself-backup-" | while IFS= read -r line; do
    local frequency=$(printf '%s\n' "$line" | sed -n 's/.*nself-backup-\([^"]*\).*/\1/p')
    local next_run=$(next_run_time "$frequency")
    cli_list_item "$frequency: $next_run"
  done
}

next_run_time() {
  local frequency="$1"
  local now=$(date +%s)

  case "$frequency" in
    hourly)
      date -d "@$((now + 3600))" "+%Y-%m-%d %H:00" 2>/dev/null || date -r $((now + 3600)) "+%Y-%m-%d %H:00"
      ;;
    daily)
      date -d "tomorrow 02:00" "+%Y-%m-%d 02:00" 2>/dev/null || date -v +1d -v 2H -v 0M "+%Y-%m-%d 02:00" 2>/dev/null || echo "Tomorrow 02:00"
      ;;
    weekly)
      echo "Next Sunday 02:00"
      ;;
    monthly)
      echo "1st of next month 02:00"
      ;;
    *)
      echo "Unknown"
      ;;
  esac
}

# =============================================================================
# VERIFY SUBCOMMAND
# =============================================================================

cmd_verify() {
  local backup_name="${1:-}"

  if [[ -z "$backup_name" ]]; then
    cli_error "Backup name required"
    printf "Usage: nself backup verify <backup_name>\n"
    return 1
  fi

  cli_section "Verifying backup"
  printf "\n"

  local backup_path=""
  if [[ -f "$BACKUP_DIR/$backup_name" ]]; then
    backup_path="$BACKUP_DIR/$backup_name"
  elif [[ -f "$backup_name" ]]; then
    backup_path="$backup_name"
  else
    cli_error "Backup not found: $backup_name"
    return 1
  fi

  cli_info "Checking: $backup_path"

  if tar -tzf "$backup_path" >/dev/null 2>&1; then
    cli_success "Backup is valid"
    return 0
  else
    cli_error "Backup is corrupt or invalid"
    return 1
  fi
}

# =============================================================================
# RETENTION SUBCOMMAND
# =============================================================================

cmd_retention() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cli_section "Backup Retention Configuration"
      printf "\n"
      cli_list_item "Retention days:  ${BACKUP_RETENTION_DAYS:-30}"
      cli_list_item "Maximum backups: ${BACKUP_RETENTION_COUNT:-10}"
      cli_list_item "Minimum backups: ${BACKUP_RETENTION_MIN:-3}"
      ;;
    set)
      local setting="${1:-}"
      local value="${2:-}"

      if [[ -z "$setting" ]] || [[ -z "$value" ]]; then
        cli_error "Usage: nself backup retention set <days|count|min> <value>"
        return 1
      fi

      local env_key=""
      case "$setting" in
        days) env_key="BACKUP_RETENTION_DAYS" ;;
        count) env_key="BACKUP_RETENTION_COUNT" ;;
        min) env_key="BACKUP_RETENTION_MIN" ;;
        *)
          cli_error "Unknown setting: $setting (valid: days, count, min)"
          return 1
          ;;
      esac

      if [[ -f ".env" ]]; then
        local tmp_file
        tmp_file=$(mktemp)
        if grep -q "^${env_key}=" .env 2>/dev/null; then
          grep -v "^${env_key}=" .env > "$tmp_file" || true
        else
          cp .env "$tmp_file"
        fi
        printf "%s=%s\n" "$env_key" "$value" >> "$tmp_file"
        cp "$tmp_file" .env
        rm -f "$tmp_file"
      else
        printf "%s=%s\n" "$env_key" "$value" > .env
      fi

      case "$setting" in
        days)  cli_success "Retention days set to: $value" ;;
        count) cli_success "Retention count set to: $value" ;;
        min)   cli_success "Retention minimum set to: $value" ;;
      esac
      ;;
    *)
      cli_error "Unknown retention action: $action (valid: status, set)"
      return 1
      ;;
  esac
}

# =============================================================================
# CLOUD SUBCOMMAND
# =============================================================================

cmd_cloud() {
  local action="${1:-status}"
  shift || true

  case "$action" in
    status)
      cli_section "Cloud Backup Status"
      printf "\n"
      local provider="${BACKUP_CLOUD_PROVIDER:-none}"
      cli_list_item "Provider: $provider"
      if [[ "$provider" == "none" ]] || [[ -z "$provider" ]]; then
        cli_info "No cloud provider configured"
        cli_info "Configure with: nself backup cloud configure <s3|gcs|azure|b2>"
      fi
      ;;
    configure)
      local provider="${1:-}"
      if [[ -z "$provider" ]]; then
        cli_error "Provider required: nself backup cloud configure <s3|gcs|azure|b2>"
        return 1
      fi
      cli_info "Configure $provider cloud backup in .env"
      cli_list_item "BACKUP_CLOUD_PROVIDER=$provider"
      ;;
    *)
      cli_error "Unknown cloud action: $action (valid: status, configure)"
      return 1
      ;;
  esac
}

# =============================================================================
# CLEAN SUBCOMMAND (from clean.sh)
# =============================================================================

cmd_clean() {
  local clean_images=false
  local clean_containers=false
  local clean_volumes=false
  local clean_all=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --images | -i)
        clean_images=true
        shift
        ;;
      --containers | -c)
        clean_containers=true
        shift
        ;;
      --volumes | -v)
        clean_volumes=true
        shift
        ;;
      --all | -a)
        clean_all=true
        shift
        ;;
      *)
        shift
        ;;
    esac
  done

  # Default to images if nothing specified
  if [[ "$clean_images" == "false" ]] && [[ "$clean_containers" == "false" ]] &&
    [[ "$clean_volumes" == "false" ]] && [[ "$clean_all" == "false" ]]; then
    clean_images=true
  fi

  cli_section "Cleaning Docker resources"
  printf "\n"

  load_env_with_priority
  local project_name="${PROJECT_NAME:-nself}"

  # Clean containers
  if [[ "$clean_all" == "true" ]] || [[ "$clean_containers" == "true" ]]; then
    cli_info "Cleaning stopped containers..."
    local stopped=$(docker ps -a --filter "name=${project_name}" --filter "status=exited" -q)
    if [[ -n "$stopped" ]]; then
      printf '%s\n' "$stopped" | xargs docker rm >/dev/null 2>&1
      cli_success "Removed stopped containers"
    else
      cli_info "No stopped containers to clean"
    fi
  fi

  # Clean images
  if [[ "$clean_all" == "true" ]] || [[ "$clean_images" == "true" ]]; then
    cli_info "Cleaning project images..."
    local images=$(docker images | grep -E "^${project_name}" | awk '{print $3}' | sort -u)
    if [[ -n "$images" ]]; then
      printf '%s\n' "$images" | xargs docker rmi -f >/dev/null 2>&1
      cli_success "Removed project images"
    else
      cli_info "No images to clean"
    fi

    docker builder prune -f >/dev/null 2>&1
  fi

  # Clean volumes
  if [[ "$clean_all" == "true" ]] || [[ "$clean_volumes" == "true" ]]; then
    cli_warning "This will delete all data in project volumes!"
    read -p "Are you sure? (y/N): " confirm
    if [[ "$confirm" == "y" ]] || [[ "$confirm" == "Y" ]]; then
      local volumes=$(docker volume ls --filter "name=${project_name}" -q)
      if [[ -n "$volumes" ]]; then
        printf '%s\n' "$volumes" | xargs docker volume rm >/dev/null 2>&1
        cli_success "Removed project volumes"
      else
        cli_info "No volumes to clean"
      fi
    fi
  fi

  # System prune if --all
  if [[ "$clean_all" == "true" ]]; then
    docker system prune -f >/dev/null 2>&1
    cli_success "System cleanup complete"
  fi

  printf "\n"
  cli_success "Cleanup complete!"
}

# =============================================================================
# MAIN COMMAND ROUTER
# =============================================================================

main() {
  local subcommand="${1:-help}"

  # Check for help
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]] || [[ "$subcommand" == "help" ]]; then
    show_help
    return 0
  fi

  shift || true

  # Route to subcommand
  case "$subcommand" in
    create)
      cmd_create "$@"
      ;;
    list | ls)
      cmd_list "$@"
      ;;
    restore)
      cmd_restore "$@"
      ;;
    prune)
      cmd_prune "$@"
      ;;
    rollback)
      cmd_rollback "$@"
      ;;
    reset)
      cmd_reset "$@"
      ;;
    clean)
      cmd_clean "$@"
      ;;
    verify)
      cmd_verify "$@"
      ;;
    retention)
      cmd_retention "$@"
      ;;
    cloud)
      cmd_cloud "$@"
      ;;
    schedule)
      cmd_schedule "$@"
      ;;
    help | -h | --help)
      show_help
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      show_help
      return 1
      ;;
  esac
}

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_help
      exit 0
    fi
  done
  pre_command "backup" || exit $?
  main "$@"
  exit_code=$?
  post_command "backup" $exit_code
  exit $exit_code
fi
