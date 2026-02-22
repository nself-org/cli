#!/usr/bin/env bash
# hasura.sh - Hasura GraphQL management commands
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/utils/display.sh"
source "$SCRIPT_DIR/../lib/utils/cli-output.sh"
source "$SCRIPT_DIR/../lib/utils/env.sh"

cmd_hasura() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    metadata) hasura_metadata "$@" ;;
    console) hasura_console "$@" ;;
    help|--help|-h) hasura_usage ;;
    *) cli_error "Unknown subcommand: $subcommand" && hasura_usage && exit 1 ;;
  esac
}

hasura_metadata() {
  local action="${1:-help}"
  shift || true

  case "$action" in
    apply) metadata_apply "$@" ;;
    export) metadata_export "$@" ;;
    reload) metadata_reload "$@" ;;
    *) cli_error "Unknown action: $action" && exit 1 ;;
  esac
}

metadata_apply() {
  cli_header "nself db hasura metadata apply"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET}"

  [[ -z "$admin_secret" ]] && cli_error "HASURA_GRAPHQL_ADMIN_SECRET not set" && exit 1

  if command -v hasura >/dev/null 2>&1; then
    hasura metadata apply --endpoint "$hasura_url" --admin-secret "$admin_secret"
  else
    cli_warning "Hasura CLI not found"
    cli_info "Install: npm install -g hasura-cli"
    exit 1
  fi
}

metadata_export() {
  cli_header "nself db hasura metadata export"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET}"

  [[ -z "$admin_secret" ]] && cli_error "HASURA_GRAPHQL_ADMIN_SECRET not set" && exit 1

  if command -v hasura >/dev/null 2>&1; then
    hasura metadata export --endpoint "$hasura_url" --admin-secret "$admin_secret"
  else
    cli_warning "Hasura CLI not found"
    cli_info "Install: npm install -g hasura-cli"
    exit 1
  fi
}

metadata_reload() {
  cli_header "nself db hasura metadata reload"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET}"

  curl -s -X POST \
    -H "Content-Type: application/json" \
    -H "X-Hasura-Admin-Secret: $admin_secret" \
    -d '{"type": "reload_metadata", "args": {}}' \
    "$hasura_url/v1/metadata" >/dev/null && cli_success "Metadata reloaded"
}

hasura_console() {
  cli_header "nself db hasura console"
  load_env_with_priority true

  local hasura_url="${HASURA_GRAPHQL_ENDPOINT:-http://localhost:${HASURA_PORT:-8080}}"

  if command -v hasura >/dev/null 2>&1; then
    hasura console --endpoint "$hasura_url" --admin-secret "${HASURA_GRAPHQL_ADMIN_SECRET}"
  else
    cli_info "Open: $hasura_url/console"
  fi
}

hasura_usage() {
  printf "\nUsage: nself db hasura <subcommand>\n\n"
  printf "SUBCOMMANDS:\n"
  printf "  console            Open Hasura Console\n"
  printf "  metadata apply     Apply metadata\n"
  printf "  metadata export    Export metadata\n"
  printf "  metadata reload    Reload metadata cache\n\n"
  printf "NOTE: 'nself hasura' is deprecated. Use 'nself db hasura' instead.\n\n"
}

export -f cmd_hasura

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  printf "\033[0;33m⚠\033[0m  WARNING: 'nself hasura' is deprecated. Use 'nself db hasura' instead.\n" >&2
  printf "   This compatibility wrapper will be removed in v1.0.0\n\n" >&2
  cmd_hasura "$@"
fi
