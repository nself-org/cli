#!/usr/bin/env bash

# display.sh - Centralized display utilities for consistent output

# Prevent double-sourcing
[[ "${DISPLAY_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export DISPLAY_SOURCED=1

# Color definitions (using printf for portability)
export COLOR_RESET=$'\033[0m'
export COLOR_RED=$'\033[0;31m'
export COLOR_GREEN=$'\033[0;32m'
export COLOR_YELLOW=$'\033[0;33m'
export COLOR_BLUE=$'\033[0;34m'
export COLOR_MAGENTA=$'\033[0;35m'
export COLOR_CYAN=$'\033[0;36m'
export COLOR_WHITE=$'\033[0;37m'
export COLOR_BOLD=$'\033[1m'
export COLOR_DIM=$'\033[2m'

# Icons
export ICON_SUCCESS="✓"
export ICON_FAILURE="✗"
export ICON_WARNING="⚠"
export ICON_INFO="ℹ"
export ICON_ARROW="→"
export ICON_BULLET="•"

# Check for NO_COLOR environment variable
if [[ -n "${NO_COLOR:-}" ]]; then
  # User explicitly requested no colors
  COLOR_RESET=""
  COLOR_RED=""
  COLOR_GREEN=""
  COLOR_YELLOW=""
  COLOR_BLUE=""
  COLOR_MAGENTA=""
  COLOR_CYAN=""
  COLOR_WHITE=""
  COLOR_BOLD=""
  COLOR_DIM=""
elif [[ ! -t 1 ]]; then
  # Not a terminal — disable colors so piped output and tests get plain text
  COLOR_RESET=""
  COLOR_RED=""
  COLOR_GREEN=""
  COLOR_YELLOW=""
  COLOR_BLUE=""
  COLOR_MAGENTA=""
  COLOR_CYAN=""
  COLOR_WHITE=""
  COLOR_BOLD=""
  COLOR_DIM=""
fi

# Logging functions - All commands must use these
log_info() {
  printf "%bℹ%b %s\n" "${COLOR_BLUE}" "${COLOR_RESET}" "$1"
}

log_success() {
  printf "%b✓%b %s\n" "${COLOR_GREEN}" "${COLOR_RESET}" "$1"
}

log_secondary() {
  printf "%b✓%b %s\n" "${COLOR_BLUE}" "${COLOR_RESET}" "$1"
}

log_warning() {
  printf "%b✱%b %s\n" "${COLOR_YELLOW}" "${COLOR_RESET}" "$1" >&2
}

log_error() {
  printf "%b✗%b %s\n" "${COLOR_RED}" "${COLOR_RESET}" "$1" >&2
}

log_debug() {
  if [[ "${DEBUG:-false}" == "true" ]]; then
    printf "%b[DEBUG]%b %s\n" "${COLOR_MAGENTA}" "${COLOR_RESET}" "$1"
  fi
}

# Export logging functions
export -f log_info
export -f log_success
export -f log_warning
export -f log_error
export -f log_debug

# Source the standardized header utilities
UTILS_DIR="$(dirname "${BASH_SOURCE[0]}")"
if [[ -f "$UTILS_DIR/header.sh" ]]; then
  . "$UTILS_DIR/header.sh"
fi

# Header is now provided by header.sh but we keep this for backward compatibility
# It delegates to the standardized function

# Standardized command header with box
# Args: $1 = title (e.g., "nself build"), $2 = subtitle (description)
show_command_header() {
  local title="${1:-}"
  local subtitle="${2:-}"
  local box_width=60
  local content_width=$((box_width - 2)) # Account for borders only

  # Calculate padding for title
  local title_len=${#title}
  local title_padding=$((content_width - title_len - 1)) # -1 for the leading space

  # Calculate padding for subtitle
  local subtitle_len=${#subtitle}
  local subtitle_padding=$((content_width - subtitle_len - 1)) # -1 for the leading space

  echo
  printf "${COLOR_BLUE}╔══════════════════════════════════════════════════════════╗${COLOR_RESET}\n"
  printf "${COLOR_BLUE}║${COLOR_RESET} ${COLOR_BOLD}%s${COLOR_RESET}%*s${COLOR_BLUE}║${COLOR_RESET}\n" "$title" $title_padding " "
  printf "${COLOR_BLUE}║${COLOR_RESET} ${COLOR_DIM}%s${COLOR_RESET}%*s${COLOR_BLUE}║${COLOR_RESET}\n" "$subtitle" $subtitle_padding " "
  printf "${COLOR_BLUE}╚══════════════════════════════════════════════════════════╝${COLOR_RESET}\n"
}

# Alias for compatibility
log_header() {
  show_command_header "$@"
}

# Another alias for backward compatibility
show_header() {
  show_command_header "$@"
}

show_section() {
  local title="$1"
  echo
  printf "${COLOR_CYAN}▶ $title${COLOR_RESET}\n"
}

# Table formatting
show_table_header() {
  local -a headers=("$@")

  printf "┌"
  for header in "${headers[@]}"; do
    printf "─%.0s" $(seq 1 $((${#header} + 2)))
    printf "┬"
  done
  printf "\b┐\n"

  printf "│"
  for header in "${headers[@]}"; do
    printf " %-${#header}s │" "$header"
  done
  printf "\n"

  printf "├"
  for header in "${headers[@]}"; do
    printf "─%.0s" $(seq 1 $((${#header} + 2)))
    printf "┼"
  done
  printf "\b┤\n"
}

show_table_row() {
  printf "│"
  for value in "$@"; do
    printf " %s │" "$value"
  done
  printf "\n"
}

show_table_footer() {
  local -a headers=("$@")
  printf "└"
  for header in "${headers[@]}"; do
    printf "─%.0s" $(seq 1 $((${#header} + 2)))
    printf "┴"
  done
  printf "\b┘\n"
}

# Box drawing
draw_box() {
  local message="$1"
  local type="${2:-info}"
  local width=$((${#message} + 4))

  case "$type" in
    success) local color="$COLOR_GREEN" ;;
    error) local color="$COLOR_RED" ;;
    warning) local color="$COLOR_YELLOW" ;;
    *) local color="$COLOR_BLUE" ;;
  esac

  printf "${color}┌$(printf '─%.0s' $(seq 1 $width))┐${COLOR_RESET}\n"
  printf "${color}│  $message  │${COLOR_RESET}\n"
  printf "${color}└$(printf '─%.0s' $(seq 1 $width))┘${COLOR_RESET}\n"
}

# Strip colors for log file output
strip_colors() {
  sed 's/\x1b\[[0-9;]*m//g'
}

# Loading spinner that can be replaced with a message
# Usage: LOADING_PID=$(start_loading "message")
#        stop_loading $LOADING_PID "replacement message"
start_loading() {
  local message="$1"
  local spin='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  local i=0

  # Only show spinner in terminal
  if [[ -t 1 ]]; then
    (
      while true; do
        i=$(((i + 1) % 10))
        printf "\r%b%s%b %s" "${COLOR_BLUE}" "${spin:$i:1}" "${COLOR_RESET}" "$message"
        sleep 0.1
      done
    ) &
    echo $!
  else
    echo 0 # Return dummy PID when not in terminal
  fi
}

stop_loading() {
  local pid="$1"
  local message="$2"

  # Kill the spinner process if it's running
  if [[ "$pid" != "0" ]]; then
    kill $pid 2>/dev/null
    wait $pid 2>/dev/null
  fi

  # Clear the line and show the final message
  if [[ -t 1 ]]; then
    printf "\r\033[K%s\n" "$message"
  else
    echo "$message"
  fi
}

# Export all functions
export -f log_info log_success log_secondary log_warning log_error log_debug log_header
export -f show_command_header show_header show_section log_header
export -f show_table_header show_table_row show_table_footer
export -f draw_box strip_colors start_loading stop_loading
