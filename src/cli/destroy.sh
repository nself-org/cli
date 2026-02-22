#!/usr/bin/env bash
set -euo pipefail

# destroy.sh - Safe destruction of project infrastructure
# Part of the v1.0 command tree - comprehensive cleanup with safety checks

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/docker.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Load environment with smart defaults
if [[ -f "$CLI_SCRIPT_DIR/../lib/config/smart-defaults.sh" ]]; then
  source "$CLI_SCRIPT_DIR/../lib/config/smart-defaults.sh"
  load_env_with_defaults >/dev/null 2>&1 || true
fi

# Command function
cmd_destroy() {
  local force=false
  local dry_run=false
  local keep_volumes=false
  local containers_only=false
  local volumes_only=false
  local networks_only=false
  local generated_only=false
  local verbose=false
  local confirmed=false

  # Parse options
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force | -f)
        force=true
        shift
        ;;
      --dry-run)
        dry_run=true
        shift
        ;;
      --keep-volumes)
        keep_volumes=true
        shift
        ;;
      --containers-only)
        containers_only=true
        shift
        ;;
      --volumes-only)
        volumes_only=true
        shift
        ;;
      --networks-only)
        networks_only=true
        shift
        ;;
      --generated-only)
        generated_only=true
        shift
        ;;
      --verbose)
        verbose=true
        shift
        ;;
      --yes | -y)
        confirmed=true
        shift
        ;;
      --help | -h)
        show_destroy_help
        return 0
        ;;
      -*)
        log_error "Unknown option: $1"
        show_destroy_help
        return 1
        ;;
      *)
        log_error "Unknown argument: $1"
        show_destroy_help
        return 1
        ;;
    esac
  done

  # Check for docker-compose.yml existence for context
  if [[ ! -f "docker-compose.yml" ]]; then
    log_warning "docker-compose.yml not found"
    log_info "Nothing to destroy"
    return 0
  fi

  # Load environment
  if [[ -f ".env" ]] || [[ -f ".env.dev" ]]; then
    set -a
    load_env_with_priority
    set +a
  fi

  # Get project name
  local project_name="${PROJECT_NAME:-nself}"

  # Show header
  show_command_header "nself destroy" "Safe destruction of project infrastructure"

  # Determine what will be destroyed
  local destroy_containers=true
  local destroy_volumes=true
  local destroy_networks=true
  local destroy_generated=true

  # Handle selective destruction flags
  if [[ "$containers_only" == true ]]; then
    destroy_volumes=false
    destroy_networks=false
    destroy_generated=false
  elif [[ "$volumes_only" == true ]]; then
    destroy_containers=false
    destroy_networks=false
    destroy_generated=false
  elif [[ "$networks_only" == true ]]; then
    destroy_containers=false
    destroy_volumes=false
    destroy_generated=false
  elif [[ "$generated_only" == true ]]; then
    destroy_containers=false
    destroy_volumes=false
    destroy_networks=false
  fi

  # Override volumes if keep flag is set
  if [[ "$keep_volumes" == true ]]; then
    destroy_volumes=false
  fi

  # Get current state
  local running_containers=$(docker ps --filter "name=^${project_name}_" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
  local stopped_containers=$(docker ps -a --filter "name=^${project_name}_" --filter "status=exited" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')
  local all_containers=$((running_containers + stopped_containers))
  local project_volumes=$(docker volume ls --filter "name=${project_name}" -q 2>/dev/null | wc -l | tr -d ' ')
  local project_networks=$(docker network ls --filter "name=${project_name}" -q 2>/dev/null | wc -l | tr -d ' ')

  # Show what will be destroyed
  printf "\n${COLOR_CYAN}Destruction Plan:${COLOR_RESET}\n\n"

  if [[ "$destroy_containers" == true ]] && [[ $all_containers -gt 0 ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} ${COLOR_BOLD}%d containers${COLOR_RESET} (%d running, %d stopped)\n" \
      "$all_containers" "$running_containers" "$stopped_containers"
  fi

  if [[ "$destroy_volumes" == true ]] && [[ $project_volumes -gt 0 ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} ${COLOR_BOLD}%d volumes${COLOR_RESET} ${COLOR_YELLOW}(ALL DATA WILL BE LOST)${COLOR_RESET}\n" \
      "$project_volumes"
  fi

  if [[ "$destroy_networks" == true ]] && [[ $project_networks -gt 0 ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} ${COLOR_BOLD}%d networks${COLOR_RESET}\n" "$project_networks"
  fi

  if [[ "$destroy_generated" == true ]]; then
    printf "  ${COLOR_RED}✗${COLOR_RESET} ${COLOR_BOLD}Generated files:${COLOR_RESET}\n"
    printf "      - docker-compose.yml\n"
    printf "      - nginx/ directory\n"
    printf "      - services/ directory (custom services)\n"
    printf "      - monitoring/ directory (if exists)\n"
    printf "      - ssl/ directory (if exists)\n"
  fi

  # Preserved items
  printf "\n${COLOR_GREEN}Preserved:${COLOR_RESET}\n"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} .env files (configuration)\n"
  printf "  ${COLOR_GREEN}✓${COLOR_RESET} Source code and custom files\n"

  if [[ "$keep_volumes" == true ]] || [[ "$destroy_volumes" == false ]]; then
    if [[ $project_volumes -gt 0 ]]; then
      printf "  ${COLOR_GREEN}✓${COLOR_RESET} Docker volumes (data preserved)\n"
    fi
  fi

  printf "\n"

  # Dry run mode - just show what would happen
  if [[ "$dry_run" == true ]]; then
    printf "${COLOR_BLUE}→${COLOR_RESET} ${COLOR_BOLD}DRY RUN MODE${COLOR_RESET} - Nothing will be destroyed\n"
    printf "${COLOR_DIM}Run without --dry-run to execute destruction${COLOR_RESET}\n\n"
    return 0
  fi

  # Safety confirmation (unless --force or --yes)
  if [[ "$force" != true ]] && [[ "$confirmed" != true ]]; then
    # Critical data warning if volumes will be destroyed
    if [[ "$destroy_volumes" == true ]] && [[ $project_volumes -gt 0 ]]; then
      printf "${COLOR_RED}${COLOR_BOLD}WARNING: This will permanently delete all data!${COLOR_RESET}\n"
      printf "${COLOR_RED}This includes:${COLOR_RESET}\n"
      printf "  - PostgreSQL databases\n"
      printf "  - Redis cache data\n"
      printf "  - MinIO stored files\n"
      printf "  - All other persistent data\n\n"
    fi

    # Get confirmation
    printf "Are you sure you want to destroy project '%s'? (yes/no): " "$project_name"
    read -r response
    response=$(echo "$response" | tr '[:upper:]' '[:lower:]')

    if [[ "$response" != "yes" ]] && [[ "$response" != "y" ]]; then
      printf "\n${COLOR_YELLOW}✱${COLOR_RESET} Destruction cancelled\n\n"
      return 0
    fi

    # Double confirmation for volume destruction
    if [[ "$destroy_volumes" == true ]] && [[ $project_volumes -gt 0 ]]; then
      printf "\n${COLOR_RED}${COLOR_BOLD}FINAL WARNING:${COLOR_RESET} Type the project name '${COLOR_BOLD}%s${COLOR_RESET}' to confirm: " "$project_name"
      read -r confirm_name

      if [[ "$confirm_name" != "$project_name" ]]; then
        printf "\n${COLOR_YELLOW}✱${COLOR_RESET} Destruction cancelled (name mismatch)\n\n"
        return 0
      fi
    fi
  fi

  printf "\n${COLOR_CYAN}→${COLOR_RESET} Beginning destruction sequence...\n\n"

  # Track success/failure
  local errors=0

  # Step 1: Stop and remove containers
  if [[ "$destroy_containers" == true ]] && [[ $all_containers -gt 0 ]]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Stopping and removing containers..."

    if docker compose down --remove-orphans >/dev/null 2>&1; then
      printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed %d containers                                  \n" "$all_containers"
    else
      printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to remove some containers                       \n"
      errors=$((errors + 1))

      # Try force removal
      if [[ "$verbose" == true ]]; then
        printf "  ${COLOR_DIM}Attempting force removal...${COLOR_RESET}\n"
      fi

      local all_container_ids=$(docker ps -aq --filter "name=${project_name}_" 2>/dev/null)
      if [[ -n "$all_container_ids" ]]; then
        echo "$all_container_ids" | xargs docker rm -f >/dev/null 2>&1 || true
      fi
    fi
  fi

  # Step 2: Remove volumes
  if [[ "$destroy_volumes" == true ]] && [[ $project_volumes -gt 0 ]]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Removing volumes (data destruction)..."

    local volume_ids=$(docker volume ls --filter "name=${project_name}" -q 2>/dev/null)
    if [[ -n "$volume_ids" ]]; then
      if echo "$volume_ids" | xargs docker volume rm >/dev/null 2>&1; then
        printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed %d volumes                                     \n" "$project_volumes"
      else
        printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to remove some volumes                          \n"
        errors=$((errors + 1))

        # Try force removal
        if [[ "$verbose" == true ]]; then
          printf "  ${COLOR_DIM}Some volumes may be in use${COLOR_RESET}\n"
        fi
      fi
    fi
  fi

  # Step 3: Remove networks
  if [[ "$destroy_networks" == true ]] && [[ $project_networks -gt 0 ]]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Removing networks..."

    local network_ids=$(docker network ls --filter "name=${project_name}" -q 2>/dev/null)
    if [[ -n "$network_ids" ]]; then
      if echo "$network_ids" | xargs docker network rm >/dev/null 2>&1; then
        printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed %d networks                                    \n" "$project_networks"
      else
        printf "\r${COLOR_RED}✗${COLOR_RESET} Failed to remove some networks                         \n"
        errors=$((errors + 1))
      fi
    fi
  fi

  # Step 4: Remove generated files
  if [[ "$destroy_generated" == true ]]; then
    printf "${COLOR_BLUE}⠋${COLOR_RESET} Removing generated files..."

    local files_removed=0

    # Remove docker-compose.yml
    if [[ -f "docker-compose.yml" ]]; then
      rm -f docker-compose.yml
      files_removed=$((files_removed + 1))
    fi

    # Remove nginx directory
    if [[ -d "nginx" ]]; then
      rm -rf nginx/
      files_removed=$((files_removed + 1))
    fi

    # Remove services directory (custom services)
    if [[ -d "services" ]]; then
      rm -rf services/
      files_removed=$((files_removed + 1))
    fi

    # Remove monitoring directory
    if [[ -d "monitoring" ]]; then
      rm -rf monitoring/
      files_removed=$((files_removed + 1))
    fi

    # Remove SSL directory
    if [[ -d "ssl" ]]; then
      rm -rf ssl/
      files_removed=$((files_removed + 1))
    fi

    # Remove postgres init directory
    if [[ -d "postgres" ]]; then
      rm -rf postgres/
      files_removed=$((files_removed + 1))
    fi

    # Remove runtime env
    if [[ -f ".env.runtime" ]]; then
      rm -f .env.runtime
      files_removed=$((files_removed + 1))
    fi

    printf "\r${COLOR_GREEN}✓${COLOR_RESET} Removed %d generated files/directories                 \n" "$files_removed"
  fi

  # Final summary
  printf "\n"
  if [[ $errors -eq 0 ]]; then
    log_success "Destruction complete!"
    printf "${COLOR_GREEN}✓${COLOR_RESET} Project '%s' has been destroyed\n" "$project_name"

    if [[ "$destroy_volumes" != true ]] && [[ $project_volumes -gt 0 ]]; then
      printf "\n${COLOR_YELLOW}⚠${COLOR_RESET}  Data volumes preserved - to remove them later, run:\n"
      printf "   ${COLOR_BLUE}docker volume ls --filter 'name=%s' -q | xargs docker volume rm${COLOR_RESET}\n" "$project_name"
    fi

    printf "\n${COLOR_BOLD}Next steps:${COLOR_RESET}\n\n"
    printf "  ${COLOR_BLUE}nself init${COLOR_RESET}   - Initialize a new project\n"
    printf "  ${COLOR_BLUE}nself build${COLOR_RESET}  - Rebuild from existing .env\n"
    printf "\n"
  else
    log_error "Destruction completed with %d errors" "$errors"
    printf "\n${COLOR_YELLOW}Some resources may need manual cleanup${COLOR_RESET}\n"
    printf "\nManual cleanup commands:\n"
    printf "  ${COLOR_DIM}docker ps -a | grep %s${COLOR_RESET}    # Check containers\n" "$project_name"
    printf "  ${COLOR_DIM}docker volume ls | grep %s${COLOR_RESET}  # Check volumes\n" "$project_name"
    printf "  ${COLOR_DIM}docker network ls | grep %s${COLOR_RESET} # Check networks\n" "$project_name"
    printf "\n"
    return 1
  fi

  return 0
}

# Show help
show_destroy_help() {
  echo "Usage: nself destroy [OPTIONS]"
  echo ""
  echo "Safe destruction of project infrastructure with configurable scope"
  echo ""
  echo "Options:"
  echo "  -f, --force              Skip all confirmation prompts"
  echo "  -y, --yes                Auto-confirm (same as --force)"
  echo "  --dry-run                Show what would be destroyed without executing"
  echo "  --keep-volumes           Preserve data volumes (keep databases, files, etc.)"
  echo "  --containers-only        Only remove Docker containers"
  echo "  --volumes-only           Only remove Docker volumes (requires confirmation)"
  echo "  --networks-only          Only remove Docker networks"
  echo "  --generated-only         Only remove generated files (docker-compose.yml, etc.)"
  echo "  --verbose                Show detailed output"
  echo "  -h, --help               Show this help message"
  echo ""
  echo "Examples:"
  echo "  nself destroy                        # Interactive destruction (with confirmations)"
  echo "  nself destroy --dry-run              # Preview what will be destroyed"
  echo "  nself destroy --keep-volumes         # Destroy but preserve data"
  echo "  nself destroy --force                # Non-interactive destruction (DANGEROUS)"
  echo "  nself destroy --containers-only      # Only remove containers"
  echo "  nself destroy --generated-only       # Only remove generated files"
  echo ""
  echo "Destruction Scope:"
  echo "  By default, destroys:"
  echo "    ✗ All Docker containers (running and stopped)"
  echo "    ✗ All Docker volumes (databases, files, cache) - DATA LOSS!"
  echo "    ✗ All Docker networks"
  echo "    ✗ Generated files (docker-compose.yml, nginx/, services/, etc.)"
  echo ""
  echo "  Always preserved:"
  echo "    ✓ .env files (configuration)"
  echo "    ✓ Source code and custom files"
  echo ""
  echo "Safety Features:"
  echo "  - Interactive confirmation required by default"
  echo "  - Double confirmation for data volume destruction"
  echo "  - Dry-run mode to preview destruction"
  echo "  - Selective destruction with --*-only flags"
  echo "  - Color-coded warnings for dangerous operations"
  echo ""
  echo "Related Commands:"
  echo "  nself stop --volumes    # Stop services and remove volumes (lighter than destroy)"
  echo "  nself clean            # Clean Docker resources (images, cache)"
  echo "  nself backup clean     # Clean old backups"
  echo ""
  echo "⚠️  WARNING: Volume destruction is permanent and cannot be undone!"
  echo "    Always backup important data before destroying volumes"
}

# Export for use as library
export -f cmd_destroy

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_destroy_help
      exit 0
    fi
  done
  printf "\033[0;33m⚠\033[0m  WARNING: 'nself destroy' is deprecated. Use 'nself infra destroy' instead.\n" >&2
  printf "   This compatibility wrapper will be removed in v1.0.0\n\n" >&2
  pre_command "destroy" || exit $?
  cmd_destroy "$@"
  exit_code=$?
  post_command "destroy" $exit_code
  exit $exit_code
fi
