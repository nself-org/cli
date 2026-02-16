#!/usr/bin/env bash

# ux-standards.sh - Comprehensive UX standardization library for nself v0.9.8
# Provides consistent error messages, progress indicators, validation, and help text
# Cross-platform compatible (Bash 3.2+, POSIX where possible)

# Prevent double-sourcing
[[ "${UX_STANDARDS_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export UX_STANDARDS_SOURCED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_UX_STANDARDS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_UX_STANDARDS_DIR}/display.sh" 2>/dev/null || true
source "${_UX_STANDARDS_DIR}/cli-output.sh" 2>/dev/null || true
source "${_UX_STANDARDS_DIR}/platform-compat.sh" 2>/dev/null || true

# =============================================================================
# USER-FRIENDLY ERROR MESSAGES
# Format: "Problem: X. Fix: Run Y command."
# =============================================================================

# Show actionable error with problem and fix
ux_error() {
  local problem="$1"
  local fix="$2"
  local context="${3:-}"

  printf "\n${COLOR_RED}✗ Problem:${COLOR_RESET} %s\n" "$problem"

  if [[ -n "$context" ]]; then
    printf "${COLOR_DIM}Context: %s${COLOR_RESET}\n" "$context"
  fi

  printf "${COLOR_GREEN}Fix:${COLOR_RESET} %s\n\n" "$fix"
}

# Show warning with actionable advice
ux_warning() {
  local message="$1"
  local advice="${2:-}"

  printf "${COLOR_YELLOW}⚠ Warning:${COLOR_RESET} %s\n" "$message"

  if [[ -n "$advice" ]]; then
    printf "${COLOR_CYAN}→ Suggestion:${COLOR_RESET} %s\n" "$advice"
  fi
  printf "\n"
}

# Show success with next steps
ux_success() {
  local message="$1"
  local next_steps="${2:-}"

  printf "${COLOR_GREEN}✓ Success:${COLOR_RESET} %s\n" "$message"

  if [[ -n "$next_steps" ]]; then
    printf "${COLOR_CYAN}→ Next:${COLOR_RESET} %s\n" "$next_steps"
  fi
  printf "\n"
}

# Show info with helpful context
ux_info() {
  local message="$1"
  local details="${2:-}"

  printf "${COLOR_BLUE}ℹ Info:${COLOR_RESET} %s\n" "$message"

  if [[ -n "$details" ]]; then
    printf "${COLOR_DIM}%s${COLOR_RESET}\n" "$details"
  fi
  printf "\n"
}

# =============================================================================
# COMMON ERROR SCENARIOS (Top 10)
# =============================================================================

# 1. File not found error
ux_error_file_not_found() {
  local file="$1"
  local suggestion="${2:-Check the file path and try again}"

  ux_error \
    "File not found: ${file}" \
    "$suggestion" \
    "Current directory: $(pwd)"
}

# 2. Docker not running
ux_error_docker_not_running() {
  local fix_cmd

  if is_macos; then
    fix_cmd="Open Docker Desktop or run: open -a Docker"
  elif is_linux; then
    fix_cmd="sudo systemctl start docker"
  else
    fix_cmd="Start Docker Desktop from your applications"
  fi

  ux_error \
    "Docker daemon is not running" \
    "$fix_cmd" \
    "Docker is required for nself to work"
}

# 3. Configuration missing
ux_error_config_missing() {
  local config_file="${1:-.env}"

  ux_error \
    "Configuration file not found: ${config_file}" \
    "Run 'nself init' to create configuration" \
    "This will create ${config_file} with default settings"
}

# 4. Port already in use
ux_error_port_in_use() {
  local port="$1"
  local service="${2:-service}"

  local kill_cmd
  if is_macos; then
    kill_cmd="lsof -ti:${port} | xargs kill -9"
  else
    kill_cmd="sudo kill \$(lsof -ti:${port})"
  fi

  ux_error \
    "Port ${port} is already in use by another process" \
    "Stop the conflicting process: ${kill_cmd}" \
    "Service: ${service}"
}

# 5. Service failed to start
ux_error_service_failed() {
  local service="$1"
  local reason="${2:-unknown error}"

  ux_error \
    "Service '${service}' failed to start" \
    "Check logs: nself logs ${service}" \
    "Reason: ${reason}"
}

# 6. Permission denied
ux_error_permission_denied() {
  local path="$1"
  local operation="${2:-access}"

  local fix_cmd
  if [[ "$path" =~ docker ]] || [[ "$path" =~ /var/run ]]; then
    if is_linux; then
      fix_cmd="Add user to docker group: sudo usermod -aG docker \$USER (then log out and back in)"
    else
      fix_cmd="Restart Docker Desktop"
    fi
  else
    fix_cmd="Fix permissions: sudo chown -R \$(whoami) ${path}"
  fi

  ux_error \
    "Permission denied" \
    "$fix_cmd" \
    "Cannot ${operation}: ${path}"
}

# 7. Invalid input
ux_error_invalid_input() {
  local input="$1"
  local expected="${2:-valid input}"
  local example="${3:-}"

  local context="Expected: ${expected}"
  if [[ -n "$example" ]]; then
    context="${context}. Example: ${example}"
  fi

  ux_error \
    "Invalid input: '${input}'" \
    "Provide ${expected}" \
    "$context"
}

# 8. Service not running
ux_error_service_not_running() {
  local service="$1"

  ux_error \
    "Service '${service}' is not running" \
    "Start services: nself start" \
    "Or check status: nself status"
}

# 9. Network connectivity error
ux_error_network() {
  local service="$1"
  local url="${2:-}"

  local context="Service: ${service}"
  if [[ -n "$url" ]]; then
    context="${context}, URL: ${url}"
  fi

  ux_error \
    "Network connection failed" \
    "Check connectivity: ping -c 3 google.com" \
    "$context"
}

# 10. Insufficient resources
ux_error_resources() {
  local resource="${1:-memory}"
  local required="${2:-}"

  local fix="Free up ${resource}"
  if [[ "$resource" == "memory" ]]; then
    fix="Close unnecessary applications or increase Docker memory allocation"
  elif [[ "$resource" == "disk" ]]; then
    fix="Clean up space: docker system prune -a --volumes"
  fi

  ux_error \
    "Insufficient ${resource}" \
    "$fix" \
    "Required: ${required}"
}

# =============================================================================
# PROGRESS INDICATORS
# =============================================================================

# Global progress tracking
UX_PROGRESS_STEPS=()
UX_PROGRESS_STATUS=()
UX_CURRENT_STEP=0
UX_SHOW_PROGRESS=true

# Initialize progress tracker
ux_progress_init() {
  UX_PROGRESS_STEPS=()
  UX_PROGRESS_STATUS=()
  UX_CURRENT_STEP=0

  # Check if running in terminal
  if [[ ! -t 1 ]]; then
    UX_SHOW_PROGRESS=false
  fi
}

# Add progress step
ux_progress_add() {
  local step="$1"
  UX_PROGRESS_STEPS+=("$step")
  UX_PROGRESS_STATUS+=("pending")
}

# Update progress step status
ux_progress_update() {
  local step_index="$1"
  local status="$2"  # pending, running, done, error
  local message="${3:-}"

  if [[ "$step_index" -ge "${#UX_PROGRESS_STEPS[@]}" ]]; then
    return 1
  fi

  UX_PROGRESS_STATUS[$step_index]="$status"

  if [[ "$UX_SHOW_PROGRESS" == "false" ]]; then
    return 0
  fi

  local step_name="${UX_PROGRESS_STEPS[$step_index]}"

  case "$status" in
    pending)
      printf "  ${COLOR_DIM}○${COLOR_RESET} %s\n" "$step_name"
      ;;
    running)
      printf "\r  ${COLOR_BLUE}⠋${COLOR_RESET} %s..." "$step_name"
      ;;
    done)
      printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} %-40s" "$step_name"
      if [[ -n "$message" ]]; then
        printf " ${COLOR_DIM}%s${COLOR_RESET}" "$message"
      fi
      printf "\n"
      ;;
    error)
      printf "\r  ${COLOR_RED}✗${COLOR_RESET} %-40s" "$step_name"
      if [[ -n "$message" ]]; then
        printf " ${COLOR_RED}%s${COLOR_RESET}" "$message"
      fi
      printf "\n"
      ;;
  esac
}

# Show spinner for long-running operations
ux_spinner_start() {
  local message="$1"
  local pid_file="${2:-/tmp/ux_spinner.pid}"

  if [[ ! -t 1 ]]; then
    printf "%s...\n" "$message"
    echo "0" > "$pid_file"
    return
  fi

  (
    local spinners=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
    local i=0
    while true; do
      printf "\r  ${COLOR_BLUE}%s${COLOR_RESET} %s..." "${spinners[$i]}" "$message"
      i=$(((i + 1) % 10))
      sleep 0.1
    done
  ) &

  echo $! > "$pid_file"
}

# Stop spinner
ux_spinner_stop() {
  local pid_file="${1:-/tmp/ux_spinner.pid}"
  local final_message="${2:-}"

  if [[ -f "$pid_file" ]]; then
    local spinner_pid=$(cat "$pid_file")
    if [[ "$spinner_pid" != "0" ]]; then
      kill "$spinner_pid" 2>/dev/null || true
      wait "$spinner_pid" 2>/dev/null || true
    fi
    rm -f "$pid_file"
  fi

  if [[ -n "$final_message" ]]; then
    printf "\r  ${COLOR_GREEN}✓${COLOR_RESET} %s\n" "$final_message"
  else
    printf "\r%-60s\r" " "
  fi
}

# =============================================================================
# INPUT VALIDATION
# =============================================================================

# Validate required argument
ux_validate_required() {
  local arg_value="$1"
  local arg_name="$2"
  local example="${3:-}"

  if [[ -z "$arg_value" ]]; then
    ux_error_invalid_input \
      "(empty)" \
      "${arg_name}" \
      "$example"
    return 1
  fi

  return 0
}

# Validate port number
ux_validate_port() {
  local port="$1"

  if ! [[ "$port" =~ ^[0-9]+$ ]]; then
    ux_error_invalid_input \
      "$port" \
      "numeric port (1-65535)" \
      "3000, 8080, 5432"
    return 1
  fi

  if [[ "$port" -lt 1 ]] || [[ "$port" -gt 65535 ]]; then
    ux_error_invalid_input \
      "$port" \
      "port between 1 and 65535" \
      "Common ports: 3000, 8080, 8000"
    return 1
  fi

  return 0
}

# Validate file exists
ux_validate_file_exists() {
  local file="$1"
  local suggestion="${2:-Check the file path}"

  if [[ ! -f "$file" ]]; then
    ux_error_file_not_found "$file" "$suggestion"
    return 1
  fi

  return 0
}

# Validate directory exists
ux_validate_dir_exists() {
  local dir="$1"
  local suggestion="${2:-Check the directory path}"

  if [[ ! -d "$dir" ]]; then
    ux_error \
      "Directory not found: ${dir}" \
      "$suggestion" \
      "Current directory: $(pwd)"
    return 1
  fi

  return 0
}

# Validate environment name
ux_validate_env() {
  local env="$1"

  case "$env" in
    local|dev|development|staging|stage|prod|production)
      return 0
      ;;
    *)
      ux_error_invalid_input \
        "$env" \
        "valid environment name" \
        "local, staging, production"
      return 1
      ;;
  esac
}

# Validate Docker is running
ux_validate_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    ux_error \
      "Docker is not installed" \
      "Install Docker from https://docker.com" \
      "Docker is required for nself to work"
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    ux_error_docker_not_running
    return 1
  fi

  return 0
}

# =============================================================================
# STANDARDIZED HELP TEXT
# =============================================================================

# Show command help header
ux_help_header() {
  local command="$1"
  local description="$2"

  printf "\n${COLOR_BOLD}%s${COLOR_RESET} - %s\n\n" "$command" "$description"
}

# Show help section
ux_help_section() {
  local title="$1"
  printf "${COLOR_CYAN}%s:${COLOR_RESET}\n" "$title"
}

# Show help option
ux_help_option() {
  local flag="$1"
  local description="$2"

  printf "  ${COLOR_YELLOW}%-20s${COLOR_RESET} %s\n" "$flag" "$description"
}

# Show help example
ux_help_example() {
  local description="$1"
  local command="$2"

  printf "  ${COLOR_DIM}%s${COLOR_RESET}\n" "$description"
  printf "  ${COLOR_GREEN}%s${COLOR_RESET}\n\n" "$command"
}

# Standard help template
ux_show_help() {
  local command="$1"
  local description="$2"
  local usage="$3"
  shift 3

  ux_help_header "$command" "$description"

  ux_help_section "Usage"
  printf "  %s\n\n" "$usage"

  # Additional sections passed as arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --section)
        ux_help_section "$2"
        shift 2
        ;;
      --option)
        ux_help_option "$2" "$3"
        shift 3
        ;;
      --example)
        ux_help_example "$2" "$3"
        shift 3
        ;;
      *)
        shift
        ;;
    esac
  done

  ux_help_section "See Also"
  printf "  ${COLOR_DIM}Run 'nself help' for all commands${COLOR_RESET}\n"
  printf "  ${COLOR_DIM}Docs: .wiki/commands/$(echo "$command" | tr ' ' '-' | tr '[:upper:]' '[:lower:]').md${COLOR_RESET}\n\n"
}

# =============================================================================
# COMMAND ALIASES
# =============================================================================

# Resolve alias to actual command (Bash 3.2 compatible)
ux_resolve_alias() {
  local alias="$1"
  case "$alias" in
    ps) echo "status" ;;
    ls) echo "list" ;;
    rm|del) echo "remove" ;;
    restart-all) echo "restart" ;;
    log|tail) echo "logs" ;;
    run|shell) echo "exec" ;;
    up) echo "start" ;;
    down) echo "stop" ;;
    *) echo "$alias" ;;  # Return original if no alias
  esac
}

# =============================================================================
# COLOR & FORMATTING STANDARDS
# =============================================================================

# Color coding standards
UX_COLOR_SUCCESS="${COLOR_GREEN}"
UX_COLOR_ERROR="${COLOR_RED}"
UX_COLOR_WARNING="${COLOR_YELLOW}"
UX_COLOR_INFO="${COLOR_BLUE}"
UX_COLOR_HINT="${COLOR_CYAN}"
UX_COLOR_DIM="${COLOR_DIM}"

# Symbol standards
UX_SYMBOL_SUCCESS="✓"
UX_SYMBOL_ERROR="✗"
UX_SYMBOL_WARNING="⚠"
UX_SYMBOL_INFO="ℹ"
UX_SYMBOL_ARROW="→"
UX_SYMBOL_BULLET="•"

# =============================================================================
# EXPORT FUNCTIONS
# =============================================================================

export -f ux_error ux_warning ux_success ux_info
export -f ux_error_file_not_found ux_error_docker_not_running ux_error_config_missing
export -f ux_error_port_in_use ux_error_service_failed ux_error_permission_denied
export -f ux_error_invalid_input ux_error_service_not_running ux_error_network ux_error_resources
export -f ux_progress_init ux_progress_add ux_progress_update
export -f ux_spinner_start ux_spinner_stop
export -f ux_validate_required ux_validate_port ux_validate_file_exists
export -f ux_validate_dir_exists ux_validate_env ux_validate_docker
export -f ux_help_header ux_help_section ux_help_option ux_help_example ux_show_help
export -f ux_resolve_alias
