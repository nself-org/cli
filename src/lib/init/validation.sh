#!/usr/bin/env bash

# validation.sh - Validation functions for nself init command
#
# This module provides validation functions for checking dependencies,
# project state, and security settings.

# Source configuration if not already loaded
if [[ -z "${INIT_E_SUCCESS:-}" ]]; then

set -euo pipefail

  source "$(dirname "${BASH_SOURCE[0]}")/config.sh"
fi

# Ensure safe_echo is available
if ! type -t safe_echo >/dev/null 2>&1; then
  source "$(dirname "${BASH_SOURCE[0]}")/platform.sh"
fi

# Source platform compatibility utilities
VALIDATION_DIR="$(dirname "${BASH_SOURCE[0]}")"
if [[ -f "$VALIDATION_DIR/../utils/platform-compat.sh" ]]; then
  source "$VALIDATION_DIR/../utils/platform-compat.sh"
fi

# Check for required command dependencies
# Inputs: None (uses INIT_REQUIRED_COMMANDS array)
# Outputs: Error messages for missing commands
# Returns: 0 if all present, error code if missing
check_dependencies() {
  local has_issues=false
  local missing_commands=()

  # Check for required commands
  for cmd in "${INIT_REQUIRED_COMMANDS[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing_commands+=("$cmd")
      has_issues=true
    fi
  done

  if [[ "$has_issues" == true ]]; then
    log_error "Required commands not found:"
    for cmd in "${missing_commands[@]}"; do
      echo "  - $cmd" >&2
    done
    echo "Please install missing dependencies and try again" >&2
    return $INIT_E_TEMPFAIL
  fi

  return $INIT_E_SUCCESS
}

# Check if project already initialized
# Inputs: $1 - force flag (optional)
# Outputs: Warning message if already initialized
# Returns: 0 if ok to proceed, error code if not
check_existing_config() {
  local force="${1:-false}"

  if [[ -f ".env" ]] && [[ "$force" != true ]]; then
    echo ""
    log_warning "Project already initialized"
    echo ""
    echo "Found existing configuration files:"
    [[ -f ".env" ]] && echo "  ✓ .env"
    [[ -f ".env.example" ]] && echo "  ✓ .env.example"
    [[ -f ".gitignore" ]] && echo "  ✓ .gitignore"
    [[ -f "docker-compose.yml" ]] && echo "  ✓ docker-compose.yml (project already built)"
    echo ""
    echo "Options:"
    if [[ -f "docker-compose.yml" ]]; then
      echo "  • nself start           - Start your existing services"
      echo "  • nself build           - Rebuild with current configuration"
    else
      echo "  • nself build           - Build with current configuration"
      echo "  • nself start           - Start services after building"
    fi
    echo "  • nself init --force    - Reinitialize (overwrites config)"
    echo "  • nself reset           - Remove all project files and start fresh"
    echo ""
    return $INIT_E_CONFIG
  fi

  return $INIT_E_SUCCESS
}

# Perform security checks
# Inputs: None
# Outputs: Warning messages for security issues
# Returns: 0 to continue, error code to abort
security_checks() {
  # Check if running as root (not recommended)
  if [[ $EUID -eq 0 ]]; then
    log_warning "Running as root is not recommended for development."
    printf "%s" "Continue anyway? (y/N) " >&2
    read -r response
    # Bash 3.2 compatible check for yes/no
    case "$response" in
      [Yy]) ;;
      *) return $INIT_E_NOPERM ;;
    esac
  fi

  # Check umask for reasonable permissions
  local current_umask
  current_umask=$(umask)
  if [[ "$current_umask" != "0022" ]] && [[ "$current_umask" != "0002" ]]; then
    log_info "Current umask is $current_umask. Files will be created with these permissions."
  fi

  return $INIT_E_SUCCESS
}

# Validate project directory
# Inputs: None
# Outputs: Error messages if directory invalid
# Returns: 0 if valid, error code if not
validate_project_dir() {
  # Check if current directory is writable
  if [[ ! -w "." ]]; then
    log_error "Current directory is not writable"
    return $INIT_E_NOPERM
  fi

  # Check if in git repo root (silent check)
  if command -v git >/dev/null 2>&1; then
    if git rev-parse --git-dir >/dev/null 2>&1; then
      # Silent - no message about being in git repo
      true
    else
      # Silent - no message about not being in git repo
      true
    fi
  fi

  # Check for sufficient disk space (minimum 100MB)
  local available_space
  if command -v df >/dev/null 2>&1; then
    # Get available space in KB
    available_space=$(df -k . | awk 'NR==2 {print $4}')
    if [[ -n "$available_space" ]] && [[ "$available_space" -lt 102400 ]]; then
      log_warning "Low disk space available (< 100MB)"
    fi
  fi

  return $INIT_E_SUCCESS
}

# Validate environment mode
# Inputs: $1 - environment mode
# Outputs: None
# Returns: 0 if valid, 1 if not
validate_env_mode() {
  local mode="$1"

  case "$mode" in
    dev | development | prod | production | staging | test)
      return 0
      ;;
    *)
      log_error "Invalid environment mode: $mode"
      echo "Valid modes: dev, prod, staging, test" >&2
      return 1
      ;;
  esac
}

# Validate that Bash version meets requirements
# Inputs: None
# Outputs: Warning if version is old
# Returns: 0 (always continues, just warns)
check_bash_version() {
  local bash_version="${BASH_VERSION:-unknown}"
  local major_version="${bash_version%%.*}"

  # We support Bash 3.2+ but warn if very old
  if [[ "$major_version" =~ ^[0-9]+$ ]] && [[ "$major_version" -lt 3 ]]; then
    log_warning "Bash version $bash_version is very old. Some features may not work."
    echo "Recommended: Bash 3.2 or newer" >&2
  fi

  return 0
}

# Check for conflicting processes
# Inputs: None
# Outputs: Warning if nself services running
# Returns: 0 (always continues, just warns)
check_running_services() {
  # Check if Docker is running and has nself containers
  if command -v docker >/dev/null 2>&1; then
    local running_containers
    running_containers=$(docker ps --filter "label=com.nself.project" --format "{{.Names}}" 2>/dev/null | wc -l)

    if [[ "$running_containers" -gt 0 ]]; then
      printf "ℹ Found %s running nself container(s)\n" "$running_containers" >&2
      printf "Consider 'nself stop' before reinitializing\n" >&2
    fi
  fi

  return 0
}

# Validate file permissions
# Inputs: $1 - file path, $2 - expected permissions (octal)
# Outputs: Warning if permissions don't match
# Returns: 0 if match, 1 if not
validate_file_permissions() {
  local file="$1"
  local expected_perms="$2"

  if [[ ! -f "$file" ]]; then
    return 1 # File doesn't exist
  fi

  # Get current permissions (portable way)
  local current_perms

  # Try safe function if available, fallback to manual detection
  if type -t safe_stat_perms >/dev/null 2>&1; then
    current_perms=$(safe_stat_perms "$file" 2>/dev/null)
  elif command -v stat >/dev/null 2>&1; then
    # Try GNU stat first (Linux)
    current_perms=$(stat -c "%a" "$file" 2>/dev/null) || {
      # Fall back to BSD stat (macOS/FreeBSD)
      current_perms=$(stat -f "%OLp" "$file" 2>/dev/null) || {
        # If both fail, use ls as fallback
        current_perms=$(ls -l "$file" | awk '{print $1}' 2>/dev/null)
      }
    }
  else
    # No stat command, use ls
    current_perms=$(ls -l "$file" | awk '{print $1}' 2>/dev/null)
  fi

  if [[ "$current_perms" != "$expected_perms" ]]; then
    log_warning "File $file has permissions $current_perms, expected $expected_perms"
    return 1
  fi

  return 0
}

# Perform all validation checks
# Inputs: $1 - force flag, $2 - quiet flag
# Outputs: Various validation messages
# Returns: 0 if all pass, error code on first failure
perform_all_validations() {
  local force="${1:-false}"
  local quiet="${2:-false}"

  # Check Bash version (silent)
  check_bash_version >/dev/null 2>&1 || true

  # Check dependencies (silent)
  check_dependencies || return $?

  # Check existing config
  check_existing_config "$force" || return $?

  # Security checks
  security_checks || return $?

  # Validate project directory (silent unless there's an issue)
  validate_project_dir >/dev/null 2>&1 || return $?

  # Check for running services (silent)
  check_running_services >/dev/null 2>&1 || true

  # Single success line
  if [[ "$quiet" != true ]]; then
    safe_echo "${COLOR_BLUE:-}${CHECK_MARK:-✓} Validation passed${COLOR_RESET:-}"
  fi

  return $INIT_E_SUCCESS
}

# Export functions for use in other scripts
export -f check_dependencies check_existing_config security_checks
export -f validate_project_dir validate_env_mode check_bash_version
export -f check_running_services validate_file_permissions
export -f perform_all_validations
