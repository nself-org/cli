#!/usr/bin/env bash
#
# nself docs - Open online documentation
#

set -euo pipefail

# Load utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/utils/display.sh"

# Documentation URLs
WIKI_URL="https://github.com/nself-org/cli/wiki"
REPO_URL="https://github.com/nself-org/cli"

show_help() {
  cat <<EOF
Usage: nself docs [options] [section]

Open nself documentation in your browser or display doc URLs.

OPTIONS:
  -h, --help          Show this help message
  -u, --url-only      Print URL instead of opening browser
  -w, --wiki          Open wiki home (default)
  -r, --readme        Open repository README
  -g, --guides        Open guides section
  -a, --api           Open API reference
  -c, --commands      Open commands reference

SECTIONS:
  quick-start         Quick start guide
  configuration       Configuration guide
  deployment          Deployment guide
  troubleshooting     Troubleshooting guide
  faq                 Frequently asked questions
  releases            Release notes
  roadmap             Project roadmap

EXAMPLES:
  nself docs                    # Open wiki in browser
  nself docs quick-start        # Open quick start guide
  nself docs --url-only         # Print wiki URL
  nself docs -g                 # Open guides section
  nself docs configuration      # Open configuration docs

ONLINE DOCUMENTATION:
  Wiki:       ${WIKI_URL}
  Repository: ${REPO_URL}

All documentation is maintained online at GitHub.
Local docs are not installed to keep installations minimal.

EOF
}

# Open URL in browser
open_url() {
  local url="$1"

  if command -v open >/dev/null 2>&1; then
    # macOS
    open "$url"
  elif command -v xdg-open >/dev/null 2>&1; then
    # Linux
    xdg-open "$url"
  elif command -v start >/dev/null 2>&1; then
    # Windows
    start "$url"
  else
    log_warning "Could not detect browser opener command"
    printf "Please open this URL manually:\n\n  %s\n\n" "$url"
    return 1
  fi

  log_success "Opening documentation in browser..."
  return 0
}

# Main function
main() {
  local url="$WIKI_URL"
  local url_only=false

  # Parse arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h | --help)
        show_help
        exit 0
        ;;
      -u | --url-only)
        url_only=true
        shift
        ;;
      -w | --wiki)
        url="$WIKI_URL"
        shift
        ;;
      -r | --readme)
        url="$REPO_URL"
        shift
        ;;
      -g | --guides)
        url="${WIKI_URL}/Guides"
        shift
        ;;
      -a | --api)
        url="${WIKI_URL}/API-Reference"
        shift
        ;;
      -c | --commands)
        url="${WIKI_URL}/Commands"
        shift
        ;;
      quick-start)
        url="${WIKI_URL}/Quick-Start"
        shift
        ;;
      configuration)
        url="${WIKI_URL}/Configuration"
        shift
        ;;
      deployment)
        url="${WIKI_URL}/Deployment"
        shift
        ;;
      troubleshooting)
        url="${WIKI_URL}/Troubleshooting"
        shift
        ;;
      faq)
        url="${WIKI_URL}/FAQ"
        shift
        ;;
      releases)
        url="${REPO_URL}/releases"
        shift
        ;;
      roadmap)
        url="${WIKI_URL}/Roadmap"
        shift
        ;;
      *)
        log_error "Unknown option or section: $1"
        echo ""
        echo "Run 'nself docs --help' for usage information."
        exit 1
        ;;
    esac
  done

  # Print URL or open in browser
  if [[ "$url_only" == "true" ]]; then
    echo "$url"
  else
    open_url "$url"
  fi
}

main "$@"
