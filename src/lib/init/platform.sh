#!/usr/bin/env bash

# platform.sh - Platform detection and terminal capabilities for nself init
#
# This module handles platform-specific detection, terminal capabilities,
# and display settings to ensure cross-platform compatibility.

# Detect the operating system platform
# Inputs: None
# Outputs: Sets global OS variable
# Returns: 0
detect_platform() {

set -euo pipefail

  case "$OSTYPE" in
    linux*) OS="linux" ;;
    darwin*) OS="macos" ;;
    msys* | cygwin* | mingw*) OS="windows-bash" ;;
    freebsd*) OS="freebsd" ;;
    openbsd*) OS="openbsd" ;;
    solaris*) OS="solaris" ;;
    *) OS="unknown" ;;
  esac

  export OS
  return 0
}

# Detect terminal type and capabilities
# Inputs: None
# Outputs: Sets TERMINAL, SUPPORTS_UNICODE, SUPPORTS_COLOR variables
# Returns: 0
detect_terminal() {
  # Ensure TERM is set (required for tput and other terminal operations)
  : ${TERM:=xterm}

  # Detect terminal type
  TERMINAL="${TERM:-dumb}"

  # Check for Unicode support
  SUPPORTS_UNICODE=false
  case "$TERMINAL" in
    *256color* | *unicode* | xterm* | rxvt* | screen* | tmux* | alacritty* | kitty*)
      # These terminals generally support Unicode
      if [[ "$OS" != "windows-bash" ]] || [[ -n "${WT_SESSION:-}" ]]; then
        # Not Windows bash, or Windows Terminal detected
        SUPPORTS_UNICODE=true
      fi
      ;;
  esac

  # Override check: LANG/LC_ALL
  if [[ "${LANG:-}" == *UTF* ]] || [[ "${LC_ALL:-}" == *UTF* ]]; then
    SUPPORTS_UNICODE=true
  elif [[ "${LANG:-}" == "C" ]] || [[ "${LC_ALL:-}" == "C" ]]; then
    SUPPORTS_UNICODE=false
  fi

  # Check for color support
  SUPPORTS_COLOR=false
  if [[ -t 1 ]]; then # Check if stdout is a terminal
    case "$TERMINAL" in
      *color* | xterm* | rxvt* | screen* | tmux* | vt100* | linux | alacritty* | kitty*)
        SUPPORTS_COLOR=true
        ;;
    esac

    # Additional check using tput if available
    if command -v tput >/dev/null 2>&1; then
      # Set TERM if not set (needed in CI environments)
      : ${TERM:=xterm}
      if [[ $(tput colors 2>/dev/null) -ge 8 ]]; then
        SUPPORTS_COLOR=true
      fi
    fi
  fi

  # Check for NO_COLOR environment variable (https://no-color.org/)
  if [[ -n "${NO_COLOR:-}" ]]; then
    SUPPORTS_COLOR=false
  fi

  # Force color if requested
  if [[ "${FORCE_COLOR:-}" == "1" ]] || [[ "${FORCE_COLOR:-}" == "true" ]]; then
    SUPPORTS_COLOR=true
  fi

  export TERMINAL SUPPORTS_UNICODE SUPPORTS_COLOR
  return 0
}

# Setup terminal colors based on capabilities
# Inputs: None (uses SUPPORTS_COLOR variable)
# Outputs: Sets COLOR_* variables
# Returns: 0
setup_colors() {
  if [[ "$SUPPORTS_COLOR" == true ]]; then
    COLOR_RESET="\033[0m"
    COLOR_GREEN="\033[32m"
    COLOR_RED="\033[31m"
    COLOR_YELLOW="\033[33m"
    COLOR_BLUE="\033[34m"
    COLOR_CYAN="\033[36m"
    COLOR_BOLD="\033[1m"
    COLOR_DIM="\033[2m"
  else
    COLOR_RESET=""
    COLOR_GREEN=""
    COLOR_RED=""
    COLOR_YELLOW=""
    COLOR_BLUE=""
    COLOR_CYAN=""
    COLOR_BOLD=""
    COLOR_DIM=""
  fi

  # Export for consistency
  export COLOR_RESET COLOR_GREEN COLOR_RED COLOR_YELLOW
  export COLOR_BLUE COLOR_CYAN COLOR_BOLD COLOR_DIM

  return 0
}

# Setup display characters based on Unicode support
# Inputs: None (uses SUPPORTS_UNICODE variable)
# Outputs: Sets display character variables
# Returns: 0
setup_display_chars() {
  if [[ "$SUPPORTS_UNICODE" == true ]]; then
    CHECK_MARK="✓"
    CROSS_MARK="✗"
    WARNING_SIGN="⚠"
    INFO_SIGN="ℹ"
    LIGHTNING="⚡"
    ARROW="➞"
    BULLET="•"
  else
    CHECK_MARK="[OK]"
    CROSS_MARK="[X]"
    WARNING_SIGN="[!]"
    INFO_SIGN="[i]"
    LIGHTNING="*"
    ARROW="->"
    BULLET="-"
  fi

  # Export display characters for global use
  export CHECK_MARK CROSS_MARK WARNING_SIGN INFO_SIGN
  export LIGHTNING ARROW BULLET

  return 0
}

# Setup display functions if not already loaded
# Inputs: None
# Outputs: Defines log_* functions if needed
# Returns: 0
setup_display_functions() {
  # Only define if not already loaded from display.sh
  if ! type -t log_error >/dev/null 2>&1; then
    log_error() { echo "${COLOR_RED}${CROSS_MARK}${COLOR_RESET} $*" >&2; }
    log_warning() { echo "${COLOR_YELLOW}${WARNING_SIGN}${COLOR_RESET} $*" >&2; }
    log_info() { echo "${COLOR_BLUE}${INFO_SIGN}${COLOR_RESET} $*" >&2; }
    log_success() { echo "${COLOR_GREEN}${CHECK_MARK}${COLOR_RESET} $*" >&2; }
    log_secondary() { echo "${COLOR_BLUE}${CHECK_MARK}${COLOR_RESET} $*" >&2; }

    export -f log_error log_warning log_info log_success log_secondary
  fi

  # Ensure show_command_header exists
  if ! type -t show_command_header >/dev/null 2>&1; then
    show_command_header() {
      echo ""
      echo "${COLOR_BOLD}$1${COLOR_RESET}"
      echo "$2"
      echo ""
    }

    export -f show_command_header
  fi

  return 0
}

# Safe cross-platform echo with color support
# Inputs: $1 - text to output (with color codes)
# Outputs: Formatted text
# Returns: 0
safe_echo() {
  local text="$1"

  # Use printf for portability - it's POSIX and works everywhere
  # The %b flag interprets backslash escapes
  printf '%b\n' "$text"
}

# Safe echo without newline
# Inputs: $1 - text to output
# Outputs: Formatted text without newline
# Returns: 0
safe_echo_n() {
  local text="$1"
  printf '%b' "$text"
}

# Initialize all platform and display settings
# Inputs: None
# Outputs: Sets all platform, terminal, and display variables
# Returns: 0
init_platform() {
  detect_platform
  detect_terminal
  setup_colors
  setup_display_chars
  setup_display_functions
  return 0
}

# Get terminal width
# Inputs: None
# Outputs: Terminal width as integer
# Returns: 0 on success, 1 on failure
get_terminal_width() {
  local width=80 # Default

  if command -v tput >/dev/null 2>&1; then
    # Ensure TERM is set for tput
    TERM="${TERM:-xterm}" width=$(tput cols 2>/dev/null) || width=80
  elif [[ -n "${COLUMNS:-}" ]]; then
    width="$COLUMNS"
  fi

  echo "$width"
  return 0
}

# Check if terminal meets minimum width requirement
# Inputs: $1 - minimum width (optional, defaults to INIT_MIN_TERM_WIDTH)
# Outputs: None
# Returns: 0 if meets requirement, 1 if not
check_terminal_width() {
  local min_width="${1:-${INIT_MIN_TERM_WIDTH:-40}}"
  local current_width
  current_width=$(get_terminal_width)

  if [[ $current_width -ge $min_width ]]; then
    return 0
  else
    return 1
  fi
}

# Export functions for use in other scripts
export -f detect_platform detect_terminal setup_colors setup_display_chars
export -f setup_display_functions init_platform get_terminal_width
export -f check_terminal_width safe_echo safe_echo_n
