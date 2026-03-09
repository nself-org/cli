#!/usr/bin/env bash
# plugin_suite.sh — Predefined plugin bundle installer for nself (T-0345).
#
# Commands:
#   nself plugins install-suite <name>   Install a predefined bundle in dep order
#   nself plugins list-suites            List all available suites with descriptions
#
# Bash 3.2 compatible — no declare -A, no echo -e, no ${var,,}.

set -o pipefail

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared utilities
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/topological_sort.sh" 2>/dev/null || true

# Fallback display functions if display.sh didn't load
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_warning >/dev/null 2>&1; then
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
fi

# ---------------------------------------------------------------------------
# Suite registry
# ---------------------------------------------------------------------------
# Format: SUITE_PLUGINS_<name>=<space-separated plugins>
#         SUITE_DESC_<name>=<description>
# Bash 3.2 compat — no declare -A.

SUITE_PLUGINS_ai_suite="notify ai mux claw"
SUITE_DESC_ai_suite="AI agent stack — notification routing, AI provider, event mux, and nClaw agent"

SUITE_PLUGINS_voice_suite="notify voice"
SUITE_DESC_voice_suite="Voice communications — notification routing and multi-provider voice/SMS"

SUITE_PLUGINS_automation_suite="cron browser mux"
SUITE_DESC_automation_suite="Automation stack — scheduled jobs, headless browser, and event routing"

# List of all suite names (space-separated)
ALL_SUITES="ai_suite voice_suite automation_suite"

# Display names (mapped via case statement for Bash 3.2)
suite_display_name() {
  local name="$1"
  case "$name" in
    ai_suite)         printf "ai-suite" ;;
    voice_suite)      printf "voice-suite" ;;
    automation_suite) printf "automation-suite" ;;
    *)                printf "%s" "$name" ;;
  esac
}

# Normalise user-supplied suite name: replace hyphens with underscores
normalise_suite_name() {
  printf "%s" "$1" | tr '-' '_'
}

# ---------------------------------------------------------------------------
# plugin_suite_list
#   Prints all available suites with descriptions and plugin members.
# ---------------------------------------------------------------------------
plugin_suite_list() {
  printf "\nAvailable plugin suites:\n\n"
  for suite in $ALL_SUITES; do
    local display
    display=$(suite_display_name "$suite")

    local desc_var="SUITE_DESC_${suite}"
    local desc
    eval "desc=\"\${${desc_var}:-No description}\""

    local plugins_var="SUITE_PLUGINS_${suite}"
    local plugins
    eval "plugins=\"\${${plugins_var}:-}\""

    printf "  \033[1m%s\033[0m\n" "$display"
    printf "    %s\n" "$desc"
    printf "    Plugins: %s\n\n" "$plugins"
  done
}

# ---------------------------------------------------------------------------
# plugin_suite_install <suite-name>
#   Resolves the suite's plugins in dependency order via plugin_topo_sort,
#   prompts for confirmation, then delegates each install to
#   `nself plugin install <name>` (or the plugin core library if available).
# ---------------------------------------------------------------------------
plugin_suite_install() {
  local raw_name="$1"
  local suite
  suite=$(normalise_suite_name "$raw_name")

  # Validate suite exists
  local plugins_var="SUITE_PLUGINS_${suite}"
  local plugins
  eval "plugins=\"\${${plugins_var}:-}\""
  if [ -z "$plugins" ]; then
    log_error "Unknown suite: $raw_name"
    printf "Run 'nself plugins list-suites' to see available suites.\n" >&2
    return 1
  fi

  local desc_var="SUITE_DESC_${suite}"
  local desc
  eval "desc=\"\${${desc_var}:-}\""

  local display
  display=$(suite_display_name "$suite")

  # Resolve install order via topological sort
  local ordered
  if command -v plugin_topo_sort >/dev/null 2>&1; then
    ordered=$(plugin_topo_sort $plugins) || {
      log_error "Failed to resolve dependency order for suite: $display"
      return 1
    }
  else
    # Fallback: use the raw list order if topological_sort.sh not sourced
    ordered=$(printf "%s\n" $plugins)
  fi

  local total
  total=$(printf "%s\n" "$ordered" | wc -l | tr -d ' ')

  # Show suite summary
  printf "\n\033[1m%s\033[0m\n" "$display"
  printf "%s\n\n" "$desc"
  printf "This will install %s plugins in dependency order:\n" "$total"
  local i=1
  for p in $ordered; do
    printf "  %d. nself-%s\n" "$i" "$p"
    i=$((i + 1))
  done
  printf "\n"

  # Prompt for confirmation (skip if --yes flag set)
  if [ "${NSELF_FORCE_YES:-false}" != "true" ]; then
    printf "Continue? [y/N] "
    read -r answer
    case "$answer" in
      [yY]|[yY][eE][sS]) ;;
      *)
        printf "Aborted.\n"
        return 0
        ;;
    esac
  fi

  printf "\n"

  # Install each plugin in resolved order
  local failed=0
  i=1
  for p in $ordered; do
    printf "[%d/%d] Installing nself-%s...\n" "$i" "$total" "$p"

    # Try delegating to the plugin core install function if available,
    # otherwise fall through to the nself CLI command.
    if command -v plugin_install >/dev/null 2>&1; then
      plugin_install "$p" || {
        log_warning "Failed to install nself-$p — continuing with remaining plugins"
        failed=$((failed + 1))
      }
    else
      # Locate nself binary relative to this script
      local nself_bin
      nself_bin="$(cd "$CLI_SCRIPT_DIR/../../.." && pwd)/bin/nself"
      if [ ! -x "$nself_bin" ]; then
        nself_bin="nself"
      fi
      "$nself_bin" plugin install "$p" || {
        log_warning "Failed to install nself-$p — continuing with remaining plugins"
        failed=$((failed + 1))
      }
    fi

    i=$((i + 1))
  done

  printf "\n"
  if [ "$failed" -eq 0 ]; then
    log_success "Suite '$display' installed successfully ($total plugins)."
  else
    log_warning "Suite '$display' installed with $failed failure(s). Check output above."
    return 1
  fi
}

# ---------------------------------------------------------------------------
# plugin_suite_help
# ---------------------------------------------------------------------------
plugin_suite_help() {
  printf "Usage: nself plugins install-suite <suite-name>\n"
  printf "       nself plugins list-suites\n"
  printf "\n"
  printf "Commands:\n"
  printf "  install-suite <name>  Install all plugins in a predefined suite\n"
  printf "  list-suites           List all available suites with descriptions\n"
  printf "\n"
  printf "Available suites:\n"
  for suite in $ALL_SUITES; do
    printf "  %-20s  " "$(suite_display_name "$suite")"
    local desc_var="SUITE_DESC_${suite}"
    local desc
    eval "desc=\"\${${desc_var}:-}\""
    printf "%s\n" "$desc"
  done
  printf "\n"
  printf "Flags:\n"
  printf "  -y, --yes             Skip confirmation prompt\n"
  printf "  -h, --help            Show this help\n"
}

# ---------------------------------------------------------------------------
# Entry point — only runs if script is executed directly (not sourced)
# ---------------------------------------------------------------------------
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  cmd="${1:-}"
  shift 2>/dev/null || true

  case "$cmd" in
    install-suite)
      suite_name="${1:-}"
      shift 2>/dev/null || true
      # Parse flags
      for arg in "$@"; do
        case "$arg" in
          -y|--yes) NSELF_FORCE_YES=true ;;
        esac
      done
      if [ -z "$suite_name" ]; then
        log_error "Suite name required."
        plugin_suite_help
        exit 1
      fi
      plugin_suite_install "$suite_name"
      ;;
    list-suites)
      plugin_suite_list
      ;;
    -h|--help|help)
      plugin_suite_help
      ;;
    "")
      plugin_suite_help
      exit 1
      ;;
    *)
      log_error "Unknown subcommand: $cmd"
      plugin_suite_help
      exit 1
      ;;
  esac
fi

export -f plugin_suite_install
export -f plugin_suite_list
export -f plugin_suite_help
