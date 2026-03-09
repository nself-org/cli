#!/usr/bin/env bash
# plugin_info.sh - nself plugin info <name>
# Shows plugin description, tier, installed version, required env vars, deps, Docker image
# Dispatched from plugin.sh as: cmd_info "$@"
# Bash 3.2+ compatible

set -euo pipefail

CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/utils/platform-compat.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/core.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/registry.sh" 2>/dev/null || true
source "$CLI_SCRIPT_DIR/../lib/plugin/licensing.sh" 2>/dev/null || true

# Fallback log functions
if ! declare -f log_info >/dev/null 2>&1; then
  log_info()    { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
  log_error()   { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
  log_warning() { printf "\033[0;33m[WARNING]\033[0m %s\n" "$1"; }
fi

PLUGIN_DIR="${NSELF_PLUGIN_DIR:-$HOME/.nself/plugins}"
PLUGIN_REGISTRY_URL="${NSELF_PLUGIN_REGISTRY:-https://plugins.nself.org}"

# ============================================================================
# HELPERS
# ============================================================================

_json_field() {
  local file="$1"
  local field="$2"
  if command -v jq >/dev/null 2>&1; then
    jq -r ".${field} // empty" "$file" 2>/dev/null
  else
    grep "\"${field}\"" "$file" | head -1 \
      | sed 's/.*"[^"]*"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
  fi
}

_json_bool_field() {
  local file="$1"
  local field="$2"
  if command -v jq >/dev/null 2>&1; then
    jq -r ".${field} // false" "$file" 2>/dev/null
  else
    grep "\"${field}\"" "$file" | head -1 \
      | sed 's/.*"[^"]*"[[:space:]]*:[[:space:]]*\([a-z]*\).*/\1/'
  fi
}

_json_array_items() {
  local file="$1"
  local field="$2"
  if command -v jq >/dev/null 2>&1; then
    jq -r ".${field}[]? // empty" "$file" 2>/dev/null
  else
    # Minimal grep extraction for simple string arrays
    sed -n "/\"${field}\"/,/\]/p" "$file" \
      | grep '"' | grep -v "\"${field}\"" \
      | sed 's/.*"\([^"]*\)".*/\1/' | grep -v '^\s*$'
  fi
}

# ============================================================================
# MAIN COMMAND
# ============================================================================

cmd_info() {
  local plugin_name="${1:-}"
  local output_json=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --json)
        output_json=true
        shift
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
    printf "\nUsage: nself plugin info <name>\n"
    printf "       nself plugin info <name> --json\n"
    return 1
  fi

  local plugin_dir="$PLUGIN_DIR/$plugin_name"
  local manifest="$plugin_dir/plugin.json"
  local is_installed=false

  [[ -d "$plugin_dir" ]] && [[ -f "$manifest" ]] && is_installed=true

  # Try registry if not installed
  if [[ "$is_installed" == "false" ]]; then
    # Attempt to show info from remote registry
    local registry
    registry=$(fetch_registry 2>/dev/null || true)
    if [[ -z "$registry" ]]; then
      log_error "Plugin '$plugin_name' is not installed and registry is unreachable"
      return 1
    fi
    # Minimal info from registry JSON
    local reg_desc reg_ver reg_cat
    reg_ver=$(printf '%s' "$registry" | grep -A10 "\"$plugin_name\"" | grep '"version"' | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    reg_desc=$(printf '%s' "$registry" | grep -A10 "\"$plugin_name\"" | grep '"description"' | head -1 | sed 's/.*"description"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    reg_cat=$(printf '%s' "$registry" | grep -A10 "\"$plugin_name\"" | grep '"category"' | head -1 | sed 's/.*"category"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

    if [[ -z "$reg_ver" ]] && [[ -z "$reg_desc" ]]; then
      # Check pro list
      local found_pro=false
      if declare -f license_is_paid_plugin >/dev/null 2>&1 && license_is_paid_plugin "$plugin_name"; then
        found_pro=true
      fi
      if [[ "$found_pro" == "false" ]]; then
        log_error "Plugin '$plugin_name' not found. Run 'nself plugin list' to see available plugins."
        return 1
      fi
      reg_desc="Pro plugin (requires license)"
      reg_ver="1.0.0"
      reg_cat="pro"
    fi

    printf "\n\033[1m%s\033[0m  \033[2m(not installed)\033[0m\n\n" "$plugin_name"
    printf "  Description : %s\n" "${reg_desc:-unknown}"
    printf "  Version     : %s\n" "${reg_ver:-unknown}"
    printf "  Category    : %s\n" "${reg_cat:-unknown}"
    printf "\nInstall with: nself plugin install %s\n\n" "$plugin_name"
    return 0
  fi

  # Read manifest fields
  local description version category tier docker_image github_url health_endpoint
  description=$(_json_field  "$manifest" "description")
  version=$(_json_field      "$manifest" "version")
  category=$(_json_field     "$manifest" "category")
  tier=$(_json_field         "$manifest" "tier")
  docker_image=$(_json_field "$manifest" "docker_image")
  github_url=$(_json_field   "$manifest" "github_url")
  health_endpoint=$(_json_field "$manifest" "health_endpoint")
  local is_commercial
  is_commercial=$(_json_bool_field "$manifest" "isCommercial")

  # Resolve tier display
  local tier_label="FREE"
  if [[ "$is_commercial" == "true" ]] || [[ "$tier" == "pro" ]] || [[ "$tier" == "max" ]]; then
    tier_label="PRO"
  fi

  # Config file values
  local config_file="$plugin_dir/config.env"
  local has_config=false
  [[ -f "$config_file" ]] && has_config=true

  # Required env vars
  local required_vars
  if command -v jq >/dev/null 2>&1; then
    required_vars=$(jq -r '.env.required // [] | to_entries[] | "\(.key)=\(.value.description // "")"' "$manifest" 2>/dev/null || \
      jq -r '(.env_vars // [])[] | .name' "$manifest" 2>/dev/null || true)
  else
    required_vars=$(grep -A1 '"required"' "$manifest" | grep -o '"[A-Z_][A-Z0-9_]*"' | tr -d '"' || true)
  fi

  # Optional env vars
  local optional_vars
  if command -v jq >/dev/null 2>&1; then
    optional_vars=$(jq -r '.env.optional // [] | to_entries[] | "\(.key)=\(.value.description // "")"' "$manifest" 2>/dev/null || true)
  else
    optional_vars=""
  fi

  # Dependencies
  local deps
  if command -v jq >/dev/null 2>&1; then
    deps=$(jq -r '.dependencies // [] | .[]' "$manifest" 2>/dev/null || \
      jq -r '.requires // [] | .[]' "$manifest" 2>/dev/null || true)
  else
    deps=$(_json_array_items "$manifest" "dependencies")
  fi

  if [[ "$output_json" == "true" ]]; then
    printf '{"name":"%s","version":"%s","description":"%s","tier":"%s","category":"%s","installed":true,"health_endpoint":"%s"}\n' \
      "$plugin_name" "${version:-unknown}" "${description:-}" "$tier_label" "${category:-}" "${health_endpoint:-}"
    return 0
  fi

  # ── Pretty print ─────────────────────────────────────────────────────────
  printf "\n"
  printf "\033[1m%s\033[0m" "$plugin_name"
  if [[ "$tier_label" == "PRO" ]]; then
    printf "  \033[0;35m[PRO]\033[0m"
  else
    printf "  \033[0;32m[FREE]\033[0m"
  fi
  printf "  v%s\n" "${version:-unknown}"
  printf "\n"
  [[ -n "${description:-}" ]] && printf "  %s\n\n" "$description"

  # Category
  [[ -n "${category:-}" ]] && printf "  Category     : %s\n" "$category"

  # Docker image
  if [[ -n "${docker_image:-}" ]]; then
    printf "  Docker image : %s\n" "$docker_image"
  fi

  # GitHub link
  if [[ -n "${github_url:-}" ]]; then
    printf "  GitHub       : %s\n" "$github_url"
  fi

  # Health endpoint
  if [[ -n "${health_endpoint:-}" ]]; then
    printf "  Health check : %s\n" "$health_endpoint"
    # Attempt quick health check if installed
    local health_status="unknown"
    if command -v curl >/dev/null 2>&1; then
      local http_code
      http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$health_endpoint" 2>/dev/null || true)
      if [[ "$http_code" == "200" ]]; then
        health_status="\033[0;32mhealthy\033[0m"
      elif [[ -n "$http_code" ]] && [[ "$http_code" != "000" ]]; then
        health_status="\033[0;31munhealthy (HTTP $http_code)\033[0m"
      else
        health_status="\033[0;33mnot reachable\033[0m"
      fi
      printf "  Health status: %b\n" "$health_status"
    fi
  fi

  # Installation path
  printf "  Installed at : %s\n" "$plugin_dir"

  # Required env vars
  printf "\n  \033[1mRequired Environment Variables\033[0m\n"
  if [[ -n "${required_vars:-}" ]]; then
    while IFS= read -r var_line; do
      [[ -z "$var_line" ]] && continue
      local var_name="${var_line%%=*}"
      local var_desc="${var_line#*=}"
      local is_set="  \033[0;31m(not set)\033[0m"
      if [[ -n "${!var_name:-}" ]]; then
        is_set="  \033[0;32m(set)\033[0m"
      elif [[ "$has_config" == "true" ]] && grep -q "^${var_name}=" "$config_file" 2>/dev/null; then
        is_set="  \033[0;32m(set in config.env)\033[0m"
      fi
      printf "    %-40s%b" "$var_name" "$is_set"
      [[ -n "$var_desc" ]] && [[ "$var_desc" != "$var_name" ]] && printf "  # %s" "$var_desc"
      printf "\n"
    done <<EOF
$required_vars
EOF
  else
    printf "    (none)\n"
  fi

  # Optional env vars
  if [[ -n "${optional_vars:-}" ]]; then
    printf "\n  \033[1mOptional Environment Variables\033[0m\n"
    while IFS= read -r var_line; do
      [[ -z "$var_line" ]] && continue
      local var_name="${var_line%%=*}"
      local var_desc="${var_line#*=}"
      printf "    %-40s  # %s\n" "$var_name" "${var_desc:-optional}"
    done <<EOF
$optional_vars
EOF
  fi

  # Dependencies
  if [[ -n "${deps:-}" ]]; then
    printf "\n  \033[1mPlugin Dependencies\033[0m\n"
    while IFS= read -r dep; do
      [[ -z "$dep" ]] && continue
      local dep_status="\033[0;31mnot installed\033[0m"
      if declare -f is_plugin_installed >/dev/null 2>&1 && is_plugin_installed "$dep"; then
        dep_status="\033[0;32minstalled\033[0m"
      fi
      printf "    %-30s  %b\n" "$dep" "$dep_status"
    done <<EOF
$deps
EOF
  fi

  printf "\n"
  printf "  Configure : nself plugin config %s\n" "$plugin_name"
  printf "  Logs      : nself plugin logs %s\n" "$plugin_name"
  printf "\n"
}

# ── Standalone invocation ────────────────────────────────────────────────────
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  cmd_info "$@"
fi
