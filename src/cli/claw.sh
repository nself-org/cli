#!/usr/bin/env bash
# claw.sh - claw plugin management for nself
# Manages email rules and thread sessions for the nself-claw pro plugin.
#
# Commands:
#   nself claw email-rules list                                          List ClawDelegate email rules
#   nself claw email-rules test-delegate --email <addr> --subject <text> [--body <text>]
#                                                                        Test delegate routing for an email
#   nself claw email-threads                                             List claw thread→session mappings
#
# Usage: nself claw <subcommand> [options]

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

claw_usage() {
  printf "nself claw — claw plugin management\n\n"
  printf "Usage: nself claw <subcommand> [options]\n\n"
  printf "Subcommands:\n"
  printf "  email-rules list                                      List ClawDelegate email routing rules\n"
  printf "  email-rules test-delegate --email <addr>              Test delegate routing for an email address\n"
  printf "                            --subject <text>\n"
  printf "                            [--body <text>]\n"
  printf "  email-threads                                         List claw thread-to-session mappings\n\n"
  printf "Environment:\n"
  printf "  NSELF_MUX_URL   mux plugin base URL  (default: http://localhost:3711)\n"
  printf "  NSELF_CLAW_URL  claw plugin base URL (default: http://localhost:3713)\n\n"
  printf "Examples:\n"
  printf "  nself claw email-rules list\n"
  printf "  nself claw email-rules test-delegate --email user@example.com --subject 'Help request'\n"
  printf "  nself claw email-rules test-delegate --email user@example.com --subject 'Question' --body 'Can you help?'\n"
  printf "  nself claw email-threads\n"
}

# ============================================================================
# Top-level dispatcher
# ============================================================================

cmd_claw() {
  local subcommand="${1:-}"

  if [ -z "$subcommand" ]; then
    claw_usage
    exit 0
  fi

  shift

  case "$subcommand" in
    email-rules)
      cmd_claw_email_rules "$@"
      ;;
    email-threads)
      cmd_claw_email_threads "$@"
      ;;
    help | --help | -h)
      claw_usage
      exit 0
      ;;
    *)
      cli_error "Unknown subcommand: $subcommand"
      printf "\n"
      claw_usage
      exit 1
      ;;
  esac
}

# ============================================================================
# email-rules subcommand dispatcher
# ============================================================================

cmd_claw_email_rules() {
  local subcmd="${1:-}"

  if [ -z "$subcmd" ]; then
    cli_error "email-rules action required"
    printf "Actions: list, test-delegate\n"
    exit 1
  fi

  shift

  case "$subcmd" in

    list)
      # List all email routing rules that use the ClawDelegate action type.
      # Usage: nself claw email-rules list
      local mux_url="${NSELF_MUX_URL:-http://localhost:3711}"
      local response=""
      response=$(curl -s "${mux_url}/mux/rules" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from mux service at ${mux_url}. Is it running?"
        return 1
      fi

      # Pretty-print with jq if available; filter to ClawDelegate action type
      if command -v jq >/dev/null 2>&1; then
        local delegate_rules=""
        delegate_rules=$(printf '%s' "$response" | jq -r '.[] | select(.action.type == "ClawDelegate") | [.id, .name, (.priority // 0 | tostring), (.enabled // true | if . then "enabled" else "disabled" end)] | @tsv' 2>/dev/null || true)

        if [ -z "$delegate_rules" ]; then
          log_info "No ClawDelegate rules found."
          return 0
        fi

        printf "\033[34m%-36s %-30s %-8s %s\033[0m\n" "ID" "Name" "Priority" "Status"
        printf "%-36s %-30s %-8s %s\n" "------------------------------------" "------------------------------" "--------" "-------"

        printf '%s\n' "$delegate_rules" | while IFS=$(printf '\t') read -r rule_id rule_name priority status; do
          if [ "$status" = "enabled" ]; then
            printf "\033[32m%-36s %-30s %-8s %s\033[0m\n" "$rule_id" "$rule_name" "$priority" "$status"
          else
            printf "\033[33m%-36s %-30s %-8s %s\033[0m\n" "$rule_id" "$rule_name" "$priority" "$status"
          fi
        done
      else
        # No jq — print raw response
        printf '%s\n' "$response"
      fi
      ;;

    test-delegate)
      # Test delegate routing by sending a synthetic email to the claw plugin.
      # Usage: nself claw email-rules test-delegate --email <addr> --subject <text> [--body <text>]
      local email_addr="" subject_text="" body_text=""

      while [ $# -gt 0 ]; do
        case "$1" in
          --email)   email_addr="$2";   shift 2 ;;
          --subject) subject_text="$2"; shift 2 ;;
          --body)    body_text="$2";    shift 2 ;;
          *) shift ;;
        esac
      done

      if [ -z "$email_addr" ] || [ -z "$subject_text" ]; then
        printf "Usage: nself claw email-rules test-delegate --email <addr> --subject <text> [--body <text>]\n" >&2
        return 1
      fi

      local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"

      # Escape values for JSON embedding (backslash, then double-quote, then newline)
      local safe_email="" safe_subject="" safe_body=""
      safe_email=$(printf '%s' "$email_addr"   | sed 's/\\/\\\\/g; s/"/\\"/g')
      safe_subject=$(printf '%s' "$subject_text" | sed 's/\\/\\\\/g; s/"/\\"/g')
      safe_body=$(printf '%s' "$body_text"     | sed 's/\\/\\\\/g; s/"/\\"/g')

      local payload="{\"account_email\":\"cli-test@test.local\",\"gmail_thread_id\":\"test-thread-cli\",\"gmail_message_id\":\"test-msg-1\",\"from_email\":\"${safe_email}\",\"subject\":\"${safe_subject}\",\"body_text\":\"${safe_body}\"}"

      log_info "Sending test email to claw at ${claw_url}..."
      log_info "  From:    ${email_addr}"
      log_info "  Subject: ${subject_text}"

      local resp=""
      resp=$(curl -s -X POST "${claw_url}/internal/process_email" \
        -H "Content-Type: application/json" \
        -d "$payload" 2>/dev/null)

      if [ -z "$resp" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      # Extract and display result action + reply_text
      if command -v jq >/dev/null 2>&1; then
        local action="" reply_text=""
        action=$(printf '%s' "$resp" | jq -r '.action // "unknown"' 2>/dev/null || true)
        reply_text=$(printf '%s' "$resp" | jq -r '.reply_text // ""' 2>/dev/null || true)

        printf "\n"
        printf "  Action:     \033[1m%s\033[0m\n" "$action"
        if [ -n "$reply_text" ]; then
          printf "  Reply text: %s\n" "$reply_text"
        fi
        printf "\n"
      else
        printf '%s\n' "$resp"
      fi
      ;;

    help | --help | -h)
      claw_usage
      exit 0
      ;;

    *)
      cli_error "Unknown email-rules action: $subcmd"
      printf "Actions: list, test-delegate\n"
      exit 1
      ;;

  esac
}

# ============================================================================
# email-threads subcommand
# ============================================================================

cmd_claw_email_threads() {
  # List claw thread-to-session mappings from the mux plugin.
  # Usage: nself claw email-threads
  local mux_url="${NSELF_MUX_URL:-http://localhost:3711}"
  local response=""
  response=$(curl -s "${mux_url}/mux/claw-threads" 2>/dev/null)

  if [ -z "$response" ]; then
    cli_error "No response from mux service at ${mux_url}. Is it running?"
    return 1
  fi

  if command -v jq >/dev/null 2>&1; then
    local thread_count=""
    thread_count=$(printf '%s' "$response" | jq 'if type == "array" then length else 1 end' 2>/dev/null || true)

    if [ "${thread_count:-0}" -eq 0 ]; then
      log_info "No claw thread mappings found."
      return 0
    fi

    printf "\033[34m%-40s %-36s %s\033[0m\n" "Gmail Thread ID" "Claw Session ID" "Last Active"
    printf "%-40s %-36s %s\n" "----------------------------------------" "------------------------------------" "-----------"

    printf '%s' "$response" | jq -r '.[] | [.gmail_thread_id, (.claw_session_id // "-"), (.last_active_at // "-")] | @tsv' 2>/dev/null | \
    while IFS=$(printf '\t') read -r thread_id session_id last_active; do
      printf "%-40s %-36s %s\n" "$thread_id" "$session_id" "$last_active"
    done
  else
    printf '%s\n' "$response"
  fi
}

# ============================================================================
# Entry point
# ============================================================================

cmd_claw "$@"
