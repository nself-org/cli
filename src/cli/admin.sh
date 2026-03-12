#!/usr/bin/env bash
# admin.sh - Admin CLI
# Part of nself v0.7.0

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Only source API functions if not showing help
if [[ "${1:-help}" != "help" ]] && [[ "${1:-help}" != "--help" ]] && [[ "${1:-help}" != "-h" ]]; then
  [[ -f "$SCRIPT_DIR/../lib/admin/api.sh" ]] && source "$SCRIPT_DIR/../lib/admin/api.sh" || true
fi

cmd_admin() {
  case "${1:-help}" in
    stats) admin_stats_overview ;;
    users) admin_users_list "${2:-50}" "${3:-0}" ;;
    activity) admin_activity_recent "${2:-24}" ;;
    security) admin_security_events ;;
    help | --help | -h)
      cat <<'HELP'
nself admin - Admin dashboard API

COMMANDS:
  stats              Get overview statistics
  users [limit]      List users
  activity [hours]   Recent activity
  security           Security events

EXAMPLES:
  nself admin stats
  nself admin users 100
  nself admin activity 48
  nself admin security
HELP
      ;;
    *)
      echo "ERROR: Unknown command" >&2
      return 1
      ;;
  esac
}

export -f cmd_admin
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then cmd_admin "$@"; fi
