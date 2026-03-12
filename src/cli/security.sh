#!/usr/bin/env bash
# security.sh - DEPRECATED - Wrapper for 'nself auth security'
# This command has been consolidated into 'nself auth security'
# This wrapper provides backward compatibility

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Handle --help before delegation to avoid deprecation warning
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" || "${1:-}" == "help" ]]; then
  printf "nself security - Security scanning and auditing\n\n"
  printf "NOTE: This command is deprecated. Use 'nself auth security' instead.\n\n"
  printf "USAGE:\n  nself security <subcommand>\n\n"
  printf "SUBCOMMANDS:\n"
  printf "  scan    Vulnerability scan\n"
  printf "  audit   Security audit\n"
  printf "  report  Generate security report\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  WARNING: 'nself security' is deprecated. Use 'nself auth security' instead.\n" >&2
printf "   This compatibility wrapper will be removed in v1.0.0\n\n" >&2

# Delegate to new auth command
exec bash "$SCRIPT_DIR/auth.sh" security "$@"
