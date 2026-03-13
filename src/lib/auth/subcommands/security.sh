#!/usr/bin/env bash
# security.sh - Security command implementation
# Part of nself auth security subcommands

set -euo pipefail

# Compute security subcommand root without clobbering the caller's NSELF_ROOT.
# This file is at src/lib/auth/subcommands/security.sh → go up 4 levels to reach the CLI root.
_SEC_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_SEC_CLI_ROOT="$(cd "$_SEC_SCRIPT_DIR/../../../.." && pwd)"
_SEC_LIB="$_SEC_CLI_ROOT/src/lib"

source "$_SEC_LIB/utils/cli-output.sh" 2>/dev/null || true
source "$_SEC_LIB/utils/display.sh" 2>/dev/null || true
source "$_SEC_LIB/config/constants.sh" 2>/dev/null || true
source "$_SEC_LIB/security/comprehensive-scanner.sh" 2>/dev/null || true
source "$_SEC_LIB/security/audit.sh" 2>/dev/null || true
source "$_SEC_LIB/security/secrets.sh" 2>/dev/null || true

unset _SEC_SCRIPT_DIR _SEC_CLI_ROOT _SEC_LIB

cmd_security() {
  local action="${1:-scan}"
  shift || true

  # Handle --help for any subcommand
  for _sarg in "$@"; do
    if [[ "$_sarg" == "--help" || "$_sarg" == "-h" ]]; then
      printf "Usage: nself auth security {scan|audit|report|rotate} [options]\n\n"
      printf "Options:\n"
      printf "  --no-docker    Run without Docker connectivity checks\n"
      printf "  --format json  Output results as JSON\n"
      printf "  --env=FILE     Use specific .env file (default: .env)\n"
      printf "  --help, -h     Show this help\n"
      return 0
    fi
  done
  unset _sarg

  case "$action" in
    --help|-h)
      printf "Usage: nself auth security {scan|audit|report|rotate} [options]\n\n"
      printf "Subcommands:\n"
      printf "  scan    Vulnerability scan\n"
      printf "  audit   Security audit\n"
      printf "  report  Generate security report\n"
      printf "  rotate  Rotate secrets\n"
      return 0
      ;;
    scan)
      local deep_scan=false
      local env_file=".env"
      [[ "${1:-}" == "--deep" ]] && deep_scan=true
      [[ "${1:-}" == --env=* ]] && env_file="${1#*=}"
      security_scan_comprehensive "$deep_scan" "$env_file"
      ;;
    audit)
      local env_file=".env"
      local no_docker=false
      local format=""
      local _prev=""
      for _aarg in "$@"; do
        case "$_aarg" in
          --no-docker) no_docker=true ;;
          --format) _prev="format" ;;
          --format=*) format="${_aarg#*=}" ;;
          json) [[ "$_prev" == "format" ]] && format="json" ;;
          --env=*) env_file="${_aarg#*=}" ;;
        esac
        [[ "$_aarg" != "--format" ]] && _prev=""
      done
      unset _aarg _prev
      security_audit_comprehensive "$env_file" "$no_docker" "$format"
      ;;
    report)
      local output_file="security-report-$(date +%Y%m%d-%H%M%S).txt"
      [[ "${1:-}" == --output=* ]] && output_file="${1#*=}"
      security_generate_report "$output_file" "${2:-.env}"
      ;;
    rotate)
      secrets::rotate "${1:-}" "${2:-.env.secrets}"
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
