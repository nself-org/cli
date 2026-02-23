#!/usr/bin/env bash
set -euo pipefail

# restore.sh - Restore backed up configuration files from tar.gz archives

# Source display utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/utils/header.sh"

# Main restore function
cmd_restore() {
  local backup_dir="_backup"
  local specific_backup=""
  local list_only=false
  local verbose=false
  local force=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --list | -l)
        list_only=true
        shift
        ;;
      --verbose | -v)
        verbose=true
        shift
        ;;
      --force | -f)
        force=true
        shift
        ;;
      --help | -h)
        show_restore_help
        return 0
        ;;
      *)
        # Assume it's a backup file or timestamp
        specific_backup="$1"
        shift
        ;;
    esac
  done

  # Show header
  show_command_header "nself restore" "Restore configuration from backup"

  # Check if backup directory exists
  if [[ ! -d "$backup_dir" ]]; then
    log_error "No backup directory found"
    echo "Run 'nself reset' to create a backup first"
    return 1
  fi

  # Get list of available tar.gz backups (sorted by newest first)
  local backups=($(ls -1t "$backup_dir"/*.tar.gz 2>/dev/null || true))

  if [[ ${#backups[@]} -eq 0 ]]; then
    log_error "No backups found"
    echo "Run 'nself reset' to create a backup first"
    return 1
  fi

  # List mode
  if [[ "$list_only" == true ]]; then
    echo "Available backups:"
    echo
    for backup in "${backups[@]}"; do
      local filename=$(basename "$backup")
      local filesize=$(ls -lh "$backup" | awk '{print $5}')

      # Extract timestamp from filename (assumes format: projectname_YYYYMMDD_HHMMSS.tar.gz)
      local timestamp=$(echo "$filename" | grep -oE '[0-9]{8}_[0-9]{6}' || echo "unknown")

      if [[ "$timestamp" != "unknown" ]]; then
        local date_formatted="${timestamp:0:4}-${timestamp:4:2}-${timestamp:6:2} ${timestamp:9:2}:${timestamp:11:2}:${timestamp:13:2}"
        printf "  ${COLOR_BLUE}$filename${COLOR_RESET}\n"
        printf "    Date: $date_formatted\n"
        printf "    Size: $filesize\n"
      else
        printf "  ${COLOR_BLUE}$filename${COLOR_RESET} (${filesize})\n"
      fi
      echo
    done
    echo "To restore a specific backup:"
    printf "  ${COLOR_BLUE}nself restore $filename${COLOR_RESET}\n"
    echo
    echo "To restore the most recent backup:"
    printf "  ${COLOR_BLUE}nself restore${COLOR_RESET}\n"
    return 0
  fi

  # Determine which backup to restore
  local backup_to_restore=""
  if [[ -n "$specific_backup" ]]; then
    # Check if it's a full filename
    if [[ -f "$backup_dir/$specific_backup" ]]; then
      backup_to_restore="$backup_dir/$specific_backup"
    # Check if it's a timestamp pattern that matches a file
    elif compgen -G "$backup_dir/*${specific_backup}*.tar.gz" >/dev/null; then
      backup_to_restore=$(ls -1t "$backup_dir"/*${specific_backup}*.tar.gz 2>/dev/null | head -1)
    else
      log_error "Backup not found: $specific_backup"
      echo
      echo "Run 'nself restore --list' to see available backups"
      return 1
    fi
  else
    # Use most recent backup
    backup_to_restore="${backups[0]}"
  fi

  local backup_filename=$(basename "$backup_to_restore")

  # Show what we're restoring
  echo "Restoring from: ${COLOR_BLUE}$backup_filename${COLOR_RESET}"
  echo

  # Show contents of the archive
  if [[ "$verbose" == true ]] || [[ "$force" != true ]]; then
    echo "Contents of backup:"
    tar -tzf "$backup_to_restore" | sed 's/^/  • /'
    echo
  fi

  # Ask for confirmation unless force flag is used
  if [[ "$force" != true ]]; then
    printf "${COLOR_YELLOW}⚠${COLOR_RESET}  This will overwrite existing files\n"
    printf "%s" "Continue? [y/N]: "
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
      echo "Restore cancelled"
      return 0
    fi
    echo
  fi

  # Create backup of current state before restoring
  local pre_restore_backup="_backup/before-restore_$(date +%Y%m%d_%H%M%S)"
  local files_to_backup=""

  # Check what files exist that would be overwritten
  for file in $(tar -tzf "$backup_to_restore"); do
    # Skip directories
    [[ "$file" =~ /$ ]] && continue

    if [[ -f "$file" ]] || [[ -d "$file" ]]; then
      files_to_backup="$files_to_backup $file"
    fi
  done

  if [[ -n "$files_to_backup" ]]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Creating backup of current files..."
    mkdir -p "$pre_restore_backup"

    for item in $files_to_backup; do
      if [[ -f "$item" ]]; then
        mkdir -p "$pre_restore_backup/$(dirname "$item")"
        cp "$item" "$pre_restore_backup/$item" 2>/dev/null || true
      elif [[ -d "$item" ]]; then
        mkdir -p "$pre_restore_backup/$(dirname "$item")"
        cp -r "$item" "$pre_restore_backup/$item" 2>/dev/null || true
      fi
    done

    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Backed up current files to: $pre_restore_backup\n"
    echo
  fi

  # Extract the archive
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Extracting backup..."

  if tar -xzf "$backup_to_restore" 2>/dev/null; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Extracted backup successfully                    \n"
  else
    printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to extract backup                         \n"
    return 1
  fi

  # Count restored files
  local file_count=$(tar -tzf "$backup_to_restore" | grep -v '/$' | wc -l | tr -d ' ')

  echo
  log_success "Restored $file_count files from backup"

  # Show next steps
  echo
  printf "${COLOR_CYAN}➞ Next Steps${COLOR_RESET}\n"
  echo
  printf "  ${COLOR_BLUE}nself build${COLOR_RESET}    ${COLOR_DIM}# Generate infrastructure from restored config${COLOR_RESET}\n"
  printf "  ${COLOR_BLUE}nself start${COLOR_RESET}    ${COLOR_DIM}# Start services${COLOR_RESET}\n"
  echo

  if [[ -n "$files_to_backup" ]]; then
    printf "${COLOR_DIM}Previous files backed up to: $pre_restore_backup${COLOR_RESET}\n"
    echo
  fi

  return 0
}

# Show help
show_restore_help() {
  echo "Usage: nself restore [options] [backup]"
  echo
  echo "Restore configuration from a previous backup"
  echo
  echo "Options:"
  echo "  -l, --list     List available backups with details"
  echo "  -f, --force    Skip confirmation prompt"
  echo "  -v, --verbose  Show detailed output"
  echo "  -h, --help     Show this help message"
  echo
  echo "Arguments:"
  echo "  backup         Backup filename or timestamp pattern"
  echo "                 If not specified, restores the most recent backup"
  echo
  echo "Examples:"
  echo "  nself restore                          # Restore most recent backup"
  echo "  nself restore --list                   # List all available backups"
  echo "  nself restore demo-app_20250923_120000.tar.gz"
  echo "  nself restore 20250923_120000          # Match by timestamp"
  echo "  nself restore --force                  # Skip confirmation"
  echo
  echo "Notes:"
  echo "  • Backups are created automatically when running 'nself reset'"
  echo "  • Current files are backed up to _backup/before-restore_* before restoring"
  echo "  • Backup format: projectname_YYYYMMDD_HHMMSS.tar.gz"
  echo "  • Backups include: .env files, docker-compose.yml, services/, nginx/, etc."
}

# Export command
export -f cmd_restore

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  printf "\033[0;33m⚠\033[0m  WARNING: 'nself restore' is deprecated. Use 'nself db restore' instead.\n" >&2
  printf "   This compatibility wrapper will be removed in v1.0.0\n\n" >&2
  cmd_restore "$@"
fi
