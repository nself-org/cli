#!/usr/bin/env bash
# ai.sh - AI plugin management for nself
# Manages accounts, authentication, usage, and source-tier routing for the nself-ai pro plugin.
#
# Commands:
#   nself ai auth login   --provider <anthropic|openai> [--label <name>] [--priority <n>]
#   nself ai auth refresh [<account_id>]
#   nself ai auth test
#   nself ai auth add     --provider <p> --key <k> [--label <l>] [--priority <n>]
#   nself ai auth list
#   nself ai auth remove  <account_id>
#   nself ai usage [--today|--week|--month]
#   nself ai stats [--today|--week|--month]
#   nself ai routing show
#   nself ai routing set  --class <task_class> --tier <local|free_gemini|api_key> --priority <n> [--disable|--enable]
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
  printf "  auth remove  <account_id>                   Deactivate an account\n"
  printf "  usage [--today|--week|--month]              Show AI usage log\n"
  printf "  stats [--today|--week|--month]              Show AI usage summary + savings\n"
  printf "  routing show                                Show source-tier routing config\n"
  printf "  routing set  --class <c> --tier <t> --priority <n> [--disable|--enable]\n"
  printf "                                              Update a routing entry\n"
  printf "  transcribe <audio-file> [--language <code>] Transcribe audio via Whisper\n\n"
  printf "Environment:\n"
  printf "  NSELF_AI_URL            AI plugin base URL (default: http://localhost:3101)\n"
  printf "  PLUGIN_INTERNAL_SECRET  required for usage/stats/routing commands\n\n"
  printf "Examples:\n"
  printf "  nself ai auth login --provider anthropic\n"
  printf "  nself ai auth login --provider openai --label my-plus\n"
  printf "  nself ai auth add --provider anthropic --key sk-ant-xxx\n"
  printf "  nself ai auth list\n"
  printf "  nself ai auth test\n"
  printf "  nself ai usage --today\n"
  printf "  nself ai stats\n"
  printf "  nself ai routing show\n"
  printf "  nself ai routing set --class chat --tier local --priority 1\n"
  printf "  nself ai routing set --class code --tier free_gemini --priority 1 --enable\n"
  printf "  nself ai models list\n"
  printf "  nself ai models install --auto\n"
  printf "  nself ai models install --model phi4-mini\n"
  printf "  nself ai models status\n"
  printf "  nself ai models remove tinyllama\n"
  printf "  nself ai transcribe audio.ogg\n"
  printf "  nself ai transcribe audio.ogg --language en\n"
}

# ============================================================================
# Top-level dispatcher
# ============================================================================

cmd_ai() {
  local subcommand="${1:-}"

  if [ -z "$subcommand" ]; then
    ai_usage
    exit 0
  fi

  shift

  case "$subcommand" in
    auth)
      cmd_ai_auth "$@"
      ;;
    usage)
      cmd_ai_usage "$@"
      ;;
    stats)
      cmd_ai_stats "$@"
      ;;
    routing)
      cmd_ai_routing "$@"
      ;;
    models)
      cmd_ai_models "$@"
      ;;
    transcribe)
      cmd_ai_transcribe "$@"
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

  if [ -z "$subcmd" ]; then
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
# T-1023: usage — direct AI plugin usage log (same as nself claw usage)
# ============================================================================

cmd_ai_usage() {
  local period="all"
  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  while [ $# -gt 0 ]; do
    case "$1" in
      --today)  period="today";  shift ;;
      --week)   period="week";   shift ;;
      --month)  period="month";  shift ;;
      *) shift ;;
    esac
  done

  if [ -z "$internal_secret" ]; then
    cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
    return 1
  fi

  local response=""
  response=$(curl -s \
    -H "x-internal-token: ${internal_secret}" \
    "${ai_url}/ai/usage?period=${period}" 2>/dev/null)

  if [ -z "$response" ]; then
    cli_error "No response from ai service at ${ai_url}. Is it running?"
    return 1
  fi

  printf '%s\n' "$response"
}

cmd_ai_stats() {
  local period="all"
  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  while [ $# -gt 0 ]; do
    case "$1" in
      --today)  period="today";  shift ;;
      --week)   period="week";   shift ;;
      --month)  period="month";  shift ;;
      *) shift ;;
    esac
  done

  if [ -z "$internal_secret" ]; then
    cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
    return 1
  fi

  local response=""
  response=$(curl -s \
    -H "x-internal-token: ${internal_secret}" \
    "${ai_url}/ai/usage/summary?period=${period}" 2>/dev/null)

  if [ -z "$response" ]; then
    cli_error "No response from ai service at ${ai_url}. Is it running?"
    return 1
  fi

  printf '%s\n' "$response"
}

# ============================================================================
# T-1031: routing — view and update source-tier routing config
# ============================================================================

cmd_ai_routing() {
  local subcmd="${1:-show}"
  shift || true

  case "$subcmd" in
    show)
      # Show current routing config as a color-coded table.
      local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
      local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/routing/config" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from ai service at ${ai_url}. Is it running?"
        return 1
      fi

      # Pretty-print with jq if available, otherwise raw JSON
      if command -v jq >/dev/null 2>&1; then
        printf "\n\033[1mAI Source-Tier Routing Config\033[0m\n"
        printf "%-20s %-15s %-10s %-8s\n" "TASK CLASS" "TIER" "PRIORITY" "ENABLED"
        printf "%-20s %-15s %-10s %-8s\n" "----------" "----" "--------" "-------"
        printf '%s' "$response" | jq -r '.[] | [.task_class, .source_tier, (.priority|tostring), (.enabled|tostring)] | @tsv' 2>/dev/null \
        | while IFS='	' read -r task_class tier priority enabled; do
            local color=""
            case "$tier" in
              local)        color="\033[0;32m" ;;  # green
              free_gemini)  color="\033[0;33m" ;;  # yellow
              api_key)      color="\033[0;31m" ;;  # red
              *)            color="\033[0m"    ;;
            esac
            printf "%-20s ${color}%-15s\033[0m %-10s %-8s\n" "$task_class" "$tier" "$priority" "$enabled"
          done
        printf "\n\033[0;32mlocal\033[0m = free (Ollama)  \033[0;33mfree_gemini\033[0m = free (Gemini quota)  \033[0;31mapi_key\033[0m = paid\n\n"
      else
        printf '%s\n' "$response"
      fi
      ;;

    set)
      # Update a routing entry.
      # Usage: nself ai routing set --class <c> --tier <t> --priority <n> [--disable|--enable]
      local task_class="" source_tier="" priority="" enabled="true"
      while [ $# -gt 0 ]; do
        case "$1" in
          --class)    task_class="$2";  shift 2 ;;
          --tier)     source_tier="$2"; shift 2 ;;
          --priority) priority="$2";    shift 2 ;;
          --disable)  enabled="false";  shift ;;
          --enable)   enabled="true";   shift ;;
          *)          shift ;;
        esac
      done

      if [ -z "$task_class" ] || [ -z "$source_tier" ] || [ -z "$priority" ]; then
        printf "Usage: nself ai routing set --class <task_class> --tier <local|free_gemini|api_key> --priority <n> [--disable|--enable]\n" >&2
        return 1
      fi

      # Validate tier
      case "$source_tier" in
        local|free_gemini|api_key) ;;
        *)
          cli_error "Invalid tier '${source_tier}'. Must be: local, free_gemini, or api_key"
          return 1
          ;;
      esac

      local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
      local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local body="[{\"task_class\":\"${task_class}\",\"source_tier\":\"${source_tier}\",\"priority\":${priority},\"enabled\":${enabled}}]"

      local response=""
      response=$(curl -s -X PUT \
        -H "Content-Type: application/json" \
        -H "x-internal-token: ${internal_secret}" \
        -d "$body" \
        "${ai_url}/ai/routing/config" 2>/dev/null)

      if [ -z "$response" ]; then
        log_success "Routing updated: ${task_class} → ${source_tier} (priority ${priority}, enabled=${enabled})"
      else
        printf '%s\n' "$response"
      fi
      ;;

    help | --help | -h)
      printf "nself ai routing — view and update source-tier routing config\n\n"
      printf "Usage: nself ai routing <show|set> [options]\n\n"
      printf "Subcommands:\n"
      printf "  show                          Show routing config table\n"
      printf "  set --class <c> --tier <t> --priority <n> [--disable|--enable]\n"
      printf "                                Update a routing entry\n\n"
      printf "Tiers:\n"
      printf "  local        Ollama (free, local GPU/CPU)\n"
      printf "  free_gemini  Google Gemini free quota\n"
      printf "  api_key      Paid API (OpenAI / Anthropic)\n\n"
      printf "Examples:\n"
      printf "  nself ai routing show\n"
      printf "  nself ai routing set --class chat --tier local --priority 1\n"
      printf "  nself ai routing set --class code --tier free_gemini --priority 1 --enable\n"
      printf "  nself ai routing set --class reason --tier local --priority 3 --disable\n"
      ;;

    *)
      cli_error "Unknown routing action: $subcmd"
      printf "Actions: show, set\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# T-1036: models — local model management
# ============================================================================

cmd_ai_models() {
  local subcmd="${1:-list}"
  shift || true

  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in
    list)
      # Show model catalog with installed/recommended status.
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/models/local" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from ai service at ${ai_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        printf "\n\033[1mLocal Model Catalog\033[0m\n"
        printf "%-22s %-8s %-10s %-12s %-12s\n" "NAME" "PARAMS" "RAM REQ" "STATUS" "NOTE"
        printf "%-22s %-8s %-10s %-12s %-12s\n" "----" "------" "-------" "------" "----"
        printf '%s' "$response" | jq -r '.[] | [.name, ((.params_b|tostring)+"B"), ((.ram_gb_required|tostring)+"GB"), (if .installed then .status else "not installed" end), (if .recommended then "★ recommended" else "" end)] | @tsv' 2>/dev/null \
        | while IFS='	' read -r name params ram status note; do
            local color=""
            case "$status" in
              ready)       color="\033[0;32m" ;;
              downloading) color="\033[0;33m" ;;
              failed)      color="\033[0;31m" ;;
              *)           color="\033[0m"    ;;
            esac
            local note_color=""
            if [ -n "$note" ]; then
              note_color="\033[0;33m"
            fi
            printf "%-22s %-8s %-10s ${color}%-12s\033[0m ${note_color}%s\033[0m\n" "$name" "$params" "$ram" "$status" "$note"
          done
        printf "\n"
      else
        printf '%s\n' "$response"
      fi
      ;;

    install)
      # Install a local model (or auto-select best).
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local model="" auto_select="false"
      while [ $# -gt 0 ]; do
        case "$1" in
          --model) model="$2"; shift 2 ;;
          --auto)  auto_select="true"; shift ;;
          *)       shift ;;
        esac
      done

      local body=""
      if [ "$auto_select" = "true" ]; then
        body='{"auto_select":true}'
      elif [ -n "$model" ]; then
        body="{\"model\":\"${model}\"}"
      else
        # Default: auto-select
        body='{"auto_select":true}'
      fi

      local response=""
      response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "x-internal-token: ${internal_secret}" \
        -d "$body" \
        "${ai_url}/ai/models/local/install" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from ai service at ${ai_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        local model_name="" eta=""
        model_name=$(printf '%s' "$response" | jq -r '.model // empty')
        eta=$(printf '%s' "$response" | jq -r '.estimated_minutes // empty')
        if [ -n "$model_name" ]; then
          log_info "Downloading ${model_name} (estimated: ${eta} min). Run 'nself ai models status' to check progress."
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    status)
      # Show installed models and their download status.
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/models/local" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from ai service at ${ai_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        printf "\n\033[1mInstalled Local Models\033[0m\n"
        printf "%-22s %-12s\n" "MODEL" "STATUS"
        printf "%-22s %-12s\n" "-----" "------"
        printf '%s' "$response" | jq -r '.[] | select(.installed == true) | [.name, .status] | @tsv' 2>/dev/null \
        | while IFS='	' read -r name status; do
            local color=""
            case "$status" in
              ready)       color="\033[0;32m" ;;
              downloading) color="\033[0;33m" ;;
              failed)      color="\033[0;31m" ;;
              *)           color="\033[0m"    ;;
            esac
            printf "%-22s ${color}%s\033[0m\n" "$name" "$status"
          done
        printf "\n"
      else
        printf '%s' "$response" | grep -v '"installed":false' 2>/dev/null || printf '%s\n' "$response"
      fi
      ;;

    remove)
      # Remove a local model.
      local model_name="${1:-}"
      if [ -z "$model_name" ]; then
        printf "Usage: nself ai models remove <model_name>\n" >&2
        return 1
      fi

      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s -X DELETE \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/models/local/remove/${model_name}" 2>/dev/null)

      if command -v jq >/dev/null 2>&1; then
        local removed=""
        removed=$(printf '%s' "$response" | jq -r '.removed // empty' 2>/dev/null)
        if [ -n "$removed" ]; then
          log_success "Model ${removed} removed."
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    help | --help | -h)
      printf "nself ai models — manage local AI models\n\n"
      printf "Usage: nself ai models <list|install|status|remove> [options]\n\n"
      printf "Subcommands:\n"
      printf "  list                     Show model catalog with installed status\n"
      printf "  install [--auto]         Install auto-selected best model for this VPS\n"
      printf "  install --model <name>   Install a specific model\n"
      printf "  status                   Show installed models and download progress\n"
      printf "  remove <model>           Remove an installed model\n\n"
      printf "Examples:\n"
      printf "  nself ai models list\n"
      printf "  nself ai models install --auto\n"
      printf "  nself ai models install --model mistral\n"
      printf "  nself ai models status\n"
      printf "  nself ai models remove tinyllama\n"
      ;;

    *)
      cli_error "Unknown models action: $subcmd"
      printf "Actions: list, install, status, remove\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# transcribe subcommand — T-1118
# ============================================================================

cmd_ai_transcribe() {
  # Upload an audio file to POST /ai/transcribe and print the transcript.
  # Usage: nself ai transcribe <audio-file> [--language <code>]

  # Show help if first arg is --help/-h or no args given.
  case "${1:-}" in
    --help | -h | "")
      printf "Usage: nself ai transcribe <audio-file> [--language <code>]\n\n"
      printf "  audio-file   Path to OGG, WAV, MP3, or M4A file\n"
      printf "  --language   Language code (default: auto-detect). Examples: en, ar, fr\n\n"
      printf "Requires Whisper installed: nself ai models install --model openai/whisper\n"
      return 0
      ;;
  esac

  local audio_file="${1:-}"
  shift || true

  local language=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --language) language="$2"; shift 2 ;;
      -l)         language="$2"; shift 2 ;;
      *) shift ;;
    esac
  done

  if [ -z "$audio_file" ]; then
    cli_error "Audio file path required"
    printf "Usage: nself ai transcribe <audio-file> [--language <code>]\n" >&2
    exit 1
  fi

  if [ ! -f "$audio_file" ]; then
    cli_error "File not found: $audio_file"
    exit 1
  fi

  local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  if [ -z "$internal_secret" ]; then
    cli_error "PLUGIN_INTERNAL_SECRET is required"
    exit 1
  fi

  log_info "Transcribing $(basename "$audio_file") ..."

  local curl_args=()
  curl_args+=(-s -X POST)
  curl_args+=(-H "x-internal-secret: $internal_secret")
  curl_args+=(-F "audio=@${audio_file}")
  if [ -n "$language" ]; then
    curl_args+=(-F "language=$language")
  fi
  curl_args+=("${ai_url}/ai/transcribe")

  local response=""
  response=$(curl "${curl_args[@]}" 2>/dev/null)

  if [ -z "$response" ]; then
    cli_error "No response from /ai/transcribe. Is nself-ai running?"
    exit 1
  fi

  local http_status=""
  if command -v jq >/dev/null 2>&1; then
    http_status=$(printf '%s' "$response" | jq -r '.status // empty' 2>/dev/null)
  fi

  # Handle 503 — Whisper not installed.
  if [ "$http_status" = "503" ] || printf '%s' "$response" | grep -q '"no_whisper"'; then
    local msg=""
    msg=$(printf '%s' "$response" | jq -r '.message // .error // "Whisper not installed"' 2>/dev/null \
      || printf "Whisper not installed")
    cli_error "$msg"
    printf "Install Whisper: nself ai models install --model openai/whisper\n" >&2
    exit 1
  fi

  # Print transcript to stdout.
  if command -v jq >/dev/null 2>&1; then
    local transcript duration
    transcript=$(printf '%s' "$response" | jq -r '.transcript // .text // empty' 2>/dev/null)
    duration=$(printf '%s' "$response" | jq -r '.duration_seconds // empty' 2>/dev/null)
    if [ -n "$transcript" ]; then
      printf '%s\n' "$transcript"
      if [ -n "$duration" ]; then
        printf "\n\033[2mDuration: %ss\033[0m\n" "$duration"
      fi
    else
      printf '%s\n' "$response"
    fi
  else
    printf '%s\n' "$response"
  fi
}

# ============================================================================
# Entry point
# ============================================================================

cmd_ai "$@"
