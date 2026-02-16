#!/usr/bin/env bash

# cli-output.sh - Standardized CLI output library for nself
# Cross-platform compatible (Bash 3.2+), POSIX-compliant where possible
#
# DESIGN PRINCIPLES:
# - Uses printf exclusively (not echo with -e flag) for portability
# - Supports NO_COLOR environment variable
# - Works in both interactive terminals and CI/non-TTY environments
# - Consistent spacing, alignment, and visual hierarchy
# - Clean, predictable API

# Prevent double-sourcing
[[ "${CLI_OUTPUT_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export CLI_OUTPUT_SOURCED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_CLI_OUTPUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_CLI_OUTPUT_DIR}/platform-compat.sh" 2>/dev/null || true

# =============================================================================
# COLOR AND FORMATTING CODES
# =============================================================================

# ANSI color codes (using ANSI-C quoting for portability)
if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  # Terminal supports colors
  CLI_RESET=$'\033[0m'
  CLI_BOLD=$'\033[1m'
  CLI_DIM=$'\033[2m'
  CLI_UNDERLINE=$'\033[4m'

  # Standard colors
  CLI_BLACK=$'\033[0;30m'
  CLI_RED=$'\033[0;31m'
  CLI_GREEN=$'\033[0;32m'
  CLI_YELLOW=$'\033[0;33m'
  CLI_BLUE=$'\033[0;34m'
  CLI_MAGENTA=$'\033[0;35m'
  CLI_CYAN=$'\033[0;36m'
  CLI_WHITE=$'\033[0;37m'

  # Bright colors
  CLI_BRIGHT_RED=$'\033[1;31m'
  CLI_BRIGHT_GREEN=$'\033[1;32m'
  CLI_BRIGHT_YELLOW=$'\033[1;33m'
  CLI_BRIGHT_BLUE=$'\033[1;34m'
  CLI_BRIGHT_MAGENTA=$'\033[1;35m'
  CLI_BRIGHT_CYAN=$'\033[1;36m'
  CLI_BRIGHT_WHITE=$'\033[1;97m'

  # Background colors (for highlights)
  CLI_BG_RED=$'\033[41m'
  CLI_BG_GREEN=$'\033[42m'
  CLI_BG_YELLOW=$'\033[43m'
  CLI_BG_BLUE=$'\033[44m'
else
  # NO_COLOR set or not a terminal - disable all colors
  CLI_RESET=""
  CLI_BOLD=""
  CLI_DIM=""
  CLI_UNDERLINE=""
  CLI_BLACK=""
  CLI_RED=""
  CLI_GREEN=""
  CLI_YELLOW=""
  CLI_BLUE=""
  CLI_MAGENTA=""
  CLI_CYAN=""
  CLI_WHITE=""
  CLI_BRIGHT_RED=""
  CLI_BRIGHT_GREEN=""
  CLI_BRIGHT_YELLOW=""
  CLI_BRIGHT_BLUE=""
  CLI_BRIGHT_MAGENTA=""
  CLI_BRIGHT_CYAN=""
  CLI_BRIGHT_WHITE=""
  CLI_BG_RED=""
  CLI_BG_GREEN=""
  CLI_BG_YELLOW=""
  CLI_BG_BLUE=""
fi

# =============================================================================
# UNICODE SYMBOLS AND BOX DRAWING CHARACTERS
# =============================================================================

# Icons for message types
CLI_ICON_SUCCESS="✓"
CLI_ICON_ERROR="✗"
CLI_ICON_WARNING="⚠"
CLI_ICON_INFO="ℹ"
CLI_ICON_ARROW="→"
CLI_ICON_BULLET="•"
CLI_ICON_CHECK="✓"
CLI_ICON_CROSS="✗"
CLI_ICON_STAR="★"
CLI_ICON_GEAR="⚙"
CLI_ICON_ROCKET="🚀"
CLI_ICON_PACKAGE="📦"
CLI_ICON_FIRE="🔥"
CLI_ICON_SPARKLES="✨"

# Box drawing characters (single-line)
CLI_BOX_HORIZONTAL="─"
CLI_BOX_VERTICAL="│"
CLI_BOX_TOP_LEFT="┌"
CLI_BOX_TOP_RIGHT="┐"
CLI_BOX_BOTTOM_LEFT="└"
CLI_BOX_BOTTOM_RIGHT="┘"
CLI_BOX_CROSS="┼"
CLI_BOX_T_DOWN="┬"
CLI_BOX_T_UP="┴"
CLI_BOX_T_RIGHT="├"
CLI_BOX_T_LEFT="┤"

# Box drawing characters (double-line for emphasis)
CLI_BOX_DOUBLE_HORIZONTAL="═"
CLI_BOX_DOUBLE_VERTICAL="║"
CLI_BOX_DOUBLE_TOP_LEFT="╔"
CLI_BOX_DOUBLE_TOP_RIGHT="╗"
CLI_BOX_DOUBLE_BOTTOM_LEFT="╚"
CLI_BOX_DOUBLE_BOTTOM_RIGHT="╝"

# Progress bar characters
CLI_PROGRESS_FILLED="█"
CLI_PROGRESS_PARTIAL="▓"
CLI_PROGRESS_EMPTY="░"

# Spinner frames for animations
CLI_SPINNER_FRAMES=("⠋" "⠙" "⠹" "⠸" "⠼" "⠴" "⠦" "⠧" "⠇" "⠏")

# =============================================================================
# BASIC MESSAGE OUTPUT FUNCTIONS
# =============================================================================

# Print a success message with checkmark icon
# Usage: cli_success "Operation completed successfully"
cli_success() {
  local message="$1"
  printf "%b%s%b %s\n" "${CLI_GREEN}" "${CLI_ICON_SUCCESS}" "${CLI_RESET}" "${message}"
}

# Print an error message with cross icon
# Usage: cli_error "Failed to connect to database"
cli_error() {
  local message="$1"
  printf "%b%s%b %s\n" "${CLI_RED}" "${CLI_ICON_ERROR}" "${CLI_RESET}" "${message}" >&2
}

# Print a warning message with warning icon
# Usage: cli_warning "Port 8080 is already in use"
cli_warning() {
  local message="$1"
  printf "%b%s%b %s\n" "${CLI_YELLOW}" "${CLI_ICON_WARNING}" "${CLI_RESET}" "${message}" >&2
}

# Print an info message with info icon
# Usage: cli_info "Loading configuration..."
cli_info() {
  local message="$1"
  printf "%b%s%b %s\n" "${CLI_BLUE}" "${CLI_ICON_INFO}" "${CLI_RESET}" "${message}"
}

# Print a debug message (only shown when DEBUG=true)
# Usage: cli_debug "Variable value: $var"
cli_debug() {
  local message="$1"
  if [[ "${DEBUG:-false}" == "true" ]]; then
    printf "%b[DEBUG]%b %s\n" "${CLI_MAGENTA}" "${CLI_RESET}" "${message}"
  fi
}

# Print a plain message without icons
# Usage: cli_message "Hello, world"
cli_message() {
  local message="$1"
  printf "%s\n" "${message}"
}

# Print a bold message
# Usage: cli_bold "Important announcement"
cli_bold() {
  local message="$1"
  printf "%b%s%b\n" "${CLI_BOLD}" "${message}" "${CLI_RESET}"
}

# Print a dimmed/subtle message
# Usage: cli_dim "Additional context information"
cli_dim() {
  local message="$1"
  printf "%b%s%b\n" "${CLI_DIM}" "${message}" "${CLI_RESET}"
}

# =============================================================================
# SECTION AND HEADER FUNCTIONS
# =============================================================================

# Print a section header with arrow
# Usage: cli_section "Database Configuration"
cli_section() {
  local title="$1"
  printf "\n%b%s%b %b%s%b\n" "${CLI_BLUE}" "${CLI_ICON_ARROW}" "${CLI_RESET}" "${CLI_BOLD}" "${title}" "${CLI_RESET}"
}

# Print a major section header with box
# Usage: cli_header "Build Process"
cli_header() {
  local title="$1"
  local width=60
  local title_len=${#title}
  local padding=$(((width - title_len - 2) / 2))
  local right_padding=$((width - title_len - padding - 2))

  printf "\n"
  printf "%b%s%b\n" "${CLI_BLUE}" "$(printf '%*s' "$width" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_RESET}"
  printf "%b%*s%s%*s%b\n" "${CLI_BOLD}" "$padding" "" "${title}" "$right_padding" "" "${CLI_RESET}"
  printf "%b%s%b\n" "${CLI_BLUE}" "$(printf '%*s' "$width" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_RESET}"
  printf "\n"
}

# Print a subheader (smaller, dimmed text under main header)
# Usage: cli_subheader "Description text"
cli_subheader() {
  local text="$1"
  printf "%b%s%b\n" "${CLI_DIM}" "${text}" "${CLI_RESET}"
}

# Print a step indicator (e.g., "Step 1/5")
# Usage: cli_step 1 5 "Installing dependencies"
cli_step() {
  local current="$1"
  local total="$2"
  local message="$3"

  printf "\n%b%s%b %bStep %d/%d%b %s %s\n" \
    "${CLI_BLUE}" "${CLI_ICON_GEAR}" "${CLI_RESET}" \
    "${CLI_BOLD}" "$current" "$total" "${CLI_RESET}" \
    "${CLI_BOX_HORIZONTAL}" "${message}"
}

# =============================================================================
# BOX DRAWING FUNCTIONS
# =============================================================================

# Draw a simple box around text
# Usage: cli_box "Message to display" [type]
# Types: info (default), success, error, warning
cli_box() {
  local message="$1"
  local type="${2:-info}"
  local width=$((${#message} + 4))
  local color="${CLI_BLUE}"

  case "$type" in
    success) color="${CLI_GREEN}" ;;
    error) color="${CLI_RED}" ;;
    warning) color="${CLI_YELLOW}" ;;
  esac

  printf "\n"
  printf "%b%s%s%s%b\n" "${color}" "${CLI_BOX_TOP_LEFT}" "$(printf '%*s' "$width" | tr ' ' "${CLI_BOX_HORIZONTAL}")" "${CLI_BOX_TOP_RIGHT}" "${CLI_RESET}"
  printf "%b%s%b  %s  %b%s%b\n" "${color}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}" "${message}" "${color}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}"
  printf "%b%s%s%s%b\n" "${color}" "${CLI_BOX_BOTTOM_LEFT}" "$(printf '%*s' "$width" | tr ' ' "${CLI_BOX_HORIZONTAL}")" "${CLI_BOX_BOTTOM_RIGHT}" "${CLI_RESET}"
  printf "\n"
}

# Draw an enhanced box with title and content
# Usage: cli_box_detailed "Title" "Content text here"
cli_box_detailed() {
  local title="$1"
  local content="$2"
  local width=60
  local content_width=$((width - 4))

  printf "\n"
  # Top border
  printf "%b%s%s%s%b\n" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_TOP_LEFT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_BOX_DOUBLE_TOP_RIGHT}" "${CLI_RESET}"

  # Title
  local title_len=${#title}
  local title_padding=$(((content_width - title_len) / 2))
  local title_right_padding=$((content_width - title_len - title_padding))
  printf "%b%s%b %*s%b%s%b%*s %b%s%b\n" \
    "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}" \
    "$title_padding" "" \
    "${CLI_BOLD}" "${title}" "${CLI_RESET}" \
    "$title_right_padding" "" \
    "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}"

  # Separator
  printf "%b%s%s%s%b\n" "${CLI_BLUE}" "${CLI_BOX_T_RIGHT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_HORIZONTAL}")" "${CLI_BOX_T_LEFT}" "${CLI_RESET}"

  # Content (word wrap at content_width)
  local words=($content)
  local line=""
  for word in "${words[@]}"; do
    if [[ -z "$line" ]]; then
      line="$word"
    elif [[ $((${#line} + ${#word} + 1)) -le $content_width ]]; then
      line="$line $word"
    else
      # Print current line
      printf "%b%s%b %-*s %b%s%b\n" \
        "${CLI_BLUE}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}" \
        "$content_width" "$line" \
        "${CLI_BLUE}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}"
      line="$word"
    fi
  done

  # Print last line if any
  if [[ -n "$line" ]]; then
    printf "%b%s%b %-*s %b%s%b\n" \
      "${CLI_BLUE}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}" \
      "$content_width" "$line" \
      "${CLI_BLUE}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}"
  fi

  # Bottom border
  printf "%b%s%s%s%b\n" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_BOTTOM_LEFT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_BOX_DOUBLE_BOTTOM_RIGHT}" "${CLI_RESET}"
  printf "\n"
}

# =============================================================================
# TABLE FUNCTIONS
# =============================================================================

# Internal: Calculate column widths
_cli_calc_column_widths() {
  local -a headers=("$@")
  local -a widths=()

  for header in "${headers[@]}"; do
    widths+=("${#header}")
  done

  # Return widths as space-separated string
  printf "%s " "${widths[@]}"
}

# Print table header
# Usage: cli_table_header "Column1" "Column2" "Column3"
cli_table_header() {
  local -a headers=("$@")
  local -a widths

  # Calculate column widths
  read -ra widths <<<"$(_cli_calc_column_widths "${headers[@]}")"

  # Top border
  printf "%s" "${CLI_BOX_TOP_LEFT}"
  for i in "${!headers[@]}"; do
    printf "%s" "$(printf '%*s' "$((widths[i] + 2))" | tr ' ' "${CLI_BOX_HORIZONTAL}")"
    if [[ $i -lt $((${#headers[@]} - 1)) ]]; then
      printf "%s" "${CLI_BOX_T_DOWN}"
    fi
  done
  printf "%s\n" "${CLI_BOX_TOP_RIGHT}"

  # Header row
  printf "%s" "${CLI_BOX_VERTICAL}"
  for i in "${!headers[@]}"; do
    printf " %b%-*s%b " "${CLI_BOLD}" "${widths[$i]}" "${headers[$i]}" "${CLI_RESET}"
    printf "%s" "${CLI_BOX_VERTICAL}"
  done
  printf "\n"

  # Separator
  printf "%s" "${CLI_BOX_T_RIGHT}"
  for i in "${!headers[@]}"; do
    printf "%s" "$(printf '%*s' "$((widths[i] + 2))" | tr ' ' "${CLI_BOX_HORIZONTAL}")"
    if [[ $i -lt $((${#headers[@]} - 1)) ]]; then
      printf "%s" "${CLI_BOX_CROSS}"
    fi
  done
  printf "%s\n" "${CLI_BOX_T_LEFT}"

  # Store widths for cli_table_row to use
  export CLI_TABLE_WIDTHS="${widths[*]}"
}

# Print table row
# Usage: cli_table_row "Value1" "Value2" "Value3"
cli_table_row() {
  local -a values=("$@")
  local -a widths

  # Use stored widths from header
  read -ra widths <<<"${CLI_TABLE_WIDTHS:-}"

  printf "%s" "${CLI_BOX_VERTICAL}"
  for i in "${!values[@]}"; do
    local width="${widths[$i]:-20}"
    printf " %-*s " "$width" "${values[$i]}"
    printf "%s" "${CLI_BOX_VERTICAL}"
  done
  printf "\n"
}

# Print table footer
# Usage: cli_table_footer "Column1" "Column2" "Column3"
cli_table_footer() {
  local -a headers=("$@")
  local -a widths

  # Use stored widths from header
  read -ra widths <<<"${CLI_TABLE_WIDTHS:-}"

  # Bottom border
  printf "%s" "${CLI_BOX_BOTTOM_LEFT}"
  for i in "${!headers[@]}"; do
    local width="${widths[$i]:-20}"
    printf "%s" "$(printf '%*s' "$((width + 2))" | tr ' ' "${CLI_BOX_HORIZONTAL}")"
    if [[ $i -lt $((${#headers[@]} - 1)) ]]; then
      printf "%s" "${CLI_BOX_T_UP}"
    fi
  done
  printf "%s\n" "${CLI_BOX_BOTTOM_RIGHT}"

  # Clear stored widths
  unset CLI_TABLE_WIDTHS
}

# =============================================================================
# LIST FUNCTIONS
# =============================================================================

# Print a bullet list item
# Usage: cli_list_item "Item text"
cli_list_item() {
  local item="$1"
  printf "  %b%s%b %s\n" "${CLI_BLUE}" "${CLI_ICON_BULLET}" "${CLI_RESET}" "${item}"
}

# Print a numbered list item
# Usage: cli_list_numbered 1 "First item"
cli_list_numbered() {
  local number="$1"
  local item="$2"
  printf "  %b%d.%b %s\n" "${CLI_BOLD}" "$number" "${CLI_RESET}" "${item}"
}

# Print a checklist item (checked)
# Usage: cli_list_checked "Completed task"
cli_list_checked() {
  local item="$1"
  printf "  %b[%s]%b %s\n" "${CLI_GREEN}" "${CLI_ICON_CHECK}" "${CLI_RESET}" "${item}"
}

# Print a checklist item (unchecked)
# Usage: cli_list_unchecked "Pending task"
cli_list_unchecked() {
  local item="$1"
  printf "  %b[ ]%b %s\n" "${CLI_DIM}" "${CLI_RESET}" "${item}"
}

# =============================================================================
# PROGRESS AND LOADING INDICATORS
# =============================================================================

# Show a progress bar
# Usage: cli_progress "Building project" 45 100
cli_progress() {
  local task="$1"
  local current="$2"
  local total="$3"
  local width=40

  local percent=$((current * 100 / total))
  local filled=$((current * width / total))
  local empty=$((width - filled))

  # Only use \r (carriage return) in interactive terminals
  if [[ -t 1 ]]; then
    printf "\r"
  fi

  printf "%b%s%b %s [" "${CLI_BLUE}" "${CLI_ICON_GEAR}" "${CLI_RESET}" "${task}"

  # Filled portion
  local i
  for ((i = 0; i < filled; i++)); do
    printf "%b%s%b" "${CLI_GREEN}" "${CLI_PROGRESS_FILLED}" "${CLI_RESET}"
  done

  # Empty portion
  for ((i = 0; i < empty; i++)); do
    printf "%s" "${CLI_PROGRESS_EMPTY}"
  done

  printf "] %b%3d%%%b" "${CLI_BOLD}" "$percent" "${CLI_RESET}"

  # Newline when complete
  if [[ $current -eq $total ]]; then
    printf " %b%s%b\n" "${CLI_GREEN}" "${CLI_ICON_SUCCESS}" "${CLI_RESET}"
  fi
}

# Show a spinner with message (backgrounds a process)
# Usage: cli_spinner_start "Loading data..."
# Returns: PID of spinner process (store and use with cli_spinner_stop)
cli_spinner_start() {
  local message="$1"

  # Only show spinner in interactive terminals
  if [[ ! -t 1 ]]; then
    printf "%s...\n" "${message}"
    echo "0"
    return
  fi

  (
    local frame=0
    while true; do
      printf "\r%b%s%b %s" \
        "${CLI_BLUE}" "${CLI_SPINNER_FRAMES[$frame]}" "${CLI_RESET}" "${message}"
      frame=$(((frame + 1) % ${#CLI_SPINNER_FRAMES[@]}))
      sleep 0.1
    done
  ) &

  echo $!
}

# Stop a running spinner
# Usage: cli_spinner_stop $SPINNER_PID "Complete message"
cli_spinner_stop() {
  local pid="$1"
  local message="${2:-Done}"

  if [[ "$pid" != "0" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi

  if [[ -t 1 ]]; then
    printf "\r\033[K"
  fi

  cli_success "${message}"
}

# =============================================================================
# SPECIAL OUTPUT FUNCTIONS
# =============================================================================

# Print a summary box with multiple items
# Usage: cli_summary "Build Complete" "Item 1" "Item 2" "Item 3"
cli_summary() {
  local title="$1"
  shift
  local -a items=("$@")
  local width=60

  printf "\n"
  # Top border
  printf "%b%s%s%s%b\n" "${CLI_GREEN}" "${CLI_BOX_DOUBLE_TOP_LEFT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_BOX_DOUBLE_TOP_RIGHT}" "${CLI_RESET}"

  # Title
  local title_text="${CLI_ICON_STAR} ${title} ${CLI_ICON_STAR}"
  local title_len=$((${#title} + 4)) # Account for icons
  local title_padding=$(((width - title_len - 2) / 2))
  local title_right_padding=$((width - title_len - title_padding - 2))
  printf "%b%s%b %*s%b%s %s %s%b%*s %b%s%b\n" \
    "${CLI_GREEN}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}" \
    "$title_padding" "" \
    "${CLI_BOLD}" "${CLI_ICON_STAR}" "${title}" "${CLI_ICON_STAR}" "${CLI_RESET}" \
    "$title_right_padding" "" \
    "${CLI_GREEN}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}"

  # Separator
  printf "%b%s%s%s%b\n" "${CLI_GREEN}" "${CLI_BOX_T_RIGHT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_HORIZONTAL}")" "${CLI_BOX_T_LEFT}" "${CLI_RESET}"

  # Items
  for item in "${items[@]}"; do
    printf "%b%s%b  %b%s%b %-*s %b%s%b\n" \
      "${CLI_GREEN}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}" \
      "${CLI_BLUE}" "${CLI_ICON_BULLET}" "${CLI_RESET}" \
      "$((width - 6))" "${item}" \
      "${CLI_GREEN}" "${CLI_BOX_VERTICAL}" "${CLI_RESET}"
  done

  # Bottom border
  printf "%b%s%s%s%b\n" "${CLI_GREEN}" "${CLI_BOX_DOUBLE_BOTTOM_LEFT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_BOX_DOUBLE_BOTTOM_RIGHT}" "${CLI_RESET}"
  printf "\n"
}

# Print a banner for major events
# Usage: cli_banner "nself v1.0.0" "Modern Full-Stack Platform"
cli_banner() {
  local title="$1"
  local subtitle="${2:-}"
  local width=60

  printf "\n"
  printf "%b%s%s%s%b\n" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_TOP_LEFT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_BOX_DOUBLE_TOP_RIGHT}" "${CLI_RESET}"
  printf "%b%s%b %*s %b%s%b\n" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}" "$((width - 2))" "" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}"

  # Title (centered)
  local title_len=${#title}
  local title_padding=$(((width - title_len - 2) / 2))
  local title_right_padding=$((width - title_len - title_padding - 2))
  printf "%b%s%b %*s%b%s%b%*s %b%s%b\n" \
    "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}" \
    "$title_padding" "" \
    "${CLI_BOLD}" "${title}" "${CLI_RESET}" \
    "$title_right_padding" "" \
    "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}"

  if [[ -n "$subtitle" ]]; then
    # Subtitle (centered, dimmed)
    local subtitle_len=${#subtitle}
    local subtitle_padding=$(((width - subtitle_len - 2) / 2))
    local subtitle_right_padding=$((width - subtitle_len - subtitle_padding - 2))
    printf "%b%s%b %*s%b%s%b%*s %b%s%b\n" \
      "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}" \
      "$subtitle_padding" "" \
      "${CLI_DIM}" "${subtitle}" "${CLI_RESET}" \
      "$subtitle_right_padding" "" \
      "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}"
  fi

  printf "%b%s%b %*s %b%s%b\n" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}" "$((width - 2))" "" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_VERTICAL}" "${CLI_RESET}"
  printf "%b%s%s%s%b\n" "${CLI_BLUE}" "${CLI_BOX_DOUBLE_BOTTOM_LEFT}" "$(printf '%*s' "$((width - 2))" | tr ' ' "${CLI_BOX_DOUBLE_HORIZONTAL}")" "${CLI_BOX_DOUBLE_BOTTOM_RIGHT}" "${CLI_RESET}"
  printf "\n"
}

# Print a horizontal separator/divider
# Usage: cli_separator [width]
cli_separator() {
  local width="${1:-60}"
  printf "%b%s%b\n" "${CLI_DIM}" "$(printf '%*s' "$width" | tr ' ' "${CLI_BOX_HORIZONTAL}")" "${CLI_RESET}"
}

# =============================================================================
# UTILITY FUNCTIONS
# =============================================================================

# Strip all ANSI color codes from text (useful for logging to files)
# Usage: echo "colored text" | cli_strip_colors
cli_strip_colors() {
  sed 's/\x1b\[[0-9;]*m//g'
}

# Print blank line(s)
# Usage: cli_blank [count]
cli_blank() {
  local count="${1:-1}"
  local i
  for ((i = 0; i < count; i++)); do
    printf "\n"
  done
}

# Center text within a given width
# Usage: cli_center "text" 60
cli_center() {
  local text="$1"
  local width="${2:-60}"
  local text_len=${#text}

  if [[ $text_len -ge $width ]]; then
    printf "%s\n" "${text:0:$width}"
  else
    local padding=$(((width - text_len) / 2))
    local right_padding=$((width - text_len - padding))
    printf "%*s%s%*s\n" "$padding" "" "$text" "$right_padding" ""
  fi
}

# Print an indented message
# Usage: cli_indent "message" [level]
cli_indent() {
  local message="$1"
  local level="${2:-1}"
  local indent=$((level * 2))
  printf "%*s%s\n" "$indent" "" "$message"
}

# =============================================================================
# EXPORT ALL FUNCTIONS
# =============================================================================

export -f cli_success cli_error cli_warning cli_info cli_debug
export -f cli_message cli_bold cli_dim
export -f cli_section cli_header cli_step
export -f cli_box cli_box_detailed
export -f cli_table_header cli_table_row cli_table_footer
export -f cli_list_item cli_list_numbered cli_list_checked cli_list_unchecked
export -f cli_progress cli_spinner_start cli_spinner_stop
export -f cli_summary cli_banner cli_separator
export -f cli_strip_colors cli_blank cli_center cli_indent

# Export constants for external use
export CLI_RESET CLI_BOLD CLI_DIM CLI_UNDERLINE
export CLI_RED CLI_GREEN CLI_YELLOW CLI_BLUE CLI_MAGENTA CLI_CYAN CLI_WHITE
export CLI_BRIGHT_RED CLI_BRIGHT_GREEN CLI_BRIGHT_YELLOW CLI_BRIGHT_BLUE
export CLI_ICON_SUCCESS CLI_ICON_ERROR CLI_ICON_WARNING CLI_ICON_INFO
export CLI_ICON_ARROW CLI_ICON_BULLET CLI_ICON_CHECK CLI_ICON_CROSS
export CLI_BOX_HORIZONTAL CLI_BOX_VERTICAL
export CLI_BOX_TOP_LEFT CLI_BOX_TOP_RIGHT CLI_BOX_BOTTOM_LEFT CLI_BOX_BOTTOM_RIGHT

# Mark as loaded
export CLI_OUTPUT_LOADED=1
