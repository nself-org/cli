#!/usr/bin/env bash
# update.sh - Update nself CLI and nself-admin to the latest versions

# Don't use set -e - we handle all errors explicitly
set -uo pipefail

set -euo pipefail


# Get script directory with absolute path
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_SCRIPT_DIR="$SCRIPT_DIR"

# Source utilities
[[ -z "${DISPLAY_SOURCED:-}" ]] && source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$SCRIPT_DIR/../lib/hooks/post-command.sh"

# Constants
GITHUB_REPO_OWNER="nself-org"
GITHUB_REPO_NAME="nself"
DOCKER_IMAGE="nself/nself-admin"

# Show help for update command
show_update_help() {
  printf "nself update - Update nself CLI and nself-admin\n"
  printf "\n"
  printf "Usage: nself update [OPTIONS]\n"
  printf "\n"
  printf "Description:\n"
  printf "  Updates nself CLI and nself-admin to their latest versions.\n"
  printf "  When run from a project directory, automatically:\n"
  printf "    - Regenerates docker-compose.yml with new CLI\n"
  printf "    - Restarts running services with new configuration\n"
  printf "    - Updates nself-admin container to latest version\n"
  printf "\n"
  printf "Options:\n"
  printf "  --check             Check for updates without installing\n"
  printf "  --cli               Only update the CLI\n"
  printf "  --admin             Only update nself-admin\n"
  printf "  --force             Force update even if already up to date\n"
  printf "  --restart           Force restart even if no updates (useful for troubleshooting)\n"
  printf "  -h, --help          Show this help message\n"
  printf "\n"
  printf "Examples:\n"
  printf "  nself update                   # Update everything automatically\n"
  printf "  nself update --check           # Check for updates only\n"
  printf "  nself update --cli             # Only update CLI\n"
  printf "  nself update --admin           # Only update nself-admin\n"
  printf "  nself update --force           # Force re-download even if current\n"
  printf "\n"
  printf "What Happens:\n"
  printf "  1. Checks for CLI updates on GitHub\n"
  printf "  2. Checks for nself-admin updates on Docker Hub\n"
  printf "  3. Downloads and installs updates if available\n"
  printf "  4. If in a project directory:\n"
  printf "     - Regenerates docker-compose.yml (if CLI updated)\n"
  printf "     - Restarts running services with new config\n"
  printf "     - Updates nself-admin container\n"
  printf "\n"
  printf "Notes:\n"
  printf "  - Run from your project directory for full automation\n"
  printf "  - Running services are gracefully restarted\n"
  printf "  - Your data and configuration are preserved\n"
  printf "  - Requires internet connection and Docker\n"
}

# Compare semantic versions
# Returns: 0 if v1 > v2, 1 if v1 = v2, 2 if v1 < v2
compare_versions() {
  local v1="$1"
  local v2="$2"

  # Remove 'v' prefix if present
  v1="${v1#v}"
  v2="${v2#v}"

  if [[ "$v1" == "$v2" ]]; then
    return 1
  fi

  # Split versions into arrays using IFS
  local IFS='.'
  local v1_parts=($v1)
  local v2_parts=($v2)

  # Compare each part
  local i
  for ((i = 0; i < 3; i++)); do
    local n1="${v1_parts[$i]:-0}"
    local n2="${v2_parts[$i]:-0}"

    if ((n1 > n2)); then
      return 0
    elif ((n1 < n2)); then
      return 2
    fi
  done

  return 1
}

# ============================================================================
# CLI UPDATE FUNCTIONS
# ============================================================================

# Check for CLI updates
check_cli_updates() {
  local version_file="$SCRIPT_DIR/../VERSION"
  local github_api="https://api.github.com/repos/$GITHUB_REPO_OWNER/$GITHUB_REPO_NAME/releases/latest"

  # Get current version
  if [[ -f "$version_file" ]]; then
    CLI_CURRENT_VERSION=$(cat "$version_file")
  else
    CLI_CURRENT_VERSION="0.0.0"
  fi

  # Get latest version from GitHub
  local latest_json
  if ! latest_json=$(curl -sL --max-time 30 --connect-timeout 10 --retry 2 "$github_api" 2>/dev/null); then
    log_error "Failed to check CLI updates - network error"
    return 1
  fi

  # Parse version from JSON response
  CLI_LATEST_VERSION=$(printf '%s' "$latest_json" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)".*/\1/')

  # Remove 'v' prefix if present
  CLI_LATEST_VERSION="${CLI_LATEST_VERSION#v}"

  # Validate version format
  if [[ -z "$CLI_LATEST_VERSION" ]] || [[ ! "$CLI_LATEST_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log_error "Could not determine valid CLI version from GitHub"
    return 1
  fi

  # Compare versions
  compare_versions "$CLI_LATEST_VERSION" "$CLI_CURRENT_VERSION"
  local cmp_result=$?

  if [[ $cmp_result -eq 0 ]]; then
    CLI_UPDATE_AVAILABLE=true
  else
    CLI_UPDATE_AVAILABLE=false
  fi

  return 0
}

# Perform CLI update
perform_cli_update() {
  local version_file="$SCRIPT_DIR/../VERSION"
  local github_api="https://api.github.com/repos/$GITHUB_REPO_OWNER/$GITHUB_REPO_NAME/releases/latest"

  # Check required commands
  local missing_deps=()
  for cmd in curl tar rsync; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing_deps+=("$cmd")
    fi
  done

  if [[ ${#missing_deps[@]} -gt 0 ]]; then
    log_error "Missing required commands: ${missing_deps[*]}"
    return 1
  fi

  # Get latest release info
  local latest_json
  if ! latest_json=$(curl -sL --max-time 30 --connect-timeout 10 --retry 2 "$github_api" 2>/dev/null); then
    log_error "Failed to fetch CLI release information"
    return 1
  fi

  # Get download URL
  local asset_url
  asset_url=$(printf '%s' "$latest_json" | grep -o '"tarball_url"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)".*/\1/')

  if [[ -z "$asset_url" ]] || [[ ! "$asset_url" =~ ^https:// ]]; then
    log_error "No valid download URL found for CLI"
    return 1
  fi

  # Create temporary directory
  local tmp_dir
  tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t nself_update.XXXXXX)
  local archive_file="$tmp_dir/nself_latest.tar.gz"
  local extract_dir="$tmp_dir/extracted"

  # Download
  log_info "Downloading nself CLI $CLI_LATEST_VERSION..."
  if ! curl -L --max-time 300 --connect-timeout 10 --retry 3 --progress-bar "$asset_url" -o "$archive_file" 2>&1; then
    log_error "Failed to download CLI update"
    rm -rf "$tmp_dir"
    return 1
  fi

  # Verify it's a valid gzip file
  if ! gzip -t "$archive_file" 2>/dev/null; then
    log_error "Downloaded CLI file is corrupted"
    rm -rf "$tmp_dir"
    return 1
  fi

  # Extract
  log_info "Extracting CLI update..."
  mkdir -p "$extract_dir"
  if ! tar -xzf "$archive_file" -C "$extract_dir" 2>/dev/null; then
    log_error "Failed to extract CLI update"
    rm -rf "$tmp_dir"
    return 1
  fi

  # Find extracted directory
  local extracted_dir
  extracted_dir=$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | head -1)

  if [[ ! -d "$extracted_dir" ]] || [[ ! -d "$extracted_dir/src" ]]; then
    log_error "Invalid CLI archive structure"
    rm -rf "$tmp_dir"
    return 1
  fi

  # Update installation
  log_info "Installing CLI update..."
  local install_dir
  install_dir="$(dirname "$(dirname "$SCRIPT_DIR")")"

  # Update directories
  for dir in bin src docs; do
    if [[ -d "$extracted_dir/$dir" ]]; then
      if ! rsync -a --delete "$extracted_dir/$dir/" "$install_dir/$dir/"; then
        log_error "Failed to update CLI $dir/"
        rm -rf "$tmp_dir"
        return 1
      fi
    fi
  done

  # Update root files
  for file in LICENSE README.md; do
    if [[ -f "$extracted_dir/$file" ]]; then
      cp "$extracted_dir/$file" "$install_dir/" 2>/dev/null || true
    fi
  done

  # Update version file
  printf '%s' "$CLI_LATEST_VERSION" >"$version_file"

  # Clean up
  rm -rf "$tmp_dir"

  log_success "CLI updated to $CLI_LATEST_VERSION"
  return 0
}

# ============================================================================
# NSELF-ADMIN UPDATE FUNCTIONS
# ============================================================================

# Get current nself-admin version
get_current_admin_version() {
  # Try to get version from running container first
  local container_version
  container_version=$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' "$DOCKER_IMAGE" 2>/dev/null || true)

  if [[ -n "$container_version" ]] && [[ "$container_version" != "<no value>" ]]; then
    printf '%s' "$container_version"
    return 0
  fi

  # Try to get from local image
  local image_version
  image_version=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' "$DOCKER_IMAGE:latest" 2>/dev/null || true)

  if [[ -n "$image_version" ]] && [[ "$image_version" != "<no value>" ]]; then
    printf '%s' "$image_version"
    return 0
  fi

  # No local image
  printf "not installed"
  return 0
}

# Get latest nself-admin version from Docker Hub
get_latest_admin_version() {
  # Query Docker Hub API for tags
  local hub_response
  hub_response=$(curl -sL --max-time 30 "https://hub.docker.com/v2/repositories/$DOCKER_IMAGE/tags?page_size=20" 2>/dev/null)

  if [[ -z "$hub_response" ]]; then
    printf ""
    return 1
  fi

  # Extract version tags (format: v0.0.X or 0.0.X) and find the latest
  local latest_tag
  latest_tag=$(printf '%s' "$hub_response" | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/' | grep -E '^v?[0-9]+\.[0-9]+\.[0-9]+$' | sed 's/^v//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)

  if [[ -n "$latest_tag" ]]; then
    printf '%s' "$latest_tag"
    return 0
  fi

  printf ""
  return 1
}

# Check for nself-admin updates
check_admin_updates() {
  # Check if Docker is available
  if ! command -v docker >/dev/null 2>&1; then
    log_error "Docker is not installed - cannot check admin updates"
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    log_error "Docker is not running - cannot check admin updates"
    return 1
  fi

  # Get current version
  ADMIN_CURRENT_VERSION=$(get_current_admin_version)

  # Get latest version from Docker Hub
  ADMIN_LATEST_VERSION=$(get_latest_admin_version)

  if [[ -z "$ADMIN_LATEST_VERSION" ]]; then
    log_error "Could not determine latest admin version from Docker Hub"
    return 1
  fi

  # Compare versions
  if [[ "$ADMIN_CURRENT_VERSION" == "not installed" ]]; then
    ADMIN_UPDATE_AVAILABLE=true
  else
    compare_versions "$ADMIN_LATEST_VERSION" "$ADMIN_CURRENT_VERSION"
    local cmp_result=$?

    if [[ $cmp_result -eq 0 ]]; then
      ADMIN_UPDATE_AVAILABLE=true
    else
      ADMIN_UPDATE_AVAILABLE=false
    fi
  fi

  return 0
}

# Perform nself-admin update
perform_admin_update() {
  local force="${1:-false}"

  log_info "Pulling nself-admin:latest..."

  # Pull the latest image
  if ! docker pull "$DOCKER_IMAGE:latest" 2>&1; then
    log_error "Failed to pull nself-admin image"
    return 1
  fi

  # Also pull the versioned tag if available
  if [[ -n "${ADMIN_LATEST_VERSION:-}" ]]; then
    docker pull "$DOCKER_IMAGE:v$ADMIN_LATEST_VERSION" 2>/dev/null || true
  fi

  log_success "nself-admin updated to ${ADMIN_LATEST_VERSION:-latest}"
  return 0
}

# Check if services are running in the current project
services_are_running() {
  if [[ ! -f "docker-compose.yml" ]]; then
    return 1
  fi

  local project_name="${PROJECT_NAME:-$(basename "$PWD")}"
  local running_count
  running_count=$(docker ps --filter "label=com.docker.compose.project=$project_name" --format "{{.Names}}" 2>/dev/null | wc -l | tr -d ' ')

  if [[ "$running_count" -gt 0 ]]; then
    return 0
  fi

  return 1
}

# Restart nself-admin container
restart_admin_container() {
  log_info "Restarting nself-admin..."

  # Find any running admin container
  local admin_container
  admin_container=$(docker ps --filter "ancestor=$DOCKER_IMAGE" --format "{{.Names}}" 2>/dev/null | head -1)

  if [[ -z "$admin_container" ]]; then
    # Try finding by name pattern
    admin_container=$(docker ps --filter "name=admin" --format "{{.Names}}" 2>/dev/null | grep -E 'admin|nself-admin' | head -1)
  fi

  if [[ -n "$admin_container" ]]; then
    # Get the container's compose project
    local compose_project
    compose_project=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$admin_container" 2>/dev/null || true)

    if [[ -n "$compose_project" ]] && [[ "$compose_project" != "<no value>" ]]; then
      # Use docker compose to restart (preserves networking and volumes)
      local compose_dir
      compose_dir=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project.working_dir"}}' "$admin_container" 2>/dev/null || true)

      if [[ -n "$compose_dir" ]] && [[ -d "$compose_dir" ]]; then
        (cd "$compose_dir" && docker compose up -d --force-recreate nself-admin 2>/dev/null) ||
          (cd "$compose_dir" && docker-compose up -d --force-recreate nself-admin 2>/dev/null) ||
          docker restart "$admin_container"
      else
        docker restart "$admin_container"
      fi
    else
      docker restart "$admin_container"
    fi

    log_success "nself-admin restarted"
  else
    log_info "No running nself-admin container found to restart"
  fi

  return 0
}

# Gracefully restart all services with new configuration
restart_all_services() {
  log_info "Restarting services with updated configuration..."

  # Use docker compose to recreate services with new config
  if docker compose up -d --force-recreate 2>/dev/null; then
    log_success "Services restarted with new configuration"
    return 0
  elif docker-compose up -d --force-recreate 2>/dev/null; then
    log_success "Services restarted with new configuration"
    return 0
  else
    log_warning "Could not restart services automatically"
    log_info "Run 'nself start' to apply changes"
    return 1
  fi
}

# ============================================================================
# MAIN COMMAND
# ============================================================================

# Main command function
cmd_update() {
  local check_only=false
  local cli_only=false
  local admin_only=false
  local force_update=false
  local do_restart=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case $1 in
      --check)
        check_only=true
        shift
        ;;
      --cli)
        cli_only=true
        shift
        ;;
      --admin)
        admin_only=true
        shift
        ;;
      --force)
        force_update=true
        shift
        ;;
      --restart)
        do_restart=true
        shift
        ;;
      -h | --help)
        show_update_help
        return 0
        ;;
      *)
        log_error "Unknown option: $1"
        log_info "Use 'nself update --help' for usage information"
        return 1
        ;;
    esac
  done

  # If both --cli and --admin are specified, that's the same as neither
  if [[ "$cli_only" == "true" ]] && [[ "$admin_only" == "true" ]]; then
    cli_only=false
    admin_only=false
  fi

  # Show header
  show_command_header "nself update" "Check for updates"

  local cli_checked=false
  local admin_checked=false
  local any_updates=false
  local any_errors=false

  # Initialize version variables
  CLI_CURRENT_VERSION=""
  CLI_LATEST_VERSION=""
  CLI_UPDATE_AVAILABLE=false
  ADMIN_CURRENT_VERSION=""
  ADMIN_LATEST_VERSION=""
  ADMIN_UPDATE_AVAILABLE=false

  # Check CLI updates (unless --admin only)
  if [[ "$admin_only" != "true" ]]; then
    printf "\n"
    printf "%b%b nself CLI%b\n" "${COLOR_BOLD}" "${COLOR_CYAN}" "${COLOR_RESET}"

    if check_cli_updates; then
      cli_checked=true
      printf "  Current: %s\n" "$CLI_CURRENT_VERSION"
      printf "  Latest:  %s\n" "$CLI_LATEST_VERSION"

      if [[ "$CLI_UPDATE_AVAILABLE" == "true" ]] || [[ "$force_update" == "true" ]]; then
        any_updates=true
        if [[ "$force_update" == "true" ]] && [[ "$CLI_UPDATE_AVAILABLE" != "true" ]]; then
          printf "  %b→ Force update requested%b\n" "${COLOR_YELLOW}" "${COLOR_RESET}"
        else
          printf "  %b→ Update available%b\n" "${COLOR_GREEN}" "${COLOR_RESET}"
        fi
      else
        printf "  %b✓ Up to date%b\n" "${COLOR_GREEN}" "${COLOR_RESET}"
      fi
    else
      any_errors=true
      printf "  %b✗ Failed to check%b\n" "${COLOR_RED}" "${COLOR_RESET}"
    fi
  fi

  # Check admin updates (unless --cli only)
  if [[ "$cli_only" != "true" ]]; then
    printf "\n"
    printf "%b%b nself-admin%b\n" "${COLOR_BOLD}" "${COLOR_CYAN}" "${COLOR_RESET}"

    if check_admin_updates; then
      admin_checked=true
      printf "  Current: %s\n" "$ADMIN_CURRENT_VERSION"
      printf "  Latest:  %s\n" "$ADMIN_LATEST_VERSION"

      if [[ "$ADMIN_UPDATE_AVAILABLE" == "true" ]] || [[ "$force_update" == "true" ]]; then
        any_updates=true
        if [[ "$force_update" == "true" ]] && [[ "$ADMIN_UPDATE_AVAILABLE" != "true" ]]; then
          printf "  %b→ Force update requested%b\n" "${COLOR_YELLOW}" "${COLOR_RESET}"
        else
          printf "  %b→ Update available%b\n" "${COLOR_GREEN}" "${COLOR_RESET}"
        fi
      else
        printf "  %b✓ Up to date%b\n" "${COLOR_GREEN}" "${COLOR_RESET}"
      fi
    else
      any_errors=true
      printf "  %b✗ Failed to check%b\n" "${COLOR_RED}" "${COLOR_RESET}"
    fi
  fi

  printf "\n"

  # If check only, we're done
  if [[ "$check_only" == "true" ]]; then
    if [[ "$any_updates" == "true" ]]; then
      log_info "Run 'nself update' to install updates"
    fi
    return 0
  fi

  # No updates available and not forcing
  if [[ "$any_updates" != "true" ]] && [[ "$force_update" != "true" ]]; then
    log_success "Everything is up to date!"
    return 0
  fi

  # Perform updates
  local cli_updated=false
  local admin_updated=false

  # Update CLI
  if [[ "$admin_only" != "true" ]]; then
    if [[ "$CLI_UPDATE_AVAILABLE" == "true" ]] || [[ "$force_update" == "true" ]]; then
      printf "\n"
      log_info "Updating CLI..."
      if perform_cli_update; then
        cli_updated=true
      else
        any_errors=true
      fi
    fi
  fi

  # Update admin
  if [[ "$cli_only" != "true" ]]; then
    if [[ "$ADMIN_UPDATE_AVAILABLE" == "true" ]] || [[ "$force_update" == "true" ]]; then
      printf "\n"
      log_info "Updating nself-admin..."
      if perform_admin_update "$force_update"; then
        admin_updated=true
      else
        any_errors=true
      fi
    fi
  fi

  # Restart admin if requested and updated
  if [[ "$do_restart" == "true" ]] && [[ "$admin_updated" == "true" ]]; then
    printf "\n"
    restart_admin_container
  elif [[ "$admin_updated" == "true" ]] && [[ "$do_restart" != "true" ]]; then
    printf "\n"
    log_info "Run 'nself restart admin' or use '--restart' flag to apply admin update"
  fi

  # Check if we're in a project directory with docker-compose.yml
  local in_project=false
  local needs_rebuild=false
  local was_running=false

  if [[ -f "docker-compose.yml" ]]; then
    in_project=true
    # Check if services are currently running
    if services_are_running; then
      was_running=true
    fi
  fi

  # If CLI was updated and we're in a project, always rebuild to get new config
  if [[ "$cli_updated" == "true" ]] && [[ "$in_project" == "true" ]]; then
    needs_rebuild=true
  fi

  # Auto-rebuild if needed
  if [[ "$needs_rebuild" == "true" ]]; then
    printf "\n"
    log_info "Regenerating project configuration with updated CLI..."

    # Run build to regenerate docker-compose.yml with new CLI
    if [[ -f "$SCRIPT_DIR/build.sh" ]]; then
      if bash "$SCRIPT_DIR/build.sh" --force 2>&1; then
        log_success "Project configuration updated"
      else
        log_warning "Build completed with warnings - check 'nself doctor' if issues persist"
      fi
    fi
  fi

  # If services were running, restart them with new configuration
  if [[ "$was_running" == "true" ]] && [[ "$needs_rebuild" == "true" || "$admin_updated" == "true" ]]; then
    printf "\n"
    restart_all_services
  elif [[ "$admin_updated" == "true" ]] && [[ "$in_project" == "true" ]]; then
    # Just admin was updated, restart only admin
    printf "\n"
    restart_admin_container
  fi

  # Summary
  printf "\n"
  if [[ "$cli_updated" == "true" ]] || [[ "$admin_updated" == "true" ]]; then
    printf "%b%b Update Complete! %b\n" "${COLOR_BOLD}" "${COLOR_GREEN}" "${COLOR_RESET}"
    printf "\n"
    if [[ "$cli_updated" == "true" ]]; then
      printf "  CLI:   %s → %s\n" "$CLI_CURRENT_VERSION" "$CLI_LATEST_VERSION"
    fi
    if [[ "$admin_updated" == "true" ]]; then
      printf "  Admin: %s → %s\n" "$ADMIN_CURRENT_VERSION" "$ADMIN_LATEST_VERSION"
    fi
    if [[ "$needs_rebuild" == "true" ]]; then
      printf "\n"
      printf "  ✓ Project configuration regenerated\n"
    fi
    if [[ "$was_running" == "true" ]]; then
      printf "  ✓ Services restarted with new configuration\n"
    fi
  fi

  # If not in project, remind user
  if [[ "$in_project" != "true" ]] && [[ "$cli_updated" == "true" || "$admin_updated" == "true" ]]; then
    printf "\n"
    log_info "To update your projects, run 'nself update' from within each project directory"
    log_info "Or run 'nself build --force' then 'nself start' in each project"
  fi

  if [[ "$any_errors" == "true" ]]; then
    return 1
  fi

  return 0
}

# Export for use as library
export -f cmd_update

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_update_help
      exit 0
    fi
  done
  pre_command "update" || exit $?
  cmd_update "$@"
  exit_code=$?
  post_command "update" $exit_code
  exit $exit_code
fi
