#!/usr/bin/env bash

# progress.sh - Centralized progress indicators and spinners

# Source display utilities for colors (only if not already sourced)
if [[ -z "${DISPLAY_SOURCED:-}" ]]; then

set -euo pipefail

  SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
  source "$SCRIPT_DIR/display.sh"
fi

# Spinner animation
show_spinner() {
  local pid=$1
  local message=${2:-"Processing..."}
  local spin='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  local i=0

  tput civis # Hide cursor
  while kill -0 "$pid" 2>/dev/null; do
    i=$(((i + 1) % 10))
    printf "\r${COLOR_BLUE}%s${COLOR_RESET} %s" "${spin:$i:1}" "$message"
    sleep 0.1
  done
  printf "\r\033[K" # Clear line
  tput cnorm        # Show cursor
}

# Progress bar
show_progress() {
  local current=$1
  local total=$2
  local label="${3:-}"
  local width=${4:-50}

  local percent=$((current * 100 / total))
  local filled=$((width * current / total))

  printf "\r["
  printf "%${filled}s" | tr ' ' '='
  printf "%$((width - filled))s" | tr ' ' '-'
  printf "] %3d%%" "$percent"

  [[ -n "$label" ]] && printf " %s" "$label"
  [[ $current -eq $total ]] && echo
}

# Enhanced progress with time estimate
show_enhanced_progress() {
  local current=$1
  local total=$2
  local message="$3"
  local start_time=$4

  local percent=$((current * 100 / total))
  local elapsed=$(($(date +%s) - start_time))

  if [[ $current -gt 0 ]]; then
    local eta=$((elapsed * total / current - elapsed))
    printf "\r[%3d%%] %s (ETA: %ds)" "$percent" "$message" "$eta"
  else
    printf "\r[%3d%%] %s" "$percent" "$message"
  fi

  [[ $current -eq $total ]] && echo
}

# Step counter
show_steps() {
  local current=$1
  local total=$2
  local description=$3
  local status="${4:-}"

  printf "%s\n" "${COLOR_BOLD}Step $current/$total:${COLOR_RESET} $description"
  [[ -n "$status" ]] && echo "  Status: $status"
}

# Confirmation with timeout
confirm_with_timeout() {
  local message="$1"
  local timeout="${2:-30}"
  local default="${3:-n}"

  local response
  printf "%s\n" "${COLOR_YELLOW}${ICON_WARNING}${COLOR_RESET} $message"

  if read -t "$timeout" -p "Continue? [y/N] (timeout in ${timeout}s): " response; then
    [[ "$response" =~ ^[Yy]$ ]]
  else
    echo
    [[ "$default" == "y" ]]
  fi
}

# Start spinner with message
start_spinner() {
  local message="${1:-Processing...}"

  # Simple inline spinner - no background process
  printf "${COLOR_BLUE}⠋${COLOR_RESET} %s" "$message"
}

# Stop spinner with status
stop_spinner() {
  local status="${1:-success}"
  local message="${2:-Done}"

  # Clear the current line
  printf "\r\033[K"

  case "$status" in
    success)
      printf "%s\n" "${COLOR_GREEN}${ICON_SUCCESS}${COLOR_RESET} $message"
      ;;
    error)
      printf "%s\n" "${COLOR_RED}${ICON_FAILURE}${COLOR_RESET} $message"
      ;;
    warning)
      printf "%s\n" "${COLOR_YELLOW}${ICON_WARNING}${COLOR_RESET} $message"
      ;;
    *)
      printf "%s\n" "${COLOR_BLUE}${ICON_INFO}${COLOR_RESET} $message"
      ;;
  esac
}

# Export functions
export -f show_spinner show_progress show_enhanced_progress
export -f show_steps confirm_with_timeout start_spinner stop_spinner
