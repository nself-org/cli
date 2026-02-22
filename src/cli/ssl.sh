#!/usr/bin/env bash
# ssl.sh - DEPRECATED - Wrapper for 'nself auth ssl'
# This command has been consolidated into 'nself auth ssl'
# This wrapper provides backward compatibility

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Intercept help before delegating (auth ssl help exits 1 on unknown action)
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ] || [ "${1:-}" = "help" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself ssl' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself auth ssl\n\n"
  printf "  Run 'nself auth ssl help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  WARNING: 'nself ssl' is deprecated. Use 'nself auth ssl' instead.\n" >&2
printf "   This compatibility wrapper will be removed in v1.0.0\n\n" >&2

# Delegate to new auth command
exec bash "$SCRIPT_DIR/auth.sh" ssl "$@"
