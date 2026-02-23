#!/usr/bin/env bash
# security.sh.backup - Security command implementation
# Part of nself auth security subcommands

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NSELF_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

source "$NSELF_ROOT/src/lib/utils/cli-output.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/config/constants.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/security/comprehensive-scanner.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/security/audit.sh" 2>/dev/null || true
source "$NSELF_ROOT/src/lib/security/secrets.sh" 2>/dev/null || true

cmd_security() {
  local action="${1:-scan}"
  shift || true

  case "$action" in
    scan)
      local deep_scan=false
      local env_file=".env"
      [[ "$1" == "--deep" ]] && deep_scan=true
      [[ "$1" == --env=* ]] && env_file="${1#*=}"
      security_scan_comprehensive "$deep_scan" "$env_file"
      ;;
    audit)
      local env_file=".env"
      [[ "$1" == --env=* ]] && env_file="${1#*=}"
      security_audit_comprehensive "$env_file"
      ;;
    report)
      local output_file="security-report-$(date +%Y%m%d-%H%M%S).txt"
      [[ "$1" == --output=* ]] && output_file="${1#*=}"
      security_generate_report "$output_file" "${2:-.env}"
      ;;
    rotate)
      secrets::rotate "$1" "${2:-.env.secrets}"
      ;;
    *)
      printf "Usage: nself auth security {scan|audit|report|rotate}\n"
      exit 1
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_security "$@"
fi
