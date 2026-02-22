#!/usr/bin/env bash

# post-command.sh - Post-command cleanup and reporting hooks

# Source utilities (namespaced to avoid clobbering caller globals)
_HOOKS_SHARED_DIR="$(dirname "${BASH_SOURCE[0]}")/.."

set -euo pipefail

source "$_HOOKS_SHARED_DIR/utils/display.sh" 2>/dev/null || true

# Post-command cleanup
post_command() {
  local command="$1"
  local exit_code="${2:-0}"

  log_debug "Running post-command hooks for: $command (exit: $exit_code)"

  # Log command completion
  log_command_completion "$command" "$exit_code"

  # Clean up temporary files
  cleanup_temp_files

  # Show URLs after certain commands
  if [[ "$exit_code" -eq 0 ]]; then
    case "$command" in
      up | status)
        show_service_urls
        ;;
    esac
  fi

  # Show completion status
  if [[ "$exit_code" -eq 0 ]]; then
    post_command_success "$command"
  else
    post_command_failure "$command"
  fi

  return "$exit_code"
}

# Log command completion
log_command_completion() {
  local command="$1"
  local exit_code="$2"
  local log_file="logs/nself.log"

  local status="SUCCESS"
  if [[ $exit_code -ne 0 ]]; then status="FAILED"; fi

  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Completed: nself $command - $status (exit: $exit_code)" >>"$log_file"
}

# Clean up temporary files
cleanup_temp_files() {
  # Remove old temporary files (older than 1 hour)
  find /tmp -name "nself_*.tmp" -mmin +60 -delete 2>/dev/null || true

  # Remove empty log files
  find logs -name "*.log" -size 0 -delete 2>/dev/null || true

  # Show cursor if it was hidden
  tput cnorm 2>/dev/null || true
}

# Show service URLs
show_service_urls() {
  if [[ -n "${BASE_DOMAIN:-}" ]]; then
    echo
    show_section "Service URLs"
    echo "  ${ICON_ARROW} GraphQL:    ${COLOR_CYAN}https://api.${BASE_DOMAIN}${COLOR_RESET}"
    echo "  ${ICON_ARROW} Auth:       ${COLOR_CYAN}https://auth.${BASE_DOMAIN}${COLOR_RESET}"
    echo "  ${ICON_ARROW} Storage:    ${COLOR_CYAN}https://storage.${BASE_DOMAIN}${COLOR_RESET}"
    echo "  ${ICON_ARROW} Dashboard:  ${COLOR_CYAN}https://dashboard.${BASE_DOMAIN}${COLOR_RESET}"
    echo
  fi
}

# Success message
post_command_success() {
  local command="$1"

  case "$command" in
    init)
      draw_box "Project initialized successfully!" "success"
      echo "Next steps:"
      echo "  1. Review and edit .env.local"
      echo "  2. Run: nself build"
      echo "  3. Run: nself start"
      ;;

    build)
      draw_box "Build completed successfully!" "success"
      ;;

    up)
      draw_box "Services started successfully!" "success"
      ;;

    down)
      draw_box "Services stopped successfully!" "success"
      ;;

    *)
      log_success "Command completed successfully"
      ;;
  esac
}

# Failure message
post_command_failure() {
  local command="$1"

  draw_box "Command failed: $command" "error"
  echo "Check the logs above for details."
  echo "For help, run: nself help"
}

# Export functions
export -f post_command log_command_completion cleanup_temp_files
export -f show_service_urls post_command_success post_command_failure
