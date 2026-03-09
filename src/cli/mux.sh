#!/usr/bin/env bash
# mux.sh - mux plugin management for nself
# Manages tokens and pipeline configuration for the nself-mux pro plugin.
#
# Commands:
#   nself mux tokens import --file <path.json>  Bulk-import delivery auth tokens
#   nself mux tokens list                        List stored token names
#
# Usage: nself mux <subcommand> [options]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source display helpers
source "$NSELF_ROOT/src/lib/utils/cli-output.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/utils/display.sh" 2>/dev/null || true

# Fallbacks if display helpers didn't load
if ! declare -f cli_error >/dev/null 2>&1; then
  cli_error() { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
fi
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
fi

# ============================================================================
# Usage
# ============================================================================

mux_usage() {
  printf "nself mux — mux pipeline plugin management\n\n"
  printf "Usage: nself mux <subcommand> [options]\n\n"
  printf "Subcommands:\n"
  printf "  tokens import --file <path>  Bulk-import delivery auth tokens from JSON\n"
  printf "  tokens list                  List stored token names (no values shown)\n\n"
  printf "Environment:\n"
  printf "  NSELF_MUX_URL  mux plugin base URL (default: http://localhost:3711)\n\n"
  printf "Examples:\n"
  printf "  nself mux tokens import --file ./tokens.json\n"
  printf "  nself mux tokens list\n\n"
  printf "Token file format (tokens.json):\n"
  printf '  [{"name":"my-webhook","token":"Bearer abc123","description":"Main webhook"}]\n'
}

# ============================================================================
# Top-level dispatcher
# ============================================================================

cmd_mux() {
  local subcommand="${1:-}"

  if [ -z "$subcommand" ]; then
    mux_usage
    exit 0
  fi

  shift

  case "$subcommand" in
    tokens)
      cmd_mux_tokens "$@"
      ;;
    help | --help | -h)
      mux_usage
      exit 0
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      mux_usage
      exit 1
      ;;
  esac
}

# ============================================================================
# Tokens subcommand dispatcher
# ============================================================================

cmd_mux_tokens() {
  local subcmd="${1:-}"

  if [ -z "$subcmd" ]; then
    cli_error "Tokens action required"
    printf "Actions: import, list\n"
    exit 1
  fi

  shift

  case "$subcmd" in

    import)
      # Bulk-import delivery auth tokens from a JSON file.
      # Usage: nself mux tokens import --file <path>
      # File format: [{"name": string, "token": string, "description"?: string}, ...]
      local file_path=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --file | -f) file_path="$2"; shift 2 ;;
          *) shift ;;
        esac
      done

      if [ -z "$file_path" ]; then
        printf "Usage: nself mux tokens import --file <path.json>\n" >&2
        return 1
      fi

      if [ ! -f "$file_path" ]; then
        cli_error "File not found: $file_path"
        return 1
      fi

      # Read file content
      local tokens_json=""
      tokens_json=$(cat "$file_path")

      if [ -z "$tokens_json" ]; then
        cli_error "Token file is empty: $file_path"
        return 1
      fi

      local mux_url="${NSELF_MUX_URL:-http://localhost:3711}"

      log_info "Importing tokens from: $file_path"

      local payload="{\"tokens\":${tokens_json}}"
      local resp=""
      resp=$(curl -s -X POST "${mux_url}/tokens/import" \
        -H "Content-Type: application/json" \
        -d "$payload" 2>/dev/null)

      if [ -z "$resp" ]; then
        cli_error "No response from mux service at ${mux_url}. Is it running?"
        return 1
      fi

      # Extract imported/skipped counts
      local imported="" skipped=""
      imported=$(printf '%s' "$resp" | grep -o '"imported":[0-9]*' | cut -d':' -f2 || true)
      skipped=$(printf '%s' "$resp" | grep -o '"skipped":[0-9]*' | cut -d':' -f2 || true)

      if [ -n "$imported" ]; then
        log_success "Import complete: ${imported} imported, ${skipped:-0} skipped"
      else
        # Show raw response if unexpected
        printf '%s\n' "$resp"
      fi
      ;;

    list)
      # List stored token names (values are never shown).
      # Usage: nself mux tokens list
      local mux_url="${NSELF_MUX_URL:-http://localhost:3711}"
      local result=""
      result=$(curl -s "${mux_url}/tokens" 2>/dev/null)
      printf '%s\n' "$result"
      ;;

    help | --help | -h)
      mux_usage
      exit 0
      ;;

    *)
      cli_error "Unknown tokens action: $subcmd"
      printf "Actions: import, list\n"
      exit 1
      ;;

  esac
}

# ============================================================================
# Entry point
# ============================================================================

cmd_mux "$@"
