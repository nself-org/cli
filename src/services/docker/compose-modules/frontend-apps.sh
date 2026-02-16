#!/usr/bin/env bash
set -euo pipefail
# frontend-apps.sh - Frontend apps configuration (for nginx routing only)
# NOTE: Frontend apps run EXTERNALLY - this module does not create containers
# Frontend configuration is used only for nginx proxy routing and API setup

# Generate a single frontend app service
generate_frontend_app() {
  # Frontend apps are external - no Docker containers needed
  # This function returns nothing as frontends run outside nself
  return 0
}

# Main function to generate all frontend app services
generate_frontend_apps() {
  # Frontend apps are external - no Docker containers needed
  # Nginx will proxy to external frontend apps based on configuration
  return 0
}

# Export functions
export -f generate_frontend_app
export -f generate_frontend_apps