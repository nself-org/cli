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
  printf "nself claw — ɳClaw AI assistant management\n\n"
  printf "Usage: nself claw <subcommand> [options]\n\n"
  printf "Subcommands:\n"
  printf "  setup [--auto|--status|--reset]                       Run onboarding wizard (or --auto for unattended)\n"
  printf "  models list                                           List available local AI models\n"
  printf "  models install [--auto|--model <name>]                Install a local AI model\n"
  printf "  models status                                         Show download/ready state of local models\n"
  printf "  models remove <name>                                  Remove a local model\n"
  printf "  gemini add [chat_id]                                  Add a Gemini account via Google OAuth\n"
  printf "  gemini list                                           Show Gemini accounts with quota usage\n"
  printf "  gemini status                                         Show Gemini quota summary\n"
  printf "  gemini remove <email>                                 Remove a Gemini account\n"
  printf "  routing show                                          Show current AI routing config\n"
  printf "  routing set <task_class> <tier_order>                 Update routing for a task class\n"
  printf "  chat \"<message>\" [--model <name>] [--tier <tier>]      Send a one-shot chat message\n"
  printf "  usage [--today|--week|--month]                        Show AI usage log\n"
  printf "  stats [--today|--week|--month]                        Show AI usage summary + savings\n"
  printf "  admin status                                          Show stack state snapshot\n"
  printf "  admin enable --session <id>                           Enable admin mode for a session\n"
  printf "  admin context [--session <id>]                        Show admin context for a session\n"
  printf "  admin refresh                                         Force a fresh stack state snapshot\n"
  printf "  email-rules list                                      List ClawDelegate email routing rules\n"
  printf "  email-rules test-delegate --email <addr>              Test delegate routing for an email\n"
  printf "                            --subject <text>\n"
  printf "                            [--body <text>]\n"
  printf "  email-threads                                         List claw thread-to-session mappings\n\n"
  printf "Environment:\n"
  printf "  NSELF_MUX_URL           mux plugin base URL  (default: http://localhost:3711)\n"
  printf "  NSELF_CLAW_URL          claw plugin base URL (default: http://localhost:3713)\n"
  printf "  NSELF_AI_URL            ai plugin base URL   (default: http://localhost:3101)\n"
  printf "  PLUGIN_INTERNAL_SECRET  required for most commands\n\n"
  printf "Examples:\n"
  printf "  nself claw setup\n"
  printf "  nself claw setup --auto\n"
  printf "  nself claw models install --auto\n"
  printf "  nself claw gemini add\n"
  printf "  nself claw routing show\n"
  printf "  nself claw usage --today\n"
  printf "  nself claw stats --month\n"
  printf "  nself claw admin status\n"
}

# ============================================================================
# Top-level dispatcher
# ============================================================================

cmd_claw() {
  local subcommand="${1:-}"

  if [ -z "$subcommand" ]; then
    # T-1047: Show state-dependent message based on setup status
    local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
    local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"
    local setup_status="unknown"
    if [ -n "$internal_secret" ]; then
      local _setup_json=""
      _setup_json=$(curl -s "${claw_url}/claw/setup/status" 2>/dev/null)
      if [ -n "$_setup_json" ] && command -v jq >/dev/null 2>&1; then
        setup_status=$(printf '%s' "$_setup_json" | jq -r '.status // "unknown"' 2>/dev/null || printf "unknown")
      fi
    fi

    if [ "$setup_status" = "complete" ] || [ "$setup_status" = "skipped" ]; then
      printf "ɳClaw is ready.\n\n"
      printf "Quick actions:\n"
      printf "  nself claw usage --today      Show today's AI usage\n"
      printf "  nself claw models status      Check local model status\n"
      printf "  nself claw gemini list        Show Gemini account quota\n"
      printf "  nself claw routing show       Show routing config\n\n"
      printf "Run 'nself claw --help' for all commands.\n"
    else
      printf "ɳClaw is not set up yet.\n\n"
      printf "Run: nself claw setup\n"
      printf "  or: nself claw setup --auto    (unattended, installs best model + defaults)\n"
    fi
    exit 0
  fi

  shift

  case "$subcommand" in
    setup)
      cmd_claw_setup "$@"
      ;;
    email-rules)
      cmd_claw_email_rules "$@"
      ;;
    email-threads)
      cmd_claw_email_threads "$@"
      ;;
    usage)
      cmd_claw_usage "$@"
      ;;
    stats)
      cmd_claw_stats "$@"
      ;;
    admin)
      cmd_claw_admin "$@"
      ;;
    gemini)
      cmd_claw_gemini "$@"
      ;;
    routing)
      cmd_claw_routing "$@"
      ;;
    models)
      cmd_claw_models "$@"
      ;;
    chat)
      cmd_claw_chat "$@"
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
# T-1023: usage subcommand — show AI usage log
# ============================================================================

cmd_claw_usage() {
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

  if command -v jq >/dev/null 2>&1; then
    local count=""
    count=$(printf '%s' "$response" | jq 'if type == "array" then length else 0 end' 2>/dev/null || true)

    if [ "${count:-0}" -eq 0 ]; then
      log_info "No usage data found for period: ${period}."
      return 0
    fi

    printf "\033[34m%-12s %-22s %-6s %-6s %-8s %-10s %s\033[0m\n" \
      "Tier" "Provider/Model" "Prompt" "Compl" "Cached" "Cost" "Time"
    printf "%-12s %-22s %-6s %-6s %-8s %-10s %s\n" \
      "------------" "----------------------" "------" "------" "--------" "----------" "-------------------"

    printf '%s' "$response" | jq -r '.[] | [
      (.tier_source // "unknown"),
      ((.provider // "?") + "/" + (.model // "?")),
      (.prompt_tokens | tostring),
      (.completion_tokens | tostring),
      (if .is_cached then "yes" else "no" end),
      ("$" + (.cost_usd | tostring)),
      (.created_at // "-")
    ] | @tsv' 2>/dev/null | \
    while IFS=$(printf '\t') read -r tier provmodel pt ct cached cost created; do
      case "$tier" in
        local)       color="\033[32m" ;;
        free_gemini) color="\033[33m" ;;
        cache)       color="\033[36m" ;;
        api_key)     color="\033[31m" ;;
        *)           color="\033[0m"  ;;
      esac
      printf "${color}%-12s %-22s %-6s %-6s %-8s %-10s %s\033[0m\n" \
        "$tier" "$provmodel" "$pt" "$ct" "$cached" "$cost" "$created"
    done
  else
    printf '%s\n' "$response"
  fi
}

# ============================================================================
# T-1023: stats subcommand — AI usage summary + savings
# ============================================================================

cmd_claw_stats() {
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

  if command -v jq >/dev/null 2>&1; then
    local total_req="" total_tok="" total_cost="" cache_rate="" savings=""
    local local_ct="" gemini_ct="" api_ct="" cache_ct=""
    total_req=$(printf '%s' "$response"  | jq -r '.total_requests // 0')
    total_tok=$(printf '%s' "$response"  | jq -r '.total_tokens // 0')
    total_cost=$(printf '%s' "$response" | jq -r '.total_cost_usd // 0')
    cache_rate=$(printf '%s' "$response" | jq -r '(.cache_hit_rate * 100 | round | tostring) + "%"')
    savings=$(printf '%s' "$response"    | jq -r '"$" + (.estimated_savings_usd | tostring)')
    local_ct=$(printf '%s' "$response"   | jq -r '.by_tier_source.local // 0')
    gemini_ct=$(printf '%s' "$response"  | jq -r '.by_tier_source.free_gemini // 0')
    api_ct=$(printf '%s' "$response"     | jq -r '.by_tier_source.api_key // 0')
    cache_ct=$(printf '%s' "$response"   | jq -r '.by_tier_source.cache // 0')

    printf "\nnClaw AI Usage Summary (%s)\n" "$period"
    printf "%-30s %s\n" "------------------------------" "-------"
    printf "  Total requests:          %s\n" "$total_req"
    printf "  Total tokens:            %s\n" "$total_tok"
    printf "  Total cost:              \$%s\n" "$total_cost"
    printf "  Cache hit rate:          %s\n" "$cache_rate"
    printf "  Estimated savings:       %s\n" "$savings"
    printf "\nTier Breakdown:\n"
    printf "  \033[32mLocal (free):            %s\033[0m\n" "$local_ct"
    printf "  \033[33mFree Gemini:             %s\033[0m\n" "$gemini_ct"
    printf "  \033[36mCache hits:              %s\033[0m\n" "$cache_ct"
    printf "  \033[31mPaid API key:            %s\033[0m\n" "$api_ct"
    printf "\n"
  else
    printf '%s\n' "$response"
  fi
}

# ============================================================================
# T-1087: admin subcommand — stack context engine management
# ============================================================================

cmd_claw_admin() {
  local subcmd="${1:-}"

  if [ -z "$subcmd" ]; then
    cli_error "admin action required"
    printf "Actions: status, enable, context, refresh\n"
    exit 1
  fi

  shift

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  if [ -z "$internal_secret" ]; then
    cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
    return 1
  fi

  case "$subcmd" in

    status)
      # Show the latest stack state snapshot.
      # Usage: nself claw admin status
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/admin/status" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        local ram_used="" ram_total="" cpu_cores="" disk_used="" disk_total=""
        local table_count="" captured_at=""
        ram_used=$(printf '%s' "$response"    | jq -r '.ram_used_gb // 0')
        ram_total=$(printf '%s' "$response"   | jq -r '.ram_total_gb // 0')
        cpu_cores=$(printf '%s' "$response"   | jq -r '.cpu_cores // 0')
        disk_used=$(printf '%s' "$response"   | jq -r '.disk_used_gb // 0')
        disk_total=$(printf '%s' "$response"  | jq -r '.disk_total_gb // 0')
        table_count=$(printf '%s' "$response" | jq -r '.table_count // 0')
        captured_at=$(printf '%s' "$response" | jq -r '.captured_at // "-"')

        printf "\nnClaw Stack Status\n"
        printf "%-30s %s\n" "------------------------------" "-------"
        printf "  RAM:         %.1f GB / %.1f GB\n" "$ram_used" "$ram_total"
        printf "  CPU cores:   %s\n" "$cpu_cores"
        printf "  Disk:        %.1f GB / %.1f GB\n" "$disk_used" "$disk_total"
        printf "  DB tables:   %s\n" "$table_count"
        printf "  Captured at: %s\n\n" "$captured_at"

        printf "Services:\n"
        printf '%s' "$response" | jq -r '.services[] | [.name, (if .running then "running" else "stopped" end)] | @tsv' 2>/dev/null | \
        while IFS=$(printf '\t') read -r svc_name svc_status; do
          if [ "$svc_status" = "running" ]; then
            printf "  \033[32m%-20s %s\033[0m\n" "$svc_name" "$svc_status"
          else
            printf "  \033[31m%-20s %s\033[0m\n" "$svc_name" "$svc_status"
          fi
        done
        printf "\n"
      else
        printf '%s\n' "$response"
      fi
      ;;

    enable)
      # Enable admin mode for a session.
      # Usage: nself claw admin enable --session <uuid>
      local session_id=""

      while [ $# -gt 0 ]; do
        case "$1" in
          --session) session_id="$2"; shift 2 ;;
          *) shift ;;
        esac
      done

      if [ -z "$session_id" ]; then
        cli_error "Missing --session <uuid>"
        printf "Usage: nself claw admin enable --session <uuid>\n" >&2
        return 1
      fi

      local safe_session=""
      safe_session=$(printf '%s' "$session_id" | sed 's/[^a-fA-F0-9-]//g')

      local response=""
      response=$(curl -s -X POST \
        -H "x-internal-token: ${internal_secret}" \
        -H "Content-Type: application/json" \
        -d "{\"session_id\":\"${safe_session}\",\"enabled\":true}" \
        "${claw_url}/claw/admin/mode" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        local ok=""
        ok=$(printf '%s' "$response" | jq -r '.ok // false' 2>/dev/null || true)
        if [ "$ok" = "true" ]; then
          log_success "Admin mode enabled for session ${safe_session}."
        else
          local err=""
          err=$(printf '%s' "$response" | jq -r '.error // "unknown error"' 2>/dev/null || true)
          cli_error "Failed to enable admin mode: ${err}"
          return 1
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    context)
      # Show admin context (snapshot + schema info) for a session.
      # Usage: nself claw admin context [--session <uuid>]
      local session_id="none"

      while [ $# -gt 0 ]; do
        case "$1" in
          --session) session_id="$2"; shift 2 ;;
          *) shift ;;
        esac
      done

      local safe_session=""
      safe_session=$(printf '%s' "$session_id" | sed 's/[^a-fA-F0-9-]//g')

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/admin/context?session_id=${safe_session}" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        printf '%s' "$response" | jq . 2>/dev/null || printf '%s\n' "$response"
      else
        printf '%s\n' "$response"
      fi
      ;;

    refresh)
      # Force a fresh stack state snapshot (bypass the 5-minute cache).
      # Usage: nself claw admin refresh
      local response=""
      response=$(curl -s -X POST \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/admin/refresh" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        local captured_at=""
        captured_at=$(printf '%s' "$response" | jq -r '.captured_at // "-"' 2>/dev/null || true)
        log_success "Stack snapshot refreshed at ${captured_at}."
      else
        printf '%s\n' "$response"
      fi
      ;;

    help | --help | -h)
      claw_usage
      exit 0
      ;;

    *)
      cli_error "Unknown admin action: $subcmd"
      printf "Actions: status, enable, context, refresh\n"
      exit 1
      ;;

  esac
}

# ============================================================================
# T-1042: gemini — Gemini account management (add, list, status, remove)
# ============================================================================

cmd_claw_gemini() {
  local subcmd="${1:-list}"
  shift || true

  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local claw_url="${NSELF_CLAW_URL:-http://localhost:3711}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in
    add)
      # Start Google OAuth flow for Gemini.
      # Opens browser or prints URL if browser not available.
      local chat_id="${1:-0}"
      local base_url="${CLAW_BASE_URL:-}"

      if [ -z "$base_url" ]; then
        cli_error "CLAW_BASE_URL not set. Set it to your public claw URL (e.g. https://api.yourdomain.com)"
        return 1
      fi

      local oauth_url="${base_url}/claw/oauth/google/start?service=gemini&chat_id=${chat_id}&message_id=0&label=gemini-cli"
      printf "\033[0;34m[INFO]\033[0m Open this URL to authorize Gemini:\n\n  %s\n\n" "$oauth_url"

      if command -v open >/dev/null 2>&1; then
        open "$oauth_url" 2>/dev/null || true
      elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "$oauth_url" 2>/dev/null || true
      fi
      ;;

    list)
      # List configured Gemini accounts with quota info.
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/gemini/accounts" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from ai service at ${ai_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        printf "\n\033[1mGemini Accounts\033[0m\n"
        printf "%-30s %-15s %-8s %-8s\n" "EMAIL" "TOKENS TODAY" "RPM" "ENABLED"
        printf "%-30s %-15s %-8s %-8s\n" "-----" "------------" "---" "-------"
        printf '%s' "$response" | jq -r '.[] | [.account_email, (.tokens_used_today|tostring), (.rpm_used_this_minute|tostring), (.enabled|tostring)] | @tsv' 2>/dev/null \
        | while IFS='	' read -r email tokens rpm enabled; do
            printf "%-30s %-15s %-8s %-8s\n" "$email" "$tokens" "$rpm" "$enabled"
          done
        printf "\nDaily limit: 1,000,000 tokens | RPM limit: 15\n\n"
      else
        printf '%s\n' "$response"
      fi
      ;;

    status)
      # Show quota summary for all Gemini accounts.
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/gemini/accounts" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from ai service at ${ai_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        local total_accounts="" total_tokens=""
        total_accounts=$(printf '%s' "$response" | jq 'length' 2>/dev/null)
        total_tokens=$(printf '%s' "$response" | jq '[.[].tokens_used_today] | add // 0' 2>/dev/null)
        log_info "Gemini accounts: ${total_accounts} | Total tokens used today: ${total_tokens} / 1,000,000 per account"
        printf '%s' "$response" | jq -r '.[] | "  \(.account_email): \(.tokens_used_today) tokens, \(.rpm_used_this_minute) rpm, enabled=\(.enabled)"' 2>/dev/null
      else
        printf '%s\n' "$response"
      fi
      ;;

    remove)
      local email="${1:-}"
      if [ -z "$email" ]; then
        printf "Usage: nself claw gemini remove <email>\n" >&2
        return 1
      fi

      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local response=""
      response=$(curl -s -X DELETE \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/gemini/accounts/${email}" 2>/dev/null)

      printf '%s\n' "$response"
      log_success "Gemini account ${email} removed."
      ;;

    help | --help | -h)
      printf "nself claw gemini — Gemini account management\n\n"
      printf "Usage: nself claw gemini <add|list|status|remove> [options]\n\n"
      printf "Subcommands:\n"
      printf "  add [chat_id]   Start Google OAuth flow (opens browser)\n"
      printf "  list            Show configured accounts with quota\n"
      printf "  status          Show quota summary\n"
      printf "  remove <email>  Remove a Gemini account\n\n"
      ;;

    *)
      cli_error "Unknown gemini action: $subcmd"
      printf "Actions: add, list, status, remove\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# T-1046: nself claw setup — onboarding wizard
# ============================================================================

cmd_claw_setup() {
  local auto=0
  local show_status=0
  local do_reset=0

  while [ $# -gt 0 ]; do
    case "$1" in
      --auto)    auto=1    ;;
      --status)  show_status=1 ;;
      --reset)   do_reset=1 ;;
      -h | --help)
        printf "nself claw setup — ɳClaw onboarding wizard\n\n"
        printf "Usage: nself claw setup [--auto|--status|--reset]\n\n"
        printf "Options:\n"
        printf "  (none)     Run interactive wizard step by step\n"
        printf "  --auto     Install best model + defaults with no prompts\n"
        printf "  --status   Show current wizard state\n"
        printf "  --reset    Reset wizard to step 0 (re-run from scratch)\n"
        return 0
        ;;
      *) cli_error "Unknown option: $1"; return 1 ;;
    esac
    shift
  done

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  if [ -z "$internal_secret" ]; then
    cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
    return 1
  fi

  # --status: show current wizard state
  if [ "$show_status" -eq 1 ]; then
    local status_json=""
    status_json=$(curl -s "${claw_url}/claw/setup/status" 2>/dev/null)
    if [ -z "$status_json" ]; then
      cli_error "Could not reach ɳClaw at ${claw_url}. Is it running?"
      return 1
    fi
    if command -v jq >/dev/null 2>&1; then
      local step="" wiz_status="" model="" gemini_count=""
      step=$(printf '%s' "$status_json" | jq -r '.step // 0')
      wiz_status=$(printf '%s' "$status_json" | jq -r '.status // "unknown"')
      model=$(printf '%s' "$status_json" | jq -r '.model_selected // "(none)"')
      gemini_count=$(printf '%s' "$status_json" | jq -r '.gemini_accounts_added // 0')
      printf "ɳClaw Setup Status\n"
      printf "  Step:             %s/7\n" "$step"
      printf "  Status:           %s\n" "$wiz_status"
      printf "  Model selected:   %s\n" "$model"
      printf "  Gemini accounts:  %s\n" "$gemini_count"
    else
      printf '%s\n' "$status_json"
    fi
    return 0
  fi

  # --reset: reset wizard
  if [ "$do_reset" -eq 1 ]; then
    curl -s -X POST "${claw_url}/claw/setup/reset" >/dev/null 2>&1
    log_success "Setup wizard reset to step 0."
    return 0
  fi

  # --auto: fully automated setup with best defaults
  if [ "$auto" -eq 1 ]; then
    printf "ɳClaw Auto-Setup\n"
    printf "=================\n\n"

    # Step 1: Detect resources + install best model
    printf "Step 1/4: Detecting server resources...\n"
    local models_json=""
    models_json=$(curl -s \
      -H "x-internal-token: ${internal_secret}" \
      "${ai_url}/ai/models/local" 2>/dev/null)

    local recommended=""
    if command -v jq >/dev/null 2>&1; then
      recommended=$(printf '%s' "$models_json" | jq -r '.recommended // ""' 2>/dev/null || true)
      local free_ram=""
      free_ram=$(printf '%s' "$models_json" | jq -r '.free_ram_gb // "?"' 2>/dev/null || true)
      printf "  Free RAM: %s GB\n" "$free_ram"
    fi

    if [ -n "$recommended" ]; then
      printf "Step 2/4: Installing model: %s...\n" "$recommended"
      curl -s -X POST \
        -H "x-internal-token: ${internal_secret}" \
        -H "Content-Type: application/json" \
        -d "{\"model\":\"${recommended}\"}" \
        "${ai_url}/ai/models/local/install" >/dev/null 2>&1
      printf "  Model install started in background.\n"
      printf "  Check progress: nself claw models status\n"
    else
      printf "Step 2/4: No model recommended (not enough RAM or AI plugin unavailable).\n"
      printf "  You can install manually: nself claw models install --model <name>\n"
    fi

    # Step 3: Skip Google OAuth in auto mode (requires browser)
    printf "Step 3/4: Google OAuth skipped (--auto mode). Add later: nself claw gemini add\n"

    log_success "ɳClaw auto-setup complete."
    printf "\n  Next steps:\n"
    printf "  - Add a Google account:  nself claw gemini add\n"
    printf "  - Add API keys:          nself claw gemini add (for OAuth) or /add_key in Telegram\n"
    printf "  - Check routing:         nself claw routing show\n"
    printf "  - Model status:          nself claw models status\n"
    return 0
  fi

  # Interactive wizard
  printf "ɳClaw Setup Wizard\n"
  printf "==================\n\n"
  printf "This wizard will configure your AI assistant.\n"
  printf "Press Enter to continue each step, or type 'skip' to skip it.\n\n"

  # Step 1: Model detection
  printf "Step 1: Detect server resources and install a local AI model\n"
  printf "  This gives you free, private AI with no API key.\n"
  printf "  [Enter to continue, 'skip' to skip]: "
  local input=""
  read -r input
  if [ "$input" != "skip" ]; then
    local models_json2=""
    models_json2=$(curl -s \
      -H "x-internal-token: ${internal_secret}" \
      "${ai_url}/ai/models/local" 2>/dev/null)
    local rec=""
    if command -v jq >/dev/null 2>&1; then
      rec=$(printf '%s' "$models_json2" | jq -r '.recommended // ""' 2>/dev/null || true)
      local ram2=""
      ram2=$(printf '%s' "$models_json2" | jq -r '.free_ram_gb // "?"' 2>/dev/null || true)
      printf "  Free RAM: %s GB\n" "$ram2"
    fi
    if [ -n "$rec" ]; then
      printf "  Recommended model: %s\n" "$rec"
      printf "  Install it? [Y/n]: "
      read -r yn
      if [ "$yn" != "n" ] && [ "$yn" != "N" ]; then
        curl -s -X POST \
          -H "x-internal-token: ${internal_secret}" \
          -H "Content-Type: application/json" \
          -d "{\"model\":\"${rec}\"}" \
          "${ai_url}/ai/models/local/install" >/dev/null 2>&1
        log_success "Model install started. Check progress: nself claw models status"
      fi
    else
      printf "  No model recommended. Install manually later: nself claw models install --model <name>\n"
    fi
  else
    printf "  Skipped.\n"
  fi

  printf "\n"

  # Step 2: Google OAuth
  printf "Step 2: Link a Google account for free Gemini access\n"
  printf "  This gives you 1M free tokens/day per account (up to 3 accounts).\n"
  printf "  [Enter to get the OAuth URL, 'skip' to skip]: "
  read -r input
  if [ "$input" != "skip" ]; then
    local claw_base_url=""
    local _base_json=""
    _base_json=$(curl -s "${claw_url}/health" 2>/dev/null)
    if [ -n "$_base_json" ] && command -v jq >/dev/null 2>&1; then
      claw_base_url=$(printf '%s' "$_base_json" | jq -r '.base_url // ""' 2>/dev/null || printf "")
    fi
    if [ -z "$claw_base_url" ]; then
      printf "  Could not determine your server's public URL.\n"
      printf "  Set CLAW_BASE_URL in your .env and rebuild, then:\n"
      printf "  nself claw gemini add\n"
    else
      printf "  Open this URL in your browser:\n"
      printf "  %s/claw/oauth/google/start\n" "$claw_base_url"
      printf "\n  After authorizing, your Gemini account will be linked automatically.\n"
      printf "  Press Enter when done, or 'skip': "
      read -r input
    fi
  else
    printf "  Skipped. Add later with: nself claw gemini add\n"
  fi

  printf "\n"

  # Step 3: Review routing
  printf "Step 3: Review routing configuration\n"
  printf "  [Enter to see current routing, 'skip' to skip]: "
  read -r input
  if [ "$input" != "skip" ]; then
    cmd_claw_routing show 2>/dev/null || printf "  (routing info unavailable)\n"
  else
    printf "  Skipped. View later: nself claw routing show\n"
  fi

  printf "\n"
  log_success "ɳClaw setup complete!"
  printf "\n  Next steps:\n"
  printf "  - Check model status:  nself claw models status\n"
  printf "  - Add API keys:        Start a Telegram conversation and use /add_key\n"
  printf "  - Check routing:       nself claw routing show\n"
}

# ============================================================================
# nself claw models — local model management (T-1046)
# ============================================================================

cmd_claw_models() {
  local subcmd="${1:-list}"
  shift || true

  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in

    list)
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/models/local" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "Could not reach AI plugin at ${ai_url}. Is it running?"
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        local recommended=""
        recommended=$(printf '%s' "$response" | jq -r '.recommended // ""' 2>/dev/null || true)
        printf "%-30s %-8s %-8s %s\n" "Model" "RAM req" "Status" "Notes"
        printf "%-30s %-8s %-8s %s\n" "-----" "-------" "------" "-----"
        printf '%s' "$response" | jq -r '
          .catalog[] |
          [.ollama_tag,
           ((.ram_gb_required | tostring) + " GB"),
           (.status // "-"),
           (if .recommended then "★ recommended" else "" end)
          ] | @tsv' 2>/dev/null | \
          while IFS=$'\t' read -r tag ram_req status notes; do
            printf "%-30s %-8s %-8s %s\n" "$tag" "$ram_req" "$status" "$notes"
          done
      else
        printf '%s\n' "$response"
      fi
      ;;

    install)
      local auto=0
      local model_name=""

      while [ $# -gt 0 ]; do
        case "$1" in
          --auto)           auto=1 ;;
          --model | -m)     model_name="${2:-}"; shift ;;
          *) cli_error "Unknown option: $1"; return 1 ;;
        esac
        shift
      done

      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set."
        return 1
      fi

      local body="{}"
      if [ "$auto" -eq 1 ]; then
        body='{"auto":true}'
      elif [ -n "$model_name" ]; then
        body="{\"model\":\"${model_name}\"}"
      else
        cli_error "Specify --auto or --model <name>"
        return 1
      fi

      local response=""
      response=$(curl -s -X POST \
        -H "x-internal-token: ${internal_secret}" \
        -H "Content-Type: application/json" \
        -d "$body" \
        "${ai_url}/ai/models/local/install" 2>/dev/null)

      if command -v jq >/dev/null 2>&1; then
        local started_model=""
        started_model=$(printf '%s' "$response" | jq -r '.model // ""' 2>/dev/null || true)
        if [ -n "$started_model" ]; then
          log_success "Model install started: ${started_model}"
          printf "  Track progress: nself claw models status\n"
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    status)
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/models/local" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "Could not reach AI plugin at ${ai_url}."
        return 1
      fi

      if command -v jq >/dev/null 2>&1; then
        printf "Local Model Status\n"
        printf "%-30s %-12s %s\n" "Model" "Status" "Size"
        printf "%-30s %-12s %s\n" "-----" "------" "----"
        printf '%s' "$response" | jq -r '
          .models[] |
          [.model_name, .status,
           (if .size_bytes then (.size_bytes / 1073741824 | tostring | .[0:5]) + " GB" else "-" end)
          ] | @tsv' 2>/dev/null | \
          while IFS=$'\t' read -r name status size; do
            printf "%-30s %-12s %s\n" "$name" "$status" "$size"
          done
        local ollama_reachable=""
        ollama_reachable=$(printf '%s' "$response" | jq -r '.ollama_reachable // false' 2>/dev/null || true)
        printf "\nOllama reachable: %s\n" "$ollama_reachable"
      else
        printf '%s\n' "$response"
      fi
      ;;

    remove)
      local model_name="${1:-}"
      if [ -z "$model_name" ]; then
        cli_error "Specify model name: nself claw models remove <name>"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set."
        return 1
      fi
      local response=""
      response=$(curl -s -X DELETE \
        -H "x-internal-token: ${internal_secret}" \
        "${ai_url}/ai/models/local/remove/${model_name}" 2>/dev/null)
      log_success "Model remove requested: ${model_name}"
      printf '%s\n' "$response"
      ;;

    -h | --help | help)
      printf "nself claw models — local AI model management\n\n"
      printf "Usage: nself claw models <list|install|status|remove> [options]\n\n"
      printf "Subcommands:\n"
      printf "  list                  List all available models with RAM requirements\n"
      printf "  install --auto        Install the best model for this server\n"
      printf "  install --model <n>   Install a specific model by Ollama tag\n"
      printf "  status                Show download status of installed models\n"
      printf "  remove <name>         Remove a model\n"
      ;;

    *)
      cli_error "Unknown models action: $subcmd"
      printf "Actions: list, install, status, remove\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# T-1031 alias: nself claw routing — alias to nself ai routing
# ============================================================================

cmd_claw_routing() {
  # nself claw routing is an alias for nself ai routing
  # Source ai.sh and delegate
  local ai_sh
  ai_sh="$(dirname "${BASH_SOURCE[0]}")/ai.sh"
  if [ -f "$ai_sh" ]; then
    # shellcheck source=/dev/null
    source "$ai_sh" 2>/dev/null || true
    cmd_ai_routing "$@"
  else
    cli_error "ai.sh not found at ${ai_sh}"
    return 1
  fi
}

# ============================================================================
# T-1072: chat subcommand — quick one-shot chat message
# ============================================================================

cmd_claw_chat() {
  local message=""
  local model=""
  local tier=""
  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    printf "Usage: nself claw chat \"<message>\" [--model <name>] [--tier local|free_gemini|api_key]\n\n"
    printf "Send a one-shot chat message to the AI and print the response.\n\n"
    printf "Options:\n"
    printf "  --model <name>   Use a specific model (e.g. phi4, llama3)\n"
    printf "  --tier <tier>    Force a routing tier: local, free_gemini, api_key\n\n"
    printf "Environment:\n"
    printf "  NSELF_AI_URL           AI plugin base URL (default: http://localhost:3101)\n"
    printf "  PLUGIN_INTERNAL_SECRET required\n\n"
    printf "Examples:\n"
    printf "  nself claw chat \"What is Docker?\"\n"
    printf "  nself claw chat \"Explain RLS\" --tier local\n"
    printf "  nself claw chat \"Write a haiku\" --model phi4\n"
    return 0
  fi

  # First positional arg is the message
  if [ $# -gt 0 ]; then
    case "$1" in
      --*) ;;
      *) message="$1"; shift ;;
    esac
  fi

  # Parse remaining flags
  while [ $# -gt 0 ]; do
    case "$1" in
      --model) model="${2:-}"; shift 2 ;;
      --model=*) model="${1#*=}"; shift ;;
      --tier) tier="${2:-}"; shift 2 ;;
      --tier=*) tier="${1#*=}"; shift ;;
      *) shift ;;
    esac
  done

  if [ -z "$message" ]; then
    cli_error "Message required. Usage: nself claw chat \"<message>\""
    return 1
  fi

  if [ -z "$internal_secret" ]; then
    cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
    return 1
  fi

  # Build JSON body — avoid associative arrays for Bash 3.2 compat
  local body='{"message":"'
  # Escape special JSON characters in the message
  local escaped_message
  escaped_message=$(printf '%s' "$message" | sed 's/\\/\\\\/g; s/"/\\"/g; s/$/\\n/; s/\t/\\t/g' | tr -d '\n' | sed 's/\\n$//')
  body="${body}${escaped_message}\",\"stream\":false"
  if [ -n "$model" ]; then
    body="${body},\"model\":\"${model}\""
  fi
  if [ -n "$tier" ]; then
    body="${body},\"preferred_tier\":\"${tier}\""
  fi
  body="${body}}"

  local response=""
  response=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -H "x-internal-token: ${internal_secret}" \
    -d "$body" \
    "${ai_url}/ai/chat" 2>/dev/null)

  if [ -z "$response" ]; then
    cli_error "No response from AI service at ${ai_url}. Is it running?"
    return 1
  fi

  if command -v jq >/dev/null 2>&1; then
    # Extract response text and tier info
    local reply tier_src model_used latency
    reply=$(printf '%s' "$response" | jq -r '.response // .content // .message // empty' 2>/dev/null)
    tier_src=$(printf '%s' "$response" | jq -r '.tier_source // empty' 2>/dev/null)
    model_used=$(printf '%s' "$response" | jq -r '.model // empty' 2>/dev/null)
    latency=$(printf '%s' "$response" | jq -r '.latency_ms // empty' 2>/dev/null)

    if [ -n "$reply" ]; then
      printf '%s\n' "$reply"
      if [ -n "$tier_src" ]; then
        printf "\n"
        if [ -n "$model_used" ] && [ -n "$latency" ]; then
          printf "\033[2m%s • %s • %sms\033[0m\n" "$tier_src" "$model_used" "$latency"
        elif [ -n "$tier_src" ]; then
          printf "\033[2m%s\033[0m\n" "$tier_src"
        fi
      fi
    else
      # Fallback: print raw response
      printf '%s\n' "$response"
    fi
  else
    printf '%s\n' "$response"
  fi
}

# ============================================================================
# Entry point
# ============================================================================

cmd_claw "$@"
