#!/usr/bin/env bash
set -euo pipefail

# trust.sh - Trust and install SSL certificates

# Source shared utilities
CLI_SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$CLI_SCRIPT_DIR/../lib/utils/env.sh"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh"
source "$CLI_SCRIPT_DIR/../lib/ssl/trust.sh"
source "$CLI_SCRIPT_DIR/../lib/ssl/ssl.sh"

# Helper functions

# Command function
cmd_trust() {
  local action="${1:-install}"

  if [[ "$action" == "--help" ]] || [[ "$action" == "-h" ]]; then
    show_trust_help
    return 0
  fi

  case "$action" in
  install)
    show_command_header "nself trust" "Install SSL certificates to system trust store"
    
    # Auto-generate certificates if they don't exist
    if [[ -f ".env" ]] || [[ -f ".env.dev" ]]; then
      # Load environment to get BASE_DOMAIN
      set -a
      load_env_with_priority
      set +a
      
      # Check if we're in a project directory
      if [[ -f "docker-compose.yml" ]] || [[ -f ".env" ]]; then
        # Generate certificates if needed
        if ! ssl::generate_for_project "." "${BASE_DOMAIN:-localhost}" 2>/dev/null; then
          log_warning "Could not auto-generate certificates"
        fi
      fi
    fi
    
    trust::install
    ;;
  uninstall | remove)
    show_command_header "nself trust uninstall" "Remove SSL certificates from trust store"
    trust::uninstall_root_ca
    ;;
  status | verify | check)
    show_command_header "nself trust status" "Check certificate trust status"
    trust::status
    ;;
  *)
    log_error "Unknown action: $action"
    show_trust_help
    return 1
    ;;
  esac
}

# Show help
show_trust_help() {
  echo "Usage: nself trust [action]"
  echo
  echo "Manage SSL certificate trust for local development"
  echo
  echo "Actions:"
  echo "  install    Install root CA to system trust store (default)"
  echo "  uninstall  Remove root CA from trust store"
  echo "  status     Check certificate trust status"
  echo
  echo "Examples:"
  echo "  nself trust              # Install root CA"
  echo "  nself trust install      # Install root CA"
  echo "  nself trust status       # Check trust status"
  echo "  nself trust uninstall    # Remove root CA"
  echo
  echo "Note:"
  echo "  - Run 'nself ssl bootstrap' first to generate certificates"
  echo "  - After installing, browsers will trust local certificates"
  echo "  - You may need to restart your browser after installation"
}

# Export for use as library
export -f cmd_trust

# Execute if run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_trust "$@"
fi
