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
  printf "  playbooks list                                        List incident response playbooks\n"
  printf "  playbooks add --pattern <text> --steps-file <path>   Add a new playbook\n"
  printf "  playbooks test --id <uuid> [--dry-run]               Test a playbook\n"
  printf "  usage [--today|--week|--month]                        Show AI usage log\n"
  printf "  stats [--json]                                        Show AI usage stats (today + this month)\n"
  printf "  admin status                                          Show stack state snapshot\n"
  printf "  admin enable --session <id>                           Enable admin mode for a session\n"
  printf "  admin context [--session <id>]                        Show admin context for a session\n"
  printf "  admin refresh                                         Force a fresh stack state snapshot\n"
  printf "  voice status                                          Show voice feature status + Whisper install\n"
  printf "  voice enable                                          Enable STT/TTS voice features\n"
  printf "  voice test [--text <text>]                            Test voice synthesis (TTS)\n"
  printf "  knowledge search <query> [--category <cat>] [--top N] Search the nSelf knowledge base\n"
  printf "  knowledge list [<category>]                           List knowledge chunks (optionally by category)\n"
  printf "  knowledge version                                     Show knowledge base version + stats\n"
  printf "  knowledge note add --chunk <id> --note <text>         Add an operator note to a chunk\n"
  printf "  knowledge note list [--chunk <id>]                    List operator notes\n"
  printf "  knowledge note delete --id <uuid>                     Delete an operator note\n"
  printf "  api keys list [--json]                                List gateway API keys\n"
  printf "  api keys create --name <n> [--admin] [--rpm <N>]     Create a new gateway API key\n"
  printf "  api keys revoke <id>                                  Revoke (delete) a gateway API key\n"
  printf "  api usage [--key <id>] [--json]                       Show gateway usage (all keys or one key)\n"
  printf "  api test [--url <url>] [--key <key>] [--verbose]      Verify the gateway endpoint responds\n"
  printf "  memories list --user <id>                             List stored memories for a user\n"
  printf "  memories add  --user <id> --content <text>           Add an explicit memory\n"
  printf "  memories delete --id <uuid>                          Delete a memory by ID\n"
  printf "  memories clear --user <id>                           Clear all memories for a user\n"
  printf "  memories stats --user <id>                           Show memory counts and limits\n"
  printf "  proactive status                                      List all scheduled jobs and state\n"
  printf "  proactive enable  <job_type>                         Enable a proactive job\n"
  printf "  proactive disable <job_type>                         Disable a proactive job\n"
  printf "  proactive run                                         Preview next morning digest\n"
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
  printf "  nself claw api keys list\n"
  printf "  nself claw api keys create --name myapp --rpm 60\n"
  printf "  nself claw api test\n"
  printf "  nself claw voice status\n"
  printf "  nself claw voice test --text 'Hello from ɳClaw'\n"
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
    knowledge)
      cmd_claw_knowledge "$@"
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
    playbooks)
      cmd_claw_playbooks "$@"
      ;;
    voice)
      cmd_claw_voice "$@"
      ;;
    api)
      cmd_claw_api "$@"
      ;;
    memories)
      cmd_claw_memories "$@"
      ;;
    proactive)
      cmd_claw_proactive "$@"
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
  local json_flag=false
  local ai_url="${NSELF_AI_URL:-http://localhost:3101}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  while [ $# -gt 0 ]; do
    case "$1" in
      --json)   json_flag=true;  shift ;;
      --today|--week|--month) shift ;;  # ignored: endpoint always shows current month
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
    "${ai_url}/ai/stats/summary" 2>/dev/null)

  if [ -z "$response" ]; then
    cli_error "No response from ai service at ${ai_url}. Is it running?"
    return 1
  fi

  # --json: output raw JSON for scripting (T-1078)
  if [ "$json_flag" = "true" ]; then
    printf '%s\n' "$response"
    return 0
  fi

  if command -v jq >/dev/null 2>&1; then
    local req_today="" req_month="" tok_today="" cost_today="" cost_month=""
    local local_pct="" gemini_pct="" api_pct="" cache_pct="" savings=""
    req_today=$(printf '%s' "$response"  | jq -r '.requests_today // 0')
    req_month=$(printf '%s' "$response"  | jq -r '.requests_month // 0')
    tok_today=$(printf '%s' "$response"  | jq -r '.tokens_today // 0')
    cost_today=$(printf '%s' "$response" | jq -r '(.cost_usd_today // 0) | "*100|round/100" | @text' 2>/dev/null \
                 || printf '%s' "$response" | jq -r '.cost_usd_today // 0')
    cost_month=$(printf '%s' "$response" | jq -r '.cost_usd_month // 0')
    local_pct=$(printf '%s' "$response"  | jq -r '(.local_requests_pct // 0 | round | tostring) + "%"')
    gemini_pct=$(printf '%s' "$response" | jq -r '(.free_gemini_pct // 0 | round | tostring) + "%"')
    api_pct=$(printf '%s' "$response"    | jq -r '(.paid_api_pct // 0 | round | tostring) + "%"')
    cache_pct=$(printf '%s' "$response"  | jq -r '(.cache_hit_rate // 0 | round | tostring) + "%"')
    savings=$(printf '%s' "$response"    | jq -r '"$" + (.savings_usd_month // 0 | tostring)')

    printf "\nnClaw AI Usage Stats\n"
    printf "%s\n" "-----------------------------"
    printf "Today:\n"
    printf "  Requests:                %s\n" "$req_today"
    printf "  Tokens:                  %s\n" "$tok_today"
    printf "  Cost:                    \$%s\n" "$cost_today"
    printf "\nThis Month:\n"
    printf "  Requests:                %s\n" "$req_month"
    printf "  Cost:                    \$%s\n" "$cost_month"
    printf "  Estimated savings:       %s\n" "$savings"
    printf "\nTier Breakdown (month):\n"
    printf "  \033[32mLocal (free):            %s\033[0m\n" "$local_pct"
    printf "  \033[33mFree Gemini:             %s\033[0m\n" "$gemini_pct"
    printf "  \033[36mCache hits:              %s\033[0m\n" "$cache_pct"
    printf "  \033[31mPaid API key:            %s\033[0m\n" "$api_pct"

    # Top models (if present)
    local top_models=""
    top_models=$(printf '%s' "$response" | jq -r '.top_models[]? | "  " + .model + ": " + (.requests | tostring)' 2>/dev/null)
    if [ -n "$top_models" ]; then
      printf "\nTop Models (month):\n"
      printf '%s\n' "$top_models"
    fi
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
# T-1076: playbooks subcommand — incident response playbook management
# ============================================================================

cmd_claw_playbooks() {
  local subcmd="${1:-list}"
  shift || true

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in
    --help|-h)
      printf "Usage: nself claw playbooks <action> [options]\n\n"
      printf "Actions:\n"
      printf "  list                            List all incident response playbooks\n"
      printf "  add --pattern <text> \\          Add a new playbook\n"
      printf "      --steps-file <path>\n"
      printf "  edit --id <uuid> [--pattern <text>] [--steps-file <path>]\n"
      printf "       [--confirm|--no-confirm]    Update an existing playbook\n"
      printf "  test --id <uuid> [--dry-run]    Test a playbook (dry-run: show steps only)\n\n"
      printf "Examples:\n"
      printf "  nself claw playbooks list\n"
      printf "  nself claw playbooks test --id abc123 --dry-run\n"
      printf "  nself claw playbooks edit --id abc123 --pattern \"oom|killed\"\n"
      return 0
      ;;
    list)
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/playbooks" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local count
        count=$(printf '%s' "$response" | jq 'if type == "array" then length else (.playbooks | length) end' 2>/dev/null || printf "0")
        if [ "${count:-0}" -eq 0 ]; then
          printf "No playbooks configured.\n"
          printf "Add one with: nself claw playbooks add --pattern \"OOM\" --steps-file steps.json\n"
          return 0
        fi
        printf "\n\033[34m%-38s %-30s %s\033[0m\n" "ID" "Pattern" "Destructive"
        printf "%-38s %-30s %s\n" "--------------------------------------" "------------------------------" "-----------"
        printf '%s' "$response" | jq -r '(if type == "array" then . else .playbooks end) // [] | .[] | [
          (.id // "-"),
          (.pattern // "-"),
          (if .requires_confirmation then "yes (confirm)" else "no" end)
        ] | @tsv' 2>/dev/null | \
        while IFS=$(printf '\t') read -r id pattern destr; do
          printf "%-38s %-30s %s\n" "$id" "$pattern" "$destr"
        done
      else
        printf '%s\n' "$response"
      fi
      ;;
    add)
      local pattern="" steps_file=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --pattern) pattern="${2:-}"; shift 2 ;;
          --pattern=*) pattern="${1#*=}"; shift ;;
          --steps-file) steps_file="${2:-}"; shift 2 ;;
          --steps-file=*) steps_file="${1#*=}"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$pattern" ]; then
        cli_error "--pattern required"
        return 1
      fi
      if [ -z "$steps_file" ]; then
        cli_error "--steps-file required"
        return 1
      fi
      if [ ! -f "$steps_file" ]; then
        cli_error "Steps file not found: $steps_file"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local steps_json
      steps_json=$(cat "$steps_file" 2>/dev/null)
      if [ -z "$steps_json" ]; then
        cli_error "Steps file is empty: $steps_file"
        return 1
      fi
      local body="{\"pattern\":\"${pattern}\",\"steps\":${steps_json}}"
      local response=""
      response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "x-internal-token: ${internal_secret}" \
        -d "$body" \
        "${claw_url}/claw/playbooks" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local new_id
        new_id=$(printf '%s' "$response" | jq -r '.id // empty' 2>/dev/null)
        if [ -n "$new_id" ]; then
          printf "Playbook created: %s\n" "$new_id"
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;
    edit)
      # Update an existing playbook (pattern, steps, and/or confirmation flag).
      # Usage: nself claw playbooks edit --id <uuid> [--pattern <text>] [--steps-file <path>]
      #                                              [--confirm|--no-confirm] [--enable|--disable]
      local playbook_id="" pattern="" steps_file="" confirm_flag="" enable_flag=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --id) playbook_id="${2:-}"; shift 2 ;;
          --id=*) playbook_id="${1#*=}"; shift ;;
          --pattern) pattern="${2:-}"; shift 2 ;;
          --pattern=*) pattern="${1#*=}"; shift ;;
          --steps-file) steps_file="${2:-}"; shift 2 ;;
          --steps-file=*) steps_file="${1#*=}"; shift ;;
          --confirm) confirm_flag="true"; shift ;;
          --no-confirm) confirm_flag="false"; shift ;;
          --enable) enable_flag="true"; shift ;;
          --disable) enable_flag="false"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$playbook_id" ]; then
        cli_error "--id required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      # Build JSON patch body
      local patch_body="{}"
      if [ -n "$pattern" ] && [ -n "$steps_file" ]; then
        if [ ! -f "$steps_file" ]; then
          cli_error "Steps file not found: $steps_file"
          return 1
        fi
        local steps_json
        steps_json=$(cat "$steps_file" 2>/dev/null)
        if [ -z "$steps_json" ]; then
          cli_error "Steps file is empty: $steps_file"
          return 1
        fi
        patch_body="{\"pattern\":\"${pattern}\",\"steps\":${steps_json}}"
      elif [ -n "$pattern" ]; then
        patch_body="{\"pattern\":\"${pattern}\"}"
      elif [ -n "$steps_file" ]; then
        if [ ! -f "$steps_file" ]; then
          cli_error "Steps file not found: $steps_file"
          return 1
        fi
        local steps_json
        steps_json=$(cat "$steps_file" 2>/dev/null)
        patch_body="{\"steps\":${steps_json}}"
      fi
      if [ -n "$confirm_flag" ]; then
        patch_body=$(printf '%s' "$patch_body" | sed "s/}$/,\"requires_confirmation\":${confirm_flag}}/")
      fi
      if [ -n "$enable_flag" ]; then
        patch_body=$(printf '%s' "$patch_body" | sed "s/}$/,\"enabled\":${enable_flag}}/")
      fi
      # Handle empty-body edge case
      if [ "$patch_body" = "{}" ]; then
        cli_error "No fields to update. Provide at least one of: --pattern, --steps-file, --confirm, --no-confirm, --enable, --disable"
        return 1
      fi
      local response=""
      response=$(curl -s -X PATCH \
        -H "Content-Type: application/json" \
        -H "x-internal-token: ${internal_secret}" \
        -d "$patch_body" \
        "${claw_url}/claw/playbooks/${playbook_id}" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local ok
        ok=$(printf '%s' "$response" | jq -r '.ok // empty' 2>/dev/null)
        if [ "$ok" = "true" ]; then
          log_success "Playbook ${playbook_id} updated."
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;
    test)
      local playbook_id="" dry_run=false
      while [ $# -gt 0 ]; do
        case "$1" in
          --id) playbook_id="${2:-}"; shift 2 ;;
          --id=*) playbook_id="${1#*=}"; shift ;;
          --dry-run) dry_run=true; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$playbook_id" ]; then
        cli_error "--id required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local endpoint="${claw_url}/claw/playbooks/${playbook_id}/test"
      if [ "$dry_run" = "true" ]; then
        endpoint="${endpoint}?dry_run=true"
      fi
      local response=""
      response=$(curl -s -X POST \
        -H "x-internal-token: ${internal_secret}" \
        "$endpoint" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      printf '%s\n' "$response"
      ;;
    *)
      cli_error "Unknown playbooks action: $subcmd"
      printf "Actions: list, add, test\n"
      return 1
      ;;
  esac
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
# voice subcommand — T-1118
# ============================================================================

cmd_claw_voice() {
  local subcmd="${1:-status}"
  shift || true

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local ai_url="${NSELF_AI_URL:-http://localhost:3710}"
  local voice_url="${NSELF_VOICE_URL:-http://localhost:3720}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in

    status)
      # Show voice settings + Whisper install status.
      printf "ɳClaw Voice Status\n"
      printf "==================\n\n"

      # Voice settings from claw plugin.
      if [ -n "$internal_secret" ]; then
        local settings_json=""
        settings_json=$(curl -s \
          -H "x-internal-secret: $internal_secret" \
          "${claw_url}/claw/voice/settings" 2>/dev/null)
        if [ -n "$settings_json" ] && command -v jq >/dev/null 2>&1; then
          local stt_enabled tts_enabled
          stt_enabled=$(printf '%s' "$settings_json" | jq -r '.stt_enabled // false' 2>/dev/null)
          tts_enabled=$(printf '%s' "$settings_json" | jq -r '.tts_enabled // false' 2>/dev/null)
          printf "STT (speech-to-text): %s\n" "$stt_enabled"
          printf "TTS (text-to-speech): %s\n" "$tts_enabled"
        else
          printf "Voice settings: not configured (run: nself claw voice enable)\n"
        fi
      else
        printf "Voice settings: PLUGIN_INTERNAL_SECRET not set\n"
      fi

      # Whisper model status from ai plugin.
      printf "\n"
      if [ -n "$internal_secret" ]; then
        local models_json=""
        models_json=$(curl -s \
          -H "x-internal-secret: $internal_secret" \
          "${ai_url}/ai/models/local" 2>/dev/null)
        local whisper_status="not installed"
        if [ -n "$models_json" ] && command -v jq >/dev/null 2>&1; then
          whisper_status=$(printf '%s' "$models_json" \
            | jq -r '[.[] | select(.name | test("whisper"; "i"))] | if length > 0 then .[0].name + " (ready)" else "not installed" end' \
            2>/dev/null || printf "not installed")
        fi
        printf "Whisper model: %s\n" "$whisper_status"
        if [ "$whisper_status" = "not installed" ]; then
          printf "  Install with: nself ai models install --model openai/whisper\n"
        fi
      fi

      # nself-voice plugin availability.
      printf "\n"
      local voice_health=""
      voice_health=$(curl -s --max-time 2 "${voice_url}/health" 2>/dev/null)
      if [ -n "$voice_health" ]; then
        printf "nself-voice plugin: running (port %s)\n" "${NSELF_VOICE_URL:-3720}"
      else
        printf "nself-voice plugin: not running\n"
        printf "  Max license required. Install: nself plugin install voice\n"
      fi
      ;;

    enable)
      # Enable STT and TTS features via claw plugin settings endpoint.
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET is required"
        exit 1
      fi

      local payload='{"stt_enabled":true,"tts_enabled":true}'
      local resp=""
      resp=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "x-internal-secret: $internal_secret" \
        -d "$payload" \
        "${claw_url}/claw/voice/settings" 2>/dev/null)

      if command -v jq >/dev/null 2>&1; then
        local ok=""
        ok=$(printf '%s' "$resp" | jq -r '.ok // .status // empty' 2>/dev/null)
        if [ "$ok" = "true" ] || [ "$ok" = "ok" ] || [ "$ok" = "updated" ]; then
          log_success "Voice features enabled (STT + TTS)."
        else
          cli_error "Unexpected response: $resp"
          exit 1
        fi
      else
        printf '%s\n' "$resp"
      fi
      ;;

    test)
      # Test TTS via nself-voice plugin (if running) or print OS TTS fallback message.
      local text="Hello from ɳClaw. Voice synthesis is working."
      while [ $# -gt 0 ]; do
        case "$1" in
          --text) text="$2"; shift 2 ;;
          *)      shift ;;
        esac
      done

      local voice_health=""
      voice_health=$(curl -s --max-time 2 "${voice_url}/health" 2>/dev/null)
      if [ -n "$voice_health" ]; then
        log_info "Sending test request to nself-voice at $voice_url ..."
        local payload=""
        payload=$(printf '{"text":"%s","speed":1.0}' "$text")
        local http_code=""
        http_code=$(curl -s -o /dev/null -w "%{http_code}" \
          -X POST \
          -H "Content-Type: application/json" \
          -d "$payload" \
          "${voice_url}/voice/synthesize" 2>/dev/null)
        if [ "$http_code" = "200" ]; then
          log_success "nself-voice responded 200 — synthesis route is healthy."
        else
          cli_error "nself-voice returned HTTP $http_code for /voice/synthesize"
          exit 1
        fi
      else
        printf "nself-voice plugin is not running. Using OS TTS only.\n"
        printf "\n"
        printf "To enable server TTS (Max license required):\n"
        printf "  nself plugin install voice\n"
        printf "  nself build && nself start\n"
      fi
      ;;

    help | --help | -h)
      printf "nself claw voice — voice feature management\n\n"
      printf "Usage: nself claw voice <status|enable|test> [options]\n\n"
      printf "Subcommands:\n"
      printf "  status                   Show STT/TTS settings and Whisper install status\n"
      printf "  enable                   Enable STT and TTS features\n"
      printf "  test [--text <text>]     Test voice synthesis endpoint\n\n"
      printf "Examples:\n"
      printf "  nself claw voice status\n"
      printf "  nself claw voice enable\n"
      printf "  nself claw voice test --text 'Testing one two three'\n"
      ;;

    *)
      cli_error "Unknown voice action: $subcmd"
      printf "Actions: status, enable, test\n"
      exit 1
      ;;
  esac
}

# ============================================================================
# knowledge subcommand dispatcher (T-1151, T-1152, T-1153)
# ============================================================================

cmd_claw_knowledge() {
  local subcmd="${1:-}"
  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"

  if [ -z "$subcmd" ]; then
    printf "Usage: nself claw knowledge <action> [options]\n"
    printf "Actions: search, list, version, note\n"
    return 1
  fi

  shift

  case "$subcmd" in
    search)
      # Usage: nself claw knowledge search <query> [--category <cat>] [--top N] [--json]
      local query="" category="" top="8" json_flag=0

      query="${1:-}"
      if [ -n "$query" ]; then shift; fi

      while [ $# -gt 0 ]; do
        case "$1" in
          --category) category="$2"; shift 2 ;;
          --top)      top="$2";      shift 2 ;;
          --json)     json_flag=1;   shift ;;
          *) shift ;;
        esac
      done

      if [ -z "$query" ]; then
        printf "Usage: nself claw knowledge search <query> [--category <cat>] [--top N] [--json]\n" >&2
        return 1
      fi

      local url="${claw_url}/claw/knowledge/search?q=$(printf '%s' "$query" | tr ' ' '+')&top=${top}"
      if [ -n "$category" ]; then
        url="${url}&category=$(printf '%s' "$category" | tr ' ' '+')"
      fi

      local response=""
      response=$(curl -s "$url" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      if [ "$json_flag" -eq 1 ]; then
        printf '%s\n' "$response"
        return 0
      fi

      if command -v jq >/dev/null 2>&1; then
        local count=""
        count=$(printf '%s' "$response" | jq -r '.chunks | length' 2>/dev/null || printf "0")
        printf "\033[34mFound %s result(s) for \"%s\"\033[0m\n\n" "$count" "$query"

        printf '%s' "$response" | jq -r '.chunks[] | "\(.title)\t\(.category)\t\(.content[0:100])"' 2>/dev/null \
          | while IFS=$(printf '\t') read -r title cat snippet; do
            printf "\033[32m%s\033[0m \033[33m[%s]\033[0m\n" "$title" "$cat"
            printf "  %s…\n\n" "$snippet"
          done
      else
        printf '%s\n' "$response"
      fi
      ;;

    list)
      # Usage: nself claw knowledge list [<category>] [--json]
      local category="" json_flag=0

      while [ $# -gt 0 ]; do
        case "$1" in
          --json) json_flag=1; shift ;;
          --*)    shift ;;
          *)      category="$1"; shift ;;
        esac
      done

      local url="${claw_url}/claw/knowledge/search?q=&top=100"
      if [ -n "$category" ]; then
        url="${claw_url}/claw/knowledge/search?q=&top=100&category=$(printf '%s' "$category" | tr ' ' '+')"
      fi

      local response=""
      response=$(curl -s "$url" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}."
        return 1
      fi

      if [ "$json_flag" -eq 1 ]; then
        printf '%s\n' "$response"
        return 0
      fi

      if command -v jq >/dev/null 2>&1; then
        local cat_url="${claw_url}/claw/knowledge/categories"
        local cats_resp=""
        cats_resp=$(curl -s "$cat_url" 2>/dev/null)
        if command -v jq >/dev/null 2>&1 && [ -n "$cats_resp" ]; then
          printf "\033[34mCategories:\033[0m\n"
          printf '%s' "$cats_resp" | jq -r '.categories[]?.category // empty' 2>/dev/null \
            | while read -r cat; do
              printf "  • %s\n" "$cat"
            done
          printf "\n"
        fi

        printf '%s' "$response" | jq -r '.chunks[] | "\(.id)\t\(.title)\t\(.category)"' 2>/dev/null \
          | while IFS=$(printf '\t') read -r cid ctitle ccat; do
            printf "\033[32m%-38s\033[0m  \033[33m%-20s\033[0m  %s\n" "$cid" "$ccat" "$ctitle"
          done
      else
        printf '%s\n' "$response"
      fi
      ;;

    version)
      # Usage: nself claw knowledge version [--json]
      local json_flag=0
      while [ $# -gt 0 ]; do
        case "$1" in
          --json) json_flag=1; shift ;;
          *) shift ;;
        esac
      done

      local response=""
      response=$(curl -s "${claw_url}/claw/knowledge/version" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}."
        return 1
      fi

      if [ "$json_flag" -eq 1 ]; then
        printf '%s\n' "$response"
        return 0
      fi

      if command -v jq >/dev/null 2>&1; then
        local ver total_chunks schema_ver description
        ver=$(printf '%s' "$response" | jq -r '.version // "unknown"')
        total_chunks=$(printf '%s' "$response" | jq -r '.total_chunks // 0')
        schema_ver=$(printf '%s' "$response" | jq -r '.schema_version // 1')
        description=$(printf '%s' "$response" | jq -r '.description // ""')
        printf "\033[34mnSelf Knowledge Base\033[0m\n"
        printf "  Version:      %s\n" "$ver"
        printf "  Schema:       v%s\n" "$schema_ver"
        printf "  Total chunks: %s\n" "$total_chunks"
        if [ -n "$description" ]; then
          printf "  Description:  %s\n" "$description"
        fi
        printf "\nCategories:\n"
        printf '%s' "$response" | jq -r '.categories[]? // empty' 2>/dev/null \
          | while read -r cat; do printf "  • %s\n" "$cat"; done
      else
        printf '%s\n' "$response"
      fi
      ;;

    note)
      # Usage: nself claw knowledge note <add|list|delete> [options]
      local note_action="${1:-}"
      if [ -z "$note_action" ]; then
        printf "Usage: nself claw knowledge note <add|list|delete> [options]\n" >&2
        return 1
      fi
      shift

      case "$note_action" in
        add)
          local chunk_id="" note_text=""
          while [ $# -gt 0 ]; do
            case "$1" in
              --chunk) chunk_id="$2"; shift 2 ;;
              --note)  note_text="$2"; shift 2 ;;
              *) shift ;;
            esac
          done

          if [ -z "$chunk_id" ] || [ -z "$note_text" ]; then
            printf "Usage: nself claw knowledge note add --chunk <id> --note <text>\n" >&2
            return 1
          fi

          local payload
          payload=$(printf '{"chunk_id":"%s","note":"%s"}' "$chunk_id" "$note_text")
          local response=""
          response=$(curl -s -X POST \
            -H "Content-Type: application/json" \
            -d "$payload" \
            "${claw_url}/claw/knowledge/notes" 2>/dev/null)

          if command -v jq >/dev/null 2>&1 && printf '%s' "$response" | jq -e '.id' >/dev/null 2>&1; then
            local note_id
            note_id=$(printf '%s' "$response" | jq -r '.id')
            log_success "Note added (id: ${note_id})"
          else
            cli_error "Failed to add note. Response: ${response}"
            return 1
          fi
          ;;

        list)
          local chunk_id="" json_flag=0
          while [ $# -gt 0 ]; do
            case "$1" in
              --chunk) chunk_id="$2"; shift 2 ;;
              --json)  json_flag=1;   shift ;;
              *) shift ;;
            esac
          done

          local url="${claw_url}/claw/knowledge/notes"
          if [ -n "$chunk_id" ]; then
            url="${url}?chunk_id=${chunk_id}"
          fi

          local response=""
          response=$(curl -s "$url" 2>/dev/null)

          if [ -z "$response" ]; then
            cli_error "No response from claw service."
            return 1
          fi

          if [ "$json_flag" -eq 1 ]; then
            printf '%s\n' "$response"
            return 0
          fi

          if command -v jq >/dev/null 2>&1; then
            local count=""
            count=$(printf '%s' "$response" | jq -r '.notes | length' 2>/dev/null || printf "0")
            if [ "$count" -eq 0 ]; then
              log_info "No notes found."
              return 0
            fi
            printf "\033[34m%-36s  %-20s  %s\033[0m\n" "ID" "Chunk" "Note"
            printf '%s' "$response" | jq -r '.notes[] | "\(.id)\t\(.chunk_id)\t\(.note)"' 2>/dev/null \
              | while IFS=$(printf '\t') read -r nid nchunk nnote; do
                printf "%-36s  %-20s  %s\n" "$nid" "$nchunk" "$nnote"
              done
          else
            printf '%s\n' "$response"
          fi
          ;;

        delete)
          local note_id=""
          while [ $# -gt 0 ]; do
            case "$1" in
              --id) note_id="$2"; shift 2 ;;
              *) shift ;;
            esac
          done

          if [ -z "$note_id" ]; then
            printf "Usage: nself claw knowledge note delete --id <uuid>\n" >&2
            return 1
          fi

          local response=""
          response=$(curl -s -X DELETE \
            "${claw_url}/claw/knowledge/notes/${note_id}" 2>/dev/null)

          if command -v jq >/dev/null 2>&1 && printf '%s' "$response" | jq -e '.deleted' >/dev/null 2>&1; then
            log_success "Note ${note_id} deleted."
          else
            cli_error "Failed to delete note. Response: ${response}"
            return 1
          fi
          ;;

        *)
          cli_error "Unknown note action: $note_action"
          printf "Actions: add, list, delete\n"
          return 1
          ;;
      esac
      ;;

    *)
      cli_error "Unknown knowledge action: $subcmd"
      printf "Actions: search, list, version, note\n"
      return 1
      ;;
  esac
}

# ============================================================================
# T-1182: api subcommand — gateway API key + usage management
# ============================================================================

cmd_claw_api() {
  local subcmd="${1:-}"

  if [ -z "$subcmd" ]; then
    cli_error "api action required"
    printf "Actions: keys, usage, test\n"
    exit 1
  fi

  shift

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in

    keys)
      # Manage gateway API keys.
      local keys_action="${1:-list}"
      shift || true

      case "$keys_action" in

        list)
          # List all gateway API keys.
          # Usage: nself claw api keys list [--json]
          local json_flag=false
          while [ $# -gt 0 ]; do
            case "$1" in
              --json) json_flag=true; shift ;;
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
            "${claw_url}/claw/v1/api-keys" 2>/dev/null)

          if [ -z "$response" ]; then
            cli_error "No response from claw service at ${claw_url}. Is it running?"
            return 1
          fi

          if [ "$json_flag" = "true" ]; then
            printf '%s\n' "$response"
            return 0
          fi

          if command -v jq >/dev/null 2>&1; then
            local count=""
            count=$(printf '%s' "$response" | jq '.keys | if type == "array" then length else 0 end' 2>/dev/null || true)

            if [ "${count:-0}" -eq 0 ]; then
              log_info "No API keys found. Create one with: nself claw api keys create --name <name>"
              return 0
            fi

            printf "\033[34m%-36s %-20s %-8s %-6s %s\033[0m\n" \
              "ID" "Name" "RPM" "Admin" "Created"
            printf "%-36s %-20s %-8s %-6s %s\n" \
              "------------------------------------" "--------------------" "--------" "------" "-------------------"

            printf '%s' "$response" | jq -r '.keys[] | [
              .id,
              (.name // "-"),
              ((.rpm_limit // 0) | tostring),
              (if (.admin_allowed // false) then "yes" else "no" end),
              (.created_at // "-")
            ] | @tsv' 2>/dev/null | \
            while IFS=$(printf '\t') read -r key_id key_name rpm_limit is_admin created_at; do
              printf "%-36s %-20s %-8s %-6s %s\n" \
                "$key_id" "$key_name" "$rpm_limit" "$is_admin" "$created_at"
            done
          else
            printf '%s\n' "$response"
          fi
          ;;

        create)
          # Create a new gateway API key.
          # Usage: nself claw api keys create --name <name> [--admin] [--rpm <N>]
          local key_name="" admin_allowed=false rpm_limit=60

          while [ $# -gt 0 ]; do
            case "$1" in
              --name)  key_name="$2";          shift 2 ;;
              --admin) admin_allowed=true;      shift ;;
              --rpm)   rpm_limit="$2";          shift 2 ;;
              *) shift ;;
            esac
          done

          if [ -z "$key_name" ]; then
            cli_error "Missing --name <name>"
            printf "Usage: nself claw api keys create --name <name> [--admin] [--rpm <N>]\n" >&2
            return 1
          fi

          if [ -z "$internal_secret" ]; then
            cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
            return 1
          fi

          local safe_name=""
          safe_name=$(printf '%s' "$key_name" | sed 's/\\/\\\\/g; s/"/\\"/g')

          local payload="{\"name\":\"${safe_name}\",\"admin_allowed\":${admin_allowed},\"rpm_limit\":${rpm_limit}}"

          local response=""
          response=$(curl -s -X POST \
            -H "x-internal-token: ${internal_secret}" \
            -H "Content-Type: application/json" \
            -d "$payload" \
            "${claw_url}/claw/v1/api-keys" 2>/dev/null)

          if [ -z "$response" ]; then
            cli_error "No response from claw service at ${claw_url}. Is it running?"
            return 1
          fi

          if command -v jq >/dev/null 2>&1; then
            local new_key="" key_id=""
            new_key=$(printf '%s' "$response" | jq -r '.key // ""' 2>/dev/null || true)
            key_id=$(printf '%s' "$response"  | jq -r '.id // ""' 2>/dev/null || true)

            if [ -z "$new_key" ]; then
              local err=""
              err=$(printf '%s' "$response" | jq -r '.error // "unknown error"' 2>/dev/null || true)
              cli_error "Failed to create API key: ${err}"
              return 1
            fi

            log_success "API key created."
            printf "\n"
            printf "  ID:    %s\n" "$key_id"
            printf "  Key:   \033[1m%s\033[0m\n" "$new_key"
            printf "  Admin: %s\n" "$admin_allowed"
            printf "  RPM:   %s\n" "$rpm_limit"
            printf "\n"
            printf "Save this key — it will not be shown again.\n"
          else
            printf '%s\n' "$response"
          fi
          ;;

        revoke)
          # Revoke (delete) a gateway API key.
          # Usage: nself claw api keys revoke <id>
          local key_id="${1:-}"

          if [ -z "$key_id" ]; then
            cli_error "Missing key ID"
            printf "Usage: nself claw api keys revoke <id>\n" >&2
            return 1
          fi

          if [ -z "$internal_secret" ]; then
            cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
            return 1
          fi

          local safe_id=""
          safe_id=$(printf '%s' "$key_id" | sed 's/[^a-fA-F0-9-]//g')

          local http_status=""
          http_status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
            -H "x-internal-token: ${internal_secret}" \
            "${claw_url}/claw/v1/api-keys/${safe_id}" 2>/dev/null)

          if [ "$http_status" = "200" ] || [ "$http_status" = "204" ]; then
            log_success "API key ${safe_id} revoked."
          elif [ "$http_status" = "404" ]; then
            cli_error "API key not found: ${safe_id}"
            return 1
          else
            cli_error "Failed to revoke API key (HTTP ${http_status})."
            return 1
          fi
          ;;

        help | --help | -h)
          printf "Usage: nself claw api keys <action> [options]\n"
          printf "Actions: list, create, revoke\n"
          ;;

        *)
          cli_error "Unknown keys action: $keys_action"
          printf "Actions: list, create, revoke\n"
          return 1
          ;;
      esac
      ;;

    usage)
      # Show gateway API usage log.
      # Usage: nself claw api usage [--key <id>] [--json]
      local key_id="" json_flag=false

      while [ $# -gt 0 ]; do
        case "$1" in
          --key)  key_id="$2";   shift 2 ;;
          --json) json_flag=true; shift ;;
          *) shift ;;
        esac
      done

      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi

      local url="${claw_url}/claw/v1/usage"
      if [ -n "$key_id" ]; then
        local safe_key_id=""
        safe_key_id=$(printf '%s' "$key_id" | sed 's/[^a-fA-F0-9-]//g')
        url="${claw_url}/claw/v1/usage?key_id=${safe_key_id}"
      fi

      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "$url" 2>/dev/null)

      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi

      if [ "$json_flag" = "true" ]; then
        printf '%s\n' "$response"
        return 0
      fi

      if command -v jq >/dev/null 2>&1; then
        local count=""
        count=$(printf '%s' "$response" | jq '.usage | if type == "array" then length else 0 end' 2>/dev/null || true)

        if [ "${count:-0}" -eq 0 ]; then
          log_info "No gateway usage data found."
          return 0
        fi

        printf "\033[34m%-36s %-8s %-8s %-6s %s\033[0m\n" \
          "Key ID" "Prompt" "Compl" "Model" "Time"
        printf "%-36s %-8s %-8s %-6s %s\n" \
          "------------------------------------" "--------" "--------" "------" "-------------------"

        printf '%s' "$response" | jq -r '.usage[] | [
          (.key_id // "-"),
          ((.prompt_tokens // 0) | tostring),
          ((.completion_tokens // 0) | tostring),
          (.model // "-"),
          (.created_at // "-")
        ] | @tsv' 2>/dev/null | \
        while IFS=$(printf '\t') read -r k_id pt ct model ts; do
          printf "%-36s %-8s %-8s %-6s %s\n" "$k_id" "$pt" "$ct" "$model" "$ts"
        done
      else
        printf '%s\n' "$response"
      fi
      ;;

    test)
      # Test that the gateway endpoint is reachable and responding.
      # Usage: nself claw api test [--url <url>] [--key <key>] [--verbose]
      local base_url="" api_key="" verbose=false

      while [ $# -gt 0 ]; do
        case "$1" in
          --url)     base_url="$2"; shift 2 ;;
          --key)     api_key="$2";  shift 2 ;;
          --verbose) verbose=true;  shift ;;
          *) shift ;;
        esac
      done

      # Derive gateway URL from NSELF_CLAW_URL if not overridden
      if [ -z "$base_url" ]; then
        base_url="${NSELF_CLAW_URL:-http://localhost:3713}"
      fi

      local models_url="${base_url}/v1/models"

      if [ "$verbose" = "true" ]; then
        log_info "Testing gateway at ${models_url}..."
      fi

      local http_status="" response=""
      if [ -n "$api_key" ]; then
        response=$(curl -s -w "\n__STATUS__%{http_code}" \
          -H "Authorization: Bearer ${api_key}" \
          "$models_url" 2>/dev/null)
      else
        response=$(curl -s -w "\n__STATUS__%{http_code}" \
          "$models_url" 2>/dev/null)
      fi

      http_status=$(printf '%s' "$response" | grep '__STATUS__' | sed 's/__STATUS__//')
      local body=""
      body=$(printf '%s' "$response" | grep -v '__STATUS__')

      if [ "$http_status" = "200" ]; then
        if command -v jq >/dev/null 2>&1; then
          local model_count=""
          model_count=$(printf '%s' "$body" | jq '.data | if type == "array" then length else 0 end' 2>/dev/null || true)
          log_success "Gateway is reachable. ${model_count} models available."
          if [ "$verbose" = "true" ] && [ -n "$body" ]; then
            printf '\n'
            printf '%s' "$body" | jq -r '.data[]? | "  " + .id' 2>/dev/null || true
            printf '\n'
          fi
        else
          log_success "Gateway is reachable (HTTP 200)."
        fi
      elif [ "$http_status" = "401" ]; then
        cli_error "Gateway returned 401 Unauthorized. Check your API key."
        return 1
      elif [ -z "$http_status" ]; then
        cli_error "No response from gateway at ${base_url}. Is nClaw running?"
        return 1
      else
        cli_error "Gateway returned HTTP ${http_status}."
        if [ "$verbose" = "true" ] && [ -n "$body" ]; then
          printf '%s\n' "$body"
        fi
        return 1
      fi
      ;;

    help | --help | -h)
      printf "Usage: nself claw api <action> [options]\n"
      printf "Actions: keys, usage, test\n"
      ;;

    *)
      cli_error "Unknown api action: $subcmd"
      printf "Actions: keys, usage, test\n"
      return 1
      ;;

  esac
}

# ============================================================================
# memories subcommand — T-1209
# ============================================================================
#
# Usage:
#   nself claw memories list  --user <id>                 List memories for a user
#   nself claw memories add   --user <id> --content <text> Add an explicit memory
#   nself claw memories delete --id <uuid>                Delete a memory by ID
#   nself claw memories clear  --user <id>                Clear all memories for a user
#   nself claw memories stats  --user <id>                Show memory stats for a user

cmd_claw_memories() {
  local subcmd="${1:-list}"
  shift || true

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in
    --help|-h)
      printf "Usage: nself claw memories <action> [options]\n\n"
      printf "Actions:\n"
      printf "  list  --user <id>                       List stored memories for a user\n"
      printf "  add   --user <id> --content <text>      Add an explicit memory\n"
      printf "  delete --id <uuid>                      Delete a memory by ID\n"
      printf "  clear  --user <id>                      Clear all memories for a user\n"
      printf "  stats  --user <id>                      Show memory counts and limits\n\n"
      printf "Examples:\n"
      printf "  nself claw memories list --user abc123\n"
      printf "  nself claw memories add --user abc123 --content \"Prefers concise answers\"\n"
      printf "  nself claw memories delete --id a1b2c3d4-...\n"
      printf "  nself claw memories clear --user abc123\n"
      printf "  nself claw memories stats --user abc123\n"
      return 0
      ;;

    list)
      local user_id=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --user) user_id="${2:-}"; shift 2 ;;
          --user=*) user_id="${1#*=}"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$user_id" ]; then
        cli_error "--user <id> required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/memories?user_id=${user_id}" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local count
        count=$(printf '%s' "$response" | jq '.memories | if type == "array" then length else 0 end' 2>/dev/null || printf "0")
        if [ "${count:-0}" -eq 0 ]; then
          printf "No memories stored for user %s.\n" "$user_id"
          return 0
        fi
        printf "\n\033[34m%-38s %-12s %s\033[0m\n" "ID" "Source" "Content"
        printf "%-38s %-12s %s\n" "--------------------------------------" "------------" "-------"
        printf '%s' "$response" | jq -r '.memories // [] | .[] | [
          (.id // "-"),
          (.source // "explicit"),
          ((.content // "-") | gsub("\n"; " ") | .[0:80])
        ] | @tsv' 2>/dev/null | \
        while IFS=$(printf '\t') read -r id src content; do
          printf "%-38s %-12s %s\n" "$id" "$src" "$content"
        done
      else
        printf '%s\n' "$response"
      fi
      ;;

    add)
      local user_id="" content=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --user) user_id="${2:-}"; shift 2 ;;
          --user=*) user_id="${1#*=}"; shift ;;
          --content) content="${2:-}"; shift 2 ;;
          --content=*) content="${1#*=}"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$user_id" ]; then
        cli_error "--user <id> required"
        return 1
      fi
      if [ -z "$content" ]; then
        cli_error "--content <text> required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      # Escape content for JSON using printf
      local escaped_content
      escaped_content=$(printf '%s' "$content" | sed 's/\\/\\\\/g; s/"/\\"/g')
      local body="{\"user_id\":\"${user_id}\",\"content\":\"${escaped_content}\"}"
      local response=""
      response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "x-internal-token: ${internal_secret}" \
        -d "$body" \
        "${claw_url}/claw/memories" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local new_id
        new_id=$(printf '%s' "$response" | jq -r '.id // empty' 2>/dev/null)
        if [ -n "$new_id" ]; then
          log_success "Memory stored: ${new_id}"
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    delete)
      local mem_id=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --id) mem_id="${2:-}"; shift 2 ;;
          --id=*) mem_id="${1#*=}"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$mem_id" ]; then
        cli_error "--id <uuid> required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local http_code=""
      http_code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/memories/${mem_id}" 2>/dev/null)
      if [ "$http_code" = "204" ]; then
        log_success "Memory deleted."
      elif [ "$http_code" = "404" ]; then
        cli_error "Memory not found: ${mem_id}"
        return 1
      elif [ -z "$http_code" ] || [ "$http_code" = "000" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      else
        cli_error "Unexpected response: HTTP ${http_code}"
        return 1
      fi
      ;;

    clear)
      local user_id=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --user) user_id="${2:-}"; shift 2 ;;
          --user=*) user_id="${1#*=}"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$user_id" ]; then
        cli_error "--user <id> required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local response=""
      response=$(curl -s -X DELETE \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/memories?user_id=${user_id}" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local deleted
        deleted=$(printf '%s' "$response" | jq -r '.deleted // 0' 2>/dev/null)
        log_success "Cleared ${deleted} memories for user ${user_id}."
      else
        printf '%s\n' "$response"
      fi
      ;;

    stats)
      local user_id=""
      while [ $# -gt 0 ]; do
        case "$1" in
          --user) user_id="${2:-}"; shift 2 ;;
          --user=*) user_id="${1#*=}"; shift ;;
          *) shift ;;
        esac
      done
      if [ -z "$user_id" ]; then
        cli_error "--user <id> required"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/memories/stats?user_id=${user_id}" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local total explicit semantic
        total=$(printf '%s' "$response" | jq -r '.total // 0' 2>/dev/null)
        explicit=$(printf '%s' "$response" | jq -r '.explicit // 0' 2>/dev/null)
        semantic=$(printf '%s' "$response" | jq -r '.semantic // 0' 2>/dev/null)
        printf "Memory stats for user %s:\n" "$user_id"
        printf "  Total:    %s\n" "$total"
        printf "  Explicit: %s\n" "$explicit"
        printf "  Semantic: %s\n" "$semantic"
      else
        printf '%s\n' "$response"
      fi
      ;;

    *)
      cli_error "Unknown memories action: $subcmd"
      printf "Actions: list, add, delete, clear, stats\n"
      return 1
      ;;
  esac
}

# ============================================================================
# proactive subcommand — T-1209
# ============================================================================
#
# Usage:
#   nself claw proactive status                  List all scheduled jobs + enabled state
#   nself claw proactive enable  <job_type>      Enable a job
#   nself claw proactive disable <job_type>      Disable a job
#   nself claw proactive run                     Preview next digest (no send)

cmd_claw_proactive() {
  local subcmd="${1:-status}"
  shift || true

  local claw_url="${NSELF_CLAW_URL:-http://localhost:3713}"
  local internal_secret="${PLUGIN_INTERNAL_SECRET:-}"

  case "$subcmd" in
    --help|-h)
      printf "Usage: nself claw proactive <action> [options]\n\n"
      printf "Actions:\n"
      printf "  status                   List all scheduled jobs and their enabled state\n"
      printf "  enable  <job_type>       Enable a proactive job\n"
      printf "  disable <job_type>       Disable a proactive job\n"
      printf "  run                      Preview the next morning digest (does not send)\n\n"
      printf "Job types: morning_digest, health_report, ssl_check, disk_check, anomaly_detect\n\n"
      printf "Examples:\n"
      printf "  nself claw proactive status\n"
      printf "  nself claw proactive enable morning_digest\n"
      printf "  nself claw proactive disable health_report\n"
      printf "  nself claw proactive run\n"
      return 0
      ;;

    status)
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/proactive/jobs" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local count
        count=$(printf '%s' "$response" | jq '.jobs | if type == "array" then length else 0 end' 2>/dev/null || printf "0")
        if [ "${count:-0}" -eq 0 ]; then
          printf "No proactive jobs configured.\n"
          return 0
        fi
        printf "\n\033[34m%-22s %-10s %-20s %-6s %s\033[0m\n" \
          "Job Type" "Enabled" "Schedule" "Fails" "Last Run"
        printf "%-22s %-10s %-20s %-6s %s\n" \
          "----------------------" "----------" "--------------------" "------" "-------------------"
        printf '%s' "$response" | jq -r '.jobs // [] | .[] | [
          (.job_type // "-"),
          (if .enabled then "yes" else "no" end),
          (.cron_expr // "-"),
          ((.failure_count // 0) | tostring),
          (.last_run_at // "never")
        ] | @tsv' 2>/dev/null | \
        while IFS=$(printf '\t') read -r jtype enabled cron fails last; do
          printf "%-22s %-10s %-20s %-6s %s\n" "$jtype" "$enabled" "$cron" "$fails" "$last"
        done
      else
        printf '%s\n' "$response"
      fi
      ;;

    enable|disable)
      local job_type="${1:-}"
      if [ -z "$job_type" ]; then
        cli_error "job_type required (e.g. morning_digest, ssl_check)"
        return 1
      fi
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local enabled_val="true"
      if [ "$subcmd" = "disable" ]; then
        enabled_val="false"
      fi
      local response=""
      response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "x-internal-token: ${internal_secret}" \
        -d "{\"enabled\":${enabled_val}}" \
        "${claw_url}/claw/proactive/jobs/${job_type}/toggle" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local ok
        ok=$(printf '%s' "$response" | jq -r '.ok // .updated // empty' 2>/dev/null)
        if [ -n "$ok" ] && [ "$ok" != "false" ] && [ "$ok" != "null" ]; then
          if [ "$subcmd" = "enable" ]; then
            log_success "${job_type} enabled."
          else
            log_success "${job_type} disabled."
          fi
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    run)
      if [ -z "$internal_secret" ]; then
        cli_error "PLUGIN_INTERNAL_SECRET not set. Source your .env file first."
        return 1
      fi
      local response=""
      response=$(curl -s \
        -H "x-internal-token: ${internal_secret}" \
        "${claw_url}/claw/proactive/digest" 2>/dev/null)
      if [ -z "$response" ]; then
        cli_error "No response from claw service at ${claw_url}. Is it running?"
        return 1
      fi
      if command -v jq >/dev/null 2>&1; then
        local text
        text=$(printf '%s' "$response" | jq -r '.text // empty' 2>/dev/null)
        if [ -n "$text" ]; then
          printf "\n--- Digest preview (not sent) ---\n\n"
          printf '%s\n' "$text"
          printf "\n---------------------------------\n"
        else
          printf '%s\n' "$response"
        fi
      else
        printf '%s\n' "$response"
      fi
      ;;

    *)
      cli_error "Unknown proactive action: $subcmd"
      printf "Actions: status, enable, disable, run\n"
      return 1
      ;;
  esac
}

# ============================================================================
# Entry point
# ============================================================================

cmd_claw "$@"
