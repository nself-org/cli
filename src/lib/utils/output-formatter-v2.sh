#!/bin/bash


# Enhanced output formatter with professional styling

# Namespaced to avoid clobbering caller's SCRIPT_DIR
_OUTPUT_FMT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

source "${_OUTPUT_FMT_DIR}/display.sh"

# Unicode box drawing characters
BOX_TOP_LEFT="╭"
BOX_TOP_RIGHT="╮"
BOX_BOTTOM_LEFT="╰"
BOX_BOTTOM_RIGHT="╯"
BOX_HORIZONTAL="─"
BOX_VERTICAL="│"
BOX_CROSS="┼"
BOX_T_DOWN="┬"
BOX_T_UP="┴"
BOX_T_RIGHT="├"
BOX_T_LEFT="┤"

# Icons for different message types
ICON_SUCCESS="✅"
ICON_ERROR="❌"
ICON_WARNING="⚠️"
ICON_INFO="ℹ️"
ICON_CHECK="✓"
ICON_CROSS="✗"
ICON_ARROW="→"
ICON_BULLET="•"
ICON_LOADING="⏳"
ICON_ROCKET="🚀"
ICON_GEAR="⚙️"
ICON_PACKAGE="📦"
ICON_LOCK="🔒"
ICON_KEY="🔑"
ICON_FOLDER="📁"
ICON_FILE="📄"
ICON_STAR="⭐"
ICON_FIRE="🔥"
ICON_LIGHTBULB="💡"
ICON_SPARKLES="✨"

# Animation frames
SPINNER_FRAMES=("⠋" "⠙" "⠹" "⠸" "⠼" "⠴" "⠦" "⠧" "⠇" "⠏")
PROGRESS_FRAMES=("░" "▒" "▓" "█")

# Enhanced section formatting
format_section() {
  local title="$1"
  local width="${2:-60}"

  echo
  printf "${BLUE}${BOX_TOP_LEFT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"

  # Center the title
  local title_len=${#title}
  local padding=$(((width - title_len - 4) / 2))
  local left_pad=$(printf '%*s' $padding)
  local right_pad=$(printf '%*s' $((width - title_len - padding - 4)))

  printf "${BLUE}${BOX_VERTICAL}${RESET} ${left_pad}${BOLD}${title}${RESET}${right_pad} ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_BOTTOM_LEFT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  echo
}

# Professional step indicator
format_step() {
  local current="$1"
  local total="$2"
  local message="$3"

  echo
  printf "${BLUE}${ICON_GEAR}${RESET} ${BOLD}Step ${current}/${total}${RESET} ${BOX_HORIZONTAL} ${message}\n"
}

# Enhanced success message
format_success() {
  local message="$1"
  printf "${GREEN}${ICON_CHECK}${RESET} ${GREEN}${message}${RESET}\n"
}

# Enhanced error message
format_error() {
  local error="$1"
  local suggestion="${2:-}"

  echo
  printf "${RED}${BOX_TOP_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"
  printf "${RED}${BOX_VERTICAL}${RESET} ${RED}${ICON_CROSS} ERROR${RESET}$(printf '%*s' 50)${RED}${BOX_VERTICAL}${RESET}\n"
  printf "${RED}${BOX_T_RIGHT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_T_LEFT}${RESET}"
  printf "${RED}${BOX_VERTICAL}${RESET} ${error}$(printf '%*s' $((57 - ${#error})))${RED}${BOX_VERTICAL}${RESET}\n"

  if [[ -n "$suggestion" ]]; then
    printf "${RED}${BOX_T_RIGHT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_T_LEFT}${RESET}"
    printf "${RED}${BOX_VERTICAL}${RESET} ${YELLOW}${ICON_LIGHTBULB}${RESET} ${DIM}${suggestion}${RESET}$(printf '%*s' $((53 - ${#suggestion})))${RED}${BOX_VERTICAL}${RESET}\n"
  fi

  printf "${RED}${BOX_BOTTOM_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  echo
}

# Enhanced warning message
format_warning() {
  local warning="$1"
  local suggestion="${2:-}"

  echo
  printf "${YELLOW}${ICON_WARNING} WARNING${RESET}\n"
  printf "${YELLOW}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${BOX_HORIZONTAL}${RESET}\n"
  printf "${warning}\n"

  if [[ -n "$suggestion" ]]; then
    printf "${DIM}${ICON_ARROW} ${suggestion}${RESET}\n"
  fi
}

# Enhanced info message
format_info() {
  local message="$1"
  printf "${BLUE}${ICON_INFO}${RESET} ${message}\n"
}

# Beautiful progress bar
show_progress() {
  local current=$1
  local total=$2
  local message="${3:-Processing}"
  local width=40

  local percent=$((current * 100 / total))
  local filled=$((current * width / total))
  local empty=$((width - filled))

  printf "\r${BLUE}${ICON_LOADING}${RESET} ${message} ["

  # Filled portion with gradient effect
  for ((i = 0; i < filled; i++)); do
    if [[ $i -lt $((filled - 1)) ]]; then
      printf "${GREEN}█${RESET}"
    else
      printf "${GREEN}▓${RESET}"
    fi
  done

  # Empty portion
  printf "%${empty}s" | tr ' ' '░'

  printf "] ${BOLD}%3d%%${RESET}" "$percent"

  if [[ $current -eq $total ]]; then
    printf " ${GREEN}${ICON_CHECK}${RESET}\n"
  fi
}

# Animated spinner with message
show_spinner() {
  local message="$1"
  local pid=$2
  local frame=0

  while kill -0 $pid 2>/dev/null; do
    printf "\r${BLUE}${SPINNER_FRAMES[frame]}${RESET} ${message}"
    frame=$(((frame + 1) % ${#SPINNER_FRAMES[@]}))
    sleep 0.1
  done

  printf "\r${GREEN}${ICON_CHECK}${RESET} ${message}\n"
}

# Enhanced summary box
format_summary() {
  local title="$1"
  shift
  local items=("$@")
  local width=60

  echo
  printf "${GREEN}${BOX_TOP_LEFT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"

  # Title
  local title_with_icon="${ICON_STAR} ${title} ${ICON_STAR}"
  local title_len=${#title_with_icon}
  local padding=$(((width - title_len - 2) / 2))
  printf "${GREEN}${BOX_VERTICAL}${RESET}%*s${BOLD}%s${RESET}%*s${GREEN}${BOX_VERTICAL}${RESET}\n" \
    $padding "" "$title_with_icon" $((width - title_len - padding - 2)) ""

  printf "${GREEN}${BOX_T_RIGHT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_T_LEFT}${RESET}"

  # Items
  for item in "${items[@]}"; do
    printf "${GREEN}${BOX_VERTICAL}${RESET}  ${ICON_BULLET} %-*s${GREEN}${BOX_VERTICAL}${RESET}\n" $((width - 6)) "$item"
  done

  printf "${GREEN}${BOX_BOTTOM_LEFT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  echo
}

# Validation output formatter
format_validation_results() {
  local errors=("$@")
  local warnings=()
  local fixes=()

  # Separate arrays based on markers
  local mode="errors"
  for item in "$@"; do
    case "$item" in
      "WARNINGS:")
        mode="warnings"
        continue
        ;;
      "FIXES:")
        mode="fixes"
        continue
        ;;
    esac

    case "$mode" in
      errors) [[ "$item" != "WARNINGS:" ]] && [[ "$item" != "FIXES:" ]] && errors+=("$item") ;;
      warnings) [[ "$item" != "FIXES:" ]] && warnings+=("$item") ;;
      fixes) fixes+=("$item") ;;
    esac
  done

  # Display errors if any
  if [[ ${#errors[@]} -gt 0 ]]; then
    echo
    printf "${RED}${BOX_TOP_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"
    printf "${RED}${BOX_VERTICAL}${RESET} ${RED}${ICON_CROSS} Validation Errors${RESET}$(printf '%*s' 38)${RED}${BOX_VERTICAL}${RESET}\n"
    printf "${RED}${BOX_T_RIGHT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_T_LEFT}${RESET}"

    for error in "${errors[@]}"; do
      [[ -n "$error" ]] && [[ "$error" != "WARNINGS:" ]] && [[ "$error" != "FIXES:" ]] &&
        printf "${RED}${BOX_VERTICAL}${RESET}  ${RED}${ICON_CROSS}${RESET} %-*s${RED}${BOX_VERTICAL}${RESET}\n" 54 "$error"
    done

    printf "${RED}${BOX_BOTTOM_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  fi

  # Display warnings if any
  if [[ ${#warnings[@]} -gt 0 ]]; then
    echo
    printf "${YELLOW}${BOX_TOP_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"
    printf "${YELLOW}${BOX_VERTICAL}${RESET} ${YELLOW}${ICON_WARNING} Warnings${RESET}$(printf '%*s' 46)${YELLOW}${BOX_VERTICAL}${RESET}\n"
    printf "${YELLOW}${BOX_T_RIGHT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_T_LEFT}${RESET}"

    for warning in "${warnings[@]}"; do
      [[ -n "$warning" ]] && [[ "$warning" != "FIXES:" ]] &&
        printf "${YELLOW}${BOX_VERTICAL}${RESET}  ${YELLOW}${ICON_WARNING}${RESET} %-*s${YELLOW}${BOX_VERTICAL}${RESET}\n" 54 "$warning"
    done

    printf "${YELLOW}${BOX_BOTTOM_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  fi

  # Display available fixes if any
  if [[ ${#fixes[@]} -gt 0 ]]; then
    echo
    printf "${BLUE}${BOX_TOP_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"
    printf "${BLUE}${BOX_VERTICAL}${RESET} ${BLUE}${ICON_GEAR} Auto-Fixes Available${RESET}$(printf '%*s' 35)${BLUE}${BOX_VERTICAL}${RESET}\n"
    printf "${BLUE}${BOX_T_RIGHT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_T_LEFT}${RESET}"

    for fix in "${fixes[@]}"; do
      [[ -n "$fix" ]] &&
        printf "${BLUE}${BOX_VERTICAL}${RESET}  ${BLUE}${ICON_ARROW}${RESET} %-*s${BLUE}${BOX_VERTICAL}${RESET}\n" 54 "$fix"
    done

    printf "${BLUE}${BOX_BOTTOM_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  fi
}

# Docker output formatter
format_docker_output() {
  local line="$1"

  case "$line" in
    *"Pulling from"*)
      printf "${BLUE}${ICON_PACKAGE}${RESET} Downloading Docker images...\n"
      ;;
    *"Pull complete"* | *"Already exists"*)
      printf "${GREEN}.${RESET}"
      ;;
    *"Downloaded newer image"* | *"Image is up to date"*)
      printf " ${GREEN}${ICON_CHECK}${RESET}\n"
      ;;
    *"Creating"*)
      local container=$(echo "$line" | sed 's/.*Creating //' | sed 's/ .*//')
      printf "${BLUE}${ICON_GEAR}${RESET} Creating: ${BOLD}$container${RESET}\n"
      ;;
    *"Started"*)
      local container=$(echo "$line" | sed 's/.*Started //' | sed 's/ .*//')
      printf "${GREEN}${ICON_CHECK}${RESET} Started: ${BOLD}$container${RESET}\n"
      ;;
    *"Error"* | *"ERROR"*)
      printf "${RED}${ICON_CROSS}${RESET} ${RED}$line${RESET}\n"
      ;;
    *"Warning"* | *"WARNING"*)
      printf "${YELLOW}${ICON_WARNING}${RESET} ${YELLOW}$line${RESET}\n"
      ;;
  esac
}

# Service status formatter
format_service_status() {
  local service="$1"
  local status="$2"
  local health="${3:-}"

  local status_icon=""
  local status_color=""

  case "$status" in
    "running")
      status_icon="${ICON_CHECK}"
      status_color="${GREEN}"
      ;;
    "stopped")
      status_icon="${ICON_CROSS}"
      status_color="${RED}"
      ;;
    "starting")
      status_icon="${ICON_LOADING}"
      status_color="${YELLOW}"
      ;;
    *)
      status_icon="${ICON_WARNING}"
      status_color="${YELLOW}"
      ;;
  esac

  printf "${status_color}%s${RESET} %-20s %s" "$status_icon" "$service" "$status"

  if [[ -n "$health" ]]; then
    printf " (Health: %s)" "$health"
  fi

  echo
}

# Welcome banner
show_welcome_banner() {
  echo
  printf "${BLUE}${BOX_TOP_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"
  printf "${BLUE}${BOX_VERTICAL}${RESET}                                                          ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_VERTICAL}${RESET}  ${BOLD}${ICON_ROCKET} Welcome to nself ${ICON_ROCKET}${RESET}                              ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_VERTICAL}${RESET}  ${DIM}Modern Full-Stack Platform${RESET}                           ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_VERTICAL}${RESET}                                                          ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_VERTICAL}${RESET}  ${DIM}Build production-ready applications with ease${RESET}         ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_VERTICAL}${RESET}                                                          ${BLUE}${BOX_VERTICAL}${RESET}\n"
  printf "${BLUE}${BOX_BOTTOM_LEFT}$(printf '%*s' 58 | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  echo
}

# Success banner
show_success_banner() {
  local message="$1"
  local width=60
  local msg_len=${#message}
  local padding=$(((width - msg_len - 4) / 2))

  echo
  printf "${GREEN}${BOX_TOP_LEFT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_TOP_RIGHT}${RESET}"
  printf "${GREEN}${BOX_VERTICAL}${RESET}$(printf '%*s' $((width - 2)))${GREEN}${BOX_VERTICAL}${RESET}\n"
  printf "${GREEN}${BOX_VERTICAL}${RESET}$(printf '%*s' $padding)${GREEN}${ICON_SPARKLES} SUCCESS ${ICON_SPARKLES}${RESET}$(printf '%*s' $((width - padding - 14)))${GREEN}${BOX_VERTICAL}${RESET}\n"
  printf "${GREEN}${BOX_VERTICAL}${RESET}$(printf '%*s' $padding)${message}$(printf '%*s' $((width - padding - msg_len - 2)))${GREEN}${BOX_VERTICAL}${RESET}\n"
  printf "${GREEN}${BOX_VERTICAL}${RESET}$(printf '%*s' $((width - 2)))${GREEN}${BOX_VERTICAL}${RESET}\n"
  printf "${GREEN}${BOX_BOTTOM_LEFT}$(printf '%*s' $((width - 2)) | tr ' ' \n"$BOX_HORIZONTAL")${BOX_BOTTOM_RIGHT}${RESET}"
  echo
}

# Export all functions
export -f format_section
export -f format_step
export -f format_success
export -f format_error
export -f format_warning
export -f format_info
export -f show_progress
export -f show_spinner
export -f format_summary
export -f format_validation_results
export -f format_docker_output
export -f format_service_status
export -f show_welcome_banner
export -f show_success_banner
