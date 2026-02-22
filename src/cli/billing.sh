#!/usr/bin/env bash
#
# nself billing.sh - Billing Management CLI (DEPRECATED)
#
# WARNING: This command is deprecated. Use 'nself tenant billing' instead.
#
# This file now acts as a wrapper that redirects to 'nself tenant billing'
# with a deprecation warning.
#

set -euo pipefail

# Script directory resolution
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source cli-output for formatting
source "${SCRIPT_DIR}/../lib/utils/cli-output.sh"

# Show deprecation warning
show_deprecation_warning() {
  printf "\n"
  cli_warning "DEPRECATION NOTICE"
  printf "\n"
  printf "  The 'nself billing' command is deprecated.\n"
  printf "  Please use 'nself tenant billing' instead.\n"
  printf "\n"
  cli_info "Redirecting to 'nself tenant billing'..."
  printf "\n"
  sleep 1
}

# Main function - redirect to tenant billing
main() {
  show_deprecation_warning

  # Delegate to tenant billing command (tenant.sh is in same directory)
  exec bash "$(dirname "${BASH_SOURCE[0]}")/tenant.sh" billing "$@"
}

# Execute main function
main "$@"
