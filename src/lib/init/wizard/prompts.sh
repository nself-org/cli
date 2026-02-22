#!/usr/bin/env bash

# prompts.sh - Interactive UI components for wizard

# Get script directory for sourcing utilities
PROMPTS_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Source display utilities for standard headers
source "$PROMPTS_SCRIPT_DIR/../../utils/display.sh"

# Show wizard header using standard command header
show_wizard_header() {
  local title="$1"
  local subtitle="$2"

  show_command_header "$title" "$subtitle"
}

# Show wizard step
show_wizard_step() {
  local current="$1"
  local total="$2"
  local title="$3"

  # Use same width as standard headers (60 chars)
  local step_text="Step $current of $total: $title"
  local padding_needed=$((56 - ${#step_text})) # 56 = content width

  echo
  printf "%s┌──────────────────────────────────────────────────────────┐%s\n" "${COLOR_CYAN}" "${COLOR_RESET}"
  printf "${COLOR_CYAN}│${COLOR_RESET} %s%*s ${COLOR_CYAN}│${COLOR_RESET}\n" "$step_text" $padding_needed ""
  printf "%s└──────────────────────────────────────────────────────────┘%s\n" "${COLOR_CYAN}" "${COLOR_RESET}"
  echo
}

# Single select menu
select_option() {
  local prompt="$1"
  local options_var="$2"
  local result_var="$3"

  # Use eval to handle array references (Bash 3.2 compatible)
  eval "local options=(\"\${${options_var}[@]}\")"

  echo "$prompt"
  echo ""

  local i=1
  for option in "${options[@]}"; do
    echo "  $i) $option"
    i=$((i + 1))
  done

  echo ""
  echo -n "Selection [1]: "
  local choice
  read choice
  choice="${choice:-1}"

  # Validate choice (Bash 3.2 compatible)
  if echo "$choice" | grep -q '^[0-9][0-9]*$' && [ "$choice" -ge 1 ] && [ "$choice" -le "${#options[@]}" ]; then
    eval "${result_var}=$((choice - 1))"
  else
    eval "${result_var}=0"
  fi
}

# Multi-select menu (simplified version)
multi_select() {
  local options_var="$1"
  local selected_var="$2"

  # Use eval to handle array references (Bash 3.2 compatible)
  eval "local options=(\"\${${options_var}[@]}\")"

  echo "(Enter numbers separated by spaces, or 'all' for all, 'none' for none)"
  echo ""

  local i=1
  for option in "${options[@]}"; do
    echo "  $i) $option"
    i=$((i + 1))
  done

  echo ""
  echo -n "Selection [none]: "
  local choices
  read choices
  choices="${choices:-none}"

  local selected_items=()

  if [[ "$choices" == "all" ]]; then
    selected_items=("${options[@]}")
  elif [[ "$choices" != "none" ]]; then
    for choice in $choices; do
      if echo "$choice" | grep -q '^[0-9][0-9]*$' && [ "$choice" -ge 1 ] && [ "$choice" -le "${#options[@]}" ]; then
        selected_items+=("${options[$((choice - 1))]}")
      fi
    done
  fi

  # Assign back to the variable (handle empty array safely)
  if [[ ${#selected_items[@]} -gt 0 ]]; then
    eval "${selected_var}=(\"\${selected_items[@]}\")"
  else
    eval "${selected_var}=()"
  fi
}

# Input prompt with validation
prompt_input() {
  local prompt="$1"
  local default="$2"
  local result_var="$3"
  local pattern="${4:-.*}"

  while true; do
    if [[ -n "$default" ]]; then
      echo -n "$prompt [$default]: "
    else
      echo -n "$prompt: "
    fi

    local input
    read input
    input="${input:-$default}"

    # Validate input (Bash 3.2 compatible)
    if echo "$input" | grep -q "$pattern"; then
      eval "${result_var}=\"\$input\""
      break
    else
      # Provide specific error message for project name
      if [[ "$prompt" == *"Project name"* ]]; then
        echo "Invalid project name. Must:"
        echo "  • Start with a letter or number"
        echo "  • End with a letter or number"
        echo "  • Contain only lowercase letters, numbers, and hyphens"
        echo "  • Examples: myapp, web-app, project1"
      else
        echo "Invalid input. Please try again."
      fi
    fi
  done
}

# Yes/No prompt
prompt_yes_no() {
  local prompt="$1"
  local default="${2:-n}"

  local yn
  if [[ "$default" == "y" ]]; then
    echo -n "$prompt (Y/n): "
  else
    echo -n "$prompt (y/N): "
  fi

  read yn
  yn="${yn:-$default}"

  [[ "$yn" == "y" ]] || [[ "$yn" == "Y" ]]
}

# Progress bar
show_progress() {
  local current="$1"
  local total="$2"
  local width=50

  local percent=$((current * 100 / total))
  local filled=$((current * width / total))

  printf "\r["
  printf "%${filled}s" | tr ' ' '='
  printf "%$((width - filled))s" | tr ' ' '-'
  printf "] %3d%%" "$percent"

  if [[ $current -eq $total ]]; then
    echo ""
  fi
}

# Spinner animation
show_spinner() {
  local pid=$1
  local delay=0.1
  local spinstr='|/-\'

  while kill -0 $pid 2>/dev/null; do
    local temp=${spinstr#?}
    printf " [%c]  " "$spinstr"
    local spinstr=$temp${spinstr%"$temp"}
    sleep $delay
    printf "\b\b\b\b\b\b"
  done
  printf "    \b\b\b\b"
}

# Press any key to continue
press_any_key() {
  echo -n "Press any key to continue..."
  read -n 1 -s
  echo ""
}

# Color output helpers
print_success() {
  printf "\033[32m✓\033[0m %s\n" "$1"
}

print_error() {
  printf "\033[31m✗\033[0m %s\n" "$1"
}

print_warning() {
  printf "\033[33m⚠\033[0m %s\n" "$1"
}

print_info() {
  printf "\033[34mℹ\033[0m %s\n" "$1"
}

# Export all functions
export -f show_wizard_header
export -f show_wizard_step
export -f select_option
export -f multi_select
export -f prompt_input
export -f prompt_yes_no
export -f show_progress
export -f show_spinner
export -f press_any_key
export -f print_success
export -f print_error
export -f print_warning
export -f print_info
