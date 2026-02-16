#!/usr/bin/env bash


# Portable timeout implementation for cross-platform compatibility
# Works with Bash 3.2+ and on systems without GNU timeout

# Check if we have a native timeout command
if command -v timeout >/dev/null 2>&1; then

set -euo pipefail

  TIMEOUT_CMD="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD="gtimeout"
else
  TIMEOUT_CMD=""
fi

# Portable timeout function
# Usage: portable_timeout <seconds> <command> [args...]
portable_timeout() {
  local timeout_seconds="$1"
  shift

  # Use native timeout if available
  if [[ -n "$TIMEOUT_CMD" ]]; then
    "$TIMEOUT_CMD" "$timeout_seconds" "$@"
    return $?
  fi

  # Fallback: Use background process with kill
  # This works on all Unix-like systems with Bash 3.2+
  local temp_script=$(mktemp)
  local pid_file=$(mktemp)

  # Create wrapper script
  cat >"$temp_script" <<'EOF'
#!/bin/bash
echo $$ > "$PID_FILE"
exec "$@"
EOF

  chmod +x "$temp_script"

  # Export PID_FILE for the script
  export PID_FILE="$pid_file"

  # Start the command in background
  "$temp_script" "$@" &
  local cmd_pid=$!

  # Start timeout monitor in background
  (
    sleep "$timeout_seconds"
    if kill -0 "$cmd_pid" 2>/dev/null; then
      # Command still running, kill it
      kill -TERM "$cmd_pid" 2>/dev/null
      sleep 1
      kill -KILL "$cmd_pid" 2>/dev/null
    fi
  ) &
  local timeout_pid=$!

  # Wait for command to complete
  local exit_code=0
  if wait "$cmd_pid" 2>/dev/null; then
    exit_code=0
  else
    exit_code=124 # Standard timeout exit code
  fi

  # Clean up timeout monitor
  kill "$timeout_pid" 2>/dev/null || true
  wait "$timeout_pid" 2>/dev/null || true

  # Clean up temp files
  rm -f "$temp_script" "$pid_file" 2>/dev/null || true
  unset PID_FILE

  return $exit_code
}

# Portable stat function for getting modification time
# Usage: portable_stat_mtime <file>
portable_stat_mtime() {
  local file="$1"

  # Try macOS/BSD stat first, then GNU stat
  if stat -f %m "$file" 2>/dev/null; then
    return 0
  elif stat -c %Y "$file" 2>/dev/null; then
    return 0
  else
    return 1
  fi
}

# Export functions for use in other scripts
export -f portable_timeout
export -f portable_stat_mtime
