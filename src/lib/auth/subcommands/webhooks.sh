#!/usr/bin/env bash
# webhooks.sh - Webhook management CLI
# Part of nself v0.6.0 - Phase 2

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$SCRIPT_DIR/../lib/webhooks"

[[ -f "$LIB_DIR/core.sh" ]] && source "$LIB_DIR/core.sh"

cmd_webhooks() {
  local subcommand="${1:-help}"
  shift || true

  case "$subcommand" in
    init) webhook_init && printf "✓ Webhook system initialized\n" ;;
    create) cmd_webhooks_create "$@" ;;
    list) cmd_webhooks_list "$@" ;;
    delete) cmd_webhooks_delete "$@" ;;
    test) cmd_webhooks_test "$@" ;;
    help|--help|-h) cmd_webhooks_help ;;
    *)
      echo "ERROR: Unknown command: $subcommand"
      echo "Run 'nself webhooks help' for usage"
      return 1
      ;;
  esac
}

cmd_webhooks_create() {
  local url=""
  local events=()
  local description=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --url) url="$2"; shift 2 ;;
      --event) events+=("$2"); shift 2 ;;
      --description) description="$2"; shift 2 ;;
      *) [[ -z "$url" ]] && url="$1"; shift ;;
    esac
  done

  if [[ -z "$url" ]] || [[ ${#events[@]} -eq 0 ]]; then
    echo "ERROR: URL and at least one event required"
    echo "Usage: nself webhooks create --url <url> --event <event> [--event <event2>...]"
    return 1
  fi

  local events_json
  events_json=$(printf '%s\n' "${events[@]}" | jq -R . | jq -s .)

  local result
  result=$(webhook_create_endpoint "$url" "$events_json" "$description")

  echo "$result" | jq '.'
  printf "\n✓ Webhook endpoint created\n"
  printf "  Save the secret - it won't be shown again!\n"
}

cmd_webhooks_list() {
  local endpoints
  endpoints=$(webhook_list_endpoints)

  if [[ "$endpoints" == "[]" ]]; then
    echo "No webhook endpoints configured"
    return 0
  fi

  echo "$endpoints" | jq -r '["ID", "URL", "EVENTS", "ENABLED"],
    (.[] | [.id, .url, (.events | join(", ")), .enabled]) | @tsv' | column -t
}

cmd_webhooks_delete() {
  local endpoint_id="${1:-}"

  if [[ -z "$endpoint_id" ]]; then
    echo "ERROR: Endpoint ID required"
    return 1
  fi

  webhook_delete_endpoint "$endpoint_id"
  printf "✓ Webhook endpoint deleted\n"
}

cmd_webhooks_test() {
  local event="${1:-user.created}"
  local payload="${2:-{\"test\": true}}"

  webhook_trigger "$event" "$payload"
  printf "✓ Webhook triggered for event: %s\n" "$event"
}

cmd_webhooks_help() {
  cat <<'EOF'
nself webhooks - Webhook management

USAGE:
  nself webhooks <command> [options]

COMMANDS:
  init      Initialize webhook system
  create    Create webhook endpoint
  list      List webhook endpoints
  delete    Delete webhook endpoint
  test      Test webhook delivery

EXAMPLES:
  nself webhooks init
  nself webhooks create --url https://example.com/webhook --event user.created --event user.login
  nself webhooks list
  nself webhooks test user.created '{"user_id": "123"}'
  nself webhooks delete <endpoint-id>

EVENTS:
  user.created, user.updated, user.deleted
  user.login, user.logout
  session.created, session.revoked
  mfa.enabled, mfa.disabled
  role.assigned, role.revoked
EOF
}

export -f cmd_webhooks

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && cmd_webhooks "$@"
