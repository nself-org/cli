#!/usr/bin/env bash
# k8s.sh - DEPRECATED: Use 'nself infra k8s' instead

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself k8s' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself infra k8s\n\n"
  printf "  Run 'nself infra k8s --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself k8s' command is deprecated.\n"
printf "   Please use: \033[1mnself infra k8s\033[0m\n\n"

# Delegate to new command
exec "${SCRIPT_DIR}/infra.sh" k8s "$@"
