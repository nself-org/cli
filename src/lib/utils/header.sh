#!/usr/bin/env bash

# header.sh - Standardized 60-character header generation

# Constants for header formatting
HEADER_WIDTH=60

set -euo pipefail

CONTENT_WIDTH=56 # 60 - 2 borders - 2 spaces on each side
BORDER_CHAR="═"
CORNER_TL="╔"
CORNER_TR="╗"
CORNER_BL="╚"
CORNER_BR="╝"
SIDE_CHAR="║"

# Generate a centered line with padding
# Args: $1 = text, $2 = width
center_text() {
  local text="$1"
  local width="${2:-$CONTENT_WIDTH}"
  local text_len=${#text}

  if [[ $text_len -ge $width ]]; then
    echo "${text:0:$width}"
  else
    local padding=$((width - text_len))
    local left_pad=$((padding / 2))
    local right_pad=$((padding - left_pad))
    printf "%*s%s%*s" $left_pad "" "$text" $right_pad ""
  fi
}

# Word wrap text to fit within width
# Args: $1 = text, $2 = width
word_wrap() {
  local text="$1"
  local width="${2:-$CONTENT_WIDTH}"
  local result=""
  local current_line=""

  # Split text into words
  for word in $text; do
    local test_line="${current_line}${current_line:+ }${word}"
    if [[ ${#test_line} -le $width ]]; then
      current_line="$test_line"
    else
      if [[ -n "$current_line" ]]; then
        result="${result}${result:+\n}${current_line}"
      fi
      current_line="$word"
    fi
  done

  # Add remaining text
  if [[ -n "$current_line" ]]; then
    result="${result}${result:+\n}${current_line}"
  fi

  printf "%s\n" "$result"
}

# Generate top border
generate_top_border() {
  echo "${CORNER_TL}$(printf '%*s' $((HEADER_WIDTH - 2)) '' | tr ' ' "$BORDER_CHAR")${CORNER_TR}"
}

# Generate bottom border
generate_bottom_border() {
  echo "${CORNER_BL}$(printf '%*s' $((HEADER_WIDTH - 2)) '' | tr ' ' "$BORDER_CHAR")${CORNER_BR}"
}

# Generate content line with borders
# Args: $1 = content (already padded to correct width)
generate_content_line() {
  local content="$1"
  local padded_content

  if [[ -z "$content" ]]; then
    # Empty line - just spaces
    padded_content=$(printf '%*s' $CONTENT_WIDTH '')
  else
    # Ensure content is exactly CONTENT_WIDTH characters
    local content_len=${#content}
    if [[ $content_len -lt $CONTENT_WIDTH ]]; then
      # Pad with spaces on the right
      padded_content=$(printf '%-*s' $CONTENT_WIDTH "$content")
    else
      # Truncate if too long
      padded_content="${content:0:$CONTENT_WIDTH}"
    fi
  fi

  echo "${SIDE_CHAR} ${padded_content} ${SIDE_CHAR}"
}

# Generate a complete header box
# Args: $1 = title, $2 = content (optional)
generate_header() {
  local title="$1"
  local content="$2"
  local prefix="${3:-}" # Optional prefix for comment lines

  local output=""

  # Top border
  output="${prefix}$(generate_top_border)\n"

  # Title line (centered)
  local centered_title=$(center_text "$title" $CONTENT_WIDTH)
  output="${output}${prefix}$(generate_content_line "$centered_title")\n"

  # Content if provided
  if [[ -n "$content" ]]; then
    # Blank line after title
    output="${output}${prefix}$(generate_content_line "")\n"

    # Word wrap and add content lines
    local wrapped_content=$(word_wrap "$content" $CONTENT_WIDTH)
    while IFS= read -r line; do
      # Left-align content (already padded correctly by generate_content_line)
      output="${output}${prefix}$(generate_content_line "$line")\n"
    done <<<"$wrapped_content"
  fi

  # Bottom border
  output="${output}${prefix}$(generate_bottom_border)"

  printf "%s\n" "$output"
}

# Generate header for .env files (with # prefix)
generate_env_header() {
  local title="$1"
  local content="$2"
  generate_header "$title" "$content" "# "
}

# Terminal output header (no prefix)
show_header() {
  local title="$1"
  local content="${2:-}"
  echo
  printf "%s\n" "${COLOR_CYAN}$(generate_header "$title" "$content")${COLOR_RESET}"
  echo
}

# Compact header for simple titles
show_header_simple() {
  local title="$1"
  echo
  echo "$(generate_top_border)"
  echo "$(generate_content_line "$(center_text "$title" $CONTENT_WIDTH)")"
  echo "$(generate_bottom_border)"
  echo
}

# Export functions for use in other scripts
export -f center_text word_wrap generate_top_border generate_bottom_border
export -f generate_content_line generate_header generate_env_header
export -f show_header show_header_simple
