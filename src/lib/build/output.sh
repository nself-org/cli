#!/usr/bin/env bash

# output.sh - Output helpers for build

# Color codes (portable)
setup_colors() {

set -euo pipefail

  if [[ -t 1 ]]; then
    COLOR_RED='\033[0;31m'
    COLOR_GREEN='\033[0;32m'
    COLOR_YELLOW='\033[0;33m'
    COLOR_BLUE='\033[0;34m'
    COLOR_RESET='\033[0m'
    COLOR_BOLD='\033[1m'
  else
    COLOR_RED=''
    COLOR_GREEN=''
    COLOR_YELLOW=''
    COLOR_BLUE=''
    COLOR_RESET=''
    COLOR_BOLD=''
  fi
}

# Initialize colors
setup_colors

# Show progress message
show_progress() {
  local message="$1"
  printf "${COLOR_BLUE}⠋${COLOR_RESET} %s" "$message"
}

# Show success message
show_success() {
  local message="$1"
  printf "\r${COLOR_GREEN}✓${COLOR_RESET} %-60s\n" "$message"
}

# Show error message
show_error() {
  local message="$1"
  printf "\r${COLOR_RED}✗${COLOR_RESET} %-60s\n" "$message" >&2
}

# Show warning message
show_warning() {
  local message="$1"
  printf "\r${COLOR_YELLOW}⚠${COLOR_RESET} %-60s\n" "$message"
}

# Show info message
show_info() {
  local message="$1"
  printf "${COLOR_BLUE}ℹ${COLOR_RESET} %s\n" "$message"
}

# Clear current line
clear_line() {
  printf "\r%-80s\r" " "
}

# Show build summary
show_build_summary() {
  echo ""
  echo "${COLOR_BOLD}Build Summary:${COLOR_RESET}"
  echo "─────────────────────────────────────────"

  if [[ ${#CREATED_FILES[@]} -gt 0 ]]; then
    echo ""
    echo "${COLOR_GREEN}✓ Created:${COLOR_RESET}"
    for file in "${CREATED_FILES[@]}"; do
      echo "  • $file"
    done
  fi

  if [[ ${#UPDATED_FILES[@]} -gt 0 ]]; then
    echo ""
    echo "${COLOR_BLUE}↻ Updated:${COLOR_RESET}"
    for file in "${UPDATED_FILES[@]}"; do
      echo "  • $file"
    done
  fi

  if [[ ${#SKIPPED_FILES[@]} -gt 0 ]]; then
    echo ""
    echo "${COLOR_YELLOW}⊝ Skipped:${COLOR_RESET}"
    for file in "${SKIPPED_FILES[@]}"; do
      echo "  • $file"
    done
  fi

  if [[ ${#BUILD_ERRORS[@]} -gt 0 ]]; then
    echo ""
    echo "${COLOR_RED}✗ Errors:${COLOR_RESET}"
    for error in "${BUILD_ERRORS[@]}"; do
      echo "  • $error"
    done
  fi

  echo ""
  echo "─────────────────────────────────────────"

  # Show next steps
  show_next_steps
}

# Show next steps
show_next_steps() {
  echo ""
  echo "${COLOR_BOLD}Next Steps:${COLOR_RESET}"

  local has_errors=false
  # Initialize BUILD_ERRORS if not set
  if [[ -z "${BUILD_ERRORS+x}" ]]; then
    BUILD_ERRORS=()
  fi
  if [[ ${#BUILD_ERRORS[@]} -gt 0 ]]; then
    has_errors=true
    echo "  1. Fix the errors listed above"
    echo "  2. Run 'nself build' again"
  else
    local base_domain="${BASE_DOMAIN:-localhost}"
    local needs_trust=false

    # Check if we need to run trust command
    if [[ "$base_domain" == "localhost" ]] || [[ "$base_domain" == *".localhost" ]]; then
      # Check if mkcert CA is trusted
      if command -v mkcert >/dev/null 2>&1; then
        if ! mkcert -install -check 2>/dev/null; then
          needs_trust=true
        fi
      fi
    fi

    if [[ "$needs_trust" == "true" ]]; then
      echo "  1. Run 'nself trust' to install SSL certificates"
      echo "     Trust the root CA for green locks in browsers"
      echo "  2. Run 'nself start' to launch services"
      echo "  3. Access your application at:"
    else
      echo "  1. Review the generated configuration"
      echo "  2. Run 'nself start' to launch services"
      echo "  3. Access your application at:"
    fi

    if [[ "${SSL_ENABLED:-true}" == "true" ]]; then
      echo "     • https://${base_domain}"
    else
      echo "     • http://${base_domain}"
    fi

    if [[ "${HASURA_ENABLED:-false}" == "true" ]]; then
      echo "     • GraphQL: https://${HASURA_ROUTE:-api.${base_domain}}/console"
    fi

    if [[ "${NSELF_ADMIN_ENABLED:-false}" == "true" ]]; then
      echo "     • Admin: https://${NSELF_ADMIN_ROUTE:-admin.${base_domain}}"
    fi
  fi

  echo ""
  echo "For help, run: nself help"
  echo ""
}

# Progress spinner
show_spinner() {
  local pid=$1
  local message="${2:-Processing...}"
  local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  local delay=0.1

  while kill -0 "$pid" 2>/dev/null; do
    local temp=${spinstr#?}
    printf "\r${COLOR_BLUE}%c${COLOR_RESET} %s" "${spinstr:0:1}" "$message"
    spinstr=$temp${spinstr%"$temp"}
    sleep $delay
  done

  wait "$pid"
  local exit_code=$?

  if [[ $exit_code -eq 0 ]]; then
    printf "\r${COLOR_GREEN}✓${COLOR_RESET} %-60s\n" "$message"
  else
    printf "\r${COLOR_RED}✗${COLOR_RESET} %-60s\n" "$message"
  fi

  return $exit_code
}

# Verbose output (for debugging)
verbose_output() {
  if [[ "${VERBOSE:-false}" == "true" ]]; then
    echo "DEBUG: $*" >&2
  fi
}

# Log to file
log_to_file() {
  local log_file="${BUILD_LOG:-build.log}"
  local message="$1"
  local timestamp=$(date "+%Y-%m-%d %H:%M:%S")

  echo "[$timestamp] $message" >>"$log_file"
}

# Export functions
export -f setup_colors
export -f show_progress
export -f show_success
export -f show_error
export -f show_warning
export -f show_info
export -f clear_line
export -f show_build_summary
export -f show_next_steps
export -f show_spinner
export -f verbose_output
export -f log_to_file
