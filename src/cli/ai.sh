#!/usr/bin/env bash
# ai.sh - AI plugin management for nself
# Manages accounts and authentication for the nself-ai pro plugin.
#
# Commands:
#   nself ai auth login   --provider <anthropic|openai> [--label <name>] [--priority <n>]
#   nself ai auth refresh [<account_id>]
#   nself ai auth test
#   nself ai auth add     --provider <p> --key <k> [--label <l>] [--priority <n>]
#   nself ai auth list
#   nself ai auth remove  <account_id>
#
# Usage: nself ai <subcommand> [options]

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

ai_usage() {
  printf "nself ai — AI plugin account management\n\n"
  printf "Usage: nself ai <subcommand> [options]\n\n"
  printf "Subcommands:\n"
  printf "  auth login   --provider <anthropic|openai>  OAuth2 PKCE login for subscription accounts\n"
  printf "  auth refresh [<account_id>]                 Force token refresh\n"
  printf "  auth test                                   Test all active AI accounts\n"
  printf "  auth add     --provider <p> --key <k>       Add an API key account\n"
  printf "  auth list                                   List all configured accounts\n"
  printf "  auth remove  <account_id>                   Deactivate an account\n\n"
  printf "Environment:\n"
  printf "  NSELF_AI_URL  AI plugin base URL (default: http://localhost:3710)\n\n"
  printf "Examples:\n"
  printf "  nself ai auth login --provider anthropic\n"
  printf "  nself ai auth login --provider openai --label my-plus\n"
  printf "  nself ai auth add --provider anthropic --key sk-ant-xxx\n"
  printf "  nself ai auth list\n"
  printf "  nself ai auth test\n"
}

# ============================================================================
# Top-level dispatcher
# ============================================================================

cmd_ai() {
  local subcommand="${1:-}"

  if [[ -z "$subcommand" ]]; then
    ai_usage
    exit 0
  fi

  shift

  case "$subcommand" in
    auth)
      cmd_ai_auth "$@"
      ;;
    help | --help | -h)
      ai_usage
      exit 0
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      ai_usage
      exit 1
      ;;
  esac
}

# ============================================================================
# Auth subcommand dispatcher
# ============================================================================

cmd_ai_auth() {
  local subcmd="${1:-}"

  if [[ -z "$subcmd" ]]; then
    cli_error "Auth action required"
    printf "Actions: login, refresh, test, add, list, remove\n"
    exit 1
  fi

  shift

  case "$subcmd" in

    login)
      # OAuth2 PKCE login for subscription accounts (Claude Max, ChatGPT Plus).
      # Usage: nself ai auth login --provider <anthropic|openai> [--label <name>] [--priority <n>]
      local provider="" label="" priority="10"
      while [ $# -gt 0 ]; do
        case "$1" in
          --provider) provider="$2"; shift 2 ;;
          --label)    label="$2";    shift 2 ;;
          --priority) priority="$2"; shift 2 ;;
          *)          shift ;;
        esac
      done

      if [ -z "$provider" ]; then
        printf "Usage: nself ai auth login --provider <anthropic|openai> [--label <name>] [--priority <n>]\n" >&2
        return 1
      fi

      local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
      local resp="" session_id="" auth_url=""

      resp=$(curl -s -X POST "${ai_url}/credentials/oauth/start" \
        -H "Content-Type: application/json" \
        -d "{\"provider\":\"${provider}\"}" 2>/dev/null)

      session_id=$(printf '%s' "$resp" | grep -o '"session_id":"[^"]*"' | cut -d'"' -f4)
      auth_url=$(printf '%s' "$resp" | grep -o '"auth_url":"[^"]*"' | cut -d'"' -f4)

      if [ -z "$session_id" ]; then
        log_error "Failed to start OAuth session: ${resp}"
        return 1
      fi

      printf "\033[0;34m[INFO]\033[0m Open this URL to authenticate:\n\n  %s\n\n" "$auth_url"

      if command -v open >/dev/null 2>&1; then
        open "$auth_url" 2>/dev/null || true
      fi

      printf "\033[0;34m[INFO]\033[0m Waiting for authentication (timeout: 5 min)...\n"

      local status="" attempts=0 max_attempts=150
      while [ "$status" != "complete" ] && [ "$status" != "failed" ] && [ "$status" != "expired" ]; do
        sleep 2
        attempts=$((attempts + 1))
        if [ "$attempts" -ge "$max_attempts" ]; then
          log_error "Timeout waiting for authentication."
          return 1
        fi
        status=$(curl -s "${ai_url}/credentials/oauth/status/${session_id}" 2>/dev/null \
          | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
      done

      if [ "$status" != "complete" ]; then
        log_error "Authentication failed or expired."
        return 1
      fi

      printf "\033[0;32m[SUCCESS]\033[0m Authenticated with %s. Account added.\n" "$provider"
      ;;

    refresh)
      # Force token refresh for an account.
      # Usage: nself ai auth refresh [<account_id>]
      local account_id="${1:-}"
      local ai_url="${NSELF_AI_URL:-http://localhost:3710}"

      if [ -z "$account_id" ]; then
        curl -s -X POST "${ai_url}/credentials/oauth/refresh-all" 2>/dev/null
        printf "\033[0;32m[SUCCESS]\033[0m All OAuth tokens refreshed.\n"
      else
        curl -s -X POST "${ai_url}/credentials/oauth/refresh/${account_id}" 2>/dev/null
        printf "\033[0;32m[SUCCESS]\033[0m Token refreshed for account %s.\n" "$account_id"
      fi
      ;;

    test)
      # Test all active AI accounts.
      # Usage: nself ai auth test
      local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
      local result=""
      result=$(curl -s "${ai_url}/credentials/test" 2>/dev/null)
      printf '%s\n' "$result"
      ;;

    add)
      # Add an API key account.
      # Usage: nself ai auth add --provider <p> --key <k> [--label <l>] [--priority <n>]
      local provider="" api_key="" label="" priority="1"
      while [ $# -gt 0 ]; do
        case "$1" in
          --provider) provider="$2"; shift 2 ;;
          --key)      api_key="$2";  shift 2 ;;
          --label)    label="$2";    shift 2 ;;
          --priority) priority="$2"; shift 2 ;;
          *)          shift ;;
        esac
      done

      if [ -z "$provider" ] || [ -z "$api_key" ]; then
        printf "Usage: nself ai auth add --provider <p> --key <k> [--label <l>] [--priority <n>]\n" >&2
        return 1
      fi

      local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
      local body=""
      if [ -n "$label" ]; then
        body="{\"provider\":\"${provider}\",\"api_key\":\"${api_key}\",\"label\":\"${label}\",\"priority\":${priority}}"
      else
        body="{\"provider\":\"${provider}\",\"api_key\":\"${api_key}\",\"priority\":${priority}}"
      fi

      local resp=""
      resp=$(curl -s -X POST "${ai_url}/accounts" \
        -H "Content-Type: application/json" \
        -d "$body" 2>/dev/null)

      printf '%s\n' "$resp"
      ;;

    list)
      # List all configured accounts.
      # Usage: nself ai auth list
      local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
      local result=""
      result=$(curl -s "${ai_url}/accounts" 2>/dev/null)
      printf '%s\n' "$result"
      ;;

    remove)
      # Deactivate an account.
      # Usage: nself ai auth remove <account_id>
      local account_id="${1:-}"
      if [ -z "$account_id" ]; then
        printf "Usage: nself ai auth remove <account_id>\n" >&2
        return 1
      fi

      local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
      local result=""
      result=$(curl -s -X DELETE "${ai_url}/accounts/${account_id}" 2>/dev/null)
      printf '%s\n' "$result"
      ;;

    help | --help | -h)
      ai_usage
      exit 0
      ;;

    *)
      cli_error "Unknown auth action: $subcmd"
      printf "Actions: login, refresh, test, add, list, remove\n"
      exit 1
      ;;

  esac
}

# ============================================================================
# Entry point
# ============================================================================

cmd_ai "$@"
