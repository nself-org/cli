#!/usr/bin/env bash

# platform-compat.sh - Platform compatibility utilities
# POSIX-compliant, cross-platform helpers

# Platform-safe sed inline editing
safe_sed_inline() {

set -euo pipefail

  local file="$1"
  shift # Remove file from arguments

  if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS requires empty string after -i
    sed -i '' "$@" "$file"
  else
    # Linux doesn't need empty string
    sed -i "$@" "$file"
  fi
}

# Platform-safe readlink
safe_readlink() {
  local path="$1"

  if command -v realpath >/dev/null 2>&1; then
    realpath "$path"
  elif command -v readlink >/dev/null 2>&1; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      # macOS readlink doesn't have -f
      local dir="$path"
      while [[ -L "$dir" ]]; do
        dir=$(readlink "$dir")
      done
      echo "$dir"
    else
      readlink -f "$path"
    fi
  else
    # Fallback - resolve manually
    if [[ "$path" = /* ]]; then
      echo "$path"
    else
      echo "$PWD/${path#./}"
    fi
  fi
}

# Platform-safe mktemp
safe_mktemp() {
  if command -v mktemp >/dev/null 2>&1; then
    mktemp "$@"
  else
    # Fallback for systems without mktemp
    local template="${1:-/tmp/tmp.XXXXXX}"
    local file="${template/XXXXXX/$$_$RANDOM}"
    touch "$file"
    echo "$file"
  fi
}

# Platform-safe date formatting
safe_date() {
  local format="${1:-%Y%m%d_%H%M%S}"

  if date --version 2>/dev/null | grep -q GNU; then
    # GNU date (Linux)
    date "+$format"
  else
    # BSD date (macOS)
    date "+$format"
  fi
}

# Platform-safe stat for file modification time
safe_stat_mtime() {
  local file="$1"

  if stat --version 2>/dev/null | grep -q GNU; then
    # GNU stat (Linux)
    stat -c %Y "$file"
  else
    # BSD stat (macOS)
    stat -f %m "$file"
  fi
}

# Platform-safe stat for file permissions (octal)
safe_stat_perms() {
  local file="$1"

  if stat --version 2>/dev/null | grep -q GNU; then
    # GNU stat (Linux) - permissions in octal
    stat -c "%a" "$file"
  else
    # BSD stat (macOS) - permissions in octal
    stat -f "%OLp" "$file"
  fi
}

# Platform-safe stat for file size in bytes
safe_stat_size() {
  local file="$1"

  if stat --version 2>/dev/null | grep -q GNU; then
    # GNU stat (Linux) - size in bytes
    stat -c "%s" "$file"
  else
    # BSD stat (macOS) - size in bytes
    stat -f "%z" "$file"
  fi
}

# Platform-safe grep with extended regex
safe_grep_extended() {
  if grep --version 2>/dev/null | grep -q GNU; then
    # GNU grep
    grep -E "$@"
  else
    # BSD grep (macOS)
    grep -E "$@"
  fi
}

# Platform-safe find with maxdepth
safe_find() {
  local dir="${1:-.}"
  shift

  if find --version 2>/dev/null | grep -q GNU; then
    # GNU find
    find "$dir" "$@"
  else
    # BSD find (macOS) - same syntax
    find "$dir" "$@"
  fi
}

# Get number of CPU cores
get_cpu_cores() {
  if command -v nproc >/dev/null 2>&1; then
    # Linux
    nproc
  elif command -v sysctl >/dev/null 2>&1; then
    # macOS
    sysctl -n hw.ncpu
  else
    # Fallback
    echo "1"
  fi
}

# Get total memory in MB
get_total_memory_mb() {
  if command -v free >/dev/null 2>&1; then
    # Linux
    free -m | awk 'NR==2{print $2}'
  elif command -v sysctl >/dev/null 2>&1; then
    # macOS - returns bytes, convert to MB
    local bytes=$(sysctl -n hw.memsize)
    echo $((bytes / 1024 / 1024))
  else
    # Fallback
    echo "1024"
  fi
}

# Check if running on WSL
is_wsl() {
  if [[ -f /proc/version ]]; then
    grep -qi microsoft /proc/version && return 0
  fi
  return 1
}

# Check if running on macOS
is_macos() {
  [[ "$OSTYPE" == "darwin"* ]] && return 0
  return 1
}

# Check if running on Linux
is_linux() {
  [[ "$OSTYPE" == "linux-gnu"* ]] && return 0
  return 1
}

# Get platform name
get_platform() {
  if is_macos; then
    echo "macos"
  elif is_wsl; then
    echo "wsl"
  elif is_linux; then
    echo "linux"
  else
    echo "unknown"
  fi
}

# Platform-safe array handling (no associative arrays)
# Use parallel arrays instead of associative arrays
declare -a COMPAT_KEYS
declare -a COMPAT_VALUES

# Set a key-value pair
compat_set() {
  local key="$1"
  local value="$2"
  local found=false
  local i=0

  # Update existing key
  for ((i = 0; i < ${#COMPAT_KEYS[@]}; i++)); do
    if [[ "${COMPAT_KEYS[$i]}" == "$key" ]]; then
      COMPAT_VALUES[$i]="$value"
      found=true
      break
    fi
  done

  # Add new key
  if [[ "$found" == "false" ]]; then
    COMPAT_KEYS+=("$key")
    COMPAT_VALUES+=("$value")
  fi
}

# Get a value by key
compat_get() {
  local key="$1"
  local i=0

  for ((i = 0; i < ${#COMPAT_KEYS[@]}; i++)); do
    if [[ "${COMPAT_KEYS[$i]}" == "$key" ]]; then
      echo "${COMPAT_VALUES[$i]}"
      return 0
    fi
  done

  return 1
}

# Platform-safe timeout command
safe_timeout() {
  local timeout_seconds="$1"
  shift

  # Check for native timeout command
  if command -v timeout >/dev/null 2>&1; then
    timeout "$timeout_seconds" "$@"
    return $?
  elif command -v gtimeout >/dev/null 2>&1; then
    # macOS with coreutils installed
    gtimeout "$timeout_seconds" "$@"
    return $?
  else
    # Fallback: run without timeout on systems that don't have it
    # This ensures compatibility but loses timeout functionality
    "$@"
    return $?
  fi
}

# Platform-safe lsof check for port usage
safe_check_port() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    # Use lsof if available
    lsof -ti ":$port" 2>/dev/null
    return $?
  elif command -v netstat >/dev/null 2>&1; then
    # Fallback to netstat (more universal)
    netstat -an 2>/dev/null | grep -E "[:.]${port}[[:space:]]" | grep -q LISTEN
    return $?
  elif command -v ss >/dev/null 2>&1; then
    # Modern Linux alternative to netstat
    ss -ltn 2>/dev/null | grep -q ":${port} "
    return $?
  else
    # No tool available - can't check
    return 1
  fi
}

# Sanitize filename to prevent path traversal attacks
# Removes path separators and dangerous characters
# Keeps only alphanumeric, dots, dashes, underscores
# Limits to 255 characters (standard filesystem limit)
sanitize_filename() {
  local filename="$1"
  local sanitized=""
  local i

  # Convert to lowercase for consistency
  filename=$(printf '%s' "$filename" | tr '[:upper:]' '[:lower:]')

  # Process character by character
  for ((i = 0; i < ${#filename}; i++)); do
    local char="${filename:$i:1}"
    case "$char" in
      # Allow alphanumeric
      [a-z0-9])
        sanitized="${sanitized}${char}"
        ;;
      # Allow safe special characters
      [._-])
        sanitized="${sanitized}${char}"
        ;;
      # Convert spaces to underscores
      ' ')
        sanitized="${sanitized}_"
        ;;
      # Skip all other characters (includes / \ and other dangerous chars)
      *)
        :
        ;;
    esac
  done

  # Handle edge cases
  if [[ -z "$sanitized" ]]; then
    sanitized="file"
  fi

  # Remove leading/trailing dashes (can cause issues with some tools)
  sanitized="${sanitized#-}"
  sanitized="${sanitized%-}"

  # Limit to 255 characters (standard filesystem limit)
  sanitized="${sanitized:0:255}"

  printf '%s' "$sanitized"
}

# Export all functions
export -f safe_sed_inline
export -f safe_readlink
export -f safe_mktemp
export -f safe_date
export -f safe_stat_mtime
export -f safe_stat_perms
export -f safe_grep_extended
export -f safe_find
export -f get_cpu_cores
export -f get_total_memory_mb
export -f is_wsl
export -f is_macos
export -f is_linux
export -f get_platform
export -f compat_set
export -f compat_get
export -f safe_timeout
export -f safe_check_port
export -f sanitize_filename
