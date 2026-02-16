#!/usr/bin/env bash

# history.sh - Deployment and operation history/audit trail
# v0.4.6 - Feedback implementation

set -euo pipefail

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$CLI_SCRIPT_DIR"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/header.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/pre-command.sh"
source "$CLI_SCRIPT_DIR/../lib/hooks/post-command.sh"

# Color fallbacks
: "${COLOR_GREEN:=\033[0;32m}"
: "${COLOR_YELLOW:=\033[0;33m}"
: "${COLOR_RED:=\033[0;31m}"
: "${COLOR_CYAN:=\033[0;36m}"
: "${COLOR_RESET:=\033[0m}"
: "${COLOR_DIM:=\033[2m}"
: "${COLOR_BOLD:=\033[1m}"

# Show help
show_history_help() {
  cat <<'EOF'
nself history - View deployment and operation history

Usage: nself history [subcommand] [options]

Subcommands:
  show                  Show recent history (default)
  deployments           Show deployment history
  migrations            Show database migration history
  rollbacks             Show rollback history
  commands              Show command execution history
  search <query>        Search history
  export                Export history to file
  clear                 Clear history (with confirmation)

Options:
  --limit N             Number of entries (default: 20)
  --since DATE          Show entries since date (YYYY-MM-DD)
  --until DATE          Show entries until date
  --env NAME            Filter by environment
  --type TYPE           Filter by type (deploy, migrate, rollback, etc.)
  --json                Output in JSON format
  --csv                 Output in CSV format
  -h, --help            Show this help message

Examples:
  nself history                           # Recent history
  nself history deployments --limit 50    # Last 50 deployments
  nself history --env prod                # Production only
  nself history search "migration"        # Search for migrations
  nself history export --json             # Export as JSON
EOF
}

# Initialize history environment
init_history() {
  load_env_with_priority

  HISTORY_DIR="${HISTORY_DIR:-.nself/history}"
  mkdir -p "$HISTORY_DIR"

  # History files
  DEPLOYMENTS_LOG="${HISTORY_DIR}/deployments.jsonl"
  MIGRATIONS_LOG="${HISTORY_DIR}/migrations.jsonl"
  ROLLBACKS_LOG="${HISTORY_DIR}/rollbacks.jsonl"
  COMMANDS_LOG="${HISTORY_DIR}/commands.jsonl"
  ALL_LOG="${HISTORY_DIR}/all.jsonl"
}

# Record an event to history
record_event() {
  local type="$1"
  local description="$2"
  local env="${3:-local}"
  local status="${4:-success}"
  local details="${5:-}"

  local timestamp=$(date -Iseconds)
  local user=$(whoami)

  local event=$(
    cat <<EOF
{"timestamp": "$timestamp", "type": "$type", "description": "$description", "env": "$env", "status": "$status", "user": "$user", "details": "$details"}
EOF
  )

  # Write to all log
  echo "$event" >>"$ALL_LOG"

  # Write to type-specific log
  case "$type" in
    deploy | deployment)
      echo "$event" >>"$DEPLOYMENTS_LOG"
      ;;
    migrate | migration)
      echo "$event" >>"$MIGRATIONS_LOG"
      ;;
    rollback)
      echo "$event" >>"$ROLLBACKS_LOG"
      ;;
    command)
      echo "$event" >>"$COMMANDS_LOG"
      ;;
  esac
}

# Parse history entry for display
format_entry() {
  local line="$1"
  local format="${2:-table}"

  local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')
  local type=$(echo "$line" | grep -o '"type": *"[^"]*"' | sed 's/"type": *"\([^"]*\)"/\1/')
  local desc=$(echo "$line" | grep -o '"description": *"[^"]*"' | sed 's/"description": *"\([^"]*\)"/\1/')
  local env=$(echo "$line" | grep -o '"env": *"[^"]*"' | sed 's/"env": *"\([^"]*\)"/\1/')
  local status=$(echo "$line" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')

  case "$format" in
    table)
      local status_color="$COLOR_GREEN"
      [[ "$status" == "failed" ]] && status_color="$COLOR_RED"
      [[ "$status" == "pending" ]] && status_color="$COLOR_YELLOW"

      printf "  %-20s %-12s %-10s ${status_color}%-10s${COLOR_RESET} %s\n" \
        "$ts" "$type" "$env" "$status" "$desc"
      ;;
    csv)
      printf '"%s","%s","%s","%s","%s"\n' "$ts" "$type" "$env" "$status" "$desc"
      ;;
    json)
      echo "$line"
      ;;
  esac
}

# Show all recent history
cmd_show() {
  local limit="${LIMIT:-20}"
  local since="${SINCE:-}"
  local until="${UNTIL:-}"
  local filter_env="${FILTER_ENV:-}"
  local filter_type="${FILTER_TYPE:-}"
  local json_mode="${JSON_OUTPUT:-false}"
  local csv_mode="${CSV_OUTPUT:-false}"

  init_history

  local log_file="$ALL_LOG"

  if [[ ! -f "$log_file" ]]; then
    if [[ "$json_mode" == "true" ]]; then
      echo '{"history": [], "message": "No history recorded"}'
    else
      log_info "No history recorded yet"
    fi
    return 0
  fi

  if [[ "$json_mode" != "true" ]] && [[ "$csv_mode" != "true" ]]; then
    show_command_header "nself history" "Operation History"
    echo ""

    printf "  %-20s %-12s %-10s %-10s %s\n" "Timestamp" "Type" "Env" "Status" "Description"
    printf "  %-20s %-12s %-10s %-10s %s\n" "---------" "----" "---" "------" "-----------"
  fi

  if [[ "$csv_mode" == "true" ]]; then
    echo '"Timestamp","Type","Environment","Status","Description"'
  fi

  local format="table"
  [[ "$json_mode" == "true" ]] && format="json"
  [[ "$csv_mode" == "true" ]] && format="csv"

  # Build filter pipeline without eval - apply grep filters safely
  # SECURITY: Avoid eval with user-influenced filter values
  _history_apply_filters() {
    local _src_file="$1"
    local _fenv="$2"
    local _ftype="$3"
    if [[ -n "$_fenv" ]] && [[ -n "$_ftype" ]]; then
      grep "\"env\": *\"$_fenv\"" "$_src_file" | grep "\"type\": *\"$_ftype\""
    elif [[ -n "$_fenv" ]]; then
      grep "\"env\": *\"$_fenv\"" "$_src_file"
    elif [[ -n "$_ftype" ]]; then
      grep "\"type\": *\"$_ftype\"" "$_src_file"
    else
      cat "$_src_file"
    fi
  }

  if [[ "$json_mode" == "true" ]]; then
    printf '{"history": ['
    _history_apply_filters "$log_file" "$filter_env" "$filter_type" | tail -n "$limit" | tr '\n' ',' | sed 's/,$//'
    printf '], "count": %d}\n' "$(_history_apply_filters "$log_file" "$filter_env" "$filter_type" | wc -l | tr -d ' ')"
  else
    _history_apply_filters "$log_file" "$filter_env" "$filter_type" | tail -n "$limit" | while read -r line; do
      format_entry "$line" "$format"
    done

    if [[ "$csv_mode" != "true" ]]; then
      echo ""
      local total=$(_history_apply_filters "$log_file" "$filter_env" "$filter_type" | wc -l | tr -d ' ')
      log_info "Showing last $limit of $total entries"
    fi
  fi
}

# Show deployment history
cmd_deployments() {
  local limit="${LIMIT:-20}"
  local json_mode="${JSON_OUTPUT:-false}"

  init_history

  local log_file="$DEPLOYMENTS_LOG"

  if [[ ! -f "$log_file" ]]; then
    if [[ "$json_mode" == "true" ]]; then
      echo '{"deployments": []}'
    else
      log_info "No deployment history recorded"
    fi
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself history" "Deployment History"
    echo ""

    printf "  %-20s %-15s %-10s %s\n" "Timestamp" "Environment" "Status" "Description"
    printf "  %-20s %-15s %-10s %s\n" "---------" "-----------" "------" "-----------"

    tail -n "$limit" "$log_file" | while read -r line; do
      local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')
      local env=$(echo "$line" | grep -o '"env": *"[^"]*"' | sed 's/"env": *"\([^"]*\)"/\1/')
      local status=$(echo "$line" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
      local desc=$(echo "$line" | grep -o '"description": *"[^"]*"' | sed 's/"description": *"\([^"]*\)"/\1/')

      local status_color="$COLOR_GREEN"
      [[ "$status" == "failed" ]] && status_color="$COLOR_RED"

      printf "  %-20s %-15s ${status_color}%-10s${COLOR_RESET} %s\n" "$ts" "$env" "$status" "$desc"
    done
  else
    printf '{"deployments": ['
    tail -n "$limit" "$log_file" | tr '\n' ',' | sed 's/,$//'
    printf ']}\n'
  fi
}

# Show migration history
cmd_migrations() {
  local limit="${LIMIT:-20}"
  local json_mode="${JSON_OUTPUT:-false}"

  init_history

  local log_file="$MIGRATIONS_LOG"

  # Also check standard migrations directory
  local migration_dir="postgres/migrations"
  [[ ! -d "$migration_dir" ]] && migration_dir="nself/migrations"

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself history" "Migration History"
    echo ""
  fi

  # Show recorded migrations
  if [[ -f "$log_file" ]]; then
    if [[ "$json_mode" != "true" ]]; then
      printf "${COLOR_CYAN}➞ Recorded Migrations${COLOR_RESET}\n"
      echo ""

      printf "  %-20s %-15s %-10s %s\n" "Timestamp" "Environment" "Status" "Migration"
      printf "  %-20s %-15s %-10s %s\n" "---------" "-----------" "------" "---------"

      tail -n "$limit" "$log_file" | while read -r line; do
        local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')
        local env=$(echo "$line" | grep -o '"env": *"[^"]*"' | sed 's/"env": *"\([^"]*\)"/\1/')
        local status=$(echo "$line" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
        local desc=$(echo "$line" | grep -o '"description": *"[^"]*"' | sed 's/"description": *"\([^"]*\)"/\1/')

        printf "  %-20s %-15s %-10s %s\n" "$ts" "$env" "$status" "$desc"
      done

      echo ""
    fi
  fi

  # Show available migration files
  if [[ -d "$migration_dir" ]]; then
    if [[ "$json_mode" != "true" ]]; then
      printf "${COLOR_CYAN}➞ Migration Files${COLOR_RESET}\n"
      echo ""

      ls -1 "$migration_dir"/*.sql 2>/dev/null | while read -r file; do
        local basename=$(basename "$file")
        printf "  %s\n" "$basename"
      done
    else
      printf '{"migrations": ['
      ls -1 "$migration_dir"/*.sql 2>/dev/null | while read -r file; do
        local basename=$(basename "$file")
        printf '"%s",' "$basename"
      done | sed 's/,$//'
      printf ']}\n'
    fi
  fi
}

# Show rollback history
cmd_rollbacks() {
  local limit="${LIMIT:-20}"
  local json_mode="${JSON_OUTPUT:-false}"

  init_history

  local log_file="$ROLLBACKS_LOG"

  if [[ ! -f "$log_file" ]]; then
    if [[ "$json_mode" == "true" ]]; then
      echo '{"rollbacks": []}'
    else
      log_info "No rollback history recorded"
    fi
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself history" "Rollback History"
    echo ""

    printf "  %-20s %-15s %-10s %s\n" "Timestamp" "Environment" "Type" "Description"
    printf "  %-20s %-15s %-10s %s\n" "---------" "-----------" "----" "-----------"

    tail -n "$limit" "$log_file" | while read -r line; do
      local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')
      local env=$(echo "$line" | grep -o '"env": *"[^"]*"' | sed 's/"env": *"\([^"]*\)"/\1/')
      local desc=$(echo "$line" | grep -o '"description": *"[^"]*"' | sed 's/"description": *"\([^"]*\)"/\1/')

      printf "  %-20s %-15s %-10s %s\n" "$ts" "$env" "rollback" "$desc"
    done
  else
    printf '{"rollbacks": ['
    tail -n "$limit" "$log_file" | tr '\n' ',' | sed 's/,$//'
    printf ']}\n'
  fi
}

# Show command history
cmd_commands() {
  local limit="${LIMIT:-50}"
  local json_mode="${JSON_OUTPUT:-false}"

  init_history

  local log_file="$COMMANDS_LOG"

  if [[ ! -f "$log_file" ]]; then
    if [[ "$json_mode" == "true" ]]; then
      echo '{"commands": []}'
    else
      log_info "No command history recorded"
    fi
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself history" "Command History"
    echo ""

    printf "  %-20s %-10s %s\n" "Timestamp" "Status" "Command"
    printf "  %-20s %-10s %s\n" "---------" "------" "-------"

    tail -n "$limit" "$log_file" | while read -r line; do
      local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/' | cut -d'+' -f1 | tr 'T' ' ')
      local status=$(echo "$line" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
      local desc=$(echo "$line" | grep -o '"description": *"[^"]*"' | sed 's/"description": *"\([^"]*\)"/\1/')

      local status_color="$COLOR_GREEN"
      [[ "$status" == "failed" ]] && status_color="$COLOR_RED"

      printf "  %-20s ${status_color}%-10s${COLOR_RESET} %s\n" "$ts" "$status" "$desc"
    done
  else
    printf '{"commands": ['
    tail -n "$limit" "$log_file" | tr '\n' ',' | sed 's/,$//'
    printf ']}\n'
  fi
}

# Search history
cmd_search() {
  local query="$1"
  local limit="${LIMIT:-50}"
  local json_mode="${JSON_OUTPUT:-false}"

  if [[ -z "$query" ]]; then
    log_error "Search query required"
    return 1
  fi

  init_history

  if [[ ! -f "$ALL_LOG" ]]; then
    log_info "No history to search"
    return 0
  fi

  if [[ "$json_mode" != "true" ]]; then
    show_command_header "nself history" "Search: $query"
    echo ""

    printf "  %-20s %-12s %-10s %s\n" "Timestamp" "Type" "Env" "Description"
    printf "  %-20s %-12s %-10s %s\n" "---------" "----" "---" "-----------"
  fi

  local count=0

  if [[ "$json_mode" == "true" ]]; then
    printf '{"query": "%s", "results": [' "$query"
    grep -i "$query" "$ALL_LOG" 2>/dev/null | tail -n "$limit" | tr '\n' ',' | sed 's/,$//'
    printf ']}\n'
  else
    grep -i "$query" "$ALL_LOG" 2>/dev/null | tail -n "$limit" | while read -r line; do
      format_entry "$line" "table"
      count=$((count + 1))
    done

    echo ""
    log_info "Found $(grep -ic "$query" "$ALL_LOG" 2>/dev/null || echo 0) matching entries"
  fi
}

# Export history
cmd_export() {
  local output_file="${OUTPUT_FILE:-}"
  local json_mode="${JSON_OUTPUT:-false}"
  local csv_mode="${CSV_OUTPUT:-false}"

  init_history

  if [[ ! -f "$ALL_LOG" ]]; then
    log_error "No history to export"
    return 1
  fi

  local format="json"
  [[ "$csv_mode" == "true" ]] && format="csv"

  if [[ -z "$output_file" ]]; then
    local timestamp=$(date +%Y%m%d_%H%M%S)
    output_file="${HISTORY_DIR}/export_${timestamp}.${format}"
  fi

  show_command_header "nself history" "Exporting History"
  echo ""

  if [[ "$format" == "csv" ]]; then
    echo '"Timestamp","Type","Environment","Status","User","Description"' >"$output_file"
    while read -r line; do
      local ts=$(echo "$line" | grep -o '"timestamp": *"[^"]*"' | sed 's/"timestamp": *"\([^"]*\)"/\1/')
      local type=$(echo "$line" | grep -o '"type": *"[^"]*"' | sed 's/"type": *"\([^"]*\)"/\1/')
      local env=$(echo "$line" | grep -o '"env": *"[^"]*"' | sed 's/"env": *"\([^"]*\)"/\1/')
      local status=$(echo "$line" | grep -o '"status": *"[^"]*"' | sed 's/"status": *"\([^"]*\)"/\1/')
      local user=$(echo "$line" | grep -o '"user": *"[^"]*"' | sed 's/"user": *"\([^"]*\)"/\1/')
      local desc=$(echo "$line" | grep -o '"description": *"[^"]*"' | sed 's/"description": *"\([^"]*\)"/\1/')

      printf '"%s","%s","%s","%s","%s","%s"\n' "$ts" "$type" "$env" "$status" "$user" "$desc" >>"$output_file"
    done <"$ALL_LOG"
  else
    printf '{"exported": "%s", "history": [' "$(date -Iseconds)" >"$output_file"
    cat "$ALL_LOG" | tr '\n' ',' | sed 's/,$//' >>"$output_file"
    printf ']}\n' >>"$output_file"
  fi

  local count=$(wc -l <"$ALL_LOG" | tr -d ' ')
  log_success "Exported $count entries to $output_file"
}

# Clear history
cmd_clear() {
  local force="${FORCE:-false}"

  init_history

  show_command_header "nself history" "Clear History"
  echo ""

  log_warning "This will permanently delete all history"
  echo ""

  if [[ "$force" != "true" ]]; then
    read -p "Type 'DELETE' to confirm: " confirm
    if [[ "$confirm" != "DELETE" ]]; then
      log_info "Clear cancelled"
      return 1
    fi
  fi

  # Create backup first
  local backup_file="${HISTORY_DIR}/backup_$(date +%Y%m%d_%H%M%S).tar.gz"
  tar -czf "$backup_file" -C "$HISTORY_DIR" . 2>/dev/null || true

  # Clear files
  rm -f "$DEPLOYMENTS_LOG" "$MIGRATIONS_LOG" "$ROLLBACKS_LOG" "$COMMANDS_LOG" "$ALL_LOG"

  log_success "History cleared"
  log_info "Backup saved to: $backup_file"
}

# Main command handler
cmd_history() {
  local subcommand="${1:-show}"

  # Check for help first
  if [[ "$subcommand" == "-h" ]] || [[ "$subcommand" == "--help" ]]; then
    show_history_help
    return 0
  fi

  # Parse global options
  local args=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --limit)
        LIMIT="$2"
        shift 2
        ;;
      --since)
        SINCE="$2"
        shift 2
        ;;
      --until)
        UNTIL="$2"
        shift 2
        ;;
      --env)
        FILTER_ENV="$2"
        shift 2
        ;;
      --type)
        FILTER_TYPE="$2"
        shift 2
        ;;
      --output)
        OUTPUT_FILE="$2"
        shift 2
        ;;
      --force)
        FORCE=true
        shift
        ;;
      --json)
        JSON_OUTPUT=true
        shift
        ;;
      --csv)
        CSV_OUTPUT=true
        shift
        ;;
      -h | --help)
        show_history_help
        return 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  # Restore positional arguments
  set -- "${args[@]}"
  subcommand="${1:-show}"

  case "$subcommand" in
    show)
      cmd_show
      ;;
    deployments | deploy)
      cmd_deployments
      ;;
    migrations | migrate)
      cmd_migrations
      ;;
    rollbacks | rollback)
      cmd_rollbacks
      ;;
    commands | command)
      cmd_commands
      ;;
    search)
      shift
      cmd_search "$@"
      ;;
    export)
      cmd_export
      ;;
    clear)
      cmd_clear
      ;;
    *)
      log_error "Unknown subcommand: $subcommand"
      show_history_help
      return 1
      ;;
  esac
}

# Export for use as library
export -f cmd_history
export -f record_event

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Help is read-only - bypass init/env guards
  for _arg in "$@"; do
    if [[ "$_arg" == "--help" ]] || [[ "$_arg" == "-h" ]]; then
      show_history_help
      exit 0
    fi
  done
  pre_command "history" || exit $?
  cmd_history "$@"
  exit_code=$?
  post_command "history" $exit_code
  exit $exit_code
fi
