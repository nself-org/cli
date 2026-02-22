#!/usr/bin/env bash
# search.sh - DEPRECATED: Use 'nself service search' instead
# Supported engines: postgres, meilisearch, typesense, elasticsearch, opensearch, sonic

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail


# Intercept --help before delegating
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  printf "DEPRECATION NOTICE\n\n"
  printf "  'nself search' is deprecated and will be removed in v1.0.0.\n"
  printf "  Please use: nself service search\n\n"
  printf "  Run 'nself service search --help' for full usage information.\n"
  exit 0
fi

# Show deprecation warning
printf "\033[0;33m⚠\033[0m  The 'nself search' command is deprecated.\n"
printf "   Please use: \033[1mnself service search\033[0m\n\n"

# Backward-compatibility stubs — functionality now lives in 'nself service search'
# Uses safe_sed_inline for cross-platform config file editing
search_enable() {
  exec "${SCRIPT_DIR}/service.sh" search enable "$@"
}

search_status() {
  exec "${SCRIPT_DIR}/service.sh" search status "$@"
}

search_test() {
  exec "${SCRIPT_DIR}/service.sh" search test "$@"
}

# Delegate to new command
search_enable "$@"
