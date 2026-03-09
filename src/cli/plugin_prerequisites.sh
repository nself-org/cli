#!/usr/bin/env bash
# plugin_prerequisites.sh - nself plugin prerequisites <name>
# Check all prerequisites for a plugin before install
# Also handles plugin dependency chain (auto-install on y)
# Dispatched from plugin.sh as: cmd_prerequisites "$@"
# Bash 3.2+ compatible

set -euo pipefail

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/core.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/registry.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/licensing.sh" 2>/dev/null || true

if ! declare -f log_info >/dev/null 2>&1; then
  log_info()    { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
  log_error()   { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi

PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"

# ============================================================================
# PREREQUISITE CHECKS
# ============================================================================

# Check 1: Docker is running
_check_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    printf "  \033[0;31m✗\033[0m Docker is not installed\n"
    printf "    Install: https://docs.docker.com/get-docker/\n"
    return 1
  fi

  if ! docker info >/dev/null 2>&1; then
    printf "  \033[0;31m✗\033[0m Docker daemon is not running\n"
    printf "    Start Docker and retry\n"
    return 1
  fi

  printf "  \033[0;32m✓\033[0m Docker is running\n"
  return 0
}

# Check 2: Sufficient RAM
_check_ram() {
  local min_mb="${1:-256}"
  local available_mb=0

  if [[ -f /proc/meminfo ]]; then
    local mem_kb
    mem_kb=$(grep MemAvailable /proc/meminfo | awk '{print $2}' 2>/dev/null || true)
    if [[ -n "$mem_kb" ]]; then
      available_mb=$((mem_kb / 1024))
    fi
  elif command -v vm_stat >/dev/null 2>&1; then
    # macOS
    local free_pages
    free_pages=$(vm_stat | grep "Pages free" | awk '{print $3}' | tr -d '.' 2>/dev/null || true)
    if [[ -n "$free_pages" ]]; then
      # 4096 bytes per page
      available_mb=$(((free_pages * 4096) / 1048576))
    fi
  fi

  if [[ $available_mb -eq 0 ]]; then
    printf "  \033[0;33m~\033[0m RAM check skipped (cannot determine available memory)\n"
    return 0
  fi

  if [[ $available_mb -lt $min_mb ]]; then
    printf "  \033[0;31m✗\033[0m Insufficient RAM: %dMB available, %dMB required\n" "$available_mb" "$min_mb"
    return 1
  fi

  printf "  \033[0;32m✓\033[0m RAM: %dMB available (min %dMB required)\n" "$available_mb" "$min_mb"
  return 0
}

# Check 3: Required env vars present
_check_required_env_vars() {
  local manifest="$1"
  local missing=0
  local required_vars=""

  if command -v jq >/dev/null 2>&1; then
    required_vars=$(jq -r '.env.required // {} | keys[]' "$manifest" 2>/dev/null || \
      jq -r '.env_vars[]? | select(.required == true) | .name' "$manifest" 2>/dev/null || true)
  else
    required_vars=$(grep -A2 '"required"' "$manifest" 2>/dev/null | \
      grep -o '"[A-Z_][A-Z0-9_]*"' | tr -d '"' || true)
  fi

  if [[ -z "${required_vars:-}" ]]; then
    printf "  \033[0;32m✓\033[0m No required environment variables\n"
    return 0
  fi

  # Also check config.env for plugin-specific vars
  local plugin_name
  plugin_name=$(basename "$(dirname "$manifest")" 2>/dev/null || true)
  local config_env="${PLUGIN_DIR}/${plugin_name}/config.env"

  while IFS= read -r var; do
    [[ -z "$var" ]] && continue
    local val="${!var:-}"

    # Also check config.env
    if [[ -z "$val" ]] && [[ -f "$config_env" ]]; then
      val=$(grep "^${var}=" "$config_env" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'" || true)
    fi
    # Check .env.local
    if [[ -z "$val" ]] && [[ -f ".env.local" ]]; then
      val=$(grep "^${var}=" ".env.local" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'" || true)
    fi
    # Check .env
    if [[ -z "$val" ]] && [[ -f ".env" ]]; then
      val=$(grep "^${var}=" ".env" 2>/dev/null | tail -1 | cut -d'=' -f2- | tr -d '"' | tr -d "'" || true)
    fi

    if [[ -z "$val" ]]; then
      printf "  \033[0;31m✗\033[0m Missing required variable: %s\n" "$var"
      printf "    Set with: nself plugin config %s set %s <value>\n" "$plugin_name" "$var"
      missing=$((missing + 1))
    else
      printf "  \033[0;32m✓\033[0m %s is set\n" "$var"
    fi
  done <<EOF
$required_vars
EOF

  return $missing
}

# Check 4: Network connectivity to required external services
_check_network() {
  local manifest="$1"
  local failed=0

  local endpoints=""
  if command -v jq >/dev/null 2>&1; then
    endpoints=$(jq -r '.prerequisites.network // [] | .[]' "$manifest" 2>/dev/null || true)
  fi

  if [[ -z "${endpoints:-}" ]]; then
    # Common checks based on plugin name
    local plugin_name
    plugin_name=$(basename "$(dirname "$manifest")" 2>/dev/null || true)
    case "$plugin_name" in
      ai)       endpoints="https://api.openai.com" ;;
      livekit)  endpoints="https://livekit.io" ;;
      stripe)   endpoints="https://api.stripe.com" ;;
      shopify)  endpoints="https://shopify.com" ;;
    esac
  fi

  if [[ -z "${endpoints:-}" ]]; then
    return 0
  fi

  while IFS= read -r endpoint; do
    [[ -z "$endpoint" ]] && continue
    if command -v curl >/dev/null 2>&1; then
      if curl -s --max-time 5 -o /dev/null -w "%{http_code}" "$endpoint" >/dev/null 2>&1; then
        printf "  \033[0;32m✓\033[0m Network: %s reachable\n" "$endpoint"
      else
        printf "  \033[0;33m~\033[0m Network: %s may not be reachable (check connectivity)\n" "$endpoint"
        # Not a hard failure — network may be fine during runtime
      fi
    fi
  done <<EOF
$endpoints
EOF

  return 0
}

# Check 5: Port availability
_check_port() {
  local port="$1"
  local service="${2:-plugin}"

  if [[ -z "$port" ]] || [[ "$port" -eq 0 ]]; then
    return 0
  fi

  local in_use=false
  if command -v ss >/dev/null 2>&1; then
    if ss -tlnp 2>/dev/null | grep -q ":${port}[[:space:]]"; then
      in_use=true
    fi
  elif command -v netstat >/dev/null 2>&1; then
    if netstat -tlnp 2>/dev/null | grep -q ":${port}[[:space:]]"; then
      in_use=true
    fi
  elif command -v lsof >/dev/null 2>&1; then
    # macOS
    if lsof -i ":${port}" -sTCP:LISTEN 2>/dev/null | grep -q .; then
      in_use=true
    fi
  fi

  if [[ "$in_use" == "true" ]]; then
    printf "  \033[0;31m✗\033[0m Port %d is already in use\n" "$port"
    printf "    Set a different port in .env (e.g. PLUGIN_%s_PORT=%d)\n" \
      "$(printf '%s' "$service" | tr '[:lower:]' '[:upper:]')" "$((port + 1))"
    return 1
  fi

  printf "  \033[0;32m✓\033[0m Port %d is available\n" "$port"
  return 0
}

# Check 6: Plugin-to-plugin dependencies
_check_plugin_dependencies() {
  local plugin_name="$1"
  local manifest="$2"
  local auto_install="${3:-false}"
  local missing=0

  local deps=""
  if command -v jq >/dev/null 2>&1; then
    deps=$(jq -r '.dependencies // [] | .[]' "$manifest" 2>/dev/null || \
      jq -r '.requires // [] | .[]' "$manifest" 2>/dev/null || true)
  fi

  if [[ -z "${deps:-}" ]]; then
    return 0
  fi

  printf "\n  \033[1mPlugin dependencies:\033[0m\n"

  while IFS= read -r dep; do
    [[ -z "$dep" ]] && continue
    local dep_installed=false
    if declare -f is_plugin_installed >/dev/null 2>&1 && is_plugin_installed "$dep"; then
      dep_installed=true
    fi

    if [[ "$dep_installed" == "true" ]]; then
      printf "  \033[0;32m✓\033[0m %s is installed\n" "$dep"
    else
      printf "  \033[0;31m✗\033[0m %s is not installed\n" "$dep"
      if [[ "$auto_install" == "true" ]]; then
        printf "    Auto-installing %s...\n" "$dep"
        if bash "$CLI_SCRIPT_DIR/plugin.sh" install "$dep" 2>/dev/null; then
          printf "    \033[0;32m✓\033[0m %s installed\n" "$dep"
        else
          printf "    \033[0;31m✗\033[0m Failed to install %s\n" "$dep" >&2
          missing=$((missing + 1))
        fi
      else
        printf "    Install with: nself plugin install %s\n" "$dep"
        missing=$((missing + 1))
      fi
    fi
  done <<EOF
$deps
EOF

  return $missing
}

# ============================================================================
# MAIN COMMAND
# ============================================================================

cmd_prerequisites() {
  local plugin_name=""
  local auto_install=false
  local quiet=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --auto-install|-y)
        auto_install=true
        shift
        ;;
      --quiet|-q)
        quiet=true
        shift
        ;;
      --help|-h)
        printf "Usage: nself plugin prerequisites <name> [--auto-install]\n\n"
        printf "Check all prerequisites for a plugin before installing:\n"
        printf "  - Docker running\n"
        printf "  - Sufficient RAM\n"
        printf "  - Required environment variables set\n"
        printf "  - Port availability\n"
        printf "  - Plugin dependencies installed\n"
        printf "  - Network connectivity\n\n"
        printf "Options:\n"
        printf "  --auto-install, -y   Auto-install missing plugin dependencies\n"
        printf "  --quiet, -q          Only output failures\n"
        return 0
        ;;
      -*)
        log_error "Unknown option: $1"
        return 1
        ;;
      *)
        plugin_name="$1"
        shift
        ;;
    esac
  done

  if [[ -z "$plugin_name" ]]; then
    log_error "Plugin name required"
    printf "\nUsage: nself plugin prerequisites <name>\n"
    return 1
  fi

  printf "\n\033[1mPrerequisite check: %s\033[0m\n\n" "$plugin_name"

  local total_failures=0

  # Check Docker
  printf "  \033[1mDocker\033[0m\n"
  _check_docker || total_failures=$((total_failures + 1))

  # Check RAM (default 256MB, override from manifest)
  printf "\n  \033[1mSystem Resources\033[0m\n"
  local min_ram=256
  local plugin_manifest="${PLUGIN_DIR}/${plugin_name}/plugin.json"
  if [[ -f "$plugin_manifest" ]] && command -v jq >/dev/null 2>&1; then
    local manifest_ram
    manifest_ram=$(jq -r '.prerequisites.min_ram_mb // 256' "$plugin_manifest" 2>/dev/null || true)
    [[ -n "$manifest_ram" ]] && [[ "$manifest_ram" =~ ^[0-9]+$ ]] && min_ram="$manifest_ram"
  fi
  _check_ram "$min_ram" || total_failures=$((total_failures + 1))

  # Check port (if manifest specifies a default port)
  if [[ -f "$plugin_manifest" ]]; then
    local default_port=0
    if command -v jq >/dev/null 2>&1; then
      default_port=$(jq -r '.port // 0' "$plugin_manifest" 2>/dev/null || true)
    else
      default_port=$(grep '"port"' "$plugin_manifest" | head -1 | grep -o '[0-9]*' | head -1 || true)
    fi
    if [[ -n "$default_port" ]] && [[ "$default_port" =~ ^[0-9]+$ ]] && [[ "$default_port" -gt 0 ]]; then
      printf "\n  \033[1mPort Availability\033[0m\n"
      _check_port "$default_port" "$plugin_name" || total_failures=$((total_failures + 1))
    fi
  fi

  # Check env vars
  if [[ -f "$plugin_manifest" ]]; then
    printf "\n  \033[1mEnvironment Variables\033[0m\n"
    local env_failures=0
    _check_required_env_vars "$plugin_manifest" || env_failures=$?
    total_failures=$((total_failures + env_failures))
  fi

  # Check plugin dependencies
  if [[ -f "$plugin_manifest" ]]; then
    local dep_failures=0
    if [[ "$auto_install" == "true" ]]; then
      _check_plugin_dependencies "$plugin_name" "$plugin_manifest" "true" || dep_failures=$?
    else
      _check_plugin_dependencies "$plugin_name" "$plugin_manifest" "false" || dep_failures=$?
    fi
    total_failures=$((total_failures + dep_failures))
  fi

  # Check network
  if [[ -f "$plugin_manifest" ]]; then
    printf "\n  \033[1mNetwork Connectivity\033[0m\n"
    _check_network "$plugin_manifest"
  fi

  printf "\n"
  if [[ $total_failures -eq 0 ]]; then
    log_success "All prerequisites met for '$plugin_name'"
    printf "  Install with: nself plugin install %s\n\n" "$plugin_name"
    return 0
  else
    log_error "%d prerequisite(s) not met for '%s'" "$total_failures" "$plugin_name"
    printf "\n  Resolve the issues above and retry:\n"
    printf "    nself plugin prerequisites %s\n\n" "$plugin_name"
    return 1
  fi
}

# ── Standalone invocation ────────────────────────────────────────────────────
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_prerequisites "$@"
fi
