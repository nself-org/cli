#!/usr/bin/env bash

# logging.sh - Structured logging system for nself
# Provides secure, comprehensive logging with multiple levels and outputs
# Cross-platform compatible (Bash 3.2+)

# Prevent double-sourcing
[[ "${LOGGING_SOURCED:-}" == "1" ]] && return 0

set -euo pipefail

export LOGGING_SOURCED=1

# Source dependencies (namespaced to avoid clobbering caller's SCRIPT_DIR)
_LOGGING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${_LOGGING_DIR}/platform-compat.sh" 2>/dev/null || true

# =============================================================================
# LOGGING CONFIGURATION
# =============================================================================

# Default log levels (0-5, higher = more verbose)
export LOG_LEVEL_FATAL=0
export LOG_LEVEL_ERROR=1
export LOG_LEVEL_WARN=2
export LOG_LEVEL_INFO=3
export LOG_LEVEL_DEBUG=4
export LOG_LEVEL_TRACE=5

# Current log level (default: INFO)
export NSELF_LOG_LEVEL="${NSELF_LOG_LEVEL:-${LOG_LEVEL_INFO}}"

# Log output configuration
export NSELF_LOG_DIR="${NSELF_LOG_DIR:-${HOME}/.nself/logs}"
export NSELF_LOG_FILE="${NSELF_LOG_FILE:-nself.log}"
export NSELF_LOG_TO_FILE="${NSELF_LOG_TO_FILE:-true}"
export NSELF_LOG_TO_CONSOLE="${NSELF_LOG_TO_CONSOLE:-true}"
export NSELF_LOG_TIMESTAMP_FORMAT="${NSELF_LOG_TIMESTAMP_FORMAT:-%Y-%m-%d %H:%M:%S}"

# Log rotation settings
export NSELF_LOG_MAX_SIZE="${NSELF_LOG_MAX_SIZE:-10485760}" # 10MB default
export NSELF_LOG_MAX_FILES="${NSELF_LOG_MAX_FILES:-5}"

# Security settings
export NSELF_LOG_SANITIZE="${NSELF_LOG_SANITIZE:-true}"
export NSELF_LOG_REDACT_PATTERNS="${NSELF_LOG_REDACT_PATTERNS:-password|secret|token|key|auth}"

# Color codes for console output
export LOG_COLOR_FATAL=$'\033[1;35m'  # Bold Magenta
export LOG_COLOR_ERROR=$'\033[1;31m'  # Bold Red
export LOG_COLOR_WARN=$'\033[1;33m'   # Bold Yellow
export LOG_COLOR_INFO=$'\033[0;36m'   # Cyan
export LOG_COLOR_DEBUG=$'\033[0;90m'  # Gray
export LOG_COLOR_TRACE=$'\033[0;37m'  # White
export LOG_COLOR_RESET=$'\033[0m'

# =============================================================================
# INITIALIZATION
# =============================================================================

# Initialize logging system
log_init() {
  local log_dir="${1:-${NSELF_LOG_DIR}}"

  # Create log directory if it doesn't exist
  if [[ ! -d "$log_dir" ]]; then
    mkdir -p "$log_dir" 2>/dev/null || {
      printf "WARNING: Failed to create log directory: %s\n" "$log_dir" >&2
      export NSELF_LOG_TO_FILE=false
      return 1
    }
  fi

  # Set proper permissions (owner only for security)
  chmod 700 "$log_dir" 2>/dev/null || true

  # Create log file if it doesn't exist
  local log_path="${log_dir}/${NSELF_LOG_FILE}"
  if [[ ! -f "$log_path" ]]; then
    touch "$log_path" 2>/dev/null || {
      printf "WARNING: Failed to create log file: %s\n" "$log_path" >&2
      export NSELF_LOG_TO_FILE=false
      return 1
    }
  fi

  # Set proper permissions (owner read/write only)
  chmod 600 "$log_path" 2>/dev/null || true

  return 0
}

# =============================================================================
# UTILITY FUNCTIONS
# =============================================================================

# Get current timestamp
_log_timestamp() {
  date "+${NSELF_LOG_TIMESTAMP_FORMAT}"
}

# Get log level name
_log_level_name() {
  local level="$1"
  case "$level" in
    0) printf "FATAL" ;;
    1) printf "ERROR" ;;
    2) printf "WARN" ;;
    3) printf "INFO" ;;
    4) printf "DEBUG" ;;
    5) printf "TRACE" ;;
    *) printf "UNKNOWN" ;;
  esac
}

# Get log level color
_log_level_color() {
  local level="$1"
  case "$level" in
    0) printf "%s" "$LOG_COLOR_FATAL" ;;
    1) printf "%s" "$LOG_COLOR_ERROR" ;;
    2) printf "%s" "$LOG_COLOR_WARN" ;;
    3) printf "%s" "$LOG_COLOR_INFO" ;;
    4) printf "%s" "$LOG_COLOR_DEBUG" ;;
    5) printf "%s" "$LOG_COLOR_TRACE" ;;
    *) printf "%s" "$LOG_COLOR_RESET" ;;
  esac
}

# Sanitize sensitive data from log message
_log_sanitize() {
  local message="$1"

  if [[ "$NSELF_LOG_SANITIZE" != "true" ]]; then
    printf "%s" "$message"
    return
  fi

  # Redact common sensitive patterns
  local sanitized="$message"

  # Redact passwords in various formats
  sanitized=$(printf "%s" "$sanitized" | sed -E 's/(password|PASSWORD|pwd|PWD)[[:space:]]*[=:][[:space:]]*[^[:space:]&,;]+/\1=***REDACTED***/g')

  # Redact tokens and keys
  sanitized=$(printf "%s" "$sanitized" | sed -E 's/(token|TOKEN|key|KEY|secret|SECRET)[[:space:]]*[=:][[:space:]]*[^[:space:]&,;]+/\1=***REDACTED***/g')

  # Redact API keys (format: Bearer xxx, api_key=xxx, etc.)
  sanitized=$(printf "%s" "$sanitized" | sed -E 's/(Bearer|bearer|api_key|API_KEY)[[:space:]]*[^[:space:]&,;]+/\1 ***REDACTED***/g')

  # Redact basic auth (user:pass@host)
  sanitized=$(printf "%s" "$sanitized" | sed -E 's#://[^:]+:[^@]+@#://***REDACTED***@#g')

  # Redact JWT tokens (eyJ...)
  sanitized=$(printf "%s" "$sanitized" | sed -E 's/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/***JWT_REDACTED***/g')

  # Redact UUIDs if they appear near sensitive keywords
  sanitized=$(printf "%s" "$sanitized" | sed -E "s/(${NSELF_LOG_REDACT_PATTERNS})[[:space:]]*[=:][[:space:]]*[a-f0-9-]{8,}/\1=***REDACTED***/gi")

  # Redact file paths that might contain user info
  sanitized=$(printf "%s" "$sanitized" | sed -E 's#/home/[^/]+#/home/***USER***#g')
  sanitized=$(printf "%s" "$sanitized" | sed -E 's#/Users/[^/]+#/Users/***USER***#g')

  printf "%s" "$sanitized"
}

# Rotate log files if needed
_log_rotate() {
  local log_path="${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"

  # Check if log file exists
  [[ ! -f "$log_path" ]] && return 0

  # Get file size
  local file_size
  file_size=$(safe_stat_size "$log_path" 2>/dev/null || echo "0")

  # Check if rotation is needed
  if [[ "$file_size" -lt "$NSELF_LOG_MAX_SIZE" ]]; then
    return 0
  fi

  # Rotate existing logs
  local i=$((NSELF_LOG_MAX_FILES - 1))
  while [[ $i -gt 0 ]]; do
    local old_log="${log_path}.$i"
    local new_log="${log_path}.$((i + 1))"

    if [[ -f "$old_log" ]]; then
      if [[ $((i + 1)) -le $NSELF_LOG_MAX_FILES ]]; then
        mv "$old_log" "$new_log" 2>/dev/null || true
      else
        rm -f "$old_log" 2>/dev/null || true
      fi
    fi

    i=$((i - 1))
  done

  # Move current log to .1
  mv "$log_path" "${log_path}.1" 2>/dev/null || true

  # Create new log file
  touch "$log_path" 2>/dev/null || true
  chmod 600 "$log_path" 2>/dev/null || true
}

# =============================================================================
# CORE LOGGING FUNCTIONS
# =============================================================================

# Main logging function
# Args: level, message, [context_data...]
_log_write() {
  local level="$1"
  local message="$2"
  shift 2
  local context=("$@")

  # Check if this log level should be logged
  if [[ "$level" -gt "$NSELF_LOG_LEVEL" ]]; then
    return 0
  fi

  # Get metadata
  local timestamp
  timestamp=$(_log_timestamp)
  local level_name
  level_name=$(_log_level_name "$level")
  local pid=$$

  # Sanitize message
  local sanitized_message
  sanitized_message=$(_log_sanitize "$message")

  # Build context string
  local context_str=""
  if [[ ${#context[@]} -gt 0 ]]; then
    local ctx
    for ctx in "${context[@]}"; do
      local sanitized_ctx
      sanitized_ctx=$(_log_sanitize "$ctx")
      context_str="${context_str} ${sanitized_ctx}"
    done
  fi

  # Format log entry (structured format for parsing)
  local log_entry
  log_entry=$(printf "[%s] [%s] [PID:%s] %s%s" \
    "$timestamp" "$level_name" "$pid" "$sanitized_message" "$context_str")

  # Write to file if enabled
  if [[ "$NSELF_LOG_TO_FILE" == "true" ]]; then
    local log_path="${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"

    # Rotate if needed
    _log_rotate

    # Append to log file
    printf "%s\n" "$log_entry" >> "$log_path" 2>/dev/null || true
  fi

  # Write to console if enabled
  if [[ "$NSELF_LOG_TO_CONSOLE" == "true" ]] && [[ -t 2 ]]; then
    local color
    color=$(_log_level_color "$level")

    # Colorized output for console
    printf "%b[%s]%b %s\n" \
      "$color" "$level_name" "$LOG_COLOR_RESET" "$sanitized_message" >&2
  elif [[ "$NSELF_LOG_TO_CONSOLE" == "true" ]]; then
    # Plain output when not a terminal
    printf "[%s] %s\n" "$level_name" "$sanitized_message" >&2
  fi
}

# Public logging functions
log_fatal() {
  _log_write "$LOG_LEVEL_FATAL" "$@"
  return 1  # Fatal errors should cause failure
}

log_error() {
  _log_write "$LOG_LEVEL_ERROR" "$@"
}

log_warn() {
  _log_write "$LOG_LEVEL_WARN" "$@"
}

log_info() {
  _log_write "$LOG_LEVEL_INFO" "$@"
}

log_debug() {
  _log_write "$LOG_LEVEL_DEBUG" "$@"
}

log_trace() {
  _log_write "$LOG_LEVEL_TRACE" "$@"
}

# =============================================================================
# SPECIALIZED LOGGING FUNCTIONS
# =============================================================================

# Log command execution
log_command() {
  local cmd="$1"
  local sanitized_cmd
  sanitized_cmd=$(_log_sanitize "$cmd")
  _log_write "$LOG_LEVEL_DEBUG" "Executing command: ${sanitized_cmd}"
}

# Log file access
log_file_access() {
  local operation="$1"
  local filepath="$2"

  # Sanitize path to remove user-specific info
  local sanitized_path
  sanitized_path=$(_log_sanitize "$filepath")

  _log_write "$LOG_LEVEL_TRACE" "File ${operation}: ${sanitized_path}"
}

# Log security event
log_security() {
  local event="$1"
  shift
  _log_write "$LOG_LEVEL_WARN" "SECURITY: ${event}" "$@"
}

# Log with error code
log_with_code() {
  local level="$1"
  local code="$2"
  local message="$3"
  shift 3

  _log_write "$level" "[${code}] ${message}" "$@"
}

# =============================================================================
# LOG MANAGEMENT FUNCTIONS
# =============================================================================

# Clear all logs
log_clear() {
  local log_path="${NSELF_LOG_DIR}/${NSELF_LOG_FILE}"

  if [[ -f "$log_path" ]]; then
    : > "$log_path"
    log_info "Logs cleared"
  fi

  # Clear rotated logs
  local i=1
  while [[ $i -le $NSELF_LOG_MAX_FILES ]]; do
    if [[ -f "${log_path}.${i}" ]]; then
      rm -f "${log_path}.${i}"
    fi
    i=$((i + 1))
  done
}

# Get log file path
log_get_path() {
  printf "%s/%s" "$NSELF_LOG_DIR" "$NSELF_LOG_FILE"
}

# Tail logs (follow mode)
log_tail() {
  local lines="${1:-50}"
  local log_path
  log_path=$(log_get_path)

  if [[ ! -f "$log_path" ]]; then
    printf "No logs found at: %s\n" "$log_path" >&2
    return 1
  fi

  if command -v tail >/dev/null 2>&1; then
    tail -n "$lines" "$log_path"
  else
    # Fallback for systems without tail
    local total_lines
    total_lines=$(wc -l < "$log_path" 2>/dev/null || echo "0")
    local skip=$((total_lines - lines))
    [[ $skip -lt 0 ]] && skip=0

    local i=0
    while IFS= read -r line; do
      if [[ $i -ge $skip ]]; then
        printf "%s\n" "$line"
      fi
      i=$((i + 1))
    done < "$log_path"
  fi
}

# Follow logs in real-time
log_follow() {
  local log_path
  log_path=$(log_get_path)

  if [[ ! -f "$log_path" ]]; then
    printf "No logs found at: %s\n" "$log_path" >&2
    return 1
  fi

  if command -v tail >/dev/null 2>&1; then
    tail -f "$log_path"
  else
    printf "tail command not available\n" >&2
    return 1
  fi
}

# Search logs
log_search() {
  local pattern="$1"
  local log_path
  log_path=$(log_get_path)

  if [[ ! -f "$log_path" ]]; then
    printf "No logs found at: %s\n" "$log_path" >&2
    return 1
  fi

  if command -v grep >/dev/null 2>&1; then
    grep -i "$pattern" "$log_path" || {
      printf "No matches found for: %s\n" "$pattern" >&2
      return 1
    }
  else
    printf "grep command not available\n" >&2
    return 1
  fi
}

# Export logs to file
log_export() {
  local output_file="$1"
  local log_path
  log_path=$(log_get_path)

  if [[ ! -f "$log_path" ]]; then
    printf "No logs found at: %s\n" "$log_path" >&2
    return 1
  fi

  cp "$log_path" "$output_file" 2>/dev/null || {
    printf "Failed to export logs to: %s\n" "$output_file" >&2
    return 1
  }

  printf "Logs exported to: %s\n" "$output_file"
}

# Get log statistics
log_stats() {
  local log_path
  log_path=$(log_get_path)

  if [[ ! -f "$log_path" ]]; then
    printf "No logs found\n"
    return 1
  fi

  local total_lines
  total_lines=$(wc -l < "$log_path" 2>/dev/null || echo "0")

  local fatal_count
  fatal_count=$(grep -c "\[FATAL\]" "$log_path" 2>/dev/null || echo "0")

  local error_count
  error_count=$(grep -c "\[ERROR\]" "$log_path" 2>/dev/null || echo "0")

  local warn_count
  warn_count=$(grep -c "\[WARN\]" "$log_path" 2>/dev/null || echo "0")

  local info_count
  info_count=$(grep -c "\[INFO\]" "$log_path" 2>/dev/null || echo "0")

  local file_size
  file_size=$(safe_stat_size "$log_path" 2>/dev/null || echo "0")

  local size_mb=$((file_size / 1024 / 1024))

  printf "Log Statistics\n"
  printf "==============\n"
  printf "Total entries: %s\n" "$total_lines"
  printf "FATAL: %s\n" "$fatal_count"
  printf "ERROR: %s\n" "$error_count"
  printf "WARN: %s\n" "$warn_count"
  printf "INFO: %s\n" "$info_count"
  printf "File size: %s MB\n" "$size_mb"
}

# =============================================================================
# EXPORT FUNCTIONS
# =============================================================================

export -f log_init
export -f log_fatal
export -f log_error
export -f log_warn
export -f log_info
export -f log_debug
export -f log_trace
export -f log_command
export -f log_file_access
export -f log_security
export -f log_with_code
export -f log_clear
export -f log_get_path
export -f log_tail
export -f log_follow
export -f log_search
export -f log_export
export -f log_stats

# Auto-initialize logging on source
log_init >/dev/null 2>&1 || true
