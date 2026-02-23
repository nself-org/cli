#!/usr/bin/env bash
# devices.sh - Device management CLI
# Part of nself v0.6.0 - Phase 2

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -f "$SCRIPT_DIR/../lib/auth/device-manager.sh" ]] && source "$SCRIPT_DIR/../lib/auth/device-manager.sh"

cmd_devices() {
  case "${1:-help}" in
    init) device_init && printf "✓ Device management initialized\n" ;;
    list) device_list_user "$2" | jq '.' ;;
    trust) device_trust "$2" && printf "✓ Device trusted\n" ;;
    revoke) device_revoke "$2" && printf "✓ Device revoked\n" ;;
    help|--help|-h)
      cat <<'HELP'
nself devices - Device management

COMMANDS:
  init              Initialize device management
  list <user_id>    List user's devices
  trust <device_id> Trust a device
  revoke <device_id> Revoke device access

EXAMPLES:
  nself devices init
  nself devices list <user-uuid>
  nself devices trust <device-id>
  nself devices revoke <device-id>
HELP
      ;;
    *) echo "ERROR: Unknown command. Run 'nself devices help'" >&2; return 1 ;;
  esac
}

export -f cmd_devices
[[ "${BASH_SOURCE[0]}" == "${0}" ]] && cmd_devices "$@"
