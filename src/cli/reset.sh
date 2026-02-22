#!/usr/bin/env bash
set -euo pipefail

# reset.sh - Reset project to clean state

# Source utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$(dirname "$SCRIPT_DIR")/lib"

source "$LIB_DIR/utils/display.sh"
source "$LIB_DIR/utils/env.sh"
source "$LIB_DIR/utils/header.sh"
source "$LIB_DIR/hooks/pre-command.sh" 2>/dev/null || true
source "$LIB_DIR/hooks/post-command.sh" 2>/dev/null || true

# Command function
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
      --help | -h)
        show_reset_help
        return 0
        ;;
      *)
        log_error "Unknown option: $1"
        show_reset_help
        return 1
        ;;
    esac
  done

  # Show standardized header
  show_command_header "nself reset" "Reset project to clean state"

  printf "${COLOR_YELLOW}⚠${COLOR_RESET}  This will:\n"
  echo "  • Stop and remove all containers"
  echo "  • Delete all Docker volumes"
  echo "  • Remove all generated files"
  echo "  • Backup env files to _backup folder"
  echo

  if [[ "$force_reset" != "true" ]]; then
    read -p "Are you sure you want to reset everything? [y/N]: " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      log_info "Reset cancelled"
      return 1
    fi
  fi

  echo

  # Get project name first
  local project="${PROJECT_NAME:-myproject}"
  if [[ -f ".env" ]] || [[ -f ".env.dev" ]]; then
    set -a
    load_env_with_priority 2>/dev/null || true
    set +a
    project="${PROJECT_NAME:-myproject}"
  fi

  # Stopping services
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Stopping services..."

  # First try docker-compose down if file exists
  if [[ -f "docker-compose.yml" ]]; then
    docker compose down -v >/dev/null 2>&1 || true
  fi

  # Then forcefully stop and remove ALL project containers
  local container_count=$(docker ps -aq --filter "name=${project}" | wc -l | tr -d ' ')
  if [[ $container_count -gt 0 ]]; then
    # Stop all project containers
    local running=$(docker ps -q --filter "name=${project}")
    if [ -n "$running" ]; then
      echo "$running" | xargs docker stop >/dev/null 2>&1 || true
    fi
    # Remove all project containers
    local containers=$(docker ps -aq --filter "name=${project}")
    if [ -n "$containers" ]; then
      echo "$containers" | xargs docker rm -f >/dev/null 2>&1 || true
    fi
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Stopped and removed $container_count containers         \n"
  else
    printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} No containers to stop                           \n"
  fi

  # Remove all volumes for this project
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Removing Docker volumes..."
  local volume_count=$(docker volume ls -q | grep "^${project}_" | wc -l | tr -d ' ')
  if [[ $volume_count -gt 0 ]]; then
    local volumes=$(docker volume ls -q | grep "^${project}_")
    if [ -n "$volumes" ]; then
      echo "$volumes" | xargs docker volume rm -f >/dev/null 2>&1 || true
    fi
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed $volume_count Docker volumes                     \n"
  else
    printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} No volumes to remove                            \n"
  fi

  # Also remove the Docker network
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Removing Docker network..."
  if docker network ls | grep -q "${project}_network"; then
    docker network rm "${project}_network" >/dev/null 2>&1 || true
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed Docker network                          \n"
  else
    printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} No network to remove                            \n"
  fi

  # Create backup archive in _backup folder
  if [[ "$create_backup" == "true" ]]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Creating backup..."

    # Create _backup directory if it doesn't exist
    mkdir -p "_backup"

    # Use consistent timestamp format: projectname_YYYYMMDD_HHMMSS.tar.gz
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_name="_backup/${project}_${timestamp}.tar.gz"

    # Build list of files to backup
    local backup_files=""

    # Add all env files that exist
    for env_file in .env .env.*; do
      [[ -f "$env_file" ]] && backup_files="$backup_files $env_file"
    done

    # Add other important files and directories if they exist
    [[ -f "docker-compose.yml" ]] && backup_files="$backup_files docker-compose.yml"
    [[ -f "docker-compose.override.yml" ]] && backup_files="$backup_files docker-compose.override.yml"
    [[ -d "services" ]] && backup_files="$backup_files services/"
    [[ -d "nginx" ]] && backup_files="$backup_files nginx/"
    [[ -d "monitoring" ]] && backup_files="$backup_files monitoring/"
    [[ -d "postgres" ]] && backup_files="$backup_files postgres/"
    [[ -f "schema.dbml" ]] && backup_files="$backup_files schema.dbml"

    # Create the backup if we have files to backup
    if [[ -n "$backup_files" ]]; then
      tar czf "$backup_name" $backup_files 2>/dev/null || true

      if [[ -f "$backup_name" ]]; then
        printf "\r${COLOR_GREEN}✓${COLOR_RESET} Backup created: $backup_name              \n"
      else
        printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} Backup failed                               \n"
      fi
    else
      printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} No files to backup                        \n"
    fi
    echo
  fi

  # Remove ALL generated files and directories
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Removing generated files..."
  local items_to_remove=(
    # Docker files
    "docker-compose.yml"
    "docker-compose.yml.backup"
    "docker-compose.yml.healthcheck-backup"
    "docker-compose.override.yml"
    ".dockerignore"

    # Runtime and environment files
    ".env.runtime"
    ".env.example"
    ".env.prod-template"
    ".env.prod-secrets"
    ".gitignore"
    "frontend-apps.env"

    # Log files
    "*.log"
    "logs"

    # Cache directories
    ".nself"
    ".cache"

    # Any leftover backup files (removed after backing up)

    # Service directories
    "nginx"
    "nginx.backup"
    "postgres"
    "hasura"
    "functions"
    "functions-runtime"
    "monitoring"
    "prometheus"
    "services"
    "nestjs-run"
    "backend"
    "storage"
    "auth"
    "minio"
    "mlflow"
    "dashboard"
    "ssl"

    # Certificates and binaries
    "certs"
    "bin"

    # Data directories
    "data"
    "volumes"
    ".volumes"
    "postgres-data"
    "minio-data"
    "redis-data"

    # Database files
    "db"
    "init.sql"
    "schema.dbml"
    "seeds"
    "migrations"
    "metadata"
    "postgres/init"

    # Log files
    "logs"

    # Temporary files and scripts
    ".needs-rebuild"
    ".last-build-hash"
    ".port-overrides"
    "fix-healthchecks.sh"
    "fix-*.sh"

    # Other generated files
    "node_modules"
    "package-lock.json"
    "yarn.lock"
    "go.mod"
    "go.sum"
    "requirements.txt"
    "Pipfile"
    "Pipfile.lock"
  )

  local removed_count=0
  for item in "${items_to_remove[@]}"; do
    if [[ -e "$item" ]]; then
      rm -rf "$item"
      removed_count=$((removed_count + 1))
    fi
  done

  # Remove files matching patterns
  for pattern in *.log *.pid *.lock .DS_Store docker-compose.*.yml docker-compose.yml.*; do
    for file in $pattern; do
      if [[ -f "$file" ]]; then
        rm -f "$file"
        removed_count=$((removed_count + 1))
      fi
    done
  done

  # Remove any .backup and .old files
  for backup_file in .env.*.backup *.old .*.old; do
    if [[ -f "$backup_file" ]]; then
      rm -f "$backup_file"
      removed_count=$((removed_count + 1))
    fi
  done

  # Remove any project-prefixed directories (leftover from previous runs)
  for dir in ${project}-*; do
    if [[ -d "$dir" ]]; then
      rm -rf "$dir"
      removed_count=$((removed_count + 1))
    fi
  done

  printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed $removed_count files and directories           \n"

  # Update .gitignore to exclude backup folders
  if [[ -f ".gitignore" ]]; then
    # Check if _backup* is already in .gitignore
    if ! grep -q "^_backup" .gitignore; then
      printf "\n# Backup folders from nself reset\n_backup*\n" >>.gitignore
      printf "${COLOR_GREEN}✓${COLOR_RESET} Added _backup* to .gitignore\n"
    fi
  else
    # Don't create .gitignore - leave directory completely clean except _backup
    # User can run 'nself init' to get a fresh start with proper .gitignore
    :
  fi

  # Clean up Docker system (optional)
  printf "${COLOR_BLUE}⠋${COLOR_RESET} Cleaning Docker system..."
  if docker system prune -f >/dev/null 2>&1; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Docker system cleaned                           \n"
  else
    printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} Docker cleanup skipped                          \n"
  fi

  log_success "Project reset complete!"
  echo

  printf "${COLOR_CYAN}➞ Next Steps${COLOR_RESET}\n"
  echo

  printf "  ${COLOR_BLUE}nself init${COLOR_RESET}     ${COLOR_DIM}# Create new configuration${COLOR_RESET}\n"
  printf "  ${COLOR_BLUE}nself restore${COLOR_RESET}  ${COLOR_DIM}# ..or restore latest backup${COLOR_RESET}\n"
  echo
  printf "  ${COLOR_BLUE}nself build${COLOR_RESET}    ${COLOR_DIM}# Generate infrastructure${COLOR_RESET}\n"
  printf "  ${COLOR_BLUE}nself start${COLOR_RESET}    ${COLOR_DIM}# Start services${COLOR_RESET}\n"

  return 0
}

# Show help
show_reset_help() {
  echo "Usage: nself reset [options]"
  echo
  echo "Reset the project to a clean state"
  echo
  echo "Options:"
  echo "  -f, --force       Skip confirmation prompt"
  echo "  --no-backup       Don't create backup before reset"
  echo "  --keep-env        Preserve environment files"
  echo "  -h, --help        Show this help message"
  echo
  echo "Actions:"
  echo "  • Stop and remove all containers"
  echo "  • Delete all Docker volumes"
  echo "  • Remove all generated files"
  echo "  • Create backup archive (unless --no-backup)"
  echo "  • Clean .env.runtime, logs, and cache"
  echo
  echo "Examples:"
  echo "  nself reset              # Interactive reset with backup"
  echo "  nself reset --force      # Skip confirmation"
  echo "  nself reset --no-backup  # Reset without backup"
  echo "  nself reset --keep-env   # Reset but keep .env files"
  echo
  echo "Backup files are saved in: _backup/projectname_YYYYMMDD_HHMMSS.tar.gz"
}

# Export for use as library
export -f cmd_reset

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_reset_help
      exit 0
    fi
  done
  pre_command "reset" || exit $?
  cmd_reset "$@"
  exit_code=$?
  post_command "reset" $exit_code
  exit $exit_code
fi
