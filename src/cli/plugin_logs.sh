#!/usr/bin/env bash
# plugin_logs.sh - nself plugin logs <name>
# Tail plugin container logs with optional follow and color highlighting
# Dispatched from plugin.sh as: cmd_logs "$@"
# Bash 3.2+ compatible

set -euo pipefail

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/core.sh" 2>/dev/null || true

if ! declare -f log_info >/dev/null 2>&1; then
  log_info()    { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
  log_error()   { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi

PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
NSELF_LOG_DIR="${NSELF_LOG_DIR:-$HOME/.nself/logs}"

# Color-highlight a log line on the fly using sed
_colorize_line() {
  # Use printf-based pipeline: no echo -e
  local line="$1"
  local out="$line"
  case "$line" in
    *ERROR*|*error*|*FATAL*|*fatal*|*CRIT*|*crit*)
      out="\033[0;31m${line}\033[0m" ;;
    *WARN*|*warn*|*WARNING*|*warning*)
      out="\033[0;33m${line}\033[0m" ;;
    *INFO*|*info*|*DEBUG*|*debug*)
      out="\033[0;36m${line}\033[0m" ;;
  esac
  printf "%b\n" "$out"
}

# ============================================================================
# MAIN COMMAND
# ============================================================================

cmd_logs() {
  local plugin_name=""
  local tail_lines=100
  local follow=false
  local no_follow=false
  local no_color=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --lines|-n)
        tail_lines="${2:-100}"
        shift 2
        ;;
      --tail)
        tail_lines="${2:-100}"
        shift 2
        ;;
      --follow|-f)
        follow=true
        shift
        ;;
      --no-follow)
        no_follow=true
        shift
        ;;
      --no-color)
        no_color=true
        shift
        ;;
      --help|-h)
        printf "Usage: nself plugin logs <name> [--lines N] [--follow] [--no-follow]\n"
        printf "\n"
        printf "Options:\n"
        printf "  --lines N, -n N    Number of lines to show (default: 100)\n"
        printf "  --tail N           Alias for --lines\n"
        printf "  --follow, -f       Stream logs in real time (like tail -f)\n"
        printf "  --no-follow        Print N lines and exit (default)\n"
        printf "  --no-color         Disable color output\n"
        return 0
        ;;
      -*)
        log_error "Unknown option: $1"
        return 1
        ;;
      *)
        plugin_name="$1"
        shift
        ;;
    esac
  done

  if [[ -z "$plugin_name" ]]; then
    log_error "Plugin name required"
    printf "\nUsage: nself plugin logs <name> [--lines N] [--follow]\n"
    return 1
  fi

  # Validate name
  if [[ ! "$plugin_name" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    log_error "Invalid plugin name: $plugin_name"
    return 1
  fi

  # Validate plugin is installed
  if declare -f is_plugin_installed >/dev/null 2>&1; then
    if ! is_plugin_installed "$plugin_name"; then
      log_error "Plugin '$plugin_name' is not installed"
      return 1
    fi
  fi

  # Prefer Docker container logs (most reliable for running plugins)
  local container_name="nself-${plugin_name}"
  local use_docker=false

  if command -v docker >/dev/null 2>&1; then
    if docker inspect "$container_name" >/dev/null 2>&1; then
      use_docker=true
    fi
  fi

  if [[ "$use_docker" == "true" ]]; then
    log_info "Streaming logs from container: $container_name"
    if [[ "$follow" == "true" ]] && [[ "$no_follow" == "false" ]]; then
      # follow mode — stream without colorizing (too complex for raw TTY)
      docker logs -f --tail "$tail_lines" "$container_name" 2>&1
    else
      # Non-follow: capture output and colorize
      local log_output
      log_output=$(docker logs --tail "$tail_lines" "$container_name" 2>&1 || true)
      if [[ -z "$log_output" ]]; then
        log_info "No log output from container $container_name"
        return 0
      fi
      if [[ "$no_color" == "true" ]]; then
        printf '%s\n' "$log_output"
      else
        while IFS= read -r line; do
          _colorize_line "$line"
        done <<EOF
$log_output
EOF
      fi
    fi
    return 0
  fi

  # Fallback: file-based log
  local plugin_log_file="${NSELF_LOG_DIR}/plugins/${plugin_name}.log"

  # Also check in project directory
  if [[ ! -f "$plugin_log_file" ]] && [[ -d ".nself/logs" ]]; then
    plugin_log_file=".nself/logs/plugins/${plugin_name}.log"
  fi

  if [[ ! -f "$plugin_log_file" ]]; then
    log_warning "No log file found for plugin '$plugin_name'"
    printf "Expected locations:\n"
    printf "  %s\n" "${NSELF_LOG_DIR}/plugins/${plugin_name}.log"
    printf "  .nself/logs/plugins/%s.log\n" "$plugin_name"
    printf "\nIf the plugin runs as a Docker container, try: docker logs nself-%s\n" "$plugin_name"
    return 1
  fi

  log_info "Reading logs from: $plugin_log_file"

  if [[ "$follow" == "true" ]] && [[ "$no_follow" == "false" ]]; then
    # tail -f equivalent
    tail -f "$plugin_log_file"
  else
    if [[ "$no_color" == "true" ]]; then
      tail -n "$tail_lines" "$plugin_log_file"
    else
      local log_tail
      log_tail=$(tail -n "$tail_lines" "$plugin_log_file")
      while IFS= read -r line; do
        _colorize_line "$line"
      done <<EOF
$log_tail
EOF
    fi
  fi
}

# ── Standalone invocation ────────────────────────────────────────────────────
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_logs "$@"
fi
